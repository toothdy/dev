package sys

import (
	"context"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestNormalizeRoleResponseUsesNodeFieldTypes(t *testing.T) {
	item := normalizeRoleResponse(map[string]interface{}{
		"relevance":        int64(1),
		"menuIdList":       `[1,2]`,
		"departmentIdList": []byte(`[3]`),
	})
	if item["relevance"] != true {
		t.Fatalf("expected boolean relevance, got %#v", item["relevance"])
	}
	if !reflect.DeepEqual(item["menuIdList"], []int64{1, 2}) || !reflect.DeepEqual(item["departmentIdList"], []int64{3}) {
		t.Fatalf("expected ID arrays, got %#v", item)
	}
}

func TestRoleUpdateDataContainsOnlyPresentedFields(t *testing.T) {
	values, err := roleUpdateData(map[string]interface{}{
		"id":         2,
		"label":      nil,
		"relevance":  false,
		"menuIdList": []interface{}{},
	}, []int64{}, nil)
	if err != nil {
		t.Fatalf("build role update failed: %v", err)
	}
	expected := map[string]interface{}{
		"label":        nil,
		"relevance":    false,
		"menuIdList": "[]",
	}
	if !reflect.DeepEqual(values, expected) {
		t.Fatalf("unexpected partial role update: %#v", values)
	}
}

func TestProtectedAuthorizationRoleUsesLabelAndScopeNotID(t *testing.T) {
	if !isProtectedAuthorizationRole(platformAdminRoleLabel, nil) ||
		!isProtectedAuthorizationRole(platformAdminRoleLabel, int64(0)) {
		t.Fatal("global admin role should be protected")
	}
	if isProtectedAuthorizationRole(platformAdminRoleLabel, int64(3)) ||
		isProtectedAuthorizationRole("operator", nil) {
		t.Fatal("tenant admin label and ordinary global role should not be protected")
	}
}

func TestRoleServiceUsesInjectedSessionStore(t *testing.T) {
	store := security.NewMemorySessionStore()
	service := newTestRoleServiceWithSessions(nil, baseModel.BaseSysRole(), store)
	if service.Sessions != store {
		t.Fatal("role service must share the application session store")
	}
}

func TestRoleRelationPathsRejectMissingScope(t *testing.T) {
	service := newTestRoleService(nil, baseModel.BaseSysRole())
	if _, err := service.Info(context.Background(), crud.InfoRequest{ID: 1}); err == nil {
		t.Fatal("role relation info must reject missing scope")
	}
}
