package tenant

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

// Kind 表示当前请求的租户作用域类型
type Kind uint8

const (
	// KindMissing 表示请求没有租户身份或显式作用域
	KindMissing Kind = iota
	// KindPlatform 表示已认证的平台用户
	KindPlatform
	// KindTenant 表示具体租户
	KindTenant
	// KindBypass 表示显式跨租户内部操作
	KindBypass
)

type scopeContextKey struct{}

// Scope 表示一次数据库操作的不可变租户作用域
type Scope struct {
	kind     Kind
	tenantID int64
}

/**
 * 解析当前租户作用域
 * @param ctx 请求上下文
 * @returns 租户作用域
 */
func Resolve(ctx context.Context) Scope {
	if ctx == nil {
		return Scope{kind: KindMissing}
	}
	if override, ok := ctx.Value(scopeContextKey{}).(Scope); ok {
		return override
	}
	user, ok := security.UserFromContext(ctx)
	if !ok || user.TenantId.IsMissing() {
		return Scope{kind: KindMissing}
	}
	if user.TenantId.IsPlatform() {
		return Scope{kind: KindPlatform}
	}
	tenantID, ok := user.TenantId.TenantID()
	if !ok {
		return Scope{kind: KindMissing}
	}
	return Scope{kind: KindTenant, tenantID: tenantID}
}

/**
 * 创建指定租户的派生上下文
 * @param ctx 父上下文
 * @param tenantID 租户 ID
 * @returns 派生上下文和校验错误
 */
func ForTenant(ctx context.Context, tenantID int64) (context.Context, error) {
	if ctx == nil {
		return nil, gerror.New("父上下文不能为空")
	}
	if tenantID <= 0 {
		return nil, gerror.New("租户 ID 必须大于 0")
	}
	return context.WithValue(ctx, scopeContextKey{}, Scope{kind: KindTenant, tenantID: tenantID}), nil
}

/**
 * 创建显式跨租户派生上下文
 * @param ctx 父上下文
 * @returns 派生上下文
 */
func WithoutTenant(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, scopeContextKey{}, Scope{kind: KindBypass})
}

/**
 * 获取作用域类型
 * @returns 作用域类型
 */
func (s Scope) Kind() Kind {
	return s.kind
}

/**
 * 获取具体租户 ID
 * @returns 租户 ID 和是否存在
 */
func (s Scope) TenantID() (int64, bool) {
	if s.kind != KindTenant {
		return 0, false
	}
	return s.tenantID, true
}
