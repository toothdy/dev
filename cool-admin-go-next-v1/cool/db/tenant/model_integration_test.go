package tenant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

type scopedModelIntegrationDO struct {
	Name     interface{} `orm:"name"`
	TenantID interface{} `orm:"tenantId"`
}

/**
 * 验证真实 MySQL 中 DB、TX 和事务回滚的租户边界
 * @param t 测试上下文
 * @returns null
 */
func TestScopedModelMySQLDBAndTransaction(t *testing.T) {
	if os.Getenv("COOL_TENANT_INTEGRATION") != "1" {
		t.Skip("set COOL_TENANT_INTEGRATION=1 to run real MySQL tenant model test")
	}

	var (
		ctx       = context.Background()
		db        = g.DB()
		tableName = fmt.Sprintf("cool_tenant_scope_test_%d", time.Now().UnixNano())
	)
	_, err := db.Exec(ctx, fmt.Sprintf(
		"CREATE TABLE `%s` (`id` bigint unsigned NOT NULL AUTO_INCREMENT, `name` varchar(64) NOT NULL, `tenantId` bigint unsigned NULL, PRIMARY KEY (`id`), KEY `idx_tenant` (`tenantId`)) ENGINE=InnoDB",
		tableName,
	))
	if err != nil {
		t.Fatalf("create tenant model integration table failed: %v", err)
	}
	defer func() {
		if _, dropErr := db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS `%s`", tableName)); dropErr != nil {
			t.Errorf("drop tenant model integration table failed: %v", dropErr)
		}
	}()

	definition := entity.NewDefinition("test", "TenantScope", tableName).Fields([]entity.Field{
		entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement(),
		entity.NewField("name", "name", "varchar").NotNull(),
		entity.NewField("tenantId", "tenantId", "bigint").Unsigned().Nullable(),
	})
	tenantA := scopedModelTenantContext(t, 101)
	tenantB := scopedModelTenantContext(t, 102)

	modelA, err := ScopedModel(tenantA, db, definition, "")
	if err != nil {
		t.Fatalf("create tenant A database model failed: %v", err)
	}
	if _, err = modelA.Data(scopedModelIntegrationDO{Name: "persisted", TenantID: int64(999)}).Insert(); err != nil {
		t.Fatalf("insert tenant A row failed: %v", err)
	}

	rollbackErr := errors.New("rollback tenant model test")
	err = db.Transaction(tenantA, func(ctx context.Context, tx gdb.TX) error {
		txModel, modelErr := ScopedModel(ctx, tx, definition, "")
		if modelErr != nil {
			return modelErr
		}
		if _, modelErr = txModel.Data(scopedModelIntegrationDO{Name: "rolled-back"}).Insert(); modelErr != nil {
			return modelErr
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("tenant transaction did not return rollback cause: %v", err)
	}

	modelA, err = ScopedModel(tenantA, db, definition, "")
	if err != nil {
		t.Fatalf("recreate tenant A database model failed: %v", err)
	}
	countA, err := modelA.Count()
	if err != nil || countA != 1 {
		t.Fatalf("unexpected tenant A row count: count=%d err=%v", countA, err)
	}
	platformModel, err := ScopedModel(scopedModelPlatformContext(), db, definition, "")
	if err != nil {
		t.Fatalf("create platform database model failed: %v", err)
	}
	rolledBackCount, err := platformModel.Where("name", "rolled-back").Count()
	if err != nil || rolledBackCount != 0 {
		t.Fatalf("transaction row was not rolled back: count=%d err=%v", rolledBackCount, err)
	}

	modelB, err := ScopedModel(tenantB, db, definition, "")
	if err != nil {
		t.Fatalf("create tenant B database model failed: %v", err)
	}
	countB, err := modelB.Count()
	if err != nil || countB != 0 {
		t.Fatalf("tenant B observed tenant A rows: count=%d err=%v", countB, err)
	}
	updateResult, err := modelB.Data(scopedModelIntegrationDO{Name: "forged"}).Where("name", "persisted").Update()
	if err != nil {
		t.Fatalf("tenant B update failed unexpectedly: %v", err)
	}
	updated, err := updateResult.RowsAffected()
	if err != nil || updated != 0 {
		t.Fatalf("tenant B updated tenant A row: affected=%d err=%v", updated, err)
	}
	deleteResult, err := modelB.Where("name", "persisted").Delete()
	if err != nil {
		t.Fatalf("tenant B delete failed unexpectedly: %v", err)
	}
	deleted, err := deleteResult.RowsAffected()
	if err != nil || deleted != 0 {
		t.Fatalf("tenant B deleted tenant A row: affected=%d err=%v", deleted, err)
	}
	rawCount, err := db.GetCount(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName))
	if err != nil || rawCount != 1 {
		t.Fatalf("raw SQL boundary returned unexpected count: count=%d err=%v", rawCount, err)
	}
}
