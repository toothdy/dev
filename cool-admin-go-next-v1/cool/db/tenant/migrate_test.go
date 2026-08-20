package tenant

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

var _ MigrationRunner = (gdb.DB)(nil)

type migrationCall struct {
	query string
	args  []interface{}
}

type migrationTestRunner struct {
	calls     []migrationCall
	remaining map[string]int64
	err       error
	nilResult bool
}

/**
 * 模拟执行 legacy tenant 迁移 SQL
 * @param ctx 上下文
 * @param query SQL
 * @param args SQL 参数
 * @returns SQL 结果和错误
 */
func (r *migrationTestRunner) Exec(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	r.calls = append(r.calls, migrationCall{query: query, args: append([]interface{}{}, args...)})
	if r.err != nil {
		return nil, r.err
	}
	if r.nilResult {
		return nil, nil
	}
	for tableName, count := range r.remaining {
		if strings.Contains(query, quoteIdentifier(tableName)) {
			r.remaining[tableName] = 0
			return migrationSQLResult{affected: count}, nil
		}
	}
	return migrationSQLResult{}, nil
}

type migrationSQLResult struct {
	affected int64
	err      error
}

func (migrationSQLResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r migrationSQLResult) RowsAffected() (int64, error) {
	return r.affected, r.err
}

/**
 * 创建租户感知模型定义
 * @param tableName 数据表名
 * @returns 模型定义
 */
func migrationAwareDefinition(tableName string) entity.Definition {
	return entity.NewDefinition("demo", tableName, tableName).Fields(entity.BaseFields())
}

/**
 * 验证迁移仅处理编译为租户感知的表且每表执行一次
 * @param t 测试上下文
 * @returns null
 */
func TestMigrateLegacyTenantZeroSelectsAwareTables(t *testing.T) {
	goods := migrationAwareDefinition("demo_goods")
	orders := migrationAwareDefinition("demo_orders")
	relation := entity.NewDefinition("demo", "GoodsTag", "demo_goods_tag").Fields([]entity.Field{
		entity.NewField("goodsId", "goods_id", "bigint").Unsigned().NotNull(),
	})
	disabled := migrationAwareDefinition("demo_global").WithTenantMode(entity.TenantModeDisabled)
	runner := &migrationTestRunner{remaining: map[string]int64{
		"demo_goods":  2,
		"demo_orders": 3,
	}}

	result, err := MigrateLegacyTenantZero(context.Background(), runner, []entity.Definition{
		goods,
		relation,
		disabled,
		orders,
		goods,
	})
	if err != nil {
		t.Fatalf("migrate legacy tenant zero failed: %v", err)
	}
	if !reflect.DeepEqual(result.Tables, []string{"demo_goods", "demo_orders"}) || result.Affected != 5 {
		t.Fatalf("unexpected migration result: %#v", result)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("unexpected migration calls: %#v", runner.calls)
	}
	for _, call := range runner.calls {
		if !strings.Contains(call.query, "SET `tenantId` = NULL") || !strings.Contains(call.query, "WHERE `tenantId` = ?") {
			t.Fatalf("unexpected migration SQL: %s", call.query)
		}
		if !reflect.DeepEqual(call.args, []interface{}{int64(0)}) {
			t.Fatalf("unexpected migration arguments: %#v", call.args)
		}
	}
}

/**
 * 验证 legacy tenant 迁移可以安全重复执行
 * @param t 测试上下文
 * @returns null
 */
func TestMigrateLegacyTenantZeroIsRepeatable(t *testing.T) {
	runner := &migrationTestRunner{remaining: map[string]int64{"demo_goods": 4}}
	definitions := []entity.Definition{migrationAwareDefinition("demo_goods")}

	first, err := MigrateLegacyTenantZero(context.Background(), runner, definitions)
	if err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	second, err := MigrateLegacyTenantZero(context.Background(), runner, definitions)
	if err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
	if first.Affected != 4 || second.Affected != 0 || len(runner.calls) != 2 {
		t.Fatalf("migration was not repeatable: first=%#v second=%#v calls=%d", first, second, len(runner.calls))
	}
}

/**
 * 验证非法模型定义和表名在执行 SQL 前失败
 * @param t 测试上下文
 * @returns null
 */
func TestMigrateLegacyTenantZeroRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name       string
		definition entity.Definition
		message    string
	}{
		{
			name: "invalid metadata",
			definition: entity.NewDefinition("demo", "Required", "demo_required").
				WithTenantMode(entity.TenantModeRequired),
			message: "模型缺少租户字段",
		},
		{
			name:       "unsafe table name",
			definition: migrationAwareDefinition("demo_goods; DROP TABLE demo_goods"),
			message:    "数据表名无效",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &migrationTestRunner{}
			_, err := MigrateLegacyTenantZero(context.Background(), runner, []entity.Definition{test.definition})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q error, got %v", test.message, err)
			}
			if len(runner.calls) != 0 {
				t.Fatalf("invalid definition executed SQL: %#v", runner.calls)
			}
		})
	}
}

/**
 * 验证 Runner 和数据库错误会明确返回
 * @param t 测试上下文
 * @returns null
 */
func TestMigrateLegacyTenantZeroReportsRunnerErrors(t *testing.T) {
	definition := migrationAwareDefinition("demo_goods")
	if _, err := MigrateLegacyTenantZero(context.Background(), nil, []entity.Definition{definition}); err == nil {
		t.Fatal("expected missing runner error")
	}

	runner := &migrationTestRunner{err: errors.New("database unavailable")}
	_, err := MigrateLegacyTenantZero(context.Background(), runner, []entity.Definition{definition})
	if err == nil || !strings.Contains(err.Error(), "database unavailable") || !strings.Contains(err.Error(), "demo_goods") {
		t.Fatalf("unexpected execution error: %v", err)
	}

	runner = &migrationTestRunner{nilResult: true}
	_, err = MigrateLegacyTenantZero(context.Background(), runner, []entity.Definition{definition})
	if err == nil || !strings.Contains(err.Error(), "迁移结果为空") {
		t.Fatalf("unexpected empty result error: %v", err)
	}
}
