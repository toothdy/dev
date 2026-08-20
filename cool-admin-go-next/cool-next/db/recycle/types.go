package recycle

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

// 回收记录表名
const TableName = "cool_recycle"

// 回收记录
type Record struct {
	g.Meta `orm:"table:cool_recycle" description:"删除回收记录"`
	entity.Base
	DatabaseGroup string  `json:"databaseGroup" orm:"databaseGroup" description:"数据库组" cool:"size=128"`
	TableName     string  `json:"tableName" orm:"tableName" description:"业务表名" cool:"size=128"`
	Data          []byte  `json:"data" orm:"data" description:"强类型快照"`
	Count         uint64  `json:"count" orm:"count" description:"记录数量"`
	Source        *string `json:"source" orm:"source" description:"脱敏来源" cool:"size=2048"`
	Params        *string `json:"params" orm:"params" description:"脱敏参数" cool:"size=4096"`
	OperatorType  *string `json:"operatorType" orm:"operatorType" description:"操作者类型" cool:"size=32"`
	OperatorID    *string `json:"operatorId" orm:"operatorId" description:"操作者 ID" cool:"size=128"`
}

// 删除归档入口
type Deleter interface {
	Delete(ctx context.Context, descriptor entity.RuntimeDescriptor, ids []any) error
}
