package entity

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func TestRecycleModelFactories(t *testing.T) {
	definitions := []entity.Definition{Data(), Item()}
	if len(definitions) != 2 {
		t.Fatalf("expected 2 models, got %d", len(definitions))
	}
	if definitions[0].TableName != "recycle_data" || definitions[1].TableName != "recycle_item" {
		t.Fatalf("unexpected recycle models: %s, %s", definitions[0].TableName, definitions[1].TableName)
	}
	if _, ok := definitions[0].FieldByJSONName("remainingCount"); !ok {
		t.Fatal("recycle_data missing remainingCount")
	}
	if _, ok := definitions[1].FieldByJSONName("parentItemId"); !ok {
		t.Fatal("recycle_item missing parentItemId")
	}
}

func TestRecycleModelsHaveTenantIndexes(t *testing.T) {
	for _, definition := range []entity.Definition{Data(), Item()} {
		hasTenantIndex := false
		for _, index := range definition.Indexes {
			for _, column := range index.Columns {
				if column == "tenantId" {
					hasTenantIndex = true
				}
			}
		}
		if !hasTenantIndex {
			t.Fatalf("model %s missing tenant index", definition.TableName)
		}
	}
}
