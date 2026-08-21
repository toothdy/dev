# 模块 40：静态 Application 装配与组件生命周期设计

> 日期：2026-08-13  
> 状态：待复核  
> 模块：40 Application Graph 与组件生命周期  
> 对应拆分项：40.1-40.11  
> 前置模块：03 配置加载、12 Provider 依赖图、14 静态模块注册表

## 1. 文档定位

本文补全总架构中 `modules.Generated()` 到 `app.Run()` 之间尚未冻结的运行装配契约，并解决模块 14 的纯静态 `module.Graph` 与当前组件级 `Factory/Registry` 实现之间的冲突。

事实来源按以下顺序解释：

1. 本设计；
2. 模块 03、09、10、12、14、15 的已批准契约；
3. 模块拆分设计；
4. 总架构设计。

本文不改变业务模块、Service、Controller、生命周期方法或根 `main.go` 的既定写法。

## 2. 决策

模块 40 采用“纯静态 Graph + 单一生成装配函数 + 运行期 Assembly”方案：

```text
业务源码
-> cool generate 静态分析
-> module.Graph：纯静态、不可执行元数据
-> 生成 assemble：直接、强类型调用构造器
-> app.Assembly：已构造组件的生命周期与 Transport 接口
-> Application Host：Init、Prepare、Start、Ready、Stop
```

固定原则：

1. `module.Graph` 不保存组件 `Factory`、实例、运行期 Registry 或 `any` Provider 值；
2. 生成文件按 Provider 拓扑直接调用具体 Go 构造函数，依赖通过具体类型局部变量传递；
3. 不提供 `Resolve[T]`、字符串 Service Locator、反射 DI 或第三方 DI 容器；
4. `app.Assembly` 只保存明确的生命周期接口和 `Transport`，不提供通用实例查询；
5. Host 负责阶段协调、清理资格、回滚和错误聚合，不解释依赖图；
6. 配置、Database Runtime、Recycle、Base、Transport 和后续 Outbox 组件均由生成装配函数直接调用各自构造器创建；
7. `core/app` 不导入 HTTP、gRPC、Outbox 或业务模块实现包。

## 3. 保持不变的业务写法

业务组件继续只声明普通 Go 构造函数：

```go
func NewGoodsService(
   base *coreservice.Base[entity.Goods, uint64],
   config Config,
   audit contract.AuditWriter,
) (*GoodsService, error)
```

依赖完全由参数静态类型推导。业务代码不声明 Provider 名称、不手写依赖列表，也不调用 Graph、Assembly 或装配 API。

生命周期继续通过可选接口声明：

```go
func (*GoodsService) OnInit(context.Context) error
func (*GoodsService) OnStart(context.Context) error
func (*GoodsService) OnStop(context.Context) error
```

Controller 工厂继续直接接收具体 Service：

```go
func GoodsController(service *GoodsService) controller.Definition
```

入口保持：

```go
func main() {
   ctx := gctx.GetInitCtx()
   if err := app.Run(ctx, modules.Generated()); err != nil {
      g.Log().Error(ctx, err)
      os.Exit(1)
   }
}
```

## 4. 静态 Graph

`module.Graph` 只保存生成期可验证事实：

- 模块 Identity、Order 和声明元数据；
- Descriptor 元数据；
- Provider 类别、静态类型和归属；
- 组件身份、依赖边和拓扑顺序；
- Initializer、Starter、Stopper 和 Transport 能力标记；
- Controller、Route、Producer、Consumer 和后续专项定义的静态注册信息。

Graph 内部字段保持私有，集合访问器返回防御性副本。Graph 包含一个私有“已验证”状态，只能由 `BuildGraph` 在全部校验成功后置位；因此 `BuildGraph(GraphInput{})` 产生的合法空应用与未构建的 Go 零值 `Graph{}` 可以明确区分。Graph 构建器校验完整性、唯一性、Provider 对齐、依赖拓扑和静态注册一致性，但不得调用默认配置工厂、Schema 函数或业务构造器。

本设计覆盖模块 14 草案中与运行期值有关的字段：`GraphInput` 不再接收 `Values`，`StaticModule` 不再保存 `DefaultFactory`，Graph 也不保存任何默认配置函数。模块声明的默认值只由生成装配代码在调用 `module.Compile[T]` 时直接引用；模块 14 中“默认配置工厂属于静态模块描述”的表述以本条为准。

以下运行行为从 `core/module` 删除：

- `Factory func(context.Context, *Registry) (any, error)`；
- `Registry` 和 `Resolve[T]`；
- `ValueDefinition` 运行期值工厂；
- `Lifecycle.Construct`；
- 为运行期实例 nil 检测引入的反射。

模块默认配置工厂和 Descriptor helper 仍可作为生成文件中的强类型函数存在，但不作为可执行值写入 Graph。

## 5. Definition 与装配输入

`modules.Generated()` 改为返回不可变 `app.Definition`，但调用形式不变：

```go
type Definition struct { /* private graph and assembler */ }

type AssembleInput struct { /* private root configuration view */ }

type AssembleFunc func(
   context.Context,
   AssembleInput,
) (*Assembly, error)

func Define(module.Graph, AssembleFunc) Definition
func Run(context.Context, Definition) error
```

`Definition` 私有保存纯静态 Graph 和唯一生成装配函数。`Define` 拒绝未经过 `BuildGraph` 验证的 Graph 或 nil 装配函数，但接受由 `BuildGraph(GraphInput{})` 产生的合法空应用；`cool check` 禁止业务模块直接调用 `Define` 或 Assembly 登记 API。

`Run` 在调用生成的 `AssembleFunc` 前，由模块 46 实现的 Host loader 根据当前启动上下文和已验证 Graph 创建 `AssembleInput`。因此生产入口仍只需调用 `app.Run(ctx, modules.Generated())`；生成包不读取文件、不解析根配置路径，也不自行创建 `AssembleInput`。loader 的默认配置文件定位、根 YAML 解析和模块 Source 切片属于模块 46，`core/app` 只消费其返回的协议无关视图。

生成的 `Generated()` 先构造一次 Graph，再用闭包捕获其中对应 `StaticModule.Identity()` 和其他静态描述，最后把参数完全匹配的闭包传给 `app.Define`。这样装配函数使用的是 Graph 已验证的身份，不从字符串重新创建 `module.Identity`，也不重复执行 Graph 构建。

`AssembleInput` 只承载 Host 已读取的根配置视图。模块 40 冻结的唯一公开读取契约是：

```go
func (AssembleInput) ModuleSource(module.Identity) configuration.Source
func (AssembleInput) RootSource() configuration.Source
```

`ModuleSource` 按模块身份返回隔离的配置来源副本；不存在对应配置时返回空 `configuration.Source`，使模块默认配置仍可正常编译。`RootSource` 返回不属于任何业务模块的基础设施配置来源副本，供生成代码中的 Database、Transport 等强类型 helper 使用。两个方法都必须复制 `Main` 字节切片，保留 `LookupEnv` 函数，不得暴露内部配置树。`AssembleInput` 不得包含通用 Provider map，不得承载 Database、Recycle 或业务实例，也不得提供按类型或名称查询实例的能力。

根配置文件定位、多模块配置树布局以及生产 `AssembleInput` 的创建属于模块 46。模块 40 的 Host 单元测试在 `core/app` 包内使用测试专用的内存 loader 注入 Source，不增加生产 API，也不提前冻结模块 46 的文件路径策略。

Database Runtime、Recycle Store、Base Service、HTTP/gRPC Adapter 和 Outbox 组件不放入通用 Bootstrap 容器。Host 只负责读取根配置并构造不可变 `AssembleInput`；生成装配函数负责从 `RootSource` 或具体模块 Source 取得配置，通过强类型 helper 解码基础设施配置，并直接调用对应包的构造器。

## 6. 生成装配

生成文件包含一个私有 `assemble`，并按已验证拓扑生成直接调用。`Generated()` 负责绑定 Graph 中的静态 Identity：

```go
func Generated() app.Definition {
   graph := buildGraph()
   goodsIdentity := graph.Modules()[goodsModuleIndex].Identity()
   return app.Define(graph, func(ctx context.Context, input app.AssembleInput) (*app.Assembly, error) {
      return assemble(ctx, input, goodsIdentity)
   })
}
```

装配主体保持普通强类型函数：

```go
func assemble(
   ctx context.Context,
   input app.AssembleInput,
   goodsIdentity module.Identity,
) (*app.Assembly, error) {
   assembly := app.NewAssembly()

   databaseConfig, err := generatedDatabaseConfig(
      ctx,
      input.RootSource(),
   )
   if err != nil {
      return assembly, err
   }

   runtime, err := coredb.New(ctx, databaseConfig)
   if err != nil {
      return assembly, err
   }

   recycler, err := corerecycle.New(
      runtime,
      generatedRecycleConfig(input),
      goodsDescriptor(),
   )
   if err != nil {
      return assembly, err
   }

   auditWriter, err := generatedAuditWriter(ctx, runtime)
   if err != nil {
      return assembly, err
   }

   goodsConfig, err := module.Compile(
      ctx,
      goodsIdentity,
      goods.ModuleConfig(),
      input.ModuleSource(goodsIdentity),
   )
   if err != nil {
      return assembly, err
   }

   goodsBase, err := coreservice.NewBase(
      goodsDescriptor(),
      runtime,
      recycler,
   )
   if err != nil {
      return assembly, err
   }

   goodsService, err := goodsservice.NewGoodsService(
      goodsBase,
      goodsConfig.Config(),
      auditWriter,
   )
   if err != nil {
      return assembly, err
   }

   goodsController := goodscontroller.GoodsController(goodsService)
   httpTransport, err := httpadapter.New(
      generatedHTTPConfig(input.RootSource()),
      []controller.Definition{goodsController},
   )
   if err != nil {
      return assembly, err
   }

   assembly.AddComponent(goodsComponent, app.Hooks{
      Initializer: goodsService,
      Starter:     goodsService,
      Stopper:     goodsService,
   })
   assembly.AddTransport(httpComponent, httpTransport, app.Hooks{})

   return assembly, nil
}
```

示例中的构造器名称只表达生成形态，不冻结模块 42/43 的 Adapter 参数细节，也不要求所有组件实现全部生命周期接口。Controller 工厂在其 Service 依赖构造完成后由生成代码直接调用，所得不可变 `controller.Definition` 只作为强类型参数传给对应 Transport Adapter；Host 不查询 Controller 或 Service。基础设施若具有生命周期能力，生成器同样须在其构造成功后按 Graph 拓扑登记；为突出业务依赖传递，示例省略了这些重复的 `AddComponent` 调用。生成器只写组件实际实现的接口字段。

每个构造结果使用具体静态类型局部变量。接口依赖由 Go 的可赋值规则在生成期选择唯一具体变量，并由生成文件的真实编译再次证明。禁止把局部变量写入 `map[string]any` 后再解析。

构造器成功后，生成代码立即把其生命周期能力登记到部分 Assembly。能力登记不等于进入 Host 清理栈：后续构造器失败时，Host 根据已登记前缀和第 8 节规则计算清理资格，只回滚此时已经具备资格的组件。

任何已构造且拥有外部资源的基础设施对象都必须满足其一：作为 Graph 中的组件登记到 Assembly 并由对应 `Stopper` 释放，或明确由已登记父组件拥有并在父组件的 `OnStop` 中释放。示例中的 `runtime`、`recycler` 和 `auditWriter` 仅表示依赖传递；若具体实现持有独立外部资源，生成器必须为其生成对应的基础设施组件和生命周期登记，不得把资源留在局部变量中跨过后续构造失败点。生成器必须在每个资源构造成功后立即登记其所有权组件，或使用已登记父组件的所有权；不能等全部业务组件构造成功后再补登记。

## 7. Assembly

`app.Assembly` 是一次启动尝试的运行结果，不是 DI 容器：

```go
type Hooks struct {
   Initializer module.Initializer
   Starter     module.Starter
   Stopper     module.Stopper
}

type Assembly struct { /* private ordered components and transports */ }

func NewAssembly() *Assembly
func (*Assembly) AddComponent(module.ComponentDefinition, Hooks)
func (*Assembly) AddTransport(module.ComponentDefinition, Transport, Hooks)
```

`AddTransport` 将 Transport 实例及其所属组件的可选生命周期 Hooks 作为一次登记写入 Assembly；Transport 组件不得再调用 `AddComponent` 重复登记。非 Transport 组件只调用 `AddComponent`。Assembly 只支持生成代码按拓扑追加，不提供按字符串、类型、接口或组件身份返回业务实例的查询方法。组件间依赖必须在生成函数的局部变量中完成。

Host 在执行任何 `OnInit` 前，将 Assembly 与 Graph 做一次完整一致性校验：

- 组件身份和顺序与 Graph 相同；
- 生命周期接口的有无与静态能力标记相同；
- Transport 身份、顺序和能力与 Graph 相同；
- 不存在重复、未知或遗漏登记。

一致性失败属于 Core 启动错误，并按已登记清理资格回滚。

若 `assemble` 返回错误，Host 不执行要求“完整无遗漏”的全量校验，而是先校验返回的部分 Assembly 必须是 Graph 拓扑的合法连续前缀，且前缀内身份、顺序和能力完全一致。部分 Assembly 为 nil、包含越序、重复、未知或能力不匹配登记时，Host 将其作为 Core 装配契约错误与原始构造错误聚合；随后仍只按可信的合法前缀执行构造阶段回滚。

若 `assemble` 返回 nil Assembly 且 error 为 nil，Host 将其视为 Core 装配契约错误；只有非 nil Assembly 才能进入部分前缀校验或完整 Assembly 校验。

## 8. 生命周期与清理

阶段顺序固定为：

```text
读取并校验根配置
-> 校验静态 Graph
-> 调用生成 assemble 构造全部组件
-> 校验 Assembly 与 Graph 一致
-> 按拓扑 OnInit
-> Prepare 全部启用 Transport
-> 按拓扑 OnStart
-> Start 全部 Transport
-> 标记 Ready
```

清理资格保持总架构原规则：

1. 实现 Stopper 但不实现 Initializer 的组件，在构造并登记成功后立即具备清理资格；
2. 同时实现 Initializer 和 Stopper 的组件，仅在 `OnInit` 成功后具备清理资格；
3. `OnInit` 失败的当前组件不进入 Host 清理栈，由组件自行清理未完成初始化产生的局部资源；
4. `OnStart` 不影响清理资格；
5. `OnStop` 严格按实际成功登记清理资格的逆序执行；
6. 每个组件的 `OnStop` 最多执行一次；
7. 单个停止失败不阻止后续清理，错误按执行顺序使用 `errors.Join` 确定性聚合；
8. 构造、初始化、Prepare 或 Start 任一失败均返回原始 Cause 并回滚。

Assembly 只记录静态声明对应的运行期能力；Host 为每次启动尝试独立维护清理栈和“已停止”状态。构造阶段结束时，Host 先把“有 Stopper、无 Initializer”的合法登记压入清理栈；之后仅在每个 `OnInit` 成功时把“同时有 Initializer 和 Stopper”的组件压栈。该状态不写回 Graph 或 Assembly，避免重复运行或并发关闭共享可变状态。

并发 `Stop`、运行期 Transport 终止监督、Ready 撤销和全局关闭 Deadline 继续由模块 41承担。

## 9. 与其他模块的边界

| 模块 | 固定边界 |
|---|---|
| 03/10 配置 | Host 提供根配置输入；生成函数使用具体 `module.Compile[T]` 产生模块 Config |
| 09 Database | 生成函数直接调用 `db.New`；`core/app` 不保存数据库 Provider map |
| 12 Provider 图 | 继续负责唯一 Provider、接口可赋值、循环和稳定拓扑，不建立运行期容器 |
| 14 静态注册表 | Graph 保持无 Factory；模块 40 将生成入口从 `Generated() module.Graph` 具体化为保存 Graph 和 `assemble` 的 `Generated() app.Definition`，静态注册字段仍由模块 14 负责 |
| 15 生成流水线 | 唯一输出仍是 `modules/modules_gen.go`，继续执行格式化、类型检查和原子替换 |
| 25/30/31 CRUD 与路由 | 生成函数把已构造 Service 直接传给 Controller 工厂；CallableRef 只用于静态校验和文档元数据 |
| 41 Host | `[]Transport` 来自 Assembly；Graph 只保存并校验 Transport 静态身份 |
| 42/43 Transport | 生成函数直接构造具体 Adapter，Host 只依赖 `Transport` 接口 |
| 45 EPS/OpenAPI | 继续只消费 Graph 中的 Descriptor 与静态路由元数据 |
| 46 应用入口 | `app.Run(ctx, modules.Generated())` 不变；模块 46 实现根配置定位和完整启动输入 |
| 53/55/57 Outbox | Worker、Publisher、Consumer Adapter 直接强类型构造并登记生命周期或 Transport |
| 60-62 Event/Schedule/Queue | 后续专项复用同一生成装配和 Assembly，不增加第二套容器 |

任务 41 的“由生成 Graph 注入 `[]Transport`”解释为：Graph 静态声明并校验 Transport，生成装配函数将对应具体实例登记到 Assembly，Host 从已校验 Assembly 取得 `[]Transport`。

总架构中的 `app.Run(context.Context, module.Graph)` 由本设计具体化为 `app.Run(context.Context, app.Definition)`；根入口源码不变。

## 10. 错误与诊断

- 生成期缺失、歧义、循环、非法生命周期签名和静态注册冲突继续返回稳定 codegen Diagnostic；
- Graph 构建失败不得执行配置、Schema 或业务构造函数；
- 构造器错误保留原 Cause，由 Host 添加组件身份与“构造失败”上下文；
- Assembly/Graph 不一致返回 Core 启动错误，不暴露配置值或连接信息；
- 回滚错误与启动错误确定性聚合，启动错误始终排在清理错误之前；
- Host 不 Fatal、不调用 `os.Exit`，最终错误交给根入口。

## 11. 验收

必须覆盖：

1. Graph 不含组件 Factory、Registry、Resolve 或运行期实例；
2. 生成文件直接调用带/不带 error 的构造器，并保持参数原顺序；
3. Config、Descriptor、Base、具体组件和接口依赖均通过真实临时 workspace 编译；
4. 构造顺序遵循拓扑，同层按 Order、模块路径和符号稳定排序；
5. 构造中途失败返回部分 Assembly，并只清理已具备资格的组件；
6. Init、Start、Stop 顺序和清理资格符合第 8 节；
7. Stopper 最多执行一次，停止错误不短路且聚合顺序稳定；
8. Assembly 缺失、重复、乱序或能力不匹配时启动失败；
9. Transport 通过 Assembly 注入且与 Graph 静态声明一致；
10. 生成结果无反射 DI、字符串 Service 查找和通用 `map[string]any` Provider；
11. `BuildGraph(GraphInput{})` 可定义合法空应用，Go 零值 `Graph{}` 被 `Define` 拒绝；
12. 构造失败返回的部分 Assembly 只接受 Graph 拓扑的合法连续前缀；
13. 相同输入重复生成字节一致；
14. `go test -race` 覆盖重复停止和并发关闭。

固定门禁：

```text
go test ./cool-next/core/module ./cool-next/core/app ./cool-next/codegen -count=1
go test -race ./cool-next/core/module ./cool-next/core/app ./cool-next/codegen -count=1
go test ./... -count=1
go vet ./...
make check
git diff --check
```

## 12. 非目标

模块 40 不实现：

- 根配置文件路径和最终多模块 YAML 布局；
- HTTP/gRPC Adapter 细节；
- Ready Gate、信号处理和运行期终止 Channel 监督；
- EPS/OpenAPI 输出；
- Outbox Store、Worker 或 Broker Adapter；
- Event、Schedule 和 Queue 的业务 Handler 契约；
- 通用作用域、限定符、懒加载、可选 Provider 或运行时覆盖机制。

这些能力不得以“后续扩展”为理由在 Graph 或 Assembly 中预留通用容器 API。

## 13. 完成标准

1. 业务构造器、Controller、生命周期和根入口写法保持总架构既定形式；
2. Graph 恢复为模块 14 定义的纯静态蓝图；
3. 生成装配使用直接、强类型调用，不存在组件级 Factory/Registry；
4. Assembly 与 Graph 在启动前完整对齐；
5. 生命周期顺序、清理资格、失败回滚和错误聚合满足 40.1-40.11；
6. 与模块 03、09、10、12、14、15、31、41、46 和 57 的边界按第 9 节闭合；
7. 全部专项测试、全量测试、Race Test 和静态门禁通过。
