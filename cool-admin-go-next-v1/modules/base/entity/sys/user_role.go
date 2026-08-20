package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysUserRole() entity.Definition {
	return entity.NewDefinition("base", "BaseSysUserRole", "base_sys_user_role").
		WithResource("base.userRole").
		Comment("用户角色关联").
		Fields([]entity.Field{
			entity.NewField("userId", "userId", "bigint").Unsigned().NotNull().Comment("用户ID"),
			entity.NewField("roleId", "roleId", "bigint").Unsigned().NotNull().Comment("角色ID"),
		}).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_user_role", "userId", "roleId"),
			entity.NewIndex("idx_base_sys_user_role_role_id", "roleId"),
		)
}
