package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 系统用户管理路由
func AdminSysUserController(user *service.UserService) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "系统用户", TagName: "系统用户"}).
		Curd(controller.CurdOption{
			API:     controller.AllAPI(),
			Entity:  entity.User{},
			Service: user,
			InsertParam: controller.Insert(func(ctx context.Context, input *coreservice.Mutable[entity.User]) error {
				identity, err := auth.Admin(ctx)
				if err != nil {
					return err
				}

				return input.Set("userId", identity.UserID)
			}),
			HiddenFields:   []controller.ColumnRef{controller.Field("password")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("passwordV"), controller.Field("socketId")},
		}).
		Route(controller.Route{
			Method:  http.MethodPost,
			Path:    "/move",
			Summary: "移动部门",
			Handler: controller.Handle(user.Move),
			Bind:    controller.BindJSON,
		}).
		Build()
}
