package sys

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

const (
	relationScopeTenantA      int64 = 971001
	relationScopeTenantB      int64 = 971002
	relationScopeUserA        int64 = 971011
	relationScopeUserB        int64 = 971012
	relationScopeRoleA        int64 = 971021
	relationScopeRoleB        int64 = 971022
	relationScopeMenuA        int64 = 971031
	relationScopeMenuB        int64 = 971032
	relationScopeDepartmentA  int64 = 971041
	relationScopeDepartmentB  int64 = 971042
	relationScopeFixtureStamp       = "2026-07-28 00:00:00"
)

func TestRelationScopeMySQLIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run relation scope integration test")
	}

	ctx := context.Background()
	db := g.DB()
	if _, err := schema.NewSyncer(db).Sync(ctx, baseModelDefinitions()); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}
	cleanupRelationScopeFixtures(t, ctx)
	t.Cleanup(func() {
		cleanupRelationScopeFixtures(t, ctx)
	})
	seedRelationScopeFixtures(t, ctx)

	tenantAIdentity, err := security.NewTenantIdentity(relationScopeTenantA)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	tenantACtx := security.ContextWithUser(ctx, security.UserContext{
		UserId:   relationScopeUserA,
		RoleIds:  []int64{relationScopeRoleA},
		TenantId: tenantAIdentity,
	})
	platformCtx := security.ContextWithUser(ctx, security.UserContext{TenantId: security.PlatformTenant()})

	userService := newTestUserServiceWithSessions(db, baseModel.BaseSysUser(), security.NewMemorySessionStore())
	roleService := newTestRoleServiceWithSessions(db, baseModel.BaseSysRole(), security.NewMemorySessionStore())
	menuService := newTestMenuService(db, baseModel.BaseSysMenu())
	departmentService := newTestDepartmentService(db, baseModel.BaseSysDepartment())
	permissionService := NewPermissionService(db)

	assertRelationScopeReads(t, tenantACtx, platformCtx, userService, roleService, departmentService, permissionService)
	assertRelationScopeWrites(t, tenantACtx, db, userService, roleService, menuService, departmentService)
}

func assertRelationScopeReads(
	t *testing.T,
	tenantACtx context.Context,
	platformCtx context.Context,
	userService *UserService,
	roleService *RoleService,
	departmentService *DepartmentService,
	permissionService *PermissionService,
) {
	t.Helper()

	tenantUserValue, err := userService.Info(tenantACtx, crud.InfoRequest{ID: relationScopeUserA})
	if err != nil {
		t.Fatalf("read tenant user failed: %v", err)
	}
	tenantUser := tenantUserValue.(map[string]interface{})
	if stringValue(tenantUser["departmentName"]) != "" {
		t.Fatalf("tenant user leaked cross-tenant department name: %#v", tenantUser)
	}
	if roleIDs := interfaceIDs(tenantUser["roleIdList"]); !reflect.DeepEqual(roleIDs, []int64{relationScopeRoleA}) {
		t.Fatalf("tenant user leaked cross-tenant roles: %#v", roleIDs)
	}
	if _, err = userService.Info(tenantACtx, crud.InfoRequest{ID: relationScopeUserB}); err == nil {
		t.Fatal("tenant user info must reject another tenant parent")
	}

	tenantRoleValue, err := roleService.Info(tenantACtx, crud.InfoRequest{ID: relationScopeRoleA})
	if err != nil {
		t.Fatalf("read tenant role failed: %v", err)
	}
	tenantRole := tenantRoleValue.(map[string]interface{})
	if !reflect.DeepEqual(tenantRole["menuIdList"], []int64{relationScopeMenuA}) ||
		!reflect.DeepEqual(tenantRole["departmentIdList"], []int64{relationScopeDepartmentA}) {
		t.Fatalf("tenant role leaked cross-tenant relations: %#v", tenantRole)
	}
	otherTenantRoleValue, err := roleService.Info(tenantACtx, crud.InfoRequest{ID: relationScopeRoleB})
	if err != nil {
		t.Fatalf("read another tenant role failed: %v", err)
	}
	if otherTenantRole := otherTenantRoleValue.(map[string]interface{}); len(otherTenantRole) != 0 {
		t.Fatalf("tenant role info leaked another tenant parent: %#v", otherTenantRole)
	}

	platformUserValue, err := userService.Info(platformCtx, crud.InfoRequest{ID: relationScopeUserA})
	if err != nil {
		t.Fatalf("platform user read failed: %v", err)
	}
	platformUser := platformUserValue.(map[string]interface{})
	if roleIDs := interfaceIDs(platformUser["roleIdList"]); !reflect.DeepEqual(roleIDs, []int64{relationScopeRoleA, relationScopeRoleB}) {
		t.Fatalf("platform scope lost cross-tenant visibility: %#v", roleIDs)
	}
	platformRoleValue, err := roleService.Info(platformCtx, crud.InfoRequest{ID: relationScopeRoleA})
	if err != nil {
		t.Fatalf("platform role read failed: %v", err)
	}
	platformRole := platformRoleValue.(map[string]interface{})
	if !reflect.DeepEqual(platformRole["menuIdList"], []int64{relationScopeMenuA, relationScopeMenuB}) ||
		!reflect.DeepEqual(platformRole["departmentIdList"], []int64{relationScopeDepartmentA, relationScopeDepartmentB}) {
		t.Fatalf("platform role scope lost cross-tenant visibility: %#v", platformRole)
	}

	permissionResult, err := permissionService.PermMenu(tenantACtx, security.UserContext{UserId: relationScopeUserA})
	if err != nil {
		t.Fatalf("read tenant permissions failed: %v", err)
	}
	menuIDs := make([]int64, 0, len(permissionResult.Menus))
	for _, menu := range permissionResult.Menus {
		menuIDs = append(menuIDs, int64Value(menu["id"]))
	}
	if !reflect.DeepEqual(menuIDs, []int64{relationScopeMenuA}) {
		t.Fatalf("tenant permissions leaked cross-tenant menus: %#v", menuIDs)
	}

	departmentsValue, err := departmentService.List(tenantACtx, crud.QueryRequest{})
	if err != nil {
		t.Fatalf("read tenant departments failed: %v", err)
	}
	departments := departmentsValue.([]map[string]interface{})
	if len(departments) != 1 || int64Value(departments[0]["id"]) != relationScopeDepartmentA || stringValue(departments[0]["parentName"]) != "" {
		t.Fatalf("tenant department leaked cross-tenant parent: %#v", departments)
	}
}

func assertRelationScopeWrites(
	t *testing.T,
	tenantACtx context.Context,
	db gdb.DB,
	userService *UserService,
	roleService *RoleService,
	menuService *MenuService,
	departmentService *DepartmentService,
) {
	t.Helper()

	if _, err := roleService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id":         relationScopeRoleA,
		"name":       "relation-scope-partial-write",
		"menuIdList": []interface{}{relationScopeMenuB},
	}}); err == nil {
		t.Fatal("cross-tenant menu assignment must fail")
	}
	role, err := db.GetOne(tenantACtx, "SELECT name FROM base_sys_role WHERE id = ?", relationScopeRoleA)
	if err != nil {
		t.Fatalf("read role after rejected update failed: %v", err)
	}
	if role["name"].String() != "relation-scope-role-a" {
		t.Fatalf("invalid relation update partially changed role: %#v", role)
	}

	if _, err = userService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id":         relationScopeUserA,
		"roleIdList": []interface{}{relationScopeRoleB},
	}}); err == nil {
		t.Fatal("cross-tenant role assignment must fail")
	}
	if _, err = departmentService.Delete(tenantACtx, crud.DeleteRequest{IDs: []interface{}{relationScopeDepartmentB}}); err == nil {
		t.Fatal("cross-tenant department delete must fail")
	}
	if _, err = departmentService.Add(tenantACtx, crud.AddRequest{Data: map[string]interface{}{
		"name": "relation-scope-invalid-parent", "parentId": relationScopeDepartmentB,
	}}); err == nil {
		t.Fatal("cross-tenant department parent add must fail")
	}
	if _, err = departmentService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id": relationScopeDepartmentA, "name": "relation-scope-partial-department", "parentId": relationScopeDepartmentB,
	}}); err == nil {
		t.Fatal("cross-tenant department parent update must fail")
	}
	parentID := relationScopeDepartmentB
	if err = departmentService.Order(tenantACtx, []dto.DepartmentOrderItem{{
		ID: relationScopeDepartmentA, ParentID: &parentID, OrderNum: 9,
	}}); err == nil {
		t.Fatal("cross-tenant department parent order must fail")
	}
	if err = userService.Move(tenantACtx, dto.MoveReq{
		DepartmentID: relationScopeDepartmentB,
		UserIDs:      []int64{relationScopeUserA},
	}); err == nil {
		t.Fatal("cross-tenant user move must fail")
	}
	department, err := db.GetOne(tenantACtx, "SELECT id FROM base_sys_department WHERE id = ?", relationScopeDepartmentB)
	if err != nil || department.IsEmpty() {
		t.Fatalf("rejected department delete changed another tenant: %#v %v", department, err)
	}
	departmentA, err := db.GetOne(tenantACtx, "SELECT name, orderNum FROM base_sys_department WHERE id = ?", relationScopeDepartmentA)
	if err != nil || departmentA["name"].String() != "relation-scope-department-a" || departmentA["orderNum"].Int64() != 0 {
		t.Fatalf("rejected department relation partially updated parent: %#v %v", departmentA, err)
	}
	invalidParentCount, err := db.GetCount(tenantACtx, "SELECT COUNT(*) FROM base_sys_department WHERE name = ?", "relation-scope-invalid-parent")
	if err != nil || invalidParentCount != 0 {
		t.Fatalf("rejected department add partially inserted row: %d %v", invalidParentCount, err)
	}

	if _, err = menuService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id": relationScopeMenuA, "isShow": false,
	}}); err != nil {
		t.Fatalf("partial menu update failed: %v", err)
	}
	menu, err := db.GetOne(tenantACtx, "SELECT name, isShow, createTime, tenantId FROM base_sys_menu WHERE id = ?", relationScopeMenuA)
	if err != nil || menu["name"].String() != "relation-scope-menu-a" || menu["isShow"].Bool() ||
		menu["createTime"].String() != relationScopeFixtureStamp || menu["tenantId"].Int64() != relationScopeTenantA {
		t.Fatalf("partial menu update changed protected fields: %#v %v", menu, err)
	}

	if _, err = menuService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id":         relationScopeMenuA,
		"parentId":   nil,
		"name":       "relation-scope-menu-updated",
		"router":     "/relation-scope",
		"type":       1,
		"icon":       nil,
		"orderNum":   0,
		"viewPath":   "modules/demo/views/home/index.vue",
		"keepAlive":  true,
		"isShow":     true,
		"tenantId":   relationScopeTenantB,
		"createTime": "1900-01-01 00:00:00",
		"updateTime": "1900-01-01 00:00:00",
	}}); err != nil {
		t.Fatalf("full menu update failed: %v", err)
	}
	menu, err = db.GetOne(tenantACtx, "SELECT name, isShow, createTime, updateTime, tenantId FROM base_sys_menu WHERE id = ?", relationScopeMenuA)
	if err != nil || menu["name"].String() != "relation-scope-menu-updated" || !menu["isShow"].Bool() ||
		menu["createTime"].String() != relationScopeFixtureStamp || menu["updateTime"].String() == "1900-01-01 00:00:00" ||
		menu["tenantId"].Int64() != relationScopeTenantA {
		t.Fatalf("full menu update did not preserve server fields: %#v %v", menu, err)
	}

	if _, err = departmentService.Update(tenantACtx, crud.UpdateRequest{Data: map[string]interface{}{
		"id":         relationScopeDepartmentA,
		"parentId":   nil,
		"orderNum":   0,
		"userId":     relationScopeUserB,
		"tenantId":   relationScopeTenantB,
		"createTime": "1900-01-01 00:00:00",
		"updateTime": "1900-01-01 00:00:00",
	}}); err != nil {
		t.Fatalf("partial department update failed: %v", err)
	}
	departmentA, err = db.GetOne(tenantACtx, "SELECT name, userId, parentId, orderNum, createTime, updateTime, tenantId FROM base_sys_department WHERE id = ?", relationScopeDepartmentA)
	if err != nil || departmentA["name"].String() != "relation-scope-department-a" || !departmentA["userId"].IsNil() ||
		!departmentA["parentId"].IsNil() || departmentA["orderNum"].Int64() != 0 ||
		departmentA["createTime"].String() != relationScopeFixtureStamp || departmentA["updateTime"].String() == "1900-01-01 00:00:00" ||
		departmentA["tenantId"].Int64() != relationScopeTenantA {
		t.Fatalf("partial department update changed protected fields: %#v %v", departmentA, err)
	}
}

func seedRelationScopeFixtures(t *testing.T, ctx context.Context) {
	t.Helper()
	db := g.DB()
	statements := []struct {
		sql  string
		args []interface{}
	}{
		{
			sql: "INSERT INTO base_sys_department (id, parentId, name, createTime, updateTime, tenantId) VALUES (?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?)",
			args: []interface{}{relationScopeDepartmentA, relationScopeDepartmentB, "relation-scope-department-a", relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantA,
				relationScopeDepartmentB, nil, "relation-scope-department-b", relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantB},
		},
		{
			sql: "INSERT INTO base_sys_menu (id, name, createTime, updateTime, tenantId) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)",
			args: []interface{}{relationScopeMenuA, "relation-scope-menu-a", relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantA,
				relationScopeMenuB, "relation-scope-menu-b", relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantB},
		},
		{
			sql: "INSERT INTO base_sys_role (id, userId, name, label, relevance, menuIdList, departmentIdList, createTime, updateTime, tenantId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			args: []interface{}{relationScopeRoleA, relationScopeUserA, "relation-scope-role-a", "relation_scope_role_a", false, fmt.Sprintf("[%d,%d]", relationScopeMenuA, relationScopeMenuB), fmt.Sprintf("[%d,%d]", relationScopeDepartmentA, relationScopeDepartmentB), relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantA,
				relationScopeRoleB, relationScopeUserB, "relation-scope-role-b", "relation_scope_role_b", false, fmt.Sprintf("[%d]", relationScopeMenuB), fmt.Sprintf("[%d]", relationScopeDepartmentB), relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantB},
		},
		{
			sql: "INSERT INTO base_sys_user (id, departmentId, username, password, passwordV, status, createTime, updateTime, tenantId) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?), (?, ?, ?, ?, ?, ?, ?, ?, ?)",
			args: []interface{}{relationScopeUserA, relationScopeDepartmentB, "relation-scope-user-a", "password", 1, 1, relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantA,
				relationScopeUserB, relationScopeDepartmentB, "relation-scope-user-b", "password", 1, 1, relationScopeFixtureStamp, relationScopeFixtureStamp, relationScopeTenantB},
		},
		{
			sql:  "INSERT INTO base_sys_user_role (userId, roleId) VALUES (?, ?), (?, ?), (?, ?)",
			args: []interface{}{relationScopeUserA, relationScopeRoleA, relationScopeUserA, relationScopeRoleB, relationScopeUserB, relationScopeRoleB},
		},
		{
			sql:  "INSERT INTO base_sys_role_menu (roleId, menuId) VALUES (?, ?), (?, ?), (?, ?)",
			args: []interface{}{relationScopeRoleA, relationScopeMenuA, relationScopeRoleA, relationScopeMenuB, relationScopeRoleB, relationScopeMenuB},
		},
		{
			sql:  "INSERT INTO base_sys_role_department (roleId, departmentId) VALUES (?, ?), (?, ?), (?, ?)",
			args: []interface{}{relationScopeRoleA, relationScopeDepartmentA, relationScopeRoleA, relationScopeDepartmentB, relationScopeRoleB, relationScopeDepartmentB},
		},
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seed relation scope fixture failed: %v", err)
		}
	}
}

func cleanupRelationScopeFixtures(t *testing.T, ctx context.Context) {
	t.Helper()
	statements := []string{
		"DELETE FROM base_sys_user_role WHERE userId IN (971011, 971012) OR roleId IN (971021, 971022)",
		"DELETE FROM base_sys_role_menu WHERE roleId IN (971021, 971022) OR menuId IN (971031, 971032)",
		"DELETE FROM base_sys_role_department WHERE roleId IN (971021, 971022) OR departmentId IN (971041, 971042)",
		"DELETE FROM base_sys_user WHERE id IN (971011, 971012)",
		"DELETE FROM base_sys_role WHERE id IN (971021, 971022)",
		"DELETE FROM base_sys_menu WHERE id IN (971031, 971032)",
		"DELETE FROM base_sys_department WHERE id IN (971041, 971042)",
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			t.Errorf("cleanup relation scope fixture failed for %s: %v", statement, err)
		}
	}
}

func interfaceIDs(value interface{}) []int64 {
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, int64Value(item))
	}
	sort.Slice(ids, func(left int, right int) bool {
		return ids[left] < ids[right]
	})
	return ids
}
