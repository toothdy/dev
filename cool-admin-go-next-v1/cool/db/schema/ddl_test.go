package schema_test

import (
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func TestColumnSQL(t *testing.T) {
	field := entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名")
	sql := schema.ColumnSQL(field)

	checks := []string{"`username` varchar(100)", "NOT NULL", "COMMENT '用户名'"}
	for _, check := range checks {
		if !strings.Contains(sql, check) {
			t.Fatalf("expected column sql to contain %q, got %s", check, sql)
		}
	}
}

func TestColumnSQLUsesSafeDatetimeDefault(t *testing.T) {
	field := entity.NewField("createTime", "createTime", "datetime").NotNull().Comment("创建时间")
	sql := schema.ColumnSQL(field)

	if !strings.Contains(sql, "DEFAULT CURRENT_TIMESTAMP") {
		t.Fatalf("expected datetime column to use safe default, got %s", sql)
	}
}

func TestCreateTableSQL(t *testing.T) {
	definition := entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Comment("系统用户").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		}).
		WithIndexes(entity.NewUniqueIndex("uk_base_sys_user_username", "username"))

	sql := schema.CreateTableSQL(definition)
	checks := []string{
		"CREATE TABLE IF NOT EXISTS `base_sys_user`",
		"`id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID'",
		"PRIMARY KEY (`id`)",
		"UNIQUE KEY `uk_base_sys_user_username` (`username`)",
		"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户'",
	}
	for _, check := range checks {
		if !strings.Contains(sql, check) {
			t.Fatalf("expected create sql to contain %q, got %s", check, sql)
		}
	}
}

func TestAddColumnSQL(t *testing.T) {
	field := entity.NewField("nickName", "nickName", "varchar").Size(100).Nullable().Comment("昵称")
	sql := schema.AddColumnSQL("base_sys_user", field)
	if !strings.Contains(sql, "ALTER TABLE `base_sys_user` ADD COLUMN `nickName` varchar(100) NULL COMMENT '昵称'") {
		t.Fatalf("unexpected add column sql: %s", sql)
	}
}

func TestCreateIndexSQL(t *testing.T) {
	index := entity.NewUniqueIndex("uk_base_sys_user_username", "username")
	sql := schema.CreateIndexSQL("base_sys_user", index)
	if sql != "CREATE UNIQUE INDEX `uk_base_sys_user_username` ON `base_sys_user` (`username`)" {
		t.Fatalf("unexpected index sql: %s", sql)
	}
}
