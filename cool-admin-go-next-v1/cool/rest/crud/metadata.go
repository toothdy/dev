package crud

import (
	"context"
	"fmt"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// 运行时查询字段元数据
type QueryMetadata struct {
	KeywordFields map[string]entity.Field
	EqualFields   map[string]entity.Field
	LikeFields    map[string]entity.Field
}

// 运行时资源定义
type Resource struct {
	Spec             ResourceSpec
	Service          interface{}
	InsertParam      func(ctx context.Context) map[string]interface{}
	FieldsByJSON     map[string]entity.Field
	FieldsByColumn   map[string]entity.Field
	API              map[string]bool
	ListQuery        QueryMetadata
	PageQuery        QueryMetadata
	KeywordFields    map[string]entity.Field
	EqualFields      map[string]entity.Field
	LikeFields       map[string]entity.Field
	SortFields       map[string]entity.Field
	HiddenFields     map[string]bool
	ReadonlyFields   map[string]bool
	InfoIgnoreFields map[string]bool
	PrimaryField     entity.Field
	Tenant           tenant.Metadata
}

// CRUD 资源注册表
type Registry struct {
	resources map[string]Resource
}

/**
 * 创建 CRUD 注册表
 * @param specs 资源配置
 * @returns *Registry
 */
func NewRegistry(specs []ResourceSpec) (*Registry, error) {
	registry := &Registry{
		resources: map[string]Resource{},
	}
	for _, spec := range specs {
		resource, err := buildResource(spec)
		if err != nil {
			return nil, err
		}
		if _, ok := registry.resources[spec.Name]; ok {
			return nil, exception.Core(fmt.Sprintf("CRUD 资源重复: %s", spec.Name))
		}
		registry.resources[spec.Name] = resource
	}
	return registry, nil
}

/**
 * 获取 CRUD 资源
 * @param name 资源名称
 * @returns Resource
 */
func (r *Registry) Resource(name string) (Resource, bool) {
	resource, ok := r.resources[name]
	return resource, ok
}

/**
 * 获取资源列表
 * @returns []Resource
 */
func (r *Registry) Resources() []Resource {
	resources := make([]Resource, 0, len(r.resources))
	for _, resource := range r.resources {
		resources = append(resources, resource)
	}
	return resources
}

/**
 * 构建运行时资源
 * @param spec 资源配置
 * @returns Resource
 */
func buildResource(spec ResourceSpec) (Resource, error) {
	if spec.Name == "" || spec.Prefix == "" || spec.Model.TableName == "" {
		return Resource{}, exception.Core("CRUD 资源配置不完整")
	}
	primaryField, ok := spec.Model.PrimaryField()
	if !ok {
		return Resource{}, exception.Core(fmt.Sprintf("CRUD 资源缺少主键: %s", spec.Name))
	}
	tenantMetadata, err := tenant.CompileMetadata(spec.Model)
	if err != nil {
		return Resource{}, exception.Core(err.Error())
	}

	resource := Resource{
		Spec:             spec,
		Service:          spec.Service,
		InsertParam:      spec.InsertParam,
		FieldsByJSON:     map[string]entity.Field{},
		FieldsByColumn:   map[string]entity.Field{},
		API:             map[string]bool{},
		ListQuery:        newQueryMetadata(),
		PageQuery:        newQueryMetadata(),
		KeywordFields:    map[string]entity.Field{},
		EqualFields:      map[string]entity.Field{},
		LikeFields:       map[string]entity.Field{},
		SortFields:       map[string]entity.Field{},
		HiddenFields:     map[string]bool{},
		ReadonlyFields:   map[string]bool{},
		InfoIgnoreFields: map[string]bool{},
		PrimaryField:     primaryField,
		Tenant:           tenantMetadata,
	}
	for _, field := range spec.Model.FieldsValue {
		resource.FieldsByJSON[field.JSONName] = field
		resource.FieldsByColumn[field.ColumnName] = field
	}
	for _, api := range spec.API {
		resource.API[api] = true
	}
	for _, fieldName := range spec.HiddenFields {
		resource.HiddenFields[fieldName] = true
	}
	for _, fieldName := range spec.ReadonlyFields {
		resource.ReadonlyFields[fieldName] = true
	}
	for _, fieldName := range spec.InfoIgnoreFields {
		resource.InfoIgnoreFields[fieldName] = true
	}
	for _, fieldName := range defaultReadonlyFields(primaryField.JSONName) {
		resource.ReadonlyFields[fieldName] = true
	}
	legacyQuery := QuerySpec{
		KeywordFields: spec.KeywordFields,
		EqualFields:   spec.EqualFields,
		LikeFields:    spec.LikeFields,
	}
	hasNewQuerySemantics := hasQuerySpecFields(spec.ListQuery) || hasQuerySpecFields(spec.PageQuery)
	listQuery := spec.ListQuery
	pageQuery := spec.PageQuery
	if !hasNewQuerySemantics {
		listQuery = querySpecWithFallback(spec.ListQuery, legacyQuery)
		pageQuery = querySpecWithFallback(spec.PageQuery, legacyQuery)
	}
	if err := fillQueryMetadata(spec.Name, listQuery, resource.FieldsByJSON, &resource.ListQuery); err != nil {
		return Resource{}, err
	}
	if err := fillQueryMetadata(spec.Name, pageQuery, resource.FieldsByJSON, &resource.PageQuery); err != nil {
		return Resource{}, err
	}
	// 兼容现有调用，Task 4 将 handler 按 list/page 选择对应元数据。
	resource.KeywordFields = resource.PageQuery.KeywordFields
	resource.EqualFields = resource.PageQuery.EqualFields
	resource.LikeFields = resource.PageQuery.LikeFields
	if len(spec.SortFields) == 0 {
		resource.SortFields[primaryField.JSONName] = primaryField
	} else if err := fillFieldSet(spec.Name, spec.SortFields, resource.FieldsByJSON, resource.SortFields); err != nil {
		return Resource{}, err
	}
	return resource, nil
}

/**
 * 创建空查询元数据
 * @returns QueryMetadata
 */
func newQueryMetadata() QueryMetadata {
	return QueryMetadata{
		KeywordFields: map[string]entity.Field{},
		EqualFields:   map[string]entity.Field{},
		LikeFields:    map[string]entity.Field{},
	}
}

/**
 * 使用兼容配置补全查询配置
 * @param query 查询配置
 * @param fallback 兼容查询配置
 * @returns QuerySpec
 */
func querySpecWithFallback(query QuerySpec, fallback QuerySpec) QuerySpec {
	if len(query.KeywordFields) == 0 && len(query.EqualFields) == 0 && len(query.LikeFields) == 0 {
		return fallback
	}
	return query
}

/**
 * 判断查询配置是否包含字段
 * @param query 查询配置
 * @returns bool
 */
func hasQuerySpecFields(query QuerySpec) bool {
	return len(query.KeywordFields) > 0 || len(query.EqualFields) > 0 || len(query.LikeFields) > 0
}

/**
 * 填充运行时查询元数据
 * @param resourceName 资源名称
 * @param query 查询配置
 * @param fields 资源字段
 * @param target 运行时查询元数据
 * @returns error
 */
func fillQueryMetadata(resourceName string, query QuerySpec, fields map[string]entity.Field, target *QueryMetadata) error {
	if err := fillFieldSet(resourceName, query.KeywordFields, fields, target.KeywordFields); err != nil {
		return err
	}
	if err := fillFieldSet(resourceName, query.EqualFields, fields, target.EqualFields); err != nil {
		return err
	}
	return fillFieldSet(resourceName, query.LikeFields, fields, target.LikeFields)
}

/**
 * 填充字段集合
 * @param resourceName 资源名称
 * @param fieldNames 字段名列表
 * @param source 源字段集合
 * @param target 目标字段集合
 * @returns error
 */
func fillFieldSet(resourceName string, fieldNames []string, source map[string]entity.Field, target map[string]entity.Field) error {
	for _, fieldName := range fieldNames {
		lookupName := fieldName
		qualified := false
		if index := strings.LastIndex(lookupName, "."); index >= 0 {
			qualified = true
			lookupName = lookupName[index+1:]
		}
		field, ok := source[lookupName]
		if !ok {
			// Qualified fields can belong to a joined entity handled by a custom service.
			if qualified {
				target[fieldName] = entity.Field{JSONName: lookupName, ColumnName: fieldName}
				continue
			}
			return exception.Core(fmt.Sprintf("CRUD 资源 %s 字段不存在: %s", resourceName, fieldName))
		}
		target[fieldName] = field
	}
	return nil
}

/**
 * 默认只读字段
 * @param primaryJSONName 主键 JSON 字段名
 * @returns []string
 */
func defaultReadonlyFields(primaryJSONName string) []string {
	return []string{primaryJSONName, "createTime", "updateTime", "tenantId"}
}
