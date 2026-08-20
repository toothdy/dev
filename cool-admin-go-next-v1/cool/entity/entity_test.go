package entity_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func TestBaseFields(t *testing.T) {
	fields := entity.BaseFields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 base fields, got %d", len(fields))
	}

	checks := map[string]string{
		"id":         "id",
		"createTime": "createTime",
		"updateTime": "updateTime",
		"tenantId":   "tenantId",
	}
	for columnName, jsonName := range checks {
		found := false
		for _, field := range fields {
			if field.ColumnName == columnName && field.JSONName == jsonName {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing base field %s/%s", columnName, jsonName)
		}
	}
}

func TestDefinitionFieldByColumn(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Comment("系统用户").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
			entity.NewField("nickName", "nickName", "varchar").Size(100).Comment("昵称"),
		})

	field, ok := definition.FieldByColumn("nickName")
	if !ok {
		t.Fatal("expected nick_name field")
	}
	if field.JSONName != "nickName" {
		t.Fatalf("expected json name nickName, got %s", field.JSONName)
	}
}

func TestDefinitionFieldByJSONName(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Fields([]entity.Field{
			entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
			entity.NewField("nickName", "nickName", "varchar").Size(100).Comment("昵称"),
		})

	field, ok := definition.FieldByJSONName("nickName")
	if !ok {
		t.Fatal("expected nickName field")
	}
	if field.ColumnName != "nickName" {
		t.Fatalf("expected column name nick_name, got %s", field.ColumnName)
	}
}

func TestDefinitionPrimaryField(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		})

	field, ok := definition.PrimaryField()
	if !ok {
		t.Fatal("expected primary field")
	}
	if field.JSONName != "id" || field.ColumnName != "id" {
		t.Fatalf("unexpected primary field: %#v", field)
	}
}

func TestDefinitionResourceKeyUsesExplicitAndStableFallback(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user")
	if resource := definition.ResourceKey(); resource != "base.user" {
		t.Fatalf("unexpected fallback resource key: %s", resource)
	}
	if resource := definition.WithResource("base.user").ResourceKey(); resource != "base.user" {
		t.Fatalf("unexpected explicit resource key: %s", resource)
	}
}

func TestIndexConstructors(t *testing.T) {
	index := entity.NewIndex("idx_user_status", "status", "tenantId")
	if index.Name != "idx_user_status" {
		t.Fatalf("unexpected index name: %s", index.Name)
	}
	if index.IsUnique {
		t.Fatal("expected normal index")
	}
	if len(index.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(index.Columns))
	}

	uniqueIndex := entity.NewUniqueIndex("uk_user_username", "username")
	if !uniqueIndex.IsUnique {
		t.Fatal("expected unique index")
	}
}

/**
 * 验证模型租户模式可显式设置
 * @param t 测试上下文
 * @returns null
 */
func TestDefinitionTenantMode(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user")
	if definition.TenantMode != entity.TenantModeAuto {
		t.Fatalf("expected auto tenant mode, got %d", definition.TenantMode)
	}
	disabled := definition.WithTenantMode(entity.TenantModeDisabled)
	if disabled.TenantMode != entity.TenantModeDisabled {
		t.Fatalf("expected disabled tenant mode, got %d", disabled.TenantMode)
	}
	if definition.TenantMode != entity.TenantModeAuto {
		t.Fatal("tenant mode builder mutated original definition")
	}
}