package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/util/guid"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
)

func TestRegisterRejectsDuplicateNames(t *testing.T) {
	server := ghttp.GetServer(guid.S())
	handler := func(r *ghttp.Request) { r.Middleware.Next() }
	err := middleware.Register(server, []middleware.Definition{
		{Name: "base.authority", Handler: handler},
		{Name: "base.authority", Handler: handler},
	})
	if err == nil || !strings.Contains(err.Error(), "base.authority") {
		t.Fatalf("expected duplicate name error, got %v", err)
	}
}

func TestValidateRejectsReservedNamesAndOrders(t *testing.T) {
	handler := func(r *ghttp.Request) { r.Middleware.Next() }
	for _, definition := range []middleware.Definition{
		{Name: "cool.custom", Order: 250, Handler: handler},
		{Name: middleware.RecoveryName, Order: middleware.RecoveryOrder, Handler: handler},
		{Name: "base.authority", Order: 199, Handler: handler},
	} {
		if _, err := middleware.Validate([]middleware.Definition{definition}); err == nil {
			t.Fatalf("expected reserved middleware rejected: %#v", definition)
		}
	}
}

func TestValidateAcceptsCoreDefinitions(t *testing.T) {
	definitions := middleware.CoreErrorDefinitions(middleware.ErrorBoundaryOptions{})
	if _, err := middleware.Validate(definitions); err != nil {
		t.Fatalf("expected framework core definitions accepted: %v", err)
	}
}

func TestValidateModuleRejectsGlobalCoreOrderException(t *testing.T) {
	handler := func(r *ghttp.Request) { r.Middleware.Next() }
	_, err := middleware.ValidateModule([]middleware.Definition{{
		Name: "base.translate", Order: 100, Handler: handler,
	}})
	if err == nil || !strings.Contains(err.Error(), "base.translate") {
		t.Fatalf("expected module middleware core order rejected: %v", err)
	}
}

func TestRegisterUsesStableOrder(t *testing.T) {
	server := ghttp.GetServer(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	calls := []string{}
	definition := func(name string, order int) middleware.Definition {
		return middleware.Definition{
			Name:  name,
			Order: order,
			Handler: func(r *ghttp.Request) {
				calls = append(calls, name)
				r.Middleware.Next()
			},
		}
	}
	if err := middleware.Register(server, []middleware.Definition{
		definition("log", 400),
		definition("base.translate", 100),
		definition("authority", 200),
		definition("permission", 300),
		definition("permission-second", 300),
	}); err != nil {
		t.Fatalf("register middleware failed: %v", err)
	}
	server.BindHandler("/order", func(r *ghttp.Request) {
		calls = append(calls, "handler")
		r.Response.Write("ok")
	})
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/order", nil))
	want := []string{"base.translate", "authority", "permission", "permission-second", "log", "handler"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unexpected middleware order: got %#v want %#v", calls, want)
	}
}
