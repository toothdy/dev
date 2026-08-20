package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

/**
 * 字典信息表
 * @returns entity.Definition
 */
func DictInfo() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("typeId", "typeId", "bigint").Unsigned().NotNull().Comment("类型ID"),
		entity.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		entity.NewField("value", "value", "varchar").Size(255).Nullable().Comment("值"),
		entity.NewField("orderNum", "orderNum", "int").NotNull().Default("0").Comment("排序"),
		entity.NewField("remark", "remark", "varchar").Size(255).Nullable().Comment("备注"),
		entity.NewField("parentId", "parentId", "bigint").Unsigned().Nullable().Comment("父ID"),
	)

	return entity.NewDefinition("dict", "DictInfo", "dict_info").
		WithResource("dict.info").
		Comment("字典信息").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_dict_info_type_id", "typeId"),
			entity.NewIndex("idx_dict_info_parent_id", "parentId"),
			entity.NewIndex("idx_dict_info_tenant_id", "tenantId"),
		)
}
