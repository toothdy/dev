package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	coolModule "github.com/toothdy/cool-admin-go-next/cool/module"
	recycleModule "github.com/toothdy/cool-admin-go-next/modules/recycle"
)

func TestRecycleConfigUsesDocumentedDefaults(t *testing.T) {
	withRecycleConfig(t, "")
	declaration := recycleModule.ModuleConfig()
	config, err := coolModule.LoadConfig(context.Background(), "recycle", declaration.Defaults)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if config.CleanupInterval != 24*time.Hour || config.CleanupBatch != 500 || config.LockName != "cool-admin:recycle:cleanup" {
		t.Fatalf("unexpected defaults: %#v", config)
	}
	if declaration.Name != "数据回收" || declaration.Description != "收集被删除的数据，管理和恢复" || declaration.Order != 0 {
		t.Fatalf("unexpected declaration: %#v", declaration)
	}
}

func TestRecycleConfigOverlaysAndValidates(t *testing.T) {
	withRecycleConfig(t, "module:\n  recycle:\n    cleanupInterval: 12h\n    cleanupBatch: 20\n    lockName: cleanup")
	config, err := coolModule.LoadConfig(context.Background(), "recycle", recycleModule.ModuleConfig().Defaults)
	if err != nil {
		t.Fatalf("load config failed: %v", err)
	}
	if config.CleanupInterval != 12*time.Hour || config.CleanupBatch != 20 || config.LockName != "cleanup" {
		t.Fatalf("unexpected custom config: %#v", config)
	}

	withRecycleConfig(t, "module:\n  recycle:\n    cleanupBatch: 0")
	if _, err = coolModule.LoadConfig(context.Background(), "recycle", recycleModule.ModuleConfig().Defaults); err == nil {
		t.Fatal("expected invalid cleanup batch to fail")
	}
}

func withRecycleConfig(t *testing.T, content string) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previous := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() { config.SetAdapter(previous) })
}
