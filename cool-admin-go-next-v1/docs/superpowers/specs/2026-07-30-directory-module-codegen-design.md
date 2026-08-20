# 基于目录约定的模块自动装配设计

- 日期：2026-07-30
- 项目：`cool-admin-go-next`
- 状态：设计已批准，等待书面审核
- 工具链：Go 1.26、GoFrame v2.10.2、`golang.org/x/tools v0.48.0`

## 1. 背景

当前业务模块通过两层手工清单接入应用：

1. `modules/modules.go` 空白导入每个模块，触发包初始化。
2. 每个模块的 `register.go` 在 `init()` 中构造并注册 `registry.ModuleSpec`。
3. `entity/models.go` 汇总模块模型。

这种方式运行期成本低、类型安全，但新增模型或模块需要同步维护清单。Node 版本可以在开发期扫描模块目录，并在生产构建前生成实体集合。Go 不能在运行时执行未导入、未编译的普通 Go 包，因此本设计采用构建期扫描和静态代码生成，在不牺牲性能、稳定性、跨平台和单一二进制部署的前提下提供相同的开发体验。

## 2. 目标

1. 新增 `modules/<key>` 后不手工维护 `modules.go`、`register.go` 或 `models.go`。
2. 不增加 `module.yaml`、注释指令或其他模块清单。
3. 完全依靠目录、导出函数名和 Go 类型签名识别模块组件。
4. 支持 `entity/sys`、`service/sys`、`controller/admin/sys` 等任意深度子目录。
5. 在构建期完成依赖解析、歧义检查、循环检测和生成结果类型检查。
6. 生成普通静态 Go 装配代码；线上不扫描源码、不使用反射 DI、不加载动态库。
7. 最终产物继续保持跨平台、单一可执行文件和当前请求性能。

## 3. 非目标

- 不支持应用运行后热加入核心 Controller、Service 或 Entity。
- 不使用标准库 `plugin`、Yaegi、HashiCorp `go-plugin` 或运行时 RPC 作为内置模块机制。
- 不替业务代码决定 Redis/Local 模式、配置分支或多个接口实现的选择。
- 不从任意目录推测框架组件；自动发现只适用于 `modules/<key>` 下的约定目录。
- 不提供模块级 Name、Description 或人工 Order 声明。
- 不改变后台安装型 WASM 插件的独立设计。

## 4. 核心决策

### 4.1 构建期代码生成

模块发现流程固定为：

```text
modules/*
  -> 递归加载 Go package
  -> AST 与类型分析
  -> 识别组件并构建依赖图
  -> 校验和拓扑排序
  -> 生成静态 Go 装配代码
  -> go build
```

生成文件和手写 `register.go` 一样参与普通 Go 编译。生成器、AST 包和类型分析依赖不进入主程序二进制。

### 4.2 纯目录模块边界

`modules` 的直接子目录是唯一模块边界：

```text
modules/base                         -> base 模块
modules/base/entity/sys              -> base 的实体包
modules/base/service/sys             -> base 的 Service 包
modules/base/controller/admin/sys    -> base 的后台 Controller 包
```

模块内部允许任意层级。内部目录不会再次成为模块。

### 4.3 静态显式启动

移除空白导入和 `init()` 注册链：

```text
main
  -> modules.Specs()
  -> app.Run(context.Background(), modules.Specs())
```

`modules.Specs()` 由生成器维护。`app.Options` 增加显式的 `ModuleSpecs []registry.ModuleSpec`，`app.Run` 的默认启动入口接收该集合并传入 Build 流程。应用不再空白导入业务模块，也不再依赖可变的全局模块注册表和包初始化顺序。

## 5. 目录协议

生成器递归扫描以下目录：

| 相对目录 | 组件 |
| --- | --- |
| `entity/**` | 模型定义 |
| `service/**` | Service、Store 和其他 Provider |
| `controller/**` | Controller 定义 |
| `event/**` | Event 与 Runtime |
| `schedule/**` | Schedule 与 Runtime |
| `middleware/**` | 模块中间件 |
| `middleware/global/**` | 全局中间件 |
| `handler/**` | Task Handler 定义 |
| `queue/**` | 队列相关 Provider |
| 模块根目录 | `LoadConfig` 等强类型配置 Provider |

`_test.go`、`testdata`、隐藏目录和生成文件不参与发现。

模块属性完全由约定产生：

| 属性 | 来源 |
| --- | --- |
| Key | `modules` 直接子目录名 |
| Name | 默认等于 Key |
| Description | 空 |
| DB | 模块根目录存在 `db.json` |
| Menu | 模块根目录存在 `menu.json` |
| Order | 不再人工声明 |
| 启动顺序 | 依赖拓扑顺序，同级按 Key 排序 |

## 6. 组件识别

### 6.1 Model

`entity/**` 中无参数且返回 `model.Definition` 的导出函数被识别为模型工厂。模型按完整包路径和函数名稳定排序。

因为所有模型返回相同 Go 类型，依赖模型的参数使用精确命名绑定：

| 模型函数 | 参数名 |
| --- | --- |
| `BaseSysUser()` | `baseSysUserModel` |
| `DictInfo()` | `dictInfoModel` |
| `TaskLog()` | `taskLogModel` |

找不到唯一模型时生成失败，不进行模糊匹配。

### 6.2 Provider

`service/**` 等 Provider 目录中的自动构造函数必须满足：

1. 函数已导出。
2. 函数名为 `New` 加返回具体类型名。
3. 返回 `T` 或 `(T, error)`。
4. 不使用可变参数。
5. 同一输出类型只有一个标准 Provider。

`NewAuthServiceWithCache` 等辅助构造函数可以保留，但不会参与自动装配。现有多个主构造入口需要收敛为唯一标准 Provider。

Provider 在一次应用装配中只执行一次，返回对象默认为模块级单例。

### 6.3 Config

现有 `config.go` 是运行参数逻辑，不是模块清单，可以保留。模块根目录中的：

```text
LoadConfig(context.Context) (Config, error)
```

被识别为该模块 Config 的 Provider。

原始 `string`、`int`、`[]string` 等类型不作为通用自动依赖。上传目录、时区、锁名称等值必须进入强类型 Config 或专用命名类型，避免按参数名猜测标量含义。

### 6.4 Controller

`controller/**` 中名称以 `Controller` 结尾，并返回 `controller.Definition` 或 `(controller.Definition, error)` 的导出函数会被收集。参数从 Provider 图解析。

Controller 顺序固定为相对包路径升序，再按函数名升序。

### 6.5 Interface

接口依赖根据 Go 的可赋值规则解析：

- 唯一实现：自动注入。
- 没有实现：生成失败。
- 多个实现：生成失败，业务代码提供一个标准适配 Provider。

Local/Redis Scheduler 等运行时选择继续由业务构造函数根据强类型 Config 决定，生成器不代替业务分支。

### 6.6 Runtime

`event/**`、`schedule/**` 等目录中，标准 Provider 返回实现 `registry.Runtime` 的类型时自动成为模块 Runtime。

模块可以有多个 Runtime。框架生成组合 Runtime，按依赖顺序启动、反序停止；启动失败时停止已经启动的部分并返回包含模块和 Runtime 名称的错误。

返回 `*recycle.Manager` 的标准 Provider 被识别为应用级 Recycle Provider。整个项目只能有一个，发现多个时生成失败。

### 6.7 Middleware

目录决定作用域：

- `middleware/global/**`：全局中间件。
- 其他 `middleware/**`：只作用于当前模块路由。

Middleware 构造参数同样从 Provider 图解析，可以依赖 Config、Runtime、Service 和 Controller 集合。

### 6.8 Task Handler

`handler/**` 中返回 `task.HandlerDefinition` 的函数自动加入 Handler 集合。任务名称、超时、重试和实际函数仍由 HandlerDefinition 表达，生成器只负责发现与汇总。

## 7. 框架依赖与循环

框架直接提供上下文、数据库、认证、会话、Recycle、Task Handler 和 Controller 集合等稳定依赖。原始标量改用强类型包装。

生成器对完整依赖图执行拓扑排序。一般循环直接失败，并打印完整链路。

EPS 等有意依赖最终 Controller 集合的场景使用框架定义的强类型 `registry.ControllerProvider`。它是唯一认可的惰性集合边界，用于打断：

```text
EPSService -> Controller 集合 -> OpenController -> EPSService
```

其他循环不得通过通用 Service Locator 绕过。

## 8. 生成器结构

```text
cool/codegen/module/
├── scanner.go
├── analyzer.go
├── graph.go
├── validator.go
├── renderer.go
└── writer.go

cmd/cool/
└── main.go
```

项目的 `go` 指令升级为 `1.26.0`。生成器固定使用当前最新的官方 `golang.org/x/tools v0.48.0`，通过 `go/packages` 加载 AST、类型、接口实现和包依赖。该依赖只属于开发工具链。

每个模块生成自己的装配文件，全局只生成模块集合：

```text
modules/base/module_gen.go
modules/dict/module_gen.go
modules/recycle/module_gen.go
modules/task/module_gen.go
modules/modules_gen.go
```

生成文件顶部包含标准声明和生成期排除约束：

```text
//go:build !cool_generate
// Code generated by cool generate. DO NOT EDIT.
```

生成器以 `cool_generate` Build Tag 加载业务源码，避免旧生成文件阻止重新生成或被重复识别。

## 9. 生成算法

1. 从当前目录向上查找 `go.mod`。
2. 读取 Module Path 并定位 `modules`。
3. 按名称枚举直接子目录。
4. 递归加载每个模块的 Go package。
5. 分类候选函数并读取完整类型签名。
6. 注册模型、Provider、Controller、Runtime、Middleware 和 Handler 节点。
7. 解析框架依赖、模型命名绑定和接口实现。
8. 检测缺失、重复、歧义与循环依赖。
9. 对依赖图和输出符号稳定排序。
10. 在内存中渲染全部文件并通过 `go/format`。
11. 使用内存 Overlay 对生成结果进行完整类型检查。
12. 全部成功后比较并原子写入变化文件。

语义或类型校验失败时不改磁盘上的旧生成文件。删除陈旧输出时只允许操作带标准 Generated Header 的文件。

## 10. 确定性和构建性能

生成结果不包含时间戳、绝对路径或文件系统遍历顺序。模块、包、函数和 import 都按稳定规则排序，同一源码在不同平台和 CI 中生成相同内容。

第一版不增加额外缓存清单：

- `go/packages` 复用 Go Build Cache。
- 一次加载完成全部类型分析。
- 生成结果只在内容变化时写盘。
- 未变化文件保持修改时间，保留 Go 增量编译收益。

只有实际基准证明大型项目扫描过慢时才增加内容哈希缓存。

自动发现组件不得使用平台专属 Build Tag 或文件后缀。平台相关内部实现可以分文件，但所有目标必须暴露相同标准构造函数签名。

## 11. CLI 与 CI

提供：

```text
cool generate
cool check
cool build
cool run
```

- `generate`：校验并更新生成文件。
- `check`：只比较预期输出，不修改文件。
- `build`：先生成，再执行 `go build`。
- `run`：先生成，再执行 `go run`。

未安装 CLI 时使用 `go run ./cmd/cool <command>`。

Go 原生 `go build` 不运行生成器。生成文件提交 Git，因此普通 `go build` 和 `go test ./...` 可以直接使用当前生成结果；正式开发和发布使用 `cool build`。CI 首先执行 `cool check`，拒绝生成文件过期的提交。

## 12. 错误策略

错误必须包含模块、包、符号、原因和修复方向。典型错误包括：

- 标准 Provider 缺失或重复。
- Controller 参数无法解析。
- 模型参数名没有唯一匹配。
- 接口实现为零或多个。
- Provider 依赖循环。
- Recycle Provider 重复。
- Runtime 或 Middleware 签名非法。
- 生成输出无法通过类型检查。

看起来属于框架组件但签名非法的导出函数不得静默跳过。

## 13. 现有模块迁移

### 13.1 Base

- 递归发现 `entity/sys`、`service/sys` 和 `controller/admin/sys`。
- 三个全局中间件移动到 `middleware/global`。
- 多个 Service 主构造入口收敛为标准 Provider。
- 模型参数改为精确名称。
- Base Log 自动识别为 Runtime。
- EPS 使用 `registry.ControllerProvider`。
- 删除 `register.go` 和 `entity/sys/models.go`。

### 13.2 Dict

- 自动发现两个模型、两个 Service 和两个 Controller。
- `definition` 参数改为明确模型参数名。
- Recycle Manager 可变参数改为确定参数。
- 自动检测 `db.json`。
- 删除 `register.go` 和 `entity/models.go`。

Dict 作为首个端到端迁移模块，用于对比手写与生成 ModuleSpec。

### 13.3 Task

- 自动发现模型、Service、Controller、Runtime 和模块中间件。
- 内联 Task Handler 声明移动到 `handler/demo.go`。
- Task Runtime 直接依赖强类型 Config。
- Scheduler 模式继续由 Runtime 业务代码选择。
- 删除 `register.go` 和 `entity/models.go`。

### 13.4 Recycle

- 自动发现 Data、Item、Store、Manager、Service、Schedule Runtime 和 Controller。
- 根据 `*recycle.Manager` 输出识别唯一应用级 Provider。
- 依赖图保证所有模型先于 Catalog 和 Manager 建立。
- 删除 `register.go` 和 `entity/models.go`。

## 14. 测试

### 14.1 生成器单元测试

`cool/codegen/module/testdata` 覆盖：

- 一级与任意深度目录。
- Provider 唯一解析。
- 模型精确匹配。
- 接口唯一实现、缺失和歧义。
- 普通循环和合法 Lazy Provider。
- 多 Runtime 生命周期顺序。
- 全局与模块中间件分类。
- 忽略测试和生成文件。
- 删除模块与陈旧文件处理。
- 不同文件创建顺序产生相同输出。
- `check` 不写文件。

生成文件使用 Golden Test。

### 14.2 行为回归

迁移前后比较：

- 模块 Key。
- 模型表名、资源名和字段。
- Controller 名称、前缀、顺序、CRUD、自定义路由和权限。
- Middleware 作用域。
- Task Handler。
- Seed 路径。
- Runtime 启停行为。

除已明确取消的 Name、Description 和 Order 外，其他行为必须一致。

### 14.3 性能与稳定性

验证最终主应用依赖不包含 `golang.org/x/tools` 或运行时 DI 容器，不使用反射创建业务对象，每个 Provider 只创建一次，请求路径不增加分派层。

执行：

```text
cool check
go test ./...
go test -race ./cool/app ./modules/...
go vet ./...
cool build
```

同时对比迁移前后的二进制大小、应用启动时间、模块装配耗时、路由数量和相关 Benchmark。请求性能出现可测量回退视为实现错误。

CI 使用 Go 1.26.x 最新补丁版守住项目最低版本兼容性。未来 Go 发布新的稳定大版本后，再增加当前稳定版任务以提前发现工具链演进问题。

## 15. 迁移顺序

1. 将 `go.mod` 基线升级到 Go 1.26.0，并使用 Go 1.26.x 最新补丁版完成现有全量测试。
2. 引入 `golang.org/x/tools v0.48.0`，实现生成器核心和 Fixture 测试。
3. 生成 Dict，但暂不接入应用启动。
4. 对比手写和生成的 Dict ModuleSpec。
5. 标准化 Base、Task、Recycle 的构造签名和目录。
6. 生成四个模块并完成元数据快照对比。
7. 将应用入口切换为 `modules.Specs()`。
8. 删除全局注册表、全部 `register.go` 和 `models.go`。
9. 使用 Go 1.26.x 最新补丁版运行单元、Race、集成和性能验证。
10. 更新模块开发文档和示例。

迁移不长期保留两套注册机制。最终切换提交必须保持全量测试通过。
