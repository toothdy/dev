package controller

import (
	"context"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	"github.com/toothdy/cool-admin-go-next/cool/util/route"
)

const permissionDeniedMessage = "登录失效或无权限访问~"

// 权限检查器
type PermissionChecker interface {
	HasPermission(ctx context.Context, user security.UserContext, permission string) (bool, error)
}

/**
 * 注册权限 middleware
 * @param server HTTP 服务
 * @param checker 权限检查器
 * @param permissions 路由权限映射
 * @returns null
 */
func RegisterPermissionMiddleware(server *ghttp.Server, checker PermissionChecker, permissions map[string]string) {
	if server == nil {
		return
	}
	server.Use(NewPermissionMiddleware(checker, permissions))
}

// 创建权限中间件
func NewPermissionMiddleware(checker PermissionChecker, permissions map[string]string) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		permission, ok := RoutePermission(permissions, r.Method, r.URL.Path)
		if !ok {
			r.Middleware.Next()
			return
		}

		user, hasUser := security.UserFromContext(r.Context())
		if !hasUser {
			r.SetError(exception.Unauthorized())
			return
		}
		if checker == nil {
			r.SetError(exception.Forbidden())
			return
		}

		allowed, err := checker.HasPermission(r.Context(), user, permission)
		if err != nil {
			r.SetError(exception.Internal(err, "检查路由权限失败"))
			return
		}
		if !allowed {
			r.SetError(exception.Forbidden())
			return
		}
		r.Middleware.Next()
	}
}

/**
 * 获取路由权限码
 * @param permissions 权限映射
 * @param method HTTP 方法
 * @param path 路径
 * @returns 权限码和是否存在
 */
func RoutePermission(permissions map[string]string, method string, path string) (string, bool) {
	key, err := route.Key(method, path)
	if err != nil {
		return "", false
	}
	permission, ok := permissions[key]
	return permission, ok
}

/**
 * 写入无权限响应
 * @param r HTTP 请求
 * @returns null
 */
func WriteForbidden(r *ghttp.Request) {
	r.Response.WriteHeader(http.StatusForbidden)
	r.Response.WriteJson(response.Body{
		Code:    exception.CodeCommFail,
		Message: permissionDeniedMessage,
	})
}
