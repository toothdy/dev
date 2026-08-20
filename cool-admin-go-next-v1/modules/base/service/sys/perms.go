package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

// base 权限服务
type PermissionService struct {
	DB permissionDatabase
}

// 权限菜单响应 data(已移至 base/dto/perms.go)

/**
 * 创建权限服务
 * @param db 数据库实例
 * @returns *PermissionService
 */
func NewPermissionService(db gdb.DB) *PermissionService {
	return newPermissionService(db)
}

func newPermissionService(db permissionDatabase) *PermissionService {
	return &PermissionService{DB: db}
}

/**
 * 拆分权限字符串
 * @param value 权限字符串
 * @returns []string
 */
func SplitPerms(value string) []string {
	perms := make([]string, 0, strings.Count(value, ",")+1)
	for _, part := range strings.Split(value, ",") {
		item := strings.TrimSpace(part)
		if item != "" {
			perms = append(perms, item)
		}
	}
	return perms
}

/**
 * 字符串去重并保持顺序
 * @param items 字符串列表
 * @returns []string
 */
func UniqueStrings(items []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

/**
 * 当前用户是否超管
 * @param ctx 上下文
 * @param user 当前用户
 * @returns bool
 */
func (s *PermissionService) IsAdmin(ctx context.Context, user security.UserContext) (bool, error) {
	return isPlatformAdministrator(ctx, s.DB, user.UserId)
}

/**
 * 查询权限菜单
 * @param ctx 上下文
 * @param user 当前用户
 * @returns PermMenuResult
 */
func (s *PermissionService) PermMenu(ctx context.Context, user security.UserContext) (dto.PermMenuResult, error) {
	if err := requireRelationScope(ctx); err != nil {
		return dto.PermMenuResult{}, err
	}
	isAdmin, err := s.IsAdmin(ctx, user)
	if err != nil {
		return dto.PermMenuResult{}, err
	}
	return s.permMenu(ctx, user, isAdmin)
}

func (s *PermissionService) permMenu(ctx context.Context, user security.UserContext, isAdmin bool) (dto.PermMenuResult, error) {
	var err error
	var rows gdb.Result
	if isAdmin {
		rows, err = s.allMenuRows(ctx)
	} else {
		rows, err = s.userMenuRows(ctx, user.UserId)
	}
	if err != nil {
		return dto.PermMenuResult{}, err
	}

	menus := make([]map[string]interface{}, 0, len(rows))
	perms := make([]string, 0)
	for _, row := range rows {
		values := recordValues(row)
		perms = append(perms, SplitPerms(stringValue(values["perms"]))...)
		menus = append(menus, normalizeMenuResponse(values))
	}

	return dto.PermMenuResult{
		Menus: menus,
		Perms: UniqueStrings(perms),
	}, nil
}

/**
 * 是否拥有权限码
 * @param ctx 上下文
 * @param user 当前用户
 * @param permission 权限码
 * @returns bool
 */
func (s *PermissionService) HasPermission(ctx context.Context, user security.UserContext, permission string) (bool, error) {
	if permission == "" {
		return true, nil
	}
	if err := requireRelationScope(ctx); err != nil {
		return false, err
	}
	isAdmin, err := s.IsAdmin(ctx, user)
	if err != nil {
		return false, err
	}
	if isAdmin {
		return true, nil
	}
	result, err := s.permMenu(ctx, user, false)
	if err != nil {
		return false, err
	}
	for _, item := range result.Perms {
		if item == permission {
			return true, nil
		}
	}
	return false, nil
}

/**
 * 菜单行是否为可显示的前端路由
 * @param row 菜单行
 * @returns bool
 */
func MenuRowShouldEnterMenus(row map[string]interface{}) bool {
	return int64Value(row["type"]) != 2 && int64Value(row["isShow"]) == 1
}

func normalizeMenuResponse(menu map[string]interface{}) map[string]interface{} {
	menu["keepAlive"] = booleanValue(menu["keepAlive"])
	menu["isShow"] = booleanValue(menu["isShow"])
	return menu
}

func (s *PermissionService) allMenuRows(ctx context.Context) (gdb.Result, error) {
	condition, err := permissionTenantCondition(ctx, baseEntity.BaseSysMenu(), "m")
	if err != nil {
		return nil, err
	}
	query := menuSelectSQL() + " m"
	args := []interface{}{}
	if condition.SQL != "" {
		query += " WHERE " + condition.SQL
		args = append(args, condition.Args...)
	}
	result, err := s.DB.GetAll(ctx, query+" ORDER BY m.orderNum ASC, m.id ASC", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询全部菜单权限失败")
	}
	return result, nil
}

func (s *PermissionService) userMenuRows(ctx context.Context, userId int64) (gdb.Result, error) {
	menuCondition, err := permissionTenantCondition(ctx, baseEntity.BaseSysMenu(), "m")
	if err != nil {
		return nil, err
	}
	roleCondition, err := permissionTenantCondition(ctx, baseEntity.BaseSysRole(), "r")
	if err != nil {
		return nil, err
	}
	userCondition, err := permissionTenantCondition(ctx, baseEntity.BaseSysUser(), "u")
	if err != nil {
		return nil, err
	}
	roleQuery := menuSelectSQL() + " m" +
		" INNER JOIN base_sys_role_menu rm ON rm.menuId = m.id" +
		" INNER JOIN base_sys_role r ON r.id = rm.roleId" +
		" INNER JOIN base_sys_user_role ur ON ur.roleId = r.id" +
		" INNER JOIN base_sys_user u ON u.id = ur.userId" +
		" WHERE ur.userId = ?"
	roleArgs := []interface{}{userId}
	for _, condition := range []tenant.Condition{menuCondition, roleCondition, userCondition} {
		if condition.SQL != "" {
			roleQuery += " AND " + condition.SQL
			roleArgs = append(roleArgs, condition.Args...)
		}
	}
	roleRows, err := s.DB.GetAll(ctx, roleQuery+" ORDER BY m.orderNum ASC, m.id ASC", roleArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户菜单权限失败")
	}
	allQuery := menuSelectSQL() + " m"
	allArgs := []interface{}{}
	if menuCondition.SQL != "" {
		allQuery += " WHERE " + menuCondition.SQL
		allArgs = append(allArgs, menuCondition.Args...)
	}
	allRows, err := s.DB.GetAll(ctx, allQuery+" ORDER BY m.orderNum ASC, m.id ASC", allArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户菜单权限失败")
	}

	selectedIds := make(map[int64]bool, len(roleRows))
	rowsById := make(map[int64]gdb.Record, len(allRows))
	for _, row := range roleRows {
		selectedIds[int64Value(recordValue(row, "id"))] = true
	}
	for _, row := range allRows {
		rowsById[int64Value(recordValue(row, "id"))] = row
	}

	for _, row := range roleRows {
		parentId := int64Value(recordValue(row, "parentId", "parentId"))
		for parentId != 0 && !selectedIds[parentId] {
			parent, ok := rowsById[parentId]
			if !ok {
				break
			}
			selectedIds[parentId] = true
			parentId = int64Value(recordValue(parent, "parentId", "parentId"))
		}
	}

	selectedRows := make(gdb.Result, 0, len(selectedIds))
	for _, row := range allRows {
		if selectedIds[int64Value(recordValue(row, "id"))] {
			selectedRows = append(selectedRows, row)
		}
	}
	return selectedRows, nil
}

func menuSelectSQL() string {
	return "SELECT DISTINCT m.id, m.parentId AS parentId, m.name, m.router, m.perms, m.type, m.icon, m.orderNum AS orderNum, m.viewPath AS viewPath, m.keepAlive AS keepAlive, m.isShow AS isShow, m.createTime AS createTime, m.updateTime AS updateTime FROM base_sys_menu"
}

// 返回权限资源的租户条件
func permissionTenantCondition(ctx context.Context, definition entity.Definition, alias string) (tenant.Condition, error) {
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		return tenant.Condition{}, err
	}
	return tenant.Predicate(ctx, metadata, alias)
}
