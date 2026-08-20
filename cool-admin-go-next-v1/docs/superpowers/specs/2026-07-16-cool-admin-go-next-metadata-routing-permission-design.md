# cool-admin-go-next 阶段 6B Metadata 驱动路由和权限设计

日期：2026-07-16

## 1. 背景

阶段 6A 目标是把 base 模块源码结构对齐 Node 风格：`controller/`、`service/`、`model/` 分层，并通过 Fluent Builder 声明 controller metadata。

阶段 6B 在 6A 的 metadata 基础上，替换当前分散的手写注册逻辑，让路由注册、auth ignore paths、CRUD specs、权限映射都从同一份 controller metadata 派生。

当前阶段 5B 的注册方式仍是手写串联：

```go
baseModule.RegisterAuthRoutes(a.server, authService)
a.server.Use(auth.NewMiddleware(...))
baseModule.RegisterPermissionRoutes(a.server, permissionService)
baseModule.RegisterPermissionMiddleware(a.server, permissionService)
modules.RegisterRoutes(a.server, runtime)
```

这会产生长期维护问题：

1. open/comm 路由在 auth routes 文件中维护。
2. permission routes 单独维护。
3. CRUD routes 由 `crud.ResourceSpec` 维护。
4. 权限映射由另一套逻辑派生。
5. 后续 EPS 还会需要接口清单。

阶段 6B 的目标是让这些全部回到 controller metadata 单一来源。

## 2. 目标

阶段 6B 完成后应满足：

1. `cool/controller` 能根据 controller metadata 注册 HTTP 路由。
2. Auth ignore paths 从 controller metadata 中的 `IgnoreAuth` 派生。
3. CRUD `ResourceSpec` 从 admin controller metadata 派生。
4. 权限映射从 controller metadata 派生。
5. app 注册流程由 metadata 统一驱动，替代 base 模块手写注册串联。
6. 现有 HTTP 行为不变。
7. 阶段 6C 能直接复用 controller metadata 生成 EPS。

## 3. 非目标

阶段 6B 不做：

1. `/admin/base/open/eps` 的最终 EPS JSON 输出。
2. Vue 前端真实联调。
3. user move、menu parse/create/export/import、department order、param html、log clear/setKeep/getKeep 的业务实现。
4. CRUD before/after hooks 接入。
5. 插件系统或运行时动态模块加载。
6. CLI 代码生成。

## 4. 总体设计

阶段 6B 将 app 注册流程改成：

```text
/health
  ↓
collect controllers from modules
  ↓
register IgnoreAuth routes
  ↓
auth middleware with IgnorePathsFromControllers
  ↓
register protected non-CRUD routes
  ↓
permission middleware from PermissionMapFromControllers
  ↓
register CRUD routes from ResourceSpecsFromControllers
```

关键原则：

1. controller metadata 是路由、权限、CRUD、后续 EPS 的唯一接口来源。
2. open/comm/admin route 的 HTTP path 和 method 不变。
3. permission middleware 必须在 auth middleware 后、CRUD route 前。
4. IgnoreAuth 路由必须在 auth middleware 前注册或被 middleware 放行。
5. CRUD route 注册仍复用 `cool/crud` runtime，不重复实现 CRUD 行为。

## 5. controller metadata 扩展

阶段 6A 的 `controller.Definition` 需要能完整支撑路由注册。

```go
type Definition struct {
   Module      string
   Area        Area
   Prefix      string
   Name        string
   Description string
   Model       model.Definition
   Service     interface{}
   CRUD        *CRUDDefinition
   Routes      []RouteDefinition
}
```

阶段 6B 对 `RouteDefinition` 明确运行时字段：

```go
type RouteDefinition struct {
   Name              string
   Method            string
   Path              string
   FullPath          string
   Description       string
   IgnoreAuth        bool
   RequirePermission bool
   Permission        string
   Handler           Handler
}
```

推荐定义统一 handler 类型：

```go
type Handler func(r *ghttp.Request)
```

如果阶段 6A 中 `Handler` 使用 `interface{}` 过渡，6B 需要收敛为明确类型。这样可以避免反射注册路由。

## 6. 路由注册设计

### 6.1 注册入口

新增：

```go
func RegisterRoutes(server *ghttp.Server, options RegisterOptions) error

type RegisterOptions struct {
   Controllers       []Definition
   Runtime           *crud.Runtime
   PermissionService PermissionChecker
}
```

`PermissionChecker` 用于 permission middleware：

```go
type PermissionChecker interface {
   HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error)
}
```

### 6.2 IgnoreAuth 路由

```go
func IgnorePaths(controllers []Definition) []string
```

规则：

1. 遍历所有 `Routes`。
2. 收集 `IgnoreAuth == true` 的 `FullPath`。
3. 去重并稳定排序。
4. 不包含 CRUD routes，除非未来 CRUD 明确允许 IgnoreAuth；第一阶段不允许。

当前应生成：

```text
/admin/base/open/login
/admin/base/open/refreshToken
/admin/base/open/captcha
/admin/base/open/eps
/admin/base/comm/program
```

说明：`/admin/base/open/eps` 在 6B 可以先作为 route metadata 存在，handler 可返回占位或继续沿用当前未实现状态；6C 负责真实 EPS 输出。

### 6.3 非 CRUD routes 注册

```go
func RegisterCustomRoutes(server *ghttp.Server, controllers []Definition, filter RouteFilter) error
```

注册规则：

1. 遍历所有 controller 的 `Routes`。
2. 用 `Method + ":" + FullPath` 注册到 GoFrame。
3. `IgnoreAuth` route 可在 auth middleware 前注册。
4. protected route 可在 auth middleware 后注册。
5. handler 必须负责把 service 结果写成 Node 兼容 response。

阶段 6B 不改变 handler 行为；只改变注册来源。

### 6.4 CRUD routes 注册

阶段 6B 不直接从 controller route 逐个注册 CRUD，而是继续复用 `crud.RegisterResourceRoutes`。

新增转换函数：

```go
func ResourceSpecs(controllers []Definition) ([]crud.ResourceSpec, error)
```

规则：

1. 只处理 `CRUD != nil` 的 controller。
2. 使用 controller 的 `Prefix`、`Model`、`CRUD.APIs`、`CRUD.PageQuery`、`CRUD.SortFields`、`CRUD.HiddenFields`、`CRUD.DefaultSort`、`CRUD.DefaultOrder`。
3. 生成 `crud.ResourceSpec`。
4. 如果缺少 Model 或 CRUD APIs，返回 gerror。

这样 CRUD runtime 仍是唯一 CRUD 执行层。

## 7. 权限映射设计

阶段 5B 的 `CRUDPermissionMap()` 已改为从 CRUD specs 派生。阶段 6B 进一步从 controller metadata 派生。

新增：

```go
func PermissionMap(controllers []Definition) map[string]string
```

规则：

### 7.1 CRUD 权限

对 `CRUD != nil` 的 controller：

1. 遍历 `CRUD.APIs`。
2. 通过 `crud.RouteKey(controller.Prefix, api)` 获取 route key。
3. 权限码默认由 prefix 和 api 生成：

```text
/admin/base/sys/user + page => base:sys:user:page
```

### 7.2 自定义路由权限

对 `Routes`：

1. 如果 `Permission` 非空，则加入权限映射。
2. route key 为 `METHOD:FullPath`。
3. 如果 `RequirePermission == true` 但 `Permission == ""`，返回设计/注册错误。
4. IgnoreAuth route 不进入权限映射。

### 7.3 稳定性

`PermissionMap` 返回 map，但测试需要验证核心 key 存在。后续 EPS 或调试输出如需稳定顺序，再提供 `PermissionEntries`。

## 8. app 注册流程设计

当前 app 注册将改成：

```go
func (a *Application) registerRoutes() {
   a.server.BindHandler("/health", ...)

   controllers := modules.Controllers(a.modules)

   controller.RegisterIgnoreAuthRoutes(a.server, controllers)

   a.server.Use(auth.NewMiddleware(auth.MiddlewareOptions{
      Manager:     a.authManager,
      IgnorePaths: controller.IgnorePaths(controllers),
   }))

   controller.RegisterProtectedRoutes(a.server, controllers)

   permissionService := baseService.NewPermissionService(g.DB())
   a.server.Use(controller.NewPermissionMiddleware(controller.PermissionOptions{
      PermissionMap: controller.PermissionMap(controllers),
      Checker:       permissionService,
   }))

   specs, err := controller.ResourceSpecs(controllers)
   ...
   modules.RegisterCRUDRoutes(a.server, runtime, controllers)
}
```

实际实现可以拆成更小函数，但顺序必须保持：

1. health
2. IgnoreAuth routes
3. auth middleware
4. protected custom routes
5. permission middleware
6. CRUD routes

## 9. modules 聚合设计

新增模块聚合函数：

```go
func Controllers(modules []module.Module) []controller.Definition
```

通过可选接口获取：

```go
type ControllerProvider interface {
   ModuleControllers() []controller.Definition
}
```

优点：

1. 不强制所有模块一次性修改接口。
2. base 模块先接入。
3. 后续新模块只需实现 `ModuleControllers()`。

## 10. base 模块迁移影响

阶段 6B 会替换或废弃以下手写注册入口：

| 当前入口 | 6B 后 |
|---|---|
| `base.RegisterAuthRoutes` | controller metadata 注册 open/comm routes |
| `base.AuthIgnorePaths` | `controller.IgnorePaths(controllers)` |
| `base.RegisterPermissionRoutes` | controller metadata 注册 comm/permmenu route |
| `base.RegisterPermissionMiddleware` | controller metadata 权限 middleware |
| `modules.CRUDResourceSpecs` | `controller.ResourceSpecs(controllers)` |
| `modules.RegisterRoutes` | controller/CRUD metadata 注册 |

为了降低风险，可以先保留旧函数作为 wrapper，但 app 不再直接调用它们。

## 11. 错误处理

1. metadata 构造错误使用 `gerror.Wrap` 或 `gerror.Newf`。
2. route 注册时发现重复 `METHOD:FullPath`，必须返回错误并阻止启动。
3. CRUD controller 缺少 Model 时必须返回错误。
4. 自定义 route 缺少 Handler 时必须返回错误。
5. RequirePermission route 缺少 Permission 时必须返回错误。
6. permission 查询内部错误仍对前端返回 `403` 和 `{"code":1001,"message":"权限不足~"}`。

## 12. 测试策略

### 12.1 `cool/controller` 单元测试

覆盖：

1. `IgnorePaths` 输出 open login/refreshToken/captcha/eps 和 comm/program。
2. `PermissionMap` 包含 CRUD route 权限。
3. `PermissionMap` 包含自定义 route 显式权限。
4. `ResourceSpecs` 从 admin controller 生成 CRUD specs。
5. duplicate route 检测。
6. missing model / handler / permission 错误。

### 12.2 base 模块测试

覆盖：

1. base module 返回 controller metadata。
2. open controller routes 完整。
3. comm controller routes 完整。
4. 6 个 admin controller 的 prefix/API 与原 CRUD specs 一致。

### 12.3 app 集成测试

覆盖：

1. `app.New(StartServer: true)` 注册 metadata 驱动路由不 panic。
2. auth login / person 仍通过。
3. permmenu 仍通过。
4. admin CRUD 仍通过。
5. limited user CRUD 仍返回 403 精确 body。

必跑命令：

```bash
go test ./cool/controller ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

## 13. 迁移步骤建议

阶段 6B 实现时建议按以下顺序：

1. 为 controller metadata 增加运行时注册所需字段和验证函数。
2. 实现 `IgnorePaths`、`PermissionMap`、`ResourceSpecs`。
3. 为 base controller metadata 补齐 open/comm/admin 的 route handler。
4. 改 app 注册流程使用 controller metadata。
5. 保留旧注册函数但从 app 中移除调用。
6. 跑 auth/permission/CRUD 集成测试。
7. 删除或标记不再使用的旧 wrapper，避免死代码。

## 14. 风险和约束

1. GoFrame middleware 和 route 注册顺序敏感，必须用集成测试验证。
2. 同一路径重复注册可能造成行为不确定，必须检测重复 route。
3. IgnoreAuth route 如果没有被正确收集，会导致 login/refreshToken/captcha/eps 被 401。
4. permission middleware 如果放在 CRUD route 后，CRUD 不会被保护。
5. metadata 与 handler 分离后，handler nil 会造成启动后 panic，必须启动时检测。
6. 阶段 6B 不应改变 HTTP 响应 body。
7. 阶段 6B 不应引入 EPS 真实输出，避免同时改两层。

## 15. 验收标准

阶段 6B 完成后必须满足：

1. app 不再直接调用 base 手写 auth/permission/CRUD 注册入口。
2. IgnoreAuth paths 来自 controller metadata。
3. CRUD ResourceSpecs 来自 controller metadata。
4. 权限映射来自 controller metadata。
5. `GET /admin/base/comm/permmenu` 行为不变。
6. login / refreshToken / captcha / person / logout / program 行为不变。
7. base CRUD 行为不变。
8. 缺 CRUD 权限返回 HTTP 403 和精确 body。
9. `go test ./...` 通过。
10. `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1` 通过。
11. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 16. 自检

本设计只覆盖 metadata 驱动路由和权限，不实现 EPS JSON，不补自定义 API，不改变 HTTP 协议。它承接阶段 6A 的 controller/service metadata，为阶段 6C EPS runtime 提供单一来源。
