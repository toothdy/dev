package entity

import "github.com/toothdy/cool-admin-go-next/cool/entity"

// TaskInfo 返回任务定义表元数据
func TaskInfo() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("jobId", "jobId", "varchar").Size(64).Nullable().Comment("调度ID"),
		entity.NewField("repeatConf", "repeatConf", "text").Nullable().Comment("重复配置"),
		entity.NewField("name", "name", "varchar").Size(255).NotNull().Comment("名称"),
		entity.NewField("cron", "cron", "varchar").Size(255).Nullable().Comment("Cron表达式"),
		entity.NewField("limit", "limit", "int").Unsigned().Nullable().Comment("最大执行次数"),
		entity.NewField("every", "every", "bigint").Unsigned().Nullable().Comment("执行间隔毫秒"),
		entity.NewField("remark", "remark", "varchar").Size(255).Nullable().Comment("备注"),
		entity.NewField("status", "status", "tinyint").NotNull().Default("1").Comment("状态 0停止 1运行"),
		entity.NewField("startDate", "startDate", "datetime").Nullable().Comment("开始时间"),
		entity.NewField("endDate", "endDate", "datetime").Nullable().Comment("结束时间"),
		entity.NewField("data", "data", "text").Nullable().Comment("业务数据"),
		entity.NewField("service", "service", "varchar").Size(1000).NotNull().Comment("处理器表达式"),
		entity.NewField("type", "type", "tinyint").NotNull().Default("0").Comment("类型 0系统 1用户"),
		entity.NewField("nextRunTime", "nextRunTime", "datetime").Nullable().Comment("下次执行时间"),
		entity.NewField("taskType", "taskType", "tinyint").NotNull().Default("0").Comment("任务类型 0Cron 1间隔"),
		entity.NewField("lastExecuteTime", "lastExecuteTime", "datetime").Nullable().Comment("最近领取时间"),
		entity.NewField("lockExpireTime", "lockExpireTime", "datetime").Nullable().Comment("执行租约到期时间"),
		entity.NewField("lockOwner", "lockOwner", "varchar").Size(64).Nullable().Comment("执行租约所有者"),
	)

	return entity.NewDefinition("task", "TaskInfoEntity", "task_info").
		WithResource("task.info").
		Comment("任务信息").
		Fields(fields).
		WithTenantMode(entity.TenantModeRequired).
		WithIndexes(
			entity.NewUniqueIndex("uk_task_info_job_id", "jobId"),
			entity.NewIndex("idx_task_info_tenant_status", "tenantId", "status"),
			entity.NewIndex("idx_task_info_next_run", "nextRunTime"),
		)
}
