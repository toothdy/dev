# 模块 05：DOValue 与字段四态设计

> 日期：2026-08-04  
> 状态：active  
> 模块：05 DOValue 与字段四态  
> 对应拆分项：05.1-05.9  
> 前置模块：04 实体与 Descriptor 元数据

## 1. 文档定位

本文冻结模块 05 的字段提交状态、`DOValue` 能力、`SetColumn` 校验、GoFrame DO 结构和 `DBData` 转换，供代码生成、CRUD 和数据库写入共同使用。

事实来源按以下顺序解释：

1. 本模块设计；
2. `2026-08-03-cool-admin-go-next-module-decomposition-design.md`；
3. `2026-07-31-cool-admin-go-next-architecture-design.md`；
4. `2026-08-04-04-entity-and-descriptor-metadata-design.md`；
5. 仓库 `README.md`。

模块 05 不改变模块 04 已冻结的实体、字段、索引或冲突规则，只在其 Descriptor 上增加数据库写入值能力。

## 2. 目标与非目标

模块 05 提供一套由 Descriptor 驱动、与具体数据库方言无关的结构化写入值能力：

1. 每次 `Descriptor.NewDO()` 创建独立的可变 `DOValue`；
2. 每个字段明确区分未提交、提交零值、提交 `false` 和显式 `null`；
3. `SetColumn` 只接受逻辑字段名，并立即校验字段、类型和 nullable；
4. 显式 `null` 转换为 `gdb.Raw("NULL")`，未提交字段保持 `nil`；
5. `DBData()` 返回符合 GoFrame `do:true` 约定的具体 struct 值；
6. 数据库写入对象不使用 `g.Map`、`map[string]any` 或同类动态 map；
7. `uint64`、时间、named type、零值和空值在转换中不失真；
8. 所有校验错误统一进入模块 02 的 Core 异常模型。

本模块不实现：

- AST 扫描、字段常量或 `modules/modules_gen.go`，这些属于模块 13；
- 数据库类型翻译、DDL、Schema 同步或数据库连接，这些属于模块 06、07、09；
- Mutable、字段清理、只读/隐藏策略、Hook 或实际 DML，这些属于模块 22-25；
- 通用 SQL 表达式入口；除框架内部生成的 `gdb.Raw("NULL")` 外，调用方不能通过 `SetColumn` 注入 Raw SQL；
- 全局 DO 注册表、运行时目录扫描或跨 Descriptor 共享可变状态。

## 3. 对外接口

代码归属为 `cool-next/core/entity`。模块 04 的公开接口扩展为：

```go
type DOValue interface {
   Has(field string) bool
   IsNull(field string) bool
   SetColumn(field string, value any) error
   DBData() any
}

type Descriptor[E any, ID comparable] interface {
   Metadata
   EntityType() reflect.Type
   IDType() reflect.Type
   NewDO() DOValue
}
```

固定语义：

- `NewDO` 每次返回新实例；修改一个实例不影响 Descriptor 或其他实例；
- `Has` 和 `IsNull` 使用逻辑字段名；未知字段返回 `false`；
- `IsNull(field)` 只有在该字段已经显式提交 `null` 时返回 `true`；未提交、普通值和未知字段均返回 `false`；
- `SetColumn` 成功后覆盖该字段之前的状态和值；失败时该字段原状态和值保持不变；
- `DBData` 返回当前状态的 struct 值快照，不返回内部状态表或指针；
- `DOValue` 是单次写入构造器，不承诺并发安全，不得被多个 goroutine 同时修改。

## 4. 字段四态

每个 Descriptor 字段独立处于以下状态之一：

| 状态 | `Has` | `IsNull` | DO 字段值 | ORM 行为 |
|---|---:|---:|---|---|
| 未提交 | `false` | `false` | `nil` | 忽略该列 |
| 提交零值 | `true` | `false` | 对应类型零值 | 写入零值 |
| 提交 `false` | `true` | `false` | `false` | 写入 `false` |
| 显式 `null` | `true` | `true` | `gdb.Raw("NULL")` | 写入 SQL NULL |
| 普通值 | `true` | `false` | 原类型值 | 参数化写入 |

零值和 `false` 是普通的已提交值，不能通过空值过滤丢弃。未提交和显式 `null` 都不使用普通 Go 零值推断：前者由初始状态表示，后者只能由调用方传入 nil 明确触发。

`DBData` 中未提交字段为 `any(nil)`。DO struct 的 `g.Meta` 带 `orm:"table:<table>,do:true"`，GoFrame v2.10.2 会对 DO struct 自动应用 nil data 过滤，因此该字段不会参与 Insert/Update。显式空值使用非 nil 的 `gdb.Raw("NULL")`，不会被上述过滤移除。

## 5. SetColumn 名称、空值与类型规则

`SetColumn` 的执行顺序固定为：

1. 通过 `Metadata.Field(field)` 按逻辑字段名精确查找；
2. 判断传入值是否为 nil，包括 interface 中与字段声明形状匹配的 typed nil pointer/slice；
3. nil 值只在 `Field.Nullable()` 为 `true` 时成功，并写入显式 `null` 状态；不相关类型的 typed nil 仍按类型错误拒绝；
4. 非 nil 值按字段声明的实际值类型执行严格检查；
5. 全部校验通过后一次性覆盖字段状态和 DO 值。

实际值类型由 `Field.GoType()` 决定：普通字段使用声明类型；一层 pointer 字段使用其元素类型，pointer 只表达数据库 nullable，不要求调用方传入 pointer。Base 的 `createTime/updateTime` 虽然是固定的非空 pointer 形状，其实际值类型同样为 `gtime.Time`。named type 必须保持其声明类型，不能用同底层普通类型替代。

严格类型规则如下：

- 只接受 `reflect.Type` 完全一致的值，不执行数值、字符串、pointer 或 named type 自动转换；
- `uint64` 不经过 `float64`、`int64` 或字符串中间值；
- `time.Time` 与 `gtime.Time` 不互转；
- nullable `*string` 字段的非空值类型是 `string`，nil 表示 SQL NULL；
- typed nil pointer 只有与 `Field.GoType()` 完全一致时才表示 null；nullable `*string` 因此可接收 `(*string)(nil)`，但不能接收其他 typed nil；
- `[]byte` 的非空值必须是 `[]byte`；typed nil slice 只有与实际值类型完全一致时才按 nil 处理，因此非 nullable bytes 字段拒绝该值；
- 调用方直接传入 `gdb.Raw`、其他 SQL 表达式或错误类型均失败；
- 错误信息只包含实体、逻辑字段、期望类型和实际类型，不包含字段值。

字段是否主键、自增或系统维护不属于本模块的写权限策略。模块 05 只保证类型和空值安全；模块 24 在最终 DML 前清理客户端 ID 和只读字段，框架基础设施仍可按其职责设置这些字段。

## 6. GoFrame DO 结构

每个 Descriptor 对应一个具体 DO struct 形状：

```go
type goodsDO struct {
   g.Meta `orm:"table:demo_goods,do:true"`

   ID         any `orm:"id"`
   CreateTime any `orm:"createTime"`
   UpdateTime any `orm:"updateTime"`
   Title      any `orm:"title"`
   Remark     any `orm:"remark"`
   Status     any `orm:"status"`
}
```

约束如下：

- DO 类型不作为业务 API 导出；
- `g.Meta` 必须匿名嵌入，`orm` 同时包含准确表名和 `do:true`；
- 每个持久化字段在 DO 中恰好对应一个导出字段；
- DO 字段顺序与 `Metadata.Fields()` 一致；
- DO 字段类型统一为 `any`，`orm` Tag 使用准确物理列名；
- 不携带 `json`、`description`、`cool` 或 `omitempty` Tag；
- `DBData()` 返回 DO struct 值，不返回 map、slice、entity 或任意用户提供对象。

当前显式 `Compile[E, ID]` 已通过反射读取明确的实体类型。模块 05 为该路径按 Descriptor 构造等价的私有运行时 struct 类型，并在 Descriptor 内保存不可变的类型和字段位置映射；`NewDO` 只创建值，不重复编译类型。模块 13 生成命名的私有 DO struct、状态存储和直接赋值代码，替代生产生成路径中的反射构造，但必须实现本文完全相同的公开语义。

该运行时实现是完整可用的显式编译路径，不扫描目录、不生成文件、不注册全局状态，也不作为模块 13 可省略的理由。

## 7. 状态存储与 DBData 快照

私有 `DOValue` 同时保存：

- 所属 Descriptor 的只读引用；
- 独立的 DO struct 可设置值；
- 与字段稳定顺序对应的提交状态；
- 从逻辑字段名到 DO 字段位置的只读映射。

`SetColumn` 先完成全部校验，再写 DO 字段和状态。普通值直接存入对应 `any` 字段；显式 `null` 存入 `gdb.Raw("NULL")`。重新赋值允许以下转换：普通值到普通值、普通值到 null、null 到普通值；每次以最后一次成功调用为准。

`DBData()` 每次返回当前 DO struct 的值副本。之后继续调用 `SetColumn` 不改变此前取得的 struct 快照；调用方也无法通过返回值修改 DOValue 内部字段状态。字段中引用类型值的深拷贝不属于本模块职责，数据库调用方不得在提交期间并发修改其传入的 slice 或时间对象。

空 DOValue 的 `DBData()` 仍返回合法的全 nil DO struct；是否允许执行无字段 DML 由后续 CRUD/Service 层检查，模块 05 不改变 `DBData() any` 的既有接口为返回 error。

## 8. 错误语义

以下情况返回错误且不改变已有字段状态：

- 逻辑字段名不存在；
- 非 nullable 字段提交 nil 或 typed nil；
- 非 nil 值类型与字段实际值类型不一致；
- 调用方直接提交 SQL Raw 或其他不受支持值。

所有错误：

- 可通过 `errors.As` 识别为模块 02 `BaseException`；
- 分类为 `CoreFail`；
- 原始校验 Cause 可通过错误链访问；
- 消息包含稳定的实体、字段和类型定位；
- 不包含运行时字段值、Raw SQL 内容或完整 DBData。

`Has`、`IsNull` 和 `DBData` 不返回错误。未知字段查询统一返回 `false`，避免把只读状态探测变成异常控制流。

## 9. 文件职责

| 文件 | 职责 |
|---|---|
| `types.go` | `DOValue` 接口和 `Descriptor.NewDO()` 扩展 |
| `descriptor.go` | Descriptor 保存 DO 类型/字段位置并创建独立 DOValue |
| `do.go` | 私有四态、类型校验、SetColumn 和 DBData 实现 |
| `compile.go` | 从已校验实体/字段编译 GoFrame DO struct 形状 |
| `do_test.go` | 四态、类型、nullable、快照和错误语义测试 |
| `do_goframe_test.go` | GoFrame DO 元数据、字段形状和 nil/Raw 转换契约测试 |

模块 13 后续生成的类型只写入 `modules/modules_gen.go`，不得反向把生成文件或业务实体依赖引入 `cool-next/core/entity`。

## 10. 测试与验收

单元测试至少覆盖：

- `NewDO` 多次调用状态隔离；
- 未提交字段的 `Has=false`、`IsNull=false` 和 DO 字段 nil；
- `0`、空字符串、空 `[]byte`、`false` 均保持已提交且不失真；
- nullable 字段普通值与 nil 的不同状态；
- 非 nullable 字段拒绝 untyped nil、typed nil pointer 和 typed nil slice；
- unknown field、错误普通类型、named type 混用、整数宽度混用、`time.Time/gtime.Time` 混用和直接 Raw 均失败；
- `uint64` 最大值、全部有符号/无符号宽度、float32/64、named type、bytes 和两种时间类型原样保留；
- 同一字段重复赋值及普通值/null 双向覆盖；
- 失败赋值不改变此前成功状态；
- `DBData` 是具体 struct，包含准确 `g.Meta`、`do:true`、表名、字段顺序、`any` 类型和 `orm` Tag；
- `DBData` 不为 map，未提交字段为 nil，显式 null 恰为 `gdb.Raw("NULL")`；
- 先取得的 `DBData` 快照不受后续 `SetColumn` 影响；
- 所有失败均为 Core 异常且错误文本不包含字段值。

门禁命令：

```bash
go test ./cool-next/core/entity -count=1
go test -race ./cool-next/core/entity -count=1
go vet ./...
make check
```

模块 05 不建立数据库连接。GoFrame v2.10.2 的 `do:true` nil 过滤契约通过固定 struct 形状和依赖版本验证；三数据库实际写入验收属于第一阶段总验收门，并依赖模块 06-09。

## 11. 完成标准

1. 05.1-05.9 均有实现和测试证据；
2. Descriptor 可创建彼此隔离的 DOValue；
3. 未提交、零值、`false` 和显式 `null` 可稳定区分；
4. 字段查找、类型和 nullable 在状态修改前完成校验；
5. DBData 是符合 GoFrame `do:true` 约定的具体 struct；
6. 不使用 map 作为数据库写入对象；
7. uint64、时间、named type、零值和空值不失真；
8. 未实现模块 06、07、09、13 或 CRUD 能力；
9. 单测、Race、Vet 和仓库快速门禁通过。

## 12. 上位设计覆盖表

| 拆分项 | 本文位置 |
|---|---|
| 05.1 | 第 4、7 节 |
| 05.2 | 第 3 节 |
| 05.3 | 第 3、5 节 |
| 05.4 | 第 5、8 节 |
| 05.5 | 第 4、7 节 |
| 05.6 | 第 6 节 |
| 05.7 | 第 6、7 节 |
| 05.8 | 第 2、6 节 |
| 05.9 | 第 5、10 节 |
