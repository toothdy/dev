package controller

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gsession"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

type closeTrackingReader struct {
	*strings.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

type nilTestResult struct{}

func (*nilTestResult) Write(*ghttp.Request) error { return nil }

func TestActionRuntimeRejectsMissingAndUnsupportedContentType(t *testing.T) {
	server := newActionTestServer(t, "controller-content-type")
	controllers := []Definition{
		Open("test").Route(RouteOptions{
			Name: "content", Method: http.MethodPost, Path: "/content",
			Action: func(request *actionRequest) string { return request.Name },
		}).Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}

	for _, contentType := range []string{"", "text/plain"} {
		request := httptest.NewRequest(http.MethodPost, "/admin/test/content", strings.NewReader(`{"name":"alice"}`))
		if contentType != "" {
			request.Header.Set("Content-Type", contentType)
		}
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnsupportedMediaType || !strings.Contains(recorder.Body.String(), `"code":1002`) {
			t.Fatalf("expected 415 for content type %q, got %d %s", contentType, recorder.Code, recorder.Body.String())
		}
	}
}

func TestActionRuntimeMapsBodyLimitToPayloadTooLarge(t *testing.T) {
	server := newActionTestServerWithBodyLimit(t, "controller-body-limit", 24)
	controllers := []Definition{
		Open("test").Route(RouteOptions{
			Name: "limited", Method: http.MethodPost, Path: "/limited",
			Action: func(request *actionRequest) string { return request.Name },
		}).Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodPost,
		"/admin/test/limited",
		strings.NewReader(`{"name":"this body is deliberately too large"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || recorder.Body.String() != `{"code":1002,"message":"请求体过大"}` {
		t.Fatalf("unexpected payload-too-large response: %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestResultFileStreamAndHeadContracts(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "result.txt")
	if err := os.WriteFile(filePath, []byte("file-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader := &closeTrackingReader{Reader: strings.NewReader("stream-content")}
	server := newActionTestServer(t, "controller-result-contracts")
	controllers := []Definition{
		Open("test").
			Route(RouteOptions{
				Name: "file", Method: http.MethodGet, Path: "/file",
				Action: func() Result { return File(filePath) },
			}).
			Route(RouteOptions{
				Name: "stream", Method: http.MethodGet, Path: "/stream",
				Action: func() Result { return Stream("text/plain", reader) },
			}).
			Route(RouteOptions{
				Name: "head", Method: http.MethodHead, Path: "/head",
				Action: func() Result { return HTML("not-written") },
			}).
			Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}

	fileResponse := serveActionRequest(server, http.MethodGet, "/admin/test/file", "")
	if fileResponse.Body.String() != "file-content" || !strings.HasPrefix(fileResponse.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("unexpected file response: %#v %q", fileResponse.Header(), fileResponse.Body.String())
	}
	streamResponse := serveActionRequest(server, http.MethodGet, "/admin/test/stream", "")
	if streamResponse.Body.String() != "stream-content" || !reader.closed {
		t.Fatalf("unexpected stream response or reader not closed: %q closed=%v", streamResponse.Body.String(), reader.closed)
	}
	headResponse := serveActionRequest(server, http.MethodHead, "/admin/test/head", "")
	if headResponse.Body.Len() != 0 || !strings.HasPrefix(headResponse.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("unexpected HEAD response: %#v %q", headResponse.Header(), headResponse.Body.String())
	}
}

func TestActionRuntimeDistinguishesNilDataAndRejectsTypedNilResult(t *testing.T) {
	server := newActionTestServer(t, "controller-nil-contracts")
	controllers := []Definition{
		Open("test").
			Route(RouteOptions{
				Name: "nil-interface", Method: http.MethodGet, Path: "/nil-interface",
				Action: func() interface{} { return nil },
			}).
			Route(RouteOptions{
				Name: "typed-nil-data", Method: http.MethodGet, Path: "/typed-nil-data",
				Action: func() *actionRequest { return nil },
			}).
			Route(RouteOptions{
				Name: "typed-nil-result", Method: http.MethodGet, Path: "/typed-nil-result",
				Action: func() Result {
					var result *nilTestResult
					return result
				},
			}).
			Build(),
	}
	if err := RegisterRoutes(server, nil, controllers); err != nil {
		t.Fatalf("register routes failed: %v", err)
	}

	assertBody := func(path string, want string) {
		t.Helper()
		response := serveActionRequest(server, http.MethodGet, path, "")
		if response.Body.String() != want {
			t.Fatalf("unexpected response for %s: %s", path, response.Body.String())
		}
	}
	assertBody("/admin/test/nil-interface", `{"code":1000,"message":"success"}`)
	assertBody("/admin/test/typed-nil-data", `{"code":1000,"message":"success","data":null}`)
	assertBody("/admin/test/typed-nil-result", `{"code":1001,"message":"操作失败"}`)
}

func TestCompileRoutePlanRejectsCustomCRUDConflict(t *testing.T) {
	controllers := []Definition{
		Admin("base/sys/user").
			Model(deriveModel()).
			CRUD(CRUDOptions{API: []string{crud.Add}}).
			Route(RouteOptions{
				Name: "custom-add", Method: http.MethodPost, Path: "/add",
				Action: func() string { return "custom" },
			}).
			Build(),
	}
	specs, err := CRUDResourceSpecs(controllers)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := crud.NewRegistry(specs)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileRoutePlan(crud.NewRuntime(nil, registry), controllers)
	if err == nil || !strings.Contains(err.Error(), "路由冲突") {
		t.Fatalf("expected custom/CRUD route conflict, got %v", err)
	}
}

func newActionTestServerWithBodyLimit(t *testing.T, name string, bodyLimit int64) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S() + "-" + name)
	server.SetPort(0)
	server.SetClientMaxBodySize(bodyLimit)
	server.SetSessionStorage(gsession.NewStorageMemory())
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	server.Use(definitions[0].Handler, definitions[1].Handler)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return server
}
