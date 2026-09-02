package gnentity

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

type doValueEntity struct {
	g.Meta `orm:"table:do_values" description:"DO 值"`
	Base
	Count           int64   `json:"count" orm:"count" description:"数量"`
	Enabled         bool    `json:"enabled" orm:"enabled" description:"是否启用"`
	Title           string  `json:"title" orm:"title" description:"标题"`
	Payload         []byte  `json:"payload" orm:"payload" description:"载荷"`
	Remark          *string `json:"remark" orm:"remark" description:"备注"`
	OptionalPayload *[]byte `json:"optionalPayload" orm:"optionalPayload" description:"可空载荷"`
}

type doNamedString string

type doExactTypesEntity struct {
	g.Meta `orm:"table:do_exact_types" description:"DO 精确类型"`
	Base
	BoolValue    bool          `json:"boolValue" orm:"boolValue" description:"布尔"`
	IntValue     int           `json:"intValue" orm:"intValue" description:"整型"`
	Int8Value    int8          `json:"int8Value" orm:"int8Value" description:"八位整型"`
	Int16Value   int16         `json:"int16Value" orm:"int16Value" description:"十六位整型"`
	Int32Value   int32         `json:"int32Value" orm:"int32Value" description:"三十二位整型"`
	Int64Value   int64         `json:"int64Value" orm:"int64Value" description:"六十四位整型"`
	UintValue    uint          `json:"uintValue" orm:"uintValue" description:"无符号整型"`
	Uint8Value   uint8         `json:"uint8Value" orm:"uint8Value" description:"八位无符号整型"`
	Uint16Value  uint16        `json:"uint16Value" orm:"uint16Value" description:"十六位无符号整型"`
	Uint32Value  uint32        `json:"uint32Value" orm:"uint32Value" description:"三十二位无符号整型"`
	Uint64Value  uint64        `json:"uint64Value" orm:"uint64Value" description:"六十四位无符号整型"`
	Float32Value float32       `json:"float32Value" orm:"float32Value" description:"单精度浮点"`
	Float64Value float64       `json:"float64Value" orm:"float64Value" description:"双精度浮点"`
	StringValue  string        `json:"stringValue" orm:"stringValue" description:"字符串"`
	BytesValue   []byte        `json:"bytesValue" orm:"bytesValue" description:"字节"`
	TimeValue    time.Time     `json:"timeValue" orm:"timeValue" description:"标准时间"`
	GTimeValue   gtime.Time    `json:"gtimeValue" orm:"gtimeValue" description:"框架时间"`
	NamedValue   doNamedString `json:"namedValue" orm:"namedValue" description:"命名字符串"`
}

type doJSONEntity struct {
	g.Meta `orm:"table:do_json" description:"DO JSON"`
	Base
	RoleIDs []uint64        `json:"roleIds" orm:"roleIds" description:"角色" cool:"json=true"`
	Params  *map[string]any `json:"params" orm:"params" description:"参数" cool:"json=true"`
}

func TestDOValuePreservesExactJSONTypes(t *testing.T) {
	descriptor, err := Compile[doJSONEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()
	roleIDs := []uint64{1, 3}
	params := map[string]any{"path": "/admin/base/sys/user/page"}
	if err = value.SetColumn("roleIds", roleIDs); err != nil {
		t.Fatal(err)
	}
	if err = value.SetColumn("params", params); err != nil {
		t.Fatal(err)
	}
	if got := getDOColumnValue(t, value.DBData(), "roleIds"); !reflect.DeepEqual(got, roleIDs) || reflect.TypeOf(got) != reflect.TypeOf(roleIDs) {
		t.Fatalf("roleIds = %#v (%T)", got, got)
	}
	if got := getDOColumnValue(t, value.DBData(), "params"); !reflect.DeepEqual(got, params) || reflect.TypeOf(got) != reflect.TypeOf(params) {
		t.Fatalf("params = %#v (%T)", got, got)
	}
	if err = descriptor.NewDO().SetColumn("roleIds", []int{1, 3}); err == nil {
		t.Fatal("JSON slice element type mismatch should fail")
	}
	if err = descriptor.NewDO().SetColumn("params", map[string]string{"path": "value"}); err == nil {
		t.Fatal("JSON map value type mismatch should fail")
	}
}

func TestDOValueHandlesJSONTypedNil(t *testing.T) {
	descriptor, err := Compile[doJSONEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	var (
		nullRoleIDs []uint64
		nullParams  map[string]any
	)
	if err = descriptor.NewDO().SetColumn("roleIds", nullRoleIDs); err == nil {
		t.Fatal("non-nullable JSON slice should reject typed nil")
	}
	value := descriptor.NewDO()
	if err = value.SetColumn("params", nullParams); err != nil {
		t.Fatal(err)
	}
	if !value.Has("params") || !value.IsNull("params") || getDOColumnValue(t, value.DBData(), "params") != gdb.Raw("NULL") {
		t.Fatalf("params state = has:%v null:%v data:%#v", value.Has("params"), value.IsNull("params"), value.DBData())
	}
}

func TestDOValueHandlesExplicitNull(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()

	var nullString *string
	if err := value.SetColumn("remark", nullString); err != nil {
		t.Fatal(err)
	}
	var nullBytes []byte
	if err := value.SetColumn("optionalPayload", nullBytes); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"remark", "optionalPayload"} {
		if !value.Has(field) || !value.IsNull(field) {
			t.Fatalf("state %s = has:%v null:%v", field, value.Has(field), value.IsNull(field))
		}
		if got := getDOColumnValue(t, value.DBData(), field); got != gdb.Raw("NULL") {
			t.Fatalf("DBData %s = %#v", field, got)
		}
	}
}

func TestDOValueRejectsInvalidNullAndTypes(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	wrongRemark := "wrong"
	var wrongNull *int
	var nullBytes []byte
	tests := []struct {
		name  string
		field string
		value any
	}{
		{name: "unknown field", field: "missing", value: "value"},
		{name: "non nullable nil", field: "title", value: nil},
		{name: "non nullable typed nil", field: "payload", value: nullBytes},
		{name: "unrelated typed nil", field: "remark", value: wrongNull},
		{name: "integer width", field: "count", value: int(0)},
		{name: "non nil pointer", field: "remark", value: &wrongRemark},
		{name: "raw expression", field: "remark", value: gdb.Raw("secret raw")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := descriptor.NewDO()
			if err := value.SetColumn(test.field, test.value); err == nil {
				t.Fatalf("SetColumn(%s, %T) error = nil", test.field, test.value)
			}
		})
	}
}

func TestDOValueFailedSetKeepsPreviousState(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()
	if err := value.SetColumn("title", "before"); err != nil {
		t.Fatal(err)
	}
	if err := value.SetColumn("title", 1); err == nil {
		t.Fatal("wrong type error = nil")
	}
	if err := value.SetColumn("title", nil); err == nil {
		t.Fatal("non nullable nil error = nil")
	}

	if !value.Has("title") || value.IsNull("title") {
		t.Fatalf("title state = has:%v null:%v", value.Has("title"), value.IsNull("title"))
	}
	if got := getDOColumnValue(t, value.DBData(), "title"); got != "before" {
		t.Fatalf("title = %#v", got)
	}
}

func TestDOValuePreservesExactGoTypes(t *testing.T) {
	descriptor, err := Compile[doExactTypesEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	standardTime := time.Date(2026, time.August, 4, 12, 30, 45, 123, time.UTC)
	frameworkTime := *gtime.New(standardTime)
	tests := []struct {
		field string
		value any
	}{
		{field: "boolValue", value: true},
		{field: "intValue", value: int(-1)},
		{field: "int8Value", value: int8(-8)},
		{field: "int16Value", value: int16(-16)},
		{field: "int32Value", value: int32(-32)},
		{field: "int64Value", value: int64(-64)},
		{field: "uintValue", value: uint(1)},
		{field: "uint8Value", value: uint8(8)},
		{field: "uint16Value", value: uint16(16)},
		{field: "uint32Value", value: uint32(32)},
		{field: "uint64Value", value: uint64(math.MaxUint64)},
		{field: "float32Value", value: float32(3.25)},
		{field: "float64Value", value: float64(6.5)},
		{field: "stringValue", value: "value"},
		{field: "bytesValue", value: []byte{0, 1, 255}},
		{field: "timeValue", value: standardTime},
		{field: "gtimeValue", value: frameworkTime},
		{field: "namedValue", value: doNamedString("named")},
	}

	value := descriptor.NewDO()
	for _, test := range tests {
		if err := value.SetColumn(test.field, test.value); err != nil {
			t.Fatalf("SetColumn(%s) error = %v", test.field, err)
		}
		got := getDOColumnValue(t, value.DBData(), test.field)
		if reflect.TypeOf(got) != reflect.TypeOf(test.value) || !reflect.DeepEqual(got, test.value) {
			t.Fatalf("DBData %s = %#v (%T), want %#v (%T)", test.field, got, got, test.value, test.value)
		}
	}

	if err := descriptor.NewDO().SetColumn("namedValue", "named"); err == nil {
		t.Fatal("plain string for named field error = nil")
	}
	if err := descriptor.NewDO().SetColumn("timeValue", frameworkTime); err == nil {
		t.Fatal("gtime.Time for time.Time field error = nil")
	}
	if err := descriptor.NewDO().SetColumn("gtimeValue", standardTime); err == nil {
		t.Fatal("time.Time for gtime.Time field error = nil")
	}
}

func TestDOValueLastSuccessfulSetWins(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()
	if err := value.SetColumn("remark", "first"); err != nil {
		t.Fatal(err)
	}
	if err := value.SetColumn("remark", nil); err != nil {
		t.Fatal(err)
	}
	if !value.IsNull("remark") {
		t.Fatal("remark must be null after second set")
	}
	if err := value.SetColumn("remark", "last"); err != nil {
		t.Fatal(err)
	}
	if !value.Has("remark") || value.IsNull("remark") {
		t.Fatalf("remark state = has:%v null:%v", value.Has("remark"), value.IsNull("remark"))
	}
	if got := getDOColumnValue(t, value.DBData(), "remark"); got != "last" {
		t.Fatalf("remark = %#v", got)
	}
}

func TestDescriptorNewDOValuesAreIsolated(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	first := descriptor.NewDO()
	second := descriptor.NewDO()
	if err := first.SetColumn("title", "first"); err != nil {
		t.Fatal(err)
	}
	if err := second.SetColumn("enabled", true); err != nil {
		t.Fatal(err)
	}

	if !first.Has("title") || first.Has("enabled") || second.Has("title") || !second.Has("enabled") {
		t.Fatal("NewDO values share field state")
	}
	if getDOColumnValue(t, first.DBData(), "enabled") != nil ||
		getDOColumnValue(t, second.DBData(), "title") != nil {
		t.Fatal("NewDO values share DBData")
	}
}

func TestDescriptorNewDORejectsTransientFields(t *testing.T) {
	descriptor, err := Compile[transientFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	if err = descriptor.NewDO().SetColumn("roleIdList", []uint64{1, 2}); err == nil {
		t.Fatal("SetColumn(roleIdList) error = nil")
	}
}

func TestDOValueDBDataReturnsSnapshot(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()
	if err := value.SetColumn("title", "first"); err != nil {
		t.Fatal(err)
	}
	first := value.DBData()
	if err := value.SetColumn("title", "second"); err != nil {
		t.Fatal(err)
	}
	second := value.DBData()

	if got := getDOColumnValue(t, first, "title"); got != "first" {
		t.Fatalf("first snapshot title = %#v", got)
	}
	if got := getDOColumnValue(t, second, "title"); got != "second" {
		t.Fatalf("second snapshot title = %#v", got)
	}
}

func TestDOValueErrorsAreCoreExceptionsAndHideValues(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	secret := "do-value-secret"
	tests := []error{
		descriptor.NewDO().SetColumn("missing", secret),
		descriptor.NewDO().SetColumn("count", secret),
		descriptor.NewDO().SetColumn("title", nil),
		descriptor.NewDO().SetColumn("remark", gdb.Raw(secret)),
	}
	for _, err := range tests {
		var coreException *exception.BaseException
		if !errors.As(err, &coreException) || coreException.Code != exception.CoreFail {
			t.Fatalf("error = %v, want CoreFail", err)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks value: %v", err)
		}
	}
}

// 按 orm 标签读取 DO 字段值
func getDOColumnValue(t *testing.T, data any, column string) any {
	t.Helper()
	value := reflect.ValueOf(data)
	typ := value.Type()
	for index := 1; index < typ.NumField(); index++ {
		if typ.Field(index).Tag.Get("orm") == column {
			return value.Field(index).Interface()
		}
	}
	t.Fatalf("DBData column %s not found", column)

	return nil
}

func TestDOValuePreservesSubmittedZeroValues(t *testing.T) {
	descriptor, err := Compile[doValueEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}
	value := descriptor.NewDO()

	tests := []struct {
		field string
		value any
	}{
		{field: "count", value: int64(0)},
		{field: "enabled", value: false},
		{field: "title", value: ""},
		{field: "payload", value: []byte{}},
	}
	for _, test := range tests {
		if err := value.SetColumn(test.field, test.value); err != nil {
			t.Fatalf("SetColumn(%s) error = %v", test.field, err)
		}
		if !value.Has(test.field) || value.IsNull(test.field) {
			t.Fatalf(
				"state %s = has:%v null:%v",
				test.field,
				value.Has(test.field),
				value.IsNull(test.field),
			)
		}
	}
	if value.Has("remark") || value.IsNull("remark") {
		t.Fatal("unsubmitted remark must stay absent")
	}
	if value.Has("missing") || value.IsNull("missing") {
		t.Fatal("unknown field must stay absent")
	}
}
