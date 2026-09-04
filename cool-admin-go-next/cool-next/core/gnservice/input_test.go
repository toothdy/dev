package gnservice

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
)

type inputEntity struct {
	g.Meta `orm:"table:service_inputs" description:"服务输入"`
	gnentity.Base
	Count   uint64    `json:"count" orm:"count" description:"数量"`
	Enabled bool      `json:"enabled" orm:"enabled" description:"启用"`
	At      time.Time `json:"at" orm:"at" description:"时间"`
	Data    []byte    `json:"data" orm:"data" description:"数据"`
	Note    *string   `json:"note" orm:"note" description:"备注"`
	RoleIDs *[]uint64 `json:"roleIds" description:"角色 ID" cool:"transient"`
}

type wrongIDDescriptor struct {
	gnentity.Descriptor[inputEntity, uint64]
}

func (wrongIDDescriptor) IDType() reflect.Type { return reflect.TypeFor[string]() }

func TestMutablePreservesFieldStatesAndCopiesValues(t *testing.T) {
	descriptor := inputDescriptor(t)
	timestamp := time.Date(2026, time.August, 10, 9, 8, 7, 6, time.UTC)
	original := []byte{1, 2}
	originalRoleIDs := []uint64{1, 2}
	field := Value("data", original)
	roleField := Value("roleIds", originalRoleIDs)
	original[0] = 9
	originalRoleIDs[0] = 9

	mutable, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{
		Value("count", uint64(0)),
		Value("enabled", false),
		Value("at", timestamp),
		field,
		roleField,
		Null("note"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if mutable.Has("missing") || !mutable.Has("count") || mutable.IsNull("count") {
		t.Fatalf("count state = has:%v null:%v", mutable.Has("count"), mutable.IsNull("count"))
	}
	if value, exists := mutable.Get("count"); !exists || value != uint64(0) {
		t.Fatalf("count = %#v, %v", value, exists)
	}
	if value, exists := mutable.Get("enabled"); !exists || value != false {
		t.Fatalf("enabled = %#v, %v", value, exists)
	}
	if value, exists := mutable.Get("at"); !exists || value != timestamp {
		t.Fatalf("at = %#v, %v", value, exists)
	}
	if value, exists := mutable.Get("note"); !exists || value != nil || !mutable.IsNull("note") {
		t.Fatalf("note = %#v, %v, null:%v", value, exists, mutable.IsNull("note"))
	}
	data, exists := mutable.Get("data")
	if !exists || !reflect.DeepEqual(data, []byte{1, 2}) {
		t.Fatalf("data = %#v, %v", data, exists)
	}
	data.([]byte)[0] = 8
	data, _ = mutable.Get("data")
	if !reflect.DeepEqual(data, []byte{1, 2}) {
		t.Fatalf("data after mutation = %#v", data)
	}
	roleIDs, exists := mutable.Get("roleIds")
	if !exists || !reflect.DeepEqual(roleIDs, []uint64{1, 2}) {
		t.Fatalf("roleIds = %#v, %v", roleIDs, exists)
	}
	roleIDs.([]uint64)[0] = 8
	if roleIDs, _ = mutable.Get("roleIds"); !reflect.DeepEqual(roleIDs, []uint64{1, 2}) {
		t.Fatalf("roleIds after mutation = %#v", roleIDs)
	}

	if err := mutable.Set("enabled", true); err != nil {
		t.Fatal(err)
	}
	if enabled, exists := mutable.Get("enabled"); !exists || enabled != true {
		t.Fatalf("enabled after set = %#v, %v", enabled, exists)
	}
	if err := mutable.SetNull("note"); err != nil {
		t.Fatal(err)
	}
	if err := mutable.SetNull("enabled"); err == nil {
		t.Fatal("SetNull(enabled) error = nil")
	}
	if err := mutable.Set("count", int(0)); err == nil {
		t.Fatal("Set(count, int(0)) error = nil")
	}
	if err := mutable.Unset("count"); err != nil {
		t.Fatal(err)
	}
	if mutable.Has("count") {
		t.Fatal("count still exists after Unset")
	}
	if err := mutable.Unset("missing"); err == nil {
		t.Fatal("Unset(missing) error = nil")
	}
}

func TestMutableTracksFieldSource(t *testing.T) {
	descriptor := inputDescriptor(t)
	mutable, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{
		Value("count", uint64(1)),
		Value("enabled", false),
		Null("note"),
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"count", "enabled", "note"} {
		source, exists := mutable.source(field)
		if !exists || source != fieldSourceClient {
			t.Fatalf("source(%q) = %v, %v", field, source, exists)
		}
	}
	if _, exists := mutable.source("missing"); exists {
		t.Fatal("source(missing) exists")
	}

	if err := mutable.Set("count", uint64(2)); err != nil {
		t.Fatal(err)
	}
	if source, exists := mutable.source("count"); !exists || source != fieldSourceServer {
		t.Fatalf("source(count) after Set = %v, %v", source, exists)
	}
	if err := mutable.SetNull("note"); err != nil {
		t.Fatal(err)
	}
	if source, exists := mutable.source("note"); !exists || source != fieldSourceServer {
		t.Fatalf("source(note) after SetNull = %v, %v", source, exists)
	}

	if err := mutable.Set("count", int(3)); err == nil {
		t.Fatal("Set(count, int(3)) error = nil")
	}
	if source, _ := mutable.source("count"); source != fieldSourceServer {
		t.Fatalf("source(count) after failed Set = %v", source)
	}
	if value, exists := mutable.Get("count"); !exists || value != uint64(2) {
		t.Fatalf("count after failed Set = %#v, %v", value, exists)
	}

	if err := mutable.SetNull("enabled"); err == nil {
		t.Fatal("SetNull(enabled) error = nil")
	}
	if source, _ := mutable.source("enabled"); source != fieldSourceClient {
		t.Fatalf("source(enabled) after failed SetNull = %v", source)
	}
	if value, exists := mutable.Get("enabled"); !exists || value != false {
		t.Fatalf("enabled after failed SetNull = %#v, %v", value, exists)
	}
}

func TestInputConstructorsKeepShapeAndCopySlices(t *testing.T) {
	descriptor := inputDescriptor(t)
	first := newMutable(t, descriptor, "first")
	second := newMutable(t, descriptor, "second")

	object, err := NewAddObject[inputEntity, uint64](descriptor, first)
	if err != nil {
		t.Fatal(err)
	}
	if object.IsMany() || object.One() != first || object.Many() != nil {
		t.Fatalf("object shape = many:%v one:%p list:%#v", object.IsMany(), object.One(), object.Many())
	}
	values := []*Mutable[inputEntity]{first, second}
	array, err := NewAddArray[inputEntity, uint64](descriptor, values)
	if err != nil {
		t.Fatal(err)
	}
	values[0] = second
	returned := array.Many()
	if !array.IsMany() || len(returned) != 2 || returned[0] != first {
		t.Fatalf("array shape = many:%v values:%#v", array.IsMany(), returned)
	}
	returned[0] = second
	if array.Many()[0] != first {
		t.Fatal("Many() leaked internal slice")
	}

	ids := []uint64{1, 2, 1}
	deleted, err := NewDeleteInput[inputEntity](descriptor, ids)
	if err != nil {
		t.Fatal(err)
	}
	ids[0] = 9
	if got := deleted.IDs(); !reflect.DeepEqual(got, []uint64{1, 2}) {
		t.Fatalf("IDs() = %#v", got)
	}
	got := deleted.IDs()
	got[0] = 8
	if got := deleted.IDs(); !reflect.DeepEqual(got, []uint64{1, 2}) {
		t.Fatalf("IDs() after mutation = %#v", got)
	}

	item, err := NewUpdateItem(descriptor, uint64(1), first)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := NewUpdateObject(descriptor, item)
	if err != nil {
		t.Fatal(err)
	}
	if updated.IsMany() || updated.One().ID() != uint64(1) || updated.Many() != nil {
		t.Fatalf("update object shape = %#v", updated)
	}
	items := []UpdateItem[inputEntity, uint64]{item}
	updates, err := NewUpdateArray(descriptor, items)
	if err != nil {
		t.Fatal(err)
	}
	items[0] = UpdateItem[inputEntity, uint64]{}
	if !updates.IsMany() || updates.Many()[0].ID() != uint64(1) {
		t.Fatalf("update array = %#v", updates.Many())
	}
}

func TestConstructorsRejectInvalidInput(t *testing.T) {
	descriptor := inputDescriptor(t)
	value := newMutable(t, descriptor, "value")

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "unknown field",
			call: func() error {
				_, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("missing", "value")})
				return err
			},
		},
		{
			name: "duplicate field",
			call: func() error {
				_, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("count", uint64(1)), Value("count", uint64(2))})
				return err
			},
		},
		{
			name: "plain nil",
			call: func() error {
				_, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("note", nil)})
				return err
			},
		},
		{
			name: "non nullable null",
			call: func() error {
				_, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Null("enabled")})
				return err
			},
		},
		{
			name: "wrong value type",
			call: func() error {
				_, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("count", int(1))})
				return err
			},
		},
		{
			name: "empty add array",
			call: func() error {
				_, err := NewAddArray[inputEntity, uint64](descriptor, nil)
				return err
			},
		},
		{
			name: "empty delete ids",
			call: func() error {
				_, err := NewDeleteInput[inputEntity, uint64](descriptor, nil)
				return err
			},
		},
		{
			name: "wrong id descriptor",
			call: func() error {
				_, err := NewDeleteInput[inputEntity](wrongIDDescriptor{Descriptor: descriptor}, []uint64{1})
				return err
			},
		},
		{
			name: "empty update array",
			call: func() error {
				_, err := NewUpdateArray[inputEntity, uint64](descriptor, nil)
				return err
			},
		},
		{
			name: "nonpositive pagination",
			call: func() error {
				_, err := NewQuery(nil, 0, 15)
				return err
			},
		},
		{
			name: "nonpositive list limit",
			call: func() error {
				_, err := NewListQuery(nil, 0)
				return err
			},
		},
		{
			name: "different mutable descriptor",
			call: func() error {
				_, err := NewAddObject[inputEntity, uint64](descriptor, &Mutable[inputEntity]{})
				return err
			},
		},
		{
			name: "invalid update item",
			call: func() error {
				_, err := NewUpdateItem[inputEntity](descriptor, uint64(1), nil)
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if err == nil {
				t.Fatal("error = nil")
			}
			assertValidateError(t, err)
		})
	}
	if _, err := NewUpdateItem(descriptor, uint64(1), value); err != nil {
		t.Fatal(err)
	}
}

func TestAddResultRecordAndQuery(t *testing.T) {
	one, err := json.Marshal(AddResult[uint64]{one: 1})
	if err != nil || string(one) != `{"id":1}` {
		t.Fatalf("single result = %s, %v", one, err)
	}
	many, err := json.Marshal(AddResult[uint64]{isMany: true, many: []uint64{1, 2}})
	if err != nil || string(many) != `{"id":[1,2]}` {
		t.Fatalf("array result = %s, %v", many, err)
	}
	result := AddResult[uint64]{isMany: true, many: []uint64{1, 2}}
	ids := result.Many()
	ids[0] = 9
	if got := result.Many(); !reflect.DeepEqual(got, []uint64{1, 2}) {
		t.Fatalf("result IDs = %#v", got)
	}

	timestamp := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	record := Record{values: map[string]any{
		"id":      uint64(math.MaxUint64),
		"enabled": false,
		"at":      timestamp,
	}}
	if value, exists := record.Get("id"); !exists || value != uint64(math.MaxUint64) {
		t.Fatalf("record ID = %#v, %v", value, exists)
	}
	encoded, err := json.Marshal(PageResult{List: []Record{record}, Pagination: Pagination{Page: 1, Size: 15}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"id":18446744073709551615`) {
		t.Fatalf("page JSON = %s", encoded)
	}
	var scanned struct {
		ID      uint64    `json:"id"`
		Enabled bool      `json:"enabled"`
		At      time.Time `json:"at"`
	}
	if err := record.Scan(&scanned); err != nil {
		t.Fatal(err)
	}
	if scanned.ID != math.MaxUint64 || scanned.Enabled || scanned.At != timestamp {
		t.Fatalf("scanned = %#v", scanned)
	}

	request, err := crud.NewQueryRequest([]crud.RequestValue{crud.RequestField("enabled", false)})
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(request, 1, 15)
	if err != nil {
		t.Fatal(err)
	}
	if query.Request() != request || query.PageNumber() != 1 || query.PageSize() != 15 {
		t.Fatalf("query = %#v", query)
	}
	list, err := NewListQuery(request, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if list.Request() != request || list.PageNumber() != 1 || list.PageSize() != 1 || list.ListLimit() != 1000 {
		t.Fatalf("list query = %#v", list)
	}
}

func inputDescriptor(t *testing.T) gnentity.Descriptor[inputEntity, uint64] {
	t.Helper()
	descriptor, err := gnentity.Compile[inputEntity, uint64](gnentity.Schema{})
	if err != nil {
		t.Fatal(err)
	}

	return descriptor
}

func newMutable(
	t *testing.T,
	descriptor gnentity.Descriptor[inputEntity, uint64],
	note string,
) *Mutable[inputEntity] {
	t.Helper()
	value, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("note", note)})
	if err != nil {
		t.Fatal(err)
	}

	return value
}

func assertValidateError(t *testing.T, err error) {
	t.Helper()
	var base *exception.BaseException
	if !errors.As(err, &base) || base.Code != exception.ValidateFail {
		t.Fatalf("error = %#v", err)
	}
}
