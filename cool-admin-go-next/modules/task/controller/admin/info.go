package admin

import (
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
	"github.com/toothdy/cool-admin-go-next/modules/task/service"
)

// 任务管理路由
func AdminTaskInfoController(info *service.InfoService, normalizer *BodyNormalizer) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "任务", TagName: "任务"}).
		Curd(controller.CurdOption{
			API:     controller.API(controller.Add, controller.Delete, controller.Update, controller.Info, controller.Page),
			Entity:  entity.Info{},
			Service: info,
			Before:  normalizer.Trim,
			PageQueryOp: controller.StaticQuery(controller.QueryOp{
				FieldEq: []controller.FieldEq{
					controller.Eq(controller.Field("status")),
					controller.Eq(controller.Field("type")),
				},
			}),
		}).
		Route(
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/once",
				Summary: "执行一次",
				Handler: controller.Handle(info.Once),
				Bind:    controller.BindJSON,
				// 执行器自行管理抢占与结算事务，外层再包一层会让行锁横跨整个任务调用
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/stop",
				Summary: "停止",
				Handler: controller.Handle(info.Stop),
				Bind:    controller.BindJSON,
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/start",
				Summary: "开始",
				Handler: controller.Handle(info.Start),
				Bind:    controller.BindJSON,
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/log",
				Summary:     "日志",
				Handler:     controller.Handle(info.Log),
				Bind:        controller.BindQuery,
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
