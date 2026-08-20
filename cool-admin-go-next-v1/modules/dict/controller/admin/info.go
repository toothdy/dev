package admin

import (
	"context"
	"net/http"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/dict/dto"
	"github.com/toothdy/cool-admin-go-next/modules/dict/service"
)

func InfoController(infoService *service.DictInfoService, dictInfoModel entity.Definition) controller.Definition {
	return controller.Admin("dict/info").
		Name("DictInfoEntity").
		Description("字典信息").
		Model(dictInfoModel).
		Service(infoService).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			PageQuery: controller.QueryOptions{
				FieldEq:           []string{"typeId"},
				KeyWordLikeFields: []string{"name"},
			},
			ListQuery: controller.QueryOptions{
				FieldEq:           []string{"typeId"},
				KeyWordLikeFields: []string{"name"},
			},
			SortFields:   []string{"id", "createTime", "updateTime", "orderNum"},
			DefaultSort:  "createTime",
			DefaultOrder: "ASC",
		}).
		Route(controller.RouteOptions{
			Name: "data", Method: http.MethodPost, Path: "/data",
			Description: "获得字典数据", Action: data(infoService),
		}).
		Route(controller.RouteOptions{
			Name: "types", Method: http.MethodGet, Path: "/types",
			Description: "获得所有字典类型", IgnoreAuth: true,
			Action: types(infoService),
		}).
		Build()
}

func types(service *service.DictInfoService) func(context.Context) ([]map[string]interface{}, error) {
	return func(ctx context.Context) ([]map[string]interface{}, error) {
		if service == nil {
			return nil, exception.Internal(nil, "字典服务不可用")
		}
		return service.GlobalTypes(ctx)
	}
}

func data(service *service.DictInfoService) func(context.Context, *dto.DataReq) (map[string]interface{}, error) {
	return func(ctx context.Context, request *dto.DataReq) (map[string]interface{}, error) {
		if service == nil {
			return nil, exception.Internal(nil, "字典服务不可用")
		}
		return service.Data(ctx, request.Types)
	}
}
