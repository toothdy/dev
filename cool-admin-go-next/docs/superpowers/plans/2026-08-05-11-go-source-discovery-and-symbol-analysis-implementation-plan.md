# 11 Go Source Discovery And Symbol Analysis Implementation Plan

**Goal:** 在 `cool-next/codegen` 提供只读、可重复的 Go 模块源码分析模型，为后续依赖图和单文件生成提供 AST、类型、位置和诊断基础。

**Architecture:** `Analyze` 先在工作区文件系统中发现模块根和可参与发现的文件，再由 `go/packages` 取得完整的 Package、AST 和 types 信息。专用发现器从已筛选 AST 构建模块配置、Ref、实体、Schema 和构造器模型；所有发现错误汇总为稳定排序的位置诊断。

**Tech Stack:** Go 1.26、`golang.org/x/tools/go/packages`、标准库 `go/ast` / `go/token` / `go/types` / `path/filepath`、模块 04 entity、模块 10 module、Go `testing`

## File Structure

- Create: `cool-next/codegen/analyze.go` - Options、工作区扫描、主分析编排
- Create: `cool-next/codegen/load.go` - `go/packages` 加载和 AST/类型索引
- Create: `cool-next/codegen/model.go` - 不可变 Model、Module、声明和访问器
- Create: `cool-next/codegen/diagnostic.go` - 位置、错误码、排序和 DiagnosticError
- Create: `cool-next/codegen/module.go` - ModuleConfig 静态形状和 Ref 目标解析
- Create: `cool-next/codegen/entity.go` - 实体与 XxxSchema 发现
- Create: `cool-next/codegen/constructor.go` - New 构造器签名收集与校验
- Create: `cool-next/codegen/analyze_test.go` - 临时 Go module 工作区的端到端测试夹具
- Modify: `go.mod`, `go.sum` - 增加 `golang.org/x/tools` 的受控依赖

### Task 1: Analyzer Contract And Diagnostics

- [ ] 在测试中先定义空 modules 根、Options 路径错误和稳定 DiagnosticError 的期望。
- [ ] 增加与 Go 1.26 兼容的 `golang.org/x/tools` 依赖，并核对 `go/packages` 的实际 API。
- [ ] 实现 Options 校验、SourcePosition、Diagnostic、DiagnosticError 及确定性排序。
- [ ] 实现不可变 Model 基础访问器，确保切片和诊断不会泄露内部状态。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestAnalyzeOptions|TestDiagnostic|TestModel' -count=1`。

### Task 2: Filesystem Discovery And Package Loading

- [ ] 写失败测试，覆盖直接/嵌套模块、重叠 config.go 根、允许目录、测试/隐藏/testdata/生成/越界文件过滤。
- [ ] 实现 modules 根扫描、模块 Identity 构造、允许文件判定和稳定文件顺序。
- [ ] 使用 `packages.Load` 获取 Packages、Syntax、Types、TypesInfo 与 Imports，将 load/语法/types 错误转换为源码位置诊断。
- [ ] 建立按绝对文件、模块相对文件和 Package 路径查询的内部索引，不把排除文件写入发现集合。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestAnalyze(Empty|Discovers|Excludes|ReportsLoad)' -count=1`。

### Task 3: ModuleConfig And Reference Resolution

- [ ] 写测试，覆盖导入别名、合法的静态 Declaration 字面量、普通/全局 Ref、未知/跨模块/非函数/排除目标及其位置。
- [ ] 用 `go/types` 验证 ModuleConfig、module.Declaration 泛型实例、当前根包 Config 和静态 return 字面量。
- [ ] 收集 Middlewares 与 GlobalMiddlewares 的直接 `module.Ref` 常量字符串参数，以真实对象校验函数并解析模块内唯一包级函数。
- [ ] 为错误函数签名、非静态构造、重复 ModuleConfig 和非法 Ref 生成稳定诊断。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestAnalyze(ModuleConfig|References)' -count=1`。

### Task 4: Entity, Schema And Constructor Discovery

- [ ] 写测试，覆盖带 `g.Meta` 和 entity.Base 的实体、XxxSchema 配对、合法 `New` 返回形式及所有非法构造器返回/variadic/泛型情形。
- [ ] 在 `entity/**` 识别导出命名结构体的直接嵌入；记录实体类型与位置但不编译 Descriptor。
- [ ] 验证 XxxSchema 的命名、目标实体和精确 `entity.Schema` 签名。
- [ ] 收集允许目录的 `New`/`NewXxx` 函数参数和 `*T` 或 `(*T, error)` 返回类型，拒绝模块 12 前的无效形状但不做可注入性判断。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestAnalyze(Entities|Schemas|Constructors)' -count=1`。

### Task 5: Determinism And Full Verification

- [ ] 写测试，比较重复分析的模块、声明、Ref、构造器和诊断文本；并发读取同一成功 Model。
- [ ] 确认分析过程不读取配置、不执行 ModuleConfig、不连接数据库、不写生成文件或全局注册表。
- [ ] 运行 `gofmt -w cool-next/codegen/*.go`。
- [ ] 运行 `go test ./cool-next/codegen -count=1`、`go test -race ./cool-next/codegen -count=1`、`go test ./... -count=1`。
- [ ] 运行 `go vet ./...`、`make check`、`git diff --check`，确认提交只包含模块 11 实现、测试与依赖锁定变更。
