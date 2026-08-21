# 模块 08：事务 Scope 与 Runner 设计

> 日期：2026-08-05
> 状态：待复核
> 模块：08 事务 Scope 与 Runner
> 对应拆分项：08.1-08.10
> 前置模块：02 核心异常模型、03 配置加载与基础校验

## 1. 目标

本模块在 `cool-next/db/tx` 提供框架唯一的事务边界。调用方只通过 `Runner.Within` 进入事务，并通过派生 `context.Context` 获取当前框架组的 `gdb.TX`。

必须满足：

1. 无 Scope 时开启一个 GoFrame 事务，由最外层 Runner 唯一决定提交或回滚；
2. 同数据库组的嵌套调用复用同一 `gdb.TX`，不创建 savepoint，也不取得提交权；
3. 跨数据库组嵌套立即返回模块 02 的 Core 异常；
4. 内层任意失败都使外层 rollback-only，且始终保留第一个失败；
5. panic 回滚后原样重新抛出；
6. 回调返回后 Scope 失效，之后不能再通过保留的 Context 获取事务；
7. 并发关闭或重复结束不会改变首次结束决定。

本模块不实现数据库连接组解析、Dialect/能力探测、Schema、DML、CRUD、重试、隔离级别选择或跨库分布式事务；它们分别属于模块 06、07、09 和后续 CRUD 模块。

## 2. 前置与边界

- 只依赖标准库 `context`、`sync`、`errors`，GoFrame `gdb` / `gerror`，以及模块 02 `exception`；
- 不读取配置。模块 09 负责从规范化配置创建 `Runner` 并保证框架全局组一致；
- 不使用 GoFrame 的 Context Transaction Key 作为框架 API。框架 Scope 使用私有 Key，避免让不同组或生命周期外的事务被隐式接受；
- 最外层由 Runner 显式调用 `gdb.DB.Begin`、`gdb.TX.Commit` 和 `gdb.TX.Rollback`。不能使用 `gdb.DB.Transaction`，因为它会捕获并转换 callback panic；
- 模块 08 的 Scope 只对框架 API 有效。调用方故意保存并直接调用已经取得的 `gdb.TX` 不受语言层面阻止，属于禁止绕过 `Current` 的框架约束。

## 3. 对外接口

代码归属固定为 `cool-next/db/tx`，包名为 `tx`。公开接口如下：

```go
type Callback func(ctx context.Context) error

type Runner interface {
    Group() string
    Within(ctx context.Context, callback Callback) error
}

func NewRunner(group string, database gdb.DB) (Runner, error)
func Current(ctx context.Context) (transaction gdb.TX, group string, exists bool)
```

语义：

- `NewRunner` 拒绝空白 `group`、`nil database`、以及 `database.GetGroup()` 与 `group` 不相同的构造；错误均为 Core 异常；
- `Group` 返回 Runner 的不可变连接组名；
- `Current` 在 Context 中存在且仍有效的框架 Scope 时返回其事务和组名，否则返回 `nil, "", false`；它不回退到 GoFrame 的隐式 `gdb.TXFromCtx`；
- `Within` 拒绝 nil callback 并返回 Core 异常；nil Context 以 `context.Background()` 处理；
- callback 只接收派生 Context，不直接接收 `gdb.TX`。实际 DML 使用 `Current(callbackCtx)` 取得当前事务后调用 `transaction.Ctx(callbackCtx)`。

`Runner` 是并发安全的不可变配置对象；派生 Scope 是每次最外层调用独有的可变状态，不能跨 goroutine 并发使用。后续 Service 与 Route 可以并行请求各自 Scope，但不得在一个 Scope 内并发执行数据库操作。

## 4. Scope 状态

私有 Scope 至少保存：

```go
type scope struct {
    group     string
    tx        gdb.TX
    completed bool
    rollback  bool
    firstErr  error
    mutex     sync.Mutex
}
```

状态转换受 `mutex` 保护：

| 事件 | completed | rollback | firstErr |
|---|---:|---:|---|
| 创建并进入最外层回调 | false | false | nil |
| 任一回调返回错误 | false | true | 首个非 nil 错误 |
| 所有嵌套回调成功 | false | false | nil |
| 最外层结束 | true | 保持 | 保持 |

`recordFailure(err)` 仅在 `err != nil` 且尚未记录失败时保存错误；后续失败不覆盖它。`close()` 是幂等操作：第一次结束将 `completed` 置为 true，之后的关闭没有副作用。`Current` 必须在同一把锁下读取 `completed`，确保并发关闭后不会返回可用 Scope。

## 5. 执行流程

### 5.1 无事务 Scope

`Within` 发现 `Current(ctx)` 不存在时：

1. 调用 `database.Begin(ctx)`；开始失败直接返回 GoFrame 原始错误；
2. 用返回的 `transaction` 创建 Scope，并将它写入只派生一次的 Context；
3. 执行 callback；若 callback 返回错误，记录为首个失败；
4. callback 成功但 Scope 已 rollback-only 时，以首个失败作为最外层错误；
5. 最外层 `defer` 一律关闭 Scope；正常错误路径调用 `Rollback`，正常成功路径调用 `Commit`；
6. 若 callback panic，defer 先记录回滚状态、调用 `Rollback`、关闭 Scope，再 `panic(recovered)`；
7. Commit/Rollback 失败且已有 callback 错误时返回 `errors.Join(firstErr, databaseErr)`；仅有数据库结束错误时原样返回。

GoFrame 2.10.2 的 `gdb.DB.Transaction` 会恢复 panic 并转换成 error，不满足本模块的 panic 契约。因此 Runner 使用 `Begin` 管理最外层生命周期。panic 路径的 Rollback 错误不得替换原 panic；实现可记录该错误，但调用方观察到的 panic 值必须保持不变。

### 5.2 同组嵌套

若 `Current(ctx)` 返回有效 Scope 且组名等于 Runner 的 `Group()`：

1. 不调用 `database.Transaction`，不创建新 `gdb.TX` 或 savepoint；
2. 调用 callback 并传入原 Scope 派生的 Context；
3. callback 返回错误时调用 `recordFailure`，并将原始错误返回给内层调用者；
4. 上层即使吞掉该错误并返回 nil，最外层仍通过 `rollback` 和 `firstErr` 回滚；
5. 内层不会关闭 Scope，也不能提交或回滚。

### 5.3 跨组嵌套

若当前有效 Scope 的组与 Runner 的组不同，`Within` 不执行 callback，立即返回 Core 异常。错误消息只包含当前和请求的组名，不包含连接信息、DSN 或密码。

### 5.4 生命周期外 Context

最外层返回前 Scope 已关闭。之后即使调用方保留 callback Context，`Current` 也必须返回 `exists=false`。新 Runner 调用该 Context 时表现为没有 Scope，并可建立独立事务；它不得复用已结束的事务。

## 6. 错误与 panic

- 参数、构造和跨组错误均使用 `exception.WrapCore`，保留 GoFrame/标准库 cause；
- callback 的业务错误不重新包装，保持 `errors.Is` / `errors.As` 语义；
- rollback-only 的最外层返回第一个 callback 错误，后续错误不替换它；
- 事务开始、提交或回滚失败由 GoFrame 原样返回；若已有首个 callback 错误，使用 `errors.Join(firstErr, databaseErr)`，确保第一个业务失败仍可由 `errors.Is` 找到；
- panic 不转换为 error、不吞掉，也不包装。Scope 先关闭，随后用原 panic 值重新抛出；
- 回调不得自行调用 `Commit` 或 `Rollback`，也不得把 `gdb.TX` 作为跨 Scope 资源保存。

## 7. 文件职责

| 文件 | 职责 |
|---|---|
| `types.go` | `Callback`、`Runner`、私有 Context Key 和公开 `Current` |
| `runner.go` | `NewRunner`、最外层 Begin/Commit/Rollback、嵌套分派、panic 处理 |
| `scope.go` | Scope 状态、首错记录、关闭和有效性检查 |
| `errors.go` | 与模块 02 对齐的 Core 异常构造 |
| `runner_test.go` | 生命周期、同组、跨组、rollback-only、panic 和错误语义 |
| `scope_test.go` | `Current`、并发关闭、重复结束和 race 覆盖 |

测试使用项目已有模式创建临时文件 SQLite `gdb.DB`，通过实际写入验证 Commit 和 Rollback。包内测试辅助构造器可包装 `Begin` 以计数事务创建次数，但不模拟庞大的 `gdb.DB` / `gdb.TX` 接口。三数据库的真实事务能力已经属于模块 06 探测和第一阶段总验收门；模块 08 不新增容器依赖。

## 8. 测试与验收

单元测试至少覆盖：

1. 无 Scope 时只创建一次事务，成功回调只提交一次；
2. callback 失败时回滚，并保持原始错误可被 `errors.Is` 识别；
3. 同组嵌套只创建一个事务、共享同一 `gdb.TX`，内层不取得结束权；
4. 内层错误即使被外层吞掉，最外层仍回滚并返回首个错误；
5. 多个嵌套失败始终返回首个错误；
6. 跨组嵌套不执行 callback，并返回可 `errors.As` 为 `exception.BaseException` 的 Core 错误；
7. callback panic 后 Rollback 已发生，且 panic 值保持不变；
8. 回调结束后的 Context 无法通过 `Current` 得到事务；
9. nil Context、nil callback、空组、nil DB 和组名不匹配均被拒绝或按固定语义处理；
10. 多 goroutine 并发 `Current` 与 `close`、重复 `close` 在 `-race` 下无数据竞争，且完成后不再报告 Scope。

门禁命令：

```bash
go test ./cool-next/db/tx -count=1
go test -race ./cool-next/db/tx -count=1
go vet ./...
make check
```

## 9. 完成标准

1. 08.1-08.10 均有实现和测试证据；
2. 事务提交权仅存在于最外层 Runner；
3. 同组嵌套、跨组拒绝与 rollback-only 语义固定；
4. Context 生命周期外不可再获得框架事务；
5. panic 可见且事务已回滚；
6. 并发结束和重复结束不产生数据竞争或改变首次结果；
7. 未提前实现模块 09 Runtime、CRUD 或分布式事务能力。
