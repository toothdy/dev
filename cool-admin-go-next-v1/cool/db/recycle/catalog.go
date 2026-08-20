package recycle

import (
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// ModelMetadata 表示经启动期校验的可恢复模型
type ModelMetadata struct {
	Resource       string
	Definition     entity.Definition
	FieldsByJSON   map[string]entity.Field
	FieldsByColumn map[string]entity.Field
	IdentityFields []entity.Field
	Tenant         tenant.Metadata
}

// Catalog 表示启动期冻结的可恢复模型目录
type Catalog struct {
	byResource map[string]ModelMetadata
	byTable    map[string]ModelMetadata
}

/**
 * 创建冻结模型目录
 * @param definitions 模型定义
 * @returns *Catalog 和校验错误
 */
func NewCatalog(definitions []entity.Definition) (*Catalog, error) {
	catalog := &Catalog{
		byResource: make(map[string]ModelMetadata, len(definitions)),
		byTable:    make(map[string]ModelMetadata, len(definitions)),
	}
	for _, definition := range definitions {
		definition = cloneDefinition(definition)
		metadata, err := compileModelMetadata(definition)
		if err != nil {
			return nil, err
		}
		if _, ok := catalog.byResource[metadata.Resource]; ok {
			return nil, gerror.Newf("回收站模型资源重复: %s", metadata.Resource)
		}
		if _, ok := catalog.byTable[definition.TableName]; ok {
			return nil, gerror.Newf("回收站模型数据表重复: %s", definition.TableName)
		}
		catalog.byResource[metadata.Resource] = metadata
		catalog.byTable[definition.TableName] = metadata
	}
	return catalog, nil
}

/**
 * 按稳定资源名读取模型
 * @param resource 稳定资源名
 * @returns ModelMetadata 和是否存在
 */
func (c *Catalog) Model(resource string) (ModelMetadata, bool) {
	if c == nil {
		return ModelMetadata{}, false
	}
	metadata, ok := c.byResource[resource]
	return cloneModelMetadata(metadata), ok
}

/**
 * 按数据表读取模型
 * @param tableName 数据表名
 * @returns ModelMetadata 和是否存在
 */
func (c *Catalog) ModelByTable(tableName string) (ModelMetadata, bool) {
	if c == nil {
		return ModelMetadata{}, false
	}
	metadata, ok := c.byTable[tableName]
	return cloneModelMetadata(metadata), ok
}

/**
 * 返回冻结模型列表
 * @returns []ModelMetadata
 */
func (c *Catalog) Models() []ModelMetadata {
	if c == nil {
		return nil
	}
	models := make([]ModelMetadata, 0, len(c.byResource))
	for _, metadata := range c.byResource {
		models = append(models, cloneModelMetadata(metadata))
	}
	return models
}

func compileModelMetadata(definition entity.Definition) (ModelMetadata, error) {
	resource := definition.ResourceKey()
	if resource == "" || !validResource(resource) {
		return ModelMetadata{}, gerror.Newf("回收站模型资源名无效: %s", resource)
	}
	if !validIdentifier(definition.TableName) {
		return ModelMetadata{}, gerror.Newf("回收站数据表名无效: %s", definition.TableName)
	}
	metadata := ModelMetadata{
		Resource:       resource,
		Definition:     definition,
		FieldsByJSON:   make(map[string]entity.Field, len(definition.FieldsValue)),
		FieldsByColumn: make(map[string]entity.Field, len(definition.FieldsValue)),
	}
	for _, field := range definition.FieldsValue {
		if !validIdentifier(field.JSONName) || !validIdentifier(field.ColumnName) {
			return ModelMetadata{}, gerror.Newf("回收站模型 %s 字段名无效", resource)
		}
		if _, ok := metadata.FieldsByJSON[field.JSONName]; ok {
			return ModelMetadata{}, gerror.Newf("回收站模型 %s JSON 字段重复: %s", resource, field.JSONName)
		}
		if _, ok := metadata.FieldsByColumn[field.ColumnName]; ok {
			return ModelMetadata{}, gerror.Newf("回收站模型 %s 数据库字段重复: %s", resource, field.ColumnName)
		}
		metadata.FieldsByJSON[field.JSONName] = field
		metadata.FieldsByColumn[field.ColumnName] = field
		if field.IsPrimary {
			metadata.IdentityFields = append(metadata.IdentityFields, field)
		}
	}
	if len(metadata.IdentityFields) == 0 {
		uniqueIndexes := make([]entity.Index, 0, 1)
		for _, index := range definition.Indexes {
			if index.IsUnique {
				uniqueIndexes = append(uniqueIndexes, index)
			}
		}
		if len(uniqueIndexes) == 1 {
			for _, columnName := range uniqueIndexes[0].Columns {
				field, ok := metadata.FieldsByColumn[columnName]
				if !ok {
					return ModelMetadata{}, gerror.Newf("回收站模型 %s 唯一索引字段不存在: %s", resource, columnName)
				}
				metadata.IdentityFields = append(metadata.IdentityFields, field)
			}
		}
	}
	tenantMetadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		return ModelMetadata{}, gerror.Wrapf(err, "编译回收站模型 %s 租户元数据失败", resource)
	}
	metadata.Tenant = tenantMetadata
	return metadata, nil
}

func cloneModelMetadata(metadata ModelMetadata) ModelMetadata {
	metadata.Definition = cloneDefinition(metadata.Definition)
	metadata.FieldsByJSON = cloneFields(metadata.FieldsByJSON)
	metadata.FieldsByColumn = cloneFields(metadata.FieldsByColumn)
	metadata.IdentityFields = append([]entity.Field{}, metadata.IdentityFields...)
	return metadata
}

func cloneDefinition(definition entity.Definition) entity.Definition {
	cloned := definition
	cloned.FieldsValue = append([]entity.Field{}, definition.FieldsValue...)
	for index := range cloned.FieldsValue {
		cloned.FieldsValue[index].Dict = append([]string{}, definition.FieldsValue[index].Dict...)
	}
	cloned.Indexes = append([]entity.Index{}, definition.Indexes...)
	for index := range cloned.Indexes {
		cloned.Indexes[index].Columns = append([]string{}, definition.Indexes[index].Columns...)
	}
	return cloned
}

func cloneFields(fields map[string]entity.Field) map[string]entity.Field {
	cloned := make(map[string]entity.Field, len(fields))
	for name, field := range fields {
		cloned[name] = field
	}
	return cloned
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, item := range []byte(value) {
		isLetter := item >= 'a' && item <= 'z' || item >= 'A' && item <= 'Z'
		isDigit := item >= '0' && item <= '9'
		if !(isLetter || item == '_' || index > 0 && isDigit) {
			return false
		}
	}
	return true
}

func validResource(value string) bool {
	for _, part := range strings.Split(value, ".") {
		if !validIdentifier(strings.ReplaceAll(part, "-", "_")) {
			return false
		}
	}
	return value != ""
}
