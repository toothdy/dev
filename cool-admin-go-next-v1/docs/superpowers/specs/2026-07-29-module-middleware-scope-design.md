# Go Next 模块中间件作用域设计

## 1. 目标

`cool-admin-go-next` 的中间件需要与 Node/Midway 版保持一致的作用域语义：

1. `GlobalMiddlewares` 对整个应用生效。
2. `Middlewares` 仅对当前模块的 Controller 路由生效。
3. 模块作者只在 `ModuleSpec` 中配置一次，不需要为 Controller 或单个接口重复绑定。
4. 中间件作用域由已编译的 Controller 元数据决定，不由 Handler 在请求期自行猜测模块。
5. 模块中间件使用 GoFrame Handler 绑定中间件链，避免为每条路由增加额外的全局中间件路由匹配项。

## 2. 现状与问题

当前 `ModuleSpec.Middlewares` 会由 Application 汇总，最终全部通过 `server.Use` 注册。因此它只表达“由模块提供”，没有表达“仅对模块路由生效”。

目前的 Task 中间件虽然由 Task 模块声明，但实际会进入所有动态请求，再由 Handler 内部判断 HTTP method 和 path 决定是否执行保护逻辑。这与 Node 版 `middlewares` 的模块作用域不一致，也会让新模块中间件重复实现路径过滤。

## 3. 用户端 API

### 3.1 ModuleSpec

中间件构建函数保留现有的显式依赖注入和启动错误返回能力：

```go
type MiddlewareFactory func(
    deps MiddlewareDeps,
) ([]middleware.Definition, error)

type ModuleSpec struct {
    Key               string
    Name              string
    GlobalMiddlewares MiddlewareFactory
    Middlewares       MiddlewareFactory
    Controllers       func(Deps) []controller.Definition
}
```

字段语义：

| 字段 | 作用域 |
| --- | --- |
| `GlobalMiddlewares` | 所有动态请求 |
| `Middlewares` | 当前 `ModuleSpec.Key` 所属 Controller 的路由 |

`Middlewares` 从现有的全局语义切换为模块语义。这是已确认的兼容性变更；外部模块如果需要保持全局行为，必须迁移到 `GlobalMiddlewares`。

### 3.2 模块配置写法

模块配置使用命名构建函数，避免在 `ModuleSpec` 中内联复杂逻辑：

```go
func buildTaskMiddlewares(
    deps registry.MiddlewareDeps,
) ([]coolMiddleware.Definition, error) {
    runtime, ok := deps.Runtime.(*taskEvent.Comm)
    if !ok || runtime == nil {
        return nil, gerror.New("Task Comm Runtime 注入失败")
    }

    return []coolMiddleware.Definition{
        taskMiddleware.Definition(runtime.Info()),
    }, nil
}

func init() {
    registry.RegisterModule(registry.ModuleSpec{
        Key:         "task",
        Name:        "任务调度",
        Middlewares: buildTaskMiddlewares,
        Controllers: buildTaskControllers,
    })
}
```

模块作者在配置中只写一次 `Middlewares: buildTaskMiddlewares`。新增的 Task CRUD 或自定义 Action 只要通过标准 Controller 元数据声明，就会自动获得 Task 中间件。

### 3.3 与 Node 版的对应

```text
Node: middlewares: [TaskMiddleware]
Go:   Middlewares: buildTaskMiddlewares
```

两者的配置次数和作用域一致。Node 由 Midway 容器实例化 Class 并注入依赖；Go 由 Application 在 Runtime 就绪后调用构建函数，显式传入 `MiddlewareDeps` 并接收 `error`。

## 4. 作用域模型

每条编译路由保留 Controller 的模块标识：

```text
compiledRoute
  method
  path
  key
  module
  handler
```

中间件链固定为：

```text
核心全局中间件
  -> 模块贡献的全局中间件
    -> 当前路由所属模块的中间件
      -> Action/CRUD Handler
```

全局定义按 `Order` 稳定排序；每个模块的定义在模块内按 `Order` 稳定排序。全局与模块作用域之间不使用 `Order` 交叉；全局链始终包裹模块链，与 Node 版“应用中间件 -> Controller 路由中间件”的层级一致。

## 5. 编译与绑定

### 5.1 启动数据流

```text
构建 Runtime 和 Controller
  -> 验证 ModuleSpec.Key 与 Controller.Module
  -> 构建 GlobalMiddlewares 和 Middlewares
  -> 全量验证中间件定义
  -> 编译所有 Action/CRUD 路由
  -> 注册全局中间件
  -> 按模块绑定已编译路由和模块中间件
```

所有可预见的配置、作用域、中间件和路由错误必须在修改 GoFrame Server 之前返回。

### 5.2 内部 RouterGroup

RoutePlan 根据 `compiledRoute.module` 选择对应的模块中间件。框架为每个拥有局部中间件的模块创建内部根 RouterGroup：

```text
Task RouterGroup("/")
  middleware: task.health
  routes:
    POST:/admin/task/info/add
    POST:/admin/task/info/start
    GET:/admin/task/info/page
    GET:/app/task/info/page
```

RouterGroup 只是 `cool/controller` 绑定层的实现细节，不作为模块或插件必须使用的公开注册 API。根 Group 使用已编译的完整路由键，因此不依赖 `/admin/<module>` 或 `/app/<module>` 的 URL 前缀分组，一个模块可以同时提供 Admin 和 App 路由。

绑定时按 RoutePlan 的已验证顺序逐条提交，不使用 map 迭代决定路由顺序。只支持 `cool/route` 已声明的 HTTP method，绑定层不再宽松接受其他 method。

### 5.3 性能原则

GoFrame RouterGroup 会将 Group 中间件存入目标 Handler 的 `Middleware` 数组。请求命中业务路由后，直接执行该 Handler 的局部中间件链。

模块中间件不使用每路由 `server.BindMiddleware(METHOD:path)` 实现，因为后者会为每条路由创建额外的 `HandlerTypeMiddleware` 路由项，增加路由表和请求匹配工作。

全局中间件数量少且必须匹配全局请求，继续通过 `server.Use` 注册。

## 6. 验证与错误处理

### 6.1 模块归属

每个 `ModuleSpec.Controllers` 返回的 Controller 必须满足：

```text
controller.Module == moduleSpec.Key
```

不一致时 Application 启动失败，错误同时包含 ModuleSpec key、Controller name/prefix 和 Controller module。禁止在不一致时静默按 URL 重新猜测归属。

### 6.2 中间件定义

全局和所有模块定义展平后统一验证：

1. `Name` 不能为空。
2. 名称在全局和所有模块之间必须唯一。
3. `Handler` 不能为空。
4. 普通模块不能使用 `cool.*` 保留名称。
5. 普通模块不能占用核心保留 Order。

全量验证负责跨作用域的唯一性和保留值检查；全局列表和每个模块列表再分别稳定排序，不把不同作用域的 Order 混成一条执行链。

### 6.3 运行时错误

`cool.recovery` 和 `cool.error` 保持强制全局注册，且位于模块 RouterGroup 中间件外层。模块中间件设置的 request error 或抛出的 panic 继续由核心错误边界统一记录和渲染。

## 7. Override 兼容性

`MiddlewareOverride` 的两种 mode 保持现有名称，语义扩展到两种模块贡献：

| Mode | 行为 |
| --- | --- |
| `append` | 保留模块贡献的全局和局部中间件，将 override definitions 追加为全局中间件 |
| `replace-modules` | 跳过模块贡献的全局和局部中间件，将 override definitions 注册为全局中间件 |

`MiddlewareDefinitions` 继续映射为 `replace-modules`。`cool.recovery`、`cool.error` 和 Application 强制注册的运行时中间件不受 override 替换。

Override definitions 不携带模块标识，因此本次不新增“通过 Application Options 注入某模块局部中间件”的 API。模块局部行为必须由对应 `ModuleSpec.Middlewares` 声明。

## 8. 内置模块迁移

| 模块 | `GlobalMiddlewares` | `Middlewares` |
| --- | --- | --- |
| Base | translate、authority、permission、log | 空 |
| Task | 空 | task.health |
| Dict | 空 | 空 |
| Recycle | 空 | 空 |

Base 的四个中间件保持全局，因为它们需要处理 Base 以外的 Admin/App 路由。Task 的 `task.health` 只进入 Task 路由链，但仍保留对 add、update、start、stop、once 等需要健康检查的写操作过滤。

## 9. 测试与性能验证

### 9.1 单元和集成测试

1. Base 声明 `GlobalMiddlewares`，不声明 `Middlewares`。
2. Task 声明 `Middlewares`，不声明 `GlobalMiddlewares`。
3. Dict 和 Recycle 两种声明均为空。
4. Task 模块中间件对 Task CRUD 和自定义 Action 生效。
5. Task 模块中间件不进入 Base、Dict、`/health` 和不存在的路由。
6. 同一模块的 Admin 和 App 路由都获得模块中间件。
7. 相同 path 的不同 HTTP method 不串用中间件或 Handler。
8. 全局中间件包裹模块中间件，模块内顺序按 `Order` 稳定。
9. 模块中间件的 error 和 panic 由核心错误边界处理。
10. Controller 模块归属不一致时启动失败且 Server 不包含部分新路由。
11. 全局与模块之间的重名定义在启动时失败。
12. `append`、`replace-modules` 和 `MiddlewareDefinitions` 兼容语义通过回归测试。

### 9.2 Benchmark

在 `cool/controller` 增加路由绑定和请求基准，对比：

1. 无模块中间件的基线路由。
2. 通过 RouterGroup 附着一个模块中间件的路由。
3. 多模块、多路由情况下的启动绑定开销。

验证记录 `ns/op`、`B/op` 和 `allocs/op`。基准不设置对共享 CI 环境敏感的绝对时间阈值；实现验收时保留基准数据，并确认模块中间件没有产生每请求的路由集合构建、反射扫描或模块查找。

## 10. 非目标

1. 本次不新增 Controller 级或 Route 级中间件 API。
2. 本次不将 RouterGroup 暴露给模块或插件作者。
3. 本次不支持运行时动态增删模块路由或中间件。
4. 本次不改变 Base 鉴权、权限、日志和翻译的业务规则。
5. 本次不为 Application override 增加模块定向注入接口。
