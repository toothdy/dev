package integration

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/modules"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestGeneratedModuleKeepsRecycleContract(t *testing.T) {
	spec := findRecycleSpec(t)
	if spec.Name != "数据回收" || spec.Description != "收集被删除的数据，管理和恢复" || spec.Order != 0 {
		t.Fatalf("unexpected generated Recycle metadata: %#v", spec)
	}
	if spec.DB != "modules/recycle/db.json" || spec.Menu != "" {
		t.Fatalf("unexpected generated Recycle seed paths: %q/%q", spec.DB, spec.Menu)
	}
	if len(spec.Models) != 2 || spec.Models[0].TableName != "recycle_data" || spec.Models[1].TableName != "recycle_item" {
		t.Fatalf("unexpected generated Recycle models: %#v", spec.Models)
	}
	if spec.RecycleProvider == nil || spec.Runtime != nil || spec.Controllers == nil {
		t.Fatal("generated Recycle factories do not match the module contract")
	}
	if spec.Configure != nil {
		if err := spec.Configure(context.Background()); err != nil {
			t.Fatalf("configure Recycle module failed: %v", err)
		}
	}

	db, err := gdb.New(gdb.ConfigNode{
		Type: "mysql", Host: "127.0.0.1", Port: "3306", User: "test", Pass: "test", Name: "test", DryRun: true,
	})
	if err != nil {
		t.Fatalf("create generated Recycle test database failed: %v", err)
	}
	defer db.Close(context.Background())
	runtimeModels := append([]entity.Definition{}, spec.Models...)
	runtimeModels = append(runtimeModels, baseEntity.BaseSysConf())
	manager, runtime, err := spec.RecycleProvider(module.RuntimeDeps{
		Context: context.Background(), DB: db, Models: runtimeModels,
		CRUDOptions: module.CRUDOptions{SoftDelete: true},
	})
	if err != nil {
		t.Fatalf("build generated Recycle runtime failed: %v", err)
	}
	if manager == nil || runtime == nil {
		t.Fatal("generated Recycle provider must return Manager and Schedule runtime")
	}
}

func findRecycleSpec(t *testing.T) module.Spec {
	t.Helper()
	for _, spec := range modules.Specs() {
		if spec.Key == "recycle" {
			return spec
		}
	}
	t.Fatal("recycle module not found")
	return module.Spec{}
}