package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gsession"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type allowChecker struct {
	permission string
}

/**
 * 检查权限
 * @param ctx 上下文
 * @param user 用户上下文
 * @param permission 权限码
 * @returns bool
 */
func (c *allowChecker) HasPermission(ctx context.Context, user security.UserContext, permission string) (bool, error) {
	c.permission = permission
	return permission == "base:sys:user:page", nil
}

/**
 * 测试权限 checker 接口
 * @param t 测试对象
 * @returns null
 */
func TestPermissionCheckerInterface(t *testing.T) {
	checker := &allowChecker{}
	ok, err := checker.HasPermission(context.Background(), security.UserContext{Username: "demo"}, "base:sys:user:page")
	if err != nil {
		t.Fatalf("check permission failed: %v", err)
	}
	if !ok {
		t.Fatal("expected permission allowed")
	}
	if checker.permission != "base:sys:user:page" {
		t.Fatalf("expected captured permission, got %s", checker.permission)
	}
}

/**
 * 测试权限键使用大写方法和纯路径
 * @param t 测试对象
 * @returns null
 */
func TestRoutePermissionUsesMethodAndPurePath(t *testing.T) {
	permissions := map[string]string{
		"POST:/admin/base/sys/user/page": "base:sys:user:page",
	}

	permission, ok := RoutePermission(permissions, "post", "/admin/base/sys/user/page")
	if !ok || permission != "base:sys:user:page" {
		t.Fatalf("expected permission lookup success, got %q, %t", permission, ok)
	}

	if _, ok := RoutePermission(permissions, "POST", "/admin/base/sys/user/page?keyword=demo"); ok {
		t.Fatal("expected query string not to be part of permission lookup")
	}
}

/**
 * 测试未认证请求返回未授权响应
 * @param t 测试对象
 * @returns null
 */
func TestRegisterPermissionMiddlewareRejectsUnauthenticatedRequest(t *testing.T) {
	server := ghttp.GetServer("controller-permission-unauthorized-test")
	server.SetPort(0)
	server.SetSessionStorage(gsession.NewStorageMemory())
	registerTestErrorBoundary(t, server)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	RegisterPermissionMiddleware(server, &allowChecker{}, map[string]string{
		"GET:/protected": "base:protected:view",
	})
	server.BindHandler("GET:/protected", func(r *ghttp.Request) {
		r.Response.WriteJson(response.OK("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	body := response.Body{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode unauthorized response failed: %v", err)
	}
	if body.Code != exception.CodeCommFail || body.Message != "登录失效~" {
		t.Fatalf("unexpected unauthorized response: %#v", body)
	}
}

/**
 * 测试已认证请求按方法和路径检查权限
 * @param t 测试对象
 * @returns null
 */
func TestRegisterPermissionMiddlewareChecksAuthenticatedRoute(t *testing.T) {
	server := ghttp.GetServer("controller-permission-check-test")
	server.SetPort(0)
	server.SetSessionStorage(gsession.NewStorageMemory())
	registerTestErrorBoundary(t, server)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	checker := &allowChecker{}
	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{Username: "demo"}))
		r.Middleware.Next()
	})
	RegisterPermissionMiddleware(server, checker, map[string]string{
		"POST:/admin/base/sys/user/page": "base:sys:user:page",
	})
	server.BindHandler("POST:/admin/base/sys/user/page", func(r *ghttp.Request) {
		r.Response.WriteJson(response.OK("ok"))
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/base/sys/user/page?keyword=demo", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if checker.permission != "base:sys:user:page" {
		t.Fatalf("expected checker permission, got %s", checker.permission)
	}
}

/**
 * 测试权限检查错误返回固定禁止响应
 * @param t 测试对象
 * @returns null
 */
func TestRegisterPermissionMiddlewareReturnsForbiddenOnCheckerError(t *testing.T) {
	server := ghttp.GetServer("controller-permission-error-test")
	server.SetPort(0)
	server.SetSessionStorage(gsession.NewStorageMemory())
	registerTestErrorBoundary(t, server)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	server.Use(func(r *ghttp.Request) {
		r.SetCtx(security.ContextWithUser(r.Context(), security.UserContext{Username: "demo"}))
		r.Middleware.Next()
	})
	RegisterPermissionMiddleware(server, permissionErrorChecker{}, map[string]string{
		"GET:/protected": "base:protected:view",
	})
	server.BindHandler("GET:/protected", func(r *ghttp.Request) {
		r.Response.WriteJson(response.OK("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected protocol-compatible 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != `{"code":1001,"message":"操作失败"}` {
		t.Fatalf("unexpected forbidden response: %s", rec.Body.String())
	}
}

type permissionErrorChecker struct{}

/**
 * 返回权限检查错误
 * @param ctx 上下文
 * @param user 用户上下文
 * @param permission 权限码
 * @returns bool 和错误
 */
func (permissionErrorChecker) HasPermission(ctx context.Context, user security.UserContext, permission string) (bool, error) {
	return false, errors.New("checker failed")
}

/**
 * 测试无权限映射的路由不受中间件影响
 * @param t 测试对象
 * @returns null
 */
func TestRegisterPermissionMiddlewareLeavesUnmappedRouteAlone(t *testing.T) {
	server := ghttp.GetServer("controller-permission-unmapped-test")
	server.SetPort(0)
	server.SetSessionStorage(gsession.NewStorageMemory())
	registerTestErrorBoundary(t, server)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	RegisterPermissionMiddleware(server, nil, map[string]string{})
	server.BindHandler("GET:/public", func(r *ghttp.Request) {
		r.Response.WriteJson(response.OK("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/public", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != `{"code":1000,"message":"success","data":"ok"}` {
		t.Fatalf("expected public route to pass unchanged, got %d: %s", rec.Code, rec.Body.String())
	}
}

func registerTestErrorBoundary(t *testing.T, server *ghttp.Server) {
	t.Helper()
	for _, definition := range middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{}) {
		server.Use(definition.Handler)
	}
}
