package seed_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

// 验证基础模块初始化管理员使用 bcrypt。
func TestBaseAdminSeedUsesBcrypt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repositoryRoot(t), "modules/base/db.json"))
	if err != nil {
		t.Fatalf("read base seed failed: %v", err)
	}
	var fixture struct {
		Users []struct {
			Username  string `json:"username"`
			Password  string `json:"password"`
			PasswordV int64  `json:"passwordV"`
		} `json:"base_sys_user"`
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("decode base seed failed: %v", err)
	}
	for _, user := range fixture.Users {
		if user.Username != "admin" {
			continue
		}
		if !security.VerifyPassword("123456", user.Password) {
			t.Fatal("expected admin seed password to use bcrypt for 123456")
		}
		if user.PasswordV != 1 {
			t.Fatalf("expected admin password version 1, got %d", user.PasswordV)
		}
		return
	}
	t.Fatal("expected admin seed user")
}

func TestInsertSQLUsesParameters(t *testing.T) {
	sqlText, args := seed.InsertSQL(seed.MappedRecord{
		TableName: "base_sys_user",
		Values: map[string]interface{}{
			"id":       float64(1),
			"username": "admin",
		},
	})

	if !strings.HasPrefix(sqlText, "INSERT INTO `base_sys_user`") {
		t.Fatalf("unexpected insert sql: %s", sqlText)
	}
	if !strings.Contains(sqlText, "VALUES (?, ?)") {
		t.Fatalf("expected parameter placeholders, got %s", sqlText)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
}

func TestInsertSQLOrdersColumns(t *testing.T) {
	sqlText, args := seed.InsertSQL(seed.MappedRecord{
		TableName: "base_sys_user",
		Values: map[string]interface{}{
			"username":   "admin",
			"passwordV": float64(7),
			"id":         float64(1),
		},
	})

	expected := "INSERT INTO `base_sys_user` (`id`, `passwordV`, `username`) VALUES (?, ?, ?)"
	if sqlText != expected {
		t.Fatalf("expected sql %s, got %s", expected, sqlText)
	}
	if args[0] != float64(1) || args[1] != float64(7) || args[2] != "admin" {
		t.Fatalf("unexpected args: %#v", args)
	}
}
