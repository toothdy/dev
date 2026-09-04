# 08 Transaction Scope And Runner Implementation Plan

**Goal:** 在 `cool-next/db/tx` 提供受 Context 生命周期约束的单组事务 Scope，以及唯一拥有提交权的 Runner

**Architecture:** `Runner` 在最外层显式调用 `gdb.DB.Begin`，由私有 `scope` 追踪组、事务、rollback-only、首个错误和完成状态。同组嵌套仅复用 Scope，跨组拒绝；最外层统一 Commit 或 Rollback，并在 panic 时完成 Rollback 后原样重新抛出。

**Tech Stack:** Go 1.26、GoFrame v2.10.2 `gdb`、标准库 `context` / `errors` / `sync`、Go `testing`、临时 SQLite 数据库

## File Structure

- Create: `cool-next/db/tx/types.go` - 公开 Callback、Runner、Context Key 与 Current
- Create: `cool-next/db/tx/scope.go` - 私有 Scope 状态与并发安全生命周期
- Create: `cool-next/db/tx/errors.go` - 模块 02 Core 异常包装
- Create: `cool-next/db/tx/runner.go` - 构造、嵌套分派与事务结束管理
- Create: `cool-next/db/tx/runner_test.go` - SQLite 事务、嵌套、错误、panic 与参数测试
- Create: `cool-next/db/tx/scope_test.go` - Scope 当前值、关闭和并发测试

### Task 1: Scope State And Current Contract

**Files:** `types.go`、`scope.go`、`scope_test.go`

- [ ] 写失败测试，覆盖无 Scope、有效 Scope、关闭后的 Context、首错保留、重复关闭和 `Current` 与 `close` 的并发访问。
- [ ] 定义 `Callback`、`Runner`、私有 Context Key 与 `Current`，只识别有效的本模块 Scope。
- [ ] 实现带互斥锁的 Scope：读取事务、记录首错、查询 rollback-only、幂等关闭。
- [ ] 运行 `go test -race ./cool-next/db/tx -run 'TestCurrent|TestScope' -count=1`。

### Task 2: Runner Construction And Outer Transaction

**Files:** `errors.go`、`runner.go`、`runner_test.go`

- [ ] 写失败测试，覆盖空组、nil DB、DB 组不匹配、nil callback、nil Context、成功提交和回调错误回滚。
- [ ] 用 `exception.WrapCore` 实现参数错误构造，保留 Cause。
- [ ] 实现 `NewRunner` 和不可变 `Group`；最外层 `Within` 调用 `Begin`，把新 Scope 注入派生 Context。
- [ ] 按 callback / rollback-only 结果显式 Commit 或 Rollback；数据库结束失败按设计决定原样返回或 `errors.Join`。
- [ ] 运行 `go test ./cool-next/db/tx -run 'TestRunnerConstruct|TestWithinCommit|TestWithinRollback' -count=1`。

### Task 3: Nested Scope, Panic And Lifecycle

**Files:** `runner.go`、`runner_test.go`

- [ ] 写失败测试，覆盖同组复用同一 TX、内层错误被吞掉后的 rollback-only、首错固定、跨组拒绝、panic 原值重抛，以及回调结束后的 Context 失效。
- [ ] 实现同组嵌套，不调用 `Begin`、不关闭 Scope。
- [ ] 实现跨组 Core 错误，并保证 callback 不执行。
- [ ] 在最外层 defer 中对 panic 执行 Rollback、关闭 Scope，再按原值 panic；正常路径保持首错和数据库结束错误的链。
- [ ] 运行 `go test ./cool-next/db/tx -run 'TestWithinNested|TestWithinCrossGroup|TestWithinPanic|TestWithinClosedContext' -count=1`。

### Task 4: Full Verification

**Files:** `cool-next/db/tx/*.go`

- [ ] 运行 `gofmt -w cool-next/db/tx/*.go`。
- [ ] 运行 `go test ./cool-next/db/tx -count=1`。
- [ ] 运行 `go test -race ./cool-next/db/tx -count=1`。
- [ ] 运行 `go vet ./...` 和 `make check`。
- [ ] 检查 `git diff --check`，确认不包含模块 07 的既有未提交改动。
