package modules

import (
	"slices"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
)

// 权限推导落地前，真实路由图上每条路由的路径、ignoreToken 与权限标识快照。
// 这六条工具路由当时权限标识为空，改为在 handler 内做超管校验：
//
//	/admin/base/coding/{getModuleTree,createCode}
//	/admin/base/sys/menu/{parse,create,export,import}
//
// 它们是本次改动中唯一允许推导值与快照不同的路由，期望值见 toolRoutePermissions。
var permissionGolden = []struct {
	path        string
	ignoreToken bool
	permission  string
}{
	{path: "/admin/base/coding/createCode", ignoreToken: false, permission: ""},
	{path: "/admin/base/coding/getModuleTree", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/logout", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/permmenu", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/person", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/personUpdate", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/program", ignoreToken: true, permission: ""},
	{path: "/admin/base/comm/upload", ignoreToken: false, permission: ""},
	{path: "/admin/base/comm/uploadMode", ignoreToken: false, permission: ""},
	{path: "/admin/base/open/captcha", ignoreToken: true, permission: ""},
	{path: "/admin/base/open/eps", ignoreToken: true, permission: ""},
	{path: "/admin/base/open/html", ignoreToken: true, permission: ""},
	{path: "/admin/base/open/login", ignoreToken: true, permission: ""},
	{path: "/admin/base/open/refreshToken", ignoreToken: true, permission: ""},
	{path: "/admin/base/sys/department/add", ignoreToken: false, permission: "base:sys:department:add"},
	{path: "/admin/base/sys/department/delete", ignoreToken: false, permission: "base:sys:department:delete"},
	{path: "/admin/base/sys/department/list", ignoreToken: false, permission: "base:sys:department:list"},
	{path: "/admin/base/sys/department/order", ignoreToken: false, permission: "base:sys:department:order"},
	{path: "/admin/base/sys/department/update", ignoreToken: false, permission: "base:sys:department:update"},
	{path: "/admin/base/sys/log/clear", ignoreToken: false, permission: "base:sys:log:clear"},
	{path: "/admin/base/sys/log/getKeep", ignoreToken: false, permission: "base:sys:log:getKeep"},
	{path: "/admin/base/sys/log/page", ignoreToken: false, permission: "base:sys:log:page"},
	{path: "/admin/base/sys/log/setKeep", ignoreToken: false, permission: "base:sys:log:setKeep"},
	{path: "/admin/base/sys/menu/add", ignoreToken: false, permission: "base:sys:menu:add"},
	{path: "/admin/base/sys/menu/create", ignoreToken: false, permission: ""},
	{path: "/admin/base/sys/menu/delete", ignoreToken: false, permission: "base:sys:menu:delete"},
	{path: "/admin/base/sys/menu/export", ignoreToken: false, permission: ""},
	{path: "/admin/base/sys/menu/import", ignoreToken: false, permission: ""},
	{path: "/admin/base/sys/menu/info", ignoreToken: false, permission: "base:sys:menu:info"},
	{path: "/admin/base/sys/menu/list", ignoreToken: false, permission: "base:sys:menu:list"},
	{path: "/admin/base/sys/menu/page", ignoreToken: false, permission: "base:sys:menu:page"},
	{path: "/admin/base/sys/menu/parse", ignoreToken: false, permission: ""},
	{path: "/admin/base/sys/menu/update", ignoreToken: false, permission: "base:sys:menu:update"},
	{path: "/admin/base/sys/param/add", ignoreToken: false, permission: "base:sys:param:add"},
	{path: "/admin/base/sys/param/delete", ignoreToken: false, permission: "base:sys:param:delete"},
	{path: "/admin/base/sys/param/html", ignoreToken: false, permission: "base:sys:param:html"},
	{path: "/admin/base/sys/param/info", ignoreToken: false, permission: "base:sys:param:info"},
	{path: "/admin/base/sys/param/page", ignoreToken: false, permission: "base:sys:param:page"},
	{path: "/admin/base/sys/param/update", ignoreToken: false, permission: "base:sys:param:update"},
	{path: "/admin/base/sys/role/add", ignoreToken: false, permission: "base:sys:role:add"},
	{path: "/admin/base/sys/role/delete", ignoreToken: false, permission: "base:sys:role:delete"},
	{path: "/admin/base/sys/role/info", ignoreToken: false, permission: "base:sys:role:info"},
	{path: "/admin/base/sys/role/list", ignoreToken: false, permission: "base:sys:role:list"},
	{path: "/admin/base/sys/role/page", ignoreToken: false, permission: "base:sys:role:page"},
	{path: "/admin/base/sys/role/update", ignoreToken: false, permission: "base:sys:role:update"},
	{path: "/admin/base/sys/user/add", ignoreToken: false, permission: "base:sys:user:add"},
	{path: "/admin/base/sys/user/delete", ignoreToken: false, permission: "base:sys:user:delete"},
	{path: "/admin/base/sys/user/info", ignoreToken: false, permission: "base:sys:user:info"},
	{path: "/admin/base/sys/user/list", ignoreToken: false, permission: "base:sys:user:list"},
	{path: "/admin/base/sys/user/move", ignoreToken: false, permission: "base:sys:user:move"},
	{path: "/admin/base/sys/user/page", ignoreToken: false, permission: "base:sys:user:page"},
	{path: "/admin/base/sys/user/update", ignoreToken: false, permission: "base:sys:user:update"},
	{path: "/admin/dict/info/add", ignoreToken: false, permission: "dict:info:add"},
	{path: "/admin/dict/info/data", ignoreToken: false, permission: ""},
	{path: "/admin/dict/info/delete", ignoreToken: false, permission: "dict:info:delete"},
	{path: "/admin/dict/info/info", ignoreToken: false, permission: "dict:info:info"},
	{path: "/admin/dict/info/list", ignoreToken: false, permission: "dict:info:list"},
	{path: "/admin/dict/info/page", ignoreToken: false, permission: "dict:info:page"},
	{path: "/admin/dict/info/types", ignoreToken: true, permission: ""},
	{path: "/admin/dict/info/update", ignoreToken: false, permission: "dict:info:update"},
	{path: "/admin/dict/type/add", ignoreToken: false, permission: "dict:type:add"},
	{path: "/admin/dict/type/delete", ignoreToken: false, permission: "dict:type:delete"},
	{path: "/admin/dict/type/info", ignoreToken: false, permission: "dict:type:info"},
	{path: "/admin/dict/type/list", ignoreToken: false, permission: "dict:type:list"},
	{path: "/admin/dict/type/page", ignoreToken: false, permission: "dict:type:page"},
	{path: "/admin/dict/type/update", ignoreToken: false, permission: "dict:type:update"},
	{path: "/admin/task/info/add", ignoreToken: false, permission: "task:info:add"},
	{path: "/admin/task/info/delete", ignoreToken: false, permission: "task:info:delete"},
	{path: "/admin/task/info/info", ignoreToken: false, permission: "task:info:info"},
	{path: "/admin/task/info/log", ignoreToken: false, permission: "task:info:log"},
	{path: "/admin/task/info/once", ignoreToken: false, permission: "task:info:once"},
	{path: "/admin/task/info/page", ignoreToken: false, permission: "task:info:page"},
	{path: "/admin/task/info/start", ignoreToken: false, permission: "task:info:start"},
	{path: "/admin/task/info/stop", ignoreToken: false, permission: "task:info:stop"},
	{path: "/admin/task/info/update", ignoreToken: false, permission: "task:info:update"},
	{path: "/app/base/comm/eps", ignoreToken: true, permission: ""},
	{path: "/app/base/comm/param", ignoreToken: true, permission: ""},
	{path: "/app/base/comm/upload", ignoreToken: false, permission: ""},
	{path: "/app/base/comm/uploadMode", ignoreToken: false, permission: ""},
	{path: "/app/dict/info/data", ignoreToken: true, permission: ""},
	{path: "/app/dict/info/types", ignoreToken: true, permission: ""},
	{path: "/upload/{date}/{name}", ignoreToken: true, permission: ""},
}

// 六条工具路由推导后应得到的权限标识
var toolRoutePermissions = map[string]string{
	"/admin/base/coding/getModuleTree": "base:coding:getModuleTree",
	"/admin/base/coding/createCode":    "base:coding:createCode",
	"/admin/base/sys/menu/parse":       "base:sys:menu:parse",
	"/admin/base/sys/menu/create":      "base:sys:menu:create",
	"/admin/base/sys/menu/export":      "base:sys:menu:export",
	"/admin/base/sys/menu/import":      "base:sys:menu:import",
}

// 安全网：推导值必须与权限删除前的快照逐条相同。
// 该测试不依赖 Route.Permission()，因此在字段删除后仍然有效。
func TestDerivePermissionMatchesGolden(t *testing.T) {
	for _, want := range permissionGolden {
		expected := want.permission
		if tool, isTool := toolRoutePermissions[want.path]; isTool {
			expected = tool
		}
		got, err := auth.DerivePermission(want.path, want.ignoreToken)
		if err != nil {
			t.Errorf("%s 推导失败: %v", want.path, err)
			continue
		}
		if got != expected {
			t.Errorf("%s 推导值 = %q，期望 %q", want.path, got, expected)
		}
	}
}

// 快照必须覆盖真实路由图的全部路由，防止新增路由绕过安全网
func TestPermissionGoldenCoversGeneratedGraph(t *testing.T) {
	routes := generatedGraph().Routes().Routes()
	if len(routes) != len(permissionGolden) {
		t.Fatalf("路由数 = %d，快照 %d 条，请更新快照", len(routes), len(permissionGolden))
	}
	for _, route := range routes {
		index := slices.IndexFunc(permissionGolden, func(row struct {
			path        string
			ignoreToken bool
			permission  string
		}) bool {
			return row.path == route.Path()
		})
		if index < 0 {
			t.Errorf("路由 %s 不在快照中", route.Path())
			continue
		}
		if got := slices.Contains(route.Tags(), "ignoreToken"); got != permissionGolden[index].ignoreToken {
			t.Errorf("%s ignoreToken = %v，快照 %v", route.Path(), got, permissionGolden[index].ignoreToken)
		}
	}
}
