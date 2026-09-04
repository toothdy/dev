# 模块 12：Provider 依赖图与拓扑设计

> 日期：2026-08-05
> 状态：待复核
> 模块：12 Provider 依赖图与拓扑
> 对应拆分项：12.1-12.12
> 前置模块：11 Go 源码发现与符号分析

## 1. 目标

本模块在 `cool-next/codegen` 将模块 11 的构造器、配置类型和源码位置编译为不可变 Provider 图。它只判定静态可注入性、跨模块边界、循环和确定性构造顺序，不执行构造器、不生成 `modules/modules_gen.go`、不连接数据库，也不注册运行期全局容器。

## 2. Provider 与依赖

每个普通构造器返回的 `*T` 产生一个组件 Provider；构造器参数按声明顺序产生依赖。每个模块根包的 `Config` 产生一个仅可由同一模块组件消费的合成配置 Provider，不来自构造器也不参与跨模块 Provider 匹配。

匹配固定使用同一次 `go/packages` 加载产生的 `go/types`：具体参数要求 `types.AssignableTo(provider, parameter)`；接口参数从全部组件 Provider 中选择可赋值候选。任何参数必须恰有一个 Provider：零个为缺失，多个为歧义。返回类型重复的构造器同样报重复 Provider，不能依赖遍历顺序覆盖。

Descriptor、Base Service、Controller、生命周期、事件、任务、Outbox 和 gRPC Provider 在各自后续模块定义后才扩展图；模块 12 不伪造占位 Provider。

## 3. 跨模块边界

同模块组件可以依赖本模块 Config 或本模块任意组件类型。跨模块可以直接依赖目标模块公开的具体 Provider；接口依赖仍须声明在目标模块 `contract/**` 中，并由目标模块的唯一组件 Provider 满足。Config 和 Seed 是模块私有 Provider，不参与跨模块匹配。非法依赖报带参数位置和候选 Provider 的诊断。

模块 12 从模块 11 保留的类型对象与源码位置判定声明文件归属；不按包名、字符串前缀或 import alias 猜测边界。

## 4. 图、循环与排序

组件图的边方向为 Consumer -> Provider，模块图从跨模块组件边投影。先完成全部 Provider 与依赖匹配，再检测直接或间接循环；循环诊断输出闭合的构造器符号链及每条边的参数位置，模块循环同时列出模块 Identity 链。

无循环时按依赖拓扑输出构造顺序。多个可同时构造节点固定比较：

1. 模块 `Order` 降序；
2. 模块 Identity Key 升序；
3. 构造器包路径升序；
4. 构造器符号名升序。

模块 Order 永远不能突破真实依赖边。相同 Model 重复编译必须产生相同 Provider、边、诊断与顺序。

## 5. API 与诊断

```go
type Graph struct { /* 私有不可变状态 */ }

func BuildGraph(model *Model) (*Graph, error)
func (g *Graph) Providers() []Provider
func (g *Graph) Components() []Component
func (g *Graph) Modules() []ModuleNode
func (g *Graph) Order() []Component
```

`Graph` 访问器返回防御性副本。`BuildGraph` 失败返回模块 11 同类的稳定 `DiagnosticError`，每项包含错误码、依赖参数位置、Provider/声明关联位置和完整路径；它不是运行期 Core Exception。

## 6. 文件职责

| 文件 | 职责 |
| --- | --- |
| `cool-next/codegen/graph.go` | BuildGraph、Provider/组件/模块图模型和访问器 |
| `cool-next/codegen/provider.go` | Provider 登记、类型匹配、Config 隔离与 contract 校验 |
| `cool-next/codegen/topology.go` | 循环路径、模块投影和稳定拓扑排序 |
| `cool-next/codegen/graph_test.go` | 缺失/重复/歧义/边界/循环/排序与不可变性测试 |

## 7. 验收

测试至少覆盖同模块具体类型与接口注入、同模块 Config 注入、跨模块具体 Provider、跨模块唯一 contract 接口、跨模块 Config/Seed 和非 contract 接口拒绝、缺失 Provider、重复 Provider、接口歧义、直接和间接组件/模块循环、Order 只影响无依赖并列项、相同输入的顺序稳定与并发读取。

门禁：

```bash
go test ./cool-next/codegen -count=1
go test -race ./cool-next/codegen -count=1
go vet ./...
make check
```

## 8. 完成标准

1. 12.1-12.12 的 Provider、类型匹配、Config、contract、依赖图、循环、拓扑、完整诊断与不可变输出均有实现和测试；
2. 图只消费模块 11 的静态分析模型，不重扫工作区；
3. 无字符串 DI、限定符 DSL、运行时 Service Locator 或提前生成注册表；
4. 后续模块可在同一 Graph 上增加已冻结的 Provider 类别而不改变当前匹配规则。
