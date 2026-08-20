package integration

import (
	"testing"

	recycleModule "github.com/toothdy/cool-admin-go-next/modules/recycle"
	"github.com/toothdy/cool-admin-go-next/modules/recycle/entity"
)

func TestRecycleModuleDeclarationAndModels(t *testing.T) {
	declaration := recycleModule.ModuleConfig()
	if declaration.Name != "数据回收" || declaration.Description != "收集被删除的数据，管理和恢复" || declaration.Order != 0 {
		t.Fatalf("unexpected Recycle declaration: %#v", declaration)
	}
	if entity.Data().TableName != "recycle_data" || entity.Item().TableName != "recycle_item" {
		t.Fatalf("unexpected Recycle models: %#v %#v", entity.Data(), entity.Item())
	}
}
