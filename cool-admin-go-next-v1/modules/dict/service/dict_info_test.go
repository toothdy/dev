package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	dictEntity "github.com/toothdy/cool-admin-go-next/modules/dict/entity"
)

/**
 * 创建字典服务测试数据库
 * @param t 测试上下文
 * @returns 测试数据库
 */
func newDictServiceTestDB(t *testing.T) gdb.DB {
	t.Helper()
	db, err := gdb.New(gdb.ConfigNode{
		Type: "mysql", Host: "127.0.0.1", Port: "3306", User: "test", Pass: "test", Name: "test", DryRun: true,
	})
	if err != nil {
		t.Fatalf("create dict service test database failed: %v", err)
	}
	fields := map[string]*gdb.TableField{
		"id":          {Index: 0, Name: "id", Type: "bigint unsigned", Key: "PRI", Extra: "auto_increment"},
		"createTime": {Index: 1, Name: "createTime", Type: "varchar(32)"},
		"updateTime": {Index: 2, Name: "updateTime", Type: "varchar(32)"},
		"tenantId":   {Index: 3, Name: "tenantId", Type: "bigint unsigned", Null: true},
		"name":        {Index: 4, Name: "name", Type: "varchar(100)"},
		"key":         {Index: 5, Name: "key", Type: "varchar(100)"},
		"typeId":     {Index: 6, Name: "typeId", Type: "bigint unsigned"},
		"parentId":   {Index: 7, Name: "parentId", Type: "bigint unsigned", Null: true},
		"orderNum":   {Index: 8, Name: "orderNum", Type: "int"},
		"value":       {Index: 9, Name: "value", Type: "varchar(255)", Null: true},
	}
	for _, table := range []string{"dict_type", "dict_info"} {
		if err = db.GetCore().SetTableFields(context.Background(), table, fields); err != nil {
			t.Fatalf("seed %s fields failed: %v", table, err)
		}
	}
	return db
}

/**
 * 创建字典租户上下文
 * @param t 测试上下文
 * @param tenantID 租户 ID
 * @returns 租户上下文
 */
func dictTenantContext(t *testing.T, tenantID int64) context.Context {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
}

/**
 * 验证公开字典类型只读取平台数据
 * @param t 测试上下文
 * @returns null
 */
func TestDictTypesUsesGlobalOnlyScope(t *testing.T) {
	db := newDictServiceTestDB(t)
	service := NewDictInfoService(db, dictEntity.DictInfo(), dictEntity.DictType(), nil)
	sqlText, err := gdb.ToSQL(context.Background(), func(ctx context.Context) error {
		_, queryErr := service.GlobalTypes(ctx)
		return queryErr
	})
	if err != nil {
		t.Fatalf("build dict types query failed: %v", err)
	}
	if !strings.Contains(sqlText, "`dt`.`tenantId` IS NULL") || !strings.Contains(sqlText, "FROM `dict_type` AS dt") {
		t.Fatalf("unexpected global dict types query: %s", sqlText)
	}
}

/**
 * 验证普通字典类型查询拒绝缺失作用域
 * @param t 测试上下文
 * @returns null
 */
func TestDictTypesRejectsMissingScope(t *testing.T) {
	db := newDictServiceTestDB(t)
	service := NewDictInfoService(db, dictEntity.DictInfo(), dictEntity.DictType(), nil)
	if _, err := service.Types(context.Background()); err == nil {
		t.Fatal("expected missing dict types scope to be rejected")
	}
}

/**
 * 验证字典数据查询使用别名租户条件
 * @param t 测试上下文
 * @returns null
 */
func TestDictDataUsesTenantAliasScope(t *testing.T) {
	db := newDictServiceTestDB(t)
	service := NewDictInfoService(db, dictEntity.DictInfo(), dictEntity.DictType(), nil)
	sqlText, err := gdb.ToSQL(dictTenantContext(t, 23), func(ctx context.Context) error {
		_, queryErr := service.Data(ctx, []string{"occupation"})
		return queryErr
	})
	if err != nil {
		t.Fatalf("build dict data query failed: %v", err)
	}
	if !strings.Contains(sqlText, "`dt`.`tenantId` = 23") ||
		!strings.Contains(sqlText, "dt.`key` IN") || !strings.Contains(sqlText, "'occupation'") {
		t.Fatalf("unexpected tenant dict data query: %s", sqlText)
	}
}

/**
 * 验证字典数据查询拒绝缺失作用域
 * @param t 测试上下文
 * @returns null
 */
func TestDictDataRejectsMissingScope(t *testing.T) {
	db := newDictServiceTestDB(t)
	service := NewDictInfoService(db, dictEntity.DictInfo(), dictEntity.DictType(), nil)
	if _, err := service.Data(context.Background(), nil); err == nil {
		t.Fatal("expected missing dict data scope to be rejected")
	}
}

/**
 * 验证字典删除 ID 归一化
 * @param t 测试上下文
 * @returns null
 */
func TestNormalizeDictIDs(t *testing.T) {
	got := normalizeDictIDs([]interface{}{int64(3), 3, "4", int64(4), int64(3)})
	want := []interface{}{int64(3), "4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected normalized IDs %#v, got %#v", want, got)
	}
}

/**
 * 验证字典值类型转换
 * @param t 测试上下文
 * @returns null
 */
func TestConvertDictValue(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  interface{}
	}{
		{"empty keeps string", "", ""},
		{"integer becomes number", "5", float64(5)},
		{"float becomes number", "5.5", float64(5.5)},
		{"zero becomes number", "0", float64(0)},
		{"non-number keeps string", "cool", "cool"},
		{"url keeps string", "https://example.com/x.gif", "https://example.com/x.gif"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got := convertDictValue(item.input)
			if !reflect.DeepEqual(got, item.want) {
				t.Fatalf("expected %#v, got %#v", item.want, got)
			}
		})
	}
}
