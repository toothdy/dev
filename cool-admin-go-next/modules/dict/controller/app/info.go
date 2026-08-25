package app

import (
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/dict/service"
)

// App 字典信息路由
func AppDictInfoController(info *service.InfoService) controller.Definition {
	public := []controller.URLTag{{Name: controller.TagIgnoreToken}}

	return controller.App().
		Options(controller.RouterOptions{Description: "字典信息", TagName: "字典信息"}).
		Route(
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/data",
				Summary:     "获得字典数据",
				Handler:     controller.Handle(info.Data),
				Bind:        controller.BindJSON,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/types",
				Summary:     "获得所有字典类型",
				Handler:     controller.Handle(info.Types),
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
