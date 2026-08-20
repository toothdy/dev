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

func MenuController(menuService *sys.MenuService, baseSysMenuModel entity.Definition) controller.Definition {
	return controller.Admin("base/sys/menu").
		Name("BaseSysMenuEntity").
		Description("系统菜单").
		Model(baseSysMenuModel).
		Service(menuService).
		CRUD(controller.CRUDOptions{
			API:          []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			SortFields:   []string{"id", "orderNum", "createTime", "updateTime"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "parse", Method: http.MethodPost, Path: "/parse",
			Description: "解析", Permission: "base:sys:menu:parse",
			Action: parse(menuService),
		}).
		Route(controller.RouteOptions{
			Name: "export", Method: http.MethodPost, Path: "/export",
			Description: "导出", Permission: "base:sys:menu:export",
			Action: export(menuService),
		}).
		Route(controller.RouteOptions{
			Name: "import", Method: http.MethodPost, Path: "/import",
			Description: "导入", Permission: "base:sys:menu:import",
			Action: importMenu(menuService),
		}).
		Build()
}

func parse(service *sys.MenuService) func(*dto.MenuParseReq) (dto.MenuParseRes, error) {
	return func(request *dto.MenuParseReq) (dto.MenuParseRes, error) {
		if service == nil {
			return dto.MenuParseRes{}, exception.Internal(nil, "菜单服务不可用")
		}
		return service.Parse(*request)
	}
}

func export(service *sys.MenuService) func(context.Context, *dto.MenuExportReq) ([]dto.MenuExportItem, error) {
	return func(ctx context.Context, request *dto.MenuExportReq) ([]dto.MenuExportItem, error) {
		if service == nil {
			return nil, exception.Internal(nil, "菜单服务不可用")
		}
		return service.Export(ctx, request.IDs)
	}
}

func importMenu(service *sys.MenuService) func(context.Context, *dto.MenuImportReq) error {
	return func(ctx context.Context, request *dto.MenuImportReq) error {
		if service == nil {
			return exception.Internal(nil, "菜单服务不可用")
		}
		return service.Import(ctx, request.Menus)
	}
}
