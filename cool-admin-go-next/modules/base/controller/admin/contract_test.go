package admin_test

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

type routeContract struct {
	method          string
	path            string
	permission      string
	developmentOnly bool
	ignoreToken     bool
	kind            route.Kind
}

func TestAdminRouteContract(t *testing.T) {
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	model, err := codegen.Analyze(context.Background(), codegen.Options{Dir: root, ModulesRoot: "modules"})
	if err != nil {
		t.Fatal(err)
	}

	want := []routeContract{
		{method: http.MethodGet, path: "/admin/base/comm/person"},
		{method: http.MethodPost, path: "/admin/base/comm/personUpdate"},
		{method: http.MethodGet, path: "/admin/base/comm/permmenu"},
		{method: http.MethodPost, path: "/admin/base/sys/user/add", permission: "base:sys:user:add"},
		{method: http.MethodPost, path: "/admin/base/sys/user/delete", permission: "base:sys:user:delete"},
		{method: http.MethodPost, path: "/admin/base/sys/user/update", permission: "base:sys:user:update"},
		{method: http.MethodGet, path: "/admin/base/sys/user/info", permission: "base:sys:user:info"},
		{method: http.MethodPost, path: "/admin/base/sys/user/list", permission: "base:sys:user:list"},
		{method: http.MethodPost, path: "/admin/base/sys/user/page", permission: "base:sys:user:page"},
		{method: http.MethodPost, path: "/admin/base/sys/user/move", permission: "base:sys:user:move"},
		{method: http.MethodPost, path: "/admin/base/sys/role/add", permission: "base:sys:role:add"},
		{method: http.MethodPost, path: "/admin/base/sys/role/delete", permission: "base:sys:role:delete"},
		{method: http.MethodPost, path: "/admin/base/sys/role/update", permission: "base:sys:role:update"},
		{method: http.MethodGet, path: "/admin/base/sys/role/info", permission: "base:sys:role:info"},
		{method: http.MethodPost, path: "/admin/base/sys/role/list", permission: "base:sys:role:list"},
		{method: http.MethodPost, path: "/admin/base/sys/role/page", permission: "base:sys:role:page"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/add", permission: "base:sys:menu:add"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/delete", permission: "base:sys:menu:delete"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/update", permission: "base:sys:menu:update"},
		{method: http.MethodGet, path: "/admin/base/sys/menu/info", permission: "base:sys:menu:info"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/list", permission: "base:sys:menu:list"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/page", permission: "base:sys:menu:page"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/parse", permission: "base:sys:menu:parse", developmentOnly: true},
		{method: http.MethodPost, path: "/admin/base/sys/menu/create", permission: "base:sys:menu:create", developmentOnly: true},
		{method: http.MethodPost, path: "/admin/base/sys/menu/export", permission: "base:sys:menu:export"},
		{method: http.MethodPost, path: "/admin/base/sys/menu/import", permission: "base:sys:menu:import"},
		{method: http.MethodPost, path: "/admin/base/sys/department/add", permission: "base:sys:department:add"},
		{method: http.MethodPost, path: "/admin/base/sys/department/delete", permission: "base:sys:department:delete"},
		{method: http.MethodPost, path: "/admin/base/sys/department/update", permission: "base:sys:department:update"},
		{method: http.MethodPost, path: "/admin/base/sys/department/list", permission: "base:sys:department:list"},
		{method: http.MethodPost, path: "/admin/base/sys/department/order", permission: "base:sys:department:order"},
		{method: http.MethodPost, path: "/admin/base/sys/param/add", permission: "base:sys:param:add"},
		{method: http.MethodPost, path: "/admin/base/sys/param/delete", permission: "base:sys:param:delete"},
		{method: http.MethodPost, path: "/admin/base/sys/param/update", permission: "base:sys:param:update"},
		{method: http.MethodGet, path: "/admin/base/sys/param/info", permission: "base:sys:param:info"},
		{method: http.MethodPost, path: "/admin/base/sys/param/page", permission: "base:sys:param:page"},
		{method: http.MethodGet, path: "/admin/base/sys/param/html", permission: "base:sys:param:html"},
		{method: http.MethodPost, path: "/admin/base/sys/log/page", permission: "base:sys:log:page"},
		{method: http.MethodPost, path: "/admin/base/sys/log/clear", permission: "base:sys:log:clear"},
		{method: http.MethodPost, path: "/admin/base/sys/log/setKeep", permission: "base:sys:log:setKeep"},
		{method: http.MethodGet, path: "/admin/base/sys/log/getKeep", permission: "base:sys:log:getKeep"},
	}

	routes := make(map[string]routeContract)
	for _, module := range model.Modules() {
		if module.Identity().Key() != "base" {
			continue
		}
		for _, current := range module.Controllers() {
			for _, route := range current.Routes() {
				ignoreToken := slices.Contains(route.Tags(), "ignoreToken")
				permission, err := auth.DerivePermission(route.Path(), ignoreToken)
				if err != nil {
					t.Fatalf("derive permission for %s %s: %v", route.Method(), route.Path(), err)
				}
				routes[route.Method()+" "+route.Path()] = routeContract{
					method:          route.Method(),
					path:            route.Path(),
					permission:      permission,
					developmentOnly: route.DevelopmentOnly(),
					ignoreToken:     ignoreToken,
					kind:            route.Kind(),
				}
			}
		}
	}

	for _, expected := range want {
		key := expected.method + " " + expected.path
		actual, exists := routes[key]
		if !exists {
			t.Errorf("缺少路由 %s", key)
			continue
		}
		if actual.permission != expected.permission || actual.developmentOnly != expected.developmentOnly || actual.ignoreToken != expected.ignoreToken {
			t.Errorf("路由 %s = %#v, want %#v", key, actual, expected)
		}
		if expected.kind != "" && actual.kind != expected.kind {
			t.Errorf("路由 %s kind = %s, want %s", key, actual.kind, expected.kind)
		}
	}
}
