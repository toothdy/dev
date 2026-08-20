package seed_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

func testDepartmentDefinition() entity.Definition {
	return entity.NewDefinition("base", "BaseSysDepartment", "base_sys_department").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint"),
			entity.NewField("name", "name", "varchar"),
			entity.NewField("parentId", "parentId", "bigint"),
		})
}

func testMenuDefinition() entity.Definition {
	return entity.NewDefinition("base", "BaseSysMenu", "base_sys_menu").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint"),
			entity.NewField("name", "name", "varchar"),
			entity.NewField("parentId", "parentId", "bigint"),
			entity.NewField("router", "router", "varchar"),
			entity.NewField("perms", "perms", "varchar"),
			entity.NewField("type", "type", "tinyint"),
			entity.NewField("icon", "icon", "varchar"),
			entity.NewField("orderNum", "orderNum", "int"),
			entity.NewField("viewPath", "viewPath", "varchar"),
			entity.NewField("keepAlive", "keepAlive", "tinyint"),
			entity.NewField("isShow", "isShow", "tinyint"),
		})
}

func TestParseDBContentExpandsChildDatas(t *testing.T) {
	models := seed.NewModelMap([]entity.Definition{testDepartmentDefinition()})

	records, err := seed.ParseDBContent([]byte(`{
		"base_sys_department": [
			{
				"id": 1,
				"name": "COOL",
				"@childDatas": {
					"base_sys_department": [
						{"id": 2, "name": "开发", "parentId": "@id"}
					]
				}
			}
		]
	}`), models)
	if err != nil {
		t.Fatalf("parse db content failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].Values["parentId"] != float64(1) {
		t.Fatalf("expected child parentId 1, got %#v", records[1].Values["parentId"])
	}
}

func TestParseMenuContentExpandsChildMenus(t *testing.T) {
	records, err := seed.ParseMenuContent([]byte(`[
		{
			"id": 1,
			"name": "系统管理",
			"type": 0,
			"childMenus": [
				{"id": 2, "name": "用户管理", "type": 1}
			]
		}
	]`), testMenuDefinition())
	if err != nil {
		t.Fatalf("parse menu content failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].ParentIndex != 0 {
		t.Fatalf("expected child parent index 0, got %d", records[1].ParentIndex)
	}
	if records[1].ParentColumn != "parentId" {
		t.Fatalf("expected parent column parentId, got %s", records[1].ParentColumn)
	}
}

func TestParseMenuContentResolvesExplicitParentReference(t *testing.T) {
	records, err := seed.ParseMenuContent([]byte(`[
		{
			"id": 1,
			"name": "系统管理",
			"type": 0,
			"childMenus": [
				{"id": 2, "name": "用户管理", "parentId": "@id", "type": 1}
			]
		}
	]`), testMenuDefinition())
	if err != nil {
		t.Fatalf("parse menu content failed: %v", err)
	}
	if records[1].Values["parentId"] != float64(1) {
		t.Fatalf("expected explicit parent reference 1, got %#v", records[1].Values["parentId"])
	}
}

func TestParseMenuContentKeepsParentIndexWithoutExplicitIds(t *testing.T) {
	records, err := seed.ParseMenuContent([]byte(`[
		{
			"name": "系统管理",
			"router": "/sys",
			"perms": null,
			"type": 0,
			"icon": "icon-set",
			"orderNum": 2,
			"viewPath": null,
			"keepAlive": true,
			"isShow": true,
			"childMenus": [
				{
					"name": "用户列表",
					"router": "/sys/user",
					"perms": null,
					"type": 1,
					"icon": "icon-user",
					"orderNum": 0,
					"viewPath": "modules/base/views/user/index.vue",
					"keepAlive": true,
					"isShow": true,
					"childMenus": []
				}
			]
		}
	]`), testMenuDefinition())
	if err != nil {
		t.Fatalf("parse menu content failed: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[1].ParentIndex != 0 {
		t.Fatalf("expected generated child to reference parent index 0, got %d", records[1].ParentIndex)
	}
	if _, ok := records[1].Values["parentId"]; ok {
		t.Fatalf("expected parentId to be absent before importer backfills generated id")
	}
}
