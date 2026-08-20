package controller

import (
	"net/http"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

/**
 * 测试 Admin 构建器生成完整前缀
 * @param t 测试对象
 * @returns null
 */
func TestAdminBuilderBuildsPrefix(t *testing.T) {
	definition := Admin("base/sys/user").
		Name("BaseSysUserEntity").
		Description("系统用户").
		Model(entity.NewDefinition("base", "BaseSysUser", "base_sys_user")).
		CRUD(CRUDOptions{API: []string{crud.Page}}).
		Build()

	if definition.Module != "base" {
		t.Fatalf("expected module base, got %s", definition.Module)
	}
	if definition.Area != AreaAdmin {
		t.Fatalf("expected admin area, got %s", definition.Area)
	}
	if definition.Prefix != "/admin/base/sys/user" {
		t.Fatalf("expected prefix /admin/base/sys/user, got %s", definition.Prefix)
	}
	if definition.CRUD == nil || len(definition.CRUD.API) != 1 || definition.CRUD.API[0] != crud.Page {
		t.Fatalf("expected page CRUD metadata, got %#v", definition.CRUD)
	}
}

/**
 * 测试 Route 构建器生成完整路径
 * @param t 测试对象
 * @returns null
 */
func TestRouteBuildsFullPath(t *testing.T) {
	definition := Open("base/open").
		Route(RouteOptions{
			Name:       "login",
			Method:     http.MethodPost,
			Path:       "/login",
			IgnoreAuth: true,
		}).
		Build()

	if len(definition.Routes) != 1 {
		t.Fatalf("expected one route, got %d", len(definition.Routes))
	}
	route := definition.Routes[0]
	if route.FullPath != "/admin/base/open/login" {
		t.Fatalf("expected full path /admin/base/open/login, got %s", route.FullPath)
	}
	if !route.IgnoreAuth {
		t.Fatal("expected route to ignore auth")
	}
}

func TestAppBuilderBuildsAppPrefix(t *testing.T) {
	definition := App("base/comm").Route(RouteOptions{Name: "eps", Method: http.MethodGet, Path: "/eps"}).Build()
	if definition.Area != AreaApp || definition.Prefix != "/app/base/comm" {
		t.Fatalf("unexpected app definition: %#v", definition)
	}
	if definition.Routes[0].FullPath != "/app/base/comm/eps" {
		t.Fatalf("unexpected app route: %#v", definition.Routes[0])
	}
}

/**
 * 测试构建器复制切片避免外部修改
 * @param t 测试对象
 * @returns null
 */
func TestBuilderCopiesCRUDSlices(t *testing.T) {
	api := []string{crud.Add}
	definition := Admin("base/sys/role").CRUD(CRUDOptions{API: api}).Build()
	api[0] = crud.Delete

	if definition.CRUD.API[0] != crud.Add {
		t.Fatalf("expected copied api add, got %s", definition.CRUD.API[0])
	}
}

/**
 * 测试 Build 返回 CRUD 快照避免污染构建器状态
 * @param t 测试对象
 * @returns null
 */
func TestBuilderBuildReturnsCRUDSnapshot(t *testing.T) {
	builder := Admin("base/sys/user").CRUD(CRUDOptions{
		API: []string{crud.Page, crud.Info},
	})

	definition := builder.Build()
	definition.CRUD.API[0] = crud.Add

	nextDefinition := builder.Build()
	if nextDefinition.CRUD.API[0] != crud.Page {
		t.Fatalf("expected original CRUD API %s, got %s", crud.Page, nextDefinition.CRUD.API[0])
	}
}

/**
 * 测试构建器保存并返回模型切片快照
 * @param t 测试对象
 * @returns null
 */
func TestBuilderCopiesModelSlices(t *testing.T) {
	sourceModel := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Fields([]entity.Field{
			entity.NewField("status", "status", "tinyint").WithDict("启用", "禁用"),
		}).
		WithIndexes(entity.NewIndex("idx_status", "status")).
		WithTenantMode(entity.TenantModeRequired)

	builder := Admin("base/sys/user").Model(sourceModel)

	sourceModel.FieldsValue[0].JSONName = "changed_status"
	sourceModel.FieldsValue[0].Dict[0] = "已变更"
	sourceModel.Indexes[0].Name = "idx_changed_status"
	sourceModel.Indexes[0].Columns[0] = "changed_status"

	definition := builder.Build()
	if definition.Model.FieldsValue[0].JSONName != "status" {
		t.Fatalf("expected original model field name status, got %s", definition.Model.FieldsValue[0].JSONName)
	}
	if definition.Model.FieldsValue[0].Dict[0] != "启用" {
		t.Fatalf("expected original field dict value 启用, got %s", definition.Model.FieldsValue[0].Dict[0])
	}
	if definition.Model.Indexes[0].Name != "idx_status" {
		t.Fatalf("expected original index name idx_status, got %s", definition.Model.Indexes[0].Name)
	}
	if definition.Model.Indexes[0].Columns[0] != "status" {
		t.Fatalf("expected original index column status, got %s", definition.Model.Indexes[0].Columns[0])
	}
	if definition.Model.TenantMode != entity.TenantModeRequired {
		t.Fatalf("expected required tenant mode, got %d", definition.Model.TenantMode)
	}

	definition.Model.FieldsValue[0].JSONName = "returned_status"
	definition.Model.FieldsValue[0].Dict[0] = "返回污染"
	definition.Model.Indexes[0].Name = "idx_returned_status"
	definition.Model.Indexes[0].Columns[0] = "returned_status"

	nextDefinition := builder.Build()
	if nextDefinition.Model.FieldsValue[0].JSONName != "status" {
		t.Fatalf("expected build snapshot field name status, got %s", nextDefinition.Model.FieldsValue[0].JSONName)
	}
	if nextDefinition.Model.FieldsValue[0].Dict[0] != "启用" {
		t.Fatalf("expected build snapshot dict value 启用, got %s", nextDefinition.Model.FieldsValue[0].Dict[0])
	}
	if nextDefinition.Model.Indexes[0].Name != "idx_status" {
		t.Fatalf("expected build snapshot index name idx_status, got %s", nextDefinition.Model.Indexes[0].Name)
	}
	if nextDefinition.Model.Indexes[0].Columns[0] != "status" {
		t.Fatalf("expected build snapshot index column status, got %s", nextDefinition.Model.Indexes[0].Columns[0])
	}
}
