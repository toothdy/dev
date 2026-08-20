package controller

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gsession"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
)

type actionRequest struct {
	Name string `json:"name" v:"required#名称不能为空"`
}

func TestCompileRoutePlanAcceptsSupportedActionSignatures(t *testing.T) {
	actions := []interface{}{
		func() string { return "ok" },
		func() error { return nil },
		func() (string, error) { return "ok", nil },
		func(context.Context) string { return "ok" },
		func(*actionRequest) error { return nil },
		func(context.Context, *actionRequest) (string, error) { return "ok", nil },
		func() Result { return NoContent() },
	}
	for index, action := range actions {
		controllers := []Definition{
			Open("test").Route(RouteOptions{
				Name: "action", Method: http.MethodPost,
				Path: "/action-" + string(rune('a'+index)), Action: action,
			}).Build(),
		}
		if _, err := CompileRoutePlan(nil, controllers); err != nil {
			t.Fatalf("signature %d should compile: %v", index, err)
		}
	}
}

func TestCompileRoutePlanRejectsInvalidActions(t *testing.T) {
	var nilFunction func() string
	actions := []interface{}{
		nil,
		"not a function",
		nilFunction,
		func(string) string { return "" },
		func(*actionRequest, context.Context) string { return "" },
		func() {},
		func() (string, *int) { return "", nil },
	}
	for index, action := range actions {
		controllers := []Definition{
			Open("test").Route(RouteOptions{
				Name: "invalid", Method: http.MethodGet,
				Path: "/invalid", Action: action,
			}).Build(),
		}
		if _, err := CompileRoutePlan(nil, controllers); err == nil {
			t.Fatalf("invalid signature %d should fail during compile", index)
		}
	}
}

func TestCompileRoutePlanRejectsDuplicateCanonicalRoute(t *testing.T) {
	controllers := []Definition{
		Open("test").Route(RouteOptions{
			Name: "one", Method: http.MethodGet, Path: "/same/",
			Action: func() string { return "one" },
		}).Route(RouteOptions{
			Name: "two", Method: http.MethodGet, Path: "//same",
			Action: func() string { return "two" },
		}).Build(),
	}
	if _, err := CompileRoutePlan(nil, controllers); err == nil || !strings.Contains(err.Error(), "路由冲突") {
		t.Fatalf("expected route conflict, got %v", err)
	}
}

func TestActionRuntimeBindsStrictJSONAndWritesSuccess(t *testing.T) {
	server := newActionTestServer(t, "controller-action-json")
	controllers := []Definition{
		Open("test").Route(RouteOptions{
			Name: "hello", Method: http.MethodPost, Path: "/hello",
			Action: func(ctx context.Context, request *actionRequest) (string, error) {
				return request.Name, nil
			},
		}).Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}

	rec := serveActionRequest(server, http.MethodPost, "/admin/test/hello", `{"name":"alice"}`)
	if rec.Code != http.StatusOK || rec.Body.String() != `{"code":1000,"message":"success","data":"alice"}` {
		t.Fatalf("unexpected success response: %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{
		`{"name":"alice","extra":true}`,
		`{"name":"alice","name":"bob"}`,
		`{"name":"alice"} {"name":"bob"}`,
	} {
		rec = serveActionRequest(server, http.MethodPost, "/admin/test/hello", body)
		if rec.Body.String() == `{"code":1000,"message":"success","data":"alice"}` ||
			!strings.Contains(rec.Body.String(), `"code":1002`) {
			t.Fatalf("expected strict JSON validation failure for %q, got %s", body, rec.Body.String())
		}
	}
}

func TestActionRuntimeDoesNotExposeInternalError(t *testing.T) {
	server := newActionTestServer(t, "controller-action-error")
	controllers := []Definition{
		Open("test").Route(RouteOptions{
			Name: "fail", Method: http.MethodGet, Path: "/fail",
			Action: func() (string, error) {
				return "", context.Canceled
			},
		}).Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}
	rec := serveActionRequest(server, http.MethodGet, "/admin/test/fail", "")
	if rec.Body.String() != `{"code":1001,"message":"操作失败"}` {
		t.Fatalf("unexpected internal error response: %s", rec.Body.String())
	}
}

func TestResultWriters(t *testing.T) {
	server := newActionTestServer(t, "controller-action-results")
	controllers := []Definition{
		Open("test").
			Route(RouteOptions{
				Name: "html", Method: http.MethodGet, Path: "/html",
				Action: func() Result { return HTML("<b>ok</b>") },
			}).
			Route(RouteOptions{
				Name: "redirect", Method: http.MethodGet, Path: "/redirect",
				Action: func() Result { return Redirect("/target", http.StatusSeeOther) },
			}).
			Route(RouteOptions{
				Name: "empty", Method: http.MethodGet, Path: "/empty",
				Action: func() Result { return NoContent() },
			}).
			Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}
	rec := serveActionRequest(server, http.MethodGet, "/admin/test/html", "")
	if rec.Body.String() != "<b>ok</b>" || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("unexpected HTML response: %#v %q", rec.Header(), rec.Body.String())
	}
	rec = serveActionRequest(server, http.MethodGet, "/admin/test/redirect", "")
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/target" {
		t.Fatalf("unexpected redirect: %d %#v", rec.Code, rec.Header())
	}
	rec = serveActionRequest(server, http.MethodGet, "/admin/test/empty", "")
	if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("unexpected no-content response: %d %q", rec.Code, rec.Body.String())
	}
}

func newActionTestServer(t *testing.T, name string) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(name)
	server.SetPort(0)
	server.SetSessionStorage(gsession.NewStorageMemory())
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	server.Use(definitions[0].Handler, definitions[1].Handler)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return server
}

func serveActionRequest(server *ghttp.Server, method string, path string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	return rec
}
