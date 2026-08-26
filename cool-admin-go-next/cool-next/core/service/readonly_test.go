package service

import (
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
)

type readonlyEntity struct {
	g.Meta `orm:"table:readonly_entity" description:"只读字段实体"`
	entity.Base
	Name string `json:"name" orm:"name" description:"名称"`
}

func readonlyBase(t *testing.T) *Base[readonlyEntity, uint64] {
	t.Helper()

	descriptor, err := entity.Compile[readonlyEntity, uint64](entity.Schema{})
	if err != nil {
		t.Fatalf("编译实体 Descriptor 失败: %v", err)
	}

	return &Base[readonlyEntity, uint64]{descriptor: descriptor}
}

func readonlyMutable(t *testing.T, base *Base[readonlyEntity, uint64], fields []FieldValue) *Mutable[readonlyEntity] {
	t.Helper()

	value, err := NewMutable[readonlyEntity, uint64](base.descriptor, fields)
	if err != nil {
		t.Fatalf("构造可写字段集失败: %v", err)
	}

	return value
}

// 前端整行回传必然带上 createTime/updateTime，Add 与 Update 都应忽略而不是报错
func TestMutableDataIgnoresClientReadonlyFields(t *testing.T) {
	base := readonlyBase(t)
	moment := *gtime.Now()

	for _, action := range []crud.Action{crud.ActionAdd, crud.ActionUpdate} {
		t.Run(string(action), func(t *testing.T) {
			value := readonlyMutable(t, base, []FieldValue{
				Value("name", "任务"),
				Value("createTime", moment),
				Value("updateTime", moment),
			})
			_, count, err := base.mutableData(value, nil, action)
			if err != nil {
				t.Fatalf("构造写入数据失败: %v", err)
			}
			if count != 1 {
				t.Errorf("写入字段数 = %d，期望只有 name", count)
			}
		})
	}
}

// 业务代码显式写入只读字段仍然是错误，这条护栏不能被上面的放宽顺带拆掉
func TestMutableDataRejectsServerReadonlyUpdate(t *testing.T) {
	base := readonlyBase(t)
	value := readonlyMutable(t, base, []FieldValue{Value("name", "任务")})
	if err := value.Set("updateTime", *gtime.Now()); err != nil {
		t.Fatalf("设置只读字段失败: %v", err)
	}

	if _, _, err := base.mutableData(value, nil, crud.ActionUpdate); err == nil {
		t.Error("业务代码更新只读字段期望报错")
	}
}
