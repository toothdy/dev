package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

/**
 * 字典类别表
 * @returns entity.Definition
 */
func DictType() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		entity.NewField("key", "key", "varchar").Size(100).NotNull().Comment("标识"),
	)

	return entity.NewDefinition("dict", "DictType", "dict_type").
		WithResource("dict.type").
		Comment("字典类别").
		Fields(fields).
		WithIndexes(
			entity.NewUniqueIndex("uk_dict_type_key", "key"),
			entity.NewIndex("idx_dict_type_tenant_id", "tenantId"),
		)
}
