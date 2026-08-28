package service

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

const adminRoleLabel = "admin"

// 授权变更加锁用的表名。只在没有持有对应实体 Base 引用、拿不到
// Descriptor().Table() 的 service 里使用；能拿到引用的地方一律用 Descriptor
const (
	userTable       = "base_sys_user"
	menuTable       = "base_sys_menu"
	departmentTable = "base_sys_department"
)

type roleIDRow struct {
	RoleID uint64 `orm:"roleId"`
}

type roleLabelRow struct {
	Label *string `orm:"label"`
}

type userRoleDetailRow struct {
	UserID uint64 `orm:"userId"`
	RoleID uint64 `orm:"roleId"`
	Name   string `orm:"name"`
}

type userRoleWrite struct {
	g.Meta `orm:"do:true"`
	UserID any `orm:"userId"`
	RoleID any `orm:"roleId"`
}

// 用户的角色 ID 和名称
type UserRoles struct {
	IDs   []uint64
	Names []string
}

type menuIDRow struct {
	MenuID uint64 `orm:"menuId"`
}

type menuPermissionRow struct {
	Perms *string `orm:"perms"`
}

// Base 权威关系表执行后台授权
type PermissionService struct {
	user        *coreservice.Base[entity.User, uint64]
	userRole    *coreservice.Base[entity.UserRole, uint64]
	role        *coreservice.Base[entity.Role, uint64]
	roleMenu    *coreservice.Base[entity.RoleMenu, uint64]
	menu        *coreservice.Base[entity.Menu, uint64]
	menuService *MenuService
	boundary    *auth.Boundary
}

// 后台权限服务
func NewPermission(
	runtime *coredb.Runtime,
	user *coreservice.Base[entity.User, uint64],
	userRole *coreservice.Base[entity.UserRole, uint64],
	role *coreservice.Base[entity.Role, uint64],
	roleMenu *coreservice.Base[entity.RoleMenu, uint64],
	menu *coreservice.Base[entity.Menu, uint64],
	menuService *MenuService,
	sessions auth.Store,
) (*PermissionService, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(user) ||
		!validPermissionBase(userRole) || !validPermissionBase(role) ||
		!validPermissionBase(roleMenu) || !validPermissionBase(menu) || menuService == nil {
		return nil, exception.Core("权限服务依赖无效")
	}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		return nil, err
	}

	return &PermissionService{
		user: user, userRole: userRole, role: role, roleMenu: roleMenu,
		menu: menu, menuService: menuService, boundary: boundary,
	}, nil
}

// 按权威角色和菜单关系判断后台权限
func (service *PermissionService) Authorize(ctx context.Context, request auth.Authorization) (bool, error) {
	if service == nil || service.userRole == nil || service.role == nil || service.roleMenu == nil || service.menu == nil {
		return false, exception.Core("权限服务未初始化")
	}
	if request.Subject != auth.AdminKind {
		return false, nil
	}
	if request.SubjectID == 0 || strings.TrimSpace(request.Permission) == "" {
		return false, exception.Core("权限请求无效")
	}

	roleIDs, err := service.RoleIDs(ctx, request.SubjectID)
	if err != nil || len(roleIDs) == 0 {
		return false, err
	}
	isAdmin, err := service.IsAdmin(ctx, roleIDs)
	if err != nil || isAdmin {
		return isAdmin, err
	}
	permissions, err := service.permissions(ctx, roleIDs, false)
	if err != nil {
		return false, err
	}
	_, allowed := permissions[strings.TrimSpace(request.Permission)]

	return allowed, nil
}

// 用户当前关联的角色 ID
func (service *PermissionService) RoleIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if service == nil || service.userRole == nil || userID == 0 {
		return nil, exception.Core("角色查询参数无效")
	}
	roles, err := service.RolesByUsers(ctx, []uint64{userID})
	if err != nil {
		return nil, err
	}

	return roles[userID].IDs, nil
}

// 批量查询用户角色，一次返回稳定 ID 和名称顺序
func (service *PermissionService) RolesByUsers(ctx context.Context, userIDs []uint64) (map[uint64]UserRoles, error) {
	if service == nil || service.userRole == nil || service.role == nil {
		return nil, exception.Core("角色查询服务未初始化")
	}
	ids := auth.NormalizeIDs(userIDs)
	result := make(map[uint64]UserRoles, len(ids))
	for _, userID := range ids {
		result[userID] = UserRoles{IDs: []uint64{}, Names: []string{}}
	}
	if len(ids) == 0 {
		return result, nil
	}
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []userRoleDetailRow
	if err = model.As("ur").
		InnerJoin(service.role.Descriptor().Table(), "r", "r.id = ur.roleId").
		Fields("ur.userId", "ur.roleId", "r.name").
		WhereIn("ur.userId", ids).
		OrderAsc("ur.userId").OrderAsc("ur.roleId").
		Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "批量查询用户角色失败")
	}
	for _, row := range rows {
		current := result[row.UserID]
		current.IDs = append(current.IDs, row.RoleID)
		current.Names = append(current.Names, row.Name)
		result[row.UserID] = current
	}

	return result, nil
}

// 批量读取用户角色关系快照
func (service *PermissionService) RoleSnapshot(ctx context.Context, userIDs []uint64) (map[uint64][]uint64, error) {
	roles, err := service.RolesByUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[uint64][]uint64, len(roles))
	for userID, current := range roles {
		result[userID] = append([]uint64(nil), current.IDs...)
	}

	return result, nil
}

// 校验锁前后的用户角色关系未变化
func (service *PermissionService) ValidateRoleSnapshot(before, after map[uint64][]uint64) error {
	if len(before) != len(after) {
		return exception.Comm("用户角色已变更，请重试")
	}
	for userID, roleIDs := range before {
		if !slices.Equal(auth.NormalizeIDs(roleIDs), auth.NormalizeIDs(after[userID])) {
			return exception.Comm("用户角色已变更，请重试")
		}
	}

	return nil
}

// 锁定授权变更涉及的角色并返回用户角色关系快照
func (service *PermissionService) PrepareRoleChange(ctx context.Context, userIDs, nextRoleIDs []uint64) (map[uint64][]uint64, error) {
	if service == nil || service.boundary == nil {
		return nil, exception.Core("权限服务未初始化")
	}
	users := auth.NormalizeIDs(userIDs)
	next := auth.NormalizeIDs(nextRoleIDs)
	adminRoleIDs, err := service.AdminRoleIDs(ctx)
	if err != nil {
		return nil, err
	}
	if err = service.LockRoles(ctx, adminRoleIDs); err != nil {
		return nil, err
	}
	before, err := service.RoleSnapshot(ctx, users)
	if err != nil {
		return nil, err
	}
	involved := append([]uint64(nil), next...)
	for _, roleIDs := range before {
		involved = append(involved, roleIDs...)
	}
	involved = auth.NormalizeIDs(involved)
	involved = slices.DeleteFunc(involved, func(roleID uint64) bool {
		return slices.Contains(adminRoleIDs, roleID)
	})
	if err = service.LockRoles(ctx, involved); err != nil {
		return nil, err
	}

	return before, nil
}

// 按角色、用户顺序锁定授权变更并校验关系快照
func (service *PermissionService) LockUserRoleChanges(ctx context.Context, userIDs, nextRoleIDs []uint64) error {
	users := auth.NormalizeIDs(userIDs)
	before, err := service.PrepareRoleChange(ctx, users, nextRoleIDs)
	if err != nil {
		return err
	}
	if err = service.LockUsers(ctx, users); err != nil {
		return err
	}
	after, err := service.RoleSnapshot(ctx, users)
	if err != nil {
		return err
	}

	return service.ValidateRoleSnapshot(before, after)
}

// 按稳定 ID 顺序锁定并校验角色存在
func (service *PermissionService) LockRoles(ctx context.Context, roleIDs []uint64) error {
	if service == nil || service.boundary == nil || service.role == nil {
		return exception.Core("权限服务未初始化")
	}

	return service.boundary.LockTable(ctx, service.role.Descriptor().Table(), roleIDs, "锁定授权角色失败")
}

// 按稳定 ID 顺序锁定并校验用户存在
func (service *PermissionService) LockUsers(ctx context.Context, userIDs []uint64) error {
	if service == nil || service.boundary == nil || service.user == nil {
		return exception.Core("权限服务未初始化")
	}

	return service.boundary.LockTable(ctx, service.user.Descriptor().Table(), userIDs, "锁定授权用户失败")
}

// 替换单个用户的全部角色关系
func (service *PermissionService) ReplaceRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	if service == nil || service.userRole == nil || userID == 0 {
		return exception.Core("用户角色替换参数无效")
	}
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.Where("userId", userID).Delete(); err != nil {
		return exception.WrapCore(err, "删除旧用户角色关系失败")
	}
	ids := auth.NormalizeIDs(roleIDs)
	if len(ids) == 0 {
		return nil
	}
	rows := make([]userRoleWrite, len(ids))
	for index, roleID := range ids {
		rows[index] = userRoleWrite{UserID: userID, RoleID: roleID}
	}
	if _, err = model.Data(rows).Insert(); err != nil {
		return exception.WrapCore(err, "写入用户角色关系失败")
	}

	return nil
}

// 删除多个用户的角色关系
func (service *PermissionService) DeleteUserRoles(ctx context.Context, userIDs []uint64) error {
	ids := auth.NormalizeIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.WhereIn("userId", ids).Delete(); err != nil {
		return exception.WrapCore(err, "清理用户角色关系失败")
	}

	return nil
}

// 平台管理员角色 ID
func (service *PermissionService) AdminRoleIDs(ctx context.Context) ([]uint64, error) {
	if service == nil || service.role == nil {
		return nil, exception.Core("角色服务未初始化")
	}
	model, err := service.role.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").Where("label", adminRoleLabel).OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询管理员角色失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}

	return ids, nil
}

// 删除、禁用或移除管理员角色前保护最后一个有效管理员
func (service *PermissionService) EnsureAdminTransition(
	ctx context.Context,
	userID uint64,
	currentStatus int32,
	nextStatus int32,
	currentRoleIDs []uint64,
	nextRoleIDs []uint64,
) error {
	if currentStatus != 1 {
		return nil
	}
	currentIsAdmin, err := service.IsAdmin(ctx, currentRoleIDs)
	if err != nil || !currentIsAdmin {
		return err
	}
	if nextStatus == 1 {
		nextIsAdmin, checkErr := service.IsAdmin(ctx, nextRoleIDs)
		if checkErr != nil || nextIsAdmin {
			return checkErr
		}
	}

	return service.EnsureNotLastAdmin(ctx, []uint64{userID})
}

// 删除或禁用用户前保护最后一个有效管理员
func (service *PermissionService) EnsureNotLastAdmin(ctx context.Context, userIDs []uint64) error {
	if service == nil || service.user == nil || service.userRole == nil {
		return exception.Core("权限服务未初始化")
	}
	ids := auth.NormalizeIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	userModel, err := service.user.Model(ctx)
	if err != nil {
		return err
	}
	var enabled []struct {
		ID uint64 `orm:"id"`
	}
	if err = userModel.Fields("id").WhereIn("id", ids).Where("status", 1).Scan(&enabled); err != nil {
		return exception.WrapCore(err, "查询管理员状态失败")
	}
	enabledIDs := make([]uint64, len(enabled))
	for index, row := range enabled {
		enabledIDs[index] = row.ID
	}
	roles, err := service.RoleSnapshot(ctx, enabledIDs)
	if err != nil {
		return err
	}
	affectedRoleIDs := make([]uint64, 0)
	for _, roleIDs := range roles {
		affectedRoleIDs = append(affectedRoleIDs, roleIDs...)
	}
	isAdmin, err := service.IsAdmin(ctx, affectedRoleIDs)
	if err != nil || !isAdmin {
		return err
	}
	adminRoleIDs, err := service.AdminRoleIDs(ctx)
	if err != nil {
		return err
	}
	relationModel, err := service.userRole.Model(ctx)
	if err != nil {
		return err
	}
	var rows []struct {
		UserID uint64 `orm:"userId"`
	}
	if err = relationModel.Fields("userId").WhereIn("roleId", adminRoleIDs).WhereNotIn("userId", ids).Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询管理员关系失败")
	}
	remainingIDs := make([]uint64, len(rows))
	for index, row := range rows {
		remainingIDs[index] = row.UserID
	}
	remainingIDs = auth.NormalizeIDs(remainingIDs)
	if len(remainingIDs) == 0 {
		return exception.Comm("不能删除或禁用最后一个管理员")
	}
	remainingModel, err := service.user.Model(ctx)
	if err != nil {
		return err
	}
	remaining, err := remainingModel.WhereIn("id", remainingIDs).Where("status", 1).Exist()
	if err != nil {
		return exception.WrapCore(err, "查询有效管理员失败")
	}
	if !remaining {
		return exception.Comm("不能删除或禁用最后一个管理员")
	}

	return nil
}

// 撤销用户后台 Session
func (service *PermissionService) RevokeUsers(ctx context.Context, userIDs []uint64) error {
	if service == nil || service.boundary == nil {
		return exception.Core("权限服务未初始化")
	}

	return service.boundary.RevokeUsers(ctx, auth.AdminKind, userIDs)
}

// 角色集合是否包含平台管理员角色
func (service *PermissionService) IsAdmin(ctx context.Context, roleIDs []uint64) (bool, error) {
	if service == nil || service.role == nil {
		return false, exception.Core("角色服务未初始化")
	}
	if len(roleIDs) == 0 {
		return false, nil
	}
	model, err := service.role.Model(ctx)
	if err != nil {
		return false, err
	}
	var rows []roleLabelRow
	if err = model.Fields("label").WhereIn("id", roleIDs).Scan(&rows); err != nil {
		return false, exception.WrapCore(err, "查询角色标签失败")
	}
	for _, row := range rows {
		if row.Label != nil && *row.Label == adminRoleLabel {
			return true, nil
		}
	}

	return false, nil
}

// 角色集合当前关联的独立权限标识
func (service *PermissionService) permissions(ctx context.Context, roleIDs []uint64, isAdmin bool) (map[string]struct{}, error) {
	permissions := make(map[string]struct{})
	if service == nil || service.roleMenu == nil || service.menu == nil {
		return nil, exception.Core("菜单权限服务未初始化")
	}
	if len(roleIDs) == 0 {
		return permissions, nil
	}
	var menuIDs []uint64
	if !isAdmin {
		roleMenuModel, err := service.roleMenu.Model(ctx)
		if err != nil {
			return nil, err
		}
		var menuRows []menuIDRow
		if err = roleMenuModel.Fields("menuId").WhereIn("roleId", roleIDs).Scan(&menuRows); err != nil {
			return nil, exception.WrapCore(err, "查询角色菜单失败")
		}
		if len(menuRows) == 0 {
			return permissions, nil
		}
		menuIDs = make([]uint64, len(menuRows))
		for index, row := range menuRows {
			menuIDs[index] = row.MenuID
		}
	}
	menuModel, err := service.menu.Model(ctx)
	if err != nil {
		return nil, err
	}
	query := menuModel.Fields("perms")
	if !isAdmin {
		query = query.WhereIn("id", menuIDs)
	}
	var rows []menuPermissionRow
	if err = query.Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询菜单权限失败")
	}
	for _, row := range rows {
		if row.Perms == nil {
			continue
		}
		for _, permission := range strings.Split(*row.Perms, ",") {
			if permission = strings.TrimSpace(permission); permission != "" {
				permissions[permission] = struct{}{}
			}
		}
	}

	return permissions, nil
}

// 当前管理员的权限标识与可见菜单列表
func (service *PermissionService) PermissionMenu(ctx context.Context) (dto.PermissionMenuResult, error) {
	if service == nil || service.menu == nil || service.menuService == nil {
		return dto.PermissionMenuResult{}, exception.Core("权限菜单服务未初始化")
	}
	identity, err := auth.Admin(ctx)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	roleIDs, err := service.RoleIDs(ctx, identity.UserID)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	isAdmin, err := service.IsAdmin(ctx, roleIDs)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	permissions, err := service.permissions(ctx, roleIDs, isAdmin)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	perms := make([]string, 0, len(permissions))
	for permission := range permissions {
		perms = append(perms, permission)
	}
	sort.Strings(perms)
	menus, err := service.flatVisibleMenus(ctx, roleIDs, isAdmin)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}

	return dto.PermissionMenuResult{Perms: perms, Menus: menus}, nil
}

// 当前角色可见的扁平菜单列表
func (service *PermissionService) flatVisibleMenus(ctx context.Context, roleIDs []uint64, isAdmin bool) ([]dto.MenuListItem, error) {
	if !isAdmin && len(roleIDs) == 0 {
		return []dto.MenuListItem{}, nil
	}
	model, err := service.menu.Model(ctx)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		menuIDs, idErr := service.menuService.menuIDsByRoles(ctx, roleIDs)
		if idErr != nil {
			return nil, idErr
		}
		if len(menuIDs) == 0 {
			return []dto.MenuListItem{}, nil
		}
		model = model.WhereIn("id", menuIDs)
	}
	var rows []menuRow
	if err = model.OrderAsc("orderNum").OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询菜单列表失败")
	}
	items := make([]dto.MenuListItem, len(rows))
	for index, row := range rows {
		items[index] = menuListItem(row)
	}

	return items, nil
}

func validPermissionBase[E any](service *coreservice.Base[E, uint64]) bool {
	return service != nil && service.Descriptor() != nil
}

var _ auth.Authorizer = (*PermissionService)(nil)
