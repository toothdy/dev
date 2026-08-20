# cool-admin-go-next Permmenu Permission Design

日期：2026-07-16

## 1. 目标

阶段 5B 实现权限菜单、权限码和基础 CRUD 权限校验，让已登录用户可以加载后台菜单，并让 base 模块 CRUD 接口按权限码拦截访问。

阶段 5B 完成后应满足：

1. `GET /admin/base/comm/permmenu` 返回 `{ menus, perms }`。
2. admin 用户拥有全部启用菜单和全部权限码。
3. 普通用户通过角色菜单关系获得菜单和权限码。
4. base CRUD 路由根据当前用户权限码做基础校验。
5. 缺权限返回 HTTP `403` 和 Node 兼容 JSON body。

## 2. 范围

### 2.1 本期包含

接口：

| Method | Path | 说明 |
|---|---|---|
| GET | `/admin/base/comm/permmenu` | 当前用户菜单和权限码 |

权限能力：

1. admin 全权限。
2. 普通用户角色权限。
3. 菜单数组输出。
4. 权限码数组输出。
5. base CRUD 权限拦截。
6. HTTP 集成测试覆盖登录、permmenu、CRUD 成功和无权限失败。

### 2.2 本期不包含

1. EPS runtime。
2. Vue 真实联调。
3. 文件上传。
4. `personUpdate`。
5. 数据权限 / 部门范围权限。
6. 操作日志增强。
7. 多模块通用权限框架抽象。
8. 按钮级前端交互验证。

## 3. 设计原则

1. 第一阶段只服务 base 模块，不提前抽象 `cool/permission`。
2. 权限逻辑放在 `modules/base`，和当前 `auth.go`、`auth_routes.go`、`routes.go` 保持一致。
3. CRUD 权限映射显式配置，不依赖字符串猜测。
4. HTTP JSON 字段保持 Node/Vue 兼容的 camelCase。
5. 内部错误不暴露给前端。
6. 测试使用真实 HTTP 流程验证 middleware 和 route registration。

## 4. 架构

新增 base 权限服务：

```text
modules/base/permission.go
```

职责：

1. 查询当前用户权限菜单。
2. 生成 `menus`。
3. 生成 `perms`。
4. 判断用户是否拥有某个权限码。
5. 提供 CRUD 路由到权限码的显式映射。

新增 base 权限路由：

```text
modules/base/permission_routes.go
```

职责：

1. 注册 `GET /admin/base/comm/permmenu`。
2. 注册 CRUD 权限 middleware。
3. 对缺权限请求返回 HTTP `403`。

`cool/app.registerRoutes()` 的目标顺序：

```text
/health
  ↓
base auth routes
  ↓
auth middleware
  ↓
base permission routes / permission middleware
  ↓
base CRUD routes
```

说明：

- auth middleware 先解析 token 并写入 `auth.UserContext`。
- permmenu 和 CRUD 权限 middleware 依赖 `auth.UserContext`。
- open/login、open/refreshToken、open/captcha、comm/program 仍通过 `AuthIgnorePaths()` 放行。

## 5. 数据模型

使用现有表：

| 表 | 用途 |
|---|---|
| `base_sys_user` | 判断用户、admin 用户名 |
| `base_sys_user_role` | 用户角色关系 |
| `base_sys_role` | 角色信息，`label = admin` 作为超管角色标识 |
| `base_sys_role_menu` | 角色菜单关系 |
| `base_sys_menu` | 菜单和按钮权限 |

admin 判定规则：

1. 如果当前用户 `username == "admin"`，视为超管。
2. 或者用户拥有 `base_sys_role.label == "admin"` 的角色，视为超管。

普通用户规则：

```text
base_sys_user.id
  ↓ base_sys_user_role.user_id
base_sys_user_role.role_id
  ↓ base_sys_role_menu.role_id
base_sys_role_menu.menu_id
  ↓ base_sys_menu.id
base_sys_menu rows
```

## 6. permmenu 输出

### 6.1 响应结构

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "menus": [],
    "perms": []
  }
}
```

### 6.2 menus 规则

`menus` 第一版返回平铺数组，不在后端构树。

每项字段：

```json
{
  "id": 1,
  "parentId": null,
  "name": "系统管理",
  "router": "/sys",
  "perms": null,
  "type": 0,
  "icon": "icon-set",
  "orderNum": 2,
  "viewPath": null,
  "keepAlive": true,
  "isShow": true,
  "createTime": "2026-07-16 00:00:00",
  "updateTime": "2026-07-16 00:00:00"
}
```

查询规则：

1. admin 返回所有 `is_show = 1` 且 `type != 2` 的菜单。
2. 普通用户返回角色绑定菜单中 `is_show = 1` 且 `type != 2` 的菜单。
3. 按 `order_num ASC, id ASC` 排序。
4. 字段输出使用 camelCase。
5. 不返回 `password` 等用户敏感字段。

### 6.3 perms 规则

1. 来源于用户可访问的 `base_sys_menu.perms`。
2. 只收集非空 perms。
3. 支持逗号分隔权限码，例如：
   ```text
   base:sys:menu:page,base:sys:menu:list,base:sys:menu:info
   ```
4. 拆分后去空白、去重。
5. 返回稳定顺序，建议按首次出现顺序去重。
6. admin 返回所有启用菜单中的全部 perms。
7. 普通用户返回角色绑定菜单中的 perms。

## 7. CRUD 权限校验

### 7.1 受保护范围

阶段 5B 只保护 base 模块已注册 CRUD 路由：

```text
/admin/base/sys/user
/admin/base/sys/role
/admin/base/sys/menu
/admin/base/sys/department
/admin/base/sys/param
/admin/base/sys/log
```

### 7.2 权限码映射

显式映射：

| Method | Path suffix | 权限码 |
|---|---|---|
| POST | `/admin/base/sys/user/add` | `base:sys:user:add` |
| POST | `/admin/base/sys/user/delete` | `base:sys:user:delete` |
| POST | `/admin/base/sys/user/update` | `base:sys:user:update` |
| GET | `/admin/base/sys/user/info` | `base:sys:user:info` |
| POST | `/admin/base/sys/user/list` | `base:sys:user:list` |
| POST | `/admin/base/sys/user/page` | `base:sys:user:page` |
| POST | `/admin/base/sys/role/add` | `base:sys:role:add` |
| POST | `/admin/base/sys/role/delete` | `base:sys:role:delete` |
| POST | `/admin/base/sys/role/update` | `base:sys:role:update` |
| GET | `/admin/base/sys/role/info` | `base:sys:role:info` |
| POST | `/admin/base/sys/role/list` | `base:sys:role:list` |
| POST | `/admin/base/sys/role/page` | `base:sys:role:page` |
| POST | `/admin/base/sys/menu/add` | `base:sys:menu:add` |
| POST | `/admin/base/sys/menu/delete` | `base:sys:menu:delete` |
| POST | `/admin/base/sys/menu/update` | `base:sys:menu:update` |
| GET | `/admin/base/sys/menu/info` | `base:sys:menu:info` |
| POST | `/admin/base/sys/menu/list` | `base:sys:menu:list` |
| POST | `/admin/base/sys/menu/page` | `base:sys:menu:page` |
| POST | `/admin/base/sys/department/add` | `base:sys:department:add` |
| POST | `/admin/base/sys/department/delete` | `base:sys:department:delete` |
| POST | `/admin/base/sys/department/update` | `base:sys:department:update` |
| POST | `/admin/base/sys/department/list` | `base:sys:department:list` |
| POST | `/admin/base/sys/param/add` | `base:sys:param:add` |
| POST | `/admin/base/sys/param/delete` | `base:sys:param:delete` |
| POST | `/admin/base/sys/param/update` | `base:sys:param:update` |
| GET | `/admin/base/sys/param/info` | `base:sys:param:info` |
| POST | `/admin/base/sys/param/page` | `base:sys:param:page` |
| POST | `/admin/base/sys/log/page` | `base:sys:log:page` |

说明：

1. 当前未注册的自定义 API，例如 `move`、`parse`、`create`、`export`、`import`、`clear`、`setKeep`、`getKeep`，本期不拦截，因为当前没有对应路由。
2. 后续新增自定义路由时必须同步补权限映射。

### 7.3 缺权限响应

缺权限返回：

```http
HTTP/1.1 403
```

body：

```json
{
  "code": 1001,
  "message": "权限不足~"
}
```

不能使用会写入默认 body 的 API，避免响应体被 `Forbidden` 等文本污染。

### 7.4 内部错误响应

权限查询发生 DB/内部错误时：

1. 服务端日志记录原始错误。
2. 客户端返回 HTTP `403` 或 HTTP `200` 业务失败都可被前端处理；本期统一使用 HTTP `403`：

```json
{
  "code": 1001,
  "message": "权限不足~"
}
```

## 8. 文件职责

### 8.1 `modules/base/permission.go`

核心类型：

```go
type PermissionService struct {
   DB gdb.DB
}

type PermMenuResult struct {
   Menus []map[string]interface{} `json:"menus"`
   Perms []string                 `json:"perms"`
}
```

核心接口：

```go
func NewPermissionService(db gdb.DB) *PermissionService
func (s *PermissionService) PermMenu(ctx context.Context, user auth.UserContext) (PermMenuResult, error)
func (s *PermissionService) HasPermission(ctx context.Context, user auth.UserContext, permission string) (bool, error)
func CRUDPermissionMap() map[string]string
func SplitPerms(value string) []string
```

### 8.2 `modules/base/permission_routes.go`

核心接口：

```go
func RegisterPermissionRoutes(server *ghttp.Server, service *PermissionService)
func RegisterPermissionMiddleware(server *ghttp.Server, service *PermissionService)
func WriteForbidden(r *ghttp.Request)
```

职责：

1. 注册 `GET /admin/base/comm/permmenu`。
2. 注册 CRUD 权限 middleware。
3. 缺权限时返回 403 JSON。

### 8.3 `cool/app/app.go`

修改注册顺序：

```text
/health
base auth routes
auth middleware
base permission routes
base permission middleware
CRUD routes
```

## 9. 测试策略

### 9.1 单元测试

新增：

```text
modules/base/permission_test.go
modules/base/permission_routes_test.go
```

覆盖：

1. `SplitPerms` 拆分逗号权限码。
2. `SplitPerms` 去空白和空项。
3. `Perms` 去重保持稳定顺序。
4. `CRUDPermissionMap` 包含 user/role/menu/department/param/log 主要路由。
5. `WriteForbidden` 返回 HTTP 403 和精确 JSON body。

### 9.2 集成测试

新增或扩展：

```text
modules/base/permission_integration_test.go
```

环境变量：

```text
COOL_PERMISSION_INTEGRATION=1
```

流程：

1. schema sync。
2. seed db/menu。
3. 启动真实 HTTP server。
4. 登录 admin。
5. 调用 `/admin/base/comm/permmenu`。
6. 校验：
   - `menus` 非空。
   - `perms` 非空。
   - perms 包含 `base:sys:user:page`。
   - perms 包含 `base:sys:user:add`。
7. 带 admin token 调用 `/admin/base/sys/user/page` 成功。
8. 构造普通用户和无 CRUD 权限角色。
9. 登录普通用户。
10. 普通用户调用 `/admin/base/sys/user/page` 返回 HTTP 403 和精确 JSON。

### 9.3 全量验证

必须通过：

```bash
go test ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

如果权限集成测试依赖 auth integration 数据，测试必须自己完成 schema/seed/用户构造，不能依赖测试执行顺序。

## 10. 风险与约束

1. `base_sys_menu.perms` 存在逗号分隔值，不能当作单个权限码。
2. `type = 2` 的按钮权限不进入 `menus`，但必须进入 `perms`。
3. 普通用户需要保证父级菜单可访问，否则前端可能无法挂载页面；本期规则是返回角色绑定菜单及其必要父级菜单。
4. 如果角色只绑定按钮权限，后端需要补齐父级链路，避免菜单断层。
5. 权限 middleware 必须在 auth middleware 之后执行。
6. 缺权限响应不能被 GoFrame 默认 body 污染。
7. 内部 DB 错误不暴露给前端。

## 11. 验收标准

阶段 5B 完成后必须满足：

1. `GET /admin/base/comm/permmenu` 带 admin token 返回成功。
2. admin `menus` 非空。
3. admin `perms` 包含 seed 中关键权限码。
4. `perms` 能拆分逗号权限码。
5. `menus` 不包含 `type = 2` 的按钮项。
6. 普通用户按角色菜单关系返回权限。
7. 普通用户菜单包含必要父级链路。
8. admin 调 base CRUD 成功。
9. 缺少 CRUD 权限的普通用户调 base CRUD 返回 HTTP 403。
10. 403 body 精确为 `{"code":1001,"message":"权限不足~"}`。
11. `go test ./...` 通过。
12. 真实 MySQL 权限集成测试通过。
13. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 12. 后续阶段

阶段 5B 完成后，下一阶段是：

1. 阶段 6：EPS runtime。
2. 阶段 7：Vue 前端联调和兼容修正。

如果 Vue 联调发现菜单结构需要树形输出，再在阶段 7 基于真实前端行为调整，不在 5B 提前过度设计。
