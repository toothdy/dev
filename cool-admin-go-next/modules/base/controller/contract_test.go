package controller_test

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	coreroute "github.com/toothdy/cool-admin-go-next/cool-next/core/route"
)

type routeContract struct {
	area            codegen.ControllerArea
	method          string
	path            string
	bind            coreroute.BindSource
	permission      string
	developmentOnly bool
	ignoreToken     bool
	kind            coreroute.Kind
}

func TestBaseControllerContracts(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	model, err := codegen.Analyze(context.Background(), codegen.Options{Dir: root, ModulesRoot: "modules"})
	if err != nil {
		t.Fatal(err)
	}

	want := []routeContract{
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/eps", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/html", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/open/login", bind: coreroute.BindJSON, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/captcha", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/open/refreshToken", bind: coreroute.BindJSON, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/upload/{date}/{name}", bind: coreroute.BindPath, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/person", bind: coreroute.BindQuery, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/personUpdate", bind: coreroute.BindJSON, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/permmenu", bind: coreroute.BindQuery, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/upload", bind: coreroute.BindFile, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/uploadMode", bind: coreroute.BindQuery, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/logout", bind: coreroute.BindJSON, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/program", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/coding/getModuleTree", bind: coreroute.BindQuery, permission: "base:coding:getModuleTree", developmentOnly: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/coding/createCode", bind: coreroute.BindJSON, permission: "base:coding:createCode", developmentOnly: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/parse", bind: coreroute.BindJSON, permission: "base:sys:menu:parse", developmentOnly: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/create", bind: coreroute.BindJSON, permission: "base:sys:menu:create", developmentOnly: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/export", bind: coreroute.BindJSON, permission: "base:sys:menu:export", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/import", bind: coreroute.BindJSON, permission: "base:sys:menu:import", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/add", bind: coreroute.BindJSON, permission: "base:sys:user:add", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/delete", bind: coreroute.BindJSON, permission: "base:sys:user:delete", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/update", bind: coreroute.BindJSON, permission: "base:sys:user:update", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/user/info", bind: coreroute.BindQuery, permission: "base:sys:user:info", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/list", bind: coreroute.BindJSON, permission: "base:sys:user:list", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/page", bind: coreroute.BindJSON, permission: "base:sys:user:page", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/move", bind: coreroute.BindJSON, permission: "base:sys:user:move", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/add", bind: coreroute.BindJSON, permission: "base:sys:role:add", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/delete", bind: coreroute.BindJSON, permission: "base:sys:role:delete", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/update", bind: coreroute.BindJSON, permission: "base:sys:role:update", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/role/info", bind: coreroute.BindQuery, permission: "base:sys:role:info", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/list", bind: coreroute.BindJSON, permission: "base:sys:role:list", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/page", bind: coreroute.BindJSON, permission: "base:sys:role:page", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/add", bind: coreroute.BindJSON, permission: "base:sys:menu:add", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/delete", bind: coreroute.BindJSON, permission: "base:sys:menu:delete", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/update", bind: coreroute.BindJSON, permission: "base:sys:menu:update", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/menu/info", bind: coreroute.BindQuery, permission: "base:sys:menu:info", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/list", bind: coreroute.BindJSON, permission: "base:sys:menu:list", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/page", bind: coreroute.BindJSON, permission: "base:sys:menu:page", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/add", bind: coreroute.BindJSON, permission: "base:sys:department:add", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/delete", bind: coreroute.BindJSON, permission: "base:sys:department:delete", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/update", bind: coreroute.BindJSON, permission: "base:sys:department:update", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/list", bind: coreroute.BindJSON, permission: "base:sys:department:list", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/order", bind: coreroute.BindJSON, permission: "base:sys:department:order", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/add", bind: coreroute.BindJSON, permission: "base:sys:param:add", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/delete", bind: coreroute.BindJSON, permission: "base:sys:param:delete", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/update", bind: coreroute.BindJSON, permission: "base:sys:param:update", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/param/info", bind: coreroute.BindQuery, permission: "base:sys:param:info", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/page", bind: coreroute.BindJSON, permission: "base:sys:param:page", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/param/html", bind: coreroute.BindQuery, permission: "base:sys:param:html", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/page", bind: coreroute.BindJSON, permission: "base:sys:log:page", kind: coreroute.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/clear", bind: coreroute.BindJSON, permission: "base:sys:log:clear", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/setKeep", bind: coreroute.BindJSON, permission: "base:sys:log:setKeep", kind: coreroute.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/log/getKeep", bind: coreroute.BindQuery, permission: "base:sys:log:getKeep", kind: coreroute.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/param", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/eps", bind: coreroute.BindQuery, ignoreToken: true, kind: coreroute.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodPost, path: "/app/base/comm/upload", bind: coreroute.BindFile, kind: coreroute.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/uploadMode", bind: coreroute.BindQuery, kind: coreroute.KindCustom},
	}

	base := findBaseModule(t, model)
	routes := make(map[string]routeContract)
	for _, controller := range base.Controllers() {
		for _, route := range controller.Routes() {
			key := route.Method() + " " + route.Path()
			if _, exists := routes[key]; exists {
				t.Fatalf("duplicate route %s", key)
			}
			ignoreToken := slices.Contains(route.Tags(), "ignoreToken")
			permission, err := auth.DerivePermission(route.Path(), ignoreToken)
			if err != nil {
				t.Fatalf("derive permission for %s: %v", key, err)
			}
			routes[key] = routeContract{
				area:            controller.Area(),
				method:          route.Method(),
				path:            route.Path(),
				bind:            route.Bind(),
				permission:      permission,
				developmentOnly: route.DevelopmentOnly(),
				ignoreToken:     ignoreToken,
				kind:            route.Kind(),
			}
		}
	}
	if len(routes) != len(want) {
		t.Fatalf("Base routes = %d, want %d", len(routes), len(want))
	}
	for _, expected := range want {
		key := expected.method + " " + expected.path
		actual, exists := routes[key]
		if !exists {
			t.Errorf("missing route %s", key)
			continue
		}
		if actual != expected {
			t.Errorf("route %s = %#v, want %#v", key, actual, expected)
		}
	}
}

func findBaseModule(t *testing.T, model *codegen.Model) codegen.Module {
	t.Helper()
	for _, current := range model.Modules() {
		if current.Identity().Key() == "base" {
			return current
		}
	}
	t.Fatal("Base module missing")

	return codegen.Module{}
}

func hasTag(values []string, target string) bool {
	return slices.Contains(values, target)
}
