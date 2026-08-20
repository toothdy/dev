package middleware_test

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
)

type recordedError struct {
	resolved exception.Resolved
	err      error
}

type recordingErrorLogger struct {
	items []recordedError
}

func (l *recordingErrorLogger) Log(_ context.Context, resolved exception.Resolved, err error) {
	l.items = append(l.items, recordedError{resolved: resolved, err: err})
}

func TestCoreErrorBoundaryRendersTypedAndInternalErrors(t *testing.T) {
	logger := &recordingErrorLogger{}
	server := newErrorServer(t, logger, nil)
	server.BindHandler("GET:/business", func(r *ghttp.Request) {
		r.SetError(exception.Comm("数据不存在"))
	})
	server.BindHandler("GET:/internal", func(r *ghttp.Request) {
		r.SetError(stderrors.New("sql secret"))
	})

	business := serveError(server, "/business")
	if business.Code != http.StatusOK || business.Body.String() != `{"code":1001,"message":"数据不存在"}` {
		t.Fatalf("unexpected business response: %d %s", business.Code, business.Body.String())
	}
	internal := serveError(server, "/internal")
	if internal.Code != http.StatusOK || internal.Body.String() != `{"code":1001,"message":"操作失败"}` {
		t.Fatalf("unexpected internal response: %d %s", internal.Code, internal.Body.String())
	}
	if len(logger.items) != 2 || logger.items[1].resolved.Kind != exception.KindInternal {
		t.Fatalf("unexpected logged errors: %#v", logger.items)
	}
}

func TestRecoveryCatchesTranslatePanic(t *testing.T) {
	logger := &recordingErrorLogger{}
	outer := middleware.Definition{
		Name: "base.translate", Order: 100,
		Handler: func(r *ghttp.Request) {
			r.Middleware.Next()
			panic("translate failed")
		},
	}
	server := newErrorServer(t, logger, []middleware.Definition{outer})
	server.BindHandler("GET:/panic", func(r *ghttp.Request) { r.Response.Write("ok") })

	response := serveError(server, "/panic")
	if response.Body.String() != `{"code":1001,"message":"操作失败"}` {
		t.Fatalf("unexpected panic response: %d %s", response.Code, response.Body.String())
	}
	if len(logger.items) != 1 || logger.items[0].resolved.Kind != exception.KindInternal {
		t.Fatalf("unexpected panic log: %#v", logger.items)
	}
}

func newErrorServer(t *testing.T, logger middleware.ErrorLogger, extra []middleware.Definition) *ghttp.Server {
	t.Helper()
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{Logger: logger})
	definitions = append(definitions, extra...)
	if err := middleware.Register(server, definitions); err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Shutdown() })
	return server
}

func serveError(server *ghttp.Server, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	return recorder
}
