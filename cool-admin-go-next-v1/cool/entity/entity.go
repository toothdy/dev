package entity

import "strings"

// TenantMode 定义模型的租户元数据策略
type TenantMode uint8

const (
	// TenantModeAuto 根据规范字段自动启用租户隔离
	TenantModeAuto TenantMode = iota
	// TenantModeRequired 要求模型包含规范租户字段
	TenantModeRequired
	// TenantModeDisabled 显式关闭模型租户隔离
	TenantModeDisabled
)

// 表字段元数据
type Field struct {
	JSONName        string
	ColumnName      string
	DataType        string
	Length          int
	CommentText     string
	DefaultValue    string
	HasDefault      bool
	IsNullable      bool
	IsPrimary       bool
	IsAutoIncrement bool
	IsUnsigned      bool
	Dict            []string
}

// 表索引元数据
type Index struct {
	Name     string
	Columns  []string
	IsUnique bool
}

// 表模型元数据
type Definition struct {
	Module      string
	Name        string
	Resource    string
	TableName   string
	CommentText string
	FieldsValue []Field
	Indexes     []Index
	TenantMode  TenantMode
}

/**
 * 设置模型稳定资源名
 * @param resource 稳定资源名
 * @returns 模型定义
 */
func (d Definition) WithResource(resource string) Definition {
	d.Resource = resource
	return d
}

/**
 * 返回模型稳定资源名
 * @returns string
 */
func (d Definition) ResourceKey() string {
	if d.Resource != "" {
		return d.Resource
	}
	name := d.Name
	if len(name) > 0 && len(d.Module) > 0 && len(name) >= len(d.Module) && strings.EqualFold(name[:len(d.Module)], d.Module) {
		name = name[len(d.Module):]
	}
	if name == "" {
		name = d.Name
	}
	if len(name) > 3 && strings.EqualFold(name[:3], "sys") {
		name = name[3:]
	}
	if name != "" {
		name = strings.ToLower(name[:1]) + name[1:]
	}
	if d.Module == "" {
		return name
	}
	return d.Module + "." + name
}

// 创建字段元数据
// @param jsonName JSON 或 EPS 字段名
// @param columnName 数据库字段名
// @param dataType MySQL 类型
// @returns 字段元数据
func NewField(jsonName string, columnName string, dataType string) Field {
	return Field{
		JSONName:   jsonName,
		ColumnName: columnName,
		DataType:   dataType,
	}
}

// 设置字段长度
// @param length 字段长度
// @returns 字段元数据
func (f Field) Size(length int) Field {
	f.Length = length
	return f
}

// 设置字段注释
// @param comment 字段注释
// @returns 字段元数据
func (f Field) Comment(comment string) Field {
	f.CommentText = comment
	return f
}

// 设置字段不可空
// @returns 字段元数据
func (f Field) NotNull() Field {
	f.IsNullable = false
	return f
}

// 设置字段可空
// @returns 字段元数据
func (f Field) Nullable() Field {
	f.IsNullable = true
	return f
}

// 设置字段默认值
// @param value 默认值 SQL 片段
// @returns 字段元数据
func (f Field) Default(value string) Field {
	f.DefaultValue = value
	f.HasDefault = true
	return f
}

// 设置字段为主键
// @returns 字段元数据
func (f Field) Primary() Field {
	f.IsPrimary = true
	f.IsNullable = false
	return f
}

// 设置字段自增
// @returns 字段元数据
func (f Field) AutoIncrement() Field {
	f.IsAutoIncrement = true
	return f
}

// 设置无符号数字字段
// @returns 字段元数据
func (f Field) Unsigned() Field {
	f.IsUnsigned = true
	return f
}

// 设置字段字典
// @param items 字典项
// @returns 字段元数据
func (f Field) WithDict(items ...string) Field {
	f.Dict = append([]string{}, items...)
	return f
}

// 创建普通索引
// @param name 索引名
// @param columns 字段名
// @returns 索引元数据
func NewIndex(name string, columns ...string) Index {
	return Index{
		Name:    name,
		Columns: append([]string{}, columns...),
	}
}

// 创建唯一索引
// @param name 索引名
// @param columns 字段名
// @returns 索引元数据
func NewUniqueIndex(name string, columns ...string) Index {
	return Index{
		Name:     name,
		Columns:  append([]string{}, columns...),
		IsUnique: true,
	}
}

// 返回基础字段列表
// @returns 基础字段元数据
func BaseFields() []Field {
	return []Field{
		NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
		NewField("createTime", "createTime", "varchar").NotNull().Comment("创建时间"),
		NewField("updateTime", "updateTime", "varchar").NotNull().Comment("更新时间"),
		NewField("tenantId", "tenantId", "bigint").Unsigned().Nullable().Comment("租户ID"),
	}
}

// 创建模型定义
// @param module 模块名
// @param name 模型名
// @param tableName 表名
// @returns 模型定义
func NewDefinition(module string, name string, tableName string) Definition {
	return Definition{
		Module:    module,
		Name:      name,
		TableName: tableName,
	}
}

// 设置表注释
// @param comment 表注释
// @returns 模型定义
func (d Definition) Comment(comment string) Definition {
	d.CommentText = comment
	return d
}

// 设置字段列表
// @param fields 字段列表
// @returns 模型定义
func (d Definition) Fields(fields []Field) Definition {
	d.FieldsValue = append([]Field{}, fields...)
	return d
}

// 追加字段列表
// @param fields 字段列表
// @returns 模型定义
func (d Definition) AppendFields(fields ...Field) Definition {
	d.FieldsValue = append(d.FieldsValue, fields...)
	return d
}

// 设置索引列表
// @param indexes 索引列表
// @returns 模型定义
func (d Definition) WithIndexes(indexes ...Index) Definition {
	d.Indexes = append([]Index{}, indexes...)
	return d
}

/**
 * 设置模型租户模式
 * @param mode 租户模式
 * @returns 模型定义
 */
func (d Definition) WithTenantMode(mode TenantMode) Definition {
	d.TenantMode = mode
	return d
}

// 按数据库字段名查找字段
// @param columnName 数据库字段名
// @returns 字段元数据和是否存在
func (d Definition) FieldByColumn(columnName string) (Field, bool) {
	for _, field := range d.FieldsValue {
		if field.ColumnName == columnName {
			return field, true
		}
	}
	return Field{}, false
}

// 按 JSON 字段名查找字段
// @param jsonName JSON 字段名
// @returns 字段元数据和是否存在
func (d Definition) FieldByJSONName(jsonName string) (Field, bool) {
	for _, field := range d.FieldsValue {
		if field.JSONName == jsonName {
			return field, true
		}
	}
	return Field{}, false
}

// 查找主键字段
// @returns 字段元数据和是否存在
func (d Definition) PrimaryField() (Field, bool) {
	for _, field := range d.FieldsValue {
		if field.IsPrimary {
			return field, true
		}
	}
	return Field{}, false
}
