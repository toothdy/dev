# cool-admin-go-next 阶段 6D 首批自定义 API 设计

日期：2026-07-21

## 1. 背景与范围

阶段 6C 已完成匿名 EPS、图片验证码和登录闭环。Vue 前端已可登录，但常用管理界面仍依赖一批尚未实现的 base 自定义 API。本阶段以现有 Controller metadata 为唯一接口来源，补齐首批高价值能力，保证新接口自动注册、进入权限映射并出现在 EPS。

本阶段实现：

1. `POST /admin/base/comm/personUpdate`
2. `GET /admin/base/comm/uploadMode`
3. `POST /admin/base/comm/upload`
4. `POST /admin/base/sys/user/move`
5. `POST /admin/base/sys/department/order`
6. `POST /admin/base/sys/log/clear`
7. `POST /admin/base/sys/log/setKeep`
8. `GET /admin/base/sys/log/getKeep`

本阶段不实现菜单 `parse/create/export/import`、`param/html`、`open/html`、云存储适配器、数据权限、部门范围权限、定时日志清理任务，也不重构已有 `ModifyBefore/ModifyAfter` CRUD Hook。

## 2. 方案选择

采用既有 Controller/Service 分层：

```text
HTTP 请求
  -> Controller metadata 路由注册
  -> Auth middleware
  -> Permission middleware（仅 sys 管理接口）
  -> Controller handler（解析、认证上下文、统一响应）
  -> 领域 Service（输入校验、事务、数据库或文件系统）
  -> Node 兼容 response.OK
```

不在 Controller 中直接编写 SQL，不引入通用 action 框架。该方案沿用现有 Controller metadata、`cool/controller.PermissionMap`、`cool/eps.Generate` 和 base Service 结构；每个领域行为可独立测试。

## 3. Controller metadata 与权限

### 3.1 Comm Controller

在 `BaseCommController` 中新增以下已登录路由，不设置细粒度 `Permission`：

| Method | Path | Name | Summary |
|---|---|---|---|
| POST | `/personUpdate` | `personUpdate` | 修改个人信息 |
| GET | `/uploadMode` | `uploadMode` | 文件上传模式 |
| POST | `/upload` | `upload` | 文件上传 |

Node 中 `/admin/base/comm/*` 对已登录用户直接放行，因此上述路由只受认证 middleware 保护。无 token 或无效 token 时维持 HTTP 401。

### 3.2 Admin Controller

在对应 admin Controller 中声明以下 Route；每条均显式指定 `Permission`，从而自动进入 `PermissionMap`、权限 middleware 和 EPS：

| Controller | Method | Path | Permission | Summary |
|---|---|---|---|---|
| BaseSysUserEntity | POST | `/move` | `base:sys:user:move` | 移动部门 |
| BaseSysDepartmentEntity | POST | `/order` | `base:sys:department:order` | 排序 |
| BaseSysLogEntity | POST | `/clear` | `base:sys:log:clear` | 清理 |
| BaseSysLogEntity | POST | `/setKeep` | `base:sys:log:setKeep` | 日志保存时间 |
| BaseSysLogEntity | GET | `/getKeep` | `base:sys:log:getKeep` | 获得日志保存时间 |

当前 `menu.json` 已包含这五个权限码，因此 admin 可直接访问，受限用户会被权限 middleware 拦截。

## 4. 服务边界与数据契约

### 4.1 修改当前用户资料

在认证相关服务或 Comm 专用小服务中实现 `PersonUpdate(ctx, userID, request)`；Controller 从认证上下文取用户 ID，禁止接受客户端提交的目标 ID。

请求可含 `nickName`、`headImg`、`phone`、`email`、`remark`。只更新该白名单字段；忽略或拒绝 `id`、`username`、`password`、`passwordV`、`status`、角色、部门及租户字段。空对象允许并返回空成功数据。

成功响应为：

```json
{"code":1000,"message":"success","data":{}}
```

本阶段不开放 Node 中的个人密码修改逻辑，避免在尚未定义旧密码与 token 失效流程时猜测协议。

### 4.2 本地上传

上传服务仅支持 multipart 字段 `file`。`uploadMode` 固定返回 Vue 上传插件实际消费的：

```json
{"mode":"local","type":"local"}
```

`upload` 验证文件存在、文件大小不超过明确的本地上传上限，并使用随机文件名而不信任原始路径或文件名。文件按日期保存到应用专用上传目录。启动 server 时使用 GoFrame 静态路径映射将该目录暴露为 `/uploads/`；不启用目录列表，不把应用其他工作目录暴露为静态根目录。

成功 data 为 Vue 本地上传分支实际消费的 URL 字符串，例如：

```json
"/uploads/20260721/<random>.png"
```

文件缺失、超限、写入失败等预期业务错误使用 HTTP 200、`code:1001` 和不泄露物理路径的中文消息。不会实现 OSS、COS、S3、MinIO 或其他云存储协议。

### 4.3 用户移动部门

`UserService.Move(ctx, request)` 接收前端实际格式：

```json
{"departmentId":1,"userIds":[2,3]}
```

`userIds` 必须非空，ID 必须有效且无重复；目标部门必须存在。服务在单一事务内批量更新 `base_sys_user.department_id`，成功返回空对象。当前阶段不增加数据范围权限或 admin 账户额外限制，保持 Node `update({ id: In(userIds) }, { departmentId })` 的职责范围。

### 4.4 部门树排序

Vue 拖拽后实际提交数组而非包装对象：

```json
[
  {"id":1,"parentId":null,"orderNum":0},
  {"id":2,"parentId":1,"orderNum":1}
]
```

`DepartmentService.Order(ctx, entries)` 接收该数组。每项只允许 `id`、`parentId`、`orderNum`；数组不能为空，ID 必须正数且无重复，`orderNum` 必须为非负整数。服务先确认所有目标部门存在，然后在单一事务内更新 `parent_id` 和 `order_num`。任一校验或更新失败则整个事务回滚。

这与 Node 版逐项更新对象的语义一致，同时补上 Go 版事务原子性保证。

### 4.5 日志与保留配置

`LogService` 增加 `Clear(ctx)`，直接清空 `base_sys_log`，严格对齐 Node `clear(true)` 的全表行为；本阶段不叠加尚未实现的 tenant 过滤。

日志保留天数由 `base_sys_conf` 的 `c_key = "logKeep"` 保存。服务提供：

- `SetKeep(ctx, value)`：请求 `{ "value": <正整数> }`，写入或更新 `logKeep`。
- `GetKeep(ctx)`：读取 `logKeep`；若缺失，回退到初始化数据中的默认值。

`GET /getKeep` 返回值保持与配置值兼容，供 Vue 的 `Number(res)` 使用。

## 5. 错误与权限契约

| 情形 | HTTP | Body |
|---|---:|---|
| 成功 | 200 | `{"code":1000,"message":"success","data":...}` |
| 输入校验、数据不存在、上传预期失败 | 200 | `{"code":1001,"message":"<明确中文业务消息>"}` |
| 未登录或 token 无效 | 401 | 保持现有认证响应 |
| 已登录但缺少 sys 管理接口权限 | 403 | `{"code":1001,"message":"登录失效或无权限访问~"}` |
| 未预期数据库或文件系统错误 | 200 | 通用业务失败消息，不泄露 SQL、绝对路径或内部错误细节 |

Node `BaseAuthorityMiddleware` 对已登录但无 admin 路由权限时设置 403，消息为 `登录失效或无权限访问~`。Go 当前 `cool/controller.WriteForbidden` 和 Comm Controller 的重复辅助函数使用 `权限不足~`，本阶段统一改为 Node 消息，并让 Comm Controller 复用统一 `WriteForbidden`，避免两份响应实现漂移。

## 6. EPS 与前端兼容

每条新增 Route 都由 Controller metadata 声明，因此 `eps.Generate` 自动把它们追加到对应 Controller 的 `api` 列表，包含大写 HTTP method、相对 path、独立 prefix 和 `ignoreToken:false`。

Vue EPS bootstrap 将自动生成：

```text
service.base.comm.personUpdate
service.base.comm.uploadMode
service.base.comm.upload
service.base.sys.user.move
service.base.sys.department.order
service.base.sys.log.clear
service.base.sys.log.setKeep
service.base.sys.log.getKeep
```

前端不新增手写 service 文件。

## 7. 测试与验收

### 7.1 单元与 metadata 测试

1. Controller metadata 覆盖全部 8 条新 Route 的 method、path、中文 summary、handler 与权限。
2. `PermissionMap` 包含 5 条 sys 管理接口，Comm 三条接口不出现细粒度权限映射。
3. `eps.Generate` 输出包含所有新增 API。
4. `PersonUpdate` 只能更新白名单字段且目标用户 ID 来自认证上下文。
5. 上传覆盖缺失文件、超限文件、安全文件名、目录创建、保存和 URL 输出。
6. 用户移动覆盖空或重复 ID、部门不存在及批量更新成功。
7. 部门排序覆盖正确更新 `parentId/orderNum`、重复或不存在 ID 失败、事务回滚。
8. 日志覆盖清空、`logKeep` 新增/更新、读取与默认值回退。
9. `WriteForbidden` 覆盖 HTTP 403、code 1001 与 Node 兼容消息。

### 7.2 HTTP 与真实 MySQL 集成测试

用独立 GoFrame server 验证：

1. admin 登录后可调用全部 8 条接口，成功响应为 code 1000。
2. 未登录请求 Comm 和 sys 新接口均为 401。
3. 受限用户请求 5 条 sys 管理接口均返回精确的 Node 兼容 403 body。
4. `/admin/base/open/eps` 返回全部新增 API metadata。
5. 显式设置 `COOL_CUSTOM_API_INTEGRATION=1` 后，在隔离的真实 MySQL 数据中验证上传、用户移动、部门排序、日志配置读写和权限拒绝。

最终验证命令：

```bash
go test ./cool/controller ./cool/eps ./cool/crud ./modules/base ./cool/app -count=1
go test ./...
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run CustomAPIIntegration -count=1
go vet ./...
git diff --check
```

## 8. 非目标复核

本设计未引入 DAO、`internal/model/do`、`internal/model/entity`、`logic`、存储适配器抽象、手写 EPS 清单或前端 service。修改只服务于首批八个接口与 Node 403 契约对齐；菜单和参数接口仍以真实前端/Node 证据为前置条件，留待后续独立阶段。
