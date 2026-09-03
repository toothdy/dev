package gnhttp

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
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

func TestAuthenticateRequestBuildsAuditInputFromVerifiedIdentity(t *testing.T) {
	tests := []struct {
		name         string
		subject      auth.TokenSubject
		operatorType string
		operatorID   string
	}{
		{
			name: "admin",
			subject: auth.TokenSubject{
				SessionID: "admin-session",
				Subject:   auth.AdminKind,
				UserID:    42,
				Username:  "admin",
				RoleIDs:   []uint64{1},
				PasswordV: 1,
			},
			operatorType: "admin",
			operatorID:   "42",
		},
		{
			name: "app",
			subject: auth.TokenSubject{
				SessionID: "app-session",
				Subject:   auth.AppKind,
				UserID:    9007199254740993,
			},
			operatorType: "app",
			operatorID:   "9007199254740993",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := auth.Claims{TokenSubject: test.subject, JTI: "access"}
			snapshot := auth.Snapshot{
				TokenSubject: test.subject,
				AccessJTI:    "access",
				RefreshJTI:   "refresh",
				ExpiresAt:    time.Now().Add(time.Hour),
			}
			authenticator, err := auth.NewService(
				httpCodecStub{claims: claims},
				httpSessionStoreStub{snapshot: snapshot},
				httpAuthorizerStub{},
			)
			if err != nil {
				t.Fatal(err)
			}
			request, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				"http://example.com/admin/base/user/update?visible=1&token=query-secret",
				strings.NewReader(`{"id":9007199254740993,"nested":{"password":"body-secret"}}`),
			)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Authorization", "Bearer token")
			request.Header.Set("Content-Type", "application/json")
			gfRequest := &ghttp.Request{Request: request}
			if err = authenticateRequest(
				gfRequest,
				authenticator,
				"/admin/base/user/update",
				auth.Rule{Permission: "base:user:update"},
			); err != nil {
				t.Fatal(err)
			}
			input := requestAuditInput(gfRequest, gfRequest.Context())
			if input.Source != "/admin/base/user/update?visible=1&token=query-secret" ||
				input.OperatorType != test.operatorType || input.OperatorID != test.operatorID {
				t.Fatalf("audit input = %#v", input)
			}
			if input.Params["visible"] != "1" || input.Params["token"] != "query-secret" || input.Params["id"] == nil {
				t.Fatalf("audit params = %#v", input.Params)
			}
		})
	}
}

func TestRequestAuditInputDoesNotParseMultipartBody(t *testing.T) {
	const body = "multipart body must remain unread"
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		"http://example.com/admin/base/comm/upload?visible=1",
		strings.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	input := requestAuditInput(&ghttp.Request{Request: request}, request.Context())
	if len(input.Params) != 1 || input.Params["visible"] != "1" {
		t.Fatalf("audit params = %#v", input.Params)
	}
	if request.MultipartForm != nil {
		t.Fatal("audit parsed multipart body")
	}
	content, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != body {
		t.Fatalf("remaining body = %q", content)
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

type httpCodecStub struct {
	claims auth.Claims
}

func (stub httpCodecStub) IssuePair(auth.TokenSubject) (auth.Pair, error) {
	return auth.Pair{}, nil
}

func (stub httpCodecStub) Parse(string, bool) (auth.Claims, error) {
	return stub.claims, nil
}

type httpSessionStoreStub struct {
	snapshot auth.Snapshot
}

func (stub httpSessionStoreStub) Get(context.Context, string) (auth.Snapshot, bool, error) {
	return stub.snapshot, true, nil
}

func (httpSessionStoreStub) Save(context.Context, auth.Snapshot) error {
	return nil
}

func (httpSessionStoreStub) RotateRefresh(context.Context, string, string, auth.Snapshot) error {
	return nil
}

func (httpSessionStoreStub) Revoke(context.Context, string) error {
	return nil
}

func (httpSessionStoreStub) RevokeUsers(context.Context, auth.Kind, []uint64) error {
	return nil
}

type httpAuthorizerStub struct{}

func (httpAuthorizerStub) Authorize(context.Context, auth.Authorization) (bool, error) {
	return true, nil
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
