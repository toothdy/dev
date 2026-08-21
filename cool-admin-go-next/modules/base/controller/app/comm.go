package app

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/eps"
	"github.com/toothdy/cool-admin-go-next/modules/base/controller/admin"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// ParamQuery 是 App 参数读取请求。
type ParamQuery struct {
	Key string `json:"key" in:"query" v:"required"`
}

// CommHandler 适配 App 参数和上传接口。
type CommHandler struct {
	param  *service.ParamService
	upload *admin.UploadHandler
}

// NewCommHandler 创建 App 通用接口适配器。
func NewCommHandler(param *service.ParamService, upload *admin.UploadHandler) (*CommHandler, error) {
	if param == nil || upload == nil {
		return nil, exception.Core("Base App 通用接口依赖无效")
	}

	return &CommHandler{param: param, upload: upload}, nil
}

// Param 返回配置允许公开的参数值。
func (handler *CommHandler) Param(ctx context.Context, request *ParamQuery) (any, error) {
	if handler == nil || handler.param == nil || request == nil {
		return nil, exception.Core("Base App 参数接口未初始化")
	}

	return handler.param.AppDataByKey(ctx, request.Key)
}

// Upload 保存 App 身份上传的文件。
func (handler *CommHandler) Upload(ctx context.Context, request *admin.UploadRequest) (string, error) {
	if handler == nil || handler.upload == nil {
		return "", exception.Core("Base App 上传接口未初始化")
	}

	return handler.upload.AppUpload(ctx, request)
}

// UploadMode 返回 App 本地上传模式。
func (handler *CommHandler) UploadMode(ctx context.Context) (admin.UploadModeResult, error) {
	if handler == nil || handler.upload == nil {
		return admin.UploadModeResult{}, exception.Core("Base App 上传接口未初始化")
	}

	return handler.upload.AppMode(ctx)
}

// AppEPS 返回已发布的 App EPS 视图。
func AppEPS(context.Context) (eps.Document, error) {
	return eps.AppView()
}

// AppCommController 声明 Base App 通用路由。
func AppCommController(handler *CommHandler) controller.Definition {
	public := []controller.URLTag{{Name: controller.TagIgnoreToken}}

	return controller.App().
		Options(controller.RouterOptions{Description: "App 通用接口", TagName: "App 通用接口"}).
		Route(
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/param",
				Summary:     "参数配置",
				Handler:     controller.Handle(handler.Param),
				Bind:        controller.BindQuery,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/eps",
				Summary:     "实体信息与路径",
				Handler:     controller.Handle(AppEPS),
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/upload",
				Summary:     "文件上传",
				Handler:     controller.Handle(handler.Upload),
				Bind:        controller.BindFile,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/uploadMode",
				Summary:     "文件上传模式",
				Handler:     controller.Handle(handler.UploadMode),
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
