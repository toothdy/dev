# 模块 10：模块声明与模块配置设计

> 日期：2026-08-05
> 状态：待复核
> 模块：10 模块声明与模块配置
> 对应拆分项：10.1-10.10
> 前置模块：03 配置加载与基础校验

## 1. 目标

本模块在 `cool-next/core/module` 定义业务模块的声明契约、目录身份和已编译模块配置。它不扫描目录、不反射调用 `ModuleConfig`、不按字符串查找组件；模块 11 的源码分析发现 `config.go`、解析 `ModuleConfig()`、验证符号存在性后，生成静态调用代码并将声明、目录和模块专属配置来源传给本模块。

模块 10 必须：

1. 定义 `Declaration[T]`、`ComponentRef` 和 `Ref`；
2. 从 `modules/<module>/` 路径产生唯一、可复现的模块身份；
3. 校验声明名称、描述、引用语法和中间件引用去重；
4. 使用模块 03 将 `Defaults` 与模块专属 `configuration.Source` 合并、解码并执行 `gvalid`；
5. 返回不可变、可复现的模块配置结果；
6. 保留普通与全局中间件的静态引用，供模块 31 后续校验构造器类型和路由注册。

本模块不实现 Go AST/类型加载、目录扫描、符号存在性解析、构造器类型校验、路由注册、Middleware 排序、依赖图、Application Host 的根 YAML 选择或任何运行时字符串 DI。

## 2. 模块身份与配置来源

模块身份只由目录确定。输入目录必须位于 `modules` 根下，且必须是其直接或嵌套子目录；模块根本身、绝对路径、`..` 逃逸和空段均非法。身份使用相对于 `modules` 的 `/` 分隔路径，例如：

| 模块目录 | Identity.Key() |
| --- | --- |
| `modules/demo` | `demo` |
| `modules/system/user` | `system/user` |

`Declaration.Name` 是面向人的展示名称，不参与唯一性或配置定位；模块不得再手写 machine key 或依赖列表。

架构没有定义多模块根 YAML 的最终布局，且模块 03 的 `Load` 有且仅有一个强类型根。因此本模块的输入是已经按上述 Identity 选出的 `configuration.Source`，其 `Main` 仅包含该模块的配置对象：

```yaml
enabled: false
limit: 50
```

模块 11/40 后续负责从根应用配置选择每个模块对应的 YAML 片段并构造 Source。本模块不引入 `modules:`、模块名顶层键或任何未经上位设计定义的根配置约定。

## 3. 声明与编译结果

公开类型固定为：

```go
type ComponentRef string

func Ref(symbol string) ComponentRef

type Declaration[T any] struct {
    Name              string
    Description       string
    Order             int
    Middlewares       []ComponentRef
    GlobalMiddlewares []ComponentRef
    Defaults          T
}

type Identity struct { /* 私有目录键 */ }

type Compiled[T any] struct { /* 私有状态 */ }

func IdentityFromDirectory(modulesRoot, directory string) (Identity, error)
func Compile[T any](ctx context.Context, identity Identity, declaration Declaration[T], source configuration.Source) (*Compiled[T], error)

func (i Identity) Key() string
func (c *Compiled[T]) Identity() Identity
func (c *Compiled[T]) Name() string
func (c *Compiled[T]) Description() string
func (c *Compiled[T]) Order() int
func (c *Compiled[T]) Config() T
func (c *Compiled[T]) CanonicalConfigJSON() []byte
func (c *Compiled[T]) Middlewares() []ComponentRef
func (c *Compiled[T]) GlobalMiddlewares() []ComponentRef
```

`Ref` 只是将源码中的常量符号文本转成 `ComponentRef`，不执行查找。`Compile` 调用模块 03 `configuration.Load`；因此 `Config` 和 `CanonicalConfigJSON` 分别保持模块 03 的深拷贝和字节副本保证。中间件切片每次返回新切片，Identity 是纯值，所有其他元数据在构造后不可变。

## 4. 校验

`Compile` 在加载配置前执行以下校验：

1. Identity 必须由 `IdentityFromDirectory` 产生，Key 非空且使用干净相对路径；
2. `Name`、`Description` 去除首尾空白后不得为空，不得包含控制字符；原文本不作隐式 trim 或改写；
3. `Order` 保留全部 Go `int` 值。它是同层稳定排序输入，不承担唯一性、依赖或权限语义；
4. 每个 ComponentRef 必须是非空的 Go 选择器路径：一个或多个由 `.` 分隔的标识符段，例如 `middleware.New`；
5. `Middlewares` 和 `GlobalMiddlewares` 各自不能有重复引用；同一符号可同时出现在两组，因为其作用域不同；
6. 不检查 Middleware 构造器签名、包存在性、路由合法性或排序，这些需要模块 11/31 的 AST 与路由模型。

模块 11 发现同一 Identity 的多个 `config.go`、多次 `ModuleConfig` 或无法解析的 Ref 符号时，必须在调用 `Compile` 前报带源码位置的诊断。模块 10 接受已经静态调用得到的 Declaration，不添加运行时字符串解析后门。

所有本模块自身的错误均包装为模块 02 Core 异常，保留模块 03 或原始路径/声明校验 cause。错误消息包含 Identity 或声明字段定位，但不包含环境变量实际值和完整主配置内容。

## 5. 配置流程

`Compile` 固定按以下顺序执行：

1. 校验 Identity 与 Declaration；
2. 把 `Declaration.Defaults` 与传入模块 Source 调用 `configuration.Load`；
3. 取得规范化 Config 与 Canonical JSON；
4. 防御性复制声明元数据和引用切片；
5. 返回 `Compiled[T]`。

模块 03 已经保证：对象/map 递归合并，slice/array 整体替换，nullable 显式 null，模块内部未知字段和类型不匹配拒绝，以及 `v` 标签的 `gvalid` 校验。Module Config 的配置类型不能是非 struct 根、递归、函数、channel 或其他模块 03 已拒绝的结构；本模块不得重新实现这些规则。

相同 Identity、Declaration Defaults 和 Source 输入必须得到 Config 深度等价、Canonical JSON 字节相等、引用顺序相等的 Compiled 结果。模块集合中的 Identity/展示名重复检查属于模块 11 的发现模型；本模块只保证单个已知模块的编译结果。

## 6. 文件职责

| 文件 | 职责 |
| --- | --- |
| `declaration.go` | `ComponentRef`、`Ref`、`Declaration`、引用语法校验 |
| `identity.go` | 目录到不可变 Identity 的归一化与校验 |
| `compile.go` | Declaration 校验、模块 03 调用与 `Compiled[T]` 构造 |
| `errors.go` | Core 异常包装 |
| `module_test.go` | Identity、声明、引用、配置、不可变性与可重复性测试 |
| `core/configuration` | 不新增根 YAML 布局；复用既有 `Load` 和 Result 契约 |

## 7. 测试与验收

单元测试至少覆盖：

1. 直接和嵌套模块目录生成稳定 Identity，绝对路径、模块根、逃逸、空路径和非 modules 子目录拒绝；
2. `Ref` 与合法单/多段符号，空、空段、数字开头、连字符、空白与 SQL/路径片段拒绝；
3. 空白名称/描述、控制字符、各组重复中间件拒绝；跨组同 Ref 允许；
4. 默认值、模块 YAML 覆盖、环境变量覆盖、nullable null、未知字段、类型不匹配和 `gvalid` 完全复用模块 03 的语义；
5. `Config` 的 map/slice/pointer 修改、Canonical JSON 字节修改和返回引用切片修改均不影响 Compiled 内部状态；
6. 相同输入多次 Compile 的 Config、Canonical JSON 和引用顺序完全一致；
7. 模块声明和配置错误可由 `errors.As` 识别为 `exception.BaseException` 且为 CoreFail；
8. `go test -race` 下并发读取同一 Compiled 结果无数据竞争。

门禁：

```bash
go test ./cool-next/core/module -count=1
go test -race ./cool-next/core/module -count=1
go test ./cool-next/core/configuration -count=1
go vet ./...
make check
```

## 8. 完成标准

1. 10.1-10.10 在模块 10 可承担的声明、身份和配置边界内均有实现与测试；
2. 模块身份只来自目录，展示名称不参与定位；
3. Defaults、模块 Source 和环境覆盖完全通过模块 03 合并与校验；
4. 不存在运行时字符串 DI、目录扫描或反射调用 `ModuleConfig`；
5. Middleware Ref 只作为静态符号信息保存，未提前实现模块 11/31 的 AST/路由能力；
6. Compiled 结果与公开返回集合不可变、可复现；
7. 未定义的根 YAML 多模块布局保持在模块 40 前的上层配置编排边界。
