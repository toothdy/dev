# ActionPlan 与 CRUD Dispatcher 设计

## 目标

完成模块分解任务 `25.1-25.12`，在既有查询计划、事务 Runner、Base 写操作、Mutation Hook 和生成期 Adapter 之上，建立唯一的协议无关 CRUD 调度入口。

本模块保证：

- 六种 CRUD 动作使用固定名称和一致的执行边界。
- 默认 Base 与 Base 委托路径获得完整 ActionPlan 增强。
- 纯 override 不会隐式获得 Base 字段或查询增强。
- 所有 CRUD 均进入 Framework Database Group 事务。
- Add、Delete、Update 的整批 Hook、DML 和提交保持原子性。

## 范围

本次包含：

- 补全 `FieldPolicyInput` 和不可变 `FieldPolicy`。
- 编译隐藏、只读、Info 忽略、排序白名单及默认排序。
- 将字段策略和既有 `QueryPlan` 组合为 `ActionPlan`。
- 使用 `OperationScope` 在 Context 中携带 ActionPlan。
- 新增复用 `dbtx.Runner` 的协议无关 Dispatcher。
- 校验生成期选择的 Base、纯 override 和 Base 委托模式。
- 复用模块 24 的 `ExecuteMutation` 执行整批 Hook 和写操作。
- 使用 SQLite 集成测试验证事务和批量原子性。

本次不包含：

- HTTP/gRPC 路由、参数绑定、`Before` 或 `InsertParam`。
- Controller Builder 和 `CurdOption` 的声明及编译。
- 删除归档、回收站和恢复。
- 自定义 Route 的 `NonTransactional` 能力。
- 新的事务实现、查询 SQL 渲染器或运行时反射 Adapter。

上述能力分别由后续模块 26、30、31 和 32 接入。

## 现状与缺口

当前 `cool-next/crud` 已有六种 Action、最小 ActionPlan、OperationScope，以及模块 21 提供的查询计划编译和应用器。`cool-next/db/tx` 已处理事务创建、同组复用、跨组拒绝、rollback-only、提交失败和 Panic 回滚。`cool-next/core/service` 已提供 Mutation、Hook 执行单元和要求事务及 ActionPlan 的 Base CRUD。生成器也已静态区分 `base`、`override` 和 `delegate` 并生成动作 Adapter。

当前缺口不是底层能力，而是这些能力尚未形成单一调用链：

- 字段策略只有 Hidden 和 Readonly，且输入仍是字符串。
- InfoIgnore、SortFields、DefaultSort 和客户端排序尚未编译。
- Controller 后续没有统一入口建立事务并按动作模式决定是否写入 ActionPlan。
- 事务、OperationScope、生成 Adapter 和 Mutation Hook 仍由测试手工拼接。

## 方案选择

采用最小组合方案：Dispatcher 位于 `cool-next/crud`，只拥有事务边界、动作模式校验、OperationScope 和 Adapter 调用；写操作 Hook 继续复用模块 24 的 `ExecuteMutation`。

不把 Mutation 泛型或 Hook 接口迁入 `crud`。这样避免重复 Hook 外壳，也不形成 `crud -> core/service -> crud` 循环依赖。Dispatcher 接收闭包形式的生成 Adapter，后续 Controller Adapter 负责捕获具体 Service、输入和返回值。

不把 Dispatcher 放入 `core/service`。协议无关执行计划和调度边界继续归 `crud` 所有，后续 HTTP、gRPC、任务或测试均可复用。

## ActionPlan 与字段策略

公共输入遵循总架构：

```go
type FieldPolicyInput struct {
   HiddenFields       []ColumnRef
   ReadonlyFields     []ColumnRef
   InfoIgnoreProperty []ColumnRef
   SortFields         []ColumnRef
   DefaultSort        ColumnRef
   DefaultOrder       Direction
}

type PlanInput struct {
   Action Action
   Entity any
   Query  QueryOp
   Fields FieldPolicyInput
}
```

`FieldPolicy` 保持字段私有，只公开判断方法。内部集合在编译时独立复制，调用方后续修改输入切片不能改变计划。Hidden、Readonly 和 InfoIgnore 以 Descriptor 的 Go 字段名保存，写操作和查询投影均使用同一字段事实来源。SortFields 额外保存请求字段名到已解析字段的映射。

字段策略编译规则：

1. 所有字段必须属于根 Entity 的 Descriptor。
2. 同一策略内字段不能重复。
3. Hidden、Readonly 和 InfoIgnore 可以重叠；执行时采用更严格的效果。
4. DefaultSort 非空时必须存在于 SortFields。
5. DefaultOrder 只能为 `asc` 或 `desc`；未配置 DefaultSort 时不得单独配置方向。
6. 字段引用不得伪装成其他实体或关联别名。

`ActionPlan.Fields()` 返回只读策略指针。Base 写操作使用它拒绝隐藏或非法只读字段。`ActionPlan.ApplyQuery` 仍只委托模块 21 的内部应用器，不复制查询逻辑。

## 查询投影与排序

`CompilePlan` 先编译字段策略，再调用既有查询计划编译器。Add、Delete、Update 不创建 QueryPlan；Info、List、Page 必须创建 QueryPlan。

查询计划完成后执行模块 25 的收口处理：

- 所有查询删除来源字段命中 Hidden 的投影。
- Info 额外删除命中 InfoIgnoreProperty 的投影。
- 过滤后无投影字段时返回 Core 配置错误。
- List/Page 读取 `QueryRequest` 中规范化后的 `order` 和 `sort`。
- 客户端字段必须命中 SortFields，方向必须为 `asc` 或 `desc`，两者数量必须一致。
- 未提交客户端排序时使用 DefaultSort；没有默认排序时不追加动态排序。
- 动态排序追加在 `QueryOp.AddOrderBy` 和 Extend 排序之后，保持模块 21 已冻结的固定排序顺序。
- Info 不接受客户端或默认列表排序。

模块 32 的 Binder 负责把 HTTP 单值或多值参数规范化为字符串切片并执行协议层校验；`CompilePlan` 仍重复验证安全不变量，防止非 HTTP 调用绕过白名单。

## Dispatcher

`crud` 定义动作模式，并由 `core/service` 保留兼容别名供现有生成代码使用：

```go
type ActionMode string

const (
   ActionModeBase     ActionMode = "base"
   ActionModeOverride ActionMode = "override"
   ActionModeDelegate ActionMode = "delegate"
)
```

Dispatcher 保存唯一的 `dbtx.Runner`，构造时拒绝 nil。执行入口接收 Context、Action、生成期模式、可选 ActionPlan 和 Adapter 回调。Adapter 使用闭包承载具体类型和返回值，因此 Dispatcher 不需要 `any` 返回值、反射或每个动作一套接口。

调度状态与 Base 增强状态使用两个独立 Context Scope：所有模式都有只读 `DispatchScope`，供 Mutation 校验当前 Action 和 ActionMode；只有 `base/delegate` 拥有 `OperationScope` 和 ActionPlan。这样纯 override 可以执行统一 Hook，又不会获得 Base 字段或查询增强。override 进入 Dispatcher 时会显式遮蔽可能从外层 Context 继承的 OperationScope。

执行规则：

```text
校验 Context、Action、ActionMode 和 Adapter
-> base/delegate：要求 ActionPlan 存在且动作一致
-> override：要求 ActionPlan 为空
-> Runner.Within 开启或复用事务
   -> 写入 DispatchScope
   -> base/delegate：写入 OperationScope
   -> override：遮蔽外层 OperationScope
   -> 调用生成期选定的 Adapter 一次
-> Runner 拥有事务时提交
```

Dispatcher 不接受 `NonTransactional`。六种默认 CRUD，包括 Info、List、Page，均经过 Runner。已有同组 Scope 时复用事务且不取得提交权；不同组由 Runner 拒绝。

## Hook 与 Adapter

Add、Delete、Update 的 Adapter 闭包调用模块 24 的 `ExecuteMutation`：

```text
Dispatcher 事务与 OperationScope
-> ExecuteMutation 校验 Mutation 与 ActionPlan
-> ModifyBefore 一次
-> 生成期选定的 Base/override Adapter 一次
-> Add 保存 Result IDs
-> ModifyAfter 一次
-> 返回 Dispatcher
```

Info、List、Page 的 Adapter 闭包直接调用生成期选定的读取 Adapter，不构造 Mutation，也不执行修改 Hook。

`base` 与 `delegate` 都携带 ActionPlan。delegate 中业务方法直接调用 `s.Base.Xxx()` 时，Base 从当前 Context 读取同一个 Plan 和 TX；Dispatcher 和 ExecuteMutation 均不会被递归调用。

`override` 不携带 ActionPlan。`ExecuteMutation` 通过 DispatchScope 校验动作并执行写 Hook，不依赖 OperationScope。其业务实现仍在 Dispatcher 事务中，但不会自动应用 QueryOp、字段策略、默认 Base 或 Controller 前置增强。纯 override 间接调用 Base 时，Base 因找不到 ActionPlan 返回 Core 配置错误。

## 事务与错误处理

- Dispatcher 参数或模式组合错误返回现有 Core 异常。
- 字段、排序或请求形状错误返回现有 Validate 异常；框架配置矛盾返回 Core 异常。
- Adapter、Hook 和 DML 错误保持错误链并原样交给 Runner。
- 回调错误、嵌套同组失败、Panic 或提交失败统一由 Runner 回滚或返回失败。
- 批量 Add/Delete/Update 中任一项失败时，Runner 回滚整批，不允许部分成功。
- Dispatcher 不捕获后继续隐藏 Panic；Runner 回滚后重新抛出原 Panic。

## 测试

`crud` 单元测试覆盖：

- 六种 Action 和非法 Action。
- FieldPolicy 四类字段的编译、复制和重复校验。
- DefaultSort 白名单及方向校验。
- Hidden 和 InfoIgnore 对投影的过滤。
- 客户端多字段排序、数量不一致、非法方向及白名单拒绝。
- OperationScope 的写入、读取和无效状态。
- Dispatcher 对 base、delegate、override 的 Plan 约束。
- Adapter 恰好调用一次，三种模式均可见 DispatchScope，base/delegate 可见 Plan，override 不可见 Plan。
- 嵌套 override 遮蔽外层 OperationScope。

SQLite 集成测试覆盖：

- Dispatcher 成功提交。
- 已有同组事务时复用同一 TX。
- 不同组事务被拒绝。
- Adapter 返回错误或 Panic 时回滚。
- Dispatcher 与 `ExecuteMutation` 组合后，Before/After 各执行一次。
- Before、DML、After 任一步失败均回滚整批。
- 默认 Base 和 delegate 可以在同一 Plan/TX 中执行。
- 纯 override 保留事务，但调用 Base 因缺少 Plan 失败并回滚。

验收命令：

```text
go test ./cool-next/crud ./cool-next/core/service ./cool-next/codegen -count=1
go test ./... -count=1
go vet ./...
gofmt -l <本任务修改的 Go 文件>
```

## 后续衔接

模块 30 负责把 Controller 的 `CurdOption` 编译为本模块的 `FieldPolicyInput` 和 `PlanInput`。模块 32 负责绑定请求、执行 `Before/InsertParam`、构造具体 Adapter 闭包并调用 Dispatcher。模块 26 在 Base Delete 内加入事务归档，不改变 Dispatcher 和 Hook 外壳。
