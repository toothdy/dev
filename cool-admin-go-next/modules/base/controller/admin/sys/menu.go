package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// 菜单 CRUD 与解析/创建/导出/导入同源同表，与 Node 版 BaseSysMenuController 一致
func AdminSysMenuController(menu *service.MenuService, tool *MenuToolHandler) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "系统菜单", TagName: "系统菜单"}).
		Curd(controller.CurdOption{
			API:            controller.API(controller.Add, controller.Delete, controller.Update, controller.Info, controller.Page),
			Entity:         entity.Menu{},
			Service:        menu,
			HiddenFields:   []controller.ColumnRef{controller.Field("seedKey")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("seedKey")},
		}).
		Route(
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/list",
				Summary:     "列表查询",
				Handler:     controller.Handle(menu.List),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:          http.MethodPost,
				Path:            "/parse",
				Summary:         "解析",
				DevelopmentOnly: true,
				Handler:         controller.Handle(tool.ParseMenu),
				Bind:            controller.BindJSON,
				Transaction:     controller.NonTransactional(),
			},
			controller.Route{
				Method:          http.MethodPost,
				Path:            "/create",
				Summary:         "创建代码",
				DevelopmentOnly: true,
				Handler:         controller.Handle(tool.CreateMenuCode),
				Bind:            controller.BindJSON,
				Transaction:     controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/export",
				Summary:     "导出",
				Handler:     controller.Handle(tool.ExportMenu),
				Bind:        controller.BindJSON,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/import",
				Summary: "导入",
				Handler: controller.Handle(tool.ImportMenu),
				Bind:    controller.BindJSON,
			},
		).
		Build()
}

// 菜单树导入请求
type MenuImportRequest struct {
	Menus []dto.MenuTree `json:"menus" v:"required"`
}

// 菜单代码生成与树导入导出。
// 访问控制由路由权限标识承担：超管全通，其他角色需在后台菜单中获得授权
type MenuToolHandler struct {
	scaffold *codegen.Scaffold
	menu     *service.MenuService
}

// 菜单工具接口适配器
func NewMenuToolHandler(config base.Config, menu *service.MenuService) (*MenuToolHandler, error) {
	if menu == nil {
		return nil, exception.Core("菜单工具接口依赖无效")
	}
	scaffold, err := codegen.NewScaffold(config.Coding.Workspace)
	if err != nil {
		return nil, err
	}

	return &MenuToolHandler{scaffold: scaffold, menu: menu}, nil
}

// 静态解析菜单代码元数据
func (handler *MenuToolHandler) ParseMenu(ctx context.Context, request *dto.MenuParseReq) (codegen.MenuParseResult, error) {
	if handler == nil || handler.scaffold == nil {
		return codegen.MenuParseResult{}, exception.Core("菜单工具接口未初始化")
	}
	if request == nil {
		return codegen.MenuParseResult{}, exception.Validate("菜单解析请求不能为空")
	}

	return handler.scaffold.ParseMenu(request.Entity, request.Controller, request.Module)
}

// 创建菜单对应的 Go 文件
func (handler *MenuToolHandler) CreateMenuCode(ctx context.Context, request *codegen.MenuCreateInput) error {
	if handler == nil || handler.scaffold == nil {
		return exception.Core("菜单工具接口未初始化")
	}
	if request == nil {
		return exception.Validate("菜单代码创建请求不能为空")
	}

	return handler.scaffold.CreateMenuCode(*request)
}

// 导出选中的菜单树，不含维护字段
func (handler *MenuToolHandler) ExportMenu(ctx context.Context, request *dto.MenuExportReq) ([]dto.MenuTree, error) {
	if handler == nil || handler.menu == nil {
		return nil, exception.Core("菜单工具接口未初始化")
	}
	if request == nil {
		return nil, exception.Validate("菜单导出请求不能为空")
	}
	if len(request.IDs) == 0 {
		return []dto.MenuTree{}, nil
	}

	return handler.menu.Export(ctx, request.IDs)
}

// 在调用方事务中插入菜单树，并用实际新 ID 重建父子关系
func (handler *MenuToolHandler) ImportMenu(ctx context.Context, request *MenuImportRequest) error {
	if handler == nil || handler.menu == nil {
		return exception.Core("菜单工具接口未初始化")
	}
	if request == nil {
		return exception.Validate("菜单导入请求不能为空")
	}
	return handler.menu.Import(ctx, request.Menus)
}
