# cool-admin-go-next Permmenu Permission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现阶段 5B 权限菜单、权限码和 base CRUD 权限校验，让登录用户能加载菜单，并让基础 CRUD 按权限码拦截。

**Architecture:** 权限逻辑先放在 `modules/base`，不提前抽象通用 `cool/permission`。`PermissionService` 负责查询菜单、权限码和权限判断，`permission_routes.go` 注册 `permmenu` 与 CRUD 权限 middleware。app 注册顺序为 `/health`、auth routes、auth middleware、permission routes、permission middleware、CRUD routes。

**Tech Stack:** Go 1.23+、GoFrame v2.10.2、GoFrame gdb/ghttp/gerror、MySQL 8.x、标准库 `context` / `strings` / `sort` / `net/http` / `encoding/json`。

## Global Constraints

- 始终用中文编写说明文档和代码注释。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL。
- 阶段 5B 包含 `GET /admin/base/comm/permmenu` 和 base CRUD 权限校验。
- 阶段 5B 不实现 EPS runtime、Vue 真实联调、文件上传、`personUpdate`、数据权限或多模块权限框架。
- 前端请求 header 使用 `Authorization: <token>`，Go 版不能只支持 `Bearer <token>`。
- 缺 CRUD 权限返回 HTTP `403` 和精确 JSON `{ "code": 1001, "message": "权限不足~" }`。
- 内部 DB 错误不暴露给前端。
- 不使用 `git add -A`；提交时只显式 stage 本计划创建或修改的文件。
- GoFrame 自动生成文件后续必须由工具生成，不手写、不手改；本计划不创建 `dao/`、`internal/model/do/`、`internal/model/entity/`。
- 不使用 `logic/` 目录，业务逻辑直接放在 `modules/base`。
- Go 代码错误处理使用 GoFrame `gerror` 包装上下文。
- Go 文件内如果有 3 个及以上相关变量声明，使用 `var (...)` 分组。
- 为控制主 agent 上下文，实施阶段每个 Task 使用 fresh subagent，子代理只返回摘要、测试结果和关键 diff 说明。

---

## Scope Check

本计划只覆盖设计文档 `docs/superpowers/specs/2026-07-16-cool-admin-go-next-permmenu-permission-design.md` 的阶段 5B。

包含：

1. base permission service。
2. `GET /admin/base/comm/permmenu`。
3. CRUD 权限映射和权限 middleware。
4. admin 全权限和普通用户角色权限。
5. HTTP 集成测试。
6. README 更新和最终验证。

不包含：

1. EPS runtime。
2. Vue 真实联调。
3. 文件上传。
4. `personUpdate`。
5. 数据权限 / 部门范围权限。
6. 操作日志增强。
7. 多模块通用权限框架。

---

## File Structure

### 创建文件

- `modules/base/permission.go`  
  权限服务：permmenu 查询、perms 拆分、admin 判断、权限判断、CRUD 权限映射。

- `modules/base/permission_test.go`  
  测试 perms 拆分去重、CRUD 权限映射、菜单过滤 helper。

- `modules/base/permission_routes.go`  
  注册 permmenu 路由和 CRUD 权限 middleware。

- `modules/base/permission_routes_test.go`  
  测试 403 JSON 响应和 middleware 路由匹配。

- `modules/base/permission_integration_test.go`  
  `COOL_PERMISSION_INTEGRATION=1` 时执行真实 MySQL HTTP 权限集成测试。

### 修改文件

- `cool/app/app.go`  
  接入 permission routes 和 permission middleware，调整注册顺序。

- `cool/app/app_test.go`  
  测试权限注册不破坏 auth/schema/seed 基本行为。

- `README.md`  
  更新当前阶段和权限验收说明。

### 不创建文件

- 不创建 `dao/`。
- 不创建 `internal/model/do/`。
- 不创建 `internal/model/entity/`。
- 不创建 `logic/`。
- 不创建 `cool/permission/`。

---

## Implementation Tasks

### Task 1: 增加 base permission service 基础工具

**Files:**
- Create: `modules/base/permission.go`
- Create: `modules/base/permission_test.go`
- Test: `go test ./modules/base -run 'Test(SplitPerms|CRUDPermissionMap|UniquePerms)' -count=1`

**Interfaces:**
- Consumes:
  - `auth.UserContext`
- Produces:
  - `type PermissionService struct`
  - `type PermMenuResult struct`
  - `func NewPermissionService(db gdb.DB) *PermissionService`
  - `func SplitPerms(value string) []string`
  - `func UniqueStrings(items []string) []string`
  - `func CRUDPermissionMap() map[string]string`

- [ ] **Step 1: Write permission unit tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission_test.go`:

```go
package base_test

import (
   "reflect"
   "testing"

   baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
)

func TestSplitPerms(t *testing.T) {
   perms := baseModule.SplitPerms("base:sys:user:page, base:sys:user:list,,base:sys:user:info")
   expected := []string{"base:sys:user:page", "base:sys:user:list", "base:sys:user:info"}
   if !reflect.DeepEqual(perms, expected) {
      t.Fatalf("expected %#v, got %#v", expected, perms)
   }
}

func TestUniqueStringsKeepsOrder(t *testing.T) {
   items := baseModule.UniqueStrings([]string{"a", "b", "a", "c", "b"})
   expected := []string{"a", "b", "c"}
   if !reflect.DeepEqual(items, expected) {
      t.Fatalf("expected %#v, got %#v", expected, items)
   }
}

func TestCRUDPermissionMap(t *testing.T) {
   permissions := baseModule.CRUDPermissionMap()
   checks := map[string]string{
      "POST:/admin/base/sys/user/page":   "base:sys:user:page",
      "POST:/admin/base/sys/user/add":    "base:sys:user:add",
      "POST:/admin/base/sys/user/update": "base:sys:user:update",
      "POST:/admin/base/sys/user/delete": "base:sys:user:delete",
      "GET:/admin/base/sys/user/info":    "base:sys:user:info",
      "POST:/admin/base/sys/role/page":   "base:sys:role:page",
      "POST:/admin/base/sys/menu/page":   "base:sys:menu:page",
      "POST:/admin/base/sys/param/page":  "base:sys:param:page",
      "POST:/admin/base/sys/log/page":    "base:sys:log:page",
   }
   for route, permission := range checks {
      if permissions[route] != permission {
         t.Fatalf("expected %s => %s, got %s", route, permission, permissions[route])
      }
   }
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(SplitPerms|CRUDPermissionMap|UniquePerms)' -count=1
```

Expected: FAIL because permission helpers do not exist.

- [ ] **Step 3: Implement permission base file**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission.go`:

```go
package base

import (
   "strings"

   "github.com/gogf/gf/v2/database/gdb"
)

// PermissionService 是 base 权限服务。
type PermissionService struct {
   DB gdb.DB
}

// PermMenuResult 是权限菜单响应 data。
type PermMenuResult struct {
   Menus []map[string]interface{} `json:"menus"`
   Perms []string                 `json:"perms"`
}

/**
 * 创建权限服务
 * @param db 数据库实例
 * @returns *PermissionService
 */
func NewPermissionService(db gdb.DB) *PermissionService {
   return &PermissionService{DB: db}
}

/**
 * 拆分权限字符串
 * @param value 权限字符串
 * @returns []string
 */
func SplitPerms(value string) []string {
   parts := strings.Split(value, ",")
   perms := make([]string, 0, len(parts))
   for _, part := range parts {
      item := strings.TrimSpace(part)
      if item != "" {
         perms = append(perms, item)
      }
   }
   return perms
}

/**
 * 字符串去重并保持顺序
 * @param items 字符串列表
 * @returns []string
 */
func UniqueStrings(items []string) []string {
   seen := map[string]bool{}
   result := make([]string, 0, len(items))
   for _, item := range items {
      if item == "" || seen[item] {
         continue
      }
      seen[item] = true
      result = append(result, item)
   }
   return result
}

/**
 * base CRUD 路由权限映射
 * @returns map[string]string
 */
func CRUDPermissionMap() map[string]string {
   return map[string]string{
      "POST:/admin/base/sys/user/add":          "base:sys:user:add",
      "POST:/admin/base/sys/user/delete":       "base:sys:user:delete",
      "POST:/admin/base/sys/user/update":       "base:sys:user:update",
      "GET:/admin/base/sys/user/info":          "base:sys:user:info",
      "POST:/admin/base/sys/user/list":         "base:sys:user:list",
      "POST:/admin/base/sys/user/page":         "base:sys:user:page",
      "POST:/admin/base/sys/role/add":          "base:sys:role:add",
      "POST:/admin/base/sys/role/delete":       "base:sys:role:delete",
      "POST:/admin/base/sys/role/update":       "base:sys:role:update",
      "GET:/admin/base/sys/role/info":          "base:sys:role:info",
      "POST:/admin/base/sys/role/list":         "base:sys:role:list",
      "POST:/admin/base/sys/role/page":         "base:sys:role:page",
      "POST:/admin/base/sys/menu/add":          "base:sys:menu:add",
      "POST:/admin/base/sys/menu/delete":       "base:sys:menu:delete",
      "POST:/admin/base/sys/menu/update":       "base:sys:menu:update",
      "GET:/admin/base/sys/menu/info":          "base:sys:menu:info",
      "POST:/admin/base/sys/menu/list":         "base:sys:menu:list",
      "POST:/admin/base/sys/menu/page":         "base:sys:menu:page",
      "POST:/admin/base/sys/department/add":    "base:sys:department:add",
      "POST:/admin/base/sys/department/delete": "base:sys:department:delete",
      "POST:/admin/base/sys/department/update": "base:sys:department:update",
      "POST:/admin/base/sys/department/list":   "base:sys:department:list",
      "POST:/admin/base/sys/param/add":         "base:sys:param:add",
      "POST:/admin/base/sys/param/delete":      "base:sys:param:delete",
      "POST:/admin/base/sys/param/update":      "base:sys:param:update",
      "GET:/admin/base/sys/param/info":         "base:sys:param:info",
      "POST:/admin/base/sys/param/page":        "base:sys:param:page",
      "POST:/admin/base/sys/log/page":          "base:sys:log:page",
   }
}
```

- [ ] **Step 4: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(SplitPerms|CRUDPermissionMap|UniquePerms)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 1**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/permission.go modules/base/permission_test.go
git commit -m "feat: add base permission helpers" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 实现 permmenu 查询和权限判断

**Files:**
- Modify: `modules/base/permission.go`
- Modify: `modules/base/permission_test.go`
- Test: `go test ./modules/base -run 'Test(PermMenu|HasPermission|SplitPerms)' -count=1`

**Interfaces:**
- Consumes:
  - `type PermissionService struct`
  - `type PermMenuResult struct`
  - `auth.UserContext`
  - `SplitPerms(value string) []string`
  - `UniqueStrings(items []string) []string`
- Produces:
  - `func (s *PermissionService) PermMenu(ctx context.Context, user auth.UserContext) (PermMenuResult, error)`
  - `func (s *PermissionService) HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error)`
  - `func (s *PermissionService) IsAdmin(ctx context.Context, user auth.UserContext) (bool, error)`

- [ ] **Step 1: Add unit helper tests**

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission_test.go`:

```go
func TestMenuRowShouldEnterMenus(t *testing.T) {
   visiblePage := map[string]interface{}{"type": int64(1), "isShow": int64(1)}
   if !baseModule.MenuRowShouldEnterMenus(visiblePage) {
      t.Fatal("expected visible page to enter menus")
   }

   button := map[string]interface{}{"type": int64(2), "isShow": int64(1)}
   if baseModule.MenuRowShouldEnterMenus(button) {
      t.Fatal("expected button not to enter menus")
   }

   hidden := map[string]interface{}{"type": int64(1), "isShow": int64(0)}
   if baseModule.MenuRowShouldEnterMenus(hidden) {
      t.Fatal("expected hidden menu not to enter menus")
   }
}
```

- [ ] **Step 2: Implement query methods**

Modify `modules/base/permission.go` and add imports:

```go
import (
   "context"
   "fmt"
   "strings"

   "github.com/gogf/gf/v2/database/gdb"
   "github.com/gogf/gf/v2/errors/gerror"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
)
```

Add methods and helpers:

```go
/**
 * 当前用户是否超管
 * @param ctx 上下文
 * @param user 当前用户
 * @returns bool
 */
func (s *PermissionService) IsAdmin(ctx context.Context, user auth.UserContext) (bool, error) {
   if user.Username == "admin" {
      return true, nil
   }
   count, err := s.DB.GetCount(ctx, "SELECT COUNT(*) FROM base_sys_user_role ur INNER JOIN base_sys_role r ON r.id = ur.role_id WHERE ur.user_id = ? AND r.label = ?", user.UserId, "admin")
   if err != nil {
      return false, gerror.Wrap(err, "查询用户超管角色失败")
   }
   return count > 0, nil
}

/**
 * 查询权限菜单
 * @param ctx 上下文
 * @param user 当前用户
 * @returns PermMenuResult
 */
func (s *PermissionService) PermMenu(ctx context.Context, user auth.UserContext) (PermMenuResult, error) {
   isAdmin, err := s.IsAdmin(ctx, user)
   if err != nil {
      return PermMenuResult{}, err
   }

   var rows gdb.Result
   if isAdmin {
      rows, err = s.allMenuRows(ctx)
   } else {
      rows, err = s.userMenuRows(ctx, user.UserId)
   }
   if err != nil {
      return PermMenuResult{}, err
   }

   menus := make([]map[string]interface{}, 0, len(rows))
   perms := make([]string, 0)
   for _, row := range rows {
      values := permissionRecordValues(row)
      perms = append(perms, SplitPerms(stringValue(values["perms"]))...)
      if MenuRowShouldEnterMenus(values) {
         menus = append(menus, values)
      }
   }

   return PermMenuResult{
      Menus: menus,
      Perms: UniqueStrings(perms),
   }, nil
}

/**
 * 是否拥有权限码
 * @param ctx 上下文
 * @param user 当前用户
 * @param permission 权限码
 * @returns bool
 */
func (s *PermissionService) HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error) {
   if permission == "" {
      return true, nil
   }
   result, err := s.PermMenu(ctx, user)
   if err != nil {
      return false, err
   }
   for _, item := range result.Perms {
      if item == permission {
         return true, nil
      }
   }
   return false, nil
}

/**
 * 菜单行是否进入 menus
 * @param row 菜单行
 * @returns bool
 */
func MenuRowShouldEnterMenus(row map[string]interface{}) bool {
   return int64Value(row["type"]) != 2 && int64Value(row["isShow"]) == 1
}
```

Also add private query helpers using SQL aliases exactly:

```go
func (s *PermissionService) allMenuRows(ctx context.Context) (gdb.Result, error) {
   result, err := s.DB.GetAll(ctx, menuSelectSQL()+" WHERE is_show = 1 ORDER BY order_num ASC, id ASC")
   if err != nil {
      return nil, gerror.Wrap(err, "查询全部菜单权限失败")
   }
   return result, nil
}

func (s *PermissionService) userMenuRows(ctx context.Context, userId int64) (gdb.Result, error) {
   result, err := s.DB.GetAll(ctx, menuSelectSQL()+" WHERE id IN (SELECT menu_id FROM base_sys_role_menu WHERE role_id IN (SELECT role_id FROM base_sys_user_role WHERE user_id = ?)) ORDER BY order_num ASC, id ASC", userId)
   if err != nil {
      return nil, gerror.Wrap(err, "查询用户菜单权限失败")
   }
   return result, nil
}

func menuSelectSQL() string {
   return "SELECT id, parent_id AS parentId, name, router, perms, type, icon, order_num AS orderNum, view_path AS viewPath, keep_alive AS keepAlive, is_show AS isShow, create_time AS createTime, update_time AS updateTime FROM base_sys_menu"
}

func permissionRecordValues(record gdb.Record) map[string]interface{} {
   values := make(map[string]interface{}, len(record))
   for key, value := range record {
      values[key] = value.Val()
   }
   return values
}
```

If `stringValue` and `int64Value` already exist in `auth.go` as package-private helpers, reuse them instead of duplicating.

- [ ] **Step 3: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(PermMenu|HasPermission|SplitPerms|MenuRow)' -count=1
```

Expected: PASS.

- [ ] **Step 4: Run base tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 2**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/permission.go modules/base/permission_test.go
git commit -m "feat: query base permissions" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 注册 permmenu 路由和 CRUD 权限 middleware

**Files:**
- Create: `modules/base/permission_routes.go`
- Create: `modules/base/permission_routes_test.go`
- Test: `go test ./modules/base -run 'Test(WriteForbidden|PermissionRoute|PermissionMiddleware)' -count=1`

**Interfaces:**
- Consumes:
  - `PermissionService.PermMenu(ctx, user)`
  - `PermissionService.HasPermission(ctx, user, permission)`
  - `CRUDPermissionMap() map[string]string`
  - `auth.UserFromContext(ctx)`
- Produces:
  - `func RegisterPermissionRoutes(server *ghttp.Server, service *PermissionService)`
  - `func RegisterPermissionMiddleware(server *ghttp.Server, service *PermissionService)`
  - `func WriteForbidden(r *ghttp.Request)`
  - `func RoutePermission(method string, path string) (string, bool)`

- [ ] **Step 1: Write route tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission_routes_test.go`:

```go
package base_test

import (
   "net/http"
   "testing"

   "github.com/gogf/gf/v2/net/ghttp"
   baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
)

func TestRoutePermission(t *testing.T) {
   permission, ok := baseModule.RoutePermission("POST", "/admin/base/sys/user/page")
   if !ok {
      t.Fatal("expected route permission")
   }
   if permission != "base:sys:user:page" {
      t.Fatalf("unexpected permission: %s", permission)
   }

   if _, ok = baseModule.RoutePermission("GET", "/admin/base/comm/permmenu"); ok {
      t.Fatal("expected permmenu not to require CRUD permission")
   }
}

func TestWriteForbidden(t *testing.T) {
   server := ghttp.GetServer("permission-forbidden-test")
   server.BindHandler("/forbidden", func(r *ghttp.Request) {
      baseModule.WriteForbidden(r)
   })
   server.SetPort(0)
   server.Start()
   defer server.Shutdown()

   client := server.GetClient()
   response, err := client.Get("/forbidden")
   if err != nil {
      t.Fatalf("request failed: %v", err)
   }
   defer response.Close()

   if response.StatusCode != http.StatusForbidden {
      t.Fatalf("expected 403, got %d", response.StatusCode)
   }
   body := response.ReadAllString()
   if body != `{"code":1001,"message":"权限不足~"}` {
      t.Fatalf("unexpected body: %s", body)
   }
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(RoutePermission|WriteForbidden)' -count=1
```

Expected: FAIL because permission route helpers do not exist.

- [ ] **Step 3: Implement permission routes**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission_routes.go`:

```go
package base

import (
   "net/http"
   "strings"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

const permissionDeniedMessage = "权限不足~"

/**
 * 注册权限菜单路由
 * @param server HTTP 服务
 * @param service 权限服务
 * @returns null
 */
func RegisterPermissionRoutes(server *ghttp.Server, service *PermissionService) {
   server.BindHandler("GET:/admin/base/comm/permmenu", func(r *ghttp.Request) {
      user, ok := auth.UserFromContext(r.Context())
      if !ok {
         auth.Unauthorized(r)
         return
      }
      data, err := service.PermMenu(r.Context(), user)
      if err != nil {
         g.Log().Error(r.Context(), err)
         WriteForbidden(r)
         return
      }
      r.Response.WriteJson(response.OK(data))
   })
}

/**
 * 注册 CRUD 权限 middleware
 * @param server HTTP 服务
 * @param service 权限服务
 * @returns null
 */
func RegisterPermissionMiddleware(server *ghttp.Server, service *PermissionService) {
   server.Use(func(r *ghttp.Request) {
      permission, ok := RoutePermission(r.Method, r.URL.Path)
      if !ok {
         r.Middleware.Next()
         return
      }
      user, hasUser := auth.UserFromContext(r.Context())
      if !hasUser {
         auth.Unauthorized(r)
         return
      }
      allowed, err := service.HasPermission(r.Context(), user, permission)
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
 * 路由对应权限码
 * @param method HTTP 方法
 * @param path 路径
 * @returns 权限码和是否存在
 */
func RoutePermission(method string, path string) (string, bool) {
   key := strings.ToUpper(method) + ":" + path
   permission, ok := CRUDPermissionMap()[key]
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

- [ ] **Step 4: Run route tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(RoutePermission|WriteForbidden)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run base tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/permission_routes.go modules/base/permission_routes_test.go
git commit -m "feat: add permission routes" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 接入 app 权限路由和 middleware

**Files:**
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Test: `go test ./cool/app ./modules/base -count=1`

**Interfaces:**
- Consumes:
  - `base.NewPermissionService(g.DB())`
  - `base.RegisterPermissionRoutes(server, service)`
  - `base.RegisterPermissionMiddleware(server, service)`
- Produces: app route order `/health` -> auth routes -> auth middleware -> permission routes -> permission middleware -> CRUD routes.

- [ ] **Step 1: Add app route-order test**

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app_test.go`:

```go
func TestNewWithServerRegistersPermissionLayer(t *testing.T) {
   application := app.New(app.Options{StartServer: true})
   if application == nil {
      t.Fatal("expected application")
   }
}
```

This is a smoke test because GoFrame route internals are not stable API; HTTP behavior is covered by integration tests.

- [ ] **Step 2: Modify app route registration**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app.go` in `registerRoutes()`:

After base auth route registration and auth middleware, create permission service and register permission route/middleware before CRUD routes:

```go
permissionService := baseModule.NewPermissionService(g.DB())
baseModule.RegisterPermissionRoutes(a.server, permissionService)
baseModule.RegisterPermissionMiddleware(a.server, permissionService)
```

Expected final order inside `registerRoutes()`:

```go
a.server.BindHandler("/health", ...)

authService := baseModule.NewAuthService(g.DB(), a.authManager)
baseModule.RegisterAuthRoutes(a.server, authService)
a.server.Use(auth.NewMiddleware(...))

permissionService := baseModule.NewPermissionService(g.DB())
baseModule.RegisterPermissionRoutes(a.server, permissionService)
baseModule.RegisterPermissionMiddleware(a.server, permissionService)

specs, err := modules.CRUDResourceSpecs(a.Models())
...
```

- [ ] **Step 3: Run app/base tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 4: Run related tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 4**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go
git commit -m "feat: wire permission middleware" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 增加权限 HTTP 集成测试

**Files:**
- Create: `modules/base/permission_integration_test.go`
- Test: `go test ./modules/base -run PermissionIntegration -count=1`
- Integration Test: `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1`

**Interfaces:**
- Consumes:
  - app HTTP route stack.
  - schema sync and seed importer.
  - auth login HTTP routes.
  - permmenu route.
  - CRUD permission middleware.
- Produces: real MySQL HTTP permission integration coverage.

- [ ] **Step 1: Write skipped integration test skeleton**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/permission_integration_test.go` with:

```go
package base_test

import (
   "bytes"
   "context"
   "encoding/json"
   "net/http"
   "os"
   "testing"

   _ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/app"
   "github.com/toothdy/cool-admin-go-next/cool/db/schema"
   "github.com/toothdy/cool-admin-go-next/cool/seed"
   baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
)

func TestPermissionIntegrationPermmenuAndCRUDPermission(t *testing.T) {
   if os.Getenv("COOL_PERMISSION_INTEGRATION") != "1" {
      t.Skip("set COOL_PERMISSION_INTEGRATION=1 to run real MySQL permission integration test")
   }

   ctx := context.Background()
   definitions := baseModel.Register()
   if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
      t.Fatalf("schema sync failed: %v", err)
   }
   cleanupAuthSeedData(t, ctx)
   importer := seed.NewImporter(g.DB(), definitions)
   repoRoot := repositoryRoot(t)
   if _, err := importer.ImportDB(ctx, "base", repoRoot+"/modules/base/db.json"); err != nil {
      t.Fatalf("import db seed failed: %v", err)
   }
   if _, err := importer.ImportMenu(ctx, "base", repoRoot+"/modules/base/menu.json"); err != nil {
      t.Fatalf("import menu seed failed: %v", err)
   }

   server := ghttp.GetServer("permission-integration-test")
   server.SetPort(0)
   application := app.New(app.Options{StartServer: true})
   if application == nil {
      t.Fatal("expected application")
   }
   server = g.Server()
   server.SetPort(0)
   server.Start()
   defer server.Shutdown()

   baseURL := "http://127.0.0.1:" + server.GetListenedPort()
   token := loginForPermissionTest(t, baseURL, "admin", "123456")

   permmenu := getJSON(t, baseURL+"/admin/base/comm/permmenu", token)
   data := permmenu["data"].(map[string]interface{})
   menus := data["menus"].([]interface{})
   perms := data["perms"].([]interface{})
   if len(menus) == 0 || len(perms) == 0 {
      t.Fatalf("expected menus and perms, got %#v", data)
   }
   assertStringSliceContains(t, perms, "base:sys:user:page")
   assertStringSliceContains(t, perms, "base:sys:user:add")

   response := postJSON(t, baseURL+"/admin/base/sys/user/page", token, map[string]interface{}{"page": 1, "size": 15})
   if response.StatusCode != http.StatusOK {
      t.Fatalf("expected admin CRUD success, got %d", response.StatusCode)
   }

   createLimitedUser(t, ctx)
   limitedToken := loginForPermissionTest(t, baseURL, "limited", "123456")
   denied := postJSON(t, baseURL+"/admin/base/sys/user/page", limitedToken, map[string]interface{}{"page": 1, "size": 15})
   if denied.StatusCode != http.StatusForbidden {
      t.Fatalf("expected 403, got %d", denied.StatusCode)
   }
   if denied.Body != `{"code":1001,"message":"权限不足~"}` {
      t.Fatalf("unexpected forbidden body: %s", denied.Body)
   }
}
```

Add helper types/functions in the same file:

```go
type testHTTPResponse struct {
   StatusCode int
   Body       string
}

func loginForPermissionTest(t *testing.T, baseURL string, username string, password string) string { ... }
func getJSON(t *testing.T, url string, token string) map[string]interface{} { ... }
func postJSON(t *testing.T, url string, token string, body map[string]interface{}) testHTTPResponse { ... }
func assertStringSliceContains(t *testing.T, items []interface{}, expected string) { ... }
func createLimitedUser(t *testing.T, ctx context.Context) { ... }
```

Implementation details:

1. `loginForPermissionTest` POSTs `/admin/base/open/login` and returns `data.token`.
2. `getJSON` sends `Authorization: <token>` raw token header.
3. `postJSON` sends JSON body and raw token header, returns status/body.
4. `createLimitedUser` inserts:
   - `base_sys_role` label `limited`.
   - `base_sys_user` username `limited`, password md5 `e10adc3949ba59abbe56e057f20f883e`, status `1`.
   - `base_sys_user_role` relation.
   - It intentionally does not grant `base:sys:user:page`.
5. Use parameterized SQL and fixed high IDs such as `9001` for limited data after cleanup.

If the `ghttp.GetServer` global server cannot easily isolate from app.New, adapt using the existing app server pattern from `auth_integration_test.go`. The test must exercise real HTTP app routes.

- [ ] **Step 2: Run skipped integration test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run PermissionIntegration -count=1
```

Expected: PASS with skip unless env var is set.

- [ ] **Step 3: Run real permission integration test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: PASS.

- [ ] **Step 5: Commit Task 5**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/permission_integration_test.go
git commit -m "test: add permission integration coverage" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: README 更新、最终验证和 review

**Files:**
- Modify: `README.md`
- Test: `go test ./...`
- Integration Test: `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1`

**Interfaces:**
- Consumes: all previous tasks.
- Produces: documented and verified Plan5B state.

- [ ] **Step 1: Update README**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/README.md`:

1. 当前阶段改为：`阶段 5B：权限菜单和 CRUD 权限校验`。
2. 已完成列表加入：
   - `GET /admin/base/comm/permmenu`。
   - admin 全菜单和权限码。
   - 普通用户角色菜单权限。
   - base CRUD 权限 middleware。
3. 未完成列表保留：
   - EPS runtime。
   - Vue 前端联调。
4. 新增权限验收命令：

```bash
go test ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

5. 新增手工验证示例：

```bash
curl http://127.0.0.1:8001/admin/base/comm/permmenu \
  -H 'Authorization: <token>'

curl -X POST http://127.0.0.1:8001/admin/base/sys/user/page \
  -H 'Content-Type: application/json' \
  -H 'Authorization: <token>' \
  -d '{"page":1,"size":15}'
```

- [ ] **Step 2: Run unit tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run real MySQL permission integration**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify forbidden directories absent**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
test ! -d logic
test ! -d dao
test ! -d internal/model/do
test ! -d internal/model/entity
```

Expected: all commands exit `0`.

- [ ] **Step 6: Commit Task 6**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md
git commit -m "docs: document permission phase" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final Plan5B Acceptance Checklist

- [ ] `GET /admin/base/comm/permmenu` 带 admin token 返回成功。
- [ ] admin `menus` 非空。
- [ ] admin `perms` 包含 `base:sys:user:page` 和 `base:sys:user:add`。
- [ ] `perms` 能拆分逗号权限码。
- [ ] `menus` 不包含 `type = 2` 的按钮项。
- [ ] 普通用户按角色菜单关系返回权限。
- [ ] 普通用户菜单包含必要父级链路。
- [ ] admin 调 base CRUD 成功。
- [ ] 缺少 CRUD 权限的普通用户调 base CRUD 返回 HTTP 403。
- [ ] 403 body 精确为 `{"code":1001,"message":"权限不足~"}`。
- [ ] `go test ./...` 通过。
- [ ] `COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1` 通过。
- [ ] 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

---

## Self-Review

### Spec coverage

This plan covers the approved Stage 5B design:

1. Permission helper and explicit CRUD map: Task 1.
2. Permmenu query and permissions: Task 2.
3. Permmenu route and CRUD permission middleware: Task 3.
4. App route stack integration: Task 4.
5. Real HTTP permission integration: Task 5.
6. README and final verification: Task 6.

Excluded items match the spec: EPS runtime, Vue real integration, upload, personUpdate, data scope permissions, operation log enhancements, and generic permission framework.

### Placeholder scan

The plan contains no unresolved placeholder markers. Every task has concrete files, interfaces, commands, and expected outcomes.

### Type consistency

The produced interfaces are consistent across tasks:

1. `PermissionService` and `PermMenuResult` are created in Task 1 and used in Tasks 2-4.
2. `SplitPerms` and `UniqueStrings` are created in Task 1 and used in Task 2.
3. `CRUDPermissionMap` is created in Task 1 and used by `RoutePermission` in Task 3.
4. `RegisterPermissionRoutes` and `RegisterPermissionMiddleware` are created in Task 3 and consumed by app in Task 4.
