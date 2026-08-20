package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// schema 同步结果
type SyncResult struct {
	CreatedTables  int
	AddedColumns   int
	CreatedIndexes int
}

// MySQL schema 同步器
type Syncer struct {
	db gdb.DB
}

/**
 * 创建 schema 同步器
 * @param db GoFrame 数据库实例
 * @returns *Syncer
 */
func NewSyncer(db gdb.DB) *Syncer {
	return &Syncer{
		db: db,
	}
}

/**
 * 同步表结构
 * @param ctx 上下文
 * @param definitions 模型定义列表
 * @returns SyncResult
 */
func (s *Syncer) Sync(ctx context.Context, definitions []entity.Definition) (SyncResult, error) {
	result := SyncResult{}
	for _, definition := range definitions {
		isExists, err := s.tableExists(ctx, definition.TableName)
		if err != nil {
			return result, gerror.Wrap(err, fmt.Sprintf("检查表是否存在失败: %s", definition.TableName))
		}
		if !isExists {
			if _, err = s.db.Exec(ctx, CreateTableSQL(definition)); err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("创建表失败: %s", definition.TableName))
			}
			result.CreatedTables++
			result.CreatedIndexes += len(definition.Indexes)
			continue
		}

		for _, field := range definition.FieldsValue {
			isColumnExists, err := s.columnExists(ctx, definition.TableName, field.ColumnName)
			if err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("检查字段是否存在失败: %s.%s", definition.TableName, field.ColumnName))
			}
			if !isColumnExists {
				if _, err = s.db.Exec(ctx, AddColumnSQL(definition.TableName, field)); err != nil {
					isMatch, checkErr := s.columnMatches(ctx, definition.TableName, field)
					if checkErr != nil {
						return result, gerror.Wrap(checkErr, fmt.Sprintf("重新检查字段定义失败: %s.%s", definition.TableName, field.ColumnName))
					}
					if !isMatch {
						return result, gerror.Wrap(err, fmt.Sprintf("新增字段失败: %s.%s", definition.TableName, field.ColumnName))
					}
				} else {
					result.AddedColumns++
				}
			}
		}

		for _, index := range definition.Indexes {
			isIndexExists, err := s.indexExists(ctx, definition.TableName, index.Name)
			if err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("检查索引是否存在失败: %s.%s", definition.TableName, index.Name))
			}
			if !isIndexExists {
				if err = s.checkUniqueIndexConflicts(ctx, definition.TableName, index); err != nil {
					return result, gerror.Wrap(err, fmt.Sprintf("检查唯一索引冲突失败: %s.%s", definition.TableName, index.Name))
				}
				if _, err = s.db.Exec(ctx, CreateIndexSQL(definition.TableName, index)); err != nil {
					isMatch, checkErr := s.indexMatches(ctx, definition.TableName, index)
					if checkErr != nil {
						return result, gerror.Wrap(checkErr, fmt.Sprintf("重新检查索引定义失败: %s.%s", definition.TableName, index.Name))
					}
					if !isMatch {
						return result, gerror.Wrap(err, fmt.Sprintf("创建索引失败: %s.%s", definition.TableName, index.Name))
					}
				} else {
					result.CreatedIndexes++
				}
			}
		}
	}
	return result, nil
}

/**
 * 表是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @returns bool
 */
func (s *Syncer) tableExists(ctx context.Context, tableName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", tableName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

/**
 * 字段是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @param columnName 字段名
 * @returns bool
 */
func (s *Syncer) columnExists(ctx context.Context, tableName string, columnName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", tableName, columnName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// 检查唯一索引的重复值
func (s *Syncer) checkUniqueIndexConflicts(ctx context.Context, tableName string, index entity.Index) error {
	if !index.IsUnique || len(index.Columns) == 0 {
		return nil
	}

	selectParts := make([]string, 0, len(index.Columns)+1)
	whereParts := make([]string, 0, len(index.Columns))
	groupParts := make([]string, 0, len(index.Columns))
	for _, columnName := range index.Columns {
		quoted := quoteIdentifier(columnName)
		selectParts = append(selectParts, fmt.Sprintf("%s AS %s", quoted, quoteIdentifier(columnName)))
		whereParts = append(whereParts, fmt.Sprintf("%s IS NOT NULL", quoted))
		groupParts = append(groupParts, quoted)
	}
	selectParts = append(selectParts, "COUNT(*) AS conflict_count")
	rows, err := s.db.GetAll(ctx, fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s GROUP BY %s HAVING COUNT(*) > 1",
		strings.Join(selectParts, ", "),
		quoteIdentifier(tableName),
		strings.Join(whereParts, " AND "),
		strings.Join(groupParts, ", "),
	))
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	conflict := rows[0]
	values := make([]string, 0, len(index.Columns))
	for _, columnName := range index.Columns {
		values = append(values, conflict[columnName].String())
	}
	return gerror.Newf(
		"表 %s 索引 %s 存在重复值: 冲突列组合=%s, 冲突值=%s, 数量=%d, 冲突组数=%d",
		tableName,
		index.Name,
		strings.Join(index.Columns, ","),
		strings.Join(values, ","),
		conflict["conflict_count"].Int(),
		len(rows),
	)
}

/**
 * 索引是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @param indexName 索引名
 * @returns bool
 */
func (s *Syncer) indexExists(ctx context.Context, tableName string, indexName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?", tableName, indexName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// 数据库中已有字段定义
type columnDefinition struct {
	Name            string
	DataType        string
	MaxLength       int
	IsNullable      bool
	IsUnsigned      bool
	IsAutoIncrement bool
	DefaultValue    string
}

// 数据库中已有索引定义
type indexDefinition struct {
	Name      string
	Columns   []string
	IsUnique  bool
	IndexType string
	SubParts  []int
}

func (s *Syncer) columnMatches(ctx context.Context, tableName string, field entity.Field) (bool, error) {
	var rows []struct {
		ColumnName             string `json:"column_name"`
		DataType               string `json:"dataType"`
		ColumnType             string `json:"column_type"`
		CharacterMaximumLength int    `json:"character_maximum_length"`
		NumericPrecision       int    `json:"numeric_precision"`
		IsNullable             string `json:"is_nullable"`
		ColumnDefault          string `json:"column_default"`
		Extra                  string `json:"extra"`
	}
	if err := s.db.GetScan(ctx, &rows, "SELECT column_name, data_type, column_type, character_maximum_length, numeric_precision, is_nullable, column_default, extra FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", tableName, field.ColumnName); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	maxLength := rows[0].CharacterMaximumLength
	if maxLength == 0 {
		maxLength = rows[0].NumericPrecision
	}
	return columnDefinitionMatches(columnDefinition{
		Name:            rows[0].ColumnName,
		DataType:        rows[0].DataType,
		MaxLength:       maxLength,
		IsNullable:      rows[0].IsNullable == "YES",
		IsUnsigned:      strings.Contains(strings.ToLower(rows[0].ColumnType), "unsigned"),
		IsAutoIncrement: strings.Contains(strings.ToLower(rows[0].Extra), "auto_increment"),
		DefaultValue:    rows[0].ColumnDefault,
	}, field), nil
}

func columnDefinitionMatches(actual columnDefinition, target entity.Field) bool {
	if actual.Name != target.ColumnName || actual.DataType != target.DataType || actual.IsNullable != target.IsNullable {
		return false
	}
	if actual.IsUnsigned != target.IsUnsigned || actual.IsAutoIncrement != target.IsAutoIncrement {
		return false
	}
	if target.Length > 0 && actual.MaxLength != target.Length {
		return false
	}
	return defaultDefinitionMatches(actual.DefaultValue, target)
}

func defaultDefinitionMatches(actual string, target entity.Field) bool {
	if target.HasDefault {
		return normalizeDefaultValue(actual) == normalizeDefaultValue(target.DefaultValue)
	}
	if shouldUseSafeDatetimeDefault(target) {
		return normalizeDefaultValue(actual) == "CURRENT_TIMESTAMP"
	}
	return actual == ""
}

func normalizeDefaultValue(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.Trim(trimmed, "'")
	return strings.ToUpper(trimmed)
}

func (s *Syncer) indexMatches(ctx context.Context, tableName string, index entity.Index) (bool, error) {
	var rows []struct {
		IndexName  string `json:"index_name"`
		ColumnName string `json:"column_name"`
		NonUnique  int    `json:"non_unique"`
		IndexType  string `json:"index_type"`
		SubPart    int    `json:"sub_part"`
	}
	if err := s.db.GetScan(ctx, &rows, "SELECT index_name, column_name, non_unique, index_type, sub_part FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? ORDER BY seq_in_index", tableName, index.Name); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	columns := make([]string, 0, len(rows))
	subParts := make([]int, 0, len(rows))
	for _, row := range rows {
		columns = append(columns, row.ColumnName)
		subParts = append(subParts, row.SubPart)
	}
	return indexDefinitionMatches(indexDefinition{
		Name:      rows[0].IndexName,
		Columns:   columns,
		IsUnique:  rows[0].NonUnique == 0,
		IndexType: rows[0].IndexType,
		SubParts:  subParts,
	}, index), nil
}

func indexDefinitionMatches(actual indexDefinition, target entity.Index) bool {
	if actual.Name != target.Name || actual.IsUnique != target.IsUnique || len(actual.Columns) != len(target.Columns) {
		return false
	}
	if !strings.EqualFold(actual.IndexType, "BTREE") || len(actual.SubParts) != len(target.Columns) {
		return false
	}
	for i, columnName := range target.Columns {
		if actual.Columns[i] != columnName || actual.SubParts[i] != 0 {
			return false
		}
	}
	return true
}
