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
	"github.com/toothdy/cool-admin-go-next/cool-next/db"
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
	runtime *db.Runtime,
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
func (s *PermissionService) Authorize(ctx context.Context, req auth.Authorization) (bool, error) {
	if req.Subject != auth.AdminKind {
		return false, nil
	}
	if req.SubjectID == 0 || strings.TrimSpace(req.Permission) == "" {
		return false, exception.Core("权限请求无效")
	}

	roleIDs, err := s.RoleIDs(ctx, req.SubjectID)
	if err != nil || len(roleIDs) == 0 {
		return false, err
	}
	isAdmin, err := s.IsAdmin(ctx, roleIDs)
	if err != nil || isAdmin {
		return isAdmin, err
	}
	perms, err := s.permissions(ctx, roleIDs, false)
	if err != nil {
		return false, err
	}
	_, allowed := perms[strings.TrimSpace(req.Permission)]

	return allowed, nil
}

// 用户当前关联的角色 ID
func (s *PermissionService) RoleIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	if userID == 0 {
		return nil, exception.Core("角色查询参数无效")
	}
	roles, err := s.RolesByUsers(ctx, []uint64{userID})
	if err != nil {
		return nil, err
	}

	return roles[userID].IDs, nil
}

// 批量查询用户角色，一次返回稳定 ID 和名称顺序
func (s *PermissionService) RolesByUsers(ctx context.Context, userIDs []uint64) (map[uint64]UserRoles, error) {
	ids := auth.NormalizeIDs(userIDs)
	result := make(map[uint64]UserRoles, len(ids))
	for _, userID := range ids {
		result[userID] = UserRoles{IDs: []uint64{}, Names: []string{}}
	}
	if len(ids) == 0 {
		return result, nil
	}
	model, err := s.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []userRoleDetailRow
	if err = model.As("ur").
		InnerJoin(s.role.Descriptor().Table(), "r", "r.id = ur.roleId").
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
func (s *PermissionService) RoleSnapshot(ctx context.Context, userIDs []uint64) (map[uint64][]uint64, error) {
	roles, err := s.RolesByUsers(ctx, userIDs)
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
func (s *PermissionService) ValidateRoleSnapshot(before, after map[uint64][]uint64) error {
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
func (s *PermissionService) PrepareRoleChange(ctx context.Context, userIDs, nextRoleIDs []uint64) (map[uint64][]uint64, error) {
	users := auth.NormalizeIDs(userIDs)
	next := auth.NormalizeIDs(nextRoleIDs)
	adminRoles, err := s.AdminRoleIDs(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.LockRoles(ctx, adminRoles); err != nil {
		return nil, err
	}
	before, err := s.RoleSnapshot(ctx, users)
	if err != nil {
		return nil, err
	}
	involved := append([]uint64(nil), next...)
	for _, roleIDs := range before {
		involved = append(involved, roleIDs...)
	}
	involved = auth.NormalizeIDs(involved)
	involved = slices.DeleteFunc(involved, func(roleID uint64) bool {
		return slices.Contains(adminRoles, roleID)
	})
	if err = s.LockRoles(ctx, involved); err != nil {
		return nil, err
	}

	return before, nil
}

// 按稳定 ID 顺序锁定并校验角色存在
func (s *PermissionService) LockRoles(ctx context.Context, roleIDs []uint64) error {
	return s.boundary.LockTable(ctx, s.role.Descriptor().Table(), roleIDs, "锁定授权角色失败")
}

// 按稳定 ID 顺序锁定并校验用户存在
func (s *PermissionService) LockUsers(ctx context.Context, userIDs []uint64) error {
	return s.boundary.LockTable(ctx, s.user.Descriptor().Table(), userIDs, "锁定授权用户失败")
}

// 替换单个用户的全部角色关系
func (s *PermissionService) ReplaceRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	if userID == 0 {
		return exception.Core("用户角色替换参数无效")
	}
	model, err := s.userRole.Model(ctx)
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
	for i, roleID := range ids {
		rows[i] = userRoleWrite{UserID: userID, RoleID: roleID}
	}
	if _, err = model.Data(rows).Insert(); err != nil {
		return exception.WrapCore(err, "写入用户角色关系失败")
	}

	return nil
}

// 删除多个用户的角色关系
func (s *PermissionService) DeleteUserRoles(ctx context.Context, userIDs []uint64) error {
	ids := auth.NormalizeIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	model, err := s.userRole.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.WhereIn("userId", ids).Delete(); err != nil {
		return exception.WrapCore(err, "清理用户角色关系失败")
	}

	return nil
}

// 平台管理员角色 ID
func (s *PermissionService) AdminRoleIDs(ctx context.Context) ([]uint64, error) {
	model, err := s.role.Model(ctx)
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
	for i, row := range rows {
		ids[i] = row.ID
	}

	return ids, nil
}

// 删除、禁用或移除管理员角色前保护最后一个有效管理员
func (s *PermissionService) EnsureAdminTransition(
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
	wasAdmin, err := s.IsAdmin(ctx, currentRoleIDs)
	if err != nil || !wasAdmin {
		return err
	}
	if nextStatus == 1 {
		staysAdmin, checkErr := s.IsAdmin(ctx, nextRoleIDs)
		if checkErr != nil || staysAdmin {
			return checkErr
		}
	}

	return s.EnsureNotLastAdmin(ctx, []uint64{userID})
}

// 删除或禁用用户前保护最后一个有效管理员
func (s *PermissionService) EnsureNotLastAdmin(ctx context.Context, userIDs []uint64) error {
	ids := auth.NormalizeIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	userModel, err := s.user.Model(ctx)
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
	for i, row := range enabled {
		enabledIDs[i] = row.ID
	}
	roles, err := s.RoleSnapshot(ctx, enabledIDs)
	if err != nil {
		return err
	}
	affectedRoles := make([]uint64, 0)
	for _, roleIDs := range roles {
		affectedRoles = append(affectedRoles, roleIDs...)
	}
	isAdmin, err := s.IsAdmin(ctx, affectedRoles)
	if err != nil || !isAdmin {
		return err
	}
	adminRoles, err := s.AdminRoleIDs(ctx)
	if err != nil {
		return err
	}
	relModel, err := s.userRole.Model(ctx)
	if err != nil {
		return err
	}
	var rows []struct {
		UserID uint64 `orm:"userId"`
	}
	if err = relModel.Fields("userId").WhereIn("roleId", adminRoles).WhereNotIn("userId", ids).Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询管理员关系失败")
	}
	remainingIDs := make([]uint64, len(rows))
	for i, row := range rows {
		remainingIDs[i] = row.UserID
	}
	remainingIDs = auth.NormalizeIDs(remainingIDs)
	if len(remainingIDs) == 0 {
		return exception.Comm("不能删除或禁用最后一个管理员")
	}
	remainingModel, err := s.user.Model(ctx)
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
func (s *PermissionService) RevokeUsers(ctx context.Context, userIDs []uint64) error {
	return s.boundary.RevokeUsers(ctx, auth.AdminKind, userIDs)
}

// 角色集合是否包含平台管理员角色
func (s *PermissionService) IsAdmin(ctx context.Context, roleIDs []uint64) (bool, error) {
	if len(roleIDs) == 0 {
		return false, nil
	}
	model, err := s.role.Model(ctx)
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
func (s *PermissionService) permissions(ctx context.Context, roleIDs []uint64, isAdmin bool) (map[string]struct{}, error) {
	perms := make(map[string]struct{})
	if len(roleIDs) == 0 {
		return perms, nil
	}
	var menuIDs []uint64
	if !isAdmin {
		roleMenus, err := s.roleMenu.Model(ctx)
		if err != nil {
			return nil, err
		}
		var menuRows []menuIDRow
		if err = roleMenus.Fields("menuId").WhereIn("roleId", roleIDs).Scan(&menuRows); err != nil {
			return nil, exception.WrapCore(err, "查询角色菜单失败")
		}
		if len(menuRows) == 0 {
			return perms, nil
		}
		menuIDs = make([]uint64, len(menuRows))
		for i, row := range menuRows {
			menuIDs[i] = row.MenuID
		}
	}
	menuModel, err := s.menu.Model(ctx)
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
				perms[permission] = struct{}{}
			}
		}
	}

	return perms, nil
}

// 当前管理员的权限标识与可见菜单列表
func (s *PermissionService) PermissionMenu(ctx context.Context) (dto.PermissionMenuResult, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	roleIDs := identity.RoleIDs()
	isAdmin, err := s.IsAdmin(ctx, roleIDs)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	rows, err := s.visibleMenus(ctx, roleIDs, isAdmin)
	if err != nil {
		return dto.PermissionMenuResult{}, err
	}
	set := make(map[string]struct{})
	menus := make([]dto.MenuListItem, len(rows))
	for i, row := range rows {
		menus[i] = menuItem(row)
		if row.Perms == nil {
			continue
		}
		for _, permission := range strings.Split(*row.Perms, ",") {
			if permission = strings.TrimSpace(permission); permission != "" {
				set[permission] = struct{}{}
			}
		}
	}
	perms := make([]string, 0, len(set))
	for permission := range set {
		perms = append(perms, permission)
	}
	sort.Strings(perms)

	return dto.PermissionMenuResult{Perms: perms, Menus: menus}, nil
}

func (s *PermissionService) visibleMenus(ctx context.Context, roleIDs []uint64, isAdmin bool) ([]menuRow, error) {
	if !isAdmin && len(roleIDs) == 0 {
		return []menuRow{}, nil
	}
	model, err := s.menu.Model(ctx)
	if err != nil {
		return nil, err
	}
	model = model.As("m")
	if !isAdmin {
		model = model.
			InnerJoin(s.menuService.roleMenu.Descriptor().Table(), "rm", "rm.menuId = m.id").
			WhereIn("rm.roleId", roleIDs)
	}
	var rows []menuRow
	if err = model.Fields("m.*").OrderAsc("m.orderNum").OrderAsc("m.id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询菜单列表失败")
	}
	unique := make([]menuRow, 0, len(rows))
	seen := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		if _, exists := seen[row.ID]; exists {
			continue
		}
		seen[row.ID] = struct{}{}
		unique = append(unique, row)
	}

	return unique, nil
}

func validPermissionBase[E any](base *coreservice.Base[E, uint64]) bool {
	return base != nil && base.Descriptor() != nil
}

var _ auth.Authorizer = (*PermissionService)(nil)
