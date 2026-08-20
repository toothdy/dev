package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysRoleMenu() entity.Definition {
	return entity.NewDefinition("base", "BaseSysRoleMenu", "base_sys_role_menu").
		WithResource("base.roleMenu").
		Comment("角色菜单关联").
		Fields([]entity.Field{
			entity.NewField("roleId", "roleId", "bigint").Unsigned().NotNull().Comment("角色ID"),
			entity.NewField("menuId", "menuId", "bigint").Unsigned().NotNull().Comment("菜单ID"),
		}).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_role_menu", "roleId", "menuId"),
			entity.NewIndex("idx_base_sys_role_menu_menu_id", "menuId"),
		)
}
