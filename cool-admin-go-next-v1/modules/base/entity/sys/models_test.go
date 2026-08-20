package sys_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestBaseModelFactories(t *testing.T) {
	models := baseDefinitions()
	if len(models) != 10 {
		t.Fatalf("expected 10 base models, got %d", len(models))
	}

	expectedTables := map[string]bool{
		"base_sys_user":            false,
		"base_sys_role":            false,
		"base_sys_menu":            false,
		"base_sys_department":      false,
		"base_sys_param":           false,
		"base_sys_log":             false,
		"base_sys_conf":            false,
		"base_sys_user_role":       false,
		"base_sys_role_menu":       false,
		"base_sys_role_department": false,
	}
	for _, definition := range models {
		if _, ok := expectedTables[definition.TableName]; ok {
			expectedTables[definition.TableName] = true
		}
	}
	for tableName, found := range expectedTables {
		if !found {
			t.Fatalf("missing table definition: %s", tableName)
		}
	}
}

func TestBaseModelFactoriesUseStableRecycleResources(t *testing.T) {
	expected := map[string]string{
		"base_sys_user": "base.user", "base_sys_role": "base.role", "base_sys_menu": "base.menu",
		"base_sys_department": "base.department", "base_sys_param": "base.param", "base_sys_log": "base.log",
		"base_sys_conf": "base.conf", "base_sys_user_role": "base.userRole",
		"base_sys_role_menu": "base.roleMenu", "base_sys_role_department": "base.roleDepartment",
	}
	for _, definition := range baseDefinitions() {
		if definition.ResourceKey() != expected[definition.TableName] {
			t.Fatalf("unexpected resource for %s: %s", definition.TableName, definition.ResourceKey())
		}
	}
}

func baseDefinitions() []entity.Definition {
	return []entity.Definition{
		baseModel.BaseSysConf(),
		baseModel.BaseSysDepartment(),
		baseModel.BaseSysLog(),
		baseModel.BaseSysMenu(),
		baseModel.BaseSysParam(),
		baseModel.BaseSysRole(),
		baseModel.BaseSysRoleDepartment(),
		baseModel.BaseSysRoleMenu(),
		baseModel.BaseSysUser(),
		baseModel.BaseSysUserRole(),
	}
}

func TestBaseSysUserFields(t *testing.T) {
	definition := baseModel.BaseSysUser()
	for _, columnName := range []string{"id", "departmentId", "userId", "name", "username", "password", "passwordV", "nickName", "headImg", "phone", "email", "remark", "status", "socketId", "createTime", "updateTime", "tenantId"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing base_sys_user field: %s", columnName)
		}
	}
}

func TestBaseSysMenuFields(t *testing.T) {
	definition := baseModel.BaseSysMenu()
	for _, columnName := range []string{"id", "parentId", "name", "router", "perms", "type", "icon", "orderNum", "viewPath", "keepAlive", "isShow", "createTime", "updateTime", "tenantId"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing base_sys_menu field: %s", columnName)
		}
	}
	keepAlive, _ := definition.FieldByColumn("keepAlive")
	if keepAlive.DataType != "boolean" || !keepAlive.HasDefault || keepAlive.DefaultValue != "true" || keepAlive.IsNullable {
		t.Fatalf("unexpected keep_alive metadata: %#v", keepAlive)
	}
}

func TestBaseSysRoleFieldsMatchNodeMetadata(t *testing.T) {
	definition := baseModel.BaseSysRole()
	userID, ok := definition.FieldByColumn("userId")
	if !ok || userID.DataType != "varchar" || userID.IsNullable {
		t.Fatalf("unexpected role userId metadata: %#v", userID)
	}
	label, ok := definition.FieldByColumn("label")
	if !ok || !label.IsNullable || label.Length != 50 {
		t.Fatalf("unexpected role label metadata: %#v", label)
	}
	relevance, ok := definition.FieldByColumn("relevance")
	if !ok || relevance.DataType != "boolean" || !relevance.HasDefault || relevance.DefaultValue != "false" {
		t.Fatalf("unexpected role relevance metadata: %#v", relevance)
	}
	for _, columnName := range []string{"menuIdList", "departmentIdList"} {
		field, exists := definition.FieldByColumn(columnName)
		if !exists || field.DataType != "json" || field.IsNullable {
			t.Fatalf("unexpected role JSON metadata for %s: %#v", columnName, field)
		}
	}
}

func TestBaseTimeFieldsMatchNodeMetadata(t *testing.T) {
	definition := baseModel.BaseSysUser()
	for _, columnName := range []string{"createTime", "updateTime"} {
		field, ok := definition.FieldByColumn(columnName)
		if !ok || field.DataType != "varchar" || field.IsNullable {
			t.Fatalf("unexpected base time metadata for %s: %#v", columnName, field)
		}
	}
}

func TestBaseSysLogHasRetentionIndex(t *testing.T) {
	definition := baseModel.BaseSysLog()
	for _, index := range definition.Indexes {
		if index.Name == "idx_base_sys_log_create_time" && len(index.Columns) == 1 && index.Columns[0] == "createTime" {
			return
		}
	}
	t.Fatal("base_sys_log missing create_time retention index")
}
