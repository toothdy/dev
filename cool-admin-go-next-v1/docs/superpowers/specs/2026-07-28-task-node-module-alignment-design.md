# Task 模块 Node 目录对齐重构设计

- 日期：2026-07-28
- 项目：`cool-admin-go-next`
- 对照：`cool-admin-midway/.cursor/rules/module.mdc`、`task.mdc`、`service.mdc`
- 状态：已批准，等待实施计划

## 1. 背景

Task 功能已经完成，但当前 `modules/task/runtime` 同时容纳通用调度后端、Task 业务状态机、执行器和持久化逻辑。这个平铺的技术目录无法与 Node 的模块角色一一对应，也使 `cool/task` 只保留处理器注册和表达式解析，没有承载与 `@cool-midway/task` 对等的通用调度能力。

Node 文档确定了两个边界：

1. `src/modules/<name>` 是业务模块，使用 `controller`、`entity`、`event`、`middleware`、`queue`、`service` 等角色组织代码。
2. `@cool-midway/task` 提供可被任意模块复用的队列和调度基础设施，不包含 `task_info`、HTTP 或具体业务状态。

Go 版重构严格对齐这个模块角色设计，但不复制 TypeScript 的依赖注入方式、BullMQ 数据协议或重复的 Local/Bull 业务代码。

本文档取代原 Task 设计中的目录归属和默认 Seed 决策；原设计的功能、协议、安全与运行语义继续有效。

## 2. 目标

1. `cool/task` 对应 `@cool-midway/task`，只承载通用处理器、队列和调度能力。
2. `modules/task` 对应 `src/modules/task`，按 Node 角色目录组织具体 Task 业务。
3. 删除 `modules/task/runtime` 这个混合技术目录。
4. 保留已实现的统一 Engine/Executor 语义，不在 Local 和 Redis Service 中复制 CRUD、日志和执行逻辑。
5. 保持 Vue/Node 的 HTTP、EPS、权限、数据库字段和 `service.method(...)` 协议不变。
6. 保持 MySQL 租约、`jobId` 世代、租户隔离、超时重试和优雅退出语义不变。

## 3. 非目标

- 不将 Asynq 改回 Bull/BullMQ。
- 不让 Go Worker 消费 Node 队列。
- 不修改 Vue、Node、Java 或旧 Go 项目。
- 不减少测试来换取更少的文件数。
- 不为了文件名对齐而把不同责任合并成巨型 Service。

## 4. 最终目录

```text
cool/task/
├── handler.go
├── expression.go
├── queue.go
├── scheduler.go
├── local.go
├── redis.go
└── auto.go

modules/task/
├── config.go
├── register.go
├── db.json
├── controller/
│   └── admin/
│       └── info.go
├── entity/
│   ├── info.go
│   ├── log.go
│   └── models.go
├── event/
│   └── comm.go
├── middleware/
│   └── task.go
├── queue/
│   └── task.go
└── service/
    ├── info.go
    ├── local.go
    ├── redis.go
    ├── demo.go
    ├── engine.go
    ├── executor.go
    └── store.go
```

Go 测试继续使用同包 `*_test.go`，不增加独立的测试技术目录。

## 5. `cool/task` 责任

### 5.1 Handler 与表达式

`handler.go` 和 `expression.go` 保留现有不可变注册表、永久错误与严格 JSON 参数解析。其他业务模块只依赖 `cool/task` 注册 Handler，不引用 `modules/task`。

### 5.2 通用 Queue/Scheduler

`queue.go` 定义与业务无关的消息、Consumer 和队列契约。业务 Payload 由 `modules/task/queue` 负责编解码；`cool/task` 不知道 TaskInfo、租户 ID 或 HTTP 参数。

`scheduler.go` 定义注册、替换、移除、单次投递、健康检查和停止契约。`local.go`、`redis.go`、`auto.go` 分别实现进程内 Cron/Worker、Asynq/Redis 和启动时后端选择。

`cool/task` 必须满足：

- 不导入 `modules/task`。
- 不读取 `task_info` 或 `task_log`。
- 不读取 HTTP Context 中的租户。
- 不解析 `repeatCount`、`type` 或其他 Node 兼容字段。
- 不在包内直接读取 `g.Cfg()`；所有连接与调度参数通过构造函数注入。

## 6. `modules/task` 责任

### 6.1 Controller、Entity 与配置

- `controller/admin/info.go` 保留 `/admin/task/info/*` 路由、权限和 CRUD 元数据。
- `entity` 保留 TaskInfo、TaskLog 元数据和模型注册。
- `config.go` 读取 `task.*` 与 GoFrame `redis.default`，但只向 `cool/task` 传入已校验的构造参数。
- `db.json` 对齐 Node 的两条演示任务，两者均为停止状态，初始化不会自动执行。

### 6.2 Event 与 Middleware

`event/comm.go` 对应 Node `onServerReadyOnce` 和 `onLocalTaskStop`：它构建一次模块运行依赖，在 Schema/Seed 后启动初始对账，并在应用停止时退出 Engine 和 Scheduler。Scheduler consumer 通过启动前安全的委托回调统一进入 `Engine.Dispatch`，不能直接绑定 Executor，否则会跳过周期任务的 `nextRunTime` 刷新和过期计划后处理。Go 实现继续使用现有 `registry.Runtime` 生命周期，不引入第二套事件总线。

`middleware/task.go` 只保护会改变调度状态的 Add、Update、Delete、Start、Stop 和 Once。它通过健康检查在后端不可写时返回统一业务错误；Info、Page 和 Log 保持可读。

### 6.3 Queue

`queue/task.go` 是通用 `cool/task` Queue 与 Task 业务 Payload 之间的适配器：

1. 编码和解码 TaskID、JobID、TenantID、ScheduledAt、Manual 和 ExecutionID。
2. 不信任消息中的任务定义，交给 Executor 从 MySQL 重读。
3. 将永久错误转换为队列的不重试结果。
4. 保持 Go 专属 Asynq 队列和 key 命名空间。

Queue 不导入 Service 的具体类型，只接收模块组装层注入的 Consumer 回调，避免 Go 包循环依赖。

### 6.4 Service

- `info.go` 是 Controller 唯一依赖的 TaskInfo Service，处理 CRUD、启停、Once、日志、Node 字段归一化和处理器校验。
- `local.go` 封装本地 Scheduler 的构建与业务适配，对应 Node `TaskLocalService`。
- `redis.go` 封装 Redis 客户端、Asynq Scheduler 与 `auto/redis` 选择，对应 Node `TaskBullService`。
- `demo.go` 提供与 Node `TaskDemoService` 对应的显式注册 Handler。
- `engine.go` 保留 MySQL 权威状态、启动/周期对账、`jobId` 世代和日志清理。
- `executor.go` 将稳定业务 `ExecutionID` 与每次 Claim 的唯一 token 分离；统一 guard 从 Claim 成功开始覆盖处理器运行、重试退避和超时后等待。手动 Once 只在 Worker 内短暂有界等待，仍忙时由 Local/Redis 使用新 transport ID 延迟重投且保留业务 Attempt。
- `store.go` 封装 GoFrame/MySQL 操作和租户作用域，不对 Controller 暴露；`lastExecuteTime` 使用统一的秒级批次键，消息与 Handler 仍保留原毫秒时间。

Local 和 Redis Service 只是后端适配器，不复制 Info、Engine、Executor 或 Store 中的业务逻辑。

Redis busy 重投必须在当前 active delivery 内以短退避持续持久化新 delivery，成功后才确认旧 delivery；暂时 enqueue 故障不返回给 Asynq、不增加 `RetryCount`，`maxRetry=0` 也不能归档尚未执行的任务。停止时先拒收并调用 Asynq `Stop`，再取消并等待协调者、停止 producer，最后调用 Asynq `Shutdown`；协调者 Context 不得作为 worker `BaseContext`，active handler 在 `shutdownTimeout` 内保持可运行，超时后才由 Asynq 取消并回推原 delivery。

## 7. 依赖方向

```text
modules/* handlers
        |
        v
cool/task <---------------- modules/task/queue
    ^                              |
    |                              v
    +---------------- modules/task/service
                                   |
                                   v
                           modules/task/entity
```

强制规则：

1. `cool/**` 不得导入 `modules/**`。
2. Controller 只依赖 `service.InfoService`。
3. Queue 通过回调依赖执行契约，不导入具体 Service。
4. Local/Redis 共用同一 Engine、Executor 和 Store。
5. Event 是唯一运行时组装入口，Register 只声明模块元数据和工厂。

## 8. 行为不变项

重构前后必须保持：

- `/admin/task/info/add|delete|update|info|page|start|stop|once|log`。
- `task:info:*` 权限和 EPS 字段。
- `repeatCount/limit`、六段 Cron、间隔毫秒、时间窗口和次数限制。
- Add、Update、Start、Reconcile 和 duration 转换统一接受 1000 到 100000000000 毫秒，拒绝两侧越界及整数溢出。
- `auto/local/redis` 模式与启动时降级。
- Redis 专属队列 `cool-admin-go-next-task-v1`，不消费 Bull/BullMQ key。
- MySQL 原子租约，以及 `id + jobId + lockOwner + 未过期` 条件续租和 token 条件释放。
- 租户内管理、跨租户调度发现和执行租户恢复。
- Handler 显式注册，禁止任意反射调用。
- 每次尝试日志、有界重试、超时和 panic 恢复。

## 9. 错误处理

- 目录迁移不改变对外错误码和响应包络。
- Scheduler 错误从 `cool/task` 原样返回给模块 Service，由 Middleware/Service 转换为现有业务错误。
- `auto` 只在启动期从 Redis 降级到 Local，运行期不自动切换。
- 重构期间任一包循环依赖、队列协议改变或 EPS 差异都视为阻断问题。

## 10. 迁移策略

1. 先在 `cool/task` 建立业务无关的 Queue/Scheduler 契约和测试。
2. 迁移 Local、Redis、Auto 实现，保持现有队列 key 和调度计算。
3. 在 `modules/task/queue` 增加 Payload 适配器，然后切换 Engine 调用。
4. 将 Repository、Executor、Engine 迁入 `service`，将模型类型归入 Entity/Service 所有。
5. 将启停组装迁入 `event/comm.go`，增加调度写操作 Middleware。
6. 加入停止状态的 Node 对齐 `db.json`。
7. 删除 `modules/task/runtime`，全局搜索确认无旧 import path。

迁移每一步都必须保持可编译；不使用先删除再重建的方式。

## 11. 测试

1. `cool/task` 保留 Handler、Expression、Local、Redis 和 Auto 单元测试；Redis 单元测试覆盖 busy 持久化暂时失败时不重复调用 Consumer，以及关闭 Context 时保留原 delivery。
2. `modules/task` 保留 Config、Entity、Controller、Service、Engine、Executor 和 Store 单元测试。
3. HTTP/EPS 契约测试证明重构前后路由、权限、字段一致。
4. MySQL 集成测试覆盖租户、同 ExecutionID 的 token 接管、Local/Redis busy Once 重投、统一 guard 续租恢复与确认期限、毫秒批次的初次/重试/limit 幂等、every 上下界、旧世代写回和经 `Engine.Dispatch` 的 Local 端到端链路。
5. Redis 集成测试使用随机 Go 专属 namespace，逐一删除该 namespace 的 Asynq queue 元数据、任务 key 和 lease key，禁止 `FlushDB` 或触碰生产队列；覆盖 `maxRetry=0` busy 重投不归档，以及 `shutdownTimeout` 到期取消 active handler、按同一 transport ID 回推原 delivery 并由新 Worker 在不增加业务 Attempt 的前提下消费。
6. 运行 Task 聚焦测试、`-race`、`go vet`、`go test ./...` 和 `git diff --check`。

## 12. 验收标准

1. `modules/task/runtime` 不再存在。
2. `cool/task` 不导入任何 `modules/**` 包。
3. `modules/task` 生产代码只使用 Node 模块角色目录和 Go 必需的根入口文件。
4. Local 和 Redis 不复制 CRUD、日志、租约或 Handler 执行逻辑。
5. 现有对外契约和全部安全增强保持不变。
6. 聚焦测试、MySQL 集成、竞态检测、vet、全仓测试和差异检查通过。
