# 运行配置精简设计

## 目标

整理 `manifest/config/config.yaml` 与 `manifest/config/config.local.yaml`：补齐运行时常用配置及中文注释，删除仅重复代码默认值的低频配置。

## 配置边界

- 使用 `cool.crud.softDelete` 控制项目现有的回收站归档能力。
- 不配置 GoFrame `database.deletedAt`。业务实体没有删除时间字段，项目软删除由 `gnrecycle` 实现，混用两套机制会改变查询和删除语义。
- 使用 `cool.eps` 控制是否发布实体与接口元数据；关闭时发布空视图，避免线上暴露字段和路径信息。
- 使用 `cool.initDB` 与 `cool.initMenu` 分别控制模块 `db.json` 数据种子和 `menu.json` 菜单种子的导入，不影响业务表结构同步。
- 不引入 `cool.initJudge`。种子导入固定使用数据库幂等锁 `cool_seed_lock`，不提供不适合容器和多副本部署的文件锁模式。
- 保留认证、Session、HTTP、gRPC、Outbox 开关等部署时常见配置。
- Outbox 的轮询、租约、重试、载荷限制等参数沿用 `outbox.DefaultConfig()`。
- HTTP 请求体上限、CRUD 批量和分页硬限制等参数沿用各自的代码默认值。
- 本地配置额外保留 MySQL、Redis 连接信息和开发环境 bcrypt cost；不展开连接池、SQL 超时、调试模式等低频选项。

代码默认使用 `eps=false`、`initDB=false`、`initMenu=true`，与 Node 框架默认语义一致。项目的本地配置显式开启三项，默认配置显式关闭三项，使部署意图直接可见。

## 文件职责

`config.yaml` 是默认入口，提供通用应用配置。`config.local.yaml` 通过 `COOL_CONFIG_FILE` 显式启用，提供可直接用于本地开发的完整连接配置。两份文件使用相同的核心配置结构，但不会互相合并。

## 验证

- 使用 YAML 解析器检查两份文件语法。
- 使用项目配置加载路径验证字段名、类型和默认值合并行为。
- 分别验证数据库种子与菜单种子开关，确保两项可以独立控制。
- 验证 EPS 关闭时只发布空视图，开启时仍生成完整视图。
- 运行相关 Go 测试，确认配置精简未改变默认行为。
