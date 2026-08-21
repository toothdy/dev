package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 适配用户管理自定义接口
type UserHandler struct {
	user *service.UserService
}

// 用户管理接口适配器
func NewUserHandler(user *service.UserService) *UserHandler {
	return &UserHandler{user: user}
}

// 新增用户并写入角色关系
func (handler *UserHandler) Add(ctx context.Context, request *dto.UserAddReq) (coreservice.AddResult[uint64], error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}

	return handler.user.Add(ctx, *request, identity.UserID)
}

// 更新用户并按提交状态替换角色关系
func (handler *UserHandler) Update(ctx context.Context, request *dto.UserUpdateReq) error {
	return handler.user.Update(ctx, request)
}

// 批量移动用户部门
func (handler *UserHandler) Move(ctx context.Context, request *dto.UserMoveReq) error {
	return handler.user.Move(ctx, *request)
}

// 当前管理员数据范围内的用户分页
func (handler *UserHandler) Page(ctx context.Context, request *dto.UserPageReq) (service.UserPageResult, error) {
	page := request.Page
	if page == 0 {
		page = 1
	}
	size := request.Size
	if size == 0 {
		size = 15
	}

	return handler.user.Page(ctx, page, size, service.UserPageFilter{
		DepartmentIDs: request.DepartmentIDs,
		KeyWord:       request.KeyWord,
		Status:        request.Status,
	})
}

// 系统用户管理路由
func AdminSysUserController(user *service.UserService, handler *UserHandler) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "系统用户", TagName: "系统用户"}).
		Curd(controller.CurdOption{
			API:            controller.API(controller.Delete, controller.Info, controller.List),
			Entity:         entity.User{},
			Service:        user,
			HiddenFields:   []controller.ColumnRef{controller.Field("password")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("passwordV"), controller.Field("socketId")},
		}).
		Route(
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/add",
				Summary: "新增",
				Handler: controller.Handle(handler.Add),
				Bind:    controller.BindJSON,
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/update",
				Summary: "更新",
				Handler: controller.Handle(handler.Update),
				Bind:    controller.BindJSON,
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/page",
				Summary:     "分页查询",
				Handler:     controller.Handle(handler.Page),
				Bind:        controller.BindJSON,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/move",
				Summary: "移动部门",
				Handler: controller.Handle(handler.Move),
				Bind:    controller.BindJSON,
			},
		).
		Build()
}
