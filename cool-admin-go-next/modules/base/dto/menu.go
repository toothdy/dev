package dto

import "github.com/gogf/gf/v2/os/gtime"

// MenuParseReq 是静态解析菜单代码的请求。
type MenuParseReq struct {
	Entity     string `json:"entity" v:"required"`
	Controller string `json:"controller"`
	Module     string `json:"module" v:"required"`
}

// MenuExportReq 是菜单导出的选中 ID 请求。
type MenuExportReq struct {
	IDs []uint64 `json:"ids" v:"required"`
}

// MenuListItem 是菜单列表的递归响应字段。
type MenuListItem struct {
	ID         uint64         `json:"id"`
	CreateTime *gtime.Time    `json:"createTime"`
	UpdateTime *gtime.Time    `json:"updateTime"`
	ParentID   *uint64        `json:"parentId"`
	Name       string         `json:"name"`
	Router     *string        `json:"router"`
	Perms      *string        `json:"perms"`
	Type       int32          `json:"type"`
	Icon       *string        `json:"icon"`
	OrderNum   int32          `json:"orderNum"`
	ViewPath   *string        `json:"viewPath"`
	KeepAlive  bool           `json:"keepAlive"`
	IsShow     bool           `json:"isShow"`
	ParentName *string        `json:"parentName"`
	ChildMenus []MenuListItem `json:"childMenus"`
}
