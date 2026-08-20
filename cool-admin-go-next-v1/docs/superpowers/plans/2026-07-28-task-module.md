# Task 模块实施计划

**Goal:** 在不修改 Node/Java 共用 Vue 前端协议的前提下，为 `cool-admin-go-next` 实现本地调度、Asynq 分布式队列、任务日志、租户隔离和应用生命周期闭环。

**Architecture:** 应用在 Schema 与 Seed 完成后启动模块 Runtime；Task 使用单一 `Engine + Executor` 处理业务语义，以 `Scheduler` 接口切换本地 Cron 和 Redis/Asynq。MySQL 保存任务定义、调度世代、执行租约和日志，Redis 仅保存 Go 专属队列运行态。

**Tech Stack:** Go 1.23、GoFrame v2.10.2、robfig/cron v3、Asynq v0.25.1、MySQL、Redis。

## 约束

- 保持 `/admin/task/info/*`、EPS、权限和 `service.method(...)` 前端协议不变。
- 不与 Bull/BullMQ 共享 Redis key 或消息格式。
- 本地和 Redis 后端共用校验、执行、重试、日志和租户语义。
- Redis 模式采用至少一次投递，处理器必须幂等。
- 所有后台跨租户读取必须显式使用内部 Bypass，执行和日志恢复任务租户。
- 不修改用户当前未提交的 tenant、dict 和 gap-analysis 文件。

## Task 1：通用 Runtime 生命周期

- [x] 为模块注册表增加 Runtime 工厂及处理器注册依赖。
- [x] 应用在 Schema/Seed 后顺序启动 Runtime，失败时逆序回滚。
- [x] 应用关闭时按逆序停止 Runtime，并测试启动、回滚和停止顺序。

## Task 2：Task 领域核心

- [x] 增加 TaskInfo、TaskLog 模型和租户索引。
- [x] 实现不可变 HandlerRegistry、永久错误和执行输入输出契约。
- [x] 实现严格的 `service.method(JSON...)` 解析器及边界测试。
- [x] 实现配置加载、时区/超时/租约/并发校验。

## Task 3：统一执行与本地调度

- [x] 实现 TaskService 的 CRUD、启停、单次执行和日志分页。
- [x] 实现 MySQL 原子租约、jobId 校验、次数窗口和租户恢复。
- [x] 实现 Executor 的超时、panic 恢复、日志和状态更新。
- [x] 使用 robfig/cron v3 实现秒级 Cron、间隔任务和本地 Worker 并发。

## Task 4：Redis/Asynq 调度

- [x] 根据 GoFrame Redis 配置创建 Task 自管的 go-redis 连接池。
- [x] 实现 Go 专属队列、Worker、重试、延迟和优雅关闭。
- [x] 实现 Redis 协调者租约、周期消息生产、续租和故障接替。
- [x] 实现 `auto/local/redis` 启动选择及运行期断线写保护。

## Task 5：HTTP、EPS 与模块接线

- [x] 注册 Task 模块模型、Controller、Service、Runtime 和内置处理器。
- [x] 提供 Node 兼容 CRUD、start、stop、once、log 路由。
- [x] 实现 `repeatCount`/`limit`、只读运行态字段和时间格式兼容。
- [x] 在配置文件加入全部 Task 配置及逐项中文注释。

## Task 6：验证与文档

- [x] 覆盖解析器、配置、调度计算、Registry、Executor 和生命周期单元测试。
- [x] 覆盖 HTTP/EPS、租户、MySQL 租约和 Redis Worker 集成测试。
- [x] 运行聚焦测试、`go test ./...`、`go vet ./...` 和 `git diff --check`。
- [x] 保持 Vue、Node、Java 项目和现有 gap-analysis 工作区改动零修改；差距状态由独立变更维护。
