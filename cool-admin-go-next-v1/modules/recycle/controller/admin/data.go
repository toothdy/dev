package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/recycle/dto"
	recycleSchedule "github.com/toothdy/cool-admin-go-next/modules/recycle/schedule"
)

// DataController 返回 Node 兼容的数据回收 Controller。
func DataController(service recycleSchedule.Service, dataModel entity.Definition) controller.Definition {
	return controller.Admin("recycle/data").
		Name("RecycleDataEntity").
		Description("数据回收").
		Model(dataModel).
		Service(service).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Info, crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"url"},
			},
			SortFields:   []string{"id", "createTime", "updateTime", "count", "remainingCount"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "restore", Method: http.MethodPost, Path: "/restore", Description: "恢复数据",
			Permission: "recycle:data:restore", Action: restoreAction(service),
		}).
		Build()
}

func restoreAction(service recycleSchedule.Service) func(context.Context, *dto.RestoreRequest) error {
	return func(ctx context.Context, request *dto.RestoreRequest) error {
		if service == nil {
			return exception.Internal(nil, "Recycle 服务不可用")
		}
		return service.Restore(ctx, request.IDs)
	}
}
