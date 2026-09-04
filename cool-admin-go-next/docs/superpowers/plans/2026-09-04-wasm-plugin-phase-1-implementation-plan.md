# WASM 插件平台第一阶段实施计划

> 日期：2026-09-04
> 依据：`docs/superpowers/specs/2026-09-04-wasm-plugin-platform-design.md`
> 状态：已确认

## 1. 目标

先完成不依赖数据库和后台接口的最小闭环：第三方使用 Go 1.26 SDK 注册任意方法，构建为 WASI reactor；宿主通过 wazero 校验、加载和调用；`cool-plugin` 能检查、构建和打包 `.cool`。本阶段必须用真实 Go WASM 制品验证，不用 Mock 替代 Runtime。

本阶段完成后可证明以下关键假设：

1. `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared` 能稳定导出 `cool.plugin/v1` ABI；
2. Guest 和 Host 能按线性内存、长度及响应句柄安全交换 JSON；
3. 任意方法、配置、Ready、Shutdown 和 Host API 原始调用链可用；
4. 超时、取消、Trap、非法 ABI、越界内存和超大载荷不会终止宿主进程；
5. `.cool` 可复现打包，并能被下一阶段安装服务直接复用。

本阶段不创建 `modules/plugin`，不接数据库、后台接口、Hook Resolver、多节点同步或现有上传业务。这些属于后续阶段。

## 2. 已核对依据

- Go 1.26 本地官方文档：`cmd/compile/doc.go` 的 `//go:wasmimport`、`//go:wasmexport` 类型规则；
- Go 1.26 本地官方文档：`cmd/go/internal/help/helpdoc.go` 的 wasip1 `buildmode=c-shared` reactor 语义；
- wazero v1.12.0 官方源码：`Runtime.CompileModule`、`Runtime.InstantiateModule`、`RuntimeConfig.WithMemoryLimitPages`、`RuntimeConfig.WithCloseOnContextDone`、WASI 和 Host Module API；
- 当前项目 Go Module、错误封装、测试、命令行和架构门禁惯例。

依赖只新增 `github.com/tetratelabs/wazero v1.12.0`。修改 `go.mod` 和 `go.sum` 时保留当前工作区已有的验证码依赖改动，不覆盖、不回退。

## 3. 包边界

```text
cool-next/plugin/
├── abi/       # Host 与 Guest 共用的常量、签名和 Envelope
├── sdk/       # 插件作者 API、Guest 生命周期、内存与 Host 调用
├── runtime/   # wazero Host、ABI 校验、编译实例和调用
└── artifact/  # plugin.json 校验与 .cool ZIP 读写

cmd/cool-plugin/              # init/check/test/build/pack
cool-next/plugin/testdata/echo/ # 真实 Go WASI 测试插件
```

依赖方向固定为：

```text
abi <- sdk
abi <- runtime <- wazero
abi <- artifact
sdk + artifact <- cmd/cool-plugin
```

`sdk` 不导入 wazero、GoFrame、数据库或宿主模块，保证插件制品只带必要代码。`runtime` 不导入 `modules/**`，以后由静态 Plugin Manager 组合它。

## 4. 任务与顺序

### 任务 0：保护工作区并固定基线

文件：无。

步骤：

1. 每次修改前检查 `git status --short`；
2. 保留当前 `.gitignore`、`go.mod`、`go.sum`、验证码服务和测试的未提交改动；
3. 插件相关文件单独暂存和提交；`go.mod`、`go.sum` 只有在能确认补丁同时保留用户改动时才纳入；
4. 记录当前 codegen 测试已有的 `gnmodule`、`gnoutbox` 等 fixture 包别名失败，不将其误判为插件回归。

验证：

```bash
git status --short
go test ./... -run '^$'
```

### 任务 1：建立共享 ABI 契约

生产文件：

- 创建：`cool-next/plugin/abi/abi.go`
- 创建：`cool-next/plugin/abi/envelope.go`

测试文件：

- 创建：`cool-next/plugin/abi/abi_test.go`
- 创建：`cool-next/plugin/abi/envelope_test.go`

步骤：

1. 固定 ABI 名称 `cool.plugin/v1`、整数版本 `1`、Host 模块名和所有导入导出名称；
2. 用不可变签名表表达每个函数的 i32/i64 参数与结果，Host 校验直接复用；
3. 定义成功和失败 JSON Envelope，以及带稳定错误码的插件错误；
4. Envelope 只接受合法 JSON data，错误消息不携带底层堆栈；
5. 测试常量、签名、成功/失败编解码和非法 JSON。

验证：

```bash
go test ./cool-next/plugin/abi -count=1
```

### 任务 2：实现 Guest SDK 与固定 Bridge

生产文件：

- 创建：`cool-next/plugin/sdk/definition.go`
- 创建：`cool-next/plugin/sdk/context.go`
- 创建：`cool-next/plugin/sdk/runtime.go`
- 创建：`cool-next/plugin/sdk/memory.go`
- 创建：`cool-next/plugin/sdk/host_wasip1.go`
- 创建：`cool-next/plugin/sdk/host_native.go`
- 创建：`cool-next/plugin/sdk/bridge.go`

测试文件：

- 创建：`cool-next/plugin/sdk/definition_test.go`
- 创建：`cool-next/plugin/sdk/runtime_test.go`

步骤：

1. 提供 `Define`、泛型 `Method`、`RawMethod`、`Ready`、`Shutdown` 和 `Register`；
2. 方法名按设计正则校验并拒绝重复；每个实例只允许注册一次 Definition；
3. `cool_init` 保存配置副本并执行一次 Ready，`cool_invoke` 按名称调用，`cool_shutdown` 只执行一次 Shutdown；
4. Handler Context 携带 invocation ID 和不可变配置，提供泛型 `Config[T]`；
5. 提供原始 `HostCall`，wasip1 文件使用 `//go:wasmimport`，native 文件返回明确的不支持错误，便于普通 Go 单测；
6. Guest 输入先复制再解析；分配区和响应区按句柄持有，drop/free 幂等且不可读取已释放内容；
7. Bridge 暴露供插件 `main` 包薄包装的固定函数，CLI 模板只负责添加 `//go:wasmexport`，不复制状态逻辑；
8. 用普通 Go 单测覆盖类型化方法、RawMethod、配置、生命周期、未知方法、Panic 转失败 Envelope 和所有权释放。

验证：

```bash
go test ./cool-next/plugin/sdk -count=1
GOOS=wasip1 GOARCH=wasm go test -c -o /tmp/cool-plugin-sdk.test.wasm ./cool-next/plugin/sdk
```

### 任务 3：实现最小 wazero Host Runtime

生产文件：

- 修改：`go.mod`
- 修改：`go.sum`
- 创建：`cool-next/plugin/runtime/config.go`
- 创建：`cool-next/plugin/runtime/runtime.go`
- 创建：`cool-next/plugin/runtime/compiled.go`
- 创建：`cool-next/plugin/runtime/instance.go`
- 创建：`cool-next/plugin/runtime/response.go`

测试与夹具：

- 创建：`cool-next/plugin/runtime/runtime_test.go`
- 创建：`cool-next/plugin/runtime/abi_test.go`
- 创建：`cool-next/plugin/testdata/echo/main.go`
- 创建：`cool-next/plugin/testdata/echo/plugin.go`

步骤：

1. Runtime 使用 `WithMemoryLimitPages` 和 `WithCloseOnContextDone(true)`，实例化 WASI 与唯一 `cool_host` 模块；
2. `Compile` 校验 WASM magic、必需导出及精确签名，只允许 WASI 和 `cool_host` 导入；
3. `Compiled.Instantiate` 使用空模块名和 `_initialize`，允许同一 CompiledModule 创建多个独立实例；
4. Instance 初始化、调用和关闭都使用统一的内存写入、函数调用、响应复制和 drop 流程；
5. Host 响应句柄绑定调用模块，Guest 不能读取其他实例的句柄；调用结束清理未 drop 句柄；
6. Host handler 接收 operation 与 JSON，返回统一 Envelope；首期只提供单个显式函数入口，不建立 Service Locator；
7. 将 context deadline、模块关闭和 wazero Trap 映射为稳定插件错误，底层错误仅作为 cause 保留；
8. 真实构建 echo reactor，验证 Ready、任意方法、配置、HostCall、Shutdown、重复实例和编译复用；
9. 增加非法导出、非法导入、超大输入输出、越界、Panic/Trap、超时取消和关闭后调用测试。

验证：

```bash
go test ./cool-next/plugin/runtime -count=1
go test -race ./cool-next/plugin/runtime -count=1
```

### 任务 4：实现 Manifest 与 `.cool` 制品工具

生产文件：

- 创建：`cool-next/plugin/artifact/manifest.go`
- 创建：`cool-next/plugin/artifact/archive.go`

测试文件：

- 创建：`cool-next/plugin/artifact/manifest_test.go`
- 创建：`cool-next/plugin/artifact/archive_test.go`

步骤：

1. 使用 `encoding/json.Decoder.DisallowUnknownFields` 严格读取 schemaVersion 1 Manifest；
2. 使用标准库校验 key、hook、版本、路径、必需文件、条目数、压缩大小和累计解压大小；
3. 禁止绝对路径、反斜杠、父目录跳转、符号链接、设备文件和重复规范路径；
4. 计算完整 `.cool` SHA-256，并返回 Manifest、WASM 与可选资源；
5. Pack 使用稳定路径顺序、固定时间戳和固定权限生成可复现 ZIP；
6. 覆盖合法包、未知字段、重复条目、路径穿越、超限和两次打包哈希一致测试。

验证：

```bash
go test ./cool-next/plugin/artifact -count=1
```

### 任务 5：实现 `cool-plugin` CLI

生产文件：

- 创建：`cmd/cool-plugin/main.go`
- 创建：`cmd/cool-plugin/command.go`
- 创建：`cmd/cool-plugin/project.go`
- 创建：`cmd/cool-plugin/template.go`

测试文件：

- 创建：`cmd/cool-plugin/main_test.go`
- 创建：`cmd/cool-plugin/project_test.go`

步骤：

1. `init` 创建最小 Go 插件、`plugin.json`、Bridge、README 和独立 `go.mod`；已存在非空目录时拒绝覆盖；
2. CLI 从 Go BuildInfo 取得自身模块版本：正式版本写入对应 SDK require；源码开发版本写入 `v0.0.0` require 和指向当前仓库的本地 replace，使生成项目可立即构建；
3. `check` 调用 artifact 与 SDK 规则，检查目录、Manifest 和 Go 工具链；
4. `test` 使用 `exec.CommandContext` 执行 `go test ./...`，原样转发输出；
5. `build` 先 check/test，再固定使用 Go 1.26 wasip1 c-shared 参数输出 `plugin.wasm`；
6. `pack` 在 build 成功后生成 `<key>_v<version>.cool`，再用 artifact reader 自校验；
7. 命令返回稳定退出码，取消时停止子进程，不引入 Cobra 等额外依赖；
8. 测试帮助、参数错误、初始化防覆盖、源码/发布版本依赖、子命令顺序、构建参数和可复现打包。

验证：

```bash
go test ./cmd/cool-plugin -count=1
go run ./cmd/cool-plugin init --module example.com/cool/plugin-smoke /tmp/cool-plugin-smoke
go run ./cmd/cool-plugin pack /tmp/cool-plugin-smoke
```

冒烟目录实际使用 `mktemp -d` 创建，不依赖固定路径，也不删除用户目录。

### 任务 6：端到端与阶段门禁

文件：

- 可能修改：`Makefile`，只增加插件专项检查目标；若现有 `check` 可直接覆盖则不修改。

步骤：

1. 用 CLI 构建并打包 echo 插件；
2. 用 artifact reader 打开 `.cool`；
3. 用 Host Runtime 编译、初始化、调用、HostCall 和 Shutdown；
4. 第二次构建包，比较 SHA-256；
5. 运行插件专项、Race、Vet、格式、Module 和架构检查；
6. 运行全量测试并与任务 0 基线比较，禁止新增失败。

验证：

```bash
go test ./cool-next/plugin/... ./cmd/cool-plugin -count=1
go test -race ./cool-next/plugin/... ./cmd/cool-plugin -count=1
go vet ./cool-next/plugin/... ./cmd/cool-plugin
go mod tidy -diff
go mod verify
test -z "$(gofmt -l cool-next/plugin cmd/cool-plugin)"
go test ./test/architecture -count=1
go test ./... -run '^$'
git diff --check
```

## 5. 提交边界

按以下顺序形成小提交，每个提交通过对应专项测试后再继续：

1. `feat: 定义 WASM 插件 ABI 与 Guest SDK`
2. `feat: 实现 WASM 插件宿主运行时`
3. `feat: 实现 COOL 插件制品协议`
4. `feat: 添加 Go 插件开发 CLI`

如果工作区中的验证码改动尚未提交，包含 `go.mod`、`go.sum` 的第二个提交只暂存 wazero 相关增量和必要校验后的完整文件，不暂存验证码业务文件。

## 6. 停止条件

出现以下任一情况时停止当前任务，不用绕行方案扩大范围：

1. Go 1.26 c-shared reactor 无法通过固定 Bridge 暴露设计 ABI；
2. wazero 无法在 Context 超时后终止并关闭无限执行实例；
3. Go Guest 内存无法满足明确的 alloc/free 与 response/drop 所有权；
4. 为完成首期闭环必须修改静态 Module Graph、路由或数据库；
5. 用户现有未提交改动与必要依赖文件产生无法无损合并的冲突。

停止时保留最后一个通过测试的提交，说明根因和最小替代方案，不提交半完成阶段。

## 7. 完成标准

1. SDK API、ABI、Runtime、artifact 和 CLI 专项测试全部通过；
2. 真实 Go 1.26 WASI reactor 可被宿主无重启编译、实例化、调用和关闭；
3. `.cool` 两次构建字节一致，非法包和非法 WASM 在进入后续安装服务前被拒绝；
4. 超时、Trap、Panic、越界和超限均返回稳定错误，宿主测试进程继续运行；
5. 没有动态路由、数据库、运行时 DI 或与第一阶段无关的改动；
6. 全量编译检查通过，现有 codegen 基线失败没有增加。
