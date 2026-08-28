package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
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

type userWrite struct {
	g.Meta       `orm:"do:true"`
	DepartmentID any `orm:"departmentId"`
	Name         any `orm:"name"`
	NickName     any `orm:"nickName"`
	HeadImg      any `orm:"headImg"`
	Phone        any `orm:"phone"`
	Email        any `orm:"email"`
	Password     any `orm:"password"`
	PasswordV    any `orm:"passwordV"`
}

// 用户分页响应
type UserPageResult struct {
	List       []dto.UserPageItem     `json:"list"`
	Pagination coreservice.Pagination `json:"pagination"`
}

// 后台用户业务服务
type UserService struct {
	*coreservice.Base[entity.User, uint64]
	permission *PermissionService
	department *DepartmentService
	password   *bcrypt.Verifier
}

// 用户业务服务
func NewUser(
	user *coreservice.Base[entity.User, uint64],
	permission *PermissionService,
	department *DepartmentService,
	password *bcrypt.Verifier,
) (*UserService, error) {
	if !validPermissionBase(user) || permission == nil || permission.boundary == nil ||
		department == nil || department.boundary == nil || password == nil {
		return nil, exception.Core("用户服务依赖无效")
	}

	return &UserService{Base: user, permission: permission, department: department, password: password}, nil
}

// 新增用户及其角色关系
func (service *UserService) Add(ctx context.Context, input coreservice.AddInput[entity.User]) (coreservice.AddResult[uint64], error) {
	value := input.One()
	if service == nil || value == nil {
		return coreservice.AddResult[uint64]{}, exception.Validate("用户新增只支持单条记录")
	}
	roleIDs, submitted, err := userRoleIDs(value)
	if err != nil || !submitted || len(roleIDs) == 0 {
		if err != nil {
			return coreservice.AddResult[uint64]{}, err
		}
		return coreservice.AddResult[uint64]{}, exception.Validate("用户至少需要一个角色")
	}
	if err = service.hashPassword(value); err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	departmentIDs, err := userDepartmentIDs(value)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	if err = service.permission.LockRoles(ctx, roleIDs); err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	if err = service.department.LockDepartments(ctx, departmentIDs); err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	result, err := service.Base.Add(ctx, input)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}
	if err = service.permission.ReplaceRoles(ctx, result.One(), roleIDs); err != nil {
		return coreservice.AddResult[uint64]{}, err
	}

	return result, nil
}

// 更新用户及其可选角色关系
func (service *UserService) Update(ctx context.Context, input coreservice.UpdateInput[entity.User, uint64]) error {
	item := input.One()
	if service == nil || input.IsMany() || item.Mutable() == nil {
		return exception.Validate("用户更新只支持单条记录")
	}
	value := item.Mutable()
	roleIDs, hasRoles, err := userRoleIDs(value)
	if err != nil {
		return err
	}
	departmentIDs, err := userDepartmentIDs(value)
	if err != nil {
		return err
	}
	isAuthorizationChange := value.Has("status") || hasRoles
	var snapshot map[uint64][]uint64
	if isAuthorizationChange {
		snapshot, err = service.permission.PrepareRoleChange(ctx, []uint64{item.ID()}, roleIDs)
		if err != nil {
			return err
		}
	}
	if err = service.department.LockDepartments(ctx, departmentIDs); err != nil {
		return err
	}
	if err = service.permission.LockUsers(ctx, []uint64{item.ID()}); err != nil {
		return err
	}
	if isAuthorizationChange {
		after, snapshotErr := service.permission.RoleSnapshot(ctx, []uint64{item.ID()})
		if snapshotErr != nil {
			return snapshotErr
		}
		if err = service.permission.ValidateRoleSnapshot(snapshot, after); err != nil {
			return err
		}
	}
	row, err := service.userByID(ctx, item.ID())
	if err != nil || row == nil {
		return err
	}
	currentRoleIDs := snapshot[item.ID()]
	nextRoleIDs := currentRoleIDs
	if hasRoles {
		nextRoleIDs = roleIDs
	}
	nextStatus, err := userNextStatus(value, row.Status)
	if err != nil {
		return err
	}
	if isAuthorizationChange {
		if err = service.permission.EnsureAdminTransition(ctx, item.ID(), row.Status, nextStatus, currentRoleIDs, nextRoleIDs); err != nil {
			return err
		}
	}
	if err = service.updatePassword(value, row.PasswordV); err != nil {
		return err
	}
	shouldRevoke := value.Has("password") || value.Has("status") || hasRoles
	if userHasPersistentUpdate(service.Descriptor(), value) {
		if err = service.Base.Update(ctx, input); err != nil {
			return err
		}
	}
	if hasRoles {
		if err = service.permission.ReplaceRoles(ctx, item.ID(), roleIDs); err != nil {
			return err
		}
	}
	if shouldRevoke {
		return service.permission.RevokeUsers(ctx, []uint64{item.ID()})
	}

	return nil
}

// 删除用户及其角色关系
func (service *UserService) Delete(ctx context.Context, input coreservice.DeleteInput[uint64]) error {
	ids := auth.NormalizeIDs(input.IDs())
	if service == nil || len(ids) == 0 {
		return exception.Validate("用户 ID 不能为空")
	}
	snapshot, err := service.permission.PrepareRoleChange(ctx, ids, nil)
	if err != nil {
		return err
	}
	if err = service.permission.LockUsers(ctx, ids); err != nil {
		return err
	}
	after, err := service.permission.RoleSnapshot(ctx, ids)
	if err != nil {
		return err
	}
	if err = service.permission.ValidateRoleSnapshot(snapshot, after); err != nil {
		return err
	}
	if err = service.permission.EnsureNotLastAdmin(ctx, ids); err != nil {
		return err
	}
	if err = service.permission.DeleteUserRoles(ctx, ids); err != nil {
		return err
	}
	if err = service.Base.Delete(ctx, input); err != nil {
		return err
	}

	return service.permission.RevokeUsers(ctx, ids)
}

// 用户详情及角色、部门虚拟字段
func (service *UserService) Info(ctx context.Context, userID uint64) (*dto.UserInfoResult, error) {
	record, err := service.Base.Info(ctx, userID)
	if err != nil {
		return nil, err
	}
	if _, exists := record.Get("id"); !exists {
		return nil, nil
	}
	var result dto.UserInfoResult
	if err = record.Scan(&result); err != nil {
		return nil, err
	}
	if err = service.enrichUserInfo(ctx, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// 按当前管理员的数据范围返回用户分页
func (service *UserService) Page(ctx context.Context, request *dto.UserPageReq) (UserPageResult, error) {
	if service == nil || request == nil {
		return UserPageResult{}, exception.Validate("用户分页参数无效")
	}
	page, size := request.Page, request.Size
	if page == 0 {
		page = 1
	}
	if size == 0 {
		size = 15
	}
	if page < 1 || size < 1 {
		return UserPageResult{}, exception.Validate("用户分页参数无效")
	}
	orderColumn, isDescending, err := userPageOrder(service.Descriptor(), request.Order, request.Sort)
	if err != nil {
		return UserPageResult{}, err
	}
	identity, err := auth.Admin(ctx)
	if err != nil {
		return UserPageResult{}, err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return UserPageResult{}, err
	}
	model = model.WhereNot("username", "admin")
	isAdmin, err := service.permission.IsAdmin(ctx, identity.RoleIDs())
	if err != nil {
		return UserPageResult{}, err
	}
	if !isAdmin {
		departmentIDs, rangeErr := service.department.departmentIDsByRoles(ctx, identity.RoleIDs())
		if rangeErr != nil {
			return UserPageResult{}, rangeErr
		}
		if len(departmentIDs) == 0 {
			model = model.Where("userId", identity.UserID)
		} else {
			scope := model.Builder().WhereIn("departmentId", departmentIDs).WhereOr("userId", identity.UserID)
			model = model.Where(scope)
		}
	}
	if departmentIDs := auth.NormalizeIDs(request.DepartmentIDs); len(departmentIDs) > 0 {
		model = model.WhereIn("departmentId", departmentIDs)
	}
	if request.Status != nil {
		model = model.Where("status", *request.Status)
	}
	if keyword := strings.TrimSpace(request.KeyWord); keyword != "" {
		model = model.Where("name LIKE ? OR username LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	if isDescending {
		model = model.OrderDesc(orderColumn)
	} else {
		model = model.OrderAsc(orderColumn)
	}
	var (
		rows  []userRow
		total int
	)
	if err = model.Page(page, size).ScanAndCount(&rows, &total, false); err != nil {
		return UserPageResult{}, exception.WrapCore(err, "查询用户分页失败")
	}
	items, err := service.pageItems(ctx, rows)
	if err != nil {
		return UserPageResult{}, err
	}

	return UserPageResult{List: items, Pagination: coreservice.Pagination{Page: page, Size: size, Total: int64(total)}}, nil
}

// 批量移动用户部门
func (service *UserService) Move(ctx context.Context, request *dto.UserMoveReq) error {
	if service == nil || request == nil {
		return exception.Validate("移动用户参数无效")
	}
	userIDs := auth.NormalizeIDs(request.UserIDs)
	if request.DepartmentID == 0 || len(userIDs) == 0 {
		return exception.Validate("移动用户参数无效")
	}
	if err := service.department.LockDepartments(ctx, []uint64{request.DepartmentID}); err != nil {
		return err
	}
	if err := service.permission.LockUsers(ctx, userIDs); err != nil {
		return err
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.WhereIn("id", userIDs).Data(userWrite{DepartmentID: request.DepartmentID}).Update(); err != nil {
		return exception.WrapCore(err, "移动用户部门失败")
	}

	return service.permission.RevokeUsers(ctx, userIDs)
}

// 当前已认证管理员的个人资料
func (service *UserService) Person(ctx context.Context) (*dto.UserInfoResult, error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return nil, err
	}
	row, err := service.userByID(ctx, identity.UserID)
	if err != nil || row == nil {
		return nil, err
	}
	result := userInfoResult(*row)
	if err = service.enrichUserInfo(ctx, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// 更新当前用户允许修改的资料
func (service *UserService) PersonUpdate(ctx context.Context, request dto.PersonUpdateReq) error {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return err
	}
	if err = service.permission.LockUsers(ctx, []uint64{identity.UserID}); err != nil {
		return err
	}
	row, err := service.userByID(ctx, identity.UserID)
	if err != nil || row == nil {
		return err
	}
	data := userWrite{}
	hasUpdate := false
	for _, field := range []struct {
		value *string
		set   func(string)
	}{
		{request.Name, func(value string) { data.Name = value }},
		{request.NickName, func(value string) { data.NickName = value }},
		{request.HeadImg, func(value string) { data.HeadImg = value }},
		{request.Phone, func(value string) { data.Phone = value }},
		{request.Email, func(value string) { data.Email = value }},
	} {
		if field.value != nil {
			field.set(*field.value)
			hasUpdate = true
		}
	}
	shouldRevoke := request.Password != nil && strings.TrimSpace(*request.Password) != ""
	if shouldRevoke {
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
		data.Password, err = service.password.Hash(*request.Password)
		if err != nil {
			return err
		}
		data.PasswordV = row.PasswordV + 1
		hasUpdate = true
	}
	if !hasUpdate {
		return nil
	}
	model, err := service.Base.Model(ctx)
	if err != nil {
		return err
	}
	if _, err = model.Where("id", identity.UserID).Data(data).Update(); err != nil {
		return exception.WrapCore(err, "更新个人资料失败")
	}
	if shouldRevoke {
		return service.permission.RevokeUsers(ctx, []uint64{identity.UserID})
	}

	return nil
}

func (service *UserService) hashPassword(value *coreservice.Mutable[entity.User]) error {
	password, exists := value.Get("password")
	plain, ok := password.(string)
	if !exists || !ok || strings.TrimSpace(plain) == "" {
		return exception.Validate("密码不能为空")
	}
	hash, err := service.password.Hash(plain)
	if err != nil {
		return err
	}

	return value.Set("password", hash)
}

func (service *UserService) updatePassword(value *coreservice.Mutable[entity.User], passwordVersion int32) error {
	if !value.Has("password") {
		return nil
	}
	password, _ := value.Get("password")
	plain, ok := password.(string)
	if !ok || strings.TrimSpace(plain) == "" {
		return value.Unset("password")
	}
	hash, err := service.password.Hash(plain)
	if err != nil {
		return err
	}
	if err = value.Set("password", hash); err != nil {
		return err
	}

	return value.Set("passwordV", passwordVersion+1)
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

func (service *UserService) enrichUserInfo(ctx context.Context, result *dto.UserInfoResult) error {
	roleIDs, err := service.permission.RoleIDs(ctx, result.ID)
	if err != nil {
		return err
	}
	result.RoleIDList = roleIDs
	if result.DepartmentID != nil {
		name, nameErr := service.department.Name(ctx, *result.DepartmentID)
		if nameErr != nil {
			return nameErr
		}
		result.DepartmentName = &name
	}

	return nil
}

func (service *UserService) pageItems(ctx context.Context, rows []userRow) ([]dto.UserPageItem, error) {
	userIDs := make([]uint64, 0, len(rows))
	departmentIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.ID)
		if row.DepartmentID != nil {
			departmentIDs = append(departmentIDs, *row.DepartmentID)
		}
	}
	roles, err := service.permission.RolesByUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	departmentNames, err := service.department.Names(ctx, departmentIDs)
	if err != nil {
		return nil, err
	}
	items := make([]dto.UserPageItem, 0, len(rows))
	for _, row := range rows {
		var departmentName *string
		if row.DepartmentID != nil {
			if name, exists := departmentNames[*row.DepartmentID]; exists {
				departmentName = &name
			}
		}
		userRoles := roles[row.ID]
		items = append(items, dto.UserPageItem{
			ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
			DepartmentID: row.DepartmentID, Name: row.Name, Username: row.Username,
			NickName: row.NickName, HeadImg: row.HeadImg, Phone: row.Phone, Email: row.Email,
			Remark: row.Remark, Status: row.Status, DepartmentName: departmentName,
			RoleIDs: userRoles.IDs, RoleName: strings.Join(userRoles.Names, ","),
		})
	}

	return items, nil
}

func userRoleIDs(value *coreservice.Mutable[entity.User]) ([]uint64, bool, error) {
	if value == nil || !value.Has("roleIdList") {
		return nil, false, nil
	}
	if value.IsNull("roleIdList") {
		return []uint64{}, true, nil
	}
	current, _ := value.Get("roleIdList")
	roleIDs, ok := current.([]uint64)
	if !ok {
		return nil, false, exception.Validate("用户角色参数无效")
	}

	return auth.NormalizeIDs(roleIDs), true, nil
}

func userDepartmentIDs(value *coreservice.Mutable[entity.User]) ([]uint64, error) {
	if value == nil || !value.Has("departmentId") || value.IsNull("departmentId") {
		return nil, nil
	}
	current, _ := value.Get("departmentId")
	departmentID, ok := current.(uint64)
	if !ok {
		return nil, exception.Validate("用户部门参数无效")
	}
	if departmentID == 0 {
		return nil, nil
	}

	return []uint64{departmentID}, nil
}

func userNextStatus(value *coreservice.Mutable[entity.User], current int32) (int32, error) {
	if !value.Has("status") {
		return current, nil
	}
	next, exists := value.Get("status")
	status, ok := next.(int32)
	if !exists || !ok {
		return 0, exception.Validate("用户状态无效")
	}

	return status, nil
}

func userHasPersistentUpdate(descriptor coreentity.Metadata, value *coreservice.Mutable[entity.User]) bool {
	for _, field := range descriptor.PersistentFields() {
		if value.Has(field.Name()) {
			return true
		}
	}

	return false
}

func userPageOrder(descriptor coreentity.Metadata, order, sort string) (string, bool, error) {
	order = strings.TrimSpace(order)
	sort = strings.TrimSpace(sort)
	if order == "" && sort == "" {
		return descriptor.Primary().Column(), true, nil
	}
	field, exists := descriptor.JSON(order)
	if !exists || !field.Persistent() || order == entity.PasswordFieldName {
		return "", false, exception.Validate("用户排序字段无效")
	}
	switch sort {
	case "asc":
		return field.Column(), false, nil
	case "desc":
		return field.Column(), true, nil
	default:
		return "", false, exception.Validate("用户排序方向无效")
	}
}

func userInfoResult(row userRow) dto.UserInfoResult {
	return dto.UserInfoResult{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		DepartmentID: row.DepartmentID, UserID: row.UserID, Name: row.Name,
		Username: row.Username, PasswordV: row.PasswordV, NickName: row.NickName,
		HeadImg: row.HeadImg, Phone: row.Phone, Email: row.Email, Remark: row.Remark,
		Status: row.Status, SocketID: row.SocketID,
	}
}
