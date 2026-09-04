# Base 写操作、Hook 与重写语义设计

## 目标

完成模块分解任务 `24.1-24.14`，为默认 Base、纯 override 和 Base 委托 override 建立一致且可静态验证的写操作契约。本任务实现 Service 层写能力、Hook 执行单元和生成期动作识别，不提前实现任务 25 的 CRUD Dispatcher。

## 范围

本次包含：

- 单对象和数组 Add，保持输入形状并按输入顺序返回主键。
- 仅更新明确出现字段的 Update。
- 物理 Delete。
- `Mutable` 到生成 DOValue 的转换、受限字段清理和拒绝规则。
- `Mutation`、`ModifyBeforeHook`、`ModifyAfterHook` 及事务内执行单元。
- 默认 Base、纯 override、直接 Base 委托 override 的静态识别和生成 Adapter。
- 写操作、Hook 回滚与源码分析测试。

本次不包含：

- CRUD Dispatcher、HTTP/gRPC 参数绑定和路由调用链。
- `Before`、`InsertParam`、完整 FieldPolicy 编译和软删除归档。
- 自动创建事务。事务仍由 `db/tx.Runner` 或后续 Dispatcher 建立。

## 架构边界

### Base 写操作

`Base.Add`、`Base.Update` 和 `Base.Delete` 只负责 Descriptor 驱动的结构化 DML。三个方法均要求：

1. Context 中存在与动作匹配的 `ActionPlan`。
2. Context 中存在与 Base 数据库组一致的框架事务。
3. 输入由已有 Smart Constructor 构造并与 Base Descriptor 匹配。

Base 不创建事务、不查找 Hook、不执行 Controller 前置增强。缺少动作计划、缺少事务、动作不匹配或数据库组不匹配时返回 Core 配置错误。

隐藏和额外只读字段最终由任务 25 从 Controller 配置编译。为使本任务的写入安全规则可执行，`ActionPlan` 先增加只读的最小字段策略载体，由测试和生成 Adapter 显式传入字段名；本任务不分析 Controller 配置，也不实现任务 25 的 FieldPolicy 编译流程。

### Hook 执行单元

Service 层提供面向单次整批写操作的执行单元。调用方传入 Mutation、可选 Before/After Hook 和实际写入回调，执行顺序固定为：

```text
校验当前事务与动作
-> ModifyBefore，最多一次
-> 写入回调，恰好一次
-> 保存 Add Result IDs
-> ModifyAfter，最多一次
```

执行单元不提交或回滚事务。任一步骤返回错误时立即停止并原样返回；外层 `db/tx.Runner` 根据错误回滚。任务 25 的 Dispatcher 将复用该执行单元，不再实现第二套 Hook 外壳。

### 生成期 Adapter

生成器按 Service 的六个 CRUD 动作分别记录调用模式：

- `base`：Service 未直接声明同名方法，Adapter 直接调用注入的 Base。
- `override`：Service 直接声明同名方法，方法体内没有直接调用对应 Base 方法。
- `delegate`：Service 直接声明同名方法，方法体内出现接收者对应 Base 字段的直接同名调用。

直接委托只识别 override 方法体内的调用表达式。helper、函数值和反射形成的间接调用不算委托；条件分支中的直接调用仍将整个动作标记为 `delegate`。每个动作独立分析，重写 Add 不改变 Delete、Update、Info、List 或 Page。

生成结果提供任务 25 可直接消费的动作级 Adapter 和模式元数据，但不生成 Dispatcher、路由或事务入口。

## 数据处理

### Mutable 转 DOValue

转换时通过 Descriptor 的 `NewDO` 创建独立 DOValue，再按 Mutable 中明确出现的字段调用 `SetColumn`。普通值保留零值和 `false`；显式 null 使用 DOValue 已有的 null 表达能力；未出现字段不写入 DOValue。

转换过程中只使用 Descriptor 字段元数据和生成 DO，不构造数据库 map，不拼接 SQL。

### Add

Add 的每一项按以下顺序处理：

1. 拒绝隐藏字段。
2. 移除客户端来源的主键和只读字段。
3. 保留 Hook 或其他可信服务端逻辑通过 `Mutable.Set/SetNull` 写入的值。
4. 转换为独立 DOValue。
5. 使用绑定当前事务的 Model 执行 `InsertAndGetId`。
6. 将数据库主键转换为 Descriptor 的 ID 类型并按输入顺序收集。

单对象返回标量 ID，数组返回有序 ID 数组。任一项失败时不继续处理后续项。

### Update

Update 对每一项执行：

1. 拒绝主键、系统维护字段和 ActionPlan 最小字段策略指定的隐藏或只读字段。
2. 只把 Mutable 中明确出现且允许更新的字段写入 DOValue。
3. 使用 Descriptor 主键列和 UpdateItem ID 作为 Where 条件执行 Update。

ID 仅用于定位，不进入更新数据。空更新数据返回 Validate 错误，不发出无意义 DML。

### Delete

Delete 使用 Descriptor 主键列和 Smart Constructor 已验证的 ID 列表构造 `IN` 条件，并在当前事务中执行物理 Delete。软删除归档属于后续任务，不在本任务中实现。

## Mutation 契约

`Mutation[E, ID]` 字段保持私有，仅公开以下只读方法：

```go
func (mutation *Mutation[E, ID]) Action() crud.Action
func (mutation *Mutation[E, ID]) AddInput() AddInput[E]
func (mutation *Mutation[E, ID]) UpdateInput() UpdateInput[E, ID]
func (mutation *Mutation[E, ID]) DeleteIDs() []ID
func (mutation *Mutation[E, ID]) ResultIDs() []ID
```

Mutation 通过动作专用构造器创建，拒绝动作与输入形状不一致。返回的 slice 均为副本。Add 写入成功后，由执行单元在调用 After Hook 前写入 Result IDs；其他动作的 Result IDs 为空。

Hook 接口保持最小：

```go
type ModifyBeforeHook[E any, ID comparable] interface {
   ModifyBefore(context.Context, *Mutation[E, ID]) error
}

type ModifyAfterHook[E any, ID comparable] interface {
   ModifyAfter(context.Context, *Mutation[E, ID]) error
}
```

缺少某一侧 Hook 时使用 no-op Adapter，不要求业务 Service 实现空方法，也不在运行时通过反射判断重写。

## 错误处理

- 非法字段、空更新和输入不一致返回现有 Validate 异常。
- 缺少事务、缺少或不匹配 ActionPlan、无效生成 Adapter 返回现有 Core 异常。
- GoFrame ORM 错误使用现有 `wrapCoreError` 增加动作上下文。
- Hook 错误保持 `errors.Is/errors.As` 可追踪性并直接返回。
- 不吞掉单项错误，不允许批量部分成功。

## 测试

Service 单元和 SQLite 集成测试覆盖：

- 单条 Add 返回标量 ID，批量 Add 按输入顺序返回 ID。
- 客户端 ID/只读字段被清理，服务端同字段值保留，隐藏字段被拒绝。
- Update 保留零值、`false` 和 null，只更新明确出现字段。
- Update 拒绝 ID、只读字段、隐藏字段和空更新。
- Delete 按 ID 集合物理删除。
- 写操作拒绝缺失事务、缺失计划、动作不匹配和跨组 Scope。
- 批量 Before/After 各调用一次，After 可读取 Add Result IDs。
- Before、DML 或 After 失败均使 Runner 回滚整批。

生成器测试覆盖：

- 未声明动作识别为默认 Base。
- 直接声明且不委托识别为纯 override。
- 直接 `s.Base.Xxx()` 识别为 Base 委托。
- helper 间接调用不识别为委托，条件分支直接调用仍识别为委托。
- 单个动作重写不影响其他五个动作。
- 生成 Adapter 可通过完整候选源码类型检查。

验收命令：

```text
go test ./... -count=1
go vet ./...
gofmt -l <本任务修改的 Go 文件>
```

## 后续衔接

任务 25 的 Dispatcher 负责创建或复用事务、将 ActionPlan 写入 Context，并根据生成 Adapter 选择默认 Base、纯 override 或 Base 委托。Dispatcher 复用本任务的 Mutation 和 Hook 执行单元，保证事务和 Hook 外壳只有一份实现。
