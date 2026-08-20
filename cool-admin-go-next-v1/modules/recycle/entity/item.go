package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

// Item 返回回收数据项模型定义。
func Item() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("recycleId", "recycleId", "bigint").Unsigned().NotNull().Comment("回收批次ID"),
		entity.NewField("resource", "resource", "varchar").Size(120).NotNull().Comment("资源名称"),
		entity.NewField("tableName", "tableName", "varchar").Size(120).NotNull().Comment("目标表名"),
		entity.NewField("primaryKey", "primaryKey", "json").NotNull().Comment("原主键"),
		entity.NewField("data", "data", "json").NotNull().Comment("完整数据快照"),
		entity.NewField("branchKey", "branchKey", "varchar").Size(120).NotNull().Comment("依赖分支"),
		entity.NewField("parentItemId", "parentItemId", "bigint").Unsigned().Nullable().Comment("父归档项ID"),
		entity.NewField("restoreOrder", "restoreOrder", "int").NotNull().Default("0").Comment("恢复顺序"),
		entity.NewField("status", "status", "varchar").Size(20).NotNull().Default("'pending'").Comment("恢复状态"),
		entity.NewField("error", "error", "text").Nullable().Comment("最近恢复错误"),
	)

	return entity.NewDefinition("recycle", "RecycleItem", "recycle_item").
		Comment("数据回收项").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_recycle_item_restore", "recycleId", "status", "restoreOrder"),
			entity.NewIndex("idx_recycle_item_branch", "recycleId", "branchKey"),
			entity.NewIndex("idx_recycle_item_tenant_recycle", "tenantId", "recycleId"),
		)
}
