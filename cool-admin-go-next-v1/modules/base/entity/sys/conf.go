package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysConf() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("cKey", "cKey", "varchar").NotNull().Comment("配置键"),
		entity.NewField("cValue", "cValue", "varchar").NotNull().Comment("配置值"),
	)

	return entity.NewDefinition("base", "BaseSysConf", "base_sys_conf").
		WithResource("base.conf").
		Comment("系统配置").
		Fields(fields).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_conf_c_key", "cKey"),
			entity.NewIndex("idx_base_sys_conf_tenant_id", "tenantId"),
		)
}
