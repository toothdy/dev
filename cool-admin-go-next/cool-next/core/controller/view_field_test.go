package controller

import (
	"encoding/json"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

type viewFieldEntity struct {
	g.Meta `orm:"table:view_field_entity" description:"视图字段实体"`
	coreentity.Base
	Name  string `json:"name" orm:"name" description:"名称"`
	Every *int64 `json:"every" orm:"every" description:"间隔"`
}

func viewFieldDescriptor(t *testing.T) coreentity.Descriptor[viewFieldEntity, uint64] {
	t.Helper()

	descriptor, err := coreentity.Compile[viewFieldEntity, uint64](coreentity.Schema{})
	if err != nil {
		t.Fatalf("编译实体 Descriptor 失败: %v", err)
	}

	return descriptor
}

func rawBody(t *testing.T, body string) map[string]json.RawMessage {
	t.Helper()

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatalf("解析请求体失败: %v", err)
	}

	return raw
}

// cool-admin-vue 把派生字段挂到实体行上再整行回传，下划线开头的键按视图字段丢弃
func TestDecodeMutableDropsViewFields(t *testing.T) {
	descriptor := viewFieldDescriptor(t)

	mutable, err := decodeMutable[viewFieldEntity, uint64](
		rawBody(t, `{"name":"任务","every":5000,"_every":5}`),
		descriptor,
	)
	if err != nil {
		t.Fatalf("绑定实体失败: %v", err)
	}
	if mutable.Has("_every") {
		t.Error("视图字段不应进入可写字段集")
	}
	if !mutable.Has("name") || !mutable.Has("every") {
		t.Error("实体字段应正常绑定")
	}
}

// 放行的只是前端视图字段前缀，字段名拼错仍然要报错
func TestDecodeMutableStillRejectsUnknownFields(t *testing.T) {
	descriptor := viewFieldDescriptor(t)

	if _, err := decodeMutable[viewFieldEntity, uint64](
		rawBody(t, `{"nmae":"拼错了"}`),
		descriptor,
	); err == nil {
		t.Error("未知字段期望报错")
	}
}
