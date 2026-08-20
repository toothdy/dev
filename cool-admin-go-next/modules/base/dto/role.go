package dto

import "github.com/gogf/gf/v2/os/gtime"

// RolePermissionInput 是角色更新菜单和部门关系的输入。
type RolePermissionInput struct {
	MenuIDList       []uint64 `json:"menuIdList"`
	DepartmentIDList []uint64 `json:"departmentIdList"`
}

// RoleInfoResult 是角色详情及权限表单的稳定响应字段。
type RoleInfoResult struct {
	ID               uint64      `json:"id"`
	CreateTime       *gtime.Time `json:"createTime"`
	UpdateTime       *gtime.Time `json:"updateTime"`
	UserID           string      `json:"userId"`
	Name             string      `json:"name"`
	Label            *string     `json:"label"`
	Remark           *string     `json:"remark"`
	Relevance        bool        `json:"relevance"`
	MenuIDList       []uint64    `json:"menuIdList"`
	DepartmentIDList []uint64    `json:"departmentIdList"`
}
