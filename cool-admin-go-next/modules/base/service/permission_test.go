package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/recycle"
	baseentity "github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type permissionMenuTestRow struct {
	id       uint64
	parentID *uint64
	orderNum int32
}

func TestPermissionRoleDomainOperations(t *testing.T) {
	service, runtime := newPermissionRoleTestService(t)
	if _, err := runtime.DB().Exec(t.Context(), `
		INSERT INTO base_sys_user (id, username, status) VALUES (1, 'admin', 1), (2, 'backup', 1), (3, 'disabled', 0);
		INSERT INTO base_sys_role (id, name, label) VALUES (10, '管理员', 'admin'), (20, '编辑', 'editor'), (30, '访客', 'guest');
		INSERT INTO base_sys_user_role (userId, roleId) VALUES (1, 20), (1, 10), (2, 10), (3, 10)
	`); err != nil {
		t.Fatal(err)
	}

	roles, err := service.RolesByUsers(t.Context(), []uint64{2, 1, 1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(roles[1].IDs) != "[10 20]" || fmt.Sprint(roles[1].Names) != "[管理员 编辑]" ||
		fmt.Sprint(roles[2].IDs) != "[10]" {
		t.Fatalf("RolesByUsers() = %#v", roles)
	}

	if err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		if lockErr := service.LockUserRoleChanges(ctx, []uint64{1}, []uint64{30, 30, 0}); lockErr != nil {
			return lockErr
		}
		if replaceErr := service.ReplaceRoles(ctx, 1, []uint64{30, 30, 0}); replaceErr != nil {
			return replaceErr
		}
		return service.RevokeUsers(ctx, []uint64{1})
	}); err != nil {
		t.Fatal(err)
	}
	roleIDs, err := service.RoleIDs(t.Context(), 1)
	if err != nil || fmt.Sprint(roleIDs) != "[30]" {
		t.Fatalf("RoleIDs() = %v, %v", roleIDs, err)
	}
	if err = service.EnsureNotLastAdmin(t.Context(), []uint64{2}); err == nil {
		t.Fatal("EnsureNotLastAdmin(last) error = nil")
	}
	if _, err = runtime.DB().Exec(t.Context(), "INSERT INTO base_sys_user (id, username, status) VALUES (4, 'other', 1); INSERT INTO base_sys_user_role (userId, roleId) VALUES (4, 10)"); err != nil {
		t.Fatal(err)
	}
	if err = service.EnsureNotLastAdmin(t.Context(), []uint64{2}); err != nil {
		t.Fatal(err)
	}
}

func TestFlatVisibleMenusReturnsOrderedMenusWithoutNesting(t *testing.T) {
	service, runtime := newPermissionMenuTestService(t)
	seedPermissionMenus(t, runtime,
		permissionMenuTestRow{id: 1, orderNum: 30},
		permissionMenuTestRow{id: 2, parentID: permissionMenuUint64(1), orderNum: 10},
		permissionMenuTestRow{id: 3, parentID: permissionMenuUint64(2), orderNum: 20},
		permissionMenuTestRow{id: 4, parentID: permissionMenuUint64(1), orderNum: 10},
	)
	seedPermissionRoleMenus(t, runtime, [2]uint64{7, 3}, [2]uint64{7, 2}, [2]uint64{8, 1})

	testCases := []struct {
		name    string
		roleIDs []uint64
		isAdmin bool
		wantIDs []uint64
	}{
		{name: "administrator", isAdmin: true, wantIDs: []uint64{2, 4, 3, 1}},
		{name: "assigned role", roleIDs: []uint64{7}, wantIDs: []uint64{2, 3}},
		{name: "empty roles", roleIDs: []uint64{}, wantIDs: []uint64{}},
		{name: "unassigned role", roleIDs: []uint64{9}, wantIDs: []uint64{}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			menus, err := service.flatVisibleMenus(t.Context(), testCase.roleIDs, testCase.isAdmin)
			if err != nil {
				t.Fatal(err)
			}
			if len(menus) != len(testCase.wantIDs) {
				t.Fatalf("menu count = %d, want %d", len(menus), len(testCase.wantIDs))
			}
			for index, menu := range menus {
				if menu.ID != testCase.wantIDs[index] {
					t.Fatalf("menu[%d].ID = %d, want %d", index, menu.ID, testCase.wantIDs[index])
				}
				if len(menu.ChildMenus) != 0 {
					t.Fatalf("menu[%d].ChildMenus = %#v, want flat item", index, menu.ChildMenus)
				}
			}
		})
	}
}

func newPermissionMenuTestService(t *testing.T) (*PermissionService, *coredb.Runtime) {
	t.Helper()

	menuDescriptor, err := coreentity.Compile[baseentity.Menu, uint64](baseentity.MenuSchema())
	if err != nil {
		t.Fatal(err)
	}
	roleMenuDescriptor, err := coreentity.Compile[baseentity.RoleMenu, uint64](baseentity.RoleMenuSchema())
	if err != nil {
		t.Fatal(err)
	}
	group := "permission_menu_" + strings.ReplaceAll(t.Name(), "/", "_")
	runtime, err := coredb.New(t.Context(), coredb.Config{
		Group: group,
		Nodes: gdb.ConfigGroup{{
			Type: "sqlite",
			Link: fmt.Sprintf("sqlite::@file(%s)", filepath.Join(t.TempDir(), "permission-menu.sqlite")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.DB().Exec(t.Context(), `
		CREATE TABLE base_sys_menu (
			id INTEGER PRIMARY KEY,
			createTime DATETIME,
			updateTime DATETIME,
			parentId INTEGER,
			name TEXT NOT NULL,
			router TEXT,
			perms TEXT,
			type INTEGER NOT NULL DEFAULT 0,
			icon TEXT,
			orderNum INTEGER NOT NULL DEFAULT 0,
			viewPath TEXT,
			keepAlive INTEGER NOT NULL DEFAULT 1,
			isShow INTEGER NOT NULL DEFAULT 1
		);
		CREATE TABLE base_sys_role_menu (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			createTime DATETIME,
			updateTime DATETIME,
			roleId INTEGER NOT NULL,
			menuId INTEGER NOT NULL
		)
	`); err != nil {
		t.Fatal(err)
	}
	recycler, err := recycle.New(runtime, crud.Config{})
	if err != nil {
		t.Fatal(err)
	}
	menu, err := coreservice.NewBase[baseentity.Menu, uint64](menuDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	roleMenu, err := coreservice.NewBase[baseentity.RoleMenu, uint64](roleMenuDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}

	return &PermissionService{
		menu:        menu,
		menuService: &MenuService{roleMenu: roleMenu},
	}, runtime
}

func newPermissionRoleTestService(t *testing.T) (*PermissionService, *coredb.Runtime) {
	t.Helper()

	userDescriptor, err := coreentity.Compile[baseentity.User, uint64](baseentity.UserSchema())
	if err != nil {
		t.Fatal(err)
	}
	roleDescriptor, err := coreentity.Compile[baseentity.Role, uint64](baseentity.RoleSchema())
	if err != nil {
		t.Fatal(err)
	}
	userRoleDescriptor, err := coreentity.Compile[baseentity.UserRole, uint64](baseentity.UserRoleSchema())
	if err != nil {
		t.Fatal(err)
	}
	group := "permission_role_" + strings.ReplaceAll(t.Name(), "/", "_")
	runtime, err := coredb.New(t.Context(), coredb.Config{
		Group: group,
		Nodes: gdb.ConfigGroup{{Type: "sqlite", Link: fmt.Sprintf("sqlite::@file(%s)", filepath.Join(t.TempDir(), "permission-role.sqlite"))}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.DB().Exec(t.Context(), `
		CREATE TABLE base_sys_user (id INTEGER PRIMARY KEY, username TEXT NOT NULL, status INTEGER NOT NULL);
		CREATE TABLE base_sys_role (id INTEGER PRIMARY KEY, name TEXT NOT NULL, label TEXT);
		CREATE TABLE base_sys_user_role (
			id INTEGER PRIMARY KEY AUTOINCREMENT, userId INTEGER NOT NULL, roleId INTEGER NOT NULL,
			UNIQUE(userId, roleId)
		)
	`); err != nil {
		t.Fatal(err)
	}
	recycler, err := recycle.New(runtime, crud.Config{})
	if err != nil {
		t.Fatal(err)
	}
	user, err := coreservice.NewBase[baseentity.User, uint64](userDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	role, err := coreservice.NewBase[baseentity.Role, uint64](roleDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	userRole, err := coreservice.NewBase[baseentity.UserRole, uint64](userRoleDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	boundary, err := auth.NewBoundary(runtime, permissionSessionStore{})
	if err != nil {
		t.Fatal(err)
	}

	return &PermissionService{user: user, role: role, userRole: userRole, boundary: boundary}, runtime
}

type permissionSessionStore struct{}

func (permissionSessionStore) Get(context.Context, string) (auth.SessionSnapshot, bool, error) {
	return auth.SessionSnapshot{}, false, nil
}

func (permissionSessionStore) Save(context.Context, auth.SessionSnapshot) error { return nil }

func (permissionSessionStore) RotateRefresh(context.Context, string, string, auth.SessionSnapshot) error {
	return nil
}

func (permissionSessionStore) Revoke(context.Context, string) error { return nil }

func (permissionSessionStore) RevokeUser(context.Context, auth.Kind, uint64) error { return nil }

func (permissionSessionStore) RevokeUsers(context.Context, auth.Kind, []uint64) error { return nil }

func seedPermissionMenus(t *testing.T, runtime *coredb.Runtime, rows ...permissionMenuTestRow) {
	t.Helper()

	for _, row := range rows {
		if _, err := runtime.DB().Exec(
			t.Context(),
			"INSERT INTO base_sys_menu (id, parentId, name, orderNum) VALUES (?, ?, ?, ?)",
			row.id,
			row.parentID,
			fmt.Sprintf("menu-%d", row.id),
			row.orderNum,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func seedPermissionRoleMenus(t *testing.T, runtime *coredb.Runtime, rows ...[2]uint64) {
	t.Helper()

	for _, row := range rows {
		if _, err := runtime.DB().Exec(
			t.Context(),
			"INSERT INTO base_sys_role_menu (roleId, menuId) VALUES (?, ?)",
			row[0],
			row[1],
		); err != nil {
			t.Fatal(err)
		}
	}
}

func permissionMenuUint64(value uint64) *uint64 {
	return &value
}
