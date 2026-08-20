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

func ParamController(paramService *baseSysService.ParamService, baseSysParamModel entity.Definition) controller.Definition {
	return controller.Admin("base/sys/param").
		Name("BaseSysParamEntity").
		Description("系统参数").
		Model(baseSysParamModel).
		Service(paramService).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"keyName", "name"},
				FieldEq:           []string{"dataType"},
			},
			SortFields:   []string{"id", "createTime", "updateTime", "keyName"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Route(controller.RouteOptions{
			Name: "html", Method: http.MethodGet, Path: "/html",
			Description: "获得网页内容的参数值", Permission: "base:sys:param:html",
			Action: htmlByKey(paramService),
		}).
		Build()
}

func htmlByKey(service *baseSysService.ParamService) func(context.Context, *dto.HTMLReq) (controller.Result, error) {
	return func(ctx context.Context, request *dto.HTMLReq) (controller.Result, error) {
		if service == nil {
			return nil, exception.Internal(nil, "参数服务不可用")
		}
		html, err := service.HTMLByKey(ctx, request.Key)
		if err != nil {
			return nil, err
		}
		return controller.HTML(html), nil
	}
}
