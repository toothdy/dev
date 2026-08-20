package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysDepartment() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("name", "name", "varchar").NotNull().Comment("部门名称"),
		entity.NewField("userId", "userId", "bigint").Unsigned().Nullable().Comment("创建者ID"),
		entity.NewField("parentId", "parentId", "bigint").Unsigned().Nullable().Comment("上级部门ID"),
		entity.NewField("orderNum", "orderNum", "int").NotNull().Default("0").Comment("排序"),
	)

	return entity.NewDefinition("base", "BaseSysDepartment", "base_sys_department").
		WithResource("base.department").
		Comment("系统部门").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_base_sys_department_parent_id", "parentId"),
			entity.NewIndex("idx_base_sys_department_tenant_id", "tenantId"),
		)
}
