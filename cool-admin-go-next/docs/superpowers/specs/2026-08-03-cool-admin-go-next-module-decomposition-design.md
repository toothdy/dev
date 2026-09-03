# cool-admin-go-next 框架模块拆分设计

> 日期：2026-08-03  
> 状态：等待用户复核  
> 上位设计：`2026-07-31-cool-admin-go-next-architecture-design.md`  
> 范围：底层框架的文档边界、实施顺序、功能颗粒度和验收门禁

## 1. 目标

现有总架构文档同时覆盖实体元数据、数据库、生成器、CRUD、Controller、鉴权、Application Host 和 Outbox/Inbox。这些能力的依赖层级和验收方式不同，不适合放入一份实施计划。

本设计将底层框架拆为：

```text
阶段 -> 模块设计文档 -> 功能单元 -> 实施任务与测试
```

拆分后的每个模块必须：

1. 形成一个完整、可测试的行为闭环。
2. 只依赖编号更早或明确列出的前置模块。
3. 可以独立转换为一份实施计划。
4. 明确代码目录归属，但不为迎合目录而拆断跨包行为。
5. 在本模块的测试中完成自身验收，不把核心正确性推迟到上层集成。

## 2. 文档体系

总架构文档最终只保留背景、总体原则、依赖方向、全局不变量和模块索引。Descriptor、CRUD、JWT、Host、Outbox 等详细契约迁移到对应模块设计，避免产生两份事实来源。

每份后续模块设计文档固定包含：

- 模块目标与非目标；
- 代码目录归属；
- 前置模块；
- 对外类型和接口；
- 核心流程和错误语义；
- 配置、生成器和生命周期影响；
- 单元、集成、并发及模糊测试；
- 完成标准；
- 从上位设计迁入的章节；
- 本模块禁止提前实现的后续能力。

## 3. 总体阶段

首轮底层框架共拆分为 6 个阶段、44 个模块。编号中的空档用于保持阶段语义，不表示遗漏模块。

```text
01-09  框架地基
   |
10-15  编译期基础设施
   |
20-26  数据访问与 CRUD 内核
   |
30-36  接口与安全
   |
40-46  应用承载
   |
50-57  可靠异步能力
```

强制规则：

- 前一阶段验收门未通过时，不进入后一阶段的正式实施。
- 上层可以为下层增加已批准的生成或注册适配，不得改变下层已冻结的行为契约。
- 所有跨阶段变更必须回到对应模块设计复核，不能在实施计划中暗中扩大边界。

## 4. 第一阶段：框架地基

### 01 工程骨架与依赖边界

代码归属：根目录、`cool-next/`、CI。前置：无。

- `01.1` 初始化 Go Module、GoFrame 版本和基础依赖。
- `01.2` 建立 `cool-next`、`cmd`、`modules`、`manifest` 目录。
- `01.3` 定义框架包、生成代码和业务模块的依赖方向。
- `01.4` 检查 `cool-next/*` 禁止反向依赖 `modules/*`。
- `01.5` 建立格式化、静态检查、单测和 Race Test 基线。
- `01.6` 建立 MySQL、PostgreSQL、SQLite 集成测试环境。

### 02 核心异常模型

代码归属：`cool-next/core/exception`。前置：01。

- `02.1` 定义 `BaseException`、错误码、消息和 HTTP 状态。
- `02.2` 实现 `Comm`、`Validate`、`Core` 构造器。
- `02.3` 实现 `WrapComm`、`WrapValidate`、`WrapCore`。
- `02.4` 保留 Cause、错误链和堆栈。
- `02.5` 区分业务、校验、框架和未知错误。
- `02.6` 对未知错误输出安全消息，内部日志保留真实原因。
- `02.7` 为后续 HTTP/gRPC 映射提供协议无关分类。

### 03 配置加载与基础校验

代码归属：`cool-next/core/configuration`。前置：01、02。

- `03.1` 读取默认配置、主配置和环境变量。
- `03.2` 建立保留字段 presence 的配置树。
- `03.3` object/map 递归合并，scalar、slice、array 整体替换。
- `03.4` 处理 nullable 字段的显式 `null`。
- `03.5` 环境变量只覆盖指定叶子路径。
- `03.6` 拒绝未知字段和类型不匹配。
- `03.7` 解码强类型配置并执行 `gvalid`。
- `03.8` 生成不可变、可复现的规范化配置结果。
- `03.9` 配置错误统一转换为 Core 启动错误。

### 04 实体与 Descriptor 元数据

代码归属：`cool-next/core/entity`。前置：01、02。

- `04.1` 定义 `Base` 的 ID、创建时间和更新时间。
- `04.2` 解析 `g.Meta` 的表名和表描述。
- `04.3` 解析 `orm`、`json`、`description`、`cool` 标签。
- `04.4` 校验 lowerCamelCase 数据库列名。
- `04.5` 从 Go 类型和指针推导逻辑类型及 nullable。
- `04.6` 定义 `Field`、`Metadata`、`Descriptor`。
- `04.7` 提供字段名、JSON 名和列名索引。
- `04.8` 定义主键、自增字段和系统维护字段。
- `04.9` 定义 `IndexOf`、`UniqueIndexOf`。
- `04.10` 合并并校验实体 Schema 补充声明。
- `04.11` 保证 Descriptor 和返回集合不可变。
- `04.12` 校验表名、列名和索引的全局冲突。

### 05 DOValue 与字段四态

代码归属：`cool-next/core/entity`。前置：04。

- `05.1` 定义未提交、零值、`false`、显式 `null` 四态。
- `05.2` 定义 `DOValue` 能力接口。
- `05.3` 按逻辑字段名执行 `SetColumn`。
- `05.4` 校验字段存在性、类型和 nullable。
- `05.5` 使用 `gdb.Raw("NULL")` 表示显式空值。
- `05.6` 定义符合 GoFrame 形状的私有 DO Struct。
- `05.7` 将字段状态转换为结构化 `DBData`。
- `05.8` 禁止使用 `map[string]any` 作为数据库写入对象。
- `05.9` 验证 uint64、时间、零值和空值不失真。

### 06 数据库方言与能力探测

代码归属：`cool-next/db/driver`。前置：02、03、04。

- `06.1` 识别 MySQL、PostgreSQL、SQLite 并校验最低版本。
- `06.2` 实现方言化标识符引用。
- `06.3` 保证 PostgreSQL camelCase 列正确加双引号。
- `06.4` 建立 Go 类型到数据库类型的 DDL 映射。
- `06.5` 处理自增、默认值、nullable、精度和注释差异。
- `06.6` 生成普通索引和唯一索引 DDL。
- `06.7` 探测事务、条件写入和锁能力。
- `06.8` 校验 MySQL 表实际使用 InnoDB。
- `06.9` 明确本模块不处理任何业务 DML。

### 07 Schema 同步与校验

代码归属：`cool-next/db/schema`。前置：04、06。

- `07.1` 从 Descriptor 构建期望 Schema。
- `07.2` 读取数据库实际表、列、主键和索引。
- `07.3` 对数据库元数据进行方言归一化。
- `07.4` 精确比较列名大小写、类型、nullable 和索引顺序。
- `07.5` 实现 `sync` 安全增量操作。
- `07.6` 禁止删列、缩窄字段等破坏性同步。
- `07.7` 实现 `validate` 差异报告和启动失败。
- `07.8` 实现 `off` 模式的业务 Schema 跳过。
- `07.9` 保留已启用基础设施的运行结构探测。
- `07.10` 输出可定位到实体和字段的错误信息。

### 08 事务 Scope 与 Runner

代码归属：`cool-next/db/tx`。前置：02、03。

- `08.1` 定义 Context 中的事务 Scope。
- `08.2` 实现 `Current` 查询当前 TX 和连接组。
- `08.3` 实现 `Runner.Within`。
- `08.4` 无事务时开启、提交或回滚事务。
- `08.5` 同组嵌套时复用事务且不取得提交权。
- `08.6` 跨组嵌套时立即返回配置错误。
- `08.7` 内层失败时标记 rollback-only 并保存第一个失败。
- `08.8` Panic 时回滚并重新抛出。
- `08.9` 防止事务 Context 在回调结束后继续使用。
- `08.10` 验证并发关闭和重复结束不破坏状态。

### 09 Framework Database Group

代码归属：`cool-next/db`。前置：03、06、07、08。

- `09.1` 解析并验证框架数据库连接组。
- `09.2` 向 Base Service、CRUD 和事务基础设施传递同一组，Descriptor 继续保持为纯元数据。
- `09.3` 建立数据库 Runtime，集中提供 DB、Dialect 和 Runner。
- `09.4` 检查当前 Scope 与框架组一致。
- `09.5` 管理需要事务能力保证的业务表集合。
- `09.6` 在应用 Ready 前执行数据库版本和能力探测。
- `09.7` 在 `schema.mode=off` 下继续执行必要探测。
- `09.8` 统一输出数据库启动诊断，隐藏连接密码。
- `09.9` 为 Recycle、Outbox、Inbox 提供后续扩展入口。

阶段验收门：使用手写测试 Descriptor 验证三种数据库的方言、Schema 和事务契约，不依赖生成器、CRUD 或 Transport。

## 5. 第二阶段：编译期基础设施

### 10 模块声明与模块配置

代码归属：`cool-next/core/module`、`core/configuration`。前置：03。

- `10.1` 定义 `Declaration[T]`、`ComponentRef` 与 `Ref`。
- `10.2` 从目录确定模块唯一身份。
- `10.3` 解析 `ModuleConfig()` 声明。
- `10.4` 校验模块名称、描述和顺序。
- `10.5` 提取默认配置并调用 03 的配置引擎。
- `10.6` 将规范化配置解码为模块 `Config`。
- `10.7` 使用 `gvalid` 校验模块配置。
- `10.8` 记录模块中间件与全局中间件的编译期符号引用，本阶段只校验语法和符号存在性。
- `10.9` 拒绝重复模块、非法声明和未知配置；中间件构造器类型与路由注册合法性在模块 31 校验。
- `10.10` 保证相同输入产生相同模块配置。

### 11 Go 源码发现与符号分析

代码归属：`cool-next/codegen`。前置：04、10。

- `11.1` 使用 Go Package、AST 和类型系统加载源码。
- `11.2` 递归扫描 `modules/<module>` 的允许目录。
- `11.3` 排除测试文件、`testdata`、隐藏目录和生成文件。
- `11.4` 发现 ModuleConfig、实体、Schema 和通用构造器；Controller、Middleware、生命周期、gRPC 和 Outbox 由各自所属的后续模块扩展发现模型。
- `11.5` 解析 `module.Ref` 的编译期符号引用。
- `11.6` 收集构造器参数和返回类型。
- `11.7` 校验构造器只能返回 `*T` 或 `(*T, error)`。
- `11.8` 拒绝可变参数和多个业务返回值。
- `11.9` 建立与代码位置关联的诊断信息。
- `11.10` 在无数据库连接时完成分析。
- `11.11` 输出不可变且可复现的中间源码模型。

### 12 Provider 依赖图与拓扑

代码归属：`cool-next/codegen`。前置：11。

- `12.1` 将构造器返回类型登记为 Provider。
- `12.2` 将构造器参数类型登记为依赖。
- `12.3` 注入模块强类型配置和源码中已声明的通用 Provider；生成 Descriptor 与 Base Service Provider 分别在模块 13 和 23 定义后再扩展。
- `12.4` 按 Go 类型可赋值规则匹配依赖。
- `12.5` 校验接口依赖只有一个 Provider。
- `12.6` 拒绝缺失、重复或歧义 Provider。
- `12.7` 检测直接和间接循环依赖。
- `12.8` 允许跨模块具体 Provider，校验接口只面向 `contract/**`，并禁止跨模块 Config/Seed。
- `12.9` 构建模块级和组件级依赖图。
- `12.10` 按依赖拓扑、Order、模块路径和符号稳定排序。
- `12.11` 输出完整的依赖路径错误。
- `12.12` 为后续生命周期图保留静态组件类别。

### 13 Descriptor 与 DO 代码生成

代码归属：`cool-next/codegen`、`modules/modules_gen.go`。前置：05、11、12。

- `13.1` 将实体源码模型编译为 Descriptor 模型。
- `13.2` 生成 Metadata、Descriptor 和不可变 Field 实现。
- `13.3` 生成主键、字段索引和 Schema 索引元数据。
- `13.4` 生成私有 GoFrame DO Struct 与 DOValue 状态存储。
- `13.5` 生成 `SetColumn` 类型转换和 `DBData()`。
- `13.6` 为后续 Base Provider 生成保留类型化 Descriptor 输入，本模块不生成尚未定义的 Base Service。
- `13.7` 校验表名、列名、索引冲突和标识符长度。
- `13.8` 拒绝未知 `cool` 标签属性。
- `13.9` 保证生成结果不依赖源码遍历顺序。
- `13.10` 禁止向业务模块写入分散生成文件。
- `13.11` 将生成的 Descriptor 作为类型化 Provider 接入模块 12 的依赖图。

### 14 静态模块注册表生成

代码归属：`cool-next/codegen`、`modules/modules_gen.go`。前置：12、13。

- `14.1` 生成模块静态描述和规范化配置装配。
- `14.2` 按拓扑生成构造函数调用。
- `14.3` 处理返回 `error` 的构造函数。
- `14.4` 生成当前已冻结的实体、模块配置和通用 Provider 注册表。
- `14.5` 定义稳定的生成片段排序与合并机制，供后续模块增加已批准的生成扩展。
- `14.6` Controller/路由、生命周期、gRPC、Outbox 和 Event/Schedule/Queue 的契约未在所属模块冻结前，不发现、验证或生成它们。
- `14.7` 将所有当前可用的 Cool 生成内容写入唯一 `modules/modules_gen.go`。
- `14.8` 添加标准 `Code generated` 标记。
- `14.9` 保证运行期不再扫描目录或按字符串定位已生成组件。

### 15 Cool CLI 与生成流水线

代码归属：`cmd/cool`、`cool-next/codegen`。前置：14。

- `15.1` 建立 `cool` 根命令和统一退出码。
- `15.2` 实现 `cool generate`，在内存中完成分析和生成。
- `15.3` 对结果执行格式化和 Go 类型检查。
- `15.4` 全部成功后原子替换旧生成文件。
- `15.5` 失败时保持旧文件字节不变。
- `15.6` 验证重复生成字节一致。
- `15.7` 实现 `cool check` 的生成文件新鲜度、生成稳定性、实体元数据、通用 Provider 图和当前已冻结边界检查。
- `15.8` 明确 `cool check` 不替代 `gofmt`、`go vet` 和 `go test`。
- `15.9` 后续模块通过共享分析规则扩展 `cool check`；本模块不预先检查尚未定义的路由、生命周期、鉴权或 Outbox 契约。
- `15.10` 建立 AST、Golden、原子写入和可重复性测试。

阶段验收门：加入只包含模块配置、实体和普通构造器的示例模块后，只依靠源码生成唯一 `modules/modules_gen.go`；重复生成完全一致，依赖或元数据错误能在编译前定位。本阶段不要求 Controller、Base Service、Transport 或 Outbox 已存在。

## 6. 第三阶段：数据访问与 CRUD 内核

### 20 QueryRequest 与安全查询 AST

代码归属：`cool-next/crud`。前置：04。

- `20.1` 定义 presence-aware QueryRequest、RequestValue 和强类型读取。
- `20.2` 定义受校验的 ColumnRef、实体类型和表别名。
- `20.3` 定义 Eq、Ne、In、Like 等条件节点。
- `20.4` 定义 Select、Alias、Join、Group、Having、Order 节点。
- `20.5` 定义 QueryOp、FieldEq、FieldLike 和 QueryBuilder。
- `20.6` 限制 RawWhere 为常量表达式和绑定参数。
- `20.7` 拒绝动态表名、列名和 SQL 值拼接。
- `20.8` 本模块只构造查询，不接触数据库。

### 21 QueryPlan 编译与查询执行

代码归属：`cool-next/crud`。前置：20。

- `21.1` 定义不可变 QueryPlan。
- `21.2` 通过 DescriptorResolver 解析根实体、字段、别名和 Join。
- `21.3` 将 FieldEq 请求数组编译为 `IN`，缺少参数时跳过条件。
- `21.4` 校验 Select、Group、Having、Order 及各类上限。
- `21.5` 将查询节点以参数化方式应用到 `gdb.Model`。
- `21.6` 编译分页数据查询和 Count 查询。
- `21.7` 保证 ApplyQuery 不修改原始计划。
- `21.8` 建立复杂查询和模糊测试集。

### 22 Service 输入输出与 Mutable

代码归属：`cool-next/core/service`。前置：05、20。

- `22.1` 定义 Descriptor 驱动的 `Mutable[E]` 和字段来源。
- `22.2` 实现 Has、Get、Set、SetNull。
- `22.3` 定义 AddInput、DeleteInput、UpdateItem、UpdateInput。
- `22.4` 建立全部输入的 Smart Constructor。
- `22.5` 拒绝空批次、重复字段和错误 ID 类型。
- `22.6` 定义保持输入形状的 AddResult。
- `22.7` 定义只读 Record、Query、Pagination 和 PageResult。
- `22.8` 保证 uint64、时间、零值、false 和 null 不失真。
- `22.9` 让 HTTP、gRPC、任务和 Service 共用同一构造入口。

### 23 Base 数据访问与查询操作

代码归属：`cool-next/core/service`。前置：08、13、21。

- `23.1` 定义注入 Descriptor 和 Framework Database Group 的泛型 Base。
- `23.2` 实现 Descriptor、Model 和 Tx。
- `23.3` 无 Scope 时使用框架组，同组 Scope 时绑定 TX。
- `23.4` 拒绝跨组 Scope，禁止退回全局 DB。
- `23.5` 实现 Info、List、Page 和 QueryPlan 应用。
- `23.6` 定义只读 NativeStatement。
- `23.7` 校验 NativeSQL 仅为单条 SELECT/CTE。
- `23.8` 实现参数化、事务感知的 NativeQuery。
- `23.9` 禁止 Raw、Exec、Query 等旁路 DML。
- `23.10` 扩展生成器，为每个生成 Descriptor 构造对应的 Base Service Provider。

### 24 Base 写操作、Hook 与重写语义

代码归属：`cool-next/core/service`。前置：22、23。

- `24.1` 实现单对象和数组 Add，按输入顺序返回主键。
- `24.2` 实现仅更新明确出现字段的 Update。
- `24.3` 实现物理删除基础能力。
- `24.4` 将 Mutable 转为生成的 DOValue。
- `24.5` 最终 DML 前清理客户端 ID 和只读字段。
- `24.6` 对隐藏字段和非法 Update 字段直接拒绝。
- `24.7` 定义 Mutation、ModifyBeforeHook、ModifyAfterHook。
- `24.8` 批量操作前后 Hook 各执行一次。
- `24.9` Hook 失败使当前事务整体回滚。
- `24.10` 静态识别默认 Base、纯 override、Base 委托 override。
- `24.11` Base 委托复用 Plan、TX 和 Hook 外壳。
- `24.12` 纯 override 不隐式套用 Base 增强。
- `24.13` 重写一个动作不影响其他动作。
- `24.14` 扩展源码分析和生成 Adapter，静态识别默认 Base、纯 override 与直接 Base 委托。

### 25 ActionPlan 与 CRUD Dispatcher

代码归属：`cool-next/crud`。前置：10、21、24。

- `25.1` 定义六种固定 Action。
- `25.2` 定义 FieldPolicy 并编译 Hidden、Readonly、InfoIgnore、SortFields。
- `25.3` 定义包含 QueryPlan 和 FieldPolicy 的 ActionPlan。
- `25.4` 定义 OperationScope 并写入 Context。
- `25.5` Dispatcher 使用 Runner 开启或复用事务。
- `25.6` Add/Delete/Update 编排前后 Hook。
- `25.7` 调用生成期选定的 Base 或 override Adapter。
- `25.8` 默认 Base 和 Base 委托应用 Controller 前置增强。
- `25.9` 纯 override 只保留认证、绑定、校验和事务外壳。
- `25.10` 确保批量操作禁止部分成功。
- `25.11` Panic、Hook、DML 或提交失败统一回滚。
- `25.12` 禁止默认 CRUD 退出事务。

### 26 删除归档与恢复

代码归属：`cool-next/db/gnrecycle`、`cool-next/crud`。前置：07、08、24、25。

- `26.1` 定义 `cool.crud.softDelete`，默认值为 true。
- `26.2` 定义并管理 `cool_recycle` 内部 Schema。
- `26.3` 定义回收记录、操作者和脱敏来源信息。
- `26.4` 锁定并读取当前仍存在的目标记录。
- `26.5` 按主键稳定排序并确定性编码强类型快照。
- `26.6` 一次批量删除只写一条回收记录。
- `26.7` 同一事务内完成归档和物理删除并核对行数。
- `26.8` 全部 ID 不存在时幂等成功且不写空记录。
- `26.9` 三种数据库分别保证快照与实际删除内容一致。
- `26.10` 通过表名解析当前生成图中的 Descriptor。
- `26.11` 将快照解码为强类型 DO 并恢复全部记录。
- `26.12` 恢复使用普通 Insert，禁止 Upsert 和覆盖。
- `26.13` 全部恢复成功后才删除回收记录。
- `26.14` 并发恢复只允许锁持有方成功。
- `26.15` 启动前探测回收表、索引和事务能力。

阶段验收门：六个 CRUD 动作在无 HTTP/gRPC 情况下通过单条、批量、Hook、重写、字段安全、事务和三数据库回收站验收。

## 7. 第四阶段：接口与安全

### 30 Controller DSL 与 CurdOption

代码归属：`cool-next/core/controller`。前置：20、25。

- `30.1` 定义不可变 Definition 和链式 Builder。
- `30.2` 只允许通过 Admin、App 创建 Builder。
- `30.3` 根据模块及 Controller 目录推导默认路径，支持显式覆盖。
- `30.4` 定义完整 CurdOption 和六种 APIType。
- `30.5` 分别配置 PageQueryOp 和 ListQueryOp。
- `30.6` 定义 StaticQuery、DynamicQuery、Before 和强类型 InsertParam。
- `30.7` 数组 Add 按顺序逐项执行 InsertParam。
- `30.8` 编译 Hidden、Readonly、InfoIgnore、SortFields 和默认排序。
- `30.9` 校验 Entity、Service 和 Base 泛型匹配。
- `30.10` Build 后复制全部可变集合。
- `30.11` DynamicQuery 可动态改变条件、Join 和顺序，但不得返回改变默认 CRUD 响应形状的 Select 别名；StaticQuery 的 Extend 只能条件化使用外层 QueryOp.Select 已静态声明的别名，不得新增别名。

### 31 路由模型与静态注册

代码归属：`cool-next/core/route`、`core/controller`。前置：14、30。

- `31.1` 定义 RouterOptions、Route、URLTag 和绑定来源。
- `31.2` 处理 Controller、全局和模块前缀。
- `31.3` 生成六个默认 CRUD 路由并支持自定义 Route。
- `31.4` 实现 BindAuto 的无歧义推导。
- `31.5` 定义默认事务与 NonTransactional。
- `31.6` 禁止默认 CRUD 退出事务。
- `31.7` 定义中间件、权限字符串和 IgnoreToken 标签。
- `31.8` 规范化 HTTP Method 和完整 Path。
- `31.9` 检查 Alias、路由、权限、标签和中间件冲突。
- `31.10` 通过 go/types 校验 Handler 签名。
- `31.11` 生成静态路由表。
- `31.12` 扩展生成器的 Controller、Middleware、Route 发现、静态校验和注册片段。

### 32 HTTP Binder 与请求处理链

代码归属：`cool-next/core/controller`、`cool-next/crud`。前置：22、25、31。

- `32.1` 绑定 JSON 对象和顶层数组，保留形状与字段来源。
- `32.2` 规范化 Delete 的标量、逗号字符串和数组 ID。
- `32.3` 绑定 Info 主键及 List/Page 公共参数。
- `32.4` 校验 order 与 sort 数量和方向。
- `32.5` 支持 Query、Form、Path 和文件上传。
- `32.6` 对 DTO 执行 `gvalid` 并默认拒绝未知字段。
- `32.7` 默认 Base 和 Base 委托路径执行 Before、绑定、InsertParam。
- `32.8` 纯 override 只按自身 DTO 签名绑定和校验。
- `32.9` 将默认 CRUD 请求交给 Dispatcher。
- `32.10` 自定义 Route 默认通过 Runner 建立事务。
- `32.11` 强制 Body、Batch、Page、List、Export 上限。
- `32.12` 传播取消、Deadline 和 Trace Context。

### 33 HTTP 响应与异常过滤

代码归属：`cool-next/core/controller`、`core/exception`。前置：02、32。

- `33.1` 定义统一 `{code,message,data}` 响应。
- `33.2` 正确保留 `data: 0`、`false` 和空数组。
- `33.3` 编码 AddResult、Record、List 和 PageResult。
- `33.4` 将 Cool 异常映射为业务响应并支持显式 401/403。
- `33.5` 未知错误返回 HTTP 500 和安全 Comm 消息。
- `33.6` 基础设施错误返回安全 Core 消息。
- `33.7` Recover Panic 并关联 Trace ID。
- `33.8` 内部日志保留 Cause、堆栈和 Trace。
- `33.9` 响应和日志脱敏 Token、密码及连接信息。
- `33.10` 建立与 Node HTTP 契约的 Golden Test。

### 34 Identity、Context 与密码边界

代码归属：`cool-next/auth`、`auth/bcrypt`。前置：02。

- `34.1` 定义 AdminKind、AppKind 和独立 Identity。
- `34.2` RoleIDs 每次返回防御性副本。
- `34.3` 使用标准 `context.Context` 保存已验证身份。
- `34.4` 实现 `auth.Admin(ctx)` 和 `auth.App(ctx)`。
- `34.5` 缺失或 Subject 不匹配时返回安全错误。
- `34.6` 禁止业务构造伪造身份写入 Context。
- `34.7` 定义协议无关 Authorizer 调用边界。
- `34.8` 区分凭证无效、权限不足和基础设施失败。
- `34.9` 封装 bcrypt Hash 与 Verify。
- `34.10` 在模块 34 的独立设计批准前冻结 bcrypt Cost 配置、合法范围、已有 Hash 兼容和登录成功后的渐进式 Rehash 策略；这些契约未批准前不得实现密码适配器。
- `34.11` 禁止明文密码进入 Session、JWT 和日志。
- `34.12` 不在框架中猜测超级管理员业务规则。

### 35 SessionStore

代码归属：`cool-next/auth/session`。前置：03、34。

- `35.1` 定义内部不可变 Session 及 Get、Save、RotateRefresh、Revoke、RevokeUser。
- `35.2` Session 保存 Subject、用户、双 JTI、过期时间和必要快照。
- `35.3` Admin 与 App 使用严格分离的字段集。
- `35.4` 默认选择 Redis Store，启动时校验 Group 并 PING，不自动降级。
- `35.5` 定义 `prefix + sessionId` Redis Key 和版本化 UTF-8 JSON。
- `35.6` 使用十进制字符串保存 uint64。
- `35.7` 校验 Key、Session ID、Subject、JTI、必填字段和 schemaVersion。
- `35.8` Redis TTL 不得晚于 expiresAt，读取时再次验证过期。
- `35.9` 原子比较旧 Refresh JTI 并轮换双 JTI。
- `35.10` RevokeUser 同时匹配 Subject 和用户 ID。
- `35.11` 实现并发安全且按 TTL 清理的 Memory Store。
- `35.12` 明确 SessionStore 不得作为通用缓存注入。

### 36 JWT、刷新与鉴权中间件

代码归属：`cool-next/auth/jwt`、`cool-next/auth`。前置：31、34、35。

- `36.1` 解析并校验 JWT 配置，首期只允许 HS256。
- `36.2` 校验密钥长度、Issuer、Audience、TTL 和 Clock Skew。
- `36.3` 使用 currentKeyId 签发，保留旧 Key 只用于验证。
- `36.4` 强制校验 Header 中的 kid 和算法。
- `36.5` 分别定义 Admin 和 App Claims。
- `36.6` 强制校验 sessionId、jti、subject、isRefresh、iss、aud、iat、nbf、exp。
- `36.7` 区分 Access Token 和 Refresh Token。
- `36.8` 验签后读取服务端 Session 并核对身份、JTI、passwordV 和有效期。
- `36.9` 以服务端 Session 构建 Identity。
- `36.10` 刷新时重新生成 Token 对并原子轮换双 JTI。
- `36.11` 并发刷新只允许一个请求成功。
- `36.12` 检测旧 Refresh Token 重放并撤销 Session。
- `36.13` 实现 IgnoreToken、受保护路由和 Permission 鉴权流程。
- `36.14` HTTP 使用规范化 Method + Path 作为权限资源。
- `36.15` Redis 故障返回 Core，认证失败返回 401，授权不足返回 403。
- `36.16` 为 gRPC Interceptor 复用同一验证内核。

阶段验收门：HTTP Handler、Binder、响应、自定义 Route、JWT、Session 和权限调用契约可脱离 Application Host 通过适配层测试；完整 HTTP 端口运行验收归属第五阶段。角色、菜单和超级管理员规则仍属于后续 `base` 业务模块。

## 8. 第五阶段：应用承载

### 40 Application Graph 与组件生命周期

代码归属：`cool-next/core/app`、`core/module`。前置：03、12、14。

- `40.1` 定义不可变 `module.Graph`，只允许生成器受控构造。
- `40.2` 校验 Provider、生成注册片段和组件生命周期一致性。
- `40.3` 定义 Initializer、Starter、Stopper 和协议无关 Transport 接口。
- `40.4` 按拓扑构造、OnInit 和 OnStart。
- `40.5` 同层按 Order、模块路径和符号稳定排序。
- `40.6` 在正确时机登记组件清理资格。
- `40.7` 按实际成功登记顺序逆序 OnStop。
- `40.8` 保证每个组件最多停止一次。
- `40.9` 初始化失败时清理此前成功组件。
- `40.10` OnStop 失败时继续清理其他组件并确定性聚合错误。
- `40.11` 扩展生成器的生命周期接口发现、拓扑校验和 Graph 注册片段。

### 41 Host 协调、Ready 与关闭

代码归属：`cool-next/core/app`。前置：40。

- `41.1` 只接收由生成 Graph 注入的 `[]Transport`，不识别 HTTP、gRPC 或其他具体 Adapter 类型。
- `41.2` 校验 Transport 名称、启用状态和拓扑一致性。
- `41.3` 先 Prepare 全部 Transport，再启动组件和 Transport。
- `41.4` 全部成功后统一标记 Ready。
- `41.5` 任一 Prepare/Start 失败时回滚全部 Prepared Transport。
- `41.6` 监督所有可观察终止 Channel。
- `41.7` Channel 报错或意外关闭时撤销 Ready 并停止其他 Transport。
- `41.8` 系统关闭时先撤销 Ready，再停止新请求并等待在途请求。
- `41.9` 最后逆序释放生命周期组件。
- `41.10` 所有关闭动作受全局 Deadline 约束。
- `41.11` Host 本身不得 Fatal、直接 os.Exit 或导入具体 Transport 实现包。

### 42 HTTP Transport

代码归属：`cool-next/core/app/http`。前置：31、33、36、41。`core/app/http` 是独立 Go 包，可以导入 `ghttp` 和上层 `core/app` 的 Transport 契约；协议无关的 `core/app` 包不得导入该子包或 `ghttp`。

- `42.1` 解析 HTTP Enabled、地址和端口配置。
- `42.2` 创建 ghttp.Server 并安装生成路由与中间件。
- `42.3` Prepare 阶段执行 net.Listen 并通过 SetListener 交给 ghttp。
- `42.4` 保存 Prepared Listener 所有权，端口冲突在启动前返回。
- `42.5` Start 调用 ghttp.Server.Start 并在超时内等待 Running。
- `42.6` Ready 前通过 Gate 返回 HTTP 503。
- `42.7` Prepared 状态 Stop 直接关闭 Listener。
- `42.8` Started 状态在受监督 goroutine 调用 Shutdown。
- `42.9` Stop Context 超时后仍关闭自持 Listener。
- `42.10` 将 net.ErrClosed 视为幂等成功。
- `42.11` 锁定并测试当前 GoFrame Start/Shutdown 行为。
- `42.12` 明确异常 Serve Fatal 导致整个进程非零退出。

### 43 gRPC Transport

代码归属：`cool-next/grpc`。前置：02、14、34、36、41。

- `43.1` 解析 gRPC Enabled、地址和端口配置。
- `43.2` Prepare 创建 Listener、grpcx.GrpcServer 和拦截器。
- `43.3` 调用生成 GRPCRegistrar 与标准 protobuf 注册函数。
- `43.4` Adapter 负责 protobuf 与领域 DTO 转换。
- `43.5` 禁止业务 Service 接收 protobuf Request。
- `43.6` 配置服务发现时使用公开 Registry API 注册并保存返回值。
- `43.7` Start 在受监督 goroutine 调用 Server.Serve。
- `43.8` 将 Serve 异常写入 Transport 终止 Channel。
- `43.9` Ready 前返回 Unavailable。
- `43.10` Stop 先 GracefulStop，超时后强制 Stop 并关闭 Listener。
- `43.11` 停止时成对注销服务发现记录。
- `43.12` 将 Cool 异常映射为 gRPC Code 和 Error Details。
- `43.13` 扩展生成器的 gRPC Service 发现、签名校验和 GRPCRegistrar 注册片段。

### 44 HTTP/gRPC 共用上下文

代码归属：`cool-next/core/app`、`cool-next/core/app/http`、`cool-next/grpc`。前置：34、36、42、43。

- `44.1` 建立协议无关 Trace ID。
- `44.2` 传播 Admin/App Identity、Session 状态和权限信息。
- `44.3` 传播请求 Deadline 和取消信号。
- `44.4` HTTP Middleware 与 gRPC Interceptor 构建等价 Context。
- `44.5` 两种协议使用相同 JWT 验证内核和 Authorizer。
- `44.6` 领域 Service 只接收 `context.Context`。
- `44.7` 禁止传入 ghttp.Request、ServerStream 或 protobuf。
- `44.8` 验证两种协议的身份和取消传播一致。

### 45 EPS 与 OpenAPI

代码归属：`cool-next/eps`。前置：04、14、30、31、33、36。

- `45.1` 从 Descriptor 生成实体字段、类型和约束元数据。
- `45.2` 输出隐藏、只读、允许排序和描述属性。
- `45.3` 从静态路由表生成模块、Controller 和 API。
- `45.4` 生成 CRUD 请求与响应 Schema。
- `45.5` Add 单对象和数组响应使用 oneOf。
- `45.6` 输出 Join Select 别名形成的响应字段。
- `45.7` 默认 CRUD 的响应 Schema 只使用 Descriptor 字段与 StaticQuery 中已登记的 Select 别名；DynamicQuery 返回 Select 别名，或 Extend 引入外层 QueryOp.Select 未声明的别名时，生成或计划编译失败。
- `45.8` 输出认证、权限和错误响应信息。
- `45.9` 保证 description 是唯一描述来源。
- `45.10` 建立 EPS 与 OpenAPI 快照测试。

### 46 应用入口与完整启动装配

代码归属：`main.go`、`cool-next/core/app`。前置：09、15、41、42、43、44、45。

- `46.1` 实现根 `main.go` 的最小应用入口。
- `46.2` 入口只创建初始 Context、调用 `app.Run` 并设置退出码。
- `46.3` `app.Run` 按配置、Graph、Init、Prepare、Start 顺序执行。
- `46.4` 支持 HTTP-only、gRPC-only 和双 Transport 的完整装配。
- `46.5` 将启动和运行错误完整返回入口。
- `46.6` 生产部署明确要求进程监督器和健康检查。
- `46.7` 实现 `cool build` 和 `cool run`，在构建或运行前执行生成文件新鲜度与当前全部静态契约检查。
- `46.8` 建立启动、Ready、运行失败和优雅关闭集成测试。

阶段验收门：框架可作为 HTTP-only、gRPC-only 或双协议应用完整启动、Ready、监督和关闭。

## 9. 第六阶段：可靠异步能力

### 50 Envelope 与消息契约

代码归属：`cool-next/outbox`。前置：02。

- `50.1` 定义 RFC 9562 UUIDv7 MessageID。
- `50.2` 使用密码学安全随机源生成 UUIDv7。
- `50.3` 保证并发及时钟回拨时格式合法且不重复。
- `50.4` 定义字段私有的不可变 Envelope。
- `50.5` 校验 Topic、MessageType、Version 和 Payload 序列化。
- `50.6` 校验 Header 白名单并支持可选 Message Key。
- `50.7` Payload 和 Headers 返回防御性副本。
- `50.8` 定义仅供基础设施使用的 Restore。
- `50.9` 定义 Enqueuer、Publisher、Incoming 和 ConsumerHandler。
- `50.10` 定义 ConsumerDefinition 与 Consume。
- `50.11` 校验 Consumer Name 稳定唯一和 SupportedVersions。
- `50.12` 定义 Permanent Error 分类。
- `50.13` 禁止业务直接依赖 Publisher。
- `50.14` 固定 at-least-once，不承诺 exactly-once。

### 51 Outbox/Inbox Schema 与 Store 契约

代码归属：`cool-next/outbox`、`cool-next/db/schema`。前置：07、08、50。

- `51.1` 定义 pending、retry、leased、sent、dead 状态。
- `51.2` 定义 `cool_outbox`、`cool_inbox` 逻辑 Schema。
- `51.3` 固定 MessageID 字符集、长度、大小写语义及三个索引顺序。
- `51.4` 定义 Store 私有持久化 Record。
- `51.5` 定义 Enqueue、Claim、Renew、MarkSent、MarkRetry、MarkDead。
- `51.6` 定义 ReplayDead、InsertIfAbsent、ClaimToken 和 ErrClaimLost。
- `51.7` 定义 Envelope 与 Store Record 转换，避免包循环。
- `51.8` Enqueue 有 Scope 时复用 TX，无 Scope 时开启短事务。
- `51.9` 跨 Database Group 时立即拒绝。
- `51.10` 初始状态使用数据库当前时间。
- `51.11` Enqueue 只写数据库，不调用 Publisher。
- `51.12` 校验 Payload、Header 和完整 Envelope 大小。

### 52 三数据库 Outbox Store

代码归属：`cool-next/outbox/store`。前置：06、51。

- `52.1` 实现 MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+ Store。
- `52.2` MySQL/PostgreSQL 使用 READ COMMITTED 和 SKIP LOCKED 领取。
- `52.3` SQLite 使用候选读取、条件 UPDATE、RowsAffected 与 ClaimToken 回读。
- `52.4` pending/retry 领取时原子增加 attempts。
- `52.5` 过期 Lease 重新领取时保留 attempts。
- `52.6` 所有时间判断使用数据库当前时间。
- `52.7` Renew 严格推进 Lease Deadline。
- `52.8` 状态更新必须匹配 MessageID、leased 和 ClaimToken。
- `52.9` 受影响行为 0 时返回 ErrClaimLost，大于 1 时报告不变量损坏。
- `52.10` MarkSent、Retry、Dead 清理全部 Lease 字段。
- `52.11` 按各数据库的固定算法实现 Inbox 去重。
- `52.12` 探测表、主键、列、列序、固定索引及领取能力。
- `52.13` 探测事务必须无条件回滚且不调用 Publisher。

### 53 Outbox Worker 与发布状态机

代码归属：`cool-next/outbox`。前置：40、52。

- `53.1` 定义 Worker Instance ID 和轮询生命周期。
- `53.2` 按 availableAt、createTime、messageId 稳定排序领取。
- `53.3` 分别领取 pending/retry 与过期 leased，避免任一类饥饿。
- `53.4` 领取事务提交后再调用 Publisher。
- `53.5` 网络发布期间禁止持有数据库行锁。
- `53.6` Publish 使用独立 Timeout Context，长发布按需 Renew。
- `53.7` 发布成功 MarkSent，临时失败 MarkRetry，达上限 MarkDead。
- `53.8` 实现指数退避、上限和随机抖动。
- `53.9` Claim 丢失后停止修改该消息。
- `53.10` 过期 Lease 即使达到上限也要再次发布。
- `53.11` 接受 Broker 成功、MarkSent 前崩溃导致的重复。
- `53.12` 定期清理超过 Retention 的 Sent，Dead 不参加普通清理。
- `53.13` 暴露状态、延迟、重试、死信和 Lease 恢复指标。
- `53.14` 日志关联 MessageID、Topic、Attempt 和 WorkerID，默认不记录 Payload。

### 54 Inbox 幂等消费事务

代码归属：`cool-next/outbox`、`outbox/store`。前置：08、23、52。

- `54.1` 将 ConsumerDefinition 编译为不可变 Subscription。
- `54.2` 通过 Subscription 唯一定位 Consumer 和 Handler。
- `54.3` 校验 Envelope、MessageID、Type 和 Version 并解码 DTO。
- `54.4` 使用 Runner 开启 Framework Database Group 事务。
- `54.5` 在 Handler 前执行 InsertIfAbsent。
- `54.6` 重复消息跳过 Handler 并提交事务。
- `54.7` 首次消息在同一事务执行 Handler、Base.Model/Tx 和级联 Enqueue。
- `54.8` Handler 失败时回滚 Inbox、业务 DML 和级联 Outbox。
- `54.9` 本地事务提交成功后才返回 Ack 决策。
- `54.10` Ack 丢失后的重投由 Inbox 唯一键拦截。
- `54.11` 普通 Error 分类为临时失败，Permanent 及非法消息进入 DeadLetter。
- `54.12` 根据持久 Attempt 计算 Retry Delay 和最终 DeadLetter。
- `54.13` 每次 Deliver 使用独立 ConsumerTimeout。
- `54.14` 明确 Inbox 只保护同事务内的本地业务效果。

### 55 Broker Consumer Adapter

代码归属：`cool-next/outbox` 基础设施适配层。前置：40、50、54。

- `55.1` 定义 DurableAck、DurableRetryAttempts、DelayedRetry、DeadLetter、PreservesMessageID 五项能力。
- `55.2` 定义正数 MaxEnvelopeBytes。
- `55.3` Prepare 验证 Broker 连接、拓扑、五项能力和大小上限。
- `55.4` 按 Subscription 隔离同一消息契约的多个 Consumer。
- `55.5` 从 Broker 元数据恢复 Envelope，恢复失败直接进入持久 DLQ。
- `55.6` 调用 DeliverFunc 前持久推进 Attempt。
- `55.7` Attempt 以 ConsumerName + MessageID 隔离，进程重启不得归零。
- `55.8` Ack 必须持久确认成功。
- `55.9` Retry 必须先持久安排延迟重投。
- `55.10` DeadLetter 必须先持久写入 DLQ。
- `55.11` Ack、Retry 或 DLQ 失败时保持原消息未确认。
- `55.12` Visibility/Ack Deadline 必须覆盖处理 Timeout，必要时可靠续期。
- `55.13` Start 只上报不可恢复的消费循环错误。
- `55.14` Stop 先停止拉取，再排空在途操作。
- `55.15` 支持按 ConsumerName + MessageID 检查和重放 DLQ。

### 56 Outbox 运维 CLI

代码归属：`cmd/cool`。前置：15、52、53。

- `56.1` 实现 `cool outbox list`，支持 Status、Topic 和受限 Limit。
- `56.2` 实现 `cool outbox show`。
- `56.3` 默认只展示元数据和脱敏错误。
- `56.4` 禁止输出 Payload、Header、Token 和连接信息。
- `56.5` 实现 `cool outbox replay --dry-run`。
- `56.6` 实际 Replay 强制要求 Operator 和 Reason。
- `56.7` 只允许单条 dead 原子转为 retry。
- `56.8` 保持原 MessageID、Payload、Type 和 Version。
- `56.9` 禁止重放 Sent 消息，CLI 不直接调用 Publisher。
- `56.10` 并发状态变化时返回非零退出码。
- `56.11` 记录脱敏的结构化安全审计。
- `56.12` 不接受临时数据库连接串或换组参数。

### 57 配置、生成图与 Host 集成

代码归属：`cool-next/outbox`、`codegen`、`core/app`。前置：14、40、41、53、55。

- `57.1` 定义并校验完整 Outbox 配置。
- `57.2` 校验 PublishTimeout 小于 LeaseDuration。
- `57.3` 校验批量、重试、Timeout、大小和 Retry 范围。
- `57.4` 生成器识别直接依赖 Enqueuer 的 Producer。
- `57.5` 生成器发现 ConsumerDefinition 并校验名称、Topic、Type、Version。
- `57.6` 将 ConsumerDefinition 编译为 Subscription。
- `57.7` 应用图只允许一个 Publisher 和 ConsumerAdapter。
- `57.8` Producer 存在时校验 Store、Publisher、Worker、Schema。
- `57.9` Consumer 存在时校验 Inbox、Adapter、Capabilities 和 Schema。
- `57.10` 禁止降级为进程内事件或同步发布。
- `57.11` Worker 在数据库、Schema 和 Publisher 后启动。
- `57.12` Consumer 在 Inbox、Handler 和 Broker 就绪后启动。
- `57.13` Worker 停止时先停止领取，再等待在途发布。
- `57.14` Consumer 停止时先停止拉取，再排空在途事务。
- `57.15` 运行循环异常交给 Host 监督。
- `57.16` 无 Producer/Consumer 的应用允许显式关闭 Outbox。

阶段验收门：业务 DML 与 Outbox 原子提交，多 Worker 安全领取，崩溃后按 Lease 恢复，重复投递由 Inbox 去重，所有契约在 MySQL、PostgreSQL、SQLite 上通过。

## 10. 上位设计迁移映射

上位设计与后续独立模块设计按以下状态切换事实来源：

| 状态 | 权威来源 | 迁移动作 |
| --- | --- | --- |
| `pending` | 上位总架构设计 | 子模块文档尚未批准，不删除原契约 |
| `reviewed` | 上位总架构设计 | 子模块文档等待用户最终复核，不得开始实施 |
| `active` | 已批准的子模块设计 | 在同一次文档变更中将原详细契约替换为子文档链接，并更新本节状态 |

状态转为 `active` 必须是原文精简、子文档批准和迁移表更新的原子文档变更，不允许两份文档在一段时间内同时声称为权威定义。当前所有子模块均为 `pending`。

| 原章节 | 目标模块 |
| --- | --- |
| 1 背景与目标、2 总体原则、3 目录与依赖边界 | 总架构索引、01 |
| 4.1-4.4 实体、Schema 声明、Descriptor | 04、05、13 |
| 4.5 DML 与多数据库 | 06、08、09、23、24 |
| 4.6 Schema 模式 | 07、09 |
| 4.7 删除归档与恢复 | 26 |
| 5 模块声明与静态装配 | 10-15、23-24、31、40、43、57 |
| 6 Service 基类 | 22-25 |
| 7.1-7.3 Controller、CurdOption、QueryOp | 20、21、25、30 |
| 7.4-7.5 路由与请求处理顺序 | 31-33 |
| 8 Exception | 02、33、43 |
| 9 鉴权与 Session | 34-36、44 |
| 10 Application Host 与 gRPC | 40-44、46 |
| 11 可靠副作用 | 50-57 |
| 12 EPS、OpenAPI 与安全边界 | 15、20、25、30、33、45 |
| 13 测试与验收 | 按行为分配到各模块，并保留全局验收矩阵 |
| 14 必须保持的不变量 | 总架构索引，并在相关模块引用 |
| 15 后续边界 | 总架构索引、本文第 12 节 |

上位设计第 14 节的 21 条全局不变量按以下矩阵追踪。“拥有模块”负责在独立设计中保留完整契约，“验收模块”负责证明跨层行为没有破坏该约束。

| 不变量 | 拥有模块 | 验收模块 |
| --- | --- | --- |
| 1. 新框架完全重写，v1 只作为行为和测试参考 | 01 | 15、46 |
| 2. CRUD 名称固定为 Add/Delete/Update/Info/List/Page | 22、24、25 | 30、32、33 |
| 3. Add 同时支持单对象和顶层数组并保持 ID 形状 | 22、24 | 32、33 |
| 4. Service 重写后 Base 增强不隐式生效 | 24、25 | 31、32 |
| 5. Dispatcher 事务和整批 Modify Hook 始终生效，默认 CRUD 不允许 NonTransactional | 08、24、25、31 | 26、32 |
| 6. 业务实体是字段事实来源，description 贯穿 DB/EPS/OpenAPI/生成元数据 | 04、13、45 | 07、45 |
| 7. DML 使用 GoFrame ORM 与结构化对象，禁止数据库 map 和自定义 SQL 编译器 | 05、23、24 | 15、26 |
| 8. db/driver 只处理三数据库 DDL 差异 | 06 | 07、52 |
| 9. 框架不提供多租户，不保留 tenantId 或自动租户 Scope | 04、09、22、34、35、36、50、51 | 15、45、57 |
| 10. Session 默认 Redis 且不自动降级，Memory 是显式选型 | 35、36 | 36、46 |
| 11. 模块配置由框架统一合并并使用 gvalid | 03、10 | 15、46 |
| 12. 依赖只从构造函数参数类型推导 | 11、12 | 15 |
| 13. cool generate 只输出 modules/modules_gen.go，protobuf 等标准生成文件不受此限制 | 13、14、15、43 | 15、43 |
| 14. HTTP/gRPC 由同一 Host 统一 Ready、回滚和关闭 | 40、41、42、43 | 44、46 |
| 15. 不实现 Service 方法到 API 的动态映射，自定义接口显式声明 Route | 30、31 | 15、32 |
| 16. 可靠提交后副作用必须与业务 DML 同事务写入 Outbox | 08、51、54 | 53、54、57 |
| 17. Outbox 固定 at-least-once，通过稳定 MessageID、Inbox 和业务幂等处理重复 | 50、51、52、53、54、55 | 53、54、55 |
| 18. CRUD、Inbox、自定义事务 Route 和事务任务共用 dbtx Scope，同组复用、跨组拒绝 | 08、23、25、32、51、54 | 26、54 |
| 19. 数据库基线固定，Outbox/Inbox 特定 DML 只属于 outbox/store | 06、07、52 | 26、52、53、54 |
| 20. JWT 携带 Session ID 和独立 JTI，Refresh 原子轮换且重放时撤销 Session | 35、36 | 36 |
| 21. Event、Schedule、Queue 是框架能力，具体 API 必须通过专项设计 | 14、57、60、61、62 | 57、60、61、62 |

迁移时不能删除任何行为契约。同一契约如同时影响多个模块，选择一个拥有者保留完整定义，其他模块只引用编号，不复制全文。

## 11. 测试与交付规则

每个功能单元都必须有对应测试，但不要求每个功能单独建立设计文档。模块实施计划应将功能合并为可审查的任务，同时保留功能编号与测试的可追溯性。

全局交付命令按能力可用性逐步加入门禁：模块 01-14 只执行当前已存在的标准 Go 命令和模块专项测试；模块 15 完成后，后续模块加入 `cool check`；模块 46 完成后，将 `cool build` 加入固定门禁，并在受控超时和临时端口下执行 `cool run` 启停冒烟测试。最终非驻留交付命令固定包含：

```text
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
cool check
cool build
```

需要数据库行为保证的模块，必须在 MySQL 8.x、PostgreSQL 9.5+ 和 SQLite 3.24+ 上运行同一套行为用例。涉及状态共享、锁、Lease、Session 和生命周期的模块必须运行 Race Test；涉及标签、路由、查询表达式和消息解码的模块应增加 Fuzz Test。

## 12. 后续专项设计

Event、Schedule 和 Queue 不属于首轮 44 个模块的实施内容。它们分别使用以下预留编号：

| 编号 | 专项 | 必须冻结的内容 |
| --- | --- | --- |
| 60 | 进程内 Event | Definition、Handler、顺序、错误传播和生命周期 |
| 61 | Schedule | Cron、重叠策略、多实例执行、超时和补跑 |
| 62 | Queue | Definition、Ack、重试、超时、DLQ、并发和 Broker Adapter |

三个专项都必须复用本设计已冻结的 Application Host、Outbox、Inbox、MessageID 和事务边界，不得另造冲突的可靠投递或幂等协议。

## 13. 执行顺序

本文批准后，从模块 01 开始，每次只对一个模块执行以下流程：

```text
提取上位设计契约
-> 编写独立模块设计
-> 用户复核设计
-> 编写该模块实施计划
-> 按功能编号实施与测试
-> 通过模块完成标准
-> 进入下一模块
```

不在实施 01 时预先创建后续模块的空 API，也不因为某个上层功能会使用底层包就提前扩大底层抽象。
