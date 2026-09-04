package admin

import (
	"context"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 本地文件上传请求
type UploadRequest struct {
	File *ghttp.UploadFile `file:"file" v:"required"`
	Key  string            `form:"key"`
}

// 公开文件读取路径
type UploadReadRequest struct {
	Date string `json:"date" in:"path" v:"required"`
	Name string `json:"name" in:"path" v:"required"`
}

// 前端上传插件使用的固定模式
type UploadModeResult struct {
	Mode string `json:"mode"`
	Type string `json:"type"`
}

// 适配本地上传和公开文件读取接口
type UploadHandler struct {
	upload *service.UploadService
}

// 上传接口适配器
func NewUploadHandler(upload *service.UploadService) (*UploadHandler, error) {
	if upload == nil {
		return nil, exception.Core("Base 上传接口依赖无效")
	}

	return &UploadHandler{upload: upload}, nil
}

// 受控公开文件响应
func (handler *UploadHandler) Read(_ context.Context, request *UploadReadRequest) (gnctrl.FileResponse, error) {
	if handler == nil || handler.upload == nil || request == nil {
		return gnctrl.FileResponse{}, exception.Core("Base 文件读取接口未初始化")
	}

	return handler.upload.Read(request.Date, request.Name)
}

// 后台身份上传的文件
func (handler *UploadHandler) AdminUpload(ctx context.Context, request *UploadRequest) (string, error) {
	if _, err := auth.Admin(ctx); err != nil {
		return "", err
	}

	return handler.save(request)
}

// App 身份上传的文件
func (handler *UploadHandler) AppUpload(ctx context.Context, request *UploadRequest) (string, error) {
	if _, err := auth.App(ctx); err != nil {
		return "", err
	}

	return handler.save(request)
}

// 后台本地上传模式
func (handler *UploadHandler) AdminMode(ctx context.Context) (UploadModeResult, error) {
	if _, err := auth.Admin(ctx); err != nil {
		return UploadModeResult{}, err
	}

	return localUploadMode(), nil
}

// App 本地上传模式
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

// 不带全局前缀的公开文件路由
func AdminUploadController(handler *UploadHandler) gnctrl.Definition {
	return gnctrl.Admin("upload").
		Options(gnctrl.RouterOptions{
			Description:        "公开文件",
			TagName:            "公开文件",
			IgnoreGlobalPrefix: true,
		}).
		Route(gnctrl.Route{
			Method:      http.MethodGet,
			Path:        "/{date}/{name}",
			Summary:     "读取",
			Handler:     gnctrl.Handle(handler.Read),
			Bind:        gnctrl.BindPath,
			Tags:        []gnctrl.URLTag{{Name: gnctrl.TagIgnoreToken}},
			Transaction: gnctrl.NonTransactional(),
		}).
		Build()
}
