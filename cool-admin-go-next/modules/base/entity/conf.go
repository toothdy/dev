package entity

import (
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// Base 内部配置
type Conf struct {
	g.Meta `orm:"table:base_sys_conf" description:"系统配置"`
	coreentity.Base
	CKey   string `json:"cKey" orm:"cKey" description:"配置键" cool:"size=255"`
	CValue string `json:"cValue" orm:"cValue" description:"配置值" cool:"size=255"`
}

// 系统配置表补充索引
func ConfSchema() coreentity.Schema {
	return coreentity.Schema{Indexes: []coreentity.Index{
		coreentity.UniqueIndexOf("uk_base_sys_conf_c_key", "cKey"),
	}}
}
