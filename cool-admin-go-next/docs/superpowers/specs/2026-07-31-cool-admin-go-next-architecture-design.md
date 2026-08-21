# cool-admin-go-next 底层框架重构设计

> 日期：2026-07-31\
> 状态：已按确认项修订，等待用户复核\
> 范围：框架底层、模块开发契约、HTTP/gRPC 承载能力\
> 不包含：`base`、`dict`、`task`、`recycle`、`user` 等具体业务模块实现

## 1. 背景与目标

本次工作不是在 v1 上继续修补，而是重新实现 `cool-admin-go-next`。三个项目的角色必须明确：

| 来源                             | 作用            | 约束                                      |
| ------------------------------ | ------------- | --------------------------------------- |
| `cool-admin-midway`            | 已验证的业务行为      | HTTP 契约、CRUD 语义和鉴权行为的首要参考               |
| `cool-admin-midway-packages`   | Node 框架公共 API | Controller、Service、Exception 等开发体验的首要参考 |
| `cool-admin-go-next-v1`        | 能力清单和测试案例     | 只用于识别遗漏与缺陷，禁止复制实现                       |
| `cool-admin-go-next/README.md` | Go 工程目录约束     | `cool-next/` 目录及短包名保持不变                 |

目标如下：

1. 熟悉 Node 版 Cool Admin 的开发者能快速理解 Go 版实体、Service 和 Controller。
2. 在 GoFrame v2 上提供 Cool Admin 的框架层，而不是另造 ORM、Web Server 或 RPC 协议。
3. 通过编译期生成完成模块发现和装配，避免运行时扫描、字符串 DI 和反射分发。
4. 支持 MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+，并为 HTTP、gRPC 单独或同时运行保留一致的领域契约。
5. 默认路径安全、可观测、可测试；未知字段、越权字段和非法排序不得静默放行。
6. 通过 Transactional Outbox 保证业务事务与可靠消息同时提交，支持崩溃恢复、重试、死信和幂等消费。

非目标如下：

- 不保持 v1 内部 API 或实现兼容。
- 不复制 v1 的实体 DSL、自定义 SQL 编译器、运行时注册和分散生成文件。
- 不在本设计中决定具体业务模块的实体、接口和业务流程。
- 不提供框架级多租户能力，不保留自动租户字段、请求 Scope、查询过滤或绕过 API。
- 不追求逐字翻译 TypeScript 装饰器；对外概念对齐 Node，内部实现采用 Go 风格。

## 2. 总体原则

### 2.1 Node 行为优先，Go 实现优先

发生冲突时按以下顺序裁决：

1. 面向使用者的接口名称和行为优先对齐 Node。
2. Go 内部使用显式类型、构造函数、接口和 `context.Context`。
3. 数据库、HTTP、配置、校验、日志和 gRPC 优先使用 GoFrame v2 官方能力。
4. v1 只提供测试线索，不成为新架构的兼容边界。

### 2.2 编译期确定，运行时执行

模块、实体、构造函数、Controller、路由、中间件、生命周期、事件、任务、队列和 gRPC 注册器均由 `cool generate` 在编译期分析。运行时只消费静态生成的注册表。

以下禁令只针对 Cool 业务模块发现和装配：

- 目录运行时扫描；
- `init()` 隐式注册；
- 空白 import 拉起模块；
- 用字符串定位 Service；
- 用反射按方法名调用业务方法；
- 引入第三方 DI 容器。

GoFrame 数据库 Driver、标准 `database/sql` Driver 等基础设施允许按其官方机制使用包级注册和空白 import；业务模块不得借此隐式注册自身组件。

### 2.3 一份元数据，多处消费

业务实体是字段事实来源。编译器从 Go 类型、指针、`orm`、`json`、`description` 和 `cool` 标签构建不可变 Descriptor，供以下能力共同使用：

- GoFrame ORM 模型；
- CRUD 字段过滤和结构化写入；
- Schema 同步与校验；
- EPS 和 OpenAPI；
- HTTP/gRPC DTO 与 Schema 映射；
- `cool check` 静态检查。

不得为同一实体再维护一套手写字段定义。

## 3. 目录与依赖边界

目录以 `README.md` 为准，框架保持在 `cool-next/`：

```text
cool-admin-go-next/
├── main.go
├── cmd/
│   └── cool/
├── cool-next/
│   ├── core/
│   │   ├── controller/
│   │   ├── entity/
│   │   ├── service/
│   │   ├── exception/
│   │   ├── module/
│   │   ├── event/
│   │   ├── tag/
│   │   ├── configuration/
│   │   ├── app/
│   │   ├── hooks/
│   │   └── route/
│   ├── auth/
│   │   ├── bcrypt/
│   │   ├── jwt/
│   │   ├── session/
│   │   ├── context.go
│   │   └── middleware.go
│   ├── crud/
│   │   ├── types.go
│   │   ├── metadata.go
│   │   ├── selector.go
│   │   └── executor.go
│   ├── eps/
│   ├── task/
│   ├── outbox/
│   │   └── store/
│   ├── grpc/
│   ├── codegen/
│   └── db/
│       ├── tx/
│       ├── driver/
│       ├── recycle/
│       └── schema/
├── modules/
│   └── modules_gen.go
├── manifest/config/
├── resource/public/uploads/
└── docs/
```

依赖方向：

```text
main / cmd
    -> core/app
        -> generated module graph
            -> controller + service + crud + outbox + task
                -> core/entity + db
                    -> GoFrame v2
```

强约束：

- `cool-next/*` 不得 import `modules/*`。
- 框架的协议无关层不得 import `ghttp` 或 gRPC 生成的协议类型。
- `db/driver` 只处理 DDL 方言和能力差异，不接管 DML。
- `crud` 不依赖具体业务 Service。
- 模块之间不靠隐式扫描或全局变量通信；依赖由构造函数参数类型形成有向图。
- 未来拆成微服务时，领域 Service 不因传输协议变化而重写。

## 4. 实体与元数据

### 4.1 业务实体写法

实体直接使用 Go struct。表名和表描述写在 `g.Meta`，通用字段嵌入 `coreentity.Base`：

```go
package entity

import (
   "github.com/gogf/gf/v2/frame/g"
   coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
)

type Goods struct {
   g.Meta `orm:"table:demo_goods" description:"商品信息"`
   coreentity.Base

   Title  string  `json:"title" orm:"title" description:"标题" cool:"size=50"`
   Remark *string `json:"remark" orm:"remark" description:"备注"`
   Status int     `json:"status" orm:"status" description:"状态" cool:"default=1"`
}

func GoodsSchema() coreentity.Schema {
   return coreentity.Schema{
      Indexes: []coreentity.Index{
         coreentity.IndexOf("idx_goods_status", "status"),
      },
   }
}
```

字段推导规则：

| 信息     | 来源                       | 规则                         |
| ------ | ------------------------ | -------------------------- |
| 表名     | `g.Meta orm:"table:..."` | 必填且全局唯一                    |
| 表描述    | `g.Meta description`     | 用于表注释、EPS 和 OpenAPI        |
| JSON 名 | `json`                   | 外部 API 字段名                 |
| 列名     | `orm`                    | 每个持久化字段必须显式声明 lowerCamelCase 列名 |
| 逻辑类型   | Go 字段类型                  | 不在标签里重复声明数据库类型             |
| 可空     | 是否为指针                    | 指针可空，非指针不可空                |
| 字段描述   | `description`            | 同时进入列注释、EPS、OpenAPI 和生成元数据 |
| 补充属性   | `cool`                   | 只承载 Go 类型不能表达的可移植属性        |

数据库列名统一使用 lowerCamelCase，例如 `createTime`、`updateTime`、`messageId`；每个持久化业务字段都必须提供非空 `orm` 标签，生成器不从 Go/JSON 名推导数据库列名。缺少 `orm`、`orm` 列名为空，或列名包含 `update_time` 这类 snake_case 时，`cool generate` 必须失败；框架识别的 `g.Meta` 与嵌入式元数据字段不适用该字段标签规则。表名和索引名不受 lowerCamelCase 规则限制，可以继续使用 `demo_goods`、`cool_outbox` 和 `idx_goods_status` 这类 snake_case 名称。`db/driver` 和 `outbox/store` 必须按数据库方言引用列标识符，特别是 PostgreSQL 必须用双引号保留大小写，不能让 `messageId` 被折叠成 `messageid`；Schema 比较按列名精确匹配大小写。

`cool` 首期只支持确定且跨数据库可表达的属性，例如 `size`、`default`、`precision`、`scale`。未知键直接让 `cool generate` 失败，不能静默忽略。

### 4.2 基础字段

```go
type Base struct {
   ID         uint64      `json:"id" orm:"id" description:"ID"`
   CreateTime *gtime.Time `json:"createTime" orm:"createTime" description:"创建时间"`
   UpdateTime *gtime.Time `json:"updateTime" orm:"updateTime" description:"更新时间"`
}
```

业务自定义字段仍以指针表达 nullable。`Base` 是框架固定结构，只包含上述三个字段：它的 Descriptor 把 `id` 标记为主键和自增，把 `createTime/updateTime` 标记为系统维护的非空字段，并为 `createTime/updateTime` 提供 Node 基础实体已有的普通索引。业务模块自行声明的其他字段均按普通业务字段处理。

GoFrame 负责 `createTime`、`updateTime` 等时间字段维护，数据库连接配置统一指定字段名：

```yaml
database:
   default:
      createdAt: createTime
      updatedAt: updateTime
```

框架和业务代码不得重复手写维护时间。Node 的 `cool.crud.softDelete` 实际语义是“删除前归档到回收站，再从业务表物理删除”，不是在业务表维护删除时间。Go 版保持该对外语义，不在 `Base` 或业务实体中增加 `deletedTime/deleteTime`，也不为 `Info/List/Page` 注入逻辑删除过滤条件；完整事务契约见 4.7。

### 4.3 Schema 补充声明

Go 类型和标签负责单字段事实。首期 `XxxSchema()` 只表达无法放在单字段上的复合索引和唯一索引，完整契约固定为：

```go
type Schema struct {
   Indexes []Index
}

type Index struct {
   Name   string
   Fields []string
   Unique bool
}

func IndexOf(name string, fields ...string) Index
func UniqueIndexOf(name string, fields ...string) Index
```

两个构造器都返回防御性复制后的不可变声明。索引名必须非空并在实体内唯一；`fields` 必须非空、无重复，且每一项都是当前 Descriptor 中存在的逻辑字段名。`IndexOf` 生成普通索引，`UniqueIndexOf` 生成唯一索引；生成器继续校验跨实体的物理索引名冲突、数据库标识符长度和方言能力。

禁止在 `Schema()` 重复定义字段类型、可空性、描述或默认值。重复或冲突在生成阶段报错。外键、关系、数据库无关的检查约束和表级选项属于后续 Schema 专项设计；当前总架构不为尚未冻结的能力预留 `any`、map 或不完整字段，也不承诺生成器已经支持它们。

### 4.4 Descriptor 与唯一生成文件

生成器将实体编译为只读 Descriptor。Descriptor 至少包含：

```go
type Metadata interface {
   Table() string
   Description() string
   Primary() Field
   Fields() []Field
   Field(name string) (Field, bool)
   Column(name string) (Field, bool)
   Indexes() []Index
}

type Descriptor[E any, ID comparable] interface {
   Metadata
   NewDO() DOValue
}

type Field interface {
   Name() string
   JSONName() string
   Column() string
   Description() string
   Nullable() bool
}

type DOValue interface {
   Has(field string) bool
   IsNull(field string) bool
   SetColumn(field string, value any) error
   DBData() any
}
```

接口表示能力边界，不要求业务手写实现。`Field/Fields/Indexes` 返回不可变值或防御性副本。`DOValue.SetColumn` 使用 Descriptor 中的逻辑字段名并立即校验；`DBData()` 返回生成的具体 DO struct，供 GoFrame ORM 反射读取，不能返回 map。Descriptor 实现、字段常量、结构化 DOValue/DO 适配器均写入唯一文件：

```text
modules/modules_gen.go
```

业务模块目录中不得出现任何按模块或实体拆分的生成文件。业务 Service 通过 `Base.Model()` 与 `Base.Descriptor()` 访问 ORM 和元数据，不对外生成每实体 DAO/DO/Columns 文件。

私有 DO 必须使用 GoFrame 标准形状，类型可以不导出，但供 ORM 反射的字段必须导出且使用 `any`：

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

四态转换固定为：

| DOValue 状态 | DO 字段值            | ORM 行为      |
| ----------- | ----------------- | ----------- |
| 未提交         | `nil`             | 忽略该列        |
| 提交零值        | 对应类型零值            | 写入零值        |
| 提交 `false`  | `false`           | 写入 `false`  |
| 显式 `null`   | `gdb.Raw("NULL")` | 写入 SQL NULL |
| 普通值         | 参数值               | 参数化写入       |

### 4.5 DML 与多数据库

DML 统一使用 GoFrame `gdb.Model`、事务和结构化对象：

```go
func (s *GoodsService) Disable(ctx context.Context, id uint64) error {
   status, ok := s.Descriptor().Column("status")
   if !ok {
      return gerror.New("status column not found")
   }
   primary := s.Descriptor().Primary()

   data := s.Descriptor().NewDO()
   if err := data.SetColumn(status.Name(), 0); err != nil {
      return err
   }

   model, err := s.Model(ctx)
   if err != nil {
      return err
   }

   _, err = model.
      Where(primary.Column(), id).
      Data(data.DBData()).
      Update()
   return gerror.Wrap(err, "disable goods")
}
```

约束：

- DML 不经过自定义 SQL 编译器。
- DML 不使用 `g.Map`、`map[string]any` 等数据库写入对象。
- DOValue 必须能区分未提供、零值、`false` 和显式 `null`。
- 条件和值必须参数化，业务输入不能拼接进 SQL。
- `Base.Tx(ctx)` 只取得当前框架事务 Scope 的 `gdb.TX`；`Base.Model(ctx)` 校验并返回绑定该事务的 Model，事务内不得退回全局 DB。

CRUD、可靠 Consumer、自定义 Route 和任务共用 `cool-next/db/tx` 的事务边界：

```go
package dbtx

type Runner interface {
   Within(
      context.Context,
      string,
      func(context.Context) error,
   ) error
}

func Current(context.Context) (gdb.TX, string, bool)
```

`Within` 的第二个参数是数据库连接组。Context 中没有事务时，它在指定组开启事务，把 TX 和组名写入派生 Context，回调成功后提交、返回错误时回滚、Panic 时回滚后继续抛出 Panic；已有同组事务时直接复用且不取得提交权，已有不同组事务时返回 Core 配置错误。同组内层回调返回错误或 Panic 时必须把 Scope 标记为 rollback-only，并保存第一个失败；即使外层吞掉错误，最外层 Runner 也不尝试提交，而是回滚并返回该失败。首期不提供隐式嵌套事务或 Savepoint。回调及其派生 Context 不得在 `Within` 返回后继续使用。

CRUD Dispatcher、Inbox Adapter 和声明事务的自定义 Route 都通过 Runner 进入同一个 Scope；任务或事件 Handler 需要“业务 DML + Outbox”原子提交时也必须显式调用 Runner。它不是 `AfterCommit`：框架不保存回调，提交后的可靠工作仍只能写成 Outbox 消息。

Framework Database Group 固定取 `cool.outbox.databaseGroup`。生成器构造的所有 `Base`、CRUD Dispatcher、默认事务 Route、Outbox 和 Inbox 都保存并使用该同一组，不提供 Entity 级或 Route 级换组 API。其他 GoFrame 连接组只能由业务在框架事务边界之外显式使用，不能与本 Outbox 原子提交。

GoFrame ORM 支持多种数据库；本框架首期保证 MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+ 的一致行为。MySQL 只支持 8.x，不为 5.7 提供降级领取算法；PostgreSQL 基线必须支持 `FOR UPDATE SKIP LOCKED`，SQLite 基线必须支持 `ON CONFLICT DO NOTHING`。MySQL 的默认存储引擎以及所有由生成 Descriptor 管理、参与框架事务的业务表和 `cool_recycle/cool_outbox/cool_inbox` 内部表必须是 InnoDB；MyISAM 等非事务引擎不能进入 Framework Database Group。应用启动时校验数据库类型、版本和所需能力；MySQL 还要从数据库元数据验证实际表引擎，`schema.mode=off` 也不能跳过该事务能力探测，任一表不是 InnoDB 就直接失败。`db/driver` 仅把逻辑 Schema 翻译为各数据库 DDL，并维护能力矩阵，例如自增、字段类型、注释和索引差异。SQLite 等不支持原生表/列注释的数据库仍在 Descriptor、EPS 和 OpenAPI 中保留描述。CRUD DML 始终走同一套 `gdb.Model`，不得按数据库复制三份 Service。

### 4.6 Schema 模式

```yaml
cool:
   schema:
      mode: validate
```

| 模式         | 用途            | 行为                                           |
| ---------- | ------------- | -------------------------------------------- |
| `sync`     | 本地开发          | 只执行安全的增量变更，如建表、增加可空/有默认值的列、加索引；禁止删列、缩窄和破坏性变更 |
| `validate` | 测试和生产         | 只比较 Descriptor 与数据库，发现不一致则启动失败               |
| `off`      | 外部完全管理 Schema | 不同步，也不对业务 Entity 做 Descriptor 全量比较；已启用的框架基础设施仍执行自身运行必需的结构与能力探测 |

生产结构变更必须使用显式 migration。`off` 不是“假定数据库一定正确”：例如启用回收站或 Outbox/Inbox 时，启动探测仍必须验证其运行依赖的表、列、主键和固定索引；Outbox/Inbox 还验证精确列序，具体契约见 11.3。`cool generate` 只分析 Go 源码，不连接数据库；数据库不可用不得阻止生成。

### 4.7 删除归档与恢复

配置名称和默认值对齐 Node：

```yaml
cool:
   crud:
      softDelete: true
```

`softDelete` 解析后必须是布尔值；配置省略时取默认值 `true`。它作用于默认 Base 和显式 Base 委托的 `Delete`，属于应用级 CRUD 配置，不是 Entity Descriptor 上可各自切换的策略：

| 配置 | `Delete` 行为 |
| --- | --- |
| `true` | 在当前 Framework Database Group 的同一事务中，先把原记录归档到 `cool_recycle`，再从业务表物理删除 |
| `false` | 在当前事务中直接物理删除，不写 `cool_recycle` |

这里的“软删除”只沿用 Node 配置名。业务表不存在删除标记或删除时间，正常查询不需要默认过滤，框架也不启用 GoFrame/ORM 的另一套逻辑删除机制。

开启时，默认 `Delete` 的固定顺序是：

```text
规范化并去重 ID
-> 开启或复用 Dispatcher 的 dbtx Scope
-> 整批执行一次 ModifyBefore(delete)
-> 按主键锁定并读取当前仍存在的目标记录
-> 把本次读取到的记录数组写入一条 cool_recycle 记录
-> 按同一主键集合物理删除，并核对删除行数等于归档数量
-> 整批执行一次 ModifyAfter(delete)
-> 提交事务
```

空 ID 在进入事务前返回校验错误；重复 ID 先去重。不存在的 ID 不进入快照，也不计入删除行数；目标全部不存在时保持 Node 的幂等删除语义并成功返回，但不产生空回收记录。锁定、读取、归档、删除、Hook 或提交任一步失败都回滚，禁止出现“业务数据已删除但回收记录尚未写入”的窗口。归档不能通过进程内 Event、goroutine 或 Outbox 异步补写；Outbox 只用于事务提交后必须可靠执行的外部副作用。

并发一致性是 Store 契约而不是业务 Service 的方言分支：MySQL 8.x 和 PostgreSQL 对目标行使用事务内 `SELECT ... FOR UPDATE`；SQLite 依靠同一写事务的串行化，快照过期或锁竞争必须让整次删除回滚并按受限策略重试，不能拿旧快照继续删除。三种数据库最终都必须满足“归档快照就是本次实际删除的行”，删除影响行数不匹配时返回冲突并回滚。

一次批量删除对应一条回收记录，`data` 保存按确定主键顺序编码的原始记录数组。`cool_recycle` 至少保存 `id/createTime/databaseGroup/tableName/data/count/source/params/operatorType/operatorId`；列名遵守 lowerCamelCase。`tableName` 使用 Descriptor 中全局唯一的物理表名定位恢复目标，`databaseGroup` 必须等于 Framework Database Group。`source/params/operatorType/operatorId` 可空：HTTP Adapter 可以写入经过脱敏的 URL、请求摘要和 `auth.Admin(ctx)` / `auth.App(ctx)` 操作者，任务、事件或内部 Service 删除不得因为没有 HTTP Request 而跳过归档。禁止保存 Authorization、Cookie、密码、Token 或未脱敏的完整请求体。

`data` 必须从已读取的强类型实体确定性编码，恢复时通过目标 Descriptor 解码为强类型 DO，不能先解成 `map[string]any` 或 `float64`，避免 uint64 ID 和时间值失真。回收记录保存业务表删除前的完整持久化字段；展示回收站时再按权限脱敏，不修改可恢复快照。

恢复同样使用 Framework Database Group 的单个事务：锁定 `cool_recycle` 记录，按 `tableName` 解析当前生成图中的唯一 Descriptor，校验数据库组和快照形状，插入全部原记录，最后删除该回收记录。恢复使用普通 `INSERT` 语义，不使用 upsert，不覆盖现存数据；主键、唯一键、外键、字段不兼容或任一行写入失败时整体回滚并保留回收记录，禁止部分恢复。并发恢复只有取得锁的一方可以成功。

`cool-next/db/recycle` 负责上述 Store、事务和内部表契约；具体 `recycle` 业务模块只负责列表、详情、恢复权限和管理界面，不重新实现归档事务。`softDelete=true` 时，Application Host 在 Ready 前验证 `cool_recycle` 的表、列、主键、索引、数据库组和事务能力；`schema.mode=off` 也不能跳过。验证失败必须阻止启动，不能降级成硬删除。`softDelete=false` 时不要求该表存在。

## 5. 模块声明与静态装配

### 5.1 模块目录协议

沿用 README 约定：

```text
modules/<module>/
├── config.go
├── contract/**
├── entity/**
├── service/**
├── controller/**
├── middleware/global/**
├── middleware/**
├── event/**
├── schedule/**
├── queue/**
├── consumer/**
├── dto/**
├── db.json
└── menu.json
```

目录可以任意深度嵌套。测试文件、`testdata`、隐藏目录和生成文件不参与发现。

跨模块依赖只能面向目标模块的 `contract/**` 稳定接口，不能直接 import 其他模块的 `service`、`entity` 或 `controller`。模块不手写 `Dependencies`；生成器仍从构造器参数的接口类型和唯一 Provider 推导跨模块依赖边。

### 5.2 模块配置

```go
package demo

import module "github.com/toothdy/cool-admin-go-next/cool-next/core/module"

type Config struct {
   Enabled bool `json:"enabled"`
   Limit   int  `json:"limit" v:"min:1#处理数量必须大于 0"`
}

func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{
      Name:        "示例模块",
      Description: "演示模块开发方式",
      Order:       0,
      Middlewares: []module.ComponentRef{
         module.Ref("middleware.New"),
      },
      GlobalMiddlewares: []module.ComponentRef{
         module.Ref("middleware/global.NewTrace"),
      },
      Defaults: Config{
         Enabled: true,
         Limit:   100,
      },
   }
}
```

框架公开类型固定为：

```go
type ComponentRef string

func Ref(symbol string) ComponentRef

type Declaration[T any] struct {
   Name              string
   Description       string
   Order             int
   Middlewares       []ComponentRef
   GlobalMiddlewares []ComponentRef
   Defaults          T
}
```

`Declaration` 的完整字段是 `Name`、`Description`、`Order`、`Middlewares`、`GlobalMiddlewares` 和 `Defaults`。框架按“默认值 -> 主配置 -> 环境覆盖”合并配置，然后统一调用 GoFrame `gvalid` 校验 `v` 标签。

合并必须先在保留字段 presence 的结构化配置树上完成，最后一次性解码为 `T`，不能先解码成 Go 零值再猜测字段是否出现。对象/struct 按字段递归合并，map 按 key 递归合并；标量由后一级显式值整体替换；slice/array 整体替换而不追加；显式 `null` 只允许清空可空指针、map 或 slice，作用于非可空字段时校验失败。环境变量只覆盖其明确定位的叶子路径。未知字段、类型不匹配和合并后的 `gvalid` 失败都必须阻止启动；相同三层输入必须产生字节等价的规范化配置结果。

`Middlewares` 只注册到本模块路由；`GlobalMiddlewares` 注册到所有 HTTP 路由，并按依赖拓扑优先、同层 `Order` 从大到小排序。`middleware/global/**` 目录本身不构成隐式注册，仍由 `GlobalMiddlewares` 明确引用构造器。

`module.Ref("middleware.New")` 只是一条供 AST 生成器解析和类型检查的编译期符号引用，运行时不会按字符串查找组件。配置声明不放校验开关。模块身份由目录和生成器确定，不要求业务重复声明 key。模块也不手写依赖列表。

### 5.3 构造函数与依赖图

生成器识别两种 Go 风格构造函数：

```go
func NewGoodsService(
   base *coreservice.Base[entity.Goods, uint64],
   config Config,
   audit AuditWriter,
) *GoodsService

func NewGoodsService(
   base *coreservice.Base[entity.Goods, uint64],
   config Config,
   audit AuditWriter,
) (*GoodsService, error)
```

规则：

- 构造器可以声明任意数量、任意顺序且可被静态注入的依赖，包括生成的实体 `Base`、本模块 `Config`、其他具体组件或业务窄接口；依赖完全由参数的静态类型推导，不限定为示例中的固定参数集合。
- 构造器只允许返回 `*T` 或 `(*T, error)`；不允许可变参数、多个业务返回值或运行时 Service Locator。
- 接口依赖必须只有一个可赋值 Provider；零个或多个都生成失败，不提供另一套限定符 DSL。
- 缺失依赖、重复提供、循环依赖和非法构造函数均在 `cool generate` 阶段失败。
- 构造顺序首先由依赖拓扑决定；无依赖关系的同层模块按 `Order` 从大到小，再按模块路径和构造器符号名稳定排序。`Order` 不能覆盖真实依赖边。
- 构造函数返回的错误保留堆栈并终止启动。

### 5.4 生成内容

`modules/modules_gen.go` 是 `cool generate` 的唯一输出，包含：

- 实体 Descriptor 和私有 DOValue 适配器；
- 模块静态描述；
- 构造函数调用和依赖拓扑；
- HTTP Controller、路由和中间件注册；
- 生命周期组件；
- 事件、定时任务和队列注册；
- Outbox Producer 依赖和可靠 Consumer Definition；
- 可选 gRPC Service 注册器。

事件、定时任务和队列是生成图必须预留的框架组件类别，不表示当前总架构已经冻结其业务 Handler 签名。对应专项设计批准后，`cool generate` 才按专项契约实现发现、类型检查和注册；在此之前不得根据本列表自行猜测 API。

“唯一生成文件”只约束 Cool 代码生成器，防止 `*_cool_gen.go` 散落到业务模块。标准 protobuf 工具生成的 `*.pb.go`、`*_grpc.pb.go` 或第三方依赖中的生成代码不属于该限制；它们不得承担 Cool 模块发现或装配职责。

生成要求：

- 先在内存中完成分析、格式化和类型检查，再原子替换目标文件。
- 任一错误不得留下半生成文件。
- 输出稳定，同一输入重复生成字节一致。
- 文件包含标准 `Code generated ... DO NOT EDIT.` 标记。

命令统一为：

```text
cool generate
cool check
cool build
cool run
cool outbox list
cool outbox show
cool outbox replay
```

前三个 `outbox` 运维子命令只在实现 Outbox Store 后可用，使用与应用相同的配置加载、Database Group 和脱敏规则；它们的具体安全契约见 11.6。

`cool check` 在 CI 中统一执行以下检查：生成文件最新且相同输入字节稳定；构造器依赖图、生命周期图及 Producer/Consumer 图合法；路由、别名、权限、Provider 和 Consumer Name 无冲突；实体列名、字段策略、索引、配置声明和消息版本元数据合法；业务代码未绕过 Descriptor、`dbtx` Scope、受控查询、Outbox 或基础设施依赖边界。它可以调用共享分析包，但不替代 `gofmt`、`go vet`、`go test` 或 `go test -race`；CI 必须把这些标准命令作为独立步骤执行，避免 `cool check` 成为不透明的万能入口。

## 6. Service 基类

### 6.1 业务写法

```go
package service

import (
   coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
   "github.com/toothdy/cool-admin-go-next/modules/demo/entity"
)

type GoodsService struct {
   *coreservice.Base[entity.Goods, uint64]
}

func NewGoodsService(
   base *coreservice.Base[entity.Goods, uint64],
) *GoodsService {
   return &GoodsService{
      Base: base,
   }
}
```

私有 Descriptor 和 DO 位于 `modules/modules_gen.go`，生成器据此构造对应泛型 Base，再通过普通构造器参数注入业务 Service。业务包不反向 import `modules` 包。

嵌入后业务 Service 获得：

```text
Add Delete Update Info List Page
Model Tx Descriptor NativeQuery
```

不提供基于方法名的动态接口映射。自定义 Service 方法必须由业务 Controller 显式注册 Route。

四个底层辅助方法的签名固定为：

```go
type NativeStatement struct{ /* private validated SELECT */ }

func NativeSQL(query string, args ...any) (NativeStatement, error)
func (b *Base[E, ID]) Model(ctx context.Context) (*gdb.Model, error)
func (b *Base[E, ID]) Tx(ctx context.Context) (gdb.TX, error)
func (b *Base[E, ID]) Descriptor() coreentity.Descriptor[E, ID]
func (b *Base[E, ID]) NativeQuery(
   context.Context,
   NativeStatement,
   destination any,
) error
```

Base 保存生成时确定的 Framework Database Group。`Model(ctx)` 在没有 Scope 时返回该组的普通 Model；存在同组 Scope 时返回绑定 TX 的 Model；存在不同组 Scope 时返回 Core 配置错误，绝不返回未绑定或错绑事务的 Model。`Tx(ctx)` 只在存在同组 Scope 时返回 TX，没有 Scope 或组不一致时返回错误，不负责新建另一层事务；`Descriptor()` 返回不可变元数据，并提供 `Column(name)` 等受校验字段引用。请求级 Model、TX 和 Context 不得缓存到 Service 字段。

`NativeQuery` 是已确认的复杂查询逃生口，只允许单条只读 SELECT/CTE。`NativeSQL` 的 SQL 文本必须是生成期可见的常量，所有业务值通过参数绑定。执行时存在同组 `dbtx` Scope 就复用当前 TX，没有 Scope 时使用 Base 保存的 Framework Database Group；存在不同组 Scope 时返回与 `Model/Tx` 相同的配置错误，不能改用其他连接。多语句、DML 和动态拼接标识符直接拒绝。

裸 `*gdb.Model` 和 `gdb.TX` 是 GoFrame 互操作边界，不是另一条原生 SQL API。`cool check` 禁止业务模块调用 `Model.Raw/DB/TX/Unscoped`，也禁止通过 `gdb.TX.Exec/Query` 绕开 `NativeQuery`；结构化 DML 继续使用 Descriptor DO + Model。受控 `RawWhere` 只允许常量表达式和绑定值，不能用于改变表或事务。

### 6.2 基础接口完整契约

Base 的公开方法签名：

```go
func (b *Base[E, ID]) Add(context.Context, AddInput[E]) (AddResult[ID], error)
func (b *Base[E, ID]) Delete(context.Context, DeleteInput[ID]) error
func (b *Base[E, ID]) Update(context.Context, UpdateInput[E, ID]) error
func (b *Base[E, ID]) Info(context.Context, ID) (Record, error)
func (b *Base[E, ID]) List(context.Context, Query) ([]Record, error)
func (b *Base[E, ID]) Page(context.Context, Query) (PageResult, error)
```

Node `BaseService` 中其余公共/半内部辅助方法不原样复制，完整去向如下：

| Node 方法                                       | Go 版去向                                                           |
| --------------------------------------------- | ---------------------------------------------------------------- |
| `softDelete`                                  | 不新增同名动作；由 `Delete` 按全局 `cool.crud.softDelete` 执行 4.7 的事务归档或硬删除契约 |
| `addOrUpdate`                                 | 不公开；由 Dispatcher 分别协调 `Add`、`Update`                             |
| `nativeQuery`                                 | 保留为受控 `NativeQuery`；只读、参数化并复用当前事务                              |
| `getOrmManager`                               | 当前事务使用 `Tx(ctx)`；常规 DML 使用已绑定事务的 `Model(ctx)`                       |
| `entityRenderPage`                            | 默认分页使用 `Page`；自定义 DTO 分页由 `crud` 的协议无关分页执行器承载，不挂到 Base 动态方法上     |
| `sqlRenderPage`                               | 不提供 MySQL SQL 字符串分页器；复杂只读分页通过 `NativeQuery`，业务自行保证方言兼容              |
| `getOptionFind`                               | 由 `QueryOp -> ActionPlan` 编译和 Descriptor 白名单替代                   |
| `setSql` / `getCountSql` / `paramSafetyCheck` | 属于 Node SQL 字符串实现细节；由结构化查询节点、参数绑定和计划校验替代，不暴露同名 API               |
| `getUserId`                                   | 由协议无关 `auth.Admin(ctx)` / `auth.App(ctx)` 返回已验证 Identity    |

因此 Base 对业务公开的方法就是本节列出的六个 CRUD 与 `Model/Tx/Descriptor/NativeQuery`；上表是明确替代或排除清单，不存在未说明的兼容方法。

关键输入容器提供以下只读形状 API，非法状态不能由业务代码自行构造：

```go
type Mutable[E any] struct{ /* private fields */ }
type AddInput[E any] struct{ /* private fields */ }
type DeleteInput[ID comparable] struct{ /* private fields */ }
type UpdateInput[E any, ID comparable] struct{ /* private fields */ }
type UpdateItem[E any, ID comparable] struct{ /* private fields */ }
type AddResult[ID comparable] struct{ /* private fields */ }
type QueryRequest = crud.QueryRequest
type Query struct{ /* private fields */ }
type Record struct{ /* private fields */ }
type FieldValue struct{ /* private presence-aware field */ }

func (value *Mutable[E]) Has(field string) bool
func (value *Mutable[E]) IsNull(field string) bool
func (value *Mutable[E]) Get(field string) (any, bool)
func (value *Mutable[E]) Set(field string, data any) error
func (value *Mutable[E]) SetNull(field string) error

func (in AddInput[E]) IsMany() bool
func (in AddInput[E]) One() *Mutable[E]
func (in AddInput[E]) Many() []*Mutable[E]

func (in DeleteInput[ID]) IDs() []ID

func (in UpdateInput[E, ID]) IsMany() bool
func (in UpdateInput[E, ID]) One() UpdateItem[E, ID]
func (in UpdateInput[E, ID]) Many() []UpdateItem[E, ID]
func (item UpdateItem[E, ID]) ID() ID
func (item UpdateItem[E, ID]) Mutable() *Mutable[E]

func (result AddResult[ID]) IsMany() bool
func (result AddResult[ID]) One() ID
func (result AddResult[ID]) Many() []ID
func (result AddResult[ID]) MarshalJSON() ([]byte, error)

func (query Query) Request() *QueryRequest
func (query Query) PageNumber() int
func (query Query) PageSize() int

func (request *QueryRequest) Has(name string) bool
func (request *QueryRequest) Value(name string) (any, bool)
func (request *QueryRequest) String(name string) (string, bool)
func (request *QueryRequest) Bool(name string) (bool, bool)
func (request *QueryRequest) Strings(name string) ([]string, bool)

func Value(field string, data any) FieldValue
func Null(field string) FieldValue

func NewMutable[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []FieldValue,
) (*Mutable[E], error)

func NewAddObject[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   *Mutable[E],
) (AddInput[E], error)

func NewAddArray[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []*Mutable[E],
) (AddInput[E], error)

func NewDeleteInput[E any, ID comparable](
   coreentity.Descriptor[E, ID],
   []ID,
) (DeleteInput[ID], error)

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

func NewQuery(
   *crud.QueryRequest,
   page int,
   size int,
) (Query, error)

func (record Record) Get(field string) (any, bool)
func (record Record) Scan(pointer any) error
func (record Record) MarshalJSON() ([]byte, error)

type Pagination struct {
   Page  int `json:"page"`
   Size  int `json:"size"`
   Total int64 `json:"total"`
}

type PageResult struct {
   List       []Record   `json:"list"`
   Pagination Pagination `json:"pagination"`
}
```

`AddInput/DeleteInput/UpdateInput/Query/Record` 的字段均不导出，只能通过上述 smart constructor 构造；HTTP Binder、gRPC Adapter、任务、事件和业务 Service 调用同一套入口。构造器执行 Descriptor 字段/ID 类型、空批次、重复字段和形状校验，但 Controller 的隐藏/只读策略仍在 ActionPlan 阶段应用。对象与顶层数组必须分别调用 `NewAddObject/NewAddArray` 或 `NewUpdateObject/NewUpdateArray`，不能靠 slice 长度猜测原始输入形状；这些只是同一个 `Add/Update` 的输入工厂，不增加 CRUD 动作。

`QueryRequest` 的实际类型和方法归 `cool-next/crud` 所有，`core/service` 只使用类型别名；其安全构造器在 7.3 列出。`Mutable[E]` 由 Descriptor 驱动，不是数据库 map。每个字段必须区分：

- 字段未出现；
- 字段出现且为零值；
- 字段出现且为 `false`；
- 字段出现且显式为 `null`。

显式 `null` 转换为私有 DO 时使用 GoFrame 能明确写 NULL 的值，例如 `gdb.Raw("NULL")`；DO 字段为 `nil` 只表示未提交，不能同时表示 NULL。`AddResult` 按输入形状编码：单对象的 `data.id` 是标量，顶层数组的 `data.id` 是有序数组。

`AddResult.MarshalJSON` 固定输出 `{"id": scalar}` 或 `{"id": [...]}`；`Record.MarshalJSON` 输出过滤后的字段集合，因此 `PageResult.List` 不会编码成空对象。EPS/OpenAPI 从 Descriptor、Select 别名和 Add 的单条/数组 `oneOf` 生成同一响应 Schema，不通过运行一次 Marshaler 猜测结构。

默认查询的 `Record/PageResult` 是 Descriptor 驱动的只读结果，可以包含 Join/Select 别名。Info/List/Page 重写和自定义 Route 允许使用自己的请求 DTO 和返回 DTO，不强迫业务返回框架 Record。

### 6.3 Add 行为

HTTP 入口保持 Node 行为：

```json
{"title":"A"}
```

返回：

```json
{"code":1000,"message":"success","data":{"id":1}}
```

顶层数组：

```json
[
   {"title":"A"},
   {"title":"B"}
]
```

返回：

```json
{"code":1000,"message":"success","data":{"id":[1,2]}}
```

数组新增的固定时序：

```text
解析顶层对象或数组
-> 生成器确定当前动作走默认 Base、纯业务重写还是显式 Base 委托重写
-> 默认 Base 或 Base 委托路径执行 Before、默认 DTO 绑定并记录字段来源
-> 默认 Base 或 Base 委托路径对每一项执行 InsertParam
-> 开启一个事务并把 TX 写入派生 Context
-> 整批执行一次 ModifyBefore(add)
-> 默认 Base 或 Base 委托路径逐项移除客户端来源的 ID 和只读字段
-> 调用业务重写或 Base 默认写入
-> 按输入顺序收集 ID
-> 整批执行一次 ModifyAfter(add)
-> 提交事务
-> 返回标量 ID 或有序 ID 数组
```

默认 Base 和 Base 委托 Add 会静默移除客户端来源的 ID 和只读字段，保持最后确认的 Node 风格行为；隐藏字段仍直接返回校验异常。字段来源必须保留到清理阶段，因此 `InsertParam` 写入的服务端值不会被误删。默认 Base 和 Base 委托 Update 提交只读或隐藏字段时返回校验异常，其中 ID 只作为定位条件，不进入更新数据。纯 override 明确接管这些输入规则并负责达到同等安全结果，框架不会暗中改写其业务 DTO。任一项校验、Hook 或写入失败，整批不成功；事务开始后的任一失败回滚全部数据库修改。禁止部分成功。批量大小有服务端硬上限，超限返回校验异常。这是 Go 重写的原子性增强，不是复制 Node 的 TypeORM `save()` 内部实现。

按已确认时序，`ModifyBefore(add)` 看到的是安全字段清理前、但已执行 `InsertParam` 的 Mutation。Hook 是可信服务端扩展点，却不得把输入中的 ID 或只读字段当成授权依据。最终 Base DML 仍必须完成来源清理，测试要覆盖 Hook 无法让客户端来源的受限字段穿透到主记录写入。

### 6.4 Delete、Update、Info、List、Page

| 方法       | 默认行为                                                |
| -------- | --------------------------------------------------- |
| `Delete` | 规范化标量、逗号字符串或数组 ID；执行一次前置 Hook，并按 `cool.crud.softDelete` 在同一事务中归档后删除或直接硬删除，再执行一次后置 Hook；完整契约见 4.7 |
| `Update` | 支持 Node 的单对象或顶层数组输入；每项必须有合法 ID；整批事务；只更新明确出现且允许写入的字段 |
| `Info`   | 主键查询；移除 `InfoIgnoreProperty` 和隐藏字段                    |
| `List`   | 不分页查询；应用 `ListQueryOp`；受最大返回条数限制                    |
| `Page`   | 分页查询；应用 `PageQueryOp`；返回 `list + pagination`        |

分页结构：

```json
{
   "list": [],
   "pagination": {
      "page": 1,
      "size": 15,
      "total": 0
   }
}
```

### 6.5 修改 Hook

```go
type ModifyBeforeHook[E any, ID comparable] interface {
   ModifyBefore(context.Context, *Mutation[E, ID]) error
}

type ModifyAfterHook[E any, ID comparable] interface {
   ModifyAfter(context.Context, *Mutation[E, ID]) error
}

type Mutation[E any, ID comparable] struct{ /* private fields */ }

type Action = crud.Action

func (mutation *Mutation[E, ID]) Action() Action
func (mutation *Mutation[E, ID]) AddInput() AddInput[E]
func (mutation *Mutation[E, ID]) UpdateInput() UpdateInput[E, ID]
func (mutation *Mutation[E, ID]) DeleteIDs() []ID
func (mutation *Mutation[E, ID]) ResultIDs() []ID
```

`Mutation` 包含 `Action`、单个/批量输入、Delete IDs 和写入后的 Result IDs。`ModifyAfter` 因而可以读取数据库生成的主键。Hook 通过结构化 API 修改数据，不接收数据库 map。

Before/After 拆成两个小接口，业务可以只实现其中一个。`cool generate` 静态记录 Service 直接声明的 Hook 并生成调用 Adapter；缺少的一侧使用生成的 no-op，不依赖运行时反射或要求业务补空方法。

约束：

- 前置和后置 Hook 各最多执行一次。
- 批量操作的 Hook 面向整批数据，不按项重复调用。
- Dispatcher 先开启事务，再执行 `ModifyBefore`。
- `ModifyAfter` 在写入完成、事务提交前执行。
- 两个 Hook 内通过 `Model(ctx)` 进行的数据库修改都属于同一事务。
- 后置 Hook 失败会回滚数据库写入。
- 需要在事务提交后可靠执行的副作用必须在 Hook 内通过 Outbox 入队，不得直接调用外部系统或 Broker。Outbox 记录与主 DML 使用 Dispatcher 建立的当前 `dbtx` Scope，Hook 失败时一起回滚。

### 6.6 重写规则

业务 Service 直接声明同名方法即重写该动作。Info/List/Page 可以使用可绑定的自定义 DTO 和返回类型：

```go
func (s *GoodsService) Page(
   ctx context.Context,
   request *dto.GoodsPageReq,
) (*dto.GoodsPageResult, error) {
   // 完全接管 Page。
   model, err := s.Model(ctx)
   if err != nil {
      return nil, err
   }
   return renderGoodsPage(ctx, model, request)
}
```

`cool generate` 使用 AST + `go/types` 静态区分业务 Service 直接声明的方法与 Base 提升方法，并识别 override 方法体中的直接 Base 委托调用，再为每个动作生成直接调用 Adapter。运行时不通过接口断言或反射判断是否重写，因为嵌入 Base 后提升方法本身已经满足相应方法集。

Add/Delete/Update 为了让 Dispatcher 在调用业务方法前构造统一 Mutation 并执行 Hook，必须保持对应 Base 输入和返回签名：

```go
func (s *Service) Add(context.Context, AddInput[E]) (AddResult[ID], error)
func (s *Service) Delete(context.Context, DeleteInput[ID]) error
func (s *Service) Update(context.Context, UpdateInput[E, ID]) error
```

Info/List/Page 重写必须以 `context.Context` 为第一参数，随后是一个 GoFrame 可绑定 DTO 或对应 Base 输入，返回只允许 `(T, error)` 或 `error`。自定义 Route Handler 也遵守该返回规则。生成器检查具体合法集合；无法绑定、无法构造 Mutation 或无法形成 HTTP/gRPC 响应时直接失败。

选择规则：

1. 未直接声明同名方法时，生成 Adapter 调用 Base 默认实现，并把 Controller 编译出的 `ActionPlan` 交给 Base。
2. 直接声明同名方法且没有直接调用对应 `s.Base.Xxx()` 时，标记为纯 override，Adapter 不套用该动作的 Base 增强。
3. 直接声明同名方法且方法体直接调用对应 `s.Base.Xxx()` 时，标记为 Base 委托 override。Adapter 在进入 Dispatcher 前执行该动作的 `Before/InsertParam` 等 Controller 前置增强并建立 ActionPlan；业务方法运行到 Base 调用点时，Base 从当前操作 Scope 取得同一 Plan、复用事务，并跳过已执行的 Hook 外壳。
4. Base 委托是生成期的动作级选择：调用必须直接出现在该 override 方法体中；藏在 helper、函数值或反射后的间接调用不参与识别。条件分支中出现直接调用也表示整个动作选择 Base 增强，避免同一路由因运行分支改变安全边界。纯 override 中的间接 Base 调用因没有 ActionPlan 必须返回 Core 配置错误，不能退回无增强执行。
5. 同一个请求只选择一次，Controller 前置增强、Hook 和事务均按其各自范围协调一次。

`Before/InsertParam` 属于 Controller 前置阶段，不能等到已经进入 Service 后再追溯。生成器正因提前识别 Base 委托，才能让 Adapter 在正确时序恢复全部对应 Base 增强，同时保留 `s.Base.Xxx()` 类似 Node `super.xxx()` 的使用语义。

Base 专用增强边界：

| 配置/能力                             | 默认 Base / Base 委托 | 纯 override |
| --------------------------------- | ----------------- | ---------- |
| `PageQueryOp` / `ListQueryOp`     | 生效                | 不自动生效      |
| `InsertParam` / `Before`          | 生效                | 不生效        |
| `InfoIgnoreProperty`              | 生效                | 不自动生效      |
| `HiddenFields` / `ReadonlyFields` | 生效                | 不自动生效      |
| `SortFields` / 默认排序               | 生效                | 不自动生效      |
| `cool.crud.softDelete` 事务归档         | 生效                | 不自动生效      |

始终生效的框架边界是路由、中间件、认证、权限、DTO 绑定、`gvalid`、统一响应/异常、Dispatcher 事务和 `ModifyBefore/ModifyAfter`。原生查询只能走受控 `NativeQuery`。

重写某个动作不影响其余五个动作。重写方法需要对应 Base 增强时必须在方法体中直接调用 `s.Base.Xxx()`；这等价于 Node 的 `super.xxx()`，但 Controller 前置增强、Hook 和事务都不会重复执行。

明确不提供：

```text
ServiceApis
AddMany
```

不提供注册任意函数的通用 `AfterCommit` API；提交后工作只能以结构化 Outbox 消息表达。自定义 Service 方法通过显式 `Route` 暴露；需要可靠执行的缓存、消息或外部副作用由 Outbox 承载；批量新增仍是同一个 `Add` 动作。

## 7. Controller 与 CRUD 配置

### 7.1 基本写法

```go
package controller

import (
   "net/http"

   corecontroller "github.com/toothdy/cool-admin-go-next/cool-next/core/controller"
   "github.com/toothdy/cool-admin-go-next/modules/demo/entity"
   demoservice "github.com/toothdy/cool-admin-go-next/modules/demo/service"
)

func GoodsController(service *demoservice.GoodsService) corecontroller.Definition {
   return corecontroller.Admin("").
      Options(corecontroller.RouterOptions{
         Description: "商品管理",
         TagName:     "商品",
      }).
      Curd(corecontroller.CurdOption{
         API:     corecontroller.AllAPI(),
         Entity:  entity.Goods{},
         Service: service,
         PageQueryOp: corecontroller.StaticQuery(corecontroller.QueryOp{
            KeyWordLikeFields: []corecontroller.ColumnRef{
               corecontroller.Field("title"),
               corecontroller.Field("remark"),
            },
            FieldEq: []corecontroller.FieldMatch{
               corecontroller.Eq(corecontroller.Field("status")),
            },
            AddOrderBy: []corecontroller.Order{
               corecontroller.Desc(corecontroller.Field("createTime")),
            },
         }),
         SortFields: []corecontroller.ColumnRef{
            corecontroller.Field("id"),
            corecontroller.Field("status"),
            corecontroller.Field("createTime"),
         },
         DefaultSort:  corecontroller.Field("createTime"),
         DefaultOrder: corecontroller.Descending,
      }).
      Route(corecontroller.Route{
         Method:     http.MethodPost,
         Path:       "/disable",
         Summary:    "禁用商品",
         Handler:    corecontroller.Handle(service.Disable),
         Bind:       corecontroller.BindJSON,
         Permission: "demo:goods:disable",
      }).
      Build()
}
```

Definition Builder 的完整公开入口：

```go
type Definition interface{ definition() }
type Builder interface {
   Options(RouterOptions) Builder
   Curd(CurdOption) Builder
   Route(routes ...Route) Builder
   Build() Definition
}

func Admin(path string) Builder
func App(path string) Builder
```

`Builder` 只能由 `Admin/App` 创建；`Build` 返回不可变 Definition。生成器发现返回 `Definition` 的顶层函数并生成直接构造与注册代码。

保留 `CurdOption` 拼写以兼容 Node 开发者的认知，不另行改为 `CrudOption`。`Admin("")` 表示 Controller 路径由生成器根据 `modules/<module>/controller/admin/**` 静态推导；示例文件若为 `modules/demo/controller/admin/goods.go`，路径就是 `/admin/demo/goods`。也可以传入显式路径覆盖推导结果。

单一生成文件决定后，业务模块不再拥有生成的 Columns；`Field("status")` 返回 `ColumnRef`，由生成器结合当前 `Entity` 静态校验。字段不存在、别名非法或引用了其他实体但未声明 Join 时，`cool generate` 直接失败。

### 7.2 CurdOption 完整字段

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

type BeforeFunc func(context.Context) error

type InsertParam interface {
   // 只能由框架的泛型 Insert 构造器实现。
   insertParam()
}

func Insert[E any](
   fn func(context.Context, *coreservice.Mutable[E]) error,
) InsertParam

type URLTag struct {
   Name string
   URL  []APIType
}

const TagIgnoreToken = "ignoreToken"

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

type Direction = crud.Direction

const (
   Ascending  = crud.Ascending
   Descending = crud.Descending
)

type ColumnRef = crud.ColumnRef

func Field(name string) ColumnRef
func FieldOf[E any](name string) ColumnRef
```

`Field/FieldOf` 是 Controller 的同名构造包装；`FieldOf[E](name)` 通过静态实体类型 `E` 定位 Descriptor，不接收可在运行时传错的 `entity any` 参数。`ColumnRef.Of(alias)` 是 `crud.ColumnRef` 自身的方法，通过别名直接可见。`Direction/ColumnRef` 不在 Controller 重新声明新类型。

`API` 和 `AllAPI` 每次返回新切片，Controller 构建器也复制传入值；不导出可被调用方修改的全局 CRUD slice。

字段语义：

| 字段                   | 行为                                                |
| -------------------- | ------------------------------------------------- |
| `Prefix`             | CRUD 路由前缀；为空时使用 Controller 路径，非空时覆盖该路径            |
| `API`                | 可选 `add/delete/update/info/list/page`；空切片不注册 CRUD |
| `PageQueryOp`        | `page` 独立查询配置                                     |
| `ListQueryOp`        | `list` 独立查询配置，不继承 `PageQueryOp`                   |
| `InsertParam`        | 新增时给每一项注入服务端字段，注入值覆盖同名客户端值                        |
| `Before`             | Base CRUD 请求绑定前执行，只接收请求 Context                   |
| `InfoIgnoreProperty` | `info` 响应额外排除字段                                   |
| `Entity`             | 业务实体类型，用于定位 Descriptor                            |
| `Service`            | 具体 Service 实例，生成器保证类型正确                           |
| `URLTag`             | 路由标签，例如忽略 Token；不承载 Service 动态方法                  |
| `HiddenFields`       | 所有响应隐藏，所有客户端写入直接拒绝                                |
| `ReadonlyFields`     | 可读取；Add 清除客户端值，Update 拒绝客户端修改                     |
| `SortFields`         | 前端允许排序的字段白名单                                      |
| `DefaultSort`        | 默认排序字段，必须在白名单和 Descriptor 中存在                     |
| `DefaultOrder`       | `asc` 或 `desc`                                    |

`Before` 不接收或修改请求体；请求体转换由绑定器和结构化输入负责。`InsertParam` 每次只接收一个新增项，顶层数组会按输入顺序调用多次，且不能拿数据库 map。`URLTag.URL` 只选择默认 CRUD API；为空表示当前 Controller 已启用的全部默认 CRUD API。自定义 Route 直接在 Route 上声明标签。

默认只读字段是 `id/createTime/updateTime`。回收状态只存在于 `cool_recycle`，业务实体和 `coreentity.Base` 都没有 `deletedAt/deletedTime/deleteTime`，因此 CRUD 字段策略中不存在需要额外过滤的逻辑删除字段。

`Before`、`InsertParam` 和其余 `CurdOption` 增强都只属于默认 Base 或生成期识别的 Base 委托路径。纯 override 不会自动执行它们；重写方法直接调用对应 `s.Base.Xxx()` 后，生成 Adapter 在进入 Service 前恢复包括 `Before/InsertParam` 在内的全部对应 Base 增强。

完整使用片段：

```go
Before: func(ctx context.Context) error {
   _, err := auth.Admin(ctx)
   return err
},
InsertParam: corecontroller.Insert[entity.Goods](func(
   _ context.Context,
   input *coreservice.Mutable[entity.Goods],
) error {
   return input.Set("status", 1)
}),
URLTag: &corecontroller.URLTag{
   Name: corecontroller.TagIgnoreToken,
   URL: corecontroller.API(
      corecontroller.APIInfo,
      corecontroller.APIList,
   ),
},
```

### 7.3 QueryOp 完整字段

`QueryOp`、`ColumnRef`、条件、Join、Select、Order、`QueryRequest` 和受控 `QueryBuilder` 都是协议无关值，实际类型归 `cool-next/crud` 所有。`core/controller` 通过类型别名和同名构造包装保留下面的 Node 风格使用入口；因此 `crud` 和 `core/service` 不需要反向 import `core/controller`。

底层声明位于 `crud`：

```go
package crud

type Direction string

const (
   Ascending  Direction = "asc"
   Descending Direction = "desc"
)

type ColumnRef struct{ /* private validated reference */ }

func NewColumnRef(name string) ColumnRef
func NewColumnRefOf[E any](name string) ColumnRef
func (field ColumnRef) Of(alias string) ColumnRef

type QueryRequest struct{ /* private presence-aware values */ }
type RequestValue struct{ /* private presence-aware request value */ }

func RequestField(name string, value any) RequestValue
func RequestNull(name string) RequestValue
func NewQueryRequest([]RequestValue) (*QueryRequest, error)

func (request *QueryRequest) Has(name string) bool
func (request *QueryRequest) Value(name string) (any, bool)
func (request *QueryRequest) String(name string) (string, bool)
func (request *QueryRequest) Bool(name string) (bool, bool)
func (request *QueryRequest) Strings(name string) ([]string, bool)

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

type JoinOp struct {
   Entity    any
   Alias     string
   Condition Condition
   Type      JoinType
}

type JoinType string

const (
   JoinLeft  JoinType = "left"
   JoinInner JoinType = "inner"
)

type Order struct {
   Column    ColumnRef
   Direction Direction
}

type QueryExtender func(
   context.Context,
   *QueryBuilder,
   *QueryRequest,
) error

type WhereProvider interface{ whereProvider() }
type Condition interface{ condition() }
type SelectField interface{ selectField() }

func Where(conditions ...Condition) WhereProvider
func EqValue(column ColumnRef, value any) Condition
func NeValue(column ColumnRef, value any) Condition
func In(column ColumnRef, values any) Condition
func LikeValue(column ColumnRef, value string) Condition
func RawWhere(expression string, args ...any) Condition
func On(left ColumnRef, right ColumnRef) Condition

func All(alias string) SelectField
func As(column ColumnRef, alias string) SelectField
func Asc(column ColumnRef) Order
func Desc(column ColumnRef) Order
func LeftJoin(entity any, alias string, on Condition) JoinOp
func InnerJoin(entity any, alias string, on Condition) JoinOp
func Eq(column ColumnRef) FieldEq
func EqFrom(column ColumnRef, requestParam string) FieldEq
func Like(column ColumnRef) FieldLike
func LikeFrom(column ColumnRef, requestParam string) FieldLike
func Extend(QueryExtender) QueryExtender

type QueryBuilder struct{ /* private query nodes */ }

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

`QueryBuilder` 只向当前计划追加结构化查询节点，不持有或执行 `*gdb.Model`，也不能取得数据库连接。Controller 的公开入口是这些真实别名和构造包装：

```go
package controller

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

type QueryProvider interface{ queryProvider() }

func StaticQuery(QueryOp) QueryProvider
func DynamicQuery(func(context.Context) (QueryOp, error)) QueryProvider
func RequestField(name string, value any) RequestValue
func RequestNull(name string) RequestValue
func NewQueryRequest([]RequestValue) (*QueryRequest, error)

func Field(name string) ColumnRef
func FieldOf[E any](name string) ColumnRef
func Where(conditions ...Condition) WhereProvider
func EqValue(column ColumnRef, value any) Condition
func NeValue(column ColumnRef, value any) Condition
func In(column ColumnRef, values any) Condition
func LikeValue(column ColumnRef, value string) Condition
func RawWhere(expression string, args ...any) Condition
func On(left ColumnRef, right ColumnRef) Condition
func All(alias string) SelectField
func As(column ColumnRef, alias string) SelectField
func Asc(column ColumnRef) Order
func Desc(column ColumnRef) Order
func LeftJoin(entity any, alias string, on Condition) JoinOp
func InnerJoin(entity any, alias string, on Condition) JoinOp
func Eq(column ColumnRef) FieldEq
func EqFrom(column ColumnRef, requestParam string) FieldEq
func Like(column ColumnRef) FieldLike
func LikeFrom(column ColumnRef, requestParam string) FieldLike
func Extend(QueryExtender) QueryExtender
```

`PageQueryOp` 和 `ListQueryOp` 通过两种显式构造方式支持静态或上下文相关配置：

```go
corecontroller.StaticQuery(corecontroller.QueryOp{...})

corecontroller.DynamicQuery(func(ctx context.Context) (corecontroller.QueryOp, error) {
   return corecontroller.QueryOp{...}, nil
})
```

字段匹配保持 Node 的两种写法：

```go
corecontroller.Eq(corecontroller.Field("status"))
corecontroller.EqFrom(corecontroller.Field("type"), "category")
corecontroller.Like(corecontroller.Field("title"))
corecontroller.LikeFrom(corecontroller.Field("title"), "name")
```

Where、Join、Select 和有序排序组合示例：

```go
corecontroller.QueryOp{
   Join: []corecontroller.JoinOp{
      corecontroller.LeftJoin(
         entity.Category{},
         "c",
         corecontroller.On(
            corecontroller.Field("categoryId").Of("a"),
            corecontroller.FieldOf[entity.Category]("id").Of("c"),
         ),
      ),
   },
   Where: corecontroller.Where(
      corecontroller.EqValue(
         corecontroller.Field("status").Of("a"),
         1,
      ),
      corecontroller.RawWhere("a.stock > ?", 0),
   ),
   Select: []corecontroller.SelectField{
      corecontroller.All("a"),
      corecontroller.As(
         corecontroller.FieldOf[entity.Category]("name").Of("c"),
         "categoryName",
      ),
   },
   AddOrderBy: []corecontroller.Order{
      corecontroller.Desc(corecontroller.Field("createTime").Of("a")),
      corecontroller.Asc(corecontroller.Field("id").Of("a")),
   },
}
```

`Extend` 使用受控查询器：

```go
Extend: corecontroller.Extend(func(
   ctx context.Context,
   query *corecontroller.QueryBuilder,
   request *corecontroller.QueryRequest,
) error {
   hasStock, submitted := request.Bool("hasStock")
   if submitted && hasStock {
      query.WhereGT(corecontroller.Field("stock"), 0)
   }
   return nil
}),
```

查询规则：

- 根实体别名固定为 `a`；未指定别名的 `ColumnRef` 解析到根实体。未配置 `Select` 时默认只展开根实体 `a`，Join 不自动把关联实体全部字段加入响应；关联字段必须显式使用 `All(alias)` 或 `As`。
- `FieldEq` 的请求值为数组时编译为参数化 `IN`。
- 请求字段缺失、零值、`false` 和显式 `null` 必须保持不同语义。
- `FieldLike` 只允许 Descriptor 中存在且配置允许的字段。
- `AddOrderBy` 使用有序 slice，禁止用 map 表达顺序。
- `Where`、`Join` 条件和 `Extend` 必须使用结构化条件或占位参数。
- `RawWhere` 的表达式必须是生成期可见的字符串常量，所有业务值只能通过后续参数绑定。
- `Select`、排序和导出字段都经过 Descriptor 与白名单校验。
- `QueryBuilder` 内部只收集不可变查询节点，暴露受控条件、选择、分组和排序操作；不持有 `gdb.Model`，也不提供换表、`Unscoped`、清除既有条件或取出原始 Model 的方法。
- 查询计划使用包内固定上限：最多 8 个 Join、128 个规范化节点和 1000 个绑定值。三项覆盖静态 `QueryOp` 与 `Extend` 追加内容，不新增公开 `QueryLimits` 或运行时全局配置。
- 列表、分页、批量写入和导出分别设置服务端硬上限。

### 7.4 路由

Controller 级完整选项：

```go
type RouterOptions struct {
   Sensitive          *bool
   Middleware         []MiddlewareRef
   Alias              []string
   Description        string
   TagName            string
   IgnoreGlobalPrefix bool
}

type MiddlewareRef = module.ComponentRef

func Bool(value bool) *bool
```

`Sensitive == nil` 时继承 Node 默认值 `true`；显式关闭使用 `Sensitive: corecontroller.Bool(false)`。其余 bool 字段的零值就是默认行为，不需要额外 presence 状态。

自定义路由完整定义：

```go
type Route struct {
   Method             string
   Path               string
   Summary            string
   Description        string
   Handler            Handler
   Bind               BindSource
   Middleware         []MiddlewareRef
   Tags               []URLTag
   Permission         string
   Transaction        TransactionPolicy
   IgnoreGlobalPrefix bool
}

type BindSource string
type TransactionPolicy struct{ /* private validated policy */ }

// Handler 的具体函数签名由 cool generate 使用 go/types 校验。
type Handler any

func Handle(handler any) Handler
func NonTransactional() TransactionPolicy

const (
   BindAuto  BindSource = "auto"
   BindJSON  BindSource = "json"
   BindQuery BindSource = "query"
   BindForm  BindSource = "form"
   BindPath  BindSource = "path"
   BindFile  BindSource = "file"
)
```

默认 CRUD 路由保持 Node 版形式：

| 动作     | HTTP | 路径                |
| ------ | ---- | ----------------- |
| add    | POST | `<prefix>/add`    |
| delete | POST | `<prefix>/delete` |
| update | POST | `<prefix>/update` |
| info   | GET  | `<prefix>/info`   |
| list   | POST | `<prefix>/list`   |
| page   | POST | `<prefix>/page`   |

自定义接口必须显式声明，例如：

```go
corecontroller.Route{
   Method:      http.MethodPost,
   Path:        "/changeStatus",
   Summary:     "修改商品状态",
   Handler:     corecontroller.Handle(service.ChangeStatus),
   Bind:        corecontroller.BindJSON,
   Permission:  "demo:goods:changeStatus",
   Middleware:  []corecontroller.MiddlewareRef{auditMiddleware},
}
```

`BindAuto` 由 HTTP 方法和 DTO 标签选择 JSON、Query、Form 或 Path；存在歧义时生成失败，业务应显式选择来源。`BindFile` 使用 GoFrame 文件上传绑定，不把文件内容写入普通 JSON DTO。

生成器静态检查 HTTP 方法、完整路径、别名、Handler 签名、绑定来源、权限、中间件、事务策略和标签冲突。`Transaction` 零值表示通过 `dbtx.Runner` 使用唯一 Framework Database Group；纯读取、流式响应或自行接受非原子行为的接口必须显式 `NonTransactional()`。任何执行 DML 后写入 Outbox 的 Route 都不得关闭事务；框架不根据 HTTP Method 或函数体猜测写操作，也不从 Service 方法自动制造 API。

`List/Page` 对外公共参数继续使用 Node 名称：

```text
keyWord
page
size
order
sort
isExport
maxExportLimit
```

其中 `order` 是字段名，必须命中 `SortFields`；`sort` 是同位置的 `asc/desc`。多字段时二者数量必须一致。内部 `Order.Column/Direction` 不改变这套 HTTP 契约。

### 7.5 请求处理顺序

```text
HTTP 路由命中
-> Trace / Recover / Access Log
-> 鉴权与 Session
-> 权限检查
-> 使用生成期 Adapter 选择路由类型
   -> 默认 Base：Before -> 框架输入绑定和 gvalid -> Add 时逐项 InsertParam -> 取得 ActionPlan
   -> Base 委托 override：Before -> 按 override 签名绑定和 gvalid -> Add 时逐项 InsertParam -> 取得 ActionPlan
   -> 纯 override：按 override 签名绑定和 gvalid，不自动执行 Base 增强，也不建立 Base ActionPlan
   -> 上述 CRUD 分支由 Dispatcher 通过 dbtx.Runner 开启或复用事务
      -> Add/Delete/Update 执行 ModifyBefore；Info/List/Page 不定义修改 Hook
      -> 调用 override 或 Base 默认实现
      -> Add/Delete/Update 执行 ModifyAfter
      -> Dispatcher 拥有事务时提交
   -> 自定义 Route：按 Handler 签名绑定和 gvalid
      -> 默认由 dbtx.Runner 开启或复用唯一 Framework Database Group
      -> 只有显式 NonTransactional 时直接调用 Handler
      -> 调用 Handler，Runner 拥有事务时提交
-> 统一响应/异常过滤
```

为避免 `core/service -> core/controller -> core/service` import cycle，协议无关执行计划由下层 `cool-next/crud` 所有；Controller 只负责把 `CurdOption/QueryOp` 编译成该类型：

```go
package crud

type Action string

const (
   ActionAdd    Action = "add"
   ActionDelete Action = "delete"
   ActionUpdate Action = "update"
   ActionInfo   Action = "info"
   ActionList   Action = "list"
   ActionPage   Action = "page"
)

type ActionPlan struct {
   action Action
   query  QueryPlan
   fields FieldPolicy
}

type QueryPlan struct{ /* private normalized query */ }
type FieldPolicy struct{ /* private normalized field policy */ }

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

type DescriptorResolver interface {
   Resolve(entity any) (coreentity.Metadata, bool)
}

func CompilePlan(
   context.Context,
   DescriptorResolver,
   PlanInput,
   *QueryRequest,
) (*ActionPlan, error)

func (plan *ActionPlan) Action() Action
func (plan *ActionPlan) Query() *QueryPlan
func (plan *ActionPlan) Fields() *FieldPolicy
func (plan *ActionPlan) ApplyQuery(
   context.Context,
   *gdb.Model,
) (*gdb.Model, error)

type OperationScope struct {
   plan *ActionPlan
}

func WithOperation(
   context.Context,
   *ActionPlan,
) context.Context

func CurrentOperation(context.Context) (*OperationScope, bool)
func (scope *OperationScope) Plan() *ActionPlan
```

`QueryPlan` 的规范化节点、编译和 GoFrame 应用逻辑归模块 21 所有，均作为 `crud` 包内部能力；模块 25 只把查询计划与字段策略组合成 `ActionPlan`，并通过既有 `CompilePlan` 和 `ActionPlan.ApplyQuery` 暴露唯一公开入口。框架不增加公开 `CompileQuery`，也不在 `ActionPlan` 中复制查询编译逻辑。

生成图实现 `DescriptorResolver`。Controller 先解析静态/动态 `QueryProvider`，`Extend` 只向节点收集器追加结构化表达式，再把根 Entity、协议无关 `QueryOp`、字段策略和请求值交给 `crud.CompilePlan`；该函数通过 Resolver 完成根字段、Join Entity、别名、排序白名单和请求值校验，返回不可变计划，但不取得数据库 Model。`ActionPlan.ApplyQuery` 使用模块 21 的内部应用器，先克隆传入 Model、绑定 Context 和根别名 `a`，再参数化应用 Join、Where、Select、Group、Having 和 Order；调用不会修改原 Model 或 QueryPlan。

默认 Page 在 Base 中按 `plan.ApplyQuery(ctx, model) -> Page(page, size) -> AllAndCount(false)` 执行。GoFrame 从同一 Model 克隆数据与 Count 查询，Count 自动忽略分页和排序，并在存在 Group/Having 时统计分组子查询；框架不再实现 Node 的 `getCountSql` 或另一套分页 SQL 渲染器。

`ActionPlan` 不包含 `Before/InsertParam`：这两个回调由生成 Adapter 在默认 Base 或 Base 委托路径进入 Dispatcher 前执行。删除归档也不属于 Controller ActionPlan；Base 在真正执行时读取全局 `cool.crud.softDelete`，并使用自身 Descriptor 完成 4.7 的锁定、强类型快照和物理删除。Dispatcher 通过 `dbtx.Runner` 建立事务 Scope，并把 ActionPlan 另行写入派生 Context；Base 调用事务内 `Model(ctx)` 并先处理其组校验错误，再调用 `plan.ApplyQuery`，此时才把已校验节点编译到 GoFrame Model。

Base 通过 `CurrentOperation` 读取 Plan，通过 `dbtx.Current` 读取 TX，不缓存到单例 Service 字段，也不 import Controller。Base 委托 override 调用 `s.Base.Xxx()` 时复用同一 Plan 和 TX，但不重复 Controller 前置增强或 Hook。

HTTP 正常响应保持：

```json
{"code":1000,"message":"success","data":{}}
```

无数据时可省略 `data`，但 `data: 0`、`data: false` 和 `data: []` 不得因零值被错误省略。

## 8. Exception

异常名称和业务码参考 `cool-admin-midway-packages/core/src/exception`；未知错误隐藏、gRPC 映射和错误链保留是 Go 重构的安全增强：

| 概念              |     业务码 | 用途          |
| --------------- | ------: | ----------- |
| `BaseException` | 由具体异常决定 | Cool 异常共同载体 |
| `Comm`          |    1001 | 通用业务失败      |
| `Validate`      |    1002 | 参数和业务校验失败   |
| `Core`          |    1003 | 框架或基础设施失败   |

公共 Go API：

```go
type BaseException struct {
   Name       string
   Code       int
   Message    string
   StatusCode int
   Cause      error
}

func (e *BaseException) Error() string
func (e *BaseException) Unwrap() error

func Comm(message string, statusCode ...int) error
func Validate(message string, statusCode ...int) error
func Core(message string, statusCode ...int) error

func WrapComm(cause error, message string, statusCode ...int) error
func WrapValidate(cause error, message string, statusCode ...int) error
func WrapCore(cause error, message string, statusCode ...int) error
```

基础常量固定为：

```go
const (
   Success      = 1000
   CommFail     = 1001
   ValidateFail = 1002
   CoreFail     = 1003
)

const (
   MsgSuccess      = "success"
   MsgCommFail     = "comm fail"
   MsgValidateFail = "validate fail"
   MsgCoreFail     = "core fail"
)

const (
   CommException     = "CoolCommException"
   ValidateException = "CoolValidateException"
   CoreException     = "CoolCoreException"
)
```

`statusCode` 只能省略或传一个值。构造和 Wrap 方法使用 GoFrame `gerror` 保留堆栈；Wrap 方法同时保留 `Cause`，调用方使用 `errors.As/Is`，不依赖字符串比较。

`Comm/Validate/Core` 分别把 `Name` 固定为 `CommException`、`ValidateException`、`CoreException`。`StatusCode == 0` 表示调用方未指定，HTTP Filter 按默认 200 处理。

Go 用法：

```go
return exception.Comm("用户名已经存在~")
return exception.Validate("非法传参~")
return exception.Core("暂不支持当前数据库类型")
return exception.Comm("登录失效~", http.StatusUnauthorized)
return exception.WrapCore(err, "读取登录会话失败")
```

异常必须由 `gerror` 包装或实现等价堆栈能力。公共 API 不增加 Node 中不存在的异常分类体系。

### 8.1 HTTP 映射

- Cool 异常默认 HTTP Status 为 200，响应保持 `{code,message,data}`。
- 显式提供 HTTP Status 时使用该状态，例如 401、403。
- 已知 Cool 异常可以返回安全业务消息。
- 普通未知 `error` 记录完整错误链、堆栈和 Trace ID；客户端返回安全的 `1001 / comm fail`，HTTP Status 为 500。
- 已识别的 Redis 等基础设施故障使用 `WrapCore`，客户端返回安全的 `1003 / core fail`；日志保留内部 Cause。
- Redis、SQL、文件路径、连接串和内部堆栈不得进入接口响应。

### 8.2 gRPC 映射

| Cool 异常           | gRPC Code            |
| ----------------- | -------------------- |
| `Validate`        | `InvalidArgument`    |
| `Core`            | `Internal`           |
| `Comm` + HTTP 401 | `Unauthenticated`    |
| `Comm` + HTTP 403 | `PermissionDenied`   |
| 其他 `Comm`         | `FailedPrecondition` |
| 未知错误              | `Internal`           |

Cool 业务码和安全消息写入标准化 Error Details，日志保留原始 Go 错误链。HTTP 与 gRPC 只是同一异常的不同传输映射。

## 9. 鉴权与 Session

### 9.1 JWT + 服务端 Session

鉴权保留 Node 的 JWT + 服务端状态校验思想，并按已确认的 Go 方案重构为可配置 SessionStore；默认 Redis 不是 Node 当前默认缓存行为：

协议无关公共契约：

```go
type SessionStore interface {
   Get(context.Context, string) (Session, bool, error)
   Save(context.Context, Session) error
   RotateRefresh(
      ctx context.Context,
      sessionID string,
      expectedRefreshJTI string,
      next Session,
   ) error
   Revoke(context.Context, string) error
   RevokeUser(context.Context, Kind, uint64) error
}

type Kind string

const (
   AdminKind Kind = "admin"
   AppKind   Kind = "app"
)

type AdminIdentity struct {
   UserID    uint64
   Username  string
   PasswordV int
   roleIDs   []uint64
}

func (identity AdminIdentity) RoleIDs() []uint64

type AppIdentity struct {
   ID uint64
}

func Admin(context.Context) (AdminIdentity, error)
func App(context.Context) (AppIdentity, error)
```

`Admin` 和 `App` 是必需身份访问器：Context 未携带已验证身份或 Subject 类型不匹配时返回安全错误，不返回可误用的零值，也不通过 `Must*` panic。不将标准 `context.Context` 替换为带 `admin/app` 字段的自定义 Context；这使 HTTP、gRPC、任务和直接 Service 调用保持同一套协议无关签名。Identity 中的 slice 不得公开；`RoleIDs()` 每次返回防御性副本，修改返回值不能改变 Context 中的已验证身份。

`Session` 是 auth 包内部构造的不可变值，至少携带 Session ID、`Kind`、用户 ID、Access JTI、Refresh JTI、过期时间和密码版本；业务模块不能伪造 Session 后直接写入 Context。`Get/Revoke` 的字符串参数固定为 Session ID。`RotateRefresh` 在同一个原子存储操作中按 Session ID 读取旧 Session、比较调用方提供的旧 Refresh JTI，并替换为包含新 Access/Refresh JTI 的 Session；`next` 必须保持相同 Session ID、Subject 和用户 ID，只允许轮换 JTI、快照字段及有效期。旧 JTI 不匹配返回可识别的刷新重放错误，不能用普通 Get + Save 模拟。`RevokeUser` 必须同时匹配身份种类和 ID，Admin/App 数值 ID 相同也不能互相撤销。HTTP Middleware 与 gRPC Interceptor 都只通过上述函数向领域层暴露已验证身份。

```yaml
cool:
   auth:
      session:
         type: redis
         group: default
         prefix: "cool:session:"
```

省略 `cool.auth.session` 时默认使用 Redis、`default` 连接组和 `cool:session:` 前缀。只有显式配置才使用内存：

```yaml
cool:
   auth:
      session:
         type: memory
```

Redis 规则：

- 启动时读取指定 Group 并执行连接/PING；失败时日志输出完整内部错误，Application Host 启动失败。
- 不自动降级为 Memory。
- 运行时 Redis 失败：鉴权失败关闭，接口返回安全的 `1003`，不返回 Redis 细节。
- 基础设施失败不是登录失效，不得伪装成 401。

Redis Session 的持久值使用版本化 UTF-8 JSON，Key 固定为 `prefix + sessionId`。首版逻辑格式为：

```json
{
   "schemaVersion": 1,
   "sessionId": "stable-session-id",
   "subject": "admin",
   "userId": "9007199254740993",
   "username": "admin",
   "roleIds": ["1", "2"],
   "passwordV": 3,
   "accessJti": "current-access-token-id",
   "refreshJti": "current-refresh-token-id",
   "expiresAt": 1785686400000
}
```

`subject` 只允许 `admin/app`；`userId/roleIds` 使用十进制字符串，避免 uint64 经其他语言读取时丢失精度；`expiresAt` 是 Refresh Session 的 Unix 毫秒过期时间。Admin 必须携带 `username/roleIds/passwordV`，App 不得伪造 Admin 字段。解码时必须校验 Key 与 `sessionId` 一致、两个 JTI、必填字段、数值范围、Subject 字段集合和过期时间；未知字段为滚动升级兼容而忽略，未知 `schemaVersion` 则失败关闭。Redis TTL 根据 `expiresAt` 设置且不得晚于该时间，读取时即使 Key 尚存也要再次拒绝已过期 Session。密码、Token、数据库连接和任意业务 Payload 不得写入该值或日志。

Memory Store 必须并发安全，按 Session TTL 清理过期数据；进程重启后状态丢失，因此只适用于测试、本地开发或明确接受该限制的单实例部署。

JWT 默认配置对齐 Node 的有效期，并补齐安全边界：

```yaml
cool:
   auth:
      jwt:
         issuer: cool-admin-go-next
         audience: cool-admin
         algorithm: HS256
         currentKeyId: primary
         accessTTL: 2h
         refreshTTL: 360h
         clockSkew: 30s
         keys:
            primary: ${COOL_AUTH_JWT_PRIMARY_SECRET}
```

首期算法固定为 `HS256`，不能按 Token Header 动态选择其他算法。签发 Header 必须包含 `kid`；`currentKeyId` 指向当前签发密钥，`keys` 可以同时保留旧验证密钥以支持平滑轮换。全部密钥至少 32 个随机字节，不能为空、不能使用仓库默认值；Issuer、Audience、当前 Key、TTL 和 Clock Skew 在启动时校验。未知 `kid`、错误算法、错误 Issuer/Audience 或缺失必填 Claim 一律失败关闭。

Admin Access/Refresh JWT Claims 固定为：

```text
sessionId jti subject isRefresh roleIds username userId passwordV
iss aud iat nbf exp
```

Admin 密码版本命名不做边界转换：Go 字段固定为 `PasswordV`，数据库列、JSON、Redis Session 和 JWT Claim 固定为 `passwordV`。这是 v2 相对 Node `passwordVersion` Claim 的明确不兼容变更，不提供双字段或别名兼容。App JWT 单独建模，不复用 Admin Identity：

```text
sessionId jti subject id isRefresh iss aud iat nbf exp
```

`sessionId/jti/subject/isRefresh/iss/aud/iat/nbf/exp` 是两类 Token 的必填 Claim，不能再作为可选内部字段。签名校验后必须用 `sessionId` 读取 Session，并核对 Subject、用户 ID、JTI、密码版本和 Session 有效期；Identity 以服务端 Session 为准构造。Access Token 必须是 `isRefresh=false` 且 JTI 等于 Session 的 Access JTI；Refresh Token 必须是 `isRefresh=true` 且 JTI 等于 Session 的 Refresh JTI。每次刷新都以当前服务端 Session 重新生成一对 Token，并通过 `RotateRefresh` 原子替换两个 JTI，不能信任旧 Refresh Token 中携带的角色或用户快照直接续签；同一个旧 Refresh Token 并发或再次使用时最多一次成功，检测到重放后撤销整个 Session。刷新不能绕过 Session、密码版本和撤销检查。Access/Refresh 默认有效期分别为 Node 的 2 小时和 15 天，配置只能改为正数且 Access TTL 必须小于 Refresh TTL。

权限执行状态固定为：

1. `TagIgnoreToken` 路由不要求 Token，也不执行权限检查。
2. 未声明 `Permission` 的受保护路由只要求有效登录态。
3. 声明 `Permission` 的路由在登录后继续调用统一 Authorizer；HTTP 使用规范化 `method + path`，gRPC 使用完整 Method。
4. 超级管理员绕过只能由后续业务模块提供的可信策略判断，框架不得用 `username == "admin"` 等字符串规则猜测。
5. 鉴权或权限存储故障返回安全 Core 错误；只有凭证无效返回 401，权限明确不足返回 403。

这一定义只固定框架调用边界；角色、菜单、权限数据结构和超级管理员业务规则仍属于后续 `base` 模块设计，必须另行确认。

### 9.2 通用缓存边界

首期框架不提供 Node `coolCache` 对应的通用 KV Cache API。`SessionStore` 是鉴权专用持久状态，不能作为业务缓存注入或复用；GoFrame/Redis Client 也不作为框架级业务 Cache 抽象直接暴露。需要缓存的业务模块应在自身 `contract/**` 声明窄接口并注入 Adapter；数据库提交后的可靠失效必须写入 Outbox，允许丢失的进程内派生缓存则必须有 TTL 或可重建。是否增加统一 Cache 包必须在出现至少两个业务模块的共同语义后另行设计，不能从 SessionStore 顺手扩展。

## 10. Application Host 与 gRPC

### 10.1 协议无关启动内核

`cool-next/core/app` 是协议无关的 Application Host，负责配置、依赖图、生命周期和 Transport 协调。它只依赖通用 `Transport` 接口；HTTP 与 gRPC 的具体实现由适配层注入。入口保持 GoFrame 风格：

```go
// main.go：业务应用入口
package main

import (
   "os"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/gogf/gf/v2/os/gctx"
   "github.com/toothdy/cool-admin-go-next/cool-next/core/app"
   "github.com/toothdy/cool-admin-go-next/modules"
)

func main() {
   ctx := gctx.GetInitCtx()
   if err := app.Run(ctx, modules.Generated()); err != nil {
      g.Log().Error(ctx, err)
      os.Exit(1)
   }
}
```

`module.Graph` 是框架公开的只读装配结果，而不是业务可直接填写的 struct：

```go
package module

type Graph struct{ /* private validated graph */ }
```

`modules/modules_gen.go` 只通过 `module` 包提供的图编译器构造它；图编译器完成 Provider、依赖拓扑、生命周期、Transport、Producer/Consumer 和路由一致性校验后才返回不可变 `Graph`。`modules.Generated()` 返回该值，业务代码不能绕过校验修改节点或边。`app.Run(context.Context, module.Graph) error` 消费该已验证图，保留错误链并把 Host 能观察到的启动或运行期失败交给入口；协议无关 Host 自身不得 Fatal 或直接退出进程。根 `main.go` 只做应用装配和最终日志/退出码处理；`cmd/cool/main.go` 是 `cool generate/check/build/run` CLI 的独立入口，不再新增与 README 冲突的 `cmd/main.go`。两者都遵守“入口 -> app/codegen -> generated graph”的依赖方向。

### 10.2 Transport 配置

支持 HTTP-only、gRPC-only 或同时运行：

```yaml
cool:
   transports:
      http:
         enabled: true
      grpc:
         enabled: false
```

HTTP 使用 GoFrame `ghttp`，gRPC 使用 GoFrame `grpcx`。`cool-next/grpc` 只提供拦截器、异常映射、上下文和生成注册桥接，不发明私有 RPC 协议。

Application Host 内部 Transport 契约固定为：

```go
type Transport interface {
   Name() string
   Prepare(context.Context) error
   Start(context.Context) (<-chan error, error)
   Stop(context.Context) error
}
```

`Prepare` 必须同步完成配置、Handler/Service 注册和端口绑定。`Start` 在服务循环启动后返回一个只报告可观察终止结果的 channel；Transport 不得把普通业务错误写入该 channel。`Stop` 对 Prepared 和 Started 两种状态都有效：前者关闭 Listener，后者停止接收新请求并等待在途请求。该接口属于 `core/app` 内部装配边界，业务模块不直接依赖。GoFrame HTTP 的运行期 Serve 错误不在当前公开 API 的可观察集合内，采用下面明确的进程级快速失败规则，不能伪装成该 channel 能上报。

GoFrame 公开 API 没有统一的 bind-only 阶段，因此适配器固定这样实现：

- HTTP `Prepare` 创建并完整配置 `ghttp.Server`，安装生成路由和中间件，先执行 `net.Listen`，再调用 `ghttp.Server.SetListener(listener)`；Adapter 必须把该 Listener 保存在自身 Prepared 状态，端口冲突因而在进入 `OnStart` 前返回给 Host。
- HTTP `Start` 调用 `ghttp.Server.Start()` 后，仍要在独立启动超时内等待 Server Status 进入 Running；GoFrame `Start()` 在 Listener 创建完成后即可返回，不能单凭其返回值宣称服务循环已经运行。同步错误、等待超时或提前停止都返回启动失败。
- HTTP `Stop` 在 Prepared 状态直接关闭 Adapter 保存的 Listener；在 Started 状态调用的公开 API 精确为 `ghttp.Server.Shutdown() error`，它不接受 `context.Context`。Adapter 必须在受监督 goroutine 中调用无参 `Shutdown()`，由 `Stop(ctx)` 和 Host 在外部执行超时选择；Shutdown 返回、报错或外部超时后都关闭 Adapter 自己持有的 Listener，`net.ErrClosed` 视为幂等成功，超时则返回 `ctx.Err()` 供 Host 聚合。这样 Prepare 后的 `OnStart` 失败、HTTP 自身 Start 失败、尚未轮到 Start 的回滚和无法由 GoFrame 签名表达的超时都不会泄漏端口。
- GoFrame `ghttp.Server.Start()` 内部 Serve goroutine 遇到非正常 Listener/Serve 错误会调用 `Fatalf`。当前架构接受这一个由上游实现决定的进程级快速失败边界：进程必须非零退出，由 Docker、Kubernetes、systemd 等部署监督器重新拉起；单机 `cool run` 则直接把非零退出交给调用方。整个进程退出保证 HTTP 失效后不会继续对外保持 Ready，也不会出现同进程 gRPC/Worker 单独存活。部署监督器和健康检查是生产运行前置条件，不能配置 GoFrame 吞掉该错误后继续运行。
- gRPC `Prepare` 先创建 Listener、`grpcx.Server.New()`、安装拦截器并注册全部 Service；`grpcx.GrpcServer.Server` 的公开类型是 `*grpc.Server`，因此 `Start` 在受监督 goroutine 中调用 `grpcxServer.Server.Serve(listener)`。
- gRPC Adapter 不调用自行 Listen、处理信号且错误路径可能 Fatal 的 `grpcx.Run/Start`；Application Host 统一持有信号、错误传播和退出权。
- 直接调用底层 `Serve` 会绕过 `grpcx.Run` 内部的 gsvc 注册、注销和信号处理，而且对应 `doServiceRegister/doServiceDeregister` 是 grpcx 私有方法，Adapter 不得假设可以调用它们。启用服务注册发现时，Adapter 必须通过公开的 `gsvc.GetRegistry().Register/Deregister` 自己保存注册后返回的 `gsvc.Service` 并成对注销；未配置 Registry 时跳过。`Stop` 先 `GracefulStop`，超时后调用底层 `Stop`，再关闭 Listener。
- Transport 启动到 Ready 之间由强制 Ready Gate 拒绝业务请求，HTTP 返回 503，gRPC 返回 `Unavailable`。

gRPC 注册桥接使用 GoFrame 的服务端对象：

```go
type GRPCRegistrar interface {
   Register(*grpcx.GrpcServer) error
}
```

`cool generate` 当前就生成可选 Registrar Adapter，Adapter 内调用标准 protobuf 的 `RegisterXxxServer(server.Server, implementation)`；它不是未来占位，也不替代 `*.pb.go` 或 `*_grpc.pb.go`。

### 10.3 生命周期

组件按需实现 Go 风格接口：

```go
type Initializer interface {
   OnInit(context.Context) error
}

type Starter interface {
   OnStart(context.Context) error
}

type Stopper interface {
   OnStop(context.Context) error
}
```

启动顺序：

```text
读取并校验配置
-> 生成图一致性检查
-> 按拓扑构造组件
-> 按拓扑执行 OnInit
-> 对所有已启用 Transport 执行 Prepare，完成注册和端口绑定
-> 按拓扑执行 OnStart
-> 启动全部 Transport 并登记终止 channel
-> 全部成功后标记 Ready
```

拓扑顺序始终优先于 `Order`。没有依赖先后关系的同层组件，在 `OnInit` 和 `OnStart` 阶段按所属模块 `Order` 从大到小，再按模块路径和组件符号名稳定排序；`OnStop` 严格按实际成功登记顺序逆序执行。该 tie-break 与全局中间件的同层排序规则一致。

停止和失败回滚：

- 任一构造、初始化、端口绑定或启动失败，整个 Host 启动失败。
- 实现 Stopper 且实现 Initializer 的组件，在 `OnInit` 成功后进入清理栈；未实现 Initializer 的 Stopper 在构造成功后立即进入清理栈。
- `OnInit` 返回错误的组件必须自行清理本次未完成初始化产生的局部资源；Host 随后继续清理此前已入栈组件。
- 任一 Transport Prepare 失败或后续 `OnStart` 失败，按逆序 Stop 所有已 Prepared Transport，包括尚未 Start 的 Listener，再消费组件清理栈。
- 正常关闭同样消费 Transport 栈和组件清理栈，不以 `OnStart` 是否执行作为组件清理资格。
- 每个组件最多停止一次。
- HTTP 和 gRPC 同时启用时是一个部署单元，不能一个 Ready、另一个失败后继续假运行。
- 任一 Transport `Start` 失败时，按逆序 `Stop` 全部已 Prepared Transport，包括启动失败者自身、已经启动者和尚未轮到 Start 的 Listener，再消费组件清理栈。
- Ready 后任一可观察的终止 channel 报错或意外关闭时，Host 立即撤销 Ready、停止其他 Transport 并消费组件清理栈，`app.Run` 返回合并后的错误。GoFrame HTTP 内部 Fatal 路径按 10.2 直接让整个进程非零退出并由部署监督器重启，不声称 Host 有机会执行清理；未提交数据库事务由连接断开回滚，Outbox/Consumer 的在途状态按 Lease、Broker 重投和 Inbox 契约恢复。
- 关闭信号先撤销 Ready，再停止接收请求，等待在途请求，最后释放组件。
- `OnStop/Transport.Stop` 即使返回错误也必须继续清理剩余项；所有错误按确定顺序聚合，并受全局关闭超时控制。
- Outbox Worker 是受 Host 管理的生命周期组件，通过依赖图保证数据库、Schema 和 Publisher 就绪后才 `OnStart`；Worker 启动失败会阻止整个应用进入 Ready。
- Outbox Worker 的 `OnStop` 先停止领取新消息，再在全局关闭超时内等待已领取的发布完成；超时遗留消息不强制标记成功，由 Lease 到期后被其他实例重新领取。

### 10.4 HTTP/gRPC 共用上下文

两种 Transport 都要建立相同的协议无关上下文：

- Trace ID；
- Admin/App Identity；
- Session 状态；
- 权限信息；
- 请求截止时间与取消信号。

领域 Service 只接收 `context.Context` 和领域/框架请求类型。Controller 或 gRPC Adapter 负责协议 DTO 转换，禁止把 `*ghttp.Request`、`grpc.ServerStream` 或 protobuf Request 传入领域 Service。

## 11. 事件、任务与可靠副作用

### 11.1 能力边界与交付语义

- `core/event` 是进程内事件能力，只用于同一进程内解耦，不承诺跨实例投递或进程崩溃后的恢复，不得用于必须送达的副作用。Definition、Handler、顺序和错误传播由 Event 专项设计冻结。
- `schedule` 是基于时间触发的框架任务能力，由 Application Host 管理启动和停机；Cron 表达式、重叠策略、多实例单次执行、超时和补跑由 Schedule 专项设计冻结。
- `queue` 是分布式异步任务能力；Definition、Ack、重试、超时、DLQ、并发和 Broker Adapter 由 Queue 专项设计冻结。需要和业务数据库事务可靠提交的任务必须通过 Outbox 产生，消费端继续使用稳定 `messageId` 和 Inbox，不允许 Queue 专项设计另造一套冲突的事务投递或幂等协议。
- 事件、定时任务、队列、Outbox Producer 和 Inbox Consumer 最终都由 `cool generate` 静态注册并参与依赖注入；前三类在对应专项设计批准前属于明确的实现门禁，当前文档只冻结职责与可靠性边界。
- 数据库写入后必须可靠执行的缓存失效、消息通知或外部调用统一写入 Transactional Outbox；业务事务内不得直接调用 Broker 或外部系统。
- Outbox 的交付语义固定为 **at-least-once**。Inbox 和业务幂等键使同一 `messageId` 最多产生一次本地业务效果，但框架不宣称数据库、Broker 和任意外部系统之间存在分布式 exactly-once。
- Handler 必须传播 Context、Trace ID 和 `messageId`。对不支持幂等键的外部接口，业务必须接受重复风险或增加可查询、可补偿的适配层。

未来切微服务时，消息 Envelope 和消费幂等契约保持不变，只替换 Publisher/Consumer 的传输实现。

### 11.2 消息 API

业务只能构造不可变 Envelope 并入队，不能直接取得或调用 Publisher：

```go
package outbox

// MessageID 是 RFC 9562 UUIDv7 的小写标准文本形式。
type MessageID string

// Envelope 的字段私有；创建后不能修改 ID、Topic、版本或载荷。
type Envelope struct{ /* private fields */ }

type Option func(*messageOptions) error

func New[T any](topic, messageType string, payload T, options ...Option) (Envelope, error)
func WithKey(key string) Option
func WithVersion(version uint32) Option
func WithHeader(name, value string) Option

func (message Envelope) MessageID() MessageID
func (message Envelope) Topic() string
func (message Envelope) MessageType() string
func (message Envelope) Version() uint32
func (message Envelope) Key() (string, bool)
func (message Envelope) Payload() []byte
func (message Envelope) Headers() map[string]string

type Enqueuer interface {
   Enqueue(context.Context, Envelope) error
}

// Publisher 只供 Worker 的基础设施 Adapter 实现和调用。
type Publisher interface {
   Publish(context.Context, Envelope) error
}

type Incoming[T any] struct{ /* private decoded message */ }

func (message Incoming[T]) MessageID() MessageID
func (message Incoming[T]) Topic() string
func (message Incoming[T]) MessageType() string
func (message Incoming[T]) Version() uint32
func (message Incoming[T]) Key() (string, bool)
func (message Incoming[T]) Payload() T
func (message Incoming[T]) Headers() map[string]string

type ConsumerHandler[T any] func(context.Context, Incoming[T]) error
type ConsumerDefinition interface{ consumerDefinition() }

func Consume[T any](
   name string,
   topic string,
   messageType string,
   supportedVersions []uint32,
   handler ConsumerHandler[T],
) (ConsumerDefinition, error)

func Permanent(error) error

// Restore 只供基础设施 Adapter 从 Broker 元数据恢复 Envelope。
func Restore(
   messageID MessageID,
   topic string,
   messageType string,
   version uint32,
   key *string,
   payload []byte,
   headers map[string]string,
) (Envelope, error)

type Subscription struct{ /* private immutable registration */ }

func (subscription Subscription) Name() string
func (subscription Subscription) Topic() string
func (subscription Subscription) MessageType() string
func (subscription Subscription) SupportedVersions() []uint32

type DeliveryDisposition string

const (
   DeliveryAck        DeliveryDisposition = "ack"
   DeliveryRetry      DeliveryDisposition = "retry"
   DeliveryDeadLetter DeliveryDisposition = "dead-letter"
)

type DeliveryDecision struct{ /* private validated decision */ }

func (decision DeliveryDecision) Disposition() DeliveryDisposition
func (decision DeliveryDecision) RetryAfter() time.Duration
func (decision DeliveryDecision) Error() error

// attempt 从 1 开始，是 Adapter 在调用前已持久化的本次消费次数。
type DeliverFunc func(
   context.Context,
   Subscription,
   Envelope,
   uint32,
) DeliveryDecision

type ConsumerCapabilities struct {
   DurableAck            bool
   DurableRetryAttempts  bool
   DelayedRetry          bool
   DeadLetter            bool
   PreservesMessageID    bool
   MaxEnvelopeBytes      int
}

type ConsumerAdapter interface {
   Name() string
   Capabilities(context.Context) (ConsumerCapabilities, error)
   Prepare(context.Context, []Subscription, DeliverFunc) error
   Start(context.Context) (<-chan error, error)
   Stop(context.Context) error
}
```

`ConsumerCapabilities.DeadLetter` 只有在 DLQ 持久化以及按稳定 Consumer Name + `messageId` 执行检查、受控重放的管理能力同时可用时才能为 true；仅能把消息投进一个无法检查或重放的队列不满足可靠 Consumer 契约。

`New` 在进入任何数据库事务前，使用符合 RFC 9562 且随机部分来自密码学安全随机源的 UUIDv7 生成全局稳定 `messageId`；文本固定为 36 字节、小写、带连字符的标准格式。并发和本地时钟回拨时仍必须保持合法且不重复，不把严格单调排序作为协议保证。`New` 同时验证非空 Topic、非空 Message Type、Header 白名单和载荷可序列化性，并把载荷序列化为版本化 JSON。Version 默认是 `1`；显式 `WithVersion` 必须传正整数。Topic 表示投递目的地，Message Type + Version 表示独立于传输的业务消息契约。重试和重启必须复用同一个 Envelope 与 `messageId`，不得在每次发布时重新生成 ID。`Payload` 和 `Headers` Getter 返回防御性副本，调用方不能改写 Envelope。Header 只允许 Trace、Content Type 等传输元数据，不得放 Token、密码或数据库连接信息。

Message Key 只作为 Publisher 的路由或 Broker 分区键，不是消费幂等键。框架不提供全局或按 Key 顺序保证；具体 Adapter 即使支持分区内有序，业务仍必须按 at-least-once 和可重复、可乱序消息设计。

`Publisher.Publish` 成功只表示目标 Broker 或外部系统已持久接受消息，不表示消费者已经完成；仅写入进程内队列、EventEmitter 或异步发送缓冲区不能返回成功。Publisher 必须原样传递 `messageId`、Message Type 和 Version。业务模块只注入 `Enqueuer`；`Restore`、Publisher、Subscription 和 Consumer Adapter 属于基础设施边界，`cool check` 对业务代码直接依赖它们或在数据库事务内调用已知 Broker Client 报错。

可靠 Consumer 在 `modules/<module>/consumer/**` 中提供返回 `ConsumerDefinition` 或 `(ConsumerDefinition, error)` 的顶层构造函数，依赖继续通过构造器参数注入。Consumer Name 全局唯一且部署后保持稳定；Topic、Message Type 和 Supported Versions 必须是生成期可确定值。`supportedVersions` 非空、去重且全部为正整数；所有 Consumer 使用 Outbox 配置的 Database Group。同一 Definition 的所有支持版本都解码为 `T`；不兼容的数据结构必须拆成新的 Message Type 或显式兼容 DTO。

生成器把任一组件构造器直接依赖 `Enqueuer` 视为 Outbox Producer；不通过调用图猜测间接使用。一个应用图只提供一个 Publisher，多个 Broker 或外部目标由该 Publisher 内部按 Topic 路由，业务模块不能依赖具体 Adapter。构造器依赖 Enqueuer 却没有可用 Store/Publisher，或者 Consumer Definition 缺少可靠 Adapter 时，生成失败或启动失败。

Consumer Handler 返回普通错误表示临时失败，需要 Broker 重投；`Permanent(err)` 表示确定无法通过重试恢复，直接进入持久 DLQ。非法 Envelope、不支持的 Message Type/Version 和确定的反序列化错误由 Adapter 自动分类为永久错误，不重复消耗重试次数。

生成图把 Consumer Definition 编译成不可变 Subscription，并向唯一 Consumer Adapter 注入 Subscription 与框架 DeliverFunc。Adapter 为每条 Broker Delivery 保留其对应的 Subscription，先从 Broker 的独立 Message ID、Header 和 Body 恢复 Envelope；恢复失败直接 DLQ，恢复成功后再以 Broker 原生持久元数据或 Adapter 持久存储取得并原子推进从 `1` 开始的当前 Attempt，把该 Subscription、Envelope 和当前 Attempt 显式传给 DeliverFunc。DeliverFunc 以 Subscription 唯一确定 Consumer Name 和 Handler，完成版本路由、Inbox 事务及 Handler 调用，并根据当前 Attempt 返回 Ack/Retry/DeadLetter 决策及 Retry Delay；Broker Ack/Nack/DLQ 操作只能由 Adapter 在 DeliverFunc 返回后执行。Ack 决策只有在 Broker 持久确认 Ack 后才算完成；Retry 必须先持久安排带 Delay 的重投；DeadLetter 必须先持久写入 DLQ，再按 Broker 原子转移能力或确认顺序 Ack 原消息。安排重投、写 DLQ 或最终 Ack 失败时，原消息必须保持未确认并可再次交付，不能仅记录日志后丢弃。禁止通过 Context 隐藏 Subscription 或 Attempt，也不能仅凭 Topic/Message Type 猜测 Consumer，因为多个稳定 Consumer 可以订阅同一消息契约。

Adapter `Prepare` 必须验证订阅、连接、Broker 拓扑和 Capabilities。布尔 Capability 恰好是 `DurableAck`、`DurableRetryAttempts`、`DelayedRetry`、`DeadLetter`、`PreservesMessageID` 五项，必须全部为 `true`；`MaxEnvelopeBytes` 是独立的正数容量上限，不计入五项布尔能力。上述条件和目标大小上限全部满足后才能启动。`Start` 在消费循环真正运行后返回只报告不可恢复终止的 channel，普通 Handler 错误不得写入该 channel；`Stop` 先停止拉取，再等待在途 DeliverFunc 和 Ack/Nack/DLQ 完成。Host 对该 channel 的监督、启动回滚和错误聚合与 Transport 相同。

### 11.3 原子入队与存储模型

`Enqueue` 遵守以下事务规则：

`cool.outbox.databaseGroup` 选中的数据库连接组统一称为 **Framework Database Group**，不再为 Outbox 另造第二个数据库组术语。

1. 当前 Context 带有 `dbtx` Scope 时，Outbox 记录必须通过同一个 `gdb.TX` 写入，并且 Scope 组必须等于 Framework Database Group，与业务 DML 一起提交或回滚。CRUD Dispatcher、Inbox Handler、自定义事务 Route 和任务使用相同规则。
2. 当前没有业务事务时，Enqueuer 在 Framework Database Group 开启一个短数据库事务，只写入 Outbox 记录并提交；提交成功才表示框架接管了后续投递责任。
3. `Enqueue` 只写数据库，绝不在请求 goroutine 内同步调用 Broker。插入失败向调用方返回错误，并使所在业务事务失败。
4. Outbox 表必须和需要原子提交的业务表位于同一数据库及事务域。跨数据库、跨连接组或写库到 Broker 的原子性不受支持；检测到当前 Scope 与 Outbox Store 不同组时立即返回配置错误。

新记录的初始状态固定为 `pending`、`attempts = 0`、`availableAt = 数据库当前时间`，三个 Lease 字段、`sentAt` 和 `lastError` 均为 NULL；`createTime/updateTime` 使用数据库当前时间。Enqueue 不接受调用方指定状态、Attempt、Lease 或时间戳。

框架 Schema 管理以下内部表；业务不得直接读写：

```text
cool_outbox
  messageId         primary key, canonical UUIDv7 text(36)
  topic             message destination
  messageType       business message contract
  messageVersion    positive integer
  messageKey        nullable broker routing/partition key
  payload           versioned serialized body
  headers           serialized transport metadata
  status            pending | retry | leased | sent | dead
  attempts          publish attempt count
  availableAt       earliest next claim time
  leaseOwner        nullable worker instance ID
  claimToken        nullable unique token for this claim
  leaseExpiresAt    nullable lease deadline
  lastError         nullable sanitized failure summary
  createTime / updateTime / sentAt

cool_inbox
  consumer          consumer name
  messageId         canonical UUIDv7 text(36)
  processedAt       database time when this transaction inserted the Inbox marker
  primary key (consumer, messageId)
```

Message ID 的逻辑类型固定为 ASCII `varchar(36)` 并使用大小写敏感比较；MySQL 使用 ASCII binary collation，PostgreSQL 使用 `varchar(36)`，SQLite 使用 TEXT 加长度/格式检查。Payload 和 Header 的数据库类型由 `db/driver` 映射，但三种数据库保持同一逻辑 Schema 和状态语义。`lastError` 只能保存脱敏摘要，完整错误进入受控日志和 Trace。`processedAt` 在 Handler 之前随 Inbox 行一起写入，失败事务会回滚该行；它只表示成功事务开始处理该消息时的数据库时间，提交后才对其他事务可见，不宣称等于无法预先取得的精确 Commit Timestamp。

内部索引固定为：

```text
idx_cool_outbox_available (status, availableAt, createTime, messageId)
idx_cool_outbox_lease     (status, leaseExpiresAt, messageId)
idx_cool_outbox_sent      (status, sentAt, messageId)
```

`cool-next/outbox/store` 拥有 Enqueue、Claim、Renew、MarkSent、MarkRetry、MarkDead、ReplayDead 和 Inbox InsertIfAbsent 等 DML 契约，并分别实现 MySQL 8.x、PostgreSQL 和 SQLite 的并发语句。Store 使用自身私有的持久化 Record，不 import 上层 `outbox` 包；上层负责在不可变 Envelope 与 Store Record 之间转换，避免父子包循环依赖。`db/driver` 只根据相同逻辑 Schema 生成内部表和索引的 DDL，不执行这些数据库特定 DML。

Schema 模式同样约束内部表：`sync` 创建或安全补充表和索引，`validate` 完整比较后启动。`off` 不做同步，也不比较注释等非运行属性；但只要启用 Producer 或 Consumer，Store 就必须在 Worker/Consumer 启动前执行运行契约探测：确认对应内部表及本节列全部存在；确认 `cool_outbox(messageId)` 和 `cool_inbox(consumer, messageId)` 的主键列与顺序；确认三个固定 Outbox 索引均存在且列顺序分别精确为 `(status, availableAt, createTime, messageId)`、`(status, leaseExpiresAt, messageId)`、`(status, sentAt, messageId)`；并实际验证数据库版本、事务、唯一约束、条件写入及领取算法所需能力。MySQL 8.x 探测还必须确认两个内部表及所有参与同事务的生成业务表实际使用 InnoDB。需要执行 DML 的能力自检固定使用 `consumer = __cool_probe_consumer__`、`topic = __cool_probe_topic__`、`messageType = cool.internal.probe`、JSON Payload `{}` 和每次新生成的合法 UUIDv7 `messageId`；`__cool_` 前缀保留给框架，用户声明的 Consumer、Topic 和 Message Type 不得使用。探测必须在独立事务中执行并无条件回滚，即使某一步已成功或后续失败也不得提交；探测路径禁止调用 Publisher，不得遗留 Outbox/Inbox 记录。索引名称可以按数据库标识符规则规范化后比较，但不能接受只含相同列集合、顺序不同或多列/少列的近似索引。生产使用 `off` 时，外部 migration 必须先创建本节规定的完整结构；任一探测失败都禁止 Ready，不能推迟到首条消息才暴露。

`cool_inbox` 默认不自动删除。只有明确知道 Broker 最大保留期、最大重投期和人工重放窗口时，才能配置更长的 Inbox 归档/保留策略；删除去重键意味着对应旧消息再次到达时可能重新执行业务，不能复用 Outbox `sent` 记录的 Retention。

### 11.4 Worker、重试与崩溃恢复

Worker 按 `availableAt, createTime, messageId` 稳定排序并批量领取 `pending/retry` 且 `availableAt <= now` 的记录，也按 `leaseExpiresAt, messageId` 重新领取 Lease 已过期的 `leased` 记录。两类候选使用各自索引和有界子批次；两类同时非空时必须轮换优先级或各保留至少一个领取名额，一类不足时另一类可以使用剩余名额，保证持续新流量和持续 Lease 恢复都不会让另一类永久饥饿。领取在短事务内完成，随后释放数据库事务，再调用 Publisher；不得在网络发布期间持有行锁。

- MySQL 8.x 和 PostgreSQL 9.5+ 在 Framework Database Group 的 `gdb.DB` 上调用 `db.TransactionWithOptions(ctx, gdb.TxOptions{Isolation: sql.LevelReadCommitted}, ...)` 开启短领取事务，在 `READ COMMITTED` 下通过行锁与 `SKIP LOCKED` 领取批次，再原子写入 `leased`、Worker Instance ID、唯一 Claim Token 和 `leaseExpiresAt`；`sql.LevelReadCommitted` 来自标准库 `database/sql`，不得依赖连接组可能不同的默认隔离级别。
- SQLite 不依赖 `SKIP LOCKED`：先按本节稳定顺序读取有界候选，再为每个候选生成唯一 Claim Token，并以读取到的旧 `status`、时间资格条件、Lease 条件和 `messageId` 执行条件 `UPDATE`。只有 `RowsAffected == 1` 的 Worker 获胜；`0` 表示并发竞争失败并跳过或继续下一候选，其他结果视为存储不变量错误。Claim 只返回随后以本次新 Claim Token 回读到的记录，禁止把初始候选查询结果直接当作已取得 Lease。
- `availableAt`、`leaseExpiresAt` 和 Lease 过期判断统一使用数据库当前时间，不使用各 Worker 的本地墙上时钟；退避算法只计算持续时间，再由数据库时间生成下一可用时刻。
- 发布超时必须小于 Lease Duration；长发布由 Worker 续租。`Renew` 只能更新 `status = leased` 且匹配当前 Claim Token 的行，并把 `leaseExpiresAt` 严格推进到晚于原 Deadline 的数据库时间；正数 Lease Duration 和严格推进保证成功续租在 MySQL 8.x 的 changed-row 语义下也影响一行，不依赖 `CLIENT_FOUND_ROWS`。
- 从 `pending/retry` 成功领取时原子增加 `attempts`。重新领取 Lease 已过期的 `leased` 记录时生成新的 Claim Token、Owner 和 Deadline，但保留 `attempts`。每次 Claim 返回的 Record 必须包含数据库已写入的 Attempt 和 Claim Token，Worker 不得自行推测。
- 发布成功执行 `MarkSent`：仅把 `status = leased` 且匹配当前 Claim Token 的记录改为 `sent`，以数据库时间写入 `sentAt`，清空 `leaseOwner/claimToken/leaseExpiresAt` 和 `lastError`。Publisher 明确失败且尚未达到上限时执行 `MarkRetry`：转为 `retry`，用数据库时间加退避时长写入 `availableAt`，保存脱敏错误摘要，并清空三个 Lease 字段。达到上限时执行 `MarkDead`：转为 `dead`、保存脱敏错误摘要并清空三个 Lease 字段；终态不再可领取。
- `Renew/MarkSent/MarkRetry/MarkDead` 都必须以 `messageId + status = leased + claimToken` 作条件，并要求受影响行数恰好为 `1`。`0` 表示 Claim 已丢失，Store 返回可识别的 `ErrClaimLost`，Worker 只记录指标并停止操作该 Record；不得用无 Token 的更新补偿，也不得把该 Record 视为已完成。大于 `1` 表示存储不变量损坏并使 Worker 失败。这样旧 Worker 无法覆盖续租失败后被重新领取的新 Lease。
- Lease 到期表示上一个 Worker 的发布结果未知。重新领取该 `leased` 记录时保留当前 Attempt 并再次发布，即使它已等于 `publishMaxAttempts` 也不能直接转死信；只有 Publisher 明确返回失败且当前 Attempt 已达到上限时才转为 `dead`。这样进程在领取后、调用 Publisher 前崩溃不会造成永久丢失。
- `dead` 记录不删除、不自动重放，并产生结构化错误日志和指标；运维通过受控命令检查原因、修正后把原记录从 `dead` 原子转回 `retry`，把 Attempt 重置为 `0`、`availableAt` 设为数据库当前时间并清空 `leaseOwner/claimToken/leaseExpiresAt/sentAt/lastError`，同时保留原 `messageId`、Payload、类型与版本。
- `sent` 记录按 Retention 定期清理；`dead` 记录不参加普通 Retention，必须通过明确的归档或删除操作处理。

不可消除的崩溃窗口是：Broker 已接受消息，但进程在把 Outbox 标记为 `sent` 前退出。Lease 到期后消息会再次发布，因此重复是协议的一部分；不能通过“先标记 sent 再发布”换取表面去重，否则会产生永久丢失。

至少按 Topic 暴露以下可观测信息：各状态数量、最老待发消息年龄、领取/发布延迟、重试与死信计数、Lease 过期恢复数，并在日志和 Trace 中关联 Database Group、`messageId`、Topic、Attempt 和 Worker ID。Payload 默认不进入日志。

### 11.5 Inbox 幂等消费

每个可靠 Consumer 必须声明稳定且唯一的 Consumer Name，并按以下顺序处理：

1. 校验 Envelope、类型、版本和 RFC 9562 UUIDv7 `messageId`，通过 `dbtx.Runner` 在 Framework Database Group 开启事务；该事务 Scope 必须同时供 Inbox Store、Handler 的 `Base.Model/Tx` 和 Handler 内的 Enqueuer 使用。
2. 通过 `outbox/store` 的原子 InsertIfAbsent 写入 `(consumer, messageId)` 并取得是否插入。三库判定算法固定如下，未插入表示此前已提交，跳过 Handler，提交事务并向 Broker Ack。

   - MySQL 8.x 执行普通 `INSERT`。成功表示首次；只通过结构化 Driver Error 的 MySQL Code `1062` 判定重复，InnoDB 只回滚该语句且事务可继续；其他错误全部返回并回滚。禁止使用 `ON DUPLICATE KEY UPDATE`、`INSERT IGNORE` 或受 `CLIENT_FOUND_ROWS` 影响的 RowsAffected 判定。
   - PostgreSQL 执行 `INSERT ... ON CONFLICT ("consumer", "messageId") DO NOTHING RETURNING "messageId"`；冲突目标必须显式匹配 Inbox 复合主键。返回一行表示首次，`sql.ErrNoRows` 表示重复。所有 camelCase 列都按 PostgreSQL 方言加双引号，不得先触发唯一键异常再继续，因为 PostgreSQL 会把事务标记为失败。
   - SQLite 执行 `INSERT ... ON CONFLICT DO NOTHING` 并读取该语句的 RowsAffected；`1` 表示首次，`0` 表示重复，其他结果或错误均回滚。
3. 首次消息在同一事务内执行 Handler 及其业务 DML。Handler 失败则回滚 Inbox 和业务 DML，不 Ack，由 Broker 重投。
4. 本地事务提交成功后才向 Broker Ack；若 Ack 丢失，重投会由 Inbox 唯一键安全跳过。
5. Handler 产生新的可靠外部副作用时，只能在当前事务内再写 Outbox，形成 Inbox + 业务 DML + 新 Outbox 的同事务提交。

Inbox 只能保护与它位于同一数据库事务中的本地业务效果。Consumer 直接调用外部系统仍可能重复，必须改为 Outbox 级联或使用目标系统支持的 `messageId` 幂等键。无法提供稳定 Message ID 的来源不能注册为可靠 Consumer。

Consumer Adapter 必须使用支持持久 Ack/Nack、重投和 DLQ 的 Broker 契约。临时 Handler 错误执行 Nack，达到消费重试上限后进入持久 DLQ；`Permanent`、非法 Envelope、不支持的类型/版本和确定的反序列化错误直接进入 DLQ。所有路径保持 Envelope 和 `messageId` 不变，并暴露分类失败计数和人工重放入口。只提供进程内 EventEmitter、无持久重试或无法保留 `messageId` 的实现不能声明为可靠 Consumer。

消费 Attempt 必须存放在 Broker 原生持久元数据或 Adapter 的持久存储中，键至少隔离 Consumer Name 与 `messageId`，进程重启不能归零。Adapter 必须在调用 DeliverFunc 前持久取得并推进 Attempt；首次为 `1`，Broker 重投或处理超时后的下一次为前值加一，传入值不得由进程内计数器推测。DeliverFunc 对每次临时失败按 `consumerRetryBase * 2^(attempt-1)` 计算并限制到 `consumerRetryMax`，加入随机抖动；当前 Attempt 达到 `consumerMaxAttempts` 时返回 DeadLetter 而不是 Retry。每次 DeliverFunc 使用独立 `consumerTimeout` Context；Adapter 必须保证 Broker Visibility/Ack Deadline 大于该 Timeout，或在处理期间可靠续期。Ack 或成功转入 DLQ 后，Adapter 才能按其 Broker/持久存储协议清理 Attempt 元数据。

### 11.6 运维检查与重放

Outbox 运维只通过 `cool` CLI 进入 Store 的受控接口，首期不暴露 HTTP 管理路由，也禁止运维直接修改内部表：

```text
cool outbox list --status dead [--topic <topic>] [--limit <n>]
cool outbox show --message-id <uuidv7>
cool outbox replay --message-id <uuidv7> --operator <name> --reason <text> [--dry-run]
```

- 三个命令使用应用的正常配置发现与 `cool.outbox.databaseGroup`，不接受命令行数据库连接串、密码或临时换组参数。
- `list/show` 默认只显示 Message ID、Topic、Message Type、Version、Status、Attempt、时间和脱敏错误摘要；Payload、Header、Token 和连接信息不输出。`limit` 必须为正数且有硬上限。
- `replay --dry-run` 只校验目标和展示将发生的状态变化。实际重放必须提供非空 Operator 和 Reason，并在结构化安全日志中记录 Trace/Operation ID、Operator、Reason、Message ID、旧状态和结果，仍不得记录 Payload。
- Store 在 Framework Database Group 的短事务中锁定目标，只允许 `status = dead` 的同一个 `messageId` 原子转为 `retry`；更新字段严格采用 11.4 的重置规则并要求受影响行数恰好为 `1`。目标不存在、状态已变化或并发操作丢失时返回非零退出码，不得覆盖当前状态。
- CLI 不直接调用 Publisher，也不新建 Envelope 或 Message ID。事务提交后由普通 Worker 再次领取，因此停机、并发重放和发布失败继续服从同一 Lease/Claim Token 协议。
- `sent` 不允许 replay；确需业务补偿时创建一条新的业务消息，不能把已经确认成功的原记录改回待发送。删除或归档 `dead` 记录不属于 replay，首期不提供批量删除命令。

CLI 的 `cool outbox replay` 是运维层命令名；它在完成权限、参数、审计和 dry-run 检查后调用 Store 的状态专用方法 `ReplayDead`。两者分别描述外部命令与内部 DML，不要求同名。

Consumer DLQ 属于 Broker Adapter 的运维面，不复用 Outbox 表或上述命令。可靠 Adapter 必须提供按稳定 Consumer Name + `messageId` 检查和重放的管理能力；重放保持原 Envelope、Message ID 和持久 Attempt 历史，并记录 Operator/Reason。Broker 不具备这一能力时，`Prepare` 必须拒绝可靠 Consumer 启动。

### 11.7 配置、生成检查与生命周期

默认配置结构为：

```yaml
cool:
   outbox:
      enabled: true
      databaseGroup: default
      pollInterval: 500ms
      batchSize: 100
      leaseDuration: 30s
      publishTimeout: 10s
      publishMaxAttempts: 12
      publishRetryBase: 1s
      publishRetryMax: 5m
      consumerTimeout: 30s
      consumerMaxAttempts: 12
      consumerRetryBase: 1s
      consumerRetryMax: 5m
      retention: 168h
      maxPayloadBytes: 1048576
      maxHeaderBytes: 16384
```

`databaseGroup` 必须非空并存在于 GoFrame 数据库配置。`cool_outbox/cool_inbox`、Store、Worker 和所有可靠 Consumer 都固定使用该组；消息 ID 保持全局唯一。其他连接组可以执行普通业务事务，但不能与本 Outbox 原子提交，尝试在不同组的 `dbtx` Scope 中 Enqueue 会立即失败。该组只是数据库事务边界，不引入租户字段、租户 Scope 或自动租户过滤。

配置校验还要求 `publishTimeout < leaseDuration`，发布/消费重试、Timeout、批量和大小参数为正数，Retry Base 不大于对应 Retry Max。`New` 负责序列化，Enqueuer 在写库前按配置校验 Payload、Header 和完整 Envelope 大小；Publisher 或 Consumer Adapter 的目标上限更小时必须在启动阶段拒绝不兼容配置。生成图中存在 Producer 时，Outbox 被禁用、Store/Publisher/Worker 缺失或 Schema 不可用都会使应用启动失败；存在可靠 Consumer 时，Inbox Store、Consumer Adapter、全部 Capabilities、稳定名称或 Schema 任一缺失同样启动失败。不能静默降级为进程内事件或同步发布。完全没有 Producer/Consumer 的应用可以显式关闭 Outbox。

Outbox Store、Publisher、Worker 和 Consumer Adapter 都参与 Application Host 的拓扑与生命周期。Worker 在数据库/Schema 和 Publisher 就绪后启动；停机时停止领取、等待在途发布，超时则保留 Lease 供到期恢复。Consumer 停止拉取后等待在途事务完成，再关闭 Broker 连接。所有等待受 Host 全局关闭超时约束。

## 12. EPS、OpenAPI 与安全边界

EPS 从相同 Descriptor 和静态路由表生成，至少包含：

- 模块、Controller、路由、HTTP 方法和描述；
- 实体名、字段名、Go/JSON/数据库类型映射；
- 字段描述、可空、默认值、长度和读写属性；
- CRUD API、查询字段、允许排序字段；
- 请求与响应 Schema。

`description` 是表、字段和 API 文档的统一中文描述来源。业务不得另建 EPS 专用字段描述文件。

安全要求：

- 未知请求字段默认拒绝。
- 默认 Base 和 Base 委托路径中，客户端 ID、隐藏字段和只读字段绝不能进入 DML：Add 对 ID 和只读字段先按来源静默剔除，Update 对只读字段以及所有动作对隐藏字段直接拒绝。纯 override 是显式可信边界，业务实现必须自行维持同等不变量；`cool check` 对可静态识别的原始请求直写和裸 SQL 直接报错。
- 动态排序、选择、Join 和导出字段必须通过白名单。
- 所有查询值参数化；结构化条件中的列名来自 Descriptor。
- 请求体、Token、密码、数据库连接和 Session 内容按日志策略脱敏。
- 默认限制 Page Size、List Size、Batch Size 和 Export Size，并允许全局配置收紧。

## 13. 测试与验收

### 13.1 单元测试

- 实体标签解析、字段描述传播、首期 Index/Unique Index Schema 合并和冲突检测；
- 模块配置按“默认值 -> 主配置 -> 环境变量”做 presence-aware 合并：object/map 递归合并、scalar 替换、slice/array 整体替换、仅 nullable 字段接受显式 `null`、环境变量只覆盖目标叶子，并对未知字段和类型错误失败；
- 未提供/零值/`false`/显式 `null` 的 DOValue 语义；
- Binder、gRPC、任务和 Service 调用共用 smart constructor，并保持单条/数组形状；
- `CurdOption`、静态/动态 `QueryOp`、FieldEq 数组到 `IN`；
- 排序顺序、字段白名单和查询参数化；
- `NativeQuery` 的只读、常量 SQL、参数绑定、无 Scope 使用 Framework Database Group、同组 TX 复用和跨组拒绝；
- Service 单动作重写、显式调用 Base、Hook 恰好一次；
- 默认 Base / Base 委托路径的 `Before/InsertParam` 时序、纯 override 跳过、显式 Base 恢复全部对应增强；
- Exception 的 HTTP/gRPC 映射；
- 普通未知错误的安全 `1001 + HTTP 500` 与 Redis Core `1003` 区分；
- `auth.Admin(ctx)` / `auth.App(ctx)` 在身份正确时返回完整不可变 Identity，在缺失或 Subject 不匹配时返回错误且不泄露另一类身份；修改 `RoleIDs()` 的返回副本不能改变后续读取到的 Identity；
- Redis Session 按版本化 JSON 精确编解码，跨语言往返不损失 uint64 精度；Key/Session ID 不一致、两个 JTI 或其他必填字段缺失、Subject 字段集不合法、未知 `schemaVersion` 和已过期值均失败关闭，未知普通字段可忽略；Admin 只提供旧 `passwordVersion` 而缺少必填 `passwordV` 时必须失败，不启用别名兼容；写入 TTL 不得晚于 `expiresAt`；
- JWT 固定 HS256、强制 `kid/sessionId/jti/subject/isRefresh/iss/aud/iat/nbf/exp`，拒绝错误算法、未知 Key、错误 Issuer/Audience、缺失 Claim、过期和 Access/Refresh 混用；当前签发 Key 与旧验证 Key 的轮换不改变已有 Token 的验证边界；
- `RotateRefresh` 原子比较旧 Refresh JTI 并同时轮换 Access/Refresh JTI；同一 Refresh Token 并发刷新最多一次成功，旧 Token 重放会撤销 Session；
- `SessionStore` 只能由鉴权边界使用，业务模块不能将它或其 Redis 实现当作通用 Cache 依赖；相应生成图/边界检查必须拒绝这种泄漏；
- 生命周期拓扑、逆序停止和失败回滚；
- `dbtx.Runner` 新事务提交/回滚、同组复用、内层失败 rollback-only、跨组拒绝、Panic 回滚和 Context 禁止逸出；`Base.Model/Tx` 在当前 Scope 组不匹配时返回配置错误且不退回全局 DB；
- `cool.crud.softDelete` 省略时默认 `true`、显式 `false` 关闭归档、非法类型启动失败；回收快照确定性编码和强类型恢复不损失 uint64、时间、零值或 null；
- Outbox Envelope 校验、版本化序列化、状态转换、指数退避和抖动边界；UUIDv7 验证小写 36 字节标准格式、版本/variant 位、并发唯一性、密码学随机源失败以及时钟回拨时仍合法且不重复；
- Claim Record 的 Attempt/Token 来源、所有 Token 条件更新恰好影响一行、`ErrClaimLost`、终态/重试态 Lease 字段清理和人工重放字段重置；
- Consumer Definition 的稳定名称、支持版本和临时/永久错误分类；Consumer Adapter 的五项布尔 Capability、独立正数 `MaxEnvelopeBytes`、Subscription 显式路由、持久 Attempt 传递、Prepare/Start/Stop 时序、不可恢复终止 channel 监督和启动失败回滚；
- Inbox Consumer Name 和 `(consumer, messageId)` 幂等键校验。

新增测试名称必须表达同一个 `Add` 的两种输入，不创造第二个公开动作：

```text
Add single object
Add top-level array
Add array ID order
Add array transaction rollback
InsertParam per array item
hooks once per whole batch
```

### 13.2 生成器测试

- AST/类型分析 golden tests；
- 缺失依赖、重复提供、循环依赖、路由冲突和非法标签错误；
- 单一生成文件快照；
- 输出可重复；
- 生成结果 `go test`/type-check；
- 失败时旧文件不变的原子性测试；
- 无数据库环境下生成成功；
- 每个持久化字段缺失/空 `orm` 标签时生成失败；数据库列名只接受 lowerCamelCase，显式 snake_case `orm` 列名生成失败；框架识别的 Meta/嵌入字段豁免，表名和索引名仍允许 snake_case；
- 当前 Schema 只生成 Index/Unique Index；外键、关系、检查约束和表级选项在专项设计批准前不得接受或生成；
- Enqueuer 构造器依赖识别 Producer，Consumer Definition 静态注册及重复名称/非法版本错误；
- Event/Schedule/Queue 专项设计批准前，生成器只保留组件类别边界，不接受或生成自行猜测的 Handler 签名；
- 自定义 Route 默认使用唯一 Framework Database Group、`NonTransactional` 显式退出事务，且不存在 Route 级换组 API；
- 缺失 Store/Publisher/Worker/Consumer Adapter/DLQ 能力和禁用 Outbox 的图一致性错误；
- `cool check` 覆盖的生成稳定性、图、冲突、元数据和边界规则，以及它不会隐式取代 `gofmt`、`go vet`、`go test` 或 `go test -race` 的 CI 契约。

### 13.3 数据库集成测试

MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+ 使用同一套行为用例：

- Add/Delete/Update/Info/List/Page；
- 单个与批量事务；
- 时间字段自动维护；
- nullable、默认值、索引和注释能力矩阵；
- 三种数据库实际列名均保持 `createTime/updateTime/messageId` 等 lowerCamelCase；PostgreSQL DDL/DML 正确引用大小写，`messageid` 或 snake_case 列不能通过 Schema 校验；
- `sync/validate/off`；
- `softDelete=true` 时单条/批量删除先写一条回收记录再硬删除，归档失败、删除行数冲突或 Hook 失败均整体回滚；`false` 时只硬删除；不存在的 ID 不产生空回收记录；
- 回收恢复全部成功后才删除回收记录，主键/唯一键/外键/字段冲突、并发恢复或任一行失败时整体回滚并保留原记录；
- 并发更新/删除下，MySQL 和 PostgreSQL 的行锁以及 SQLite 的写事务冲突处理都保证归档快照等于实际删除内容，不产生旧快照或重复成功；
- MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+ 的版本/能力探测，不满足基线时启动失败；
- MySQL 默认引擎、所有参与框架事务的业务表及 Recycle/Outbox/Inbox 内部表均为 InnoDB；任一实际表为 MyISAM 或其他非事务引擎时启动失败，`sync/validate/off` 三种模式都不能跳过已启用基础设施的验证；
- `sync/validate/off` 对 Recycle/Outbox/Inbox 内部表的对应行为；启用相应能力后，`off` 缺列、主键错误、任一固定索引缺失、版本或必要能力不足时均启动失败，Outbox/Inbox 还要求列序精确；
- 业务 DML 回滚时没有 Outbox 记录，提交时业务 DML 和 Outbox 记录同时可见；
- Outbox 与业务事务使用不同数据库连接组时拒绝执行；
- 独立 Enqueue 使用 Framework Database Group，不同组的事务 Scope 拒绝执行；
- `Base.Model/Tx` 和 Enqueuer 在 Scope 组不匹配时采用相同的拒绝语义，不得悄悄使用默认连接；
- 自定义事务 Route 和任务中的业务 DML 与 Outbox 同事务提交/回滚；
- Outbox 新记录初始状态、批次并发领取、`pending/retry` 与过期 Lease 两类候选的公平领取、Claim Token 条件更新恰好影响一行、旧 Token 返回 `ErrClaimLost`、失败重试、指数退避、死信、各转换清理 Lease 和 Lease 到期恢复；
- Outbox 固定索引、稳定领取顺序、Payload/Header 大小限制和旧消息不饥饿；
- `cool outbox list/show` 只输出元数据和脱敏错误；`replay --dry-run` 不修改数据，实际重放强制 Operator/Reason，只允许单条 `dead -> retry`、受影响行数恰好为一，并保留原 Envelope 和 `messageId`；目标不存在、错误状态、`sent` 或并发重放竞争均失败且不直接调用 Publisher；
- 模拟 Broker 发布成功、`sent` 更新前崩溃产生重复投递，Inbox 保证同一 `messageId` 只产生一次业务效果；
- Inbox、Consumer 通过 `Base.Model/Tx` 执行的业务 DML 和级联 Outbox 共用同一个 `dbtx` Scope，Handler 失败时一起回滚；
- MySQL 普通 `INSERT` 只按结构化错误码 `1062` 判定 Inbox 重复，结果不受 `CLIENT_FOUND_ROWS` 开关影响；PostgreSQL 验证 `ON CONFLICT ("consumer", "messageId") DO NOTHING RETURNING "messageId"`，SQLite 验证 `ON CONFLICT DO NOTHING + RowsAffected` 的首次/重复路径；
- Consumer Adapter 缺任一持久 Ack/Attempt/延迟重试/DLQ/Message ID 能力时拒绝启动；同一 Topic/Message Type 的多个 Subscription 按各自稳定 Consumer Name 隔离 Inbox 和 Attempt；Retry 安排、DLQ 写入或 Ack 失败时原消息保持未确认并重投；正常 Stop 排空在途交付，不可恢复循环错误触发 Host 监督；
- Consumer Adapter 能按稳定 Consumer Name + `messageId` 检查和受控重放 DLQ，保留原 Envelope、Message ID 和持久 Attempt 历史并记录 Operator/Reason；无该能力时 `Prepare` 失败；
- 不支持版本和 `Permanent` 直接进入 DLQ；临时错误的持久 Attempt 跨进程重启不归零，按退避重投，`consumerTimeout` 与 Visibility/Ack Deadline 续期满足约束，达到上限后进入 DLQ，所有重放保留原 `messageId`。

Outbox/Inbox 的数据库集成用例必须在 MySQL 8.x、PostgreSQL 9.5+ 和 SQLite 3.24+ 全部运行。MySQL/PostgreSQL 验证 `READ COMMITTED + SKIP LOCKED` 批次领取；SQLite 使用至少两个并发 Worker 验证候选读取、条件更新、`RowsAffected == 1` 和 Claim Token 回读共同形成等价的单 Lease 效果，并验证竞争失败者跳过或重试时不会发布未取得的记录。

### 13.4 Transport 契约测试

- HTTP `{code,message,data}` 与 Node 兼容；
- HTTP 默认异常状态及显式 401/403；
- gRPC Code 与 Error Details；
- HTTP/gRPC Identity 和取消传播一致；
- HTTP/gRPC 的 Prepare 端口绑定、gRPC 与其他可观察终止 channel 的 Serve 错误上报、Ready 撤销和双 Transport 回滚；
- HTTP Prepared Listener 回滚、Start 失败关闭和 Running 状态等待超时；
- HTTP 预绑定 Listener 后才调用 `ghttp.Server.Start()`；gRPC Adapter 不调用 `grpcx.GrpcServer.Run/Start`；
- 锁定 GoFrame 版本的 HTTP `Start()`、无参 `Shutdown()` 和 Fatal 行为；验证 `Stop(ctx)` 在监督 goroutine 外部执行超时并始终关闭自持 Listener；用子进程触发非正常 Serve 错误时必须非零退出，生产部署检查必须要求监督器和健康检查，不能把 Fatal 误报成 Host 可观察终止；
- Redis 启动失败、运行时失败关闭和日志脱敏。

### 13.5 并发与模糊测试

- `go test -race ./...`；
- URL 通配、路由规范化、条件编译、字段标签解析 fuzz；
- Session 撤销、刷新和 SSO 并发；
- Admin/App 同 ID 的 Session 撤销隔离；
- 生命周期重复停止和并发关闭；
- UUIDv7 在并发生成和模拟时钟回拨下通过 race 测试，保持格式合法且无重复；
- 多 Worker 并发时同一时刻只有一个有效 Lease，旧 Claim Token 的 Renew、MarkSent、MarkRetry 和 MarkDead 均不能覆盖续租或重新领取后的状态；
- Outbox Worker 停机停止领取并排空在途发布，超时遗留 Lease 可在到期后恢复。

### 13.6 性能基线

性能验收先固定可重复场景和采样方法，不在架构阶段虚构统一 TPS：

- 单条与不同 Batch Size 的 `Add` 延迟、吞吐和内存分配，同时验证整批事务语义；
- 相同业务 DML 在不入队与同事务写入 Outbox 时的额外延迟、日志量和数据库 CPU/IO；
- 单 Worker 和多 Worker 在不同 Batch Size、发布延迟及部分失败下的 Claim/Publish 吞吐、队列时延、Lease 竞争和重试成本；
- 多实例对相同 `consumer + messageId` 并发投递时的 Inbox 去重吞吐、锁竞争和业务效果唯一性；
- 每个场景报告 p50/p95/p99、吞吐、错误率和 allocations/op，并记录 Go/GoFrame、数据库、Driver、Broker、硬件、并发度、数据规模与配置；
- 实施计划在首个可运行版本上冻结基准值和可接受回归阈值；后续 CI 使用同一环境和数据集比较，不将不同机器的绝对数字直接当作回归结论。

## 14. 必须保持的不变量

1. 新框架完全重写，v1 只能作为行为和测试参考。
2. 对外 CRUD 名称固定为 `Add/Delete/Update/Info/List/Page`。
3. `Add` 自身同时接收单对象和顶层数组，并按输入形状返回 ID。
4. Service 重写后，Base 增强不会隐式生效；需要时显式调用 Base。
5. CRUD Dispatcher 的事务始终生效，Add/Delete/Update 的 `ModifyBefore/ModifyAfter` 始终各执行一次；`NonTransactional` 只允许显式声明的自定义 Route 退出其默认 Route 事务，不适用于默认 CRUD Dispatcher。
6. 业务实体是字段事实来源，`description` 必须贯穿 DB、EPS、OpenAPI 和生成元数据。
7. DML 使用 GoFrame ORM 与结构化对象，不使用数据库 map 或自定义 SQL 编译器。
8. `db/driver` 只负责 MySQL/PostgreSQL/SQLite 的 DDL 差异。
9. 框架不提供多租户能力；`Base`、Identity、Session、JWT、CRUD 和查询计划均不保留 `tenantId` 或自动租户 Scope。
10. Session 默认使用 Redis，且 Redis 启用后绝不自动降级；启动连接/PING 失败只记录内部详情并终止启动，不存在接口响应，运行期 Redis 故障对外只返回安全 Core 错误。测试、本地开发或明确接受状态随进程丢失的单实例部署仍可显式配置 Memory Store，这属于主动选型而非 Redis 故障降级。
11. 模块配置由框架统一合并并用 `gvalid` 校验。
12. 依赖只从构造函数参数类型推导。
13. `cool generate` 只输出 `modules/modules_gen.go`；标准 protobuf 等工具的生成文件不受此限制。
14. HTTP、gRPC 或双 Transport 由同一 Application Host 管理，统一 Ready；启动失败和可观察运行错误由 Host 回滚，GoFrame HTTP 不可观察的 Serve 错误让整个进程非零退出并由部署监督器重启。
15. 不实现 Service 方法到 API 的动态映射；自定义接口显式注册 Route。
16. 可靠提交后副作用必须与业务 DML 在同一数据库事务写入 Outbox；业务事务内不得直接发布到 Broker。
17. Outbox 固定为 at-least-once，稳定 `messageId`、Inbox 唯一键和业务幂等共同处理重复；不承诺跨数据库、Broker 或外部系统 exactly-once。
18. CRUD Dispatcher、Inbox Consumer、自定义事务 Route 和事务任务共用 `dbtx` Scope；同组复用，跨组拒绝，`Base.Model/Tx` 与 Enqueuer 不得脱离当前 Scope。
19. 数据库支持基线固定为 MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+；`db/driver` 只负责 DDL，Outbox/Inbox 的数据库特定 DML 只属于 `outbox/store`。
20. JWT 必须携带 Session ID 和独立 JTI，Access/Refresh 都校验服务端 Session；Refresh 原子轮换且检测到旧 Token 重放时撤销 Session。
21. Event、Schedule 和 Queue 是框架层能力；当前总架构只冻结职责与可靠性边界，具体 API 必须由对应专项设计批准后才能实现。

## 15. 后续边界

本文件覆盖底层框架和通用业务模块开发契约。用户复核并批准本文后，再根据它编写分阶段实现计划；当前不进入实现。Event、Schedule 和 Queue 在实现前分别补充框架专项设计，冻结 Definition、Handler、生命周期、并发和失败语义；专项设计不得突破本文的进程内事件、Application Host 管理以及 Outbox/Inbox 可靠性边界。

具体业务模块开始设计前，必须逐个向用户确认：

- Entity 与字段；
- Service 默认 CRUD 与重写点；
- Controller 路由、`CurdOption` 和 `QueryOp`；
- 权限、事件、任务和队列行为；
- 与 Node 对应模块的兼容范围。

未经确认，不在底层框架实现阶段提前固化业务模块用法或复制 v1 业务代码。
