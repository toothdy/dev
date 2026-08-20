package sys

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeAuthorizationUserIDs(t *testing.T) {
	got := normalizeAuthorizationUserIDs([]int64{8, 3, 8, 0, -1, 5})
	want := []int64{3, 5, 8}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected %#v, got %#v", want, got)
	}
}

func TestSameTenantScopeTreatsNullAndZeroAsPlatform(t *testing.T) {
	if !sameTenantScope(nil, int64(0)) || !sameTenantScope(int64(0), nil) {
		t.Fatal("NULL and zero should both represent platform scope")
	}
	if sameTenantScope(int64(1), int64(2)) || sameTenantScope(nil, int64(1)) {
		t.Fatal("different tenant scopes must not match")
	}
}

/**
 * 验证数据库租户值使用严格身份转换
 * @param t 测试上下文
 * @returns null
 */
func TestTenantIdentityFromDatabase(t *testing.T) {
	for _, value := range []interface{}{nil, int64(0), []byte("0")} {
		identity, err := tenantIdentityFromDatabase(value)
		if err != nil || !identity.IsPlatform() {
			t.Fatalf("expected platform identity for %#v, got %#v, %v", value, identity, err)
		}
	}
	identity, err := tenantIdentityFromDatabase([]byte("7"))
	if err != nil {
		t.Fatalf("convert tenant identity failed: %v", err)
	}
	if tenantID, ok := identity.TenantID(); !ok || tenantID != 7 {
		t.Fatalf("expected tenant 7, got %d, %v", tenantID, ok)
	}
	for _, value := range []interface{}{int64(-1), "invalid", 1.5} {
		if _, err = tenantIdentityFromDatabase(value); err == nil {
			t.Fatalf("expected invalid database tenant rejected: %#v", value)
		}
	}
}

func TestAssignableRolePolicy(t *testing.T) {
	tests := []struct {
		name           string
		callerAdmin    bool
		callerID       int64
		callerTenant   any
		targetTenant   any
		roles          []authorizationRole
		wantAssignable bool
	}{
		{
			name: "global admin is never assignable", callerAdmin: true, callerID: 1,
			callerTenant: nil, targetTenant: nil,
			roles: []authorizationRole{{ID: 1, Label: platformAdminRoleLabel, TenantID: nil}},
		},
		{
			name: "platform admin assigns matching platform role", callerAdmin: true, callerID: 1,
			callerTenant: nil, targetTenant: int64(0),
			roles: []authorizationRole{{ID: 2, Label: "operator", TenantID: nil}}, wantAssignable: true,
		},
		{
			name: "platform admin cannot assign mismatched tenant role", callerAdmin: true, callerID: 1,
			callerTenant: nil, targetTenant: int64(2),
			roles: []authorizationRole{{ID: 2, Label: "operator", TenantID: int64(3)}},
		},
		{
			name: "tenant creator assigns own role", callerID: 10,
			callerTenant: int64(2), targetTenant: int64(2),
			roles: []authorizationRole{{ID: 4, Label: "editor", TenantID: int64(2), CreatorID: 10}}, wantAssignable: true,
		},
		{
			name: "tenant caller assigns held role", callerID: 10,
			callerTenant: int64(2), targetTenant: int64(2),
			roles: []authorizationRole{{ID: 4, Label: "editor", TenantID: int64(2), HeldByCaller: true}}, wantAssignable: true,
		},
		{
			name: "tenant caller cannot assign unrelated role", callerID: 10,
			callerTenant: int64(2), targetTenant: int64(2),
			roles: []authorizationRole{{ID: 4, Label: "editor", TenantID: int64(2)}},
		},
		{
			name: "tenant caller cannot manage another tenant", callerID: 10,
			callerTenant: int64(2), targetTenant: int64(3),
			roles: []authorizationRole{{ID: 4, Label: "editor", TenantID: int64(3), CreatorID: 10}},
		},
	}
	for _, item := range tests {
		t.Run(item.name, func(t *testing.T) {
			got := rolesAreAssignable(
				item.callerAdmin,
				item.callerID,
				item.callerTenant,
				item.targetTenant,
				item.roles,
			)
			if got != item.wantAssignable {
				t.Fatalf("expected assignable=%v, got %v", item.wantAssignable, got)
			}
		})
	}
}

func TestProtectedPlatformAdminPolicyRequiresAllConditions(t *testing.T) {
	valid := authorizationUser{ID: 1, Status: 1, TenantID: nil, HasGlobalAdminRole: true}
	if !valid.IsProtectedPlatformAdmin() {
		t.Fatal("expected enabled platform user with global admin role to be protected")
	}
	invalid := []authorizationUser{
		{ID: 1, Status: 0, TenantID: nil, HasGlobalAdminRole: true},
		{ID: 1, Status: 1, TenantID: int64(3), HasGlobalAdminRole: true},
		{ID: 1, Status: 1, TenantID: nil, HasGlobalAdminRole: false},
	}
	for _, user := range invalid {
		if user.IsProtectedPlatformAdmin() {
			t.Fatalf("unexpected protected user: %#v", user)
		}
	}
}

func TestAuthorizationUserLockQuerySortsAndUsesPlaceholders(t *testing.T) {
	query, args := authorizationUserLockQuery([]int64{9, 2, 9, 4})
	if query != "SELECT id FROM base_sys_user WHERE id IN (?, ?, ?) ORDER BY id FOR UPDATE" {
		t.Fatalf("unexpected lock query: %s", query)
	}
	if !reflect.DeepEqual(args, []any{int64(2), int64(4), int64(9)}) {
		t.Fatalf("unexpected lock args: %#v", args)
	}
	if strings.Contains(query, "9") || strings.Contains(query, "2") {
		t.Fatalf("lock query must not interpolate IDs: %s", query)
	}

	emptyQuery, emptyArgs := authorizationUserLockQuery(nil)
	if emptyQuery != "" || len(emptyArgs) != 0 {
		t.Fatalf("empty lock should be a no-op, query=%q args=%#v", emptyQuery, emptyArgs)
	}
}

func TestClaimsFromUserSnapshotRejectsDisabledOrRolelessUser(t *testing.T) {
	user := map[string]interface{}{
		"id": int64(7), "username": "tester", "status": int64(1),
		"passwordV": int64(3), "tenantId": int64(2),
	}
	claims, err := claimsFromUserSnapshot(user, []int64{5})
	if err != nil || claims.UserId != 7 || claims.Username != "tester" || !reflect.DeepEqual(claims.RoleIds, []int64{5}) {
		t.Fatalf("unexpected claims=%#v err=%v", claims, err)
	}
	if _, err = claimsFromUserSnapshot(user, nil); err == nil {
		t.Fatal("roleless user must not receive a token snapshot")
	}
	user["status"] = int64(0)
	if _, err = claimsFromUserSnapshot(user, []int64{5}); err == nil {
		t.Fatal("disabled user must not receive a token snapshot")
	}
}
