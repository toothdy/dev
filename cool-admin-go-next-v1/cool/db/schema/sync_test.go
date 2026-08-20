package schema

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func TestSyncerSkipsWithoutIntegrationFlag(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") == "1" {
		t.Skip("integration flag enabled")
	}
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "" {
		t.Fatalf("unexpected integration flag value")
	}
}

func TestSyncerCreatesTableAndIsIdempotent(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	definition := entity.NewDefinition("test", "SchemaTest", "schema_sync_test").
		Comment("建表测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		}).
		WithIndexes(entity.NewIndex("idx_schema_sync_test_name", "name"))

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_test`")

	syncer := NewSyncer(db)
	first, err := syncer.Sync(ctx, []entity.Definition{definition})
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if first.CreatedTables != 1 {
		t.Fatalf("expected 1 created table, got %d", first.CreatedTables)
	}
	if first.CreatedIndexes != 1 {
		t.Fatalf("expected 1 inline created index, got %d", first.CreatedIndexes)
	}

	second, err := syncer.Sync(ctx, []entity.Definition{definition})
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if second.CreatedTables != 0 || second.AddedColumns != 0 || second.CreatedIndexes != 0 {
		t.Fatalf("expected idempotent second sync, got %#v", second)
	}
}

func TestSyncerAddsMissingColumnAndIndex(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	baseDefinition := entity.NewDefinition("test", "SchemaPatchBase", "schema_sync_patch_test").
		Comment("补丁测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		})
	targetDefinition := baseDefinition.
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
			entity.NewField("code", "code", "varchar").Size(50).NotNull().Comment("编码"),
		}).
		WithIndexes(entity.NewIndex("idx_schema_sync_patch_test_code", "code"))

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_patch_test`")
	if _, err := db.Exec(ctx, CreateTableSQL(baseDefinition)); err != nil {
		t.Fatalf("create base table failed: %v", err)
	}

	syncer := NewSyncer(db)
	result, err := syncer.Sync(ctx, []entity.Definition{targetDefinition})
	if err != nil {
		t.Fatalf("sync patch failed: %v", err)
	}
	if result.CreatedTables != 0 || result.AddedColumns != 1 || result.CreatedIndexes != 1 {
		t.Fatalf("expected add one column and one index, got %#v", result)
	}

	second, err := syncer.Sync(ctx, []entity.Definition{targetDefinition})
	if err != nil {
		t.Fatalf("second patch sync failed: %v", err)
	}
	if second.CreatedTables != 0 || second.AddedColumns != 0 || second.CreatedIndexes != 0 {
		t.Fatalf("expected idempotent patch sync, got %#v", second)
	}
}

func TestSyncerAddsNotNullDatetimeToExistingRows(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	baseDefinition := entity.NewDefinition("test", "SchemaDatetimePatchBase", "schema_sync_datetime_patch_test").
		Comment("时间补丁测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		})
	targetDefinition := baseDefinition.
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
			entity.NewField("createTime", "createTime", "datetime").NotNull().Comment("创建时间"),
		})

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_datetime_patch_test`")
	if _, err := db.Exec(ctx, CreateTableSQL(baseDefinition)); err != nil {
		t.Fatalf("create base table failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO `schema_sync_datetime_patch_test` (`name`) VALUES ('已有数据')"); err != nil {
		t.Fatalf("insert existing row failed: %v", err)
	}

	syncer := NewSyncer(db)
	result, err := syncer.Sync(ctx, []entity.Definition{targetDefinition})
	if err != nil {
		t.Fatalf("sync datetime patch failed: %v", err)
	}
	if result.CreatedTables != 0 || result.AddedColumns != 1 || result.CreatedIndexes != 0 {
		t.Fatalf("expected add one datetime column, got %#v", result)
	}
}

func TestSyncerRejectsUniqueIndexConflictsWithoutMutatingData(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	definition := entity.NewDefinition("test", "SchemaUniqueConflict", "schema_sync_unique_conflict_test").
		Comment("唯一冲突测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
			entity.NewField("keyName", "keyName", "varchar").Size(100).NotNull().Comment("参数键"),
		}).
		WithIndexes(entity.NewUniqueIndex("uk_schema_sync_unique_conflict_test_key_name", "keyName"))

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_unique_conflict_test`")
	baseDefinition := entity.NewDefinition("test", "SchemaUniqueConflictBase", "schema_sync_unique_conflict_test").
		Comment("唯一冲突测试基础表").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
			entity.NewField("keyName", "keyName", "varchar").Size(100).NotNull().Comment("参数键"),
		})
	if _, err := db.Exec(ctx, CreateTableSQL(baseDefinition)); err != nil {
		t.Fatalf("create base table failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO `schema_sync_unique_conflict_test` (`name`, `keyName`) VALUES ('空值1', ''), ('空值2', ''), ('重复1', 'same'), ('重复2', 'same')"); err != nil {
		t.Fatalf("insert existing rows failed: %v", err)
	}

	syncer := NewSyncer(db)
	result, err := syncer.Sync(ctx, []entity.Definition{definition})
	if err == nil {
		t.Fatal("expected unique conflict error")
	}
	if !strings.Contains(err.Error(), "schema_sync_unique_conflict_test") ||
		!strings.Contains(err.Error(), "uk_schema_sync_unique_conflict_test_key_name") ||
		!strings.Contains(err.Error(), "冲突值") {
		t.Fatalf("expected conflict context, got %v", err)
	}
	if result.CreatedTables != 0 || result.AddedColumns != 0 || result.CreatedIndexes != 0 {
		t.Fatalf("expected no schema changes, got %#v", result)
	}

	var rows []struct {
		KeyName string `json:"keyName"`
	}
	if err := db.GetScan(ctx, &rows, "SELECT `keyName` FROM `schema_sync_unique_conflict_test` ORDER BY `id`"); err != nil {
		t.Fatalf("query existing rows failed: %v", err)
	}
	if len(rows) != 4 || rows[0].KeyName != "" || rows[1].KeyName != "" || rows[2].KeyName != "same" || rows[3].KeyName != "same" {
		t.Fatalf("expected rows to remain unchanged, got %#v", rows)
	}
}

func TestSyncerRejectsCompositeUniqueIndexConflictsWithoutMutatingData(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	definition := entity.NewDefinition("test", "SchemaCompositeUniqueConflict", "schema_sync_composite_unique_conflict_test").
		Comment("复合唯一冲突测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("userId", "userId", "bigint").Unsigned().NotNull().Comment("用户ID"),
			entity.NewField("roleId", "roleId", "bigint").Unsigned().NotNull().Comment("角色ID"),
		}).
		WithIndexes(entity.NewUniqueIndex("uk_schema_sync_composite_unique_conflict", "userId", "roleId"))
	baseDefinition := entity.NewDefinition("test", "SchemaCompositeUniqueConflictBase", "schema_sync_composite_unique_conflict_test").
		Comment("复合唯一冲突基础表").
		Fields(definition.FieldsValue)

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_composite_unique_conflict_test`")
	if _, err := db.Exec(ctx, CreateTableSQL(baseDefinition)); err != nil {
		t.Fatalf("create base table failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO `schema_sync_composite_unique_conflict_test` (`userId`, `roleId`) VALUES (1, 2), (1, 2), (2, 2)"); err != nil {
		t.Fatalf("insert existing rows failed: %v", err)
	}

	syncer := NewSyncer(db)
	result, err := syncer.Sync(ctx, []entity.Definition{definition})
	if err == nil {
		t.Fatal("expected composite unique conflict error")
	}
	if !strings.Contains(err.Error(), "schema_sync_composite_unique_conflict_test") ||
		!strings.Contains(err.Error(), "uk_schema_sync_composite_unique_conflict") ||
		!strings.Contains(err.Error(), "user_id,role_id") ||
		!strings.Contains(err.Error(), "1,2") {
		t.Fatalf("expected composite conflict context, got %v", err)
	}
	if result.CreatedTables != 0 || result.AddedColumns != 0 || result.CreatedIndexes != 0 {
		t.Fatalf("expected no schema changes, got %#v", result)
	}

	count, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `schema_sync_composite_unique_conflict_test` WHERE `userId` = 1 AND `roleId` = 2")
	if err != nil {
		t.Fatalf("query duplicate rows failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected duplicate rows to remain unchanged, got %d", count)
	}
}

func TestSyncerAllowsUniqueIndexWithNullValues(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	definition := entity.NewDefinition("test", "SchemaUniqueNull", "schema_sync_unique_null_test").
		Comment("唯一空值测试").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("code", "code", "varchar").Size(50).Nullable().Comment("编码"),
			entity.NewField("tenantId", "tenantId", "bigint").Unsigned().Nullable().Comment("租户ID"),
			entity.NewField("roleId", "roleId", "bigint").Unsigned().NotNull().Comment("角色ID"),
		}).
		WithIndexes(
			entity.NewUniqueIndex("uk_schema_sync_unique_null_code", "code"),
			entity.NewUniqueIndex("uk_schema_sync_unique_null_tenant_role", "tenantId", "roleId"),
		)
	baseDefinition := entity.NewDefinition("test", "SchemaUniqueNullBase", "schema_sync_unique_null_test").
		Comment("唯一空值基础表").
		Fields(definition.FieldsValue)

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_unique_null_test`")
	if _, err := db.Exec(ctx, CreateTableSQL(baseDefinition)); err != nil {
		t.Fatalf("create base table failed: %v", err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO `schema_sync_unique_null_test` (`code`, `tenantId`, `roleId`) VALUES (NULL, NULL, 1), (NULL, NULL, 1), ('a', 1, 1), ('b', 1, 2)"); err != nil {
		t.Fatalf("insert null rows failed: %v", err)
	}

	syncer := NewSyncer(db)
	result, err := syncer.Sync(ctx, []entity.Definition{definition})
	if err != nil {
		t.Fatalf("sync unique indexes with null values failed: %v", err)
	}
	if result.CreatedTables != 0 || result.AddedColumns != 0 || result.CreatedIndexes != 2 {
		t.Fatalf("expected two indexes created, got %#v", result)
	}
}

func TestColumnDefinitionMatchesRejectsMismatchedExistingColumn(t *testing.T) {
	actual := columnDefinition{
		Name:       "code",
		DataType:   "int",
		MaxLength:  0,
		IsNullable: false,
	}
	target := entity.NewField("code", "code", "varchar").Size(50).NotNull()
	if columnDefinitionMatches(actual, target) {
		t.Fatal("expected mismatched column definition to be rejected")
	}
}

func TestColumnDefinitionMatchesRejectsUnsignedMismatch(t *testing.T) {
	actual := columnDefinition{
		Name:       "userId",
		DataType:   "bigint",
		MaxLength:  20,
		IsNullable: false,
		IsUnsigned: false,
	}
	target := entity.NewField("userId", "userId", "bigint").Unsigned().NotNull()
	if columnDefinitionMatches(actual, target) {
		t.Fatal("expected unsigned mismatch to be rejected")
	}
}

func TestColumnDefinitionMatchesRejectsDefaultMismatch(t *testing.T) {
	actual := columnDefinition{
		Name:         "created_at",
		DataType:     "datetime",
		IsNullable:   false,
		DefaultValue: "NULL",
	}
	target := entity.NewField("createdAt", "created_at", "datetime").NotNull()
	if columnDefinitionMatches(actual, target) {
		t.Fatal("expected default mismatch to be rejected")
	}
}

func TestColumnDefinitionMatchesRejectsAutoIncrementMismatch(t *testing.T) {
	actual := columnDefinition{
		Name:            "id",
		DataType:        "bigint",
		MaxLength:       20,
		IsNullable:      false,
		IsUnsigned:      true,
		IsAutoIncrement: false,
	}
	target := entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement()
	if columnDefinitionMatches(actual, target) {
		t.Fatal("expected auto increment mismatch to be rejected")
	}
}

func TestIndexDefinitionMatchesRejectsMismatchedExistingIndex(t *testing.T) {
	actual := indexDefinition{
		Name:      "uk_test",
		Columns:   []string{"roleId", "userId"},
		IsUnique:  true,
		IndexType: "BTREE",
		SubParts:  []int{0, 0},
	}
	target := entity.NewUniqueIndex("uk_test", "userId", "roleId")
	if indexDefinitionMatches(actual, target) {
		t.Fatal("expected mismatched index definition to be rejected")
	}
}

func TestIndexDefinitionMatchesRejectsPrefixSubPart(t *testing.T) {
	actual := indexDefinition{
		Name:      "idx_code",
		Columns:   []string{"code"},
		IsUnique:  false,
		IndexType: "BTREE",
		SubParts:  []int{10},
	}
	target := entity.NewIndex("idx_code", "code")
	if indexDefinitionMatches(actual, target) {
		t.Fatal("expected prefix index to be rejected")
	}
}
