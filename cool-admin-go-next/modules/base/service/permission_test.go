package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
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
