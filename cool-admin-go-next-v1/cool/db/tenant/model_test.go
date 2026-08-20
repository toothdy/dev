package tenant

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type scopedModelDO struct {
	Name     interface{} `orm:"name"`
	TenantID interface{} `orm:"tenantId"`
}

type scopedModelProvider struct {
	db        gdb.DB
	callCount int
	table     interface{}
}

/**
 * 记录 Scoped Model 的数据库提供调用
 * @param tableNameQueryOrStruct 表名或模型
 * @returns GoFrame Model
 */
func (p *scopedModelProvider) Model(tableNameQueryOrStruct ...interface{}) *gdb.Model {
	p.callCount++
	if len(tableNameQueryOrStruct) > 0 {
		p.table = tableNameQueryOrStruct[0]
	}
	return p.db.Model(tableNameQueryOrStruct...)
}

var _ ModelProvider = (gdb.DB)(nil)
var _ ModelProvider = (gdb.TX)(nil)

/**
 * 创建 Scoped Model 测试数据库
 * @param t 测试上下文
 * @returns GoFrame 数据库
 */
func newScopedModelTestDB(t *testing.T) gdb.DB {
	t.Helper()
	db, err := gdb.New(gdb.ConfigNode{
		Type:   "mysql",
		Host:   "127.0.0.1",
		Port:   "3306",
		User:   "test",
		Pass:   "test",
		Name:   "test",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("create scoped model test database failed: %v", err)
	}
	err = db.GetCore().SetTableFields(context.Background(), "demo_goods", map[string]*gdb.TableField{
		"id":          {Index: 0, Name: "id", Type: "bigint unsigned", Key: "PRI", Extra: "auto_increment"},
		"createTime": {Index: 1, Name: "createTime", Type: "varchar(32)"},
		"updateTime": {Index: 2, Name: "updateTime", Type: "varchar(32)"},
		"tenantId":   {Index: 3, Name: "tenantId", Type: "bigint unsigned", Null: true},
		"name":        {Index: 4, Name: "name", Type: "varchar(255)"},
	})
	if err != nil {
		t.Fatalf("seed scoped model fields failed: %v", err)
	}
	return db
}

/**
 * 创建租户模型定义
 * @returns 模型定义
 */
func scopedModelDefinition() entity.Definition {
	return entity.NewDefinition("demo", "Goods", "demo_goods").Fields(append(
		entity.BaseFields(),
		entity.NewField("name", "name", "varchar").NotNull(),
	))
}

/**
 * 创建具体租户上下文
 * @param t 测试上下文
 * @param tenantID 租户 ID
 * @returns 租户上下文
 */
func scopedModelTenantContext(t *testing.T, tenantID int64) context.Context {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
}

/**
 * 创建平台上下文
 * @returns 平台上下文
 */
func scopedModelPlatformContext() context.Context {
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
}

/**
 * 验证 Select 和 Count 结构化追加租户条件
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelBuildsParameterizedSelectAndCount(t *testing.T) {
	db := newScopedModelTestDB(t)
	ctx := scopedModelTenantContext(t, 7)
	definition := scopedModelDefinition()

	selectSQL, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, definition, "g")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Where("`g`.`name` = ?", "alice").All()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build scoped select failed: %v", err)
	}
	if !strings.Contains(selectSQL, "`g`.`tenantId` = 7") || !strings.Contains(selectSQL, "`g`.`name` = 'alice'") {
		t.Fatalf("unexpected scoped select SQL: %s", selectSQL)
	}

	countSQL, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, definition, "g")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Count()
		return modelErr
	})
	if err != nil || !strings.Contains(countSQL, "`g`.`tenantId` = 7") || !strings.Contains(strings.ToUpper(countSQL), "COUNT(") {
		t.Fatalf("unexpected scoped count SQL: %s, %v", countSQL, err)
	}
}

/**
 * 验证 Insert Hook 覆盖伪造租户值
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelInsertOverridesTenantValue(t *testing.T) {
	db := newScopedModelTestDB(t)
	ctx := scopedModelTenantContext(t, 9)
	input := scopedModelDO{Name: "alice", TenantID: int64(999)}

	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "")
		if modelErr != nil {
			return modelErr
		}
		dbModel = dbModel.Fields("name", "tenantId")
		_, modelErr = dbModel.Data(input).Insert()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build scoped insert failed: %v", err)
	}
	if strings.Contains(sqlText, "999") || !strings.Contains(sqlText, "9") || input.TenantID != int64(999) {
		t.Fatalf("insert scope was not enforced: sql=%s input=%#v", sqlText, input)
	}
}

/**
 * 验证批量 Insert Hook 覆盖每条租户值
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelBatchInsertOverridesEveryTenantValue(t *testing.T) {
	db := newScopedModelTestDB(t)
	ctx := scopedModelTenantContext(t, 13)
	input := []scopedModelDO{
		{Name: "alice", TenantID: int64(998)},
		{Name: "bob", TenantID: int64(999)},
	}

	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "")
		if modelErr != nil {
			return modelErr
		}
		dbModel = dbModel.Fields("name", "tenantId")
		_, modelErr = dbModel.Data(input).Insert()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build scoped batch insert failed: %v", err)
	}
	if strings.Contains(sqlText, "998") || strings.Contains(sqlText, "999") || strings.Count(sqlText, "13") < 2 {
		t.Fatalf("batch insert scope was not enforced: %s", sqlText)
	}
	if input[0].TenantID != int64(998) || input[1].TenantID != int64(999) {
		t.Fatalf("batch insert mutated caller data: %#v", input)
	}
}

/**
 * 验证平台和 Bypass 新增写入 NULL
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelPlatformAndBypassWriteNull(t *testing.T) {
	db := newScopedModelTestDB(t)
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "platform", ctx: scopedModelPlatformContext()},
		{name: "bypass", ctx: WithoutTenant(context.Background())},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sqlText, err := gdb.ToSQL(test.ctx, func(ctx context.Context) error {
				dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "")
				if modelErr != nil {
					return modelErr
				}
				dbModel = dbModel.Fields("name", "tenantId")
				_, modelErr = dbModel.Data(scopedModelDO{Name: "alice", TenantID: int64(999)}).Insert()
				return modelErr
			})
			if err != nil || strings.Contains(sqlText, "999") || !strings.Contains(strings.ToUpper(sqlText), "NULL") {
				t.Fatalf("unexpected %s insert SQL: %s, %v", test.name, sqlText, err)
			}
		})
	}
}

/**
 * 验证平台和 Bypass 查询不自动过滤
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelPlatformAndBypassRemainUnfiltered(t *testing.T) {
	db := newScopedModelTestDB(t)
	tests := []struct {
		name string
		ctx  context.Context
	}{
		{name: "platform", ctx: scopedModelPlatformContext()},
		{name: "bypass", ctx: WithoutTenant(context.Background())},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sqlText, err := gdb.ToSQL(test.ctx, func(ctx context.Context) error {
				dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "g")
				if modelErr != nil {
					return modelErr
				}
				_, modelErr = dbModel.Where("`g`.`name` = ?", "alice").All()
				return modelErr
			})
			if err != nil || strings.Contains(sqlText, "tenantId") {
				t.Fatalf("unexpected %s select SQL: %s, %v", test.name, sqlText, err)
			}
		})
	}
}

/**
 * 验证 Update 和 Delete Hook 的租户二次防御
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelGuardsUpdateAndDelete(t *testing.T) {
	db := newScopedModelTestDB(t)
	ctx := scopedModelTenantContext(t, 11)

	updateSQL, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "")
		if modelErr != nil {
			return modelErr
		}
		dbModel = dbModel.Fields("id", "name", "tenantId")
		_, modelErr = dbModel.Data(scopedModelDO{Name: "alice", TenantID: int64(999)}).Where("id", 1).Update()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build scoped update failed: %v", err)
	}
	setSQL := strings.SplitN(updateSQL, " WHERE ", 2)[0]
	if strings.Contains(setSQL, "tenantId") || strings.Count(updateSQL, "tenantId") < 2 || strings.Contains(updateSQL, "999") {
		t.Fatalf("update scope was not enforced: %s", updateSQL)
	}

	deleteSQL, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, scopedModelDefinition(), "")
		if modelErr != nil {
			return modelErr
		}
		dbModel = dbModel.Fields("id", "tenantId")
		_, modelErr = dbModel.Where("id", 1).Delete()
		return modelErr
	})
	if err != nil || strings.Count(deleteSQL, "tenantId") < 2 {
		t.Fatalf("delete scope was not enforced: %s, %v", deleteSQL, err)
	}
}

/**
 * 验证 Missing、非法别名和租户字段字符串更新被拒绝
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelRejectsMissingAndUnsafeInputs(t *testing.T) {
	db := newScopedModelTestDB(t)
	definition := scopedModelDefinition()
	if _, err := ScopedModel(context.Background(), db, definition, ""); err == nil {
		t.Fatal("expected missing scope rejected")
	}
	if _, err := ScopedModel(scopedModelTenantContext(t, 1), db, definition, "g;drop"); err == nil {
		t.Fatal("expected invalid alias rejected")
	}
	if _, err := sanitizeHookUpdateData("tenantId=99", "tenantId"); err == nil {
		t.Fatal("expected string tenant update rejected")
	}
	if value, err := sanitizeHookUpdateData("deleted_at=?", "tenantId"); err != nil || value != "deleted_at=?" {
		t.Fatalf("expected soft delete update allowed: %#v, %v", value, err)
	}
	value, err := sanitizeHookUpdateData(map[string]interface{}{
		"demo_goods.tenantId": int64(999),
		"name":                "alice",
	}, "tenantId")
	if err != nil {
		t.Fatalf("sanitize qualified tenant update failed: %v", err)
	}
	values := value.(map[string]interface{})
	if _, ok := values["demo_goods.tenantId"]; ok || values["name"] != "alice" {
		t.Fatalf("qualified tenant update was not removed: %#v", values)
	}
}

/**
 * 验证非租户模型允许 Missing 上下文
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelAllowsMissingForNonTenantModel(t *testing.T) {
	db := newScopedModelTestDB(t)
	provider := &scopedModelProvider{db: db}
	definition := entity.NewDefinition("demo", "Relation", "demo_relation").Fields([]entity.Field{
		entity.NewField("id", "id", "bigint").Unsigned().Primary(),
	})
	if _, err := ScopedModel(context.Background(), provider, definition, "r"); err != nil {
		t.Fatalf("non-tenant model rejected missing scope: %v", err)
	}
	if provider.callCount != 1 || provider.table != "demo_relation" {
		t.Fatalf("model provider was not used: %#v", provider)
	}
}

/**
 * 验证 raw SQL 不会触发 Model Hook
 * @param t 测试上下文
 * @returns null
 */
func TestRawSQLDoesNotReceiveTenantScope(t *testing.T) {
	db := newScopedModelTestDB(t)
	ctx := scopedModelTenantContext(t, 17)
	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		_, queryErr := db.GetAll(ctx, "SELECT * FROM demo_goods WHERE name = ?", "alice")
		return queryErr
	})
	if err != nil {
		t.Fatalf("build raw SQL failed: %v", err)
	}
	if strings.Contains(sqlText, "tenantId") {
		t.Fatalf("raw SQL unexpectedly passed through Model hooks: %s", sqlText)
	}
}

/**
 * 验证软删除 SET 参数保持占位符顺序
 * @param t 测试上下文
 * @returns null
 */
func TestModelScopePreservesSoftDeleteArgumentOrder(t *testing.T) {
	metadata, err := CompileMetadata(scopedModelDefinition())
	if err != nil {
		t.Fatalf("compile scoped model metadata failed: %v", err)
	}
	plan := modelScope{
		metadata: metadata,
		scope:    Resolve(scopedModelTenantContext(t, 7)),
	}
	condition := "`tenantId` = ? AND `id` = ?"
	args := []interface{}{"deleted-at", int64(8), 1}
	if err = plan.appendMutationPredicate(&condition, &args, 1); err != nil {
		t.Fatalf("append soft delete predicate failed: %v", err)
	}
	if !reflect.DeepEqual(args, []interface{}{"deleted-at", int64(7), int64(8), 1}) {
		t.Fatalf("soft delete arguments were reordered incorrectly: %#v", args)
	}
	if countSQLPlaceholders("deleted_at=?, note='?', quoted=\"?\"") != 1 {
		t.Fatal("quoted question marks must not count as SQL placeholders")
	}
}

/**
 * 验证 GoFrame 软删除继续受租户 Hook 保护
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelProtectsGoFrameSoftDelete(t *testing.T) {
	db := newScopedModelTestDB(t)
	err := db.GetCore().SetTableFields(context.Background(), "demo_soft_goods", map[string]*gdb.TableField{
		"id":         {Index: 0, Name: "id", Type: "bigint unsigned", Key: "PRI"},
		"tenantId":  {Index: 1, Name: "tenantId", Type: "bigint unsigned", Null: true},
		"deleted_at": {Index: 2, Name: "deleted_at", Type: "datetime", Null: true},
	})
	if err != nil {
		t.Fatalf("seed soft delete fields failed: %v", err)
	}
	definition := entity.NewDefinition("demo", "SoftGoods", "demo_soft_goods").Fields([]entity.Field{
		entity.NewField("id", "id", "bigint").Unsigned().Primary(),
		entity.NewField("tenantId", "tenantId", "bigint").Unsigned().Nullable(),
		entity.NewField("deletedAt", "deleted_at", "datetime").Nullable(),
	})
	ctx := scopedModelTenantContext(t, 19)
	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := ScopedModel(ctx, db, definition, "")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Where("id", 1).Delete()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build scoped soft delete failed: %v", err)
	}
	if !strings.HasPrefix(strings.ToUpper(sqlText), "UPDATE ") || !strings.Contains(sqlText, "deleted_at") || strings.Count(sqlText, "tenantId") < 2 {
		t.Fatalf("soft delete did not retain tenant defense: %s", sqlText)
	}
}
