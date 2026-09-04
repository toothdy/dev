package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnctrl"
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

// 按参数键返回公开白名单内的清洗 HTML
func (handler *OpenHandler) HTML(ctx context.Context, request *HTMLQuery) (gnctrl.HTMLResponse, error) {
	if handler == nil || handler.param == nil || request == nil {
		return "", exception.Core("Base HTML 接口未初始化")
	}

	return handler.param.PublicHTMLByKey(ctx, request.Key)
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
func AdminEPS(context.Context) (map[string][]eps.Controller, error) {
	return eps.AdminView()
}

// Base 后台公开路由
func AdminOpenController(handler *OpenHandler) gnctrl.Definition {
	public := []gnctrl.URLTag{{Name: gnctrl.TagIgnoreToken}}

	return gnctrl.Admin().
		Options(gnctrl.RouterOptions{Description: "开放接口", TagName: "开放接口"}).
		Route(
			gnctrl.Route{
				Method:      http.MethodGet,
				Path:        "/eps",
				Summary:     "实体信息与路径",
				Handler:     gnctrl.Handle(AdminEPS),
				Tags:        public,
				Transaction: gnctrl.NonTransactional(),
			},
			gnctrl.Route{
				Method:      http.MethodGet,
				Path:        "/html",
				Summary:     "参数值",
				Handler:     gnctrl.Handle(handler.HTML),
				Bind:        gnctrl.BindQuery,
				Tags:        public,
				Transaction: gnctrl.NonTransactional(),
			},
			gnctrl.Route{
				Method:      http.MethodPost,
				Path:        "/login",
				Summary:     "登录",
				Handler:     gnctrl.Handle(handler.Login),
				Bind:        gnctrl.BindJSON,
				Tags:        public,
				Transaction: gnctrl.NonTransactional(),
			},
			gnctrl.Route{
				Method:      http.MethodGet,
				Path:        "/captcha",
				Summary:     "验证码",
				Handler:     gnctrl.Handle(handler.Captcha),
				Bind:        gnctrl.BindQuery,
				Tags:        public,
				Transaction: gnctrl.NonTransactional(),
			},
			gnctrl.Route{
				Method:      http.MethodPost,
				Path:        "/refreshToken",
				Summary:     "刷新token",
				Handler:     gnctrl.Handle(handler.Refresh),
				Bind:        gnctrl.BindJSON,
				Tags:        public,
				Transaction: gnctrl.NonTransactional(),
			},
		).
		Build()
}
