# 重复造轮子专项审查

> 日期：2026-08-14  
> 模式：ponytail full  
> 结论：当前生产代码存在一批可以由包内唯一事实来源、Go 标准库、GoFrame 或现有依赖替代的重复实现。建议先做低风险删除，再处理跨包能力收敛；不建议为了“统一”新增通用工具箱。

## 1. 审查范围

- 审查对象：当前工作区状态，不仅是 `HEAD`；包含尚未提交的生产代码改动。
- 覆盖：139 个生产 Go 文件，共 31,204 行。
- 排除：全部 `*_test.go`、`test/`、`testdata/`、golden 文件及测试专用辅助代码。
- 方式：三条并行包级审查后，由主审交叉验证调用链、可见性、依赖版本和替代 API。
- 核验版本：Go 1.26、GoFrame v2.10.2、`golang.org/x/tools` v0.48.0、`github.com/google/uuid` v1.6.0。
- 本次只审查并产出本文档，没有修改业务代码，也没有运行测试。

收益表示可删除代码、减少双重维护或修复边界错误的综合价值，不等同于线上故障严重度。

## 2. 结论摘要

| 优先级 | 发现 | 最小替代 | 保守净删除 | 风险 |
|---|---|---|---:|---|
| P0 | `QueryPlan` 编译后又被完整校验 | 信任包内私有不可变计划 | 约 100 行 | 低 |
| P0 | UUIDv7 生成/校验实现三次 | `google/uuid` v1.6.0 | 约 65-80 行 | 低至中 |
| P0 | 发布与消费退避算法逐行重复 | 包内一个 `retryDelay` + `math/rand/v2` | 约 25-35 行 | 低 |
| P0 | typed-nil 反射器实现六次 | GoFrame `g.IsNil` | 约 50-60 行 | 低 |
| P0 | 相对路径安全判断实现多次 | `filepath.IsLocal` | 约 12-20 行 | 低至中 |
| P0 | Codegen 重复格式化、复制、拼 `GOFLAGS` | `format.Source` 一次、`BuildFlags` | 约 20 行 | 低 |
| P0 | Session 在 `auth` 与 `auth/session` 维护两套同构模型 | 根包端口直接使用现有 `session.Session` | 约 110-125 行 | 中高 |
| P0 | Outbox 保留已被双队列认领替代的合并 Claim 路径 | 删除旧接口、SQL 和探测残余 | 约 50-60 行 | 低至中 |
| P0 | Go 1.26 下仍处理 `crypto/rand.Read` 不可能返回的错误 | 保持字节数与编码，删除虚假失败传播 | 约 30-40 行 | 低至中 |
| P1 | Trace ID 同时写 GoFrame 和私有 Context | `gtrace` 作为唯一事实来源 | 约 35-50 行 | 低至中 |
| P1 | Application Assembly 同一不变量扫描三遍 | 一个返回合法前缀的 walker | 约 35-55 行 | 中 |
| P1 | Outbox 自建第二套三数据库 Schema 探测 | 收敛到 `db/schema` | 净删约 100-160 行 | 中高 |
| P1 | CLI 重写应用 YAML 读取和节点筛选 | 复用 `core/app` 配置入口 | 约 70-100 行 | 中 |
| P1 | 索引 DDL 与自增类型规则维护两份 | `driver.Dialect` 单一编译入口 | 约 30-35 行 | 低至中 |
| P1 | 同一 Envelope 在可信链路重复恢复/校验 | 只在外部输入边界校验一次 | 约 20-30 行 | 低 |
| P1 | 数据库能力和 MySQL 引擎重复探测 | Runtime 统一登记事务表 | 约 25-30 行 | 低至中 |
| P1 | Consumer Runtime、Adapter、Host 重复维护生命周期状态 | Runtime 只负责编排，状态留给 Adapter/Host | 约 25-35 行 | 低至中 |
| 条件项 | Inbox 幂等插入手写三种方言 | GoFrame `InsertIgnore` | 约 40-60 行 | 中高 |

低风险项目保守可净删约 330-430 行；完成跨包能力收敛和 Session 模型收敛后，总量预计超过 650 行。表中项目存在交叠，不能简单相加。

## 3. 根因

1. **私有能力没有成为唯一入口。** `core/app` 的配置拆分、`db/schema` 的数据库结构读取都没有提供上层需要的窄接口，CLI 和 Outbox 只能再实现一遍。
2. **不信任包内已经编译完成的私有值。** `QueryPlan` 和 `Envelope` 字段不可由外部修改，但执行阶段仍重复完整校验，形成“编译一次、验证两到三次”。
3. **新增模块没有先搜索仓库和依赖。** UUIDv7、typed nil、线性 contains、路径安全判断、退避算法均已有标准库、GoFrame、现有依赖或同包实现。
4. **测试注入思路渗入生产实现。** 多处通过函数变量、二次复制和重复防御保证可测性；其中部分隔离没有对应的生产并发或可变边界。
5. **旧路径在新实现落地后没有删除。** Outbox 已改为 available/expired 双队列公平认领，但旧的合并 Claim 接口和 SQL 仍作为无生产调用的第三套事实来源存在。

## 4. 详细发现

### F01：删除 `QueryPlan` 的第二次完整校验

**证据**

- `cool-next/crud/apply.go:35` 每次执行先调用 `validatePlan`。
- `cool-next/crud/apply.go:86-224` 用约 139 行重新校验表、别名、字段、操作符、值和排序方向。
- `cool-next/crud/plan.go:28-93` 的 `QueryPlan` 及全部节点字段均为私有。
- 唯一生产构造路径是 `cool-next/crud/plan.go:95` 的 `compileQueryPlan`，结果只写入 `cool-next/crud/action_plan.go:93` 的私有字段。

**根因**：把内部编译产物继续当作不可信外部输入。

**建议**：保留 `ctx/model/plan != nil` 三个执行边界检查，删除 `validatePlan`、`validateCondition`、`isValidColumn` 和 `planTableNamePattern`。若未来开放计划反序列化或导出构造器，再在那个边界恢复校验。

**收益/风险**：净删约 100 行。风险低；变化仅影响包内非法手工构造的状态，公开 API 无法产生这种状态。

### F02：UUIDv7 交给现有依赖

**证据**

- `cool-next/outbox/outbox.go:516-575` 手工布局 UUIDv7 位、编码文本并校验。
- `cool-next/outbox/store/store.go:234-250` 再次手写规范文本、版本和 variant 校验。
- `cool-next/outbox/store/probe.go:416-430` 第三次手写 UUIDv7 生成。
- 当前依赖树已有 `github.com/google/uuid v1.6.0`，提供 `NewV7`、`NewV7FromReader`、`Parse`、`UUID.Version`、`UUID.Variant` 和规范小写 `String`。

**根因**：Outbox 各层独立实现同一标识符契约，没有先检查已有依赖。

**建议**：生产生成使用 `uuid.NewV7()`；严格输入校验使用 `uuid.Parse` 后检查 `Version()==7`、RFC 4122 variant，并要求 `parsed.String()==input`。Outbox 与 Store 分别直接调用依赖即可，不要为三个调用点再建 UUID facade。

**收益/风险**：净删约 65-80 行。库实现额外提供单调序列；错误文本和私有时间源注入方式会变化，公开字符串格式不变。

### F03：合并发布与消费退避算法

**证据**

- `cool-next/outbox/worker.go:513-532`。
- `cool-next/outbox/consumer.go:236-255`。

两段代码的倍增、上限、50%-100% 抖动完全相同，只是从不同 Config 字段取值。两者还都用 `crypto/rand.Int` 和 `math/big`，为非安全用途保留了不会带来业务价值的失败分支。

**建议**：同包保留一个 `retryDelay(base, maximum time.Duration, attempt uint32) time.Duration`，抖动使用 Go 1.26 标准库 `math/rand/v2.Int64N`。安全令牌继续使用 `crypto/rand`，不要混用。

**收益/风险**：净删约 25-35 行，且退避计算不再返回随机源错误。风险低，保持相同区间即可。

### F04：typed nil 直接使用 GoFrame

**证据**

以下六个 helper 都在用反射判断 interface 中的 typed nil：

- `cool-next/core/entity/validate.go:108`
- `cool-next/core/controller/controller.go:544`
- `cool-next/core/service/input.go:573`
- `cool-next/crud/plan.go:785`
- `cool-next/db/recycle/descriptor.go:38`
- `cool-next/eps/eps.go:1302`

GoFrame v2.10.2 的公开 `frame/g.IsNil` 已覆盖 nil、Chan、Map、Slice、Func、Interface、Pointer 和 UnsafePointer。

**建议**：调用点直接用 `g.IsNil`，删除六个 helper；不要新增 `core/util.IsNil`，否则只是把已有轮子换成自己的共享轮子。

**收益/风险**：净删约 50-60 行。差异是 `g.IsNil` 还识别 nil UnsafePointer 和作为参数传入的 `reflect.Value`；当前调用点均为 Descriptor、Field、Handler、Context、Hook 或目标指针，风险低。

### F05：用 `filepath.IsLocal` 统一路径安全判断

**证据**

- `cool-next/core/module/identity.go:41-59` 手写绝对路径、卷名和 `.`/`..` 分段检查。
- `cool-next/codegen/analyze.go:97`、`165-176` 手写根目录逃逸检查。
- `cool-next/codegen/query_validate.go:22-24` 再写一次 `Rel + ..` 前缀判断。
- `analyze.go:97` 的 `strings.HasPrefix(options.ModulesRoot, "..")` 会误拒绝合法本地目录名 `..cache`。

Go 1.26 `filepath.IsLocal` 的契约正是：非空、非绝对、Clean 后不含 `..`，且 `Join(base, path)` 不会逃逸 base。

**建议**：入口以 `filepath.IsLocal` 为主；需要拒绝未清理文本时额外保留 `filepath.Clean(path)==path`，模块身份继续显式拒绝 `.`。

**收益/风险**：净删约 12-20 行，并修复 `..cache` 误判。Windows 保留名会新增为拒绝项，属于更安全但可观察的行为变化。

### F06：删除 Codegen 的重复工作

**证据与替代**

1. `cool-next/codegen/render.go:151-155` 已 `format.Source`，`pipeline.go:46-54` 又格式化并复制一次。`Render` 的生产调用点只有这里。保留第一次格式化，两个路径直接返回该切片。
2. `cool-next/codegen/typecheck.go:79-89` 遍历 `os.Environ` 拼接 `GOFLAGS=-mod=readonly`；`packages.Config.BuildFlags` 就是 x/tools 的官方构建参数入口。
3. `cool-next/codegen/analyze.go:39` 已深拷贝 overlay，`load.go:32` 又完整拷贝；`packages.Load` 文档承诺不修改 Config，保留入口一次复制即可。

**收益/风险**：约 20 行和两次不必要的字节复制。风险低；需保留 `Render` 的 CG079 诊断，并验证现有外部 `GOFLAGS` 与显式 `BuildFlags` 的组合。

### F07：让 GoFrame Trace Context 成为唯一事实来源

**证据**

- `cool-next/core/app/request.go:12-68` 自建 `requestContextKey`、`requestInfo`、读取器和 W3C Trace ID 校验。
- `cool-next/core/app/http/context.go:68-81` 已先调用 `gtrace.GetTraceID/WithTraceID`，随后又用 `app.WithTraceID` 写一份相同 ID。
- `cool-next/grpc/context.go:118-132` 重复同样的双写流程。

GoFrame v2.10.2 `gtrace.WithTraceID` 使用 OpenTelemetry `TraceIDFromHex` 校验并写入 SpanContext，`gtrace.GetTraceID` 已是读取入口。

**建议**：保留 `app.WithTraceID/TraceID` 公开 API 作为薄封装，内部委托 `gtrace`；HTTP/gRPC 只调用 app API 一次。`app.NewTraceID` 可暂时保留，因为 GoFrame 没有对应的独立生成函数，CLI 也用它生成操作 ID。

**收益/风险**：净删约 35-50 行。`app.TraceID` 将能读取任何合法 OTel Span 的 Trace ID；若大写输入必须继续拒绝，需在薄封装保留一行规范文本检查。

### F08：Application Assembly 只遍历一次

**证据**

- `cool-next/core/app/lifecycle.go:139-184`：完整校验并收集 Transport。
- `cool-next/core/app/lifecycle.go:186-217`：再次校验合法前缀。
- `cool-next/core/app/lifecycle.go:219-249`：第三次寻找最长合法前缀。

三处重复组件顺序、Lifecycle hooks 和 Transport 对齐判断，差别只是成功路径需要完整结果，失败路径需要最长合法前缀回滚。

**建议**：保留一个 walker，返回 `prefix/components/transports/firstError`；成功路径检查是否走完，失败路径直接用 prefix 回滚。不要放松“只回滚最长合法前缀”的约束。

**收益/风险**：净删约 35-55 行。风险中，必须保持 Transport 名称非空/唯一和失败组件不得进入 stopper 的现有语义。

### F09：合并 Outbox 与 `db/schema` 的结构读取

**证据**

- `cool-next/outbox/store/probe.go:121-289` 自建 MySQL `information_schema`、PostgreSQL `pg_catalog`、SQLite `PRAGMA` 三套表/主键/索引读取。
- `cool-next/db/schema/inspect.go:14-197` 已维护同样的三数据库元数据读取。

**根因**：`db/schema` 的 inspector 为私有且现有 `Table` 没有完整表达列顺序与复合主键顺序，Outbox 为更严格的启动契约复制了整套数据库访问。

**建议**：扩充 `db/schema` 的只读检查结果，使其保留列顺序和主键顺序；导出一个面向 `schema.Definition` 的窄验证入口。Outbox 只提交 `OutboxDefinition/InboxDefinition`，DML 能力探测仍留在 Store。

**收益/风险**：Outbox 侧可删约 150-190 行，考虑共享层补充后净删约 100-160 行。风险中高，不能直接改用现有 `Manager.Apply`，否则会丢失 Outbox 的精确列序契约。

### F10：CLI 复用应用的正常配置入口

**证据**

- `cool-next/core/app/configuration.go:70-166` 负责单文档 YAML 解码、顶层键检查、模块拆分和节点编码。
- `cmd/cool/outbox.go:380-480` 再次实现单文档检查、重复键检查、`cool.outbox/database` 子树选择和 YAML 编码。
- 架构要求 `cool outbox` 使用应用的正常配置，但当前两条路径会独立演进。

**建议**：从 `core/app` 暴露一个只读加载入口，接收生成的 Definition 并返回 `AssembleInput`；CLI 使用 `RootSource()` 后交给现有 `configuration.Load`。不要再公开 YAML 节点 helper。

**收益/风险**：CLI 可删约 70-100 行。风险中，必须保持 `COOL_CONFIG_FILE`、未知模块、重复键和环境变量的现有错误语义。

### F11：数据库 DDL 规则只保留一份

**证据**

- `cool-next/db/driver/ddl.go:262-297` 与 `cool-next/db/schema/schema.go:122-155` 各自拼接索引 DDL。
- `cool-next/db/driver/ddl.go:80-96` 与 `cool-next/db/schema/expected.go:54-65` 各自维护三方言自增列类型。

**建议**：让 `driver.Dialect` 提供唯一的单索引编译入口，以及一个包含自增语义的有效 DDL 类型入口；建表和增量补索引共同调用。不要扩大为通用 migration builder。

**收益/风险**：净删约 30-35 行。风险低至中；保持索引遍历顺序、未知索引错误和已导出 `ColumnType` 的旧语义。

### F12：可信 Envelope 不重复恢复

**证据**

- `cool-next/outbox/outbox.go:367` 的 `Restore` 校验后，私有 `newEnvelope` 在 `outbox.go:396` 再校验一遍。
- `cool-next/outbox/consumer_adapter.go:447` 已 Restore，`consumer.go:194` 又拆开同一个不可变 Envelope 并再次 Restore。
- Envelope 所有字段私有，外部只能通过 `New/Restore` 得到非零有效值。

**建议**：`newEnvelope` 只做防御性复制；Deliverer 只检查订阅的 topic/type/version 匹配。外部 Broker 输入仍必须在 Adapter 边界完整校验。

**收益/风险**：净删约 20-30 行。风险低，零值仍会在订阅匹配处失败。

### F13：数据库能力和引擎探测收敛到 Runtime

**证据**

- `coredb.New` 已执行 `db/driver.Probe` 并保存 `Diagnostic`。
- 生成应用路径已经把 Outbox/Inbox 加入 `TransactionTables`。
- `cool-next/outbox/store/probe.go:50-77` 再检查 capabilities，`probe.go:290` 再查询 MySQL 表引擎。
- CLI 路径 `cmd/cool/outbox.go:365` 创建 Runtime 时没有传 Outbox/Inbox 事务表，导致 Store 只能再次补探测。

**建议**：CLI 创建 Runtime 时登记两个内部表；随后删除 Store 的 capability 与 MySQL engine 重复检查，只保留结构契约和实际 DML 回滚探测。

**收益/风险**：净删约 25-30 行和一次启动查询循环。风险低至中，必须先修复 CLI Runtime 参数再删除 Store 检查。

### F14：`InsertIgnore` 仅作为条件项

**证据**

- `cool-next/outbox/store/database.go:551-590` 按方言处理 Inbox 幂等写入。
- `cool-next/outbox/store/sql.go:490-510` 手写 PostgreSQL/SQLite `ON CONFLICT`。
- GoFrame `Model.InsertIgnore` 会由三个现有驱动生成 MySQL `INSERT IGNORE`、PostgreSQL `ON CONFLICT DO NOTHING`、SQLite `INSERT OR IGNORE`。

**为什么不直接建议替换**：MySQL `INSERT IGNORE` 不只忽略重复键，还可能把截断、类型转换等错误降为 warning；当前实现仅吞错误码 1062，安全语义更窄。

**建议**：只有在严格 SQL mode、结构化 DO 写入和输入预校验共同证明其他 warning 不可发生时，才使用 `InsertIgnore + RowsAffected`。否则保留现状，最多只共享 PostgreSQL/SQLite 的 SQL 生成。

**收益/风险**：可删约 40-60 行，但风险中高；这不是首批简化项。

### F15：删除 Session 双模型和纯转换 Adapter

**证据**

- `cool-next/auth/token.go:54-68` 已定义认证内核需要的 `SessionSnapshot` 和 `SessionStore`。
- `cool-next/auth/session/session.go:16-143` 又定义一套字段同构的 `Session`、`Store`、两个构造器、九个读取器和重复 sentinel。
- `cool-next/auth/session/adapter.go:11-129` 的唯一职责是逐字段双向转换两套 Session，并把 `ErrNotFound/ErrRefreshReplay` 映射为根包中同义的错误。
- `Adapter.Save/RotateRefresh` 为了模型转换再次进入 `NewAdmin/NewApp`，随后 `MemoryStore.Save/RotateRefresh` 或 Redis `encode` 又按 Store 时钟校验；应保留“不可变值构造”和“写入时有效性”两道边界，但不需要由 Adapter 为同构字段额外制造一次转换。
- 当前只有 `adapter.go` 从 `auth/session` 导入根 `auth`；删除 Adapter 后可反转这条依赖，由根 `auth` 使用 `session.Session`，而 `auth/session` 继续只依赖中立的 `auth/contract`，不会形成循环。

**根因**：为“领域模型”和“持久模型”各建一套内存结构，但持久化差异实际已经由 `wireSession` 负责，内存层 Adapter 没有增加边界语义。

**建议**：保留字段私有、通过构造器校验的现有 `session.Session`；删除 `adapter.go` 后，让根 `SessionStore` 直接读写该类型，认证 Service 通过其构造器和 getter 使用。Memory/Redis 因方法签名相同可直接满足根端口；根包 sentinel 直接别名到 `session.ErrNotFound/ErrRefreshReplay`，不再转换。必须保留 `session.Store` 的 `RevokeUser`、不可变构造器、Store 写入时钟校验、codec 恢复校验和 `RoleIDs` 防御性复制；不要为了上移私有字段反而新增导出的 Restore/Validate/Clone API。

**收益/风险**：保守净删约 110-125 行，并删除 `adapter.go` 一个生产文件。风险中高，`SessionSnapshot`、`session.Session`、`SessionStore` 方法签名和 `NewAdapter` 都是导出 API；Redis wire JSON、Key、TTL、原子 Lua、错误身份和安全校验都必须保持不变。

### F16：删除 Outbox 已失效的合并 Claim 路径

**证据**

- `cool-next/outbox/store/store.go:211-227` 同时暴露旧 `Claim` 和新 `ClaimAvailable/ClaimExpired`。
- `cool-next/outbox/worker.go:270-320` 的唯一生产认领链路只交替调用后两个方法，以避免过期 Lease 饿死新消息或反之；全仓生产代码没有 `.Claim(` 调用。
- `cool-next/outbox/store/sql.go:14-22`、`75-94`、`121-144` 仍保留合并候选查询和合并条件更新，重复维护 available 与 expired 两套条件。
- `DatabaseStore.Claim` 仅调用这套旧 SQL；旧更新语句最后一个生产消费者是 `cool-next/outbox/store/probe.go:347-348` 的 DML 探测。

**根因**：双队列公平认领替换旧算法时只新增路径，没有删除被替代路径。

**建议**：删除 `Store.Claim`、`DatabaseStore.Claim`、`claimCandidates` 和 `claim`。Probe 插入的是 pending 记录，直接使用实际生产路径的 `claimAvailable` 更新语句；不要为了 Probe 保留一套 Worker 永远不执行的 SQL。

**收益/风险**：净删约 50-60 行，并消除第三套认领条件。风险低至中；Store 公开接口会收窄。现有 Probe 本来也没有独立覆盖 expired 更新，若需要该能力应直接探测 `claimExpired`，而不是保留旧合并语句。

### F17：Consumer Runtime 不重复保存 Adapter/Host 状态

**证据**

- `cool-next/outbox/consumer_runtime.go:22-25`、`41-126` 通过 mutex、`isPrepared`、`isRunning` 再建一层状态机。
- `cool-next/outbox/consumer_adapter.go:132-165`、`179-209`、`221-254` 已完整保护 Prepare/Start/Stop 顺序、并发和在途排空。
- `cool-next/core/app/lifecycle.go:91-132`、`313-350`、`381-398` 又保证 Host 串行执行 OnInit/OnStart、成功初始化后登记一次 Stopper，并在启动后读取终止通道。
- `codegen/render.go:1052-1054` 总是把同一个 Runtime 同时登记为 Initializer、Starter、Stopper 和 Supervisor，没有绕过 Host 的第二条生产路径。
- Runtime 在 `Adapter.Start` 失败后再次调用 `Adapter.Stop`；对内置 `BrokerConsumerAdapter` 而言，Start 失败路径已自行 Stop，第二次只是 no-op，但自定义 Adapter 不一定具有这个行为。

**建议**：Runtime 只保留 `adapter/store/deliverer/terminated` 和编排逻辑；OnInit 做 Probe + Prepare，OnStart 保存 Adapter 返回的 channel，OnStop 直接委托 Adapter，Terminated 直接返回。删除 Runtime 自身状态时，仍须保留 Prepare/Start 失败后的 Stop 清理和 nil 终止通道校验，因为 `ConsumerAdapter` 允许自定义实现。保留 Adapter 内部状态机和 Host 生命周期约束，它们才分别拥有 Broker 并发状态和应用顺序。

**收益/风险**：净删约 25-35 行。风险低至中；公开方法仍存在，但绕过注释中明确的 Host 管理、并发重复调用 Runtime 的未支持用法将不再得到第二层错误提示。

### F18：删除 Go 1.26 下不可达的随机源错误分支

**证据**

以下生成器都在检查 `crypto/rand.Read` 的 error，并把它扩散到调用链：

- `cool-next/core/app/request.go:20-26`
- `cool-next/auth/service.go:256-263`
- `cool-next/auth/jwt/service.go:314-321`
- `cool-next/outbox/worker.go:535-542`
- `cool-next/outbox/store/database.go:659-665`
- `cool-next/outbox/store/probe.go:416-430`
- `cool-next/db/driver/probe.go:148-155`

Go 1.26 标准库对 `crypto/rand.Read` 的契约是：始终填满切片且永不返回错误；底层系统随机源异常时进程不可恢复地终止。因此这些 error 分支在当前 toolchain 下不可达，`func() (string, error)` 注入又让 Auth/JWT 业务流程继续处理一个生产中不可能出现的失败。

**建议**：保持现有随机字节数和十六进制格式，只删除 error 返回及私有调用链中的对应分支；不要用更短 token 换行数，也不要合并成新的全局 ID 工具箱。首批精简应保留已导出 `app.NewTraceID` 的 `(string, error)` 签名并固定返回 nil；改签名放到 F07 或下一个主版本。

**收益/风险**：私有路径净删约 30-40 行，不降低 Session/JTI/Claim Token 熵，也不改变数据库或 Token 格式；`outbox/store/probe.go` 的 UUIDv7 生成器同时属于 F02，总收益不能重复计算。风险低至中；受影响的是用于模拟系统随机源失败的测试注入，不是可恢复的生产故障。

## 5. 小型标准库替换

这些项目可以和相关包的主要修改一起做，不值得单独建抽象：

- 五个 `contains/containsString/containsAPI`：`eps.go:1270`、`db/schema/inspect.go:199`、`core/route/route.go:527`、`core/controller/plan.go:83`、`codegen/route_analysis.go:809`，直接用 `slices.Contains`，约删 35-40 行。
- `eps.go:1280` 的浅 map 复制与 `outbox.go:508` 的 Header 浅复制，直接用 `maps.Clone`。
- 大量 `append([]T(nil), source...)` 可机械改为 `slices.Clone`；Go 1.26 文档明确保持 nilness。只改一眼可证为浅复制的调用，不建立 Clone helper。
- `db/driver/version.go:20` 在已经匹配 `x.y[.z]` 后每次编译第二个正则，直接 `strings.Split(matched, ".")`。
- `core/configuration/configuration.go:77-97` 的 `preservedCause` 重复标准错误链；用项目现有 `gerror.Wrap`。该项已在 `docs/superpowers/plans/2026-08-10-core-package-simplification.md` 记录但尚未执行。
- `core/entity/compile.go:146` 的 `baseFields` 硬编码重复 `entity.Base` 标签事实来源；同样已在既有精简计划记录。

## 6. 不应删除的实现

以下代码虽然长或与框架能力相似，但当前没有等价替代，不列为重复造轮子：

- `core/configuration` 的 presence-aware 配置树：设计要求区分 absent/null/zero、拒绝重复键并生成稳定 JSON；GoFrame `gcfg/gconv` 不能直接保持这些语义。
- `db/tx` 的 rollback-only 与首错传播：GoFrame事务传播不能等价覆盖“内层错误被吞后外层仍回滚”。
- CRUD 查询 AST、Binder 与 Native SQL 静态检查：它们承担字段白名单、参数化和源码期安全边界，不是普通 SQL Builder 的重复包装。
- gRPC 自定义生命周期：`grpcx.Start/Run` 的信号/Fatal 行为不能满足当前 Host 的错误通道、Ready Gate 和超时停止契约。
- Outbox/Inbox 状态机及数据库特定认领 DML：这是 at-least-once、lease、SKIP LOCKED 和幂等事务的核心语义，不能用普通队列 helper 替换。
- Session 版本化 JSON：跨语言 uint64、字段集合、Key/Session ID 和过期校验均是明确安全契约。
- `core/exception` 的 `BaseException`：虽然 GoFrame `gcode.Detail` 能承载状态码，但直接改用 `gerror.WrapCode` 会让底层 cause 进入 `Error()` 文本，破坏 CLI 和普通日志路径的脱敏边界；`BaseException` 还是公开的 `errors.As` 契约，不能为少量行数直接删除。
- Codegen 原子文件操作和 HTTP `serverRuntime` seam：两者生产中都只有一个实现，但分别覆盖原子替换失败和并发生命周期故障。测试文件不在本次范围内，无法证明这些检查可由同等小且可靠的真实环境测试替代，因此不列为首批删除项。

## 7. 文件碎片补充审查

> 补充日期：2026-08-15

当前 139 个生产 Go 文件中，5 个不超过 20 行，13 个不超过 40 行，22 个不超过 60 行。数量上确实存在碎片，但短文件只占约 16%，全项目平均每个生产文件约 224 行；主要问题不是文件总数，而是少数文件没有独立职责。

### 建议直接删除或合并

| 当前文件 | 处理 | 目标位置 | 原因 |
|---|---|---|---|
| `cool-next/db/tx/errors.go` | 删除 | 无 | 只有一行 `package tx`，是异常模型重构后的空壳 |
| `cool-next/db/tx/types.go` | 拆并后删除 | `runner.go`、`scope.go` | `Callback/Runner` 属于 runner，`scopeContextKey/Current` 属于 scope；当前文件反而混合两个职责 |
| `cool-next/db/diagnostic.go` | 合并 | `runtime.go` | `Diagnostic` 只由 Runtime 创建、保存和返回，生产代码没有第二个所有者 |
| `cool-next/db/validate.go` | 合并 | `runtime.go` | `validateConfig` 只有 `db.New` 一个调用点，是 Runtime 构造流程的一部分 |
| `cool-next/db/recycle/descriptor.go` | 合并 | `types.go` | Record、内部 Descriptor 和 Registry 是同一组回收记录元数据；合并后约 74 行 |
| `cool-next/core/app/assembly.go` | 合并 | `definition.go` | `Assembly` 与 `Definition/AssembleFunc` 共同定义静态装配输入输出；合并后约 121 行 |
| `cool-next/auth/session/adapter.go` | 删除 | 根端口直接使用 `session.Session` | 只做同构 Session 的逐字段转换和 sentinel 映射 |

原六项无公开 API 变化，可先把生产文件从 139 个降到约 133 个。Session 模型收敛后可到约 132 个。`auth/contract/kind.go` 在这个最小依赖方向下仍用于避免根 `auth` 与 `auth/session` 成环，不应为凑文件数删除。`crud/errors.go` 也只有 12 行，但 `panicCore` 被 `ast.go`、`query.go`、`action_plan.go` 共同使用；保留一个明确的包级共享 helper 文件比随意塞进任一调用文件更清楚，暂不合并。

### 短但应保留

- `main.go`：进程入口必须独立。
- `auth/contract/kind.go`：在 F15 的最小收敛方案中仍用于避免根 `auth` 与 `auth/session` 的环，不能只因为行数少就合并或删除。
- `core/entity/base.go`：业务实体嵌入的公开事实来源，标签本身就是核心契约。
- `core/exception/code.go`：稳定错误码、消息和异常类别的集中目录。
- `core/module/lifecycle.go`、`core/app/transport.go`：协议无关的生命周期和 Transport 契约，边界清晰。
- `core/configuration/merge.go`：虽短，但完整承载一个独立的递归合并算法。
- `db/schema/types.go`、`db/driver/types.go`：公共数据库模型与实现分离，合并会混淆依赖方向。
- `db/driver/quote.go`、`auth/resource.go`、`codegen/diagnostic.go`：各自都是可独立理解和复用的完整能力。

### 不采用按行数合并

项目已经有 `codegen/render.go` 约 1,409 行、`eps/eps.go` 约 1,313 行、`core/controller/binder.go` 约 931 行等大文件。把所有短文件机械并入这些热点，只会减少目录项，却增加阅读和冲突成本。合并标准固定为：同一所有者、同一变化原因、合并后仍容易定位；只满足“代码少”不合并。

## 8. 建议执行顺序

1. 第一批：F01、F03-F06、F16、F18 和“小型标准库替换”。这些改动边界封闭，主要是删除失效路径或直接使用既有契约。
2. 第二批：F02、F07、F11-F13、F17。逐项跑对应包测试和三数据库集成测试。
3. 第三批：F08-F10、F15。先冻结合法前缀、列/主键顺序、配置错误语义和 Session wire 契约，再收敛入口。
4. 暂缓 F14，除非先证明 MySQL `INSERT IGNORE` 不会放宽失败语义。

按 ponytail 原则，首轮不创建 `core/util`、通用 Repository、Migration Builder 或配置 facade 集合；现成 API 能解决的直接调用，跨包缺口只暴露当前调用方真正需要的一个窄入口。
