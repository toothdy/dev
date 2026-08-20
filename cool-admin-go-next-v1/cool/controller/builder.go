package controller

import (
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

// controller 元数据构建器
type Builder struct {
	definition Definition
}

/**
 * 创建后台管理 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Admin(path string) *Builder {
	return newBuilder(AreaAdmin, path)
}

/**
 * 创建开放 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Open(path string) *Builder {
	return newBuilder(AreaOpen, path)
}

/**
 * 创建通用 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Comm(path string) *Builder {
	return newBuilder(AreaComm, path)
}

// 创建应用端 controller 构建器
func App(path string) *Builder {
	return newBuilder(AreaApp, path)
}

/**
 * 创建 controller 构建器
 * @param area 分区
 * @param path 模块路径
 * @returns *Builder
 */
func newBuilder(area Area, path string) *Builder {
	normalized := strings.Trim(path, "/")
	parts := strings.Split(normalized, "/")
	moduleName := ""
	if len(parts) > 0 {
		moduleName = parts[0]
	}

	root := "/admin/"
	if area == AreaApp {
		root = "/app/"
	}

	return &Builder{
		definition: Definition{
			Module: moduleName,
			Area:   area,
			Prefix: root + normalized,
		},
	}
}

/**
 * 设置 controller 名称
 * @param name 名称
 * @returns *Builder
 */
func (b *Builder) Name(name string) *Builder {
	b.definition.Name = name
	return b
}

/**
 * 设置描述
 * @param description 描述
 * @returns *Builder
 */
func (b *Builder) Description(description string) *Builder {
	b.definition.Description = description
	return b
}

/**
 * 设置模型定义
 * @param definition 模型定义
 * @returns *Builder
 */
func (b *Builder) Model(definition entity.Definition) *Builder {
	b.definition.Model = cloneModelDefinition(definition)
	return b
}

/**
 * 设置业务服务
 * @param service 业务服务
 * @returns *Builder
 */
func (b *Builder) Service(service interface{}) *Builder {
	b.definition.Service = service
	return b
}

/**
 * 设置 CRUD 元数据
 * @param options CRUD 配置
 * @returns *Builder
 */
func (b *Builder) CRUD(options CRUDOptions) *Builder {
	b.definition.CRUD = &CRUDDefinition{
		API:             cloneStrings(options.API),
		PageQuery:        cloneQueryOptions(options.PageQuery),
		ListQuery:        cloneQueryOptions(options.ListQuery),
		PageSelect:       clonePageSelectOptions(options.PageSelect),
		InsertParam:      options.InsertParam,
		InfoIgnoreFields: cloneStrings(options.InfoIgnoreFields),
		SortFields:       cloneStrings(options.SortFields),
		HiddenFields:     cloneStrings(options.HiddenFields),
		ReadonlyFields:   cloneStrings(options.ReadonlyFields),
		DefaultSort:      options.DefaultSort,
		DefaultOrder:     options.DefaultOrder,
	}
	return b
}

/**
 * 添加自定义路由
 * @param options 路由配置
 * @returns *Builder
 */
func (b *Builder) Route(options RouteOptions) *Builder {
	path := "/" + strings.Trim(options.Path, "/")
	b.definition.Routes = append(b.definition.Routes, RouteDefinition{
		Name:               options.Name,
		Method:             strings.ToUpper(options.Method),
		Path:               path,
		FullPath:           b.definition.Prefix + path,
		Description:        options.Description,
		IgnoreAuth:         options.IgnoreAuth,
		Permission:         options.Permission,
		Action:             options.Action,
		Bind:               options.Bind,
		AllowUnknownFields: options.AllowUnknownFields,
	})
	return b
}

/**
 * 构建 controller 元数据
 * @returns Definition
 */
func (b *Builder) Build() Definition {
	return CloneDefinition(b.definition)
}

/**
 * 深复制 controller 元数据
 * @param definition controller 元数据
 * @returns Definition
 */
func CloneDefinition(definition Definition) Definition {
	cloned := definition
	cloned.Model = cloneModelDefinition(definition.Model)
	cloned.CRUD = cloneCRUDDefinition(definition.CRUD)
	cloned.Routes = append([]RouteDefinition{}, definition.Routes...)
	return cloned
}

/**
 * 复制字符串切片
 * @param items 字符串切片
 * @returns []string
 */
func cloneStrings(items []string) []string {
	return append([]string{}, items...)
}

/**
 * 复制查询配置
 * @param options 查询配置
 * @returns QueryOptions
 */
func cloneQueryOptions(options QueryOptions) QueryOptions {
	return QueryOptions{
		KeyWordLikeFields: cloneStrings(options.KeyWordLikeFields),
		FieldEq:           cloneStrings(options.FieldEq),
		FieldLike:         cloneStrings(options.FieldLike),
	}
}

/**
 * 复制 CRUD 元数据
 * @param definition CRUD 元数据
 * @returns *CRUDDefinition
 */
func cloneCRUDDefinition(definition *CRUDDefinition) *CRUDDefinition {
	if definition == nil {
		return nil
	}

	return &CRUDDefinition{
		API:             cloneStrings(definition.API),
		PageQuery:        cloneQueryOptions(definition.PageQuery),
		ListQuery:        cloneQueryOptions(definition.ListQuery),
		PageSelect:       clonePageSelectOptions(definition.PageSelect),
		InsertParam:      definition.InsertParam,
		InfoIgnoreFields: cloneStrings(definition.InfoIgnoreFields),
		SortFields:       cloneStrings(definition.SortFields),
		HiddenFields:     cloneStrings(definition.HiddenFields),
		ReadonlyFields:   cloneStrings(definition.ReadonlyFields),
		DefaultSort:      definition.DefaultSort,
		DefaultOrder:     definition.DefaultOrder,
	}
}

func clonePageSelectOptions(options []PageSelectOptions) []PageSelectOptions {
	cloned := make([]PageSelectOptions, len(options))
	for index, option := range options {
		cloned[index] = option
		cloned[index].Model = cloneModelDefinition(option.Model)
		cloned[index].Fields = cloneStrings(option.Fields)
	}
	return cloned
}

/**
 * 复制模型定义
 * @param definition 模型定义
 * @returns entity.Definition
 */
func cloneModelDefinition(definition entity.Definition) entity.Definition {
	cloned := definition
	cloned.FieldsValue = cloneModelFields(definition.FieldsValue)
	cloned.Indexes = cloneModelIndexes(definition.Indexes)
	return cloned
}

/**
 * 复制模型字段切片
 * @param fields 字段切片
 * @returns []entity.Field
 */
func cloneModelFields(fields []entity.Field) []entity.Field {
	cloned := make([]entity.Field, len(fields))
	for index, field := range fields {
		cloned[index] = field
		cloned[index].Dict = cloneStrings(field.Dict)
	}
	return cloned
}

/**
 * 复制模型索引切片
 * @param indexes 索引切片
 * @returns []entity.Index
 */
func cloneModelIndexes(indexes []entity.Index) []entity.Index {
	cloned := make([]entity.Index, len(indexes))
	for index, item := range indexes {
		cloned[index] = item
		cloned[index].Columns = cloneStrings(item.Columns)
	}
	return cloned
}
