package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
)

func TestRoutePlanBindsModuleMiddlewaresByControllerModule(t *testing.T) {
	calls := make([]string, 0)
	action := func(name string) func() string {
		return func() string {
			calls = append(calls, name)
			return name
		}
	}
	controllers := []Definition{
		Admin("task/info").
			Route(RouteOptions{Name: "admin", Method: http.MethodGet, Path: "/scope", Action: action("task-admin")}).
			Route(RouteOptions{Name: "post", Method: http.MethodPost, Path: "/same", Action: action("task-post")}).
			Route(RouteOptions{Name: "get", Method: http.MethodGet, Path: "/same", Action: action("task-get")}).
			Build(),
		App("task/info").
			Route(RouteOptions{Name: "app", Method: http.MethodGet, Path: "/scope", Action: action("task-app")}).
			Build(),
		Admin("base/info").
			Route(RouteOptions{Name: "base", Method: http.MethodGet, Path: "/scope", Action: action("base")}).
			Build(),
	}
	plan, err := CompileRoutePlan(nil, controllers)
	if err != nil {
		t.Fatal(err)
	}
	global := nestingMiddleware(&calls, "global")
	moduleFirst := nestingMiddleware(&calls, "module-first")
	moduleSecond := nestingMiddleware(&calls, "module-second")
	server := newRoutePlanScopeServer(t)
	server.Use(global)
	server.BindHandler("GET:/health", func(request *ghttp.Request) {
		calls = append(calls, "health")
		request.Response.Write("ok")
	})
	if err = plan.BindWithMiddlewares(server, map[string][]ghttp.HandlerFunc{
		"task": {moduleFirst, moduleSecond},
	}); err != nil {
		t.Fatal(err)
	}
	startRoutePlanScopeServer(t, server)

	tests := []struct {
		method string
		path   string
		want   []string
	}{
		{http.MethodGet, "/admin/task/info/scope", []string{"global-before", "module-first-before", "module-second-before", "task-admin", "module-second-after", "module-first-after", "global-after"}},
		{http.MethodGet, "/app/task/info/scope", []string{"global-before", "module-first-before", "module-second-before", "task-app", "module-second-after", "module-first-after", "global-after"}},
		{http.MethodPost, "/admin/task/info/same", []string{"global-before", "module-first-before", "module-second-before", "task-post", "module-second-after", "module-first-after", "global-after"}},
		{http.MethodGet, "/admin/task/info/same", []string{"global-before", "module-first-before", "module-second-before", "task-get", "module-second-after", "module-first-after", "global-after"}},
		{http.MethodGet, "/admin/base/info/scope", []string{"global-before", "base", "global-after"}},
		{http.MethodGet, "/health", []string{"global-before", "health", "global-after"}},
		{http.MethodGet, "/admin/task/info/missing", []string{"global-before", "global-after"}},
	}
	for _, item := range tests {
		calls = calls[:0]
		server.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(item.method, item.path, nil))
		if !reflect.DeepEqual(calls, item.want) {
			t.Fatalf("unexpected calls for %s %s: got %#v want %#v", item.method, item.path, calls, item.want)
		}
	}
}

func TestRoutePlanModuleMiddlewareFailuresUseCoreErrorBoundary(t *testing.T) {
	controllers := []Definition{
		Admin("task/info").
			Route(RouteOptions{Name: "error", Method: http.MethodGet, Path: "/error", Action: func() string { return "unexpected" }}).
			Route(RouteOptions{Name: "panic", Method: http.MethodGet, Path: "/panic", Action: func() string { return "unexpected" }}).
			Build(),
	}
	plan, err := CompileRoutePlan(nil, controllers)
	if err != nil {
		t.Fatal(err)
	}
	server := newRoutePlanScopeServer(t)
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	server.Use(definitions[0].Handler, definitions[1].Handler)
	moduleMiddleware := func(request *ghttp.Request) {
		if request.URL.Path == "/admin/task/info/panic" {
			panic("module panic")
		}
		request.SetError(exception.Comm("模块失败"))
	}
	if err = plan.BindWithMiddlewares(server, map[string][]ghttp.HandlerFunc{"task": {moduleMiddleware}}); err != nil {
		t.Fatal(err)
	}
	startRoutePlanScopeServer(t, server)

	tests := []struct {
		path string
		body string
	}{
		{"/admin/task/info/error", `{"code":1001,"message":"模块失败"}`},
		{"/admin/task/info/panic", `{"code":1001,"message":"操作失败"}`},
	}
	for _, item := range tests {
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, item.path, nil))
		if recorder.Body.String() != item.body {
			t.Fatalf("unexpected error response for %s: %d %s", item.path, recorder.Code, recorder.Body.String())
		}
	}
}

func TestRoutePlanModuleMiddlewareSupportsAllRouteMethods(t *testing.T) {
	methods := []string{
		http.MethodDelete,
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
		http.MethodPatch,
		http.MethodPost,
		http.MethodPut,
	}
	builder := Admin("task/methods")
	for index, method := range methods {
		builder.Route(RouteOptions{
			Name: fmt.Sprintf("method-%d", index), Method: method, Path: fmt.Sprintf("/method-%d", index),
			Action: func() string { return "ok" },
		})
	}
	plan, err := CompileRoutePlan(nil, []Definition{builder.Build()})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	server := newRoutePlanScopeServer(t)
	if err = plan.BindWithMiddlewares(server, map[string][]ghttp.HandlerFunc{
		"task": {func(request *ghttp.Request) {
			calls++
			request.Middleware.Next()
		}},
	}); err != nil {
		t.Fatal(err)
	}
	startRoutePlanScopeServer(t, server)
	for index, method := range methods {
		server.ServeHTTP(
			httptest.NewRecorder(),
			httptest.NewRequest(method, fmt.Sprintf("/admin/task/methods/method-%d", index), nil),
		)
	}
	if calls != len(methods) {
		t.Fatalf("expected middleware for every supported method: got %d want %d", calls, len(methods))
	}
}

func BenchmarkRoutePlanRequest(b *testing.B) {
	for _, hasMiddleware := range []bool{false, true} {
		name := "without-module-middleware"
		if hasMiddleware {
			name = "with-module-middleware"
		}
		b.Run(name, func(b *testing.B) {
			controller := Admin("task/benchmark").Route(RouteOptions{
				Name: "request", Method: http.MethodGet, Path: "/request", Action: func() string { return "ok" },
			}).Build()
			plan, err := CompileRoutePlan(nil, []Definition{controller})
			if err != nil {
				b.Fatal(err)
			}
			server := ghttp.GetServer(guid.S())
			server.SetAddr("127.0.0.1:0")
			server.SetDumpRouterMap(false)
			middlewares := map[string][]ghttp.HandlerFunc{}
			if hasMiddleware {
				middlewares["task"] = []ghttp.HandlerFunc{func(request *ghttp.Request) { request.Middleware.Next() }}
			}
			if err = plan.BindWithMiddlewares(server, middlewares); err != nil {
				b.Fatal(err)
			}
			if err = server.Start(); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = server.Shutdown() })
			request := httptest.NewRequest(http.MethodGet, "/admin/task/benchmark/request", nil)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				server.ServeHTTP(httptest.NewRecorder(), request.Clone(request.Context()))
			}
		})
	}
}

func BenchmarkRoutePlanBindMultipleModules(b *testing.B) {
	controllers := make([]Definition, 0, 20)
	middlewares := make(map[string][]ghttp.HandlerFunc, 4)
	for moduleIndex := 0; moduleIndex < 4; moduleIndex++ {
		moduleName := fmt.Sprintf("module%d", moduleIndex)
		middlewares[moduleName] = []ghttp.HandlerFunc{func(request *ghttp.Request) { request.Middleware.Next() }}
		for routeIndex := 0; routeIndex < 5; routeIndex++ {
			controllers = append(controllers, Admin(fmt.Sprintf("%s/resource%d", moduleName, routeIndex)).Route(RouteOptions{
				Name: "list", Method: http.MethodGet, Path: "/list", Action: func() string { return "ok" },
			}).Build())
		}
	}
	plan, err := CompileRoutePlan(nil, controllers)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		server := ghttp.GetServer(guid.S())
		if err = plan.BindWithMiddlewares(server, middlewares); err != nil {
			b.Fatal(err)
		}
	}
}

func nestingMiddleware(calls *[]string, name string) ghttp.HandlerFunc {
	return func(request *ghttp.Request) {
		*calls = append(*calls, name+"-before")
		request.Middleware.Next()
		*calls = append(*calls, name+"-after")
	}
}

func newRoutePlanScopeServer(t *testing.T) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	return server
}

func startRoutePlanScopeServer(t *testing.T, server *ghttp.Server) {
	t.Helper()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
}
