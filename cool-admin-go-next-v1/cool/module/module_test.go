package module_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestDefinitionConfig(t *testing.T) {
	cfg := module.Config{
		Description: "系统基础能力",
		Order:       100,
	}

	mod := module.New("base").Name("基础模块").Config(cfg)

	if mod.Key() != "base" {
		t.Fatalf("unexpected key: %s", mod.Key())
	}
	if mod.NameText() != "基础模块" {
		t.Fatalf("unexpected name: %s", mod.NameText())
	}
	if mod.ModuleConfig().Description != "系统基础能力" {
		t.Fatalf("unexpected description: %s", mod.ModuleConfig().Description)
	}
}

func TestDefinitionModels(t *testing.T) {
	userModel := entity.NewDefinition("base", "BaseSysUser", "base_sys_user")
	roleModel := entity.NewDefinition("base", "BaseSysRole", "base_sys_role")

	mod := module.New("base").Models([]entity.Definition{userModel, roleModel})

	models := mod.ModuleModels()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].TableName != "base_sys_user" {
		t.Fatalf("expected first table base_sys_user, got %s", models[0].TableName)
	}
}

func TestDefinitionSeeds(t *testing.T) {
	mod := module.New("base").Seeds("modules/base/db.json", "modules/base/menu.json")
	seeds := mod.ModuleSeeds()

	if seeds.DBPath != "modules/base/db.json" {
		t.Fatalf("unexpected db seed path: %s", seeds.DBPath)
	}
	if seeds.MenuPath != "modules/base/menu.json" {
		t.Fatalf("unexpected menu seed path: %s", seeds.MenuPath)
	}
}

/**
 * 测试模块保存 controller 元数据
 * @param t 测试对象
 * @returns null
 */
func TestModuleControllersReturnsCopy(t *testing.T) {
	controllers := []controller.Definition{
		controller.Admin("base/sys/user").Build(),
	}
	mod := module.New("base").Controllers(controllers)
	controllers[0] = controller.Admin("base/sys/role").Build()

	stored := mod.ModuleControllers()
	if len(stored) != 1 {
		t.Fatalf("expected one controller, got %d", len(stored))
	}
	if stored[0].Prefix != "/admin/base/sys/user" {
		t.Fatalf("expected user prefix, got %s", stored[0].Prefix)
	}

	stored[0] = controller.Admin("base/sys/menu").Build()
	fresh := mod.ModuleControllers()
	if fresh[0].Prefix != "/admin/base/sys/user" {
		t.Fatalf("expected copied controller slice, got %s", fresh[0].Prefix)
	}
}

/**
 * 构造包含嵌套可变元数据的 controller
 * @returns controller.Definition
 */
func newMutableControllerDefinition() controller.Definition {
	return controller.Admin("base/sys/user").
		Model(entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
			Fields([]entity.Field{
				entity.NewField("status", "status", "tinyint").WithDict("启用", "禁用"),
			}).
			WithIndexes(entity.NewIndex("idx_status", "status"))).
		CRUD(controller.CRUDOptions{
			API: []string{"page"},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"name"},
			},
		}).
		Route(controller.RouteOptions{
			Name:   "detail",
			Method: "GET",
			Path:   "/detail",
		}).
		Build()
}

/**
 * 测试设置 controller 时深复制嵌套元数据
 * @param t 测试对象
 * @returns null
 */
func TestControllersDeepCopiesDefinition(t *testing.T) {
	source := newMutableControllerDefinition()
	mod := module.New("base").Controllers([]controller.Definition{source})

	source.Routes[0].Path = "/changed"
	source.CRUD.API[0] = "changed"
	source.CRUD.PageQuery.KeyWordLikeFields[0] = "changed"
	source.Model.FieldsValue[0].JSONName = "changed"
	source.Model.FieldsValue[0].Dict[0] = "changed"
	source.Model.Indexes[0].Columns[0] = "changed"

	stored := mod.ModuleControllers()[0]
	if stored.Routes[0].Path != "/detail" {
		t.Errorf("expected original route path, got %s", stored.Routes[0].Path)
	}
	if stored.CRUD.API[0] != "page" {
		t.Errorf("expected original CRUD API, got %s", stored.CRUD.API[0])
	}
	if stored.CRUD.PageQuery.KeyWordLikeFields[0] != "name" {
		t.Errorf("expected original query field, got %s", stored.CRUD.PageQuery.KeyWordLikeFields[0])
	}
	if stored.Model.FieldsValue[0].JSONName != "status" {
		t.Errorf("expected original model field, got %s", stored.Model.FieldsValue[0].JSONName)
	}
	if stored.Model.FieldsValue[0].Dict[0] != "启用" {
		t.Errorf("expected original model dict, got %s", stored.Model.FieldsValue[0].Dict[0])
	}
	if stored.Model.Indexes[0].Columns[0] != "status" {
		t.Errorf("expected original model index column, got %s", stored.Model.Indexes[0].Columns[0])
	}
}

/**
 * 测试获取 controller 时深复制嵌套元数据
 * @param t 测试对象
 * @returns null
 */
func TestModuleControllersDeepCopiesDefinition(t *testing.T) {
	mod := module.New("base").Controllers([]controller.Definition{newMutableControllerDefinition()})
	returned := mod.ModuleControllers()[0]

	returned.Routes[0].Path = "/changed"
	returned.CRUD.API[0] = "changed"
	returned.CRUD.PageQuery.KeyWordLikeFields[0] = "changed"
	returned.Model.FieldsValue[0].JSONName = "changed"
	returned.Model.FieldsValue[0].Dict[0] = "changed"
	returned.Model.Indexes[0].Columns[0] = "changed"

	fresh := mod.ModuleControllers()[0]
	if fresh.Routes[0].Path != "/detail" {
		t.Errorf("expected original route path, got %s", fresh.Routes[0].Path)
	}
	if fresh.CRUD.API[0] != "page" {
		t.Errorf("expected original CRUD API, got %s", fresh.CRUD.API[0])
	}
	if fresh.CRUD.PageQuery.KeyWordLikeFields[0] != "name" {
		t.Errorf("expected original query field, got %s", fresh.CRUD.PageQuery.KeyWordLikeFields[0])
	}
	if fresh.Model.FieldsValue[0].JSONName != "status" {
		t.Errorf("expected original model field, got %s", fresh.Model.FieldsValue[0].JSONName)
	}
	if fresh.Model.FieldsValue[0].Dict[0] != "启用" {
		t.Errorf("expected original model dict, got %s", fresh.Model.FieldsValue[0].Dict[0])
	}
	if fresh.Model.Indexes[0].Columns[0] != "status" {
		t.Errorf("expected original model index column, got %s", fresh.Model.Indexes[0].Columns[0])
	}
}

/**
 * 测试收集模块 controller 元数据
 * @param t 测试对象
 * @returns null
 */
func TestCollectControllers(t *testing.T) {
	mods := []module.Module{
		module.New("base").Controllers([]controller.Definition{controller.Admin("base/sys/user").Build()}),
		module.New("demo").Controllers([]controller.Definition{controller.Admin("demo/goods").Build()}),
	}

	controllers := module.CollectControllers(mods)
	if len(controllers) != 2 {
		t.Fatalf("expected two controllers, got %d", len(controllers))
	}
	if controllers[0].Prefix != "/admin/base/sys/user" {
		t.Errorf("expected first controller from base module, got %s", controllers[0].Prefix)
	}
	if controllers[1].Prefix != "/admin/demo/goods" {
		t.Errorf("expected second controller from demo module, got %s", controllers[1].Prefix)
	}
}

/**
 * 测试空模块列表收集结果
 * @param t 测试对象
 * @returns null
 */
func TestCollectControllersEmptyInput(t *testing.T) {
	controllers := module.CollectControllers([]module.Module{})
	if controllers == nil {
		t.Fatal("expected non-nil empty controller slice")
	}
	if len(controllers) != 0 {
		t.Fatalf("expected no controllers, got %d", len(controllers))
	}
}
