package apphttp

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

// 记录中间件实际传给认证内核的权限标识
type recordingAuthenticator struct {
	mutex   sync.Mutex
	records map[string]string
}

func (r *recordingAuthenticator) AuthenticateHTTP(
	ctx context.Context, _ string, _ string, requestPath string, permission string, ignoreToken bool,
) (context.Context, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.records[requestPath] = fmt.Sprintf("permission=%q ignoreToken=%t", permission, ignoreToken)

	return ctx, nil
}

func (r *recordingAuthenticator) get(path string) string {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return r.records[path]
}

// 端到端验证推导值经中间件送达认证内核
func TestContextMiddlewareDeliversDerivedPermission(t *testing.T) {
	cases := []struct {
		path        string
		ignoreToken bool
		want        string
	}{
		{path: "/admin/base/sys/user/move", want: `permission="base:sys:user:move" ignoreToken=false`},
		{path: "/admin/base/sys/menu/import", want: `permission="base:sys:menu:import" ignoreToken=false`},
		{path: "/admin/base/coding/createCode", want: `permission="base:coding:createCode" ignoreToken=false`},
		{path: "/admin/base/comm/person", want: `permission="" ignoreToken=false`},
		{path: "/admin/base/open/captcha", ignoreToken: true, want: `permission="" ignoreToken=true`},
		{path: "/app/base/comm/upload", want: `permission="" ignoreToken=false`},
	}

	authenticator := &recordingAuthenticator{records: map[string]string{}}
	server := g.Server("permission-wiring-test")
	server.SetDumpRouterMap(false)
	server.SetAccessLogEnabled(false)
	server.SetErrorLogEnabled(false)

	for _, testCase := range cases {
		middleware, err := NewContextMiddleware(authenticator, testCase.path, testCase.ignoreToken)
		if err != nil {
			t.Fatalf("%s 构造中间件失败: %v", testCase.path, err)
		}
		pattern := "GET:" + testCase.path
		server.BindMiddleware(pattern, middleware)
		server.BindHandler(pattern, func(request *ghttp.Request) { request.Response.Write("ok") })
	}

	server.SetPort(0)
	server.Start()
	defer func() { _ = server.Shutdown() }()

	client := g.Client()
	client.SetPrefix(fmt.Sprintf("http://127.0.0.1:%d", server.GetListenedPort()))
	for _, testCase := range cases {
		response, err := client.Get(context.Background(), testCase.path)
		if err != nil {
			t.Fatalf("%s 请求失败: %v", testCase.path, err)
		}
		status := response.StatusCode
		_ = response.Close()
		if status != http.StatusOK {
			t.Fatalf("%s 状态码 = %d", testCase.path, status)
		}
		if got := authenticator.get(testCase.path); got != testCase.want {
			t.Errorf("%s 送达认证内核的是 %s，期望 %s", testCase.path, got, testCase.want)
		}
	}
}

// 无法推导权限标识的后台路由必须在构造期失败
func TestContextMiddlewareRejectsUnderivablePath(t *testing.T) {
	authenticator := &recordingAuthenticator{records: map[string]string{}}
	for _, path := range []string{"/admin/base/file/{name}", "/admin/base/sys/user-list"} {
		if _, err := NewContextMiddleware(authenticator, path, false); err == nil {
			t.Errorf("%s 期望构造失败，实际成功", path)
		}
	}
}
