# 模块 22：Service 输入输出与 Mutable 设计

> 日期：2026-08-10
> 状态：已确认
> 模块：22 Service 输入输出与 Mutable
> 对应拆分项：22.1-22.9
> 前置模块：05 DOValue 与字段四态、20 QueryRequest 与安全查询 AST

## 1. 目标

模块 22 在 `cool-next/core/service` 定义六种 CRUD 共用的协议无关输入输出容器。它以 Descriptor 为字段和类型事实来源，保留字段 presence、显式 null、输入形状和字段来源，使 HTTP、gRPC、任务、事件与业务 Service 能通过同一组构造入口进入后续 Base、ActionPlan 和 Dispatcher。

本模块必须：

1. 定义 Descriptor 驱动的 `Mutable[E]`，保留客户端与服务端字段来源；
2. 区分字段缺失、零值、`false`、普通值和显式 `null`；
3. 定义 Add、Delete、Update 和 Query 的不可伪造输入形状；
4. 通过 Smart Constructor 在进入数据库或事务前校验输入；
5. 保持单对象与顶层数组的原始形状和顺序；
6. 定义只读 `Record`、分页结果和保持 Add 输入形状的响应；
7. 保证 `uint64`、时间、字节、零值、`false` 和 `null` 不失真；
8. 不引入数据库、Transport、Controller 或业务模块依赖。

## 2. 非目标

本模块不实现：

- Base 的 Model、Tx、Info、List、Page、Add、Update 或 Delete；
- Mutable 到 DOValue 的转换和任何数据库写入；
- Hidden、Readonly、InfoIgnore、SortFields 或其他 Controller 字段策略；
- `InsertParam`、Modify Hook、ActionPlan、Operation Scope 或 Dispatcher；
- HTTP/gRPC 请求绑定、响应包装、权限校验或路由；
- 批量大小上限、删除 ID 去重、回收站归档或恢复；
- 为未来协议预建 DTO 注册器、序列化插件或第二套输入模型。

模块 23-26 在本模块的已验证输入之上完成数据库、字段策略、事务和回收站行为，不得绕过本模块重新解析动态 map。

## 3. 公开边界

公开类型固定为：

```go
type Mutable[E any] struct{ /* private fields */ }
type FieldValue struct{ /* private presence-aware field */ }

type AddInput[E any] struct{ /* private fields */ }
type DeleteInput[ID comparable] struct{ /* private fields */ }
type UpdateItem[E any, ID comparable] struct{ /* private fields */ }
type UpdateInput[E any, ID comparable] struct{ /* private fields */ }
type AddResult[ID comparable] struct{ /* private fields */ }

type QueryRequest = crud.QueryRequest
type Query struct{ /* private fields */ }
type Record struct{ /* private fields */ }

type Pagination struct {
   Page  int   `json:"page"`
   Size  int   `json:"size"`
   Total int64 `json:"total"`
}

type PageResult struct {
   List       []Record   `json:"list"`
   Pagination Pagination `json:"pagination"`
}
```

`Mutable`、FieldValue、全部输入、AddResult、Query 和 Record 不导出内部状态。调用方只能使用本文列出的构造器与访问器；零值只用于表示无效或未构造状态，不能作为合法 CRUD 输入。

## 4. Mutable 与字段状态

`Mutable[E]` 保存 Descriptor 身份、实体类型、主键类型以及按逻辑字段名索引的私有字段状态。外部构造和访问统一使用 Descriptor 的 JSON 字段名，内部存储统一使用逻辑字段名，不根据 Go 字段名、列名或字符串转换自行推导。

公开方法固定为：

```go
func Value(field string, data any) FieldValue
func Null(field string) FieldValue

func NewMutable[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []FieldValue,
) (*Mutable[E], error)

func (value *Mutable[E]) Has(field string) bool
func (value *Mutable[E]) IsNull(field string) bool
func (value *Mutable[E]) Get(field string) (any, bool)
func (value *Mutable[E]) Set(field string, data any) error
func (value *Mutable[E]) SetNull(field string) error
```

字段状态语义固定为：

| 状态 | Has | IsNull | Get |
| --- | --- | --- | --- |
| 未出现 | `false` | `false` | `nil, false` |
| 零值或 `false` | `true` | `false` | 原类型值, `true` |
| 普通值 | `true` | `false` | 原类型值, `true` |
| 显式 `null` | `true` | `true` | `nil, true` |

`Value(field, nil)` 和 typed nil 非法；显式空值只能由 `Null` 或 `SetNull` 表达。`SetNull` 只允许用于 Descriptor 标记为 nullable 的字段。普通值必须与字段 Go 类型精确匹配；nullable 指针字段提交普通值时使用其元素类型，不能用宽松数字转换、字符串解析或 JSON 猜测类型。

## 5. 字段来源

字段来源是模块 24 安全清理所需的包内状态，不属于业务公开 API。内部只保留两个来源：

```text
client  客户端或其他协议入口构造的原始输入
server  可信服务端扩展通过 Mutable.Set/SetNull 写入的值
```

来源转换规则固定为：

1. `NewMutable` 接收的每个 `FieldValue` 标记为 `client`；
2. `Set` 和 `SetNull` 成功后将目标字段标记为 `server`；
3. 服务端覆盖客户端字段时，值、null 状态和来源一起原子替换；
4. 任一名称、类型或 nullable 校验失败时，原值和原来源保持不变；
5. 字段来源不影响 Has、IsNull 和 Get 的读取语义；
6. 来源不进入 JSON、日志、EPS、OpenAPI、数据库或 Transport；
7. 来源类型和读取能力保持包内私有，协议输入不能直接声明 `server` 来源。

HTTP/gRPC Binder、任务、事件和直接 Service 调用都通过 FieldValue + NewMutable 构造原始输入，因此初始来源统一视为客户端输入。`InsertParam` 等可信框架扩展只能通过 Set/SetNull 改写字段，由此得到服务端来源。模块 24 最终清理客户端 ID 和只读字段时必须保留服务端覆盖值。

本模块不提供公开 `FieldSource`、`ServerValue` 或来源参数。可信 Go 代码调用 Set/SetNull 本身就是显式服务端动作；协议 Binder 只能使用 FieldValue + NewMutable，不能把请求中的来源标记透传进来。

## 6. Add 输入与结果

公开契约固定为：

```go
func NewAddObject[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   *Mutable[E],
) (AddInput[E], error)

func NewAddArray[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []*Mutable[E],
) (AddInput[E], error)

func (in AddInput[E]) IsMany() bool
func (in AddInput[E]) One() *Mutable[E]
func (in AddInput[E]) Many() []*Mutable[E]

func (result AddResult[ID]) IsMany() bool
func (result AddResult[ID]) One() ID
func (result AddResult[ID]) Many() []ID
func (result AddResult[ID]) MarshalJSON() ([]byte, error)
```

单对象和顶层数组必须调用不同构造器，不能根据 slice 长度推断原始形状。数组不能为空，所有 Mutable 必须属于同一实体和主键类型，并保持输入顺序。返回的 slice 是副本，修改副本不能改变输入容器。

AddResult 由后续 Base 写入在 `core/service` 包内构造，必须保持 AddInput 形状和 ID 顺序。JSON 固定为单对象 `{"id": scalar}` 或数组 `{"id": [...]}`；单对象与长度为一的数组不得合并为同一形状。

## 7. Delete 与 Update 输入

公开契约固定为：

```go
func NewDeleteInput[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []ID,
) (DeleteInput[ID], error)

func (in DeleteInput[ID]) IDs() []ID

func NewUpdateItem[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   ID,
   *Mutable[E],
) (UpdateItem[E, ID], error)

func NewUpdateObject[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   UpdateItem[E, ID],
) (UpdateInput[E, ID], error)

func NewUpdateArray[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []UpdateItem[E, ID],
) (UpdateInput[E, ID], error)

func (in UpdateInput[E, ID]) IsMany() bool
func (in UpdateInput[E, ID]) One() UpdateItem[E, ID]
func (in UpdateInput[E, ID]) Many() []UpdateItem[E, ID]
func (item UpdateItem[E, ID]) ID() ID
func (item UpdateItem[E, ID]) Mutable() *Mutable[E]
```

Delete ID 列表和 Update 数组不能为空。每个 ID 的动态类型必须与 Descriptor 主键类型精确一致；每个 Mutable 必须属于同一 Descriptor 实体、主键类型和表。Update 同样分别保留单对象和顶层数组形状，返回集合使用防御性副本。

空 ID、重复 ID 去重、批量硬上限和不存在 ID 的业务语义由模块 24-26 在进入事务前统一处理。本模块只承担拆分项 22.5 明确要求的空批次、重复字段、Descriptor/ID 类型与形状校验。

## 8. Query、Record 与分页结果

公开契约固定为：

```go
func NewQuery(
   *crud.QueryRequest,
   page int,
   size int,
) (Query, error)

func (query Query) Request() *QueryRequest
func (query Query) PageNumber() int
func (query Query) PageSize() int

func (record Record) Get(field string) (any, bool)
func (record Record) Scan(pointer any) error
func (record Record) MarshalJSON() ([]byte, error)
```

QueryRequest 的实现归模块 20 所有，`core/service` 只提供类型别名。NewQuery 要求 page 和 size 都为正数；分页大小上限和动作差异由模块 23/25 应用。nil QueryRequest 表示没有额外请求字段，是合法输入。

Record 由模块 23/25 在包内从已验证、已过滤的查询结果构造，可以包含根实体字段和显式 Select/Join 别名。它不暴露底层 map；Get 返回受支持可变值的副本，MarshalJSON 输出实际字段集合，Scan 解码到调用方提供的目标对象。编码或扫描失败包装为 Core 异常并保留 cause。

Pagination 和 PageResult 是输出 DTO。PageResult.List 必须编码 Record 的实际字段，不能因 Record 私有存储而得到空对象。

## 9. 校验与错误模型

Smart Constructor 和 Mutable 写入按以下顺序校验：

1. Descriptor 非 nil，实体类型、主键类型和主键元数据匹配泛型参数；
2. FieldValue 已由 Value/Null 构造，JSON 字段存在且不重复；
3. 普通值非 nil、类型精确匹配；显式 null 只用于 nullable 字段；
4. Add/Delete/Update 数组非空；
5. Mutable 和 UpdateItem 属于目标 Descriptor；
6. Query 的 page、size 为正数。

输入字段、值类型、null、空批次、Descriptor 绑定和形状错误统一包装为模块 02 Validate 异常。Record 的 JSON 编码或 Scan 失败包装为 Core 异常。错误使用 GoFrame `gerror.Newf`、`gerror.Wrap` 保留堆栈与 cause，不在消息中输出字段值、Token 或其他敏感内容。

所有会修改 Mutable 的方法必须先完成全部校验再写状态。失败不得留下部分字段、错误来源或半更新值。

## 10. 数据流

```text
HTTP / gRPC / Task / Event / Service 输入
-> Value / Null
-> NewMutable（字段来源 client）
-> NewAdd* / NewUpdate* / NewDeleteInput / NewQuery
-> Controller 前置增强与 InsertParam
-> Mutable.Set / SetNull（字段来源 server）
-> 模块 24 字段策略与 DOValue 转换
-> Base DML / Dispatcher
```

模块 22 不持有 Context、数据库、事务、Model、ActionPlan 或协议请求对象。输入容器是单次操作范围内的普通值，不保证并发写安全；构造完成后的形状集合和输出 Record 支持并发只读。调用方不得并发修改同一个 Mutable。

## 11. 副本与精度

- 构造器复制传入的 ID、Mutable 指针和 UpdateItem 外层 slice；访问器返回新的外层 slice；
- 实体当前允许的可变字段类型只有字节切片，Value、Set 和 Get 必须复制字节内容；
- 标准时间、GoFrame 时间、整数、浮点、布尔和字符串按精确 Go 类型保存，不经字符串或 float64 中转；
- Record 的 JSON/Scan 测试必须包含 `math.MaxUint64` 和时间值，证明不会因中间转换丢失；
- 显式 null 只保存 null 状态，本模块不提前创建 `gdb.Raw("NULL")`；模块 24 转换到 DOValue 时再使用模块 05 的明确 NULL 语义。

本模块不实现通用递归深拷贝。Descriptor 已拒绝 map、任意 slice 和不受支持的复合字段，当前只需正确复制 `[]byte` 与容器外层 slice。

## 12. 文件职责

| 文件 | 职责 |
| --- | --- |
| `cool-next/core/service/input.go` | Mutable、字段来源、CRUD 输入输出、Smart Constructor、校验与副本 |
| `cool-next/core/service/input_test.go` | 四态、来源转换、类型、形状、不可变性、序列化和错误测试 |
| `cool-next/crud` | 不修改；复用模块 20 的 QueryRequest |
| `cool-next/core/entity` | 不修改；复用模块 04/05 的 Descriptor 与类型契约 |

不拆分新的 source、result、clone 或 errors 文件。当前能力在单个输入实现文件内仍可直接理解，等模块 23/24 引入 Base 与 DML 后再按真实职责拆分，避免为目录整齐提前增加文件。

## 13. 22.1-22.9 追踪

| 拆分项 | 设计落点 | 验收证据 |
| --- | --- | --- |
| `22.1` | 第 4、5 节 | Descriptor 绑定、client/server 来源与覆盖测试 |
| `22.2` | 第 4 节 | Has、IsNull、Get、Set、SetNull 四态测试 |
| `22.3` | 第 6、7 节 | Add/Delete/Update 类型和形状测试 |
| `22.4` | 第 4、6-8 节 | 全部 Smart Constructor 成功与失败测试 |
| `22.5` | 第 7、9 节 | 空批次、重复字段、错误 ID/Descriptor 测试 |
| `22.6` | 第 6 节 | 单对象/数组 AddResult 与顺序测试 |
| `22.7` | 第 8 节 | Record、Query、Pagination、PageResult 测试 |
| `22.8` | 第 4、11 节 | uint64、时间、字节、零值、false、null 测试 |
| `22.9` | 第 3、10 节 | 无协议依赖和共用构造入口检查 |

## 14. 测试与门禁

单元测试至少覆盖：

1. 字段缺失、零值、`false`、普通值、显式 null 和 typed nil；
2. NewMutable 字段来源为 client，Set/SetNull 成功后来源为 server；
3. 服务端覆盖客户端普通值或 null 时同步替换来源；
4. 失败的 Set/SetNull 不改变原值、null 状态和来源；
5. 未知、重复、类型错误和非 nullable null 字段拒绝；
6. Descriptor 实体、主键、表和 Mutable 归属不匹配时拒绝；
7. Add/Update 单对象与数组形状、顺序、空数组和返回 slice 副本；
8. Delete ID 列表类型、空列表、顺序和副本；
9. AddResult 标量/数组 JSON 与 ID 顺序；
10. Query 正数分页、nil 请求和 QueryRequest 原对象读取；
11. Record Get、Scan、JSON、PageResult、最大 uint64、时间和字节副本；
12. 错误可由 `errors.As` 识别为正确的 Validate/Core 异常；
13. `core/service` 不 import gdb、ghttp、gRPC 协议类型或业务模块；
14. Race Test 下并发读取已构造输入和 Record 无数据竞争。

模块门禁为：

```bash
go test ./cool-next/core/service -count=1
go test -race ./cool-next/core/service -count=1
go test ./... -count=1
go vet ./...
make check
git diff --check
```

模块 22 不连接数据库，不运行三数据库矩阵。DOValue、DML 和跨数据库验收分别属于模块 24 和第三阶段总验收门。

## 15. 不提前实现的边界

本模块明确不增加：

- 公开 FieldSource、来源 Getter、来源构造参数或可伪造的 ServerValue；
- Mutable 字段迭代、删除、清空、merge、patch、map 导入或反射 DTO 绑定；
- AddResult/Record 的公开任意构造器；
- QueryRequest 的第二份实现或分页配置对象；
- 通用深拷贝、序列化注册器、Validator 插件或输入缓存；
- 模块 23 的 Base/NativeQuery、模块 24 的 Hook/DML、模块 25 的 ActionPlan 或模块 26 的回收站行为。

## 16. 完成标准

1. 22.1-22.9 均有实现、测试和可追溯验收证据；
2. Mutable 完整保留四态和 client/server 字段来源；
3. 来源只能由 NewMutable 与 Set/SetNull 的固定路径产生，协议输入不能直接伪造；
4. Smart Constructor 拒绝非法 Descriptor、字段、类型、null、空批次和形状；
5. Add/Update 输入与 AddResult 保留单对象/数组形状和顺序；
6. Record、Query 和 PageResult 不丢失 uint64、时间、零值、false 或字段集合；
7. 所有集合和字节值遵守必要的防御性复制，失败写入不修改旧状态；
8. `core/service` 不依赖数据库、Transport、Controller 或业务模块；
9. 未提前实现模块 23-26 的数据库、字段策略、Dispatcher 或回收站能力。
