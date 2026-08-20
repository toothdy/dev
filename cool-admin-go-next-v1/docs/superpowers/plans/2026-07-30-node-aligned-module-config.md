# Node 对齐的模块配置与集中装配实施计划

> **执行者约束：** 本计划只描述后续实施步骤。后续 agent 必须按任务依赖顺序执行，每次先写失败测试，再做最小实现，再运行聚焦验证。保留工作区中所有既有修改；不得回滚、覆盖或清理用户改动；不得执行 `git reset`、`git checkout --`、`git clean`；不得 `git add` 或创建提交。

**目标：** 以 `docs/superpowers/specs/2026-07-30-node-aligned-module-config-design.md` 为唯一设计依据，把四个模块迁移为根 `config.go` 声明、`module.<key>` 统一配置加载、项目级依赖图和唯一 `modules/modules_gen.go` 集中装配。

**当前基线：** Go 1.26.0、GoFrame v2.10.2、`golang.org/x/tools` v0.48.0；现有生成器已经具备目录扫描、`go/types` 签名分析、单模块图、Overlay 类型检查、原子单文件写入和 `cool generate/check/build/run`，但仍生成四个模块内 `module_gen.go`、支持 `--module`、识别根 `LoadConfig` 与 `handler/**`，并只把跨模块 Model 暴露给单模块图。

**不变量：** 不引入运行时反射 DI、Service Locator、`init()`、空白模块导入、`module.yaml`、注释注册协议或运行时源码扫描。配置解码允许仅在启动阶段使用反射；请求路径仍是静态函数调用。所有 Go 文件修改后运行 `gofmt`，依赖变更只使用 `go mod tidy`，不手工编辑生成代码。

## 依赖与并行边界

| 阶段 | 任务 | 关系 |
| --- | --- | --- |
| 框架契约 | Task 1 -> Task 2 -> Task 3 | 必须串行；后一步依赖前一步的类型和启动语义 |
| 生成器发现 | Task 4 -> Task 5 -> Task 6 -> Task 7 | 必须串行；声明、引用、全局图逐层建立 |
| 输出链 | Task 8 -> Task 9 -> Task 10 | 必须串行；先确定单输出，再做事务写入，最后收紧 CLI |
| 业务迁移 | Task 11、12、13、14 | Task 10 后可并行；不得在各任务中生成或编辑 `modules/modules_gen.go` |
| 最终切换 | Task 15 -> Task 16 -> Task 17 | 必须串行；等待四个模块迁移全部完成 |

并行执行时，每个 agent 只修改自己任务列出的模块目录；共享文件 `cool/**`、`cmd/cool/**`、`modules/modules_gen.go`、`modules/modules_test.go`、`manifest/config/config.yaml`、README、CI 和文档由对应串行任务独占。

## Task 1：建立模块声明值契约

**Files**

- Create: `cool/module/declaration.go`
- Create: `cool/module/declaration_test.go`

**先写失败测试**

在 `cool/module/declaration_test.go` 增加：

- `TestDeclarationCarriesMetadataMiddlewareRefsAndDefaults`：断言 `Declaration[T]` 精确保留 `Name`、`Description`、`Order`、`Middlewares`、`GlobalMiddlewares`、`Defaults`。
- `TestComponentRefIsACompileTimeStringValue`：断言 `ComponentRef("middleware#Definition")` 可作为声明字段值，且不包含运行时解析行为。
- 编译期断言 `ModuleConfig() module.Declaration[Config]` 能使用具体配置类型，不能退化为 `any` 或 `map[string]interface{}`。

先运行：

```bash
go test ./cool/module -run 'Declaration|ComponentRef' -count=1
```

预期：因 `Declaration`、`ComponentRef` 尚不存在而编译失败。

**最小实现**

- 在 `cool/module/declaration.go` 定义 `type ComponentRef string` 和泛型 `Declaration[T any]`，字段严格为设计中的六项。
- 保留 `cool/module` 现有运行时 `Definition`/`Module` API，当前任务不做无关重构。
- 不在声明类型中加入模块 Key、DB/Menu 路径、Loader、缓存或运行时依赖。

**聚焦验证**

```bash
gofmt -w cool/module/declaration.go cool/module/declaration_test.go
go test ./cool/module -run 'Declaration|ComponentRef|DefinitionConfig' -count=1
```

预期：声明测试与原 `cool/module` 配置骨架测试同时通过。

## Task 2：实现唯一泛型配置加载器

**Files**

- Create: `cool/module/config_loader.go`
- Create: `cool/module/config_loader_test.go`

**先写失败测试**

在 `config_loader_test.go` 使用 `gcfg.NewAdapterContent` 覆盖：

- `TestLoadConfigUsesDeepCopiedDefaults`：缺少 `module.sample` 时返回默认值；连续两次加载的 Slice、Map、Pointer 不共享底层状态。
- `TestLoadConfigRecursivelyOverlaysExplicitZeroValues`：嵌套部分覆盖保留未配置默认值，显式 `false`、`0`、`""`、`[]` 均覆盖默认值。
- `TestLoadConfigRejectsInvalidRootUnknownFieldTypeAndNull`：标量/数组根、未知字段、错误类型、非空类型 `null` 失败；Pointer/Map/Slice 的 `null` 成功。
- `TestLoadConfigUsesJSONTagsAndDuration`：只认 `json` 标签，正确转换 `time.Duration`，错误包含 `module.sample.<field>` 完整路径。
- `TestLoadConfigCallsValueValidator`：只调用 `Validate() error` 值接收者并包装模块 Key；验证失败不返回半成品。
- `TestLoadConfigDoesNotReadLegacyPrefix`：只配置顶层 `sample.*` 时仍使用默认值。

先运行：

```bash
go test ./cool/module -run 'LoadConfig' -count=1
```

预期：因泛型 Loader 尚不存在而编译失败。

**最小实现**

- 在 `cool/module/config_loader.go` 提供唯一泛型入口 `LoadConfig[T](ctx, key, defaults) (T, error)`；读取且只读取 `module.<key>`。
- 先深复制默认值，再以 `json` 标签为唯一键递归覆盖；拒绝未知字段和非法根类型，保留显式零值，并按设计处理 `null`。
- 支持标准纯值、嵌套 Struct、Pointer、Slice、Map 与 `time.Duration`；错误逐层补齐模块 Key 和字段路径。
- 加载完成后检测 `interface{ Validate() error }`；不接受会修改配置的指针接收者作为约定。
- 反射只存在于该启动期解码文件，不缓存全局配置，不发现或构造业务组件。

**聚焦验证**

```bash
gofmt -w cool/module/config_loader.go cool/module/config_loader_test.go
go test ./cool/module -run 'LoadConfig' -count=1
go test -race ./cool/module -run 'LoadConfigUsesDeepCopiedDefaults' -count=1
```

预期：全部配置边界测试通过，Race 不报告共享默认值。

## Task 3：增加 ModuleSpec 配置准备阶段并前置 Application 副作用

**Files**

- Modify: `cool/registry/registry.go`
- Modify: `cool/registry/dependencies.go`
- Create: `cool/registry/module_spec_test.go`
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Modify: `cool/app/middleware_test.go`

**先写失败测试**

- `registry.ModuleSpec` 增加编译期断言：`Configure func(context.Context) error`。
- `TestBuildConfiguresEveryModuleBeforeFactories`：按传入的最终模块顺序调用全部 `Configure`，之后才允许 Recycle、Runtime、Controller、Middleware。
- `TestBuildStopsBeforeSideEffectsWhenConfigureFails`：配置失败后不得创建 Runtime/Controller/Middleware，不得同步 Schema、Seed 或注册路由；错误包含“准备模块配置”和模块 Key。
- `TestBuildConfiguresModuleWithoutConfigConsumers`：即使没有组件参数依赖 Config，也必须执行 `Configure`。
- `TestBuildCopiesModuleSpecsIncludingConfigure`：保留现有切片复制语义。
- 为应用级依赖写类型断言：`AuthOptions`、`I18nOptions`、`CRUDOptions`、`RedisDefaultConfig` 均是命名结构，不以裸 Bool/String/Map 注入。

先运行：

```bash
go test ./cool/registry ./cool/app -run 'ModuleSpec|Configure|Configures|SideEffects' -count=1
```

预期：`Configure` 字段及前置阶段不存在，测试失败。

**最小实现**

- 给 `registry.ModuleSpec` 增加 `Configure`，并在 `dependencies.go` 定义应用级命名依赖：认证、i18n、CRUD soft-delete 与 `redis.default` 的强类型快照。
- `BuildWithContext` 选定并复制 Specs 后，先加载应用级配置，再按 Specs 顺序执行全部非 nil `Configure`；只有全部成功才创建上传目录、Session、Recycle、Runtime、Controller、中间件、Schema、Seed 或路由。
- 保持现有显式 `ModuleSpecs`、重复/空 Key、nil Controller 校验和 Runtime 回滚语义；只移动必要副作用到配置准备之后。
- 将命名应用依赖加入 `RuntimeDeps`、`Deps`、`MiddlewareDeps`，生成器后续只按这些具体类型绑定。

**聚焦验证**

```bash
gofmt -w cool/registry/registry.go cool/registry/dependencies.go cool/registry/module_spec_test.go cool/app/app.go cool/app/app_test.go cool/app/middleware_test.go
go test ./cool/registry ./cool/app -run 'ModuleSpec|Configure|Configures|SideEffects|ExplicitModuleSpecs|Runtime' -count=1
go test -race ./cool/registry ./cool/app -run 'Configure|Runtime' -count=1
```

预期：配置准备严格先于所有模块与服务器副作用；旧显式 Specs 和 Runtime 测试继续通过。

## Task 4：收紧 Scanner 为模块根 allowlist

**Files**

- Modify: `cool/codegen/module/scanner.go`
- Modify: `cool/codegen/module/scanner_test.go`
- Modify: `cool/codegen/module/types.go`
- Create: `cool/codegen/module/testdata/scanner/valid/modules/sample/config.go`
- Create: `cool/codegen/module/testdata/scanner/invalid_root/**`

**先写失败测试**

- `TestScanRequiresRootConfigGo`：模块缺 `config.go` 失败。
- `TestScanAcceptsCompleteModuleRootAllowlist`：只接受根 `config.go`、可选 `db.json`/`menu.json`，以及 `controller`、`dto`、`entity`、`middleware`、`schedule`、`service`、`event`、`queue`。
- `TestScanRejectsUnknownRootEntry`：分别拒绝 `config/`、`handler/`、`db/`、`hooks/`、`job/`、`providers.go`、无标准 Header 的 `module_gen.go`、根测试 `.go` 和任意其他根文件/目录，错误含模块 Key 与相对路径。
- `TestScanToleratesGeneratedModuleGenOnlyForCleanup`：带标准 Header 的旧 `module_gen.go` 不进入包分析，但作为 Writer 待清理产物保留；`check` 仍必须报告它陈旧。
- `TestScanLoadsWholeProjectOnly`：扫描选项不再接受模块 Key 子集。
- 保留平台文件、隐藏目录、`testdata`、生成文件识别测试，但 `dto/**` 也纳入可移植组件目录检查。

先运行：

```bash
go test ./cool/codegen/module -run 'Scan|Discover|RootAllowlist|Portable' -count=1
```

预期：现有 Scanner 接受非法根项、缺少 `config.go`，测试失败。

**最小实现**

- 用固定文件 allowlist 和固定八目录 allowlist 校验每个直接模块子目录；不通过扩展名或“未知目录忽略”放宽协议。唯一迁移例外是带标准 Header 的旧 `module_gen.go`，它只可进入陈旧文件清理清单，不属于最终目录协议。
- 删除 `ScanOptions.ModuleKeys` 和 `DiscoverModules` 的 selected 分支；每次 `packages.Load` 覆盖全部模块。
- 根 `modules/modules_gen.go` 不是模块子目录内容，不参与模块 allowlist。
- 测试 fixture 的模块根统一改成合法 `config.go`，不再用 `sample.go`/`base.go` 占位。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/scanner.go cool/codegen/module/scanner_test.go cool/codegen/module/types.go cool/codegen/module/testdata/scanner/valid/modules/sample/config.go
go test ./cool/codegen/module -run 'Scan|Discover|RootAllowlist|Portable' -count=1
```

预期：合法八目录项目可扫描，任何未准入根职责在加载业务包前失败。

## Task 5：静态分析 ModuleConfig 与 Config 类型

**Files**

- Modify: `cool/codegen/module/types.go`
- Modify: `cool/codegen/module/signature.go`
- Modify: `cool/codegen/module/analyzer.go`
- Modify: `cool/codegen/module/analyzer_test.go`
- Create: `cool/codegen/module/declaration.go`
- Create: `cool/codegen/module/declaration_test.go`
- Create: `cool/codegen/module/testdata/declaration/**`

**先写失败测试**

- 成功解析根 `config.go` 的 `ModuleConfig() module.Declaration[Config]`，静态提取 Name、Description、Order、Defaults 和两类中间件引用。
- 分别拒绝缺失/重复 `ModuleConfig`、有参数、错误泛型实参、非当前根包具体 `Config`、非直接复合字面量、空 Name/Description、无法常量求值字段。
- 校验所有可外部配置导出字段有唯一非空且非 `-` 的 `json` 标签；递归拒绝函数、Channel、锁、DB、Redis Client 等运行时资源类型。
- 拒绝根 `config.go` 导入当前模块子包、调用 `g.Cfg()` 或声明 `New*`/Controller/Runtime 等根装配职责。
- 删除 `LoadConfig` 组件识别测试，并断言同名函数不再成为 Provider。

先运行：

```bash
go test ./cool/codegen/module -run 'ModuleConfig|Declaration|ConfigTag|ConfigType|RootConfig' -count=1
```

预期：分析结果没有声明模型，仍识别 `LoadConfig`，测试失败。

**最小实现**

- 新增内部 `ModuleDeclaration` 分析结构，保存模块 Key、元信息、配置 `types.Type`、Defaults AST、组件引用和源码位置。
- 通过 AST 与 `go/types` 验证 `ModuleConfig` 的静态形态；生成器不执行该函数。
- `Analysis` 必须持有且仅持有一个声明；删除 `ComponentConfig`/根 `LoadConfig` Provider 分支。
- 错误统一包含模块 Key、源码位置、包/符号、原因和修复方向。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/types.go cool/codegen/module/signature.go cool/codegen/module/analyzer.go cool/codegen/module/analyzer_test.go cool/codegen/module/declaration.go cool/codegen/module/declaration_test.go
go test ./cool/codegen/module -run 'ModuleConfig|Declaration|ConfigTag|ConfigType|RootConfig|Analyze' -count=1
```

预期：声明静态形态和配置纯值约束全部通过，`LoadConfig` 不再进入组件集。

## Task 6：按声明解析 Middleware，并从 service 发现 Task 定义

**Files**

- Modify: `cool/codegen/module/analyzer.go`
- Modify: `cool/codegen/module/analyzer_test.go`
- Modify: `cool/codegen/module/signature.go`
- Modify: `cool/codegen/module/types.go`
- Create: `cool/codegen/module/component_ref.go`
- Create: `cool/codegen/module/component_ref_test.go`
- Create: `cool/codegen/module/testdata/component_ref/**`

**先写失败测试**

- `service/demo.go` 中返回 `task.HandlerDefinition` 或 `(task.HandlerDefinition, error)` 的导出函数被优先分类为 Task 定义，即使函数名不是 `New*`。
- `service/**` 下 Task 定义拒绝可变参数、错误第二返回值和不可解析依赖；`service/**` 外任何返回 `task.HandlerDefinition` 的导出函数都失败。
- `handler/**` 不再扫描且由 Task 4 根 allowlist 直接拒绝。
- `ComponentRef` 只接受 `<相对包>#<导出函数>`；验证符号存在、签名、作用域、重复引用、漏声明工厂。
- `Middlewares` 只能引用普通 `middleware/**`，`GlobalMiddlewares` 只能引用 `middleware/global/**`；每个被发现工厂恰好声明一次。

先运行：

```bash
go test ./cool/codegen/module -run 'HandlerDefinition|ServiceHandler|ComponentRef|MiddlewareReference' -count=1
```

预期：现有实现只识别 `handler/**` 且自动纳入全部 Middleware，测试失败。

**最小实现**

- 在普通 Provider 命名判断之前检查 `service/**` 首个返回类型是否精确为 `task.HandlerDefinition`。
- 对其他目录做返回类型守卫，防止放错目录被静默忽略。
- 用已加载包与 `go/types` 解析声明引用，生成组件只保留声明顺序和经验证的具体符号；运行时不解析字符串。
- 保持 Middleware 工厂可返回一个定义或定义切片，且可带 error。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/analyzer.go cool/codegen/module/analyzer_test.go cool/codegen/module/signature.go cool/codegen/module/types.go cool/codegen/module/component_ref.go cool/codegen/module/component_ref_test.go
go test ./cool/codegen/module -run 'HandlerDefinition|ServiceHandler|ComponentRef|MiddlewareReference|Analyze' -count=1
```

预期：Task 定义只从 `service/**` 进入组件集，Middleware 发现与声明严格一一对应。

## Task 7：建立全局项目图与模块拓扑

**Files**

- Modify: `cool/codegen/module/graph.go`
- Modify: `cool/codegen/module/graph_test.go`
- Modify: `cool/codegen/module/types.go`
- Create: `cool/codegen/module/project_graph_test.go`
- Create: `cool/codegen/module/testdata/graph/**`

**先写失败测试**

- 全局图包含每个模块 Config、Model、普通 Provider、Task 定义、Runtime、Recycle Provider、Controller、Middleware 和框架命名依赖。
- 模块 A 可唯一注入模块 B 的 Config、Model、具体 Provider 或唯一接口实现；重复输出、零实现、多实现仍失败。
- Config 参数只能精确绑定同类型 Config 节点；原始标量仍失败；`registry.ControllerProvider` 仍是唯一 Lazy 边界。
- 跨模块构造依赖产生 `B -> A`；组件循环错误输出完整符号链，模块循环另输出完整模块链，Go import cycle 继续保留 `packages` 链。
- 全局只允许一个 `*recycle.Manager` Provider。
- 模块稳定拓扑：依赖优先；同一 ready 集按 Order 降序、Key 升序。用 `base(10)`、`dict(0)`、`recycle(0)`、`task(0)` 及逆 Order 依赖样例验证。

先运行：

```bash
go test ./cool/codegen/module -run 'ProjectGraph|CrossModule|ModuleOrder|ModuleCycle|Cycle|Binding|RecycleProvider' -count=1
```

预期：单模块图只能共享 Model，无法跨模块解析普通 Provider/Config，也没有模块拓扑，测试失败。

**最小实现**

- 用一个 `ProjectGraph` 替代 `[]*Graph`；节点 ID 继续使用完整 Import Path 与符号，Config 节点使用模块根具体类型。
- 全局解析 assignability、模型参数名、唯一接口和框架依赖，再从跨模块组件边派生模块边。
- 分别执行组件稳定拓扑与带 Order 优先级的模块稳定拓扑；错误保留两层链路。
- 删除 `ExternalModels` 和“逐模块图加全局 Model”的过渡实现。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/graph.go cool/codegen/module/graph_test.go cool/codegen/module/project_graph_test.go cool/codegen/module/types.go
go test ./cool/codegen/module -run 'ProjectGraph|CrossModule|ModuleOrder|ModuleCycle|Cycle|Binding|RecycleProvider' -count=1
```

预期：所有组件由单一全局图解析，模块顺序稳定且依赖永远覆盖 Order。

## Task 8：Renderer 只生成 modules/modules_gen.go

**Files**

- Modify: `cool/codegen/module/renderer.go`
- Modify: `cool/codegen/module/renderer_test.go`
- Modify: `cool/codegen/module/types.go`
- Create: `cool/codegen/module/testdata/golden/modules_gen.go`

**先写失败测试**

- `TestRenderProjectProducesOnlyGlobalFile`：候选列表长度为 1，路径严格为 `modules/modules_gen.go`；模块目录无候选输出。
- Golden 覆盖四类 assembly、Config 缓存与 `Configure`、跨模块 Provider、Models、Task 定义、Runtime、Recycle、Controller、两类 Middleware、DB/Menu 与 `Specs()`。
- `Specs()` 每次创建独立 assembly，配置 Slice/Map/Pointer 与 provider `sync.Once` 状态不共享。
- 输出按模块拓扑与组件拓扑稳定，不含 `Module()`、`init()`、空白导入、绝对路径、时间戳、`reflect` 或第三方 DI。
- 完整候选通过 `go/format` 和 `packages.Config.Overlay`；创建顺序变化时字节完全一致。

先运行：

```bash
go test ./cool/codegen/module -run 'Render|Golden|Overlay|Deterministic|OnlyGlobal' -count=1
```

预期：现有 Renderer 返回每模块文件加全局二级清单，测试失败。

**最小实现**

- 合并 `RenderModule` 与 `RenderGlobal` 为单一项目 Renderer；生成包为 `modules`，可同时导入模块根配置包和全部业务子包。
- 每个模块生成独立 assembly；`Configure` 调用 `module.LoadConfig(ctx, key, declaration.Defaults)` 一次并缓存值/错误，所有 Config 参数绑定该缓存。
- `ModuleConfig()` 只用于生成声明值；最终 `ModuleSpec` 的 Key 由目录产生，Name/Description/Order/中间件来自静态声明，DB/Menu 由文件存在性产生。
- `Specs()` 采用全局模块顺序创建所有 Specs，Runtime/Controller/Seed 遍历复用该顺序。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/renderer.go cool/codegen/module/renderer_test.go cool/codegen/module/types.go
go test ./cool/codegen/module -run 'Render|Golden|Overlay|Deterministic|OnlyGlobal' -count=1
```

预期：内存候选只有一个全局文件，且完整 Overlay 类型检查通过。

## Task 9：Writer 事务化替换全局文件并清理旧模块生成文件

**Files**

- Modify: `cool/codegen/module/writer.go`
- Modify: `cool/codegen/module/writer_test.go`
- Modify: `cool/codegen/module/command.go`
- Modify: `cool/codegen/module/command_test.go`

**先写失败测试**

- `TestReconcileStagesBeforeChangingWorkspace`：候选验证或预检失败时，全局旧文件与所有模块旧文件字节不变。
- `TestReconcileRemovesOnlyGeneratedModuleFiles`：成功后删除 `modules/*/module_gen.go`，但只允许带标准 Header 的文件。
- `TestReconcileRefusesHandwrittenModuleGenWithoutPartialChanges`：任一旧文件无 Header 时，既不替换全局文件，也不删除其他旧文件。
- `TestReconcileRollsBackRenameFailure`：注入 rename/remove 失败时恢复旧全局文件和已暂存旧模块文件。
- `TestReconcileKeepsMtimeForUnchangedGlobalAndStillRemovesStale`：内容未变不改写 mtime，但仍清理合法陈旧模块文件。
- `TestCheckReportsStaleWithoutWriting`：`check` 同时报告全局漂移与旧模块文件，磁盘不变。

先运行：

```bash
go test ./cool/codegen/module -run 'Writer|Reconcile|Stale|Preserves|Mtime|Check' -count=1
```

预期：当前逐文件写入后再删除，无法对整个文件集合回滚，测试失败。

**最小实现**

- 在调用 Writer 前完成扫描、分析、渲染和 Overlay 类型检查。
- Writer 先预检全部旧 `module_gen.go` Header，再在同目录写候选临时文件；把旧全局与旧模块文件原子 rename 到临时备份，最后 rename 新全局文件。
- 任一步失败都按逆序恢复备份；全部替换成功后才删除备份。不得把该机制泛化到 `config/`、`handler/`、`providers.go` 等手写路径。
- `check` 只比较候选并枚举带/不带 Header 的旧生成路径，不创建临时文件。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/writer.go cool/codegen/module/writer_test.go cool/codegen/module/command.go cool/codegen/module/command_test.go
go test ./cool/codegen/module -run 'Writer|Reconcile|Stale|Preserves|Mtime|Check' -count=1
```

预期：生成失败或文件保护失败时零可见变更；成功时只剩全局生成文件。

## Task 10：取消 --module 并强制所有命令全项目分析

**Files**

- Modify: `cmd/cool/main.go`
- Modify: `cmd/cool/main_test.go`
- Modify: `cool/codegen/module/command.go`
- Modify: `cool/codegen/module/command_test.go`
- Modify: `cool/codegen/module/scanner.go`
- Modify: `cool/codegen/module/scanner_test.go`
- Modify: `cool/codegen/module/types.go`

**先写失败测试**

- `TestRunRejectsModuleFlagForEveryCommand`：`generate/check/build/run --module dict` 都返回参数错误码 2，且不扫描、不写盘、不启动子进程。
- `TestGenerateAlwaysBuildsWholeProject`：任一模块非法使 `generate` 失败，即使调用方试图构造旧局部选项。
- `TestBuildAndRunGenerateBeforeChildProcess`、`TestCheckDoesNotWrite` 和子进程退出码/信号透传测试继续通过。

先运行：

```bash
go test ./cmd/cool ./cool/codegen/module -run 'ModuleFlag|WholeProject|ChildProcess|CheckDoesNotWrite' -count=1
```

预期：`generate --module` 仍被接受，测试失败。

**最小实现**

- 删除 CLI `module` Flag、`GenerateOptions.ModuleKeys`、`ScanOptions.ModuleKeys` 及全部 partial 分支。
- `generate`、`check`、`build`、`run` 均调用同一全项目 Build/Validate；`check` 只比较，`build/run` 先完整 reconcile 再执行 Go 子进程。
- 不保留隐藏环境变量、内部函数参数或测试专用局部生成入口。

**聚焦验证**

```bash
gofmt -w cmd/cool/main.go cmd/cool/main_test.go cool/codegen/module/command.go cool/codegen/module/command_test.go cool/codegen/module/scanner.go cool/codegen/module/scanner_test.go cool/codegen/module/types.go
go test ./cmd/cool ./cool/codegen/module -run 'ModuleFlag|WholeProject|ChildProcess|CheckDoesNotWrite|FindProjectRoot' -count=1
```

预期：四个命令只有全量模式，旧局部 API 从编译面消失。

## Task 11：迁移 Base 根声明、配置消费者与根测试

**并行性：** Task 11-14 可并行；本任务不得修改 `modules/modules_gen.go`、`modules/modules_test.go`、manifest、README 或 CI。

**Files**

- Modify: `modules/base/config.go`
- Modify: `modules/base/event/log.go`
- Modify: `modules/base/event/log_test.go`
- Modify: `modules/base/middleware/global/middleware.go`
- Modify: `modules/base/middleware/global/middleware_test.go`
- Modify: `modules/base/controller/app/comm.go`
- Modify: `modules/base/service/sys/login.go`
- Modify: `modules/base/service/sys/constructor_test.go`
- Modify: `modules/base/service/sys/constructors_helpers_test.go`
- Move: `modules/base/auth_integration_test.go` -> `modules/base/service/integration/auth_integration_test.go`
- Move: `modules/base/auth_security_integration_test.go` -> `modules/base/service/integration/auth_security_integration_test.go`
- Move: `modules/base/base_test.go` -> `modules/base/service/integration/module_test.go`
- Move: `modules/base/config_test.go` -> `modules/base/service/integration/config_contract_test.go`
- Move: `modules/base/controllers_test.go` -> `modules/base/service/integration/controllers_test.go`
- Move: `modules/base/custom_api_integration_test.go` -> `modules/base/service/integration/custom_api_integration_test.go`
- Move: `modules/base/eps_integration_test.go` -> `modules/base/service/integration/eps_integration_test.go`
- Move: `modules/base/eps_test.go` -> `modules/base/service/integration/eps_test.go`
- Move: `modules/base/generated_spec_test.go` -> `modules/base/service/integration/generated_spec_test.go`
- Move: `modules/base/module_fixtures_test.go` -> `modules/base/service/integration/module_fixtures_test.go`
- Move: `modules/base/permission_integration_test.go` -> `modules/base/service/integration/permission_integration_test.go`
- Move: `modules/base/providers_test.go` -> `modules/base/service/integration/providers_test.go`
- Delete: `modules/base/config/config.go`
- Delete: `modules/base/providers.go`

**先写失败测试**

- `ModuleConfig()` 精确声明 Node 元信息：Name `权限管理`、Description `基础的权限管理功能，包括登录，权限校验`、Order `10`。
- Defaults 只含 `allowKeys` 与 Base 自有 Middleware/日志参数；所有导出字段有 `json` 标签；不含 SSO、JWT、i18n 等应用配置。
- GlobalMiddlewares 精确列出 `middleware/global#TranslateDefinition`、`middleware/global#AuthorityDefinitions`、`middleware/global#LogDefinition`；三者保持现有 `base.translate`、`base.authority`/`base.permission`、`base.log` 行为和顺序。
- Base 子包只导入根 `modules/base` Config；根 `config.go` 不导入子包、不调用 `g.Cfg()`。
- Log Runtime 构造位于 `event`，Auth 使用 `registry.AuthOptions`，i18n 使用 `registry.I18nOptions`；现有 Controller、十个 Model、DB/Menu、EPS 和中间件快照不变。

先运行：

```bash
go test ./modules/base/... -run 'ModuleConfig|Config|Middleware|Constructor|Generated|Controller|EPS' -count=1
```

预期：声明和新中间件工厂不存在，子包仍依赖 `modules/base/config`，测试失败。

**最小实现**

- 把配置类型、默认常量、值接收者 `Validate() error` 与 `ModuleConfig()` 全部合并到根 `config.go`；删除 `LoadConfig`。
- 将 `NewLog` 桥接逻辑移入 `event/log.go`；把三类全局中间件拆成上述三个声明工厂，保留 Authority 内部同时产生认证和权限 Definition 的现有 Go 行为。
- 将所有 `baseConfig.Config` 改为根 `base.Config`，应用级 SSO/i18n 只从 registry 命名依赖取得。
- 移动根测试而不删除行为覆盖；更新查找 Spec 的 helper 为从 `modules.Specs()` 按 Key 查找。

**聚焦验证**

```bash
rg --files modules/base -g '*.go' -g '!module_gen.go' | xargs gofmt -w
go test ./modules/base/... -run 'ModuleConfig|Config|Middleware|Constructor|Generated|Controller|EPS' -count=1
go test -race ./modules/base/event ./modules/base/middleware/global -count=1
```

预期：Base 业务行为测试通过，模块根生产文件只剩 `config.go`，且无根测试 `.go`。

## Task 12：迁移 Dict 空配置声明与根测试

**并行性：** 可与 Task 11、13、14 并行；不得修改共享收口文件。

**Files**

- Create: `modules/dict/config.go`
- Move: `modules/dict/base_test.go` -> `modules/dict/service/integration/module_test.go`
- Move: `modules/dict/dict_integration_test.go` -> `modules/dict/service/integration/dict_integration_test.go`
- Move: `modules/dict/generated_spec_test.go` -> `modules/dict/service/integration/generated_spec_test.go`
- Modify: `modules/dict/service/integration/*_test.go`

**先写失败测试**

- `ModuleConfig()` 返回 `module.Declaration[Config]`，`Config` 是空纯值结构。
- 元信息精确为 Name `字典管理`、Description `数据字典等`、Order `0`；两类 Middleware 引用均为空。
- 现有两个 Model、两个 Controller、CRUD/路由快照与 `db.json` 路径不变。
- 根目录除 `config.go`、`db.json` 与允许业务目录外无其他文件。

先运行：

```bash
go test ./modules/dict/... -run 'ModuleConfig|Generated|Module|Controller' -count=1
```

预期：Dict 缺少 `config.go` 和声明，测试失败。

**最小实现**

- 新增根 `Config struct{}` 与直接返回复合字面量的 `ModuleConfig()`；不制造无意义业务字段或 Loader。
- 将根测试移入 `service/integration`，从全局 `modules.Specs()` 获取 Dict Spec；修正资源路径为仓库根定位，不依赖测试当前目录。
- 不修改已有 Service/Controller 构造图。

**聚焦验证**

```bash
rg --files modules/dict -g '*.go' -g '!module_gen.go' | xargs gofmt -w
go test ./modules/dict/... -run 'ModuleConfig|Generated|Module|Controller' -count=1
```

预期：Dict 声明与既有快照通过，模块根没有测试 `.go`。

## Task 13：迁移 Recycle 配置与业务 Provider

**并行性：** 可与 Task 11、12、14 并行；不得修改共享收口文件。

**Files**

- Modify: `modules/recycle/config.go`
- Modify: `modules/recycle/event/data.go`
- Modify: `modules/recycle/event/constructor_test.go`
- Modify: `modules/recycle/service/data.go`
- Modify: `modules/recycle/service/constructor_test.go`
- Modify: `modules/recycle/schedule/data.go`
- Modify: `modules/recycle/schedule/data_test.go`
- Move: `modules/recycle/base_test.go` -> `modules/recycle/service/integration/module_test.go`
- Move: `modules/recycle/config_test.go` -> `modules/recycle/service/config_contract_test.go`
- Move: `modules/recycle/generated_spec_test.go` -> `modules/recycle/service/integration/generated_spec_test.go`
- Move: `modules/recycle/providers_test.go` -> `modules/recycle/service/providers_test.go`
- Delete: `modules/recycle/config/config.go`
- Delete: `modules/recycle/providers.go`

**先写失败测试**

- 元信息精确为 Name `数据回收`、Description `收集被删除的数据，管理和恢复`、Order `0`，无 Middleware 引用。
- Defaults 只含 `cleanupInterval=24h`、`cleanupBatch=500`、`lockName=cool-admin:recycle:cleanup`；不含 `SoftDelete`。
- `Validate()` 值接收者覆盖周期、批量和锁名；所有字段使用 `json` 标签。
- Catalog 与唯一 Manager Provider 位于 `service` 或 `event`；Manager 从 `registry.CRUDOptions` 获得应用级 soft-delete 状态。
- `DataService` 与 `schedule.Data` 注入根 `recycle.Config`；两个 Model、Controller、Runtime、DB 与唯一 Manager 行为不变。

先运行：

```bash
go test ./modules/recycle/... -run 'ModuleConfig|Config|Provider|Constructor|Generated|Schedule' -count=1
```

预期：配置仍含 SoftDelete，Provider 在根目录，测试失败。

**最小实现**

- 合并配置类型、Defaults、Validate、ModuleConfig 到根 `config.go` 并删除 `LoadConfig`。
- 将 `Catalog`、`NewCatalog`、`NewManager` 移到其业务拥有包；保持全局唯一 `*recycle.Manager` 输出。
- 子包统一导入根 `modules/recycle` Config；应用级 soft-delete 不进入业务 Config。
- 移动并保留根测试行为覆盖，Spec helper 改从全局 `modules.Specs()` 取值。

**聚焦验证**

```bash
rg --files modules/recycle -g '*.go' -g '!module_gen.go' | xargs gofmt -w
go test ./modules/recycle/... -run 'ModuleConfig|Config|Provider|Constructor|Generated|Schedule' -count=1
go test -race ./modules/recycle/event ./modules/recycle/schedule -count=1
```

预期：Recycle 根只承担声明，业务构造链唯一，既有生命周期测试通过。

## Task 14：迁移 Task 配置、基础设施和 service HandlerDefinition

**并行性：** 可与 Task 11-13 并行；不得修改共享收口文件或 manifest。

**Files**

- Modify: `modules/task/config.go`
- Modify: `modules/task/event/comm.go`
- Modify: `modules/task/event/comm_test.go`
- Create: `modules/task/service/location.go`
- Create: `modules/task/service/location_test.go`
- Modify: `modules/task/service/demo.go`
- Create: `modules/task/service/demo_test.go`
- Create: `modules/task/queue/redis.go`
- Move: `modules/task/redis_test.go` -> `modules/task/queue/redis_test.go`
- Modify: `modules/task/middleware/task.go`
- Move: `modules/task/config_test.go` -> `modules/task/service/config_contract_test.go`
- Move: `modules/task/eps_test.go` -> `modules/task/service/integration/eps_test.go`
- Move: `modules/task/generated_spec_test.go` -> `modules/task/service/integration/generated_spec_test.go`
- Move: `modules/task/recycle_integration_test.go` -> `modules/task/service/integration/recycle_integration_test.go`
- Move: `modules/task/redis_integration_test.go` -> `modules/task/service/integration/redis_integration_test.go`
- Move: `modules/task/seed_test.go` -> `modules/task/service/integration/seed_test.go`
- Move: `modules/task/task_integration_test.go` -> `modules/task/service/integration/task_integration_test.go`
- Delete: `modules/task/config/config.go`
- Delete: `modules/task/config/config_test.go`
- Delete: `modules/task/handler/demo.go`
- Delete: `modules/task/handler/demo_test.go`

**先写失败测试**

- 元信息精确为 Name `任务调度`、Description `任务调度模块，支持分布式任务，由redis整个集群的任务`、Order `0`；Middlewares 只引用 `middleware#Definition`。
- Config 纯值字段为 Mode、Timezone、Log、Execution、Queue，全部有 `json` 标签；Defaults 保持当前业务值；不含 `Location`、Redis Client 或 `HasRedis`。
- `Validate()` 是值接收者，只做纯校验；错误键统一为 `module.task.*`。
- 只配置旧顶层 `task.*` 不生效；`module.task.*` 的显式零值/错误值由统一 Loader 处理。
- `service.DemoDefinition()` 返回 `task.HandlerDefinition` 且调用同文件演示执行函数；删除 `handler/**` 后仍被 Analyzer 发现。
- Location 由 `service.NewLocation(task.Config)` 派生；Redis Client 由 `queue.NewRedisClient(registry.RedisDefaultConfig)` 创建；`event.NewComm` 注入这些资源并保持 Local/Redis/Auto 业务选择与关闭语义。

先运行：

```bash
go test ./modules/task/... -run 'ModuleConfig|Config|DemoDefinition|Location|RedisClient|NewComm|Generated' -count=1
```

预期：Task Config 含运行时资源，旧键仍生效，Task 定义仍在 `handler/`，测试失败。

**最小实现**

- 把所有纯配置类型、默认值、值接收者 Validate 与 ModuleConfig 合并到根 `config.go`；删除 `LoadConfig`、`taskDuration`、`LoadRedisClient`。
- 在 `service/demo.go` 共置执行函数和 `DemoDefinition`，避免同名冲突并保持 `taskDemoService.test` 协议。
- 将时区和 Redis 创建拆到上述业务 Provider；Config 不持有可变资源，Comm 仍负责已创建 Redis Client 的生命周期。
- 子包统一导入根 `modules/task` Config；所有测试错误期望与配置 fixture 改成 `module.task.*`。
- 移动根测试后用仓库根解析 DB/Seed 路径，不依赖当前测试目录。

**聚焦验证**

```bash
rg --files modules/task -g '*.go' -g '!module_gen.go' | xargs gofmt -w
go test ./modules/task/... -run 'ModuleConfig|Config|DemoDefinition|Location|RedisClient|NewComm|Generated' -count=1
go test -race ./modules/task/event ./modules/task/queue ./modules/task/service -count=1
```

预期：Task 纯配置、派生资源、任务注册和 Runtime 行为通过；根无测试文件、`config/` 或 `handler/`。

## Task 15：一次性切换唯一生成文件与配置键

**前置条件：** Task 11-14 全部完成。此任务必须独占共享生成输出。

**Files**

- Modify (generated only by command): `modules/modules_gen.go`
- Modify: `modules/modules_test.go`
- Modify: `cool/app/app_test.go`
- Modify: `cool/app/middleware_test.go`
- Modify: `main.go`
- Modify: `manifest/config/config.yaml`
- Delete: `modules/base/module_gen.go`
- Delete: `modules/dict/module_gen.go`
- Delete: `modules/recycle/module_gen.go`
- Delete: `modules/task/module_gen.go`

**先写失败测试**

- `modules.Specs()` 返回依赖优先、同级 Order 降序/Key 升序的四个 Spec；每次调用的 assembly、Config、Models 与缓存状态互不共享。
- 四模块元信息、Middleware 引用结果、Model/Controller/Runtime/Task Handler/DB/Menu 快照符合各模块任务。
- `Application` 在配置失败时不构造任何组件；成功时统一最终顺序用于 Configure、Runtime、Controller、Schema 和 Seed。
- `manifest/config/config.yaml` 不存在顶层 `task:`，完整 Task 示例位于 `module.task`；`redis.default`、`cool.crud.softDelete` 等仍为应用级。
- 源码树不存在模块内 `config/`、`handler/`、`providers.go`、`module_gen.go`、根测试 `.go`。

先运行：

```bash
go test ./modules ./cool/app -run 'Specs|Module|Configure|Middleware|Config' -count=1
go run ./cmd/cool check
```

预期：旧 `modules_gen.go` 仍调用模块 `Module()`，旧模块生成文件存在，check 失败。

**最小实现**

- 先修改 manifest，把原顶层 Task 配置原样移入 `module.task`，不保留旧键或 fallback。
- 运行一次 `go run ./cmd/cool generate`；只允许命令生成 `modules/modules_gen.go` 并由 Writer 删除四个旧模块文件。
- 更新共享 Specs/Application 测试为新集中装配入口；`main.go` 继续显式 `app.Run(context.Background(), modules.Specs())`，不增加兼容注册链。
- 使用 `rg` 确认所有手写禁用路径已由模块迁移任务明确删除，而非由 Writer 删除。

**聚焦验证**

```bash
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./modules ./cool/app -run 'Specs|Module|Configure|Middleware|Config' -count=1
test -z "$(find modules -mindepth 2 -maxdepth 2 -type d \( -name config -o -name handler -o -name db -o -name hooks -o -name job \) -print)"
test -z "$(find modules -mindepth 2 -maxdepth 2 -type f \( -name providers.go -o -name module_gen.go -o -name register.go -o -name '*_test.go' \) -print)"
```

预期：check 通过，项目唯一生成装配文件为 `modules/modules_gen.go`，旧 Task 配置键与所有禁用根职责消失。

## Task 16：更新文档、CI 与静态 Guards

**Files**

- Modify: `README.md`
- Modify: `docs/module-development.md`
- Modify: `.github/workflows/go.yml`
- Modify: `cool/codegen/module/guards_test.go`
- Create: `cool/codegen/module/layout_guard_test.go`

**先写失败测试**

- Guard 断言只有 `modules/modules_gen.go` 带标准生成 Header；生成代码不含 `reflect`、第三方 DI、`init()`、空白导入或 `.Module()`。
- Layout Guard 断言每模块恰有根 `config.go`，根目录只含允许项，且全仓无模块内 `config/`、`handler/`、`db/`、`hooks/`、`job/`、`providers.go`、`module_gen.go`、`register.go`、实体汇总 `models.go`。
- Config Guard 断言模块代码无 `g.Cfg()`、无旧顶层 Task 键，四个 `ModuleConfig()` 均存在。
- CLI Guard 断言文档和帮助文本无 `--module`，README 不再声称生成模块内文件。

先运行：

```bash
go test ./cool/codegen/module -run 'Guard|Layout|GeneratedWiring|MainApplicationDependencies' -count=1
```

预期：旧 Guard 扫描所有模块生成文件，文档仍描述 `handler/**`、`LoadConfig` 与 `--module`，新增测试失败。

**最小实现**

- README 与模块开发文档写明根 `config.go` 契约、八目录 allowlist、`module.<key>`、`service/**` Task 定义、全局图、唯一生成文件和四个全量命令。
- CI 保留 Go 1.26.x，顺序执行 `cool check`、测试、Race、Vet、`cool build`；增加 Layout/Config Guard 所在包测试，不增加局部生成步骤。
- Guards 使用 Go parser/AST 或结构化文件遍历；配置键检查限定代码与 manifest，避免把历史设计文档中的迁移说明误判为生产残留。

**聚焦验证**

```bash
gofmt -w cool/codegen/module/guards_test.go cool/codegen/module/layout_guard_test.go
go test ./cool/codegen/module -run 'Guard|Layout|GeneratedWiring|MainApplicationDependencies' -count=1
rg -n -- '--module|handler/\*\*|LoadConfig\(context\.Context\)|module_gen\.go / modules_gen\.go' README.md docs/module-development.md .github/workflows/go.yml
```

预期：Guard 测试通过，最后一条 `rg` 无输出并返回 1（表示文档/CI 无旧协议残留）。

## Task 17：全矩阵验证、集成限制与最终自检

**Files**

- Verify only: all files changed by Task 1-16
- Modify only when a verification failure directly proves a plan-scoped omission; do not refactor unrelated code

**先建立验收记录**

在终端逐项记录命令、退出码和跳过原因；任何失败先用最小复现回到对应任务修复，不放宽 Guard 或删除测试。

**聚焦到全量验证**

```bash
go run ./cmd/cool check
go test ./cool/codegen/module ./cool/module ./cool/registry ./cool/app ./modules/... -count=1
go test ./... -count=1
go test -race ./cool/app ./cool/registry ./cool/module ./modules/... -count=1
go vet ./...
go run ./cmd/cool build
go list -deps -f '{{.ImportPath}}' . | rg '^golang.org/x/tools' && exit 1 || true
git diff --check
```

预期：全部退出 0；`go list` 管道确认主应用依赖闭包不含 `golang.org/x/tools`。

**集成环境限制**

以下命令需要隔离的真实 MySQL/Redis，必须在对应环境变量明确启用且目标不是共享/默认开发库时执行；环境不可用时记录“未运行：缺少隔离 MySQL/Redis”，不得伪造通过，也不得把环境失败改成单元测试跳过逻辑：

```bash
COOL_AUTH_INTEGRATION=1 go test ./modules/base/... -run 'AuthIntegration|AuthSecurityIntegration' -count=1
COOL_EPS_INTEGRATION=1 go test ./modules/base/... -run 'EPSIntegration' -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base/... -run 'PersonUpdatePartialMySQLIntegration|CustomAPIServiceIntegration|TenantBoundaryServiceIntegration|CustomAPIIntegration|RelationScopeIntegration|TenantExplainIntegration' -count=1
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base/... -run 'PermissionIntegration' -count=1
COOL_DICT_INTEGRATION=1 go test ./modules/dict/... -run 'Dict.*Integration' -count=1
COOL_TASK_INTEGRATION=1 go test ./modules/task/... -run '^TestTask' -count=1
COOL_TASK_REDIS_INTEGRATION=1 go test ./modules/task/... -run '^TestTaskRedis' -count=1
```

预期：具备隔离环境的命令通过；不具备环境的命令有明确未执行记录。迁移测试目录后，环境变量语义、测试数据清理保护和业务断言不得变化。

**完整性、歧义与占位符自检**

```bash
rg -n 'T[B]D|T[O]DO|待[定]|待[确]认|F[I]XME|\?\?\?' docs/superpowers/plans/2026-07-30-node-aligned-module-config.md
rg -n 'ModuleConfig|module\.<key>|ProjectGraph|modules/modules_gen\.go|--module|allowlist|HandlerDefinition|providers\.go|module_gen\.go|race|vet|cool build|集成环境' docs/superpowers/plans/2026-07-30-node-aligned-module-config.md
git status --short
```

预期：第一条无输出并返回 1；第二条覆盖全部核心需求；`git status` 仍保留实施前用户工作区修改且没有提交。文档中的 `<key>`、`<相对包>`、`<导出函数>` 是已定义协议元变量，不是待填占位符。

## 完成定义

- `registry.ModuleSpec.Configure` 与 `module.LoadConfig[T]` 保证每个模块配置在任何组件和应用副作用前加载一次。
- 四个根 `config.go` 是元信息、中间件、纯业务 Defaults 和 Validate 的唯一来源，业务配置唯一读取 `module.<key>`。
- Scanner 只接受八个业务目录；Analyzer 只从 `service/**` 发现 `task.HandlerDefinition`，并校验所有 Middleware 引用。
- 全项目组件图解析跨模块依赖，模块顺序遵循依赖、Order 降序、Key 升序。
- 生成器、Writer 和 CLI 只处理一个全局候选，不存在局部生成模式或旧模块生成文件。
- 模块根无 `config/`、`handler/`、`providers.go`、`module_gen.go` 或测试 `.go`；业务行为快照与集成保护保持不变。
- 聚焦测试、全量测试、Race、Vet、Build、Guards 和可用的隔离集成测试均有真实结果，且全过程没有提交、回滚或丢失用户修改。
