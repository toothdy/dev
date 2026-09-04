# 10 Module Declaration And Configuration Implementation Plan

**Goal:** 在 `cool-next/core/module` 提供静态模块声明、目录身份和复用模块 03 的不可变模块配置编译结果。

**Architecture:** `IdentityFromDirectory` 将 `modules/<module>` 目录规范化为唯一 Key。`Compile` 先校验 Declaration 与 Ref 语法，再把 Defaults 和已经选出的模块 `configuration.Source` 交给 `configuration.Load`，冻结元数据、配置和静态引用。

**Tech Stack:** Go 1.26、标准库 `path/filepath` / `regexp` / `strings`、模块 02 exception、模块 03 configuration、Go `testing`

## File Structure

- Create: `cool-next/core/module/declaration.go` - Declaration、ComponentRef、Ref 与引用校验
- Create: `cool-next/core/module/identity.go` - 目录 Identity 归一化
- Create: `cool-next/core/module/compile.go` - Compiled、配置加载和访问器
- Create: `cool-next/core/module/errors.go` - Core 错误包装
- Create: `cool-next/core/module/module_test.go` - 身份、声明、配置、不可变性、可重复性与并发测试

### Task 1: Identity And Declaration Contract

- [ ] 写失败测试，覆盖直接/嵌套目录、绝对路径、模块根、逃逸和非法 ComponentRef。
- [ ] 实现 Identity 与 `IdentityFromDirectory`，统一使用 `/` 分隔的相对键。
- [ ] 实现 `ComponentRef`、`Ref`、Declaration 校验：名称/描述、控制字符及两类中间件各自去重。
- [ ] 运行 `go test ./cool-next/core/module -run 'TestIdentity|TestDeclaration|TestRef' -count=1`。

### Task 2: Configuration Compilation And Immutability

- [ ] 写失败测试，覆盖 Defaults、YAML/环境覆盖、null、未知字段、类型错误与 gvalid。
- [ ] 实现 `Compile`，直接调用 `configuration.Load` 并将失败包装为 Core 异常。
- [ ] 实现 Compiled 访问器，确保 Config、Canonical JSON 和中间件切片均不泄露内部可变状态。
- [ ] 运行 `go test ./cool-next/core/module -run 'TestCompile|TestCompiled' -count=1`。

### Task 3: Determinism And Concurrency

- [ ] 写测试，比较相同输入的 Canonical JSON 和引用顺序，并在并发读取访问器时运行 race。
- [ ] 确认 Compile 不扫描目录、不按字符串解析符号、也不写全局注册表。
- [ ] 运行 `go test -race ./cool-next/core/module -count=1`。

### Task 4: Full Verification

- [ ] 运行 `gofmt -w cool-next/core/module/*.go`。
- [ ] 运行 `go test ./cool-next/core/module -count=1`、`go test ./cool-next/core/configuration -count=1`。
- [ ] 运行 `go vet ./...` 和 `make check`。
- [ ] 检查 `git diff --check`，确保提交只包含模块 10 文件。
