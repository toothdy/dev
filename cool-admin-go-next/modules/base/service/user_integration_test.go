package service

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
	"github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnservice"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	"github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/recycle"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

type userDescriptorResolver struct {
	descriptor gnentity.Descriptor[entity.User, uint64]
}

func (resolver userDescriptorResolver) Resolve(value any) (gnentity.Metadata, bool) {
	if reflect.TypeOf(value) != reflect.TypeFor[entity.User]() {
		return nil, false
	}

	return resolver.descriptor, resolver.descriptor != nil
}

type userSessionStore struct {
	permissionSessionStore
	revoked [][]uint64
}

func (store *userSessionStore) RevokeUsers(_ context.Context, _ auth.Kind, ids []uint64) error {
	store.revoked = append(store.revoked, append([]uint64(nil), ids...))

	return nil
}

type userTestFixture struct {
	service    *UserService
	runtime    *db.Runtime
	descriptor gnentity.Descriptor[entity.User, uint64]
	sessions   *userSessionStore
}

func TestUserMutationDomain(t *testing.T) {
	fixture := newUserTestFixture(t)
	addValue := userMutable(t, fixture.descriptor,
		gnservice.Value("username", "alice"),
		gnservice.Value("password", "secret"),
		gnservice.Value("status", int32(1)),
		gnservice.Value("departmentId", uint64(5)),
		gnservice.Value("roleIdList", []uint64{20, 20, 0}),
	)
	addInput, err := gnservice.NewAddObject[entity.User, uint64](fixture.descriptor, addValue)
	if err != nil {
		t.Fatal(err)
	}
	var userID uint64
	if err = fixture.dispatch(t, crud.ActionAdd, func(ctx context.Context) error {
		result, addErr := fixture.service.Add(ctx, addInput)
		userID = result.One()
		return addErr
	}); err != nil {
		t.Fatal(err)
	}
	var added struct {
		Password  string `orm:"password"`
		PasswordV int32  `orm:"passwordV"`
	}
	if err = fixture.runtime.DB().Model("base_sys_user").Ctx(t.Context()).Where("id", userID).Scan(&added); err != nil {
		t.Fatal(err)
	}
	verified, err := fixture.service.password.Verify("secret", added.Password)
	if err != nil || !verified.Valid || added.PasswordV != 1 {
		t.Fatalf("added password = valid:%t version:%d error:%v", verified.Valid, added.PasswordV, err)
	}
	assertUserRoles(t, fixture.runtime, userID, []uint64{20})

	clearRoles := userMutable(t, fixture.descriptor,
		gnservice.Value("password", ""),
		gnservice.Null("roleIdList"),
	)
	clearInput := userUpdateInput(t, fixture.descriptor, userID, clearRoles)
	if err = fixture.dispatch(t, crud.ActionUpdate, func(ctx context.Context) error {
		return fixture.service.Update(ctx, clearInput)
	}); err != nil {
		t.Fatal(err)
	}
	assertUserRoles(t, fixture.runtime, userID, nil)

	updateValue := userMutable(t, fixture.descriptor,
		gnservice.Value("password", "new-secret"),
		gnservice.Value("status", int32(0)),
		gnservice.Value("roleIdList", []uint64{20}),
	)
	updateInput := userUpdateInput(t, fixture.descriptor, userID, updateValue)
	if err = fixture.dispatch(t, crud.ActionUpdate, func(ctx context.Context) error {
		return fixture.service.Update(ctx, updateInput)
	}); err != nil {
		t.Fatal(err)
	}
	if err = fixture.runtime.DB().Model("base_sys_user").Ctx(t.Context()).Where("id", userID).Scan(&added); err != nil {
		t.Fatal(err)
	}
	verified, err = fixture.service.password.Verify("new-secret", added.Password)
	if err != nil || !verified.Valid || added.PasswordV != 2 {
		t.Fatalf("updated password = valid:%t version:%d error:%v", verified.Valid, added.PasswordV, err)
	}

	deleteInput, err := gnservice.NewDeleteInput[entity.User](fixture.descriptor, []uint64{userID})
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.dispatch(t, crud.ActionDelete, func(ctx context.Context) error {
		return fixture.service.Delete(ctx, deleteInput)
	}); err != nil {
		t.Fatal(err)
	}
	count, err := fixture.runtime.DB().Model("base_sys_user").Ctx(t.Context()).Where("id", userID).Count()
	if err != nil || count != 0 {
		t.Fatalf("deleted user count = %d, error = %v", count, err)
	}
	assertUserRoles(t, fixture.runtime, userID, nil)
	if len(fixture.sessions.revoked) != 3 {
		t.Fatalf("session revocations = %#v", fixture.sessions.revoked)
	}
}

func newUserTestFixture(t *testing.T) userTestFixture {
	t.Helper()
	runtime, err := db.New(t.Context(), db.Config{
		Group: "user_" + strings.ReplaceAll(t.Name(), "/", "_"),
		Nodes: gdb.ConfigGroup{{Type: "sqlite", Link: fmt.Sprintf("sqlite::@file(%s)", filepath.Join(t.TempDir(), "user.sqlite"))}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runtime.DB().Exec(t.Context(), `
		CREATE TABLE base_sys_user (
			id INTEGER PRIMARY KEY AUTOINCREMENT, createTime DATETIME, updateTime DATETIME,
			departmentId INTEGER, userId INTEGER, name TEXT, username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL, passwordV INTEGER NOT NULL DEFAULT 1, nickName TEXT,
			headImg TEXT, phone TEXT, email TEXT, remark TEXT, status INTEGER NOT NULL DEFAULT 1,
			socketId TEXT
		);
		CREATE TABLE base_sys_role (id INTEGER PRIMARY KEY, name TEXT NOT NULL, label TEXT);
		CREATE TABLE base_sys_user_role (
			id INTEGER PRIMARY KEY AUTOINCREMENT, userId INTEGER NOT NULL, roleId INTEGER NOT NULL,
			UNIQUE(userId, roleId)
		);
		CREATE TABLE base_sys_department (id INTEGER PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE base_sys_role_department (
			id INTEGER PRIMARY KEY AUTOINCREMENT, roleId INTEGER NOT NULL, departmentId INTEGER NOT NULL
		);
		INSERT INTO base_sys_role (id, name, label) VALUES (10, '管理员', 'admin'), (20, '编辑', NULL);
		INSERT INTO base_sys_department (id, name) VALUES (5, '研发');
	`); err != nil {
		t.Fatal(err)
	}
	recycler, err := recycle.New(runtime, crud.Config{})
	if err != nil {
		t.Fatal(err)
	}
	user, userDescriptor := userTestBase[entity.User](t, runtime, recycler, entity.UserSchema())
	role, _ := userTestBase[entity.Role](t, runtime, recycler, entity.RoleSchema())
	userRole, _ := userTestBase[entity.UserRole](t, runtime, recycler, entity.UserRoleSchema())
	department, _ := userTestBase[entity.Department](t, runtime, recycler, entity.DepartmentSchema())
	roleDepartment, _ := userTestBase[entity.RoleDepartment](t, runtime, recycler, entity.RoleDepartmentSchema())
	sessions := &userSessionStore{}
	boundary, err := auth.NewBoundary(runtime, sessions)
	if err != nil {
		t.Fatal(err)
	}
	permission := &PermissionService{user: user, userRole: userRole, role: role, boundary: boundary}
	departmentService := &DepartmentService{
		Base: department, user: user, roleDepartment: roleDepartment,
		permission: permission, boundary: boundary,
	}
	password, err := bcrypt.New(bcrypt.Config{Cost: 4})
	if err != nil {
		t.Fatal(err)
	}
	userService, err := NewUser(user, permission, departmentService, password)
	if err != nil {
		t.Fatal(err)
	}

	return userTestFixture{service: userService, runtime: runtime, descriptor: userDescriptor, sessions: sessions}
}

func userTestBase[E any](
	t *testing.T,
	runtime *db.Runtime,
	recycler *recycle.Store,
	schema gnentity.Schema,
) (*gnservice.Base[E, uint64], gnentity.Descriptor[E, uint64]) {
	t.Helper()
	descriptor, err := gnentity.Compile[E, uint64](schema)
	if err != nil {
		t.Fatal(err)
	}
	base, err := gnservice.NewBase[E, uint64](descriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}

	return base, descriptor
}

func (fixture userTestFixture) dispatch(t *testing.T, action crud.Action, callback func(context.Context) error) error {
	t.Helper()
	plan, err := crud.CompilePlan(t.Context(), userDescriptorResolver{fixture.descriptor}, crud.PlanInput{
		Action: action,
		Entity: entity.User{},
		Fields: crud.FieldPolicyInput{
			HiddenFields:   []crud.ColumnRef{crud.NewColumnRef("password")},
			ReadonlyFields: []crud.ColumnRef{crud.NewColumnRef("passwordV"), crud.NewColumnRef("socketId")},
		},
	}, nil)
	if err != nil {
		return err
	}
	dispatcher, err := crud.NewDispatcher(fixture.runtime.Runner())
	if err != nil {
		return err
	}

	return dispatcher.Dispatch(t.Context(), action, crud.ActionModeDelegate, plan, callback)
}

func userMutable(
	t *testing.T,
	descriptor gnentity.Descriptor[entity.User, uint64],
	fields ...gnservice.FieldValue,
) *gnservice.Mutable[entity.User] {
	t.Helper()
	value, err := gnservice.NewMutable[entity.User, uint64](descriptor, fields)
	if err != nil {
		t.Fatal(err)
	}

	return value
}

func userUpdateInput(
	t *testing.T,
	descriptor gnentity.Descriptor[entity.User, uint64],
	id uint64,
	value *gnservice.Mutable[entity.User],
) gnservice.UpdateInput[entity.User, uint64] {
	t.Helper()
	item, err := gnservice.NewUpdateItem(descriptor, id, value)
	if err != nil {
		t.Fatal(err)
	}
	input, err := gnservice.NewUpdateObject(descriptor, item)
	if err != nil {
		t.Fatal(err)
	}

	return input
}

func assertUserRoles(t *testing.T, runtime *db.Runtime, userID uint64, want []uint64) {
	t.Helper()
	var rows []roleIDRow
	if err := runtime.DB().Model("base_sys_user_role").Ctx(t.Context()).Fields("roleId").Where("userId", userID).OrderAsc("roleId").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	got := make([]uint64, len(rows))
	for index, row := range rows {
		got[index] = row.RoleID
	}
	if !slices.Equal(got, want) {
		t.Fatalf("user roles = %#v, want %#v", got, want)
	}
}
