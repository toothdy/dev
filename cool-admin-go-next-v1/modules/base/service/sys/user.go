package sys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

// 系统用户服务
type UserService struct {
	*service.Base
	Sessions      security.SessionStore
	recycle       *recycle.Manager
	userRoleModel entity.Definition
}

type userMutationRow struct {
	DepartmentID interface{} `orm:"departmentId"`
	UserID       interface{} `orm:"userId"`
	Name         interface{} `orm:"name"`
	Username     interface{} `orm:"username"`
	Password     interface{} `orm:"password"`
	PasswordV    interface{} `orm:"passwordV"`
	NickName     interface{} `orm:"nickName"`
	HeadImg      interface{} `orm:"headImg"`
	Phone        interface{} `orm:"phone"`
	Email        interface{} `orm:"email"`
	Remark       interface{} `orm:"remark"`
	Status       interface{} `orm:"status"`
	SocketID     interface{} `orm:"socketId"`
	TenantID     interface{} `orm:"tenantId"`
	CreateTime   interface{} `orm:"createTime"`
	UpdateTime   interface{} `orm:"updateTime"`
}

type userRoleMutationRow struct {
	UserID int64 `orm:"userId"`
	RoleID int64 `orm:"roleId"`
}

var userMutationFields = []string{"id", "departmentId", "userId", "name", "username", "password", "passwordV", "nickName", "headImg", "phone", "email", "remark", "status", "socketId", "tenantId", "roleIdList"}

/**
 * 创建系统用户服务
 * @param db 数据库实例
 * @param baseSysUserModel 用户模型
 * @param baseSysUserRoleModel 用户角色关系模型
 * @param sessions 会话存储
 * @param recycleManager 回收站协调器
 * @returns *UserService
 */
func NewUserService(
	db gdb.DB,
	baseSysUserModel entity.Definition,
	baseSysUserRoleModel entity.Definition,
	sessions security.SessionStore,
	recycleManager *recycle.Manager,
) *UserService {
	return &UserService{
		Base:          service.NewBase(db, baseSysUserModel),
		Sessions:      sessions,
		recycle:       recycleManager,
		userRoleModel: baseSysUserRoleModel,
	}
}

// 获得当前用户信息并移除密码
func (s *UserService) Person(ctx context.Context, userID int64) (map[string]interface{}, error) {
	record, err := s.DB.GetOne(ctx, "SELECT id, departmentId AS departmentId, userId AS userId, name, username, password, passwordV AS passwordV, nickName AS nickName, headImg AS headImg, phone, email, remark, status, socketId AS socketId, createTime AS createTime, updateTime AS updateTime, tenantId AS tenantId FROM base_sys_user WHERE id = ? LIMIT 1", userID)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户失败")
	}
	if len(record) == 0 {
		return nil, exception.Unauthorized()
	}
	return FilterUserPassword(record.Map()), nil
}

// 过滤用户密码
func FilterUserPassword(user map[string]interface{}) map[string]interface{} {
	filtered := map[string]interface{}{}
	for key, value := range user {
		if key != "password" {
			filtered[key] = value
		}
	}
	return filtered
}

// 新增用户并维护角色关系
func (s *UserService) Add(ctx context.Context, request crud.AddRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	applyTenantMutation(ctx, request.Data)
	caller, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return nil, err
	}
	request.Data["userId"] = caller.UserId
	if err := validateUserMutation(request.Data); err != nil {
		return nil, err
	}
	username, err := requiredUserString(request.Data, "username")
	if err != nil {
		return nil, err
	}
	password, err := requiredUserString(request.Data, "password")
	if err != nil {
		return nil, err
	}
	roleIDs, err := parseUserRoleIDs(request.Data["roleIdList"])
	if err != nil {
		return nil, err
	}
	if err = s.validateUserDepartment(ctx, request.Data["departmentId"]); err != nil {
		return nil, err
	}
	exists, err := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("username", username).Count()
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户名失败")
	}
	if exists > 0 {
		return nil, exception.Comm("用户名已经存在~")
	}
	row := userRowFromData(request.Data)
	now := mutationTimestamp()
	row.CreateTime = now
	row.UpdateTime = now
	row.Username = username
	row.Password, err = security.HashPassword(password)
	if err != nil {
		return nil, gerror.Wrap(err, "生成密码摘要失败")
	}
	if row.PasswordV == nil {
		row.PasswordV = 1
	}
	var insertedID int64
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if authErr := ensureUserMutationAllowed(ctx, tx, caller.UserId, authorizationUser{
			TenantID: request.Data["tenantId"],
		}, roleIDs, true); authErr != nil {
			return authErr
		}
		insertedID, err = tx.Model(s.Model.TableName).Ctx(ctx).Data(row).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "新增用户失败")
		}
		return replaceUserRoles(ctx, tx, insertedID, roleIDs)
	})
	if err != nil {
		return nil, err
	}
	return insertedID, nil
}

// 修改用户并维护角色关系
func (s *UserService) Update(ctx context.Context, request crud.UpdateRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	delete(request.Data, "tenantId")
	caller, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateUserMutation(request.Data); err != nil {
		return nil, err
	}
	id, err := requiredUserID(request.Data)
	if err != nil {
		return nil, err
	}
	if usernameValue, ok := request.Data["username"]; ok {
		username := strings.TrimSpace(fmt.Sprint(usernameValue))
		if username == "" {
			return nil, exception.Validate("username不能为空")
		}
		count, queryErr := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("username", username).WhereNot("id", id).Count()
		if queryErr != nil {
			return nil, gerror.Wrap(queryErr, "查询用户名失败")
		}
		if count > 0 {
			return nil, exception.Comm("用户名已经存在~")
		}
	}
	values := userUpdateData(request.Data)
	values["updateTime"] = mutationTimestamp()
	passwordChanged := false
	if password, ok := request.Data["password"]; ok {
		value := strings.TrimSpace(fmt.Sprint(password))
		if value == "" {
			delete(values, "password")
		} else {
			values["password"], err = security.HashPassword(value)
			if err != nil {
				return nil, gerror.Wrap(err, "生成密码摘要失败")
			}
			passwordChanged = true
		}
	}
	parsedRoleIDs, roleIDsPresent, err := optionalUserRoleIDs(request.Data)
	if err != nil {
		return nil, err
	}
	if departmentID, present := request.Data["departmentId"]; present {
		if err = s.validateUserDepartment(ctx, departmentID); err != nil {
			return nil, err
		}
	}
	authorizationChanged := passwordChanged || roleIDsPresent
	if _, present := request.Data["username"]; present {
		authorizationChanged = true
	}
	if _, present := request.Data["status"]; present {
		authorizationChanged = true
	}
	err = s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if lockErr := lockAuthorizationUsers(ctx, tx, []int64{id}); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Comm("用户不存在")
			}
			return lockErr
		}
		target, targetErr := authorizationUserFromDatabase(ctx, tx, id)
		if targetErr != nil {
			return targetErr
		}
		if authErr := ensureUserMutationAllowed(ctx, tx, caller.UserId, target, parsedRoleIDs, roleIDsPresent); authErr != nil {
			return authErr
		}
		if passwordChanged {
			current, currentErr := tx.Model(s.Model.TableName).Ctx(ctx).Fields("passwordV").Where("id", id).One()
			if currentErr != nil {
				return gerror.Wrap(currentErr, "查询用户失败")
			}
			values["passwordV"] = current["passwordV"].Int64() + 1
		}
		if _, updateErr := tx.Model(s.Model.TableName).Ctx(ctx).Where("id", id).Data(values).Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "修改用户失败")
		}
		if roleIDsPresent {
			if roleErr := replaceUserRoles(ctx, tx, id, parsedRoleIDs); roleErr != nil {
				return roleErr
			}
		}
		if authorizationChanged {
			return revokeAuthorizationSessions(ctx, s.Sessions, []int64{id}, "使用户登录会话失效失败")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// 删除用户及角色关系
func (s *UserService) Delete(ctx context.Context, request crud.DeleteRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	userIDs, err := parseUserIDs(request.IDs, "删除ID参数错误")
	if err != nil || len(userIDs) == 0 {
		return nil, exception.Validate("删除ID不能为空")
	}
	caller, err := requireAuthorizationCaller(ctx)
	if err != nil {
		return nil, err
	}
	err = runManagedDelete(ctx, s.DB, s.recycle, s.Model, request.IDs, request, func(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope) error {
		if lockErr := lockAuthorizationUsers(ctx, tx, userIDs); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Comm("用户不存在")
			}
			return lockErr
		}
		for _, userID := range userIDs {
			target, targetErr := authorizationUserFromDatabase(ctx, tx, userID)
			if targetErr != nil {
				return targetErr
			}
			if authErr := ensureUserMutationAllowed(ctx, tx, caller.UserId, target, nil, false); authErr != nil {
				return authErr
			}
		}
		roleQuery := tx.Model(s.userRoleModel.TableName).Ctx(ctx).WhereIn("userId", userIDs)
		if scope != nil && scope.IsArchiving() {
			rows, queryErr := roleQuery.Clone().OrderAsc("userId").OrderAsc("roleId").LockUpdate().All()
			if queryErr != nil {
				return gerror.Wrap(queryErr, "锁定用户角色失败")
			}
			for _, row := range rows {
				userID := row["userId"].Int64()
				parentKey, exists := scope.RootKey(userID)
				if !exists {
					return gerror.Newf("用户 %d 缺少回收站根归档项", userID)
				}
				if _, addErr := scope.AddRecord(s.userRoleModel, row.Map(), recycle.ItemOptions{
					BranchKey: strconv.FormatInt(userID, 10), ParentKey: parentKey, RestoreOrder: 10,
				}); addErr != nil {
					return addErr
				}
			}
		}
		roleResult, deleteErr := roleQuery.Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除用户角色失败")
		}
		if markErr := markManagedDeleted(scope, roleResult, "读取用户角色删除数量失败"); markErr != nil {
			return markErr
		}
		userResult, deleteErr := tx.Model(s.Model.TableName).Ctx(ctx).WhereIn("id", userIDs).Delete()
		if deleteErr != nil {
			return gerror.Wrap(deleteErr, "删除用户失败")
		}
		if markErr := markManagedDeleted(scope, userResult, "读取用户删除数量失败"); markErr != nil {
			return markErr
		}
		if scope != nil {
			return scope.AfterCommit(func(actionCtx context.Context) error {
				return revokeAuthorizationSessions(actionCtx, s.Sessions, userIDs, "使被删除用户的登录会话失效失败")
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if s.recycle == nil {
		if err = revokeAuthorizationSessions(ctx, s.Sessions, userIDs, "使被删除用户的登录会话失效失败"); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func userRowFromData(data map[string]interface{}) userMutationRow {
	return userMutationRow{
		DepartmentID: data["departmentId"], UserID: data["userId"], Name: data["name"], Username: data["username"],
		Password: data["password"], PasswordV: data["passwordV"], NickName: data["nickName"], HeadImg: data["headImg"],
		Phone: data["phone"], Email: data["email"], Remark: data["remark"], Status: data["status"], SocketID: data["socketId"], TenantID: data["tenantId"],
	}
}

func userUpdateData(data map[string]interface{}) map[string]interface{} {
	columns := map[string]string{
		"departmentId": "departmentId",
		"userId":       "userId",
		"name":         "name",
		"username":     "username",
		"password":     "password",
		"passwordV":    "passwordV",
		"nickName":     "nickName",
		"headImg":      "headImg",
		"phone":        "phone",
		"email":        "email",
		"remark":       "remark",
		"status":       "status",
		"socketId":     "socketId",
	}
	values := make(map[string]interface{}, len(data))
	for field, column := range columns {
		if value, ok := data[field]; ok {
			values[column] = value
		}
	}
	return values
}

func validateUserMutation(data map[string]interface{}) error {
	allowed := make(map[string]struct{}, len(userMutationFields))
	for _, field := range userMutationFields {
		allowed[field] = struct{}{}
	}
	for field := range data {
		if _, ok := allowed[field]; !ok {
			return exception.Validate(fmt.Sprintf("未知字段: %s", field))
		}
	}
	return nil
}

func requiredUserString(data map[string]interface{}, field string) (string, error) {
	value := strings.TrimSpace(fmt.Sprint(data[field]))
	if value == "" || value == "<nil>" {
		return "", exception.Validate(fmt.Sprintf("%s不能为空", field))
	}
	return value, nil
}

func requiredUserID(data map[string]interface{}) (int64, error) {
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

func parseUserRoleIDs(value interface{}) ([]int64, error) {
	return parseUserIDs(value, "ID列表参数错误")
}

func optionalUserRoleIDs(data map[string]interface{}) ([]int64, bool, error) {
	value, present := data["roleIdList"]
	if !present {
		return nil, false, nil
	}
	roleIDs, err := parseUserRoleIDs(value)
	return roleIDs, true, err
}

func parseUserIDs(value interface{}, message string) ([]int64, error) {
	if value == nil {
		return []int64{}, nil
	}
	values, ok := value.([]interface{})
	if !ok {
		switch typed := value.(type) {
		case []int64:
			result := make([]int64, len(typed))
			copy(result, typed)
			return result, nil
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
			return nil, exception.Validate(message)
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result, nil
}

func replaceUserRoles(ctx context.Context, tx gdb.TX, userID int64, roleIDs []int64) error {
	if _, err := tx.Model("base_sys_user_role").Ctx(ctx).Where("userId", userID).Delete(); err != nil {
		return gerror.Wrap(err, "更新用户角色失败")
	}
	if len(roleIDs) == 0 {
		return nil
	}
	rows := make([]userRoleMutationRow, 0, len(roleIDs))
	for _, roleID := range roleIDs {
		rows = append(rows, userRoleMutationRow{UserID: userID, RoleID: roleID})
	}
	if _, err := tx.Model("base_sys_user_role").Ctx(ctx).Data(rows).Insert(); err != nil {
		return gerror.Wrap(err, "更新用户角色失败")
	}
	return nil
}

func (s *UserService) validateUserDepartment(ctx context.Context, value interface{}) error {
	if value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" || fmt.Sprint(value) == "<nil>" {
		return nil
	}
	departmentID, err := strconv.ParseInt(fmt.Sprint(value), 10, 64)
	if err != nil || departmentID <= 0 {
		return exception.Validate("部门参数错误")
	}
	query := s.DB.Model("base_sys_department").Ctx(ctx).Where("id", departmentID)
	if tenantID, ok := contextTenantID(ctx); ok {
		query = query.Where("tenantId", tenantID)
	}
	count, err := query.Count()
	if err != nil {
		return gerror.Wrap(err, "校验部门归属失败")
	}
	if count != 1 {
		return exception.Comm("部门不存在")
	}
	return nil
}

/**
 * 更新用户个人信息
 * @param ctx 请求上下文
 * @param userID 用户 ID
 * @param request 个人信息更新请求
 * @returns 更新错误
 */
func (s *UserService) PersonUpdate(ctx context.Context, userID int64, request dto.PersonUpdateRequest) error {
	if userID <= 0 {
		return exception.Unauthorized()
	}

	oldPassword, password, passwordChanged, err := request.PasswordChange()
	if err != nil {
		return err
	}
	values := request.Values()
	if len(values) == 0 && !passwordChanged {
		return nil
	}
	values["updateTime"] = mutationTimestamp()
	return s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if lockErr := lockAuthorizationUsers(ctx, tx, []int64{userID}); lockErr != nil {
			if errors.Is(lockErr, errAuthorizationUserMissing) {
				return exception.Comm("用户不存在")
			}
			return lockErr
		}
		if passwordChanged {
			current, queryErr := tx.Model(s.Model.TableName).Ctx(ctx).
				Fields("password", "passwordV").
				Where("id", userID).
				One()
			if queryErr != nil {
				return gerror.Wrap(queryErr, "查询用户失败")
			}
			if !security.VerifyPassword(oldPassword, current["password"].String()) {
				return exception.Comm("原密码错误")
			}
			values["password"], err = security.HashPassword(password)
			if err != nil {
				return gerror.Wrap(err, "生成密码摘要失败")
			}
			values["passwordV"] = current["passwordV"].Int64() + 1
		}
		if _, updateErr := tx.Model(s.Model.TableName).Ctx(ctx).Where("id", userID).Data(values).Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "更新个人信息失败")
		}
		if passwordChanged {
			return revokeAuthorizationSessions(ctx, s.Sessions, []int64{userID}, "使用户登录会话失效失败")
		}
		return nil
	})
}

/**
 * 移动用户所属部门
 * @param ctx 请求上下文
 * @param request 用户移动部门请求
 * @returns 移动错误
 */
func (s *UserService) Move(ctx context.Context, request dto.MoveReq) error {
	if err := requireRelationScope(ctx); err != nil {
		return err
	}

	return s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		departmentQuery := tx.Model("base_sys_department").Ctx(ctx).Fields("id").Where("id", request.DepartmentID)
		if tenantID, ok := contextTenantID(ctx); ok {
			departmentQuery = departmentQuery.Where("tenantId", tenantID)
		}
		department, queryErr := departmentQuery.LockUpdate().One()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询部门失败")
		}
		if department.IsEmpty() {
			return exception.Comm("部门不存在")
		}
		userQuery := tx.Model(s.Model.TableName).Ctx(ctx).Fields("id").WhereIn("id", request.UserIDs).OrderAsc("id")
		if tenantID, ok := contextTenantID(ctx); ok {
			userQuery = userQuery.Where("tenantId", tenantID)
		}
		users, queryErr := userQuery.LockUpdate().All()
		if queryErr != nil {
			return gerror.Wrap(queryErr, "查询用户失败")
		}
		if len(users) != len(request.UserIDs) {
			return exception.Comm("用户不存在")
		}
		updateQuery := tx.Model(s.Model.TableName).Ctx(ctx).WhereIn("id", request.UserIDs)
		if tenantID, ok := contextTenantID(ctx); ok {
			updateQuery = updateQuery.Where("tenantId", tenantID)
		}
		if _, queryErr = updateQuery.Fields("departmentId").Data(userMutationRow{DepartmentID: request.DepartmentID}).Update(); queryErr != nil {
			return gerror.Wrap(queryErr, "移动用户失败")
		}
		return nil
	})
}

/**
 * 分页查询用户
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 分页结果
 */
func (s *UserService) Page(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	request = crud.NormalizePageRequest(request)
	page := request.Page
	size := request.Size

	where := `NOT EXISTS (
		SELECT 1 FROM base_sys_user_role protected_ur
		INNER JOIN base_sys_role protected_r ON protected_r.id = protected_ur.roleId
		WHERE protected_ur.userId = a.id
		AND protected_r.label = 'admin'
		AND (protected_r.tenantId IS NULL OR protected_r.tenantId = 0)
	)`
	args := []interface{}{}
	if request.Keyword != "" {
		where += " AND (a.name LIKE ? OR a.username LIKE ?)"
		args = append(args, "%"+request.Keyword+"%", "%"+request.Keyword+"%")
	}
	if status, ok := request.Raw["status"]; ok && status != nil {
		where += " AND a.status = ?"
		args = append(args, status)
	}
	departmentIDs, err := parseUserIDs(request.Raw["departmentIds"], "部门参数错误")
	if err != nil {
		return nil, err
	}
	if len(departmentIDs) > 0 {
		where += " AND a.departmentId IN (?)"
		args = append(args, departmentIDs)
	}
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND a.tenantId = ?"
		args = append(args, tenantID)
	}

	currentUser, authErr := requireAuthorizationCaller(ctx)
	if authErr != nil {
		return nil, authErr
	}
	isAdmin, authErr := isPlatformAdministrator(ctx, s.DB, currentUser.UserId)
	if authErr != nil {
		return nil, authErr
	}
	if !isAdmin {
		deptIDs, err := s.userDepartmentIDs(ctx, currentUser.UserId)
		if err != nil {
			return nil, err
		}
		if len(deptIDs) > 0 {
			where += " AND (a.userId = ? OR a.departmentId IN (?))"
			args = append(args, currentUser.UserId, deptIDs)
		} else {
			where += " AND a.userId = ?"
			args = append(args, currentUser.UserId)
		}
	}

	total, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_user a WHERE "+where, args...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户总数失败")
	}

	orderBy, err := pageOrderBy(request, map[string]string{
		"id": "a.id", "createTime": "a.createTime", "updateTime": "a.updateTime", "username": "a.username",
	}, "id", "DESC")
	if err != nil {
		return nil, err
	}
	limitSQL, limitArgs := sqlPageLimit(request)
	departmentJoin := " LEFT JOIN base_sys_department b ON a.departmentId = b.id"
	joinArgs := []interface{}{}
	if tenantID, ok := contextTenantID(ctx); ok {
		departmentJoin += " AND b.tenantId = ?"
		joinArgs = append(joinArgs, tenantID)
	}
	listSQL := "SELECT a.id, a.name, a.userId AS userId, a.nickName AS nickName, a.headImg AS headImg, a.email, a.remark, a.status, a.socketId AS socketId, a.createTime AS createTime, a.updateTime AS updateTime, a.tenantId AS tenantId, a.username, a.phone, a.departmentId AS departmentId, b.name AS departmentName FROM base_sys_user a" + departmentJoin + " WHERE " + where + orderBy + limitSQL
	listArgs := append(append(append([]interface{}{}, joinArgs...), args...), limitArgs...)
	rows, err := s.DB.GetAll(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户列表失败")
	}

	list := make([]map[string]interface{}, 0, len(rows))
	userIDs := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		item := row.Map()
		list = append(list, item)
		userIDs = append(userIDs, item["id"])
	}

	if len(userIDs) > 0 {
		roleSQL := "SELECT a.userId AS userId, a.roleId AS roleId, b.name FROM base_sys_user_role a INNER JOIN base_sys_role b ON a.roleId = b.id WHERE a.userId IN (?)"
		roleArgs := []interface{}{userIDs}
		if tenantID, ok := contextTenantID(ctx); ok {
			roleSQL += " AND b.tenantId = ?"
			roleArgs = append(roleArgs, tenantID)
		}
		roleRows, err := s.DB.GetAll(ctx, roleSQL, roleArgs...)
		if err != nil {
			return nil, gerror.Wrap(err, "查询用户角色失败")
		}
		for _, item := range list {
			roleNames := []string{}
			roleIDs := []interface{}{}
			for _, roleRow := range roleRows {
				if roleRow["userId"].Val() == item["id"] {
					roleNames = append(roleNames, roleRow["name"].String())
					roleIDs = append(roleIDs, roleRow["roleId"].Val())
				}
			}
			item["roleName"] = strings.Join(roleNames, ",")
			item["roleIds"] = roleIDs
		}
	}

	return crud.PageResult{
		List:       list,
		Pagination: crud.Pagination{Page: page, Size: size, Total: total},
	}, nil
}

// 返回受权限范围与服务端数量上限约束的用户列表
func (s *UserService) List(ctx context.Context, request crud.QueryRequest) (interface{}, error) {
	request.Page = 1
	request.Size = crud.MaxListSize
	request.IsExport = false
	result, err := s.Page(ctx, request)
	if err != nil {
		return nil, err
	}
	page, ok := result.(crud.PageResult)
	if !ok {
		return nil, exception.Internal(nil, "用户列表结果异常")
	}
	return page.List, nil
}

/**
 * 查询用户可访问部门
 * @param ctx 上下文
 * @param userID 用户 ID
 * @returns 部门 ID 列表
 */
func (s *UserService) userDepartmentIDs(ctx context.Context, userID int64) ([]int64, error) {
	return departmentIDsForUser(ctx, s.DB, userID)
}

/**
 * 获得用户详情
 * @param ctx 上下文
 * @param request 详情请求
 * @returns 用户详情
 */
func (s *UserService) Info(ctx context.Context, request crud.InfoRequest) (interface{}, error) {
	if err := requireRelationScope(ctx); err != nil {
		return nil, err
	}
	where := "a.id = ?"
	args := []interface{}{request.ID}
	if tenantID, ok := contextTenantID(ctx); ok {
		where += " AND a.tenantId = ?"
		args = append(args, tenantID)
	}
	departmentJoin := " LEFT JOIN base_sys_department b ON a.departmentId = b.id"
	joinArgs := []interface{}{}
	if tenantID, ok := contextTenantID(ctx); ok {
		departmentJoin += " AND b.tenantId = ?"
		joinArgs = append(joinArgs, tenantID)
	}
	queryArgs := append(joinArgs, args...)
	row, err := s.DB.GetOne(ctx, "SELECT a.id, a.name, a.nickName AS nickName, a.headImg AS headImg, a.email, a.remark, a.status, a.createTime AS createTime, a.updateTime AS updateTime, a.username, a.phone, a.departmentId AS departmentId, a.passwordV AS passwordV, a.socketId AS socketId, a.tenantId AS tenantId, a.userId AS userId, b.name AS departmentName FROM base_sys_user a"+departmentJoin+" WHERE "+where, queryArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户详情失败")
	}
	if len(row) == 0 {
		return nil, exception.Comm("数据不存在")
	}
	info := row.Map()
	roleSQL := "SELECT ur.roleId AS roleId FROM base_sys_user_role ur INNER JOIN base_sys_role r ON r.id = ur.roleId WHERE ur.userId = ?"
	roleArgs := []interface{}{request.ID}
	if tenantID, ok := contextTenantID(ctx); ok {
		roleSQL += " AND r.tenantId = ?"
		roleArgs = append(roleArgs, tenantID)
	}
	roleRows, err := s.DB.GetAll(ctx, roleSQL, roleArgs...)
	if err != nil {
		return nil, gerror.Wrap(err, "查询用户角色失败")
	}
	roleIDs := make([]interface{}, 0, len(roleRows))
	for _, roleRow := range roleRows {
		roleIDs = append(roleIDs, roleRow["roleId"].Val())
	}
	info["roleIdList"] = roleIDs
	return info, nil
}
