package admin

import (
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/modules/dict/service"
)

/**
 * 字典类型 controller 元数据
 * @param typeService 字典类型服务
 * @param dictTypeModel 字典类型模型
 * @returns controller.Definition
 */
func TypeController(typeService *service.DictTypeService, dictTypeModel entity.Definition) controller.Definition {
	return controller.Admin("dict/type").
		Name("DictTypeEntity").
		Description("字典类型").
		Model(dictTypeModel).
		Service(typeService).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"name"},
			},
			ListQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"name"},
			},
			SortFields:   []string{"id", "createTime", "updateTime"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Build()
}
