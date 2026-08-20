package admin

import (
	"context"
	"net/http"
	"sort"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// CodingCreateRequest 是批量创建 Go 文件的请求。
type CodingCreateRequest struct {
	Codes []codegen.CodeFile `json:"codes" v:"required"`
}

// MenuImportRequest 是菜单树导入请求。
type MenuImportRequest struct {
	Menus []dto.MenuTree `json:"menus" v:"required"`
}

type menuExportRow struct {
	ID        uint64  `orm:"id"`
	ParentID  *uint64 `orm:"parentId"`
	Name      string  `orm:"name"`
	Router    *string `orm:"router"`
	Perms     *string `orm:"perms"`
	Type      int32   `orm:"type"`
	Icon      *string `orm:"icon"`
	OrderNum  int32   `orm:"orderNum"`
	ViewPath  *string `orm:"viewPath"`
	KeepAlive bool    `orm:"keepAlive"`
	IsShow    bool    `orm:"isShow"`
}

type adminRoleChecker interface {
	IsAdmin(context.Context, []uint64) (bool, error)
}

// ToolHandler 适配只允许平台管理员调用的开发和菜单工具：受控代码生成走
// cool-next/codegen 的 Scaffold；菜单树导入导出是 base 自己的 Menu 表读写，
// 用 cool-next/codegen 的通用树形组装/插入原语实现。
type ToolHandler struct {
	scaffold   *codegen.Scaffold
	menu       *coreservice.Base[entity.Menu, uint64]
	permission adminRoleChecker
}

// NewToolHandler 创建开发和菜单工具适配器。
func NewToolHandler(
	config base.Config,
	menu *coreservice.Base[entity.Menu, uint64],
	permission *service.PermissionService,
) (*ToolHandler, error) {
	if menu == nil || permission == nil {
		return nil, exception.Core("Base 工具接口依赖无效")
	}
	scaffold, err := codegen.NewScaffold(config.Coding.Workspace)
	if err != nil {
		return nil, err
	}

	return &ToolHandler{scaffold: scaffold, menu: menu, permission: permission}, nil
}

// GetModuleTree 返回允许生成代码的模块名称。
func (handler *ToolHandler) GetModuleTree(ctx context.Context) ([]string, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}

	return handler.scaffold.GetModuleTree()
}

// CreateCode 批量创建经过校验的 Go 文件。
func (handler *ToolHandler) CreateCode(ctx context.Context, request *CodingCreateRequest) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("代码创建请求不能为空")
	}

	return handler.scaffold.CreateCode(request.Codes)
}

// ParseMenu 静态解析菜单代码元数据。
func (handler *ToolHandler) ParseMenu(ctx context.Context, request *dto.MenuParseReq) (codegen.MenuParseResult, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return codegen.MenuParseResult{}, err
	}
	if request == nil {
		return codegen.MenuParseResult{}, exception.Validate("菜单解析请求不能为空")
	}

	return handler.scaffold.ParseMenu(request.Entity, request.Controller, request.Module)
}

// CreateMenuCode 创建菜单对应的 Go 文件。
func (handler *ToolHandler) CreateMenuCode(ctx context.Context, request *codegen.MenuCreateInput) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单代码创建请求不能为空")
	}

	return handler.scaffold.CreateMenuCode(*request)
}

// ExportMenu 导出选中的菜单树，不含维护字段。
func (handler *ToolHandler) ExportMenu(ctx context.Context, request *dto.MenuExportReq) ([]dto.MenuTree, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, exception.Validate("菜单导出请求不能为空")
	}
	if len(request.IDs) == 0 {
		return []dto.MenuTree{}, nil
	}
	model, err := handler.menu.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []menuExportRow
	if err = model.
		Fields("id", "parentId", "name", "router", "perms", "type", "icon", "orderNum", "viewPath", "keepAlive", "isShow").
		WhereIn("id", request.IDs).
		Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询导出菜单失败")
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].OrderNum != rows[right].OrderNum {
			return rows[left].OrderNum < rows[right].OrderNum
		}

		return rows[left].ID < rows[right].ID
	})

	return codegen.BuildTree(rows, menuExportRowID, menuExportRowParentID, buildMenuTreeNode), nil
}

// ImportMenu 在调用方事务中插入菜单树，并用实际新 ID 重建父子关系。
func (handler *ToolHandler) ImportMenu(ctx context.Context, request *MenuImportRequest) error {
	if err := handler.requireAdmin(ctx); err != nil {
		return err
	}
	if request == nil {
		return exception.Validate("菜单导入请求不能为空")
	}
	if _, err := handler.menu.Tx(ctx); err != nil {
		return err
	}
	model, err := handler.menu.Model(ctx)
	if err != nil {
		return err
	}

	return codegen.InsertTree(ctx, model, handler.menu.Descriptor(), request.Menus, nil, menuTreeValues, menuTreeChildren)
}

func menuExportRowID(row menuExportRow) uint64        { return row.ID }
func menuExportRowParentID(row menuExportRow) *uint64 { return row.ParentID }

func buildMenuTreeNode(row menuExportRow, children []dto.MenuTree) dto.MenuTree {
	return dto.MenuTree{
		Name: row.Name, Router: row.Router, Perms: row.Perms, Type: row.Type, Icon: row.Icon,
		OrderNum: row.OrderNum, ViewPath: row.ViewPath, KeepAlive: row.KeepAlive, IsShow: row.IsShow,
		ChildMenus: children,
	}
}

func menuTreeValues(node dto.MenuTree) map[string]any {
	return map[string]any{
		"name": node.Name, "router": stringValue(node.Router), "perms": stringValue(node.Perms),
		"type": node.Type, "icon": stringValue(node.Icon), "orderNum": node.OrderNum,
		"viewPath": stringValue(node.ViewPath), "keepAlive": node.KeepAlive, "isShow": node.IsShow,
	}
}

func menuTreeChildren(node dto.MenuTree) []dto.MenuTree { return node.ChildMenus }

func stringValue(value *string) any {
	if value == nil {
		return nil
	}

	return *value
}

func (handler *ToolHandler) requireAdmin(ctx context.Context) error {
	if handler == nil || handler.scaffold == nil || handler.menu == nil || handler.permission == nil {
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
