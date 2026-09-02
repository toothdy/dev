package gnentity

import (
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestBaseShape(t *testing.T) {
	typ := reflect.TypeFor[Base]()
	if typ.NumField() != 3 {
		t.Fatalf("Base fields = %d, want 3", typ.NumField())
	}

	tests := []struct {
		index       int
		name        string
		typ         reflect.Type
		jsonName    string
		column      string
		description string
	}{
		{index: 0, name: "ID", typ: reflect.TypeFor[uint64](), jsonName: "id", column: "id", description: "ID"},
		{index: 1, name: "CreateTime", typ: reflect.TypeFor[*gtime.Time](), jsonName: "createTime", column: "createTime", description: "创建时间"},
		{index: 2, name: "UpdateTime", typ: reflect.TypeFor[*gtime.Time](), jsonName: "updateTime", column: "updateTime", description: "更新时间"},
	}

	for _, test := range tests {
		field := typ.Field(test.index)
		if field.Name != test.name || field.Type != test.typ {
			t.Fatalf("Base field %d = %s %s", test.index, field.Name, field.Type)
		}
		if field.Tag.Get("json") != test.jsonName || field.Tag.Get("orm") != test.column ||
			field.Tag.Get("description") != test.description {
			t.Fatalf("Base field %s tags = %q", field.Name, field.Tag)
		}
	}
}

func TestIndexConstructorsCopyFields(t *testing.T) {
	fields := []string{"status", "createTime"}
	ordinary := IndexOf("idx_goods_status_time", fields...)
	unique := UniqueIndexOf("uk_goods_status_time", fields...)
	fields[0] = "changed"

	if ordinary.Unique || ordinary.Fields[0] != "status" {
		t.Fatalf("IndexOf() = %#v", ordinary)
	}
	if !unique.Unique || unique.Fields[0] != "status" {
		t.Fatalf("UniqueIndexOf() = %#v", unique)
	}
}
