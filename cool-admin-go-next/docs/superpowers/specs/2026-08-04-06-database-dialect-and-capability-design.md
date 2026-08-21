# 06 数据库方言与能力探测设计

## 1. 目标

在 `cool-next/db/driver` 建立 MySQL 8.x、PostgreSQL 9.5+ 和 SQLite 3.24+ 的最小方言层，向后续 Schema、Runtime 和 Store 模块提供：

- 数据库类型与版本识别
- 标识符安全引用
- 实体元数据到 DDL 的确定性编译
- 真实连接上的事务、条件写入和锁能力探测
- MySQL 默认引擎与已声明业务表的 InnoDB 校验

本模块不执行业务 DML，不比对现存 Schema，不负责迁移编排。

## 2. 边界与依赖

- 输入使用 `core/entity.Metadata`、`Field` 和 `Index`，不反射业务实体
- 数据库访问仅依赖 GoFrame `gdb.DB`
- 方言层只表达 DDL 和能力差异，不包含 CRUD、查询条件或业务表写入
- 能力探测可以建立随机命名的 `cool_probe_*` 内部表并写入探测行，完成后必须删除
- 不在本模块引入第三方 SQL Builder

## 3. 公开模型

```go
type Kind string

const (
    MySQL      Kind = "mysql"
    PostgreSQL Kind = "postgresql"
    SQLite     Kind = "sqlite"
)

type Version struct {
    Major int
    Minor int
    Patch int
}

type Capabilities struct {
    Transactions     bool
    ConditionalWrite bool
    RowLock          bool
    SkipLocked       bool
    NativeComments   bool
}

type Report struct {
    Dialect      Dialect
    Version      Version
    Capabilities Capabilities
}

type DDL struct {
    CreateTable string
    Comments    []string
    Indexes     []string
}
```

`Dialect` 是只包含私有 `Kind` 的不可变值。公开入口：

- `New(kind Kind) (Dialect, error)`
- `Dialect.Kind() Kind`
- `Dialect.Quote(identifier string) (string, error)`
- `Dialect.ColumnType(field entity.Field) (string, error)`
- `Dialect.Compile(metadata entity.Metadata) (DDL, error)`
- `DDL.Statements() []string`
- `ParseVersion(raw string) (Version, error)`
- `ValidateVersion(kind Kind, raw string) (Version, error)`
- `Probe(ctx context.Context, database gdb.DB, transactionTables ...string) (Report, error)`

`DDL.Statements` 按建表、表/列注释、索引的顺序返回新切片，不泄露内部状态。

## 4. 数据库识别与版本

`Probe` 先从 `database.GetConfig().Type` 识别驱动：

| GoFrame type | Kind | 版本查询 | 最低版本 |
|---|---|---|---|
| `mysql` | MySQL | `SELECT VERSION()` | 仅 8.x |
| `pgsql` | PostgreSQL | `SHOW server_version` | 9.5+ |
| `sqlite` | SQLite | `SELECT sqlite_version()` | 3.24+ |

版本解析从服务器字符串中取第一个 `major.minor[.patch]`，缺失 patch 时使用 0。未知驱动、无法解析的版本和低于基线的版本均立即失败。MariaDB 不按 MySQL 8 兼容实现接受。

## 5. 标识符引用

- 仅接受 `[A-Za-z_][A-Za-z0-9_]*` 单段标识符
- MySQL 使用反引号
- PostgreSQL 和 SQLite 使用双引号
- 表名、列名、索引名统一引用，因此 PostgreSQL `camelCase` 列始终保留大小写
- 不接受带点路径、已引用文本或 SQL 片段

## 6. DDL 类型映射

指针字段先解引用，再根据 Go `Kind` 保留整数宽度。

| 逻辑类型 | MySQL | PostgreSQL | SQLite |
|---|---|---|---|
| bool | `BOOLEAN` | `BOOLEAN` | `INTEGER` + 0/1 CHECK |
| int8/int16/int32/int64 | `TINYINT/SMALLINT/INT/BIGINT` | `SMALLINT/SMALLINT/INTEGER/BIGINT` | `INTEGER` |
| uint8/uint16/uint32/uint64 | 对应整数 `UNSIGNED` | `SMALLINT/INTEGER/BIGINT/NUMERIC(20,0)` | `INTEGER` |
| float32/float64 | `FLOAT/DOUBLE` | `REAL/DOUBLE PRECISION` | `REAL` |
| float + precision | `DECIMAL(p,s)` | `NUMERIC(p,s)` | `NUMERIC(p,s)` |
| string | `TEXT` | `TEXT` | `TEXT` |
| string + size | `VARCHAR(n)` | `VARCHAR(n)` | `TEXT` + length CHECK |
| bytes | `BLOB` | `BYTEA` | `BLOB` |
| bytes + size | `VARBINARY(n)` | `BYTEA` + octet_length CHECK | `BLOB` + length CHECK |
| time | `DATETIME(6)` | `TIMESTAMP(6) WITHOUT TIME ZONE` | `DATETIME` |

Go `int`/`uint` 按 `strconv.IntSize` 选择 32 或 64 位映射。MySQL `DECIMAL` 限制 precision <= 65、scale <= 30；PostgreSQL 限制 precision <= 1000；超界约束返回错误。

## 7. 列、默认值与注释

- 非空字段生成 `NOT NULL`，可空字段不生成冗余 `NULL`
- MySQL 自增主键使用 `BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`
- PostgreSQL 自增主键使用 9.5 可用的 `BIGSERIAL PRIMARY KEY`
- SQLite 自增主键使用 `INTEGER PRIMARY KEY AUTOINCREMENT`
- 自增只允许用于非空的整数主键
- bool、有符号整数、无符号整数和浮点默认值必须通过对应 Go 解析
- string 默认值作为 SQL 字面量转义，不作为 SQL 片段；MySQL 8.0.0 基线不支持 `TEXT DEFAULT`，因此 MySQL 的 string 默认值必须同时声明 size
- time 默认值仅允许不区分大小写的 `CURRENT_TIMESTAMP`
- bytes 默认值因三库语义不一致而拒绝
- MySQL 在列定义和表选项中保留注释
- PostgreSQL 使用独立 `COMMENT ON TABLE/COLUMN`
- SQLite 无原生 Schema Comment，不生成注释 SQL

所有注释和字符串默认值使用单引号倍增转义；MySQL 额外倍增反斜杠，避免默认 SQL Mode 下改变字面量边界。

## 8. 建表与索引

`Compile` 保留 `Metadata.Fields()` 和 `Metadata.Indexes()` 的顺序，生成可重复比对的 SQL：

1. `CREATE TABLE` 包含全部列和主键约束
2. PostgreSQL 表注释和按字段顺序排列的列注释
3. 按元数据顺序排列的普通/唯一索引

MySQL 建表语句固定包含 `ENGINE=InnoDB`。索引字段使用逻辑名回查实际列名，未知字段立即失败。

## 9. 能力探测

`Probe` 的顺序固定：

1. 识别驱动、读取并校验版本
2. MySQL 校验 `@@default_storage_engine` 为 InnoDB
3. 建立随机命名的内部探测表
4. 在事务中写入一行并主动返回哨兵错误，验证回滚后行不存在
5. PostgreSQL/SQLite 先使用 `ON CONFLICT DO NOTHING` 验证重复插入 `RowsAffected=0`；三库再执行带期望旧值的条件 `UPDATE`，验证首次 `RowsAffected=1`、过期条件 `RowsAffected=0`；MySQL 不在方言层使用 `INSERT IGNORE`
6. MySQL/PostgreSQL 在事务内执行 `SELECT ... FOR UPDATE SKIP LOCKED`，SQLite 报告无行锁和 Skip Locked
7. 删除探测表
8. MySQL 查询 `information_schema.TABLES`，校验调用方声明的交易表全部为 InnoDB

探测表名使用密码学随机后缀，只通过已校验的标识符进入 SQL。清理使用保留 Context Value 但解除取消信号的 5 秒独立超时 Context；清理失败在主流程成功时返回，不吞掉残留表风险。

`Capabilities` 的预期结果：

| 能力 | MySQL | PostgreSQL | SQLite |
|---|---:|---:|---:|
| Transactions | true | true | true |
| ConditionalWrite | true | true | true |
| RowLock | true | true | false |
| SkipLocked | true | true | false |
| NativeComments | true | true | false |

## 10. 错误与安全

- 错误使用 GoFrame `gerror` 增加操作上下文
- 错误不包含连接密码或完整 DSN
- 任何标识符和默认值都不允许绕过结构化校验
- `Probe` 接收的交易表名先完成标识符校验，再查询系统表

## 11. 验收

- 单元测试覆盖三种版本基线、非法输入、标识符引用、全逻辑类型、约束、默认值、注释与索引
- 三数据库集成测试运行同一 `Probe` 契约并执行生成 DDL
- 对 MySQL 额外验证非 InnoDB 表被拒绝
- `go test ./cool-next/db/driver -count=1`
- `go test -race ./cool-next/db/driver -count=1`
- `go vet ./...`
- `make check`
- `test/integration/run.sh`
