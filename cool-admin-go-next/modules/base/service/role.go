package service

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type roleRow struct {
	ID               uint64      `orm:"id"`
	CreateTime       *gtime.Time `orm:"createTime"`
	UpdateTime       *gtime.Time `orm:"updateTime"`
	UserID           string      `orm:"userId"`
	Name             string      `orm:"name"`
	Label            *string     `orm:"label"`
	Remark           *string     `orm:"remark"`
	Relevance        bool        `orm:"relevance"`
	MenuIDList       []uint64    `orm:"menuIdList"`
	DepartmentIDList []uint64    `orm:"departmentIdList"`
}

// 角色分页响应
type RolePageResult struct {
	List       []dto.RoleInfoResult   `json:"list"`
	Pagination coreservice.Pagination `json:"pagination"`
}

// 角色及其权威菜单、部门关系
type RoleService struct {
	*coreservice.Base[entity.Role, uint64]
	runtime        *coredb.Runtime
	userRole       *coreservice.Base[entity.UserRole, uint64]
	roleMenu       *coreservice.Base[entity.RoleMenu, uint64]
	roleDepartment *coreservice.Base[entity.RoleDepartment, uint64]
	boundary       *auth.Boundary
}

// 角色业务服务
func NewRole(
	runtime *coredb.Runtime,
	role *coreservice.Base[entity.Role, uint64],
	userRole *coreservice.Base[entity.UserRole, uint64],
	roleMenu *coreservice.Base[entity.RoleMenu, uint64],
	roleDepartment *coreservice.Base[entity.RoleDepartment, uint64],
	sessions auth.SessionStore,
) (*RoleService, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(role) ||
		!validPermissionBase(userRole) || !validPermissionBase(roleMenu) ||
		!validPermissionBase(roleDepartment) {
		return nil, exception.Core("角色服务依赖无效")
	}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		return nil, err
	}

	return &RoleService{
		Base: role, runtime: runtime, userRole: userRole, roleMenu: roleMenu,
		roleDepartment: roleDepartment, boundary: boundary,
	}, nil
}

// 按菜单、部门顺序锁定并校验角色权限资源存在
func (service *RoleService) lockRolePermissions(ctx context.Context, menuIDs, departmentIDs []uint64) error {
	if err := service.boundary.LockTable(ctx, menuTable, menuIDs, "锁定授权菜单失败"); err != nil {
		return err
	}

	return service.boundary.LockTable(ctx, departmentTable, departmentIDs, "锁定授权部门失败")
}

// 新建角色并同步菜单、部门权限关系
func (service *RoleService) Add(
	ctx context.Context,
	input coreservice.AddInput[entity.Role],
) (coreservice.AddResult[uint64], error) {
	value := input.One()
	if value == nil {
		return coreservice.AddResult[uint64]{}, exception.Validate("角色新增只支持单条记录")
	}
	permissions, err := roleMutablePermissions(value)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}

	return service.AddWithPermissions(ctx, input, permissions)
}

// 更新角色并同步已提交的权限关系
func (service *RoleService) Update(
	ctx context.Context,
	input coreservice.UpdateInput[entity.Role, uint64],
) error {
	items := roleUpdateItems(input)
	if len(items) != 1 {
		return exception.Validate("角色更新只支持单条记录")
	}
	permissions, err := roleSubmittedPermissions(items[0].Mutable())
	if err != nil {
		return err
	}

	return service.UpdateWithPermissions(ctx, input, permissions)
}

// 在同一事务中新建角色及权限关系
func (service *RoleService) AddWithPermissions(
	ctx context.Context,
	input coreservice.AddInput[entity.Role],
	permissions dto.RolePermissionInput,
) (coreservice.AddResult[uint64], error) {
	if service == nil || service.runtime == nil {
		return coreservice.AddResult[uint64]{}, exception.Core("角色服务未初始化")
	}
	var result coreservice.AddResult[uint64]
	err := service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		value := input.One()
		if value == nil {
			return exception.Validate("角色新增只支持单条记录")
		}
		if err := setRoleCompatibilityFields(value, permissions); err != nil {
			return err
		}
		if err := service.lockRolePermissions(txCtx, permissions.MenuIDList, permissions.DepartmentIDList); err != nil {
			return err
		}
		var addErr error
		result, addErr = service.Base.Add(txCtx, input)
		if addErr != nil {
			return addErr
		}

		return service.replacePermissions(txCtx, result.One(), permissions)
	})

	return result, err
}

// 更新角色及可选权限关系，并撤销受影响用户 Session
func (service *RoleService) UpdateWithPermissions(
	ctx context.Context,
	input coreservice.UpdateInput[entity.Role, uint64],
	permissions *dto.RolePermissionInput,
) error {
	if service == nil || service.runtime == nil {
		return exception.Core("角色服务未初始化")
	}

	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		items := roleUpdateItems(input)
		if len(items) != 1 {
			return exception.Validate("角色更新只支持单条记录")
		}
		item := items[0]
		var (
			err                    error
			preservedMenuIDs       bool
			preservedDepartmentIDs bool
		)
		if permissions != nil {
			resolved := *permissions
			if !item.Mutable().Has("menuIdList") {
				preservedMenuIDs = true
				resolved.MenuIDList, err = roleRelationIDs(txCtx, service.roleMenu, "menuId", item.ID())
				if err != nil {
					return err
				}
			}
			if !item.Mutable().Has("departmentIdList") {
				preservedDepartmentIDs = true
				resolved.DepartmentIDList, err = roleRelationIDs(txCtx, service.roleDepartment, "departmentId", item.ID())
				if err != nil {
					return err
				}
			}
			permissions = &resolved
			if err = setRoleCompatibilityFields(item.Mutable(), resolved); err != nil {
				return err
			}
			if err = service.lockRolePermissions(txCtx, resolved.MenuIDList, resolved.DepartmentIDList); err != nil {
				return err
			}
		}
		if err := service.boundary.LockTable(txCtx, service.Descriptor().Table(), []uint64{item.ID()}, "锁定授权角色失败"); err != nil {
			return err
		}
		row, err := service.roleByID(txCtx, item.ID())
		if err != nil || row == nil {
			return err
		}
		if permissions != nil {
			if preservedMenuIDs {
				current, relationErr := roleRelationIDs(txCtx, service.roleMenu, "menuId", item.ID())
				if relationErr != nil {
					return relationErr
				}
				if relationErr = auth.ValidateSnapshot(permissions.MenuIDList, current, "角色菜单权限已变更，请重试"); relationErr != nil {
					return relationErr
				}
			}
			if preservedDepartmentIDs {
				current, relationErr := roleRelationIDs(txCtx, service.roleDepartment, "departmentId", item.ID())
				if relationErr != nil {
					return relationErr
				}
				if relationErr = auth.ValidateSnapshot(permissions.DepartmentIDList, current, "角色部门权限已变更，请重试"); relationErr != nil {
					return relationErr
				}
			}
		}
		if err = protectAdminRoleUpdate(*row, item.Mutable()); err != nil {
			return err
		}
		userIDs, err := service.userIDs(txCtx, []uint64{item.ID()})
		if err != nil {
			return err
		}
		if err = service.boundary.LockUsersAndRevoke(txCtx, userTable, userIDs, auth.AdminKind, "锁定授权用户失败"); err != nil {
			return err
		}
		if err = service.Base.Update(txCtx, input); err != nil {
			return err
		}
		if permissions != nil {
			return service.replacePermissions(txCtx, item.ID(), *permissions)
		}

		return nil
	})
}

// 删除角色及全部关系，并保护平台管理员角色
func (service *RoleService) Delete(ctx context.Context, input coreservice.DeleteInput[uint64]) error {
	if service == nil || service.runtime == nil {
		return exception.Core("角色服务未初始化")
	}

	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		roleIDs := businessUniqueIDs(input.IDs())
		menuIDs, departmentIDs, err := service.permissionResourceIDs(txCtx, roleIDs)
		if err != nil {
			return err
		}
		if err = service.lockRolePermissions(txCtx, menuIDs, departmentIDs); err != nil {
			return err
		}
		if err := service.boundary.LockTable(txCtx, service.Descriptor().Table(), roleIDs, "锁定授权角色失败"); err != nil {
			return err
		}
		lockedMenuIDs, lockedDepartmentIDs, err := service.permissionResourceIDs(txCtx, roleIDs)
		if err != nil {
			return err
		}
		if err = auth.ValidateSnapshot(menuIDs, lockedMenuIDs, "角色权限已变更，请重试"); err != nil {
			return err
		}
		if err = auth.ValidateSnapshot(departmentIDs, lockedDepartmentIDs, "角色权限已变更，请重试"); err != nil {
			return err
		}
		if err := service.ensureNoAdminRole(txCtx, roleIDs); err != nil {
			return err
		}
		userIDs, err := service.userIDs(txCtx, roleIDs)
		if err != nil {
			return err
		}
		if err = service.boundary.LockUsersAndRevoke(txCtx, userTable, userIDs, auth.AdminKind, "锁定授权用户失败"); err != nil {
			return err
		}
		for _, relation := range []interface {
			Model(context.Context) (*gdb.Model, error)
		}{service.userRole, service.roleMenu, service.roleDepartment} {
			model, modelErr := relation.Model(txCtx)
			if modelErr != nil {
				return modelErr
			}
			if _, modelErr = model.WhereIn("roleId", roleIDs).Delete(); modelErr != nil {
				return exception.WrapCore(modelErr, "清理角色关系失败")
			}
		}

		return service.Base.Delete(txCtx, input)
	})
}

// 角色详情及权威权限关系
func (service *RoleService) Info(ctx context.Context, roleID uint64) (*dto.RoleInfoResult, error) {
	row, err := service.roleByID(ctx, roleID)
	if err != nil || row == nil {
		return nil, err
	}
	row.MenuIDList, err = roleRelationIDs(ctx, service.roleMenu, "menuId", roleID)
	if err != nil {
		return nil, err
	}
	row.DepartmentIDList, err = roleRelationIDs(ctx, service.roleDepartment, "departmentId", roleID)
	if err != nil {
		return nil, err
	}
	result := roleInfoResult(*row)

	return &result, nil
}

// 当前管理员可见的非平台管理员角色
func (service *RoleService) List(ctx context.Context) ([]dto.RoleInfoResult, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return nil, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	model, err = service.visibleRoleModel(ctx, model, identity)
	if err != nil {
		return nil, err
	}
	var rows []roleRow
	if err = model.OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色列表失败")
	}
	result := make([]dto.RoleInfoResult, 0, len(rows))
	for _, row := range rows {
		info, infoErr := service.Info(ctx, row.ID)
		if infoErr != nil {
			return nil, infoErr
		}
		result = append(result, *info)
	}

	return result, nil
}

// 当前管理员可见的角色分页
func (service *RoleService) Page(ctx context.Context, query coreservice.Query) (RolePageResult, error) {
	if service == nil || query.PageNumber() <= 0 || query.PageSize() <= 0 {
		return RolePageResult{}, exception.Validate("角色分页参数无效")
	}
	identity, err := auth.Admin(ctx)
	if err != nil {
		return RolePageResult{}, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return RolePageResult{}, err
	}
	scope, exists := crud.CurrentOperation(ctx)
	if !exists || scope.Plan().Action() != crud.ActionPage {
		return RolePageResult{}, exception.Core("当前上下文不存在角色分页计划")
	}
	model, err = scope.Plan().ApplyQuery(ctx, model)
	if err != nil {
		return RolePageResult{}, exception.WrapCore(err, "应用角色分页查询失败")
	}
	model, err = service.visibleRoleModel(ctx, model, identity)
	if err != nil {
		return RolePageResult{}, err
	}
	result, total, err := model.Page(query.PageNumber(), query.PageSize()).AllAndCount(false)
	if err != nil {
		return RolePageResult{}, exception.WrapCore(err, "查询角色分页失败")
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = result.Structs(&rows); err != nil {
		return RolePageResult{}, exception.WrapCore(err, "解析角色分页失败")
	}
	items := make([]dto.RoleInfoResult, 0, len(rows))
	for _, row := range rows {
		info, infoErr := service.Info(ctx, row.ID)
		if infoErr != nil {
			return RolePageResult{}, infoErr
		}
		items = append(items, *info)
	}

	return RolePageResult{
		List:       items,
		Pagination: coreservice.Pagination{Page: query.PageNumber(), Size: query.PageSize(), Total: int64(total)},
	}, nil
}

func (service *RoleService) visibleRoleModel(
	ctx context.Context,
	model *gdb.Model,
	identity auth.AdminIdentity,
) (*gdb.Model, error) {
	model = model.Where("label IS NULL OR label <> ?", adminRoleLabel)
	isAdmin, err := service.isAdminIdentity(ctx, identity)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		model = model.Where("userId = ? OR id IN (?)", strconv.FormatUint(identity.UserID, 10), identity.RoleIDs())
	}

	return model, nil
}

func (service *RoleService) replacePermissions(
	ctx context.Context,
	roleID uint64,
	permissions dto.RolePermissionInput,
) error {
	if err := replaceRoleRelation(ctx, service.roleMenu, "menuId", roleID, permissions.MenuIDList); err != nil {
		return err
	}

	return replaceRoleRelation(ctx, service.roleDepartment, "departmentId", roleID, permissions.DepartmentIDList)
}

func (service *RoleService) permissionResourceIDs(
	ctx context.Context,
	roleIDs []uint64,
) ([]uint64, []uint64, error) {
	menuIDs, err := roleRelationIDsForRoles(ctx, service.roleMenu, "menuId", roleIDs)
	if err != nil {
		return nil, nil, err
	}
	departmentIDs, err := roleRelationIDsForRoles(ctx, service.roleDepartment, "departmentId", roleIDs)
	if err != nil {
		return nil, nil, err
	}

	return menuIDs, departmentIDs, nil
}

func (service *RoleService) roleByID(ctx context.Context, roleID uint64) (*roleRow, error) {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var row *roleRow
	if err = model.Where("id", roleID).Scan(&row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, exception.WrapCore(err, "查询角色失败")
	}

	return row, nil
}

func (service *RoleService) userIDs(ctx context.Context, roleIDs []uint64) ([]uint64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		UserID uint64 `orm:"userId"`
	}
	if err = model.Fields("userId").WhereIn("roleId", roleIDs).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色用户失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.UserID
	}

	return businessUniqueIDs(ids), nil
}

func (service *RoleService) ensureNoAdminRole(ctx context.Context, roleIDs []uint64) error {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	count, err := model.WhereIn("id", roleIDs).Where("label", adminRoleLabel).Count()
	if err != nil {
		return exception.WrapCore(err, "查询管理员角色失败")
	}
	if count > 0 {
		return exception.Comm("不能删除平台管理员角色")
	}

	return nil
}

func (service *RoleService) isAdminIdentity(ctx context.Context, identity auth.AdminIdentity) (bool, error) {
	if len(identity.RoleIDs()) == 0 {
		return false, nil
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return false, err
	}
	count, err := model.WhereIn("id", identity.RoleIDs()).Where("label", adminRoleLabel).Count()
	if err != nil {
		return false, exception.WrapCore(err, "查询管理员角色失败")
	}

	return count > 0, nil
}

func protectAdminRoleUpdate(row roleRow, mutable *coreservice.Mutable[entity.Role]) error {
	if row.Label == nil || *row.Label != adminRoleLabel {
		return nil
	}
	if name, exists := mutable.Get("name"); exists && name != row.Name {
		return exception.Comm("不能修改平台管理员角色名称")
	}
	if label, exists := mutable.Get("label"); exists {
		value, ok := label.(*string)
		if !ok || value == nil || *value != adminRoleLabel {
			return exception.Comm("不能修改平台管理员角色标识")
		}
	}

	return nil
}

func setRoleCompatibilityFields(
	mutable *coreservice.Mutable[entity.Role],
	permissions dto.RolePermissionInput,
) error {
	if err := mutable.Set("menuIdList", businessUniqueIDs(permissions.MenuIDList)); err != nil {
		return err
	}

	return mutable.Set("departmentIdList", businessUniqueIDs(permissions.DepartmentIDList))
}

func roleMutablePermissions(
	mutable *coreservice.Mutable[entity.Role],
) (dto.RolePermissionInput, error) {
	permissions, err := roleSubmittedPermissions(mutable)
	if err != nil {
		return dto.RolePermissionInput{}, err
	}
	if permissions == nil {
		return dto.RolePermissionInput{}, nil
	}

	return *permissions, nil
}

func roleSubmittedPermissions(
	mutable *coreservice.Mutable[entity.Role],
) (*dto.RolePermissionInput, error) {
	if mutable == nil {
		return nil, exception.Validate("角色权限参数无效")
	}
	var (
		permissions dto.RolePermissionInput
		hasValue    bool
	)
	for _, field := range []struct {
		name   string
		target *[]uint64
	}{
		{name: "menuIdList", target: &permissions.MenuIDList},
		{name: "departmentIdList", target: &permissions.DepartmentIDList},
	} {
		value, exists := mutable.Get(field.name)
		if !exists {
			continue
		}
		ids, ok := value.([]uint64)
		if !ok {
			return nil, exception.Validate("角色权限字段无效")
		}
		*field.target = ids
		hasValue = true
	}
	if !hasValue {
		return nil, nil
	}

	return &permissions, nil
}

func replaceRoleRelation[E any](
	ctx context.Context,
	base *coreservice.Base[E, uint64],
	column string,
	roleID uint64,
	ids []uint64,
) error {
	model, err := base.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.Where("roleId", roleID).Delete(); err != nil {
		return exception.WrapCore(err, "删除旧角色关系失败")
	}
	for _, id := range businessUniqueIDs(ids) {
		data, dataErr := businessDO(base.Descriptor(), businessField{name: "roleId", value: roleID}, businessField{name: column, value: id})
		if dataErr != nil {
			return dataErr
		}
		if _, err = model.Data(data).Insert(); err != nil {
			return exception.WrapCore(err, "写入角色关系失败")
		}
	}

	return nil
}

func roleRelationIDs[E any](
	ctx context.Context,
	base *coreservice.Base[E, uint64],
	column string,
	roleID uint64,
) ([]uint64, error) {
	model, err := base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields(column+" AS id").Where("roleId", roleID).OrderAsc(column).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色关系失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}

	return ids, nil
}

func roleRelationIDsForRoles[E any](
	ctx context.Context,
	base *coreservice.Base[E, uint64],
	column string,
	roleIDs []uint64,
) ([]uint64, error) {
	roleIDs = businessUniqueIDs(roleIDs)
	if len(roleIDs) == 0 {
		return nil, nil
	}
	model, err := base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields(column+" AS id").WhereIn("roleId", roleIDs).OrderAsc(column).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色关系失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}

	return businessUniqueIDs(ids), nil
}

func roleInfoResult(row roleRow) dto.RoleInfoResult {
	return dto.RoleInfoResult{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		UserID: row.UserID, Name: row.Name, Label: row.Label, Remark: row.Remark,
		Relevance: row.Relevance, MenuIDList: row.MenuIDList,
		DepartmentIDList: row.DepartmentIDList,
	}
}

func roleUpdateItems(input coreservice.UpdateInput[entity.Role, uint64]) []coreservice.UpdateItem[entity.Role, uint64] {
	if input.IsMany() {
		return input.Many()
	}

	return []coreservice.UpdateItem[entity.Role, uint64]{input.One()}
}
