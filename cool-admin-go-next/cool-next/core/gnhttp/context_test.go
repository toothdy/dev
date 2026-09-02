package gnhttp

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/app"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
)

func TestContextMiddlewareBuildsProtocolIndependentContext(t *testing.T) {
	var captured context.Context
	authenticator := httpAuthenticatorStub{authenticate: func(
		ctx context.Context,
		authorization string,
		method string,
		requestPath string,
		permission string,
		ignoreToken bool,
	) (context.Context, error) {
		if authorization != "Bearer token" || method != http.MethodPost || requestPath != "/admin/base/user/update" ||
			permission != "base:user:update" || ignoreToken {
			t.Fatalf("AuthenticateHTTP() = %q, %q, %q, %q, %t", authorization, method, requestPath, permission, ignoreToken)
		}
		return ctx, nil
	}}
	middleware, err := NewContextMiddleware(authenticator, "/admin/base/user/update", false)
	if err != nil {
		t.Fatal(err)
	}
	server, listener := startServer(t, func(server *ghttp.Server) {
		server.BindHandler("POST:/admin/base/user/update", func(request *ghttp.Request) {
			captured = request.Context()
			request.Response.Write("ok")
		})
		server.BindMiddleware("/admin/base/user/update", middleware)
	})
	t.Cleanup(func() { shutdownServer(t, server, listener) })

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+listener.Addr().String()+"/admin/base/user/update", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("HTTP status = %d", response.StatusCode)
	}
	if captured == nil || app.TraceID(captured) == "" || app.TraceID(captured) != gtrace.GetTraceID(captured) {
		t.Fatalf("Trace ID = %q, %q", app.TraceID(captured), gtrace.GetTraceID(captured))
	}
}

func TestContextMiddlewarePreservesCancellation(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(t.Context(), deadline)
	request := &ghttp.Request{Request: (&http.Request{Method: http.MethodGet, Header: http.Header{}, URL: mustURL(t, "/public")}).WithContext(parent)}
	authenticator := httpAuthenticatorStub{authenticate: func(ctx context.Context, _, _, _, _ string, _ bool) (context.Context, error) {
		return ctx, nil
	}}
	if err := authenticateRequest(request, authenticator, "/public", auth.Rule{IgnoreToken: true}); err != nil {
		t.Fatal(err)
	}
	gotDeadline, exists := request.Context().Deadline()
	if !exists || !gotDeadline.Equal(deadline) {
		t.Fatalf("Deadline() = %v, %t", gotDeadline, exists)
	}
	cancel()
	if request.Context().Err() != context.Canceled {
		t.Fatalf("Context error = %v", request.Context().Err())
	}
}

func TestContextMiddlewareRendersAuthenticationErrorBeforeHandler(t *testing.T) {
	wasHandled := false
	authenticator := httpAuthenticatorStub{authenticate: func(ctx context.Context, _, _, _, _ string, _ bool) (context.Context, error) {
		return ctx, exception.Comm("凭证无效", http.StatusUnauthorized)
	}}
	middleware, err := NewContextMiddleware(authenticator, "/admin/protected", false)
	if err != nil {
		t.Fatal(err)
	}
	server, listener := startServer(t, func(server *ghttp.Server) {
		server.Use(gnctrl.NewResponseMiddleware(nil))
		server.BindMiddleware("GET:/admin/protected", middleware)
		server.BindHandler("GET:/admin/protected", func(request *ghttp.Request) {
			wasHandled = true
			request.Response.Write("unexpected")
		})
	})
	t.Cleanup(func() { shutdownServer(t, server, listener) })

	response, err := http.Get("http://" + listener.Addr().String() + "/admin/protected")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized || string(body) != `{"code":1001,"message":"凭证无效"}` {
		t.Fatalf("authentication response = %d %s", response.StatusCode, body)
	}
	if wasHandled {
		t.Fatal("protected handler was called")
	}
}

type httpAuthenticatorStub struct {
	authenticate func(context.Context, string, string, string, string, bool) (context.Context, error)
}

// 调用测试认证函数
func (stub httpAuthenticatorStub) AuthenticateHTTP(
	ctx context.Context,
	authorization string,
	method string,
	requestPath string,
	permission string,
	ignoreToken bool,
) (context.Context, error) {
	return stub.authenticate(ctx, authorization, method, requestPath, permission, ignoreToken)
}

// 解析测试 URL
func mustURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
