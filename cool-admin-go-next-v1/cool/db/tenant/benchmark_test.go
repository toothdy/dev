package tenant_test

import (
	"context"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

var (
	benchmarkScope     tenant.Scope
	benchmarkCondition tenant.Condition
)

// BenchmarkResolveScope 测量租户作用域解析开销。
func BenchmarkResolveScope(b *testing.B) {
	identity, err := security.NewTenantIdentity(17)
	if err != nil {
		b.Fatalf("create tenant identity failed: %v", err)
	}
	ctx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkScope = tenant.Resolve(ctx)
	}
}

// BenchmarkTenantPredicate 测量已编译元数据的谓词构建开销。
func BenchmarkTenantPredicate(b *testing.B) {
	metadata, err := tenant.CompileMetadata(
		entity.NewDefinition("benchmark", "Goods", "benchmark_goods").Fields(entity.BaseFields()),
	)
	if err != nil {
		b.Fatalf("compile tenant metadata failed: %v", err)
	}
	ctx, err := tenant.ForTenant(context.Background(), 17)
	if err != nil {
		b.Fatalf("create tenant scope failed: %v", err)
	}
	scope := tenant.Resolve(ctx)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkCondition, err = tenant.PredicateForScope(scope, metadata, "g")
		if err != nil {
			b.Fatalf("build tenant predicate failed: %v", err)
		}
	}
}
