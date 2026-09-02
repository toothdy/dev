package controller_test

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
	area            codegen.ControllerArea
	method          string
	path            string
	bind            route.BindSource
	permission      string
	developmentOnly bool
	ignoreToken     bool
	kind            route.Kind
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
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/eps", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/html", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/open/login", bind: route.BindJSON, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/open/captcha", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/open/refreshToken", bind: route.BindJSON, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/upload/{date}/{name}", bind: route.BindPath, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/person", bind: route.BindQuery, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/personUpdate", bind: route.BindJSON, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/permmenu", bind: route.BindQuery, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/upload", bind: route.BindFile, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/uploadMode", bind: route.BindQuery, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/comm/logout", bind: route.BindJSON, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/comm/program", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/coding/getModuleTree", bind: route.BindQuery, permission: "base:coding:getModuleTree", developmentOnly: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/coding/createCode", bind: route.BindJSON, permission: "base:coding:createCode", developmentOnly: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/parse", bind: route.BindJSON, permission: "base:sys:menu:parse", developmentOnly: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/create", bind: route.BindJSON, permission: "base:sys:menu:create", developmentOnly: true, kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/export", bind: route.BindJSON, permission: "base:sys:menu:export", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/import", bind: route.BindJSON, permission: "base:sys:menu:import", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/add", bind: route.BindJSON, permission: "base:sys:user:add", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/delete", bind: route.BindJSON, permission: "base:sys:user:delete", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/update", bind: route.BindJSON, permission: "base:sys:user:update", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/user/info", bind: route.BindQuery, permission: "base:sys:user:info", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/list", bind: route.BindJSON, permission: "base:sys:user:list", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/page", bind: route.BindJSON, permission: "base:sys:user:page", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/user/move", bind: route.BindJSON, permission: "base:sys:user:move", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/add", bind: route.BindJSON, permission: "base:sys:role:add", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/delete", bind: route.BindJSON, permission: "base:sys:role:delete", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/update", bind: route.BindJSON, permission: "base:sys:role:update", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/role/info", bind: route.BindQuery, permission: "base:sys:role:info", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/list", bind: route.BindJSON, permission: "base:sys:role:list", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/role/page", bind: route.BindJSON, permission: "base:sys:role:page", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/add", bind: route.BindJSON, permission: "base:sys:menu:add", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/delete", bind: route.BindJSON, permission: "base:sys:menu:delete", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/update", bind: route.BindJSON, permission: "base:sys:menu:update", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/menu/info", bind: route.BindQuery, permission: "base:sys:menu:info", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/list", bind: route.BindJSON, permission: "base:sys:menu:list", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/menu/page", bind: route.BindJSON, permission: "base:sys:menu:page", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/add", bind: route.BindJSON, permission: "base:sys:department:add", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/delete", bind: route.BindJSON, permission: "base:sys:department:delete", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/update", bind: route.BindJSON, permission: "base:sys:department:update", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/list", bind: route.BindJSON, permission: "base:sys:department:list", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/department/order", bind: route.BindJSON, permission: "base:sys:department:order", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/add", bind: route.BindJSON, permission: "base:sys:param:add", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/delete", bind: route.BindJSON, permission: "base:sys:param:delete", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/update", bind: route.BindJSON, permission: "base:sys:param:update", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/param/info", bind: route.BindQuery, permission: "base:sys:param:info", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/param/page", bind: route.BindJSON, permission: "base:sys:param:page", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/param/html", bind: route.BindQuery, permission: "base:sys:param:html", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/page", bind: route.BindJSON, permission: "base:sys:log:page", kind: route.KindCRUD},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/clear", bind: route.BindJSON, permission: "base:sys:log:clear", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodPost, path: "/admin/base/sys/log/setKeep", bind: route.BindJSON, permission: "base:sys:log:setKeep", kind: route.KindCustom},
		{area: codegen.ControllerAdmin, method: http.MethodGet, path: "/admin/base/sys/log/getKeep", bind: route.BindQuery, permission: "base:sys:log:getKeep", kind: route.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/param", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/eps", bind: route.BindQuery, ignoreToken: true, kind: route.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodPost, path: "/app/base/comm/upload", bind: route.BindFile, kind: route.KindCustom},
		{area: codegen.ControllerApp, method: http.MethodGet, path: "/app/base/comm/uploadMode", bind: route.BindQuery, kind: route.KindCustom},
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
