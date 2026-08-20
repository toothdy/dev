package entity

import (
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// RoleMenu 是角色与菜单关系。
type RoleMenu struct {
	g.Meta `orm:"table:base_sys_role_menu" description:"角色菜单关系"`
	coreentity.Base
	RoleID uint64 `json:"roleId" orm:"roleId" description:"角色ID"`
	MenuID uint64 `json:"menuId" orm:"menuId" description:"菜单ID"`
}

// RoleMenuSchema 返回角色菜单关系表补充索引。
func RoleMenuSchema() coreentity.Schema {
	return coreentity.Schema{Indexes: []coreentity.Index{
		coreentity.UniqueIndexOf("uk_base_sys_role_menu_role_menu", "roleId", "menuId"),
		coreentity.IndexOf("idx_base_sys_role_menu_menu_id", "menuId"),
	}}
}
