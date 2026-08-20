package seed

import (
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

const (
	childDatasKey = "@childDatas"
	childMenusKey = "childMenus"
)

// seed 文件中的原始记录
type RawRecord map[string]interface{}

// 映射到数据库字段后的记录
type MappedRecord struct {
	TableName    string
	Values       map[string]interface{}
	ParentIndex  int
	ParentColumn string
}

// 按表名索引的模型定义
type ModelMap map[string]entity.Definition

/**
 * 创建模型映射
 * @param definitions 模型定义列表
 * @returns ModelMap
 */
func NewModelMap(definitions []entity.Definition) ModelMap {
	items := make(ModelMap, len(definitions))
	for _, definition := range definitions {
		items[definition.TableName] = definition
	}
	return items
}

/**
 * 映射 seed 记录
 * @param models 模型映射
 * @param tableName 表名
 * @param record 原始记录
 * @param parent 父级记录
 * @returns 映射记录
 */
func MapRecord(models ModelMap, tableName string, record RawRecord, parent RawRecord) (MappedRecord, error) {
	definition, ok := models[tableName]
	if !ok {
		return MappedRecord{}, gerror.Newf("未知 seed 表: %s", tableName)
	}

	fields := fieldsByJSONName(definition)
	values := make(map[string]interface{}, len(record))
	for jsonName, value := range record {
		if isControlField(jsonName) {
			continue
		}

		field, ok := fields[jsonName]
		if !ok {
			return MappedRecord{}, gerror.Newf("未知 seed 字段: %s.%s", tableName, jsonName)
		}

		resolved, err := resolveValue(value, parent)
		if err != nil {
			return MappedRecord{}, gerror.Wrap(err, "解析 seed 字段引用失败")
		}
		values[field.ColumnName] = resolved
	}
	now := time.Now().Format("2006-01-02 15:04:05")
	for _, fieldName := range []string{"createTime", "updateTime"} {
		if field, exists := fields[fieldName]; exists {
			if _, provided := values[field.ColumnName]; !provided {
				values[field.ColumnName] = now
			}
		}
	}

	return MappedRecord{
		TableName:   tableName,
		Values:      values,
		ParentIndex: -1,
	}, nil
}

/**
 * 按 JSON 字段名索引字段
 * @param definition 模型定义
 * @returns map[string]entity.Field
 */
func fieldsByJSONName(definition entity.Definition) map[string]entity.Field {
	fields := make(map[string]entity.Field, len(definition.FieldsValue))
	for _, field := range definition.FieldsValue {
		fields[field.JSONName] = field
	}
	return fields
}

/**
 * 是否为控制字段
 * @param name 字段名
 * @returns bool
 */
func isControlField(name string) bool {
	return name == childDatasKey || name == childMenusKey
}

/**
 * 解析字段值
 * @param value 原始值
 * @param parent 父级记录
 * @returns 解析后的值
 */
func resolveValue(value interface{}, parent RawRecord) (interface{}, error) {
	text, ok := value.(string)
	if !ok || !strings.HasPrefix(text, "@") || len(text) == 1 {
		return value, nil
	}
	if parent == nil {
		return nil, gerror.Newf("引用 %s 缺少父级记录", text)
	}

	fieldName := strings.TrimPrefix(text, "@")
	parentValue, ok := parent[fieldName]
	if !ok {
		return nil, gerror.Newf("父级记录不存在字段: %s", fieldName)
	}
	return parentValue, nil
}
