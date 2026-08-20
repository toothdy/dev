package app

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/util/eps"
	"github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/controller/admin"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/upload"
)

// 应用端通用接口
func BaseAppCommController(paramService *sys.ParamService, epsService *admin.EPSService, uploadService *upload.Service, config base.Config) controller.Definition {
	allowed := make(map[string]struct{}, len(config.AllowKeys))
	for _, key := range config.AllowKeys {
		allowed[key] = struct{}{}
	}
	return controller.App("base/comm").
		Name("BaseAppCommController").
		Description("应用端通用接口").
		Route(controller.RouteOptions{
			Name:        "param",
			Method:      http.MethodGet,
			Path:        "/param",
			Description: "参数配置",
			IgnoreAuth:  true,
			Action:      param(paramService, allowed),
		}).
		Route(controller.RouteOptions{
			Name:        "eps",
			Method:      http.MethodGet,
			Path:        "/eps",
			Description: "实体信息与路径",
			IgnoreAuth:  true,
			Action:      getEps(epsService),
		}).
		Route(controller.RouteOptions{
			Name:        "upload",
			Method:      http.MethodPost,
			Path:        "/upload",
			Description: "文件上传",
			Action:      uploadFile(uploadService),
		}).
		Route(controller.RouteOptions{
			Name:        "uploadMode",
			Method:      http.MethodGet,
			Path:        "/uploadMode",
			Description: "文件上传模式",
			Action:      uploadMode,
		}).
		Build()
}

// 获取参数配置
func param(s *sys.ParamService, allowed map[string]struct{}) func(context.Context, *dto.ParamReq) (interface{}, error) {
	return func(ctx context.Context, r *dto.ParamReq) (interface{}, error) {
		if _, ok := allowed[r.Key]; !ok {
			return nil, exception.Comm("非法操作")
		}
		if s == nil {
			return nil, exception.Internal(nil, "参数服务不可用")
		}
		return s.DataByKey(ctx, r.Key)
	}
}

// 获取 EPS
func getEps(s *admin.EPSService) func(context.Context) (map[string][]eps.Controller, error) {
	return func(ctx context.Context) (map[string][]eps.Controller, error) {
		if s == nil {
			return map[string][]eps.Controller{}, nil
		}
		return s.App(ctx)
	}
}

// 上传文件
func uploadFile(s *upload.Service) func(context.Context, *dto.UploadReq) (string, error) {
	return func(ctx context.Context, r *dto.UploadReq) (string, error) {
		if s == nil {
			return "", exception.Internal(nil, "上传服务不可用")
		}
		return s.UploadWithKey(ctx, r.File, r.Key)
	}
}

// 上传模式
func uploadMode() map[string]string {
	return map[string]string{"mode": "local", "type": "local"}
}
