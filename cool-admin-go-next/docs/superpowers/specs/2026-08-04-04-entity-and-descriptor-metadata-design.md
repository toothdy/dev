# 模块 04：实体与 Descriptor 元数据设计

> 日期：2026-08-04  
> 状态：active  
> 模块：04 实体与 Descriptor 元数据  
> 对应拆分项：04.1-04.12  
> 前置模块：01 工程骨架与依赖边界、02 核心异常模型

## 1. 文档定位

本文冻结模块 04 的实体写法、标签解释、逻辑字段类型、索引声明、Descriptor 查询和集合冲突校验，供后续 DOValue、数据库方言、Schema、代码生成、CRUD 和 EPS 共同使用。

事实来源按以下顺序解释：

1. 本模块设计；
2. `2026-08-03-cool-admin-go-next-module-decomposition-design.md`；
3. `2026-07-31-cool-admin-go-next-architecture-design.md`；
4. 仓库 `README.md`。

## 2. 目标与非目标

模块 04 提供一套协议无关、数据库无关的只读实体元数据能力：

1. 业务实体继续以 Go struct 作为字段事实来源；
2. 显式编译调用读取已知实体类型，不扫描目录、不隐式注册；
3. 从 `g.Meta`、`Base`、Go 类型和 struct tag 构造不可变 Descriptor；
4. 字段可按 Go 名、JSON 名或数据库列名精确查询；
5. Schema 只补充复合普通索引和唯一索引；
6. 单实体和 Descriptor 集合中的名称冲突在进入后续阶段前失败；
7. 错误统一为模块 02 的 Core 异常并保留 Cause。

本模块不实现：

- `DOValue`、`SetColumn`、四态或 `NewDO()`，这些属于模块 05；
- 数据库类型翻译、标识符长度和方言能力，这些属于模块 06；
- Schema DDL、同步或实际数据库比较，这些属于模块 07；
- AST 包扫描、生成 Descriptor 实现或写入 `modules/modules_gen.go`，这些属于模块 13；
- 关系、外键、检查约束、表级选项、软删除字段或租户字段；
- 全局可变注册表和运行时目录扫描。

## 3. 对外接口

代码归属为 `cool-next/core/entity`。公开能力固定为：

```go
type Base struct {
   ID         uint64      `json:"id" orm:"id" description:"ID"`
   CreateTime *gtime.Time `json:"createTime" orm:"createTime" description:"创建时间"`
   UpdateTime *gtime.Time `json:"updateTime" orm:"updateTime" description:"更新时间"`
}

type LogicalType string

const (
   LogicalBool   LogicalType = "bool"
   LogicalInt    LogicalType = "int"
   LogicalUint   LogicalType = "uint"
   LogicalFloat  LogicalType = "float"
   LogicalString LogicalType = "string"
   LogicalBytes  LogicalType = "bytes"
   LogicalTime   LogicalType = "time"
)

type Constraints struct {
   Size          uint64
   HasSize       bool
   Default       string
   HasDefault    bool
   Precision     uint64
   HasPrecision  bool
   Scale         uint64
   HasScale      bool
}

type Field interface {
   Name() string
   JSONName() string
   Column() string
   Description() string
   LogicalType() LogicalType
   GoType() reflect.Type
   Nullable() bool
   Primary() bool
   AutoIncrement() bool
   SystemMaintained() bool
   Constraints() Constraints
}

type Index struct {
   Name   string
   Fields []string
   Unique bool
}

type Schema struct {
   Indexes []Index
}

type Metadata interface {
   Table() string
   Description() string
   Primary() Field
   Fields() []Field
   Field(name string) (Field, bool)
   JSON(name string) (Field, bool)
   Column(name string) (Field, bool)
   Indexes() []Index
}

type Descriptor[E any, ID comparable] interface {
   Metadata
   EntityType() reflect.Type
   IDType() reflect.Type
}

func Compile[E any, ID comparable](schema Schema) (Descriptor[E, ID], error)
func IndexOf(name string, fields ...string) Index
func UniqueIndexOf(name string, fields ...string) Index
func ValidateSet(descriptors ...Metadata) error
```

`Descriptor` 本阶段不包含 `NewDO()`。模块 05 定义 `DOValue` 后再扩展该能力，避免模块 04 提前实现字段四态。

## 4. 实体结构

`Compile` 只接受非指针 struct 类型，并要求：

- 直接匿名嵌入且只嵌入一个 `g.Meta`；
- 直接匿名嵌入且只嵌入一个 `coreentity.Base`；
- 不允许其他匿名字段；
- 不允许未导出业务字段；
- `ID` 必须与 `Base.ID` 的 `uint64` 类型完全一致；
- `g.Meta`、`Base` 和业务字段顺序只影响 `Fields()` 的稳定顺序，不影响查询结果。

`g.Meta` 的标签固定为：

```go
g.Meta `orm:"table:demo_goods" description:"商品信息"`
```

表名和表描述必填。表名使用可移植的小写数据库标识符，匹配 `[a-z][a-z0-9_]*`；表描述去除首尾空白后不得为空。`orm` 中只允许一个 `table:<name>` 指令，未知或重复指令直接失败。

## 5. 字段标签与名称

每个业务持久化字段必须同时显式提供：

- `json:"<name>"`：逻辑字段名和外部字段名；
- `orm:"<column>"`：数据库列名；
- `description:"<text>"`：字段描述。

固定规则：

- JSON 名和列名均匹配 lowerCamelCase：`[a-z][A-Za-z0-9]*`；
- 不从 Go 名、JSON 名或列名互相推导；
- `json:"-"`、空名称、JSON 选项、ORM 选项和空描述均拒绝；
- `Field.Name()` 固定返回显式 JSON 名，供 `IndexOf` 和后续 `SetColumn` 使用；
- 同一实体内逻辑/JSON 名和列名分别唯一，比较区分大小写；
- `g.Meta` 和 `Base` 是唯一豁免普通业务字段校验的嵌入字段。

错误消息包含实体、字段和标签名称，但不输出字段运行值。

## 6. 逻辑类型与 Nullable

普通字段支持以下 Go 类型：

| Go 类型 | LogicalType |
|---|---|
| `bool` 或同底层 named type | `bool` |
| `int`、`int8`、`int16`、`int32`、`int64` 或同底层 named type | `int` |
| `uint`、`uint8`、`uint16`、`uint32`、`uint64` 或同底层 named type | `uint` |
| `float32`、`float64` 或同底层 named type | `float` |
| `string` 或同底层 named type | `string` |
| `[]byte` | `bytes` |
| `time.Time`、`gtime.Time` | `time` |

一层指针表示 nullable，逻辑类型取指针元素类型。双重指针、其他 struct、array、非字节 slice、map、interface、func、chan、complex 和 unsafe pointer 均拒绝。

`Base` 是固定例外：

- `id` 为 `uint`、非空、主键、自增、非系统维护；
- `createTime` 和 `updateTime` 为 `time`、非空、非主键、非自增、系统维护；
- 虽然两个时间字段使用指针承接 GoFrame 时间值，其数据库 nullable 固定为 `false`；
- 业务代码不得重复声明主键、自增或系统维护字段。

## 7. cool 约束

`cool` 标签使用英文逗号分隔 `key=value`，首期只允许：

- `size`：正整数，只适用于 string 或 bytes；
- `default`：非空原始文本，保留给后续方言按逻辑类型解释；
- `precision`：正整数，只适用于 float；
- `scale`：非负整数，只适用于 float，必须同时存在 precision 且不大于 precision。

空项、重复键、未知键、缺少等号、非法整数和不适用目标类型均失败。`Constraints()` 返回纯值副本，调用方修改不影响 Descriptor。

## 8. 索引与合并

`IndexOf` 和 `UniqueIndexOf` 接收物理索引名和逻辑字段名：

- 构造时立即复制 `fields`；
- 物理索引名匹配 `[a-z][a-z0-9_]*`；
- 索引字段至少一个、无空值、无重复；
- 字段必须能通过 `Metadata.Field` 找到；
- 同一 Descriptor 内索引名唯一；
- 保留声明字段顺序，不排序、不去重。

`Base` 自动加入以下普通索引：

```text
idx_<table>_create_time (createTime)
idx_<table>_update_time (updateTime)
```

`Schema.Indexes` 在上述系统索引之后按调用方顺序追加。与系统索引同名的声明直接失败，不进行覆盖或合并。

## 9. 查询、顺序与不可变性

字段稳定顺序固定为 `id`、`createTime`、`updateTime`，随后是业务字段声明顺序。索引稳定顺序固定为两个系统索引，随后是 Schema 声明顺序。

Descriptor 在构造时建立三份只读索引：

- 逻辑字段名 -> Field；
- JSON 名 -> Field；
- 数据库列名 -> Field。

`Fields()` 返回新 slice；`Indexes()` 返回新 slice，并继续复制每个 `Index.Fields`。`IndexOf`、`UniqueIndexOf` 和 `Compile` 都不持有调用方传入 slice 的引用。`Field` 实现只有不可变值，`reflect.Type` 本身不可变。

## 10. Descriptor 集合校验

`ValidateSet` 不维护全局注册状态，只校验调用方明确提交的 Descriptor 集合：

- Descriptor 不得为 nil；
- 物理表名在集合中全局唯一；
- 物理索引名在集合中全局唯一；
- 每个 Descriptor 的字段名、JSON 名、列名和索引名必须保持内部唯一；
- 错误按输入顺序稳定返回首个冲突，并同时指出冲突双方的表名。

数据库标识符最大长度和不同方言的命名空间差异留给模块 06/13；模块 04 只执行已冻结的可移植语法和全局唯一规则。

## 11. 错误语义

实体类型、标签、逻辑类型、Schema、索引和集合冲突错误全部：

- 返回 `error`；
- 可通过 `errors.As` 识别为模块 02 `BaseException`；
- 分类为 `CoreFail`；
- 原始解析或校验 Cause 可通过错误链访问；
- 消息包含稳定的实体/字段/索引定位，不包含运行时数据值。

## 12. 测试与验收

单元测试至少覆盖：

- `Base` 的字段、标签和固定语义；
- `g.Meta` 表名、描述及非法表标签；
- 业务字段三个标签、lowerCamelCase 和名称重复；
- 全部逻辑类型、named type、一层 pointer 和非法类型；
- Base 主键、自增、系统维护、非空语义和系统索引；
- `size/default/precision/scale` 成功与失败；
- 普通索引、唯一索引、字段顺序和错误引用；
- 按 Go/JSON/列名查询；
- 输入和输出 slice 的防御性复制；
- 表名与物理索引名跨 Descriptor 冲突；
- 所有失败均为 Core 异常并保留 Cause。

门禁命令：

```bash
go test ./cool-next/core/entity -count=1
go test -race ./cool-next/core/entity -count=1
go vet ./...
make check
```

模块 04 是纯元数据能力，不需要数据库集成测试或 Fuzz。

## 13. 完成标准

1. 04.1-04.12 均有实现和测试证据；
2. 实体类型和标签是字段唯一事实来源；
3. Descriptor 查询确定且返回集合不可修改内部状态；
4. Base 主键、时间字段和系统索引语义固定；
5. Schema 只表达普通/唯一复合索引；
6. 单实体和集合冲突均在进入数据库或生成器前失败；
7. 未实现模块 05、06、07 或 13 的能力；
8. 单测、Race、Vet 和仓库快速门禁通过。

## 14. 上位设计覆盖表

| 拆分项 | 本文位置 |
|---|---|
| 04.1 | 第 3、6 节 |
| 04.2 | 第 4 节 |
| 04.3 | 第 5、7 节 |
| 04.4 | 第 5 节 |
| 04.5 | 第 6 节 |
| 04.6 | 第 3 节 |
| 04.7 | 第 9 节 |
| 04.8 | 第 6、8 节 |
| 04.9 | 第 8 节 |
| 04.10 | 第 8 节 |
| 04.11 | 第 9 节 |
| 04.12 | 第 10 节 |
