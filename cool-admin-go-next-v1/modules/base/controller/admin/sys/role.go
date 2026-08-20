package sys

import (
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

/**
 * 系统角色 controller 元数据
 * @param roleService 角色服务
 * @param baseSysRoleModel 角色模型
 * @returns controller.Definition
 */
func RoleController(roleService *baseSysService.RoleService, baseSysRoleModel entity.Definition) controller.Definition {
	return controller.Admin("base/sys/role").
		Name("BaseSysRoleEntity").
		Description("系统角色").
		Model(baseSysRoleModel).
		Service(roleService).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Add, crud.Delete, crud.Update, crud.Info, crud.List, crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"name", "label"},
			},
			SortFields:   []string{"id", "createTime", "updateTime", "name"},
			DefaultSort:  "id",
			DefaultOrder: "DESC",
		}).
		Build()
}
