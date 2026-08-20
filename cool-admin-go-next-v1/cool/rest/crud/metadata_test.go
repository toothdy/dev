package crud

import (
	"testing"

	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestNewRegistryBuildsResourceMetadata(t *testing.T) {
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:          "user",
			Prefix:        "/admin/base/sys/user",
			Model:         baseModel.BaseSysUser(),
			API:          []string{Add, Page},
			KeywordFields: []string{"username"},
			EqualFields:   []string{"status"},
			HiddenFields:  []string{"password"},
		},
	})
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}

	resource, ok := registry.Resource("user")
	if !ok {
		t.Fatal("expected user resource")
	}
	if !resource.API[Add] || !resource.API[Page] {
		t.Fatalf("expected add and page api, got %#v", resource.API)
	}
	if resource.PrimaryField.JSONName != "id" {
		t.Fatalf("expected id primary field, got %#v", resource.PrimaryField)
	}
	if !resource.HiddenFields["password"] {
		t.Fatal("expected password hidden")
	}
	if !resource.ReadonlyFields["id"] || !resource.ReadonlyFields["createTime"] || !resource.ReadonlyFields["updateTime"] || !resource.ReadonlyFields["tenantId"] {
		t.Fatalf("expected default readonly fields, got %#v", resource.ReadonlyFields)
	}
	if resource.FieldsByJSON["departmentId"].ColumnName != "departmentId" {
		t.Fatalf("expected departmentId mapping, got %#v", resource.FieldsByJSON["departmentId"])
	}
	if !resource.Tenant.IsAware() || resource.Tenant.Column() != "tenantId" {
		t.Fatalf("expected compiled tenant metadata, got %#v", resource.Tenant)
	}
}

func TestNewRegistryBuildsSeparateListAndPageQueryMetadata(t *testing.T) {
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:   "base/sys/user",
			Prefix: "/admin/base/sys/user",
			Model:  baseModel.BaseSysUser(),
			API:   []string{List, Page},
			ListQuery: QuerySpec{
				EqualFields: []string{"departmentId"},
			},
			PageQuery: QuerySpec{
				KeywordFields: []string{"username"},
				EqualFields:   []string{"status"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}

	resource, ok := registry.Resource("base/sys/user")
	if !ok {
		t.Fatal("expected user resource")
	}
	if resource.ListQuery.EqualFields["departmentId"].JSONName != "departmentId" {
		t.Fatalf("expected list departmentId metadata, got %#v", resource.ListQuery.EqualFields)
	}
	if resource.PageQuery.KeywordFields["username"].JSONName != "username" {
		t.Fatalf("expected page username metadata, got %#v", resource.PageQuery.KeywordFields)
	}
	if resource.PageQuery.EqualFields["status"].JSONName != "status" {
		t.Fatalf("expected page status metadata, got %#v", resource.PageQuery.EqualFields)
	}
	if _, ok := resource.ListQuery.EqualFields["status"]; ok {
		t.Fatalf("expected list query not to inherit page status filter, got %#v", resource.ListQuery.EqualFields)
	}
}

func TestNewRegistryRejectsInvalidFields(t *testing.T) {
	_, err := NewRegistry([]ResourceSpec{
		{
			Name:          "user",
			Prefix:        "/admin/base/sys/user",
			Model:         baseModel.BaseSysUser(),
			KeywordFields: []string{"missing"},
		},
	})
	if err == nil {
		t.Fatal("expected invalid field error")
	}
}

func TestNewRegistryAllowsQualifiedJoinedQueryFields(t *testing.T) {
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:   "base/sys/log",
			Prefix: "/admin/base/sys/log",
			Model:  baseModel.BaseSysLog(),
			PageQuery: QuerySpec{
				KeywordFields: []string{"b.name", "a.action", "a.ip"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry with joined query field failed: %v", err)
	}

	resource, ok := registry.Resource("base/sys/log")
	if !ok {
		t.Fatal("expected log resource")
	}
	if resource.PageQuery.KeywordFields["b.name"].ColumnName != "b.name" {
		t.Fatalf("expected qualified joined field metadata, got %#v", resource.PageQuery.KeywordFields)
	}
}

func TestNewRegistryKeepsEmptyListQueryWhenPageQueryIsConfigured(t *testing.T) {
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:   "base/sys/user-query",
			Prefix: "/admin/base/sys/user-query",
			Model:  baseModel.BaseSysUser(),
			EqualFields: []string{
				"status",
			},
			PageQuery: QuerySpec{
				KeywordFields: []string{"username"},
			},
		},
	})
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}

	resource, ok := registry.Resource("base/sys/user-query")
	if !ok {
		t.Fatal("expected user-query resource")
	}
	if len(resource.ListQuery.EqualFields) != 0 || len(resource.ListQuery.KeywordFields) != 0 || len(resource.ListQuery.LikeFields) != 0 {
		t.Fatalf("expected empty list query when page query is configured, got %#v", resource.ListQuery)
	}
	if resource.PageQuery.KeywordFields["username"].JSONName != "username" {
		t.Fatalf("expected page query username metadata, got %#v", resource.PageQuery)
	}
}
