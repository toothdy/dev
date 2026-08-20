package middleware

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/response"
)

const (
	RecoveryName  = "cool.recovery"
	RecoveryOrder = 0
	ErrorName     = "cool.error"
	ErrorOrder    = 150
)

// 记录完整错误，Resolved 只包含安全传输信息
type ErrorLogger interface {
	Log(ctx context.Context, resolved exception.Resolved, err error)
}

// 写入统一失败响应
type ErrorRenderer interface {
	Write(r *ghttp.Request, resolved exception.Resolved)
}

// 核心错误边界依赖
type ErrorBoundaryOptions struct {
	Logger   ErrorLogger
	Renderer ErrorRenderer
}

// 创建不可关闭的核心错误中间件
func CoreErrorDefinitions(options ErrorBoundaryOptions) []Definition {
	boundary := newErrorBoundary(options)
	return []Definition{
		{Name: RecoveryName, Order: RecoveryOrder, Handler: boundary.recovery, core: true},
		{Name: ErrorName, Order: ErrorOrder, Handler: boundary.errors, core: true},
	}
}

type errorBoundary struct {
	logger   ErrorLogger
	renderer ErrorRenderer
}

func newErrorBoundary(options ErrorBoundaryOptions) *errorBoundary {
	logger := options.Logger
	if logger == nil {
		logger = defaultErrorLogger{}
	}
	renderer := options.Renderer
	if renderer == nil {
		renderer = defaultErrorRenderer{}
	}
	return &errorBoundary{logger: logger, renderer: renderer}
}

func (b *errorBoundary) recovery(r *ghttp.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			b.handle(r, panicError(recovered))
			return
		}
		if err := r.GetError(); err != nil {
			r.SetError(nil)
			b.handle(r, err)
		}
	}()
	r.Middleware.Next()
}

func (b *errorBoundary) errors(r *ghttp.Request) {
	defer func() {
		if recovered := recover(); recovered != nil {
			b.handle(r, panicError(recovered))
			return
		}
		if err := r.GetError(); err != nil {
			r.SetError(nil)
			b.handle(r, err)
		}
	}()
	r.Middleware.Next()
}

func (b *errorBoundary) handle(r *ghttp.Request, err error) {
	err = normalizeRequestError(err)
	resolved := exception.Resolve(err)
	b.logger.Log(r.Context(), resolved, err)
	if responseCommitted(r) {
		return
	}
	b.renderer.Write(r, resolved)
}

func responseCommitted(r *ghttp.Request) bool {
	return r.Response.IsHeaderWrote() || r.Response.BytesWritten() > 0 || r.Response.IsHijacked()
}

func panicError(recovered any) error {
	if err, ok := recovered.(error); ok {
		if normalized := normalizeRequestError(err); normalized != err {
			return normalized
		}
	}
	return exception.Internal(
		fmt.Errorf("panic: %v\n%s", recovered, debug.Stack()),
		"request panic",
	)
}

func normalizeRequestError(err error) error {
	if err == nil {
		return nil
	}
	var maxBytesError *http.MaxBytesError
	if stderrors.As(err, &maxBytesError) {
		return exception.WrapPayloadTooLarge(err, "请求体过大")
	}
	return err
}

type defaultErrorRenderer struct{}

func (defaultErrorRenderer) Write(r *ghttp.Request, resolved exception.Resolved) {
	r.Response.ClearBuffer()
	r.Response.Status = 0
	r.Response.Header().Del("Content-Length")
	r.Response.Header().Set("Content-Type", "application/json; charset=utf-8")
	r.Response.WriteHeader(resolved.HTTPStatus)
	r.Response.WriteJson(response.Body{Code: resolved.BusinessCode, Message: resolved.Message})
}

type defaultErrorLogger struct{}

func (defaultErrorLogger) Log(ctx context.Context, resolved exception.Resolved, err error) {
	if err == nil || resolved.LogLevel == exception.LogNone {
		return
	}
	switch resolved.LogLevel {
	case exception.LogDebug:
		g.Log().Debug(ctx, err)
	case exception.LogInfo:
		g.Log().Info(ctx, err)
	case exception.LogWarn:
		g.Log().Warning(ctx, err)
	default:
		g.Log().Error(ctx, err)
	}
}
