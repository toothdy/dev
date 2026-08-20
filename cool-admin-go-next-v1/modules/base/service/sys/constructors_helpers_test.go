package sys

import (
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func newTestUserService(db gdb.DB, baseSysUserModel entity.Definition) *UserService {
	return newTestUserServiceWithSessions(db, baseSysUserModel, nil)
}

func newTestUserServiceWithSessions(db gdb.DB, baseSysUserModel entity.Definition, sessions security.SessionStore) *UserService {
	return NewUserService(db, baseSysUserModel, baseModel.BaseSysUserRole(), sessions, nil)
}

func newTestRoleService(db gdb.DB, baseSysRoleModel entity.Definition) *RoleService {
	return newTestRoleServiceWithSessions(db, baseSysRoleModel, nil)
}

func newTestRoleServiceWithSessions(db gdb.DB, baseSysRoleModel entity.Definition, sessions security.SessionStore) *RoleService {
	return NewRoleService(
		db,
		baseSysRoleModel,
		baseModel.BaseSysUserRole(),
		baseModel.BaseSysRoleMenu(),
		baseModel.BaseSysRoleDepartment(),
		sessions,
		nil,
	)
}

func newTestDepartmentService(db gdb.DB, baseSysDepartmentModel entity.Definition) *DepartmentService {
	return newTestDepartmentServiceWithSessions(db, baseSysDepartmentModel, nil)
}

func newTestDepartmentServiceWithSessions(db gdb.DB, baseSysDepartmentModel entity.Definition, sessions security.SessionStore) *DepartmentService {
	return NewDepartmentService(
		db,
		baseSysDepartmentModel,
		baseModel.BaseSysUser(),
		baseModel.BaseSysUserRole(),
		baseModel.BaseSysRoleDepartment(),
		sessions,
		nil,
	)
}

func newTestMenuService(db gdb.DB, baseSysMenuModel entity.Definition) *MenuService {
	return NewMenuService(db, baseSysMenuModel, baseModel.BaseSysRole(), baseModel.BaseSysRoleMenu(), nil)
}

func newTestParamService(db gdb.DB, baseSysParamModel entity.Definition) *ParamService {
	return NewParamService(db, baseSysParamModel, nil)
}

func newTestLogService(db gdb.DB, baseSysLogModel entity.Definition) *LogService {
	return NewLogService(db, baseSysLogModel, baseModel.BaseSysUser(), baseModel.BaseSysConf())
}
