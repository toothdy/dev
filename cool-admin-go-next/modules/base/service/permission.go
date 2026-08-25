package service

import (
	"context"
	"sort"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
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

type menuIDRow struct {
	MenuID uint64 `orm:"menuId"`
}

type menuPermissionRow struct {
	Perms *string `orm:"perms"`
}

// Base 权威关系表执行后台授权
type PermissionService struct {
	userRole    *coreservice.Base[entity.UserRole, uint64]
	role        *coreservice.Base[entity.Role, uint64]
	roleMenu    *coreservice.Base[entity.RoleMenu, uint64]
	menu        *coreservice.Base[entity.Menu, uint64]
	menuService *MenuService
}

// 后台权限服务
func NewPermission(
	userRole *coreservice.Base[entity.UserRole, uint64],
	role *coreservice.Base[entity.Role, uint64],
	roleMenu *coreservice.Base[entity.RoleMenu, uint64],
	menu *coreservice.Base[entity.Menu, uint64],
	menuService *MenuService,
) (*PermissionService, error) {
	if !validPermissionBase(userRole) || !validPermissionBase(role) ||
		!validPermissionBase(roleMenu) || !validPermissionBase(menu) || menuService == nil {
		return nil, exception.Core("权限服务依赖无效")
	}

	return &PermissionService{userRole: userRole, role: role, roleMenu: roleMenu, menu: menu, menuService: menuService}, nil
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
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []roleIDRow
	if err = model.Fields("roleId").Where("userId", userID).OrderAsc("roleId").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询用户角色失败")
	}
	roleIDs := make([]uint64, len(rows))
	for index, row := range rows {
		roleIDs[index] = row.RoleID
	}

	return roleIDs, nil
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
