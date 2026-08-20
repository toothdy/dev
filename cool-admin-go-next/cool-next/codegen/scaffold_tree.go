package codegen

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
	"github.com/toothdy/cool-admin-go-next/cool-next/seed"
)

// BuildTree 把已按展示顺序排好的扁平行组装为父子嵌套的树，用于"导出一张树形
// 表为 JSON"这类开发工具。idOf/parentOf 从一行里取出主键和父键；build 把一行
// 连同它已组装好的子节点列表转换为调用方自己的树节点类型 T——具体业务字段
// （比如菜单的 router/perms）由调用方决定，本函数只负责父子嵌套本身。
// 出现环时截断该分支，不重复展开，避免无限递归。
func BuildTree[T, R any](
	rows []R,
	idOf func(R) uint64,
	parentOf func(R) *uint64,
	build func(row R, children []T) T,
) []T {
	children := make(map[uint64][]R)
	var roots []R
	for _, row := range rows {
		if parentID := parentOf(row); parentID != nil {
			children[*parentID] = append(children[*parentID], row)
		} else {
			roots = append(roots, row)
		}
	}
	var walk func(row R, ancestors map[uint64]bool) T
	walk = func(row R, ancestors map[uint64]bool) T {
		id := idOf(row)
		if ancestors[id] {
			return build(row, nil)
		}
		ancestors[id] = true
		nested := children[id]
		built := make([]T, 0, len(nested))
		for _, child := range nested {
			built = append(built, walk(child, ancestors))
		}
		delete(ancestors, id)

		return build(row, built)
	}
	result := make([]T, 0, len(roots))
	for _, root := range roots {
		result = append(result, walk(root, make(map[uint64]bool)))
	}

	return result
}

// InsertTree 把节点树按父子顺序逐个插入 model，每层用上一层实际分配的新 ID
// 重新关联，不做业务键去重——每次调用都产生全新记录。用于"把导出的 JSON 树
// 重新导入"这类总是新建的开发工具场景；需要按业务键幂等补齐的启动期种子数据
// 用 cool-next/seed.SyncTree，语义不同不通用。
//
// values 从一个节点取出待写入字段（不含 parentId，由本函数按层级注入）；
// children 取出该节点的子节点列表。
func InsertTree[T any](
	ctx context.Context,
	model *gdb.Model,
	descriptor coreentity.RuntimeDescriptor,
	nodes []T,
	parentID *uint64,
	values func(T) map[string]any,
	children func(T) []T,
) error {
	for _, node := range nodes {
		fields := values(node)
		if fields == nil {
			fields = map[string]any{}
		}
		if parentID == nil {
			fields["parentId"] = nil
		} else {
			fields["parentId"] = *parentID
		}
		do, err := seed.NewDO(descriptor, fields, true)
		if err != nil {
			return exception.WrapCore(err, "构造导入节点失败")
		}
		insertedID, err := model.Data(do.DBData()).InsertAndGetId()
		if err != nil {
			return exception.WrapCore(err, "导入节点失败")
		}
		if insertedID <= 0 {
			return exception.Core("导入节点未返回有效 ID")
		}
		id := uint64(insertedID)
		if err = InsertTree(ctx, model, descriptor, children(node), &id, values, children); err != nil {
			return err
		}
	}

	return nil
}
