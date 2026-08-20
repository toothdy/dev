package security

import (
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	"github.com/toothdy/cool-admin-go-next/cool/util/route"
)

// auth middleware 配置
type MiddlewareOptions struct {
	Manager           *Manager
	Sessions          SessionStore
	IgnorePaths       []string
	IgnoreRouteKeys   map[string]struct{}
	ProtectedPrefixes []string
	SSO               bool
}

/**
 * 路径是否放行
 * @param path 请求路径
 * @returns bool
 */
func (o MiddlewareOptions) IsIgnored(path string) bool {
	for _, item := range o.IgnorePaths {
		if item == path {
			return true
		}
	}
	return false
}

// 按 method 和规范化 path 判断认证白名单
func (o MiddlewareOptions) IsRouteIgnored(method string, path string) bool {
	key, err := route.Key(method, path)
	if err == nil {
		if _, ok := o.IgnoreRouteKeys[key]; ok {
			return true
		}
	}
	return o.IsIgnored(path)
}

// 判断路径是否属于当前认证范围
func (o MiddlewareOptions) IsProtected(path string) bool {
	if len(o.ProtectedPrefixes) == 0 {
		return true
	}
	for _, prefix := range o.ProtectedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

/**
 * 创建 token middleware
 * @param options middleware 配置
 * @returns ghttp.HandlerFunc
 */
func NewMiddleware(options MiddlewareOptions) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		if !options.IsProtected(r.URL.Path) || options.IsRouteIgnored(r.Method, r.URL.Path) {
			r.Middleware.Next()
			return
		}
		token := AuthorizationToken(r)
		if token == "" || options.Manager == nil {
			r.SetError(exception.Unauthorized())
			return
		}
		claims, err := options.Manager.ParseAccessToken(token)
		if err != nil {
			r.SetError(exception.Unauthorized())
			return
		}
		if options.Sessions == nil {
			r.SetError(exception.Internal(nil, "登录会话存储不可用"))
			return
		}
		session, ok, sessionErr := options.Sessions.Get(r.Context(), claims.SessionID)
		if sessionErr != nil {
			r.SetError(exception.Internal(sessionErr, "读取登录会话失败"))
			return
		}
		if !ok ||
			session.UserID != claims.UserId ||
			session.PasswordVersion != claims.PasswordVersion ||
			(options.SSO && session.AccessJTIHash != HashTokenID(claims.JTI)) {
			r.SetError(exception.Unauthorized())
			return
		}
		r.SetCtx(ContextWithUser(r.Context(), UserContext{
			SessionID:       claims.SessionID,
			AccessJTI:       claims.JTI,
			UserId:          claims.UserId,
			Username:        claims.Username,
			RoleIds:         append([]int64(nil), claims.RoleIds...),
			PasswordVersion: claims.PasswordVersion,
			TenantId:        claims.TenantId,
		}))
		r.Middleware.Next()
	}
}

/**
 * 从请求读取 Authorization token
 * @param r HTTP 请求
 * @returns string
 */
func AuthorizationToken(r *ghttp.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}

/**
 * 写入未授权响应
 * @param r HTTP 请求
 * @returns null
 */
func Unauthorized(r *ghttp.Request) {
	r.Response.WriteHeader(http.StatusUnauthorized)
	r.Response.WriteJson(response.Body{
		Code:    exception.CodeCommFail,
		Message: "登录失效~",
	})
}
