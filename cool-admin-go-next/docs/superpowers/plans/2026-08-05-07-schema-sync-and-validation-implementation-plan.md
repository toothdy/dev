# 模块 07：Schema 同步与校验实施计划

**Goal:** 基于实体 Descriptor 构建、探测、比较和安全同步业务数据库 Schema，并在不一致时提供确定的结构化报告。

**Architecture:** `cool-next/db/schema` 保持为 Descriptor 与 `driver.Dialect` 之间的编排层。期望结构和实际结构都映射为独立值对象 `Table`，差异比较不依赖数据库连接；`Manager` 只在 `sync` 模式执行经过方言引用和编译的安全 DDL。

**Tech Stack:** Go 1.26、GoFrame v2 `gdb`、模块 04 `entity.Metadata`、模块 06 `driver.Dialect`、SQLite 临时文件测试

## File Structure

- Create: `cool-next/db/schema/types.go` - Mode、规范化结构模型、差异报告和 ValidationError
- Create: `cool-next/db/schema/expected.go` - Descriptor 到期望 Table 的编译和类型归一化
- Create: `cool-next/db/schema/inspect.go` - MySQL、PostgreSQL、SQLite 的实际结构探测
- Create: `cool-next/db/schema/diff.go` - 精确比较、稳定差异排序和可读错误文本
- Create: `cool-next/db/schema/schema.go` - Manager、模式分派、安全同步和 Differences
- Create: `cool-next/db/schema/schema_test.go` - SQLite 端到端模式和安全边界测试
- Modify: `cool-next/db/driver/ddl.go` - 导出单字段 `CompileColumn` 供安全加列复用

### Task 1: Schema Values And Expected Tables

- [x] 定义 `Sync`、`Validate`、`Off` 及不可变的 Column、Index、Table、Difference、Report。
- [x] 从 Descriptor 字段和索引生成期望表，逻辑索引字段转换为物理列名。
- [x] 按方言归一化等价类型，同时保留列名、唯一性和字段顺序。
- [x] 拒绝 nil Descriptor、无效表名和引用未知字段的索引。

### Task 2: Actual Schema Inspection

- [x] 使用 GoFrame `Tables` 与 `TableFields` 读取实际表和列。
- [x] 读取 MySQL `information_schema.STATISTICS`、PostgreSQL 系统目录和 SQLite PRAGMA 索引信息。
- [x] 跳过主键索引，保留普通/唯一索引及其列顺序。
- [x] 对 SQLite PRAGMA 表与索引标识符使用 Dialect 引用。

### Task 3: Difference Report And Validation

- [x] 比较表存在性、列集合、类型、nullable、主键、自增、索引集合、唯一性与列顺序。
- [x] 标记缺失表、缺失 nullable 列、默认值列和缺失索引为可安全修复的候选。
- [x] 把删列、缩窄或修改类型/约束、索引定义变化标记为不安全。
- [x] 按 Kind 和 Subject 固定差异顺序，并通过 `ValidationError` 与 `Differences` 对外暴露完整 Report。

### Task 4: Mode Handling And Safe Sync

- [x] 实现 `New` 与 `Apply`，拒绝无效 Manager 和未知 Mode，并规范化 nil Context。
- [x] `off` 直接返回空业务表差异，不探测 Descriptor 表。
- [x] `validate` 只比较，任一差异返回 `ValidationError`。
- [x] `sync` 先拒绝任一不安全差异，再创建缺失表、添加安全缺失列和创建缺失索引。
- [x] 复用 `driver.Compile`、`driver.CompileColumn` 和 `Dialect.Quote`，同步后清理 GoFrame 缓存并重新比较。

### Task 5: Verification

- [x] 使用 SQLite 临时文件验证从空库建表、补充 nullable 列和索引后可通过 validate。
- [x] 验证 validate 的差异顺序稳定且可由 `errors.As` 提取。
- [x] 验证遗留列等破坏性差异拒绝 sync，off 跳过业务表，非法模式失败。
- [x] 执行 `go test ./cool-next/db/schema -count=1` 和 `go test -race ./cool-next/db/schema -count=1`。
- [x] 执行 `go vet ./...`、`make check` 与 `git diff --check`。

## Completion Notes

实现已于 `5181518 feat: 实现模块 07 Schema 管理器与 CompileColumn 导出` 完成。本计划补充该实现的范围、文件职责和验证证据；模块 09 仍负责在所有 Schema Mode 下执行 `driver.Probe`，基础设施表的运行结构探测由后续对应模块承担。
