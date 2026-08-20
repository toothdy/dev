# cool-admin-go-next CoolController Runtime 设计

日期：2026-07-16

## 1. 背景

`cool-admin-go-next` 的目标不是普通 GoFrame 项目，而是 Go 版 cool-admin 后台管理快速开发框架。

Node 版 cool-admin-midway 的核心定位是：框架封装后台管理系统的通用能力，业务模块只声明实体、Controller、Service 和少量自定义逻辑。其核心设计包括：

1. `@CoolController` 声明 CRUD、自定义路由、实体、Service 和查询配置。
2. `BaseController` 负责标准 CRUD 接口编排。
3. `BaseService` 负责默认 CRUD、SQL 工具、分页工具和 `modifyBefore` / `modifyAfter` hooks。
4. `CoolUrlTag` / `CoolTag` 负责生成忽略 token 的 URL。
5. EPS 从 controller、entity、router 和 tag metadata 统一派生。

Go 版没有装饰器和传统继承，不能照搬 Node 实现。但可以用 Fluent Builder、显式 metadata、结构体嵌入和接口调度实现同等框架体验。

当前 `cool-admin-go-next` 已有 `cool/crud`、`cool/auth`、`cool/module`、`cool/model` 等基础能力，但 `modules/base` 仍存在手写 route、手写 CRUD specs、手写 auth ignore paths、手写 permission map 等重复逻辑。继续在此基础上实现 EPS 或更多业务 API，会让路由、权限、CRUD、EPS 各维护一份元数据。

因此本阶段要实现 Go 版 CoolController Runtime 的最小闭环，让 controller metadata 成为路由、权限、CRUD 和后续 EPS 的单一来源。

## 2. 目标

本设计目标：

1. 在 `cool/` 中沉淀后台管理框架核心能力。
2. 在 `modules/` 中只保留业务模块代码。
3. 新增 Go 版 `cool/controller`，作为 Node `@CoolController` 的对应抽象。
4. 新增 `cool/service`，提供组合式 `BaseService` 和 hook 约定。
5. 扩展 `cool/module`，让模块原生暴露 controller metadata。
6. 扩展 `cool/crud`，支持默认 CRUD 与业务 Service 重写 CRUD。
7. 将 `modules/base` 迁移为第一个内置业务模块和样板模块。
8. 用 controller metadata 派生：
   - auth ignore paths
   - permission map
   - CRUD resource specs
   - routes
9. 替换 app 层对 base 模块的手写注册流程。
10. 保持现有 HTTP 路径、method、响应结构和测试不回退。

## 3. 非目标

本阶段不做：

1. 完整 EPS runtime 输出。
2. Node 版全部自定义 API 补齐。
3. `serviceApis` 完整机制。
4. join / select / where function 的完整高级查询配置。
5. 插件系统。
6. CLI 代码生成。
7. GoFrame `dao/`、`internal/model/do/`、`internal/model/entity/` 生成。
8. `logic/` 目录。
9. PostgreSQL / SQLite 深度适配。
10. 运行时模块卸载。

## 4. 总体架构

项目分为框架核心层和业务模块层。

```text
cool-admin-go-next/
├── cool/
│   ├── controller/   # Controller metadata、builder、route/permission/CRUD 派生与注册
│   ├── service/      # BaseService、hooks、业务 service 组合基础
│   ├── crud/         # 默认 CRUD runtime、查询、字段过滤、service override 调度
│   ├── module/       # 模块注册、models/controllers/seeds 聚合
│   ├── auth/         # JWT、auth middleware、auth context
│   ├── model/        # 模型元数据
│   └── response/     # 统一响应
└── modules/
    └── base/
        ├── controller/
        ├── service/
        ├── model/
        ├── config.go
        ├── db.json
        └── menu.json
```

框架职责：

1. `cool/controller` 负责声明和消费 controller metadata。
2. `cool/crud` 负责执行默认 CRUD，并在业务 Service 重写时优先调用业务实现。
3. `cool/service` 提供业务 Service 的组合基础和 hooks。
4. `cool/module` 聚合模块暴露的 models、controllers 和 seeds。
5. `cool/auth` 继续负责认证基础设施，不承载 base 登录业务。

业务模块职责：

1. 在 `model/` 声明模型。
2. 在 `service/` 编写业务逻辑。
3. 在 `controller/` 声明 CRUD、自定义路由、权限和 service 绑定。
4. 在模块入口暴露 models、controllers 和 seeds。

## 5. `cool/controller` 设计

### 5.1 定位

`cool/controller` 是 Go 版 `@CoolController`，负责 controller metadata 的声明、派生和注册。

它不负责 CRUD 的具体数据库执行。具体执行仍由 `cool/crud` 完成。

### 5.2 Area

```go
type Area string

const (
   AreaAdmin Area = "admin"
   AreaOpen  Area = "open"
   AreaComm  Area = "comm"
)
```

路径规则：

| Builder | 输入 | 输出 prefix |
|---|---|---|
| `Admin("base/sys/user")` | `base/sys/user` | `/admin/base/sys/user` |
| `Open("base/open")` | `base/open` | `/admin/base/open` |
| `Comm("base/comm")` | `base/comm` | `/admin/base/comm` |

说明：Node 版 base 的 open/comm 实际仍在 `/admin/base/open` 与 `/admin/base/comm` 下，本阶段保持现有 HTTP 行为。

### 5.3 Definition

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

字段说明：

| 字段 | 说明 |
|---|---|
| `Module` | 模块名，例如 `base` |
| `Area` | 分区：admin / open / comm |
| `Prefix` | 完整 HTTP prefix，例如 `/admin/base/sys/user` |
| `Name` | controller/entity 名称，用于后续 EPS |
| `Description` | 中文描述 |
| `Model` | 模型元数据 |
| `Service` | 绑定的业务 service |
| `CRUD` | CRUD 声明 |
| `Routes` | 自定义 route 声明 |

### 5.4 Builder API

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

示例：

```go
func UserController(userService *service.UserService, userModel model.Definition) controller.Definition {
   return controller.Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("系统用户").
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

### 5.5 CRUD 声明

```go
type CRUDOptions struct {
   APIs             []string
   PageQuery        QueryOptions
   ListQuery        QueryOptions
   InsertParam      InsertParamFunc
   InfoIgnoreFields []string
   SortFields       []string
   HiddenFields     []string
   ReadonlyFields   []string
   DefaultSort      string
   DefaultOrder     string
}

type QueryOptions struct {
   KeyWordLikeFields []string
   FieldEq           []string
   FieldLike         []string
}

type InsertParamFunc func(ctx context.Context) map[string]interface{}
```

`CRUDOptions` 先覆盖当前 `crud.ResourceSpec` 所需能力，并预留 Node 版 `insertParam`、`listQueryOp`、`infoIgnoreProperty` 的 Go 化入口。

本阶段不实现 Node 版完整 `join`、`select`、`where` function，但类型设计不得阻塞后续扩展。

### 5.6 Route 声明

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

示例：

```go
Route(controller.RouteOptions{
   Name:        "login",
   Method:      http.MethodPost,
   Path:        "/login",
   Description: "登录",
   IgnoreAuth:  true,
   Handler:     authService.Login,
})
```

`FullPath` 由 builder 自动生成：`Prefix + Path`。

### 5.7 派生能力

`cool/controller` 需要提供以下派生函数：

```go
func IgnoreAuthPaths(controllers []Definition) []string
func PermissionMap(controllers []Definition) (map[string]string, error)
func CRUDResourceSpecs(controllers []Definition) ([]crud.ResourceSpec, error)
func RegisterRoutes(server *ghttp.Server, runtime *crud.Runtime, controllers []Definition) error
```

派生规则：

1. `IgnoreAuthPaths` 收集 `RouteDefinition.IgnoreAuth == true` 的 `FullPath`。
2. `PermissionMap` 从 CRUD 和显式声明 `Permission` 的自定义 route 派生权限。
3. `CRUDResourceSpecs` 从有 CRUD 声明的 controller 生成 `crud.ResourceSpec`。
4. `RegisterRoutes` 注册自定义 route 和 CRUD route。

## 6. `cool/crud` 设计

### 6.1 定位

`cool/crud` 继续作为 CRUD 执行层。

它负责：

1. 默认 CRUD runtime。
2. add / delete / update / info / list / page。
3. query 构建。
4. 字段过滤。
5. 排序校验。
6. 统一响应处理。
7. 业务 service CRUD override 调度。
8. 默认 CRUD hooks 调用。

### 6.2 Service override 接口

业务 service 可以按需实现以下接口，覆盖默认 CRUD：

```go
type AddHandler interface {
   Add(ctx context.Context, request AddRequest) (interface{}, error)
}

type DeleteHandler interface {
   Delete(ctx context.Context, request DeleteRequest) (interface{}, error)
}

type UpdateHandler interface {
   Update(ctx context.Context, request UpdateRequest) (interface{}, error)
}

type InfoHandler interface {
   Info(ctx context.Context, request InfoRequest) (interface{}, error)
}

type ListHandler interface {
   List(ctx context.Context, request QueryRequest) (interface{}, error)
}

type PageHandler interface {
   Page(ctx context.Context, request QueryRequest) (interface{}, error)
}
```

请求类型应与现有 `crud.QueryRequest` 风格保持一致。缺失的 `AddRequest`、`DeleteRequest`、`UpdateRequest`、`InfoRequest` 在实施时补齐。

运行逻辑：

```text
收到 CRUD 请求
  ↓
找到 resource 绑定的 service
  ↓
判断 service 是否实现当前 action 对应接口
  ├── 是：调用 service override
  └── 否：执行默认 CRUD
```

### 6.3 Hooks

默认 CRUD 支持 hooks：

```go
type ModifyBeforeHook interface {
   ModifyBefore(ctx context.Context, action string, data interface{}) error
}

type ModifyAfterHook interface {
   ModifyAfter(ctx context.Context, action string, data interface{}) error
}
```

默认 CRUD 执行顺序：

```text
ModifyBefore
  ↓
默认 CRUD
  ↓
ModifyAfter
```

如果业务 service 完整 override 某个 CRUD action，则由业务 service 自己决定是否调用通用 hook。框架不在 override 外层强制调用 hook，以避免业务实现被重复包裹或语义不清。

### 6.4 InsertParam

Node 版 `insertParam` 用于新增时自动合并上下文参数，例如当前用户 ID。

Go 版在 `CRUDOptions` 中保留：

```go
type InsertParamFunc func(ctx context.Context) map[string]interface{}
```

默认 `Add` 执行前合并 `InsertParam` 返回值。

如果业务 service override `Add`，则传入的 `AddRequest` 应已经包含合并后的参数。

## 7. `cool/service` 设计

### 7.1 定位

Go 没有传统继承，本项目采用结构体嵌入模拟 Node 版 `extends BaseService` 的开发体验。

框架层：

```go
type BaseService struct {
   DB    gdb.DB
   Model model.Definition
}

func NewBaseService(db gdb.DB, definition model.Definition) *BaseService
```

业务层：

```go
type UserService struct {
   *service.BaseService
}

func NewUserService(db gdb.DB, definition model.Definition) *UserService {
   return &UserService{
      BaseService: service.NewBaseService(db, definition),
   }
}
```

### 7.2 设计原则

1. Service 层可以使用结构体嵌入。
2. Controller 层不模拟继承，使用 Fluent Builder。
3. CRUD 重写不依赖继承多态，使用接口调度。
4. `BaseService` 初期保持轻量，只承载 DB、Model 和后续可复用 helper。
5. 后续可逐步加入类似 Node 版 `setSql`、`nativeQuery`、`sqlRenderPage` 的工具方法。

## 8. `cool/module` 设计

模块系统需要原生支持 controllers。

当前写法：

```go
module.New("base").
   Name("基础模块").
   Models(...).
   Seeds(...)
```

目标写法：

```go
module.New("base").
   Name("基础模块").
   Models(...).
   Controllers(...).
   Seeds(...)
```

新增能力：

```go
func (d *Definition) Controllers(controllers []controller.Definition) *Definition
func (d *Definition) ModuleControllers() []controller.Definition
func CollectControllers(modules []Module) []controller.Definition
```

`Module` 接口可以直接加入 `ModuleControllers()`，因为 controller 是框架核心能力，不是临时兼容能力。

## 9. `modules/base` 迁移设计

### 9.1 目标结构

```text
modules/base/
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
├── service/
│   ├── auth.go
│   ├── permission.go
│   ├── sys_department.go
│   ├── sys_log.go
│   ├── sys_menu.go
│   ├── sys_param.go
│   ├── sys_role.go
│   └── sys_user.go
└── model/
    └── models.go
```

### 9.2 Controller

base controller 只声明 metadata：

1. admin sys controllers：
   - user
   - role
   - menu
   - department
   - param
   - log
2. open controller：
   - login
   - refreshToken
   - captcha
   - eps 预留
   - html 后续补齐
3. comm controller：
   - person
   - permmenu
   - logout
   - program
   - personUpdate / upload / uploadMode 后续补齐

本阶段必须保持当前已实现 HTTP 行为：

| Method | Path |
|---|---|
| POST | `/admin/base/open/login` |
| POST | `/admin/base/open/refreshToken` |
| GET | `/admin/base/open/captcha` |
| GET | `/admin/base/comm/person` |
| GET | `/admin/base/comm/permmenu` |
| POST | `/admin/base/comm/logout` |
| GET | `/admin/base/comm/program` |
| CRUD | `/admin/base/sys/*` |

### 9.3 Service

迁移现有：

| 当前文件 | 目标文件 |
|---|---|
| `modules/base/auth.go` | `modules/base/service/auth.go` |
| `modules/base/permission.go` | `modules/base/service/permission.go` |

新增资源 service：

| 资源 | Service |
|---|---|
| user | `UserService` |
| role | `RoleService` |
| menu | `MenuService` |
| department | `DepartmentService` |
| param | `ParamService` |
| log | `LogService` |

这些 service 初期可以只嵌入 `cool/service.BaseService`，后续逐步补充 Node 版自定义行为。

### 9.4 兼容策略

为了降低迁移风险，base 根包可以短期保留 wrapper：

```go
type AuthService = service.AuthService
type PermissionService = service.PermissionService

func NewAuthService(...) *service.AuthService
func NewPermissionService(...) *service.PermissionService
```

但 app 层应切换到框架化注册，不再依赖 base 手写 route 注册函数。

## 10. app 注册流程

当前流程：

```text
app.registerRoutes()
├── base.RegisterAuthRoutes()
├── auth.NewMiddleware(base.AuthIgnorePaths())
├── base.RegisterPermissionRoutes()
├── base.RegisterPermissionMiddleware()
├── modules.CRUDResourceSpecs()
└── modules.RegisterRoutes()
```

目标流程：

```text
app.registerRoutes()
├── 注册 /health
├── 收集 modules
├── 收集 controllers
├── controller.IgnoreAuthPaths()
├── 注册 auth middleware
├── controller.PermissionMap()
├── 注册 permission middleware
├── controller.CRUDResourceSpecs()
├── 创建 crud.Registry
├── 创建 crud.Runtime
└── controller.RegisterRoutes()
```

app 层不再知道 base 模块有哪些具体接口。

## 11. 权限设计

### 11.1 CRUD 权限

CRUD 权限从 controller metadata 派生。

规则：

```text
METHOD:FULL_PATH -> permission code
```

示例：

```text
POST:/admin/base/sys/user/add    -> base:sys:user:add
POST:/admin/base/sys/user/delete -> base:sys:user:delete
POST:/admin/base/sys/user/update -> base:sys:user:update
GET:/admin/base/sys/user/info    -> base:sys:user:info
POST:/admin/base/sys/user/list   -> base:sys:user:list
POST:/admin/base/sys/user/page   -> base:sys:user:page
```

权限码生成规则：

1. 去掉 `/admin/` 前缀。
2. 将 `/` 替换为 `:`。
3. 追加 CRUD api 名称。

### 11.2 自定义 route 权限

自定义 route 如果声明 `Permission`，进入权限映射。

自定义 route 如果 `IgnoreAuth == true`，不进入权限校验。

comm route 默认要求登录，但不进入 CRUD 权限校验，除非显式声明 `Permission`。

## 12. Node 版对应关系

| Node 版 | Go 版 |
|---|---|
| `@CoolController` | `controller.Admin(...).CRUD(...).Route(...).Build()` |
| `BaseController` | `cool/controller + cool/crud runtime` |
| `BaseService` | `cool/service.BaseService` |
| `extends BaseService` | `struct { *service.BaseService }` |
| 重写 `page()` | 实现 `PageHandler` |
| `modifyBefore/modifyAfter` | hook interface |
| `CoolTag(TagTypes.IGNORE_TOKEN)` | `RouteOptions.IgnoreAuth` |
| 自动追加 CRUD route | `RegisterRoutes()` 注册 CRUD route |
| EPS 扫描 controller/entity/router | 后续从 `controller.Definition + model.Definition` 生成 |

## 13. 测试策略

必须覆盖：

1. `cool/controller` builder prefix 生成。
2. CRUD metadata 生成。
3. Route full path 生成。
4. IgnoreAuth paths 生成。
5. Permission map 生成。
6. CRUD specs 从 controller metadata 派生。
7. route 注册不改变现有路径和 method。
8. 默认 CRUD 仍可用。
9. service 覆盖 CRUD 时优先调用业务 service。
10. hooks 在默认 CRUD 前后执行。
11. base auth、permission、CRUD、integration 测试不回退。

必跑命令：

```bash
go test ./cool/controller ./cool/service ./cool/crud ./cool/module ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

## 14. 风险和约束

1. `cool/controller` 不能依赖具体业务模块。
2. `cool/crud` 不应反向依赖 `cool/controller`，避免循环依赖。需要通过 `crud.ResourceSpec` 承载 service 绑定。
3. `modules/base/service` 不应依赖 `modules/base/controller`。
4. `modules/base/controller` 可以依赖 `modules/base/service`、`cool/controller`、`cool/crud`、`cool/model`。
5. app 层不再依赖 base 手写注册函数。
6. 迁移期间 root wrapper 只能作为兼容手段，不作为长期设计目标。
7. 本阶段不能改变外部 HTTP 路径、method、响应结构。
8. 不创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 15. 验收标准

完成后必须满足：

1. `cool/controller` 存在 Fluent Builder API。
2. `cool/controller` 能派生 auth ignore paths、permission map、CRUD specs、routes。
3. `cool/service` 存在 `BaseService` 和 hook interface。
4. `cool/module` 能聚合 controller metadata。
5. `cool/crud` 支持默认 CRUD、service override 和 hooks。
6. `modules/base/controller/admin` 声明 6 个 sys controller。
7. `modules/base/controller/open` 声明 open controller。
8. `modules/base/controller/comm` 声明 comm controller。
9. `modules/base/service` 承载 auth、permission 和资源 service。
10. app 层通过 controller metadata 完成 base 路由、权限、CRUD 注册。
11. 现有 HTTP 行为不变。
12. `go test ./...` 通过。
13. `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1` 通过。
14. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 16. 自检

本设计聚焦 Go 版 CoolController Runtime 的最小闭环，明确将后台管理通用能力沉淀到 `cool/`，让 `modules/` 只承载业务模块代码。设计对齐 Node 版 cool-admin 的开发体验，但使用 Go 的显式 metadata、结构体嵌入和接口调度实现，不强行模拟装饰器或传统继承。
