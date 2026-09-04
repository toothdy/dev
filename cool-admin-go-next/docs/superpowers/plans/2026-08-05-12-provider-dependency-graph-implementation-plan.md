# 12 Provider Dependency Graph Implementation Plan

**Goal:** 在 `cool-next/codegen` 将模块 11 的静态构造器模型编译成可诊断、可重复、不可变的 Provider 依赖图与构造顺序。

**Architecture:** `BuildGraph` 只消费 `Model` 中保留的 `go/types`，先登记组件与同模块 Config Provider，再匹配参数、校验 contract 边界，最后构建组件/模块图、检测循环并执行稳定拓扑排序。

**Tech Stack:** Go 1.26、标准库 `go/types` / `sort`、模块 11 codegen Model 与 DiagnosticError、Go `testing`

## File Structure

- Create: `cool-next/codegen/graph.go` - Graph、Provider、Component、ModuleNode 与访问器
- Create: `cool-next/codegen/provider.go` - Provider 登记、类型匹配、Config 与 contract 校验
- Create: `cool-next/codegen/topology.go` - 循环诊断、模块投影、稳定拓扑排序
- Create: `cool-next/codegen/graph_test.go` - 临时模块工作区的 Provider 图端到端测试
- Modify: `cool-next/codegen/model.go` - 暴露同包 Graph 所需的受控内部类型/位置访问，不改变公开不可变契约

### Task 1: Graph Model And Provider Registration

- [ ] 写测试，覆盖空 Model、单构造器、同模块 Config 合成 Provider、重复返回类型和访问器防御性复制。
- [ ] 定义 Graph、Provider、Component、ModuleNode 和依赖边的私有状态与稳定公开访问器。
- [ ] 从 Module/Constructor 登记组件返回类型及 Config Provider，保留模块 Identity、Order、源码位置和内部 `go/types.Type`。
- [ ] 为重复 Provider 生成稳定 DiagnosticError，不建立运行期容器。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestBuildGraph(Empty|Registers|RejectsDuplicate|Immutable)' -count=1`。

### Task 2: Dependency Matching And Boundaries

- [ ] 写测试，覆盖同模块具体类型、同模块接口、Config、缺失、接口歧义及跨模块 contract 接口的唯一 Provider。
- [ ] 使用 `types.AssignableTo` 匹配构造器参数，保持参数顺序与源位置。
- [ ] 限制 Config Provider 只能匹配本模块参数；跨模块允许目标模块具体 Provider，接口依赖检查目标声明文件是否位于 `contract/**`。
- [ ] 为缺失、歧义、跨模块具体类型及非 contract 接口输出 Consumer、参数和候选关联位置。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestBuildGraph(Resolves|RejectsMissing|RejectsAmbiguous|RejectsCrossModule)' -count=1`。

### Task 3: Cycle Detection And Stable Topology

- [ ] 写测试，覆盖直接/间接组件循环、模块循环、同层 Order、模块 Identity、包路径与符号名的并列排序。
- [ ] 从已匹配组件边投影模块边，记录闭合循环路径。
- [ ] 在无环图上执行 Kahn 拓扑排序，并以固定比较器选择 ready 节点。
- [ ] 输出 Graph 的组件、模块、Provider 和构造顺序，保证重复 BuildGraph 字节等价。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestBuildGraph(RejectsCycle|Orders)' -count=1`。

### Task 4: Full Verification

- [ ] 写并发读取 Graph 与重复 BuildGraph 测试。
- [ ] 确认实现不重扫源码、不执行构造器、不写生成文件、不连接数据库。
- [ ] 运行 `gofmt -w cool-next/codegen/*.go`。
- [ ] 运行 `go test ./cool-next/codegen -count=1`、`go test -race ./cool-next/codegen -count=1`、`go test ./... -count=1`。
- [ ] 运行 `go vet ./...`、`make check`、`git diff --check`，确认提交仅包含模块 12 文件。
