# cool-admin-go-next 阶段 6A Controller/Service 源码形态对齐设计

日期：2026-07-16

## 1. 背景

阶段 5B 已完成 auth、permmenu、base CRUD 权限校验，但当前 `modules/base` 仍是阶段性平铺实现：

```text
modules/base/auth.go
modules/base/auth_routes.go
modules/base/permission.go
modules/base/permission_routes.go
modules/base/routes.go
```

这和初始设计文档中的目标结构不一致：

```text
modules/base/controller/
modules/base/service/
modules/base/model/
```

如果直接在现有平铺结构上实现 EPS，会导致 EPS、路由、权限、CRUD 元数据各维护一份，后续切换到设计结构时还要迁移。阶段 6A 先对齐源码形态，为后续阶段 6B/6C 铺路。

## 2. 目标

阶段 6A 的目标是把当前 base 模块迁移到接近 Node `@CoolController` / service 的源码组织方式，并引入 Go 版 Fluent Builder controller metadata。

阶段 6A 完成后应满足：

1. `modules/base` 按 controller / service / model 分层。
2. 现有 auth、permission、CRUD 业务逻辑从平铺文件迁移到 `modules/base/service`。
3. base open、comm、admin sys controller 以声明式 Fluent Builder 形式定义。
4. controller metadata 能表达：模块、分区、prefix、CRUD API、自定义 route、是否忽略 auth、权限码、模型和 service。
5. 现有 HTTP 行为和测试不回退。
6. 暂不实现 EPS runtime 输出，但 metadata 形状必须能支撑阶段 6C 生成 EPS。

## 3. 非目标

阶段 6A 不做：

1. `/admin/base/open/eps` 的真实 EPS 输出。
2. Vue 前端真实联调。
3. user move、menu parse/create/export/import、department order、param html、log clear/setKeep/getKeep 等自定义 API 的业务补齐。
4. 插件系统。
5. CLI 代码生成。
6. GoFrame dao/do/entity 生成。
7. PostgreSQL / SQLite。
8. 运行时模块卸载。

## 4. 总体方案

采用 Fluent Builder 模拟 Node 装饰器源码形态。

Node 写法：

```ts
@CoolController({
   api: ["add", "delete", "update", "info", "list", "page"],
   entity: BaseSysUserEntity,
   service: BaseSysUserService,
})
export class BaseSysUserController {}
```

Go 写法：

```go
func UserController(userService *service.UserService, userModel model.Definition) controller.Definition {
   return controller.Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("用户管理").
      Model(userModel).
      Service(userService).
      CRUD(controller.CRUDOptions{
         APIs: []string{
            crud.APIAdd,
            crud.APIDelete,
            crud.APIUpdate,
            crud.APIInfo,
            crud.APIList,
            crud.APIPage,
         },
      }).
      Build()
}
```

核心原则：

1. 代码形态尽量声明式，接近 Node controller。
2. Go 内部保持类型安全，不用反射实现复杂装饰器。
3. controller metadata 成为后续路由、权限、EPS 的单一来源。
4. 阶段 6A 只迁移结构和 metadata，不改变外部 HTTP 协议。

## 5. 目标目录结构

阶段 6A 后的目标结构：

```text
cool-admin-go-next/
├── cool/
│   ├── controller/
│   │   ├── builder.go
│   │   ├── crud.go
│   │   ├── definition.go
│   │   └── route.go
│   └── service/
│       └── base.go
└── modules/
    ├── modules.go
    ├── routes.go
    └── base/
        ├── base.go
        ├── config.go
        ├── db.json
        ├── menu.json
        ├── controller/
        │   ├── admin/
        │   │   ├── sys_department.go
        │   │   ├── sys_log.go
        │   │   ├── sys_menu.go
        │   │   ├── sys_param.go
        │   │   ├── sys_role.go
        │   │   └── sys_user.go
        │   ├── comm/
        │   │   └── comm.go
        │   └── open/
        │       └── open.go
        ├── model/
        │   └── models.go
        └── service/
            ├── auth.go
            ├── permission.go
            ├── sys_department.go
            ├── sys_log.go
            ├── sys_menu.go
            ├── sys_param.go
            ├── sys_role.go
            └── sys_user.go
```

阶段 6A 可以保留兼容 wrapper 文件，但最终应避免长期同时维护两套定义。

## 6. `cool/controller` 设计

### 6.1 核心类型

```go
type Area string

const (
   AreaAdmin Area = "admin"
   AreaOpen  Area = "open"
   AreaComm  Area = "comm"
)

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

字段说明：

| 字段 | 说明 |
|---|---|
| `Module` | 模块名，例如 `base` |
| `Area` | 分区：admin/open/comm |
| `Prefix` | 完整 HTTP prefix，例如 `/admin/base/sys/user` |
| `Name` | EPS controller/entity 名称，例如 `BaseSysUserEntity` |
| `Description` | 中文描述 |
| `Model` | 模型元数据 |
| `Service` | 绑定 service 实例 |
| `CRUD` | CRUD 声明 |
| `Routes` | 自定义路由声明 |

### 6.2 Builder API

```go
func Admin(path string) *Builder
func Open(path string) *Builder
func Comm(path string) *Builder

func (b *Builder) Name(name string) *Builder
func (b *Builder) Description(description string) *Builder
func (b *Builder) Model(definition model.Definition) *Builder
func (b *Builder) Service(service interface{}) *Builder
func (b *Builder) CRUD(options CRUDOptions) *Builder
func (b *Builder) Route(options RouteOptions) *Builder
func (b *Builder) Build() Definition
```

Prefix 规则：

| Builder | 输入 | 输出 prefix |
|---|---|---|
| `Admin("base/sys/user")` | `base/sys/user` | `/admin/base/sys/user` |
| `Open("base/open")` | `base/open` | `/admin/base/open` |
| `Comm("base/comm")` | `base/comm` | `/admin/base/comm` |

### 6.3 CRUD 声明

```go
type CRUDOptions struct {
   APIs          []string
   PageQuery     QueryOptions
   SortFields    []string
   HiddenFields  []string
   DefaultSort   string
   DefaultOrder  string
}

type QueryOptions struct {
   KeyWordLikeFields []string
   FieldEq           []string
   FieldLike         []string
}

type CRUDDefinition struct {
   APIs         []string
   PageQuery    QueryOptions
   SortFields   []string
   HiddenFields []string
   DefaultSort  string
   DefaultOrder string
}
```

阶段 6A 中 `CRUDOptions` 先覆盖现有 `crud.ResourceSpec` 能力。阶段 6B 再由 controller metadata 派生 `crud.ResourceSpec`。

### 6.4 自定义 Route 声明

```go
type RouteOptions struct {
   Name        string
   Method      string
   Path        string
   Description string
   IgnoreAuth  bool
   Permission  string
   Handler     interface{}
}

type RouteDefinition struct {
   Name        string
   Method      string
   Path        string
   FullPath    string
   Description string
   IgnoreAuth  bool
   Permission  string
   Handler     interface{}
}
```

阶段 6A route handler 允许先保存为 `interface{}`，实际统一注册可以在阶段 6B 完成。

## 7. `cool/service` 设计

阶段 6A 新增最小 service 基类，为 Node service hooks 预留形态。

```go
type BaseService struct {
   DB    gdb.DB
   Model model.Definition
}

func NewBaseService(db gdb.DB, definition model.Definition) *BaseService
```

预留 hook interface：

```go
type ModifyBeforeHook interface {
   ModifyBefore(ctx context.Context, action string, data map[string]interface{}) error
}

type ModifyAfterHook interface {
   ModifyAfter(ctx context.Context, action string, data map[string]interface{}) error
}
```

阶段 6A 不强制 CRUD runtime 调用 hooks；阶段 6D 再接入。

## 8. base 模块迁移设计

### 8.1 service 迁移

当前文件迁移：

| 当前文件 | 目标文件 |
|---|---|
| `modules/base/auth.go` | `modules/base/service/auth.go` |
| `modules/base/permission.go` | `modules/base/service/permission.go` |

服务包名：

```go
package service
```

迁移后类型名保留：

```go
type AuthService struct {}
type PermissionService struct {}
```

为了降低一次性改动风险，可以在 `modules/base` 根包提供短期 wrapper：

```go
type AuthService = service.AuthService
type PermissionService = service.PermissionService
```

但 wrapper 只用于迁移兼容，不作为长期设计目标。

### 8.2 admin controller

每个 sys 资源一个 controller 文件。

示例：

```go
func UserController(userService *service.UserService, userModel model.Definition) controller.Definition {
   return controller.Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("用户管理").
      Model(userModel).
      Service(userService).
      CRUD(controller.CRUDOptions{
         APIs: []string{
            crud.APIAdd,
            crud.APIDelete,
            crud.APIUpdate,
            crud.APIInfo,
            crud.APIList,
            crud.APIPage,
         },
         PageQuery: controller.QueryOptions{
            KeyWordLikeFields: []string{"name", "username", "nickName"},
            FieldEq:           []string{"status", "departmentId"},
         },
         SortFields:   []string{"id", "createTime", "updateTime", "username"},
         HiddenFields: []string{"password"},
         DefaultSort:  "id",
         DefaultOrder: "DESC",
      }).
      Build()
}
```

阶段 6A 需要为以下资源建立 controller：

1. user
2. role
3. menu
4. department
5. param
6. log

### 8.3 open controller

```go
func OpenController(authService *service.AuthService) controller.Definition {
   return controller.Open("base/open").
      Description("开放接口").
      Route(controller.RouteOptions{
         Name:       "login",
         Method:     http.MethodPost,
         Path:       "/login",
         IgnoreAuth: true,
         Handler:    authService.Login,
      }).
      Route(controller.RouteOptions{
         Name:       "refreshToken",
         Method:     http.MethodPost,
         Path:       "/refreshToken",
         IgnoreAuth: true,
         Handler:    authService.RefreshToken,
      }).
      Route(controller.RouteOptions{
         Name:       "captcha",
         Method:     http.MethodGet,
         Path:       "/captcha",
         IgnoreAuth: true,
         Handler:    authService.Captcha,
      }).
      Build()
}
```

阶段 6A 只声明已存在行为；`eps` route 可作为 metadata 预留，但真实输出放阶段 6C。

### 8.4 comm controller

```go
func CommController(authService *service.AuthService, permissionService *service.PermissionService) controller.Definition {
   return controller.Comm("base/comm").
      Description("通用接口").
      Route(controller.RouteOptions{
         Name:    "person",
         Method:  http.MethodGet,
         Path:    "/person",
         Handler: authService.Person,
      }).
      Route(controller.RouteOptions{
         Name:    "permmenu",
         Method:  http.MethodGet,
         Path:    "/permmenu",
         Handler: permissionService.PermMenu,
      }).
      Route(controller.RouteOptions{
         Name:    "logout",
         Method:  http.MethodPost,
         Path:    "/logout",
         Handler: authService.Logout,
      }).
      Route(controller.RouteOptions{
         Name:       "program",
         Method:     http.MethodGet,
         Path:       "/program",
         IgnoreAuth: true,
         Handler:    authService.Program,
      }).
      Build()
}
```

## 9. 模块聚合设计

`modules/base/base.go` 需要对外暴露 controller metadata。

```go
type Module struct {
   controllers []controller.Definition
   models      []model.Definition
}

func New() *Module
func (m *Module) ModuleControllers() []controller.Definition
```

`cool/module.Module` 接口需要新增 controller 能力，或在兼容层新增可选接口：

```go
type ControllerProvider interface {
   ModuleControllers() []controller.Definition
}
```

推荐先用可选接口，避免一次性改动所有模块接口。

## 10. 和现有路由的关系

阶段 6A 以“不改变运行时行为”为主：

1. 现有 `RegisterAuthRoutes`、`RegisterPermissionRoutes`、`RegisterRoutes` 可以先保留。
2. 新 controller metadata 与旧注册并存，但测试必须防止行为漂移。
3. 阶段 6B 再把 app 注册改成由 controller metadata 统一驱动。

这样拆分的原因：

- 6A 专注源码形态和元数据。
- 6B 专注路由注册替换。
- 任何回归可以快速定位。

## 11. 后续阶段衔接

### 11.1 阶段 6B：metadata 驱动路由和权限

目标：

1. `controller.RegisterRoutes()` 统一注册 open/comm/admin routes。
2. `auth.IgnorePathsFromControllers()` 生成忽略列表。
3. `permission.MapFromControllers()` 生成 CRUD 和自定义权限映射。
4. `crud.ResourceSpecsFromControllers()` 生成 CRUD specs。

### 11.2 阶段 6C：EPS runtime

目标：

1. `/admin/base/open/eps` 从 controller metadata 生成。
2. 输出满足 `docs/protocol/base-api-contract.md` 的 EPS 规则。
3. 对齐 `docs/protocol/fixtures/eps-admin-success.json`。

### 11.3 阶段 6D：Node 自定义 API 与 hooks

目标：

1. user move。
2. menu parse/create/export/import。
3. department order。
4. param html。
5. log clear/setKeep/getKeep。
6. CRUD before/after hooks。

## 12. 测试策略

阶段 6A 必须覆盖：

1. `cool/controller` builder prefix 生成。
2. CRUD metadata 生成。
3. Route metadata full path 生成。
4. base admin controllers 的 prefix/API 与现有 CRUD specs 一致。
5. open/comm controllers 的 route method/path/IgnoreAuth 正确。
6. 现有 auth、permission、CRUD、integration 测试不回退。

必跑命令：

```bash
go test ./cool/controller ./cool/service ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

## 13. 风险和约束

1. 一次性移动文件容易造成 import cycle，实施时必须先设计包依赖方向。
2. `modules/base/service` 不应反向依赖 `modules/base/controller`。
3. `cool/controller` 不能依赖具体 `modules/base`。
4. `cool/service` 不能依赖具体 `modules/base`。
5. 阶段 6A 不应改变外部 HTTP 路径、method、响应结构。
6. wrapper 只能作为迁移过渡，后续阶段应逐步移除。
7. 不创建手写 `dao/`、`internal/model/do/`、`internal/model/entity/`。
8. 不使用 `logic/` 目录。

## 14. 验收标准

阶段 6A 完成后必须满足：

1. `cool/controller` 存在 Fluent Builder API。
2. `cool/service` 存在 BaseService 和 hook interface 预留。
3. `modules/base/controller/admin` 声明 6 个 sys controller。
4. `modules/base/controller/open` 声明 open controller。
5. `modules/base/controller/comm` 声明 comm controller。
6. `modules/base/service` 承载 auth 和 permission service。
7. base module 能返回 controller metadata。
8. 现有 HTTP 行为不变。
9. `go test ./...` 通过。
10. `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1` 通过。
11. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 15. 自检

本设计聚焦阶段 6A，不直接实现 EPS runtime，不补自定义 API，不引入插件系统。它先解决源码形态和 metadata 单一来源问题，为后续 6B/6C 降低返工风险。
