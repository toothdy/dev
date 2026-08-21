# 模块 20：QueryRequest 与安全查询 AST 设计

> 日期：2026-08-07
> 状态：待复核
> 模块：20 QueryRequest 与安全查询 AST
> 对应拆分项：20.1-20.8
> 前置模块：04 实体与 Descriptor 元数据、15 Cool CLI 与生成流水线

## 1. 目标

模块 20 在 `cool-next/crud` 定义协议无关、只可结构化构造的查询请求和查询 AST。它负责保留请求字段的 presence 语义，表达字段、条件、选择、关联、分组、Having 和排序，并为后续模块 21 编译不可变 QueryPlan 提供封闭输入。

本模块必须：

1. 区分请求字段缺失、零值、`false`、空字符串和显式 `null`；
2. 通过私有状态的 `ColumnRef` 表达逻辑字段、可选实体类型和表别名；
3. 只允许结构化条件或常量 `RawWhere` 加绑定参数；
4. 保持 Select、Join、Group、Having 和 Order 的调用顺序；
5. 提供只能追加节点的 `QueryBuilder`；
6. 在 `cool check` 中拒绝动态 SQL 表达式、字段名、别名和请求参数名；
7. 不连接数据库，不持有或执行 `gdb.Model`。

模块 20 不解析 Descriptor、不把逻辑字段翻译为数据库列、不编译 SQL、不处理分页或 Count，也不决定不同数据库的执行差异。这些职责属于模块 21。

## 2. 设计原则

### 2.1 封闭 AST

`Condition`、`WhereProvider` 和 `SelectField` 使用包内私有方法封闭实现，业务代码不能实现自定义节点绕过校验。所有公开构造器只产生框架已知节点，`QueryBuilder` 只追加节点，不暴露内部切片、原始模型、连接或清除既有条件的能力。

### 2.2 构造与编译分离

模块 20 只验证不依赖 Descriptor 和数据库的信息：名称语法、别名语法、方向、Join 类型、实体形状、集合形状和 AST 结构。模块 21 再结合 DescriptorResolver 验证字段存在性、实体归属、别名关系、值类型、查询上限和数据库执行。

### 2.3 外部输入与编程错误分层

`NewQueryRequest` 处理 HTTP、gRPC、任务或 Service 传入的数据，失败返回 Validate 异常。字段 DSL、Join、Select、条件和 Builder 属于开发者静态配置；其构造器签名已由上位设计冻结为无 `error` 返回，因此动态调用违反基础语法时 panic 一个 Core 异常。`cool check` 必须在常量可见时提前报告源码诊断，避免合法业务模块依赖运行时 panic 发现错误。

### 2.4 最小兼容

保留 Node 对外可见的 `FieldEq`、`FieldLike`、关键词模糊查询、Join 和有序排序语义，不复制 TypeORM QueryBuilder、字符串列名拼接、SQL 二维数组、客户端任意排序或数据库专属查询分支。

## 3. QueryRequest

公开契约固定为：

```go
type QueryRequest struct { /* 私有 presence-aware 值 */ }
type RequestValue struct { /* 私有字段名、值和 null 状态 */ }

func RequestField(name string, value any) RequestValue
func RequestNull(name string) RequestValue
func NewQueryRequest(values []RequestValue) (*QueryRequest, error)

func (request *QueryRequest) Has(name string) bool
func (request *QueryRequest) Value(name string) (any, bool)
func (request *QueryRequest) String(name string) (string, bool)
func (request *QueryRequest) Bool(name string) (bool, bool)
func (request *QueryRequest) Strings(name string) ([]string, bool)
```

`RequestField(name, nil)` 非法；显式 `null` 只能用 `RequestNull` 表达。`NewQueryRequest` 校验字段名、拒绝重复字段并冻结输入。请求字段名使用 lowerCamelCase，不允许点号、表别名、SQL 片段或路径。

读取语义固定为：

| 状态 | `Has` | `Value` | 强类型读取 |
| --- | --- | --- | --- |
| 缺失 | `false` | `nil, false` | 零值, `false` |
| 显式 `null` | `true` | `nil, true` | 零值, `false` |
| 类型匹配 | `true` | 值, `true` | 值, `true` |
| 类型不匹配 | `true` | 值, `true` | 零值, `false` |

强类型读取的第二个返回值表示“字段存在、非 null 且类型匹配”，不单独表示字段是否提交；调用方使用 `Has` 或 `Value` 区分缺失、null 和类型错误。`Strings` 只接受真实 `[]string`，不解析逗号字符串，并在返回时复制切片。

`RequestValue` 和 `QueryRequest` 不导出字段。构造时复制输入切片，读取时不返回内部可变切片。相同输入顺序不影响按名称读取结果。

## 4. 字段引用与实体类型

公开契约固定为：

```go
type ColumnRef struct { /* 私有逻辑字段、实体类型和别名 */ }

func NewColumnRef(name string) ColumnRef
func NewColumnRefOf[E any](name string) ColumnRef
func (field ColumnRef) Of(alias string) ColumnRef
```

字段名和别名均使用 lowerCamelCase 标识符规则。`NewColumnRef` 只记录当前根实体的逻辑字段；`NewColumnRefOf[E]` 额外记录 `reflect.TypeFor[E]()`，供模块 21 定位 Descriptor；`Of` 返回带别名的新值，不修改原引用。

泛型实体类型和 Join 实体必须是非指针、具名 struct。指针、slice、map、接口、匿名 struct 和 nil 均非法。模块 20 不实例化实体，也不根据 struct tag 重建 Descriptor。

`ColumnRef` 零值无效。字段是否真实存在、未显式实体的字段属于哪个根 Descriptor、别名是否对应已声明 Join，均由模块 21 校验。

## 5. 条件 AST

条件公开契约固定为：

```go
type Condition interface{ condition() }
type WhereProvider interface{ whereProvider() }

func Where(conditions ...Condition) WhereProvider
func EqValue(column ColumnRef, value any) Condition
func NeValue(column ColumnRef, value any) Condition
func In(column ColumnRef, values any) Condition
func LikeValue(column ColumnRef, value string) Condition
func RawWhere(expression string, args ...any) Condition
func On(left ColumnRef, right ColumnRef) Condition
```

`Where` 按传入顺序保存条件并固定使用 AND 组合，不增加公开 OR DSL。`KeyWordLikeFields` 所需的 OR 分组由模块 21 在包内编译，不扩大业务公开接口。

`EqValue`、`NeValue` 和比较节点保存结构化字段与绑定值。`In` 只接受非空 slice 或 array，复制集合后保存；nil、标量和空集合属于无效 DSL。集合元素是否与字段类型匹配由模块 21 验证。

`LikeValue` 使用调用方提供的完整 LIKE 模式，不自动添加 `%`。`FieldLike` 属于请求匹配语义，由模块 21 读取请求字符串后自动编译为 `%value%`。两者不能混用隐式规则。

`On` 只表达两个 `ColumnRef` 的相等关联，不接受业务值或原始表达式。当前公开 API 不增加 `Or`、`Not`、任意操作符、子查询或表达式解析器。

## 6. Select、Join、Group、Having 与 Order

公开值和构造器固定为：

```go
type Direction string

const (
   Ascending  Direction = "asc"
   Descending Direction = "desc"
)

type JoinType string

const (
   JoinLeft  JoinType = "left"
   JoinInner JoinType = "inner"
)

type SelectField interface{ selectField() }

type JoinOp struct {
   Entity    any
   Alias     string
   Condition Condition
   Type      JoinType
}

type Order struct {
   Column    ColumnRef
   Direction Direction
}

func All(alias string) SelectField
func As(column ColumnRef, alias string) SelectField
func Asc(column ColumnRef) Order
func Desc(column ColumnRef) Order
func LeftJoin(entity any, alias string, on Condition) JoinOp
func InnerJoin(entity any, alias string, on Condition) JoinOp
```

`All` 选择已声明别名对应实体的全部可见字段；是否允许、具体展开字段和字段安全策略由模块 21/25 决定。`As` 的输出别名使用 lowerCamelCase，不能包含点号、引号、空白或 SQL 片段。

Join 构造器只接受非指针、具名 struct 实体值、合法别名和 `On` 条件。模块 20 保存声明顺序；模块 21 校验别名唯一性、实体 Descriptor、关联字段和 Join 可达性。

排序只接受 `Ascending` 或 `Descending`。Group 和 Having 不在 `QueryOp` 增加重复字段，统一通过 `QueryBuilder.AddGroupBy` 和 `AddHaving` 追加，保持上位 API 不变。

## 7. QueryOp 与 QueryBuilder

公开查询配置固定为：

```go
type QueryOp struct {
   KeyWordLikeFields []ColumnRef
   Where             WhereProvider
   Select            []SelectField
   FieldLike         []FieldLike
   FieldEq           []FieldEq
   AddOrderBy        []Order
   Join              []JoinOp
   Extend            QueryExtender
}

type FieldMatch struct {
   Column       ColumnRef
   RequestParam string
}

type FieldEq = FieldMatch
type FieldLike = FieldMatch

func Eq(column ColumnRef) FieldEq
func EqFrom(column ColumnRef, requestParam string) FieldEq
func Like(column ColumnRef) FieldLike
func LikeFrom(column ColumnRef, requestParam string) FieldLike
```

`Eq` 和 `Like` 默认使用字段逻辑名读取请求；`EqFrom` 和 `LikeFrom` 使用显式 lowerCamelCase 请求参数名。`FieldEq` 的数组请求值在模块 21 编译为参数化 `IN`；字段缺失时跳过条件。`FieldLike` 只接受字符串请求值并由模块 21 包裹 `%`。

`QueryOp` 中所有 slice 在进入后续编译边界时复制，顺序具有语义。`AddOrderBy` 禁止改为 map。`KeyWordLikeFields` 的关键词请求参数名和 OR 组合规则由模块 21 冻结，本模块只保存字段列表。

扩展契约固定为：

```go
type QueryExtender func(
   context.Context,
   *QueryBuilder,
   *QueryRequest,
) error

func Extend(QueryExtender) QueryExtender

type QueryBuilder struct { /* 私有有序节点 */ }

func (query *QueryBuilder) Where(conditions ...Condition) *QueryBuilder
func (query *QueryBuilder) WhereGT(column ColumnRef, value any) *QueryBuilder
func (query *QueryBuilder) WhereGTE(column ColumnRef, value any) *QueryBuilder
func (query *QueryBuilder) WhereLT(column ColumnRef, value any) *QueryBuilder
func (query *QueryBuilder) WhereLTE(column ColumnRef, value any) *QueryBuilder
func (query *QueryBuilder) AddSelect(fields ...SelectField) *QueryBuilder
func (query *QueryBuilder) AddJoin(joins ...JoinOp) *QueryBuilder
func (query *QueryBuilder) AddGroupBy(fields ...ColumnRef) *QueryBuilder
func (query *QueryBuilder) AddHaving(conditions ...Condition) *QueryBuilder
func (query *QueryBuilder) AddOrderBy(orders ...Order) *QueryBuilder
```

每个方法按调用顺序追加节点并返回同一 Builder 以支持链式调用。Builder 不提供替换表、清空条件、读取内部节点、取得原始 Model、执行查询或访问数据库的方法。nil Builder 属于编程错误，不提供静默 no-op。

`Extend` 只保留非 nil 函数。函数何时执行、如何把追加节点冻结进 QueryPlan 和错误如何传播，由模块 21 定义。

## 8. RawWhere 与源码静态检查

`RawWhere` 是受控逃生口，不是通用 SQL Builder。运行时构造器只接受非空表达式并复制绑定参数；它不解析 SQL、不统计占位符，也无法从一个 `string` 判断其源码是否为常量。

因此模块 20 同时扩展现有 codegen 分析规则：

1. 按 `go/types` 函数身份识别 `crud.RawWhere`，不按文本函数名猜测；
2. 第一个参数必须可由 `go/types` 求值为编译期字符串常量；
3. 允许字符串字面量、命名常量和编译期常量拼接；
4. 拒绝变量、函数返回值、`fmt.Sprintf` 和其他运行时表达式；
5. 业务值只能出现在后续绑定参数中；
6. 诊断包含源码位置和稳定 codegen 错误码，不打印绑定值。

同一静态规则校验 `NewColumnRef`、`NewColumnRefOf`、`ColumnRef.Of`、`EqFrom`、`LikeFrom`、`All`、`As` 和 Join 构造器中的字段名、请求参数名与别名必须是编译期字符串常量。公开 API 不提供表名参数，因此不存在动态换表入口。

运行时基础校验与源码静态检查互补：静态检查提供提前定位，运行时保护动态调用边界。两者都不实现 SQL parser，也不把 `RawWhere` 扩展为可拼接业务值的接口。

## 9. 错误模型

- `NewQueryRequest` 的字段名、nil 值、重复字段等外部输入错误包装为模块 02 Validate 异常；
- DSL 构造、实体形状、方向、Join、集合和 Builder 的编程错误包装为 Core 异常并由无错误返回的构造器 panic；
- 后续模块 21 的 Descriptor、字段、别名和值类型编译错误返回 Core 异常；
- 源码中的动态名称或动态 RawWhere 表达式使用现有 `DiagnosticError` 和稳定错误码；
- 错误消息可以包含字段名、别名和源码位置，不包含请求值、RawWhere 绑定参数或敏感数据。

本模块不新增公开错误类型，复用 `gerror` 保留调用栈并通过 `exception.WrapValidate` / `exception.WrapCore` 统一分类。

## 10. 数据流与不可变性

```text
协议 Binder / Service
-> RequestField / RequestNull
-> NewQueryRequest
-> QueryOp + QueryBuilder 结构化节点
-> 模块 21 DescriptorResolver 与 QueryPlan 编译
-> 模块 21 参数化应用到 gdb.Model
```

模块 20 的公开值不暴露私有节点。构造器复制传入的 slice/array 外层，访问器返回副本；`ColumnRef.Of` 返回新值；Builder 只在单次编译流程中追加，模块 21 冻结 QueryPlan 时再次复制。调用方修改原始切片或 getter 返回切片不能改变已构造请求。

模块 20 不建立全局注册表、缓存或锁。AST 是请求或配置范围内的普通值，不需要引入接口实现注册器、工厂或插件系统。

## 11. 文件职责

| 文件 | 职责 |
| --- | --- |
| `cool-next/crud/request.go` | RequestValue、QueryRequest、presence 与强类型读取 |
| `cool-next/crud/ast.go` | ColumnRef、Condition、Where、Select、Join 与 Order 节点 |
| `cool-next/crud/query.go` | QueryOp、FieldMatch、QueryExtender 与 QueryBuilder |
| `cool-next/crud/errors.go` | Validate/Core 异常包装与 DSL panic 边界 |
| `cool-next/crud/request_test.go` | presence、类型读取、重复字段和副本测试 |
| `cool-next/crud/ast_test.go` | 字段、条件、Join、Select、顺序与非法 DSL 测试 |
| `cool-next/crud/query_test.go` | QueryOp、FieldMatch、Builder 追加与封闭边界测试 |
| `cool-next/crud/fuzz_test.go` | 请求字段和 AST 构造输入的最小模糊测试 |
| `cool-next/codegen/query_validate.go` | 查询 DSL 常量参数和 RawWhere 静态规则 |
| `cool-next/codegen/query_validate_test.go` | 常量、常量拼接、动态表达式和诊断位置测试 |

不修改 `core/entity`、`db`、`core/service` 或 `core/controller`。codegen 只增加模块 20 已冻结规则，不提前分析模块 21-57 的 API。

## 12. 20.1-20.8 追踪

| 拆分项 | 设计落点 | 验收证据 |
| --- | --- | --- |
| `20.1` | 第 3 节 QueryRequest | presence、typed getter、null 和副本测试 |
| `20.2` | 第 4 节 ColumnRef 与实体 | 名称、实体类型、别名和零值测试 |
| `20.3` | 第 5 节条件 AST | Eq、Ne、In、Like、On 与 Where 顺序测试 |
| `20.4` | 第 6 节查询节点 | Select、Alias、Join、Group、Having、Order 测试 |
| `20.5` | 第 7 节 QueryOp 与 Builder | FieldEq、FieldLike 和追加式 Builder 测试 |
| `20.6` | 第 8 节 RawWhere | 常量、常量拼接、动态表达式和绑定参数测试 |
| `20.7` | 第 4、8 节名称边界 | 动态字段/别名拒绝、无表名入口和 SQL 拼接测试 |
| `20.8` | 第 2、10、13 节协议边界 | 依赖检查与无数据库执行测试 |

## 13. 测试与验收

单元测试至少覆盖：

1. 缺失、零值、`false`、空字符串、显式 null、类型错误和 `[]string` 副本；
2. 空名、非法字段名/别名、重复请求字段、nil RequestField；
3. `ColumnRef.Of` 不修改原值，实体类型和 Join 实体限制生效；
4. Eq、Ne、In、Like、On、Where 的节点类型和输入顺序；
5. In 的非空 slice/array、空集合、nil、标量和输入副本；
6. Select、Join、Group、Having、Order 和多次 Builder 调用的追加顺序；
7. FieldEq 默认/显式请求参数、FieldLike 默认/显式请求参数；
8. nil Extender、nil Builder 和无效 DSL 的 Core panic；
9. RawWhere 字面量、命名常量、常量拼接、变量、格式化字符串和错误位置；
10. `crud` 不 import `gdb`、`ghttp`、gRPC 协议类型或业务模块；
11. 模糊输入不会绕过名称、集合和封闭接口边界；
12. Race Test 下并发读取同一 QueryRequest 不产生数据竞争。

模块门禁为：

```bash
go test ./cool-next/crud -count=1
go test ./cool-next/codegen -run 'TestQueryValidation' -count=1
go test -race ./cool-next/crud ./cool-next/codegen -count=1
go run ./cmd/cool check
go test ./... -count=1
go vet ./...
make check
git diff --check
```

模块 20 不运行三数据库矩阵测试，因为没有数据库连接或 SQL 执行。数据库查询语义和跨方言验收从模块 21 开始。

## 14. 不提前实现的边界

本模块明确不实现：

- DescriptorResolver、QueryPlan、SQL 编译、`ApplyQuery`、分页和 Count；
- 字段存在性、字段类型、Join 可达性和查询上限校验；
- HTTP Binder、Controller 别名包装、Service Query 或 CRUD Dispatcher；
- 客户端动态字段、动态表名、任意排序、裸 SQL 值拼接或裸 `gdb.Model`；
- OR、NOT、子查询、Union、窗口函数或数据库专属表达式 DSL；
- Node 的 TypeORM QueryBuilder、`setSql`、`getOptionFind` 或三数据库复制实现；
- 为后续查询节点预建 visitor、插件、序列化格式或公开内部 AST；
- 模块 21 以后才需要的数据库 fixture 和三数据库集成测试。

## 15. 完成标准

1. 20.1-20.8 均有实现、测试和可追溯验收证据；
2. QueryRequest 完整保留 presence 与显式 null 语义；
3. 查询只能通过封闭结构化节点或常量 RawWhere 加绑定参数表达；
4. 动态字段名、别名、请求参数名和 RawWhere 表达式在 `cool check` 阶段拒绝；
5. QueryBuilder 只追加节点，不暴露数据库或内部 AST；
6. 公开集合不泄露内部可变状态，调用顺序稳定；
7. `cool-next/crud` 不 import 数据库、Transport、Controller 或业务模块；
8. 未提前实现模块 21 的 Descriptor 解析、QueryPlan 或查询执行。
