package base_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module"
)

type menuSeedItem struct {
	Name       string         `json:"name"`
	Router     string         `json:"router"`
	ChildMenus []menuSeedItem `json:"childMenus"`
}

func findBaseSpec(t *testing.T) module.Spec {
	t.Helper()
	return moduleSpec("base")
}

func TestBaseModuleRegistered(t *testing.T) {
	spec := findBaseSpec(t)
	if spec.Name != "权限管理" {
		t.Fatalf("expected 权限管理, got %s", spec.Name)
	}
	if spec.Order != 10 {
		t.Fatalf("expected generated order 10, got %d", spec.Order)
	}
	if spec.Description != "基础的权限管理功能，包括登录，权限校验" {
		t.Fatalf("unexpected description: %s", spec.Description)
	}
}

func TestBaseModels(t *testing.T) {
	spec := findBaseSpec(t)
	if len(spec.Models) != 10 {
		t.Fatalf("expected 10 models, got %d", len(spec.Models))
	}
}

func TestBaseSeeds(t *testing.T) {
	spec := findBaseSpec(t)
	if spec.DB != "modules/base/db.json" {
		t.Fatalf("unexpected db seed path: %s", spec.DB)
	}
	if spec.Menu != "modules/base/menu.json" {
		t.Fatalf("unexpected menu seed path: %s", spec.Menu)
	}
}

func TestBaseMenuPlacesRecycleUnderDataManagement(t *testing.T) {
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), "modules", "base", "menu.json"))
	if err != nil {
		t.Fatalf("read menu failed: %v", err)
	}
	var menus []menuSeedItem
	if err = json.Unmarshal(content, &menus); err != nil {
		t.Fatalf("decode menu failed: %v", err)
	}

	for _, menu := range menus {
		if menu.Router == "/recycle/data" {
			t.Fatal("recycle menu must not be a top-level menu")
		}
		if menu.Name != "数据管理" {
			continue
		}
		for _, child := range menu.ChildMenus {
			if child.Router == "/recycle/data" {
				return
			}
		}
		t.Fatal("data management must contain the recycle menu")
	}
	t.Fatal("data management menu not found")
}
