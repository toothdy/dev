package gnentity

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type baseOnlyEntity struct {
	g.Meta `orm:"table:base_only" description:"基础实体"`
	Base
}

func TestCompileReadsMetaAndBase(t *testing.T) {
	descriptor, err := Compile[baseOnlyEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if descriptor.Table() != "base_only" || descriptor.Description() != "基础实体" {
		t.Fatalf("metadata = %q/%q", descriptor.Table(), descriptor.Description())
	}
	if descriptor.EntityType() != reflect.TypeFor[baseOnlyEntity]() ||
		descriptor.IDType() != reflect.TypeFor[uint64]() {
		t.Fatalf("types = %s/%s", descriptor.EntityType(), descriptor.IDType())
	}

	primary := descriptor.Primary()
	if primary == nil || primary.Name() != "id" || primary.Column() != "id" ||
		!primary.Primary() || !primary.AutoIncrement() || primary.Nullable() || primary.SystemMaintained() {
		t.Fatalf("primary = %#v", primary)
	}
	fields := descriptor.Fields()
	if len(fields) != 3 || fields[1].Name() != "createTime" || fields[2].Name() != "updateTime" {
		t.Fatalf("fields = %#v", fields)
	}
	for _, field := range fields[1:] {
		if field.Nullable() || !field.SystemMaintained() || field.Primary() || field.AutoIncrement() {
			t.Fatalf("system field %s has invalid flags", field.Name())
		}
	}
}

type missingMetaEntity struct {
	Base
}

type missingBaseEntity struct {
	g.Meta `orm:"table:missing_base" description:"缺少基础字段"`
}

type extraEmbedding struct{}

type extraAnonymousEntity struct {
	g.Meta `orm:"table:extra_anonymous" description:"额外嵌入"`
	Base
	extraEmbedding
}

type unexportedEntity struct {
	g.Meta `orm:"table:unexported" description:"未导出字段"`
	Base
	hidden string
}

type missingTableEntity struct {
	g.Meta `description:"缺少表名"`
	Base
}

type emptyTableEntity struct {
	g.Meta `orm:"table:" description:"空表名"`
	Base
}

type invalidTableEntity struct {
	g.Meta `orm:"table:Demo_Goods" description:"非法表名"`
	Base
}

type unknownMetaDirectiveEntity struct {
	g.Meta `orm:"table:unknown_directive,do:true" description:"未知指令"`
	Base
}

type duplicateTableDirectiveEntity struct {
	g.Meta `orm:"table:first,table:second" description:"重复指令"`
	Base
}

type emptyTableDescriptionEntity struct {
	g.Meta `orm:"table:empty_description" description:"  "`
	Base
}

func TestCompileRejectsInvalidEntityShape(t *testing.T) {
	tests := []struct {
		name    string
		compile func() error
	}{
		{
			name: "pointer root",
			compile: func() error {
				_, err := Compile[*baseOnlyEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "missing meta",
			compile: func() error {
				_, err := Compile[missingMetaEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "missing base",
			compile: func() error {
				_, err := Compile[missingBaseEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "extra anonymous field",
			compile: func() error {
				_, err := Compile[extraAnonymousEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "unexported field",
			compile: func() error {
				_, err := Compile[unexportedEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "wrong id type",
			compile: func() error {
				_, err := Compile[baseOnlyEntity, string](Schema{})
				return err
			},
		},
		{
			name: "missing table",
			compile: func() error {
				_, err := Compile[missingTableEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "empty table",
			compile: func() error {
				_, err := Compile[emptyTableEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "invalid table",
			compile: func() error {
				_, err := Compile[invalidTableEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "unknown meta directive",
			compile: func() error {
				_, err := Compile[unknownMetaDirectiveEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "duplicate table directive",
			compile: func() error {
				_, err := Compile[duplicateTableDirectiveEntity, uint64](Schema{})
				return err
			},
		},
		{
			name: "empty description",
			compile: func() error {
				_, err := Compile[emptyTableDescriptionEntity, uint64](Schema{})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.compile(); err == nil {
				t.Fatal("Compile() error = nil")
			}
		})
	}
}

type namedString string

type supportedFieldsEntity struct {
	g.Meta `orm:"table:supported_fields" description:"支持字段"`
	Base
	Bool     bool        `json:"bool" orm:"bool" description:"布尔"`
	Int      int         `json:"int" orm:"int" description:"整数"`
	Int8     int8        `json:"int8" orm:"int8" description:"八位整数"`
	Int16    int16       `json:"int16" orm:"int16" description:"十六位整数"`
	Int32    int32       `json:"int32" orm:"int32" description:"三十二位整数"`
	Int64    int64       `json:"int64" orm:"int64" description:"六十四位整数"`
	Uint     uint        `json:"uint" orm:"uint" description:"无符号整数"`
	Uint8    uint8       `json:"uint8" orm:"uint8" description:"八位无符号整数"`
	Uint16   uint16      `json:"uint16" orm:"uint16" description:"十六位无符号整数"`
	Uint32   uint32      `json:"uint32" orm:"uint32" description:"三十二位无符号整数"`
	Uint64   uint64      `json:"uint64" orm:"uint64" description:"六十四位无符号整数"`
	Float32  float32     `json:"float32" orm:"float32" description:"单精度"`
	Float64  float64     `json:"float64" orm:"float64" description:"双精度"`
	String   string      `json:"string" orm:"string" description:"字符串"`
	Bytes    []byte      `json:"bytes" orm:"bytes" description:"字节"`
	StdTime  time.Time   `json:"stdTime" orm:"stdTime" description:"标准时间"`
	GFTime   gtime.Time  `json:"gfTime" orm:"gfTime" description:"框架时间"`
	Named    namedString `json:"named" orm:"named" description:"命名字符串"`
	Optional *string     `json:"optional" orm:"optional" description:"可空字符串"`
}

func TestCompileInfersFieldMetadata(t *testing.T) {
	descriptor, err := Compile[supportedFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	tests := []struct {
		name       string
		logical    LogicalType
		goType     reflect.Type
		isNullable bool
	}{
		{name: "bool", logical: LogicalBool, goType: reflect.TypeFor[bool]()},
		{name: "int", logical: LogicalInt, goType: reflect.TypeFor[int]()},
		{name: "int8", logical: LogicalInt, goType: reflect.TypeFor[int8]()},
		{name: "int16", logical: LogicalInt, goType: reflect.TypeFor[int16]()},
		{name: "int32", logical: LogicalInt, goType: reflect.TypeFor[int32]()},
		{name: "int64", logical: LogicalInt, goType: reflect.TypeFor[int64]()},
		{name: "uint", logical: LogicalUint, goType: reflect.TypeFor[uint]()},
		{name: "uint8", logical: LogicalUint, goType: reflect.TypeFor[uint8]()},
		{name: "uint16", logical: LogicalUint, goType: reflect.TypeFor[uint16]()},
		{name: "uint32", logical: LogicalUint, goType: reflect.TypeFor[uint32]()},
		{name: "uint64", logical: LogicalUint, goType: reflect.TypeFor[uint64]()},
		{name: "float32", logical: LogicalFloat, goType: reflect.TypeFor[float32]()},
		{name: "float64", logical: LogicalFloat, goType: reflect.TypeFor[float64]()},
		{name: "string", logical: LogicalString, goType: reflect.TypeFor[string]()},
		{name: "bytes", logical: LogicalBytes, goType: reflect.TypeFor[[]byte]()},
		{name: "stdTime", logical: LogicalTime, goType: reflect.TypeFor[time.Time]()},
		{name: "gfTime", logical: LogicalTime, goType: reflect.TypeFor[gtime.Time]()},
		{name: "named", logical: LogicalString, goType: reflect.TypeFor[namedString]()},
		{name: "optional", logical: LogicalString, goType: reflect.TypeFor[*string](), isNullable: true},
	}

	for _, test := range tests {
		field, exists := descriptor.Field(test.name)
		if !exists {
			t.Fatalf("Field(%q) not found", test.name)
		}
		if field.LogicalType() != test.logical || field.GoType() != test.goType ||
			field.Nullable() != test.isNullable {
			t.Fatalf("Field(%q) = %s/%s nullable=%v", test.name, field.LogicalType(), field.GoType(), field.Nullable())
		}
	}
}

type constrainedFieldsEntity struct {
	g.Meta `orm:"table:constrained_fields" description:"约束字段"`
	Base
	Name    string  `json:"name" orm:"name" description:"名称" cool:"size=50"`
	Payload []byte  `json:"payload" orm:"payload" description:"内容" cool:"size=1024"`
	Price   float64 `json:"price" orm:"price" description:"价格" cool:"precision=10,scale=2,default=0"`
}

func TestCompileParsesFieldConstraints(t *testing.T) {
	descriptor, err := Compile[constrainedFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	name, exists := descriptor.Field("name")
	if !exists {
		t.Fatal("Field(name) not found")
	}
	if constraints := name.Constraints(); !constraints.HasSize || constraints.Size != 50 {
		t.Fatalf("Name constraints = %#v", constraints)
	}
	payload, exists := descriptor.Field("payload")
	if !exists {
		t.Fatal("Field(payload) not found")
	}
	if constraints := payload.Constraints(); !constraints.HasSize || constraints.Size != 1024 {
		t.Fatalf("Payload constraints = %#v", constraints)
	}
	price, exists := descriptor.Field("price")
	if !exists {
		t.Fatal("Field(price) not found")
	}
	constraints := price.Constraints()
	if !constraints.HasPrecision || constraints.Precision != 10 ||
		!constraints.HasScale || constraints.Scale != 2 ||
		!constraints.HasDefault || constraints.Default != "0" {
		t.Fatalf("Price constraints = %#v", constraints)
	}
}

type missingJSONEntity struct {
	g.Meta `orm:"table:missing_json" description:"缺少 JSON"`
	Base
	Value string `orm:"value" description:"值"`
}

type ignoredJSONEntity struct {
	g.Meta `orm:"table:ignored_json" description:"忽略 JSON"`
	Base
	Value string `json:"-" orm:"value" description:"值"`
}

type jsonOptionEntity struct {
	g.Meta `orm:"table:json_option" description:"JSON 选项"`
	Base
	Value string `json:"value,omitempty" orm:"value" description:"值"`
}

type missingORMEntity struct {
	g.Meta `orm:"table:missing_orm" description:"缺少 ORM"`
	Base
	Value string `json:"value" description:"值"`
}

type snakeColumnEntity struct {
	g.Meta `orm:"table:snake_column" description:"蛇形列"`
	Base
	Value string `json:"value" orm:"field_value" description:"值"`
}

type emptyFieldDescriptionEntity struct {
	g.Meta `orm:"table:empty_field_description" description:"空字段描述"`
	Base
	Value string `json:"value" orm:"value" description:" "`
}

type duplicateJSONEntity struct {
	g.Meta `orm:"table:duplicate_json" description:"重复 JSON"`
	Base
	Value string `json:"id" orm:"otherId" description:"重复 ID"`
}

type duplicateColumnEntity struct {
	g.Meta `orm:"table:duplicate_column" description:"重复列"`
	Base
	Value string `json:"otherId" orm:"id" description:"重复 ID"`
}

type doublePointerEntity struct {
	g.Meta `orm:"table:double_pointer" description:"双重指针"`
	Base
	Value **string `json:"value" orm:"value" description:"值"`
}

type unsupportedMapEntity struct {
	g.Meta `orm:"table:unsupported_map" description:"Map 字段"`
	Base
	Value map[string]string `json:"value" orm:"value" description:"值"`
}

type unsupportedSliceEntity struct {
	g.Meta `orm:"table:unsupported_slice" description:"Slice 字段"`
	Base
	Value []uint64 `json:"value" orm:"value" description:"值"`
}

type jsonFieldsEntity struct {
	g.Meta `orm:"table:json_fields" description:"JSON 字段"`
	Base
	RoleIDs []uint64          `json:"roleIds" orm:"roleIds" description:"角色" cool:"json=true"`
	Params  *map[string]any   `json:"params" orm:"params" description:"参数" cool:"json=true"`
	Labels  map[string]string `json:"labels" orm:"labels" description:"标签" cool:"json=true"`
}

type jsonScalarEntity struct {
	g.Meta `orm:"table:json_scalar" description:"JSON 标量"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"json=true"`
}

type jsonBytesEntity struct {
	g.Meta `orm:"table:json_bytes" description:"JSON 字节"`
	Base
	Value []byte `json:"value" orm:"value" description:"值" cool:"json=true"`
}

type invalidJSONMarkerEntity struct {
	g.Meta `orm:"table:invalid_json_marker" description:"无效 JSON 标记"`
	Base
	Value []uint64 `json:"value" orm:"value" description:"值" cool:"json=false"`
}

type transientFieldsEntity struct {
	g.Meta `orm:"table:transient_fields" description:"临时字段"`
	Base
	Name       string    `json:"name" orm:"name" description:"名称"`
	RoleIDList *[]uint64 `json:"roleIdList" description:"角色 ID 列表" cool:"transient"`
}

type transientORMEntity struct {
	g.Meta `orm:"table:transient_orm" description:"临时字段 ORM"`
	Base
	RoleIDList *[]uint64 `json:"roleIdList" orm:"roleIdList" description:"角色 ID 列表" cool:"transient"`
}

type transientMapEntity struct {
	g.Meta `orm:"table:transient_map" description:"临时 Map 字段"`
	Base
	Value map[string]string `json:"value" description:"值" cool:"transient"`
}

type invalidTransientMarkerEntity struct {
	g.Meta `orm:"table:invalid_transient_marker" description:"无效临时标记"`
	Base
	Value []uint64 `json:"value" description:"值" cool:"transient=true"`
}

func TestCompileSupportsExplicitJSONFields(t *testing.T) {
	descriptor, err := Compile[jsonFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	tests := []struct {
		name       string
		goType     reflect.Type
		isNullable bool
	}{
		{name: "roleIds", goType: reflect.TypeFor[[]uint64]()},
		{name: "params", goType: reflect.TypeFor[*map[string]any](), isNullable: true},
		{name: "labels", goType: reflect.TypeFor[map[string]string]()},
	}
	for _, test := range tests {
		field, exists := descriptor.Field(test.name)
		if !exists {
			t.Fatalf("Field(%q) not found", test.name)
		}
		if field.LogicalType() != LogicalJSON || field.GoType() != test.goType || field.Nullable() != test.isNullable {
			t.Fatalf("Field(%q) = %s/%s nullable=%v", test.name, field.LogicalType(), field.GoType(), field.Nullable())
		}
	}
}

func TestCompileSeparatesTransientAndPersistentFields(t *testing.T) {
	descriptor, err := Compile[transientFieldsEntity, uint64](Schema{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	field, exists := descriptor.Field("roleIdList")
	if !exists || field.Persistent() || field.Column() != "" || field.LogicalType() != LogicalJSON ||
		field.GoType() != reflect.TypeFor[*[]uint64]() || !field.Nullable() {
		t.Fatalf("Field(roleIdList) = %#v", field)
	}
	if byJSON, jsonExists := descriptor.JSON("roleIdList"); !jsonExists || byJSON != field {
		t.Fatalf("JSON(roleIdList) = %#v/%v", byJSON, jsonExists)
	}
	if _, columnExists := descriptor.Column("roleIdList"); columnExists {
		t.Fatal("Column(roleIdList) exists")
	}
	if got := descriptor.Fields(); len(got) != 5 || got[4] != field {
		t.Fatalf("Fields() = %#v", got)
	}
	persistent := descriptor.PersistentFields()
	if len(persistent) != 4 || persistent[3].Name() != "name" {
		t.Fatalf("PersistentFields() = %#v", persistent)
	}
}

func TestCompileRejectsInvalidBusinessFields(t *testing.T) {
	tests := []struct {
		name    string
		want    string
		compile func() error
	}{
		{name: "missing json", want: "字段 Value 缺少 json", compile: func() error { _, err := Compile[missingJSONEntity, uint64](Schema{}); return err }},
		{name: "ignored json", want: "字段 Value 的 json", compile: func() error { _, err := Compile[ignoredJSONEntity, uint64](Schema{}); return err }},
		{name: "json option", want: "字段 Value 的 json", compile: func() error { _, err := Compile[jsonOptionEntity, uint64](Schema{}); return err }},
		{name: "missing orm", want: "字段 Value 缺少 orm", compile: func() error { _, err := Compile[missingORMEntity, uint64](Schema{}); return err }},
		{name: "snake column", want: "列名", compile: func() error { _, err := Compile[snakeColumnEntity, uint64](Schema{}); return err }},
		{name: "empty description", want: "字段 Value 的 description", compile: func() error { _, err := Compile[emptyFieldDescriptionEntity, uint64](Schema{}); return err }},
		{name: "duplicate json", want: "重复 json 名", compile: func() error { _, err := Compile[duplicateJSONEntity, uint64](Schema{}); return err }},
		{name: "duplicate column", want: "重复列名", compile: func() error { _, err := Compile[duplicateColumnEntity, uint64](Schema{}); return err }},
		{name: "double pointer", want: "字段 Value 的类型", compile: func() error { _, err := Compile[doublePointerEntity, uint64](Schema{}); return err }},
		{name: "unsupported map", want: "字段 Value 的类型", compile: func() error { _, err := Compile[unsupportedMapEntity, uint64](Schema{}); return err }},
		{name: "unsupported slice", want: "字段 Value 的类型", compile: func() error { _, err := Compile[unsupportedSliceEntity, uint64](Schema{}); return err }},
		{name: "json scalar", want: "cool 标签", compile: func() error { _, err := Compile[jsonScalarEntity, uint64](Schema{}); return err }},
		{name: "json bytes", want: "cool 标签", compile: func() error { _, err := Compile[jsonBytesEntity, uint64](Schema{}); return err }},
		{name: "invalid json marker", want: "字段 Value", compile: func() error { _, err := Compile[invalidJSONMarkerEntity, uint64](Schema{}); return err }},
		{name: "transient orm", want: "transient 字段不能声明 orm", compile: func() error { _, err := Compile[transientORMEntity, uint64](Schema{}); return err }},
		{name: "transient map", want: "字段 Value 的类型", compile: func() error { _, err := Compile[transientMapEntity, uint64](Schema{}); return err }},
		{name: "invalid transient marker", want: "cool 标签", compile: func() error { _, err := Compile[invalidTransientMarkerEntity, uint64](Schema{}); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.compile()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("Compile() error = %v, want %q", err, test.want)
			}
		})
	}
}

type emptyCoolItemEntity struct {
	g.Meta `orm:"table:empty_cool_item" description:"空约束项"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"size=10,"`
}

type duplicateCoolKeyEntity struct {
	g.Meta `orm:"table:duplicate_cool_key" description:"重复约束"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"size=10,size=20"`
}

type unknownCoolKeyEntity struct {
	g.Meta `orm:"table:unknown_cool_key" description:"未知约束"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"nullable=true"`
}

type invalidCoolNumberEntity struct {
	g.Meta `orm:"table:invalid_cool_number" description:"非法数字"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"size=no"`
}

type invalidCoolTargetEntity struct {
	g.Meta `orm:"table:invalid_cool_target" description:"非法目标"`
	Base
	Value int `json:"value" orm:"value" description:"值" cool:"size=10"`
}

type missingPrecisionEntity struct {
	g.Meta `orm:"table:missing_precision" description:"缺少精度"`
	Base
	Value float64 `json:"value" orm:"value" description:"值" cool:"scale=2"`
}

type excessiveScaleEntity struct {
	g.Meta `orm:"table:excessive_scale" description:"过大标度"`
	Base
	Value float64 `json:"value" orm:"value" description:"值" cool:"precision=2,scale=3"`
}

type emptyDefaultEntity struct {
	g.Meta `orm:"table:empty_default" description:"空默认值"`
	Base
	Value string `json:"value" orm:"value" description:"值" cool:"default="`
}

func TestCompileRejectsInvalidCoolConstraints(t *testing.T) {
	tests := []struct {
		name    string
		compile func() error
	}{
		{name: "empty item", compile: func() error { _, err := Compile[emptyCoolItemEntity, uint64](Schema{}); return err }},
		{name: "duplicate key", compile: func() error { _, err := Compile[duplicateCoolKeyEntity, uint64](Schema{}); return err }},
		{name: "unknown key", compile: func() error { _, err := Compile[unknownCoolKeyEntity, uint64](Schema{}); return err }},
		{name: "invalid number", compile: func() error { _, err := Compile[invalidCoolNumberEntity, uint64](Schema{}); return err }},
		{name: "invalid target", compile: func() error { _, err := Compile[invalidCoolTargetEntity, uint64](Schema{}); return err }},
		{name: "missing precision", compile: func() error { _, err := Compile[missingPrecisionEntity, uint64](Schema{}); return err }},
		{name: "excessive scale", compile: func() error { _, err := Compile[excessiveScaleEntity, uint64](Schema{}); return err }},
		{name: "empty default", compile: func() error { _, err := Compile[emptyDefaultEntity, uint64](Schema{}); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.compile()
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cool 标签") {
				t.Fatalf("Compile() error = %v, want cool constraint error", err)
			}
		})
	}
}

func TestCompilePreservesConstraintParseCause(t *testing.T) {
	_, err := Compile[invalidCoolNumberEntity, uint64](Schema{})
	var numberError *strconv.NumError
	if !errors.As(err, &numberError) {
		t.Fatalf("Compile() error = %v, want strconv.NumError cause", err)
	}
}
