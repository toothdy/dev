package tenant_test

import (
	"context"
	"sync"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

/**
 * 验证请求上下文解析为四种租户作用域
 * @param t 测试上下文
 * @returns null
 */
func TestResolveScopeKinds(t *testing.T) {
	if tenant.Resolve(context.Background()).Kind() != tenant.KindMissing {
		t.Fatal("expected missing scope")
	}
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	if tenant.Resolve(platformCtx).Kind() != tenant.KindPlatform {
		t.Fatal("expected platform scope")
	}
	tenantIdentity, err := security.NewTenantIdentity(8)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	tenantCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: tenantIdentity})
	resolved := tenant.Resolve(tenantCtx)
	if tenantID, ok := resolved.TenantID(); !ok || tenantID != 8 {
		t.Fatalf("expected tenant 8, got %d, %v", tenantID, ok)
	}
	if tenant.Resolve(tenant.WithoutTenant(tenantCtx)).Kind() != tenant.KindBypass {
		t.Fatal("expected bypass scope")
	}
}

/**
 * 验证派生作用域不修改原上下文
 * @param t 测试上下文
 * @returns null
 */
func TestDerivedScopeDoesNotMutateParent(t *testing.T) {
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	tenantCtx, err := tenant.ForTenant(platformCtx, 9)
	if err != nil {
		t.Fatalf("derive tenant scope failed: %v", err)
	}
	if tenant.Resolve(platformCtx).Kind() != tenant.KindPlatform {
		t.Fatal("parent scope was mutated")
	}
	if tenantID, ok := tenant.Resolve(tenantCtx).TenantID(); !ok || tenantID != 9 {
		t.Fatalf("expected derived tenant 9, got %d, %v", tenantID, ok)
	}
	if tenant.Resolve(tenant.WithoutTenant(tenantCtx)).Kind() != tenant.KindBypass {
		t.Fatal("expected innermost bypass override")
	}
	if _, err = tenant.ForTenant(platformCtx, 0); err == nil {
		t.Fatal("expected zero tenant override rejected")
	}
}

/**
 * 验证派生作用域并发读取互不干扰
 * @param t 测试上下文
 * @returns null
 */
func TestDerivedScopeConcurrentIsolation(t *testing.T) {
	parent := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	var waitGroup sync.WaitGroup
	for id := int64(1); id <= 20; id++ {
		waitGroup.Add(1)
		go func(tenantID int64) {
			defer waitGroup.Done()
			ctx, err := tenant.ForTenant(parent, tenantID)
			if err != nil {
				t.Errorf("derive tenant %d failed: %v", tenantID, err)
				return
			}
			got, ok := tenant.Resolve(ctx).TenantID()
			if !ok || got != tenantID {
				t.Errorf("expected tenant %d, got %d, %v", tenantID, got, ok)
			}
		}(id)
	}
	waitGroup.Wait()
	if tenant.Resolve(parent).Kind() != tenant.KindPlatform {
		t.Fatal("concurrent derived scopes mutated parent")
	}
}
