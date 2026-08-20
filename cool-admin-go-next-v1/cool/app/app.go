package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// schema 同步执行器
type SchemaSyncRunner func(ctx context.Context, definitions []entity.Definition) error

// auth manager 工厂
type AuthManagerFactory func(ctx context.Context) *security.Manager

// 初始化数据导入选项
type SeedOptions struct {
	InitDB   bool
	InitMenu bool
}

// 初始化数据导入执行器
type SeedRunner func(ctx context.Context, modules []module.Module, definitions []entity.Definition, options SeedOptions) error

// 显式模块中间件覆盖语义
type MiddlewareMode string

const (
	MiddlewareAppend         MiddlewareMode = "append"
	MiddlewareReplaceModules MiddlewareMode = "replace-modules"
)

// 模块中间件注入配置；核心错误边界不受覆盖影响
type MiddlewareOverride struct {
	Mode        MiddlewareMode
	Definitions []middleware.Definition
}

// 应用启动选项
type Options struct {
	StartServer        bool
	StartRuntimes      bool
	Server             *ghttp.Server
	UploadDir          string
	Specs        []module.Spec
	AutoSyncSchema     bool
	UseConfigAutoSync  bool
	SchemaSyncRunner   SchemaSyncRunner
	AutoInitDB         bool
	AutoInitMenu       bool
	UseConfigInit      bool
	SeedRunner         SeedRunner
	AuthManagerFactory AuthManagerFactory
	SessionStore       security.SessionStore
	MiddlewareOverride *MiddlewareOverride
	// 已废弃；非 nil 时等价于 replace-modules
	MiddlewareDefinitions []middleware.Definition
	Translator            middleware.Translator
	ErrorLogger           middleware.ErrorLogger
	ErrorRenderer         middleware.ErrorRenderer
	Runtimes              []module.Runtime
}

// cool 应用实例
type Application struct {
	server             *ghttp.Server
	uploadDir          string
	moduleSpecs        []module.Spec
	modules            []module.Module
	models             []entity.Definition
	schemaSyncRunner   SchemaSyncRunner
	seedRunner         SeedRunner
	autoSyncSchema     bool
	useConfigAutoSync  bool
	autoInitDB         bool
	autoInitMenu       bool
	useConfigInit      bool
	authManagerFactory AuthManagerFactory
	authManager        *security.Manager
	sessionStore       security.SessionStore
	sso                bool
	i18nEnabled        bool
	i18nLanguages      []string
	tenantEnabled      bool
	softDeleteEnabled  bool
	authOptions        module.AuthOptions
	i18nOptions        module.I18nOptions
	crudOptions        module.CRUDOptions
	redisDefault       module.RedisDefaultConfig
	middlewareOverride *MiddlewareOverride
	translator         middleware.Translator
	errorLogger        middleware.ErrorLogger
	errorRenderer      middleware.ErrorRenderer
	schemaSynced       bool
	seedInitialized    bool
	runtimes           []module.Runtime
	moduleRuntimes     map[string]module.Runtime
	recycleManager     *recycle.Manager
	runtimesStarted    int
	useInjectedRuntime bool
	runtimesEnabled    bool
}

/**
 * 创建应用实例
 * @param options 启动选项
 * @returns *Application
 */
func New(options Options) *Application {
	return NewWithContext(context.Background(), options)
}

// 创建应用并返回所有启动期配置、路由和中间件编译错误
func Build(options Options) (*Application, error) {
	return BuildWithContext(context.Background(), options)
}

/**
 * 使用上下文创建应用实例
 * @param ctx 上下文
 * @param options 启动选项
 * @returns *Application
 */
func NewWithContext(ctx context.Context, options Options) *Application {
	application, err := BuildWithContext(ctx, options)
	if err != nil {
		// 兼容旧 API；新代码应使用 Build/BuildWithContext 显式处理错误。
		panic(err)
	}
	return application
}

// 使用上下文创建应用并返回启动期错误
func BuildWithContext(ctx context.Context, options Options) (*Application, error) {
	runtimeConfig := loadRuntimeConfig(ctx)
	redisDefault, err := loadRedisDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	if runtimeConfig.isTenantRequired && !runtimeConfig.isTenantEnabled {
		return nil, gerror.New("cool.tenant.enable 不能在 cool.tenant.requireEnabled 为 true 时关闭")
	}
	moduleSpecs, err := selectSpecs(options.Specs)
	if err != nil {
		return nil, err
	}
	for _, spec := range moduleSpecs {
		if spec.Configure == nil {
			continue
		}
		if err = spec.Configure(ctx); err != nil {
			return nil, gerror.Wrapf(err, "准备模块配置 %s 失败", spec.Key)
		}
	}
	moduleSpecs = cloneSpecs(moduleSpecs)
	usesDefaultAuthFactory := options.AuthManagerFactory == nil
	if options.StartServer && usesDefaultAuthFactory {
		jwtSecret, err := bootstrapJWTSecret(defaultConfigPath, runtimeConfig.jwtSecret)
		if err != nil {
			return nil, err
		}
		runtimeConfig.jwtSecret = jwtSecret
	}
	schemaRunner := options.SchemaSyncRunner
	if schemaRunner == nil {
		schemaRunner = defaultSchemaSyncRunner
	}
	seedRunner := options.SeedRunner
	if seedRunner == nil {
		seedRunner = defaultSeedRunner
	}
	authFactory := options.AuthManagerFactory
	if authFactory == nil {
		authFactory = func(context.Context) *security.Manager {
			return security.NewManager(runtimeConfig.jwtSecret, runtimeConfig.tokenExpire, runtimeConfig.refreshExpire)
		}
	}
	authManager := authFactory(ctx)
	if authManager == nil {
		return nil, fmt.Errorf("auth manager 不能为空")
	}
	if options.StartServer && unsafeJWTSecret(string(authManager.Secret)) {
		return nil, fmt.Errorf("cool.auth.jwtSecret 必须是至少 32 字节的非默认密钥")
	}
	sessionStore := options.SessionStore
	sessionStore, err = resolveSessionStore(ctx, sessionStore, defaultRedisSessionClient)
	if err != nil {
		return nil, err
	}
	uploadDir := options.UploadDir
	if uploadDir == "" {
		uploadDir = filepath.Join("resource", "public", "uploads")
	}
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload directory failed: %w", err)
	}

	override := options.MiddlewareOverride
	if override == nil && options.MiddlewareDefinitions != nil {
		override = &MiddlewareOverride{
			Mode:        MiddlewareReplaceModules,
			Definitions: append([]middleware.Definition{}, options.MiddlewareDefinitions...),
		}
	}
	if override != nil {
		cloned := *override
		cloned.Definitions = append([]middleware.Definition{}, override.Definitions...)
		override = &cloned
	}

	application := &Application{
		uploadDir:          uploadDir,
		moduleSpecs:        moduleSpecs,
		schemaSyncRunner:   schemaRunner,
		seedRunner:         seedRunner,
		autoSyncSchema:     options.AutoSyncSchema,
		useConfigAutoSync:  options.UseConfigAutoSync,
		autoInitDB:         options.AutoInitDB,
		autoInitMenu:       options.AutoInitMenu,
		useConfigInit:      options.UseConfigInit,
		authManagerFactory: authFactory,
		authManager:        authManager,
		sessionStore:       sessionStore,
		sso:                runtimeConfig.sso,
		i18nEnabled:        runtimeConfig.i18nEnabled,
		i18nLanguages:      append([]string{}, runtimeConfig.i18nLanguages...),
		tenantEnabled:      runtimeConfig.isTenantEnabled,
		softDeleteEnabled:  runtimeConfig.isSoftDeleteEnabled,
		authOptions:        module.AuthOptions{SSO: runtimeConfig.sso},
		i18nOptions: module.I18nOptions{
			Enabled: runtimeConfig.i18nEnabled, Languages: append([]string{}, runtimeConfig.i18nLanguages...),
		},
		crudOptions:        module.CRUDOptions{SoftDelete: runtimeConfig.isSoftDeleteEnabled},
		redisDefault:       redisDefault,
		middlewareOverride: override,
		translator:         options.Translator,
		errorLogger:        options.ErrorLogger,
		errorRenderer:      options.ErrorRenderer,
		runtimes:           append([]module.Runtime{}, options.Runtimes...),
		moduleRuntimes:     map[string]module.Runtime{},
		useInjectedRuntime: options.Runtimes != nil,
		runtimesEnabled:    options.StartServer || options.StartRuntimes || options.Runtimes != nil,
	}
	if err := application.bindRuntimeControllers(ctx); err != nil {
		return nil, err
	}

	if options.StartServer {
		application.server = options.Server
		if application.server == nil {
			application.server = g.Server()
		}
		if err := application.registerRoutes(ctx); err != nil {
			return nil, err
		}
	}
	if application.AutoSyncSchemaEnabled(ctx) {
		if err := application.SyncSchema(ctx); err != nil {
			return nil, err
		}
		application.schemaSynced = true
	}

	return application, nil
}

func selectSpecs(explicit []module.Spec) ([]module.Spec, error) {
	selected := append([]module.Spec{}, explicit...)
	seen := make(map[string]struct{}, len(selected))
	for _, spec := range selected {
		if strings.TrimSpace(spec.Key) == "" {
			return nil, gerror.New("模块 Key 不能为空")
		}
		if _, exists := seen[spec.Key]; exists {
			return nil, gerror.Newf("模块 Key 重复: %s", spec.Key)
		}
		if spec.Controllers == nil {
			return nil, gerror.Newf("模块 %s Controllers 不能为空", spec.Key)
		}
		seen[spec.Key] = struct{}{}
	}
	return selected, nil
}

func cloneSpecs(specs []module.Spec) []module.Spec {
	cloned := append([]module.Spec{}, specs...)
	for index := range cloned {
		cloned[index].Models = append([]entity.Definition{}, specs[index].Models...)
		cloned[index].TaskHandlers = append([]task.HandlerDefinition{}, specs[index].TaskHandlers...)
	}
	return cloned
}

func unsafeJWTSecret(secret string) bool {
	value := strings.TrimSpace(secret)
	if len(value) < 32 {
		return true
	}
	lowerValue := strings.ToLower(value)
	switch lowerValue {
	case jwtSecretPlaceholder, "cool-admin-go-next-jwt-secret-key", "change-me", "changeme", "replace-me", "your-jwt-secret":
		return true
	}
	return strings.Contains(lowerValue, "change-me") ||
		strings.Contains(lowerValue, "replace-me") ||
		strings.Contains(lowerValue, "your-jwt-secret")
}

/**
 * 启动默认应用
 * @param ctx 上下文
 * @param specs 模块声明
 * @returns error
 */
func Run(ctx context.Context, specs []module.Spec) error {
	application, err := BuildWithContext(ctx, Options{
		StartServer: true, UseConfigAutoSync: true, UseConfigInit: true, Specs: specs,
	})
	if err != nil {
		return err
	}
	return application.Run(ctx)
}

/**
 * 当前模块列表
 * @returns []module.Module
 */
func (a *Application) Modules() []module.Module {
	return a.modules
}

/**
 * 当前模型列表
 * @returns []entity.Definition
 */
func (a *Application) Models() []entity.Definition {
	return append([]entity.Definition{}, a.models...)
}

/**
 * 判断租户隔离是否启用
 * @returns 是否启用
 */
func (a *Application) TenantEnabled() bool {
	return a.tenantEnabled
}

/**
 * 返回应用级回收站协调器
 * @returns *recycle.Manager
 */
func (a *Application) RecycleManager() *recycle.Manager {
	return a.recycleManager
}

/**
 * 同步数据库结构
 * @param ctx 上下文
 * @returns error
 */
func (a *Application) SyncSchema(ctx context.Context) error {
	return a.schemaSyncRunner(ctx, a.Models())
}

/**
 * 初始化 seed 数据
 * @param ctx 上下文
 * @returns error
 */
func (a *Application) InitSeed(ctx context.Context) error {
	options := SeedOptions{
		InitDB:   a.InitDBEnabled(ctx),
		InitMenu: a.InitMenuEnabled(ctx),
	}
	if !options.InitDB && !options.InitMenu {
		return nil
	}
	return a.seedRunner(ctx, a.modules, a.Models(), options)
}

// 创建默认 auth manager
func DefaultAuthManagerFactory(ctx context.Context) *security.Manager {
	config := loadRuntimeConfig(ctx)
	return security.NewManager(
		config.jwtSecret,
		config.tokenExpire,
		config.refreshExpire,
	)
}

type runtimeConfig struct {
	jwtSecret           string
	tokenExpire         int64
	refreshExpire       int64
	sso                 bool
	i18nEnabled         bool
	i18nLanguages       []string
	isTenantEnabled     bool
	isTenantRequired    bool
	isSoftDeleteEnabled bool
}

func loadRuntimeConfig(ctx context.Context) runtimeConfig {
	return runtimeConfig{
		jwtSecret:           g.Cfg().MustGet(ctx, "cool.auth.jwtSecret", "").String(),
		tokenExpire:         g.Cfg().MustGet(ctx, "cool.auth.tokenExpire", 7200).Int64(),
		refreshExpire:       g.Cfg().MustGet(ctx, "cool.auth.refreshExpire", 1296000).Int64(),
		sso:                 g.Cfg().MustGet(ctx, "cool.auth.sso", false).Bool(),
		i18nEnabled:         g.Cfg().MustGet(ctx, "cool.i18n.enable", false).Bool(),
		i18nLanguages:       g.Cfg().MustGet(ctx, "cool.i18n.languages", []string{"zh-cn", "zh-tw", "en"}).Strings(),
		isTenantEnabled:     g.Cfg().MustGet(ctx, "cool.tenant.enable", true).Bool(),
		isTenantRequired:    g.Cfg().MustGet(ctx, "cool.tenant.requireEnabled", true).Bool(),
		isSoftDeleteEnabled: g.Cfg().MustGet(ctx, "cool.crud.softDelete", true).Bool(),
	}
}

func loadRedisDefaultConfig(ctx context.Context) (module.RedisDefaultConfig, error) {
	value, err := g.Cfg().Get(ctx, "redis.default")
	if err != nil {
		return module.RedisDefaultConfig{}, gerror.Wrap(err, "读取 redis.default 配置失败")
	}
	if value == nil {
		return module.RedisDefaultConfig{}, nil
	}
	parsed, err := gredis.ConfigFromMap(value.Map())
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Address) == "" {
		return module.RedisDefaultConfig{}, gerror.New("redis.default 配置无效")
	}
	return module.RedisDefaultConfig{Configured: true, Config: *parsed}, nil
}

/**
 * 默认 schema 同步执行器
 * @param ctx 上下文
 * @param definitions 模型定义列表
 * @returns error
 */
func defaultSchemaSyncRunner(ctx context.Context, definitions []entity.Definition) error {
	_, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions)
	return err
}

/**
 * 默认 seed 导入执行器
 * @param ctx 上下文
 * @param modules 模块列表
 * @param definitions 模型定义列表
 * @param options seed 选项
 * @returns error
 */
func defaultSeedRunner(ctx context.Context, modules []module.Module, definitions []entity.Definition, options SeedOptions) error {
	importer := seed.NewImporter(g.DB(), definitions)
	for _, mod := range modules {
		seeds := mod.ModuleSeeds()
		if options.InitDB && seeds.DBPath != "" {
			if _, err := importer.ImportDB(ctx, mod.Key(), seeds.DBPath); err != nil {
				return err
			}
		}
		if options.InitMenu && seeds.MenuPath != "" {
			if _, err := importer.ImportMenu(ctx, mod.Key(), seeds.MenuPath); err != nil {
				return err
			}
		}
	}
	return nil
}

/**
 * 健康检查
 * @param ctx 上下文
 * @returns response.Body
 */
func (a *Application) Health(ctx context.Context) response.Body {
	return response.OK(map[string]string{
		"status": "ok",
	})
}

/**
 * 运行应用
 * @param ctx 上下文
 * @returns error
 */
func (a *Application) Run(ctx context.Context) error {
	if !a.schemaSynced && a.AutoSyncSchemaEnabled(ctx) {
		if err := a.SyncSchema(ctx); err != nil {
			return err
		}
		a.schemaSynced = true
	}
	if !a.seedInitialized && (a.InitDBEnabled(ctx) || a.InitMenuEnabled(ctx)) {
		if err := a.InitSeed(ctx); err != nil {
			return err
		}
		a.seedInitialized = true
	}
	if err := a.startRuntimes(ctx); err != nil {
		return err
	}
	defer func() {
		if err := a.stopRuntimes(context.Background()); err != nil {
			g.Log().Error(context.Background(), err)
		}
	}()
	if a.server == nil {
		return nil
	}

	a.server.Run()
	return nil
}

// 读取 schema 自动同步开关
func (a *Application) AutoSyncSchemaEnabled(ctx context.Context) bool {
	if a.useConfigAutoSync {
		return g.Cfg().MustGet(ctx, "cool.schema.autoSync", true).Bool()
	}
	return a.autoSyncSchema
}

// 读取 DB seed 初始化开关
func (a *Application) InitDBEnabled(ctx context.Context) bool {
	if a.useConfigInit {
		return g.Cfg().MustGet(ctx, "cool.initDB", true).Bool()
	}
	return a.autoInitDB
}

// 读取 menu seed 初始化开关
func (a *Application) InitMenuEnabled(ctx context.Context) bool {
	if a.useConfigInit {
		return g.Cfg().MustGet(ctx, "cool.initMenu", true).Bool()
	}
	return a.autoInitMenu
}

/**
 * 绑定运行时 controller 元数据
 * @returns null
 */
func (a *Application) bindRuntimeControllers(ctx context.Context) error {
	a.modules = []module.Module{}
	a.models = []entity.Definition{}
	a.moduleRuntimes = map[string]module.Runtime{}
	specs := a.moduleSpecs
	if len(specs) == 0 {
		return nil
	}
	taskHandlers := make([]task.HandlerDefinition, 0)
	for _, spec := range specs {
		taskHandlers = append(taskHandlers, spec.TaskHandlers...)
		a.models = append(a.models, spec.Models...)
	}
	providerIndex, err := recycleProviderIndex(specs, a.softDeleteEnabled)
	if err != nil {
		return err
	}
	if providerIndex >= 0 {
		providerSpec := specs[providerIndex]
		manager, runtime, err := providerSpec.RecycleProvider(module.RuntimeDeps{
			Context: ctx, DB: g.DB(), Models: append([]entity.Definition{}, a.models...),
			TaskHandlers: append([]task.HandlerDefinition{}, taskHandlers...),
			AuthOptions:  a.authOptions, I18nOptions: a.i18nOptions, CRUDOptions: a.crudOptions, RedisDefault: a.redisDefault,
		})
		if err != nil {
			return gerror.Wrapf(err, "构建模块 %s RecycleProvider 失败", providerSpec.Key)
		}
		if manager == nil {
			return gerror.Newf("模块 %s RecycleProvider Manager 不能为空", providerSpec.Key)
		}
		if runtime == nil {
			return gerror.Newf("模块 %s RecycleProvider Runtime 不能为空", providerSpec.Key)
		}
		a.recycleManager = manager
		a.moduleRuntimes[providerSpec.Key] = runtime
		if !a.useInjectedRuntime {
			a.runtimes = append(a.runtimes, runtime)
		}
	}
	for index, spec := range specs {
		if index == providerIndex || spec.Runtime == nil {
			continue
		}
		created, err := spec.Runtime(module.RuntimeDeps{
			Context: ctx, DB: g.DB(), Models: append([]entity.Definition{}, a.models...), Recycle: a.recycleManager,
			TaskHandlers: append([]task.HandlerDefinition{}, taskHandlers...),
			AuthOptions:  a.authOptions, I18nOptions: a.i18nOptions, CRUDOptions: a.crudOptions, RedisDefault: a.redisDefault,
		})
		if err != nil {
			return gerror.Wrapf(err, "构建模块 %s Runtime 失败", spec.Key)
		}
		if created == nil {
			return gerror.Newf("模块 %s Runtime 不能为空", spec.Key)
		}
		a.moduleRuntimes[spec.Key] = created
		if !a.useInjectedRuntime {
			a.runtimes = append(a.runtimes, created)
		}
	}
	baseDeps := module.Deps{
		Context: ctx, DB: g.DB(), AuthManager: a.authManager, SessionStore: a.sessionStore, SSO: a.sso,
		EPSProvider: func() []controller.Definition { return module.CollectControllers(a.modules) },
		UploadDir:   a.uploadDir, UploadDirectory: module.UploadDirectory(a.uploadDir),
		Recycle: a.recycleManager, Models: append([]entity.Definition{}, a.models...),
		AuthOptions: a.authOptions, I18nOptions: a.i18nOptions, CRUDOptions: a.crudOptions, RedisDefault: a.redisDefault,
	}
	for _, spec := range specs {
		deps := baseDeps
		deps.Runtime = a.moduleRuntimes[spec.Key]
		controllers, err := spec.Controllers(deps)
		if err != nil {
			return gerror.Wrapf(err, "构建模块 %s Controllers 失败", spec.Key)
		}
		if err := validateControllerModules(spec, controllers); err != nil {
			return err
		}
		mod := module.New(spec.Key).
			Name(spec.Name).
			Config(module.Config{Description: spec.Description, Order: spec.Order}).
			Models(spec.Models).
			Seeds(spec.DB, spec.Menu).
			Controllers(controllers)
		a.modules = append(a.modules, mod)
	}
	if a.recycleManager != nil {
		a.recycleManager.FreezeRestoreHooks()
	}
	return nil
}

// 校验模块声明返回的 Controller 归属
func validateControllerModules(
	spec module.Spec,
	controllers []controller.Definition,
) error {
	for _, definition := range controllers {
		if definition.Module == spec.Key {
			continue
		}
		controllerName := definition.Name
		if controllerName == "" {
			controllerName = definition.Prefix
		}
		return gerror.Newf(
			"模块 Controller 归属不一致: spec=%s controller=%s prefix=%s module=%s",
			spec.Key,
			controllerName,
			definition.Prefix,
			definition.Module,
		)
	}
	return nil
}

/**
 * 查找应用级唯一回收站提供器
 * @param specs 模块声明
 * @param isRequired 是否必须存在
 * @returns 提供器索引和校验错误
 */
func recycleProviderIndex(specs []module.Spec, isRequired bool) (int, error) {
	providerIndex := -1
	for index, spec := range specs {
		if spec.RecycleProvider == nil {
			continue
		}
		if providerIndex >= 0 {
			return -1, gerror.New("回收站 RecycleProvider 只能注册一个")
		}
		providerIndex = index
	}
	if providerIndex < 0 && isRequired {
		return -1, gerror.New("cool.crud.softDelete 开启时必须注册 RecycleProvider")
	}
	return providerIndex, nil
}

// 按模块顺序启动 Runtime，失败时回滚已启动项
func (a *Application) startRuntimes(ctx context.Context) error {
	if !a.runtimesEnabled {
		return nil
	}
	if a.runtimesStarted > 0 {
		return nil
	}
	for index, runtime := range a.runtimes {
		if runtime == nil {
			_ = a.stopRuntimes(context.Background())
			return gerror.Newf("第 %d 个模块 Runtime 为空", index+1)
		}
		if err := runtime.Start(ctx); err != nil {
			_ = a.stopRuntimes(context.Background())
			return gerror.Wrapf(err, "启动第 %d 个模块 Runtime 失败", index+1)
		}
		a.runtimesStarted++
	}
	return nil
}

// 按启动逆序停止 Runtime
func (a *Application) stopRuntimes(ctx context.Context) error {
	var stopErr error
	for a.runtimesStarted > 0 {
		a.runtimesStarted--
		if err := a.runtimes[a.runtimesStarted].Stop(ctx); err != nil && stopErr == nil {
			stopErr = gerror.Wrapf(err, "停止第 %d 个模块 Runtime 失败", a.runtimesStarted+1)
		}
	}
	return stopErr
}

/**
 * 注册基础路由
 * @returns null
 */
func (a *Application) registerRoutes(ctx context.Context) error {
	controllers := module.CollectControllers(a.modules)
	specs, err := controller.CRUDResourceSpecs(controllers)
	if err != nil {
		return err
	}
	crudRegistry, err := crud.NewRegistry(specs)
	if err != nil {
		return err
	}
	var runtime *crud.Runtime
	if len(specs) > 0 {
		db := g.DB()
		runtime = crud.NewRuntime(db, crudRegistry)
		if a.recycleManager != nil {
			runtime = crud.NewRuntime(db, crudRegistry, a.recycleManager)
		}
	}
	plan, err := controller.CompileRoutePlan(runtime, controllers)
	if err != nil {
		return err
	}
	middlewares, err := a.compileMiddlewares(ctx, controllers)
	if err != nil {
		return err
	}

	// 只有所有 compile/validation 都成功后才修改 server。
	a.server.SetIndexFolder(false)
	a.server.AddStaticPath("/upload", a.uploadDir)
	a.server.AddStaticPath("/uploads", a.uploadDir)
	for _, pattern := range []string{"/upload/*path", "/uploads/*path"} {
		a.server.BindHookHandler(pattern, ghttp.HookBeforeOutput, setUploadSecurityHeaders)
	}
	a.server.BindHandler("/health", func(r *ghttp.Request) {
		r.Response.WriteJson(a.Health(r.Context()))
	})
	if err = middleware.Register(a.server, middlewares.global); err != nil {
		return err
	}
	return plan.BindWithMiddlewares(a.server, middlewares.moduleHandlers())
}

type middlewarePlan struct {
	global  []middleware.Definition
	modules map[string][]middleware.Definition
}

// 编译全局和模块局部中间件计划
func (a *Application) compileMiddlewares(
	ctx context.Context,
	controllers []controller.Definition,
) (*middlewarePlan, error) {
	plan := &middlewarePlan{modules: make(map[string][]middleware.Definition)}
	if a.middlewareOverride != nil &&
		a.middlewareOverride.Mode != MiddlewareAppend &&
		a.middlewareOverride.Mode != MiddlewareReplaceModules {
		return nil, gerror.Newf("未知 middleware override mode: %s", a.middlewareOverride.Mode)
	}
	if a.middlewareOverride == nil || a.middlewareOverride.Mode == MiddlewareAppend {
		for _, spec := range a.moduleSpecs {
			deps := module.MiddlewareDeps{
				Context:       ctx,
				DB:            g.DB(),
				AuthManager:   a.authManager,
				SessionStore:  a.sessionStore,
				SSO:           a.sso,
				I18nEnabled:   a.i18nEnabled,
				I18nLanguages: append([]string{}, a.i18nLanguages...),
				Translator:    a.translator,
				Controllers:   controllers,
				Runtime:       a.moduleRuntimes[spec.Key],
				Recycle:       a.recycleManager,
				Models:        append([]entity.Definition{}, a.models...),
				AuthOptions:   a.authOptions,
				I18nOptions:   a.i18nOptions,
				CRUDOptions:   a.crudOptions,
				RedisDefault:  a.redisDefault,
			}
			if spec.GlobalMiddlewares != nil {
				items, err := spec.GlobalMiddlewares(deps)
				if err != nil {
					return nil, gerror.Wrapf(err, "构建模块 %s 全局中间件失败", spec.Key)
				}
				plan.global = append(plan.global, items...)
			}
			if spec.Middlewares != nil {
				items, err := spec.Middlewares(deps)
				if err != nil {
					return nil, gerror.Wrapf(err, "构建模块 %s 局部中间件失败", spec.Key)
				}
				plan.modules[spec.Key] = append(plan.modules[spec.Key], items...)
			}
		}
	}
	if a.middlewareOverride != nil {
		plan.global = append(plan.global, a.middlewareOverride.Definitions...)
	}
	if !a.tenantEnabled {
		plan.global = append(plan.global, middleware.Definition{
			Name:    "app.tenant-disabled",
			Order:   200,
			Handler: disabledTenantMiddleware,
		})
	}
	plan.global = append(plan.global, middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{
		Logger:   a.errorLogger,
		Renderer: a.errorRenderer,
	})...)
	if err := validateMiddlewarePlan(plan, a.moduleSpecs); err != nil {
		return nil, err
	}
	return plan, nil
}

// 跨作用域校验名称，并分别稳定排序
func validateMiddlewarePlan(plan *middlewarePlan, specs []module.Spec) error {
	allDefinitions := append([]middleware.Definition{}, plan.global...)
	for _, spec := range specs {
		allDefinitions = append(allDefinitions, plan.modules[spec.Key]...)
	}
	if _, err := middleware.Validate(allDefinitions); err != nil {
		return err
	}
	global, err := middleware.Validate(plan.global)
	if err != nil {
		return err
	}
	modules := make(map[string][]middleware.Definition, len(plan.modules))
	for _, spec := range specs {
		if len(plan.modules[spec.Key]) == 0 {
			continue
		}
		items, validateErr := middleware.ValidateModule(plan.modules[spec.Key])
		if validateErr != nil {
			return validateErr
		}
		modules[spec.Key] = items
	}
	plan.global = global
	plan.modules = modules
	return nil
}

// 提取已排序的模块 Handler 链
func (p *middlewarePlan) moduleHandlers() map[string][]ghttp.HandlerFunc {
	items := make(map[string][]ghttp.HandlerFunc, len(p.modules))
	for moduleName, definitions := range p.modules {
		handlers := make([]ghttp.HandlerFunc, 0, len(definitions))
		for _, definition := range definitions {
			handlers = append(handlers, definition.Handler)
		}
		items[moduleName] = handlers
	}
	return items
}

// 显式关闭租户隔离时为整个请求注入跨租户作用域
func disabledTenantMiddleware(r *ghttp.Request) {
	r.SetCtx(tenant.WithoutTenant(r.Context()))
	r.Middleware.Next()
}

func setUploadSecurityHeaders(r *ghttp.Request) {
	r.Response.Header().Set("X-Content-Type-Options", "nosniff")
	r.Response.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
}
