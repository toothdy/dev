package seed_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestImporterSkipsWithoutIntegrationFlag(t *testing.T) {
	if os.Getenv("COOL_SEED_INTEGRATION") == "1" {
		t.Skip("integration flag enabled")
	}
	if os.Getenv("COOL_SEED_INTEGRATION") != "" {
		t.Fatalf("unexpected integration flag value")
	}
}

func TestImporterImportsBaseSeedsAndSkipsSecondRun(t *testing.T) {
	if os.Getenv("COOL_SEED_INTEGRATION") != "1" {
		t.Skip("set COOL_SEED_INTEGRATION=1 to run real MySQL seed test")
	}

	ctx := context.Background()
	db := g.DB()
	definitions := moduleSpecs("base").Models
	if _, err := schema.NewSyncer(db).Sync(ctx, definitions); err != nil {
		t.Fatalf("sync schema failed: %v", err)
	}
	cleanupBaseSeedData(t, ctx)

	importer := seed.NewImporter(db, definitions)
	repoRoot := repositoryRoot(t)
	dbResult, err := importer.ImportDB(ctx, "base", filepath.Join(repoRoot, "modules/base/db.json"))
	if err != nil {
		t.Fatalf("import db seed failed: %v", err)
	}
	if dbResult.Skipped {
		t.Fatal("expected first db seed import not to be skipped")
	}
	if dbResult.InsertedRecords == 0 {
		t.Fatal("expected db seed records to be inserted")
	}

	menuResult, err := importer.ImportMenu(ctx, "base", filepath.Join(repoRoot, "modules/base/menu.json"))
	if err != nil {
		t.Fatalf("import menu seed failed: %v", err)
	}
	if menuResult.Skipped {
		t.Fatal("expected first menu seed import not to be skipped")
	}
	if menuResult.InsertedRecords == 0 {
		t.Fatal("expected menu seed records to be inserted")
	}

	assertBaseSeedData(t, ctx)
	assertRecycleMenuSeedData(t, ctx)

	secondDBResult, err := importer.ImportDB(ctx, "base", filepath.Join(repoRoot, "modules/base/db.json"))
	if err != nil {
		t.Fatalf("second db seed import failed: %v", err)
	}
	if !secondDBResult.Skipped {
		t.Fatal("expected second db seed import to be skipped")
	}

	secondMenuResult, err := importer.ImportMenu(ctx, "base", filepath.Join(repoRoot, "modules/base/menu.json"))
	if err != nil {
		t.Fatalf("second menu seed import failed: %v", err)
	}
	if !secondMenuResult.Skipped {
		t.Fatal("expected second menu seed import to be skipped")
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory failed: %v", err)
	}
	for {
		if _, err = os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", current)
		}
		current = parent
	}
}

func cleanupBaseSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	db := g.DB()
	statements := []string{
		"DELETE FROM `base_sys_role_menu`",
		"DELETE FROM `base_sys_role_department`",
		"DELETE FROM `base_sys_user_role`",
		"DELETE FROM `base_sys_menu`",
		"DELETE FROM `base_sys_user`",
		"DELETE FROM `base_sys_role`",
		"DELETE FROM `base_sys_department`",
		"DELETE FROM `base_sys_param`",
		"DELETE FROM `base_sys_conf` WHERE `cKey` IN ('logKeep', 'recycleKeep', 'init_db_base', 'init_menu_base', 'init_menu_recycle')",
	}
	for _, statement := range statements {
		if _, err := db.Exec(ctx, statement); err != nil {
			t.Fatalf("cleanup failed: %s: %v", statement, err)
		}
	}
}

/**
 * 验证回收站菜单 seed 数据
 * @param t 测试实例
 * @param ctx 上下文
 * @returns null
 */
func assertRecycleMenuSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	db := g.DB()

	markerCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_conf` WHERE `cKey` = ?", "init_menu_recycle")
	if err != nil {
		t.Fatalf("query recycle menu marker failed: %v", err)
	}
	if markerCount != 0 {
		t.Fatalf("expected no recycle menu marker, got %d", markerCount)
	}

	recycleMenuCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_menu` child INNER JOIN `base_sys_menu` parent ON parent.id = child.parent_id WHERE child.router = ? AND parent.name = ?", "/recycle/data", "数据管理")
	if err != nil {
		t.Fatalf("query recycle menu placement failed: %v", err)
	}
	if recycleMenuCount != 1 {
		t.Fatalf("expected recycle menu under data management, got %d", recycleMenuCount)
	}

	roleMenuCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_role_menu` rm INNER JOIN `base_sys_menu` m ON m.id = rm.menu_id WHERE rm.role_id = 1 AND m.perms LIKE ?", "%recycle:data:%")
	if err != nil {
		t.Fatalf("query recycle role menu count failed: %v", err)
	}
	if roleMenuCount != 3 {
		t.Fatalf("expected 3 recycle role menu records, got %d", roleMenuCount)
	}

	departmentCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_department`")
	if err != nil {
		t.Fatalf("query department count failed: %v", err)
	}
	roleDepartmentCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_role_department` WHERE `roleId` = 1")
	if err != nil {
		t.Fatalf("query admin role department count failed: %v", err)
	}
	if roleDepartmentCount != departmentCount {
		t.Fatalf("expected %d admin role department records, got %d", departmentCount, roleDepartmentCount)
	}
}

func moduleSpecs(key string) module.Spec {
	for _, spec := range modules.Specs() {
		if spec.Key == key {
			return spec
		}
	}
	panic("module spec not found: " + key)
}

func assertBaseSeedData(t *testing.T, ctx context.Context) {
	t.Helper()
	db := g.DB()

	user, err := db.GetOne(ctx, "SELECT `password`, `passwordV` FROM `base_sys_user` WHERE `username` = ? AND `status` = 1", "admin")
	if err != nil {
		t.Fatalf("query admin user failed: %v", err)
	}
	if user.IsEmpty() {
		t.Fatal("expected admin user")
	}
	if !security.VerifyPassword("123456", user["password"].String()) {
		t.Fatal("expected admin seed password to use bcrypt for 123456")
	}
	if user["passwordV"].Int64() != 1 {
		t.Fatalf("expected admin password version 1, got %d", user["passwordV"].Int64())
	}

	roleCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_role` WHERE `label` = ?", "admin")
	if err != nil {
		t.Fatalf("query admin role failed: %v", err)
	}
	if roleCount != 1 {
		t.Fatalf("expected admin role, got %d", roleCount)
	}

	markerCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_conf` WHERE `cKey` IN (?, ?)", "init_db_base", "init_menu_base")
	if err != nil {
		t.Fatalf("query init markers failed: %v", err)
	}
	if markerCount != 2 {
		t.Fatalf("expected 2 init markers, got %d", markerCount)
	}

	menuCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_menu`")
	if err != nil {
		t.Fatalf("query menu count failed: %v", err)
	}
	if menuCount == 0 {
		t.Fatal("expected menu records")
	}

	roleMenuCount, err := db.GetCount(ctx, "SELECT COUNT(*) FROM `base_sys_role_menu` WHERE `roleId` = 1")
	if err != nil {
		t.Fatalf("query role menu count failed: %v", err)
	}
	if roleMenuCount == 0 {
		t.Fatal("expected admin role menu records")
	}
}
