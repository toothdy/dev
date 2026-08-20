package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

// TaskLog 返回任务日志表元数据
func TaskLog() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("taskId", "taskId", "bigint").Unsigned().NotNull().Comment("任务ID"),
		entity.NewField("status", "status", "tinyint").NotNull().Default("0").Comment("状态 0失败 1成功"),
		entity.NewField("detail", "detail", "text").Nullable().Comment("详情描述"),
	)

	return entity.NewDefinition("task", "TaskLogEntity", "task_log").
		WithResource("task.log").
		Comment("任务日志").
		Fields(fields).
		WithTenantMode(entity.TenantModeRequired).
		WithIndexes(
			entity.NewIndex("idx_task_log_tenant_task_time", "tenantId", "taskId", "createTime"),
		)
}
