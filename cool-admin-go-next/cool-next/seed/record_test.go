package seed

import (
	"encoding/json"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
)

type transientSeedEntity struct {
	g.Meta `orm:"table:transient_seed" description:"临时字段种子"`
	gnentity.Base
	Name    string    `json:"name" orm:"name" description:"名称"`
	RoleIDs *[]uint64 `json:"roleIds" description:"角色 ID" cool:"transient"`
}

func TestSeedRejectsTransientFields(t *testing.T) {
	descriptor, err := gnentity.Compile[transientSeedEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	record := Record{"roleIds": json.RawMessage("[1,2]")}
	if _, err = record.SeedData(descriptor); err == nil {
		t.Fatal("SeedData() error = nil")
	}
	if _, err = record.Values(descriptor); err == nil {
		t.Fatal("Values() error = nil")
	}
	if _, err = NewDO(descriptor, map[string]any{"roleIds": []uint64{1, 2}}, true); err == nil {
		t.Fatal("NewDO() error = nil")
	}
}

func TestRuntimeOnInitHonorsDisabledImports(t *testing.T) {
	runtime := &Runtime{}
	if err := runtime.OnInit(t.Context()); err != nil {
		t.Fatalf("OnInit() with disabled imports error = %v", err)
	}

	for _, config := range []Config{
		{ShouldImportDB: true},
		{ShouldImportMenu: true},
	} {
		runtime.config = config
		if err := runtime.OnInit(t.Context()); err == nil {
			t.Fatalf("OnInit() with config %#v error = nil", config)
		}
	}
}
