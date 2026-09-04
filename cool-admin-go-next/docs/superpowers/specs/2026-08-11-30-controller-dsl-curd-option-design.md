# 模块 30：Controller DSL 与 CurdOption 设计

> 日期：2026-08-11
> 状态：已实现，自审通过
> 模块：30 Controller DSL 与 CurdOption
> 对应拆分项：30.1-30.11
> 前置模块：20 QueryRequest 与安全查询 AST、25 ActionPlan 与 CRUD Dispatcher

## 1. 目标

模块 30 在 `cool-next/core/controller` 建立默认 CRUD 的声明和编译边界。业务 Controller 使用密封 Builder 声明区域、路径、实体、Service、启用动作、查询配置和字段策略；框架在生成期确认源码绑定，在请求期把不可变配置编译为既有 `crud.ActionPlan`。

本模块必须：

1. 提供只能由 `Admin`、`App` 创建的链式 Builder；
2. 让 `Build` 返回外部无法构造或修改的 Definition；
3. 在生成期推导空路径并校验 Entity、Service、Base 泛型关系；
4. 完整定义 `CurdOption`、六种 `APIType`、静态/动态查询、`Before` 和强类型 `InsertParam`；
5. 分别处理 Page 和 List 查询，不建立隐式继承；
6. 把字段策略和查询配置交给 `crud.CompilePlan`，不复制查询编译器；
7. 限制动态查询和 Extend 不能改变默认 CRUD 的响应形状；
8. 对所有可变集合做防御性复制；
9. 不提前实现模块 31 的路由表或模块 32 的 HTTP Binder。

## 2. 非目标

本模块不实现：

- 自定义 Route、RouterOptions、中间件、权限、绑定来源和事务策略；
- 默认 CRUD HTTP 路由注册、模块/全局路由前缀组合和冲突检查；
- JSON、Query、Form、Path、File 绑定或 `gvalid` DTO 校验；
- Dispatcher 调用、Service Adapter 调用、HTTP 响应和异常过滤；
- EPS、OpenAPI、鉴权、Session 或 Transport；
- 运行期源码路径探测、Service 反射或第二套查询 AST。

上述能力分别属于模块 31、32、33、35、36 和 45。

## 3. 方案选择

采用“静态分析 + 薄 DSL”方案。

`core/controller` 只拥有 Controller 配置的密封、复制、解析和到 `crud.PlanInput` 的转换。字段、Join、Select、条件、排序和请求值继续使用 `cool-next/crud` 已有类型。`ActionPlan` 仍由 `crud.CompilePlan` 产生，Controller 不读取数据库 Model，也不执行 SQL。

默认路径和泛型匹配依赖源码位置及类型参数，必须使用现有 `go/ast`、`go/types` 分析器。禁止用 `runtime.Caller` 推导路径，也禁止在 `Build` 中反射 Service 内嵌字段。前者受内联和 `trimpath` 影响，后者把本应在生成期失败的配置错误推迟到启动或请求期。

模块 30 只增加 Controller 工厂声明的发现和校验结果，不生成路由表。模块 31 在同一分析模型上继续发现 Route、Middleware 并渲染静态注册片段，不重复 Controller 工厂发现。

## 4. 公开 DSL

模块 30 的公开入口固定为：

```go
type Definition interface {
   definition()
}

type Builder interface {
   Curd(CurdOption) Builder
   Build() Definition
}

func Admin(path string) Builder
func App(path string) Builder
```

`Builder` 和 `Definition` 的实现均不导出。外部不能使用结构体字面量构造、类型断言后修改或自行实现这两个接口。模块 31 冻结 `RouterOptions` 和 `Route` 后，再给密封 Builder 增加 `Options`、`Route` 方法；模块 30 不为尚未批准的路由类型创建占位声明。

同一 Builder 最多调用一次 `Curd`，重复声明属于程序配置错误。`Build` 不要求必须存在 Curd：这为模块 31 的纯自定义路由 Controller 保留同一 Definition 入口；对没有 Curd 的 Definition 调用默认 CRUD 编译或前置增强函数时返回 Core 配置错误。

`Admin` 和 `App` 保存区域及显式相对路径。非空路径使用 `/` 分段，拒绝绝对路径、空段、`.`、`..`、查询参数和片段。最终路径由区域根和相对路径组成：

```text
Admin("demo/goods") -> /admin/demo/goods
App("demo/goods")   -> /app/demo/goods
```

空路径由生成器根据模块身份、Controller 区域目录、子目录和文件名推导：

```text
modules/demo/controller/admin/goods.go     -> /admin/demo/goods
modules/demo/controller/admin/sys/user.go  -> /admin/demo/sys/user
modules/shop/controller/app/order/item.go  -> /app/shop/order/item
```

不为 `index.go`、`default.go` 等文件名增加特殊规则；文件名去掉 `.go` 后就是最后一个路径段。显式路径完全覆盖目录推导结果，但 `Admin` 必须位于 `controller/admin`，`App` 必须位于 `controller/app`，避免声明区域与源码区域不一致。

## 5. CurdOption 与 APIType

公开配置固定为：

```go
type CurdOption struct {
   Prefix             string
   API                []APIType
   PageQueryOp        QueryProvider
   ListQueryOp        QueryProvider
   InsertParam        InsertParam
   Before             BeforeFunc
   InfoIgnoreProperty []ColumnRef
   Entity             any
   Service            any
   URLTag             *URLTag

   HiddenFields   []ColumnRef
   ReadonlyFields []ColumnRef
   SortFields     []ColumnRef
   DefaultSort    ColumnRef
   DefaultOrder   Direction
}

type APIType string

const (
   APIAdd    APIType = "add"
   APIDelete APIType = "delete"
   APIUpdate APIType = "update"
   APIInfo   APIType = "info"
   APIList   APIType = "list"
   APIPage   APIType = "page"
)

func API(values ...APIType) []APIType
func AllAPI() []APIType
```

保留 `CurdOption` 拼写，不增加同义 `CrudOption`。`API` 和 `AllAPI` 每次返回新切片。API 必须属于六种固定动作且不能重复；空 API 表示不注册默认 CRUD。

`URLTag` 在本模块只定义 CurdOption 所需的最小值对象：名称和适用 API。构造时复制 API 切片并校验动作必须已在 CurdOption 启用；URL 为空表示应用于当前 CurdOption 启用的全部 API。标签如何影响路由、鉴权和冲突检查归模块 31、36 所有。

```go
type URLTag struct {
   Name string
   URL  []APIType
}

const TagIgnoreToken = "ignoreToken"
```

`Prefix` 为空时使用 Controller 的最终路径；非空时使用与 Admin/App 显式路径相同的区域内相对路径规则，并覆盖 Controller 路径。例如 Admin Controller 的 `Prefix: "demo/archive"` 编译为 `/admin/demo/archive`。Prefix 必须是生成期可见的字符串常量，由 Controller 分析器规范化并写入声明模型。

`Entity` 必须是非指针具名结构体零值，`Service` 必须是非 nil 指针。Builder 做不依赖源码的基础校验；准确的实体、Service 和 Base 泛型关系由生成期分析完成。

`id`、`createTime`、`updateTime` 的默认只读安全由 `service.Base.mutableData` 根据 Descriptor 的主键和系统维护字段元数据统一保证。Controller 只把用户声明的 `ReadonlyFields` 原样交给 `crud.FieldPolicyInput`，不重复注入系统字段，也不为此增加 ColumnRef 解包 API。Hidden、Readonly、InfoIgnore、SortFields、DefaultSort 和 DefaultOrder 的 Descriptor 校验继续由模块 25 的 `crud.CompilePlan` 完成。

## 6. 不可变性与复制

`Curd` 接收配置时复制全部外层切片和 `URLTag`。`Build` 再生成一份独立快照，避免 Builder 后续复用或调用方修改原切片影响已构建 Definition。

需要复制的集合包括：

- API；
- InfoIgnoreProperty、HiddenFields、ReadonlyFields、SortFields；
- URLTag.URL；
- Page/List 静态 QueryOp 中的 KeyWordLikeFields、Select、FieldLike、FieldEq、AddOrderBy、Join。

查询 AST 节点由 `crud` 的受控构造器创建，节点具体类型和字段均不导出，创建时已经复制可变值；Controller 只复制 QueryOp 的外层集合，不实现通用递归深拷贝。

函数值、Entity 零值和 Service 指针按身份保存。框架不复制函数闭包或 Service 单例，也不承诺业务代码并发修改 Service 自身状态的安全性。

## 7. QueryProvider

Controller 对 `crud` 查询类型使用真实别名和同名薄包装，不重新声明新类型：

```go
type Direction = crud.Direction
type ColumnRef = crud.ColumnRef
type QueryRequest = crud.QueryRequest
type RequestValue = crud.RequestValue
type QueryOp = crud.QueryOp
type FieldMatch = crud.FieldMatch
type FieldEq = crud.FieldEq
type FieldLike = crud.FieldLike
type JoinOp = crud.JoinOp
type JoinType = crud.JoinType
type Order = crud.Order
type QueryExtender = crud.QueryExtender
type WhereProvider = crud.WhereProvider
type Condition = crud.Condition
type SelectField = crud.SelectField
type QueryBuilder = crud.QueryBuilder
```

`Field`、`FieldOf`、Where、Condition、Select、Join、Order、RequestValue 和 Extend 的 Controller 函数只转调 `crud` 对应构造器。

QueryProvider 是密封接口，只能由以下函数创建：

```go
type QueryProvider interface {
   queryProvider()
}

func StaticQuery(QueryOp) QueryProvider
func DynamicQuery(func(context.Context) (QueryOp, error)) QueryProvider
```

StaticQuery 在构造时复制 QueryOp，解析时再次返回副本。DynamicQuery 保存非 nil 函数，每次请求恰好调用一次，并复制其返回值。PageQueryOp 和 ListQueryOp 独立解析；未配置的一侧使用空 QueryOp，不继承另一侧配置。

动态函数返回的已有框架异常保持原类型和错误链；普通错误直接作为 cause 包装为现有 Core 异常，并使用“解析动态查询配置失败”作为安全消息。错误消息不包含请求参数值。

## 8. 查询响应形状约束

`crud` 的 Select 具体节点和 QueryBuilder 追加节点均为私有状态。为避免 Controller 使用反射读取私有 AST，本模块在 `crud.QueryOp` 增加私有形状策略，并提供给 QueryProvider 使用的最小标记函数；实际校验和合并仍在 `crud.CompilePlan` 执行 Extend 后完成。不新增第二套 AST、Visitor 或公开节点 Getter。

规则固定为：

1. StaticQuery 外层 QueryOp.Select 是静态响应字段事实来源，可以声明根实体、Join 字段和输出别名。
2. StaticQuery 的 Extend 可以动态追加条件、Join、Group、Having 和 Order。
3. StaticQuery 的 Extend 如追加 Select，只能使用外层 QueryOp.Select 已声明的单字段输出别名；该动态节点替换同名静态节点，最终 SQL 只保留一个输出列。不得用 `All(alias)` 动态展开字段，不得产生新别名。
4. DynamicQuery 可以动态改变条件、Join、Group、Having 和 Order。
5. DynamicQuery 的 Select 只能为空或保持默认根实体形状，不得声明 `As` 输出别名、非根 `All(alias)`，其 Extend 也不得追加 Select。
6. StaticQuery 外层 Select 为空时只登记默认根实体形状，Extend 不得据此新增自定义输出别名。
7. 形状违规返回 Core 配置错误，必须发生在 `crud.CompilePlan` 返回 ActionPlan 之前。

该守卫只约束响应形状。字段存在性、Join 可达性、别名重复、节点上限和绑定值上限仍由既有查询计划编译器统一校验。

## 9. Before 与 InsertParam

公开入口固定为：

```go
type BeforeFunc func(context.Context) error

type InsertParam interface {
   insertParam()
}

func Insert[E any](
   func(context.Context, *service.Mutable[E]) error,
) InsertParam
```

`Insert` 拒绝 nil 函数并把实体泛型保存在不可伪造的私有实现中。模块 30 提供给后续生成 Adapter 使用的强类型执行函数；它从不可变 Definition 读取 InsertParam，接收同一实体类型的 Mutable 列表，按索引从前到后逐项调用，任一项失败立即停止并返回原错误。

```go
func ApplyInsertParam[E any](
   context.Context,
   Definition,
   []*service.Mutable[E],
) error
```

执行函数在调用前校验 Context、InsertParam 实体类型和 Mutable 非 nil。nil InsertParam 表示无服务端注入，是合法 no-op。错误发生时此前 Mutable 可能已被修改，但此阶段尚未进入 Dispatcher 或执行 DML，整个请求直接失败，不存在数据库部分成功。

`Before` 只是 Controller 前置回调值，不进入 ActionPlan。模块 30 提供以下消费入口，nil Before 是合法 no-op，回调错误原样返回：

```go
func ApplyBefore(context.Context, Definition) error
```

模块 32 的生成 Adapter 按既定顺序调用这些函数：Before、绑定与 gvalid、Add 逐项 InsertParam、编译 ActionPlan、进入 Dispatcher。模块 30 不提前实现该 HTTP 调用链。

## 10. ActionPlan 编译

Controller 提供一个协议无关编译入口，输入 Definition、动作、DescriptorResolver、Context 和可选 QueryRequest，输出既有 `*crud.ActionPlan`。

```go
func CompilePlan(
   context.Context,
   crud.DescriptorResolver,
   Definition,
   crud.Action,
   *crud.QueryRequest,
) (*crud.ActionPlan, error)
```

编译顺序固定为：

```text
校验 Definition、Context、动作及 API 是否启用
-> 复制 CurdOption 快照
-> Page 解析 PageQueryOp；List 解析 ListQueryOp；其余动作使用空 QueryOp
-> 应用查询响应形状守卫
-> 传递用户字段策略
-> 构造 crud.FieldPolicyInput 与 crud.PlanInput
-> crud.CompilePlan
```

Info 不使用 Page/List 查询配置，只由字段策略过滤响应。Add、Delete、Update 仍编译字段策略，但不创建 QueryPlan。Plan 编译入口不缓存请求相关结果；DynamicQuery、QueryRequest 和客户端排序都属于单次请求。

Controller 不实现 Descriptor 注册表。调用方使用生成图提供的既有 `crud.DescriptorResolver`，从而让根 Entity、Join Entity、字段、别名、默认排序和字段策略继续共享唯一元数据事实来源。

## 11. 生成期 Controller 校验

现有分析模型增加只读 ControllerDeclaration。模块 30 只发现位于 `modules/<module>/controller/admin/**` 或 `controller/app/**`、返回 `core/controller.Definition` 的顶层工厂函数。

为了保持静态分析确定性，Controller 工厂中的模块 30 DSL 必须满足：

- 返回值最终调用 `Admin` 或 `App` 和 `Build`，中间可以有零个或一个 `Curd`；
- Admin/App 路径是字符串常量；
- Curd 存在时，其参数是当前函数内可静态定位的 `CurdOption` 复合字面量；
- Curd 存在时，CurdOption.Prefix 是空值或字符串常量；
- Curd 存在时，Entity 是非指针具名实体零值；
- Curd 存在时，Service 引用工厂函数参数中的具体 Service 指针；
- InsertParam 如存在，其泛型实体可由 `go/types` 取得。

分析器校验：

1. 源码区域与 Admin/App 一致；
2. 空路径按模块、子目录和文件名推导，非空路径按显式覆盖规则规范化；
3. Curd 存在时，Service 是已发现的、直接匿名嵌入 `*service.Base[E, ID]` 的具体类型；
4. Curd 存在时，CurdOption.Entity 的类型与 Base 的 `E` 完全一致；
5. Curd 存在时，Base 的 `ID` 与实体 Descriptor 主键类型一致；当前实体约束下必须为 `uint64`；
6. InsertParam 的 `E` 与 CurdOption.Entity 一致；
7. 同一 Controller 工厂不能重复声明 Curd；无 Curd 的工厂仍写入 ControllerDeclaration，供模块 31 增加自定义 Route。

无法静态定位、类型不匹配或路径非法时返回带文件、行、列的新 `CGxxx` 诊断。模块 30 不渲染 Controller、Route、Middleware 或 Handler Adapter；模块 31 消费 ControllerDeclaration 并扩展剩余静态检查。

## 12. 错误处理

- DSL 构造器、Curd 或 Build 接收非法程序配置时沿用项目模式 Panic，并携带现有 Core 异常；这些入口的既定签名不返回 error；
- ActionPlan 编译和 InsertParam 执行中的可恢复配置错误返回 Core 异常；
- 请求排序、请求字段和协议输入错误继续由 `crud` 返回 Validate 异常；
- DynamicQuery 返回的已有框架异常保持原类型；普通错误直接作为 cause 包装为 Core 异常并增加“解析动态查询配置失败”消息，不预先重复 Wrap；
- Before、InsertParam 的回调错误原样返回，保持业务错误分类和 `errors.Is/errors.As` 链；
- 生成期错误使用稳定 Diagnostic Code 和源码位置，不输出运行期 Service 内容或请求值。

该处理遵循 GoFrame v2 `gerror.New/Newf` 创建带堆栈错误、`gerror.Wrap/Wrapf` 保留 cause 的官方约定，并复用项目现有 Exception 分类。

## 13. 文件职责

预计修改保持在以下真实职责内：

| 目录/文件 | 职责 |
| --- | --- |
| `cool-next/core/controller` | Definition、Builder、CurdOption、QueryProvider、InsertParam、别名包装和 Plan 编译 |
| `cool-next/core/controller/*_test.go` | DSL、复制、查询解析、InsertParam、字段策略和 Plan 编译测试 |
| `cool-next/crud` | 最小查询响应形状守卫 |
| `cool-next/crud/*_test.go` | Static/Dynamic Select 形状约束测试 |
| `cool-next/codegen` | Controller 工厂发现、路径推导、泛型绑定诊断和分析模型 |
| `cool-next/codegen/*_test.go` | 源码位置、路径、Entity/Service/Base/Insert 泛型测试 |

不新增 Controller registry、运行期 resolver、反射 helper、路由占位包或配置插件。

## 14. 30.1-30.11 追踪

| 拆分项 | 设计落点 | 验收证据 |
| --- | --- | --- |
| `30.1` | 第 4、6 节 | 密封 Builder/Definition 与构建后不可变测试 |
| `30.2` | 第 4 节 | 外部包无法构造 Builder 的编译测试 |
| `30.3` | 第 4、11 节 | Admin/App 默认、嵌套及显式路径测试 |
| `30.4` | 第 5 节 | 六种 API、重复和非法动作测试 |
| `30.5` | 第 7、10 节 | Page/List 独立解析和不继承测试 |
| `30.6` | 第 7、9 节 | Static/Dynamic、Before、Insert 类型测试 |
| `30.7` | 第 9 节 | 数组顺序、停止位置和错误传播测试 |
| `30.8` | 第 5、10 节 | 用户字段策略 Plan 与 Base 默认系统字段保护测试 |
| `30.9` | 第 11 节 | Entity/Service/Base/Insert 泛型诊断测试 |
| `30.10` | 第 6 节 | 输入、Builder、Definition 三方修改隔离测试 |
| `30.11` | 第 8 节 | Static Extend 与 Dynamic Select 形状测试 |

## 15. 测试与门禁

单元测试至少覆盖：

1. Admin/App 只能通过公开构造器取得 Builder，Definition 外部不可伪造；
2. 六种 API、空 API、重复 API、非法 API 和每次返回新切片；
3. CurdOption 所有切片和 URLTag 在 Curd、Build、读取/编译阶段互不影响；
4. Page/List 静态配置互不继承，DynamicQuery 每次只执行一次；
5. DynamicQuery 错误链、nil 回调和响应形状拒绝；
6. Static Extend 的已声明别名替换、新增别名拒绝、动态 All 拒绝和最终输出不重复；
7. InsertParam 单项/数组顺序、类型匹配、nil Mutable、首错停止和原错误传播；
8. 用户字段策略及 Info/Page/List QueryOp 正确转换到 ActionPlan，Base 默认系统字段保护不回归；
9. 无 Curd Definition、禁用 API、非法 Definition、nil Context 和 nil Resolver 被拒绝；
10. 生成器推导 admin/app、嵌套目录、显式覆盖和非法路径；
11. 生成器接受无 Curd 工厂，并拒绝区域不匹配、动态 Curd 配置、重复 Curd、错误 Service、Base Entity/ID 和 Insert Entity；
12. 现有 QueryPlan、Service Adapter 和唯一生成文件内容不回归。

模块门禁为：

```bash
go test ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen -count=1
go test -race ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen -count=1
go test ./... -count=1
go vet ./...
make check
git diff --check
```

模块 30 不连接数据库，不运行三数据库矩阵。数据库行为已经由模块 21、23-26 验收，本模块只证明配置编译得到相同 ActionPlan。

## 16. 不提前实现的边界

本模块明确不增加：

- `runtime.Caller`、运行期 Service 反射或 Controller 自动扫描；
- 可由外部实现的 QueryProvider、InsertParam、Builder 或 Definition；
- Query AST Getter、Visitor、通用 Clone、规则注册表或查询缓存；
- PageQueryOp 到 ListQueryOp 的回退；
- 自定义 Route、HTTP Method、Bind、Permission、Middleware、TransactionPolicy；
- Controller 静态注册表、HTTP Handler、Binder、Dispatcher Adapter、EPS 或 OpenAPI。

## 17. 完成标准

1. `30.1-30.11` 均有实现、测试和可追溯验收证据；
2. Controller DSL 只复用 `crud` 类型和编译器，没有第二套查询模型；
3. Build 后修改任一输入集合、Builder 或返回副本都不能改变 Definition；
4. 默认路径、Entity/Service/Base/Insert 泛型错误在生成期带源码位置失败；
5. Static/Dynamic 查询不能在请求期产生未静态登记的响应字段；
6. Page/List 配置独立，InsertParam 对数组按顺序逐项执行；
7. ActionPlan 继续由 `crud.CompilePlan` 唯一生成；
8. 未提前实现模块 31、32、33、45 的路由、HTTP 和文档能力。
