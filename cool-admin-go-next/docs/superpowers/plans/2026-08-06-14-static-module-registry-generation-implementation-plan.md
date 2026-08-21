# 14 Static Module Registry Generation Implementation Plan

**Goal:** 将分析模型、Provider 图和 Descriptor 片段渲染为稳定的单文件模块注册表源码，并提供不含可执行 DI 工厂的不可变 `module.Graph` 静态蓝图。

**Architecture:** `core/module` 只保存模块、Descriptor、Provider、Dependency 和拓扑元数据；组件以 `(module, packagePath, constructorName)` 唯一标识。`cool-next/codegen` 对齐 Model、Provider Graph 与 DescriptorSet，生成强类型 `build...` 编译证明和静态 Graph 输入。模块 14 不保存 `Factory any`、不使用反射、不实例化组件；模块 40 再生成直接强类型装配代码。模块 15 负责最终格式化、类型检查和原子写入。

**Tech Stack:** Go 1.26、标准库 `go/format` / `go/token` / `go/types` / `sort`、模块 10 module、模块 12 codegen Graph、模块 13 DescriptorSet、Go testing

## File Structure

- Modify: `cool-next/core/module/graph.go` - 不可变静态 Graph、Provider/Dependency/拓扑元数据和构建校验
- Modify: `cool-next/core/module/graph_test.go` - Graph 静态契约、包感知身份和防御性副本测试
- Modify: `cool-next/codegen/graph.go` - Provider 包路径与完整组件身份
- Modify: `cool-next/codegen/provider.go` - 依赖 Provider 元数据与参数序号传递
- Modify: `cool-next/codegen/topology.go` - 基于完整组件身份的稳定排序和循环路径
- Modify: `cool-next/codegen/render.go` - 生成入口、完整身份对齐、强类型包装函数和静态 Graph 渲染
- Modify: `cool-next/codegen/render_test.go` - 静态元数据、跨包同名构造器、稳定输出和端到端编译测试
- Modify only if required: `cool-next/codegen/descriptor.go` / `cool-next/codegen/do_emit.go` - Descriptor Provider 包路径元数据

### Task 1: Immutable Static Blueprint

- [ ] 先写失败测试，覆盖空图、重复模块、Descriptor/Provider 归属、完整组件身份和访问器防御性副本。
- [ ] 删除 `ComponentDefinition.Factory`、内部函数值、`Component.Factory()`、`reflect` 依赖和为函数值服务的 nil 反射检查。
- [ ] 定义 Provider、Dependency 和组件拓扑输入；Provider 保存类别、模块、包路径、名称和类型文本。
- [ ] 组件身份固定为 `(module, packagePath, constructorName)`，允许不同包的同名构造器，拒绝完整身份重复。
- [ ] Dependency 保留构造器参数序号，拒绝序号重复、缺口、未知 Provider 和组件 Provider 晚于 Consumer 的拓扑输入。
- [ ] DescriptorDefinition 同时关联 `entity.Metadata` 和 Descriptor Provider 元数据。
- [ ] 保留默认配置工厂的模块配置边界，Graph 构建期不调用它，也不将其解释为组件 DI 工厂。
- [ ] 运行 `go test ./cool-next/core/module -run 'TestGraph|TestBuildGraph' -count=1`。

### Task 2: Package-aware Provider Graph

- [ ] 先写失败测试，覆盖同一模块两个包各自声明 `New`、Provider 包路径和依赖参数序号。
- [ ] 为 Config、Component 和 Descriptor 三类 Provider 填充稳定包路径。
- [ ] 构造器查找、去重、拓扑排序和循环诊断统一使用完整组件身份。
- [ ] 保留 `Dependency.ParameterIndex()`，依赖边按 Consumer 完整身份和参数序号稳定输出。
- [ ] 不改变模块 12 已冻结的 Go 类型可赋值匹配、跨模块 `contract/**` 和歧义检查语义。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestBuildGraph.*(Package|Parameter|SameName|Cycle)' -count=1`。

### Task 3: Static Registry Emission

- [ ] 先写失败测试，覆盖包感知构造器对齐、三类 Provider、Dependency 参数顺序、拓扑顺序与无 Factory 输出。
- [ ] `renderModel` 和 `renderComponents` 统一以 `(module, packagePath, constructorName)` 对齐 Model 与 Provider Graph。
- [ ] 按 Provider 拓扑生成强类型 `build...` 包装函数；参数按构造器原顺序，有 error 时原样返回，无 error 时包装为 nil error。
- [ ] 包装函数只作为 Go 编译期签名和直接调用证明，不写入 Graph Definition，不在 `Generated()` 中调用。
- [ ] 生成的 GraphInput 登记模块、Descriptor Metadata/Provider、通用 Provider、Dependency 及拓扑元数据，不生成 `Factory:` 字段。
- [ ] 强类型 Descriptor helper 返回 `entity.Descriptor[E, uint64]`，且对应 Descriptor Provider 可以满足构造器依赖。
- [ ] 按包路径稳定分配 import alias，相同输入必须产生字节相同的 `go/format` 结果。
- [ ] 只生成模块、实体和普通 Provider，不生成 Controller、路由、生命周期、gRPC、Outbox、Event、Schedule 或 Queue。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestRender' -count=1`。

### Task 4: End-to-end Compile Verification

- [ ] 构建临时 Go workspace，包含 Config、Entity/Schema、Descriptor 依赖、普通组件依赖、跨包同名构造器和返回 error 的构造器。
- [ ] 完整执行 `Analyze -> CompileDescriptors -> BuildGraphWithDescriptors -> Render`。
- [ ] 将 Render 结果仅写入测试临时目录的 `modules/modules_gen.go`。
- [ ] 在临时 workspace 执行 `go test -mod=mod ./...`，同时证明 Descriptor helper、构造器包装函数和 Graph 静态输入可真实编译。
- [ ] 重复渲染并比较字节，确认输出不依赖源码遍历顺序。
- [ ] 验证 Render 不执行 `ModuleConfig`、Schema 或任何构造器，不写入真实业务模块目录。

### Task 5: Full Verification

- [ ] 按 `docs/COMMENT_STYLE.md` 检查新增 Go 注释，不在注释中重述标识符或堆叠实现契约。
- [ ] 运行 `gofmt` 检查本模块修改的 Go 文件。
- [ ] 运行 `go test ./cool-next/core/module ./cool-next/codegen -count=1`。
- [ ] 运行 `go test -race ./cool-next/core/module ./cool-next/codegen -count=1`。
- [ ] 运行 `go test ./... -count=1`、`go vet ./...`、`make check` 和 `git diff --check`。
- [ ] 确认 `core/module` 不导入 `reflect`，生成源码不包含 `Factory:`、反射 DI、字符串组件查找或模块 40 的运行装配行为。
- [ ] 仅提交模块 14 的文档、代码和测试，不混入其他模块的已有修改。
