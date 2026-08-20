package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysRole() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("userId", "userId", "varchar").NotNull().Comment("用户ID"),
		entity.NewField("name", "name", "varchar").NotNull().Comment("名称"),
		entity.NewField("label", "label", "varchar").Size(50).Nullable().Comment("角色标签"),
		entity.NewField("remark", "remark", "varchar").Nullable().Comment("备注"),
		entity.NewField("relevance", "relevance", "boolean").NotNull().Default("false").Comment("数据权限是否关联上下级"),
		entity.NewField("menuIdList", "menuIdList", "json").NotNull().Comment("菜单权限"),
		entity.NewField("departmentIdList", "departmentIdList", "json").NotNull().Comment("部门权限"),
	)

	return entity.NewDefinition("base", "BaseSysRole", "base_sys_role").
		WithResource("base.role").
		Comment("系统角色").
		Fields(fields).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_role_name", "name"),
			entity.NewUniqueIndex("uk_base_sys_role_label", "label"),
			entity.NewIndex("idx_base_sys_role_tenant_id", "tenantId"),
		)
}
