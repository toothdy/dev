package sys

import "github.com/toothdy/cool-admin-go-next/cool/entity"

func BaseSysUser() entity.Definition {
	fields := entity.BaseFields()
	fields = append(fields,
		entity.NewField("departmentId", "departmentId", "bigint").Unsigned().Nullable().Comment("部门ID"),
		entity.NewField("userId", "userId", "bigint").Unsigned().Nullable().Comment("创建者ID"),
		entity.NewField("name", "name", "varchar").NotNull().Comment("姓名"),
		entity.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		entity.NewField("password", "password", "varchar").Size(255).NotNull().Comment("密码"),
		entity.NewField("passwordV", "passwordV", "int").NotNull().Default("1").Comment("密码版本, 作用是改完密码，让原来的token失效"),
		entity.NewField("nickName", "nickName", "varchar").Nullable().Comment("昵称"),
		entity.NewField("headImg", "headImg", "varchar").Nullable().Comment("头像"),
		entity.NewField("phone", "phone", "varchar").Size(20).Nullable().Comment("手机"),
		entity.NewField("email", "email", "varchar").Nullable().Comment("邮箱"),
		entity.NewField("remark", "remark", "varchar").Nullable().Comment("备注"),
		entity.NewField("status", "status", "tinyint").NotNull().Default("1").Comment("状态 0-禁用 1-启用").WithDict("禁用", "启用"),
		entity.NewField("socketId", "socketId", "varchar").Nullable().Comment("socketId"),
	)

	return entity.NewDefinition("base", "BaseSysUser", "base_sys_user").
		WithResource("base.user").
		Comment("系统用户").
		Fields(fields).
		WithIndexes(
			entity.NewUniqueIndex("uk_base_sys_user_username", "username"),
			entity.NewIndex("idx_base_sys_user_department_id", "departmentId"),
			entity.NewIndex("idx_base_sys_user_status", "status"),
			entity.NewIndex("idx_base_sys_user_tenant_id", "tenantId"),
		)
}
