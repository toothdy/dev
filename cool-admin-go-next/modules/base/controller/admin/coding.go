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

// 批量创建 Go 文件的请求
type CodingCreateRequest struct {
	Codes []codegen.CodeFile `json:"codes" v:"required"`
}

type adminRoleChecker interface {
	IsAdmin(context.Context, []uint64) (bool, error)
}

// 平台管理员可调的实体无关代码生成工具，与 Node 版 AdminCodingController 对齐
type ToolHandler struct {
	scaffold   *codegen.Scaffold
	permission adminRoleChecker
}

// 通用代码生成工具适配器
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

// 允许生成代码的模块名称
func (handler *ToolHandler) GetModuleTree(ctx context.Context) ([]string, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}

	return handler.scaffold.GetModuleTree()
}

// 批量创建经过校验的 Go 文件
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

// 开发环境代码生成路由
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
