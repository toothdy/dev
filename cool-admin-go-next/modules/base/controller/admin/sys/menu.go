package sys

import (
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// MenuController 声明系统菜单管理路由。
func MenuController(menu *service.MenuService) controller.Definition {
	return controller.Admin("").
		Options(controller.RouterOptions{Description: "系统菜单", TagName: "系统菜单"}).
		Curd(controller.CurdOption{
			API:            controller.APIs(controller.APIAdd, controller.APIDelete, controller.APIUpdate, controller.APIInfo, controller.APIPage),
			Entity:         entity.Menu{},
			Service:        menu,
			HiddenFields:   []controller.ColumnRef{controller.Field("seedKey")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("seedKey")},
		}).
		Route(controller.Route{
			Method:      http.MethodPost,
			Path:        "/list",
			Summary:     "列表查询",
			Handler:     controller.Handle(menu.List),
			Permission:  "base:sys:menu:list",
			Transaction: controller.NonTransactional(),
		}).
		Build()
}
