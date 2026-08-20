package service

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type userRow struct {
	ID           uint64      `orm:"id"`
	CreateTime   *gtime.Time `orm:"createTime"`
	UpdateTime   *gtime.Time `orm:"updateTime"`
	DepartmentID *uint64     `orm:"departmentId"`
	UserID       *uint64     `orm:"userId"`
	Name         *string     `orm:"name"`
	Username     string      `orm:"username"`
	Password     string      `orm:"password"`
	PasswordV    int32       `orm:"passwordV"`
	NickName     *string     `orm:"nickName"`
	HeadImg      *string     `orm:"headImg"`
	Phone        *string     `orm:"phone"`
	Email        *string     `orm:"email"`
	Remark       *string     `orm:"remark"`
	Status       int32       `orm:"status"`
	SocketID     *string     `orm:"socketId"`
}

type userRoleNameRow struct {
	RoleID uint64 `orm:"roleId"`
	Name   string `orm:"name"`
}

// UserPageFilter 是用户分页的固定筛选条件。
type UserPageFilter struct {
	DepartmentIDs []uint64
	KeyWord       string
	Status        *int32
}

// UserPageResult 是用户分页响应。
type UserPageResult struct {
	List       []dto.UserPageItem     `json:"list"`
	Pagination coreservice.Pagination `json:"pagination"`
}

// UserService 管理后台用户、角色关系和管理员保护。
type UserService struct {
	*coreservice.Base[entity.User, uint64]
	runtime    *coredb.Runtime
	userRole   *coreservice.Base[entity.UserRole, uint64]
	role       *coreservice.Base[entity.Role, uint64]
	department *coreservice.Base[entity.Department, uint64]
	password   *bcrypt.Verifier
	boundary   *auth.Boundary
}

// NewUser 创建用户业务服务。
func NewUser(
	runtime *coredb.Runtime,
	user *coreservice.Base[entity.User, uint64],
	userRole *coreservice.Base[entity.UserRole, uint64],
	role *coreservice.Base[entity.Role, uint64],
	department *coreservice.Base[entity.Department, uint64],
	password *bcrypt.Verifier,
	sessions auth.SessionStore,
) (*UserService, error) {
	if runtime == nil || runtime.Runner() == nil || !validPermissionBase(user) || !validPermissionBase(userRole) ||
		!validPermissionBase(role) || !validPermissionBase(department) || password == nil {
		return nil, exception.Core("用户服务依赖无效")
	}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		return nil, err
	}
	return &UserService{Base: user, runtime: runtime, userRole: userRole, role: role, department: department, password: password, boundary: boundary}, nil
}

// AddWithRoles 在同一事务中新建用户并写入角色关系。
func (service *UserService) AddWithRoles(ctx context.Context, input coreservice.AddInput[entity.User], roleIDs []uint64) (coreservice.AddResult[uint64], error) {
	if service == nil || service.runtime == nil {
		return coreservice.AddResult[uint64]{}, exception.Core("用户服务未初始化")
	}
	var result coreservice.AddResult[uint64]
	err := service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		value := input.One()
		if value == nil {
			return exception.Validate("用户新增只支持单条记录")
		}
		departmentIDs, err := submittedUserDepartmentIDs(value)
		if err != nil {
			return err
		}
		if err := service.hashPassword(value, false); err != nil {
			return err
		}
		if err = service.boundary.LockTable(txCtx, service.department.Descriptor().Table(), departmentIDs, "锁定授权部门失败"); err != nil {
			return err
		}
		if err = service.boundary.LockTable(txCtx, service.role.Descriptor().Table(), roleIDs, "锁定授权角色失败"); err != nil {
			return err
		}
		result, err = service.Base.Add(txCtx, input)
		if err != nil {
			return err
		}
		return service.replaceRoles(txCtx, result.One(), roleIDs)
	})
	return result, err
}

// UpdateWithRoles 更新用户，并在提交前替换可选的角色关系。
func (service *UserService) UpdateWithRoles(ctx context.Context, input coreservice.UpdateInput[entity.User, uint64], roleIDs *[]uint64) error {
	if service == nil || service.runtime == nil {
		return exception.Core("用户服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		items := userUpdateItems(input)
		if len(items) != 1 {
			return exception.Validate("用户更新只支持单条记录")
		}
		item := items[0]
		departmentIDs, err := submittedUserDepartmentIDs(item.Mutable())
		if err != nil {
			return err
		}
		if err = service.boundary.LockTable(txCtx, service.department.Descriptor().Table(), departmentIDs, "锁定授权部门失败"); err != nil {
			return err
		}
		isAuthorizationChange := item.Mutable().Has("status") || roleIDs != nil
		if isAuthorizationChange {
			var newRoleIDs []uint64
			if roleIDs != nil {
				newRoleIDs = *roleIDs
			}
			if err := service.lockUserAuthorizationChanges(txCtx, []uint64{item.ID()}, newRoleIDs); err != nil {
				return err
			}
		} else if err := service.lockUsers(txCtx, []uint64{item.ID()}); err != nil {
			return err
		}
		row, err := service.userByID(txCtx, item.ID())
		if err != nil || row == nil {
			return err
		}
		if isAuthorizationChange {
			currentRoleIDs, roleErr := service.roleIDs(txCtx, item.ID())
			if roleErr != nil {
				return roleErr
			}
			nextRoleIDs := currentRoleIDs
			if roleIDs != nil {
				nextRoleIDs = *roleIDs
			}
			nextStatus := row.Status
			if item.Mutable().Has("status") {
				status, exists := item.Mutable().Get("status")
				statusValue, ok := status.(int32)
				if !exists || !ok {
					return exception.Validate("用户状态无效")
				}
				nextStatus = statusValue
			}
			if err = service.ensureAdminTransition(txCtx, item.ID(), row.Status, nextStatus, currentRoleIDs, nextRoleIDs); err != nil {
				return err
			}
		}
		if err = service.hashPassword(item.Mutable(), true); err != nil {
			return err
		}
		if item.Mutable().Has("password") {
			if err = item.Mutable().Set("passwordV", row.PasswordV+1); err != nil {
				return err
			}
		}
		if item.Mutable().Has("password") || item.Mutable().Has("status") || roleIDs != nil {
			if err = service.revokeUsers(txCtx, []uint64{item.ID()}); err != nil {
				return err
			}
		}
		if err = service.Base.Update(txCtx, input); err != nil {
			return err
		}
		if roleIDs != nil {
			return service.replaceRoles(txCtx, item.ID(), *roleIDs)
		}
		return nil
	})
}

// UpdateRelations 替换一个用户的角色关系并撤销其旧 Session。
func (service *UserService) UpdateRelations(ctx context.Context, userID uint64, input dto.UserRoleInput) error {
	if service == nil || service.runtime == nil || userID == 0 {
		return exception.Validate("用户角色关系参数无效")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		if err := service.lockUserAuthorizationChanges(txCtx, []uint64{userID}, input.RoleIDList); err != nil {
			return err
		}
		row, err := service.userByID(txCtx, userID)
		if err != nil || row == nil {
			return err
		}
		currentRoleIDs, err := service.roleIDs(txCtx, userID)
		if err != nil {
			return err
		}
		if err = service.ensureAdminTransition(txCtx, userID, row.Status, row.Status, currentRoleIDs, input.RoleIDList); err != nil {
			return err
		}
		if err = service.revokeUsers(txCtx, []uint64{userID}); err != nil {
			return err
		}
		return service.replaceRoles(txCtx, userID, input.RoleIDList)
	})
}

// Delete 删除用户及其角色关系，并保护最后一个有效管理员。
func (service *UserService) Delete(ctx context.Context, input coreservice.DeleteInput[uint64]) error {
	if service == nil || service.runtime == nil {
		return exception.Core("用户服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		ids := businessUniqueIDs(input.IDs())
		if err := service.lockUserAuthorizationChanges(txCtx, ids, nil); err != nil {
			return err
		}
		if err := service.ensureNotLastAdmin(txCtx, ids); err != nil {
			return err
		}
		if err := service.revokeUsers(txCtx, ids); err != nil {
			return err
		}
		model, err := service.userRole.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = model.WhereIn("userId", ids).Delete(); err != nil {
			return exception.WrapCore(err, "清理用户角色关系失败")
		}
		return service.Base.Delete(txCtx, input)
	})
}

// Move 批量移动用户部门。
func (service *UserService) Move(ctx context.Context, request dto.UserMoveReq) error {
	if service == nil || service.runtime == nil || request.DepartmentID == 0 || len(request.UserIDs) == 0 {
		return exception.Validate("移动用户参数无效")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		if err := service.boundary.LockTable(txCtx, service.department.Descriptor().Table(), []uint64{request.DepartmentID}, "锁定授权部门失败"); err != nil {
			return err
		}
		if err := service.boundary.LockUsersAndRevoke(txCtx, service.Descriptor().Table(), request.UserIDs, auth.AdminKind, "锁定授权用户失败"); err != nil {
			return err
		}
		data, err := businessDO(service.Descriptor(), businessField{name: "departmentId", value: request.DepartmentID})
		if err != nil {
			return err
		}
		model, err := service.Base.Model(txCtx)
		if err != nil {
			return err
		}
		if _, err = model.WhereIn("id", businessUniqueIDs(request.UserIDs)).Data(data).Update(); err != nil {
			return exception.WrapCore(err, "移动用户部门失败")
		}
		return nil
	})
}

// Info 返回用户详情及角色、部门虚拟字段，绝不返回密码摘要。
func (service *UserService) Info(ctx context.Context, userID uint64) (*dto.UserInfoResult, error) {
	row, err := service.userByID(ctx, userID)
	if err != nil || row == nil {
		return nil, err
	}
	result := userInfoResult(*row)
	result.RoleIDList, err = service.roleIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	result.DepartmentName, err = service.departmentName(ctx, row.DepartmentID)
	return &result, err
}

// Person 返回当前已认证管理员的个人资料。
func (service *UserService) Person(ctx context.Context) (*dto.UserInfoResult, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return nil, err
	}
	return service.Info(ctx, identity.UserID)
}

// PersonUpdate 更新当前用户允许修改的资料；修改密码时校验旧密码并递增版本。
func (service *UserService) PersonUpdate(ctx context.Context, request dto.PersonUpdateReq) error {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return err
	}
	if service == nil || service.runtime == nil {
		return exception.Core("用户服务未初始化")
	}
	return service.runtime.Runner().Within(ctx, func(txCtx context.Context) error {
		row, err := service.lockedUser(txCtx, identity.UserID)
		if err != nil || row == nil {
			return err
		}
		fields := []businessField{}
		for _, value := range []struct {
			name  string
			value *string
		}{{"name", request.Name}, {"nickName", request.NickName}, {"headImg", request.HeadImg}, {"phone", request.Phone}, {"email", request.Email}} {
			if value.value != nil {
				fields = append(fields, businessField{name: value.name, value: *value.value})
			}
		}
		if request.Password != nil && strings.TrimSpace(*request.Password) != "" {
			if request.OldPassword == nil || strings.TrimSpace(*request.OldPassword) == "" {
				return exception.Comm("原密码不能为空")
			}
			verified, verifyErr := service.password.Verify(*request.OldPassword, row.Password)
			if verifyErr != nil {
				return verifyErr
			}
			if !verified.Valid {
				return exception.Comm("原密码错误")
			}
			hash, hashErr := service.password.Hash(*request.Password)
			if hashErr != nil {
				return hashErr
			}
			fields = append(fields, businessField{name: "password", value: hash}, businessField{name: "passwordV", value: row.PasswordV + 1})
			if err = service.revokeUsers(txCtx, []uint64{identity.UserID}); err != nil {
				return err
			}
		}
		if len(fields) == 0 {
			return nil
		}
		data, err := businessDO(service.Descriptor(), fields...)
		if err != nil {
			return err
		}
		model, err := service.Base.Model(txCtx)
		if err != nil {
			return err
		}
		_, err = model.Where("id", identity.UserID).Data(data).Update()
		return exception.WrapCore(err, "更新个人资料失败")
	})
}

// Page 按当前管理员的数据范围返回用户分页和虚拟字段。
func (service *UserService) Page(ctx context.Context, page, size int, filter UserPageFilter) (UserPageResult, error) {
	if service == nil || page <= 0 || size <= 0 {
		return UserPageResult{}, exception.Validate("用户分页参数无效")
	}
	identity, err := auth.Admin(ctx)
	if err != nil {
		return UserPageResult{}, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return UserPageResult{}, err
	}
	isAdmin, err := service.isAdmin(ctx, identity.UserID)
	if err != nil {
		return UserPageResult{}, err
	}
	if !isAdmin {
		departmentIDs, rangeErr := service.departmentIDs(ctx, identity.UserID)
		if rangeErr != nil {
			return UserPageResult{}, rangeErr
		}
		if len(departmentIDs) > 0 {
			model = model.WhereIn("departmentId", departmentIDs).WhereOr("userId", identity.UserID)
		} else {
			model = model.Where("userId", identity.UserID)
		}
	}
	if len(filter.DepartmentIDs) > 0 {
		model = model.WhereIn("departmentId", businessUniqueIDs(filter.DepartmentIDs))
	}
	if filter.Status != nil {
		model = model.Where("status", *filter.Status)
	}
	if keyWord := strings.TrimSpace(filter.KeyWord); keyWord != "" {
		model = model.Where("name LIKE ? OR username LIKE ?", "%"+keyWord+"%", "%"+keyWord+"%")
	}
	var rows []userRow
	result, total, err := model.OrderAsc("id").Page(page, size).AllAndCount(false)
	if err != nil {
		return UserPageResult{}, exception.WrapCore(err, "查询用户分页失败")
	}
	if err = result.ScanList(&rows, ""); err != nil {
		return UserPageResult{}, exception.WrapCore(err, "解析用户分页失败")
	}
	items, err := service.pageItems(ctx, rows)
	if err != nil {
		return UserPageResult{}, err
	}
	return UserPageResult{List: items, Pagination: coreservice.Pagination{Page: page, Size: size, Total: int64(total)}}, nil
}

func (service *UserService) hashPassword(value *coreservice.Mutable[entity.User], isUpdate bool) error {
	password, exists := value.Get("password")
	if !exists {
		return nil
	}
	plain, ok := password.(string)
	if !ok || strings.TrimSpace(plain) == "" {
		return exception.Validate("密码不能为空")
	}
	hash, err := service.password.Hash(plain)
	if err != nil {
		return err
	}
	if err = value.Set("password", hash); err != nil {
		return err
	}
	_ = isUpdate
	return nil
}

func (service *UserService) lockedUser(ctx context.Context, userID uint64) (*userRow, error) {
	if err := service.lockUsers(ctx, []uint64{userID}); err != nil {
		return nil, err
	}
	return service.userByID(ctx, userID)
}

func (service *UserService) userByID(ctx context.Context, userID uint64) (*userRow, error) {
	model, err := service.Base.Model(ctx)
	if err != nil {
		return nil, err
	}
	var row *userRow
	err = model.Where("id", userID).Scan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, exception.WrapCore(err, "查询用户失败")
	}
	return row, nil
}

func (service *UserService) lockUserAuthorizationChanges(ctx context.Context, userIDs, newRoleIDs []uint64) error {
	adminRoleIDs, err := service.adminRoleIDs(ctx)
	if err != nil {
		return err
	}
	if err = service.boundary.LockTable(ctx, service.role.Descriptor().Table(), adminRoleIDs, "锁定授权角色失败"); err != nil {
		return err
	}
	currentRoleIDs, err := service.roleIDsForUsers(ctx, userIDs)
	if err != nil {
		return err
	}
	involvedRoleIDs := make([]uint64, 0, len(currentRoleIDs)+len(newRoleIDs))
	for _, roleID := range businessUniqueIDs(append(currentRoleIDs, newRoleIDs...)) {
		if !slices.Contains(adminRoleIDs, roleID) {
			involvedRoleIDs = append(involvedRoleIDs, roleID)
		}
	}
	if err = service.boundary.LockTable(ctx, service.role.Descriptor().Table(), involvedRoleIDs, "锁定授权角色失败"); err != nil {
		return err
	}
	if err = service.lockUsers(ctx, userIDs); err != nil {
		return err
	}
	lockedRoleIDs, err := service.roleIDsForUsers(ctx, userIDs)
	if err != nil {
		return err
	}
	return auth.ValidateSnapshot(currentRoleIDs, lockedRoleIDs, "用户角色已变更，请重试")
}

func (service *UserService) lockUsers(ctx context.Context, userIDs []uint64) error {
	return service.boundary.LockTable(ctx, service.Descriptor().Table(), userIDs, "锁定授权用户失败")
}

func (service *UserService) revokeUsers(ctx context.Context, userIDs []uint64) error {
	return service.boundary.RevokeUsers(ctx, auth.AdminKind, userIDs)
}

func (service *UserService) replaceRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.Where("userId", userID).Delete(); err != nil {
		return exception.WrapCore(err, "删除旧用户角色关系失败")
	}
	for _, roleID := range businessUniqueIDs(roleIDs) {
		data, dataErr := businessDO(service.userRole.Descriptor(), businessField{name: "userId", value: userID}, businessField{name: "roleId", value: roleID})
		if dataErr != nil {
			return dataErr
		}
		if _, err = model.Data(data).Insert(); err != nil {
			return exception.WrapCore(err, "写入用户角色关系失败")
		}
	}
	return nil
}

func submittedUserDepartmentIDs(mutable *coreservice.Mutable[entity.User]) ([]uint64, error) {
	if mutable == nil || !mutable.Has("departmentId") {
		return nil, nil
	}
	value, exists := mutable.Get("departmentId")
	if !exists || value == nil {
		return nil, nil
	}
	departmentID, ok := value.(*uint64)
	if !ok {
		return nil, exception.Validate("用户部门参数无效")
	}
	if departmentID == nil || *departmentID == 0 {
		return nil, nil
	}

	return []uint64{*departmentID}, nil
}

func (service *UserService) roleIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		RoleID uint64 `orm:"roleId"`
	}
	if err = model.Fields("roleId").Where("userId", userID).OrderAsc("roleId").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询用户角色失败")
	}
	result := make([]uint64, len(rows))
	for index, row := range rows {
		result[index] = row.RoleID
	}
	return result, nil
}

func (service *UserService) roleIDsForUsers(ctx context.Context, userIDs []uint64) ([]uint64, error) {
	ids := businessUniqueIDs(userIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	model, err := service.userRole.Model(ctx)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		RoleID uint64 `orm:"roleId"`
	}
	if err = model.Fields("roleId").WhereIn("userId", ids).OrderAsc("roleId").Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询用户角色失败")
	}
	roleIDs := make([]uint64, len(rows))
	for index, row := range rows {
		roleIDs[index] = row.RoleID
	}

	return businessUniqueIDs(roleIDs), nil
}

func (service *UserService) adminRoleIDs(ctx context.Context) ([]uint64, error) {
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
	roleIDs := make([]uint64, len(rows))
	for index, row := range rows {
		roleIDs[index] = row.ID
	}

	return roleIDs, nil
}

func (service *UserService) ensureAdminTransition(
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
	currentIsAdmin, err := service.containsAdminRole(ctx, currentRoleIDs)
	if err != nil || !currentIsAdmin {
		return err
	}
	if nextStatus == 1 {
		nextIsAdmin, checkErr := service.containsAdminRole(ctx, nextRoleIDs)
		if checkErr != nil || nextIsAdmin {
			return checkErr
		}
	}

	return service.ensureNotLastAdmin(ctx, []uint64{userID})
}

func (service *UserService) ensureNotLastAdmin(ctx context.Context, userIDs []uint64) error {
	ids := businessUniqueIDs(userIDs)
	if len(ids) == 0 {
		return nil
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	var affected []struct {
		ID uint64 `orm:"id"`
	}
	if err = model.Fields("id").WhereIn("id", ids).Where("status", 1).Scan(&affected); err != nil {
		return exception.WrapCore(err, "查询管理员状态失败")
	}
	removesAdmin := false
	for _, user := range affected {
		isAdmin, checkErr := service.isAdmin(ctx, user.ID)
		if checkErr != nil {
			return checkErr
		}
		if isAdmin {
			removesAdmin = true
			break
		}
	}
	if !removesAdmin {
		return nil
	}
	adminRoleIDs, err := service.adminRoleIDs(ctx)
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
	if err = relationModel.
		Fields("userId").
		WhereIn("roleId", adminRoleIDs).
		WhereNotIn("userId", ids).
		Scan(&rows); err != nil {
		return exception.WrapCore(err, "查询管理员关系失败")
	}
	remainingUserIDs := make([]uint64, len(rows))
	for index, row := range rows {
		remainingUserIDs[index] = row.UserID
	}
	if len(remainingUserIDs) == 0 {
		return exception.Comm("不能删除或禁用最后一个管理员")
	}
	remainingModel, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	remainingAdmin, err := remainingModel.
		WhereIn("id", businessUniqueIDs(remainingUserIDs)).
		Where("status", 1).
		Exist()
	if err != nil {
		return exception.WrapCore(err, "查询有效管理员失败")
	}
	if !remainingAdmin {
		return exception.Comm("不能删除或禁用最后一个管理员")
	}

	return nil
}

func (service *UserService) isAdmin(ctx context.Context, userID uint64) (bool, error) {
	roleIDs, err := service.roleIDs(ctx, userID)
	if err != nil || len(roleIDs) == 0 {
		return false, err
	}

	return service.containsAdminRole(ctx, roleIDs)
}

func (service *UserService) containsAdminRole(ctx context.Context, roleIDs []uint64) (bool, error) {
	roleIDs = businessUniqueIDs(roleIDs)
	if len(roleIDs) == 0 {
		return false, nil
	}
	model, err := service.role.Model(ctx)
	if err != nil {
		return false, err
	}
	var count int
	count, err = model.WhereIn("id", roleIDs).Where("label", adminRoleLabel).Count()
	if err != nil {
		return false, exception.WrapCore(err, "查询管理员角色失败")
	}
	return count > 0, nil
}

func (service *UserService) departmentIDs(ctx context.Context, userID uint64) ([]uint64, error) {
	roleIDs, err := service.roleIDs(ctx, userID)
	if err != nil || len(roleIDs) == 0 {
		return nil, err
	}
	var rows []struct {
		DepartmentID uint64 `orm:"departmentId"`
	}
	if err = service.runtime.DB().Model("base_sys_role_department").Ctx(ctx).Fields("departmentId").WhereIn("roleId", roleIDs).Scan(&rows); err != nil {
		return nil, exception.WrapCore(err, "查询部门权限失败")
	}
	ids := make([]uint64, len(rows))
	for index, row := range rows {
		ids[index] = row.DepartmentID
	}
	return businessUniqueIDs(ids), nil
}

func (service *UserService) departmentName(ctx context.Context, departmentID *uint64) (*string, error) {
	if departmentID == nil || *departmentID == 0 {
		return nil, nil
	}
	model, err := service.department.Model(ctx)
	if err != nil {
		return nil, err
	}
	var row *struct {
		Name string `orm:"name"`
	}
	if err = model.Fields("name").Where("id", *departmentID).Scan(&row); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, exception.WrapCore(err, "查询部门名称失败")
	}
	if row == nil {
		return nil, nil
	}
	return &row.Name, nil
}

func (service *UserService) pageItems(ctx context.Context, rows []userRow) ([]dto.UserPageItem, error) {
	result := make([]dto.UserPageItem, 0, len(rows))
	for _, row := range rows {
		roles, err := service.roleNames(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		departmentName, err := service.departmentName(ctx, row.DepartmentID)
		if err != nil {
			return nil, err
		}
		roleIDs := make([]uint64, len(roles))
		roleNames := make([]string, len(roles))
		for index, role := range roles {
			roleIDs[index], roleNames[index] = role.RoleID, role.Name
		}
		result = append(result, dto.UserPageItem{ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, DepartmentID: row.DepartmentID, Name: row.Name, Username: row.Username, NickName: row.NickName, HeadImg: row.HeadImg, Phone: row.Phone, Email: row.Email, Remark: row.Remark, Status: row.Status, DepartmentName: departmentName, RoleIDs: roleIDs, RoleName: strings.Join(roleNames, ",")})
	}
	return result, nil
}

func (service *UserService) roleNames(ctx context.Context, userID uint64) ([]userRoleNameRow, error) {
	statement, err := coreservice.NativeSQL(`SELECT ur.roleId, r.name FROM base_sys_user_role ur INNER JOIN base_sys_role r ON r.id = ur.roleId WHERE ur.userId = ? ORDER BY ur.roleId ASC`, userID)
	if err != nil {
		return nil, err
	}
	var rows []userRoleNameRow
	if err = service.Base.NativeQuery(ctx, statement, &rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func userInfoResult(row userRow) dto.UserInfoResult {
	return dto.UserInfoResult{ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, DepartmentID: row.DepartmentID, UserID: row.UserID, Name: row.Name, Username: row.Username, PasswordV: row.PasswordV, NickName: row.NickName, HeadImg: row.HeadImg, Phone: row.Phone, Email: row.Email, Remark: row.Remark, Status: row.Status, SocketID: row.SocketID}
}

func userUpdateItems(input coreservice.UpdateInput[entity.User, uint64]) []coreservice.UpdateItem[entity.User, uint64] {
	if input.IsMany() {
		return input.Many()
	}
	return []coreservice.UpdateItem[entity.User, uint64]{input.One()}
}

type businessField struct {
	name  string
	value any
}

func businessDO(descriptor coreentity.RuntimeDescriptor, fields ...businessField) (any, error) {
	if descriptor == nil {
		return nil, exception.Core("实体 Descriptor 无效")
	}
	value := descriptor.NewDO()
	if value == nil {
		return nil, exception.Core("实体 DO 无效")
	}
	for _, field := range fields {
		if err := value.SetColumn(field.name, field.value); err != nil {
			return nil, err
		}
	}
	return value.DBData(), nil
}

func businessUniqueIDs(ids []uint64) []uint64 {
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id != 0 {
			result = append(result, id)
		}
	}
	slices.Sort(result)
	return slices.Compact(result)
}
