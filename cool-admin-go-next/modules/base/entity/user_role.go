package entity

import (
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// UserRole 是用户与角色关系。
type UserRole struct {
	g.Meta `orm:"table:base_sys_user_role" description:"用户角色关系"`
	coreentity.Base
	UserID uint64 `json:"userId" orm:"userId" description:"用户ID"`
	RoleID uint64 `json:"roleId" orm:"roleId" description:"角色ID"`
}

// UserRoleSchema 返回用户角色关系表补充索引。
func UserRoleSchema() coreentity.Schema {
	return coreentity.Schema{Indexes: []coreentity.Index{
		coreentity.UniqueIndexOf("uk_base_sys_user_role_user_role", "userId", "roleId"),
		coreentity.IndexOf("idx_base_sys_user_role_role_id", "roleId"),
	}}
}
