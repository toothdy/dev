package sys

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type permissionDBStub struct {
	count      int
	query      string
	args       []any
	allQueries []string
	allArgs    [][]any
}

func (s *permissionDBStub) GetCount(_ context.Context, query string, args ...any) (int, error) {
	s.query = query
	s.args = append([]any(nil), args...)
	return s.count, nil
}

func (s *permissionDBStub) GetAll(_ context.Context, query string, args ...any) (gdb.Result, error) {
	s.allQueries = append(s.allQueries, query)
	s.allArgs = append(s.allArgs, append([]any(nil), args...))
	return nil, nil
}

func TestSplitPermsTrimsAndSkipsEmptyValues(t *testing.T) {
	perms := SplitPerms("base:sys:user:page, base:sys:user:list,,base:sys:user:info")
	expected := []string{"base:sys:user:page", "base:sys:user:list", "base:sys:user:info"}
	if !reflect.DeepEqual(perms, expected) {
		t.Fatalf("expected %#v, got %#v", expected, perms)
	}
}

func TestUniqueStringsKeepsFirstOccurrenceOrder(t *testing.T) {
	items := UniqueStrings([]string{"a", "b", "a", "", "c", "b"})
	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(items, expected) {
		t.Fatalf("expected %#v, got %#v", expected, items)
	}
}

func TestMenuRowShouldEnterMenusOnlyForVisibleNonButtons(t *testing.T) {
	cases := []struct {
		name string
		row  map[string]interface{}
		want bool
	}{
		{
			name: "visible page",
			row:  map[string]interface{}{"type": int64(1), "isShow": int64(1)},
			want: true,
		},
		{
			name: "visible button",
			row:  map[string]interface{}{"type": int64(2), "isShow": int64(1)},
			want: false,
		},
		{
			name: "hidden page",
			row:  map[string]interface{}{"type": int64(1), "isShow": int64(0)},
			want: false,
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if got := MenuRowShouldEnterMenus(item.row); got != item.want {
				t.Fatalf("expected %t, got %t", item.want, got)
			}
		})
	}
}

func TestMenuResponseConvertsFlagsToBoolean(t *testing.T) {
	menu := normalizeMenuResponse(map[string]interface{}{
		"keepAlive": int64(1),
		"isShow":    int64(0),
		"name":      "系统管理",
	})

	if keepAlive, ok := menu["keepAlive"].(bool); !ok || !keepAlive {
		t.Fatalf("expected keepAlive true bool, got %#v", menu["keepAlive"])
	}
	if isShow, ok := menu["isShow"].(bool); !ok || isShow {
		t.Fatalf("expected isShow false bool, got %#v", menu["isShow"])
	}
	if name, ok := menu["name"].(string); !ok || name != "系统管理" {
		t.Fatalf("expected unrelated menu fields unchanged, got %#v", menu["name"])
	}
}

func TestPermissionServiceIsAdminDoesNotTrustUsername(t *testing.T) {
	db := &permissionDBStub{}
	service := newPermissionService(db)

	isAdmin, err := service.IsAdmin(context.Background(), security.UserContext{UserId: 42, Username: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if isAdmin {
		t.Fatal("username alone must not grant platform administrator")
	}
	if len(db.args) != 2 || db.args[0] != int64(42) || db.args[1] != "admin" {
		t.Fatalf("expected authoritative user ID query, got %#v", db.args)
	}
	for _, condition := range []string{"u.status = 1", "u.tenantId IS NULL", "r.label = ?", "r.tenantId IS NULL"} {
		if !strings.Contains(db.query, condition) {
			t.Fatalf("admin query missing %q: %s", condition, db.query)
		}
	}
}

func TestPermissionServiceIsAdminUsesAuthoritativeRelation(t *testing.T) {
	db := &permissionDBStub{count: 1}
	service := newPermissionService(db)
	isAdmin, err := service.IsAdmin(context.Background(), security.UserContext{UserId: 9, Username: "ordinary"})
	if err != nil || !isAdmin {
		t.Fatalf("expected authoritative relation to grant admin, isAdmin=%v err=%v", isAdmin, err)
	}
}

func TestPermissionServiceHasPermissionAllowsEmptyPermission(t *testing.T) {
	service := NewPermissionService(nil)

	hasPermission, err := service.HasPermission(context.Background(), security.UserContext{}, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !hasPermission {
		t.Fatal("expected empty permission to be allowed")
	}
}

func TestPermissionServiceHasPermissionAllowsAdministrator(t *testing.T) {
	service := newPermissionService(&permissionDBStub{count: 1})
	ctx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})

	hasPermission, err := service.HasPermission(
		ctx,
		security.UserContext{UserId: 1, Username: "admin"},
		"base:sys:menu:export",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !hasPermission {
		t.Fatal("expected administrator to bypass menu permission lookup")
	}
}

func TestPermissionMenuQueriesScopeUsersRolesAndMenus(t *testing.T) {
	db := &permissionDBStub{}
	service := newPermissionService(db)
	ctx := baseTenantServiceContext(t, 43)
	if _, err := service.userMenuRows(ctx, 7); err != nil {
		t.Fatalf("build permission menu queries failed: %v", err)
	}
	if len(db.allQueries) != 2 || len(db.allArgs) != 2 {
		t.Fatalf("unexpected permission queries: %#v", db.allQueries)
	}
	for _, condition := range []string{"`m`.`tenantId` = ?", "`r`.`tenantId` = ?", "`u`.`tenantId` = ?"} {
		if !strings.Contains(db.allQueries[0], condition) {
			t.Fatalf("permission relation query missing %q: %s", condition, db.allQueries[0])
		}
	}
	if !reflect.DeepEqual(db.allArgs[0], []any{int64(7), int64(43), int64(43), int64(43)}) {
		t.Fatalf("unexpected permission relation args: %#v", db.allArgs[0])
	}
	if !strings.Contains(db.allQueries[1], "`m`.`tenantId` = ?") || !reflect.DeepEqual(db.allArgs[1], []any{int64(43)}) {
		t.Fatalf("permission parent query is not scoped: %s %#v", db.allQueries[1], db.allArgs[1])
	}
}
