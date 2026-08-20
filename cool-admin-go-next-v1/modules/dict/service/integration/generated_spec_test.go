package dict_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/controller"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestGeneratedModuleKeepsDictContract(t *testing.T) {
	generated := findSpec(t, "dict")
	if generated.Name != "字典管理" || generated.Description != "数据字典等" || generated.Order != 0 {
		t.Fatalf("unexpected generated Dict metadata: %#v", generated)
	}
	if generated.DB != "modules/dict/db.json" || generated.Menu != "" {
		t.Fatalf("unexpected generated Dict seed paths: %q/%q", generated.DB, generated.Menu)
	}
	if !reflect.DeepEqual(modelResources(generated), []string{"dict_info:dict.info", "dict_type:dict.type"}) {
		t.Fatalf("unexpected generated Dict models: %v", modelResources(generated))
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "mysql", DryRun: true})
	if err != nil {
		t.Fatalf("create dry-run db failed: %v", err)
	}
	definitions, err := generated.Controllers(module.Deps{DB: db})
	if err != nil {
		t.Fatalf("build generated Dict controllers failed: %v", err)
	}
	snapshots := controllerSnapshots(definitions)
	if len(snapshots) != 2 || snapshots[0].Prefix != "/admin/dict/info" || snapshots[0].Model != "dict_info" ||
		snapshots[1].Prefix != "/admin/dict/type" || snapshots[1].Model != "dict_type" {
		t.Fatalf("unexpected generated Dict controllers: %#v", snapshots)
	}
}

func findSpec(t *testing.T, key string) module.Spec {
	t.Helper()
	for _, spec := range modules.Specs() {
		if spec.Key == key {
			return spec
		}
	}
	t.Fatalf("module %s not found", key)
	return module.Spec{}
}

func modelResources(spec module.Spec) []string {
	resources := make([]string, 0, len(spec.Models))
	for _, definition := range spec.Models {
		resources = append(resources, definition.TableName+":"+definition.ResourceKey())
	}
	sort.Strings(resources)
	return resources
}

type controllerSnapshot struct {
	Prefix string
	Model  string
}

func controllerSnapshots(definitions []controller.Definition) []controllerSnapshot {
	snapshots := make([]controllerSnapshot, 0, len(definitions))
	for _, definition := range definitions {
		snapshots = append(snapshots, controllerSnapshot{Prefix: definition.Prefix, Model: definition.Model.TableName})
	}
	return snapshots
}
