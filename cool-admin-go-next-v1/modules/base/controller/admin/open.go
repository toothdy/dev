package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/util/eps"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

// EPS 服务
type EPSService struct {
	controllers module.ControllerProvider
}

func NewEPSService(controllers module.ControllerProvider) *EPSService {
	return &EPSService{controllers: controllers}
}

func (s *EPSService) Admin(ctx context.Context) (map[string][]eps.Controller, error) {
	_ = ctx
	if s == nil || s.controllers == nil {
		return map[string][]eps.Controller{}, nil
	}
	return eps.GenerateAdmin(s.controllers.Controllers()), nil
}

func (s *EPSService) App(ctx context.Context) (map[string][]eps.Controller, error) {
	_ = ctx
	if s == nil || s.controllers == nil {
		return map[string][]eps.Controller{}, nil
	}
	return eps.GenerateApp(s.controllers.Controllers()), nil
}

// 管理系统开放接口
func BaseAdminOpenController(authService *sys.BaseSysLoginService, epsService *EPSService) controller.Definition {
	getEps := func(context.Context) (map[string][]eps.Controller, error) {
		return map[string][]eps.Controller{}, nil
	}
	if epsService != nil {
		getEps = epsService.Admin
	}

	return controller.Open("base/open").
		Name("BaseAdminOpenController").
		Description("管理系统开放接口").
		Service(authService).
		Route(controller.RouteOptions{
			Name:       "login",
			Method:     http.MethodPost,
			Path:       "/login",
			IgnoreAuth: true,
			Action:     login(authService),
		}).
		Route(controller.RouteOptions{
			Name:       "refreshToken",
			Method:     http.MethodPost,
			Path:       "/refreshToken",
			IgnoreAuth: true,
			Action:     refreshToken(authService),
		}).
		Route(controller.RouteOptions{
			Name:       "captcha",
			Method:     http.MethodGet,
			Path:       "/captcha",
			IgnoreAuth: true,
			Action:     captcha(authService),
		}).
		Route(controller.RouteOptions{
			Name:        "eps",
			Method:      http.MethodGet,
			Path:        "/eps",
			Description: "EPS",
			IgnoreAuth:  true,
			Action:      getEps,
		}).Build()
}

// 登录
func login(s *sys.BaseSysLoginService) func(context.Context, *dto.LoginReq) (security.TokenPair, error) {
	return func(ctx context.Context, r *dto.LoginReq) (security.TokenPair, error) {
		if s == nil {
			return security.TokenPair{}, exception.Internal(nil, "认证服务不可用")
		}
		return s.Login(ctx, *r)
	}
}

// 刷新token
func refreshToken(s *sys.BaseSysLoginService) func(context.Context, *dto.RefreshTokenReq) (security.TokenPair, error) {
	return func(ctx context.Context, r *dto.RefreshTokenReq) (security.TokenPair, error) {
		if s == nil {
			return security.TokenPair{}, exception.Internal(nil, "认证服务不可用")
		}
		return s.RefreshToken(ctx, r.RefreshToken)
	}
}

// 获取验证码
func captcha(s *sys.BaseSysLoginService) func(context.Context, *dto.CaptchaReq) (sys.Captcha, error) {
	return func(ctx context.Context, r *dto.CaptchaReq) (sys.Captcha, error) {
		if s == nil {
			return sys.Captcha{}, exception.Internal(nil, "认证服务不可用")
		}
		return s.Captcha(ctx, r.Height, r.Width, r.Color)
	}
}
