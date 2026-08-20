package seed_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

func testUserDefinition() entity.Definition {
	return entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint"),
			entity.NewField("username", "username", "varchar"),
			entity.NewField("passwordV", "passwordV", "int"),
			entity.NewField("parentId", "parentId", "bigint"),
		})
}

func TestMapRecordConvertsJsonFieldsToColumns(t *testing.T) {
	models := seed.NewModelMap([]entity.Definition{testUserDefinition()})
	mapped, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
		"id":        float64(1),
		"username":  "admin",
		"passwordV": float64(1),
	}, nil)
	if err != nil {
		t.Fatalf("map record failed: %v", err)
	}

	if mapped.TableName != "base_sys_user" {
		t.Fatalf("unexpected table: %s", mapped.TableName)
	}
	if mapped.Values["passwordV"] != float64(1) {
		t.Fatalf("expected password_v to be mapped, got %#v", mapped.Values)
	}
}

func TestMapRecordRejectsUnknownField(t *testing.T) {
	models := seed.NewModelMap([]entity.Definition{testUserDefinition()})
	_, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
		"unknown": "value",
	}, nil)
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestMapRecordResolvesParentReference(t *testing.T) {
	models := seed.NewModelMap([]entity.Definition{testUserDefinition()})
	mapped, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
		"parentId": "@id",
	}, seed.RawRecord{
		"id": float64(9),
	})
	if err != nil {
		t.Fatalf("map record failed: %v", err)
	}
	if mapped.Values["parentId"] != float64(9) {
		t.Fatalf("expected parent reference value 9, got %#v", mapped.Values["parentId"])
	}
}

func TestMapRecordSkipsControlFields(t *testing.T) {
	models := seed.NewModelMap([]entity.Definition{testUserDefinition()})
	mapped, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
		"username":    "admin",
		"@childDatas": map[string]interface{}{},
		"childMenus":  []interface{}{},
	}, nil)
	if err != nil {
		t.Fatalf("map record failed: %v", err)
	}
	if _, ok := mapped.Values["@childDatas"]; ok {
		t.Fatal("expected @childDatas to be skipped")
	}
	if _, ok := mapped.Values["childMenus"]; ok {
		t.Fatal("expected childMenus to be skipped")
	}
}

func TestMapRecordAddsNodeVarcharTimestamps(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").Fields(append(
		entity.BaseFields(),
		entity.NewField("username", "username", "varchar"),
	))
	mapped, err := seed.MapRecord(seed.NewModelMap([]entity.Definition{definition}), "base_sys_user", seed.RawRecord{"username": "admin"}, nil)
	if err != nil {
		t.Fatalf("map record failed: %v", err)
	}
	if mapped.Values["createTime"] == "" || mapped.Values["updateTime"] == "" {
		t.Fatalf("expected generated seed timestamps, got %#v", mapped.Values)
	}
}
