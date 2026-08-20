package sys

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestLogKeepRequestRejectsInvalidValue(t *testing.T) {
	err := (dto.LogKeepRequest{}).Validate()
	if err == nil || err.Error() != "日志保留天数必须大于0" {
		t.Fatalf("expected keep validation error, got %v", err)
	}
}

func TestConfUpdateValueWritesExplicitTimestampWithoutUpdatingKey(t *testing.T) {
	db, err := gdb.New(gdb.ConfigNode{
		Type: "mysql", Host: "127.0.0.1", Port: "3306", User: "test", Pass: "test", Name: "test", DryRun: true, UpdatedAt: "updateTime",
	})
	if err != nil {
		t.Fatalf("create config service test database failed: %v", err)
	}
	fields := map[string]*gdb.TableField{
		"id":          {Index: 0, Name: "id", Type: "bigint unsigned", Key: "PRI", Extra: "auto_increment"},
		"cKey":       {Index: 1, Name: "cKey", Type: "varchar(255)"},
		"cValue":     {Index: 2, Name: "cValue", Type: "varchar(255)"},
		"updateTime": {Index: 3, Name: "updateTime", Type: "varchar(32)"},
	}
	if err = db.GetCore().SetTableFields(context.Background(), "base_sys_conf", fields); err != nil {
		t.Fatalf("seed config table fields failed: %v", err)
	}

	service := NewConfService(db, baseEntity.BaseSysConf())
	sqlText, err := gdb.ToSQL(context.Background(), func(ctx context.Context) error {
		return service.UpdateValue(ctx, "logKeep", 2)
	})
	if err != nil {
		t.Fatalf("build config update SQL failed: %v", err)
	}
	setSQL := strings.SplitN(sqlText, " WHERE ", 2)[0]
	if !strings.Contains(setSQL, "`cValue`=2") || !strings.Contains(setSQL, "`updateTime`=") || strings.Contains(strings.ToLower(setSQL), "`updateTime`=null") {
		t.Fatalf("config update SQL must write value and non-null timestamp: %s", sqlText)
	}
	if strings.Contains(setSQL, "`cKey`=") {
		t.Fatalf("config update SQL must not redundantly update key: %s", sqlText)
	}
	if !strings.Contains(sqlText, "WHERE `cKey`='logKeep'") {
		t.Fatalf("config update SQL lost key predicate: %s", sqlText)
	}
}
