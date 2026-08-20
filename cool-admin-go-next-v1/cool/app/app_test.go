package app_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gsession"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	coolTask "github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestAuthManagerFactoryUsesConfig(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  auth:
    jwtSecret: "test-secret"
    tokenExpire: 11
    refreshExpire: 22`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	manager := app.DefaultAuthManagerFactory(context.Background())
	if string(manager.Secret) != "test-secret" {
		t.Fatalf("unexpected secret: %s", string(manager.Secret))
	}
	if manager.Expire != 11 || manager.RefreshExpire != 22 {
		t.Fatalf("unexpected expires: %#v", manager)
	}
}

func TestAuthManagerFactoryUsesNodeRefreshExpireDefault(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent("")
	if err != nil {
		t.Fatalf("create empty config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	manager := app.DefaultAuthManagerFactory(context.Background())
	if manager.Expire != 7200 || manager.RefreshExpire != 1296000 {
		t.Fatalf("unexpected default expires: %#v", manager)
	}
}

func TestBuildValidatesTenantConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
	}{
		{
			name:    "missing tenant config defaults enabled and required",
			content: "",
		},
		{
			name: "missing enable defaults true",
			content: `cool:
  tenant:
    requireEnabled: true`,
		},
		{
			name: "explicit disabled conflicts with required default",
			content: `cool:
  tenant:
    enable: false`,
			wantError: true,
		},
		{
			name: "explicit disabled compatibility mode",
			content: `cool:
  tenant:
    enable: false
    requireEnabled: false`,
		},
		{
			name: "explicit enabled and required",
			content: `cool:
  tenant:
    enable: true
    requireEnabled: true`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configContent := `database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
` + test.content
			adapter, err := gcfg.NewAdapterContent(configContent)
			if err != nil {
				t.Fatalf("create config adapter failed: %v", err)
			}
			config := g.Cfg()
			previousAdapter := config.GetAdapter()
			config.SetAdapter(adapter)
			t.Cleanup(func() {
				config.SetAdapter(previousAdapter)
			})

			_, err = app.BuildWithContext(context.Background(), app.Options{
				UploadDir:          t.TempDir(),
				AuthManagerFactory: testAuthManagerFactory,
			})
			if !test.wantError {
				if err != nil {
					t.Fatalf("build application failed: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected tenant configuration validation error")
			}
			if !strings.Contains(err.Error(), "cool.tenant.enable") || !strings.Contains(err.Error(), "cool.tenant.requireEnabled") {
				t.Fatalf("unexpected tenant configuration error: %v", err)
			}
		})
	}
}

func TestBuildPropagatesDisabledTenantScope(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  tenant:
    enable: false
    requireEnabled: false`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	var resolved tenant.Scope
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()
	application, err := app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareReplaceModules,
			Definitions: []middleware.Definition{{
				Name:  "test.tenant-scope",
				Order: 250,
				Handler: func(r *ghttp.Request) {
					resolved = tenant.Resolve(r.Context())
					r.Middleware.Next()
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("build application failed: %v", err)
	}
	if application.TenantEnabled() {
		t.Fatal("expected tenant enforcement disabled")
	}
	if err = server.Start(); err != nil {
		t.Fatalf("start app test server failed: %v", err)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected health status: %d", recorder.Code)
	}
	if resolved.Kind() != tenant.KindBypass {
		t.Fatalf("expected disabled tenant scope to bypass enforcement, got %d", resolved.Kind())
	}
}

func TestNewRegistersAllModules(t *testing.T) {
	useModuleTestConfig(t)
	application := app.New(app.Options{
		StartServer: false,
		UploadDir:   t.TempDir(),
		Specs: modules.Specs(),
	})

	mods := application.Modules()
	if len(mods) != 4 {
		t.Fatalf("expected 4 modules, got %d", len(mods))
	}
	keys := map[string]bool{}
	for _, mod := range mods {
		keys[mod.Key()] = true
	}
	for _, key := range []string{"base", "dict", "task", "recycle"} {
		if !keys[key] {
			t.Fatalf("expected module %s registered", key)
		}
	}
}

func TestNewCollectsAllControllerMetadata(t *testing.T) {
	useModuleTestConfig(t)
	application := app.New(app.Options{
		StartServer: false,
		UploadDir:   t.TempDir(),
		Specs: modules.Specs(),
	})

	controllers := module.CollectControllers(application.Modules())
	if len(controllers) != 13 {
		t.Fatalf("expected 13 controllers, got %d", len(controllers))
	}

	seen := map[string]bool{}
	for _, definition := range controllers {
		seen[definition.Prefix] = true
	}
	for _, prefix := range []string{
		"/admin/base/sys/user",
		"/admin/base/sys/role",
		"/admin/base/sys/menu",
		"/admin/base/sys/department",
		"/admin/base/sys/param",
		"/admin/base/sys/log",
		"/admin/base/open",
		"/admin/base/comm",
		"/app/base/comm",
		"/admin/dict/info",
		"/admin/dict/type",
		"/admin/task/info",
		"/admin/recycle/data",
	} {
		if !seen[prefix] {
			t.Fatalf("expected controller metadata for %s", prefix)
		}
	}
}

func TestBuildUsesExplicitSpecsInsteadOfGlobalRegistry(t *testing.T) {
	useModuleTestConfig(t)
	specs := []module.Spec{{
		Key: "custom",
		Controllers: func(module.Deps) ([]controller.Definition, error) {
			return nil, nil
		},
	}}

	application, err := app.Build(app.Options{
		UploadDir:   t.TempDir(),
		Specs: specs,
	})
	if err != nil {
		t.Fatalf("build with explicit module specs failed: %v", err)
	}
	modules := application.Modules()
	if len(modules) != 1 || modules[0].Key() != "custom" {
		t.Fatalf("explicit module specs were not selected: %#v", modules)
	}
}

func TestBuildReturnsControllerFactoryError(t *testing.T) {
	useModuleTestConfig(t)
	wantErr := errors.New("controller factory failed")
	_, err := app.Build(app.Options{
		UploadDir: t.TempDir(),
		Specs: []module.Spec{{
			Key: "custom",
			Controllers: func(module.Deps) ([]controller.Definition, error) {
				return nil, wantErr
			},
		}},
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "custom") {
		t.Fatalf("controller factory error was not returned with module context: %v", err)
	}
}

func TestBuildWithoutExplicitSpecsHasNoImplicitModules(t *testing.T) {
	useModuleTestConfig(t)
	application, err := app.Build(app.Options{UploadDir: t.TempDir()})
	if err != nil {
		t.Fatalf("build without module specs failed: %v", err)
	}
	if len(application.Modules()) != 0 || len(application.Models()) != 0 {
		t.Fatalf("application loaded implicit modules: modules=%d models=%d", len(application.Modules()), len(application.Models()))
	}
}

func TestBuildCopiesExplicitSpecsBeforeAssembly(t *testing.T) {
	useModuleTestConfig(t)
	var specs []module.Spec
	specs = []module.Spec{
		{
			Key: "first",
			Controllers: func(module.Deps) ([]controller.Definition, error) {
				specs[1].Key = "mutated"
				return nil, nil
			},
		},
		{
			Key: "second",
			Controllers: func(module.Deps) ([]controller.Definition, error) {
				return nil, nil
			},
		},
	}

	application, err := app.Build(app.Options{
		UploadDir:   t.TempDir(),
		Specs: specs,
	})
	if err != nil {
		t.Fatalf("build with explicit module specs failed: %v", err)
	}
	modules := application.Modules()
	if len(modules) != 2 || modules[1].Key() != "second" {
		t.Fatalf("application observed caller mutation: %#v", modules)
	}
}

func TestBuildRejectsInvalidExplicitSpecs(t *testing.T) {
	useModuleTestConfig(t)
	controllerFactory := func(module.Deps) ([]controller.Definition, error) { return nil, nil }
	tests := []struct {
		name      string
		specs     []module.Spec
		wantError string
	}{
		{
			name:      "empty key",
			specs:     []module.Spec{{Controllers: controllerFactory}},
			wantError: "Key",
		},
		{
			name: "duplicate key",
			specs: []module.Spec{
				{Key: "base", Controllers: controllerFactory},
				{Key: "base", Controllers: controllerFactory},
			},
			wantError: "base",
		},
		{
			name:      "nil controller factory",
			specs:     []module.Spec{{Key: "base"}},
			wantError: "Controllers",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := app.Build(app.Options{
				UploadDir:   t.TempDir(),
				Specs: test.specs,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("expected module spec error containing %q, got %v", test.wantError, err)
			}
		})
	}
}

func TestBuildConfiguresEveryModuleBeforeFactories(t *testing.T) {
	useModuleTestConfig(t)
	calls := []string{}
	newSpec := func(key string) module.Spec {
		return module.Spec{
			Key: key,
			Configure: func(context.Context) error {
				calls = append(calls, "configure:"+key)
				return nil
			},
			Runtime: func(module.RuntimeDeps) (module.Runtime, error) {
				calls = append(calls, "runtime:"+key)
				return &runtimeStub{name: key, calls: &calls}, nil
			},
			Controllers: func(module.Deps) ([]controller.Definition, error) {
				calls = append(calls, "controller:"+key)
				return nil, nil
			},
			Middlewares: func(module.MiddlewareDeps) ([]middleware.Definition, error) {
				calls = append(calls, "middleware:"+key)
				return nil, nil
			},
		}
	}
	server := ghttp.GetServer(guid.S())
	defer server.Shutdown()
	_, err := app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		Specs:        []module.Spec{newSpec("first"), newSpec("second")},
	})
	if err != nil {
		t.Fatalf("构建应用失败: %v", err)
	}
	if len(calls) < 2 || calls[0] != "configure:first" || calls[1] != "configure:second" {
		t.Fatalf("组件工厂在全部配置准备前执行: %#v", calls)
	}
	for index, call := range calls {
		if strings.HasPrefix(call, "configure:") || index >= 2 {
			continue
		}
		t.Fatalf("配置准备顺序错误: %#v", calls)
	}
}

func TestBuildStopsBeforeSideEffectsWhenConfigureFails(t *testing.T) {
	useModuleTestConfig(t)
	wantErr := errors.New("配置无效")
	uploadDir := filepath.Join(t.TempDir(), "not-created", "uploads")
	factoryCalls := 0
	schemaCalls := 0
	_, err := app.Build(app.Options{
		UploadDir:      uploadDir,
		AutoSyncSchema: true,
		SchemaSyncRunner: func(context.Context, []entity.Definition) error {
			schemaCalls++
			return nil
		},
		AuthManagerFactory: func(context.Context) *security.Manager {
			factoryCalls++
			return testAuthManagerFactory(context.Background())
		},
		Specs: []module.Spec{{
			Key: "broken",
			Configure: func(context.Context) error {
				return wantErr
			},
			Runtime: func(module.RuntimeDeps) (module.Runtime, error) {
				factoryCalls++
				return nil, nil
			},
			Controllers: func(module.Deps) ([]controller.Definition, error) {
				factoryCalls++
				return nil, nil
			},
			Middlewares: func(module.MiddlewareDeps) ([]middleware.Definition, error) {
				factoryCalls++
				return nil, nil
			},
		}},
	})
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "准备模块配置") || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("配置错误缺少阶段或模块上下文: %v", err)
	}
	if factoryCalls != 0 || schemaCalls != 0 {
		t.Fatalf("配置失败后执行了副作用: factories=%d schema=%d", factoryCalls, schemaCalls)
	}
	if _, statErr := os.Stat(uploadDir); !os.IsNotExist(statErr) {
		t.Fatalf("配置失败后创建了上传目录: %v", statErr)
	}
}

func TestBuildConfiguresModuleWithoutConfigConsumers(t *testing.T) {
	useModuleTestConfig(t)
	configured := false
	_, err := app.Build(app.Options{
		UploadDir: t.TempDir(),
		Specs: []module.Spec{{
			Key: "empty",
			Configure: func(context.Context) error {
				configured = true
				return nil
			},
			Controllers: func(module.Deps) ([]controller.Definition, error) { return nil, nil },
		}},
	})
	if err != nil || !configured {
		t.Fatalf("无配置消费者模块未执行 Configure: configured=%v err=%v", configured, err)
	}
}

func TestBuildSnapshotsTaskHandlersAfterConfigure(t *testing.T) {
	useModuleTestConfig(t)
	handlers := make([]coolTask.HandlerDefinition, 1)
	var received []coolTask.HandlerDefinition
	_, err := app.Build(app.Options{
		UploadDir: t.TempDir(),
		Specs: []module.Spec{{
			Key: "task",
			Configure: func(context.Context) error {
				handlers[0] = coolTask.HandlerDefinition{
					Name: "taskDemoService.test",
					Handler: func(context.Context, coolTask.Invocation) (interface{}, error) {
						return nil, nil
					},
				}
				return nil
			},
			TaskHandlers: handlers,
			Runtime: func(deps module.RuntimeDeps) (module.Runtime, error) {
				received = deps.TaskHandlers
				return &runtimeStub{}, nil
			},
			Controllers: func(module.Deps) ([]controller.Definition, error) { return nil, nil },
		}},
	})
	if err != nil {
		t.Fatalf("构建应用失败: %v", err)
	}
	if len(received) != 1 || received[0].Name != "taskDemoService.test" || received[0].Handler == nil {
		t.Fatalf("Configure 生成的任务处理器未进入应用快照: %#v", received)
	}
	handlers[0] = coolTask.HandlerDefinition{}
	if received[0].Name != "taskDemoService.test" || received[0].Handler == nil {
		t.Fatalf("应用任务处理器快照被调用方修改: %#v", received)
	}
}

func TestBuildCopiesSpecsIncludingConfigure(t *testing.T) {
	useModuleTestConfig(t)
	var specs []module.Spec
	specs = []module.Spec{
		{
			Key: "first",
			Configure: func(context.Context) error {
				specs[1].Key = "mutated"
				return nil
			},
			Controllers: func(module.Deps) ([]controller.Definition, error) { return nil, nil },
		},
		{
			Key:         "second",
			Controllers: func(module.Deps) ([]controller.Definition, error) { return nil, nil },
		},
	}
	application, err := app.Build(app.Options{UploadDir: t.TempDir(), Specs: specs})
	if err != nil {
		t.Fatalf("构建应用失败: %v", err)
	}
	modules := application.Modules()
	if len(modules) != 2 || modules[1].Key() != "second" {
		t.Fatalf("应用观察到调用方切片修改: %#v", modules)
	}
}

func TestHealthMatchesNodeResponse(t *testing.T) {
	application := app.New(app.Options{
		StartServer: false,
		Specs: modules.Specs(),
		UploadDir:   t.TempDir(),
	})

	body := application.Health(context.Background())
	if body.Code != exception.CodeSuccess {
		t.Fatalf("expected success code %d, got %d", exception.CodeSuccess, body.Code)
	}
	if body.Message != "success" {
		t.Fatalf("expected success message, got %q", body.Message)
	}
}

func TestNewCollectsAllModels(t *testing.T) {
	useModuleTestConfig(t)
	application := app.New(app.Options{
		StartServer: false,
		UploadDir:   t.TempDir(),
		Specs: modules.Specs(),
	})

	models := application.Models()
	if len(models) != 16 {
		t.Fatalf("expected 16 models, got %d", len(models))
	}
	tables := map[string]bool{}
	for _, definition := range models {
		tables[definition.TableName] = true
	}
	for _, tableName := range []string{"task_info", "task_log", "recycle_data", "recycle_item"} {
		if !tables[tableName] {
			t.Fatalf("expected model metadata for %s", tableName)
		}
	}
}

func TestSyncSchemaUsesRunner(t *testing.T) {
	called := false
	application := app.New(app.Options{
		StartServer: false,
		Specs: modules.Specs(),
		UploadDir:   t.TempDir(),
		SchemaSyncRunner: func(ctx context.Context, definitions []entity.Definition) error {
			called = true
			if len(definitions) != 16 {
				t.Fatalf("expected 16 definitions, got %d", len(definitions))
			}
			return nil
		},
	})

	if err := application.SyncSchema(context.Background()); err != nil {
		t.Fatalf("sync schema failed: %v", err)
	}
	if !called {
		t.Fatal("expected schema sync runner to be called")
	}
}

func TestRunUsesConfiguredSchemaSyncAndContext(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  schema:
    autoSync: false`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	called := false
	ctx := context.WithValue(context.Background(), struct{}{}, "value")
	application := app.New(app.Options{
		StartServer:       false,
		UploadDir:         t.TempDir(),
		UseConfigAutoSync: true,
		SchemaSyncRunner: func(received context.Context, definitions []entity.Definition) error {
			called = true
			if received != ctx {
				t.Fatalf("expected runner to receive caller context")
			}
			return nil
		},
	})
	if err := application.Run(ctx); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called {
		t.Fatal("expected schema runner not to be called when autoSync is disabled")
	}

	adapter.SetContent(`cool:
  schema:
    autoSync: true`)
	if err := application.Run(ctx); err != nil {
		t.Fatalf("run with auto sync failed: %v", err)
	}
	if !called {
		t.Fatal("expected schema runner to be called when autoSync is enabled")
	}
}

func TestRunUsesExplicitSchemaSyncOptionBeforeConfig(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  schema:
    autoSync: true`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	called := false
	application := app.New(app.Options{
		StartServer:    false,
		UploadDir:      t.TempDir(),
		AutoSyncSchema: false,
		SchemaSyncRunner: func(ctx context.Context, definitions []entity.Definition) error {
			called = true
			return nil
		},
	})
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run with explicit false failed: %v", err)
	}
	if called {
		t.Fatal("expected explicit false to disable schema sync even when config is true")
	}

	adapter.SetContent(`cool:
  schema:
    autoSync: false`)
	application = app.New(app.Options{
		StartServer:    false,
		UploadDir:      t.TempDir(),
		AutoSyncSchema: true,
		SchemaSyncRunner: func(ctx context.Context, definitions []entity.Definition) error {
			called = true
			return nil
		},
	})
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run with explicit true failed: %v", err)
	}
	if !called {
		t.Fatal("expected explicit true to enable schema sync even when config is false")
	}
}

func TestNewWithContextUsesCallerContextForExplicitSchemaSync(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "value")
	called := false
	app.NewWithContext(ctx, app.Options{
		StartServer:    false,
		UploadDir:      t.TempDir(),
		AutoSyncSchema: true,
		SchemaSyncRunner: func(received context.Context, definitions []entity.Definition) error {
			called = true
			if received != ctx {
				t.Fatalf("expected runner to receive NewWithContext context")
			}
			return nil
		},
	})
	if !called {
		t.Fatal("expected explicit true to sync during NewWithContext")
	}
}

func TestNewDefaultsToNoSchemaSync(t *testing.T) {
	called := false
	application := app.New(app.Options{
		StartServer: false,
		Specs: modules.Specs(),
		UploadDir:   t.TempDir(),
		SchemaSyncRunner: func(ctx context.Context, definitions []entity.Definition) error {
			called = true
			return nil
		},
	})
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called {
		t.Fatal("expected New default options not to auto sync schema")
	}
}

func TestRunCallsSchemaBeforeSeed(t *testing.T) {
	calls := []string{}
	application := app.New(app.Options{
		StartServer:    false,
		UploadDir:      t.TempDir(),
		AutoSyncSchema: true,
		AutoInitDB:     true,
		AutoInitMenu:   true,
		SchemaSyncRunner: func(ctx context.Context, definitions []entity.Definition) error {
			calls = append(calls, "schema")
			return nil
		},
		SeedRunner: func(ctx context.Context, modules []module.Module, definitions []entity.Definition, options app.SeedOptions) error {
			calls = append(calls, "seed")
			if !options.InitDB || !options.InitMenu {
				t.Fatalf("expected db and menu seed enabled, got %#v", options)
			}
			return nil
		},
	})

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(calls) != 2 || calls[0] != "schema" || calls[1] != "seed" {
		t.Fatalf("expected schema then seed, got %#v", calls)
	}
}

func TestRunStartsRuntimesAfterSeedAndStopsInReverseOrder(t *testing.T) {
	useModuleTestConfig(t)
	calls := []string{}
	application := app.New(app.Options{
		StartServer:  false,
		UploadDir:    t.TempDir(),
		AutoInitDB:   true,
		AutoInitMenu: true,
		SeedRunner: func(context.Context, []module.Module, []entity.Definition, app.SeedOptions) error {
			calls = append(calls, "seed")
			return nil
		},
		Runtimes: []module.Runtime{
			&runtimeStub{name: "first", calls: &calls},
			&runtimeStub{name: "second", calls: &calls},
		},
	})

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	want := []string{"seed", "start:first", "start:second", "stop:second", "stop:first"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected lifecycle order: %#v", calls)
	}
}

func TestRunRollsBackStartedRuntimesWhenStartupFails(t *testing.T) {
	useModuleTestConfig(t)
	calls := []string{}
	wantErr := errors.New("runtime unavailable")
	application := app.New(app.Options{
		StartServer: false,
		Specs: modules.Specs(),
		UploadDir:   t.TempDir(),
		Runtimes: []module.Runtime{
			&runtimeStub{name: "first", calls: &calls},
			&runtimeStub{name: "second", calls: &calls, startErr: wantErr},
		},
	})

	err := application.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected startup error, got %v", err)
	}
	want := []string{"start:first", "start:second", "stop:first"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected rollback order: %#v", calls)
	}
}

func TestRunSkipsSeedWhenConfigDisabled(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  initDB: false
  initMenu: false
  schema:
    autoSync: false`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	called := false
	application := app.New(app.Options{
		StartServer:       false,
		UploadDir:         t.TempDir(),
		UseConfigAutoSync: true,
		UseConfigInit:     true,
		SeedRunner: func(ctx context.Context, modules []module.Module, definitions []entity.Definition, options app.SeedOptions) error {
			called = true
			return nil
		},
	})

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if called {
		t.Fatal("expected seed runner not to be called")
	}
}

func TestRunUsesExplicitSeedOptionBeforeConfig(t *testing.T) {
	adapter, err := gcfg.NewAdapterContent(`cool:
  initDB: false
  initMenu: false`)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})

	called := false
	application := app.New(app.Options{
		StartServer:  false,
		UploadDir:    t.TempDir(),
		AutoInitDB:   true,
		AutoInitMenu: true,
		SeedRunner: func(ctx context.Context, modules []module.Module, definitions []entity.Definition, options app.SeedOptions) error {
			called = true
			if !options.InitDB || !options.InitMenu {
				t.Fatalf("expected explicit seed options to be enabled, got %#v", options)
			}
			return nil
		},
	})

	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !called {
		t.Fatal("expected explicit seed options to enable seed runner")
	}
}

func TestNewWithServerRegistersPermissionLayer(t *testing.T) {
	application := app.New(app.Options{
		StartServer: true, UploadDir: t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
	})
	if application == nil {
		t.Fatal("expected application")
	}
}

/**
 * 测试应用使用注入的 HTTP 服务注册路由
 * @param t 测试对象
 * @returns null
 */
func TestNewUsesInjectedServer(t *testing.T) {
	server := ghttp.GetServer("app-injected-server-test")
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestNewServesOnlyInjectedUploadDirectory(t *testing.T) {
	uploadDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(uploadDir, "20260721"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "20260721", "proof.txt"), []byte("uploaded"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := ghttp.GetServer("app-upload-static-test")
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	server.SetSessionStorage(gsession.NewStorageMemory())
	defer server.Shutdown()
	app.New(app.Options{
		StartServer: true, Server: server, UploadDir: uploadDir,
		AuthManagerFactory: testAuthManagerFactory,
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/uploads/20260721/proof.txt", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "uploaded" {
		t.Fatalf("unexpected static upload response: %d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" || recorder.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatalf("missing upload isolation headers: %#v", recorder.Header())
	}
}

func testAuthManagerFactory(context.Context) *security.Manager {
	return security.NewManager("0123456789abcdef0123456789abcdef", 7200, 604800)
}

func useModuleTestConfig(t *testing.T) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(`database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
cool:
  tenant:
    enable: false
    requireEnabled: false
  crud:
    softDelete: false`)
	if err != nil {
		t.Fatalf("create module test config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})
}

type runtimeStub struct {
	name     string
	calls    *[]string
	startErr error
	stopErr  error
}

func (s *runtimeStub) Start(context.Context) error {
	*s.calls = append(*s.calls, "start:"+s.name)
	return s.startErr
}

func (s *runtimeStub) Stop(context.Context) error {
	*s.calls = append(*s.calls, "stop:"+s.name)
	return s.stopErr
}
