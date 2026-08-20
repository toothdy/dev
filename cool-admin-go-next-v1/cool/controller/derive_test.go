package controller

import (
	"context"
	"net/http"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
)

type deriveService struct{}

/**
 * 创建测试模型定义
 * @returns entity.Definition
 */
func deriveModel() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields, entity.NewField("name", "name", "varchar").Comment("名称"))
	return entity.NewDefinition("base", "BaseSysUser", "base_sys_user").Fields(fields)
}

/**
 * 测试忽略认证路径从 route 元数据派生
 * @param t 测试对象
 * @returns null
 */
func TestIgnoreAuthPathsFromRoutes(t *testing.T) {
	controllers := []Definition{
		Open("base/open").
			Route(RouteOptions{Name: "login", Method: http.MethodPost, Path: "/login", IgnoreAuth: true}).
			Route(RouteOptions{Name: "private", Method: http.MethodGet, Path: "/private"}).
			Build(),
	}

	paths := IgnoreAuthPaths(controllers)
	if len(paths) != 1 || paths[0] != "/admin/base/open/login" {
		t.Fatalf("expected login ignore path, got %#v", paths)
	}
}

/**
 * 测试 CRUD 权限映射从 controller 派生
 * @param t 测试对象
 * @returns null
 */
func TestPermissionMapFromCRUDAndRoutes(t *testing.T) {
	controllers := []Definition{
		Admin("base/sys/user").
			CRUD(CRUDOptions{API: []string{crud.Page}}).
			Route(RouteOptions{Name: "move", Method: http.MethodPost, Path: "/move", Permission: "base:sys:user:move"}).
			Build(),
	}

	permissions, err := PermissionMap(controllers)
	if err != nil {
		t.Fatalf("build permission map failed: %v", err)
	}
	if permissions["POST:/admin/base/sys/user/page"] != "base:sys:user:page" {
		t.Fatalf("expected page permission, got %#v", permissions)
	}
	if permissions["POST:/admin/base/sys/user/move"] != "base:sys:user:move" {
		t.Fatalf("expected move permission, got %#v", permissions)
	}
}

/**
 * 测试 CRUD specs 从 controller 派生
 * @param t 测试对象
 * @returns null
 */
func TestCRUDResourceSpecsFromControllers(t *testing.T) {
	service := &deriveService{}
	insertParam := func(ctx context.Context) map[string]interface{} {
		return map[string]interface{}{"userId": int64(1)}
	}
	controllers := []Definition{
		Admin("base/sys/user").
			Model(deriveModel()).
			Service(service).
			CRUD(CRUDOptions{
				API:             []string{crud.Add, crud.Page},
				PageQuery:        QueryOptions{KeyWordLikeFields: []string{"name"}},
				InsertParam:      insertParam,
				InfoIgnoreFields: []string{"password"},
				SortFields:       []string{"id", "name"},
				HiddenFields:     []string{"password"},
				ReadonlyFields:   []string{"tenantId"},
				DefaultSort:      "id",
				DefaultOrder:     "DESC",
			}).
			Build(),
	}

	specs, err := CRUDResourceSpecs(controllers)
	if err != nil {
		t.Fatalf("build specs failed: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected one spec, got %d", len(specs))
	}
	spec := specs[0]
	if spec.Name != "base/sys/user" {
		t.Fatalf("expected resource base/sys/user, got %s", spec.Name)
	}
	if spec.Prefix != "/admin/base/sys/user" {
		t.Fatalf("expected prefix /admin/base/sys/user, got %s", spec.Prefix)
	}
	if spec.Service != service {
		t.Fatal("expected service to be carried into resource spec")
	}
	if spec.InsertParam == nil {
		t.Fatal("expected insert param function")
	}
	if len(spec.KeywordFields) != 1 || spec.KeywordFields[0] != "name" {
		t.Fatalf("expected keyword field name, got %#v", spec.KeywordFields)
	}
}

func TestCRUDResourceSpecsPreserveSeparateListAndPageQueryMetadata(t *testing.T) {
	controllers := []Definition{
		Admin("base/sys/user").
			Model(deriveModel()).
			CRUD(CRUDOptions{
				API: []string{crud.List, crud.Page},
				ListQuery: QueryOptions{
					FieldEq: []string{"name"},
				},
				PageQuery: QueryOptions{
					KeyWordLikeFields: []string{"name"},
					FieldEq:           []string{"id"},
				},
			}).
			Build(),
	}

	specs, err := CRUDResourceSpecs(controllers)
	if err != nil {
		t.Fatalf("build specs failed: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected one spec, got %d", len(specs))
	}
	if len(specs[0].ListQuery.EqualFields) != 1 || specs[0].ListQuery.EqualFields[0] != "name" {
		t.Fatalf("expected list query equal name, got %#v", specs[0].ListQuery)
	}
	if len(specs[0].PageQuery.KeywordFields) != 1 || specs[0].PageQuery.KeywordFields[0] != "name" {
		t.Fatalf("expected page query keyword name, got %#v", specs[0].PageQuery)
	}

	registry, err := crud.NewRegistry(specs)
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}
	resource, ok := registry.Resource("base/sys/user")
	if !ok {
		t.Fatal("expected user resource")
	}
	if resource.ListQuery.EqualFields["name"].JSONName != "name" {
		t.Fatalf("expected list query name filter, got %#v", resource.ListQuery)
	}
	if _, ok := resource.ListQuery.KeywordFields["name"]; ok {
		t.Fatalf("expected list query not to inherit page keyword filter, got %#v", resource.ListQuery)
	}
	if resource.PageQuery.KeywordFields["name"].JSONName != "name" {
		t.Fatalf("expected page query keyword filter, got %#v", resource.PageQuery)
	}
	if resource.PageQuery.EqualFields["id"].JSONName != "id" {
		t.Fatalf("expected page query id filter, got %#v", resource.PageQuery)
	}
}

/**
 * 测试不同模块相同末级路径生成唯一资源名
 * @param t 测试对象
 * @returns null
 */
func TestCRUDResourceSpecsUseNamespacedResourceNames(t *testing.T) {
	controllers := []Definition{
		Admin("base/sys/user").Model(deriveModel()).CRUD(CRUDOptions{API: []string{crud.Page}}).Build(),
		Admin("demo/sys/user").Model(deriveModel()).CRUD(CRUDOptions{API: []string{crud.Page}}).Build(),
	}

	specs, err := CRUDResourceSpecs(controllers)
	if err != nil {
		t.Fatalf("build specs failed: %v", err)
	}
	if specs[0].Name == specs[1].Name {
		t.Fatalf("expected unique resource names, got %s", specs[0].Name)
	}
	if specs[0].Name != "base/sys/user" || specs[1].Name != "demo/sys/user" {
		t.Fatalf("expected namespaced resource names, got %#v", []string{specs[0].Name, specs[1].Name})
	}
}
