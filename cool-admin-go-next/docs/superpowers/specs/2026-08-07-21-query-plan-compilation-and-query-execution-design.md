# 模块 21：QueryPlan 编译与查询执行设计

> 日期：2026-08-07
> 状态：已确认
> 模块：21 QueryPlan 编译与查询执行
> 对应拆分项：21.1-21.8
> 前置模块：04 实体与 Descriptor 元数据、20 QueryRequest 与安全查询 AST

## 1. 目标

模块 21 将模块 20 的 `QueryOp`、`QueryRequest` 和追加式 `QueryBuilder` 编译为不可变 `QueryPlan`，再把已验证节点参数化应用到 GoFrame `gdb.Model`。它对应 Node 版 `BaseService.getOptionFind()` 与 `entityRenderPage()` 的查询部分，但不复制 Node 的 SQL 字符串拼接和三数据库重复实现。

本模块必须：

1. 通过 `DescriptorResolver` 解析根实体、Join 实体、字段和物理列；
2. 固定根别名、Join 可达性、Select 输出和请求匹配语义；
3. 在取得数据库 Model 前完成字段、别名、类型和上限校验；
4. 生成只含私有规范化数据节点的不可变 `QueryPlan`；
5. 将同一计划应用到克隆后的 `gdb.Model`；
6. 依靠 GoFrame 从同一 Model 执行分页数据与 Count；
7. 在 MySQL、PostgreSQL、SQLite 上验证相同行为；
8. 不增加第二套公开查询编译 API。

## 2. 非目标

本模块不实现：

- `ActionPlan`、`FieldPolicy`、CRUD Dispatcher 或 Operation Scope；
- Controller 的 `StaticQuery`、`DynamicQuery`、排序白名单和字段隐藏策略；
- Service 的 `Query`、`Record`、`PageResult` 或分页大小配置；
- HTTP/gRPC Binder、客户端 order/sort 解析或导出；
- NativeQuery、完整 SQL Builder、子查询、Union、窗口函数或任意 OR DSL；
- 生成图的 `DescriptorResolver` 适配；该适配随模块 25 的 ActionPlan 装配加入；
- 查询缓存、计划缓存、插件、Visitor 或运行时节点注册器。

## 3. Node 对应关系

Node 的调用链为：

```text
BaseController.page/list
-> BaseService.page/list
-> getOptionFind(query, pageQueryOp/listQueryOp)
-> TypeORM QueryBuilder
-> entityRenderPage/sqlRenderPage
```

模块 20 已替代 Node `QueryOp` 的字符串输入形状；模块 21 替代 `getOptionFind()` 中的 Join、Where、Select、关键词、FieldEq、FieldLike、Order 和 Extend 编译，并使用 GoFrame `AllAndCount(false)` 替代 `getCountSql()` 与方言化分页 SQL。

保留的行为：

- `keyWord` 对多个字段做 OR 模糊匹配；
- `FieldEq` 数组转 `IN`；
- `FieldLike` 自动包裹 `%`；
- QueryOp 固定条件与请求条件共同生效；
- Page 数据与 total 来自相同过滤条件。

主动收紧的行为：

- 字段、实体和别名必须能由 Descriptor 解析；
- Join 不自动把关联实体全部字段加入响应；
- 请求值、Select 输出和结构数量在执行前验证；
- SQL 值只能通过绑定参数进入查询。

## 4. 所有权与公开边界

总架构保留唯一公开入口：

```go
type QueryPlan struct{ /* private normalized query */ }

type DescriptorResolver interface {
   Resolve(entity any) (coreentity.Metadata, bool)
}

func CompilePlan(
   context.Context,
   DescriptorResolver,
   PlanInput,
   *QueryRequest,
) (*ActionPlan, error)

func (plan *ActionPlan) ApplyQuery(
   context.Context,
   *gdb.Model,
) (*gdb.Model, error)
```

模块 21 定义 `QueryPlan` 及同包内部的编译、冻结和应用函数。模块 25 的 `CompilePlan` 调用内部查询编译器并把结果放入 `ActionPlan.query`；`ActionPlan.ApplyQuery` 只委托内部应用器。框架不公开 `CompileQuery`、`ApplyQuery(QueryPlan)` 或 `QueryLimits`。

这样模块 21 可以独立测试，业务侧仍只有 ActionPlan 一条查询入口。

## 5. 内部计划模型

`QueryPlan` 只保存普通数据，不保存闭包、Context、Resolver 或 Model。内部最小节点包括：

- 根实体元数据与固定别名；
- 已解析 Join 的表、别名、类型和两侧列；
- 已解析 Select 的来源列和输出名；
- Where/Having 的操作符、列和绑定值；
- Group 列；
- Order 列和方向。

已解析列至少保存：

```text
表名
表别名
数据库列名
逻辑字段名
Go 类型
逻辑类型
nullable
```

节点不保存业务提供的动态表名或动态列名。QueryPlan 的字段全部私有，不提供节点 getter、序列化、修改或重新绑定 Resolver 的 API。

## 6. 根实体、字段与 Join

### 6.1 根实体

编译器通过 `DescriptorResolver.Resolve(input.Entity)` 解析根实体。解析失败、nil Metadata 或无主键 Metadata 均为 Core 配置错误。

根实体别名固定为 `a`。未指定别名的 `ColumnRef` 解析到根实体；显式 `.Of("a")` 与未指定别名等价。

### 6.2 字段定位

字段按逻辑名调用 `Metadata.Field(name)` 解析，再读取 `Field.Column()` 形成物理列。禁止按调用方字符串直接访问数据库列。

`NewColumnRefOf[E]` 的静态实体类型必须与目标别名对应的 Descriptor 实体类型一致：

- 无别名且类型是根实体：合法；
- 无别名但类型不是根实体：非法；
- 有别名且类型与该 Join 实体一致：合法；
- 类型与别名实体不一致：Core 错误。

### 6.3 Join

别名 `a` 为根实体保留。Join 别名必须唯一且不能重复使用根别名。

Join 按声明顺序解析。每个 `On(left, right)` 必须满足：

1. 一侧属于当前新 Join 别名；
2. 另一侧属于根实体或此前已声明的 Join；
3. 两侧字段去除一层 nullable 指针后的 Go 类型相同；
4. Join 实体能由 Resolver 唯一解析。

因此后声明 Join 可以依赖此前 Join，不能引用尚未声明的别名，也不能声明与当前新表无关的悬空条件。

首期只支持 Left Join 和 Inner Join，以及 `On` 的字段相等条件。

## 7. 编译流程

固定流程为：

```text
校验输入与解析根实体
-> 创建空 QueryBuilder
-> 执行一次 Extend
-> 冻结 Extend 追加节点
-> 按声明顺序解析 QueryOp Join 和 Extend Join
-> 解析 QueryOp Select 和 Extend Select
-> 编译固定 Where
-> 编译 keyWord、FieldLike、FieldEq
-> 追加 Extend Where、Group、Having、Order
-> 校验输出名、类型和上限
-> 深复制绑定值
-> 返回 QueryPlan
```

QueryOp 节点先于 Extend 的同类追加节点。Builder 分类别保存节点，因此不承诺 Where 与 AddSelect 之间的跨类别调用顺序；每一类别内部保持原始追加顺序。

`Extend` 恰好执行一次。它接收原始 `QueryRequest` 和一个新 Builder，只能追加模块 20 已定义的结构化节点。Extend 结束后不再持有 Builder，也不延迟到 Model 应用阶段执行。

`QueryRequest` 为 nil 时等价于没有提交任何请求字段，静态 QueryOp 和 Extend 仍正常编译。

## 8. 请求匹配与条件语义

### 8.1 KeyWordLikeFields

关键词请求参数固定为 `keyWord`。字段缺失或空字符串时不增加条件；非空字符串编译为：

```text
(field1 LIKE ? OR field2 LIKE ? ...)
```

绑定值统一为 `%value%`，整个 OR 分组与其他 Where 条件使用 AND。关键词字段必须是 Descriptor 中的字符串字段。显式 null 或非字符串值返回 Validate 错误。

### 8.2 FieldEq

- 请求参数缺失：跳过；
- 标量零值或 `false`：正常生成 `=`；
- 非空 slice/array：生成参数化 `IN`；
- 显式 null：nullable 字段生成 `IS NULL`；
- 非 nullable 字段的 null：Validate 错误；
- 空 slice/array：Validate 错误。

### 8.3 FieldLike

FieldLike 只接受字符串请求值，并绑定为 `%value%`。参数缺失时跳过；显式 null 或其他类型返回 Validate 错误。显式提交空字符串会生成 `LIKE '%%'`，与缺失状态保持不同。

### 8.4 静态条件

- `EqValue` / `NeValue` 支持所有 Descriptor 字段类型；nil 分别编译为 `IS NULL` / `IS NOT NULL`，且字段必须 nullable；
- `In` 元素必须非 nil、类型一致且与目标字段兼容；
- `LikeValue` 只允许字符串字段，使用调用方提供的完整模式；
- GT/GTE/LT/LTE 只允许数字、字符串和时间字段；
- `Where` 中多个条件固定使用 AND；
- `RawWhere` 保留常量表达式和绑定参数，不解析或改写其内部 SQL。

请求值产生的错误归 Validate；QueryOp、Where 或 Extend 中的开发者配置错误归 Core。

## 9. 类型兼容

字段为 nullable 指针时，类型校验使用其元素类型；nil 仍只表达显式 null。

非 nil 值必须满足：

- 动态类型可直接赋值给字段的非指针 Go 类型；
- IN 的每个元素分别满足同一规则；
- 不执行字符串转数字、浮点转整数、有符号转无符号或命名类型隐式转换；
- 时间、`gtime.Time` 和 `[]byte` 必须使用字段声明的实际类型。

协议 Binder 后续应按 Descriptor 构造正确 Go 类型。模块 21 不引入第二套宽松转换器，避免溢出、精度损失和不同协议产生不同值。

## 10. Select 与输出名

未配置 Select 时默认等价于 `All("a")`。Join 不自动增加 `alias.*`。

`All(alias)` 按对应 Descriptor 的字段顺序展开持久化字段，输出名使用字段 JSON 名。`As(column, alias)` 选择单列并使用显式输出名。

展开完成后：

- 输出名必须唯一；
- 字段与别名必须存在；
- 同一物理列可以被显式选择多次，但必须使用不同输出名；
- 隐藏字段、只读字段和响应字段策略由模块 25 的 FieldPolicy 校验，本模块不提前拥有 Controller 策略。

默认只返回根实体字段是相对 Node 的安全收紧，避免 Join 字段覆盖根字段或未经声明进入响应。

## 11. Group、Having 与 Order

Group、Having 只来自 Extend 的追加节点。Group 字段按调用顺序保留；Having 条件使用 AND 组合，并在应用阶段一次性交给 GoFrame，避免多次 `Having` 覆盖前值。

Order 保留 QueryOp `AddOrderBy` 后接 Extend Order 的顺序。每个字段必须能由 Descriptor 解析，方向只能是 asc 或 desc。

客户端 `order/sort`、SortFields 白名单和默认排序属于 Controller/ActionPlan 后续模块，不在模块 21 读取普通请求字段并生成动态排序。

## 12. GoFrame 应用

内部应用器接收 Context、QueryPlan 和 Base 已取得的 `*gdb.Model`：

```text
校验输入
-> model.Clone()
-> Ctx(ctx)
-> As("a")
-> Join
-> Where
-> Fields
-> Group
-> Having
-> Order
-> 返回新 Model
```

应用规则：

- 表和列只来自 Descriptor；
- 标识符使用 GoFrame 当前数据库的引用能力；
- 普通值只通过占位参数传递；
- Join On、关键词 OR 分组和内部条件表达式只拼接已验证且已引用的标识符与固定操作符；
- 每次应用重新复制 slice/array 绑定值；
- 不调用 `Model.Raw/DB/TX/Unscoped`；
- 不执行 All、Scan、Count 或其他数据库 I/O。

应用器不会修改原 Model 或 QueryPlan。同一个 QueryPlan 可以被多个 goroutine 并发应用到不同 Model。

## 13. 分页与 Count

模块 21 不增加公开分页执行器。默认 Page 在 Base 中按以下路径执行：

```text
model, err := plan.ApplyQuery(ctx, baseModel)
-> model.Page(query.PageNumber(), query.PageSize())
-> model.AllAndCount(false)
```

GoFrame 2.10.2 的 `AllAndCount(false)` 会克隆数据 Model 和 Count Model。Count 使用 `COUNT(1)`，自动忽略分页与排序；存在 Group/Having 时，GoFrame 使用分组子查询计算结果行数。

因此数据与 total 共享 Join、Where、Group 和 Having，不需要 `getCountSql`、原生 SQL 分页器或数据库方言分支。

模块 22 的 `NewQuery` 校验 page/size 形状；后续 Binder/Service 应用分页、列表和导出数量上限。

## 14. 查询结构上限

模块 21 使用三个包内常量：

```text
最大 Join 数：8
最大规范化节点数：128
最大绑定值数：1000
```

规范化节点包括展开后的 Select、Where、Having、Group、Order、关键词字段、FieldEq 和 FieldLike。绑定值包括普通比较值、IN 元素和 RawWhere 参数。

静态 QueryOp 与 Extend 追加内容共同计数；`All(alias)` 按展开后的实际字段数计数。超过上限时，外部请求导致的值数量超限返回 Validate，静态或 Extend 结构超限返回 Core。

这些上限不是公开配置，不建立全局状态。出现真实业务证据后再在上位配置设计中增加可配置入口。

## 15. 错误语义

- nil Context、Resolver、根实体、Metadata、QueryPlan 或 Model：Core；
- 根实体、Join Entity、字段、别名、输出名、Join 可达性或静态类型错误：Core；
- FieldEq/FieldLike/keyWord 请求值的类型、null、空集合或数量错误：Validate；
- Extend 返回 Cool 异常：原样传播；
- Extend 返回普通错误：保留 Cause 并包装为 Core；
- GoFrame Model 应用前的内部不变量失败：Core；
- 后续 All/AllAndCount 的数据库错误：由 Base 包装为基础设施 Core。

错误消息可以包含实体、字段、别名和请求参数名，不得包含请求值、RawWhere 参数、Token、密码或数据库连接信息。

模块 21 不 recover DSL 构造器 Panic，也不把配置错误静默降级为跳过条件。

## 16. 不可变性与并发

编译时复制 QueryOp、Builder 和 QueryRequest 中使用的所有 slice/array；QueryPlan 不引用 Builder 内部集合。应用时再次复制可变绑定值，避免 GoFrame Model 与计划共享 slice。

QueryPlan 不缓存 Model、Context、SQL、执行结果或数据库方言。计划构造完成后只读，因此不需要锁。

同一输入必须产生节点顺序、输出字段顺序和绑定参数顺序一致的计划。

## 17. 文件职责

后续实现控制为：

| 文件 | 职责 |
| --- | --- |
| `cool-next/crud/plan.go` | QueryPlan、解析节点、内部编译与上限 |
| `cool-next/crud/apply.go` | 将 QueryPlan 参数化应用到克隆的 gdb.Model |
| `cool-next/crud/plan_test.go` | Resolver、字段、Join、请求匹配、Select、限制与错误测试 |
| `cool-next/crud/apply_test.go` | Model 应用、参数化、不变性与 SQLite 行为测试 |
| `cool-next/crud/fuzz_test.go` | 请求值、IN、别名和节点边界的最小模糊测试 |
| `cool-next/crud/plan_integration_test.go` | 使用 `package crud` 访问内部编译与应用函数，执行 MySQL、PostgreSQL、SQLite 查询矩阵 |

模块 21 不修改 Controller、Service、Application Host 或生成器。模块 25 再接入 ActionPlan 和生成图 Resolver。

## 18. 测试与验收

### 18.1 编译单元测试

至少覆盖：

1. 根别名、未限定字段和显式 `a`；
2. Join 别名重复、类型不匹配、向前引用和悬空 On；
3. 默认 Select、All 展开、As 和输出名冲突；
4. FieldEq 缺失、零值、false、null、标量、数组和空数组；
5. FieldLike、keyWord OR 分组和字符串类型限制；
6. Eq/Ne null、In 元素、比较操作符和 RawWhere；
7. Extend 恰好一次、追加顺序和错误传播；
8. 三项内部上限；
9. 输入集合修改不影响已编译计划。

### 18.2 应用测试

至少覆盖：

1. Apply 不修改原 Model；
2. Apply 不修改 QueryPlan；
3. 同一 Plan 重复和并发应用结果一致；
4. Join、Where、Select、Group、Having 和 Order 顺序稳定；
5. 注入形状的请求字符串只作为绑定值；
6. nil Context、Model 和 Plan 返回 Core。

### 18.3 三数据库矩阵

同一组 fixture 在 MySQL、PostgreSQL、SQLite 验证：

- camelCase 列引用；
- Left/Inner Join；
- 标量 Eq 与数组 IN；
- FieldLike 与 keyWord；
- Select 别名；
- Group/Having；
- 多字段 Order；
- Page 与 `AllAndCount(false)` 的 list/total 一致。

集成测试断言结果和参数化行为，不绑定 GoFrame 内部 SQL 格式，不建立整条 SQL Golden Test。
使用现有 `test/integration/run.sh -- go test ./cool-next/crud -run Integration -count=1 -v` 在三数据库矩阵环境中执行 `cool-next/crud` 包的集成测试，不修改脚本默认行为。

### 18.4 模块门禁

```bash
go test ./cool-next/crud -count=1
go test -race ./cool-next/crud -count=1
test/integration/run.sh -- go test ./cool-next/crud -run Integration -count=1 -v
go run ./cmd/cool check
go test ./... -count=1
go vet ./...
make check
git diff --check
```

## 19. 21.1-21.8 追踪

| 拆分项 | 设计落点 | 验收证据 |
| --- | --- | --- |
| `21.1` | 第 4、5、16 节 | 私有节点、副本和并发测试 |
| `21.2` | 第 6 节 | 根实体、字段、别名和 Join Resolver 测试 |
| `21.3` | 第 8.2 节 | 缺失跳过、标量 Eq、数组 IN 和 null 测试 |
| `21.4` | 第 6、9、11、14 节 | Select/Group/Having/Order 与上限测试 |
| `21.5` | 第 12 节 | 参数化 Model 应用与注入值测试 |
| `21.6` | 第 13、18.3 节 | 三数据库 Page/AllAndCount list/total 测试 |
| `21.7` | 第 12、16 节 | 原 Model、原 Plan 不变测试 |
| `21.8` | 第 18 节 | 复杂查询、Race、Fuzz 和三数据库矩阵 |

## 20. 完成标准

1. 21.1-21.8 均有实现、测试和可追溯验收证据；
2. QueryPlan 只保存不可变规范化数据，不保存闭包、Context 或 Model；
3. Resolver 在执行前完成实体、字段、别名和 Join 校验；
4. FieldEq、FieldLike、keyWord、null 和数组语义明确且参数化；
5. 默认 Select 只展开根实体，关联输出必须显式声明；
6. ApplyQuery 不修改原 Model 或 QueryPlan；
7. Page 数据与 Count 共享同一过滤计划；
8. 三数据库复杂查询结果一致；
9. 没有新增公开 CompileQuery、QueryLimits、SQL Builder、缓存或插件机制；
10. 未提前实现模块 22-57 的 Service、ActionPlan、Controller 或 Host 能力。
