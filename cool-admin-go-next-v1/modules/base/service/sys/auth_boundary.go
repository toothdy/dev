package sys

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
)

const platformAdminRoleLabel = "admin"

var (
	errAuthorizationUserMissing  = errors.New("authorization user missing")
	errAuthorizationUserInactive = errors.New("authorization user inactive")
	errAuthorizationUserRoleless = errors.New("authorization user has no role")
)

const platformAdminCountSQL = `SELECT COUNT(*)
	FROM base_sys_user u
	INNER JOIN base_sys_user_role ur ON ur.userId = u.id
	INNER JOIN base_sys_role r ON r.id = ur.roleId
	WHERE u.id = ? AND u.status = 1
	AND (u.tenantId IS NULL OR u.tenantId = 0)
	AND r.label = ? AND (r.tenantId IS NULL OR r.tenantId = 0)`

type permissionDatabase interface {
	GetCount(ctx context.Context, sql string, args ...any) (int, error)
	GetAll(ctx context.Context, sql string, args ...any) (gdb.Result, error)
}

type authorizationModeler interface {
	Model(tableNameQueryOrStruct ...any) *gdb.Model
}

type authorizationUser struct {
	ID                 int64
	Status             int64
	TenantID           any
	HasGlobalAdminRole bool
}

func (u authorizationUser) IsProtectedPlatformAdmin() bool {
	return u.ID > 0 && u.Status == 1 && tenantScope(u.TenantID) == 0 && u.HasGlobalAdminRole
}

type authorizationRole struct {
	ID           int64
	Label        string
	TenantID     any
	CreatorID    int64
	HeldByCaller bool
}

func isPlatformAdministrator(ctx context.Context, db permissionDatabase, userID int64) (bool, error) {
	if db == nil || userID <= 0 {
		return false, nil
	}
	count, err := db.GetCount(ctx, platformAdminCountSQL, userID, platformAdminRoleLabel)
	if err != nil {
		return false, gerror.Wrap(err, "查询用户超管角色失败")
	}
	return count > 0, nil
}

func rolesAreAssignable(
	callerAdmin bool,
	callerID int64,
	callerTenant any,
	targetTenant any,
	roles []authorizationRole,
) bool {
	if !callerAdmin && !sameTenantScope(callerTenant, targetTenant) {
		return false
	}
	for _, role := range roles {
		if role.ID <= 0 ||
			(role.Label == platformAdminRoleLabel && tenantScope(role.TenantID) == 0) ||
			!sameTenantScope(role.TenantID, targetTenant) {
			return false
		}
		if !callerAdmin && role.CreatorID != callerID && !role.HeldByCaller {
			return false
		}
	}
	return true
}

func sameTenantScope(left any, right any) bool {
	return tenantScope(left) == tenantScope(right)
}

func tenantScope(value any) int64 {
	return int64Value(value)
}

func normalizeAuthorizationUserIDs(userIDs []int64) []int64 {
	unique := make(map[int64]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID > 0 {
			unique[userID] = struct{}{}
		}
	}
	result := make([]int64, 0, len(unique))
	for userID := range unique {
		result = append(result, userID)
	}
	sort.Slice(result, func(left int, right int) bool {
		return result[left] < result[right]
	})
	return result
}

func authorizationUserLockQuery(userIDs []int64) (string, []any) {
	normalized := normalizeAuthorizationUserIDs(userIDs)
	if len(normalized) == 0 {
		return "", nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(normalized)), ", ")
	arguments := make([]any, len(normalized))
	for index, userID := range normalized {
		arguments[index] = userID
	}
	return "SELECT id FROM base_sys_user WHERE id IN (" + placeholders + ") ORDER BY id FOR UPDATE", arguments
}

func lockAuthorizationUsers(ctx context.Context, tx gdb.TX, userIDs []int64) error {
	query, arguments := authorizationUserLockQuery(userIDs)
	if query == "" {
		return nil
	}
	rows, err := tx.Ctx(ctx).GetAll(query, arguments...)
	if err != nil {
		return gerror.Wrap(err, "锁定授权用户失败")
	}
	if len(rows) != len(arguments) {
		return errAuthorizationUserMissing
	}
	return nil
}

func claimsFromUserSnapshot(user map[string]interface{}, roleIDs []int64) (security.Claims, error) {
	if int64Value(user["id"]) <= 0 {
		return security.Claims{}, errAuthorizationUserMissing
	}
	if int64Value(user["status"]) != 1 {
		return security.Claims{}, errAuthorizationUserInactive
	}
	if len(roleIDs) == 0 {
		return security.Claims{}, errAuthorizationUserRoleless
	}
	tenantIdentity, err := tenantIdentityFromDatabase(user["tenantId"])
	if err != nil {
		return security.Claims{}, err
	}
	return security.Claims{
		RoleIds:         append([]int64(nil), roleIDs...),
		Username:        stringValue(user["username"]),
		UserId:          int64Value(user["id"]),
		PasswordVersion: int64Value(user["passwordV"]),
		TenantId:        tenantIdentity,
	}, nil
}

/**
 * 将数据库租户值转换为认证身份
 * @param value 数据库租户值
 * @returns 租户身份和转换错误
 */
func tenantIdentityFromDatabase(value interface{}) (security.TenantIdentity, error) {
	if value == nil {
		return security.PlatformTenant(), nil
	}
	var tenantID int64
	switch item := value.(type) {
	case int:
		tenantID = int64(item)
	case int8:
		tenantID = int64(item)
	case int16:
		tenantID = int64(item)
	case int32:
		tenantID = int64(item)
	case int64:
		tenantID = item
	case uint:
		if uint64(item) > uint64(^uint64(0)>>1) {
			return security.TenantIdentity{}, gerror.New("数据库租户 ID 超出范围")
		}
		tenantID = int64(item)
	case uint8:
		tenantID = int64(item)
	case uint16:
		tenantID = int64(item)
	case uint32:
		tenantID = int64(item)
	case uint64:
		if item > uint64(^uint64(0)>>1) {
			return security.TenantIdentity{}, gerror.New("数据库租户 ID 超出范围")
		}
		tenantID = int64(item)
	case []byte:
		parsed, err := strconv.ParseInt(string(item), 10, 64)
		if err != nil {
			return security.TenantIdentity{}, gerror.Wrap(err, "数据库租户 ID 格式错误")
		}
		tenantID = parsed
	case string:
		parsed, err := strconv.ParseInt(item, 10, 64)
		if err != nil {
			return security.TenantIdentity{}, gerror.Wrap(err, "数据库租户 ID 格式错误")
		}
		tenantID = parsed
	default:
		return security.TenantIdentity{}, gerror.Newf("数据库租户 ID 类型错误: %T", value)
	}
	if tenantID == 0 {
		return security.PlatformTenant(), nil
	}
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		return security.TenantIdentity{}, gerror.Wrap(err, "数据库租户 ID 无效")
	}
	return identity, nil
}

func authorizationUserFromDatabase(
	ctx context.Context,
	db authorizationModeler,
	userID int64,
) (authorizationUser, error) {
	row, err := db.Model("base_sys_user").Ctx(ctx).
		Fields("id", "status", "tenantId").
		Where("id", userID).
		One()
	if err != nil {
		return authorizationUser{}, gerror.Wrap(err, "查询授权用户失败")
	}
	if row.IsEmpty() {
		return authorizationUser{}, errAuthorizationUserMissing
	}
	adminRoleCount, err := db.Model("base_sys_user_role ur").Ctx(ctx).
		InnerJoin("base_sys_role r", "r.id = ur.roleId").
		Where("ur.userId", userID).
		Where("r.label", platformAdminRoleLabel).
		Where("(r.tenantId IS NULL OR r.tenantId = ?)", 0).
		Count()
	if err != nil {
		return authorizationUser{}, gerror.Wrap(err, "查询用户超管关系失败")
	}
	return authorizationUser{
		ID:                 row["id"].Int64(),
		Status:             row["status"].Int64(),
		TenantID:           row["tenantId"].Val(),
		HasGlobalAdminRole: adminRoleCount > 0,
	}, nil
}

func authorizationRolesFromDatabase(
	ctx context.Context,
	db authorizationModeler,
	callerID int64,
	roleIDs []int64,
) ([]authorizationRole, error) {
	if len(roleIDs) == 0 {
		return []authorizationRole{}, nil
	}
	normalized := normalizeAuthorizationUserIDs(roleIDs)
	rows, err := db.Model("base_sys_role").Ctx(ctx).
		Fields("id", "label", "tenantId", "userId").
		WhereIn("id", normalized).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询授权角色失败")
	}
	if len(rows) != len(normalized) {
		return nil, exception.Forbidden("非法操作")
	}
	heldRows, err := db.Model("base_sys_user_role").Ctx(ctx).
		Fields("roleId").
		Where("userId", callerID).
		WhereIn("roleId", normalized).
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "查询调用者角色失败")
	}
	held := make(map[int64]struct{}, len(heldRows))
	for _, row := range heldRows {
		held[row["roleId"].Int64()] = struct{}{}
	}
	roles := make([]authorizationRole, 0, len(rows))
	for _, row := range rows {
		roleID := row["id"].Int64()
		_, heldByCaller := held[roleID]
		roles = append(roles, authorizationRole{
			ID:           roleID,
			Label:        row["label"].String(),
			TenantID:     row["tenantId"].Val(),
			CreatorID:    row["userId"].Int64(),
			HeldByCaller: heldByCaller,
		})
	}
	return roles, nil
}

func ensureUserMutationAllowed(
	ctx context.Context,
	db authorizationModeler,
	callerID int64,
	target authorizationUser,
	roleIDs []int64,
	validateRoles bool,
) error {
	caller, err := authorizationUserFromDatabase(ctx, db, callerID)
	if err != nil {
		if errors.Is(err, errAuthorizationUserMissing) {
			return exception.Forbidden("非法操作")
		}
		return err
	}
	if caller.Status != 1 || target.IsProtectedPlatformAdmin() {
		return exception.Forbidden("非法操作")
	}
	roles := []authorizationRole{}
	if validateRoles {
		roles, err = authorizationRolesFromDatabase(ctx, db, callerID, roleIDs)
		if err != nil {
			return err
		}
	}
	if !rolesAreAssignable(
		caller.IsProtectedPlatformAdmin(),
		caller.ID,
		caller.TenantID,
		target.TenantID,
		roles,
	) {
		return exception.Forbidden("非法操作")
	}
	return nil
}

func requireAuthorizationCaller(ctx context.Context) (security.UserContext, error) {
	user, ok := security.UserFromContext(ctx)
	if !ok || user.UserId <= 0 {
		return security.UserContext{}, exception.Unauthorized()
	}
	return user, nil
}

func revokeAuthorizationSessions(
	ctx context.Context,
	sessions security.SessionStore,
	userIDs []int64,
	operation string,
) error {
	normalized := normalizeAuthorizationUserIDs(userIDs)
	if len(normalized) == 0 {
		return nil
	}
	if sessions == nil {
		return exception.Internal(nil, operation)
	}
	if err := sessions.DeleteUsers(ctx, normalized); err != nil {
		return exception.Internal(err, operation)
	}
	return nil
}

func isProtectedAuthorizationRole(label string, tenantID any) bool {
	return label == platformAdminRoleLabel && tenantScope(tenantID) == 0
}
