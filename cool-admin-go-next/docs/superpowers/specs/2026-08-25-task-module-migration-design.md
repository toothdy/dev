# 任务调度模块迁移设计

> 日期：2026-08-25  
> 状态：已实现  
> 源模块：`cool-admin-midway/src/modules/task`  
> 目标模块：`cool-admin-go-next/modules/task`

## 1. 目标

在 `cool-admin-go-next` 中实现 Node 任务调度模块，使现有 `cool-admin-vue` 无需修改即可切换到 Go 后端。

Node 版是任务业务行为的首要事实来源。Go 版保持相同的 HTTP 路径、请求字段、响应数据、CRUD 能力、权限语义、EPS 元数据、初始化数据和执行语义，同时遵循 Go v2 已有的模块、实体、Service、Controller、事务、种子和静态生成架构。

## 2. 后端选型

Node 模块同时提供两套执行后端，由 `@cool-midway/task` 插件是否启用决定：

| 后端 | 依赖 | Node 默认 |
| --- | --- | --- |
| `bull` | Redis + BullMQ Queue/Worker | 插件启用时 |
| `local` | `cron` 库 + 数据库行锁 | 插件未启用时 |

`docs/差异化.md` 已判定 Go-next 没有 BullMQ 等价物，且明确不能用 Redis 或 Outbox 顶替通用队列。因此 Go 版只实现 `local` 语义，用 GoFrame `gcron` 承担定时触发、用 `task_info.lockExpireTime` 承担集群互斥——这与 Node `TaskLocalService` 的做法逐条对应。

Go 版不保留 `TaskInfoService` 的 `bull`/`local` 分派层、`TaskInfoQueue` 和 `TaskMiddleware`：没有第二个后端时它们只是空转的间接层。

### 2.1 与 Node local 的取舍

实现基准是 Node `local`。仅在 local 存在明显缺陷、且修正不需要新增数据库状态时，改取 `bull` 语义：

| 行为 | Node local | Node bull | Go 取值 | 理由 |
| --- | --- | --- | --- | --- |
| 手动执行一次 | 执行后把 `status` 改成 1、刷新 `nextRunTime` | 传 `isOnce`，不动任务状态 | bull | local 会把已停止任务显示成"进行中"，但并没有注册定时器 |
| 手动执行已停止任务 | 照常执行 | 照常入队执行 | 两者一致，照常执行 | 定时触发才要求运行状态 |
| `endDate` | 不生效 | 作为 repeat 结束时间 | bull | 字段在表单和实体里存在却完全不起作用是陷阱 |
| `startDate` | 早于开始时间跳过执行 | 作为 repeat 开始时间 | local | 两者可观察结果一致 |
| `limit`（最大执行次数） | 不生效 | 由队列内部计数 | local | 计数是队列内部状态，Go 不为此新增列 |
| 每次执行后回写 `status = 1` | 有 | 有 | 不实现 | 正在执行的任务状态必然已是 1，回写只会复活并发停止的任务 |

`limit` 与 `repeatCount` 保留为列和请求字段，但不参与调度，`repeatConf` 同理保持为 `null`——它们是队列后端的持久化痕迹，删除会破坏与 Node 库表和 EPS 契约的一致。

## 3. 范围

本次实现包含：

1. `task_info` 和 `task_log` 两张表及其 CRUD；
2. Admin 任务管理接口，含启动、停止、立即执行和日志分页；
3. `gcron` 定时注册、启动装载与优雅停止；
4. 基于数据库行锁的集群互斥执行；
5. 任务执行结果落库与过期日志清理；
6. `service` 字符串到 Go 可调用目标的静态注册表；
7. Node `db.json` 初始数据；
8. 权限、EPS、模块装配和契约测试。

以下能力沿用 Go v2 已批准的全局架构边界，不在本次扩建：

1. 不引入 BullMQ、通用队列、延迟队列或队列运维接口；
2. 不引入新的第三方依赖；
3. 不增加 `tenantId`、租户过滤或租户绕过逻辑；
4. 不修改前端；
5. 不新增 `menu.json`——任务菜单和全部权限项已经位于 `modules/base/menu.json`。

## 4. 模块结构

```text
modules/task/
├── config.go
├── db.json
├── dto/
│   └── info.go
├── entity/
│   ├── info.go
│   └── log.go
├── service/
│   ├── demo.go
│   ├── executor.go
│   ├── info.go
│   ├── pattern.go
│   ├── registry.go
│   └── scheduler.go
└── controller/
    └── admin/
        ├── info.go
        └── normalizer.go
└── schedule/
    └── task_job.go
```

`docs/superpowers/specs/2026-08-20-base-module-design.md` §3.3 规定定时任务组件放在协议目录 `schedule/`，并在框架 Schedule 专项设计落地前直接使用 `gcron` 兜底，参照 `modules/base/schedule/log_job.go`。

因此 `schedule/task_job.go` 只承担生命周期：`OnStart` 装载、`OnStop` 优雅停机。持有 `gcron.Cron` 的 `Scheduler` 与 `Executor` 留在 `service` 包——`InfoService` 需要在增删改后立即同步定时器，反过来 `Scheduler` 需要 `Executor`，若把它们放进 `schedule/` 会形成包级循环依赖。方向固定为 `schedule` → `service`。

模块声明保持 Node 元数据：

- 名称：`任务调度`；
- 描述：`任务调度模块，支持分布式任务，由redis整个集群的任务`；
- 加载顺序：`0`；
- 无模块中间件和全局中间件；
- 配置 `log.keepDays` 默认 `20`。

## 5. 数据模型

### 5.1 任务信息

表名固定为 `task_info`，字段与 Node 实体逐列对齐：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` / `updateTime` | `*gtime.Time` | 框架维护 |
| `jobId` | `*string` | 可空，定时器条目名 |
| `repeatConf` | `*string` | 可空，长度 1000，队列后端遗留 |
| `name` | `string` | 非空 |
| `cron` | `*string` | 可空 |
| `limit` | `*int64` | 可空，不参与调度 |
| `every` | `*int64` | 可空，毫秒 |
| `remark` | `*string` | 可空 |
| `status` | `int32` | 默认 `1`，0-停止 1-运行 |
| `startDate` / `endDate` | `*gtime.Time` | 可空 |
| `data` | `*string` | 可空 |
| `service` | `*string` | 可空，调用目标字符串 |
| `type` | `int32` | 默认 `0`，0-系统 1-用户 |
| `nextRunTime` | `*gtime.Time` | 可空 |
| `taskType` | `int32` | 默认 `0`，0-cron 1-时间间隔 |
| `lastExecuteTime` | `*gtime.Time` | 可空 |
| `lockExpireTime` | `*gtime.Time` | 可空，执行锁 |

`limit` 是 MySQL 保留字，写入和查询一律通过 GoFrame ORM 的标识符引用，不拼接裸 SQL。

### 5.2 任务日志

表名固定为 `task_log`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` / `updateTime` | `*gtime.Time` | 框架维护 |
| `taskId` | `*uint64` | 可空，带索引 |
| `status` | `int32` | 默认 `0`，0-失败 1-成功 |
| `detail` | `*string` | 可空，无长度约束映射为 TEXT |

不声明数据库外键。任务与日志的级联删除由 Service 在同一业务事务中完成。

## 6. HTTP 契约

前缀：`/admin/task/info`

| 路径 | 方法 | 权限 | 行为 |
| --- | --- | --- | --- |
| `/add` | POST | `task:info:add` | 新增并按状态注册定时器 |
| `/delete` | POST | `task:info:delete` | 移除定时器、删除任务及其日志 |
| `/update` | POST | `task:info:update` | 更新并重新注册定时器 |
| `/info` | GET | `task:info:info` | 详情 |
| `/page` | POST | `task:info:page` | 分页，`status`、`type` 等值过滤 |
| `/once` | POST | `task:info:once` | 立即执行一次 |
| `/stop` | POST | `task:info:stop` | 停止 |
| `/start` | POST | `task:info:start` | 开始 |
| `/log` | GET | `task:info:log` | 任务日志分页 |

Node Controller 只开放 `add/delete/update/info/page`，Go 版保持一致，不额外开放 `list`。`modules/base/menu.json` 里多出的 `task:info:list` 是 Node 菜单数据的历史冗余，不因此增加路由。

请求与响应：

- `/once`、`/stop` 请求体 `{ "id": number }`；
- `/start` 请求体 `{ "id": number, "type"?: number }`；
- `/log` 查询参数 `id`（必填）、`status`（可选）、`page`、`size`，响应 `{ list, pagination }`，列表项为 `task_log` 全列加 `taskName`，默认按 `id DESC` 排序，与 Node `entityRenderPage` 一致；
- 四条自定义路由都不加 `ignoreToken`，权限标识由现有路径推导规则产出。

## 7. 定时规则

Node 把任务配置翻译成 `cron` 库的表达式；Go 翻译成 `gcron` 模式：

| `taskType` | Node local | Go |
| --- | --- | --- |
| `0` cron | 原样使用 `cron` 字段 | 5 段表达式补秒位后使用 6 段模式 |
| `1` 间隔 | `*/{every/1000} * * * * *` | `@every {every/1000}s` |

补秒位对齐 Node `cron` 库接受 5 段表达式的行为；`@every` 取代 `*/N` 是因为 Node 写法在间隔超过 59 秒时会生成非法表达式。

`every` 不足 1000 毫秒按参数校验失败处理，Node 在这种输入下同样会因为生成 `*/0.5` 而抛错。

`nextRunTime` 的计算：

1. 间隔任务为当前时间加间隔秒数；
2. cron 任务按 6 段模式逐秒前向扫描，上限一年，扫描不到时保持为 `null`；
3. 模式含月份或星期英文名时不计算，保持为 `null`——`gcron` 支持这些写法，但它们在前端表单里不可达。

## 8. 执行语义

### 8.1 注册与同步

`Scheduler` 是模块生命周期组件：

1. `OnStart` 装载全部 `status = 1` 的任务并注册定时器；
2. `OnStop` 移除全部条目并等待在跑的任务结束；
3. `Sync(ctx, id)` 按任务当前行状态注册或移除单个条目，并刷新 `nextRunTime`；
4. `jobId` 在新增时用 `guid.S()` 生成，作为条目名的稳定前缀；实际条目名每次注册追加一个自增序号。

条目名不能跨次复用：`gcron` 结束一个条目时按名字从注册表里删除，正在执行的旧条目在收尾阶段会把同名的新条目一并删掉，而旧条目自己的 gtimer 仍在走，于是每重注册一次就多出一个还在跑但已不可管理的条目。

同理，定时回调内只刷新 `nextRunTime`，不重新注册条目——回调里重注册会稳定触发上面这条路径，让每秒一次的任务频率随执行次数翻倍。

定时器一律以干净的后台 Context 注册：`gcron` 在触发时复用注册期 Context，用请求事务 Context 注册会让任务在已关闭的事务上执行。

`Sync` 在 CRUD 事务内调用，事务回滚后可能残留条目。执行入口每次重新读取任务行，行不存在或 `status != 1` 时自行注销，残留条目因此自愈。

### 8.2 单次执行

1. 按 ID 重新读取任务，缺失时注销条目并返回；定时触发下任务已停止同样注销并返回，手动执行不看运行状态；
2. `startDate` 晚于当前时间或 `endDate` 早于当前时间时跳过；
3. 抢占执行租约：在框架事务内调用 `db.Runtime.LockRows(ctx, "task_info", []uint64{id})` 取行锁，锁内判定 `lockExpireTime` 是否仍被其他实例持有，未被持有则写入 `lastExecuteTime` 与新的 `lockExpireTime` 并提交；
4. 解析并调用 `service` 目标；
5. 成功写入 `status = 1` 日志和序列化结果，失败写入 `status = 0` 日志和错误消息；
6. 删除该任务超过 `log.keepDays` 天的日志；
7. 释放执行锁；
8. 非手动执行时刷新 `nextRunTime`。

租约时长固定 5 分钟，与 Node 一致。抢占用框架行锁而不是 Node 的条件更新加受影响行数判断：`LockRows` 已经封装了 MySQL/PostgreSQL 的 `SELECT ... FOR UPDATE` 与 SQLite 的写锁提升，判定与写入因此是同一把锁内的原子操作，不依赖各方言对"值未变化"时受影响行数的不同解释。释放租约和写日志不受调用失败影响。

任务真正的执行发生在抢占事务提交之后、结算事务开始之前，行锁不会横跨整个任务调用。

### 8.3 调用目标

Node 用 IoC 容器按 `taskDemoService.test(1,2)` 动态取实例并反射调用方法。Go 用静态注册表取代动态容器：

```go
type Callable func(ctx context.Context, arguments []any) (any, error)

func NewRegistry(demo *DemoService) (*Registry, error)
```

注册表键是 Node 的 `目标.方法` 字符串，值是显式登记的 Go 函数。新增任务目标等于在 `NewRegistry` 里加一行，不引入反射、不依赖运行时容器，并与项目的静态装配架构一致。

`service` 字符串按 `目标.方法(参数)` 解析：

1. 参数按顶层逗号切分，括号、方括号、花括号和引号内的逗号不切分；
2. 每个参数先按 JSON 解析，失败时按原始字符串传入；
3. 无参数写法 `方法()` 传入空参数表；
4. 键不存在时按业务异常返回，并作为失败日志落库。

顶层切分比 Node 的裸 `split(',')` 严格更强：Node 对 `test([1, 2])` 会切成 `[1` 和 `2]`，前端占位符里的示例在 Node 上本就不可用。

## 9. 写入请求体裁剪

`cool-admin-vue` 的任务表单用 `form: { ...item }` 打开，提交时回传整条记录，并额外带上列表渲染阶段附加的 `_every` 视图字段。Node 侧 TypeORM 静默忽略多余字段，Go 侧 Binder 明确拒绝未知字段，`Base` 也拒绝更新 `createTime`、`updateTime` 这类系统维护字段——不处理会让前端"编辑任务"直接失败。

对应 Node Controller 用 `before` 钩子改写请求体的做法，Go 版在 `CurdOption.Before` 上挂 `BodyNormalizer.Trim`：

1. 只处理 `add` 与 `update` 两个路径，`delete`、`page` 等请求体原样通过；
2. 按实体 Descriptor 的可写字段白名单裁剪，主键保留、系统维护字段与未知字段丢弃；
3. 对象与数组两种请求体形状都支持，字段值以 `json.RawMessage` 透传，不做二次编解码；
4. 请求体超过 1 MB 时原样交回 Binder，由 Binder 按自身上限报错。

裁剪器放在 Controller 层：架构设计 §10.4 禁止把 `*ghttp.Request` 传入领域 Service。

这只放宽了任务模块两个写入接口的字段严格性。其他 Vue 页面若同样以整条记录回传，属于框架 Binder 策略层面的统一问题，不在本模块处理。

## 10. 初始化、菜单与装配

`modules/task/db.json` 原样迁移 Node 的两条示例任务，保留显式 ID、`service` 字符串、`taskType`、`every`、`cron` 和 `status = 0`。

执行现有生成命令后，静态生成文件负责注册模块与两个实体 Descriptor、构造业务 Service 与生命周期组件、注册 Controller、嵌入 `db.json`、发布 Admin 路由和 EPS 元数据。

## 11. 错误与安全

1. 输入绑定、ID 校验、未知字段、分页参数和数据库错误继续使用现有 Controller、Service 和 `exception` 机制；
2. 任务执行失败只写入日志，不让 panic 逃逸出定时器回调；
3. 所有查询使用 GoFrame 参数化 ORM 条件，不拼接用户输入；
4. `service` 字符串只能命中注册表内的显式目标，不能触达任意 Go 符号；
5. 模块不开放任何公开路由，全部接口要求后台身份和菜单权限。

## 12. 验证

### 12.1 单元与契约测试

至少覆盖：

1. Controller 的路径、方法、CRUD、查询配置、标签与 EPS 投影；
2. 实体 Descriptor 字段、默认值和可空性；
3. 定时规则编译：cron 6 段、cron 5 段补位、间隔任务、非法间隔；
4. `nextRunTime` 前向扫描：秒级步进、分钟步进、扫描不到时返回空；
5. `service` 字符串解析：无参、多参、JSON 参数、嵌套结构、含逗号字符串、非法写法；
6. 注册表命中与未命中；
7. 权限推导覆盖四条自定义路由；
8. SQLite 上按真实 Schema 建表并跑通执行链路：成功与失败日志、租约释放、状态与时间窗跳过、手动执行已停止任务、任务不存在报错、日志分页与关联任务名、`gcron` 真实触发一次间隔任务；
9. 请求体裁剪只作用于写入路径，且裁剪后只剩实体可写字段。

### 12.2 工程门禁

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./modules/task/... -count=1
go test ./... -count=1
go vet ./...
gofmt -l modules/task
```

## 13. 验收标准

1. `cool-admin-vue` 不需要任何代码或配置修改；
2. Node 任务模块前端用到的全部接口在 Go 后端可用；
3. 前端任务列表可以完成新增、编辑、启停、立即执行、删除和日志查看；
4. 服务重启后 `status = 1` 的任务自动恢复调度；
5. 多实例部署时同一任务在同一时刻只被一个实例执行；
6. 任务执行结果按成功和失败落库，超期日志被清理；
7. EPS 发布任务契约，权限标识与 `modules/base/menu.json` 已有数据一致；
8. MySQL、PostgreSQL 和 SQLite 使用相同业务代码，不引入方言 SQL；
9. 全量测试、静态检查和格式检查通过。
