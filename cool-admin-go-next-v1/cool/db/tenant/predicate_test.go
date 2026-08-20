package tenant_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

/**
 * 验证具体租户生成参数化谓词
 * @param t 测试上下文
 * @returns null
 */
func TestPredicateUsesQualifiedParameter(t *testing.T) {
	identity, err := security.NewTenantIdentity(12)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	metadata := tenantMetadata(t)
	ctx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
	condition, err := tenant.Predicate(ctx, metadata, "a")
	if err != nil {
		t.Fatalf("build tenant predicate failed: %v", err)
	}
	if condition.SQL != "`a`.`tenantId` = ?" || !reflect.DeepEqual(condition.Args, []interface{}{int64(12)}) {
		t.Fatalf("unexpected tenant condition: %#v", condition)
	}
}

/**
 * 验证平台和 Bypass 不追加谓词
 * @param t 测试上下文
 * @returns null
 */
func TestPredicateAllowsPlatformAndBypass(t *testing.T) {
	metadata := tenantMetadata(t)
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	for _, ctx := range []context.Context{platformCtx, tenant.WithoutTenant(platformCtx)} {
		condition, err := tenant.Predicate(ctx, metadata, "")
		if err != nil || condition.SQL != "" || len(condition.Args) != 0 {
			t.Fatalf("expected empty condition, got %#v, %v", condition, err)
		}
	}
}

func TestGlobalOnlyPredicateUsesQualifiedNullCondition(t *testing.T) {
	metadata := tenantMetadata(t)
	condition, err := tenant.GlobalOnlyPredicate(metadata, "g")
	if err != nil {
		t.Fatalf("build global-only predicate failed: %v", err)
	}
	if condition.SQL != "`g`.`tenantId` IS NULL" || len(condition.Args) != 0 {
		t.Fatalf("unexpected global-only predicate: %#v", condition)
	}
	if _, err = tenant.GlobalOnlyPredicate(metadata, "g;drop"); err == nil {
		t.Fatal("expected unsafe global-only alias rejected")
	}
}

/**
 * 验证缺失作用域和非法别名被拒绝
 * @param t 测试上下文
 * @returns null
 */
func TestPredicateRejectsMissingScopeAndInvalidAlias(t *testing.T) {
	metadata := tenantMetadata(t)
	if _, err := tenant.Predicate(context.Background(), metadata, ""); err == nil {
		t.Fatal("expected missing tenant scope rejected")
	}
	ctx, err := tenant.ForTenant(context.Background(), 3)
	if err != nil {
		t.Fatalf("derive tenant scope failed: %v", err)
	}
	for _, alias := range []string{"a.b", "a`", "1a", "a b"} {
		if _, err = tenant.Predicate(ctx, metadata, alias); err == nil {
			t.Fatalf("expected invalid alias rejected: %s", alias)
		}
	}
}

/**
 * 编译测试租户元数据
 * @param t 测试上下文
 * @returns 租户元数据
 */
func tenantMetadata(t *testing.T) tenant.Metadata {
	t.Helper()
	metadata, err := tenant.CompileMetadata(
		entity.NewDefinition("demo", "Goods", "demo_goods").Fields(entity.BaseFields()),
	)
	if err != nil {
		t.Fatalf("compile tenant metadata failed: %v", err)
	}
	return metadata
}
