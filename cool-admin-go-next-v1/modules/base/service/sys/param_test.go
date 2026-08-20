package sys

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

/**
 * 创建 Base 租户服务测试数据库
 * @param t 测试上下文
 * @returns 测试数据库
 */
func newBaseTenantServiceTestDB(t *testing.T) gdb.DB {
	t.Helper()
	db, err := gdb.New(gdb.ConfigNode{
		Type: "mysql", Host: "127.0.0.1", Port: "3306", User: "test", Pass: "test", Name: "test", DryRun: true,
	})
	if err != nil {
		t.Fatalf("create base tenant service test database failed: %v", err)
	}
	fields := map[string]*gdb.TableField{
		"id":          {Index: 0, Name: "id", Type: "bigint unsigned", Key: "PRI", Extra: "auto_increment"},
		"createTime": {Index: 1, Name: "createTime", Type: "varchar(32)"},
		"updateTime": {Index: 2, Name: "updateTime", Type: "varchar(32)"},
		"tenantId":   {Index: 3, Name: "tenantId", Type: "bigint unsigned", Null: true},
		"keyName":    {Index: 4, Name: "keyName", Type: "varchar(255)"},
		"name":        {Index: 5, Name: "name", Type: "varchar(255)", Null: true},
		"data":        {Index: 6, Name: "data", Type: "text", Null: true},
		"dataType":   {Index: 7, Name: "dataType", Type: "tinyint"},
		"remark":      {Index: 8, Name: "remark", Type: "varchar(255)", Null: true},
		"userId":     {Index: 9, Name: "userId", Type: "bigint unsigned", Null: true},
		"action":      {Index: 10, Name: "action", Type: "varchar(255)"},
		"ip":          {Index: 11, Name: "ip", Type: "varchar(255)", Null: true},
		"params":      {Index: 12, Name: "params", Type: "json", Null: true},
		"parentId":   {Index: 13, Name: "parentId", Type: "bigint unsigned", Null: true},
		"router":      {Index: 14, Name: "router", Type: "varchar(255)", Null: true},
		"perms":       {Index: 15, Name: "perms", Type: "text", Null: true},
		"type":        {Index: 16, Name: "type", Type: "tinyint"},
		"icon":        {Index: 17, Name: "icon", Type: "varchar(255)", Null: true},
		"orderNum":   {Index: 18, Name: "orderNum", Type: "int"},
		"viewPath":   {Index: 19, Name: "viewPath", Type: "varchar(255)", Null: true},
		"keepAlive":  {Index: 20, Name: "keepAlive", Type: "tinyint"},
		"isShow":     {Index: 21, Name: "isShow", Type: "tinyint"},
	}
	for _, table := range []string{"base_sys_param", "base_sys_log", "base_sys_user", "base_sys_menu", "base_sys_role", "base_sys_department"} {
		if err = db.GetCore().SetTableFields(context.Background(), table, fields); err != nil {
			t.Fatalf("seed %s fields failed: %v", table, err)
		}
	}
	return db
}

/**
 * 创建 Base 租户测试上下文
 * @param t 测试上下文
 * @param tenantID 租户 ID
 * @returns 租户上下文
 */
func baseTenantServiceContext(t *testing.T, tenantID int64) context.Context {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
}

func TestParamRowNormalizesDataByType(t *testing.T) {
	jsonRow, err := paramRowFromData(map[string]interface{}{"dataType": float64(0), "data": map[string]interface{}{"enabled": true}}, 1)
	if err != nil || jsonRow.Data != `{"enabled":true}` {
		t.Fatalf("unexpected JSON row: %#v, %v", jsonRow, err)
	}
	listRow, err := paramRowFromData(map[string]interface{}{"dataType": 2, "data": []interface{}{"a", "b"}}, 0)
	if err != nil || listRow.Data != "a,b" {
		t.Fatalf("unexpected list row: %#v, %v", listRow, err)
	}
}

func TestParamUpdateDataContainsOnlyPresentedFields(t *testing.T) {
	values, err := paramUpdateData(map[string]interface{}{
		"id":       1,
		"dataType": 0,
		"remark":   nil,
	}, 2)
	if err != nil {
		t.Fatalf("build param update failed: %v", err)
	}
	expected := map[string]interface{}{"dataType": 0, "remark": nil}
	if !reflect.DeepEqual(values, expected) {
		t.Fatalf("unexpected partial param update: %#v", values)
	}
}

func TestParseParamInfoDataMatchesNodeReplacementRule(t *testing.T) {
	array, ok := parseParamInfoData(`[1,2]`)
	if !ok || len(array.([]interface{})) != 2 {
		t.Fatalf("expected JSON array, got %#v, %v", array, ok)
	}
	object, ok := parseParamInfoData(`{"enabled":true}`)
	if ok || object != `{"enabled":true}` {
		t.Fatalf("expected object JSON to remain text, got %#v, %v", object, ok)
	}
}

func TestParamPublicLookupUsesExplicitReadScope(t *testing.T) {
	service := newTestParamService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysParam())
	globalSQL, err := gdb.ToSQL(context.Background(), func(ctx context.Context) error {
		_, queryErr := service.DataByKey(ctx, "siteName")
		return queryErr
	})
	if err != nil || !strings.Contains(globalSQL, "`tenantId` IS NULL") || strings.Contains(globalSQL, "tenantId` = 0") {
		t.Fatalf("unexpected global param SQL: %s, %v", globalSQL, err)
	}

	tenantSQL, err := gdb.ToSQL(baseTenantServiceContext(t, 31), func(ctx context.Context) error {
		_, queryErr := service.DataByKey(ctx, "siteName")
		return queryErr
	})
	if err != nil || !strings.Contains(tenantSQL, "`tenantId` = 31") {
		t.Fatalf("unexpected tenant param SQL: %s, %v", tenantSQL, err)
	}
}

func TestParamManagementRejectsMissingScopeAndAllowsPlatform(t *testing.T) {
	service := newTestParamService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysParam())
	if _, err := service.Info(context.Background(), crud.InfoRequest{ID: 1}); err == nil {
		t.Fatal("expected missing param management scope rejected")
	}
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	condition, err := service.paramTenantCondition(platformCtx, "p")
	if err != nil || condition.SQL != "" || len(condition.Args) != 0 {
		t.Fatalf("platform param condition must be unscoped: %#v, %v", condition, err)
	}
}
