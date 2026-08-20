package base_test

import (
	"context"
	"sort"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestGeneratedModuleKeepsBaseContract(t *testing.T) {
	spec := findBaseSpec(t)
	if spec.Key != "base" || spec.Name != "权限管理" || spec.Description != "基础的权限管理功能，包括登录，权限校验" || spec.Order != 10 {
		t.Fatalf("unexpected generated Base metadata: %#v", spec)
	}
	if spec.DB != "modules/base/db.json" || spec.Menu != "modules/base/menu.json" {
		t.Fatalf("unexpected generated Base seed paths: %q/%q", spec.DB, spec.Menu)
	}
	if len(spec.Models) != 10 {
		t.Fatalf("unexpected generated Base model count: %d", len(spec.Models))
	}
	if err := spec.Configure(context.Background()); err != nil {
		t.Fatalf("configure generated Base module failed: %v", err)
	}

	db, err := gdb.New(gdb.ConfigNode{Type: "mysql", DryRun: true})
		if err != nil {
			t.Fatalf("create dry-run db failed: %v", err)
		}
		controllers, err := spec.Controllers(module.Deps{
			Context:      context.Background(),
			DB:           db,
			SessionStore: security.NewMemorySessionStore(),
			AuthOptions:  module.AuthOptions{},
		})
	if err != nil {
		t.Fatalf("build generated Base controllers failed: %v", err)
	}
	if len(controllers) != 9 {
		t.Fatalf("unexpected generated Base controller count: %d", len(controllers))
	}
	prefixes := make([]string, 0, len(controllers))
	for _, definition := range controllers {
		prefixes = append(prefixes, definition.Prefix)
	}
	sort.Strings(prefixes)
	wantPrefixes := []string{
		"/admin/base/comm",
		"/admin/base/open",
		"/admin/base/sys/department",
		"/admin/base/sys/log",
		"/admin/base/sys/menu",
		"/admin/base/sys/param",
		"/admin/base/sys/role",
		"/admin/base/sys/user",
		"/app/base/comm",
	}
	if len(prefixes) != len(wantPrefixes) {
		t.Fatalf("unexpected generated Base controller prefixes: %v", prefixes)
	}
	for index := range prefixes {
		if prefixes[index] != wantPrefixes[index] {
			t.Fatalf("unexpected generated Base controller prefixes: %v", prefixes)
		}
	}
	if spec.Runtime == nil || spec.GlobalMiddlewares == nil || spec.Middlewares != nil || spec.RecycleProvider != nil {
		t.Fatal("generated Base factories do not match the module contract")
	}
}
