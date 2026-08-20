# 基于目录约定的模块自动装配实施计划

> **For agentic workers:** 严格按任务顺序实施。每项生产行为先写失败测试，再完成最小实现。不得跳过生成结果类型检查，不得把运行时反射或 Service Locator 引入业务请求路径。

**Goal:** 删除所有手写 register.go、models.go 和全局模块导入清单，使构建工具仅根据 modules/<key> 目录、函数名和 Go 类型签名生成静态模块装配代码。

**Architecture:** cool/codegen/module 使用 go/packages 递归分析模块源码，构建强类型 Provider 图，在内存中生成并类型检查每模块 module_gen.go 与全局 modules_gen.go。主程序显式传入 modules.Specs()，线上只执行普通静态 Go 调用。

**Tech Stack:** Go 1.26、GoFrame v2.10.2、golang.org/x/tools v0.48.0、Go AST/types/packages、现有 cool/registry、cool/app、cool/module。

**Design:** docs/superpowers/specs/2026-07-30-directory-module-codegen-design.md

## 全局约束

- 不增加 module.yaml、注释指令、模块清单或运行时目录扫描。
- modules 的直接子目录是模块；内部目录支持任意深度。
- 生成器必须使用 Go 类型信息，不用正则或纯文本推测函数签名。
- 生成代码必须确定性排序，不包含时间戳、绝对路径或遍历顺序。
- 生成失败不得覆盖上一版可编译输出。
- 主应用最终依赖中不得包含 x/tools 或运行时 DI 容器。
- 原始 string、int、[]string 等标量不参与通用自动注入；使用强类型 Config 或专用类型。
- 自动 Provider 不允许可变参数，同一输出类型只允许一个标准 New<Type> 构造函数。
- 修改 Go 文件后执行 gofmt，先跑聚焦测试，再扩大测试范围。
- 每项只暂存计划内文件，不使用 git add -A，不改写无关工作区内容。

## Task 1：升级 Go 1.26 工具链基线

**Files:** Modify go.mod, go.sum, README.md; create or modify .github/workflows/go.yml if CI workflow is absent.

- [ ] 写失败检查：go.mod 的 go 指令必须为 1.26.0，CI 使用 Go 1.26.x 最新补丁版。
- [ ] 将项目基线升级到 Go 1.26.0，暂不引入代码生成依赖。
- [ ] 使用 Go 1.26.5 运行现有全量测试、核心 Race 测试和 go vet，先隔离工具链升级回归。
- [ ] 更新 README 的最低 Go 版本。
- [ ] 验证：go mod tidy；go test ./... -count=1；go test -race ./cool/app ./cool/registry ./cool/module -count=1；go vet ./...。
- [ ] 提交：build: upgrade to Go 1.26。

## Task 2：建立显式模块输入和运行时契约

**Files:** Modify cool/registry/registry.go, cool/app/app.go, cool/app/app_test.go; create cool/registry/runtime_group.go, runtime_group_test.go, dependencies.go, dependencies_test.go.

- [ ] 写失败测试：app.Options.ModuleSpecs 显式输入覆盖旧全局注册表；输入切片被复制；重复 Key、空 Key 和 nil Controller 工厂启动失败。
- [ ] 增加 registry.ControllerProvider、上传目录等必要强类型框架依赖，消除生成器对原始标量的猜测。
- [ ] 实现组合 Runtime：依赖顺序启动、逆序停止、启动失败回滚、错误包含模块和 Runtime 名称。
- [ ] app.Options 增加 ModuleSpecs；迁移期间 nil 才临时回退旧 registry.Modules()，最终切换任务删除回退。
- [ ] 应用内部只读取已选定的不可变 Specs 切片。
- [ ] 验证：go test ./cool/registry ./cool/app -count=1。
- [ ] 提交：refactor: accept explicit module specs。

## Task 3：实现模块与递归包扫描器

**Files:** Modify go.mod, go.sum; create cool/codegen/module/scanner.go, scanner_test.go, types.go, testdata/scanner/**.

- [ ] 固定 golang.org/x/tools v0.48.0，并确认它不在主应用依赖闭包中。
- [ ] 写失败测试：只把 modules 直接子目录识别为模块；递归发现 entity/sys、controller/admin/sys；忽略测试、testdata、隐藏目录和生成文件。
- [ ] 扫描入口支持可选的精确模块 Key 集合；默认仍扫描全部模块，未知 Key 直接失败。
- [ ] 使用 packages.Load 和 cool_generate Build Tag 加载语法、类型、导入和文件位置。
- [ ] 缺失 go.mod、modules、非法模块目录和包加载失败必须给出明确错误。
- [ ] 拒绝自动发现组件使用平台专属 Build Tag 或 GOOS/GOARCH 文件后缀。
- [ ] 验证：go test ./cool/codegen/module -run 'Scanner|Discover' -count=1。
- [ ] 提交：feat: discover module packages。

## Task 4：实现组件签名分析

**Files:** Create cool/codegen/module/analyzer.go, analyzer_test.go, component.go, signature.go, testdata/analyzer/**.

- [ ] 写失败测试：识别 Model、标准 Provider、Config、Controller、Runtime、全局/模块 Middleware、Task Handler。
- [ ] Model 仅接受 entity/** 中无参数并返回 model.Definition 的导出函数。
- [ ] Provider 仅接受 New<Type> 和 T/(T,error) 返回；拒绝可变参数、重复构造和类型不匹配。
- [ ] Controller 仅接受名称以 Controller 结尾并返回 controller.Definition 或带 error 的导出函数。
- [ ] Runtime 使用 go/types 判断返回类型是否实现 registry.Runtime。
- [ ] Middleware 由目录确定作用域；Handler 必须返回 task.HandlerDefinition。
- [ ] 位于框架目录但签名无效的导出候选必须报错，不能静默忽略。
- [ ] 验证：go test ./cool/codegen/module -run 'Analyze|Signature|Component' -count=1。
- [ ] 提交：feat: analyze module components。

## Task 5：实现 Provider 图和生命周期排序

**Files:** Create cool/codegen/module/graph.go, resolver.go, graph_test.go, resolver_test.go, testdata/graph/**.

- [ ] 写失败测试：唯一具体依赖、唯一接口实现、零实现、多实现、重复 Provider、普通循环和合法 ControllerProvider Lazy 边界。
- [ ] 将框架依赖、Config、模型、Provider、Runtime、Controller、Middleware 建成有类型节点。
- [ ] 模型严格采用 <lowerCamelModelFactory>Model 参数名绑定；错误列出候选和期望名称。
- [ ] 接口只在唯一实现时解析；原始标量依赖直接失败。
- [ ] 执行稳定拓扑排序；同层按完整 Import Path 和符号名排序。
- [ ] 识别唯一 *recycle.Manager Provider；多个时失败。
- [ ] 多 Runtime 生成组合生命周期所需的有序节点。
- [ ] 验证：go test ./cool/codegen/module -run 'Graph|Resolve|Cycle|Binding' -count=1。
- [ ] 提交：feat: resolve module dependency graphs。

## Task 6：生成并类型检查静态装配代码

**Files:** Create cool/codegen/module/renderer.go, renderer_test.go, writer.go, writer_test.go, testdata/golden/**.

- [ ] 写 Golden 失败测试：每模块生成 module_gen.go，全局生成 modules_gen.go，并带 Build Tag 和 Generated Header。
- [ ] 每模块输出静态 Model、Provider、Runtime、Controller、Middleware、Handler、DB/Menu 和 Module()。
- [ ] 全局输出稳定 import 和 Specs()，不生成 init() 或空白导入。
- [ ] import 别名、变量名、组件和模块稳定排序，不写时间戳和绝对路径。
- [ ] 使用 go/format 格式化，通过 packages.Config.Overlay 对全部候选输出做完整类型检查。
- [ ] 全部成功后才原子替换；只删除带标准 Header 的陈旧文件；内容未变不写盘。
- [ ] 验证：go test ./cool/codegen/module -run 'Render|Golden|Overlay|Writer' -count=1。
- [ ] 提交：feat: render static module wiring。

## Task 7：提供 cool generate/check/build/run CLI

**Files:** Create cmd/cool/main.go, main_test.go, cool/codegen/module/command.go, command_test.go; modify README.md.

- [ ] 使用标准库 flag/os/exec，不引入额外 CLI 框架。
- [ ] generate 更新输出；check 只比较并返回非零状态。
- [ ] generate --module <key> 只更新指定模块文件且不改全局 modules_gen.go，用于迁移和聚焦调试；check/build/run 不允许局部模式并始终分析全部模块。
- [ ] build 先生成再执行 go build；run 先生成再执行 go run，透传退出码、信号和参数。
- [ ] 写失败测试：从子目录定位 go.mod、check 不写文件、生成失败不启动子进程、子进程失败原样返回。
- [ ] README 将正式入口改为 cool build/cool run，明确原生 go build 使用已提交生成结果。
- [ ] 验证：go test ./cmd/cool ./cool/codegen/module -count=1；go run ./cmd/cool check。
- [ ] 提交：feat: add module codegen CLI。

## Task 8：以 Dict 验证端到端生成

**Files:** Modify modules/dict/service/dict_info.go, dict_type.go and tests; generate modules/dict/module_gen.go; create modules/dict/generated_spec_test.go.

- [ ] 写对比测试：手写与生成 Spec 的模型、Controller、CRUD、路由和 Seed 一致；Name/Description/Order 按设计排除。
- [ ] 将 definition 改为 dictInfoModel/dictTypeModel，将 Recycle 可变参数收敛为确定依赖。
- [ ] 只有 New<Type> 参与自动 Provider，辅助构造函数不得造成歧义。
- [ ] 生成 module_gen.go，并确认其中没有业务判断或运行时反射。
- [ ] 使用 generate --module dict 聚焦生成；此阶段不生成全局 modules_gen.go。
- [ ] 暂保留 Dict register.go；生成的 Module() 不执行全局注册。
- [ ] 验证普通与 COOL_DICT_INTEGRATION 测试。
- [ ] 提交：refactor: generate dict module wiring。

## Task 9：迁移 Base Provider、全局中间件和 EPS

**Files:** Modify Base Service constructors, Controller parameters, config.go and tests; move modules/base/middleware/*.go to middleware/global/; generate modules/base/module_gen.go; create generated_spec_test.go.

- [ ] 写对比测试：十个模型、全部 Controller、CRUD、路由、权限、DB/Menu、全局中间件和 Log Runtime 一致。
- [ ] 将 Auth/User/Role/Department 等主构造入口收敛为唯一 New<Type>。
- [ ] 将上传目录、AllowKeys、i18n 等标量改为强类型 Config 或框架依赖。
- [ ] 模型参数统一为完整精确名称。
- [ ] 移动三个全局中间件到 middleware/global，并更新引用与测试。
- [ ] 使用 registry.ControllerProvider 保留 EPS 延迟集合，不增加 Service Locator。
- [ ] Base Log 自动识别为 Runtime，生命周期行为不变。
- [ ] 使用 generate --module base 聚焦生成，不要求尚未迁移的 Task/Recycle 通过全量分析。
- [ ] 验证 Base、App、Registry 和相关 Race 测试。
- [ ] 提交：refactor: generate base module wiring。

## Task 10：迁移 Task Handler、Config 和 Runtime

**Files:** Modify modules/task/config.go, event/comm.go, Service constructors, Controller/Middleware parameters; create modules/task/handler/demo.go and tests; generate module_gen.go and generated_spec_test.go.

- [ ] 写对比测试：TaskInfo/TaskLog、InfoController、中间件、Runtime、内置 Handler 和 db.json 一致。
- [ ] 将 taskDemoService.test HandlerDefinition 移入 handler/demo.go，保持任务协议不变。
- [ ] Runtime 标准构造函数依赖强类型 Task Config；Local/Redis 选择继续由业务代码完成。
- [ ] Redis Client、Store、Executor、Engine、InfoService 构造图必须唯一。
- [ ] 模型参数使用 taskInfoModel/taskLogModel。
- [ ] 使用 generate --module task 聚焦生成，不要求尚未迁移的 Recycle 通过全量分析。
- [ ] 验证普通、Race、COOL_TASK_INTEGRATION 和 COOL_TASK_REDIS_INTEGRATION 测试。
- [ ] 提交：refactor: generate task module wiring。

## Task 11：迁移 Recycle Provider 和 Schedule Runtime

**Files:** Modify Recycle Store/Service/Schedule constructors and parameters; create or modify canonical Manager Provider and tests; generate module_gen.go, modules/modules_gen.go and generated_spec_test.go.

- [ ] 写对比测试：Data/Item、唯一 Manager、DataService、Schedule Runtime、DataController 和 db.json 一致。
- [ ] 将 Catalog、Store、Manager、Service、Schedule 建成唯一依赖链，保证全部模型先于 Catalog。
- [ ] 标准 Provider 返回 *recycle.Manager，第二个候选必须在生成期失败。
- [ ] Schedule Runtime 依赖 DataService，启动/停止和清理周期不变。
- [ ] 四个模块均适配后首次执行全量 generate，生成 modules/modules_gen.go 并通过全量 Overlay 类型检查。
- [ ] 验证普通与 COOL_RECYCLE_INTEGRATION 测试。
- [ ] 提交：refactor: generate recycle module wiring。

## Task 12：切换 modules.Specs() 并删除旧注册链

**Files:** Modify main.go, cool/app/app.go, app/module/registry tests and module tests; delete modules/modules.go, all module register.go, all entity models.go; update generated files.

- [ ] 写失败测试：Application 只接受显式 Specs；不导入模块时没有隐式注册；Specs() 返回副本且 Key 稳定排序。
- [ ] 入口改为 app.Run(context.Background(), modules.Specs())，移除 cool/app 对 modules 的空白导入。
- [ ] 删除 registry.RegisterModule、registry.Modules 和全局切片。
- [ ] 删除 cool/module 中仅服务旧全局注册的 Register/List/Reset API。
- [ ] 删除四个 register.go、四个 models.go 和人工 modules.go，测试改从 Specs() 或 Module() 获取定义。
- [ ] Name=Key、Description 为空且不再使用人工 Order，更新明确依赖旧展示元数据的测试。
- [ ] cool check 必须证明不存在陈旧生成输出。
- [ ] 验证：go test ./cool/app ./cool/registry ./cool/module ./modules/... -count=1；go test ./... -count=1。
- [ ] 提交：refactor: replace manual module registration。

## Task 13：全矩阵验证、性能检查和文档收口

**Files:** Modify README.md, module development docs and affected architecture docs; add dependency guards and benchmarks where needed.

- [ ] 使用 Go 1.26.5 执行 cool check、全量测试、Race、go vet 和 cool build。
- [ ] 运行 Base、Dict、Task、Recycle、Auth、Custom API、Task Redis 集成测试。
- [ ] 增加依赖守卫：主应用依赖不含 x/tools，生成代码不导入 reflect 或第三方 DI。
- [ ] 对比迁移前后模块、模型、Controller、路由、权限、Handler 和 Seed 快照。
- [ ] 对比二进制大小、启动时间和现有 Benchmark；不设置依赖机器速度的绝对阈值。
- [ ] 从临时目录创建最小模块，证明只新增约定目录和组件即可自动进入 Specs。
- [ ] 更新目录协议、构造签名、模型参数、Middleware 作用域、CLI 和错误示例文档。
- [ ] 最终确认无 register.go、models.go、RegisterModule、空白模块导入、module.yaml 或注释指令。
- [ ] 运行 git diff --check 并提交：docs: document automatic module discovery。
