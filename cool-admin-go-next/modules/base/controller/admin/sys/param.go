package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 按参数键读取富文本的查询请求
type ParamHTMLQuery struct {
	Key string `json:"key" in:"query" v:"required"`
}

// 适配后台参数富文本接口
type ParamHTMLHandler struct {
	param *service.ParamService
}

// 参数富文本接口适配器
func NewParamHTMLHandler(param *service.ParamService) *ParamHTMLHandler {
	return &ParamHTMLHandler{param: param}
}

// 按参数键返回原始 HTML
func (handler *ParamHTMLHandler) HTML(ctx context.Context, request *ParamHTMLQuery) (controller.HTMLResponse, error) {
	return handler.param.HTMLByKey(ctx, request.Key)
}

// 系统参数管理路由
func AdminSysParamController(param *service.ParamService, handler *ParamHTMLHandler) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "参数配置", TagName: "参数配置"}).
		Curd(controller.CurdOption{
			API:     controller.API(controller.Add, controller.Delete, controller.Update, controller.Info, controller.Page),
			Entity:  entity.Param{},
			Service: param,
		}).
		Route(controller.Route{
			Method:      http.MethodGet,
			Path:        "/html",
			Summary:     "获得网页内容的参数值",
			Handler:     controller.Handle(handler.HTML),
			Bind:        controller.BindQuery,
			Transaction: controller.NonTransactional(),
		}).
		Build()
}
