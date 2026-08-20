package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/task/dto"
	taskEvent "github.com/toothdy/cool-admin-go-next/modules/task/event"
	taskService "github.com/toothdy/cool-admin-go-next/modules/task/service"
)

// InfoController 返回 Node 兼容的任务管理 Controller
func InfoController(comm *taskEvent.Comm, taskInfoModel entity.Definition) controller.Definition {
	var service *taskService.InfoService
	if comm != nil {
		service = comm.Info()
	}
	return controller.Admin("task/info").
		Name("TaskInfoEntity").
		Description("任务管理").
		Model(taskInfoModel).
		Service(service).
		CRUD(controller.CRUDOptions{
			API:         []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.Page},
			PageQuery:    controller.QueryOptions{FieldEq: []string{"status", "type"}},
			HiddenFields: []string{"lockOwner"},
			ReadonlyFields: []string{
				"jobId", "repeatConf", "nextRunTime", "lastExecuteTime", "lockExpireTime", "lockOwner", "tenantId",
			},
			SortFields:  []string{"id", "createTime", "updateTime", "nextRunTime", "lastExecuteTime"},
			DefaultSort: "id", DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "once", Method: http.MethodPost, Path: "/once", Description: "执行一次",
			Permission: "task:info:once", Action: idAction(service, service.Once),
		}).
		Route(controller.RouteOptions{
			Name: "stop", Method: http.MethodPost, Path: "/stop", Description: "停止任务",
			Permission: "task:info:stop", Action: idAction(service, service.Stop),
		}).
		Route(controller.RouteOptions{
			Name: "start", Method: http.MethodPost, Path: "/start", Description: "开始任务",
			Permission: "task:info:start", Action: startAction(service),
		}).
		Route(controller.RouteOptions{
			Name: "log", Method: http.MethodGet, Path: "/log", Description: "任务日志",
			Permission: "task:info:log", Bind: controller.BindQuery, Action: logAction(service),
		}).
		Build()
}

func idAction(service *taskService.InfoService, action func(context.Context, int64) error) func(context.Context, *dto.IDReq) error {
	return func(ctx context.Context, request *dto.IDReq) error {
		if service == nil || action == nil {
			return exception.Internal(nil, "任务服务不可用")
		}
		return action(ctx, request.ID)
	}
}

func startAction(service *taskService.InfoService) func(context.Context, *dto.StartReq) error {
	return func(ctx context.Context, request *dto.StartReq) error {
		if service == nil {
			return exception.Internal(nil, "任务服务不可用")
		}
		return service.Start(ctx, request.ID, request.Type)
	}
}

func logAction(service *taskService.InfoService) func(context.Context, *dto.InfoLogRequest) (map[string]interface{}, error) {
	return func(ctx context.Context, request *dto.InfoLogRequest) (map[string]interface{}, error) {
		if service == nil {
			return nil, exception.Internal(nil, "任务服务不可用")
		}
		return service.Log(ctx, *request)
	}
}
