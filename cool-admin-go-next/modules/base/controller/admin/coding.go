package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	base "github.com/toothdy/cool-admin-go-next/modules/base"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// CodingCreateRequest 是批量创建 Go 文件的请求。
type CodingCreateRequest struct {
	Codes []codegen.CodeFile `json:"codes" v:"required"`
}

type adminRoleChecker interface {
	IsAdmin(context.Context, []uint64) (bool, error)
}

// ToolHandler 适配只允许平台管理员调用的通用代码生成工具（与具体实体无关，
// 对应 Node 版 AdminCodingController/BaseCodingService）。菜单驱动的代码
// 生成向导（解析/创建/导出/导入）与 Node 同源同表，挂在
// controller/admin/sys/menu.go 的 MenuToolHandler 下，不在这里。
type ToolHandler struct {
	scaffold   *codegen.Scaffold
	permission adminRoleChecker
}

// NewToolHandler 创建通用代码生成工具适配器。
func NewToolHandler(config base.Config, permission *service.PermissionService) (*ToolHandler, error) {
	if permission == nil {
		return nil, exception.Core("Base 工具接口依赖无效")
	}
	scaffold, err := codegen.NewScaffold(config.Coding.Workspace)
	if err != nil {
		return nil, err
	}

	return &ToolHandler{scaffold: scaffold, permission: permission}, nil
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

func (handler *ToolHandler) requireAdmin(ctx context.Context) error {
	if handler == nil || handler.scaffold == nil || handler.permission == nil {
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

// AdminCodingController 声明开发环境代码生成路由。
func AdminCodingController(handler *ToolHandler) controller.Definition {
	return controller.Admin().
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
