package admin

import (
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/dict/entity"
	"github.com/toothdy/cool-admin-go-next/modules/dict/service"
)

// 字典类型管理路由
func AdminDictTypeController(dictType *service.TypeService) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "字典类型", TagName: "字典类型"}).
		Curd(controller.CurdOption{
			API:     controller.AllAPI(),
			Entity:  entity.Type{},
			Service: dictType,
			ListQueryOp: controller.StaticQuery(controller.QueryOp{
				KeyWordLikeFields: []controller.ColumnRef{controller.Field("name")},
			}),
		}).
		Build()
}
