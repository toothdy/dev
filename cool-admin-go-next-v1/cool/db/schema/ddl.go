package schema

import (
	"fmt"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// 生成创建表 SQL
// @param definition 模型定义
// @returns 创建表 SQL
func CreateTableSQL(definition entity.Definition) string {
	parts := make([]string, 0, len(definition.FieldsValue)+len(definition.Indexes)+1)
	primaryColumns := make([]string, 0)

	for _, field := range definition.FieldsValue {
		parts = append(parts, ColumnSQL(field))
		if field.IsPrimary {
			primaryColumns = append(primaryColumns, quoteIdentifier(field.ColumnName))
		}
	}
	if len(primaryColumns) > 0 {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryColumns, ", ")))
	}
	for _, index := range definition.Indexes {
		parts = append(parts, inlineIndexSQL(index))
	}

	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n  %s\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='%s'",
		quoteIdentifier(definition.TableName),
		strings.Join(parts, ",\n  "),
		escapeSQLString(definition.CommentText),
	)
}

// 生成新增字段 SQL
// @param tableName 表名
// @param field 字段元数据
// @returns 新增字段 SQL
func AddColumnSQL(tableName string, field entity.Field) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdentifier(tableName), ColumnSQL(field))
}

// 生成创建索引 SQL
// @param tableName 表名
// @param index 索引元数据
// @returns 创建索引 SQL
func CreateIndexSQL(tableName string, index entity.Index) string {
	kind := "INDEX"
	if index.IsUnique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf(
		"CREATE %s %s ON %s (%s)",
		kind,
		quoteIdentifier(index.Name),
		quoteIdentifier(tableName),
		quoteIdentifiers(index.Columns),
	)
}

// 生成字段 SQL
// @param field 字段元数据
// @returns 字段 SQL
func ColumnSQL(field entity.Field) string {
	parts := []string{quoteIdentifier(field.ColumnName), columnTypeSQL(field)}
	if field.IsNullable {
		parts = append(parts, "NULL")
	} else {
		parts = append(parts, "NOT NULL")
	}
	if field.IsAutoIncrement {
		parts = append(parts, "AUTO_INCREMENT")
	}
	if field.HasDefault {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", field.DefaultValue))
	} else if shouldUseSafeDatetimeDefault(field) {
		parts = append(parts, "DEFAULT CURRENT_TIMESTAMP")
	}
	if field.CommentText != "" {
		parts = append(parts, fmt.Sprintf("COMMENT '%s'", escapeSQLString(field.CommentText)))
	}
	return strings.Join(parts, " ")
}

// 生成字段类型 SQL
// @param field 字段元数据
// @returns 字段类型 SQL
func columnTypeSQL(field entity.Field) string {
	dataType := field.DataType
	if field.Length > 0 && supportsLength(dataType) {
		dataType = fmt.Sprintf("%s(%d)", dataType, field.Length)
	} else if strings.EqualFold(dataType, "varchar") {
		dataType = "varchar(255)"
	}
	if field.IsUnsigned {
		dataType = fmt.Sprintf("%s unsigned", dataType)
	}
	return dataType
}

// 判断字段类型是否支持长度
// @param dataType 字段类型
// @returns 是否支持长度
func supportsLength(dataType string) bool {
	switch dataType {
	case "varchar", "char", "int", "tinyint", "bigint":
		return true
	default:
		return false
	}
}

// 判断是否需要安全时间默认值
// @param field 字段元数据
// @returns 是否需要默认当前时间
func shouldUseSafeDatetimeDefault(field entity.Field) bool {
	return field.DataType == "datetime" && !field.IsNullable && !field.HasDefault
}

// 生成建表语句中的索引 SQL
// @param index 索引元数据
// @returns 内联索引 SQL
func inlineIndexSQL(index entity.Index) string {
	kind := "KEY"
	if index.IsUnique {
		kind = "UNIQUE KEY"
	}
	return fmt.Sprintf("%s %s (%s)", kind, quoteIdentifier(index.Name), quoteIdentifiers(index.Columns))
}

// 引用 SQL 标识符
// @param name 标识符
// @returns 已引用标识符
func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// 引用多个 SQL 标识符
// @param names 标识符列表
// @returns 已引用标识符列表
func quoteIdentifiers(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, quoteIdentifier(name))
	}
	return strings.Join(quoted, ", ")
}

// 转义 SQL 字符串
// @param value 字符串
// @returns 已转义字符串
func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
