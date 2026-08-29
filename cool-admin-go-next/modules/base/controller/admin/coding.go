package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/codegen"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/modules/base"
)

// 批量创建 Go 文件的请求
type CodingCreateRequest struct {
	Codes []codegen.CodeFile `json:"codes" v:"required"`
}

// 实体无关的代码生成工具，与 Node 版 AdminCodingController 对齐。
// 访问控制由路由权限标识承担：超管全通，其他角色需在后台菜单中获得授权
type ToolHandler struct {
	scaffold *codegen.Scaffold
}

// 通用代码生成工具适配器
func NewToolHandler(config base.Config) (*ToolHandler, error) {
	scaffold, err := codegen.NewScaffold(config.Coding.Workspace)
	if err != nil {
		return nil, err
	}

	return &ToolHandler{scaffold: scaffold}, nil
}

// 允许生成代码的模块名称
func (handler *ToolHandler) GetModuleTree(ctx context.Context) ([]string, error) {
	if handler == nil || handler.scaffold == nil {
		return nil, exception.Core("Base 工具接口未初始化")
	}

	return handler.scaffold.GetModuleTree()
}

// 批量创建经过校验的 Go 文件
func (handler *ToolHandler) CreateCode(ctx context.Context, request *CodingCreateRequest) error {
	if handler == nil || handler.scaffold == nil {
		return exception.Core("Base 工具接口未初始化")
	}
	if request == nil {
		return exception.Validate("代码创建请求不能为空")
	}

	return handler.scaffold.CreateCode(request.Codes)
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
