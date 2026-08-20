package tenant

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// MigrationRunner 表示可显式执行租户数据迁移的数据库提供者
type MigrationRunner interface {
	Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// MigrationResult 保存 legacy tenant 迁移结果
type MigrationResult struct {
	Tables   []string
	Affected int64
}

/**
 * 将租户感知表中的 legacy tenant zero 迁移为 NULL
 * @param ctx 上下文
 * @param runner 数据库执行器
 * @param definitions 模型定义
 * @returns 迁移结果和错误
 */
func MigrateLegacyTenantZero(ctx context.Context, runner MigrationRunner, definitions []entity.Definition) (MigrationResult, error) {
	if runner == nil {
		return MigrationResult{}, gerror.New("legacy tenant 迁移缺少 Runner")
	}
	tables, err := compileMigrationTables(definitions)
	if err != nil {
		return MigrationResult{}, err
	}
	result := MigrationResult{Tables: tables}
	for _, tableName := range tables {
		query := fmt.Sprintf(
			"UPDATE %s SET %s = NULL WHERE %s = ?",
			quoteIdentifier(tableName),
			quoteIdentifier(tenantColumn),
			quoteIdentifier(tenantColumn),
		)
		execResult, execErr := runner.Exec(ctx, query, int64(0))
		if execErr != nil {
			return result, gerror.Wrapf(execErr, "迁移 legacy tenant 数据失败: %s", tableName)
		}
		if execResult == nil {
			return result, gerror.Newf("legacy tenant 迁移结果为空: %s", tableName)
		}
		affected, affectedErr := execResult.RowsAffected()
		if affectedErr != nil {
			return result, gerror.Wrapf(affectedErr, "读取 legacy tenant 迁移结果失败: %s", tableName)
		}
		result.Affected += affected
	}
	return result, nil
}

/**
 * 编译并去重需要迁移的租户感知表
 * @param definitions 模型定义
 * @returns 安全表名和错误
 */
func compileMigrationTables(definitions []entity.Definition) ([]string, error) {
	tables := make([]string, 0, len(definitions))
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		metadata, err := CompileMetadata(definition)
		if err != nil {
			return nil, gerror.Wrapf(err, "编译 legacy tenant 迁移模型失败: %s", definition.Name)
		}
		if !metadata.IsAware() {
			continue
		}
		if !isIdentifier(definition.TableName) {
			return nil, gerror.Newf("legacy tenant 迁移数据表名无效: %s", definition.TableName)
		}
		if _, exists := seen[definition.TableName]; exists {
			continue
		}
		seen[definition.TableName] = struct{}{}
		tables = append(tables, definition.TableName)
	}
	return tables, nil
}
