package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// CodingCreateRequest 是批量创建 Go 文件的请求。
type CodingCreateRequest struct {
	Codes []service.CodeFile `json:"codes" v:"required"`
}

// MenuImportRequest 是菜单树导入请求。
type MenuImportRequest struct {
	Menus []service.MenuTree `json:"menus" v:"required"`
}

type adminRoleChecker interface {
	IsAdmin(context.Context, []uint64) (bool, error)
}

// ToolHandler 适配只允许平台管理员调用的开发和菜单工具。
type ToolHandler struct {
	coding     *service.CodingService
	menu       *service.MenuToolService
	permission adminRoleChecker
}

// NewToolHandler 创建开发和菜单工具适配器。
func NewToolHandler(
	coding *service.CodingService,
	menu *service.MenuToolService,
	permission *service.PermissionService,
) (*ToolHandler, error) {
	if coding == nil || menu == nil || permission == nil {
		return nil, exception.Core("Base 工具接口依赖无效")
	}

	return &ToolHandler{coding: coding, menu: menu, permission: permission}, nil
}

// GetModuleTree 返回允许生成代码的模块名称。
func (handler *ToolHandler) GetModuleTree(ctx context.Context) ([]string, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}

	return handler.coding.GetModuleTree()
}

// CreateCode 批量创建经过校验的 Go 文件。
func (handler *ToolHandler) CreateCode(ctx context.Context, request *CodingCreateRequest) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("代码创建请求不能为空")
	}

	return handler.coding.CreateCode(request.Codes)
}

// ParseMenu 静态解析菜单代码元数据。
func (handler *ToolHandler) ParseMenu(ctx context.Context, request *dto.MenuParseReq) (service.MenuParseResult, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return service.MenuParseResult{}, err
	}
	if request == nil {
		return service.MenuParseResult{}, exception.Validate("菜单解析请求不能为空")
	}

	return handler.menu.Parse(request.Entity, request.Controller, request.Module)
}

// CreateMenuCode 创建菜单对应的 Go 文件。
func (handler *ToolHandler) CreateMenuCode(ctx context.Context, request *service.MenuCreateInput) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单代码创建请求不能为空")
	}

	return handler.menu.Create(*request)
}

// ExportMenu 导出选中的菜单树。
func (handler *ToolHandler) ExportMenu(ctx context.Context, request *dto.MenuExportReq) ([]service.MenuTree, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, exception.Validate("菜单导出请求不能为空")
	}

	return handler.menu.Export(ctx, request.IDs)
}

// ImportMenu 导入菜单树。
func (handler *ToolHandler) ImportMenu(ctx context.Context, request *MenuImportRequest) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单导入请求不能为空")
	}

	return handler.menu.Import(ctx, request.Menus)
}

func (handler *ToolHandler) requireAdmin(ctx context.Context) error {
	if handler == nil || handler.coding == nil || handler.menu == nil || handler.permission == nil {
		return exception.Core("Base 工具接口未初始化")
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

// CodingController 声明开发环境代码生成路由。
func CodingController(handler *ToolHandler) controller.Definition {
	return controller.Admin("").
		Options(controller.RouterOptions{
			Description:     "AI 编码",
			TagName:         "AI 编码",
			DevelopmentOnly: true,
		}).
		Route(
			controller.Route{
				Method:      http.MethodGet,
				Path:        "/getModuleTree",
				Summary:     "获取模块目录结构",
				Handler:     controller.Handle(handler.GetModuleTree),
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/createCode",
				Summary:     "创建代码",
				Handler:     controller.Handle(handler.CreateCode),
				Bind:        controller.BindJSON,
				Transaction: controller.NonTransactional(),
			},
		).
		Build()
}

// MenuToolController 声明菜单导入导出和开发代码路由。
func MenuToolController(handler *ToolHandler) controller.Definition {
	return controller.Admin("base/sys/menu").
		Options(controller.RouterOptions{Description: "菜单工具", TagName: "菜单工具"}).
		Route(
			controller.Route{
				Method:          http.MethodPost,
				Path:            "/parse",
				Summary:         "解析",
				DevelopmentOnly: true,
				Handler:         controller.Handle(handler.ParseMenu),
				Bind:            controller.BindJSON,
				Transaction:     controller.NonTransactional(),
			},
			controller.Route{
				Method:          http.MethodPost,
				Path:            "/create",
				Summary:         "创建代码",
				DevelopmentOnly: true,
				Handler:         controller.Handle(handler.CreateMenuCode),
				Bind:            controller.BindJSON,
				Transaction:     controller.NonTransactional(),
			},
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/export",
				Summary:     "导出",
				Handler:     controller.Handle(handler.ExportMenu),
				Bind:        controller.BindJSON,
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:  http.MethodPost,
				Path:    "/import",
				Summary: "导入",
				Handler: controller.Handle(handler.ImportMenu),
				Bind:    controller.BindJSON,
			},
		).
		Build()
}
