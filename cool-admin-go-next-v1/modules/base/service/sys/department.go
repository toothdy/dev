package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

// 系统部门服务
type DepartmentService struct {
	*service.Base
	Sessions            security.SessionStore
	recycle             *recycle.Manager
	userModel           entity.Definition
	userRoleModel       entity.Definition
	roleDepartmentModel entity.Definition
}

// 部门排序项(已迁移至 dto.DepartmentOrderItem,见 modules/base/dto/department.go)

type departmentMutationRow struct {
	Name       interface{} `orm:"name"`
	UserID     interface{} `orm:"userId"`
	ParentID   interface{} `orm:"parentId"`
	OrderNum   interface{} `orm:"orderNum"`
	TenantID   interface{} `orm:"tenantId"`
	CreateTime interface{} `orm:"createTime"`
	UpdateTime interface{} `orm:"updateTime"`
}

type departmentOrderRow struct {
	ParentID interface{} `orm:"parentId"`
	OrderNum int64       `orm:"orderNum"`
}

/**
 * 构建部门局部更新数据
 * @param data 请求数据
 * @returns 更新行、数据库字段和校验错误
 */
func departmentUpdateMutation(data map[string]interface{}) (departmentMutationRow, []string, error) {
	row := departmentMutationRow{}
	fields := make([]string, 0, 4)
	if value, ok := data["name"]; ok {
		name, valid := value.(string)
		name = strings.TrimSpace(name)
		if !valid || name == "" {
			return departmentMutationRow{}, nil, exception.Validate("name不能为空")
		}
		row.Name = name
		fields = append(fields, "name")
	}
	if value, ok := data["parentId"]; ok {
		row.ParentID = value
		fields = append(fields, "parentId")
	}
	if value, ok := data["orderNum"]; ok {
		if value == nil {
			return departmentMutationRow{}, nil, exception.Validate("orderNum不能为空")
		}
		row.OrderNum = value
		fields = append(fields, "orderNum")
	}
	return row, fields, nil
}


/**
 * 创建系统部门服务
 * @param db 数据库实例
 * @param baseSysDepartmentModel 部门模型
 * @param baseSysUserModel 用户模型
 * @param baseSysUserRoleModel 用户角色关系模型
 * @param baseSysRoleDepartmentModel 角色部门关系模型
 * @param sessions 会话存储
 * @param recycleManager 回收站协调器
 * @returns *DepartmentService
 */
func NewDepartmentService(
	db gdb.DB,
	baseSysDepartmentModel entity.Definition,
	baseSysUserModel entity.Definition,
	baseSysUserRoleModel entity.Definition,
	baseSysRoleDepartmentModel entity.Definition,
	sessions security.SessionStore,
	recycleManager *recycle.Manager,
) *DepartmentService {
	return &DepartmentService{
		Base:         service.NewBase(db, baseSysDepartmentModel),
		Sessions:            sessions,
		recycle:             recycleManager,
		userModel:           baseSysUserModel,
		userRoleModel:       baseSysUserRoleModel,
		roleDepartmentModel: baseSysRoleDepartmentModel,
	}
}

// 新增部门，并强制使用当前登录用户和租户
func (s *DepartmentService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	for field := range request.Data {
		switch field {
		case "name", "parentId", "orderNum", "userId", "tenantId":
		default:
			return nil, exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	name := strings.TrimSpace(fmt.Sprint(request.Data["name"]))
	if name == "" || name == "<nil>" {
		return nil, exception.Validate("name不能为空")
	}
	user, err := security.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	row := departmentMutationRow{
		Name:     name,
		UserID:   user.UserId,
		ParentID: request.Data["parentId"],
		OrderNum: request.Data["orderNum"],
	}
	if _, ok := request.Data["orderNum"]; !ok {
		row.OrderNum = 0
	}
	now := mutationTimestamp()
	row.CreateTime = now
	row.UpdateTime = now
	dbModel, err := tenant.ScopedModel(ctx, s.DB, s.Model, "")
	if err != nil {
		return nil, err
	}
	parentID := int64Value(row.ParentID)
	if parentID <= 0 {
		id, insertErr := dbModel.Data(row).InsertAndGetId()
		if insertErr != nil {
			return nil, gerror.Wrap(insertErr, "新增部门失败")
		}
		return map[string]interface{}{"id": id}, nil
	}
	var id int64
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		departments, queryErr := s.lockScopedDepartments(ctx, tx, []int64{parentID})
		if queryErr != nil {
			return queryErr
		}
		if _, ok := departments[parentID]; !ok {
			return exception.Comm("上级部门不存在")
		}
		insertModel, modelErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if modelErr != nil {
			return modelErr
		}
		id, modelErr = insertModel.Data(row).InsertAndGetId()
		if modelErr != nil {
			return gerror.Wrap(modelErr, "新增部门失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": id}, nil
}

// 只修改当前租户的部门
func (s *DepartmentService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	delete(request.Data, "tenantId")
	delete(request.Data, "userId")
	delete(request.Data, "createTime")
	delete(request.Data, "updateTime")
	for field := range request.Data {
		switch field {
		case "id", "name", "parentId", "orderNum":
		default:
			return nil, exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	id := int64Value(request.Data["id"])
	if id <= 0 {
		return nil, exception.Validate("id参数错误")
	}
	row, fields, err := departmentUpdateMutation(request.Data)
	if err != nil {
		return nil, err
	}
	if row.ParentID != nil && int64Value(row.ParentID) == id {
		return nil, exception.Validate("上级部门不能是自身")
	}
	row.UpdateTime = mutationTimestamp()
	fields = append(fields, "updateTime")
	if _, err := tenant.ScopedModel(ctx, s.DB, s.Model, ""); err != nil {
		return nil, err
	}
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		lockIDs := []int64{id}
		parentID := int64Value(row.ParentID)
		if parentID > 0 {
			lockIDs = append(lockIDs, parentID)
		}
		departments, queryErr := s.lockScopedDepartments(ctx, tx, lockIDs)
		if queryErr != nil {
			return queryErr
		}
		if _, ok := departments[id]; !ok {
			return exception.Comm("部门不存在")
		}
		if parentID > 0 {
			if _, ok := departments[parentID]; !ok {
				return exception.Comm("上级部门不存在")
			}
		}
		updateModel, modelErr := tenant.ScopedModel(ctx, tx, s.Model, "")
		if modelErr != nil {
			return modelErr
		}
		if _, modelErr = updateModel.Fields(fields).Where("id", id).Data(row).Update(); modelErr != nil {
			return gerror.Wrap(modelErr, "修改部门失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// 返回当前用户可见部门，并补充父部门名称
func (s *DepartmentService) List(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	user, err := security.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	join := " LEFT JOIN base_sys_department p ON a.parentId = p.id"
	args := []interface{}{}
	if tenantID, ok := contextTenantID(ctx); ok {
		join += " AND p.tenantId = ?"
		args = append(args, tenantID)
	}
	where := " WHERE 1 = 1"
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND a.tenantId = ?"
		args = append(args, tenantID)
	}
	isAdmin, err := isPlatformAdministrator(ctx, s.DB, user.UserId)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		departmentIDs, queryErr := departmentIDsForUser(ctx, s.DB, user.UserId)
		if queryErr != nil {
			return nil, queryErr
		}
		if len(departmentIDs) == 0 {
			where += " AND a.userId = ?"
			args = append(args, user.UserId)
		} else {
			where += " AND (a.id IN (?) OR a.userId = ?)"
			args = append(args, departmentIDs, user.UserId)
		}
	}
	rows, err := s.DB.GetAll(ctx, "SELECT a.id, a.name, a.userId AS userId, a.parentId AS parentId, a.orderNum AS orderNum, a.createTime AS createTime, a.updateTime AS updateTime, a.tenantId AS tenantId, p.name AS parentName FROM base_sys_department a"+join+where+" ORDER BY a.orderNum ASC", args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询部门列表失败")
	}
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Map())
	}
	return result, nil
}

// 删除部门，并按 deleteUser 决定删除或迁移部门用户
func (s *DepartmentService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	ids, err := parseUserIDs(request.IDs, "删除ID参数错误")
	if err != nil || len(ids) == 0 {
		return nil, exception.Validate("删除ID不能为空")
	}
	user, err := security.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	deleteUser := booleanValue(request.Data["deleteUser"])
	deletedUserIDs := []int64{}
	requestIDs := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		requestIDs = append(requestIDs, id)
	}
	err = runManagedDelete(ctx, s.DB, s.recycle, s.Model, requestIDs, request, func(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope) error {
		departmentQuery := tx.Model(s.Model.TableName).Ctx(ctx).
			Fields("id").
			WhereIn("id", ids).
			OrderAsc("id")
		if tenantID, ok := contextTenantID(ctx); ok {
			departmentQuery = departmentQuery.Where("tenantId", tenantID)
		}
		departments, queryErr := departmentQuery.LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询部门失败")
		}
		if len(departments) != len(ids) {
			return exception.Comm("部门不存在")
		}
		userQuery := tx.Model("base_sys_user").Ctx(ctx).WhereIn("departmentId", ids)
		if tenantID, ok := contextTenantID(ctx); ok {
			userQuery = userQuery.Where("tenantId", tenantID)
		}
		users, queryErr := userQuery.Fields("id").All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询部门用户失败")
		}
		for _, item := range users {
			deletedUserIDs = append(deletedUserIDs, item["id"].Int64())
		}
		if deleteUser && len(deletedUserIDs) > 0 {
			if lockErr := lockAuthorizationUsers(ctx, tx, deletedUserIDs); lockErr != nil {
				return lockErr
			}
			for _, userID := range deletedUserIDs {
				target, targetErr := authorizationUserFromDatabase(ctx, tx, userID)
				if targetErr != nil {
					return targetErr
				}
				if authErr := ensureUserMutationAllowed(ctx, tx, user.UserId, target, nil, false); authErr != nil {
					return authErr
				}
			}
			userKeys := map[int64]string{}
			userBranches := map[int64]string{}
			if scope != nil && scope.IsArchiving() {
				userRows, archiveErr := tx.Model(s.userModel.TableName).Ctx(ctx).
					WhereIn("id", deletedUserIDs).OrderAsc("id").LockUpdate().All()
				if archiveErr != nil {
					return gerror.Wrap(archiveErr, "锁定部门用户失败")
				}
				for _, row := range userRows {
					userID := row["id"].Int64()
					departmentID := row["departmentId"].Int64()
					parentKey, exists := scope.RootKey(departmentID)
					if !exists {
						return gerror.Newf("部门 %d 缺少回收站根归档项", departmentID)
					}
					branchKey := fmt.Sprint(departmentID)
					itemKey, addErr := scope.AddRecord(s.userModel, row.Map(), recycle.ItemOptions{
						BranchKey: branchKey, ParentKey: parentKey, RestoreOrder: 10,
					})
					if addErr != nil {
						return addErr
					}
					userKeys[userID] = itemKey
					userBranches[userID] = branchKey
				}
				roleRows, archiveErr := tx.Model(s.userRoleModel.TableName).Ctx(ctx).
					WhereIn("userId", deletedUserIDs).OrderAsc("userId").OrderAsc("roleId").LockUpdate().All()
				if archiveErr != nil {
					return gerror.Wrap(archiveErr, "锁定部门用户角色失败")
				}
				for _, row := range roleRows {
					userID := row["userId"].Int64()
					if _, addErr := scope.AddRecord(s.userRoleModel, row.Map(), recycle.ItemOptions{
						BranchKey: userBranches[userID], ParentKey: userKeys[userID], RestoreOrder: 20,
					}); addErr != nil {
						return addErr
					}
				}
			}
			roleResult, deleteErr := tx.Model(s.userRoleModel.TableName).Ctx(ctx).WhereIn("userId", deletedUserIDs).Delete()
			if deleteErr != nil {
				return gerror.Wrap(deleteErr, "删除用户角色失败")
			}
			if markErr := markManagedDeleted(scope, roleResult, "读取部门用户角色删除数量失败"); markErr != nil {
				return markErr
			}
			userResult, deleteErr := tx.Model(s.userModel.TableName).Ctx(ctx).WhereIn("id", deletedUserIDs).Delete()
			if deleteErr != nil {
				return gerror.Wrap(deleteErr, "删除部门用户失败")
			}
			if markErr := markManagedDeleted(scope, userResult, "读取部门用户删除数量失败"); markErr != nil {
				return markErr
			}
		} else if len(deletedUserIDs) > 0 {
			topQuery := tx.Model(s.Model.TableName).Ctx(ctx).Fields("id").WhereNull("parentId").WhereNotIn("id", ids)
			if tenantID, ok := contextTenantID(ctx); ok {
				topQuery = topQuery.Where("tenantId", tenantID)
			}
			top, topErr := topQuery.OrderAsc("orderNum").One()
			if topErr != nil {
				return gerror.Wrap(topErr, "查询顶级部门失败")
			}
			departmentID := interface{}(gdb.Raw("NULL"))
			if !top.IsEmpty() {
				departmentID = top["id"].Int64()
			}
			if _, updateErr := tx.Model("base_sys_user").Ctx(ctx).WhereIn("id", deletedUserIDs).Data(map[string]interface{}{"departmentId": departmentID}).Update(); updateErr != nil {
				return gerror.Wrap(updateErr, "迁移部门用户失败")
			}
		}
		relationQuery := tx.Model(s.roleDepartmentModel.TableName).Ctx(ctx).WhereIn("departmentId", ids)
		if scope != nil && scope.IsArchiving() {
			rows, archiveErr := relationQuery.Clone().OrderAsc("departmentId").OrderAsc("roleId").LockUpdate().All()
			if archiveErr != nil {
				return gerror.Wrap(archiveErr, "锁定角色部门关系失败")
			}
			for _, row := range rows {
				departmentID := row["departmentId"].Int64()
				parentKey, exists := scope.RootKey(departmentID)
				if !exists {
					return gerror.Newf("部门 %d 缺少回收站根归档项", departmentID)
				}
				if _, addErr := scope.AddRecord(s.roleDepartmentModel, row.Map(), recycle.ItemOptions{
					BranchKey: fmt.Sprint(departmentID), ParentKey: parentKey, RestoreOrder: 30,
				}); addErr != nil {
					return addErr
				}
			}
		}
		relationResult, deleteErr := relationQuery.Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除角色部门关系失败")
		}
		if markErr := markManagedDeleted(scope, relationResult, "读取角色部门关系删除数量失败"); markErr != nil {
			return markErr
		}
		deleteQuery := tx.Model(s.Model.TableName).Ctx(ctx).WhereIn("id", ids)
		if tenantID, ok := contextTenantID(ctx); ok {
			deleteQuery = deleteQuery.Where("tenantId", tenantID)
		}
		deleteResult, deleteErr := deleteQuery.Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除部门失败")
		}
		if markErr := markManagedDeleted(scope, deleteResult, "读取部门删除数量失败"); markErr != nil {
			return markErr
		}
		if deleteUser && len(deletedUserIDs) > 0 && scope != nil {
			return scope.AfterCommit(func(actionCtx context.Context) error {
				return revokeAuthorizationSessions(actionCtx, s.Sessions, deletedUserIDs, "使部门用户登录会话失效失败")
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.recycle == nil && deleteUser && len(deletedUserIDs) > 0 {
		if err = revokeAuthorizationSessions(ctx, s.Sessions, deletedUserIDs, "使部门用户登录会话失效失败"); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func departmentIDsForUser(ctx context.Context, db gdb.DB, userID int64) ([]int64, error) {
	query := "SELECT DISTINCT rd.departmentId AS departmentId FROM base_sys_user_role ur INNER JOIN base_sys_user u ON u.id = ur.userId INNER JOIN base_sys_role r ON r.id = ur.roleId INNER JOIN base_sys_role_department rd ON rd.roleId = r.id INNER JOIN base_sys_department d ON d.id = rd.departmentId WHERE ur.userId = ?"
	args := []interface{}{userID}
	if tenantID, ok := contextTenantID(ctx); ok {
		query += " AND u.tenantId = ? AND r.tenantId = ? AND d.tenantId = ?"
		args = append(args, tenantID, tenantID, tenantID)
	}
	rows, err := db.GetAll(ctx, query, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户部门权限失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["departmentId"].Int64())
	}
	return ids, nil
}

func booleanValue(value interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

/**
 * 更新部门排序
 * @param ctx 请求上下文
 * @param items 部门排序项
 * @returns 更新错误
 */
func (s *DepartmentService) Order(ctx context.Context, items []dto.DepartmentOrderItem) error {
	if err := requireRelationScope(ctx); err != nil {
		return err
	}

	departmentIDs := make([]int64, len(items))
	for index, item := range items {
		departmentIDs[index] = item.ID
	}
	return s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		lockIDs := append([]int64{}, departmentIDs...)
		for _, item := range items {
			if item.ParentID != nil && *item.ParentID > 0 {
				if *item.ParentID == item.ID {
					return exception.Validate("上级部门不能是自身")
				}
				lockIDs = append(lockIDs, *item.ParentID)
			}
		}
		departments, queryErr := s.lockScopedDepartments(ctx, tx, lockIDs)
		if queryErr != nil {
			return queryErr
		}
		for _, item := range items {
			if _, ok := departments[item.ID]; !ok {
				return exception.Comm("部门不存在")
			}
			if item.ParentID != nil && *item.ParentID > 0 {
				if _, ok := departments[*item.ParentID]; !ok {
					return exception.Comm("上级部门不存在")
				}
			}
		}
		for _, item := range items {
			updateQuery, modelErr := tenant.ScopedModel(ctx, tx, s.Model, "")
			if modelErr != nil {
				return modelErr
			}
			if _, modelErr = updateQuery.Where("id", item.ID).Data(departmentOrderRow{
				ParentID: item.ParentID,
				OrderNum: item.OrderNum,
			}).Update(); modelErr != nil {
				return gerror.Wrap(modelErr, "更新部门排序失败")
			}
		}
		return nil
	})
}

// 按固定顺序锁定当前作用域的部门
func (s *DepartmentService) lockScopedDepartments(ctx context.Context, provider tenant.ModelProvider, ids []int64) (map[int64]struct{}, error) {
	uniqueIDs := normalizeAuthorizationUserIDs(ids)
	query, err := tenant.ScopedModel(ctx, provider, s.Model, "")
	if err != nil {
		return nil, err
	}
	rows, err := query.Fields("id").WhereIn("id", uniqueIDs).OrderAsc("id").LockUpdate().All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询部门失败")
	}
	result := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		result[row["id"].Int64()] = struct{}{}
	}
	return result, nil
}
