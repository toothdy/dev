package gnentity

import (
	"reflect"
	"strings"
	"testing"
)

func TestDescriptorIndexesFieldsByEveryName(t *testing.T) {
	descriptor, err := Compile[supportedFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	byName, nameExists := descriptor.Field("optional")
	byJSON, jsonExists := descriptor.JSON("optional")
	byColumn, columnExists := descriptor.Column("optional")
	if !nameExists || !jsonExists || !columnExists || byName != byJSON || byName != byColumn {
		t.Fatalf("field indexes = %#v/%#v/%#v", byName, byJSON, byColumn)
	}
	if _, exists := descriptor.Field("missing"); exists {
		t.Fatal("Field(missing) exists")
	}
	if _, exists := descriptor.JSON("missing"); exists {
		t.Fatal("JSON(missing) exists")
	}
	if _, exists := descriptor.Column("missing"); exists {
		t.Fatal("Column(missing) exists")
	}
}

func TestCompileMergesSystemAndDeclaredIndexes(t *testing.T) {
	descriptor, err := Compile[supportedFieldsEntity, uint64](Schema{Indexes: []Index{
		IndexOf("idx_supported_bool_time", "bool", "stdTime"),
		UniqueIndexOf("uk_supported_named", "named"),
	}})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	want := []Index{
		IndexOf("idx_supported_fields_create_time", "createTime"),
		IndexOf("idx_supported_fields_update_time", "updateTime"),
		IndexOf("idx_supported_bool_time", "bool", "stdTime"),
		UniqueIndexOf("uk_supported_named", "named"),
	}
	if got := descriptor.Indexes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Indexes() = %#v, want %#v", got, want)
	}
}

func TestCompileRejectsInvalidIndexes(t *testing.T) {
	tests := []struct {
		name   string
		schema Schema
		want   string
	}{
		{name: "empty name", schema: Schema{Indexes: []Index{IndexOf("", "bool")}}, want: "索引名"},
		{name: "invalid name", schema: Schema{Indexes: []Index{IndexOf("Invalid-Name", "bool")}}, want: "索引名"},
		{name: "empty fields", schema: Schema{Indexes: []Index{IndexOf("idx_empty")}}, want: "字段"},
		{name: "empty field", schema: Schema{Indexes: []Index{IndexOf("idx_empty_field", "")}}, want: "字段"},
		{name: "duplicate field", schema: Schema{Indexes: []Index{IndexOf("idx_duplicate_field", "bool", "bool")}}, want: "重复字段"},
		{name: "unknown field", schema: Schema{Indexes: []Index{IndexOf("idx_unknown_field", "Missing")}}, want: "未知字段"},
		{name: "duplicate name", schema: Schema{Indexes: []Index{IndexOf("idx_duplicate", "bool"), IndexOf("idx_duplicate", "int")}}, want: "重复索引名"},
		{name: "system conflict", schema: Schema{Indexes: []Index{IndexOf("idx_supported_fields_create_time", "bool")}}, want: "重复索引名"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile[supportedFieldsEntity, uint64](test.schema)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestCompileRejectsTransientIndexFields(t *testing.T) {
	_, err := Compile[transientFieldsEntity, uint64](Schema{Indexes: []Index{
		IndexOf("idx_transient_role_ids", "roleIdList"),
	}})
	if err == nil || !strings.Contains(err.Error(), "未知字段") {
		t.Fatalf("Compile() error = %v", err)
	}
}

func TestDescriptorCollectionsAreImmutable(t *testing.T) {
	indexFields := []string{"bool", "int"}
	schema := Schema{Indexes: []Index{IndexOf("idx_mutable_input", indexFields...)}}
	descriptor, err := Compile[supportedFieldsEntity, uint64](schema)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	indexFields[0] = "Changed"
	schema.Indexes[0].Name = "changed_name"
	schema.Indexes[0].Fields[0] = "Changed"
	fields := descriptor.Fields()
	fields[0] = nil
	persistentFields := descriptor.PersistentFields()
	persistentFields[0] = nil
	indexes := descriptor.Indexes()
	if len(indexes) != 3 {
		t.Fatalf("Indexes() length = %d, want 3", len(indexes))
	}
	indexes[0].Name = "changed_system"
	indexes[0].Fields[0] = "Changed"
	indexes[2].Name = "changed_declared"
	indexes[2].Fields[0] = "Changed"

	if got := descriptor.Fields()[0]; got == nil || got.Name() != "id" {
		t.Fatalf("Fields() exposed internal state: %#v", got)
	}
	if got := descriptor.PersistentFields()[0]; got == nil || got.Name() != "id" {
		t.Fatalf("PersistentFields() exposed internal state: %#v", got)
	}
	want := []Index{
		IndexOf("idx_supported_fields_create_time", "createTime"),
		IndexOf("idx_supported_fields_update_time", "updateTime"),
		IndexOf("idx_mutable_input", "bool", "int"),
	}
	if got := descriptor.Indexes(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Indexes() exposed internal state: %#v", got)
	}
}
