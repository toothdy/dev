package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
)

// 系统角色服务
type RoleService struct {
	*service.Base
	Sessions            security.SessionStore
	recycle             *recycle.Manager
	userRoleModel       entity.Definition
	roleMenuModel       entity.Definition
	roleDepartmentModel entity.Definition
}

type roleMutationRow struct {
	UserID           interface{} `orm:"userId"`
	Name             interface{} `orm:"name"`
	Label            interface{} `orm:"label"`
	Remark           interface{} `orm:"remark"`
	Relevance        interface{} `orm:"relevance"`
	MenuIDList       interface{} `orm:"menuIdList"`
	DepartmentIDList interface{} `orm:"departmentIdList"`
	TenantID         interface{} `orm:"tenantId"`
	CreateTime       interface{} `orm:"createTime"`
	UpdateTime       interface{} `orm:"updateTime"`
}

type roleMenuMutationRow struct {
	RoleID int64 `orm:"roleId"`
	MenuID int64 `orm:"menuId"`
}

type roleDepartmentMutationRow struct {
	RoleID       int64 `orm:"roleId"`
	DepartmentID int64 `orm:"departmentId"`
}

var roleMutationFields = []string{"id", "userId", "name", "label", "remark", "relevance", "menuIdList", "departmentIdList", "tenantId"}

/**
 * 创建系统角色服务
 * @param db 数据库实例
 * @param baseSysRoleModel 角色模型
 * @param baseSysUserRoleModel 用户角色关系模型
 * @param baseSysRoleMenuModel 角色菜单关系模型
 * @param baseSysRoleDepartmentModel 角色部门关系模型
 * @param sessions 会话存储
 * @param recycleManager 回收站协调器
 * @returns *RoleService
 */
func NewRoleService(
	db gdb.DB,
	baseSysRoleModel entity.Definition,
	baseSysUserRoleModel entity.Definition,
	baseSysRoleMenuModel entity.Definition,
	baseSysRoleDepartmentModel entity.Definition,
	sessions security.SessionStore,
	recycleManager *recycle.Manager,
) *RoleService {
	return &RoleService{
		Base:         service.NewBase(db, baseSysRoleModel),
		Sessions:            sessions,
		recycle:             recycleManager,
		userRoleModel:       baseSysUserRoleModel,
		roleMenuModel:       baseSysRoleMenuModel,
		roleDepartmentModel: baseSysRoleDepartmentModel,
	}
}

// 新增角色并维护权限关系
func (s *RoleService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	applyTenantMutation(ctx, request.Data)
	if currentUser, ok := security.UserFromContext(ctx); ok {
		request.Data["userId"] = currentUser.UserId
	}
	if _, ok := request.Data["relevance"]; !ok {
		request.Data["relevance"] = false
	}
	if err := validateRoleMutation(request.Data); err != nil {
		return nil, err
	}
	if _, err := requiredRoleString(request.Data, "name"); err != nil {
		return nil, err
	}
	if label, ok := optionalRoleString(request.Data, "label"); ok {
		if err := s.ensureLabelUnique(ctx, label, 0); err != nil {
			return nil, err
		}
	}
	menuIDs, err := parseRoleIDs(request.Data["menuIdList"])
	if err != nil {
		return nil, err
	}
	departmentIDs, err := parseRoleIDs(request.Data["departmentIdList"])
	if err != nil {
		return nil, err
	}
	row, err := roleRowFromData(request.Data, menuIDs, departmentIDs)
	if err != nil {
		return nil, err
	}
	if label, present := optionalRoleString(request.Data, "label"); present &&
		isProtectedAuthorizationRole(label, request.Data["tenantId"]) {
		return nil, exception.Forbidden("非法操作")
	}
	now := mutationTimestamp()
	row.CreateTime = now
	row.UpdateTime = now
	var insertedID int64
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if relationErr := s.validateRoleRelations(ctx, tx, menuIDs, departmentIDs); relationErr != nil {
			return relationErr
		}
		insertedID, err = tx.Model(s.Model.TableName).Ctx(ctx).Data(row).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "新增角色失败")
		}
		return replaceRoleRelations(ctx, tx, insertedID, menuIDs, departmentIDs)
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": insertedID}, nil
}

// 修改角色并维护权限关系
func (s *RoleService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	delete(request.Data, "tenantId")
	if err := validateRoleMutation(request.Data); err != nil {
		return nil, err
	}
	id, err := requiredRoleID(request.Data)
	if err != nil {
		return nil, err
	}
	if label, ok := optionalRoleString(request.Data, "label"); ok {
		if err = s.ensureLabelUnique(ctx, label, id); err != nil {
			return nil, err
		}
	}
	menuIDs, menuPresent := request.Data["menuIdList"]
	departmentIDs, departmentPresent := request.Data["departmentIdList"]
	parsedMenus := []int64{}
	parsedDepartments := []int64{}
	if menuPresent {
		parsedMenus, err = parseRoleIDs(menuIDs)
		if err != nil {
			return nil, err
		}
	}
	if departmentPresent {
		parsedDepartments, err = parseRoleIDs(departmentIDs)
		if err != nil {
			return nil, err
		}
	}
	values, err := roleUpdateData(request.Data, parsedMenus, parsedDepartments)
	if err != nil {
		return nil, err
	}
	values["updateTime"] = mutationTimestamp()
	authorizationChanged := menuPresent || departmentPresent
	if _, present := request.Data["label"]; present {
		authorizationChanged = true
	}
	if _, present := request.Data["relevance"]; present {
		authorizationChanged = true
	}
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		currentQuery := tx.Model(s.Model.TableName).Ctx(ctx).
			Fields("id", "label", "tenantId").
			Where("id", id)
		if tenantID, ok := contextTenantID(ctx); ok {
			currentQuery = currentQuery.Where("tenantId", tenantID)
		}
		current, currentErr := currentQuery.LockUpdate().One()
		if currentErr != nil {
			return gerror.Wrap(currentErr, "查询角色失败")
		}
		if current.IsEmpty() {
			return exception.Comm("角色不存在")
		}
		if isProtectedAuthorizationRole(current["label"].String(), current["tenantId"].Val()) {
			return exception.Forbidden("非法操作")
		}
		if label, present := optionalRoleString(request.Data, "label"); present &&
			isProtectedAuthorizationRole(label, current["tenantId"].Val()) {
			return exception.Forbidden("非法操作")
		}
		userIDs, usersErr := roleUserIDsForScope(ctx, tx, []int64{id})
		if usersErr != nil {
			return usersErr
		}
		if lockErr := lockAuthorizationUsers(ctx, tx, userIDs); lockErr != nil {
			return lockErr
		}
		if menuPresent || departmentPresent {
			var relationErr error
			if !menuPresent {
				parsedMenus, relationErr = relationIDs(ctx, tx, "base_sys_role_menu", "menuId", id)
			}
			if relationErr == nil && !departmentPresent {
				parsedDepartments, relationErr = relationIDs(ctx, tx, "base_sys_role_department", "departmentId", id)
			}
			if relationErr != nil {
				return relationErr
			}
			if relationErr = s.validateRoleRelations(ctx, tx, parsedMenus, parsedDepartments); relationErr != nil {
				return relationErr
			}
		}
		if _, updateErr := tx.Model(s.Model.TableName).Ctx(ctx).Where("id", id).Data(values).Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "修改角色失败")
		}
		if menuPresent || departmentPresent {
			if relationErr := replaceRoleRelations(ctx, tx, id, parsedMenus, parsedDepartments); relationErr != nil {
				return relationErr
			}
		}
		if authorizationChanged {
			return revokeAuthorizationSessions(ctx, s.Sessions, userIDs, "使角色用户登录会话失效失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// 删除角色及关联关系
func (s *RoleService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	roleIDs, err := parseRoleIDs(request.IDs)
	if err != nil || len(roleIDs) == 0 {
		return nil, exception.Validate("删除ID不能为空")
	}
	roleIDs = normalizeAuthorizationUserIDs(roleIDs)
	requestIDs := make([]interface{}, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		requestIDs = append(requestIDs, roleID)
	}
	var userIDsToRevoke []int64
	err = runManagedDelete(ctx, s.DB, s.recycle, s.Model, requestIDs, request, func(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope) error {
		roleQuery := tx.Model(s.Model.TableName).Ctx(ctx).
			Fields("id", "label", "tenantId").
			WhereIn("id", roleIDs).
			OrderAsc("id")
		if tenantID, ok := contextTenantID(ctx); ok {
			roleQuery = roleQuery.Where("tenantId", tenantID)
		}
		roles, queryErr := roleQuery.LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询角色失败")
		}
		if len(roles) != len(roleIDs) {
			return exception.Comm("角色不存在")
		}
		for _, role := range roles {
			if isProtectedAuthorizationRole(role["label"].String(), role["tenantId"].Val()) {
				return exception.Forbidden("非法操作")
			}
		}
		userIDs, usersErr := roleUserIDsForScope(ctx, tx, roleIDs)
		if usersErr != nil {
			return usersErr
		}
		if lockErr := lockAuthorizationUsers(ctx, tx, userIDs); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Comm("用户不存在")
			}
			return lockErr
		}
		userIDsToRevoke = append([]int64{}, userIDs...)
		relations := []entity.Definition{s.userRoleModel, s.roleMenuModel, s.roleDepartmentModel}
		for _, relation := range relations {
			relationQuery := tx.Model(relation.TableName).Ctx(ctx).WhereIn("roleId", roleIDs)
			if scope != nil && scope.IsArchiving() {
				rows, relationErr := relationQuery.Clone().OrderAsc("roleId").LockUpdate().All()
				if relationErr != nil {
					return gerror.Wrap(relationErr, "锁定角色关系失败")
				}
				for _, row := range rows {
					roleID := row["roleId"].Int64()
					parentKey, exists := scope.RootKey(roleID)
					if !exists {
						return gerror.Newf("角色 %d 缺少回收站根归档项", roleID)
					}
					if _, addErr := scope.AddRecord(relation, row.Map(), recycle.ItemOptions{
						BranchKey: strconv.FormatInt(roleID, 10), ParentKey: parentKey, RestoreOrder: 10,
					}); addErr != nil {
						return addErr
					}
				}
			}
			result, deleteErr := relationQuery.Delete()
			if deleteErr != nil {
				return gerror.Wrap(deleteErr, "删除角色关系失败")
			}
			if markErr := markManagedDeleted(scope, result, "读取角色关系删除数量失败"); markErr != nil {
				return markErr
			}
		}
		result, deleteErr := tx.Model(s.Model.TableName).Ctx(ctx).WhereIn("id", roleIDs).Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除角色失败")
		}
		if markErr := markManagedDeleted(scope, result, "读取角色删除数量失败"); markErr != nil {
			return markErr
		}
		if scope != nil {
			return scope.AfterCommit(func(actionCtx context.Context) error {
				return revokeAuthorizationSessions(actionCtx, s.Sessions, userIDsToRevoke, "使角色用户登录会话失效失败")
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.recycle == nil {
		err = revokeAuthorizationSessions(ctx, s.Sessions, userIDsToRevoke, "使角色用户登录会话失效失败")
	}
	return nil, err
}

// 查询角色及菜单、部门权限
func (s *RoleService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	where := "id = ?"
	args := []interface{}{request.ID}
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND tenantId = ?"
		args = append(args, tenantID)
	}
	row, err := s.DB.GetOne(ctx, "SELECT "+roleSelectColumns+" FROM base_sys_role WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色详情失败")
	}
	if len(row) == 0 {
		return map[string]interface{}{}, nil
	}
	id := row["id"].Int64()
	menuSQL := "SELECT rm.menuId AS id FROM base_sys_role_menu rm INNER JOIN base_sys_menu m ON m.id = rm.menuId WHERE rm.roleId = ?"
	departmentSQL := "SELECT rd.departmentId AS id FROM base_sys_role_department rd INNER JOIN base_sys_department d ON d.id = rd.departmentId WHERE rd.roleId = ?"
	relationArgs := []interface{}{id}
	if isProtectedAuthorizationRole(row["label"].String(), row["tenantId"].Val()) {
		menuSQL = "SELECT DISTINCT rm.menuId AS id FROM base_sys_role_menu rm INNER JOIN base_sys_menu m ON m.id = rm.menuId"
		departmentSQL = "SELECT DISTINCT rd.departmentId AS id FROM base_sys_role_department rd INNER JOIN base_sys_department d ON d.id = rd.departmentId"
		relationArgs = nil
	} else if tenantID, ok := contextTenantID(ctx); ok {
		menuSQL += " AND m.tenantId = ?"
		departmentSQL += " AND d.tenantId = ?"
		relationArgs = append(relationArgs, tenantID)
	}
	menuSQL += " ORDER BY id"
	departmentSQL += " ORDER BY id"
	menus, err := s.DB.GetAll(ctx, menuSQL, relationArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色菜单失败")
	}
	departments, err := s.DB.GetAll(ctx, departmentSQL, relationArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色部门失败")
	}
	info := row.Map()
	info["menuIdList"] = recordIDs(menus)
	info["departmentIdList"] = recordIDs(departments)
	return normalizeRoleResponse(info), nil
}

func (s *RoleService) ensureLabelUnique(ctx context.Context, label string, excludeID int64) error {
	query := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("label", label)
	if excludeID > 0 {
		query = query.WhereNot("id", excludeID)
	}
	count, err := query.Count()
	if err != nil {
		return gerror.Wrap(err, "查询角色标识失败")
	}
	if count > 0 {
		return exception.Comm("角色标识已经存在~")
	}
	return nil
}

func roleRowFromData(data map[string]interface{}, menuIDs []int64, departmentIDs []int64) (roleMutationRow, error) {
	row := roleMutationRow{UserID: data["userId"], Name: data["name"], Label: data["label"], Remark: data["remark"], Relevance: data["relevance"], TenantID: data["tenantId"]}
	if value, ok := data["label"]; ok && value == nil {
		row.Label = gdb.Raw("NULL")
	}
	if _, ok := data["menuIdList"]; ok {
		value, err := roleIDsJSON(menuIDs)
		if err != nil {
			return row, err
		}
		row.MenuIDList = value
	}
	if _, ok := data["departmentIdList"]; ok {
		value, err := roleIDsJSON(departmentIDs)
		if err != nil {
			return row, err
		}
		row.DepartmentIDList = value
	}
	return row, nil
}

func roleUpdateData(data map[string]interface{}, menuIDs []int64, departmentIDs []int64) (map[string]interface{}, error) {
	columns := map[string]string{
		"userId":    "userId",
		"name":      "name",
		"label":     "label",
		"remark":    "remark",
		"relevance": "relevance",
	}
	values := make(map[string]interface{}, len(data))
	for field, column := range columns {
		if value, ok := data[field]; ok {
			values[column] = value
		}
	}
	if _, ok := data["menuIdList"]; ok {
		encoded, err := roleIDsJSON(menuIDs)
		if err != nil {
			return nil, err
		}
		values["menuIdList"] = encoded
	}
	if _, ok := data["departmentIdList"]; ok {
		encoded, err := roleIDsJSON(departmentIDs)
		if err != nil {
			return nil, err
		}
		values["departmentIdList"] = encoded
	}
	return values, nil
}

func replaceRoleRelations(ctx context.Context, tx gdb.TX, roleID int64, menuIDs []int64, departmentIDs []int64) error {
	if _, err := tx.Model("base_sys_role_menu").Ctx(ctx).Where("roleId", roleID).Delete(); err != nil {
		return gerror.Wrap(err, "更新角色菜单失败")
	}
	if len(menuIDs) > 0 {
		rows := make([]roleMenuMutationRow, 0, len(menuIDs))
		for _, menuID := range menuIDs {
			rows = append(rows, roleMenuMutationRow{RoleID: roleID, MenuID: menuID})
		}
		if _, err := tx.Model("base_sys_role_menu").Ctx(ctx).Data(rows).Insert(); err != nil {
			return gerror.Wrap(err, "更新角色菜单失败")
		}
	}
	if _, err := tx.Model("base_sys_role_department").Ctx(ctx).Where("roleId", roleID).Delete(); err != nil {
		return gerror.Wrap(err, "更新角色部门失败")
	}
	if len(departmentIDs) > 0 {
		rows := make([]roleDepartmentMutationRow, 0, len(departmentIDs))
		for _, departmentID := range departmentIDs {
			rows = append(rows, roleDepartmentMutationRow{RoleID: roleID, DepartmentID: departmentID})
		}
		if _, err := tx.Model("base_sys_role_department").Ctx(ctx).Data(rows).Insert(); err != nil {
			return gerror.Wrap(err, "更新角色部门失败")
		}
	}
	return nil
}

func relationIDs(ctx context.Context, tx gdb.TX, table string, column string, roleID int64) ([]int64, error) {
	rows, err := tx.Model(table).Ctx(ctx).Fields(column).Where("roleId", roleID).All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色关系失败")
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		result = append(result, row[column].Int64())
	}
	return result, nil
}

func (s *RoleService) validateRoleRelations(ctx context.Context, db authorizationModeler, menuIDs []int64, departmentIDs []int64) error {
	tenantID, scoped := contextTenantID(ctx)
	checks := []struct {
		table string
		name  string
		ids   []int64
	}{
		{table: "base_sys_menu", name: "菜单", ids: menuIDs},
		{table: "base_sys_department", name: "部门", ids: departmentIDs},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		query := db.Model(check.table).Ctx(ctx).
			Fields("id").
			WhereIn("id", check.ids).
			OrderAsc("id")
		if scoped {
			query = query.Where("tenantId", tenantID)
		}
		rows, err := query.LockUpdate().All()
		if err != nil {
			return gerror.Wrapf(err, "校验角色%s归属失败", check.name)
		}
		if len(rows) != len(check.ids) {
			return exception.Comm("角色关系包含无权资源")
		}
	}
	return nil
}

func roleUserIDsForScope(ctx context.Context, tx gdb.TX, roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return []int64{}, nil
	}
	query := tx.Model("base_sys_user_role ur").Ctx(ctx).
		Fields("DISTINCT ur.userId").
		InnerJoin("base_sys_user u", "u.id = ur.userId").
		WhereIn("ur.roleId", roleIDs).
		OrderAsc("ur.userId")
	if tenantID, ok := contextTenantID(ctx); ok {
		query = query.Where("u.tenantId", tenantID)
	}
	rows, err := query.All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色用户失败")
	}
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row["userId"].Int64())
	}
	return normalizeAuthorizationUserIDs(userIDs), nil
}

func recordIDs(rows gdb.Result) []int64 {
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		result = append(result, row["id"].Int64())
	}
	return result
}

func validateRoleMutation(data map[string]interface{}) error {
	allowed := make(map[string]struct{}, len(roleMutationFields))
	for _, field := range roleMutationFields {
		allowed[field] = struct{}{}
	}
	for field := range data {
		if _, ok := allowed[field]; !ok {
			return exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	return nil
}

func requiredRoleString(data map[string]interface{}, field string) (string, error) {
	value := strings.TrimSpace(fmt.Sprint(data[field]))
	if value == "" || value == "<nil>" {
		return "", exception.Validate(fmt.Sprintf("%s不能为空", field))
	}
	return value, nil
}

func optionalRoleString(data map[string]interface{}, field string) (string, bool) {
	value, ok := data[field]
	if !ok || value == nil {
		return "", false
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	return text, text != ""
}

func requiredRoleID(data map[string]interface{}) (int64, error) {
	value, ok := data["id"]
	if !ok {
		return 0, exception.Validate("id不能为空")
	}
	id, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, exception.Validate("id参数错误")
	}
	return id, nil
}

func parseRoleIDs(value interface{}) ([]int64, error) {
	if value == nil {
		return []int64{}, nil
	}
	values, ok := value.([]interface{})
	if !ok {
		switch typed := value.(type) {
		case []int64:
			return append([]int64{}, typed...), nil
		case []int:
			values = make([]interface{}, len(typed))
			for index, item := range typed {
				values[index] = item
			}
		case string:
			if strings.TrimSpace(typed) == "" {
				return []int64{}, nil
			}
			parts := strings.Split(typed, ",")
			values = make([]interface{}, len(parts))
			for index, item := range parts {
				values[index] = strings.TrimSpace(item)
			}
		default:
			values = []interface{}{typed}
		}
	}
	result := make([]int64, 0, len(values))
	seen := map[int64]struct{}{}
	for _, value := range values {
		id, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		if err != nil || id <= 0 {
			return nil, exception.Validate("ID列表参数错误")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func roleIDsJSON(ids []int64) (string, error) {
	encoded, err := json.Marshal(ids)
	if err != nil {
		return "", gerror.Wrap(err, "序列化ID列表失败")
	}
	return string(encoded), nil
}

const roleSelectColumns = "id, userId AS userId, name, label, remark, relevance, menuIdList AS menuIdList, departmentIdList AS departmentIdList, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId"

/**
 * 构建角色查询条件
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 条件与参数
 */
func (s *RoleService) roleWhere(ctx context.Context, request crud.QueryRequest) (string, []interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return "", nil, err
	}
	where := "NOT (label = 'admin' AND (tenantId IS NULL OR tenantId = 0))"
	args := []interface{}{}
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND tenantId = ?"
		args = append(args, tenantID)
	}
	if request.Keyword != "" {
		where += " AND (name LIKE ? OR label LIKE ?)"
		args = append(args, "%"+request.Keyword+"%", "%"+request.Keyword+"%")
	}
	currentUser, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return "", nil, err
	}
	isAdmin, err := isPlatformAdministrator(ctx, s.DB, currentUser.UserId)
	if err != nil {
		return "", nil, err
	}
	if !isAdmin {
		if len(currentUser.RoleIds) > 0 {
			where += " AND (userId = ? OR id IN (?))"
			args = append(args, currentUser.UserId, roleIDsToInterfaces(currentUser.RoleIds))
		} else {
			where += " AND userId = ?"
			args = append(args, currentUser.UserId)
		}
	}
	return where, args, nil
}

/**
 * 分页查询角色
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 分页结果
 */
func (s *RoleService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	request = crud.NormalizePageRequest(request)
	where, args, err := s.roleWhere(ctx, request)
	if err != nil {
		return nil, err
	}

	total, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_role WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色总数失败")
	}

	orderBy, err := pageOrderBy(request, map[string]string{
		"id": "id", "createTime": "createTime", "updateTime": "updateTime", "name": "name",
	}, "id", "DESC")
	if err != nil {
		return nil, err
	}
	limitSQL, limitArgs := pageLimit(request)
	listArgs := append(append([]interface{}{}, args...), limitArgs...)
	rows, err := s.DB.GetAll(ctx, "SELECT "+roleSelectColumns+" FROM base_sys_role WHERE "+where+orderBy+limitSQL, listArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色列表失败")
	}
	list := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		list = append(list, normalizeRolePageResponse(row.Map()))
	}
	return crud.PageResult{
		List:       list,
		Pagination: crud.Pagination{Page: request.Page, Size: request.Size, Total: total},
	}, nil
}

func normalizeRolePageResponse(item map[string]interface{}) map[string]interface{} {
	item["menuIdList"] = normalizeRoleIDList(item["menuIdList"])
	item["departmentIdList"] = normalizeRoleIDList(item["departmentIdList"])
	return item
}

/**
 * 列表查询角色
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 角色列表
 */
func (s *RoleService) List(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	where := "NOT (label = 'admin' AND (tenantId IS NULL OR tenantId = 0))"
	args := []interface{}{}
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND tenantId = ?"
		args = append(args, tenantID)
	}
	currentUser, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return nil, err
	}
	isAdmin, err := isPlatformAdministrator(ctx, s.DB, currentUser.UserId)
	if err != nil {
		return nil, err
	}
	if !isAdmin {
		if len(currentUser.RoleIds) > 0 {
			where += " AND (userId = ? OR id IN (?))"
			args = append(args, currentUser.UserId, roleIDsToInterfaces(currentUser.RoleIds))
		} else {
			where += " AND userId = ?"
			args = append(args, currentUser.UserId)
		}
	}
	rows, err := s.DB.GetAll(ctx, "SELECT "+roleSelectColumns+" FROM base_sys_role WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询角色列表失败")
	}
	list := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		list = append(list, normalizeRoleResponse(row.Map()))
	}
	return list, nil
}

func normalizeRoleResponse(item map[string]interface{}) map[string]interface{} {
	item["relevance"] = booleanValue(item["relevance"])
	item["menuIdList"] = normalizeRoleIDList(item["menuIdList"])
	item["departmentIdList"] = normalizeRoleIDList(item["departmentIdList"])
	return item
}

func normalizeRoleIDList(value interface{}) []int64 {
	if encoded, ok := value.([]byte); ok {
		value = string(encoded)
	}
	if encoded, ok := value.(string); ok {
		var decoded interface{}
		if json.Unmarshal([]byte(encoded), &decoded) != nil {
			return []int64{}
		}
		value = decoded
	}
	ids, err := parseRoleIDs(value)
	if err != nil {
		return []int64{}
	}
	return ids
}

/**
 * 角色ID切片转 interface 切片
 * @param ids 角色 ID 列表
 * @returns interface 切片
 */
func roleIDsToInterfaces(ids []int64) []interface{} {
	result := make([]interface{}, len(ids))
	for index, id := range ids {
		result[index] = id
	}
	return result
}

func requireRelationScope(ctx context.Context) error {
	if tenant.Resolve(ctx).Kind() == tenant.KindMissing {
		return exception.Unauthorized()
	}
	return nil
}
