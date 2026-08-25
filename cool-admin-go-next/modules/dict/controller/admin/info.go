package admin

import (
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/dict/entity"
	"github.com/toothdy/cool-admin-go-next/modules/dict/service"
)

// 字典信息管理路由
func AdminDictInfoController(info *service.InfoService) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "字典信息", TagName: "字典信息"}).
		Curd(controller.CurdOption{
			API:     controller.AllAPI(),
			Entity:  entity.Info{},
			Service: info,
			ListQueryOp: controller.StaticQuery(controller.QueryOp{
				KeyWordLikeFields: []controller.ColumnRef{controller.Field("name")},
				FieldEq:           []controller.FieldEq{controller.Eq(controller.Field("typeId"))},
				AddOrderBy:        []controller.Order{controller.Asc(controller.Field("createTime"))},
			}),
		}).
		Route(
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/data",
				Summary:     "获得字典数据",
				Handler:     controller.Handle(info.Data),
				Bind:        controller.BindJSON,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/types",
				Summary:     "获得所有字典类型",
				Handler:     controller.Handle(info.Types),
				Tags:        []controller.URLTag{{Name: controller.TagIgnoreToken}},
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
