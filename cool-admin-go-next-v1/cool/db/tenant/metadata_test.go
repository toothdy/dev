package tenant_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

/**
 * 验证 BaseFields 自动启用租户元数据
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataAutoDetectsTenantField(t *testing.T) {
	definition := entity.NewDefinition("demo", "Goods", "demo_goods").Fields(entity.BaseFields())
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		t.Fatalf("compile tenant metadata failed: %v", err)
	}
	if !metadata.IsAware() || metadata.JSONField() != "tenantId" || metadata.Column() != "tenantId" {
		t.Fatalf("unexpected tenant metadata: %#v", metadata)
	}
}

/**
 * 验证无租户字段模型默认关闭
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataAllowsNonTenantModel(t *testing.T) {
	definition := entity.NewDefinition("demo", "Relation", "demo_relation").Fields([]entity.Field{
		entity.NewField("id", "id", "bigint").Unsigned().Primary(),
	})
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		t.Fatalf("compile non-tenant metadata failed: %v", err)
	}
	if metadata.IsAware() {
		t.Fatal("expected model without tenant field not tenant-aware")
	}
}

/**
 * 验证 Required 模式缺少字段时失败
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataRequiresTenantField(t *testing.T) {
	definition := entity.NewDefinition("demo", "Goods", "demo_goods").
		Fields([]entity.Field{entity.NewField("id", "id", "bigint").Unsigned().Primary()}).
		WithTenantMode(entity.TenantModeRequired)
	if _, err := tenant.CompileMetadata(definition); err == nil {
		t.Fatal("expected required tenant field error")
	}
}

/**
 * 验证非法租户字段定义失败
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataRejectsInvalidTenantField(t *testing.T) {
	tests := []entity.Field{
		entity.NewField("tenant", "tenantId", "bigint").Unsigned().Nullable(),
		entity.NewField("tenantId", "tenant", "bigint").Unsigned().Nullable(),
		entity.NewField("tenantId", "tenantId", "varchar").Unsigned().Nullable(),
		entity.NewField("tenantId", "tenantId", "bigint").Nullable(),
		entity.NewField("tenantId", "tenantId", "bigint").Unsigned(),
	}
	for _, field := range tests {
		definition := entity.NewDefinition("demo", "Goods", "demo_goods").Fields([]entity.Field{field})
		if _, err := tenant.CompileMetadata(definition); err == nil {
			t.Fatalf("expected invalid tenant field rejected: %#v", field)
		}
	}
}

/**
 * 验证显式关闭跳过租户字段
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataAllowsExplicitDisable(t *testing.T) {
	definition := entity.NewDefinition("demo", "Global", "demo_global").
		Fields(entity.BaseFields()).
		WithTenantMode(entity.TenantModeDisabled)
	metadata, err := tenant.CompileMetadata(definition)
	if err != nil {
		t.Fatalf("compile disabled tenant metadata failed: %v", err)
	}
	if metadata.IsAware() {
		t.Fatal("expected explicitly disabled model not tenant-aware")
	}
}

/**
 * 验证未知租户模式失败
 * @param t 测试上下文
 * @returns null
 */
func TestCompileMetadataRejectsUnknownMode(t *testing.T) {
	definition := entity.NewDefinition("demo", "Goods", "demo_goods")
	definition.TenantMode = entity.TenantMode(99)
	if _, err := tenant.CompileMetadata(definition); err == nil {
		t.Fatal("expected unknown tenant mode rejected")
	}
}
