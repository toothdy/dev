package service

import (
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

/**
 * 创建字典信息回收测试记录
 * @param id 记录 ID
 * @param typeID 类型 ID
 * @param parentID 父记录 ID
 * @returns gdb.Record
 */
func dictRecycleRecord(id int64, typeID int64, parentID interface{}) gdb.Record {
	return gdb.Record{
		"id":        gvar.New(id),
		"typeId":   gvar.New(typeID),
		"parentId": gvar.New(parentID),
	}
}

/**
 * 验证字典类型关联项按真实父链排序
 * @param t 测试上下文
 * @returns null
 */
func TestOrderDictTypeInfoNodesParentsBeforeChildren(t *testing.T) {
	rows := gdb.Result{
		dictRecycleRecord(3, 11, int64(2)),
		dictRecycleRecord(1, 11, nil),
		dictRecycleRecord(2, 11, int64(1)),
	}
	ordered, err := orderDictTypeInfoNodes(rows)
	if err != nil {
		t.Fatalf("排序字典信息父链失败: %v", err)
	}
	if len(ordered) != 3 || ordered[0].id != 1 || ordered[1].id != 2 || ordered[2].id != 3 {
		t.Fatalf("字典信息父链顺序异常: %#v", ordered)
	}
}

/**
 * 验证字典类型关联项拒绝环和跨类型父级
 * @param t 测试上下文
 * @returns null
 */
func TestOrderDictTypeInfoNodesRejectsInvalidParents(t *testing.T) {
	cases := []gdb.Result{
		{
			dictRecycleRecord(1, 11, int64(2)),
			dictRecycleRecord(2, 11, int64(1)),
		},
		{
			dictRecycleRecord(1, 11, nil),
			dictRecycleRecord(2, 12, int64(1)),
		},
	}
	for _, rows := range cases {
		if _, err := orderDictTypeInfoNodes(rows); err == nil {
			t.Fatalf("非法字典信息父链应被拒绝: %#v", rows)
		}
	}
}
