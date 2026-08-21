package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// CommHandler 适配后台通用业务接口。
type CommHandler struct {
	user       *service.UserService
	permission *service.PermissionService
}

// NewCommHandler 创建后台通用接口适配器。
func NewCommHandler(
	user *service.UserService,
	permission *service.PermissionService,
) (*CommHandler, error) {
	if user == nil || permission == nil {
		return nil, exception.Core("Base 后台通用接口依赖无效")
	}

	return &CommHandler{user: user, permission: permission}, nil
}

// Person 返回当前管理员个人信息。
func (handler *CommHandler) Person(ctx context.Context) (*dto.UserInfoResult, error) {
	if handler == nil || handler.user == nil {
		return nil, exception.Core("Base 个人信息接口未初始化")
	}

	return handler.user.Person(ctx)
}

// PersonUpdate 更新当前管理员个人信息。
func (handler *CommHandler) PersonUpdate(ctx context.Context, request *dto.PersonUpdateReq) error {
	if handler == nil || handler.user == nil || request == nil {
		return exception.Core("Base 个人信息更新接口未初始化")
	}

	return handler.user.PersonUpdate(ctx, *request)
}

// PermissionMenu 返回当前管理员的权限与菜单树。
func (handler *CommHandler) PermissionMenu(ctx context.Context) (dto.PermissionMenuResult, error) {
	if handler == nil || handler.permission == nil {
		return dto.PermissionMenuResult{}, exception.Core("Base 权限菜单接口未初始化")
	}

	return handler.permission.PermissionMenu(ctx)
}

// Program 返回当前后端实现语言。
func Program(context.Context) (string, error) {
	return "Go", nil
}

// AdminCommController 声明已有依赖可执行的后台通用路由。
func AdminCommController(handler *CommHandler, login *service.LoginService, upload *UploadHandler) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "Base 通用接口", TagName: "Base 通用接口"}).
		Route(
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/person",
				Summary:     "个人信息",
				Handler:     controller.Handle(handler.Person),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/personUpdate",
				Summary: "修改个人信息",
				Handler: controller.Handle(handler.PersonUpdate),
				Bind:    controller.BindJSON,
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/permmenu",
				Summary:     "权限与菜单",
				Handler:     controller.Handle(handler.PermissionMenu),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/upload",
				Summary:     "文件上传",
				Handler:     controller.Handle(upload.AdminUpload),
				Bind:        controller.BindFile,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/uploadMode",
				Summary:     "文件上传模式",
				Handler:     controller.Handle(upload.AdminMode),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/logout",
				Summary:     "退出",
				Handler:     controller.Handle(login.Logout),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/program",
				Summary:     "编程",
				Handler:     controller.Handle(Program),
				Tags:        []controller.URLTag{{Name: controller.TagIgnoreToken}},
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}
