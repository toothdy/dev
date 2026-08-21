package sys

import (
	"context"
	"net/http"
	"strconv"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// RoleController 声明系统角色管理路由。
func RoleController(role *service.RoleService) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "系统角色", TagName: "系统角色"}).
		Curd(controller.CurdOption{
			API:     controller.APIs(controller.APIAdd, controller.APIDelete, controller.APIUpdate, controller.APIInfo, controller.APIPage),
			Entity:  entity.Role{},
			Service: role,
			InsertParam: controller.Insert[entity.Role](func(ctx context.Context, input *coreservice.Mutable[entity.Role]) error {
				identity, err := auth.Admin(ctx)
				if err != nil {
					return err
				}

				return input.Set("userId", strconv.FormatUint(identity.UserID, 10))
			}),
		}).
		Route(controller.Route{
			Method:      http.MethodPost,
			Path:        "/list",
			Summary:     "列表查询",
			Handler:     controller.Handle(role.List),
			Permission:  "base:sys:role:list",
			Transaction: controller.NonTransactional(),
		}).
		Build()
}
