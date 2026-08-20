package entity

import (
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// Department 是后台部门节点。
type Department struct {
	g.Meta `orm:"table:base_sys_department" description:"系统部门"`
	coreentity.Base
	Name     string  `json:"name" orm:"name" description:"部门名称" cool:"size=255"`
	UserID   *uint64 `json:"userId" orm:"userId" description:"创建者ID"`
	ParentID *uint64 `json:"parentId" orm:"parentId" description:"上级部门ID"`
	OrderNum int32   `json:"orderNum" orm:"orderNum" description:"排序" cool:"default=0"`
	SeedKey  *string `json:"seedKey" orm:"seedKey" description:"初始化键" cool:"size=255"`
}

// DepartmentSchema 返回部门表补充索引。
func DepartmentSchema() coreentity.Schema {
	return coreentity.Schema{Indexes: []coreentity.Index{
		coreentity.IndexOf("idx_base_sys_department_user_id", "userId"),
		coreentity.IndexOf("idx_base_sys_department_parent_id", "parentId"),
		coreentity.UniqueIndexOf("uk_base_sys_department_seed_key", "seedKey"),
	}}
}
