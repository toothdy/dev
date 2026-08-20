package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysRoleDepartment() entity.Definition {
	return entity.NewDefinition("base", "BaseSysRoleDepartment", "base_sys_role_department").
		WithResource("base.roleDepartment").
		Comment("角色部门关联").
		Fields([]entity.Field{
			entity.NewField("roleId", "roleId", "bigint").Unsigned().NotNull().Comment("角色ID"),
			entity.NewField("departmentId", "departmentId", "bigint").Unsigned().NotNull().Comment("部门ID"),
		}).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_role_department", "roleId", "departmentId"),
			entity.NewIndex("idx_base_sys_role_department_department_id", "departmentId"),
		)
}
