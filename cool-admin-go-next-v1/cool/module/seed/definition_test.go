package seed_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module/seed"
)

func TestNewDefinition(t *testing.T) {
	definition := seed.NewDefinition("modules/base/db.json", "modules/base/menu.json")
	if definition.DBPath != "modules/base/db.json" {
		t.Fatalf("unexpected db path: %s", definition.DBPath)
	}
	if definition.MenuPath != "modules/base/menu.json" {
		t.Fatalf("unexpected menu path: %s", definition.MenuPath)
	}
}

func TestMarkerKey(t *testing.T) {
	if seed.MarkerKey(seed.KindDB, "base") != "init_db_base" {
		t.Fatalf("unexpected db marker key")
	}
	if seed.MarkerKey(seed.KindMenu, "base") != "init_menu_base" {
		t.Fatalf("unexpected menu marker key")
	}
}
