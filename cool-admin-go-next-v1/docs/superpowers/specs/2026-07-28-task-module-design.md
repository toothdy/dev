# cool-admin-go-next Task 模块设计

- 日期：2026-07-28
- 项目：`cool-admin-go-next`
- 输入：`docs/midway-gap-analysis.md`
- 状态：已实施

## 1. 背景

`cool-admin-go-next` 已具备模块注册、Schema Sync、Seed、通用 CRUD、认证、权限、租户作用域和统一响应，但尚无模块启动/停止生命周期、任务处理器注册、定时调度或 Redis 队列。

Midway 8.x 的 Task 模块提供本地 Cron、Bull/Redis 分布式任务、启停与单次执行、任务日志和 Vue 管理接口。现有 `cool-admin-vue` 同时服务 Node、Java 等后端，因此 Go 实现不能修改前端字段、EPS 命名、权限标识或 `service` 表达式格式。

本设计实现完整 Task 模块，但不要求 Go 与 Bull/BullMQ 共享队列数据。MySQL 是任务定义和执行日志的事实来源；Redis 只承载 Go 服务的分布式调度运行态。

## 2. 目标

1. 无需修改现有 Vue Task 模块即可管理 Go 后端任务。
2. 支持本地调度和 Redis 分布式调度，并在启动时自动选择后端。
3. 两种后端共用一套校验、状态、执行、日志和租户语义。
4. 使用显式处理器注册表安全实现 `xxxService.method(...)` 调用协议。
5. 支持秒级 Cron、毫秒字段兼容的间隔任务、开始/结束时间、执行次数、单次执行、延迟和失败重试。
6. 多实例下避免同一计划并发执行，并明确采用至少一次投递语义。
7. 管理接口按租户隔离，后台调度可跨租户发现任务，日志保留正确租户归属。
8. 为 Task 及后续 Job、Queue 类模块建立可复用的应用生命周期边界。

## 3. 核心决策

### 3.1 统一引擎与可切换后端

Task 模块只有一套 `TaskService`、`Engine` 和 `Executor`。`Scheduler` 是后端接口，包含本地和 Redis 两种实现。CRUD、启停、日志、处理器解析和执行状态不在两个后端中重复实现。

本地后端使用 `robfig/cron/v3` 的秒级解析和进程内计划表。Redis 后端使用 Asynq `v0.25.1` 承载入队、消费、延迟和重试，并使用 Go 专属队列命名空间。该版本要求 Go 1.22，与项目 Go 1.23 兼容；最新 Asynq `v0.26.0` 要求 Go 1.24，不纳入本次实施。六段秒级周期由 Task 自己的 `robfig/cron/v3` 协调者产生消息，不直接依赖 Asynq Scheduler 的默认五段解析。

### 3.2 兼容层级

Go 实现必须兼容：

- `task_info` 和 `task_log` 的前端可见字段。
- `/admin/task/info/*` HTTP 接口、方法、响应包络和时间格式。
- `task:info:*` 权限标识和 EPS 服务树。
- `repeatCount` 与 `limit` 的映射。
- `taskDemoService.test(1,2)` 形式的 `service` 字符串。

Go 实现不兼容 Bull/BullMQ 的 Redis key、消息格式或 Worker 协议。Node 与 Go 可以使用同一 Redis 实例，但必须使用不同命名空间，不能相互消费任务。

### 3.3 投递与幂等

Redis 队列采用至少一次投递。`jobId + scheduledAt` 用于避免协调器重复入队，MySQL 原子租约用于避免同一任务并发执行，但进程在“业务已完成、队列尚未确认”之间崩溃时仍可能再次调用处理器。

`ExecutionID` 是同一业务执行在首次 delivery、队列业务重试和 busy 重投之间保持稳定的身份。每次真正尝试 Claim 都生成新的随机 claim token，并仅将该 token 写入 `lockOwner`；队列 transport ID 是独立身份，busy 重投必须生成新的 transport ID，不能复用 `ExecutionID` 或前一条 delivery ID。

消息和 Handler 始终保留 Scheduler 产生的原始毫秒级 `scheduledAt`，执行 ID 也使用原始时间生成，确保相邻 1500 毫秒批次不碰撞。MySQL `datetime` 只保存秒精度，因此 Executor 使用唯一的批次时间规范化规则，将 `scheduledAt` 截断到秒后用于 `lastExecuteTime` 的首次领取、重复投递判断和恢复重试比较；规范化不能回写消息或 Invocation。

所有可重试处理器必须具备业务幂等性。Task 模块不承诺通用的 exactly-once；文档、注册接口和测试必须明确这一点。

## 4. 架构

### 4.1 TaskService

`TaskService` 是 Controller 使用的应用服务，负责：

- Task CRUD 的字段归一化和业务校验。
- `start`、`stop`、`once` 和 `log`。
- `repeatCount` 与 `limit` 的兼容转换。
- 租户范围内的任务与日志访问。
- 数据库事务提交后通知 Engine 对账。

通用 CRUD 只负责基础查询与持久化。所有会改变调度状态的 Add、Update、Delete 必须经过 TaskService，不能依赖通用 CRUD 的事后副作用拼接外部队列操作。

### 4.2 Engine

`Engine` 维护期望状态与运行后端之间的一致性：

- 启动时加载所有启用任务并同步到 Scheduler。
- API 修改后立即同步单个任务。
- 周期性以 MySQL 为准进行全量对账。
- 为新增或改变调度配置的任务生成新 `jobId`，使旧消息失效。
- Redis 模式下竞争调度协调者租约；Worker 不依赖协调者身份。
- 停止时先拒绝新调度，再等待正在执行的任务退出。

调度写操作在数据库事务前先检查 Scheduler 是否可接收变更；检查失败时返回明确错误且不写数据库。事务提交后 MySQL 已成为权威期望状态，此后的即时同步失败只记录内部错误，API 返回写入成功，由后台对账继续重试。这样客户端不会收到“请求失败但数据实际已提交”的歧义结果。

### 4.3 Scheduler

Scheduler 接口只表达 Task 模块需要的能力：

- 注册或替换周期计划。
- 移除计划。
- 提交一次性执行。
- 查询下次执行时间。
- 启动、健康检查和停止。

本地实现维护进程内计划表，并将到期任务提交给本地 Worker 池。Redis 实现由持有租约的协调者产生 Asynq 消息，所有健康实例均可作为 Worker 消费。

### 4.4 HandlerRegistry

框架提供不可变的 Task 处理器注册表。其他模块在应用构建期注册处理器定义，Task Engine 启动前冻结注册表。

每个定义包含：

- 与 `service` 对应的注册键，例如 `baseSysLogService.clear`。
- 处理函数。
- 可选的执行超时、最大重试次数和重试分类覆盖。

处理函数接收可取消 Context、任务 ID、租户 ID、计划执行时间、当前尝试次数、`data` 原始值和 JSON 参数。重复名称、非法名称或空处理函数使应用启动失败。

### 4.5 Executor

Executor 是唯一允许调用处理器的组件。每次执行按以下顺序进行：

1. 根据任务 ID 重新读取最新记录。
2. 校验载荷 `jobId` 和租户；周期执行继续校验状态、开始时间、结束时间和次数限制，手动执行按 Once 规则处理。
3. 为本次 Claim 生成唯一 token，以 token 作为 `lockOwner` 原子领取 MySQL 执行租约并更新 `lastExecuteTime`、`lockExpireTime`。
4. Claim 成功后立即启动统一租约守护，再构造带超时的执行 Context，调用已注册处理器并恢复 panic。
5. 按尝试结果写 TaskLog，更新 `nextRunTime` 和任务状态。
6. 仅由匹配 `taskId + jobId + lockOwner` 的执行者释放执行租约。

超时只能取消 Context，Go 无法强制终止忽略 Context 的处理器。处理器契约要求阻塞 IO 和循环主动响应取消。统一 guard 从 Claim 成功开始按 `lockTTL/3` 续租，连续覆盖 Handler 运行、业务重试退避和超时后等待，不在超时边界切换守护流程。瞬时数据库错误使用短退避在最近确认的到期时间前重试；超过确认期限或 Store 返回 owner 丢失后，guard 停止 Renew 和 Release，并取消协作 Handler。Renew 的条件必须包含未过期截止时间，不能复活已过期租约。

## 5. 框架生命周期

核心模块注册增加通用生命周期能力，而不是在 Task 包的 `init()` 中连接数据库或启动 goroutine。

生命周期顺序为：

1. 编译模块、模型、Controller、中间件和处理器元数据。
2. 完成 Schema Sync。
3. 完成 DB 和菜单 Seed。
4. 依模块顺序启动 Runtime。
5. 启动 HTTP Server。
6. 收到应用 Context 取消或关闭信号后进入 draining，拒绝新的调度写请求，并停止 HTTP Server 接收新连接。
7. 等待已进入的 HTTP 请求结束后，按逆序停止 Runtime。
8. 最后关闭数据库、Redis 等其余依赖。

Task Runtime 只构建一次，并注入 Task Controller。测试可以显式注入假 Scheduler、假时钟和处理器注册表，不依赖包级运行态。

除 `auto` 模式约定的 Redis 启动降级外，生命周期启动错误必须返回给调用者。已经启动的 Runtime 在后续 Runtime 启动失败时按逆序回滚停止。

## 6. 数据模型

### 6.1 task_info

前端可见字段与 Midway 保持一致：

| 字段 | 语义 |
| --- | --- |
| `jobId` | 当前调度世代 ID；配置改变后更新 |
| `repeatConf` | 后端生成的重复配置摘要和当前世代触发计数，保持兼容字段 |
| `name` | 任务名称 |
| `cron` | 秒级 Cron 表达式 |
| `limit` | 周期计划最大触发次数，空表示不限 |
| `every` | 间隔毫秒数 |
| `remark` | 备注 |
| `status` | `0` 停止，`1` 启用 |
| `startDate` | 最早计划执行时间 |
| `endDate` | 最晚计划执行时间 |
| `data` | 处理器可读取的原始业务数据 |
| `service` | 前端兼容的处理器表达式 |
| `type` | `0` 系统任务，`1` 用户任务 |
| `nextRunTime` | 下次计划执行时间，停止或完成时为空 |
| `taskType` | `0` Cron，`1` 固定间隔 |
| `lastExecuteTime` | 最近一次领取执行的时间 |
| `lockExpireTime` | 当前 MySQL 执行租约到期时间 |
| `lockOwner` | 当前执行租约的唯一 claim token，仅服务端持久化 |
| `tenantId` | 任务所属租户，平台任务为空 |

模型同时包含框架基础 ID、创建时间和更新时间。`jobId` 使用全局唯一值；增加租户与状态组合索引以及 `jobId` 唯一索引。

`jobId`、`repeatConf`、`nextRunTime`、`lastExecuteTime`、`lockExpireTime`、`lockOwner` 和 `tenantId` 是服务端管理字段，Add/Update 忽略客户端提交值。`lockOwner` 同时是只读和隐藏字段，不出现在 HTTP 响应或 EPS；其他既有运行字段仍按现有协议返回。`repeatConf` 使用版本化 JSON 保存后端模式、当前世代和周期触发计数；新世代会重置触发计数并清空执行租约。

### 6.2 task_log

TaskLog 保持 `taskId`、`status`、`detail`、`tenantId` 和基础字段。增加 `(tenant_id, task_id, create_time)` 组合索引。

每次尝试均可产生一条日志，因此一次最终成功的重试任务可能先有失败日志、后有成功日志。`detail` 保存可展示摘要；完整错误栈只写服务端日志。

删除任务时在同一数据库事务中删除其日志。Task 模块不创建默认可执行示例任务，避免生产安装后出现未知或无意义的处理器调用；现有 Base 菜单 Seed 已包含 Task 页面与权限。

## 7. HTTP 与 EPS 协议

必须提供：

```text
POST /admin/task/info/add
POST /admin/task/info/update
POST /admin/task/info/delete
GET  /admin/task/info/info
POST /admin/task/info/page
POST /admin/task/info/start
POST /admin/task/info/stop
POST /admin/task/info/once
GET  /admin/task/info/log
```

接口使用现有 Node 兼容响应包络和分页结构。Controller 前置归一化规则为：

- 请求有 `repeatCount` 时映射为 `limit`。
- `taskType=0` 时清空 `every`；`taskType=1` 时清空 `cron`。
- 空 `limit` 表示不限制。
- 客户端提交的运行态字段在进入 Service 前移除。
- Add 未提交 `status` 时沿用模型默认启用语义。
- Info 返回兼容的 `repeatCount`。

`once` 在两种模式中均表示“成功提交一次执行”，不等待处理器完成。单次执行使用独立执行 ID，不消耗周期任务 `limit`，也不改变原任务启停状态。它允许执行已停止或不在计划时间窗口内的任务，但仍必须通过租户、任务存在性、`jobId` 世代、处理器注册和参数安全校验。Worker 遇到忙租约只做短暂有界等待，仍忙则返回类型化 busy；Local 和 Redis 后端登记延迟重投后释放 Worker。重投保留 `ExecutionID` 和业务 `Attempt`，生成新的 transport ID，不写 TaskLog、不消耗业务重试预算；任务删除或 `jobId` 变化后终止。Redis 必须在当前 active delivery 内以短退避持续尝试持久化新 delivery，成功后才能确认旧 delivery；暂时 enqueue 故障不能返回给 Asynq、不能增加其 `RetryCount`，也不能在 `maxRetry=0` 时归档尚未执行的新 delivery。只有 Asynq 在关闭期限到达后取消当前 worker Context，持久化循环才退出并由 Asynq 回推原 delivery。

## 8. service 表达式

兼容语法为：

```text
<serviceName>.<methodName>(<JSON argument>, ...)
```

例如：

```text
taskDemoService.test()
taskDemoService.test(1, 2)
taskDemoService.test([1, 2], {"enabled": true})
```

解析器必须按 JSON 结构识别括号、数组、对象、转义字符串和参数分隔符，不能使用简单 `strings.Split(",")`。允许的参数值为字符串、数字、布尔值、`null`、数组和对象。

解析结果只用于查找完整注册键并传递 JSON 参数。实现不得按任意对象路径反射、执行脚本、解析 Go 表达式或允许请求指定包名。表达式长度、参数数量和单个参数大小必须有固定上限。

新增、修改、启用和单次执行前必须确认处理器存在。启动时发现启用任务引用未知处理器时，Engine 将该任务停止、清空 `nextRunTime` 并写失败日志，但不会阻止其他模块启动。

## 9. 调度语义

### 9.1 Cron 与间隔

Cron 使用 `task.timezone` 解释，支持现有 Vue 使用的六段秒级表达式。间隔任务使用 `every` 毫秒字段，产品范围统一为 1000 到 100000000000 毫秒（含边界）。

Add、Update、Start、启动和周期 Reconcile 都必须校验相同上下界，毫秒到 `time.Duration` 的转换先校验再计算，禁止裸乘溢出。历史遗留的已启用越界任务应被停止且不能进入 Scheduler。

`limit` 统计周期计划产生的执行批次，不统计重试和手动 `once`。达到限制或超过 `endDate` 后，任务变为停止状态并清空 `nextRunTime`。到达 `startDate` 前不调用处理器，但 `nextRunTime` 应显示第一条合法计划时间。

### 9.2 本地模式

所有实例可以加载相同计划。到期后 Executor 通过 MySQL 条件更新领取租约并写入本次 Claim 的唯一 token，确保同一任务不会在多个实例并发运行。续租和释放同时校验 `id + jobId + lockOwner`，续租还必须校验租约未过期。单实例失败后，其他实例可在租约到期后继续执行；即使重试沿用同一 `ExecutionID`，旧 guard 也不能续租或释放新 token 的租约。

本地计划只存在于内存，应用启动和周期对账从 MySQL 恢复。Task 定义不能依赖某一实例的本地状态。

### 9.3 Redis 模式

一个持有 Redis 租约的协调者负责将周期任务入队；租约丢失后立即停止产生新消息。其他实例可接替协调者。所有实例按 `concurrency` 启动 Worker。

消息至少包含任务 ID、`jobId`、租户 ID、计划执行时间、是否手动执行和尝试信息。Worker 不信任消息中的完整任务配置，执行前必须从 MySQL 重读。

同一执行批次的重试沿用原计划时间，不额外消耗 `limit`。本地后端由 Executor 执行退避重试；Redis 后端每次 Asynq delivery 只执行一个业务 attempt，由 Asynq 独占重试计数，避免两层预算叠加。处理器返回永久错误时不重试；普通错误、超时和可恢复基础设施错误按退避策略重试。达到最大重试次数后记录最终失败，但周期任务仍可等待下一次计划。

Redis 停止时先把 Scheduler 标记为拒收并调用 Asynq `Stop` 停止 dequeue，再取消并等待协调者、停止本地 producer，最后调用 Asynq `Shutdown`。协调者 Context 与 Asynq `BaseContext` 必须分离，不能在 graceful shutdown 开始前取消 active handler；active delivery 可在 `shutdownTimeout` 内完成，超时后由 Asynq abort、取消 worker Context 并回推原 delivery。

### 9.4 Redis 故障

启动行为：

- `mode=local`：不探测 Redis，直接使用本地后端。
- `mode=redis`：Redis 未配置或不可用时拒绝启动。
- `mode=auto`：Redis 配置可用时选择 Redis，否则记录明确告警并选择本地后端。

已经进入 Redis 模式后发生断线时，实例暂停产生新计划并持续重连，不在运行期切换本地模式，以避免网络分区造成两套 Scheduler 同时工作。读接口和 Delete 继续可用；Add、Update、Start、Stop 和 Once 返回服务不可用且不提交变更。

多实例生产部署应显式设置 `mode=redis`，避免不同实例在网络状态不一致时选择不同后端。`auto` 主要用于兼容部署、开发和单实例运行。

## 10. 配置

配置文件保留全部生产参数，并逐项写注释：

```yaml
task:
  # 调度模式：auto 启动时优先 Redis 并可降级本地；local 强制本地；redis 强制 Redis。
  mode: auto
  # Cron 表达式和 nextRunTime 使用的 IANA 时区。
  timezone: Asia/Shanghai
  log:
    # 成功和失败任务日志的保留天数；后台每日清理更早记录。
    keepDays: 20
  execution:
    # 单次处理器尝试的默认超时；处理器可在注册时覆盖并必须响应 Context 取消。
    timeout: 5m
    # MySQL 防重租约有效期；应大于 timeout，进程崩溃后由其自动释放。
    lockTTL: 6m
  queue:
    # 每个应用实例可并行处理的最大任务数，本地 Worker 池和 Redis Worker 均使用该值。
    concurrency: 10
    # 首次执行失败后的最大额外重试次数；3 表示一次初始尝试加三次重试。
    maxRetry: 3
    # 第一次重试前的基础等待时间，后续重试按退避策略延长。
    retryDelay: 5s
    # 应用关闭时等待正在执行任务结束的最长时间。
    shutdownTimeout: 30s
```

Redis 连接复用 GoFrame 的统一 Redis 配置，不在 Task 下重复维护地址、账号、密码和 DB。Asynq 只接受 `go-redis` 客户端，不能直接使用 GoFrame Redis 客户端类型，因此 Task Runtime 根据同一配置创建一个受自身生命周期管理的 `go-redis` 连接池，并将它共享给 Asynq Client、Server 和队列协调逻辑。配置校验在 Runtime 启动前完成：时区必须存在，时长和并发数必须为正，`lockTTL` 必须大于默认 `timeout`；处理器覆盖的超时也必须小于其有效租约时长。

## 11. 租户与权限

TaskInfo 和 TaskLog 都是租户感知模型。HTTP 请求遵循框架通用租户作用域：

- 租户只能查询、修改、启停、单次执行和删除自己的任务。
- 跨租户目标对外表现为不存在，不泄露其状态。
- 客户端提交的 `tenantId` 不能改变记录归属。
- 平台作用域保持框架已批准的平台访问语义。

`type=0/1` 只保留 Node 的系统任务/用户任务分类，不改变租户作用域。系统任务也必须属于明确的平台或租户 Context，不能因为 `type=0` 自动绕过租户过滤。

Engine 使用显式 Bypass 跨租户扫描启用任务，但每个任务随后使用 `ForTenant(task.tenantId)` 派生执行 Context。平台任务使用平台内部 Context。TaskLog 插入显式复制任务的租户 ID，修复 Midway 后台 Worker 无请求上下文时日志可能缺少租户 ID 的问题。

调度 Bypass 只允许 Task 内部 Runtime 使用，不能由 HTTP 参数或 `service` 表达式选择。现有权限字符串继续由 Controller/EPS 和权限中间件执行。

## 12. 写入与对账流程

### 12.1 Add 和 Update

1. 在租户作用域内绑定并归一化请求。
2. 校验名称、计划、日期、次数、处理器和参数。
3. Add 和 Update 均先检查 Scheduler 健康状态；不可接收变更时直接返回且不写数据库。
4. 生成或按规则更新 `jobId`，在事务中保存 TaskInfo。
5. 提交后立即要求 Engine 同步该任务。
6. Scheduler 成功后更新 `repeatConf` 和 `nextRunTime`。
7. 提交后的同步失败记录内部错误并交给后台对账，API 仍返回数据库写入成功。

Add 和 Start 总是生成新 `jobId`。Update 只有在 `taskType`、`cron`、`every`、`limit`、`startDate`、`endDate`、`service`、`data` 或 `status` 变化时生成新 `jobId`；只修改 `name`、`remark` 或 `type` 不替换调度世代。

### 12.2 Start 和 Stop

Start 先检查 Scheduler 健康状态，再在作用域内锁定 TaskInfo，校验处理器和计划，生成新 `jobId` 并将期望状态设为启用，提交后同步 Scheduler。

Stop 同样先检查 Scheduler 健康状态，再将数据库期望状态设为停止并清空 `nextRunTime`，最后移除运行计划。已经被 Worker 领取的处理器通过任务 Context 取消；若处理器忽略取消，租约和 `jobId` 校验阻止新的旧世代执行，但不能强制终止正在运行的 Go 代码。

### 12.3 Delete

Delete 在租户作用域事务内锁定并验证全部目标任务，原子删除 TaskLog 和 TaskInfo。提交后移除 Scheduler 计划。迟到消息因 TaskInfo 不存在而直接确认丢弃。

批量删除保持全有或全无；其中任一 ID 不在当前作用域时整批回滚。

### 12.4 Reconciliation

Engine 定期比较 MySQL 启用任务和 Scheduler 已知计划：

- 补建缺失计划。
- 替换 `jobId` 或配置不一致的计划。
- 删除数据库已停止或不存在的计划。
- 刷新 `nextRunTime`。

对账使用单次快照和有界批量，不在 HTTP 请求热路径执行全表扫描。

## 13. 错误与日志

| 条件 | 行为 |
| --- | --- |
| Cron、间隔、日期、次数或表达式非法 | 业务参数错误，不写入调度 |
| 处理器未注册或参数无法绑定 | 业务参数错误；遗留启用任务自动停止并记失败日志 |
| 目标任务不在租户作用域 | 使用现有 Not Found 协议 |
| Redis 模式运行期不可用 | 读接口和 Delete 可用；Add、Update、Start、Stop、Once 返回服务不可用且不提交变更 |
| 处理器返回永久错误 | 记录失败，不重试该批次 |
| 处理器返回普通错误或超时 | 记录当前尝试并按策略重试 |
| 处理器 panic | 恢复 panic、记录内部堆栈并按普通失败处理 |
| Scheduler 对账失败 | 记录结构化错误，保留 MySQL 期望状态并重试 |
| 应用关闭等待超时 | 取消任务 Context，记录未完成数量并继续关闭 |

HTTP 错误不得包含 Redis 密码、SQL、Go 路径或堆栈。TaskLog 的 `detail` 有固定长度上限；超长返回值和错误摘要安全截断。处理器返回值以 JSON 序列化，无法序列化时保存类型安全的摘要。

TaskLog 每日按 `keepDays` 跨租户清理。清理使用显式内部 Bypass、分批删除和数据库索引，不在每次任务执行后同步执行大范围删除。

## 14. 测试策略

### 14.1 单元测试

- `service` 空参数、多个参数、嵌套数组/对象、转义字符串和字符串内逗号。
- 非法路径、尾随字符、非 JSON 参数、长度和数量上限。
- 处理器重复注册、未知处理器和参数绑定错误。
- 六段 Cron、间隔、时区、开始/结束时间和次数计算。
- Add、Update、Start、Stop、Delete 和 Once 状态流转。
- 新 `jobId` 使旧消息失效。
- 超时、Context 取消、永久错误、普通错误、panic 和退避重试。
- 优雅退出和启动失败后的生命周期逆序回滚。
- Redis busy 持久化遇到暂时 enqueue 故障时在原 delivery 内重试，不重复执行业务 Consumer；关闭 Context 时保留原 delivery。

### 14.2 Engine 组件测试

使用假时钟、假 Scheduler 和假处理器验证：

- 启动恢复和周期对账。
- 缺失计划补建、旧计划替换和停止计划清理。
- `limit` 不统计重试和 `once`。
- 停止、删除与迟到消息竞争。
- Redis 运行期健康状态对写接口的影响。

### 14.3 MySQL 集成测试

显式测试库包含平台、租户 A 和租户 B 数据。验证：

- HTTP CRUD、启停、Once、Log 的完整租户矩阵。
- 客户端伪造 `tenantId` 无效。
- Engine 可跨租户扫描，Executor 和 TaskLog 恢复正确租户 Context。
- 两个实例只有一个能领取同一执行租约。
- 进程崩溃模拟后租约到期可恢复。
- 两次并发 Once 均执行且串行，不改变周期状态。
- 同一 `ExecutionID` 的不同 Claim 使用不同 token，旧 token 在接管后不能续租或释放新租约。
- 统一 guard 使用满足生产构造约束（`lockTTL-timeout >= 1s`）的参数覆盖 Handler 超时前后续租；瞬时续租失败可恢复，越过确认期限后停止并取消 Handler。
- 多个 busy Once 不占满 Worker；Local/Redis 延迟重投保留 `ExecutionID/Attempt`，前租约释放后成功。
- 毫秒计划时间按整秒落库但原样传给 Handler；同批次初次投递、重复 Attempt 0、limit 最终批次恢复和重试只计数一次。
- 相邻 1500 毫秒批次执行键不碰撞；`every` 的 1000 和最大值通过，999、最大值加一和 `MaxInt64` 在 API/Reconcile 被拒绝。
- 批量删除和日志级联的事务原子性。
- 日志保留清理不会跨越保留边界或破坏其他任务。

### 14.4 Redis 集成测试

使用显式 Redis 测试配置验证：

- `auto` 的 Redis 选择和启动不可用时本地降级。
- `redis` 强制模式启动失败。
- 多实例协调者竞争、租约续期和故障接替。
- 多 Worker 消费、并发上限、延迟和退避重试。
- 重复入队抑制、旧 `jobId` 丢弃和至少一次重投。
- `maxRetry=0` 时 busy 重投仍能执行且不会归档；`shutdownTimeout` 到期会取消 active handler、按同一 transport ID 无损回推原 delivery，新 Worker 消费时不增加业务 Attempt。
- 断线后暂停调度、写接口降级和重连恢复。
- 队列命名空间不触碰 Bull/BullMQ key。
- 每个测试使用随机 namespace，并精确删除该 namespace 的 Asynq queue 元数据、任务 key 和协调者 lease key；禁止 `FlushDB` 或清理生产 queue。

### 14.5 HTTP 与前端契约测试

- 全部 `/admin/task/info/*` 路由的方法、权限和响应包络。
- Page 和 Log 分页结构及状态过滤。
- `repeatCount`/`limit`、时间字段和 `nextRunTime`。
- EPS 中 `service.task.info` 的 Action 列表。
- 现有 `cool-admin-vue/src/modules/task` 不做任何修改即可完成管理闭环。

正常 `go test ./...` 必须通过。MySQL 与 Redis 测试使用独立显式环境开关，未设置时跳过，不能连接默认开发库或共享 Redis。

## 15. 实施范围

本次实施包含：

1. 通用模块 Runtime 生命周期。
2. Task 处理器定义与注册表。
3. TaskInfo、TaskLog 模型和 Schema Sync 元数据。
4. Task Controller、Service、Engine、Executor。
5. 本地 Scheduler 和 Redis/Asynq Scheduler。
6. 配置、逐项注释和校验。
7. Task 日志保留清理。
8. 单元、组件、MySQL、Redis 和 HTTP 契约测试。
9. Task 差距状态由独立文档变更维护，本次实现提交不修改 `docs/midway-gap-analysis.md`。

## 16. 非目标

本设计不包含：

- 与 Bull/BullMQ Redis 数据格式互通。
- 修改现有 Vue Task 页面或字段。
- 通过反射执行任意 Go Service 方法。
- 通用事件总线、RPC 或分布式事务框架。
- Redis Session Store 的生产接线。
- Plugin 模块的动态处理器加载。
- Exactly-once 业务执行保证。

未来 Plugin 模块若允许动态增加处理器，必须另行设计注册表版本、卸载与运行中任务安全边界，不能绕过本设计的启动期冻结规则。

## 17. 验收标准

1. 现有 Vue Task 页面无需改动即可完成列表、新增、编辑、删除、启停、单次执行和日志查看。
2. 本地与 Redis 模式使用相同 HTTP、状态、租户和日志语义。
3. Cron、间隔、开始/结束时间、次数限制和 `repeatCount` 兼容行为通过测试。
4. `service` 表达式只可调用显式注册处理器，复杂 JSON 参数解析正确。
5. 多实例不会并发执行同一计划批次；至少一次投递和处理器幂等要求有测试和文档证明。
6. 租户 A 无法读取或操作租户 B 的任务与日志，后台日志始终保存正确租户 ID。
7. Redis 启动降级、运行期断线暂停和恢复符合已批准策略。
8. 应用启动和退出正确管理 Task Runtime，不遗留调度 goroutine 或继续接收新任务。
9. `go test ./...`、显式 MySQL 集成测试和显式 Redis 集成测试全部通过。
10. 本设计和实施计划记录 Task 已实施；差距文档保持为独立工作区改动。
