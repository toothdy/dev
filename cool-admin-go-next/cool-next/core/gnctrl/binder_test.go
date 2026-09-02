package gnctrl

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
)

type transientBinderEntity struct {
	g.Meta `orm:"table:transient_binder" description:"临时字段绑定"`
	gnentity.Base
	Name       string    `json:"name" orm:"name" description:"名称"`
	Enabled    bool      `json:"enabled" orm:"enabled" description:"是否启用"`
	RoleIDList *[]uint64 `json:"roleIdList" description:"角色 ID 列表" cool:"transient"`
}

func TestDecodeMutableAcceptsBooleanNumbers(t *testing.T) {
	descriptor, err := gnentity.Compile[transientBinderEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		raw     string
		want    bool
		wantErr bool
	}{
		{name: "false", raw: "false"},
		{name: "true", raw: "true", want: true},
		{name: "zero", raw: "0"},
		{name: "one", raw: "1", want: true},
		{name: "invalid number", raw: "2", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, decodeErr := decodeMutable[transientBinderEntity, uint64](map[string]json.RawMessage{
				"enabled": json.RawMessage(test.raw),
			}, descriptor)
			if test.wantErr {
				if decodeErr == nil {
					t.Fatal("expected decode error")
				}
				return
			}
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			value, exists := mutable.Get("enabled")
			if !exists || value != test.want {
				t.Fatalf("Get(enabled) = %#v/%v", value, exists)
			}
		})
	}
}

func TestDecodeMutablePreservesTransientFieldStates(t *testing.T) {
	descriptor, err := gnentity.Compile[transientBinderEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		raw       map[string]json.RawMessage
		has       bool
		isNull    bool
		wantValue []uint64
	}{
		{name: "missing", raw: map[string]json.RawMessage{}},
		{name: "null", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("null")}, has: true, isNull: true},
		{name: "empty", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("[]")}, has: true, wantValue: []uint64{}},
		{name: "values", raw: map[string]json.RawMessage{"roleIdList": json.RawMessage("[1,2]")}, has: true, wantValue: []uint64{1, 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, decodeErr := decodeMutable[transientBinderEntity, uint64](test.raw, descriptor)
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if mutable.Has("roleIdList") != test.has || mutable.IsNull("roleIdList") != test.isNull {
				t.Fatalf("state = has:%v null:%v", mutable.Has("roleIdList"), mutable.IsNull("roleIdList"))
			}
			value, exists := mutable.Get("roleIdList")
			if exists != test.has || test.has && !test.isNull && !reflect.DeepEqual(value, test.wantValue) {
				t.Fatalf("Get(roleIdList) = %#v/%v", value, exists)
			}
			if values, ok := value.([]uint64); ok && len(values) > 0 {
				values[0] = 99
				if current, _ := mutable.Get("roleIdList"); reflect.DeepEqual(current, values) {
					t.Fatal("Get(roleIdList) exposed internal slice")
				}
			}
		})
	}
}
