package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type menuRow struct {
	ID         uint64      `orm:"id"`
	CreateTime *gtime.Time `orm:"createTime"`
	UpdateTime *gtime.Time `orm:"updateTime"`
	ParentID   *uint64     `orm:"parentId"`
	Name       string      `orm:"name"`
	Router     *string     `orm:"router"`
	Perms      *string     `orm:"perms"`
	Type       int32       `orm:"type"`
	Icon       *string     `orm:"icon"`
	OrderNum   int32       `orm:"orderNum"`
	ViewPath   *string     `orm:"viewPath"`
	KeepAlive  bool        `orm:"keepAlive"`
	IsShow     bool        `orm:"isShow"`
}

// MenuService 管理菜单树及角色菜单关系。
type MenuService struct {
	*coreservice.Base[entity.Menu, uint64]
	runtime  *coredb.Runtime
	role     *coreservice.Base[entity.Role, uint64]
	roleMenu *coreservice.Base[entity.RoleMenu, uint64]
	userRole *coreservice.Base[entity.UserRole, uint64]
	boundary *auth.Boundary
}

// NewMenu 创建菜单业务服务。
func NewMenu(
	runtime *coredb.Runtime,
	menu *coreservice.Base[entity.Menu, uint64],
	role *coreservice.Base[entity.Role, uint64],
	roleMenu *coreservice.Base[entity.RoleMenu, uint64],
	userRole *coreservice.Base[entity.UserRole, uint64],
	sessions auth.SessionStore,
) (*MenuService, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(menu) || !validPermissionBase(role) ||
		!validPermissionBase(roleMenu) || !validPermissionBase(userRole) {
		return nil, exception.Core("菜单服务依赖无效")
	}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		return nil, err
	}

	return &MenuService{Base: menu, runtime: runtime, role: role, roleMenu: roleMenu, userRole: userRole, boundary: boundary}, nil
}

// Add 新增菜单。
func (service *MenuService) Add(ctx context.Context, input coreservice.AddInput[entity.Menu]) (coreservice.AddResult[uint64], error) {
	if service == nil || service.runtime == nil {
		return coreservice.AddResult[uint64]{}, exception.Core("菜单服务未初始化")
	}
	var result coreservice.AddResult[uint64]
	err := service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		parentIDs, parentErr := menuAddParentIDs(input)
		if parentErr != nil {
			return parentErr
		}
		if parentErr = service.lockMenus(txCtx, parentIDs); parentErr != nil {
			return parentErr
		}
		result, parentErr = service.Base.Add(txCtx, input)
		return parentErr
	})
	return result, err
}

// Update 更新菜单并撤销受影响用户 Session。
func (service *MenuService) Update(ctx context.Context, input coreservice.UpdateInput[entity.Menu, uint64]) error {
	if service == nil || service.runtime == nil {
		return exception.Core("菜单服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		items := menuUpdateItems(input)
		targetIDs := make([]uint64, 0, len(items))
		lockIDs := make([]uint64, 0, len(items)*2)
		parents := make(map[uint64]*uint64, len(items))
		for _, item := range items {
			targetIDs = append(targetIDs, item.ID())
			lockIDs = append(lockIDs, item.ID())
			parentID, exists, parentErr := menuMutableParentID(item.Mutable())
			if parentErr != nil {
				return parentErr
			}
			if exists {
				parents[item.ID()] = parentID
				if parentID != nil && *parentID != 0 {
					lockIDs = append(lockIDs, *parentID)
				}
			}
		}
		if len(parents) > 0 {
			model, err := service.Base.Model(txCtx)
			if err != nil {
				return err
			}
			var rows []struct {
				ID uint64 `orm:"id"`
			}
			if err = model.Fields("id").OrderAsc("id").Scan(&rows); err != nil {
				return exception.WrapCore(err, "查询菜单树失败")
			}
			for _, row := range rows {
				lockIDs = append(lockIDs, row.ID)
			}
		}
		if err := service.lockMenus(txCtx, lockIDs); err != nil {
			return err
		}
		if err := service.validateMenuParents(txCtx, parents); err != nil {
			return err
		}
		users, err := service.userIDsByMenus(txCtx, businessUniqueIDs(targetIDs))
		if err != nil {
			return err
		}
		if err = service.boundary.LockUsersAndRevoke(txCtx, userTable, users, auth.AdminKind, "锁定授权用户失败"); err != nil {
			return err
		}
		return service.Base.Update(txCtx, input)
	})
}

// Delete 递归删除菜单和角色关系。
func (service *MenuService) Delete(ctx context.Context, input coreservice.DeleteInput[uint64]) error {
	if service == nil || service.runtime == nil {
		return exception.Core("菜单服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		ids, err := service.lockedDescendantIDs(txCtx, input.IDs())
		if err != nil {
			return err
		}
		users, err := service.userIDsByMenus(txCtx, ids)
		if err != nil {
			return err
		}
		if err = service.boundary.LockUsersAndRevoke(txCtx, userTable, users, auth.AdminKind, "锁定授权用户失败"); err != nil {
			return err
		}
		model, err := service.roleMenu.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = model.WhereIn("menuId", ids).Delete(); err != nil {
			return exception.WrapCore(err, "清理菜单角色关系失败")
		}
		deleteInput, err := coreservice.NewDeleteInput[entity.Menu, uint64](service.Descriptor(), ids)
		if err != nil {
			return err
		}
		return service.Base.Delete(txCtx, deleteInput)
	})
}

// Info 返回菜单详情。
func (service *MenuService) Info(ctx context.Context, menuID uint64) (*dto.MenuListItem, error) {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var row *menuRow
	if err = model.Where("id", menuID).Scan(&row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, exception.WrapCore(err, "查询菜单失败")
	}
	if row == nil {
		return nil, nil
	}
	item := menuListItem(*row)
	item.ParentName, err = service.parentName(ctx, row.ParentID)
	return &item, err
}

// List 返回当前用户可见的菜单树。
func (service *MenuService) List(ctx context.Context) ([]dto.MenuListItem, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return nil, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	if isAdmin, adminErr := service.isAdmin(ctx, identity.RoleIDs()); adminErr != nil {
		return nil, adminErr
	} else if !isAdmin {
		menuIDs, idErr := service.menuIDsByRoles(ctx, identity.RoleIDs())
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
	return buildMenuItems(rows), nil
}

func (service *MenuService) lockMenus(ctx context.Context, ids []uint64) error {
	ids = businessUniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	if err := service.boundary.LockTable(ctx, service.Descriptor().Table(), ids, "锁定授权菜单失败"); err != nil {
		return err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").WhereIn("id", ids).OrderAsc("id").Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询锁定菜单失败")
	}
	if len(rows) != len(ids) {
		return exception.Validate("菜单不存在")
	}
	return nil
}

func (service *MenuService) lockedDescendantIDs(ctx context.Context, roots []uint64) ([]uint64, error) {
	roots = businessUniqueIDs(roots)
	if len(roots) == 0 {
		return nil, exception.Validate("菜单 ID 不能为空")
	}
	if err := service.lockMenus(ctx, roots); err != nil {
		return nil, err
	}
	seen := make(map[uint64]struct{}, len(roots))
	for _, root := range roots {
		seen[root] = struct{}{}
	}
	level := roots
	for len(level) > 0 {
		model, err := service.Base.Model(ctx)
		if err != nil {
			return nil, err
		}
		var children []struct {
			ID uint64 `orm:"id"`
		}
		if err = model.Fields("id").WhereIn("parentId", level).OrderAsc("id").Scan(&children); err != nil {
			return nil, exception.WrapCore(err, "查询菜单子节点失败")
		}
		candidateIDs := make([]uint64, 0, len(children))
		for _, child := range children {
			if _, exists := seen[child.ID]; !exists {
				candidateIDs = append(candidateIDs, child.ID)
			}
		}
		candidateIDs = businessUniqueIDs(candidateIDs)
		if len(candidateIDs) == 0 {
			break
		}
		if err = service.lockMenus(ctx, candidateIDs); err != nil {
			return nil, err
		}
		model, err = service.Base.Model(ctx)
		if err != nil {
			return nil, err
		}
		var lockedChildren []menuRow
		if err = model.Fields("id", "parentId").WhereIn("id", candidateIDs).OrderAsc("id").Scan(&lockedChildren); err != nil {
			return nil, exception.WrapCore(err, "查询锁定菜单子节点失败")
		}
		parents := make(map[uint64]struct{}, len(level))
		for _, parentID := range level {
			parents[parentID] = struct{}{}
		}
		level = level[:0]
		for _, child := range lockedChildren {
			if child.ParentID == nil {
				continue
			}
			if _, exists := parents[*child.ParentID]; !exists {
				continue
			}
			seen[child.ID] = struct{}{}
			level = append(level, child.ID)
		}
	}
	ids := make([]uint64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return businessUniqueIDs(ids), nil
}

func (service *MenuService) validateMenuParents(ctx context.Context, changes map[uint64]*uint64) error {
	if len(changes) == 0 {
		return nil
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	var rows []menuRow
	if err = model.Fields("id", "parentId").OrderAsc("id").Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询菜单树失败")
	}
	parents := make(map[uint64]*uint64, len(rows))
	for _, row := range rows {
		parents[row.ID] = row.ParentID
	}
	for id, parentID := range changes {
		if _, exists := parents[id]; !exists {
			return exception.Validate("菜单不存在")
		}
		if parentID != nil && *parentID != 0 {
			if _, exists := parents[*parentID]; !exists {
				return exception.Validate("菜单父级不存在")
			}
		}
		parents[id] = parentID
	}
	for id := range changes {
		seen := make(map[uint64]struct{})
		current := id
		for {
			if _, exists := seen[current]; exists {
				return exception.Validate("菜单父级不能形成循环")
			}
			seen[current] = struct{}{}
			parentID := parents[current]
			if parentID == nil || *parentID == 0 {
				break
			}
			current = *parentID
		}
	}
	return nil
}

func menuAddParentIDs(input coreservice.AddInput[entity.Menu]) ([]uint64, error) {
	values := input.Many()
	if !input.IsMany() {
		values = []*coreservice.Mutable[entity.Menu]{input.One()}
	}
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		parentID, exists, err := menuMutableParentID(value)
		if err != nil {
			return nil, err
		}
		if exists && parentID != nil && *parentID != 0 {
			ids = append(ids, *parentID)
		}
	}
	return businessUniqueIDs(ids), nil
}

func menuMutableParentID(value *coreservice.Mutable[entity.Menu]) (*uint64, bool, error) {
	if value == nil || !value.Has("parentId") {
		return nil, false, nil
	}
	if value.IsNull("parentId") {
		return nil, true, nil
	}
	current, _ := value.Get("parentId")
	switch parentID := current.(type) {
	case uint64:
		return &parentID, true, nil
	case *uint64:
		return parentID, true, nil
	default:
		return nil, true, exception.Validate("菜单父级无效")
	}
}

func (service *MenuService) userIDsByMenus(ctx context.Context, menuIDs []uint64) ([]uint64, error) {
	if len(menuIDs) == 0 {
		return nil, nil
	}
	statement, err := coreservice.NativeSQL(`SELECT DISTINCT ur.userId FROM base_sys_role_menu rm INNER JOIN base_sys_user_role ur ON ur.roleId = rm.roleId WHERE rm.menuId IN (?)`, menuIDs)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		UserID uint64 `orm:"userId"`
	}
	if err = service.Base.NativeQuery(ctx, statement, &rows); err != nil {
		return nil, exception.WrapCore(err, "查询菜单用户失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.UserID
	}
	return businessUniqueIDs(ids), nil
}

func (service *MenuService) menuIDsByRoles(ctx context.Context, roleIDs []uint64) ([]uint64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	model, err := service.roleMenu.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		MenuID uint64 `orm:"menuId"`
	}
	if err = model.Fields("menuId").WhereIn("roleId", roleIDs).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色菜单失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.MenuID
	}
	return businessUniqueIDs(ids), nil
}

func (service *MenuService) isAdmin(ctx context.Context, roleIDs []uint64) (bool, error) {
	model, err := service.role.Model(ctx)
	if err != nil {
		return false, err
	}
	count, err := model.WhereIn("id", roleIDs).Where("label", adminRoleLabel).Count()
	if err != nil {
		return false, exception.WrapCore(err, "查询管理员角色失败")
	}
	return count > 0, nil
}

func (service *MenuService) parentName(ctx context.Context, parentID *uint64) (*string, error) {
	if parentID == nil || *parentID == 0 {
		return nil, nil
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var row *struct {
		Name string `orm:"name"`
	}
	if err = model.Fields("name").Where("id", *parentID).Scan(&row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, exception.WrapCore(err, "查询菜单父节点失败")
	}
	if row == nil {
		return nil, nil
	}
	return &row.Name, nil
}

func buildMenuItems(rows []menuRow) []dto.MenuListItem {
	items := make(map[uint64]dto.MenuListItem, len(rows))
	children := make(map[uint64][]uint64)
	roots := make([]uint64, 0)
	for _, row := range rows {
		items[row.ID] = menuListItem(row)
		if row.ParentID == nil || *row.ParentID == 0 {
			roots = append(roots, row.ID)
			continue
		}
		children[*row.ParentID] = append(children[*row.ParentID], row.ID)
	}
	var build func(uint64) dto.MenuListItem
	build = func(id uint64) dto.MenuListItem {
		item := items[id]
		for _, childID := range children[id] {
			item.ChildMenus = append(item.ChildMenus, build(childID))
		}
		return item
	}
	result := make([]dto.MenuListItem, 0, len(roots))
	for _, root := range roots {
		result = append(result, build(root))
	}
	return result
}

func menuListItem(row menuRow) dto.MenuListItem {
	return dto.MenuListItem{ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, ParentID: row.ParentID, Name: row.Name, Router: row.Router, Perms: row.Perms, Type: row.Type, Icon: row.Icon, OrderNum: row.OrderNum, ViewPath: row.ViewPath, KeepAlive: row.KeepAlive, IsShow: row.IsShow}
}

func menuUpdateItems(input coreservice.UpdateInput[entity.Menu, uint64]) []coreservice.UpdateItem[entity.Menu, uint64] {
	if input.IsMany() {
		return input.Many()
	}
	return []coreservice.UpdateItem[entity.Menu, uint64]{input.One()}
}
