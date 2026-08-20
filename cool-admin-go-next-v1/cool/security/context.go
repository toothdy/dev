package security

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool/exception"
)

type userContextKey struct{}

// 当前登录用户上下文
type UserContext struct {
	SessionID       string
	AccessJTI       string
	UserId          int64
	Username        string
	RoleIds         []int64
	PasswordVersion int64
	TenantId        TenantIdentity
}

/**
 * 写入当前用户上下文
 * @param ctx 上下文
 * @param user 当前用户
 * @returns context.Context
 */
func ContextWithUser(ctx context.Context, user UserContext) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

/**
 * 从上下文读取当前用户
 * @param ctx 上下文
 * @returns UserContext 和是否存在
 */
func UserFromContext(ctx context.Context) (UserContext, bool) {
	user, ok := ctx.Value(userContextKey{}).(UserContext)
	return user, ok
}

// 返回当前登录用户，缺失时返回统一未认证错误
func RequireUser(ctx context.Context) (UserContext, error) {
	user, ok := UserFromContext(ctx)
	if !ok {
		return UserContext{}, exception.Unauthorized()
	}
	return user, nil
}
