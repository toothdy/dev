package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

// 系统用户控制器
func BaseAdminUserController(userService *sys.UserService, baseSysUserModel entity.Definition) controller.Definition {
	return controller.Admin("base/sys/user").
		Name("BaseAdminSysUserEntity").
		Description("系统用户").
		Model(baseSysUserModel).
		Service(userService).
		CRUD(controller.CRUDOptions{
			API:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			SortFields:   []string{"id", "createTime", "updateTime", "username"},
			HiddenFields: []string{"password"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "move", Method: http.MethodPost,
			Path:        "/move",
			Description: "移动部门",
			Permission:  "base:sys:user:move",
			Action:      move(userService),
		}).
		Build()
}

// 移动部门
func move(s *sys.UserService) func(context.Context, *dto.MoveReq) error {
	return func(ctx context.Context, r *dto.MoveReq) error {
		if s == nil || s.Base == nil || s.DB == nil {
			return exception.Internal(nil, "用户服务不可用")
		}
		return s.Move(ctx, *r)
	}
}
