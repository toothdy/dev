package service

import (
	"context"
	"slices"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

// AuthorizationBoundary 统一授权关系变更的数据库锁和 Session 撤销顺序。
type AuthorizationBoundary struct {
	runtime    *coredb.Runtime
	user       *coreservice.Base[entity.User, uint64]
	role       *coreservice.Base[entity.Role, uint64]
	menu       *coreservice.Base[entity.Menu, uint64]
	department *coreservice.Base[entity.Department, uint64]
	sessions   auth.SessionStore
}

// NewAuthorizationBoundary 创建授权变更边界。
func NewAuthorizationBoundary(
	runtime *coredb.Runtime,
	user *coreservice.Base[entity.User, uint64],
	role *coreservice.Base[entity.Role, uint64],
	menu *coreservice.Base[entity.Menu, uint64],
	department *coreservice.Base[entity.Department, uint64],
	sessions auth.SessionStore,
) (*AuthorizationBoundary, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(user) ||
		!validPermissionBase(role) || !validPermissionBase(menu) ||
		!validPermissionBase(department) || sessions == nil {
		return nil, exception.Core("授权变更边界依赖无效")
	}

	return &AuthorizationBoundary{
		runtime:    runtime,
		user:       user,
		role:       role,
		menu:       menu,
		department: department,
		sessions:   sessions,
	}, nil
}

// LockRoles 按 ID 升序锁定角色记录。
func (boundary *AuthorizationBoundary) LockRoles(ctx context.Context, roleIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil || boundary.role == nil {
		return exception.Core("授权变更边界未初始化")
	}

	return lockAuthorizationTable(
		ctx,
		boundary.runtime,
		boundary.role.Descriptor().Table(),
		roleIDs,
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

	return lockAuthorizationTable(
		ctx,
		boundary.runtime,
		boundary.menu.Descriptor().Table(),
		menuIDs,
		"锁定授权菜单失败",
	)
}

// LockDepartments 按 ID 升序锁定部门记录。
func (boundary *AuthorizationBoundary) LockDepartments(ctx context.Context, departmentIDs []uint64) error {
	if boundary == nil || boundary.runtime == nil || boundary.department == nil {
		return exception.Core("授权变更边界未初始化")
	}

	return lockAuthorizationTable(
		ctx,
		boundary.runtime,
		boundary.department.Descriptor().Table(),
		departmentIDs,
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
	if err := lockAuthorizationTable(
		ctx,
		boundary.runtime,
		boundary.user.Descriptor().Table(),
		userIDs,
		"锁定授权用户失败",
	); err != nil {
		return err
	}
	if err := boundary.sessions.RevokeUsers(ctx, auth.AdminKind, userIDs); err != nil {
		return exception.WrapCore(err, "撤销用户 Session 失败")
	}

	return nil
}

// lockAuthorizationTable 锁定目标表记录并校验请求的记录全部存在。
func lockAuthorizationTable(
	ctx context.Context,
	runtime *coredb.Runtime,
	table string,
	ids []uint64,
	message string,
) error {
	ids = normalizeAuthorizationIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	locked, err := runtime.LockRows(ctx, table, ids)
	if err != nil {
		return exception.WrapCore(err, message)
	}
	if !slices.Equal(ids, locked) {
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
