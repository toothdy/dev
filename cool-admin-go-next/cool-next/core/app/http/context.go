package apphttp

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/app"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// HTTP 认证内核
type Authenticator interface {
	AuthenticateHTTP(context.Context, string, string, string, string, bool) (context.Context, error)
}

// 创建协议无关请求上下文中间件
func NewContextMiddleware(authenticator Authenticator, requestPath string, rule auth.Rule) (ghttp.HandlerFunc, error) {
	if authenticator == nil {
		return nil, exception.Core("HTTP Authenticator 不能为空")
	}
	if rule.IgnoreToken && strings.TrimSpace(rule.Permission) != "" {
		return nil, exception.Core("HTTP ignoreToken 与权限不能同时配置")
	}
	if _, err := auth.HTTPResource("GET", requestPath); err != nil {
		return nil, err
	}

	return func(request *ghttp.Request) {
		if err := authenticateRequest(request, authenticator, requestPath, rule); err != nil {
			request.SetError(err)
			return
		}
		request.Middleware.Next()
	}, nil
}

// 认证单个 HTTP 请求
func authenticateRequest(request *ghttp.Request, authenticator Authenticator, requestPath string, rule auth.Rule) error {
	if request == nil || request.Request == nil {
		return exception.Core("HTTP 请求不能为空")
	}
	ctx, err := withRequestTrace(request.Context())
	if err != nil {
		return err
	}
	request.SetCtx(ctx)
	verified, err := authenticator.AuthenticateHTTP(
		ctx,
		request.Header.Get("Authorization"),
		request.Method,
		requestPath,
		rule.Permission,
		rule.IgnoreToken,
	)
	if err != nil {
		return err
	}
	request.SetCtx(verified)

	return nil
}

// 复用或建立 HTTP Trace ID
func withRequestTrace(ctx context.Context) (context.Context, error) {
	traceID := gtrace.GetTraceID(ctx)
	if traceID == "" {
		var err error
		traceID, err = app.NewTraceID()
		if err != nil {
			return ctx, err
		}
		ctx, err = gtrace.WithTraceID(ctx, traceID)
		if err != nil {
			return ctx, exception.WrapCore(err, "写入 HTTP Trace ID 失败")
		}
	}

	return app.WithTraceID(ctx, traceID)
}
