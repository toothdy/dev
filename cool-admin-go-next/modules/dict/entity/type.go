package entity

import (
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// 字典类别
type Type struct {
	g.Meta `orm:"table:dict_type" description:"字典类别"`
	coreentity.Base
	Name string `json:"name" orm:"name" description:"名称" cool:"size=255"`
	Key  string `json:"key" orm:"key" description:"标识" cool:"size=255"`
}

// 字典类别表约束
func TypeSchema() coreentity.Schema {
	return coreentity.Schema{}
}
