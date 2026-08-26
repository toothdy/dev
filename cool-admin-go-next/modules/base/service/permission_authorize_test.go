package service

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/recycle"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

// 授权判定为只读路径，不触发删除归档
type noopDeleter struct{}

func (noopDeleter) Delete(context.Context, coreentity.RuntimeDescriptor, []any) error { return nil }

var testGroupSequence int

// 建立带 base 权限关系表的内存 sqlite 运行时
func newPermissionTestService(t *testing.T) (*PermissionService, gdb.DB) {
	t.Helper()

	testGroupSequence++
	group := fmt.Sprintf("permission_authorize_test_%d", testGroupSequence)
	ctx := context.Background()

	runtime, err := coredb.New(ctx, coredb.Config{
		Group: group,
		Nodes: gdb.ConfigGroup{{
			Type: "sqlite",
			Name: filepath.Join(t.TempDir(), "permission.db"),
		}},
	})
	if err != nil {
		t.Skipf("内存 sqlite 不可用，跳过授权集成测试: %v", err)
	}

	database := runtime.DB()
	schema := []string{
		`CREATE TABLE base_sys_user_role (id INTEGER PRIMARY KEY AUTOINCREMENT, userId INTEGER, roleId INTEGER,
			createTime DATETIME, updateTime DATETIME)`,
		`CREATE TABLE base_sys_role (id INTEGER PRIMARY KEY AUTOINCREMENT, userId TEXT, name TEXT, label TEXT,
			remark TEXT, relevance INTEGER, menuIdList TEXT, departmentIdList TEXT, createTime DATETIME, updateTime DATETIME)`,
		`CREATE TABLE base_sys_role_menu (id INTEGER PRIMARY KEY AUTOINCREMENT, roleId INTEGER, menuId INTEGER,
			createTime DATETIME, updateTime DATETIME)`,
		`CREATE TABLE base_sys_menu (id INTEGER PRIMARY KEY AUTOINCREMENT, parentId INTEGER, name TEXT, router TEXT,
			perms TEXT, type INTEGER, icon TEXT, orderNum INTEGER, viewPath TEXT, keepAlive INTEGER, isShow INTEGER,
			seedKey TEXT, createTime DATETIME, updateTime DATETIME)`,
	}
	for _, statement := range schema {
		if _, err = database.Exec(ctx, statement); err != nil {
			t.Fatalf("建表失败: %v", err)
		}
	}

	userRole := mustBase[entity.UserRole](t, entity.UserRoleSchema(), runtime)
	role := mustBase[entity.Role](t, entity.RoleSchema(), runtime)
	roleMenu := mustBase[entity.RoleMenu](t, entity.RoleMenuSchema(), runtime)
	menu := mustBase[entity.Menu](t, entity.MenuSchema(), runtime)

	// Authorize 只读取上述四张关系表，不触达 MenuService
	return &PermissionService{userRole: userRole, role: role, roleMenu: roleMenu, menu: menu}, database
}

func mustBase[E any](t *testing.T, schema coreentity.Schema, runtime *coredb.Runtime) *coreservice.Base[E, uint64] {
	t.Helper()
	descriptor, err := coreentity.Compile[E, uint64](schema)
	if err != nil {
		t.Fatalf("编译 Descriptor 失败: %v", err)
	}
	base, err := coreservice.NewBase[E, uint64](descriptor, runtime, recycle.Deleter(noopDeleter{}))
	if err != nil {
		t.Fatalf("构造 Base 失败: %v", err)
	}

	return base
}

func seedRow(t *testing.T, database gdb.DB, statement string, args ...any) {
	t.Helper()
	if _, err := database.Exec(context.Background(), statement, args...); err != nil {
		t.Fatalf("写入测试数据失败: %v", err)
	}
}

// 超管角色旁路、普通角色命中菜单权限、未命中返回拒绝
func TestPermissionServiceAuthorize(t *testing.T) {
	const (
		superAdminUserID = uint64(1)
		operatorUserID   = uint64(2)
		outsiderUserID   = uint64(3)
	)

	service, database := newPermissionTestService(t)

	seedRow(t, database, `INSERT INTO base_sys_role (id, name, label) VALUES (1, '超级管理员', 'admin')`)
	seedRow(t, database, `INSERT INTO base_sys_role (id, name, label) VALUES (2, '运维', 'ops')`)
	seedRow(t, database, `INSERT INTO base_sys_menu (id, name, perms) VALUES (10, '代码生成', 'base:coding:createCode')`)
	seedRow(t, database, `INSERT INTO base_sys_role_menu (roleId, menuId) VALUES (2, 10)`)
	seedRow(t, database, `INSERT INTO base_sys_user_role (userId, roleId) VALUES (1, 1)`)
	seedRow(t, database, `INSERT INTO base_sys_user_role (userId, roleId) VALUES (2, 2)`)

	cases := []struct {
		name       string
		userID     uint64
		permission string
		want       bool
	}{
		{name: "超管旁路未授权的工具接口", userID: superAdminUserID, permission: "base:sys:menu:import", want: true},
		{name: "超管旁路任意权限", userID: superAdminUserID, permission: "base:sys:user:move", want: true},
		{name: "普通角色命中菜单权限", userID: operatorUserID, permission: "base:coding:createCode", want: true},
		{name: "普通角色未命中被拒", userID: operatorUserID, permission: "base:coding:getModuleTree", want: false},
		{name: "普通角色未获授权的工具接口被拒", userID: operatorUserID, permission: "base:sys:menu:import", want: false},
		{name: "无角色用户被拒", userID: outsiderUserID, permission: "base:coding:createCode", want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			allowed, err := service.Authorize(context.Background(), auth.Authorization{
				Subject:    auth.AdminKind,
				SubjectID:  testCase.userID,
				Permission: testCase.permission,
				Resource:   "POST /admin/x",
			})
			if err != nil {
				t.Fatalf("授权判定失败: %v", err)
			}
			if allowed != testCase.want {
				t.Fatalf("授权结果 = %v，期望 %v", allowed, testCase.want)
			}
		})
	}
}

// App 身份不参与后台菜单权限
func TestPermissionServiceRejectsAppSubject(t *testing.T) {
	service, _ := newPermissionTestService(t)

	allowed, err := service.Authorize(context.Background(), auth.Authorization{
		Subject:    auth.AppKind,
		SubjectID:  1,
		Permission: "base:coding:createCode",
		Resource:   "POST /app/x",
	})
	if err != nil {
		t.Fatalf("授权判定失败: %v", err)
	}
	if allowed {
		t.Fatal("App 身份不应通过后台菜单权限")
	}
}
