package tenant

import (
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

const (
	tenantJSONField = "tenantId"
	tenantColumn    = "tenantId"
)

// Metadata 保存启动期编译的租户字段元数据
type Metadata struct {
	isAware  bool
	jsonName string
	column   string
}

/**
 * 编译模型租户元数据
 * @param definition 模型定义
 * @returns 租户元数据和配置错误
 */
func CompileMetadata(definition entity.Definition) (Metadata, error) {
	if definition.TenantMode > entity.TenantModeDisabled {
		return Metadata{}, gerror.Newf("模型租户模式无效: %s", definition.Name)
	}
	if definition.TenantMode == entity.TenantModeDisabled {
		return Metadata{}, nil
	}
	jsonField, hasJSONField := definition.FieldByJSONName(tenantJSONField)
	columnField, hasColumnField := definition.FieldByColumn(tenantColumn)
	if !hasJSONField && !hasColumnField {
		if definition.TenantMode == entity.TenantModeRequired {
			return Metadata{}, gerror.Newf("模型缺少租户字段: %s", definition.Name)
		}
		return Metadata{}, nil
	}
	if !hasJSONField || !hasColumnField || jsonField.JSONName != columnField.JSONName || jsonField.ColumnName != columnField.ColumnName {
		return Metadata{}, gerror.Newf("模型租户字段不规范: %s", definition.Name)
	}
	if !strings.EqualFold(jsonField.DataType, "bigint") || !jsonField.IsUnsigned || !jsonField.IsNullable {
		return Metadata{}, gerror.Newf("模型租户字段必须是 nullable unsigned bigint: %s", definition.Name)
	}
	return Metadata{
		isAware:  true,
		jsonName: jsonField.JSONName,
		column:   jsonField.ColumnName,
	}, nil
}

/**
 * 判断模型是否启用租户隔离
 * @returns 是否启用
 */
func (m Metadata) IsAware() bool {
	return m.isAware
}

/**
 * 获取租户 JSON 字段名
 * @returns JSON 字段名
 */
func (m Metadata) JSONField() string {
	return m.jsonName
}

/**
 * 获取租户数据库列名
 * @returns 数据库列名
 */
func (m Metadata) Column() string {
	return m.column
}
