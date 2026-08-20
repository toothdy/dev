package service

import (
	"context"
	"slices"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/driver"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type authorizationLockRow struct {
	ID uint64 `orm:"id"`
}

type authorizationWriteLock struct {
	ID any `orm:"id"`
}

// AuthorizationBoundary 统一授权关系变更的数据库锁和 Session 撤销顺序。
type AuthorizationBoundary struct {
	runtime  *coredb.Runtime
	user     *coreservice.Base[entity.User, uint64]
	role     *coreservice.Base[entity.Role, uint64]
	menu     *coreservice.Base[entity.Menu, uint64]
	sessions auth.SessionStore
}

// NewAuthorizationBoundary 创建授权变更边界。
func NewAuthorizationBoundary(
	runtime *coredb.Runtime,
	user *coreservice.Base[entity.User, uint64],
	role *coreservice.Base[entity.Role, uint64],
	menu *coreservice.Base[entity.Menu, uint64],
	sessions auth.SessionStore,
) (*AuthorizationBoundary, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(user) ||
		!validPermissionBase(role) || !validPermissionBase(menu) || sessions == nil {
		return nil, exception.Core("授权变更边界依赖无效")
	}

	return &AuthorizationBoundary{runtime: runtime, user: user, role: role, menu: menu, sessions: sessions}, nil
}

// LockRoles 按 ID 升序锁定角色记录。
func (boundary *AuthorizationBoundary) LockRoles(ctx context.Context, roleIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil || boundary.role == nil {
		return exception.Core("授权变更边界未初始化")
	}

	return lockAuthorizationRows(
		ctx,
		boundary.role,
		roleIDs,
		boundary.runtime.Dialect().Kind() != driver.SQLite,
		"锁定授权角色失败",
	)
}

// LockRolePermissions 按菜单、部门顺序锁定并校验角色权限资源。
func (boundary *AuthorizationBoundary) LockRolePermissions(
	ctx context.Context,
	menuIDs []uint64,
	departmentIDs []uint64,
) error {
	if err := boundary.LockMenus(ctx, menuIDs); err != nil {
		return err
	}

	return boundary.LockDepartments(ctx, departmentIDs)
}

// LockMenus 按 ID 升序锁定菜单记录。
func (boundary *AuthorizationBoundary) LockMenus(ctx context.Context, menuIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil || boundary.menu == nil {
		return exception.Core("授权变更边界未初始化")
	}

	return lockAuthorizationRows(
		ctx,
		boundary.menu,
		menuIDs,
		boundary.runtime.Dialect().Kind() != driver.SQLite,
		"锁定授权菜单失败",
	)
}

// LockDepartments 按 ID 升序锁定部门记录。
func (boundary *AuthorizationBoundary) LockDepartments(ctx context.Context, departmentIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil {
		return exception.Core("授权变更边界未初始化")
	}
	departmentIDs = normalizeAuthorizationIDs(departmentIDs)
	if len(departmentIDs) == 0 {
		return nil
	}
	transaction, exists, err := boundary.runtime.Current(ctx)
	if err != nil {
		return err
	}
	if !exists || transaction == nil {
		return exception.Core("当前上下文不存在框架事务")
	}

	return lockAuthorizationModel(
		func() (*gdb.Model, error) {
			return transaction.Model("base_sys_department").Ctx(ctx), nil
		},
		departmentIDs,
		boundary.runtime.Dialect().Kind() != driver.SQLite,
		"锁定授权部门失败",
	)
}

// LockUsersAndRevoke 按 ID 升序锁定用户并批量撤销后台 Session。
func (boundary *AuthorizationBoundary) LockUsersAndRevoke(ctx context.Context, userIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil || boundary.user == nil || boundary.sessions == nil {
		return exception.Core("授权变更边界未初始化")
	}
	userIDs = normalizeAuthorizationIDs(userIDs)
	if len(userIDs) == 0 {
		return nil
	}
	if err := lockAuthorizationRows(
		ctx,
		boundary.user,
		userIDs,
		boundary.runtime.Dialect().Kind() != driver.SQLite,
		"锁定授权用户失败",
	); err != nil {
		return err
	}
	if err := boundary.sessions.RevokeUsers(ctx, auth.AdminKind, userIDs); err != nil {
		return exception.WrapCore(err, "撤销用户 Session 失败")
	}

	return nil
}

func lockAuthorizationRows[E any](
	ctx context.Context,
	service *coreservice.Base[E, uint64],
	ids []uint64,
	useRowLock bool,
	message string,
) error {
	ids = normalizeAuthorizationIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	if _, err := service.Tx(ctx); err != nil {
		return err
	}

	return lockAuthorizationModel(
		func() (*gdb.Model, error) {
			return service.Model(ctx)
		},
		ids,
		useRowLock,
		message,
	)
}

func lockAuthorizationModel(
	modelFactory func() (*gdb.Model, error),
	ids []uint64,
	useRowLock bool,
	message string,
) error {
	model, err := modelFactory()
	if err != nil {
		return err
	}
	if !useRowLock {
		if _, err = model.Unscoped().
			Data(authorizationWriteLock{ID: gdb.Raw("id")}).
			WhereIn("id", ids).
			Update(); err != nil {
			return exception.WrapCore(err, message)
		}
		model, err = modelFactory()
		if err != nil {
			return err
		}
	}
	model = model.Fields("id").WhereIn("id", ids).OrderAsc("id")
	if useRowLock {
		model = model.LockUpdate()
	}
	var rows []authorizationLockRow
	if err = model.Scan(&rows); err != nil {
		return exception.WrapCore(err, message)
	}
	actual := make([]uint64, len(rows))
	for index, row := range rows {
		actual[index] = row.ID
	}
	if !slices.Equal(ids, normalizeAuthorizationIDs(actual)) {
		return exception.Validate(message + ": 目标记录不存在")
	}

	return nil
}

func validateAuthorizationSnapshot(before, after []uint64, message string) error {
	if !slices.Equal(normalizeAuthorizationIDs(before), normalizeAuthorizationIDs(after)) {
		return exception.Comm(message)
	}

	return nil
}

func normalizeAuthorizationIDs(ids []uint64) []uint64 {
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			result = append(result, id)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}
