# 模块 09：Framework Database Group 设计

> 日期：2026-08-05
> 状态：待复核
> 模块：09 Framework Database Group
> 对应拆分项：09.1-09.9
> 前置模块：03 配置加载与基础校验、06 数据库方言与能力探测、07 Schema 同步与校验、08 事务 Scope 与 Runner

## 1. 目标

本模块在 `cool-next/db` 建立唯一的 Framework Database Group Runtime。它将上层已经由 `configuration` 解码的 `cool.outbox.databaseGroup` 与对应 GoFrame 数据库组绑定，完成启动探测，并为后续 Base、CRUD、Route、Recycle、Outbox 和 Inbox 提供相同的数据库对象、Dialect 和事务 Runner。

Framework Group 固定来自 `cool.outbox.databaseGroup`。该约束不是可选策略：实体、路由、Service 或调用点均不得替换它；其他 GoFrame 组只能在框架事务边界之外由业务显式使用。

本模块必须：

1. 验证 Framework Group 名、GoFrame 连接节点和需保障事务的表名；
2. 在创建 Runtime 时注册并获取指定 GoFrame 组；
3. 在应用 Ready 前调用模块 06 `driver.Probe`；
4. 集中提供 DB、Dialect、Runner、能力报告和不含敏感数据的诊断；
5. 查询 Context 中的 Scope 时拒绝不同组，不回退到全局 DB；
6. 无论 Schema Mode 是否为 `off`，都执行数据库版本、能力与 MySQL InnoDB 探测；
7. 为后续 Recycle、Outbox、Inbox 将其内部表加入事务保障集合保留入口。

本模块不读取 YAML 文件、不实现 Application Host Ready、Schema Apply、实体注册、CRUD、内部表 DDL 或 Outbox/Inbox 本身。上层配置加载负责把 `cool.outbox.databaseGroup` 和 `database.<group>` 转为本模块输入；Application Host 负责在 Ready 前调用构造入口。

## 2. 配置边界

配置事实来源固定为：

```yaml
database:
   framework:
      type: mysql
      host: 127.0.0.1
      port: "3306"
      user: cool
      pass: ${DATABASE_PASSWORD}
      name: cool_admin

cool:
   outbox:
      databaseGroup: framework
```

模块 03 解码根应用配置后，上层将下面的值传给本模块：

```go
type Config struct {
    Group             string
    Nodes             gdb.ConfigGroup
    TransactionTables []string
}
```

`Config.Group` 必须恰好来自 `cool.outbox.databaseGroup`，而不是由 Entity、Route 或任意业务调用传入。`Nodes` 必须是同名 `database.<group>` 的 GoFrame 节点集合；`TransactionTables` 是生成 Descriptor 已知且参与 Framework 事务的业务物理表。后续 Recycle、Outbox、Inbox 在 Host 组装 Config 时追加其内部表名。

本模块不重复实现模块 03 的 YAML、环境变量、未知字段或类型校验。它只验证运行时约束：Group 非空、Nodes 非空、Group 与取得的 `gdb.DB.GetGroup()` 相同、表名为模块 06 所接受的单段标识符且无重复。

## 3. 公开接口

```go
type Config struct {
    Group             string
    Nodes             gdb.ConfigGroup
    TransactionTables []string
}

type Diagnostic struct {
    Group        string
    Kind         driver.Kind
    Version      driver.Version
    Capabilities driver.Capabilities
    Tables       []string
}

type Runtime struct { /* 私有字段 */ }

func New(ctx context.Context, config Config) (*Runtime, error)
func (r *Runtime) Group() string
func (r *Runtime) DB() gdb.DB
func (r *Runtime) Dialect() driver.Dialect
func (r *Runtime) Runner() tx.Runner
func (r *Runtime) Current(ctx context.Context) (gdb.TX, bool, error)
func (r *Runtime) Diagnostic() Diagnostic
```

所有 Runtime 访问器只返回构造时冻结的对象或防御性副本。`Diagnostic` 不包含 DSN、Host、Port、User、Pass、节点 `Extra`、数据库名或任何原始 GoFrame Config；`Tables` 返回新切片。

`New` 是启动期入口，单次 Runtime 与单次 GoFrame Group 绑定。它先以 `gdb.SetConfigGroup(config.Group, clonedNodes)` 注册配置，再以 `gdb.Instance(config.Group)` 获取数据库。Runtime 不在请求路径中重新注册、重连或变更组配置。

## 4. 启动流程

`New` 的顺序固定：

1. 对 nil Context 使用 `context.Background()`；
2. 验证 Group、Nodes 和表名集合，拒绝空白、重复与非法标识符；
3. 防御性复制 GoFrame 节点与表名，调用 `gdb.SetConfigGroup`；
4. 获取 `gdb.Instance(config.Group)`，并验证 `database.GetGroup()` 精确相等；
5. 调用 `driver.Probe(ctx, database, tables...)`；
6. 使用同一 `database` 与 Group 创建 `tx.NewRunner`；
7. 将 Dialect、Report、Runner 和表集合冻结为 Runtime，返回给 Application Host。

任一步失败均不返回部分可用 Runtime。Probe 负责版本、条件写入、事务、锁能力和 MySQL 默认及实际交易表 InnoDB 校验；因此 `schema.mode=off` 不能绕过步骤 5。Runtime 不根据 Schema Mode 分支。

## 5. Scope 一致性

`Runtime.Current(ctx)` 是 Framework Group 的只读 Scope 查询：

1. Context 没有模块 08 Scope 时返回 `nil, false, nil`；
2. Context Scope 的组等于 Runtime Group 时返回该 TX、`true`、`nil`；
3. Context Scope 的组不同，返回 Core 异常，不返回 DB 或 TX；
4. 已结束 Scope 由 `tx.Current` 视为不存在。

后续 `Base.Model` 在无 Scope 时使用 `Runtime.DB()`，在同组 Scope 时绑定 `Runtime.Current` 返回的 TX；不同组错误直接向上传播。`Runtime.Current` 不新建事务，事务边界仍只由 `Runtime.Runner().Within` 建立。

## 6. 事务表集合与扩展

`TransactionTables` 是确定的、去重后按字典序冻结的物理表名。模块 09 传给 `driver.Probe`，用于 MySQL InnoDB 校验，也通过 `Diagnostic` 供启动日志展示。

模块 09 不暴露 Runtime 运行后的可变注册 API。Application Host 在调用 `New` 前汇总生成业务表、已启用 Recycle 表和后续 Outbox/Inbox 表；这样 Probe 与 Ready Gate 看见的是完整集合，避免启动后才加入未验证表。

## 7. 错误与诊断

- Config 验证、组不一致和跨组 Scope 均返回模块 02 Core 异常，保留 cause；
- `gdb.SetConfigGroup`、`gdb.Instance`、`driver.Probe` 和 `tx.NewRunner` 的错误增加稳定操作上下文，但不得拼入敏感节点内容；
- 启动日志仅使用 `Diagnostic`，输出 Group、Kind、Version、Capabilities 和表名；
- 数据库身份验证或网络错误可保留驱动 error chain，但 Runtime 自己不复制或格式化 DSN，因此不泄漏密码。

## 8. 文件职责

| 文件 | 职责 |
|---|---|
| `runtime.go` | Config、Runtime、启动编排、访问器和 Scope 查询 |
| `diagnostic.go` | 无敏感信息的启动诊断与防御性副本 |
| `validate.go` | Group、节点和事务表集合校验与 Core 错误包装 |
| `runtime_test.go` | SQLite 启动、访问器、Scope、一致性、配置和诊断测试 |

测试使用临时文件 SQLite `gdb.ConfigGroup`，验证 Runtime Probe 与 Runner 操作同一具名组。MySQL 的实际 InnoDB 反例由模块 06 集成测试覆盖；模块 09 单元测试验证它将传入表集合完整交给 Probe 的公开流程。

## 9. 测试与验收

至少覆盖：

1. 有效 SQLite Group 可构造 Runtime，DB Group、Dialect、Runner Group 和 Diagnostic 相互一致；
2. Runtime Runner 写入的事务可提交和回滚，证明两者使用同一组；
3. `Current` 对无 Scope、同组 Scope、跨组 Scope 和已关闭 Scope 的固定结果；
4. 空 Group、空 Nodes、非法/重复表名、组不匹配和无法连接均失败且为 Core 异常；
5. `Diagnostic` 的切片修改不影响 Runtime，且 JSON/文本不含密码、用户、Host、DSN 或节点 Extra；
6. `schema.mode=off` 不属于 Runtime 分支，构造 Runtime 仍调用 Probe；
7. Race 下并发读取 Runtime 访问器和 Diagnostic 不产生数据竞争。

门禁：

```bash
go test ./cool-next/db -count=1
go test -race ./cool-next/db -count=1
go vet ./...
make check
```

## 10. 完成标准

1. 09.1-09.9 均有实现和测试证据；
2. 全部框架消费者可获得同一 Group、DB、Dialect 和 Runner；
3. 不存在 Entity/Route 级换组入口；
4. 版本、能力和 MySQL InnoDB 探测发生在 Runtime 创建期，独立于 Schema Mode；
5. Scope 与 Runtime 组不一致时绝不退回全局 DB；
6. 启动诊断不泄漏连接敏感信息；
7. 未提前实现 Application Host、CRUD、Recycle、Outbox 或 Inbox。
