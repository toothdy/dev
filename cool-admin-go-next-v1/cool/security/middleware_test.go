package security_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

func TestAuthorizationTokenSupportsRawToken(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "raw.token.value")
	r := &ghttp.Request{Request: request}

	if token := security.AuthorizationToken(r); token != "raw.token.value" {
		t.Fatalf("expected raw token, got %q", token)
	}
}

func TestAuthorizationTokenSupportsBearerToken(t *testing.T) {
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer raw.token.value")
	r := &ghttp.Request{Request: request}

	if token := security.AuthorizationToken(r); token != "raw.token.value" {
		t.Fatalf("expected bearer token stripped, got %q", token)
	}
}

func TestMiddlewareOptionsIgnorePath(t *testing.T) {
	options := security.MiddlewareOptions{
		Manager:     security.NewManager("secret", 7200, 604800),
		IgnorePaths: []string{"/admin/base/open/login"},
	}
	if !options.IsIgnored("/admin/base/open/login") {
		t.Fatal("expected login path ignored")
	}
	if options.IsIgnored("/admin/base/comm/person") {
		t.Fatal("expected person path protected")
	}
}

func TestUnauthorizedWritesJsonBodyAndStatus(t *testing.T) {
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.BindHandler("/unauthorized", func(r *ghttp.Request) {
		security.Unauthorized(r)
	})
	server.SetDumpRouterMap(false)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()

	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/unauthorized", server.GetListenedPort()))
	if err != nil {
		t.Fatalf("request unauthorized failed: %v", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read unauthorized body failed: %v", err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", response.StatusCode)
	}
	if string(data) != `{"code":1001,"message":"登录失效~"}` {
		t.Fatalf("expected json body, got %q", string(data))
	}
}
