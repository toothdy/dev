package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysParam() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("keyName", "keyName", "varchar").NotNull().Comment("键"),
		entity.NewField("name", "name", "varchar").NotNull().Comment("名称"),
		entity.NewField("data", "data", "text").NotNull().Comment("数据"),
		entity.NewField("dataType", "dataType", "tinyint").NotNull().Default("0").Comment("数据类型 0-字符串 1-富文本 2-文件 "),
		entity.NewField("remark", "remark", "varchar").Nullable().Comment("备注"),
	)

	return entity.NewDefinition("base", "BaseSysParam", "base_sys_param").
		WithResource("base.param").
		Comment("系统参数").
		Fields(fields).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_param_key_name", "keyName"),
			entity.NewIndex("idx_base_sys_param_tenant_id", "tenantId"),
		)
}
