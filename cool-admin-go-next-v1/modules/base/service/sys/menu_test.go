package sys

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

func TestApplyMenuTenantOverridesNestedClientValues(t *testing.T) {
	clientTenant := int64(99)
	menus := []dto.MenuImportItem{{Name: "root", TenantID: &clientTenant, ChildMenus: []dto.MenuImportItem{{Name: "child", TenantID: &clientTenant}}}}
	contextTenant := int64(7)
	result := applyMenuTenant(menus, &contextTenant)
	if result[0].TenantID == nil || *result[0].TenantID != 7 || result[0].ChildMenus[0].TenantID == nil || *result[0].ChildMenus[0].TenantID != 7 {
		t.Fatalf("context tenant did not replace nested values: %#v", result)
	}
	if *menus[0].TenantID != 99 {
		t.Fatalf("input menu was mutated: %#v", menus)
	}
	legacy := applyMenuTenant(menus, nil)
	if legacy[0].TenantID != nil || legacy[0].ChildMenus[0].TenantID != nil {
		t.Fatalf("legacy import retained client tenant: %#v", legacy)
	}
}

func TestValidateMenuImportItem(t *testing.T) {
	menuType := 0
	count := 0
	err := validateMenuImportItem(dto.MenuImportItem{
		Name: "系统管理",
		Type: &menuType,
		ChildMenus: []dto.MenuImportItem{
			{Name: "用户管理"},
		},
	}, 1, &count)
	if err != nil {
		t.Fatalf("expected valid menu tree, got %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two menu nodes, got %d", count)
	}
}

func TestValidateMenuImportItemRejectsInvalidInput(t *testing.T) {
	invalidType := 3
	for _, item := range []struct {
		name string
		menu dto.MenuImportItem
	}{
		{name: "empty name", menu: dto.MenuImportItem{}},
		{name: "invalid type", menu: dto.MenuImportItem{Name: "菜单", Type: &invalidType}},
	} {
		t.Run(item.name, func(t *testing.T) {
			count := 0
			if err := validateMenuImportItem(item.menu, 1, &count); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestMenuUpdateMutationPreservesPresentedFields(t *testing.T) {
	row, fields, err := menuUpdateMutation(map[string]interface{}{
		"parentId": nil,
		"name":     "  首页  ",
		"orderNum": 0,
		"isShow":   false,
	})
	if err != nil {
		t.Fatalf("build menu update mutation failed: %v", err)
	}
	wantFields := []string{"parentId", "name", "orderNum", "isShow"}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("unexpected menu update fields: %#v", fields)
	}
	if row.ParentID != nil || row.Name != "首页" || row.OrderNum != 0 || row.IsShow != false {
		t.Fatalf("unexpected menu update row: %#v", row)
	}
	if row.Router != nil || row.Perms != nil || row.CreateTime != nil || row.UpdateTime != nil {
		t.Fatalf("missing menu fields must remain unset: %#v", row)
	}
}

func TestMenuUpdateMutationRejectsNullRequiredFields(t *testing.T) {
	for _, field := range []string{"name", "type", "orderNum", "keepAlive", "isShow"} {
		t.Run(field, func(t *testing.T) {
			if _, _, err := menuUpdateMutation(map[string]interface{}{field: nil}); err == nil {
				t.Fatalf("expected null %s to be rejected", field)
			}
		})
	}
}

func TestMenuUpdateMutationBuildsScopedPartialSQL(t *testing.T) {
	db := newBaseTenantServiceTestDB(t)
	ctx := baseTenantServiceContext(t, 29)
	row, fields, err := menuUpdateMutation(map[string]interface{}{"isShow": false})
	if err != nil {
		t.Fatalf("build menu update mutation failed: %v", err)
	}
	row.UpdateTime = "2026-07-29 18:00:00"
	fields = append(fields, "updateTime")
	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := tenant.ScopedModel(ctx, db, baseEntity.BaseSysMenu(), "")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Fields(fields).Where("id", 42).Data(row).Update()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build menu update SQL failed: %v", err)
	}
	setSQL := strings.SplitN(sqlText, " WHERE ", 2)[0]
	if !strings.Contains(setSQL, "`isShow`=false") || !strings.Contains(setSQL, "`updateTime`=") {
		t.Fatalf("menu partial fields missing from SQL: %s", sqlText)
	}
	for _, column := range []string{"createTime", "tenantId", "name", "router", "perms"} {
		if strings.Contains(setSQL, "`"+column+"`") {
			t.Fatalf("menu partial SQL unexpectedly updates %s: %s", column, sqlText)
		}
	}
	if !strings.Contains(sqlText, "`tenantId` = 29") {
		t.Fatalf("menu partial SQL lost tenant predicate: %s", sqlText)
	}
}

func TestMenuUpdateMutationPreservesExplicitNullSQL(t *testing.T) {
	db := newBaseTenantServiceTestDB(t)
	ctx := baseTenantServiceContext(t, 29)
	row, fields, err := menuUpdateMutation(map[string]interface{}{"parentId": nil})
	if err != nil {
		t.Fatalf("build menu null mutation failed: %v", err)
	}
	row.UpdateTime = "2026-07-29 18:00:00"
	fields = append(fields, "updateTime")
	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := tenant.ScopedModel(ctx, db, baseEntity.BaseSysMenu(), "")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Fields(fields).Where("id", 42).Data(row).Update()
		return modelErr
	})
	if err != nil || !strings.Contains(strings.ToLower(sqlText), "`parentid`=null") {
		t.Fatalf("menu explicit null was not preserved: %s, %v", sqlText, err)
	}
}

func TestMenuRawConditionsScopeJoinedResources(t *testing.T) {
	service := newTestMenuService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysMenu())
	ctx := baseTenantServiceContext(t, 29)
	for _, item := range []struct {
		name       string
		definition entity.Definition
		alias      string
		wantSQL    string
	}{
		{name: "menu", definition: service.Model, alias: "a", wantSQL: "`a`.`tenantId` = ?"},
		{name: "parent", definition: service.Model, alias: "p", wantSQL: "`p`.`tenantId` = ?"},
		{name: "role", definition: service.roleModel, alias: "r", wantSQL: "`r`.`tenantId` = ?"},
	} {
		t.Run(item.name, func(t *testing.T) {
			condition, err := service.menuTenantCondition(ctx, item.definition, item.alias)
			if err != nil || condition.SQL != item.wantSQL || len(condition.Args) != 1 || condition.Args[0] != int64(29) {
				t.Fatalf("unexpected menu condition: %#v, %v", condition, err)
			}
		})
	}
}

func TestMenuManagementRejectsMissingScopeAndAllowsPlatform(t *testing.T) {
	service := newTestMenuService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysMenu())
	if _, err := service.Page(context.Background(), crud.QueryRequest{}); err == nil {
		t.Fatal("expected missing menu page scope rejected")
	}
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	condition, err := service.menuTenantCondition(platformCtx, service.Model, "a")
	if err != nil || condition.SQL != "" || len(condition.Args) != 0 {
		t.Fatalf("platform menu condition must be unscoped: %#v, %v", condition, err)
	}
}

func TestBuildMenuExportItemBuildsNestedTree(t *testing.T) {
	nodes := map[int64]menuExportNode{
		1: {item: dto.MenuExportItem{Name: "root", ChildMenus: []dto.MenuExportItem{}}},
		2: {item: dto.MenuExportItem{Name: "child", ChildMenus: []dto.MenuExportItem{}}},
		3: {item: dto.MenuExportItem{Name: "leaf", ChildMenus: []dto.MenuExportItem{}}},
	}
	item, err := buildMenuExportItem(1, nodes, map[int64][]int64{1: {2}, 2: {3}}, map[int64]bool{})
	if err != nil {
		t.Fatalf("build menu tree failed: %v", err)
	}
	if len(item.ChildMenus) != 1 || len(item.ChildMenus[0].ChildMenus) != 1 || item.ChildMenus[0].ChildMenus[0].Name != "leaf" {
		t.Fatalf("unexpected menu tree: %#v", item)
	}
}

func TestMenuParseGoModel(t *testing.T) {
	service := &MenuService{}
	result, err := service.Parse(dto.MenuParseReq{
		Module: "demo",
		Entity: `package model
import "github.com/toothdy/cool-admin-go-next/cool/entity"
func DemoUser() entity.Definition {
  return entity.NewDefinition("demo", "DemoUser", "demo_user").Fields([]entity.Field{
    entity.NewField("name", "name", "varchar").Size(100).Nullable().Comment("名称"),
  })
}`,
	})
	if err != nil {
		t.Fatalf("parse Go model failed: %v", err)
	}
	if result.ClassName != "DemoUser" || result.TableName != "demo_user" || result.FileName != "user" || result.Path != "/admin/demo/user" {
		t.Fatalf("unexpected parse result: %#v", result)
	}
	want := []dto.MenuParseColumn{{PropertyName: "name", Type: "varchar", Length: "100", Comment: "名称", Nullable: true}}
	if !reflect.DeepEqual(result.Columns, want) {
		t.Fatalf("unexpected columns: %#v", result.Columns)
	}
}

func TestMenuParseGoBaseFieldsUseNodeVarcharTimes(t *testing.T) {
	service := &MenuService{}
	result, err := service.Parse(dto.MenuParseReq{
		Module: "demo",
		Entity: `func DemoUser() entity.Definition {
  fields := entity.BaseFields()
  return entity.NewDefinition("demo", "DemoUser", "demo_user").Fields(fields)
}`,
	})
	if err != nil {
		t.Fatalf("parse Go model failed: %v", err)
	}
	if len(result.Columns) != 4 || result.Columns[2].PropertyName != "createTime" || result.Columns[2].Type != "varchar" || result.Columns[3].PropertyName != "updateTime" || result.Columns[3].Type != "varchar" {
		t.Fatalf("unexpected base columns: %#v", result.Columns)
	}
}

func TestMenuParseRejectsInvalidSource(t *testing.T) {
	if _, err := (&MenuService{}).Parse(dto.MenuParseReq{Module: "../demo", Entity: "invalid"}); err == nil {
		t.Fatal("expected invalid source to fail")
	}
}

func TestMutationHelpersNormalizeIDs(t *testing.T) {
	ids, err := parseMenuIDs([]interface{}{float64(1), "2", int64(1)})
	if err != nil || !reflect.DeepEqual(ids, []int64{1, 2}) {
		t.Fatalf("unexpected IDs: %#v, %v", ids, err)
	}
}

func TestPathWithinRejectsSibling(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "project", "modules")
	if !pathWithin(root, filepath.Join(root, "demo", "service")) {
		t.Fatal("expected module child path to be accepted")
	}
	if pathWithin(root, filepath.Join(string(filepath.Separator), "project", "outside")) {
		t.Fatal("expected sibling path to be rejected")
	}
}

func TestNormalizeMenuResponseUsesNodeBooleanTypes(t *testing.T) {
	item := normalizeMenuResponse(map[string]interface{}{"keepAlive": int64(1), "isShow": int64(0)})
	if item["keepAlive"] != true || item["isShow"] != false {
		t.Fatalf("unexpected normalized menu: %#v", item)
	}
}
