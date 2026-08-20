package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

type identityContextKey struct{}
type sessionContextKey struct{}
type permissionContextKey struct{}

type verifiedIdentity struct {
	subject Kind
	admin   AdminIdentity
	app     AppIdentity
}

// 已验证 Session 状态
type SessionState struct {
	ID        string    // Session ID
	Subject   Kind      // 身份种类
	AccessJTI string    // 当前 Access JTI
	ExpiresAt time.Time // Session 过期时间
}

// 已通过的权限信息
type PermissionState struct {
	Permission string // 权限标识
	Resource   string // 协议资源
}

// 返回管理端已验证身份
func Admin(ctx context.Context) (AdminIdentity, error) {
	identity, exists := identityFromContext(ctx)
	if !exists || identity.subject != AdminKind {
		return AdminIdentity{}, unauthenticatedError()
	}

	identity.admin.roleIDs = identity.admin.RoleIDs()
	return identity.admin, nil
}

// 返回应用端已验证身份
func App(ctx context.Context) (AppIdentity, error) {
	identity, exists := identityFromContext(ctx)
	if !exists || identity.subject != AppKind {
		return AppIdentity{}, unauthenticatedError()
	}

	return identity.app, nil
}

// 返回已验证 Session 状态
func Session(ctx context.Context) (SessionState, bool) {
	if ctx == nil {
		return SessionState{}, false
	}
	state, exists := ctx.Value(sessionContextKey{}).(SessionState)
	return state, exists
}

// 返回已通过的权限信息
func Permission(ctx context.Context) (PermissionState, bool) {
	if ctx == nil {
		return PermissionState{}, false
	}
	state, exists := ctx.Value(permissionContextKey{}).(PermissionState)
	return state, exists
}

// 写入管理端已验证身份
func withAdmin(ctx context.Context, userID uint64, username string, passwordV int, roleIDs []uint64) context.Context {
	return context.WithValue(ctx, identityContextKey{}, verifiedIdentity{
		subject: AdminKind,
		admin:   newAdminIdentity(userID, username, passwordV, roleIDs),
	})
}

// 写入应用端已验证身份
func withApp(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, identityContextKey{}, verifiedIdentity{
		subject: AppKind,
		app:     AppIdentity{ID: id},
	})
}

// 写入已验证 Session 状态
func withSession(ctx context.Context, state SessionState) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, state)
}

// 写入已通过的权限信息
func withPermission(ctx context.Context, state PermissionState) context.Context {
	return context.WithValue(ctx, permissionContextKey{}, state)
}

// 读取 Context 中的已验证身份
func identityFromContext(ctx context.Context) (verifiedIdentity, bool) {
	if ctx == nil {
		return verifiedIdentity{}, false
	}

	identity, exists := ctx.Value(identityContextKey{}).(verifiedIdentity)
	return identity, exists
}

// 未验证身份异常
func unauthenticatedError() error {
	return exception.Comm("身份未验证", http.StatusUnauthorized)
}
