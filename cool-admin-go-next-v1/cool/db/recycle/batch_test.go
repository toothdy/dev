package recycle

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func recycleTestRootModel() entity.Definition {
	return entity.NewDefinition("demo", "DemoType", "demo_type").
		WithResource("demo.type").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary(),
			entity.NewField("name", "name", "varchar").NotNull(),
		})
}

func recycleTestRelationModel() entity.Definition {
	return entity.NewDefinition("demo", "DemoTypeItem", "demo_type_item").
		WithResource("demo.typeItem").
		Fields([]entity.Field{
			entity.NewField("typeId", "typeId", "bigint").Unsigned().NotNull(),
			entity.NewField("itemId", "item_id", "bigint").Unsigned().NotNull(),
		}).
		WithIndexes(entity.NewUniqueIndex("uk_demo_type_item", "typeId", "item_id"))
}

func TestCatalogCompilesStableResourcesAndJointIdentity(t *testing.T) {
	rootModel := recycleTestRootModel()
	relationModel := recycleTestRelationModel()
	catalog, err := NewCatalog([]entity.Definition{rootModel, relationModel})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	metadata, ok := catalog.Model("demo.typeItem")
	if !ok || len(metadata.IdentityFields) != 2 {
		t.Fatalf("unexpected relation identity metadata: %#v", metadata)
	}
	if metadata.IdentityFields[0].ColumnName != "typeId" || metadata.IdentityFields[1].ColumnName != "item_id" {
		t.Fatalf("joint identity order changed: %#v", metadata.IdentityFields)
	}
}

func TestCatalogFreezesModelSlices(t *testing.T) {
	definition := recycleTestRootModel()
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	definition.FieldsValue[0].JSONName = "changed"
	metadata, ok := catalog.Model("demo.type")
	if !ok || metadata.Definition.FieldsValue[0].JSONName != "id" {
		t.Fatalf("catalog model was mutated through source slice: %#v", metadata.Definition.FieldsValue)
	}
	metadata.Definition.FieldsValue[0].JSONName = "returned-change"
	again, _ := catalog.Model("demo.type")
	if again.Definition.FieldsValue[0].JSONName != "id" {
		t.Fatalf("catalog model was mutated through returned slice: %#v", again.Definition.FieldsValue)
	}
}

func TestBatchValidatesDependenciesAndPreservesBigInt(t *testing.T) {
	rootModel := recycleTestRootModel()
	relationModel := recycleTestRelationModel()
	catalog, err := NewCatalog([]entity.Definition{rootModel, relationModel})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	archive := &Archive{}
	batch := NewBatch(catalog, archive)
	rootKey, err := batch.AddRecord(rootModel, map[string]interface{}{
		"id": uint64(18446744073709551615), "name": "root",
	}, ItemOptions{BranchKey: "branch-1"})
	if err != nil {
		t.Fatalf("add recycle root failed: %v", err)
	}
	_, err = batch.AddRecord(relationModel, map[string]interface{}{
		"typeId": uint64(18446744073709551615), "itemId": uint64(9223372036854775808),
	}, ItemOptions{BranchKey: "branch-1", ParentKey: rootKey, RestoreOrder: 2})
	if err != nil {
		t.Fatalf("add recycle child failed: %v", err)
	}
	if err = batch.Validate(); err != nil {
		t.Fatalf("validate recycle batch failed: %v", err)
	}
	if archive.Count != 2 || archive.RemainingCount != 2 || len(archive.Items) != 2 {
		t.Fatalf("unexpected archive counters: %#v", archive)
	}
	if !strings.Contains(string(archive.Items[0].Data), "18446744073709551615") ||
		!strings.Contains(string(archive.Items[1].Data), "9223372036854775808") {
		t.Fatalf("bigint snapshot lost precision: %s %s", archive.Items[0].Data, archive.Items[1].Data)
	}
	identityJSON, err := json.Marshal(archive.Items[1].Identity)
	if err != nil || !strings.Contains(string(identityJSON), "9223372036854775808") {
		t.Fatalf("bigint identity lost precision: %s, %v", identityJSON, err)
	}
}

func TestBatchRejectsDependencyCycleAndCrossBranchParent(t *testing.T) {
	rootModel := recycleTestRootModel()
	catalog, err := NewCatalog([]entity.Definition{rootModel})
	if err != nil {
		t.Fatalf("compile recycle catalog failed: %v", err)
	}
	batch := NewBatch(catalog, &Archive{})
	firstKey, err := batch.AddRecord(rootModel, map[string]interface{}{"id": uint64(1), "name": "first"}, ItemOptions{BranchKey: "a"})
	if err != nil {
		t.Fatalf("add first item failed: %v", err)
	}
	secondKey, err := batch.AddRecord(rootModel, map[string]interface{}{"id": uint64(2), "name": "second"}, ItemOptions{BranchKey: "a", ParentKey: firstKey})
	if err != nil {
		t.Fatalf("add second item failed: %v", err)
	}
	batch.items[firstKey].ParentKey = secondKey
	if err = batch.Validate(); err == nil || !strings.Contains(err.Error(), "成环") {
		t.Fatalf("expected dependency cycle rejected, got %v", err)
	}

	crossBranch := NewBatch(catalog, &Archive{})
	parentKey, _ := crossBranch.AddRecord(rootModel, map[string]interface{}{"id": uint64(3), "name": "parent"}, ItemOptions{BranchKey: "a"})
	_, _ = crossBranch.AddRecord(rootModel, map[string]interface{}{"id": uint64(4), "name": "child"}, ItemOptions{BranchKey: "b", ParentKey: parentKey})
	if err = crossBranch.Validate(); err == nil || !strings.Contains(err.Error(), "跨分支") {
		t.Fatalf("expected cross-branch dependency rejected, got %v", err)
	}
}

func TestCatalogRequiresExplicitIdentityForAmbiguousUniqueIndexes(t *testing.T) {
	definition := entity.NewDefinition("demo", "Ambiguous", "demo_ambiguous").
		Fields([]entity.Field{
			entity.NewField("code", "code", "varchar").NotNull(),
			entity.NewField("label", "label", "varchar").NotNull(),
		}).
		WithIndexes(
			entity.NewUniqueIndex("uk_demo_code", "code"),
			entity.NewUniqueIndex("uk_demo_label", "label"),
		)
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile ambiguous catalog failed: %v", err)
	}
	batch := NewBatch(catalog, &Archive{})
	if _, err = batch.AddRecord(definition, map[string]interface{}{"code": "a", "label": "b"}, ItemOptions{}); err == nil {
		t.Fatal("expected ambiguous identity rejected")
	}
	if _, err = batch.AddRecord(definition, map[string]interface{}{"code": "a", "label": "b"}, ItemOptions{IdentityFields: []string{"code"}}); err != nil {
		t.Fatalf("explicit identity should be accepted: %v", err)
	}
}

func TestBatchPreservesJSONColumnStructure(t *testing.T) {
	definition := entity.NewDefinition("demo", "DemoJSON", "demo_json").
		Fields([]entity.Field{
			entity.NewField("id", "id", "bigint").Unsigned().Primary(),
			entity.NewField("payload", "payload", "json").NotNull(),
		})
	catalog, err := NewCatalog([]entity.Definition{definition})
	if err != nil {
		t.Fatalf("compile JSON model failed: %v", err)
	}
	archive := &Archive{}
	batch := NewBatch(catalog, archive)
	if _, err = batch.AddRecord(definition, map[string]interface{}{
		"id": uint64(1), "payload": []byte(`{"enabled":true}`),
	}, ItemOptions{}); err != nil {
		t.Fatalf("archive JSON record failed: %v", err)
	}
	if !strings.Contains(string(archive.Items[0].Data), `"payload":{"enabled":true}`) {
		t.Fatalf("JSON snapshot was encoded as a string or base64: %s", archive.Items[0].Data)
	}
	metadata, _ := catalog.Model(definition.ResourceKey())
	_, args, err := buildRestoreInsert(metadata, archive.Items[0].Data)
	if err != nil || len(args) != 2 || args[1] != `{"enabled":true}` {
		t.Fatalf("JSON restore value changed: args=%#v err=%v", args, err)
	}
}
