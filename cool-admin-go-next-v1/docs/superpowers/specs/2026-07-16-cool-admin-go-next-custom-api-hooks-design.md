# cool-admin-go-next 阶段 6D Base 自定义 API 与 CRUD Hooks 设计

日期：2026-07-16

## 1. 背景

阶段 6A/6B/6C 分别设计了 controller/service 源码形态、metadata 驱动路由/权限、EPS runtime。完成这些后，前端能基于 EPS 生成 service 方法，但 base 模块仍缺 Node 版中常用的自定义 API 和 CRUD hooks。

协议文档中已列出第一阶段必须关注的 base 自定义接口：

| Prefix | 自定义 API |
|---|---|
| `/admin/base/comm` | `personUpdate`、`upload`、`uploadMode` |
| `/admin/base/sys/user` | `move` |
| `/admin/base/sys/menu` | `parse`、`create`、`export`、`import` |
| `/admin/base/sys/department` | `order` |
| `/admin/base/sys/param` | `html` |
| `/admin/base/sys/log` | `clear`、`setKeep`、`getKeep` |

阶段 6D 目标是补齐这些自定义 API 的设计，并把 CRUD hooks 机制接入 runtime，使后续业务行为更接近 Node `BaseService` / `@CoolController`。

## 2. 目标

阶段 6D 完成后应满足：

1. controller metadata 能声明 base 自定义 API。
2. EPS 能输出自定义 API。
3. 权限映射能覆盖需要权限的自定义 API。
4. CRUD runtime 支持基础 hooks：Before/After Add/Delete/Update/Info/List/Page。
5. base service 能通过 hooks 实现 Node 兼容行为。
6. 补齐前端常用 base 自定义 API。
7. 现有 auth、permmenu、CRUD 行为不回退。

## 3. 非目标

阶段 6D 不做：

1. 完整文件存储适配器生态，例如 OSS、COS、S3 全量支持。
2. 数据权限 / 部门范围权限。
3. 操作日志完整增强。
4. 插件系统。
5. 队列任务系统。
6. TypeScript SDK 生成。
7. PostgreSQL / SQLite。

## 4. 总体方案

阶段 6D 基于 6A/6B 的 controller metadata 和 service 分层实现：

```text
controller metadata
  ↓
custom route registration
  ↓
service method handler
  ↓
response.OK / forbidden / unauthorized
```

CRUD hooks 基于 service interface 自动探测：

```text
crud.Runtime
  ↓
if service implements hook interface
  ↓
call hook before/after core CRUD action
```

设计原则：

1. 自定义 API 必须声明在 controller metadata 中，不在 app 中手写注册。
2. 权限码尽量显式写在 RouteOptions 中。
3. hooks 不应污染通用 CRUD 核心逻辑；通过小接口检测调用。
4. 当前阶段优先满足 cool-admin-vue 前端实际调用，不做超出第一阶段的存储/日志/权限增强。

## 5. Route metadata 扩展

自定义 API 使用阶段 6B 的 `RouteOptions`：

```go
type RouteOptions struct {
   Name              string
   Method            string
   Path              string
   Description       string
   IgnoreAuth        bool
   RequirePermission bool
   Permission        string
   Handler           controller.Handler
}
```

阶段 6D 约束：

1. admin sys 自定义 API 默认 `RequirePermission = true`。
2. 权限码必须显式写，例如 `base:sys:log:clear`。
3. comm 接口根据协议决定是否需要 auth。
4. upload/uploadMode 可以先实现本地模式，不做云存储。
5. 所有 route 都进入 EPS，除非显式 `HideFromEPS`。

## 6. CRUD hooks 设计

### 6.1 Action 定义

新增或扩展：

```go
type Action string

const (
   ActionAdd    Action = "add"
   ActionDelete Action = "delete"
   ActionUpdate Action = "update"
   ActionInfo   Action = "info"
   ActionList   Action = "list"
   ActionPage   Action = "page"
)
```

### 6.2 Hook 参数

```go
type HookContext struct {
   Resource Resource
   Action   Action
   User     *auth.UserContext
   Input    map[string]interface{}
   Result   interface{}
}
```

说明：

| 字段 | 说明 |
|---|---|
| `Resource` | 当前 CRUD 资源 |
| `Action` | 当前动作 |
| `User` | 当前登录用户，可能为空 |
| `Input` | 请求参数，可在 Before hook 中修改 |
| `Result` | CRUD 结果，可在 After hook 中修改 |

### 6.3 Hook interface

```go
type BeforeHook interface {
   Before(ctx context.Context, hook *HookContext) error
}

type AfterHook interface {
   After(ctx context.Context, hook *HookContext) error
}
```

为了贴近 Node，也可提供语义化可选接口：

```go
type ModifyBeforeHook interface {
   ModifyBefore(ctx context.Context, hook *HookContext) error
}

type ModifyAfterHook interface {
   ModifyAfter(ctx context.Context, hook *HookContext) error
}
```

推荐 runtime 内部优先使用统一 `Before/After`，service 层可封装语义方法。

### 6.4 调用顺序

Add/Update/Delete：

```text
parse request
  ↓
Before hook
  ↓
core CRUD action
  ↓
After hook
  ↓
response.OK
```

Info/List/Page：

```text
parse query/body
  ↓
Before hook
  ↓
core query
  ↓
After hook
  ↓
response.OK
```

错误规则：

1. Before hook 返回错误，终止请求。
2. core CRUD 返回错误，终止请求。
3. After hook 返回错误，终止请求。
4. 所有错误用现有业务失败响应，不暴露内部堆栈。

## 7. Comm 自定义 API

### 7.1 `POST /admin/base/comm/personUpdate`

用途：修改当前用户个人信息。

请求：

```json
{
  "nickName": "新昵称",
  "headImg": "头像",
  "phone": "手机号",
  "email": "邮箱",
  "remark": "备注"
}
```

规则：

1. 必须登录。
2. 只能更新当前用户允许字段。
3. 禁止更新 `id`、`username`、`password`、`passwordV`、`status`、`tenantId`。
4. 成功返回空 data。

### 7.2 `POST /admin/base/comm/upload`

用途：文件上传。

阶段 6D 最小实现：

1. 支持 multipart file。
2. 保存到本地 `resource/public/uploads/YYYYMMDD/`。
3. 返回可访问 URL 或相对路径。
4. 限制单文件大小使用配置，默认可先设置 10MB。

返回建议：

```json
{
  "url": "/uploads/20260716/xxx.png",
  "filename": "xxx.png",
  "size": 12345
}
```

### 7.3 `GET /admin/base/comm/uploadMode`

阶段 6D 返回：

```json
"local"
```

如前端期望 Node 具体枚举，以真实前端行为为准调整。

## 8. User 自定义 API

### 8.1 `POST /admin/base/sys/user/move`

用途：移动用户部门。

请求建议：

```json
{
  "departmentId": 1,
  "ids": [2, 3]
}
```

规则：

1. 需要权限 `base:sys:user:move`。
2. 批量更新 `base_sys_user.department_id`。
3. 禁止移动不存在用户时静默成功；至少返回更新数量或空 data。
4. 不允许修改 admin 用户的安全字段。

## 9. Menu 自定义 API

### 9.1 `POST /admin/base/sys/menu/parse`

用途：解析前端路由或菜单定义。

阶段 6D 风险较高，因为它依赖 Node/Vue 真实菜单生成规则。

建议最小策略：

1. 先根据前端真实请求体确定输入格式。
2. 如果前端只在开发/菜单管理中使用，可实现兼容空结果或基础解析。
3. 不猜复杂 parse 行为。

### 9.2 `POST /admin/base/sys/menu/create`

用途：根据 parse 结果创建菜单。

规则：

1. 需要权限 `base:sys:menu:create`。
2. 参数按前端真实请求确认。
3. 写入 `base_sys_menu`。
4. 维护 parentId/orderNum/type/perms。

### 9.3 `POST /admin/base/sys/menu/export`

用途：导出菜单配置。

阶段 6D 可返回当前菜单树或平铺菜单 JSON。

### 9.4 `POST /admin/base/sys/menu/import`

用途：导入菜单配置。

规则：

1. 需要权限 `base:sys:menu:import`。
2. 支持前端上传/提交的菜单 JSON。
3. 必须事务执行。
4. 失败回滚。

说明：menu 四个接口依赖真实前端行为，实施计划中应先抓取前端请求或参考 Node 源码后再定最终字段。

## 10. Department 自定义 API

### 10.1 `POST /admin/base/sys/department/order`

用途：调整部门排序。

请求建议：

```json
{
  "orders": [
    { "id": 1, "orderNum": 1 },
    { "id": 2, "orderNum": 2 }
  ]
}
```

规则：

1. 需要权限 `base:sys:department:order`。
2. 批量更新 `order_num`。
3. 使用事务。
4. 成功返回空 data。

## 11. Param 自定义 API

### 11.1 `GET /admin/base/sys/param/html`

用途：根据配置参数 key 返回 HTML。

协议中 open/html 也存在类似能力，但 base sys param/html 是管理侧接口。

请求：

```text
GET /admin/base/sys/param/html?key=xxx
```

规则：

1. 需要权限 `base:sys:param:html` 或按前端行为决定是否只需登录。
2. 查询 `base_sys_param.key_name = key`。
3. 返回原始 HTML 字符串或标准 response data。
4. 若前端期望原始 HTML body，必须特殊处理，不套 `response.OK`。

该接口需在实施前确认前端消费方式。

## 12. Log 自定义 API

### 12.1 `POST /admin/base/sys/log/clear`

用途：清空日志。

规则：

1. 需要权限 `base:sys:log:clear`。
2. 删除或清空 `base_sys_log`。
3. 成功返回空 data。

### 12.2 `POST /admin/base/sys/log/setKeep`

用途：设置日志保留天数。

请求建议：

```json
{
  "value": 31
}
```

规则：

1. 需要权限 `base:sys:log:setKeep`。
2. 写入 `base_sys_conf.c_key = logKeep`。
3. 成功返回空 data。

### 12.3 `GET /admin/base/sys/log/getKeep`

用途：读取日志保留天数。

规则：

1. 需要权限 `base:sys:log:getKeep` 或按前端行为决定是否只需登录。
2. 读取 `base_sys_conf.c_key = logKeep`。
3. 不存在时返回默认值。

## 13. 权限和 EPS 规则

### 13.1 权限码

自定义 API 权限码：

| API | 权限码 |
|---|---|
| user/move | `base:sys:user:move` |
| menu/parse | `base:sys:menu:parse` |
| menu/create | `base:sys:menu:create` |
| menu/export | `base:sys:menu:export` |
| menu/import | `base:sys:menu:import` |
| department/order | `base:sys:department:order` |
| param/html | `base:sys:param:html` |
| log/clear | `base:sys:log:clear` |
| log/setKeep | `base:sys:log:setKeep` |
| log/getKeep | `base:sys:log:getKeep` |

### 13.2 EPS

所有自定义 API 都应出现在对应 controller 的 `api` 列表中：

```json
{
  "method": "POST",
  "path": "/clear",
  "summary": "清空日志",
  "prefix": "/admin/base/sys/log",
  "ignoreToken": false
}
```

## 14. 测试策略

### 14.1 Hook 单元测试

覆盖：

1. Before hook 能修改 input。
2. Before hook 返回错误时 core CRUD 不执行。
3. After hook 能修改 result。
4. hook 错误会返回业务失败。

### 14.2 自定义 API 单元 / HTTP 测试

覆盖：

1. personUpdate 只能更新允许字段。
2. uploadMode 返回 local。
3. user move 更新 departmentId。
4. department order 批量更新 orderNum。
5. log clear 清空日志。
6. log setKeep/getKeep 读写配置。
7. 缺权限返回 403 精确 body。

### 14.3 真实 MySQL 集成测试

新增：

```text
COOL_CUSTOM_API_INTEGRATION=1
```

覆盖优先级：

1. 登录 admin。
2. 调用 EPS，确认自定义 API 出现在 api 列表。
3. 调用 log setKeep/getKeep。
4. 调用 department order。
5. 调用 user move。
6. 构造无权限用户访问自定义 API 返回 403。

必跑命令：

```bash
go test ./cool/crud ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run CustomAPIIntegration -count=1
```

## 15. 实施顺序建议

阶段 6D 实施建议拆成小任务：

1. 接入 CRUD hooks 基础能力和测试。
2. 实现 comm personUpdate/uploadMode，upload 先本地最小实现。
3. 实现 log clear/setKeep/getKeep。
4. 实现 department order。
5. 实现 user move。
6. 根据 Node 源码或前端真实请求确认 menu parse/create/export/import，再实现。
7. 更新 EPS 和权限测试，确认自定义 API 出现在 metadata。

原因：menu parse/create/import/export 复杂度最高，应该放在已有 hooks/custom route 机制稳定后处理。

## 16. 风险和约束

1. menu parse/create/import/export 不能凭空猜协议，必须参考 Node 源码或真实前端请求。
2. upload 涉及文件路径和安全校验，必须限制目录穿越和文件大小。
3. param/html 可能需要返回原始 HTML body，不能默认套 response.OK。
4. log clear 是破坏性操作，集成测试必须在隔离测试数据上执行。
5. hooks 不能破坏现有 CRUD 响应结构。
6. 自定义 API 权限必须进入 permission map。
7. EPS 必须包含自定义 API，否则前端不会生成 service 方法。
8. 不创建手写 `dao/`、`internal/model/do/`、`internal/model/entity/`。
9. 不使用 `logic/` 目录。

## 17. 验收标准

阶段 6D 完成后必须满足：

1. CRUD runtime 支持 Before/After hooks。
2. 自定义 API 通过 controller metadata 注册。
3. 自定义 API 出现在 EPS。
4. 自定义 API 权限进入权限映射。
5. personUpdate 可更新当前用户允许字段。
6. uploadMode 返回可用上传模式。
7. log clear/setKeep/getKeep 可用。
8. department order 可用。
9. user move 可用。
10. menu parse/create/export/import 有明确实现或基于 Node/前端证据的兼容行为。
11. 缺权限访问自定义 API 返回 HTTP 403 和精确 body。
12. `go test ./...` 通过。
13. `COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run CustomAPIIntegration -count=1` 通过。
14. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 18. 自检

本设计覆盖 base 自定义 API 和 CRUD hooks。它不处理插件系统、数据权限、完整云存储或前端真实联调。menu 相关复杂接口明确要求实施前参考 Node 源码或真实前端请求，避免猜协议。
