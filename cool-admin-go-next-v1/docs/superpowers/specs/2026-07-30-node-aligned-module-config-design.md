# Node 对齐的模块目录、配置与集中装配设计

- 日期：2026-07-30
- 项目：`cool-admin-go-next`
- 状态：设计已确认，等待据此修订实施计划
- 上游约定：`cool-admin-midway/.cursor/rules/module.mdc`
- 修订对象：`2026-07-30-directory-module-codegen-design.md` 与 `2026-07-30-directory-module-codegen.md`

## 1. 背景

现有目录代码生成方案已经完成从 `register.go`、`entity/models.go` 和隐式 `init()` 注册向静态生成装配的迁移，但生成结果仍保留了 Go 自有的模块根职责：

- `modules/<key>/config/` 保存配置类型；
- `modules/<key>/providers.go` 保存仅用于装配的桥接 Provider；
- `modules/<key>/module_gen.go` 在模块包内完成装配；
- 根 `config.go` 自行读取 `g.Cfg()`，并被生成器识别为 `LoadConfig` Provider；
- `modules/modules_gen.go` 仅导入各模块并调用其 `Module()`。

这与 Node 模块约定仍有明显差异。Node 约定把模块根 `config.ts` 作为模块名称、描述、加载顺序、中间件和业务默认配置的唯一声明入口，并统一从 `module.<目录名>` 读取模块配置。Go 版本应保持静态类型、构建期检查和单二进制优势，同时让目录职责与该约定严格对齐。

## 2. 目标

1. `modules/<key>` 根目录只承担 Node 模块根约定的职责：必需的 `config.go`、可选的 `db.json` 和 `menu.json`，以及约定业务目录。
2. 删除所有模块内 `config/`、`handler/`、`providers.go` 和 `module_gen.go`；模块根不再承载装配实现，任务注册定义与服务实现共置。
3. `config.go` 成为模块元信息、`order`、中间件清单、强类型业务配置及其默认值的唯一声明文件。
4. 所有模块业务配置只由框架从 `module.<key>` 加载、合并、转换和校验；模块代码不直接调用 `g.Cfg()`。
5. 所有模型、Provider、Runtime、Controller、Middleware、从 `service/**` 发现的 `task.HandlerDefinition` 和模块声明的静态装配集中生成到 `modules/modules_gen.go`。
6. 保持构建期发现、类型安全、确定性生成、无运行时反射 DI、无运行时源码扫描和当前单一可执行文件部署方式。
7. 通过全局依赖图同时保证 Provider 顺序、模块顺序和循环依赖诊断；`order` 只在不违反依赖关系时决定优先级。

## 3. 非目标

- 不引入 `module.yaml`、注释指令、手工 Go 模块清单或第二份元数据来源。
- 不支持运行后热加入 Controller、Service、Entity 或 Middleware。
- 不使用 `init()`、空白导入、反射容器或 Service Locator 进行模块装配。
- 不让生成器决定 Local/Redis 等业务分支；生成器只解析依赖并调用业务构造函数。
- 不把 `server`、`database`、`redis.default`、`cool.schema`、`cool.crud`、`cool.tenant`、`cool.auth`、`cool.i18n` 等应用级配置搬入任意业务模块。
- 不在本设计中改变 `db.json`、`menu.json` 的数据格式、锁文件语义或生产环境导入策略。
- 不提供配置热重载。配置在一次 Application 构建中加载一次，修改配置后需要重启。

## 4. 方案选择

### 4.1 采用：根配置声明加全局静态生成

模块根只声明配置，生成器扫描所有模块后，在 `modules/modules_gen.go` 内构建完整依赖图和所有装配闭包。这既保留 Node 的目录职责，也符合 Go 的静态导入规则。

### 4.2 不采用：继续生成每模块 `module_gen.go`

该方案改动较小，但模块根继续混入框架装配产物，`modules/modules_gen.go` 仍只是二级清单，无法满足集中装配目标。

### 4.3 不采用：运行时扫描或注册容器

运行时目录扫描不能加载未编译 Go 包；反射 DI 或注册容器会降低可追踪性，并把生成期可发现的缺失、歧义和循环错误推迟到启动期。因此继续使用构建期类型分析和普通静态调用。

## 5. 最终目录契约

### 5.1 模块根 allowlist

```text
modules/
├── modules_gen.go                 # 唯一生成装配文件
├── base/
│   ├── config.go                  # 必需
│   ├── db.json                    # 可选
│   ├── menu.json                  # 可选
│   ├── controller/**
│   ├── dto/**
│   ├── entity/**
│   ├── middleware/**
│   ├── event/**
│   ├── schedule/**
│   ├── service/**
│   └── queue/**
└── <other-module>/...
```

每个 `modules` 直接子目录仍是一个模块，内部业务目录可任意深度嵌套。对 `module.mdc` 和 Node `src/modules` 全部模块的实际一级目录审计如下：

| 分类 | 目录 | Node 实证 | 第一版处置 |
| --- | --- | --- | --- |
| 规则明列的标准业务目录 | `controller`、`dto`、`entity`、`middleware`、`schedule`、`service` | `controller` 见 9 个模块，`dto` 见 Base，`entity` 见 8 个模块，`middleware` 见 Base/Task/User，`schedule` 见 Recycle，`service` 见 8 个模块 | 全部进入 allowlist；目录即使暂时为空或某模块未使用，也仍属于标准协议 |
| Node 实际存在但规则未列出 | `event`、`queue` | `event` 见 Base/Demo/Plugin/Recycle/Swagger/Task；`queue` 见 Demo/Task | 作为有仓库实证且当前 Go 模块已采用的显式兼容目录进入 allowlist，不把它们误写成 `module.mdc` 标准目录 |
| Node 实际存在但本设计不采用 | `db`、`hooks`、`job` | 分别只见 Base、Plugin、Base | 不进入第一版 allowlist：`db/` 不能与根 `db.json` 淆同，`hooks/`、`job/` 是模块特例且当前 Go 无对应职责；以后确需引入时必须先单独更新目录协议、迁移设计和生成器测试 |
| Go 自创目录 | `config`、`handler` | Node 没有同名一级目录；Node 配置是根 `config.ts`，Task 演示任务位于 `task/service/demo.ts` | 均禁止并删除：配置类型并入根 `config.go`；任务注册定义并入 `service/demo.go`，不得保留或改名复制 `handler/` |

因此第一版顶层业务目录的完整 allowlist 只有 `controller`、`dto`、`entity`、`middleware`、`schedule`、`service`、`event` 和 `queue`。其中前六项来自规则，后两项来自 Node 仓库实证；`db`、`hooks`、`job`、`config`、`handler` 均不准入。新增顶层职责必须先更新目录协议和生成器测试，不能由单个模块自行扩展。

模块根禁止出现：

- `config/`；
- `handler/`；
- `db/`、`hooks/`、`job/` 及其他未准入业务目录；
- `providers.go`；
- `module_gen.go`；
- `register.go`、`models.go`、模块清单文件；
- 其他生产或测试 `.go` 文件。

因此原根目录 `config_test.go`、`base_test.go`、`generated_spec_test.go` 等测试也必须迁移到对应业务包或框架/生成器测试包。根目录规则不是“生产文件例外”，而是文件级 allowlist。

### 5.2 目录职责

| 路径 | 职责 |
| --- | --- |
| `config.go` | 模块声明、强类型配置、默认值和纯校验 |
| `db.json` | 模块初始化数据 |
| `menu.json` | 模块初始化菜单 |
| `entity/**` | 模型定义 |
| `service/**` | 业务服务、普通 Provider 和与服务实现共置的 Task `HandlerDefinition` |
| `controller/**` | Controller |
| `middleware/**` | 模块或全局中间件工厂 |
| `event/**` | 事件与长生命周期 Runtime |
| `schedule/**` | 定时任务 Runtime |
| `queue/**` | 队列生产、消费和基础设施适配 |
| `dto/**` | 请求、响应和校验数据类型 |

原 `providers.go` 中有业务含义的构造函数必须移动到其拥有的业务目录。例如 Base Log Runtime 的构造归入 `event`，Recycle Catalog/Manager 的构造归入 `service` 或 `event`。不得把这些函数改名后继续放在模块根。

Task 注册定义不拥有独立目录。与 Node `task/service/demo.ts` 对齐，演示任务的执行函数和 `task.HandlerDefinition` 都归入 Go `modules/task/service/demo.go`。生成器递归扫描所有模块的 `service/**`，将首个返回值精确为 `task.HandlerDefinition` 的导出函数识别为任务注册定义；函数不可变参，可返回 `task.HandlerDefinition` 或 `(task.HandlerDefinition, error)`，参数继续按全局依赖图注入。该类型识别先于普通 Provider 识别，函数名不作为第二套注册协议。`service/**` 之外返回该类型必须生成失败，防止任务定义因放错目录而被静默忽略。

## 6. `config.go` 契约

### 6.1 唯一入口

每个模块根 `config.go` 必须且只能声明以下内容：

- 模块业务配置类型及相关纯值类型；
- 默认值常量；
- `ModuleConfig()`；
- 可选的 `Validate() error` 纯校验方法。

标准形态如下：

```go
package task

import "github.com/toothdy/cool-admin-go-next/cool/module"

type Config struct {
   Mode     Mode   `json:"mode"`
   Timezone string `json:"timezone"`
   Log      LogConfig `json:"log"`
}

func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{
      Name:        "任务调度",
      Description: "任务调度模块，支持本地与 Redis 调度",
      Order:       0,
      Middlewares: []module.ComponentRef{
         "middleware#Definition",
      },
      Defaults: Config{
         Mode:     ModeAuto,
         Timezone: "Asia/Shanghai",
         Log:      LogConfig{KeepDays: 20},
      },
   }
}
```

`module.Declaration[T]` 的框架字段固定为：

| 字段 | 契约 |
| --- | --- |
| `Name` | 必填、非空的人类可读名称 |
| `Description` | 必填、非空的模块描述 |
| `Order` | 可选，默认 `0`；值越大越优先 |
| `Middlewares` | 仅作用于本模块路由的工厂引用 |
| `GlobalMiddlewares` | 作用于所有路由的工厂引用 |
| `Defaults` | `T` 类型的业务默认配置 |

模块 Key 不在声明中重复填写，唯一来源始终是目录名。`db.json` 和 `menu.json` 路径也由文件是否存在推导，避免同一事实出现两个来源。

### 6.2 声明的静态形态

为了让 `order` 和中间件在生成期参与校验，`ModuleConfig()` 必须满足：

1. 位于模块根 `config.go`；
2. 无参数；
3. 返回 `module.Declaration[T]`，其中 `T` 是当前模块根包声明的具体 `Config`；
4. 函数体直接返回一个 `module.Declaration[T]` 复合字面量；
5. `Name`、`Description`、`Order` 和中间件引用必须是可由 `go/types` 求值的常量表达式；
6. `Defaults` 必须是纯数据，不允许函数、Channel、数据库连接、Redis Client、锁或其他运行时资源。

不满足该形态时生成失败。该限制避免生成器执行业务代码，也保证相同源码在所有环境产生相同装配结果。

每个可外部配置的导出字段必须声明非空、非 `-` 的 `json` 标签，且同一结构内不得重名。Loader 不从 Go 字段名推导备用键；缺失标签与重复标签均在生成期失败。`Validate() error` 统一由 `Config` 值接收者实现且不得修改配置。时区 Location、Redis Client 等派生值或运行时资源由消费该配置的 Provider 创建，不存入 Config。

### 6.3 中间件引用

`module.ComponentRef` 使用固定格式：

```text
<相对模块根的包路径>#<导出函数名>
```

例如：

- `middleware#Definition`；
- `middleware/global#Definitions`。

生成器必须验证引用存在、函数签名合法、作用域与字段匹配且没有重复。`Middlewares` 只能引用普通 `middleware/**` 工厂，`GlobalMiddlewares` 只能引用 `middleware/global/**` 工厂。每个被发现的中间件工厂必须恰好被声明一次；未声明的工厂和无法解析的声明都属于错误。

引用只用于生成期解析。最终文件仍生成对具体函数的直接调用，不在运行时按字符串查找组件。

### 6.4 允许与禁止

`config.go` 允许：

- 标准库值类型和纯配置结构；
- `module.Declaration` 与 `module.ComponentRef`；
- 无 I/O、无全局状态变更的默认值构造和校验。

`config.go` 禁止：

- 调用 `g.Cfg()` 或读取任意其他配置前缀；
- 创建 Redis、数据库、队列、文件或网络资源；
- 导入当前模块的任何子包；
- 声明 Service、Runtime、Controller 或装配桥接 Provider；
- 根据环境或当前时间改变模块元信息、默认值或中间件清单。

所有模块子包可以单向导入模块根包以使用 `Config`。根包禁止反向导入子包，这是避免 Go import cycle 的硬性边界。

## 7. 统一配置加载

### 7.1 唯一键空间

模块业务配置的唯一外部键为：

```text
module.<module-directory-name>
```

例如 Task 配置从顶层 `task.*` 迁移到 `module.task.*`。迁移完成后不保留旧键 fallback；否则同一配置会出现两个来源，并掩盖部署文件未迁移的问题。

应用级配置继续保留原有前缀。具体边界为：

- `redis.default` 由框架基础设施提供，不进入 Task `Config`；
- `cool.crud.softDelete` 由应用决定是否启用回收能力，不进入 Recycle `Config`；
- `cool.auth.*`、`cool.i18n.*`、`cool.tenant.*` 等由 Application 解析并通过强类型框架依赖传入模块；
- `module.base` 只保留 Base 自有的 `allowKeys`、中间件开关和日志参数；
- `module.recycle` 只保留清理周期、批量大小和锁名；
- `module.task` 保存调度模式、时区、日志、执行和队列业务参数。

模块内不得再直接读取应用级前缀。需要应用级值时，构造函数显式依赖框架提供的命名类型或依赖结构。

### 7.2 合并与转换规则

框架提供唯一泛型入口，语义等价于：

```text
load(module key, declaration defaults) -> typed Config
```

固定流程为：

1. 深复制 `Defaults`，避免不同 `Specs()` 调用共享 Slice 或 Map；
2. 读取 `module.<key>`；缺失时直接使用默认值；
3. 要求已配置值是对象，拒绝标量或数组根值；
4. 按字段递归覆盖默认值，显式 `false`、`0`、空字符串和空数组都视为有效覆盖；
5. 使用字段 `json` 标签作为唯一外部名称，拒绝未知字段；
6. 转换成强类型 `Config`，转换失败时保留完整字段路径；
7. 若 `Config` 实现 `Validate() error`，执行跨字段校验；
8. 将结果冻结在本次模块装配实例中，后续构造函数只接收该强类型值。

显式 `null` 仅允许覆盖 Pointer、Map 或 Slice；对 Bool、数字、字符串、结构体和 `time.Duration` 等非空类型使用 `null` 必须报错。配置结构被视为不可变值，Provider 不得修改其中的 Slice、Map 或 Pointer 指向内容。

配置解码可以在启动阶段使用 GoFrame 的结构化转换能力；“无运行时反射 DI”不禁止配置解码所需的结构反射。反射不得用于发现或构造业务组件，也不得进入请求路径。

### 7.3 加载时机

`registry.ModuleSpec` 增加模块配置准备阶段。每个生成 Spec 持有独立 assembly，`Configure(context.Context) error` 将对应配置加载一次并缓存值或错误。

Application 必须在创建任何模块 Runtime、Recycle Provider、Controller、Middleware，以及执行 Schema、Seed 或路由副作用之前，按最终模块顺序调用全部 `Configure`。即使当前没有组件依赖某模块的 Config，也必须加载并校验，以保证错误不会被静默跳过。

构造函数参数若类型精确等于 `modules/<key>.Config`，生成器将其绑定到该模块 Config 节点。原 `LoadConfig(context.Context)` 不再是可发现 Provider。

## 8. 集中装配

### 8.1 唯一输出

生成器只允许写入：

```text
modules/modules_gen.go
```

该文件继续包含：

```text
//go:build !cool_generate
// Code generated by cool generate. DO NOT EDIT.
```

它直接导入各模块根包和业务子包，并生成：

- 各模块独立的 assembly 状态；
- 强类型 Config 缓存和 Provider getter；
- 模型、从 `service/**` 发现的 Task `HandlerDefinition`、Runtime、Recycle Provider、Controller 和 Middleware 工厂；
- `registry.ModuleSpec`；
- 稳定排序且每次返回独立装配状态的 `Specs()`。

不再生成或调用 `modules/<key>.Module()`。模块根包不导入 `cool/registry` 装配类型，除非 `module.Declaration` 的定义所在包需要；完整 `registry.ModuleSpec` 只出现在 `modules` 生成包和框架层。

### 8.2 生成和检查命令

集中输出意味着局部写入不再安全。原计划的 `generate --module <key>` 被取消：任何模块变化都可能改变全局 import、跨模块依赖、模块拓扑和唯一 Provider 校验。

保留命令：

```text
cool generate
cool check
cool build
cool run
```

四个命令都分析完整模块集合。`check` 只比较内存候选，不写磁盘；`build` 和 `run` 先完整生成。原生 `go build` 继续使用已提交的 `modules/modules_gen.go`。

## 9. 依赖图、顺序与 import cycle

### 9.1 全局组件图

原实现先为每个模块建图，再额外暴露全局模型。新实现建立一个项目级图，节点包括：

- 每个模块的 Config；
- Model；
- 普通 Provider；
- Runtime 和 Recycle Provider；
- 从 `service/**` 发现的 Task `HandlerDefinition`；
- Controller；
- Middleware；
- 框架提供的强类型依赖。

具体类型按 Go assignability 唯一绑定，接口仍要求唯一实现，模型仍使用精确参数名绑定。原始标量不参与通用注入。`registry.ControllerProvider` 继续是唯一允许的 Controller 集合惰性边界；不得增加通用惰性解析器。

当模块 A 的组件依赖模块 B 的 Config、Model 或 Provider 时，生成模块边 `B -> A`。普通组件循环和模块循环都在生成期失败，并输出完整符号链和模块链。

### 9.2 模块顺序

模块顺序采用带优先级的稳定拓扑排序：

1. 依赖边永远优先，依赖模块必须先于消费者；
2. 当前所有入度为零的模块中，`Order` 较大者优先；
3. `Order` 相同时按 Key 升序。

因此 `order` 与 Node 一样表达“越大越优先”，但不能覆盖真实依赖。若 A 配置了更高 Order 却依赖 B，B 仍先于 A；这不是错误。循环才是错误。

`Specs()`、配置准备、Runtime 构建、Controller 构建和 Seed 遍历使用同一最终顺序。Runtime 停止仍使用启动逆序。

### 9.3 Go 包依赖方向

固定导入方向为：

```text
main
  -> modules (modules_gen.go)
       -> modules/<key> (config.go)
       -> modules/<key>/<component package>
            -> modules/<key> (typed Config, when needed)
            -> sibling component packages (acyclic only)
       -> cool/* framework packages
```

关键点是 Go 中 `modules` 与 `modules/<key>` 是不同包。模块子包可以导入自己的模块根配置包，但任何模块代码都不得导入父级 `modules` 包；模块根配置包也不得导入自己的子包。生成文件位于唯一能够同时导入根配置包和全部组件包的位置。

`go/packages` 若发现源码 import cycle，生成立即失败并保留 Go 的导入链。生成器依赖图检测的是构造关系循环，两类循环都必须分别报告。

## 10. 错误处理

### 10.1 生成期错误

以下情况必须阻止写入：

- 模块缺少 `config.go` 或 `ModuleConfig()`；
- 模块根出现 allowlist 之外的文件或目录；
- `ModuleConfig()` 签名或静态形态不合法；
- Name/Description 为空，Order 无法静态求值；
- 中间件引用缺失、重复、未声明或作用域错误；
- Config 字段缺少唯一 `json` 标签，或包含禁止的运行时资源类型；
- `task.HandlerDefinition` 位于 `service/**` 之外、签名非法或依赖不可解析；
- Provider 缺失、重复、歧义或循环；
- 跨模块依赖循环或 Go import cycle；
- 多个 `*recycle.Manager` Provider；
- 全局候选输出无法通过 `go/format` 或 Overlay 类型检查。

错误必须包含模块 Key、源码位置、包、符号、原因和可执行的修复方向。全部分析、渲染和 Overlay 类型检查成功后才原子替换 `modules/modules_gen.go`。

### 10.2 启动期错误

配置读取、未知字段、类型转换、`null`、校验和资源构造错误都返回 error，不使用 panic。错误格式至少包含：

```text
加载模块 task 配置 module.task.queue.concurrency 失败: ...
```

Application 再补充当前阶段，例如“准备模块配置”“构建 Runtime”或“构建 Controllers”。启动失败不得启动后续 Runtime、注册路由或执行 Schema/Seed。已启动 Runtime 的回滚和逆序停止沿用现有契约。

### 10.3 删除保护

生成器只可自动删除带标准 Generated Header 的旧 `module_gen.go`。`config/`、`handler/` 和 `providers.go` 是手写路径，必须在明确迁移后由实施变更删除，生成器不得泛化为删除任意不合规文件。

## 11. 迁移策略

迁移按能力建设和一次最终切换执行，不长期保留两种配置或两种装配机制：

1. 为最终目录、声明、配置合并、`service/**` 任务定义发现、全局图和单输出先写失败测试。
2. 在框架层增加 `module.Declaration[T]`、统一 Config Loader 和 `ModuleSpec.Configure`，但尚不切换业务入口。
3. 修改扫描器和分析器：要求根 `config.go`，解析 `ModuleConfig()`，把 Config 建成全局图节点，从 `service/**` 识别 `task.HandlerDefinition`，并校验根目录 allowlist 与中间件引用；删除 `handler` 目录分支。
4. 修改渲染器和 Writer：只在内存中生成 `modules/modules_gen.go`，完成全量 Overlay 类型检查后原子写入；移除局部生成模式。
5. 逐模块迁移声明和配置类型，同时把根 Provider 移入业务目录、把子包配置 import 改为模块根包；将 `modules/task/handler/demo.go` 的注册定义移入 `modules/task/service/demo.go`，与执行函数共置并解决符号命名冲突。此阶段允许工作分支暂时不可生成，但最终切换提交必须完整编译，不提交兼容 fallback。
6. 将 `manifest/config/config.yaml` 的顶层 `task` 移到 `module.task`，清理已由框架依赖提供的重复字段；不保留旧键读取。
7. 生成唯一 `modules/modules_gen.go`，切换 `Specs()` 到新的全局装配。
8. 删除所有模块内 `config/`、`handler/`、`providers.go`、`module_gen.go` 以及根目录测试文件，迁移测试位置；确认未新增 `db/`、`hooks/` 或 `job/`。
9. 删除生成器中 `LoadConfig` Provider、`handler/**` 发现逻辑、每模块 Renderer、局部 CLI 和旧陈旧文件规则。
10. 执行结构、配置、生成器、行为、Race、Vet 和 Build 全矩阵验收后更新模块开发文档。

模块具体迁移要求：

| 模块 | 配置与 Provider 迁移 |
| --- | --- |
| Base | 配置类型合并进根 `config.go`；保留 `allowKeys` 和中间件业务参数；SSO/i18n 等改用框架依赖；Log 构造移入 `event`；声明三个全局中间件引用 |
| Dict | 新增必需 `config.go`，声明 Node 对齐的名称、描述、Order 和空业务配置；无中间件 |
| Recycle | 配置类型合并进根；移除重复 SoftDelete 字段；Catalog/Manager 构造移入 `service` 或 `event`；保留清理业务配置 |
| Task | 配置类型合并进根；顶层 `task.*` 迁到 `module.task.*`；Redis Client 和解析 Location 不再存入 Config；基础设施创建移入 `queue`、`service` 或 `event`；把任务注册定义从 `handler/demo.go` 移入 `service/demo.go`，生成器从 `service/**` 按 `task.HandlerDefinition` 返回类型发现；声明模块中间件引用 |

## 12. 测试与验收

### 12.1 目录验收

- 每个模块根恰好有一个 `config.go`。
- 根目录除 `config.go`、`db.json`、`menu.json` 和八个认可业务目录外没有其他条目。
- 项目中不存在模块内 `config/`、`handler/`、`db/`、`hooks/`、`job/`、`providers.go`、`module_gen.go`、`register.go` 或实体汇总 `models.go`。
- 模块根 `config.go` 不导入当前模块子包，不调用 `g.Cfg()`，模块其他代码不直接调用 `g.Cfg()`。

### 12.2 配置单元测试

覆盖：

- 缺失 `module.<key>` 时使用完整默认值；
- 部分嵌套覆盖保留未配置默认值；
- 显式 `false`、`0`、空字符串和空数组不被误判为缺失；
- 未知字段、错误根类型、错误值类型和非法 `null` 失败；
- `time.Duration` 等约定类型正确转换；
- Validate 错误包含完整模块键和字段语义；
- 两次 `Specs()` 的配置和 assembly 状态互不共享；
- 不读取旧 `task.*` 或其他兼容前缀。

### 12.3 生成器测试

覆盖：

- 缺失或非法 `ModuleConfig()`；
- 中间件引用的成功解析、缺失、重复、漏声明和作用域冲突；
- 单文件包含全部模块静态调用，项目中不产生模块内输出；
- Config 节点注入和跨模块 Provider 解析；
- `service/demo.go` 中 `task.HandlerDefinition` 能被发现和装配，普通 Service/Provider 不受影响；
- `service/**` 下非法 `task.HandlerDefinition` 签名生成失败，`service/**` 外返回该类型生成失败，项目不再扫描或生成 `handler/**`；
- 模块拓扑以依赖优先、Order 降序、Key 升序稳定排列；
- Provider 循环、模块循环和 Go import cycle 分别给出链路；
- 不同文件创建顺序生成完全相同内容；
- 生成失败保留旧全局文件；内容未变不改写 mtime；
- `check` 不写磁盘并识别旧模块生成文件；
- 输出不含 `init()`、空白导入、`reflect` 或第三方运行时 DI。

### 12.4 行为回归

迁移前后除模块元信息和加载顺序按 Node 声明恢复外，下列行为必须一致：

- 模型、表名、资源名和 Schema；
- Controller、CRUD、自定义路由、权限和 EPS；
- 全局与模块中间件集合、Order 和作用域；
- Runtime、Recycle、Task 任务注册与执行生命周期；
- DB/Menu Seed 路径和导入行为；
- Base、Dict、Recycle、Task 的现有单元与集成场景。

### 12.5 最终命令

```text
cool check
go test ./cool/codegen/module ./cool/module ./cool/registry ./cool/app ./modules/... -count=1
go test ./... -count=1
go test -race ./cool/app ./cool/registry ./cool/module ./modules/... -count=1
go vet ./...
cool build
git diff --check
```

主应用依赖仍不得包含 `golang.org/x/tools`；生成器依赖只存在开发工具链。请求路径不得增加配置查找、反射构造或 Provider 解析。

## 13. 对原设计与计划的修订

本设计取代原 2026-07-30 设计/计划中的以下决定：

| 原决定 | 修订后 |
| --- | --- |
| 模块属性由目录推导，Name=Key、Description 空、无人工 Order | `config.go` 必须声明 Name、Description 和 Order |
| 根 `LoadConfig(context.Context)` 是强类型 Provider | 删除 `LoadConfig` 约定，由框架统一加载 `module.<key>` |
| `config/` 保存可供子包导入的配置类型 | Config 类型直接位于模块根，子包单向导入根包 |
| Middleware 仅由目录自动纳入 | 目录负责发现，`config.go` 负责显式清单，两者必须一一对应 |
| 每模块生成 `module_gen.go`，全局文件只调用 `Module()` | 所有装配只生成到 `modules/modules_gen.go` |
| 依赖图主要按模块分别解析，仅额外共享模型 | 使用项目级组件图并派生模块依赖图 |
| 模块启动同级按 Key 排序 | 依赖优先，同级按 Order 降序、Key 升序 |
| `generate --module` 可局部写单个模块文件 | 取消局部生成，所有命令分析和生成全局输出 |
| `providers.go` 可承载根装配桥接 | 根目录禁止 Provider；构造函数归入业务拥有目录 |
| `handler/**` 是 Task 注册定义的专属目录 | 删除该 Go 自创目录；注册定义与实现共置于 `service/**`，生成器按 `task.HandlerDefinition` 返回类型发现 |
| Task 使用顶层 `task.*`，各模块自行读取配置 | Task 迁到 `module.task.*`，所有模块由框架统一加载 |

以下原决定继续有效：

- `go/packages` 加 AST/types 的构建期分析；
- 标准 Provider 命名、接口唯一实现、模型精确名称绑定和原始标量禁注入；
- `registry.ControllerProvider` 是唯一 Controller 集合惰性边界；
- 生成结果确定性、`go/format`、Overlay 类型检查和原子写入；
- 生成文件提交 Git，CI 使用 `cool check`；
- 主程序不依赖 x/tools，不使用运行时反射 DI，不改变单二进制部署。

原实施计划不能在现状上继续追加任务。应以本节修订为输入重写受影响任务，尤其是原 Task 4、6 至 13；已经完成且不冲突的扫描、类型分析、Runtime 生命周期和显式 `modules.Specs()` 基础可保留测试与实现。

## 14. 完成定义

同时满足以下条件才视为迁移完成：

1. 模块目录通过八目录 allowlist，不存在 `config/`、`handler/`、`db/`、`hooks/`、`job/` 及任何旧根职责文件；
2. 每个模块的元信息、中间件和默认配置只在 `config.go` 声明；
3. 模块业务配置只从 `module.<key>` 统一加载且启动前完成校验；
4. 项目只有一个生成装配文件 `modules/modules_gen.go`；
5. Task 注册定义只位于 `service/**`，`service/demo.go` 的 `task.HandlerDefinition` 能由生成器发现并集中装配；
6. 全局依赖图、模块顺序、循环诊断和 import 方向均通过测试；
7. 全量测试、Race、Vet、`cool check` 和 `cool build` 通过，且现有业务行为无未声明回归。
