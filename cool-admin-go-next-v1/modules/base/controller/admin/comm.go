package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/upload"
)

func BaseAdminCommController(authService *sys.BaseSysLoginService, userService *sys.UserService, permissionService *sys.PermissionService, uploadService *upload.Service) controller.Definition {
	return controller.Comm("base/comm").
		Name("BaseAdminCommController").
		Description("管理系统通用接口").
		Service(authService).
		Route(controller.RouteOptions{
			Name:   "person",
			Method: http.MethodGet,
			Path:   "/person",
			Action: person(userService),
		}).
		Route(controller.RouteOptions{
			Name:        "personUpdate",
			Method:      http.MethodPost,
			Path:        "/personUpdate",
			Description: "修改个人信息",
			Action:      personUpdate(userService),
		}).
		Route(controller.RouteOptions{
			Name:        "uploadMode",
			Method:      http.MethodGet,
			Path:        "/uploadMode",
			Description: "文件上传模式",
			Action:      uploadMode,
		}).
		Route(controller.RouteOptions{
			Name:   "upload",
			Method: http.MethodPost, Path: "/upload",
			Description: "文件上传", Action: uploadFile(uploadService),
		}).
		Route(controller.RouteOptions{
			Name:   "permmenu",
			Method: http.MethodGet,
			Path:   "/permmenu",
			Action: permmenu(permissionService),
		}).
		Route(controller.RouteOptions{
			Name:   "logout",
			Method: http.MethodPost, Path: "/logout",
			Action: logout(authService),
		}).
		Route(controller.RouteOptions{
			Name:       "program",
			Method:     http.MethodGet,
			Path:       "/program",
			IgnoreAuth: true, Action: program,
		}).
		Build()
}

func person(service *sys.UserService) func(context.Context) (map[string]interface{}, error) {
	return func(ctx context.Context) (map[string]interface{}, error) {
		user, err := security.RequireUser(ctx)
		if err != nil {
			return nil, err
		}
		if service == nil || service.Base == nil || service.DB == nil {
			return nil, exception.Internal(nil, "用户服务不可用")
		}
		return service.Person(ctx, user.UserId)
	}
}

func personUpdate(service *sys.UserService) func(context.Context, *dto.PersonUpdateRequest) error {
	return func(ctx context.Context, request *dto.PersonUpdateRequest) error {
		user, err := security.RequireUser(ctx)
		if err != nil {
			return err
		}
		if service == nil || service.Base == nil || service.DB == nil {
			return exception.Internal(nil, "用户服务不可用")
		}
		return service.PersonUpdate(ctx, user.UserId, *request)
	}
}

func uploadMode() map[string]string {
	return map[string]string{"mode": "local", "type": "local"}
}

func uploadFile(s *upload.Service) func(context.Context, *dto.UploadReq) (string, error) {
	return func(ctx context.Context, request *dto.UploadReq) (string, error) {
		if s == nil {
			return "", exception.Internal(nil, "上传服务不可用")
		}
		return s.UploadWithKey(ctx, request.File, request.Key)
	}
}

func permmenu(s *sys.PermissionService) func(context.Context) (dto.PermMenuResult, error) {
	return func(ctx context.Context) (dto.PermMenuResult, error) {
		user, err := security.RequireUser(ctx)
		if err != nil {
			return dto.PermMenuResult{}, err
		}
		if s == nil {
			return dto.PermMenuResult{}, exception.Internal(nil, "权限服务不可用")
		}
		return s.PermMenu(ctx, user)
	}
}

// 退出登录
func logout(s *sys.BaseSysLoginService) func(context.Context) error {
	return func(ctx context.Context) error {
		user, err := security.RequireUser(ctx)
		if err != nil {
			return err
		}
		if s == nil {
			return nil
		}
		return s.Logout(ctx, user.SessionID)
	}
}

func program() string { return "Go" }
