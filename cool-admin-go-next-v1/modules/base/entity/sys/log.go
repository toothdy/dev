package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysLog() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("userId", "userId", "bigint").Unsigned().Nullable().Comment("用户ID"),
		entity.NewField("action", "action", "varchar").NotNull().Comment("行为"),
		entity.NewField("ip", "ip", "varchar").Nullable().Comment("ip"),
		entity.NewField("params", "params", "json").Nullable().Comment("参数"),
	)

	return entity.NewDefinition("base", "BaseSysLog", "base_sys_log").
		WithResource("base.log").
		Comment("操作日志").
		Fields(fields).
		WithIndexes(
			entity.NewIndex("idx_base_sys_log_user_id", "userId"),
			entity.NewIndex("idx_base_sys_log_tenant_id", "tenantId"),
			entity.NewIndex("idx_base_sys_log_create_time", "createTime"),
		)
}
