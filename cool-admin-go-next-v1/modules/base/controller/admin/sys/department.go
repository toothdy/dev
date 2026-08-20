package sys

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

func AdminDepartmentController(departmentService *baseSysService.DepartmentService, baseSysDepartmentModel entity.Definition) controller.Definition {
	return controller.Admin("base/sys/department").
		Name("BaseSysDepartmentEntity").
		Description("系统部门").
		Model(baseSysDepartmentModel).
		Service(departmentService).
		CRUD(controller.CRUDOptions{
			API:         []string{crud.Add, crud.Delete, crud.Update, crud.List},
			SortFields:   []string{"id", "orderNum", "createTime", "updateTime"},
			DefaultSort:  "orderNum",
			DefaultOrder: "ASC",
		}).
		Route(controller.RouteOptions{
			Name: "order", Method: http.MethodPost, Path: "/order",
			Description: "排序", Permission: "base:sys:department:order",
			Action: order(departmentService),
		}).
		Build()
}

func order(service *baseSysService.DepartmentService) func(context.Context, *dto.DepartmentOrderReq) error {
	return func(ctx context.Context, request *dto.DepartmentOrderReq) error {
		if service == nil || service.Base == nil || service.DB == nil {
			return exception.Internal(nil, "部门服务不可用")
		}
		return service.Order(ctx, request.DepartmentOrders)
	}
}
