package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysMenu() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("parentId", "parentId", "bigint").Unsigned().Nullable().Comment("父菜单ID"),
		entity.NewField("name", "name", "varchar").NotNull().Comment("菜单名称"),
		entity.NewField("router", "router", "varchar").Nullable().Comment("菜单地址"),
		entity.NewField("perms", "perms", "text").Nullable().Comment("权限标识"),
		entity.NewField("type", "type", "tinyint").NotNull().Default("0").Comment("类型 0-目录 1-菜单 2-按钮"),
		entity.NewField("icon", "icon", "varchar").Nullable().Comment("图标"),
		entity.NewField("orderNum", "orderNum", "int").NotNull().Default("0").Comment("排序"),
		entity.NewField("viewPath", "viewPath", "varchar").Nullable().Comment("视图地址"),
		entity.NewField("keepAlive", "keepAlive", "boolean").NotNull().Default("true").Comment("路由缓存"),
		entity.NewField("isShow", "isShow", "boolean").NotNull().Default("true").Comment("是否显示"),
	)

	return entity.NewDefinition("base", "BaseSysMenu", "base_sys_menu").
		WithResource("base.menu").
		Comment("系统菜单").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_base_sys_menu_parent_id", "parentId"),
			entity.NewIndex("idx_base_sys_menu_type", "type"),
			entity.NewIndex("idx_base_sys_menu_tenant_id", "tenantId"),
		)
}
