package gnentity

import (
	"reflect"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gmeta"
)

type doGoodsEntity struct {
	g.Meta `orm:"table:do_goods" description:"DO 商品"`
	Base
	Title   string    `json:"title" orm:"title" description:"标题"`
	Remark  *string   `json:"remark" orm:"remark" description:"备注"`
	Enabled bool      `json:"enabled" orm:"enabled" description:"是否启用"`
	RoleIDs *[]uint64 `json:"roleIds" description:"角色 ID" cool:"transient"`
}

func TestDescriptorNewDOCreatesGoFrameStruct(t *testing.T) {
	descriptor, err := Compile[doGoodsEntity, uint64](Schema{})
	if err != nil {
		t.Fatal(err)
	}

	data := descriptor.NewDO().DBData()
	typ := reflect.TypeOf(data)
	if typ.Kind() != reflect.Struct {
		t.Fatalf("DBData type = %s, want struct", typ)
	}
	meta := typ.Field(0)
	if !meta.Anonymous || meta.Type != reflect.TypeFor[g.Meta]() {
		t.Fatalf("meta field = %#v", meta)
	}
	if got := meta.Tag.Get("orm"); got != "table:do_goods,do:true" {
		t.Fatalf("meta orm = %q", got)
	}
	if got := gmeta.Get(data, "orm").String(); got != "table:do_goods,do:true" {
		t.Fatalf("gmeta orm = %q", got)
	}

	columns := []string{"id", "createTime", "updateTime", "title", "remark", "enabled"}
	if typ.NumField() != len(columns)+1 {
		t.Fatalf("DBData fields = %d, want %d", typ.NumField(), len(columns)+1)
	}
	for index, column := range columns {
		field := typ.Field(index + 1)
		if field.Type != reflect.TypeFor[any]() || field.Tag.Get("orm") != column {
			t.Fatalf("DO field %d = %s/%q", index, field.Type, field.Tag.Get("orm"))
		}
	}
}
