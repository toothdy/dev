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

func LogController(
	logService *baseSysService.LogService,
	confService *baseSysService.ConfService,
	baseSysLogModel entity.Definition,
	baseSysUserModel entity.Definition,
) controller.Definition {
	return controller.Admin("base/sys/log").
		Name("BaseSysLogEntity").
		Description("操作日志").
		Model(baseSysLogModel).
		Service(logService).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"b.name", "a.action", "a.ip"},
			},
			PageSelect: []controller.PageSelectOptions{
				{Alias: "a", Model: baseSysLogModel},
				{Alias: "b", Model: baseSysUserModel, Fields: []string{"name"}},
			},
			SortFields:   []string{"id", "createTime", "updateTime", "userId"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "clear", Method: http.MethodPost, Path: "/clear",
			Description: "清理", Permission: "base:sys:log:clear",
			Action: clear(logService),
		}).
		Route(controller.RouteOptions{
			Name: "setKeep", Method: http.MethodPost, Path: "/setKeep",
			Description: "日志保存时间", Permission: "base:sys:log:setKeep",
			Action: setKeep(confService),
		}).
		Route(controller.RouteOptions{
			Name: "getKeep", Method: http.MethodGet, Path: "/getKeep",
			Description: "获得日志保存时间", Permission: "base:sys:log:getKeep",
			Action: getKeep(confService),
		}).
		Build()
}

func clear(service *baseSysService.LogService) func(context.Context) error {
	return func(ctx context.Context) error {
		if service == nil || service.Base == nil || service.DB == nil {
			return exception.Internal(nil, "操作日志服务不可用")
		}
		return service.Clear(ctx)
	}
}

func setKeep(service *baseSysService.ConfService) func(context.Context, *dto.LogKeepRequest) error {
	return func(ctx context.Context, request *dto.LogKeepRequest) error {
		if service == nil || service.Base == nil || service.DB == nil {
			return exception.Internal(nil, "配置服务不可用")
		}
		return service.UpdateValue(ctx, "logKeep", request.Value)
	}
}

func getKeep(service *baseSysService.ConfService) func(context.Context) (interface{}, error) {
	return func(ctx context.Context) (interface{}, error) {
		if service == nil || service.Base == nil || service.DB == nil {
			return nil, exception.Internal(nil, "配置服务不可用")
		}
		return service.GetValue(ctx, "logKeep")
	}
}
