# CoolController Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go version of cool-admin's controller runtime so `cool/` owns CRUD, route, auth-ignore, permission, and service override behavior while `modules/` only declares business modules.

**Architecture:** Add `cool/controller` as the explicit Go replacement for Node `@CoolController`, extend `cool/crud` as the default CRUD executor plus service override dispatcher, and let `cool/module` collect controller metadata. Migrate `modules/base` into controller/service/model shape and update app startup to consume metadata instead of base-specific registration functions.

**Tech Stack:** Go 1.x, GoFrame v2, existing `cool/crud`, `cool/auth`, `cool/model`, `cool/module`, `cool/response`, standard library `net/http`, `context`, `reflect`.

## Global Constraints

- Always answer and write project-facing explanations in Chinese.
- Do not create `dao/`, `internal/model/do/`, `internal/model/entity/`, or `logic/`.
- Keep core framework abstractions under `/Users/n/数据/cool-admin/cool-admin-go-next/cool`.
- Keep business module code under `/Users/n/数据/cool-admin/cool-admin-go-next/modules`.
- Preserve existing HTTP paths, methods, and response shapes.
- Do not use `git add -A`; stage files explicitly.
- Use 3-space indentation for new code where gofmt does not control formatting; Go files must be run through `gofmt`.
- Comments and JSDoc-style Go comments must be concise Chinese comments matching existing project style.
- Every task must run its focused tests before commit.

---

## File Structure

### New files

- `cool/service/base.go` — framework service base, embedded by business services.
- `cool/service/base_test.go` — verifies BaseService stores DB and model metadata.
- `cool/controller/definition.go` — public controller metadata types.
- `cool/controller/builder.go` — Fluent Builder API: `Admin`, `Open`, `Comm`.
- `cool/controller/derive.go` — metadata derivation: ignore paths, permission map, CRUD specs.
- `cool/controller/register.go` — custom route and CRUD route registration.
- `cool/controller/permission.go` — framework permission middleware based on derived route map.
- `cool/controller/builder_test.go` — builder, prefix, route full-path tests.
- `cool/controller/derive_test.go` — ignore path, permission map, CRUD spec tests.
- `cool/controller/register_test.go` — route registration behavior tests.
- `cool/controller/permission_test.go` — permission middleware tests.
- `cool/crud/requests.go` — request structs and service override interfaces.
- `cool/crud/override_test.go` — default CRUD override and hook dispatch tests.
- `modules/base/service/auth.go` — migrated auth service.
- `modules/base/service/permission.go` — migrated permission service.
- `modules/base/service/sys_user.go` — user service embedding `cool/service.BaseService`.
- `modules/base/service/sys_role.go` — role service embedding `cool/service.BaseService`.
- `modules/base/service/sys_menu.go` — menu service embedding `cool/service.BaseService`.
- `modules/base/service/sys_department.go` — department service embedding `cool/service.BaseService`.
- `modules/base/service/sys_param.go` — param service embedding `cool/service.BaseService`.
- `modules/base/service/sys_log.go` — log service embedding `cool/service.BaseService`.
- `modules/base/service/wrappers.go` — shared helpers moved from base root when needed.
- `modules/base/controller/admin/sys_user.go` — user controller metadata.
- `modules/base/controller/admin/sys_role.go` — role controller metadata.
- `modules/base/controller/admin/sys_menu.go` — menu controller metadata.
- `modules/base/controller/admin/sys_department.go` — department controller metadata.
- `modules/base/controller/admin/sys_param.go` — param controller metadata.
- `modules/base/controller/admin/sys_log.go` — log controller metadata.
- `modules/base/controller/open/open.go` — open controller metadata.
- `modules/base/controller/comm/comm.go` — comm controller metadata.
- `modules/base/controller/controllers.go` — aggregates base controllers.
- `modules/base/controller/controllers_test.go` — verifies base controller metadata matches existing behavior.

### Modified files

- `cool/crud/types.go` — add `Service interface{}` and `InsertParam` to `ResourceSpec`.
- `cool/crud/metadata.go` — carry service and insert-param metadata into `Resource`.
- `cool/crud/runtime.go` — route Add/Delete/Update/Info/List/Page through overrides and hooks.
- `cool/crud/handler.go` — construct typed request structs and preserve response shapes.
- `cool/module/module.go` — add controllers to `Module` and `Definition`.
- `cool/module/module_test.go` — test controller storage and copy behavior.
- `cool/app/app.go` — collect controllers and register auth, permission, CRUD, and routes through framework APIs.
- `cool/app/app_test.go` — update startup tests for metadata-driven registration.
- `modules/base/base.go` — register base controllers on module construction.
- `modules/base/auth.go` — convert to root compatibility wrapper or remove after tests migrate.
- `modules/base/permission.go` — convert to root compatibility wrapper or remove after tests migrate.
- `modules/base/auth_routes.go` — remove from runtime path; keep only if a test needs short-term compatibility during task 7, then delete in task 9.
- `modules/base/permission_routes.go` — remove from runtime path; keep only if a test needs short-term compatibility during task 7, then delete in task 9.
- `modules/base/routes.go` — replace hand-written specs with controller-derived specs or delete after task 9.
- `modules/routes.go` — delegate to `controller.CRUDResourceSpecs` and `controller.RegisterRoutes` or remove unused functions.
- `modules/modules.go` — no behavior change expected beyond module returning controllers.
- `modules/base/*_test.go` — update imports from root `base` package to `modules/base/service` only when tests target service internals; keep root package behavior tests through wrappers.

---

### Task 1: Add `cool/service` BaseService

**Files:**
- Create: `cool/service/base.go`
- Create: `cool/service/base_test.go`

**Interfaces:**
- Consumes: `github.com/gogf/gf/v2/database/gdb.DB`, `github.com/toothdy/cool-admin-go-next/cool/model.Definition`
- Produces:
  - `type BaseService struct { DB gdb.DB; Model model.Definition }`
  - `func NewBaseService(db gdb.DB, definition model.Definition) *BaseService`
  - `type ModifyBeforeHook interface { ModifyBefore(ctx context.Context, action string, data interface{}) error }`
  - `type ModifyAfterHook interface { ModifyAfter(ctx context.Context, action string, data interface{}) error }`

- [ ] **Step 1: Write failing BaseService test**

Create `cool/service/base_test.go`:

```go
package service

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/model"
)

/**
 * 测试基础服务保存模型定义
 * @param t 测试对象
 * @returns null
 */
func TestNewBaseServiceStoresModel(t *testing.T) {
   definition := model.NewDefinition("base", "BaseSysUser", "base_sys_user")
   service := NewBaseService(nil, definition)

   if service.DB != nil {
      t.Fatal("expected nil DB to be stored as nil")
   }
   if service.Model.Name != "BaseSysUser" {
      t.Fatalf("expected model name BaseSysUser, got %s", service.Model.Name)
   }
   if service.Model.TableName != "base_sys_user" {
      t.Fatalf("expected table base_sys_user, got %s", service.Model.TableName)
   }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/service -run TestNewBaseServiceStoresModel -count=1
```

Expected: FAIL with package or `NewBaseService` undefined.

- [ ] **Step 3: Implement BaseService**

Create `cool/service/base.go`:

```go
package service

import (
   "context"

   "github.com/gogf/gf/v2/database/gdb"
   "github.com/toothdy/cool-admin-go-next/cool/model"
)

// BaseService 是业务服务基类。
type BaseService struct {
   DB    gdb.DB
   Model model.Definition
}

// ModifyBeforeHook 是默认 CRUD 修改前 hook。
type ModifyBeforeHook interface {
   ModifyBefore(ctx context.Context, action string, data interface{}) error
}

// ModifyAfterHook 是默认 CRUD 修改后 hook。
type ModifyAfterHook interface {
   ModifyAfter(ctx context.Context, action string, data interface{}) error
}

/**
 * 创建业务服务基类
 * @param db 数据库实例
 * @param definition 模型定义
 * @returns *BaseService
 */
func NewBaseService(db gdb.DB, definition model.Definition) *BaseService {
   return &BaseService{
      DB:    db,
      Model: definition,
   }
}
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/service/base.go cool/service/base_test.go
go test ./cool/service -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/service/base.go cool/service/base_test.go
git commit -m $'feat: add cool base service\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 2: Add `cool/controller` metadata and builder

**Files:**
- Create: `cool/controller/definition.go`
- Create: `cool/controller/builder.go`
- Create: `cool/controller/builder_test.go`

**Interfaces:**
- Consumes: `cool/model.Definition`, `cool/crud` API constants.
- Produces:
  - `type Area string`
  - `const AreaAdmin`, `AreaOpen`, `AreaComm`
  - `type Definition`, `CRUDOptions`, `CRUDDefinition`, `QueryOptions`, `RouteOptions`, `RouteDefinition`
  - `type InsertParamFunc func(ctx context.Context) map[string]interface{}`
  - `func Admin(path string) *Builder`, `func Open(path string) *Builder`, `func Comm(path string) *Builder`
  - builder methods: `Name`, `Description`, `Model`, `Service`, `CRUD`, `Route`, `Build`

- [ ] **Step 1: Write failing builder tests**

Create `cool/controller/builder_test.go`:

```go
package controller

import (
   "net/http"
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/crud"
   "github.com/toothdy/cool-admin-go-next/cool/model"
)

/**
 * 测试 Admin 构建器生成完整前缀
 * @param t 测试对象
 * @returns null
 */
func TestAdminBuilderBuildsPrefix(t *testing.T) {
   definition := Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("系统用户").
      Model(model.NewDefinition("base", "BaseSysUser", "base_sys_user")).
      CRUD(CRUDOptions{APIs: []string{crud.APIPage}}).
      Build()

   if definition.Module != "base" {
      t.Fatalf("expected module base, got %s", definition.Module)
   }
   if definition.Area != AreaAdmin {
      t.Fatalf("expected admin area, got %s", definition.Area)
   }
   if definition.Prefix != "/admin/base/sys/user" {
      t.Fatalf("expected prefix /admin/base/sys/user, got %s", definition.Prefix)
   }
   if definition.CRUD == nil || len(definition.CRUD.APIs) != 1 || definition.CRUD.APIs[0] != crud.APIPage {
      t.Fatalf("expected page CRUD metadata, got %#v", definition.CRUD)
   }
}

/**
 * 测试 Route 构建器生成完整路径
 * @param t 测试对象
 * @returns null
 */
func TestRouteBuildsFullPath(t *testing.T) {
   definition := Open("base/open").
      Route(RouteOptions{
         Name:       "login",
         Method:     http.MethodPost,
         Path:       "/login",
         IgnoreAuth: true,
      }).
      Build()

   if len(definition.Routes) != 1 {
      t.Fatalf("expected one route, got %d", len(definition.Routes))
   }
   route := definition.Routes[0]
   if route.FullPath != "/admin/base/open/login" {
      t.Fatalf("expected full path /admin/base/open/login, got %s", route.FullPath)
   }
   if !route.IgnoreAuth {
      t.Fatal("expected route to ignore auth")
   }
}

/**
 * 测试构建器复制切片避免外部修改
 * @param t 测试对象
 * @returns null
 */
func TestBuilderCopiesCRUDSlices(t *testing.T) {
   apis := []string{crud.APIAdd}
   definition := Admin("base/sys/role").CRUD(CRUDOptions{APIs: apis}).Build()
   apis[0] = crud.APIDelete

   if definition.CRUD.APIs[0] != crud.APIAdd {
      t.Fatalf("expected copied api add, got %s", definition.CRUD.APIs[0])
   }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/controller -run 'TestAdminBuilderBuildsPrefix|TestRouteBuildsFullPath|TestBuilderCopiesCRUDSlices' -count=1
```

Expected: FAIL with package or symbols undefined.

- [ ] **Step 3: Implement metadata types**

Create `cool/controller/definition.go`:

```go
package controller

import (
   "context"

   "github.com/toothdy/cool-admin-go-next/cool/model"
)

// Area 是 controller 分区。
type Area string

const (
   // AreaAdmin 是后台管理分区。
   AreaAdmin Area = "admin"
   // AreaOpen 是后台开放接口分区。
   AreaOpen Area = "open"
   // AreaComm 是后台通用接口分区。
   AreaComm Area = "comm"
)

// InsertParamFunc 是新增默认参数函数。
type InsertParamFunc func(ctx context.Context) map[string]interface{}

// Definition 是 controller 元数据。
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

// CRUDOptions 是 CRUD 构建配置。
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

// CRUDDefinition 是 CRUD 元数据。
type CRUDDefinition struct {
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

// QueryOptions 是查询配置。
type QueryOptions struct {
   KeyWordLikeFields []string
   FieldEq           []string
   FieldLike         []string
}

// RouteOptions 是自定义路由构建配置。
type RouteOptions struct {
   Name        string
   Method      string
   Path        string
   Description string
   IgnoreAuth  bool
   Permission  string
   Handler     interface{}
}

// RouteDefinition 是自定义路由元数据。
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

- [ ] **Step 4: Implement builder**

Create `cool/controller/builder.go`:

```go
package controller

import (
   "strings"

   "github.com/toothdy/cool-admin-go-next/cool/model"
)

// Builder 是 controller 元数据构建器。
type Builder struct {
   definition Definition
}

/**
 * 创建后台管理 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Admin(path string) *Builder {
   return newBuilder(AreaAdmin, path)
}

/**
 * 创建开放 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Open(path string) *Builder {
   return newBuilder(AreaOpen, path)
}

/**
 * 创建通用 controller 构建器
 * @param path 模块路径
 * @returns *Builder
 */
func Comm(path string) *Builder {
   return newBuilder(AreaComm, path)
}

/**
 * 创建 controller 构建器
 * @param area 分区
 * @param path 模块路径
 * @returns *Builder
 */
func newBuilder(area Area, path string) *Builder {
   normalized := strings.Trim(path, "/")
   parts := strings.Split(normalized, "/")
   moduleName := ""
   if len(parts) > 0 {
      moduleName = parts[0]
   }

   return &Builder{
      definition: Definition{
         Module: moduleName,
         Area:   area,
         Prefix: "/admin/" + normalized,
      },
   }
}

/**
 * 设置 controller 名称
 * @param name 名称
 * @returns *Builder
 */
func (b *Builder) Name(name string) *Builder {
   b.definition.Name = name
   return b
}

/**
 * 设置描述
 * @param description 描述
 * @returns *Builder
 */
func (b *Builder) Description(description string) *Builder {
   b.definition.Description = description
   return b
}

/**
 * 设置模型定义
 * @param definition 模型定义
 * @returns *Builder
 */
func (b *Builder) Model(definition model.Definition) *Builder {
   b.definition.Model = definition
   return b
}

/**
 * 设置业务服务
 * @param service 业务服务
 * @returns *Builder
 */
func (b *Builder) Service(service interface{}) *Builder {
   b.definition.Service = service
   return b
}

/**
 * 设置 CRUD 元数据
 * @param options CRUD 配置
 * @returns *Builder
 */
func (b *Builder) CRUD(options CRUDOptions) *Builder {
   b.definition.CRUD = &CRUDDefinition{
      APIs:             cloneStrings(options.APIs),
      PageQuery:        cloneQueryOptions(options.PageQuery),
      ListQuery:        cloneQueryOptions(options.ListQuery),
      InsertParam:      options.InsertParam,
      InfoIgnoreFields: cloneStrings(options.InfoIgnoreFields),
      SortFields:       cloneStrings(options.SortFields),
      HiddenFields:     cloneStrings(options.HiddenFields),
      ReadonlyFields:   cloneStrings(options.ReadonlyFields),
      DefaultSort:      options.DefaultSort,
      DefaultOrder:     options.DefaultOrder,
   }
   return b
}

/**
 * 添加自定义路由
 * @param options 路由配置
 * @returns *Builder
 */
func (b *Builder) Route(options RouteOptions) *Builder {
   path := "/" + strings.Trim(options.Path, "/")
   b.definition.Routes = append(b.definition.Routes, RouteDefinition{
      Name:        options.Name,
      Method:      strings.ToUpper(options.Method),
      Path:        path,
      FullPath:    b.definition.Prefix + path,
      Description: options.Description,
      IgnoreAuth:  options.IgnoreAuth,
      Permission:  options.Permission,
      Handler:     options.Handler,
   })
   return b
}

/**
 * 构建 controller 元数据
 * @returns Definition
 */
func (b *Builder) Build() Definition {
   definition := b.definition
   definition.Routes = append([]RouteDefinition{}, b.definition.Routes...)
   return definition
}

/**
 * 复制字符串切片
 * @param items 字符串切片
 * @returns []string
 */
func cloneStrings(items []string) []string {
   return append([]string{}, items...)
}

/**
 * 复制查询配置
 * @param options 查询配置
 * @returns QueryOptions
 */
func cloneQueryOptions(options QueryOptions) QueryOptions {
   return QueryOptions{
      KeyWordLikeFields: cloneStrings(options.KeyWordLikeFields),
      FieldEq:           cloneStrings(options.FieldEq),
      FieldLike:         cloneStrings(options.FieldLike),
   }
}
```

- [ ] **Step 5: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/controller/definition.go cool/controller/builder.go cool/controller/builder_test.go
go test ./cool/controller -run 'TestAdminBuilderBuildsPrefix|TestRouteBuildsFullPath|TestBuilderCopiesCRUDSlices' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/controller/definition.go cool/controller/builder.go cool/controller/builder_test.go
git commit -m $'feat: add cool controller builder\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 3: Add controller metadata derivation

**Files:**
- Create: `cool/controller/derive.go`
- Create: `cool/controller/derive_test.go`
- Modify: `cool/crud/types.go`

**Interfaces:**
- Consumes: `controller.Definition`, `crud.ResourceSpec`, `crud.RouteKey`.
- Produces:
  - `func IgnoreAuthPaths(controllers []Definition) []string`
  - `func PermissionMap(controllers []Definition) (map[string]string, error)`
  - `func CRUDResourceSpecs(controllers []Definition) ([]crud.ResourceSpec, error)`
  - `crud.ResourceSpec.Service interface{}`
  - `crud.ResourceSpec.InsertParam func(context.Context) map[string]interface{}`

- [ ] **Step 1: Write failing derivation tests**

Create `cool/controller/derive_test.go`:

```go
package controller

import (
   "context"
   "net/http"
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/crud"
   "github.com/toothdy/cool-admin-go-next/cool/model"
)

type deriveService struct{}

/**
 * 创建测试模型定义
 * @returns model.Definition
 */
func deriveModel() model.Definition {
   fields := model.BaseFields()
   fields = append(fields, model.NewField("name", "name", "varchar").Comment("名称"))
   return model.NewDefinition("base", "BaseSysUser", "base_sys_user").Fields(fields)
}

/**
 * 测试忽略认证路径从 route 元数据派生
 * @param t 测试对象
 * @returns null
 */
func TestIgnoreAuthPathsFromRoutes(t *testing.T) {
   controllers := []Definition{
      Open("base/open").
         Route(RouteOptions{Name: "login", Method: http.MethodPost, Path: "/login", IgnoreAuth: true}).
         Route(RouteOptions{Name: "private", Method: http.MethodGet, Path: "/private"}).
         Build(),
   }

   paths := IgnoreAuthPaths(controllers)
   if len(paths) != 1 || paths[0] != "/admin/base/open/login" {
      t.Fatalf("expected login ignore path, got %#v", paths)
   }
}

/**
 * 测试 CRUD 权限映射从 controller 派生
 * @param t 测试对象
 * @returns null
 */
func TestPermissionMapFromCRUDAndRoutes(t *testing.T) {
   controllers := []Definition{
      Admin("base/sys/user").
         CRUD(CRUDOptions{APIs: []string{crud.APIPage}}).
         Route(RouteOptions{Name: "move", Method: http.MethodPost, Path: "/move", Permission: "base:sys:user:move"}).
         Build(),
   }

   permissions, err := PermissionMap(controllers)
   if err != nil {
      t.Fatalf("build permission map failed: %v", err)
   }
   if permissions["POST:/admin/base/sys/user/page"] != "base:sys:user:page" {
      t.Fatalf("expected page permission, got %#v", permissions)
   }
   if permissions["POST:/admin/base/sys/user/move"] != "base:sys:user:move" {
      t.Fatalf("expected move permission, got %#v", permissions)
   }
}

/**
 * 测试 CRUD specs 从 controller 派生
 * @param t 测试对象
 * @returns null
 */
func TestCRUDResourceSpecsFromControllers(t *testing.T) {
   service := &deriveService{}
   insertParam := func(ctx context.Context) map[string]interface{} {
      return map[string]interface{}{"userId": int64(1)}
   }
   controllers := []Definition{
      Admin("base/sys/user").
         Model(deriveModel()).
         Service(service).
         CRUD(CRUDOptions{
            APIs:              []string{crud.APIAdd, crud.APIPage},
            PageQuery:         QueryOptions{KeyWordLikeFields: []string{"name"}},
            InsertParam:       insertParam,
            InfoIgnoreFields:  []string{"password"},
            SortFields:        []string{"id", "name"},
            HiddenFields:      []string{"password"},
            ReadonlyFields:    []string{"tenantId"},
            DefaultSort:       "id",
            DefaultOrder:      "DESC",
         }).
         Build(),
   }

   specs, err := CRUDResourceSpecs(controllers)
   if err != nil {
      t.Fatalf("build specs failed: %v", err)
   }
   if len(specs) != 1 {
      t.Fatalf("expected one spec, got %d", len(specs))
   }
   spec := specs[0]
   if spec.Name != "user" {
      t.Fatalf("expected resource user, got %s", spec.Name)
   }
   if spec.Prefix != "/admin/base/sys/user" {
      t.Fatalf("expected prefix /admin/base/sys/user, got %s", spec.Prefix)
   }
   if spec.Service != service {
      t.Fatal("expected service to be carried into resource spec")
   }
   if spec.InsertParam == nil {
      t.Fatal("expected insert param function")
   }
   if spec.KeywordFields[0] != "name" {
      t.Fatalf("expected keyword field name, got %#v", spec.KeywordFields)
   }
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/controller -run 'TestIgnoreAuthPathsFromRoutes|TestPermissionMapFromCRUDAndRoutes|TestCRUDResourceSpecsFromControllers' -count=1
```

Expected: FAIL with undefined derivation functions or missing `ResourceSpec.Service`.

- [ ] **Step 3: Extend `crud.ResourceSpec`**

Modify `cool/crud/types.go` imports and struct.

Add import:

```go
import (
   "context"
   "net/http"

   "github.com/toothdy/cool-admin-go-next/cool/model"
)
```

Update `ResourceSpec`:

```go
// ResourceSpec 是 CRUD 资源配置。
type ResourceSpec struct {
   Name             string
   Prefix           string
   Model            model.Definition
   Service          interface{}
   InsertParam      func(ctx context.Context) map[string]interface{}
   APIs             []string
   KeywordFields    []string
   EqualFields      []string
   LikeFields       []string
   SortFields       []string
   HiddenFields     []string
   ReadonlyFields   []string
   InfoIgnoreFields []string
   DefaultSort      string
   DefaultOrder     string
}
```

- [ ] **Step 4: Implement derivation functions**

Create `cool/controller/derive.go`:

```go
package controller

import (
   "fmt"
   "strings"

   "github.com/toothdy/cool-admin-go-next/cool/crud"
)

/**
 * 获取忽略认证路径
 * @param controllers controller 元数据
 * @returns []string
 */
func IgnoreAuthPaths(controllers []Definition) []string {
   paths := make([]string, 0)
   for _, definition := range controllers {
      for _, route := range definition.Routes {
         if route.IgnoreAuth {
            paths = append(paths, route.FullPath)
         }
      }
   }
   return paths
}

/**
 * 构建权限映射
 * @param controllers controller 元数据
 * @returns 权限映射
 */
func PermissionMap(controllers []Definition) (map[string]string, error) {
   permissions := map[string]string{}
   for _, definition := range controllers {
      if definition.CRUD != nil {
         for _, api := range definition.CRUD.APIs {
            routeKey, ok := crud.RouteKey(definition.Prefix, api)
            if !ok {
               return nil, fmt.Errorf("unsupported CRUD api: %s", api)
            }
            permissions[routeKey] = permissionCode(definition.Prefix, api)
         }
      }
      for _, route := range definition.Routes {
         if route.IgnoreAuth || route.Permission == "" {
            continue
         }
         permissions[strings.ToUpper(route.Method)+":"+route.FullPath] = route.Permission
      }
   }
   return permissions, nil
}

/**
 * 生成 CRUD 资源配置
 * @param controllers controller 元数据
 * @returns []crud.ResourceSpec
 */
func CRUDResourceSpecs(controllers []Definition) ([]crud.ResourceSpec, error) {
   specs := make([]crud.ResourceSpec, 0)
   for _, definition := range controllers {
      if definition.CRUD == nil {
         continue
      }
      resourceName := resourceNameFromPrefix(definition.Prefix)
      specs = append(specs, crud.ResourceSpec{
         Name:             resourceName,
         Prefix:           definition.Prefix,
         Model:            definition.Model,
         Service:          definition.Service,
         InsertParam:      definition.CRUD.InsertParam,
         APIs:             cloneStrings(definition.CRUD.APIs),
         KeywordFields:    cloneStrings(definition.CRUD.PageQuery.KeyWordLikeFields),
         EqualFields:      cloneStrings(definition.CRUD.PageQuery.FieldEq),
         LikeFields:       cloneStrings(definition.CRUD.PageQuery.FieldLike),
         SortFields:       cloneStrings(definition.CRUD.SortFields),
         HiddenFields:     cloneStrings(definition.CRUD.HiddenFields),
         ReadonlyFields:   cloneStrings(definition.CRUD.ReadonlyFields),
         InfoIgnoreFields: cloneStrings(definition.CRUD.InfoIgnoreFields),
         DefaultSort:      definition.CRUD.DefaultSort,
         DefaultOrder:     definition.CRUD.DefaultOrder,
      })
   }
   return specs, nil
}

/**
 * 生成权限码
 * @param prefix 路由前缀
 * @param api CRUD API
 * @returns string
 */
func permissionCode(prefix string, api string) string {
   resource := strings.Trim(strings.TrimPrefix(prefix, "/admin/"), "/")
   return strings.ReplaceAll(resource, "/", ":") + ":" + api
}

/**
 * 从前缀提取资源名
 * @param prefix 路由前缀
 * @returns string
 */
func resourceNameFromPrefix(prefix string) string {
   parts := strings.Split(strings.Trim(prefix, "/"), "/")
   if len(parts) == 0 {
      return ""
   }
   return parts[len(parts)-1]
}
```

- [ ] **Step 5: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/crud/types.go cool/controller/derive.go cool/controller/derive_test.go
go test ./cool/controller -run 'TestIgnoreAuthPathsFromRoutes|TestPermissionMapFromCRUDAndRoutes|TestCRUDResourceSpecsFromControllers' -count=1
go test ./cool/crud -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/crud/types.go cool/controller/derive.go cool/controller/derive_test.go
git commit -m $'feat: derive controller runtime metadata\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 4: Add CRUD request types, service override dispatch, and hooks

**Files:**
- Create: `cool/crud/requests.go`
- Create: `cool/crud/override_test.go`
- Modify: `cool/crud/metadata.go`
- Modify: `cool/crud/runtime.go`
- Modify: `cool/crud/handler.go`

**Interfaces:**
- Consumes: `crud.ResourceSpec.Service`, `crud.ResourceSpec.InsertParam`, `cool/service.ModifyBeforeHook`, `cool/service.ModifyAfterHook`.
- Produces:
  - `type AddRequest struct { Data map[string]interface{} }`
  - `type DeleteRequest struct { IDs []interface{} }`
  - `type UpdateRequest struct { Data map[string]interface{} }`
  - `type InfoRequest struct { ID interface{} }`
  - `type AddHandler`, `DeleteHandler`, `UpdateHandler`, `InfoHandler`, `ListHandler`, `PageHandler`
  - Runtime methods still expose existing public names and preserve response shapes.

- [ ] **Step 1: Write failing override tests**

Create `cool/crud/override_test.go` with focused in-memory tests. Use a nil DB only for override paths so default SQL is not executed.

```go
package crud

import (
   "context"
   "testing"

   coolService "github.com/toothdy/cool-admin-go-next/cool/service"
)

type pageOverrideService struct {
   called bool
}

/**
 * 重写分页查询
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 分页结果
 */
func (s *pageOverrideService) Page(ctx context.Context, request QueryRequest) (interface{}, error) {
   s.called = true
   return PageResult{Pagination: Pagination{Page: request.Page, Size: request.Size, Total: 1}}, nil
}

type hookService struct {
   before []string
   after  []string
}

/**
 * 修改前 hook
 * @param ctx 上下文
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func (s *hookService) ModifyBefore(ctx context.Context, action string, data interface{}) error {
   s.before = append(s.before, action)
   return nil
}

/**
 * 修改后 hook
 * @param ctx 上下文
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func (s *hookService) ModifyAfter(ctx context.Context, action string, data interface{}) error {
   s.after = append(s.after, action)
   return nil
}

var _ coolService.ModifyBeforeHook = (*hookService)(nil)
var _ coolService.ModifyAfterHook = (*hookService)(nil)

/**
 * 测试业务 Service 重写分页
 * @param t 测试对象
 * @returns null
 */
func TestRuntimePageUsesServiceOverride(t *testing.T) {
   service := &pageOverrideService{}
   runtime := NewRuntime(nil, nil)
   resource := Resource{Spec: ResourceSpec{Name: "user", Service: service}}

   result, err := runtime.Page(context.Background(), resource, QueryRequest{Page: 2, Size: 10})
   if err != nil {
      t.Fatalf("page override failed: %v", err)
   }
   if !service.called {
      t.Fatal("expected page override to be called")
   }
   pageResult, ok := result.(PageResult)
   if !ok {
      t.Fatalf("expected PageResult, got %T", result)
   }
   if pageResult.Pagination.Page != 2 {
      t.Fatalf("expected page 2, got %d", pageResult.Pagination.Page)
   }
}

/**
 * 测试新增合并 InsertParam 后传给重写方法
 * @param t 测试对象
 * @returns null
 */
func TestRuntimeAddMergesInsertParamBeforeOverride(t *testing.T) {
   service := &addCaptureService{}
   runtime := NewRuntime(nil, nil)
   resource := Resource{
      Spec: ResourceSpec{
         Name:    "user",
         Service: service,
         InsertParam: func(ctx context.Context) map[string]interface{} {
            return map[string]interface{}{"userId": int64(7)}
         },
      },
   }

   _, err := runtime.Add(context.Background(), resource, map[string]interface{}{"name": "neo"})
   if err != nil {
      t.Fatalf("add override failed: %v", err)
   }
   if service.data["userId"] != int64(7) {
      t.Fatalf("expected merged userId 7, got %#v", service.data)
   }
}

type addCaptureService struct {
   data map[string]interface{}
}

/**
 * 重写新增
 * @param ctx 上下文
 * @param request 新增请求
 * @returns 新增结果
 */
func (s *addCaptureService) Add(ctx context.Context, request AddRequest) (interface{}, error) {
   s.data = request.Data
   return map[string]interface{}{"id": int64(1)}, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/crud -run 'TestRuntimePageUsesServiceOverride|TestRuntimeAddMergesInsertParamBeforeOverride' -count=1
```

Expected: FAIL because request types or override dispatch are missing.

- [ ] **Step 3: Add request and handler interfaces**

Create `cool/crud/requests.go`:

```go
package crud

import "context"

// AddRequest 是新增请求。
type AddRequest struct {
   Data map[string]interface{}
}

// DeleteRequest 是删除请求。
type DeleteRequest struct {
   IDs []interface{}
}

// UpdateRequest 是更新请求。
type UpdateRequest struct {
   Data map[string]interface{}
}

// InfoRequest 是详情请求。
type InfoRequest struct {
   ID interface{}
}

// AddHandler 是新增重写接口。
type AddHandler interface {
   Add(ctx context.Context, request AddRequest) (interface{}, error)
}

// DeleteHandler 是删除重写接口。
type DeleteHandler interface {
   Delete(ctx context.Context, request DeleteRequest) (interface{}, error)
}

// UpdateHandler 是更新重写接口。
type UpdateHandler interface {
   Update(ctx context.Context, request UpdateRequest) (interface{}, error)
}

// InfoHandler 是详情重写接口。
type InfoHandler interface {
   Info(ctx context.Context, request InfoRequest) (interface{}, error)
}

// ListHandler 是列表重写接口。
type ListHandler interface {
   List(ctx context.Context, request QueryRequest) (interface{}, error)
}

// PageHandler 是分页重写接口。
type PageHandler interface {
   Page(ctx context.Context, request QueryRequest) (interface{}, error)
}
```

- [ ] **Step 4: Carry spec metadata into Resource**

Modify `cool/crud/metadata.go` `Resource` struct:

```go
// Resource 是运行时资源定义。
type Resource struct {
   Spec             ResourceSpec
   Service          interface{}
   InsertParam      func(ctx context.Context) map[string]interface{}
   FieldsByJSON     map[string]model.Field
   FieldsByColumn   map[string]model.Field
   APIs             map[string]bool
   KeywordFields    map[string]model.Field
   EqualFields      map[string]model.Field
   LikeFields       map[string]model.Field
   SortFields       map[string]model.Field
   HiddenFields     map[string]bool
   ReadonlyFields   map[string]bool
   InfoIgnoreFields map[string]bool
   PrimaryField     model.Field
}
```

Add `context` import to `metadata.go`. In `buildResource`, initialize and fill:

```go
resource := Resource{
   Spec:             spec,
   Service:          spec.Service,
   InsertParam:      spec.InsertParam,
   FieldsByJSON:     map[string]model.Field{},
   FieldsByColumn:   map[string]model.Field{},
   APIs:             map[string]bool{},
   KeywordFields:    map[string]model.Field{},
   EqualFields:      map[string]model.Field{},
   LikeFields:       map[string]model.Field{},
   SortFields:       map[string]model.Field{},
   HiddenFields:     map[string]bool{},
   ReadonlyFields:   map[string]bool{},
   InfoIgnoreFields: map[string]bool{},
   PrimaryField:     primaryField,
}
```

Then fill ignore fields:

```go
for _, fieldName := range spec.InfoIgnoreFields {
   resource.InfoIgnoreFields[fieldName] = true
}
```

- [ ] **Step 5: Update Runtime signatures to return `interface{}` for override-compatible methods**

Modify `cool/crud/runtime.go` public method signatures:

```go
func (r *Runtime) Add(ctx context.Context, resource Resource, input map[string]interface{}) (interface{}, error)
func (r *Runtime) Delete(ctx context.Context, resource Resource, ids []interface{}) (interface{}, error)
func (r *Runtime) Update(ctx context.Context, resource Resource, input map[string]interface{}) (interface{}, error)
func (r *Runtime) Info(ctx context.Context, resource Resource, id interface{}) (interface{}, error)
func (r *Runtime) List(ctx context.Context, resource Resource, request QueryRequest) (interface{}, error)
func (r *Runtime) Page(ctx context.Context, resource Resource, request QueryRequest) (interface{}, error)
```

Use this exact dispatch pattern in each method:

```go
if handler, ok := resource.Service.(PageHandler); ok {
   return handler.Page(ctx, request)
}
```

For `Add`, merge insert params before override or default:

```go
input = mergeInsertParam(ctx, resource, input)
if handler, ok := resource.Service.(AddHandler); ok {
   return handler.Add(ctx, AddRequest{Data: input})
}
```

Add helpers to `runtime.go`:

```go
/**
 * 合并新增默认参数
 * @param ctx 上下文
 * @param resource 资源定义
 * @param input 输入数据
 * @returns 合并后的输入数据
 */
func mergeInsertParam(ctx context.Context, resource Resource, input map[string]interface{}) map[string]interface{} {
   merged := map[string]interface{}{}
   for key, value := range input {
      merged[key] = value
   }
   if resource.InsertParam == nil {
      return merged
   }
   for key, value := range resource.InsertParam(ctx) {
      merged[key] = value
   }
   return merged
}
```

Default return values must preserve current JSON response behavior:

- `Add` returns `map[string]interface{}{primaryKey: id}`.
- `Delete` returns `map[string]interface{}{}`.
- `Update` returns `map[string]interface{}{}`.
- `Info` returns `map[string]interface{}`.
- `List` returns `[]map[string]interface{}`.
- `Page` returns `PageResult`.

- [ ] **Step 6: Add default CRUD hooks**

Import `cool/service` in `runtime.go` as `coolService` and add helpers:

```go
/**
 * 执行修改前 hook
 * @param ctx 上下文
 * @param service 业务服务
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func runModifyBefore(ctx context.Context, service interface{}, action string, data interface{}) error {
   hook, ok := service.(coolService.ModifyBeforeHook)
   if !ok {
      return nil
   }
   return hook.ModifyBefore(ctx, action, data)
}

/**
 * 执行修改后 hook
 * @param ctx 上下文
 * @param service 业务服务
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func runModifyAfter(ctx context.Context, service interface{}, action string, data interface{}) error {
   hook, ok := service.(coolService.ModifyAfterHook)
   if !ok {
      return nil
   }
   return hook.ModifyAfter(ctx, action, data)
}
```

Call hooks only in default `Add`, `Delete`, `Update`, not around overrides.

- [ ] **Step 7: Update handlers for new return types**

Modify `cool/crud/handler.go`:

- `handleDelete` should use returned `data` from `runtime.Delete` and write `response.OK(data)`.
- `handleUpdate` should use returned `data` from `runtime.Update` and write `response.OK(data)`.
- other handlers already write returned data and should compile after signature changes.

Exact delete/update pattern:

```go
data, err := runtime.Delete(r.Context(), resource, ids)
if err != nil {
   writeCommError(r, err)
   return
}
r.Response.WriteJson(response.OK(data))
```

```go
data, err := runtime.Update(r.Context(), resource, input)
if err != nil {
   writeCommError(r, err)
   return
}
r.Response.WriteJson(response.OK(data))
```

- [ ] **Step 8: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/crud/requests.go cool/crud/override_test.go cool/crud/types.go cool/crud/metadata.go cool/crud/runtime.go cool/crud/handler.go
go test ./cool/crud -run 'TestRuntimePageUsesServiceOverride|TestRuntimeAddMergesInsertParamBeforeOverride' -count=1
go test ./cool/crud -count=1
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/crud/requests.go cool/crud/override_test.go cool/crud/types.go cool/crud/metadata.go cool/crud/runtime.go cool/crud/handler.go
git commit -m $'feat: support crud service overrides\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 5: Add controller route registration and permission middleware

**Files:**
- Create: `cool/controller/register.go`
- Create: `cool/controller/permission.go`
- Create: `cool/controller/register_test.go`
- Create: `cool/controller/permission_test.go`

**Interfaces:**
- Consumes: `crud.RegisterResourceRoutes`, `crud.Runtime`, `auth.UserFromContext`, `auth.Unauthorized`.
- Produces:
  - `func RegisterRoutes(server *ghttp.Server, runtime *crud.Runtime, controllers []Definition) error`
  - `type PermissionChecker interface { HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error) }`
  - `func RegisterPermissionMiddleware(server *ghttp.Server, checker PermissionChecker, permissions map[string]string)`

- [ ] **Step 1: Write failing registration test**

Create `cool/controller/register_test.go`:

```go
package controller

import (
   "context"
   "net/http"
   "net/http/httptest"
   "testing"

   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

/**
 * 测试自定义路由注册
 * @param t 测试对象
 * @returns null
 */
func TestRegisterRoutesBindsCustomRoute(t *testing.T) {
   server := ghttp.GetServer("controller-register-test")
   server.SetPort(0)
   defer server.Shutdown()

   controllers := []Definition{
      Open("base/open").Route(RouteOptions{
         Name:   "ping",
         Method: http.MethodGet,
         Path:   "/ping",
         Handler: func(ctx context.Context) (response.Body, error) {
            return response.OK("pong"), nil
         },
      }).Build(),
   }

   if err := RegisterRoutes(server, nil, controllers); err != nil {
      t.Fatalf("register routes failed: %v", err)
   }

   req := httptest.NewRequest(http.MethodGet, "/admin/base/open/ping", nil)
   rec := httptest.NewRecorder()
   server.ServeHTTP(rec, req)

   if rec.Code != http.StatusOK {
      t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
   }
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/controller -run TestRegisterRoutesBindsCustomRoute -count=1
```

Expected: FAIL with `RegisterRoutes` undefined.

- [ ] **Step 3: Implement route registration**

Create `cool/controller/register.go`:

```go
package controller

import (
   "context"
   "fmt"
   "reflect"
   "strings"

   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/crud"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

/**
 * 注册 controller 路由
 * @param server HTTP 服务
 * @param runtime CRUD 运行时
 * @param controllers controller 元数据
 * @returns error
 */
func RegisterRoutes(server *ghttp.Server, runtime *crud.Runtime, controllers []Definition) error {
   for _, definition := range controllers {
      for _, route := range definition.Routes {
         route := route
         if route.Handler == nil {
            continue
         }
         server.BindHandler(strings.ToUpper(route.Method)+":"+route.FullPath, func(r *ghttp.Request) {
            handleCustomRoute(r, route.Handler)
         })
      }
      if definition.CRUD == nil || runtime == nil {
         continue
      }
      resource, ok := runtime.Registry().Resource(resourceNameFromPrefix(definition.Prefix))
      if !ok {
         return fmt.Errorf("CRUD 资源不存在: %s", resourceNameFromPrefix(definition.Prefix))
      }
      crud.RegisterResourceRoutes(server, runtime, resource)
   }
   return nil
}

/**
 * 处理自定义路由
 * @param r HTTP 请求
 * @param handler 处理函数
 * @returns null
 */
func handleCustomRoute(r *ghttp.Request, handler interface{}) {
   value := reflect.ValueOf(handler)
   if value.Kind() != reflect.Func {
      r.Response.WriteJson(response.Fail("路由处理器不是函数"))
      return
   }
   handlerType := value.Type()
   args := make([]reflect.Value, 0, handlerType.NumIn())
   for i := 0; i < handlerType.NumIn(); i++ {
      argType := handlerType.In(i)
      switch argType {
      case reflect.TypeOf((*context.Context)(nil)).Elem():
         args = append(args, reflect.ValueOf(r.Context()))
      case reflect.TypeOf((*ghttp.Request)(nil)):
         args = append(args, reflect.ValueOf(r))
      default:
         args = append(args, reflect.Zero(argType))
      }
   }
   results := value.Call(args)
   if len(results) == 0 {
      r.Response.WriteJson(response.OK(map[string]interface{}{}))
      return
   }
   if len(results) == 2 && !results[1].IsNil() {
      err, ok := results[1].Interface().(error)
      if ok {
         r.Response.WriteJson(response.Fail(err.Error()))
         return
      }
   }
   r.Response.WriteJson(response.OK(results[0].Interface()))
}
```

Note for implementer: if returning `response.Body`, this initial implementation wraps it in `response.OK`. Before completing the task, adjust `handleCustomRoute` to detect `response.Body` and write it directly:

```go
if body, ok := results[0].Interface().(response.Body); ok {
   r.Response.WriteJson(body)
   return
}
```

- [ ] **Step 4: Write permission middleware test**

Create `cool/controller/permission_test.go`:

```go
package controller

import (
   "context"
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

type allowChecker struct {
   permission string
}

/**
 * 检查权限
 * @param ctx 上下文
 * @param user 用户上下文
 * @param permission 权限码
 * @returns bool
 */
func (c *allowChecker) HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error) {
   c.permission = permission
   return permission == "base:sys:user:page", nil
}

/**
 * 测试权限 checker 接口
 * @param t 测试对象
 * @returns null
 */
func TestPermissionCheckerInterface(t *testing.T) {
   checker := &allowChecker{}
   ok, err := checker.HasPermission(context.Background(), auth.UserContext{Username: "demo"}, "base:sys:user:page")
   if err != nil {
      t.Fatalf("check permission failed: %v", err)
   }
   if !ok {
      t.Fatal("expected permission allowed")
   }
   if checker.permission != "base:sys:user:page" {
      t.Fatalf("expected captured permission, got %s", checker.permission)
   }
}
```

- [ ] **Step 5: Implement permission middleware**

Create `cool/controller/permission.go`:

```go
package controller

import (
   "context"
   "net/http"
   "strings"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

const permissionDeniedMessage = "权限不足~"

// PermissionChecker 是权限检查器。
type PermissionChecker interface {
   HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error)
}

/**
 * 注册权限 middleware
 * @param server HTTP 服务
 * @param checker 权限检查器
 * @param permissions 路由权限映射
 * @returns null
 */
func RegisterPermissionMiddleware(server *ghttp.Server, checker PermissionChecker, permissions map[string]string) {
   server.Use(func(r *ghttp.Request) {
      permission, ok := RoutePermission(permissions, r.Method, r.URL.Path)
      if !ok {
         r.Middleware.Next()
         return
      }

      user, hasUser := auth.UserFromContext(r.Context())
      if !hasUser {
         auth.Unauthorized(r)
         return
      }

      allowed, err := checker.HasPermission(r.Context(), user, permission)
      if err != nil {
         g.Log().Error(r.Context(), err)
         WriteForbidden(r)
         return
      }
      if !allowed {
         WriteForbidden(r)
         return
      }
      r.Middleware.Next()
   })
}

/**
 * 获取路由权限码
 * @param permissions 权限映射
 * @param method HTTP 方法
 * @param path 路径
 * @returns 权限码和是否存在
 */
func RoutePermission(permissions map[string]string, method string, path string) (string, bool) {
   key := strings.ToUpper(method) + ":" + path
   permission, ok := permissions[key]
   return permission, ok
}

/**
 * 写入无权限响应
 * @param r HTTP 请求
 * @returns null
 */
func WriteForbidden(r *ghttp.Request) {
   r.Response.WriteHeader(http.StatusForbidden)
   r.Response.WriteJson(response.Body{
      Code:    coolErrors.CodeCommFail,
      Message: permissionDeniedMessage,
   })
}
```

- [ ] **Step 6: Fix `handleCustomRoute` response.Body handling**

Update `cool/controller/register.go` result handling:

```go
if body, ok := results[0].Interface().(response.Body); ok {
   r.Response.WriteJson(body)
   return
}
r.Response.WriteJson(response.OK(results[0].Interface()))
```

- [ ] **Step 7: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/controller/register.go cool/controller/permission.go cool/controller/register_test.go cool/controller/permission_test.go
go test ./cool/controller -run 'TestRegisterRoutesBindsCustomRoute|TestPermissionCheckerInterface' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/controller/register.go cool/controller/permission.go cool/controller/register_test.go cool/controller/permission_test.go
git commit -m $'feat: register controller runtime routes\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 6: Extend module metadata with controllers

**Files:**
- Modify: `cool/module/module.go`
- Modify: `cool/module/module_test.go`

**Interfaces:**
- Consumes: `controller.Definition`.
- Produces:
  - `Module.ModuleControllers() []controller.Definition`
  - `Definition.Controllers(controllers []controller.Definition) *Definition`
  - `CollectControllers(modules []Module) []controller.Definition`

- [ ] **Step 1: Add failing module tests**

Append to `cool/module/module_test.go`:

```go
/**
 * 测试模块保存 controller 元数据
 * @param t 测试对象
 * @returns null
 */
func TestModuleControllersReturnsCopy(t *testing.T) {
   controllers := []controller.Definition{
      controller.Admin("base/sys/user").Build(),
   }
   mod := New("base").Controllers(controllers)
   controllers[0] = controller.Admin("base/sys/role").Build()

   stored := mod.ModuleControllers()
   if len(stored) != 1 {
      t.Fatalf("expected one controller, got %d", len(stored))
   }
   if stored[0].Prefix != "/admin/base/sys/user" {
      t.Fatalf("expected user prefix, got %s", stored[0].Prefix)
   }

   stored[0] = controller.Admin("base/sys/menu").Build()
   fresh := mod.ModuleControllers()
   if fresh[0].Prefix != "/admin/base/sys/user" {
      t.Fatalf("expected copied controller slice, got %s", fresh[0].Prefix)
   }
}

/**
 * 测试收集模块 controller 元数据
 * @param t 测试对象
 * @returns null
 */
func TestCollectControllers(t *testing.T) {
   mods := []Module{
      New("base").Controllers([]controller.Definition{controller.Admin("base/sys/user").Build()}),
      New("demo").Controllers([]controller.Definition{controller.Admin("demo/goods").Build()}),
   }

   controllers := CollectControllers(mods)
   if len(controllers) != 2 {
      t.Fatalf("expected two controllers, got %d", len(controllers))
   }
}
```

Add import to `module_test.go`:

```go
"github.com/toothdy/cool-admin-go-next/cool/controller"
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/module -run 'TestModuleControllersReturnsCopy|TestCollectControllers' -count=1
```

Expected: FAIL with missing methods.

- [ ] **Step 3: Update module interface and definition**

Modify `cool/module/module.go` imports:

```go
import (
   "sort"

   "github.com/toothdy/cool-admin-go-next/cool/controller"
   "github.com/toothdy/cool-admin-go-next/cool/model"
   "github.com/toothdy/cool-admin-go-next/cool/seed"
)
```

Add to `Module` interface:

```go
ModuleControllers() []controller.Definition
```

Add field to `Definition`:

```go
controllers []controller.Definition
```

Add methods:

```go
/**
 * 设置模块 controller 元数据
 * @param controllers controller 元数据
 * @returns *Definition
 */
func (d *Definition) Controllers(controllers []controller.Definition) *Definition {
   d.controllers = append([]controller.Definition{}, controllers...)
   return d
}

/**
 * 模块 controller 元数据
 * @returns []controller.Definition
 */
func (d *Definition) ModuleControllers() []controller.Definition {
   return append([]controller.Definition{}, d.controllers...)
}

/**
 * 收集模块 controller 元数据
 * @param modules 模块列表
 * @returns []controller.Definition
 */
func CollectControllers(modules []Module) []controller.Definition {
   controllers := make([]controller.Definition, 0)
   for _, mod := range modules {
      controllers = append(controllers, mod.ModuleControllers()...)
   }
   return controllers
}
```

- [ ] **Step 4: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/module/module.go cool/module/module_test.go
go test ./cool/module -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/module/module.go cool/module/module_test.go
git commit -m $'feat: collect module controllers\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 7: Migrate base services and preserve root compatibility

**Files:**
- Create: `modules/base/service/auth.go`
- Create: `modules/base/service/permission.go`
- Create: `modules/base/service/wrappers.go`
- Create: `modules/base/service/sys_user.go`
- Create: `modules/base/service/sys_role.go`
- Create: `modules/base/service/sys_menu.go`
- Create: `modules/base/service/sys_department.go`
- Create: `modules/base/service/sys_param.go`
- Create: `modules/base/service/sys_log.go`
- Modify: `modules/base/auth.go`
- Modify: `modules/base/permission.go`
- Modify: `modules/base/auth_test.go`
- Modify: `modules/base/permission_test.go`

**Interfaces:**
- Consumes: existing `base.AuthService`, `base.PermissionService`, model definitions.
- Produces:
  - `modules/base/service.AuthService`
  - `modules/base/service.PermissionService`
  - `modules/base/service.NewAuthService`
  - `modules/base/service.NewPermissionService`
  - resource services embedding `cool/service.BaseService`
  - root package aliases for short-term compatibility.

- [ ] **Step 1: Create service package by moving auth implementation**

Move the full current contents of `modules/base/auth.go` into `modules/base/service/auth.go` and change package line:

```go
package service
```

Keep imports identical unless package references require adjustment. The existing code's helper functions `recordValues`, `recordValue`, `stringValue`, and `int64Value` move with it.

- [ ] **Step 2: Create permission service package**

Move the full current contents of `modules/base/permission.go` into `modules/base/service/permission.go` and change package line:

```go
package service
```

Update imports that currently refer to base model:

```go
baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
```

For `CRUDResourceSpecs`, temporarily import root base with alias only if needed. Prefer adding a package-level function variable in `service/permission.go`:

```go
var buildCRUDResourceSpecs = baseCRUDResourceSpecs
```

If this creates an import cycle, leave `CRUDPermissionMap` wrapper in root `modules/base/permission.go` until task 9 replaces it with controller-derived permissions.

- [ ] **Step 3: Replace root auth file with compatibility wrapper**

Modify `modules/base/auth.go` to:

```go
package base

import (
   "github.com/gogf/gf/v2/database/gdb"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   baseService "github.com/toothdy/cool-admin-go-next/modules/base/service"
)

type AuthService = baseService.AuthService
type LoginRequest = baseService.LoginRequest

var FilterUserPassword = baseService.FilterUserPassword

/**
 * 创建认证服务
 * @param db 数据库实例
 * @param manager token 管理器
 * @returns *AuthService
 */
func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService {
   return baseService.NewAuthService(db, manager)
}
```

- [ ] **Step 4: Replace root permission file with compatibility wrapper**

Modify `modules/base/permission.go` to expose existing public API through service package. If `CRUDPermissionMap` still depends on root `CRUDResourceSpecs`, keep its current root implementation until task 9.

Minimum wrapper:

```go
package base

import (
   "github.com/gogf/gf/v2/database/gdb"
   baseService "github.com/toothdy/cool-admin-go-next/modules/base/service"
)

type PermissionService = baseService.PermissionService
type PermMenuResult = baseService.PermMenuResult

var SplitPerms = baseService.SplitPerms
var UniqueStrings = baseService.UniqueStrings
var MenuRowShouldEnterMenus = baseService.MenuRowShouldEnterMenus

/**
 * 创建权限服务
 * @param db 数据库实例
 * @returns *PermissionService
 */
func NewPermissionService(db gdb.DB) *PermissionService {
   return baseService.NewPermissionService(db)
}
```

- [ ] **Step 5: Add resource services**

Each resource service embeds `cool/service.BaseService`.

Example `modules/base/service/sys_user.go`:

```go
package service

import (
   "github.com/gogf/gf/v2/database/gdb"
   coolModel "github.com/toothdy/cool-admin-go-next/cool/model"
   coolService "github.com/toothdy/cool-admin-go-next/cool/service"
)

// UserService 是系统用户服务。
type UserService struct {
   *coolService.BaseService
}

/**
 * 创建系统用户服务
 * @param db 数据库实例
 * @param definition 模型定义
 * @returns *UserService
 */
func NewUserService(db gdb.DB, definition coolModel.Definition) *UserService {
   return &UserService{
      BaseService: coolService.NewBaseService(db, definition),
   }
}
```

Repeat same pattern for:

- `RoleService` / `NewRoleService`
- `MenuService` / `NewMenuService`
- `DepartmentService` / `NewDepartmentService`
- `ParamService` / `NewParamService`
- `LogService` / `NewLogService`

- [ ] **Step 6: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/auth.go modules/base/permission.go modules/base/service/*.go
go test ./modules/base -run 'TestLoginRequest|TestCRUDPermissionMap|TestSplitPerms|TestUniqueStrings' -count=1
go test ./modules/base/service -count=1
```

Expected: PASS. If `./modules/base/service` has no tests, expected output is successful compile with `? ... [no test files]`.

- [ ] **Step 7: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add modules/base/auth.go modules/base/permission.go modules/base/service/auth.go modules/base/service/permission.go modules/base/service/wrappers.go modules/base/service/sys_user.go modules/base/service/sys_role.go modules/base/service/sys_menu.go modules/base/service/sys_department.go modules/base/service/sys_param.go modules/base/service/sys_log.go modules/base/auth_test.go modules/base/permission_test.go
git commit -m $'refactor: move base services into service package\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 8: Add base controller metadata

**Files:**
- Create: `modules/base/controller/admin/sys_user.go`
- Create: `modules/base/controller/admin/sys_role.go`
- Create: `modules/base/controller/admin/sys_menu.go`
- Create: `modules/base/controller/admin/sys_department.go`
- Create: `modules/base/controller/admin/sys_param.go`
- Create: `modules/base/controller/admin/sys_log.go`
- Create: `modules/base/controller/open/open.go`
- Create: `modules/base/controller/comm/comm.go`
- Create: `modules/base/controller/controllers.go`
- Create: `modules/base/controller/controllers_test.go`

**Interfaces:**
- Consumes: `cool/controller`, `cool/crud`, `modules/base/model`, `modules/base/service`.
- Produces:
  - `func Controllers(db gdb.DB, manager *auth.Manager) []controller.Definition`
  - Admin sys controller functions for user/role/menu/department/param/log.
  - Open and comm controller functions.

- [ ] **Step 1: Write failing base controller tests**

Create `modules/base/controller/controllers_test.go`:

```go
package controller

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/crud"
   baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
)

/**
 * 测试 base controllers 覆盖核心资源
 * @param t 测试对象
 * @returns null
 */
func TestControllersDeclareBaseResources(t *testing.T) {
   controllers := Controllers(nil, nil)
   prefixes := map[string]bool{}
   for _, definition := range controllers {
      prefixes[definition.Prefix] = true
   }

   expected := []string{
      "/admin/base/sys/user",
      "/admin/base/sys/role",
      "/admin/base/sys/menu",
      "/admin/base/sys/department",
      "/admin/base/sys/param",
      "/admin/base/sys/log",
      "/admin/base/open",
      "/admin/base/comm",
   }
   for _, prefix := range expected {
      if !prefixes[prefix] {
         t.Fatalf("expected prefix %s in controllers", prefix)
      }
   }
}

/**
 * 测试用户 controller CRUD 配置
 * @param t 测试对象
 * @returns null
 */
func TestUserControllerCRUDMetadata(t *testing.T) {
   models := modelMap(baseModel.Register())
   definition := UserController(nil, models["base_sys_user"])

   if definition.CRUD == nil {
      t.Fatal("expected user CRUD metadata")
   }
   if definition.CRUD.APIs[0] != crud.APIAdd {
      t.Fatalf("expected first API add, got %s", definition.CRUD.APIs[0])
   }
   if definition.CRUD.HiddenFields[0] != "password" {
      t.Fatalf("expected password hidden field, got %#v", definition.CRUD.HiddenFields)
   }
}

/**
 * 模型映射
 * @param definitions 模型定义列表
 * @returns map[string]model.Definition
 */
func modelMap(definitions []model.Definition) map[string]model.Definition {
   items := map[string]model.Definition{}
   for _, definition := range definitions {
      items[definition.TableName] = definition
   }
   return items
}
```

Add import for `cool/model` if test helper uses it:

```go
"github.com/toothdy/cool-admin-go-next/cool/model"
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/controller -run 'TestControllersDeclareBaseResources|TestUserControllerCRUDMetadata' -count=1
```

Expected: FAIL with package missing.

- [ ] **Step 3: Implement user controller**

Create `modules/base/controller/admin/sys_user.go`:

```go
package admin

import (
   coolController "github.com/toothdy/cool-admin-go-next/cool/controller"
   "github.com/toothdy/cool-admin-go-next/cool/crud"
   coolModel "github.com/toothdy/cool-admin-go-next/cool/model"
   baseService "github.com/toothdy/cool-admin-go-next/modules/base/service"
)

/**
 * 系统用户 controller 元数据
 * @param userService 用户服务
 * @param userModel 用户模型
 * @returns controller.Definition
 */
func UserController(userService *baseService.UserService, userModel coolModel.Definition) coolController.Definition {
   return coolController.Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("系统用户").
      Model(userModel).
      Service(userService).
      CRUD(coolController.CRUDOptions{
         APIs: []string{crud.APIAdd, crud.APIDelete, crud.APIUpdate, crud.APIInfo, crud.APIList, crud.APIPage},
         PageQuery: coolController.QueryOptions{
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

- [ ] **Step 4: Implement remaining admin controllers**

Use the existing `modules/base/routes.go` specs as the source of truth.

`RoleController` uses:

```go
CRUDOptions{
   APIs: []string{crud.APIAdd, crud.APIDelete, crud.APIUpdate, crud.APIInfo, crud.APIList, crud.APIPage},
   PageQuery: coolController.QueryOptions{KeyWordLikeFields: []string{"name", "label"}},
   SortFields: []string{"id", "createTime", "updateTime", "name"},
   DefaultSort: "id",
   DefaultOrder: "DESC",
}
```

`MenuController` uses:

```go
CRUDOptions{
   APIs: []string{crud.APIAdd, crud.APIDelete, crud.APIUpdate, crud.APIInfo, crud.APIList, crud.APIPage},
   PageQuery: coolController.QueryOptions{
      KeyWordLikeFields: []string{"name", "perms"},
      FieldEq: []string{"parentId", "type", "isShow"},
   },
   SortFields: []string{"id", "orderNum", "createTime", "updateTime"},
   DefaultSort: "orderNum",
   DefaultOrder: "ASC",
}
```

`DepartmentController` uses:

```go
CRUDOptions{
   APIs: []string{crud.APIAdd, crud.APIDelete, crud.APIUpdate, crud.APIList},
   PageQuery: coolController.QueryOptions{
      KeyWordLikeFields: []string{"name"},
      FieldEq: []string{"parentId"},
   },
   SortFields: []string{"id", "orderNum", "createTime", "updateTime"},
   DefaultSort: "orderNum",
   DefaultOrder: "ASC",
}
```

`ParamController` uses:

```go
CRUDOptions{
   APIs: []string{crud.APIAdd, crud.APIDelete, crud.APIUpdate, crud.APIInfo, crud.APIPage},
   PageQuery: coolController.QueryOptions{KeyWordLikeFields: []string{"keyName", "name"}},
   SortFields: []string{"id", "createTime", "updateTime", "keyName"},
   DefaultSort: "id",
   DefaultOrder: "DESC",
}
```

`LogController` uses:

```go
CRUDOptions{
   APIs: []string{crud.APIPage},
   PageQuery: coolController.QueryOptions{
      KeyWordLikeFields: []string{"action", "ip"},
      FieldEq: []string{"userId"},
   },
   SortFields: []string{"id", "createTime", "updateTime", "userId"},
   DefaultSort: "id",
   DefaultOrder: "DESC",
}
```

- [ ] **Step 5: Implement open and comm controllers**

`modules/base/controller/open/open.go` must declare login, refreshToken, and captcha as `IgnoreAuth`.

`modules/base/controller/comm/comm.go` must declare person, permmenu, logout, and program. Only `program` has `IgnoreAuth: true`.

Use route handlers that accept `*ghttp.Request` initially if typed service method binding would be ambiguous. The handler can call existing service methods and write response directly. This keeps behavior stable for auth routes.

- [ ] **Step 6: Implement controller aggregator**

Create `modules/base/controller/controllers.go`:

```go
package controller

import (
   "github.com/gogf/gf/v2/database/gdb"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   coolController "github.com/toothdy/cool-admin-go-next/cool/controller"
   baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
   adminController "github.com/toothdy/cool-admin-go-next/modules/base/controller/admin"
   commController "github.com/toothdy/cool-admin-go-next/modules/base/controller/comm"
   openController "github.com/toothdy/cool-admin-go-next/modules/base/controller/open"
   baseService "github.com/toothdy/cool-admin-go-next/modules/base/service"
)

/**
 * base controller 元数据列表
 * @param db 数据库实例
 * @param manager 认证管理器
 * @returns []controller.Definition
 */
func Controllers(db gdb.DB, manager *auth.Manager) []coolController.Definition {
   models := modelMap(baseModel.Register())
   authService := baseService.NewAuthService(db, manager)
   permissionService := baseService.NewPermissionService(db)

   userService := baseService.NewUserService(db, models["base_sys_user"])
   roleService := baseService.NewRoleService(db, models["base_sys_role"])
   menuService := baseService.NewMenuService(db, models["base_sys_menu"])
   departmentService := baseService.NewDepartmentService(db, models["base_sys_department"])
   paramService := baseService.NewParamService(db, models["base_sys_param"])
   logService := baseService.NewLogService(db, models["base_sys_log"])

   return []coolController.Definition{
      adminController.UserController(userService, models["base_sys_user"]),
      adminController.RoleController(roleService, models["base_sys_role"]),
      adminController.MenuController(menuService, models["base_sys_menu"]),
      adminController.DepartmentController(departmentService, models["base_sys_department"]),
      adminController.ParamController(paramService, models["base_sys_param"]),
      adminController.LogController(logService, models["base_sys_log"]),
      openController.OpenController(authService, paramService),
      commController.CommController(authService, permissionService),
   }
}
```

Add `modelMap` helper in same file.

- [ ] **Step 7: Run focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/controller/**/*.go
go test ./modules/base/controller -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add modules/base/controller/admin/sys_user.go modules/base/controller/admin/sys_role.go modules/base/controller/admin/sys_menu.go modules/base/controller/admin/sys_department.go modules/base/controller/admin/sys_param.go modules/base/controller/admin/sys_log.go modules/base/controller/open/open.go modules/base/controller/comm/comm.go modules/base/controller/controllers.go modules/base/controller/controllers_test.go
git commit -m $'feat: declare base controller metadata\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 9: Wire metadata-driven registration into base module and app

**Files:**
- Modify: `modules/base/base.go`
- Modify: `modules/base/routes.go`
- Modify: `modules/base/auth_routes.go`
- Modify: `modules/base/permission_routes.go`
- Modify: `modules/routes.go`
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Modify: `modules/base/routes_test.go`
- Modify: `modules/base/permission_test.go`
- Modify: `modules/base/permission_routes_test.go`

**Interfaces:**
- Consumes: `module.CollectControllers`, `controller.IgnoreAuthPaths`, `controller.PermissionMap`, `controller.CRUDResourceSpecs`, `controller.RegisterPermissionMiddleware`, `controller.RegisterRoutes`.
- Produces: app startup does not call base-specific route registration functions.

- [ ] **Step 1: Update base module to provide controllers**

Modify `modules/base/base.go`:

```go
func NewModule() module.Module {
   return module.New("base").
      Name("基础模块").
      Config(NewConfig()).
      Models(baseModel.Register()).
      Controllers(baseController.Controllers(g.DB(), nil)).
      Seeds("modules/base/db.json", "modules/base/menu.json")
}
```

If `auth.Manager` is required at app startup, do not construct controllers in `NewModule`. Instead add `Controllers(db, manager)` collection in app after manager creation and keep `NewModule` model/seed only. Prefer app-time construction to avoid nil manager in auth service.

Final app-time approach:

```go
registeredControllers := baseController.Controllers(g.DB(), application.authManager)
application.modules = attachBaseControllers(registeredModules, registeredControllers)
```

- [ ] **Step 2: Update app registration flow**

Modify `cool/app/app.go` `registerRoutes()` to:

```go
func (a *Application) registerRoutes() {
   a.server.BindHandler("/health", func(r *ghttp.Request) {
      r.Response.WriteJson(a.Health(r.Context()))
   })

   controllers := module.CollectControllers(a.modules)
   ignorePaths := append([]string{"/health"}, controller.IgnoreAuthPaths(controllers)...)
   a.server.Use(auth.NewMiddleware(auth.MiddlewareOptions{
      Manager:     a.authManager,
      IgnorePaths: ignorePaths,
   }))

   permissionService := baseModule.NewPermissionService(g.DB())
   permissions, err := controller.PermissionMap(controllers)
   if err != nil {
      g.Log().Fatal(context.Background(), err)
   }
   controller.RegisterPermissionMiddleware(a.server, permissionService, permissions)

   specs, err := controller.CRUDResourceSpecs(controllers)
   if err != nil {
      g.Log().Fatal(context.Background(), err)
   }
   registry, err := crud.NewRegistry(specs)
   if err != nil {
      g.Log().Fatal(context.Background(), err)
   }
   runtime := crud.NewRuntime(g.DB(), registry)
   if err = controller.RegisterRoutes(a.server, runtime, controllers); err != nil {
      g.Log().Fatal(context.Background(), err)
   }
}
```

Add imports:

```go
coolController "github.com/toothdy/cool-admin-go-next/cool/controller"
baseController "github.com/toothdy/cool-admin-go-next/modules/base/controller"
```

Use aliases if `controller` conflicts with package names.

- [ ] **Step 3: Ensure base controllers are attached with auth manager**

Because `modules.Register()` currently calls `base.NewModule()` before app has `authManager`, add an app-level method:

```go
/**
 * 绑定运行时 controller 元数据
 * @returns null
 */
func (a *Application) bindRuntimeControllers() {
   baseControllers := baseController.Controllers(g.DB(), a.authManager)
   for index, mod := range a.modules {
      if mod.Key() != "base" {
         continue
      }
      definition, ok := mod.(*module.Definition)
      if ok {
         a.modules[index] = definition.Controllers(baseControllers)
      }
   }
}
```

Call it in `NewWithContext` after `application` is created and before `registerRoutes()`:

```go
application.bindRuntimeControllers()
```

- [ ] **Step 4: Replace module route helpers**

Modify `modules/routes.go` so helpers delegate to collected controllers or remove if unused. If kept for tests:

```go
func CRUDResourceSpecs(definitions []model.Definition) ([]crud.ResourceSpec, error) {
   return base.CRUDResourceSpecs(definitions)
}
```

Then update tests to prefer `controller.CRUDResourceSpecs(module.CollectControllers(app.Modules()))`.

- [ ] **Step 5: Retire hand-written base route files**

Once app uses controller runtime, delete these files if no tests depend on them:

```bash
rm modules/base/auth_routes.go modules/base/permission_routes.go modules/base/routes.go
```

If tests still need public compatibility functions, keep files but change comments to mark them compatibility shims and make them delegate to `cool/controller` functions. Do not call them from `cool/app`.

- [ ] **Step 6: Run focused app and base tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/app/app.go cool/app/app_test.go modules/base/base.go modules/routes.go modules/base/*.go modules/base/*_test.go
go test ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 7: Run auth and permission focused tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./cool/app ./modules/base -run 'TestAuth|TestPermission|TestCRUDResourceSpecs|TestRegister' -count=1
```

Expected: PASS or no matching tests in packages that do not define those names.

- [ ] **Step 8: Commit**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add cool/app/app.go cool/app/app_test.go modules/base/base.go modules/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/routes.go modules/base/routes_test.go modules/base/permission_test.go modules/base/permission_routes_test.go
git commit -m $'refactor: register routes from controller metadata\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

If route files were deleted, stage deletions explicitly:

```bash
git add cool/app/app.go cool/app/app_test.go modules/base/base.go modules/routes.go modules/base/routes_test.go modules/base/permission_test.go modules/base/permission_routes_test.go
git rm modules/base/auth_routes.go modules/base/permission_routes.go modules/base/routes.go
git commit -m $'refactor: register routes from controller metadata\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

### Task 10: Full verification and cleanup

**Files:**
- Modify as needed: files touched by prior tasks only.
- Test: all Go packages and permission integration test.

**Interfaces:**
- Consumes: all prior task outputs.
- Produces: verified CoolController runtime implementation.

- [ ] **Step 1: Run focused package tests from spec**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/controller ./cool/service ./cool/crud ./cool/module ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 2: Run app/auth/base tests**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full test suite**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run permission integration test**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify forbidden directories were not created**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
find . -type d \( -path './dao' -o -path './internal/model/do' -o -path './internal/model/entity' -o -path './logic' \) -print
```

Expected: no output.

- [ ] **Step 6: Inspect diff for unrelated files**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git diff --stat
```

Expected: only files listed in this plan are modified.

- [ ] **Step 7: Commit verification cleanup if any files changed**

If no files changed after tests, skip commit. If cleanup changed files, stage them explicitly:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add path/to/changed-file.go path/to/changed-test.go
git commit -m $'test: verify cool controller runtime\n\nCo-Authored-By: Claude Fable 5 <noreply@anthropic.com>'
```

---

## Self-Review

**Spec coverage:**

- `cool/controller` builder, definition, route, derivation, and registration: Tasks 2, 3, 5.
- `cool/service` BaseService and hooks: Task 1.
- `cool/crud` service override and hooks: Task 4.
- `cool/module` controllers: Task 6.
- `modules/base` controller/service/model structure: Tasks 7 and 8.
- app metadata-driven registration: Task 9.
- tests and verification commands: Tasks 1-10.
- forbidden directories constraint: Task 10.

**Placeholder scan:** No unresolved placeholders or unspecified implementation placeholders are intentionally present. Each task includes file paths, interfaces, commands, and expected results.

**Type consistency:** The plan consistently uses `controller.Definition`, `crud.ResourceSpec.Service`, `crud.ResourceSpec.InsertParam`, `crud.AddRequest`, `crud.QueryRequest`, `service.BaseService`, and `module.CollectControllers` across tasks.
