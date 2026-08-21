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

type departmentRow struct {
	ID         uint64      `orm:"id"`
	CreateTime *gtime.Time `orm:"createTime"`
	UpdateTime *gtime.Time `orm:"updateTime"`
	Name       string      `orm:"name"`
	UserID     *uint64     `orm:"userId"`
	ParentID   *uint64     `orm:"parentId"`
	OrderNum   int32       `orm:"orderNum"`
}

type departmentDeleteTree struct {
	RootID   uint64
	ParentID *uint64
	IDs      []uint64
}

// 部门树、用户归属及角色部门关系
type DepartmentService struct {
	*coreservice.Base[entity.Department, uint64]
	runtime        *coredb.Runtime
	user           *coreservice.Base[entity.User, uint64]
	role           *coreservice.Base[entity.Role, uint64]
	userRole       *coreservice.Base[entity.UserRole, uint64]
	roleDepartment *coreservice.Base[entity.RoleDepartment, uint64]
	boundary       *auth.Boundary
}

// 部门业务服务
func NewDepartment(
	runtime *coredb.Runtime,
	department *coreservice.Base[entity.Department, uint64],
	user *coreservice.Base[entity.User, uint64],
	role *coreservice.Base[entity.Role, uint64],
	userRole *coreservice.Base[entity.UserRole, uint64],
	roleDepartment *coreservice.Base[entity.RoleDepartment, uint64],
	sessions auth.SessionStore,
) (*DepartmentService, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(department) ||
		!validPermissionBase(user) || !validPermissionBase(role) || !validPermissionBase(userRole) ||
		!validPermissionBase(roleDepartment) {
		return nil, exception.Core("部门服务依赖无效")
	}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		return nil, err
	}

	return &DepartmentService{
		Base: department, runtime: runtime, user: user, role: role,
		userRole: userRole, roleDepartment: roleDepartment, boundary: boundary,
	}, nil
}

// 新增部门
func (service *DepartmentService) Add(ctx context.Context, input coreservice.AddInput[entity.Department]) (coreservice.AddResult[uint64], error) {
	if service == nil || service.runtime == nil {
		return coreservice.AddResult[uint64]{}, exception.Core("部门服务未初始化")
	}
	var result coreservice.AddResult[uint64]
	err := service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		parentIDs, parentErr := departmentAddParentIDs(input)
		if parentErr != nil {
			return parentErr
		}
		if parentErr = service.lockDepartments(txCtx, parentIDs); parentErr != nil {
			return parentErr
		}
		result, parentErr = service.Base.Add(txCtx, input)
		return parentErr
	})
	return result, err
}

// 更新部门
func (service *DepartmentService) Update(ctx context.Context, input coreservice.UpdateInput[entity.Department, uint64]) error {
	if service == nil || service.runtime == nil {
		return exception.Core("部门服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		items := departmentUpdateItems(input)
		ids := make([]uint64, 0, len(items)*2)
		for _, item := range items {
			ids = append(ids, item.ID())
			if parentID, exists, parentErr := departmentMutableParentID(item.Mutable()); parentErr != nil {
				return parentErr
			} else if exists && parentID != nil {
				ids = append(ids, *parentID)
			}
		}
		if err := service.lockDepartments(txCtx, ids); err != nil {
			return err
		}
		return service.Base.Update(txCtx, input)
	})
}

// 按 Vue 顶层数组契约更新部门排序和父级
func (service *DepartmentService) Order(ctx context.Context, request dto.DepartmentOrderReq) error {
	if service == nil || service.runtime == nil || len(request) == 0 {
		return exception.Validate("部门排序参数无效")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		ids := make([]uint64, 0, len(request)*2)
		for _, item := range request {
			ids = append(ids, item.ID)
			if item.ParentID != nil {
				ids = append(ids, *item.ParentID)
			}
		}
		if err := service.lockDepartments(txCtx, ids); err != nil {
			return err
		}
		model, err := service.Base.Model(txCtx)
		if err != nil {
			return err
		}
		for _, item := range request {
			data, dataErr := businessDO(service.Descriptor(), businessField{name: "orderNum", value: item.OrderNum})
			if dataErr != nil {
				return dataErr
			}
			if item.ParentID == nil {
				data, dataErr = businessDO(service.Descriptor(), businessField{name: "orderNum", value: item.OrderNum}, businessField{name: "parentId", value: nil})
				if dataErr != nil {
					return dataErr
				}
			}
			if item.ParentID != nil {
				data, dataErr = businessDO(service.Descriptor(), businessField{name: "orderNum", value: item.OrderNum}, businessField{name: "parentId", value: *item.ParentID})
				if dataErr != nil {
					return dataErr
				}
			}
			if _, err = model.Where("id", item.ID).Data(data).Update(); err != nil {
				return exception.WrapCore(err, "更新部门排序失败")
			}
		}
		return nil
	})
}

// 当前用户可见的部门列表
func (service *DepartmentService) List(ctx context.Context) ([]dto.DepartmentListItem, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return nil, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	isAdmin, err := service.isAdmin(ctx, identity.RoleIDs())
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		departmentIDs, idErr := service.departmentIDsByRoles(ctx, identity.RoleIDs())
		if idErr != nil {
			return nil, idErr
		}
		if len(departmentIDs) == 0 {
			model = model.Where("userId", identity.UserID)
		} else {
			model = model.WhereIn("id", departmentIDs).WhereOr("userId", identity.UserID)
		}
	}
	var rows []departmentRow
	if err = model.OrderAsc("orderNum").OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询部门列表失败")
	}
	items := make([]dto.DepartmentListItem, 0, len(rows))
	for _, row := range rows {
		parentName, nameErr := service.parentName(ctx, row.ParentID)
		if nameErr != nil {
			return nil, nameErr
		}
		items = append(items, dto.DepartmentListItem{ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Name: row.Name, UserID: row.UserID, ParentID: row.ParentID, OrderNum: row.OrderNum, ParentName: parentName})
	}
	return items, nil
}

// 删除部门树，并按请求迁移或删除归属用户
func (service *DepartmentService) Delete(ctx context.Context, request dto.DepartmentDeleteReq) error {
	if service == nil || service.runtime == nil {
		return exception.Core("部门服务未初始化")
	}
	ids := businessUniqueIDs(request.IDs)
	if len(ids) == 0 {
		return exception.Validate("部门 ID 不能为空")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		if request.DeleteUser {
			if err := service.lockAdminRole(txCtx); err != nil {
				return err
			}
		}
		trees, err := service.lockedDeleteTrees(txCtx, ids)
		if err != nil {
			return err
		}
		allIDs := departmentTreeIDs(trees)
		userIDs, err := service.userIDsByDepartments(txCtx, allIDs)
		if err != nil {
			return err
		}
		if len(userIDs) > 0 {
			if err = service.lockDepartmentUsers(txCtx, userIDs); err != nil {
				return err
			}
			if request.DeleteUser {
				if err = service.ensureNotLastAdmin(txCtx, userIDs); err != nil {
					return err
				}
			}
			if err = service.boundary.RevokeUsers(txCtx, auth.AdminKind, userIDs); err != nil {
				return err
			}
		}
		if request.DeleteUser && len(userIDs) > 0 {
			model, modelErr := service.user.Model(txCtx)
			if modelErr != nil {
				return modelErr
			}
			userRoleModel, relationErr := service.userRole.Model(txCtx)
			if relationErr != nil {
				return relationErr
			}
			if _, modelErr = userRoleModel.WhereIn("userId", userIDs).Delete(); modelErr != nil {
				return exception.WrapCore(modelErr, "清理部门用户角色关系失败")
			}
			if _, modelErr = model.WhereIn("id", userIDs).Delete(); modelErr != nil {
				return exception.WrapCore(modelErr, "删除部门用户失败")
			}
		} else if len(userIDs) > 0 {
			model, modelErr := service.user.Model(txCtx)
			if modelErr != nil {
				return modelErr
			}
			for _, tree := range trees {
				data, dataErr := service.departmentUserData(tree.ParentID)
				if dataErr != nil {
					return dataErr
				}
				if _, modelErr = model.WhereIn("departmentId", tree.IDs).Data(data).Update(); modelErr != nil {
					return exception.WrapCore(modelErr, "迁移部门用户失败")
				}
			}
		}
		model, err := service.roleDepartment.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = model.WhereIn("departmentId", allIDs).Delete(); err != nil {
			return exception.WrapCore(err, "清理部门角色关系失败")
		}
		deleteInput, err := coreservice.NewDeleteInput[entity.Department](service.Descriptor(), allIDs)
		if err != nil {
			return err
		}
		return service.Base.Delete(txCtx, deleteInput)
	})
}

func (service *DepartmentService) lockDepartments(ctx context.Context, ids []uint64) error {
	return service.boundary.LockTable(ctx, service.Descriptor().Table(), ids, "锁定部门失败")
}

func (service *DepartmentService) lockedDeleteTrees(ctx context.Context, roots []uint64) ([]departmentDeleteTree, error) {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []departmentRow
	if err = model.Fields("id", "parentId").OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询部门树失败")
	}
	allIDs := make([]uint64, len(rows))
	for index, row := range rows {
		allIDs[index] = row.ID
	}
	if err = service.lockDepartments(ctx, allIDs); err != nil {
		return nil, err
	}
	model, err = service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	if err = model.Fields("id", "parentId").OrderAsc("id").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询锁定部门树失败")
	}
	existing := make(map[uint64]struct{}, len(rows))
	for _, row := range rows {
		existing[row.ID] = struct{}{}
	}
	for _, root := range roots {
		if _, exists := existing[root]; !exists {
			return nil, exception.Validate("部门不存在")
		}
	}
	return buildDepartmentDeleteTrees(rows, roots), nil
}

func (service *DepartmentService) userIDsByDepartments(ctx context.Context, departmentIDs []uint64) ([]uint64, error) {
	model, err := service.user.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").WhereIn("departmentId", departmentIDs).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询部门用户失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}
	return businessUniqueIDs(ids), nil
}

func (service *DepartmentService) departmentIDsByRoles(ctx context.Context, roleIDs []uint64) ([]uint64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	model, err := service.roleDepartment.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		DepartmentID uint64 `orm:"departmentId"`
	}
	if err = model.Fields("departmentId").WhereIn("roleId", roleIDs).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询角色部门失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.DepartmentID
	}
	return businessUniqueIDs(ids), nil
}

func (service *DepartmentService) isAdmin(ctx context.Context, roleIDs []uint64) (bool, error) {
	if len(roleIDs) == 0 {
		return false, nil
	}
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

func (service *DepartmentService) parentName(ctx context.Context, parentID *uint64) (*string, error) {
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
		return nil, exception.WrapCore(err, "查询部门父节点失败")
	}
	if row == nil {
		return nil, nil
	}
	return &row.Name, nil
}

func departmentUpdateItems(input coreservice.UpdateInput[entity.Department, uint64]) []coreservice.UpdateItem[entity.Department, uint64] {
	if input.IsMany() {
		return input.Many()
	}
	return []coreservice.UpdateItem[entity.Department, uint64]{input.One()}
}

func (service *DepartmentService) lockAdminRole(ctx context.Context) error {
	model, err := service.role.Model(ctx)
	if err != nil {
		return err
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").Where("label", adminRoleLabel).OrderAsc("id").Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询管理员角色失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.ID
	}
	return service.boundary.LockTable(ctx, service.role.Descriptor().Table(), ids, "锁定授权角色失败")
}

func (service *DepartmentService) lockDepartmentUsers(ctx context.Context, userIDs []uint64) error {
	return service.boundary.LockTable(ctx, service.user.Descriptor().Table(), userIDs, "锁定部门用户失败")
}

func (service *DepartmentService) ensureNotLastAdmin(ctx context.Context, userIDs []uint64) error {
	statement, err := coreservice.NativeSQL(`SELECT COUNT(DISTINCT u.id) FROM base_sys_user u INNER JOIN base_sys_user_role ur ON ur.userId = u.id INNER JOIN base_sys_role r ON r.id = ur.roleId WHERE u.status = 1 AND r.label = ? AND u.id IN (?)`, adminRoleLabel, userIDs)
	if err != nil {
		return err
	}
	var affected int
	if err = service.user.NativeQuery(ctx, statement, &affected); err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}
	statement, err = coreservice.NativeSQL(`SELECT COUNT(DISTINCT u.id) FROM base_sys_user u INNER JOIN base_sys_user_role ur ON ur.userId = u.id INNER JOIN base_sys_role r ON r.id = ur.roleId WHERE u.status = 1 AND r.label = ? AND u.id NOT IN (?)`, adminRoleLabel, userIDs)
	if err != nil {
		return err
	}
	var remaining int
	if err = service.user.NativeQuery(ctx, statement, &remaining); err != nil {
		return err
	}
	if remaining == 0 {
		return exception.Comm("不能删除最后一个管理员")
	}
	return nil
}

func (service *DepartmentService) departmentUserData(parentID *uint64) (any, error) {
	value := service.user.Descriptor().NewDO()
	if value == nil {
		return nil, exception.Core("用户 DO 无效")
	}
	if parentID == nil {
		if err := value.SetColumn("departmentId", nil); err != nil {
			return nil, err
		}
	} else if err := value.SetColumn("departmentId", *parentID); err != nil {
		return nil, err
	}
	return value.DBData(), nil
}

func departmentAddParentIDs(input coreservice.AddInput[entity.Department]) ([]uint64, error) {
	values := input.Many()
	if !input.IsMany() {
		values = []*coreservice.Mutable[entity.Department]{input.One()}
	}
	ids := make([]uint64, 0, len(values))
	for _, value := range values {
		parentID, exists, err := departmentMutableParentID(value)
		if err != nil {
			return nil, err
		}
		if exists && parentID != nil {
			ids = append(ids, *parentID)
		}
	}
	return businessUniqueIDs(ids), nil
}

func departmentMutableParentID(value *coreservice.Mutable[entity.Department]) (*uint64, bool, error) {
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
		return nil, true, exception.Validate("部门父级无效")
	}
}

func buildDepartmentDeleteTrees(rows []departmentRow, roots []uint64) []departmentDeleteTree {
	parents := make(map[uint64]*uint64, len(rows))
	children := make(map[uint64][]uint64)
	selected := make(map[uint64]struct{}, len(roots))
	for _, root := range roots {
		selected[root] = struct{}{}
	}
	for _, row := range rows {
		parents[row.ID] = row.ParentID
		if row.ParentID != nil {
			children[*row.ParentID] = append(children[*row.ParentID], row.ID)
		}
	}
	effective := make([]uint64, 0, len(roots))
	for _, root := range roots {
		parentID := parents[root]
		hasSelectedAncestor := false
		seen := map[uint64]struct{}{root: {}}
		for parentID != nil {
			if _, exists := selected[*parentID]; exists {
				hasSelectedAncestor = true
				break
			}
			if _, exists := seen[*parentID]; exists {
				break
			}
			seen[*parentID] = struct{}{}
			parentID = parents[*parentID]
		}
		if !hasSelectedAncestor {
			effective = append(effective, root)
		}
	}
	trees := make([]departmentDeleteTree, 0, len(effective))
	for _, root := range effective {
		ids := []uint64{root}
		for index := 0; index < len(ids); index++ {
			ids = append(ids, children[ids[index]]...)
		}
		trees = append(trees, departmentDeleteTree{RootID: root, ParentID: parents[root], IDs: businessUniqueIDs(ids)})
	}
	return trees
}

func departmentTreeIDs(trees []departmentDeleteTree) []uint64 {
	ids := make([]uint64, 0)
	for _, tree := range trees {
		ids = append(ids, tree.IDs...)
	}
	return businessUniqueIDs(ids)
}
