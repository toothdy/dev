package sys

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
	"github.com/toothdy/cool-admin-go-next/modules/base/service"
)

// DepartmentHandler 适配部门删除和排序接口。
type DepartmentHandler struct {
	department *service.DepartmentService
}

// DepartmentOrderBody 保持前端顶层数组请求契约。
type DepartmentOrderBody struct {
	Items dto.DepartmentOrderReq `json:"-"`
}

// UnmarshalJSON 解码顶层部门排序数组。
func (request *DepartmentOrderBody) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &request.Items)
}

// NewDepartmentHandler 创建部门管理接口适配器。
func NewDepartmentHandler(department *service.DepartmentService) *DepartmentHandler {
	return &DepartmentHandler{department: department}
}

// Delete 按用户处理策略删除部门树。
func (handler *DepartmentHandler) Delete(ctx context.Context, request *dto.DepartmentDeleteReq) error {
	return handler.department.Delete(ctx, *request)
}

// Order 更新部门树顺序。
func (handler *DepartmentHandler) Order(ctx context.Context, request *DepartmentOrderBody) error {
	return handler.department.Order(ctx, request.Items)
}

// AdminSysDepartmentController 声明系统部门管理路由。
func AdminSysDepartmentController(department *service.DepartmentService, handler *DepartmentHandler) controller.Definition {
	return controller.Admin().
		Options(controller.RouterOptions{Description: "系统部门", TagName: "系统部门"}).
		Curd(controller.CurdOption{
			API:     controller.API(controller.Add, controller.Update),
			Entity:  entity.Department{},
			Service: department,
			InsertParam: controller.Insert(func(ctx context.Context, input *coreservice.Mutable[entity.Department]) error {
				identity, err := auth.Admin(ctx)
				if err != nil {
					return err
				}

				return input.Set("userId", identity.UserID)
			}),
			HiddenFields:   []controller.ColumnRef{controller.Field("seedKey")},
			ReadonlyFields: []controller.ColumnRef{controller.Field("seedKey")},
		}).
		Route(
			controller.Route{
				Method:      http.MethodPost,
				Path:        "/list",
				Summary:     "列表查询",
				Handler:     controller.Handle(department.List),
				Permission:  "base:sys:department:list",
				Transaction: controller.NonTransactional(),
			},
			controller.Route{
				Method:     http.MethodPost,
				Path:       "/delete",
				Summary:    "删除",
				Handler:    controller.Handle(handler.Delete),
				Bind:       controller.BindJSON,
				Permission: "base:sys:department:delete",
			},
			controller.Route{
				Method:     http.MethodPost,
				Path:       "/order",
				Summary:    "排序",
				Handler:    controller.Handle(handler.Order),
				Bind:       controller.BindJSON,
				Permission: "base:sys:department:order",
			},
		).
		Build()
}
