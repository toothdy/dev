package eps

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

func TestGenerateBuildsCRUDAndCustomAPIs(t *testing.T) {
	definition := controller.Admin("base/sys/user").
		Name("BaseSysUserEntity").
		Description("用户管理").
		Model(testUserModel()).
		CRUD(controller.CRUDOptions{
			API: []string{crud.Add, crud.Info, crud.Page},
			PageQuery: controller.QueryOptions{
				KeyWordLikeFields: []string{"username"},
				FieldEq:           []string{"status"},
			},
		}).
		Build()
	open := controller.Open("base/open").
		Name("BaseOpenController").
		Route(controller.RouteOptions{
			Name: "login", Method: http.MethodPost, Path: "/login",
			Description: "登录", IgnoreAuth: true,
		}).
		Build()

	data := Generate([]controller.Definition{definition, open})
	if len(data["base"]) != 2 {
		t.Fatalf("expected two base EPS controllers, got %#v", data)
	}
	user := data["base"][0]
	if user.API[0].Method != http.MethodPost || user.API[0].Path != "/add" || user.API[0].Prefix != "/admin/base/sys/user" {
		t.Fatalf("unexpected CRUD api: %#v", user.API[0])
	}
	if user.PageQueryOp.KeyWordLikeFields[0] != "a.username" || user.PageQueryOp.FieldEq[0] != "a.status" {
		t.Fatalf("unexpected page query op: %#v", user.PageQueryOp)
	}
	if data["base"][1].API[0].IgnoreToken != true || data["base"][1].API[0].Path != "/login" {
		t.Fatalf("unexpected custom API: %#v", data["base"][1].API[0])
	}
}

func TestGenerateMapsModelFieldsAndKeepsEmptyQueryArrays(t *testing.T) {
	definition := controller.Admin("base/sys/user").
		Model(testUserModel()).
		CRUD(controller.CRUDOptions{}).
		Build()

	item := Generate([]controller.Definition{definition})["base"][0]
	if item.Columns[0].PropertyName != "id" || item.Columns[0].Type != "int" || item.Columns[0].Source != "a.id" {
		t.Fatalf("unexpected id column: %#v", item.Columns[0])
	}
	if item.Columns[1].PropertyName != "username" || item.Columns[1].Type != "varchar" || item.Columns[1].Length != "100" {
		t.Fatalf("unexpected username column: %#v", item.Columns[1])
	}
	if item.Columns[2].PropertyName != "status" || item.Columns[2].Type != "int" || item.Columns[2].DefaultValue != int64(1) || !reflect.DeepEqual(item.Columns[2].Dict, []string{"禁用", "启用"}) {
		t.Fatalf("unexpected status column: %#v", item.Columns[2])
	}
	if item.PageQueryOp.KeyWordLikeFields == nil || item.PageQueryOp.FieldEq == nil || item.PageQueryOp.FieldLike == nil {
		t.Fatalf("expected non-nil empty page query arrays: %#v", item.PageQueryOp)
	}
}

func TestGenerateOmitsFieldsThatAreHiddenAndReadonly(t *testing.T) {
	definition := controller.Admin("task/info").
		Model(entity.NewDefinition("task", "TaskInfo", "task_info").Fields([]entity.Field{
			entity.NewField("name", "name", "varchar"),
			entity.NewField("lockOwner", "lockOwner", "varchar"),
			entity.NewField("secret", "secret", "varchar"),
		})).
		CRUD(controller.CRUDOptions{
			HiddenFields:   []string{"lockOwner", "secret"},
			ReadonlyFields: []string{"lockOwner"},
		}).
		Build()

	columns := Generate([]controller.Definition{definition})["task"]
	if len(columns) != 1 || len(columns[0].Columns) != 2 {
		t.Fatalf("unexpected internal field filtering: %#v", columns)
	}
	if columns[0].Columns[0].PropertyName != "name" || columns[0].Columns[1].PropertyName != "secret" {
		t.Fatalf("hidden writable fields must remain in EPS: %#v", columns[0].Columns)
	}
}

func testUserModel() entity.Definition {
	return entity.NewDefinition("base", "BaseSysUser", "base_sys_user").Fields([]entity.Field{
		entity.NewField("id", "id", "bigint").Primary().Comment("ID"),
		entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		entity.NewField("status", "status", "tinyint").NotNull().Default("1").Comment("状态").WithDict("禁用", "启用"),
	})
}

func TestGeneratePreservesControllerAndAPIOrder(t *testing.T) {
	first := controller.Admin("base/sys/first").
		Name("First").
		CRUD(controller.CRUDOptions{API: []string{crud.Page, crud.Add}}).
		Route(controller.RouteOptions{Name: "custom", Method: http.MethodGet, Path: "/custom"}).
		Build()
	second := controller.Open("base/open").
		Name("Second").
		Route(controller.RouteOptions{Name: "login", Method: http.MethodPost, Path: "/login"}).
		Build()

	controllers := Generate([]controller.Definition{first, second})["base"]
	if controllers[0].Name != "First" || controllers[1].Name != "Second" {
		t.Fatalf("unexpected controller order: %#v", controllers)
	}
	api := controllers[0].API
	if api[0].Path != "/page" || api[1].Path != "/add" || api[2].Path != "/custom" {
		t.Fatalf("unexpected API order: %#v", api)
	}
}

func TestGenerateSeparatesAdminAndAppControllers(t *testing.T) {
	admin := controller.Admin("base/open").Name("Admin").Route(controller.RouteOptions{Name: "eps", Method: http.MethodGet, Path: "/eps"}).Build()
	app := controller.App("base/comm").Name("App").Route(controller.RouteOptions{Name: "eps", Method: http.MethodGet, Path: "/eps"}).Build()
	if controllers := GenerateAdmin([]controller.Definition{admin, app})["base"]; len(controllers) != 1 || controllers[0].Name != "Admin" {
		t.Fatalf("unexpected admin EPS: %#v", controllers)
	}
	if controllers := GenerateApp([]controller.Definition{admin, app})["base"]; len(controllers) != 1 || controllers[0].Name != "App" || controllers[0].Prefix != "/app/base/comm" {
		t.Fatalf("unexpected app EPS: %#v", controllers)
	}
}
