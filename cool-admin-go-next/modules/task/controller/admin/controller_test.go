package admin_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	admin "github.com/toothdy/cool-admin-go-next/modules/task/controller/admin"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
	"github.com/toothdy/cool-admin-go-next/modules/task/service"
)

func snapshot(t *testing.T) controller.DefinitionSnapshot {
	t.Helper()
	result, err := controller.Snapshot(admin.AdminTaskInfoController(&service.InfoService{}))
	if err != nil {
		t.Fatalf("读取 Controller 快照失败: %v", err)
	}

	return result
}

// Node Controller 只开放五个 CRUD 动作，菜单里多出的 list 权限不带来路由
func TestTaskControllerExposesNodeCurdActions(t *testing.T) {
	current := snapshot(t)
	if current.Area != controller.AreaAdmin {
		t.Errorf("区域 = %q，期望 admin", current.Area)
	}
	if current.Curd == nil {
		t.Fatal("缺少 CRUD 声明")
	}
	expected := []controller.APIType{controller.Add, controller.Delete, controller.Update, controller.Info, controller.Page}
	if !slices.Equal(current.Curd.API, expected) {
		t.Errorf("CRUD 动作 = %v，期望 %v", current.Curd.API, expected)
	}
}

func TestTaskControllerPageFiltersStatusAndType(t *testing.T) {
	current := snapshot(t)
	descriptor, err := coreentity.Compile[entity.Info, uint64](entity.InfoSchema())
	if err != nil {
		t.Fatalf("编译任务信息 Descriptor 失败: %v", err)
	}
	resolver := controller.DescriptorResolverFunc(func(value any) (coreentity.Metadata, bool) {
		_, matches := value.(entity.Info)

		return descriptor, matches
	})
	projection, static, err := controller.ProjectQuery(current.Curd.PageQueryOp, resolver, entity.Info{})
	if err != nil {
		t.Fatalf("投影分页查询失败: %v", err)
	}
	if !static {
		t.Fatal("分页查询必须可静态投影")
	}
	columns := make([]string, len(projection.FieldEq))
	for index, field := range projection.FieldEq {
		columns[index] = field.RequestParam
	}
	if !slices.Equal(columns, []string{"status", "type"}) {
		t.Errorf("等值过滤 = %v，期望 status 与 type", columns)
	}
}

func TestTaskControllerCustomRoutes(t *testing.T) {
	expected := map[string]string{
		"/once":  http.MethodPost,
		"/stop":  http.MethodPost,
		"/start": http.MethodPost,
		"/log":   http.MethodGet,
	}
	routes := snapshot(t).Routes
	if len(routes) != len(expected) {
		t.Fatalf("自定义路由数 = %d，期望 %d", len(routes), len(expected))
	}
	for _, route := range routes {
		method, exists := expected[route.Path]
		if !exists {
			t.Errorf("多出路由 %s", route.Path)
			continue
		}
		if route.Method != method {
			t.Errorf("%s 方法 = %s，期望 %s", route.Path, route.Method, method)
		}
		if len(route.Tags) != 0 {
			t.Errorf("%s 不应带标签，任务模块没有公开接口", route.Path)
		}
	}
}

// 前端按 EPS 路径推导权限，四条自定义路由必须落在菜单已有的权限标识上
func TestTaskRoutesDerivePermissionInMenu(t *testing.T) {
	expected := map[string]string{
		"/admin/task/info/add":    "task:info:add",
		"/admin/task/info/delete": "task:info:delete",
		"/admin/task/info/update": "task:info:update",
		"/admin/task/info/info":   "task:info:info",
		"/admin/task/info/page":   "task:info:page",
		"/admin/task/info/once":   "task:info:once",
		"/admin/task/info/stop":   "task:info:stop",
		"/admin/task/info/start":  "task:info:start",
		"/admin/task/info/log":    "task:info:log",
	}
	for path, want := range expected {
		got, err := auth.DerivePermission(path, false)
		if err != nil {
			t.Errorf("%s 推导失败: %v", path, err)
			continue
		}
		if got != want {
			t.Errorf("%s 权限 = %q，期望 %q", path, got, want)
		}
	}
}
