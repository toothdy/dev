# 模块 11：Go 源码发现与符号分析设计

> 日期：2026-08-05
> 状态：待复核
> 模块：11 Go 源码发现与符号分析
> 对应拆分项：11.1-11.11
> 前置模块：04 实体与元数据、10 模块声明与模块配置

## 1. 目标

本模块在 `cool-next/codegen` 实现 `cool generate` 的只读源码分析基础：加载 Go Package、AST 和类型信息，识别模块、模块配置、实体、Schema、普通构造器与 `module.Ref` 的目标符号，并输出可供模块 12-15 消费的不可变中间模型。

分析只访问工作区源码和 Go 编译期依赖，绝不创建数据库连接、读取应用配置、调用 `ModuleConfig` 或写入 `modules/modules_gen.go`。生成、依赖图、Descriptor 编译、路由和生命周期注册仍属于后续模块。

## 2. API 与数据模型

公开入口固定为：

```go
type Options struct {
    Dir         string // Go module 根目录
    ModulesRoot string // 相对 Dir 的 modules 目录
}

type Model struct { /* 私有不可变状态 */ }
type Module struct { /* 私有不可变状态 */ }
type Constructor struct { /* 私有不可变状态 */ }
type Reference struct { /* 私有不可变状态 */ }
type Diagnostic struct { /* 私有诊断信息 */ }

func Analyze(ctx context.Context, options Options) (*Model, error)

func (m *Model) Modules() []Module
func (m *Model) Diagnostics() []Diagnostic
func (m Module) Identity() module.Identity
func (m Module) Config() ConfigDeclaration
func (m Module) Entities() []EntityDeclaration
func (m Module) Schemas() []SchemaDeclaration
func (m Module) Constructors() []Constructor
func (m Module) References() []Reference
```

模型访问器返回防御性副本。内部保留 `go/types` 对象和类型，供同包的模块 12 做可赋值性判断；公开结构只暴露稳定的包路径、符号名、类型文本和源码位置，不泄漏可修改集合。

`Analyze` 在发现任一错误级诊断时返回 `nil, *DiagnosticError`。该错误按稳定顺序聚合全部诊断，保留 `errors.As` 可识别的诊断集合；它不是运行期业务失败，不伪装成 Core Exception。成功模型的 Diagnostics 只允许非错误提示，当前实现不产生提示。

## 3. 工作区与文件边界

`Options.Dir` 必须是包含当前 Go module 的目录，`ModulesRoot` 必须是其下的相对目录，默认由调用方显式传入 `modules`。分析器先用文件系统发现候选模块，再以 `golang.org/x/tools/go/packages` 在该 module 根加载所需 Package，模式至少包含 Name、Files、CompiledGoFiles、Syntax、Types、TypesInfo 和 Imports。

模块根是 `ModulesRoot` 下包含 `config.go` 的目录，Identity 通过模块 10 的 `IdentityFromDirectory` 计算。一个模块根必须恰有一个未排除的 `config.go`，模块根之间不得祖先/后代重叠。空 `modules` 目录合法，模型为空。

模块根内只递归接受下列来源：根目录的 `config.go`，以及 `contract`、`entity`、`service`、`controller`、`middleware`、`event`、`schedule`、`queue`、`consumer`、`dto` 目录中的 Go 文件。目录可以任意嵌套。下列文件或目录不参与发现：

1. `_test.go`；
2. 任意 `testdata` 或以 `.` 开头的目录；
3. Go 生成文件，包括含 `Code generated` 标记的文件和 `*.pb.go`、`*_grpc.pb.go`、`*_gen.go`；
4. 不在允许目录中的 Go 文件。

被排除文件仍可能被 Go 的类型检查用作同包编译上下文，但其 AST 节点绝不生成模块、实体、Schema、构造器或 Ref 记录。无法加载、语法错误或类型错误均产生带文件、行和列的诊断。

## 4. 基线发现规则

### 4.1 模块配置

模块根 `config.go` 必须在根包精确声明一个顶层 `ModuleConfig` 函数：无类型参数、无参数，返回唯一的 `module.Declaration[Config]` 实例。`module` 通过真实 import path `github.com/toothdy/cool-admin-go-next/cool-next/core/module` 识别，不依赖导入别名。`Config` 必须是当前根包声明的非指针命名结构体。

为保证 Ref 与配置类型在生成期确定，函数体固定为单个 `return module.Declaration[Config]{...}`；不接受分支、局部变量、辅助函数、反射或运行期拼装。模块 10 继续负责 Declaration 字段值、默认配置和 `gvalid` 的运行期编译校验；本模块只验证静态形状、记录 `Config` 类型及 Ref 调用位置。

`Middlewares` 与 `GlobalMiddlewares` 中的元素必须是直接 `module.Ref` 调用，参数必须为常量字符串。分析器以 `go/types` 确认被调用对象正是 `module.Ref`，解析以最后一个 `.` 分开的包内相对符号路径，并在同一模块已加载包中解析目标。目标必须是唯一的包级函数；类型和路由适配性由模块 31 校验。无法解析、指向排除文件、非函数或跨模块符号都会报诊断。

### 4.2 实体与 Schema

`entity/**` 中的导出命名结构体，若直接嵌入精确的 `g.Meta` 与 `core/entity.Base`，即记录为实体候选；实体字段、标签、索引以及跨实体物理冲突的完整校验仍由模块 13 完成。

同一目录中，`XxxSchema` 是与实体 `Xxx` 配对的顶层函数：无类型参数、无参数、唯一返回 `core/entity.Schema`。模块 11 记录函数和目标实体，拒绝没有同名实体、重复 Schema 或错误签名；不执行函数体、不读取 Index 声明，也不在此阶段编译 Descriptor。

### 4.3 普通构造器

在允许目录内，除 `config.go` 的 `ModuleConfig` 与 `XxxSchema` 外，所有导出的顶层 `New` 或 `NewXxx` 函数都是普通构造器候选。构造器不得有类型参数或可变参数；参数按源码顺序记录为 `go/types.Type`。返回值只能是：

```go
*T
(*T, error)
```

`T` 必须是非接口命名类型，第二返回值必须是内建 `error`。零返回值、值返回、nilable 非指针、多个业务返回值和 `error` 位置错误均报诊断。模块 11 不判断参数能否注入、不登记 Provider、不检查重复或循环依赖，这些属于模块 12。

## 5. 诊断、稳定性与扩展

每个诊断包含严重级别、稳定错误码、说明、文件相对路径、行列和可选关联位置。诊断按相对文件路径、行、列、错误码和消息稳定排序；`Analyze` 从不依赖文件遍历顺序或 map 遍历顺序。

模型中的模块按 Identity Key 排序；同一模块的包、实体、Schema、构造器与 Ref 分别按源码路径、位置和符号名排序。相同源码树重复分析必须得到等价模型与字节等价诊断文本。

Controller、Middleware 的构造器适配、生命周期、gRPC、Event、Schedule、Queue、Outbox 和 Consumer 不在本模块建立类别规则。后续模块可使用同一 Package/AST/类型加载与位置模型增加专用发现器，但不得改变本模块已冻结的模块配置、实体、Schema、构造器或 Ref 语义。

## 6. 文件职责

| 文件 | 职责 |
| --- | --- |
| `cool-next/codegen/analyze.go` | Options、入口、文件发现与稳定编排 |
| `cool-next/codegen/load.go` | `go/packages` 加载与包索引 |
| `cool-next/codegen/model.go` | 不可变中间模型及访问器 |
| `cool-next/codegen/diagnostic.go` | 位置诊断、排序与错误聚合 |
| `cool-next/codegen/module.go` | ModuleConfig 与 Ref 分析 |
| `cool-next/codegen/entity.go` | 实体与 Schema 发现 |
| `cool-next/codegen/constructor.go` | 构造器签名收集与校验 |
| `cool-next/codegen/analyze_test.go` | 临时模块工作区的发现、排除、错误与稳定性测试 |

## 7. 测试与验收

单元测试至少覆盖：

1. 直接和嵌套模块 Identity、空 modules 根、重复或重叠模块根；
2. 允许目录递归发现与测试、隐藏、testdata、生成、越界文件排除；
3. 真实导入别名下的 ModuleConfig、Config 类型、Middleware 与 GlobalMiddleware Ref 目标解析；
4. 非静态 ModuleConfig、错误签名、重复函数、未知 Ref、跨模块 Ref、非函数 Ref 与排除文件目标的诊断位置；
5. 实体、`XxxSchema` 配对及各类错误 Schema 签名；
6. `New`/`NewXxx` 参数与两种合法返回，及 variadic、泛型、值返回、错误顺序和多返回拒绝；
7. package 加载、语法和类型错误的稳定诊断；
8. 同一工作区重复分析得到等价模型与诊断，且全程不需要数据库连接。

门禁：

```bash
go test ./cool-next/codegen -count=1
go test -race ./cool-next/codegen -count=1
go vet ./...
make check
```

## 8. 完成标准

1. 11.1-11.11 的 Package/AST/types 加载、受控扫描、基线发现、Ref 解析、构造器签名、位置诊断、无数据库运行和不可变稳定模型均有实现与测试；
2. 模块 11 不写生成文件、不执行 ModuleConfig、不建立 Provider 图，也不实现未冻结组件类别；
3. 后续模块能够仅消费 Model 继续完成依赖图、Descriptor 与代码生成，而无需重新扫描工作区；
4. 任何源码或类型问题都能在编译前返回稳定且可定位的诊断。
