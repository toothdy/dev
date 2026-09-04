# 模块 07：Schema 同步与校验设计

> 日期：2026-08-05
> 状态：已实现，补充归档
> 模块：07 Schema 同步与校验
> 对应拆分项：07.1-07.10
> 前置模块：04 实体与 Descriptor 元数据、06 数据库方言与能力探测

## 1. 目标

模块 07 在 `cool-next/db/schema` 以实体 Descriptor 为唯一事实来源，比较期望数据库结构与实际结构，并按 `sync`、`validate`、`off` 三种模式处理业务实体表。

必须满足：

1. 从 `entity.Metadata` 推导表、列、主键、自增属性和有序索引；
2. 分别读取 MySQL、PostgreSQL 和 SQLite 的真实表、列和索引；
3. 归一化各方言的类型名称，同时保留列名及索引列顺序的精确比较；
4. `sync` 只创建表、补充可空或有默认值的列、创建缺失索引；
5. `validate` 只报告差异并阻止启动，不执行 DDL；
6. `off` 不读取或比较业务实体表；
7. 所有未自动修复的差异都能定位到表及列或索引。

本模块不连接或注册数据库组、不执行数据库版本/事务/InnoDB 能力探测、不管理 Recycle、Outbox、Inbox 等基础设施表，也不实现迁移版本、删列、改列、数据回填或 DML。数据库组和能力探测由模块 09 与模块 06 负责；`off` 不能成为它们的绕过路径。

## 2. 公开契约

```go
type Mode string

const (
    Sync     Mode = "sync"
    Validate Mode = "validate"
    Off      Mode = "off"
)

type Manager struct { /* 私有 database 和 dialect */ }

func New(database gdb.DB, dialect driver.Dialect) (*Manager, error)
func (m *Manager) Apply(ctx context.Context, mode Mode, metadata ...entity.Metadata) (Report, error)
func Differences(err error) (Report, bool)
```

`New` 拒绝 nil 数据库，并用方言引用一个固定探测标识符来确认 Dialect 有效。`Apply` 对 nil Context 使用 `context.Background()`，拒绝未知模式；返回的 `Report` 固定携带方言和确定排序的差异。

`ValidationError` 保存完整 `Report`。调用方通过 `Differences` 或 `errors.As` 获取差异，不能依赖人类可读错误字符串解析状态。

## 3. 期望结构与实际探测

`expectedTable` 从 Descriptor 构建 `Table`：字段以物理列名、方言归一化类型、nullable、主键和自增属性表达；实体索引的逻辑字段名先解析为 Descriptor 字段，再转换为物理列名，保留声明顺序。

`inspectTable` 使用 GoFrame 元数据接口读取表和字段。索引读取按方言分派：

| 方言 | 索引来源 | 特殊处理 |
| --- | --- | --- |
| MySQL | `information_schema.STATISTICS` | 跳过 `PRIMARY`，按 `SEQ_IN_INDEX` 保留顺序 |
| PostgreSQL | `pg_index`、`pg_class`、`pg_attribute` | 跳过主键索引，按 `indkey` ordinal 保留顺序 |
| SQLite | `PRAGMA index_list` 与 `PRAGMA index_info` | 标识符始终经过 Dialect 引用 |

类型归一化只消除方言等价表达，例如 PostgreSQL `BIGSERIAL` 与 `BIGINT`、SQLite integer affinity。列名不大小写折叠，索引名、唯一性和字段数组均精确比较，避免 `messageId` 被误认成 `messageid` 或乱序索引被接受。

## 4. 差异与模式语义

每项 `Difference` 记录 `Table`、`Subject`、`Kind`、`Expected`、`Actual` 和 `Safe`。比较覆盖：缺失表、缺失或多余列、类型、nullable、主键、自增、缺失或多余索引，以及索引唯一性和有序字段差异。单表差异按 `Kind` 和 `Subject` 稳定排序。

| 模式 | 行为 | 返回结果 |
| --- | --- | --- |
| `sync` | 探测后仅执行安全 DDL，并再次探测 | 尚有差异或出现不安全差异时返回 `ValidationError` |
| `validate` | 只探测和比较 | 任意差异返回 `ValidationError` |
| `off` | 跳过业务 Descriptor 的探测和比较 | 返回空差异 `Report` |

安全差异限定为：缺失表、缺失的 nullable 列、缺失且具有明确默认值的列、缺失索引。多余列或索引、类型变化、nullable/主键/自增变化、索引定义变化均为不安全差异。`sync` 在发现任一不安全差异后不得执行部分修复。

## 5. 同步流程

`sync` 的处理顺序固定：

1. 若表不存在，调用 `driver.Dialect.Compile` 并逐条执行完整建表 DDL；
2. 若表存在，仅对安全的缺失列调用 `CompileColumn` 并执行 `ALTER TABLE ... ADD COLUMN`；
3. 仅对安全的缺失索引按 Descriptor 编译 `CREATE [UNIQUE] INDEX`；
4. 清理 GoFrame 表字段缓存；
5. `Apply` 再次探测并比较，以真实数据库状态决定最终结果。

所有表名、列名和索引名均通过 `Dialect.Quote`。数据库错误使用 `gerror.Wrap` 增加操作和对象上下文，但不构造 SQL 文本拼接入口给业务输入。

## 6. 错误与边界

- Descriptor 为 nil、表名或索引字段无效、未知模式、无效 Manager 均立即返回错误；
- 读取元数据、编译 DDL、执行 DDL 或清理缓存失败均保留底层错误链；
- 不安全差异返回 `ValidationError`，并完整保留该次比较的 `Report`；
- `off` 只跳过业务实体结构。应用启动仍须由上层调用 `driver.Probe`，并在启用基础设施时执行其运行契约探测；
- 本模块不推断或实施破坏性数据库迁移，必须交由显式 migration 流程处理。

## 7. 文件职责

| 文件 | 职责 |
| --- | --- |
| `types.go` | Mode、Table、Column、Index、Difference、Report 和 ValidationError |
| `expected.go` | Descriptor 到期望表结构和方言类型归一化 |
| `inspect.go` | 三方言实际表、字段和索引探测 |
| `diff.go` | 精确比较、稳定排序、ValidationError 文本 |
| `schema.go` | Manager 构造、三种模式、同步 DDL 和错误提取 |
| `schema_test.go` | SQLite 端到端同步、校验、拒绝破坏性差异和 off 模式 |

## 8. 测试与验收

当前测试至少覆盖：

1. SQLite 从空库创建 Descriptor 表后可通过 `validate`；
2. 可空列和索引可由 `sync` 安全补齐，随后 `validate` 成功；
3. `validate` 返回稳定排序且可 `errors.As` 的完整差异报告；
4. 现有遗留列被视为破坏性差异，`sync` 不执行变更；
5. `off` 不要求业务表存在，返回空差异；
6. 未知 Mode 被拒绝。

门禁命令：

```bash
go test ./cool-next/db/schema -count=1
go test -race ./cool-next/db/schema -count=1
go vet ./...
make check
```

## 9. 完成标准

1. 07.1-07.8 与 07.10 的业务实体 Schema 职责均由上述 API 和测试覆盖；07.9 由上层在启用基础设施时接入对应运行探测；
2. 无论方言如何，列名大小写、类型、nullable、主键、自增和索引顺序均不被模糊比较；
3. `sync` 不包含删列、缩窄类型、修改约束或其他破坏性操作；
4. `validate` 以结构化 Report 阻止不一致数据库进入启动完成状态；
5. `off` 不影响上层数据库能力和已启用基础设施的运行探测；
6. 未提前实现 Application Host、基础设施 Schema 或通用迁移框架。
