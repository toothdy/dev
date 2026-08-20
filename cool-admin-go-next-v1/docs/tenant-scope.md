# 通用租户作用域

本文说明 `cool-admin-go-next` 的租户数据语义、运行时边界和自定义查询约束。

## 数据语义

租户字段只有两种规范化持久值：

| 数据库值 | 含义 |
|---|---|
| `tenant_id IS NULL` | 平台数据 |
| `tenant_id > 0` | 指定租户数据 |

`tenant_id = 0` 仅用于兼容升级前的数据和令牌。启动迁移会将租户表中的零值幂等转换为 `NULL`；新写入不得产生零值。

## 作用域

`cool/tenant` 将一次数据库操作解析为四种不可变作用域：

| 作用域 | 行为 |
|---|---|
| Tenant | 读取和修改追加参数化 `tenant_id = ?`；新增强制写入该租户 ID |
| Platform | 已认证平台用户可跨租户管理；新增写入 `NULL` |
| Bypass | 仅供 Seed、迁移、登录前日志等已审计内部路径显式使用 |
| Missing | 租户资源立即拒绝，不得退化为平台或 Bypass |

`tenant.ForTenant` 用于平台任务明确派生一个正租户作用域；`tenant.WithoutTenant` 用于明确的内部跨租户操作。HTTP 参数和客户端提交的 `tenantId` 都不能选择作用域。

## 查询边界

通用 CRUD 在启动期编译模型租户元数据，并在运行时执行以下约束：

- Add/AddMany 删除客户端租户值，再注入服务端作用域值。
- Info/List/Page/Count 使用参数化租户谓词。
- Update/Delete 在同一事务内校验受影响行数；批量操作不允许部分命中后提交。
- 平台和 Bypass 只省略租户谓词，不省略命中数量及事务一致性校验。

自定义 ORM 查询使用 `tenant.ScopedModel`。复杂 JOIN 或 raw SQL 必须对每个带租户字段的别名追加 `tenant.Predicate`，或先在同一事务中锁定并验证父实体。

`user_role`、`role_menu`、`role_department` 等无 `tenant_id` 的关系表不能伪造租户条件。写入和删除关系前必须先按作用域验证两侧主实体；污染关系读取也必须 JOIN 主实体并验证作用域。

`cool/tenant/raw_access_test.go` 通过 Go AST 扫描模块 Service 中的 `GetOne`、`GetAll`、`GetCount`、`Exec` 和 `Model`。白名单精确到文件、函数、操作、调用指纹、重复序号和用途；新增或变化的直接数据库入口必须重新审计。

## 公开读取

公开路由不会自动获得 Bypass。允许读取平台公共配置或字典时，必须显式使用 `GlobalOnlyPredicate`，只返回 `tenant_id IS NULL` 的数据。不得使用 `tenant_id IS NULL OR tenant_id = 0` 作为长期公开策略。

## 配置与迁移

生产环境应启用：

```yaml
cool:
  tenant:
    enable: true
    requireEnabled: true
```

`requireEnabled` 用于阻止要求租户隔离的部署在关闭功能时启动。零值迁移只处理已编译为 tenant-aware 的模型表，不处理无租户列的关系表。

## 性能验证

租户作用域不执行额外的租户授权查询，不解析或正则改写 SQL，也不使用全局锁或每请求反射。

2026-07-28 的代表性 MySQL EXPLAIN 结果：

| 操作 | 访问计划 |
|---|---|
| Param tenant Page | `idx_base_sys_param_tenant_id`，`type=ref` |
| Param tenant Update | `PRIMARY`，`type=range`；tenant 索引在 `possible_keys` |
| Param tenant Delete | `PRIMARY`，`type=range`；tenant 索引在 `possible_keys` |

点更新和点删除优先使用主键比强制 tenant 索引更高效，租户谓词仍参与最终行过滤。`cool/tenant` 同时提供 Scope 和 Predicate microbenchmark，仅报告相对变化，不设置依赖机器速度的阈值。

## 验证矩阵

```bash
go test ./... -count=1
go test -race ./cool/auth ./cool/tenant ./cool/crud ./modules/base/service/sys ./modules/dict -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test -p=1 ./modules/base ./modules/base/service/sys -count=1
COOL_DICT_INTEGRATION=1 go test ./modules/dict -count=1
```

真实 MySQL 用例覆盖 Tenant/Platform、伪造租户字段、跨租户 CRUD、关系污染、递归删除、公开 GlobalOnly 和事务完整回滚。
