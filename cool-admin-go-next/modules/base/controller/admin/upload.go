package admin

import (
	"context"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// UploadRequest 是本地文件上传请求。
type UploadRequest struct {
	File *ghttp.UploadFile `file:"file" v:"required"`
	Key  string            `form:"key"`
}

// UploadReadRequest 是公开文件读取路径。
type UploadReadRequest struct {
	Date string `json:"date" in:"path" v:"required"`
	Name string `json:"name" in:"path" v:"required"`
}

// UploadModeResult 是前端上传插件使用的固定模式。
type UploadModeResult struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
}

// UploadHandler 适配本地上传和公开文件读取接口。
type UploadHandler struct {
	upload *service.UploadService
}

// NewUploadHandler 创建上传接口适配器。
func NewUploadHandler(upload *service.UploadService) (*UploadHandler, error) {
	if upload == nil {
		return nil, exception.Core("Base 上传接口依赖无效")
	}

	return &UploadHandler{upload: upload}, nil
}

// Read 返回受控公开文件响应。
func (handler *UploadHandler) Read(_ context.Context, request *UploadReadRequest) (controller.FileResponse, error) {
	if handler == nil || handler.upload == nil || request == nil {
		return controller.FileResponse{}, exception.Core("Base 文件读取接口未初始化")
	}

	return handler.upload.Read(request.Date, request.Name)
}

// AdminUpload 保存后台身份上传的文件。
func (handler *UploadHandler) AdminUpload(ctx context.Context, request *UploadRequest) (string, error) {
	if _, err := auth.Admin(ctx); err != nil {
		return "", err
	}

	return handler.save(request)
}

// AppUpload 保存 App 身份上传的文件。
func (handler *UploadHandler) AppUpload(ctx context.Context, request *UploadRequest) (string, error) {
	if _, err := auth.App(ctx); err != nil {
		return "", err
	}

	return handler.save(request)
}

// AdminMode 返回后台本地上传模式。
func (handler *UploadHandler) AdminMode(ctx context.Context) (UploadModeResult, error) {
	if _, err := auth.Admin(ctx); err != nil {
		return UploadModeResult{}, err
	}

	return localUploadMode(), nil
}

// AppMode 返回 App 本地上传模式。
func (handler *UploadHandler) AppMode(ctx context.Context) (UploadModeResult, error) {
	if _, err := auth.App(ctx); err != nil {
		return UploadModeResult{}, err
	}

	return localUploadMode(), nil
}

func (handler *UploadHandler) save(request *UploadRequest) (string, error) {
	if handler == nil || handler.upload == nil || request == nil {
		return "", exception.Core("Base 上传接口未初始化")
	}

	return handler.upload.Save(request.File, request.Key)
}

func localUploadMode() UploadModeResult {
	return UploadModeResult{Mode: "local", Type: "local"}
}

// UploadController 声明不带全局前缀的公开文件路由。
func UploadController(handler *UploadHandler) controller.Definition {
	return controller.Admin("upload").
		Options(controller.RouterOptions{
			Description:        "公开文件",
			TagName:            "公开文件",
			IgnoreGlobalPrefix: true,
		}).
		Route(controller.Route{
			Method:      http.MethodGet,
			Path:        "/{date}/{name}",
			Summary:     "读取上传文件",
			Handler:     controller.Handle(handler.Read),
			Bind:        controller.BindPath,
			Tags:        []controller.URLTag{{Name: controller.TagIgnoreToken}},
			Transaction: controller.NonTransactional(),
		}).
		Build()
}
