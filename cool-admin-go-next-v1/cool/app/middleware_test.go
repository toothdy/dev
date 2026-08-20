package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/app"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestApplicationCanReplaceConfiguredMiddlewares(t *testing.T) {
	useModuleTestConfig(t)
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	app.New(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		MiddlewareDefinitions: []middleware.Definition{{
			Name: "test.header", Order: 250,
			Handler: func(r *ghttp.Request) {
				r.Response.Header().Set("X-Test-Middleware", "enabled")
				r.Middleware.Next()
			},
		}},
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start app test server failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("X-Test-Middleware") != "enabled" {
		t.Fatalf("injected middleware was not used: status=%d headers=%v", recorder.Code, recorder.Header())
	}
}

func TestReplacingModuleMiddlewaresKeepsCoreErrorBoundary(t *testing.T) {
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	_, err := app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareReplaceModules,
			Definitions: []middleware.Definition{{
				Name: "test.failure", Order: 250,
				Handler: func(r *ghttp.Request) {
					r.SetError(exception.Comm("测试失败"))
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("build app failed: %v", err)
	}
	if err = server.Start(); err != nil {
		t.Fatalf("start app test server failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Body.String() != `{"code":1001,"message":"测试失败"}` {
		t.Fatalf("core error boundary was removed: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestMiddlewareOverrideAppendKeepsConfiguredMiddlewares(t *testing.T) {
	useModuleTestConfig(t)
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	_, err := app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareAppend,
			Definitions: []middleware.Definition{{
				Name: "test.append", Order: 250,
				Handler: func(r *ghttp.Request) {
					r.Response.Header().Set("X-Append-Middleware", "enabled")
					r.Middleware.Next()
				},
			}},
		},
	})
	if err != nil {
		t.Fatalf("build app failed: %v", err)
	}
	if err = server.Start(); err != nil {
		t.Fatalf("start app test server failed: %v", err)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("X-Append-Middleware") != "enabled" {
		t.Fatalf("append middleware was not used: status=%d headers=%v", recorder.Code, recorder.Header())
	}
}

func TestBuildRejectsCrossScopeDuplicateBeforeChangingServer(t *testing.T) {
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()
	handler := func(r *ghttp.Request) { r.Middleware.Next() }

	_, err := app.Build(app.Options{
		StartServer:        true,
		Server:             server,
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		Specs:        modules.Specs(),
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareAppend,
			Definitions: []middleware.Definition{{
				Name: "task.health", Order: 250, Handler: handler,
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "task.health") {
		t.Fatalf("expected cross-scope duplicate error, got %v", err)
	}
	if err = server.Start(); err != nil {
		t.Fatalf("start unchanged server failed: %v", err)
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("server should not contain partially registered health route: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestBuildRejectsUnknownMiddlewareOverrideMode(t *testing.T) {
	_, err := app.BuildWithContext(context.Background(), app.Options{
		StartServer:        true,
		Server:             ghttp.GetServer(guid.S()),
		UploadDir:          t.TempDir(),
		AuthManagerFactory: testAuthManagerFactory,
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: "unknown",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unknown middleware mode error, got %v", err)
	}
}

func TestBuildRejectsWeakJWTSecretWithoutExposingIt(t *testing.T) {
	secret := "short-secret"
	adapter, err := gcfg.NewAdapterContent(`cool:
  auth:
    jwtSecret: "` + secret + `"`)
	if err != nil {
		t.Fatal(err)
	}
	config := g.Cfg()
	previous := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() { config.SetAdapter(previous) })

	_, err = app.Build(app.Options{
		StartServer: true,
		Server:      ghttp.GetServer(guid.S()),
		UploadDir:   t.TempDir(),
		MiddlewareOverride: &app.MiddlewareOverride{
			Mode: app.MiddlewareReplaceModules,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "jwtSecret") || strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected weak jwtSecret error: %v", err)
	}
}

func TestRegisteredModulesDeclareNodeCompatibleMiddlewares(t *testing.T) {
	specs := modules.Specs()
	seen := map[string]bool{}
	for _, spec := range specs {
		switch spec.Key {
		case "base":
			seen["base"] = true
			if spec.GlobalMiddlewares == nil || spec.Middlewares != nil {
				t.Fatal("base module should declare global middlewares")
			}
		case "dict":
			seen["dict"] = true
			if spec.GlobalMiddlewares != nil || spec.Middlewares != nil {
				t.Fatal("dict module should keep its Node-compatible empty middleware declaration")
			}
		case "recycle":
			seen["recycle"] = true
			if spec.GlobalMiddlewares != nil || spec.Middlewares != nil {
				t.Fatal("recycle module should keep its Node-compatible empty middleware declaration")
			}
		case "task":
			seen["task"] = true
			if spec.GlobalMiddlewares != nil || spec.Middlewares == nil {
				t.Fatal("task module should declare module middlewares")
			}
		}
	}
	if !seen["base"] || !seen["dict"] || !seen["recycle"] || !seen["task"] {
		t.Fatalf("expected base and dict module specs, got %#v", seen)
	}
}
