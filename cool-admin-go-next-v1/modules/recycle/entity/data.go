package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

// Data 返回回收批次模型定义。
func Data() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("entityInfo", "entityInfo", "json").NotNull().Comment("实体信息"),
		entity.NewField("userId", "userId", "bigint").Unsigned().Nullable().Comment("操作人"),
		entity.NewField("data", "data", "json").NotNull().Comment("被删除的数据"),
		entity.NewField("url", "url", "varchar").Size(255).Nullable().Comment("请求接口"),
		entity.NewField("params", "params", "json").Nullable().Comment("请求参数"),
		entity.NewField("count", "count", "int").Unsigned().NotNull().Default("1").Comment("删除数据条数"),
		entity.NewField("restoreStatus", "restoreStatus", "varchar").Size(20).NotNull().Default("'pending'").Comment("恢复状态"),
		entity.NewField("remainingCount", "remainingCount", "int").Unsigned().NotNull().Default("0").Comment("待恢复条数"),
	)

	return entity.NewDefinition("recycle", "RecycleData", "recycle_data").
		Comment("数据回收批次").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_recycle_data_user_id", "userId"),
			entity.NewIndex("idx_recycle_data_expired", "createTime", "id"),
			entity.NewIndex("idx_recycle_data_tenant_create", "tenantId", "createTime"),
			entity.NewIndex("idx_recycle_data_tenant_user", "tenantId", "userId"),
			entity.NewIndex("idx_recycle_data_restore_status", "restoreStatus"),
		)
}
