# 09 Framework Database Group Implementation Plan

**Goal:** 在 `cool-next/db` 建立唯一 Framework Database Group Runtime，集中提供同一 DB、Dialect、Runner、Scope 查询和无敏感信息启动诊断。

**Architecture:** Runtime 接收上层从 `cool.outbox.databaseGroup` 和 `database.<group>` 解析出的 Group、GoFrame Nodes 与交易表集合。构造时注册具名组、获取 DB、执行 `driver.Probe` 并创建 `tx.Runner`；之后 Runtime 只读且不允许换组。

**Tech Stack:** Go 1.26、GoFrame v2.10.2 `gdb`、模块 02 exception、模块 06 driver、模块 08 tx、临时 SQLite 数据库

## File Structure

- Create: `cool-next/db/runtime.go` - Config、Runtime、启动流程、访问器和 Scope 查询
- Create: `cool-next/db/diagnostic.go` - 脱敏启动诊断与防御性副本
- Create: `cool-next/db/validate.go` - Config、表集合和 Core 错误校验
- Create: `cool-next/db/runtime_test.go` - Runtime 启动、Scope、诊断、错误和并发测试

### Task 1: Config Validation And Diagnostics

- [ ] 写失败测试，覆盖空 Group、空 Nodes、非法/重复表名，以及诊断切片隔离。
- [ ] 实现 `Config`、`Diagnostic`、Core 错误包装和防御性复制。
- [ ] 使用模块 06 单段标识符规则校验交易表集合，固定排序和去重语义。
- [ ] 运行 `go test ./cool-next/db -run 'TestValidate|TestDiagnostic' -count=1`。

### Task 2: Runtime Construction And Accessors

- [ ] 写失败测试，覆盖临时 SQLite 具名组的 Runtime 构造、Group/DB/Dialect/Runner 一致性与 Probe 结果。
- [ ] 克隆 Nodes 后注册 GoFrame Group，获取同名 Instance，验证 GetGroup，执行 Probe，并用同一 DB 创建 Runner。
- [ ] 实现只读访问器；Diagnostic 仅保留 Group、Kind、Version、Capabilities 和表名。
- [ ] 运行 `go test ./cool-next/db -run 'TestNewRuntime|TestRuntimeAccessors' -count=1`。

### Task 3: Scope Consistency And Transaction Integration

- [ ] 写测试，覆盖无 Scope、同组 Scope、跨组 Scope、已关闭 Scope，以及 Runner 实际提交和回滚。
- [ ] 实现 `Runtime.Current`：只返回同组 TX，不存在时无错误，跨组时 Core 错误。
- [ ] 验证 Runtime 的 DB 与 Runner 指向同一具名 GoFrame 组。
- [ ] 运行 `go test ./cool-next/db -run 'TestRuntimeCurrent|TestRuntimeRunner' -count=1`。

### Task 4: Full Verification

- [ ] 运行 `gofmt -w cool-next/db/*.go`。
- [ ] 运行 `go test ./cool-next/db -count=1`。
- [ ] 运行 `go test -race ./cool-next/db -count=1`。
- [ ] 运行 `go vet ./...` 和 `make check`。
- [ ] 检查 `git diff --check`，确认提交只包含模块 09 文件。
