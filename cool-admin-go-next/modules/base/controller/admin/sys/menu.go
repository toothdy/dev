package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// MenuController 声明系统菜单管理路由：CRUD 之外，/parse /create /export
// /import 四条开发者工具路由与 Node 版 BaseSysMenuController 同源同表，
// 因此和菜单 CRUD 挂在同一个 Controller 下，而不是放进通用的 coding.go
// （那里只留与具体实体无关的 getModuleTree/createCode）。
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
				Permission:  "base:sys:menu:list",
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

// MenuImportRequest 是菜单树导入请求。
type MenuImportRequest struct {
	Menus []dto.MenuTree `json:"menus" v:"required"`
}

type menuAdminChecker interface {
	IsAdmin(context.Context, []uint64) (bool, error)
}

// MenuToolHandler 适配只允许平台管理员调用的菜单代码生成与树导入导出。
type MenuToolHandler struct {
	scaffold   *codegen.Scaffold
	menu       *service.MenuService
	permission menuAdminChecker
}

// NewMenuToolHandler 创建菜单工具接口适配器。
func NewMenuToolHandler(
	config base.Config,
	menu *service.MenuService,
	permission *service.PermissionService,
) (*MenuToolHandler, error) {
	if menu == nil || permission == nil {
		return nil, exception.Core("菜单工具接口依赖无效")
	}
	scaffold, err := codegen.NewScaffold(config.Coding.Workspace)
	if err != nil {
		return nil, err
	}

	return &MenuToolHandler{scaffold: scaffold, menu: menu, permission: permission}, nil
}

// ParseMenu 静态解析菜单代码元数据。
func (handler *MenuToolHandler) ParseMenu(ctx context.Context, request *dto.MenuParseReq) (codegen.MenuParseResult, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return codegen.MenuParseResult{}, err
	}
	if request == nil {
		return codegen.MenuParseResult{}, exception.Validate("菜单解析请求不能为空")
	}

	return handler.scaffold.ParseMenu(request.Entity, request.Controller, request.Module)
}

// CreateMenuCode 创建菜单对应的 Go 文件。
func (handler *MenuToolHandler) CreateMenuCode(ctx context.Context, request *codegen.MenuCreateInput) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单代码创建请求不能为空")
	}

	return handler.scaffold.CreateMenuCode(*request)
}

// ExportMenu 导出选中的菜单树，不含维护字段。
func (handler *MenuToolHandler) ExportMenu(ctx context.Context, request *dto.MenuExportReq) ([]dto.MenuTree, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, exception.Validate("菜单导出请求不能为空")
	}
	if len(request.IDs) == 0 {
		return []dto.MenuTree{}, nil
	}

	return handler.menu.Export(ctx, request.IDs)
}

// ImportMenu 在调用方事务中插入菜单树，并用实际新 ID 重建父子关系。
func (handler *MenuToolHandler) ImportMenu(ctx context.Context, request *MenuImportRequest) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单导入请求不能为空")
	}
	return handler.menu.Import(ctx, request.Menus)
}

func (handler *MenuToolHandler) requireAdmin(ctx context.Context) error {
	if handler == nil || handler.scaffold == nil || handler.menu == nil || handler.permission == nil {
		return exception.Core("菜单工具接口未初始化")
	}
	identity, err := auth.Admin(ctx)
	if err != nil {
		return err
	}
	isAdmin, err := handler.permission.IsAdmin(ctx, identity.RoleIDs())
	if err != nil {
		return err
	}
	if !isAdmin {
		return exception.Comm("权限不足", http.StatusForbidden)
	}

	return nil
}
