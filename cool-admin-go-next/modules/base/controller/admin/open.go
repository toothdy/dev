package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/eps"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 按参数键读取富文本的查询请求
type HTMLQuery struct {
	Key string `json:"key" in:"query" v:"required"`
}

// 适配 Base 公开接口到静态 HTTP Handler 契约
type OpenHandler struct {
	login   *service.LoginService
	captcha *service.CaptchaService
	param   *service.ParamService
}

// Base 公开接口适配器
func NewOpenHandler(
	login *service.LoginService,
	captcha *service.CaptchaService,
	param *service.ParamService,
) (*OpenHandler, error) {
	if login == nil || captcha == nil || param == nil {
		return nil, exception.Core("Base 公开接口依赖无效")
	}

	return &OpenHandler{login: login, captcha: captcha, param: param}, nil
}

// 按参数键返回原始 HTML
func (handler *OpenHandler) HTML(ctx context.Context, request *HTMLQuery) (controller.HTMLResponse, error) {
	if handler == nil || handler.param == nil || request == nil {
		return "", exception.Core("Base HTML 接口未初始化")
	}

	return handler.param.HTMLByKey(ctx, request.Key)
}

// 后台登录
func (handler *OpenHandler) Login(ctx context.Context, request *dto.LoginReq) (dto.TokenResult, error) {
	if handler == nil || handler.login == nil || request == nil {
		return dto.TokenResult{}, exception.Core("Base 登录接口未初始化")
	}

	return handler.login.Login(ctx, *request)
}

// 图形验证码
func (handler *OpenHandler) Captcha(ctx context.Context, request *dto.CaptchaQuery) (dto.CaptchaResult, error) {
	if handler == nil || handler.captcha == nil || request == nil {
		return dto.CaptchaResult{}, exception.Core("Base 验证码接口未初始化")
	}

	return handler.captcha.Generate(ctx, *request)
}

// 原子刷新后台令牌
func (handler *OpenHandler) Refresh(ctx context.Context, request *dto.RefreshReq) (dto.TokenResult, error) {
	if handler == nil || handler.login == nil || request == nil {
		return dto.TokenResult{}, exception.Core("Base 刷新令牌接口未初始化")
	}

	return handler.login.Refresh(ctx, *request)
}

// 已发布的后台 EPS 视图（按模块分组的扁平 Controller 数组，兼容 cool-admin-vue 客户端契约）
func AdminEPS(context.Context) (map[string][]eps.LegacyController, error) {
	document, err := eps.AdminView()
	if err != nil {
		return nil, err
	}

	return eps.LegacyView(document), nil
}

// Base 后台公开路由
func AdminOpenController(handler *OpenHandler) controller.Definition {
	public := []controller.URLTag{{Name: controller.TagIgnoreToken}}

	return controller.Admin().
		Options(controller.RouterOptions{Description: "开放接口", TagName: "开放接口"}).
		Route(
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/eps",
				Summary:     "实体信息与路径",
				Handler:     controller.Handle(AdminEPS),
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/html",
				Summary:     "获得网页内容的参数值",
				Handler:     controller.Handle(handler.HTML),
				Bind:        controller.BindQuery,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/login",
				Summary:     "登录",
				Handler:     controller.Handle(handler.Login),
				Bind:        controller.BindJSON,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/captcha",
				Summary:     "验证码",
				Handler:     controller.Handle(handler.Captcha),
				Bind:        controller.BindQuery,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/refreshToken",
				Summary:     "刷新token",
				Handler:     controller.Handle(handler.Refresh),
				Bind:        controller.BindJSON,
				Tags:        public,
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
