package sys

import (
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func baseModelDefinitions() []entity.Definition {
	return []entity.Definition{
		baseModel.BaseSysConf(),
		baseModel.BaseSysDepartment(),
		baseModel.BaseSysLog(),
		baseModel.BaseSysMenu(),
		baseModel.BaseSysParam(),
		baseModel.BaseSysRole(),
		baseModel.BaseSysRoleDepartment(),
		baseModel.BaseSysRoleMenu(),
		baseModel.BaseSysUser(),
		baseModel.BaseSysUserRole(),
	}
}
