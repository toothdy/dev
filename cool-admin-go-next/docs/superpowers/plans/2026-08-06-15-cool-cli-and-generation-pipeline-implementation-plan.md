# 15 Cool CLI And Generation Pipeline Implementation Plan

**Goal:** 将现有源码分析、Provider 图、Descriptor 编译和静态注册表渲染能力串成共享内存流水线，提供可测试的 `cool generate/check`，并保证候选通过格式化与类型检查后才原子替换唯一生成文件。

**Architecture:** `cool-next/codegen` 负责固定路径的分析、渲染、候选 Overlay 类型检查、稳定性/新鲜度检查和原子写入；分析阶段用占位 Overlay 隔离旧生成文件，类型检查阶段用候选 Overlay 验证新文件。`cmd/cool` 只解析命令、调用共享 API 并映射 `0/1/2` 退出码。

**Tech Stack:** Go 1.26、标准库 `context` / `bytes` / `go/format` / `os` / `path/filepath`、`golang.org/x/tools/go/packages`、模块 11-14 codegen、Go `testing`

## Preconditions

- 模块 12 完成跨模块具体 Provider/`contract/**` 接口、参数位置、依赖/循环路径、模块图和稳定拓扑契约及测试。
- 模块 13 完成 Descriptor/DO 编译、元数据校验和确定性测试。
- 模块 14 完成唯一注册表内存渲染、静态 Graph 和确定性测试。
- 运行 `go test ./cool-next/codegen ./cool-next/core/module -count=1` 和对应 Race Test 均通过。
- 若任一前置契约缺失，回到对应模块修复；不在本计划中增加 CLI 特判替代其实现。

## File Structure

- Create: `cool-next/codegen/pipeline.go` - 固定路径选项、共享内存生成入口与阶段协调
- Create: `cool-next/codegen/pipeline_test.go` - 端到端生成、旧产物隔离、失败不写盘和唯一输出测试
- Create: `cool-next/codegen/typecheck.go` - 候选文件 `go/packages` Overlay 类型检查与稳定诊断
- Create: `cool-next/codegen/typecheck_test.go` - 目标存在/缺失、候选语义错误及工作区依赖错误测试
- Create: `cool-next/codegen/atomic_write.go` - 同目录临时文件与原子替换
- Create: `cool-next/codegen/atomic_write_test.go` - 写入各阶段故障注入、旧字节与临时文件清理测试
- Create: `cool-next/codegen/check.go` - 双次生成稳定性、类型检查和生成文件新鲜度检查
- Create: `cool-next/codegen/check_test.go` - missing/stale/current/unstable 和严格只读测试
- Create: `cmd/cool/main.go` - `generate/check` 命令、Context、IO 和统一退出码
- Create: `cmd/cool/main_test.go` - 帮助、用法、成功、失败和输出通道测试
- Create: `modules/modules_gen.go` - 由 `cool generate` 首次产生并提交的唯一 Cool 生成文件
- Modify: `cool-next/codegen/analyze.go` - 增加包内受控 Overlay 分析入口，保持公开 Analyze 兼容
- Modify: `cool-next/codegen/load.go` - 把受控 Overlay 传入 `packages.Config`
- Modify: `cool-next/codegen/analyze_test.go` - 增加占位 Overlay 对旧生成文件的隔离测试

本模块不修改 `README.md`、`Makefile`、`go.mod` 或业务模块源码。

### Task 1: Predecessor Gate And Fixed Pipeline Contract

**Tracks:** 15.2、15.7、15.9

- [ ] 先运行并审查模块 12-14 的专项测试，对照其设计确认跨模块依赖、Descriptor 和 Render 契约均真实实现。
- [ ] 写测试，断言流水线只接受工作区根目录，模块根固定为 `modules`，输出固定为 `modules/modules_gen.go`。
- [ ] 定义 `PipelineOptions`、`Generate` 和 `Check` 的最小公开入口；候选源码构建函数保持包内私有。
- [ ] 按固定顺序调用 `Analyze -> CompileDescriptors -> BuildGraphWithDescriptors -> Render -> format.Source`，确保生成 Descriptor Provider 参与依赖匹配，且不执行配置、Schema、构造器或数据库操作。
- [ ] 失败诊断保留原始 `DiagnosticError`，流水线新增错误使用未占用且稳定的 codegen 错误码段。
- [ ] 不增加通用规则插件、输出路径参数或未来组件占位。

Run:

```bash
go test ./cool-next/codegen -run 'TestPipeline(UsesFixedPaths|BuildsCandidate|RejectsInvalidInput)' -count=1
```

### Task 2: Isolate The Committed Output During Analysis

**Tracks:** 15.2、15.5、15.10

- [ ] 先写测试，在临时工作区放入语法损坏或引用已删除符号的 `modules/modules_gen.go`，断言流水线仍能从有效业务源码生成候选。
- [ ] 写目标文件不存在的测试，断言同一分析路径仍成功。
- [ ] 保持公开 `Analyze(ctx, Options)` 行为兼容，增加包内入口接收只读 Overlay。
- [ ] 在流水线分析阶段仅将目标绝对路径 Overlay 为 `package modules`，并把 Overlay 传给 `packages.Load`。
- [ ] 复制调用方提供的 Overlay map/字节，避免并发修改影响分析。
- [ ] 不忽略业务源码、protobuf 或第三方生成文件中的真实类型错误。

Run:

```bash
go test ./cool-next/codegen -run 'TestAnalyzeWithOverlay|TestPipelineIgnoresInvalidCommittedOutput' -count=1
```

### Task 3: Format And Type Check The In-Memory Candidate

**Tracks:** 15.3、15.5、15.10

- [ ] 先写测试，覆盖候选语法合法但导入、泛型、构造器参数或 Graph 类型不兼容的错误。
- [ ] 写目标文件已存在与不存在的测试，证明候选 Overlay 都能让 `./modules` 被完整类型检查。
- [ ] 对 Render 结果再次调用 `format.Source`，格式化失败返回稳定诊断。
- [ ] 使用独立 `packages.Load` 调用，把目标绝对路径 Overlay 为候选源码，并请求语法、类型、导入和依赖信息。
- [ ] 收集所有相关 package errors，转换为相对工作区 Position，稳定排序后返回 `DiagnosticError`。
- [ ] 类型检查期间不写目标、临时文件或其他工作区文件。
- [ ] 验证损坏旧文件不会污染候选类型检查，因为候选 Overlay 完整替代该路径。

Run:

```bash
go test ./cool-next/codegen -run 'TestTypeCheckGenerated(ExistingTarget|MissingTarget|Rejects)' -count=1
```

### Task 4: Atomic Replacement And Failure Preservation

**Tracks:** 15.4、15.5、15.10

- [ ] 先写成功测试，断言临时文件创建在 `modules/`、提交后内容精确等于候选、权限固定且无残留临时文件。
- [ ] 先写故障注入测试，分别覆盖 create、write、chmod、sync、close 和 rename 失败，逐项断言旧目标字节不变。
- [ ] 使用小型私有文件操作依赖完成确定性故障注入，不使用可并发篡改的包级函数变量。
- [ ] 目标目录只在候选全部通过后处理；使用 `os.CreateTemp(filepath.Dir(target), pattern)` 保证同文件系统。
- [ ] 按 Write、Chmod、Sync、Close、Rename 顺序提交；任何 Rename 前错误都关闭并删除临时文件。
- [ ] Rename 成功后立即返回成功，不追加可能产生“失败但目标已替换”的 fallible 步骤。
- [ ] `Generate` 只在共享流水线完整成功后调用原子写入。

Run:

```bash
go test ./cool-next/codegen -run 'TestAtomicWrite|TestGeneratePreservesCommittedOutputOnFailure' -count=1
```

### Task 5: Read-Only Stability And Freshness Check

**Tracks:** 15.6、15.7、15.8、15.9

- [ ] 先写测试，覆盖生成文件缺失、字节过期、完全最新和候选不稳定四种结果。
- [ ] 用文件系统快照断言成功和失败的 `Check` 都不创建目录、目标或临时文件，也不改变任何已有字节。
- [ ] `Check` 两次独立执行分析、Descriptor 编译、含 Descriptor Provider 的图构建、Render 和格式化，再逐字节比较候选。
- [ ] 稳定性通过后对候选执行一次 Overlay 类型检查，再读取目标进行精确字节比较。
- [ ] 缺失或不同均返回明确的新鲜度诊断；不自动调用 `Generate` 修复。
- [ ] 通过共享阶段覆盖当前已实现的实体元数据、模块配置、Provider 图和静态注册表规则。
- [ ] 写测试或源码断言，确认 `Check` 不执行 `gofmt`、`go vet`、`go test`、数据库连接或外部进程。
- [ ] 不加入路由、生命周期、鉴权、Outbox、Event、Schedule 或 Queue 猜测规则。

Run:

```bash
go test ./cool-next/codegen -run 'TestCheck(Missing|Stale|Current|Unstable|DoesNotWrite)' -count=1
```

### Task 6: Testable Cool CLI And Exit Codes

**Tracks:** 15.1、15.8、15.10

- [ ] 先写表驱动测试，覆盖无命令、未知命令、帮助、`generate` 成功/失败和 `check` current/stale。
- [ ] 固定退出码：成功和帮助为 `0`，生成或检查失败为 `1`，用法错误为 `2`。
- [ ] 将命令执行放入接收 Context、args、cwd、stdout、stderr 的 `run` 函数；测试不调用 `os.Exit`。
- [ ] `main` 只建立信号可取消 Context、取得当前目录、调用 `run` 并把返回值交给 `os.Exit`。
- [ ] 仅暴露 `generate` 和 `check`；不暴露 `build`、`run`、`outbox` 或固定成功占位。
- [ ] 成功摘要写 stdout；诊断和用法错误写 stderr；不打印配置、环境变量或完整内部缓存路径。

Run:

```bash
go test ./cmd/cool -count=1
```

### Task 7: Golden Output And End-To-End Verification

**Tracks:** 15.2-15.10

- [ ] 增加包含模块配置、实体、Schema 和普通构造器的临时工作区 Golden，验证只产生 `modules/modules_gen.go`。
- [ ] 运行 `cool generate` 生成仓库当前唯一输出，再运行第二次并确认字节无变化。
- [ ] 运行 `cool check`，确认当前文件通过稳定性、类型和新鲜度检查。
- [ ] 断言业务模块目录没有 `*_cool_gen.go`、DAO、Columns 或其他 Cool 生成文件。
- [ ] 确认测试无需数据库、应用配置、Controller、Base Service、Transport 或 Outbox。

Run:

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./cool-next/codegen ./cmd/cool -count=1
env GOCACHE=/private/tmp/cool-admin-go-build go test -race ./cool-next/codegen ./cmd/cool -count=1
go run ./cmd/cool generate
go run ./cmd/cool check
git diff --exit-code -- modules/modules_gen.go
```

### Task 8: Full Verification

- [ ] 运行 `gofmt -w` 仅格式化本计划列出的新增或修改 Go 文件。
- [ ] 运行 codegen、core/module、CLI 专项测试和全仓单元测试。
- [ ] 运行 Race Test、`go vet ./...`、当前 `make check` 与 `git diff --check`。
- [ ] 检查工作区差异只包含本计划列出的代码、测试和唯一生成文件，不修改 README、Makefile 或业务模块。
- [ ] 检查 CLI 中不存在 `build/run/outbox` 占位，codegen 中不存在未批准组件规则或多输出路径。

Run:

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
env GOCACHE=/private/tmp/cool-admin-go-build go test -race ./... -count=1
go vet ./...
make check
git diff --check
rg -n 'build|run|outbox|controller|lifecycle|grpc|event|schedule|queue' cmd/cool cool-next/codegen/pipeline.go cool-next/codegen/check.go
find modules -type f \( -name '*_cool_gen.go' -o -name 'modules_gen.go' \)
```
