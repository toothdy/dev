# cool-admin-go-next Base API Contract

日期：2026-07-23

## 1. 契约目标

本文档记录 `cool-admin-vue` 前端与 Node 版 `cool-admin-midway` base 模块之间的真实协议。Go 版 `cool-admin-go-next` 第一阶段必须以本文档为准实现兼容后端。

## 2. 全局响应规则

成功响应外层结构（存在业务数据时）：

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

Node `ok(data)` 仅在 `data` 为真值或严格等于 `0` 时输出 `data`。`update`、`delete` 和无返回值的自定义操作成功时，响应为：

```json
{
  "code": 1000,
  "message": "success"
}
```

不存在的通用详情及菜单详情返回成功且不包含 `data`；不存在的角色详情返回成功并包含 `data: {}`。

前端 axios 响应拦截器规则：

1. 如果 `res.data` 不存在，直接返回原始 response。
2. 如果 `res.data.code` 不存在，返回 `res.data`。
3. 如果 `code === 1000`，返回 `data`。
4. 其他 `code` 返回 rejected promise，形状为 `{ code, message }`。
5. HTTP 状态码 `401` 会触发 `user.logout()`。
6. 非开发环境下，HTTP `403/500/502` 会跳转错误页。

## 3. Token 规则

前端本地保存：

| 字段 | 来源 | 用途 |
|---|---|---|
| `token` | 登录或刷新接口返回 | 请求 header `Authorization` |
| `expire` | 登录或刷新接口返回 | 本地 token 过期时间 |
| `refreshToken` | 登录或刷新接口返回 | token 过期后的刷新凭证 |
| `refreshExpire` | 登录或刷新接口返回 | 本地 refreshToken 过期时间 |

请求 header：

```text
Authorization: <token>
language: <当前语言>
```

前端不会自动添加 `Bearer ` 前缀。

刷新 Token 调用：

```ts
service.base.open.refreshToken({
   refreshToken: storage.get("refreshToken"),
})
```

刷新成功后，前端期望响应 data 至少包含：

```json
{
  "token": "new-access-token",
  "expire": 7200,
  "refreshToken": "new-refresh-token",
  "refreshExpire": 1296000
}
```

## 4. Open 接口

Controller prefix：`/admin/base/open`。

| Method | Path | 忽略 Token | 请求参数 | 成功 data | 说明 |
|---|---|---:|---|---|---|
| GET | `/admin/base/open/eps` | 是 | 无 | admin EPS object | 实体信息与路径 |
| GET | `/admin/base/open/html` | 是 | query: `key` | 原始 HTML body | 根据配置参数 key 获得网页内容 |
| POST | `/admin/base/open/login` | 是 | body: `username`, `password`, `captchaId`, `verifyCode` | `{ token, expire, refreshToken, refreshExpire }` | 登录 |
| GET | `/admin/base/open/captcha` | 是 | query: `width`, `height`, `color` | `{ captchaId, data }` | 验证码，`data` 是 base64 SVG data URL |
| POST | `/admin/base/open/refreshToken` | 是 | body: `refreshToken` | `{ token, expire, refreshToken, refreshExpire }` | 刷新 token |

图片验证码实现规则：

1. `GET /admin/base/open/captcha` 使用 `height`、`width`、`color` query 参数；Node 默认分别为 `50`、`150`、`#fff`。
2. 成功 data 是 `{ captchaId, data }`；`data` 是以 `data:image/svg+xml;base64,` 开头的非空 SVG Data URL。
3. 验证码为 4 位字母数字组合，服务端缓存 30 分钟。
4. 登录必须传入 `captchaId` 与 `verifyCode`；比较不区分大小写。缺失、过期或错误时返回 `验证码不正确`。
5. 正确匹配会在密码校验前立即消费验证码；错误匹配保留验证码以便用户重试。

登录密码规则：Node 版使用 `md5(password)` 与 `base_sys_user.password` 比较。

用户 `status === 0` 与密码错误统一返回 `账户或密码不正确~`；用户没有任何角色时返回 `该用户未设置任何角色，无法登录~`。

JWT payload 字段：

| 字段 | 说明 |
|---|---|
| `isRefresh` | 是否刷新 token |
| `roleIds` | 当前用户角色 ID 数组 |
| `username` | 用户名 |
| `userId` | 用户 ID |
| `passwordVersion` | 用户密码版本 |
| `tenantId` | 租户 ID |

刷新 token 失败时，Node 版设置 HTTP 状态码 `401`，响应 body：

```json
{
  "code": 1001,
  "message": "登录失效~"
}
```

## 5. Comm 接口

Controller prefix：`/admin/base/comm`。

| Method | Path | 忽略 Token | 请求参数 | 成功 data | 说明 |
|---|---|---:|---|---|---|
| GET | `/admin/base/comm/person` | 否 | 无 | user object | 当前登录用户个人信息 |
| POST | `/admin/base/comm/personUpdate` | 否 | body: user object | 无 | 修改个人信息 |
| GET | `/admin/base/comm/permmenu` | 否 | 无 | `{ perms, menus }` | 权限与菜单 |
| POST | `/admin/base/comm/upload` | 否 | multipart: `file`，可选 `key` | `<domain>/upload/YYYYMMDD/<name>` | 文件上传 |
| GET | `/admin/base/comm/uploadMode` | 否 | 无 | `{mode:"local",type:"local"}` | 文件上传模式 |
| POST | `/admin/base/comm/logout` | 否 | 无 | 无 | 退出 |
| GET | `/admin/base/comm/program` | 是 | 无 | `"Go"` | 实际后端运行时标识 |

## 6. Base CRUD 接口

Go 版必须实现以下 CRUD prefixes：

| 模块 | Controller prefix | 说明 | CRUD / 自定义 API |
|---|---|---|---|
| base | `/admin/base/sys/user` | 用户管理 | add/delete/update/info/list/page，另有 `POST /move` |
| base | `/admin/base/sys/role` | 角色管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/menu` | 菜单管理 | add/delete/update/info/list/page，另有 `POST /parse`、`POST /create`、`POST /export`、`POST /import` |
| base | `/admin/base/sys/department` | 部门管理 | add/delete/update/list，另有 `POST /order` |
| base | `/admin/base/sys/param` | 参数管理 | add/delete/update/info/page，另有 `GET /html` |
| base | `/admin/base/sys/log` | 日志管理 | page，另有 `POST /clear`、`POST /setKeep`、`GET /getKeep` |

CRUD HTTP 方法：

| API | Method | 请求位置 | 响应 |
|---|---|---|---|
| add | POST | body object 或 object[] | 通常为 `{id}`；批量为 `{id: number[]}`；user add 为 ID number |
| delete | POST | body: `{ ids }` | 不包含 `data` |
| update | POST | body object 或 object[] | 不包含 `data` |
| info | GET | query: `id` | 单条数据 |
| list | POST | body | 数组 |
| page | POST | body | `{ list, pagination }` 或 Node 实际分页结构 |

分页请求支持 `page`、`size`、`order`、`sort`、`isExport`、`maxExportLimit`。`order/sort` 支持数量相同的逗号分隔多字段；普通分页不额外限制 `size`，以兼容前端的 `size: 10000` 选择器。

## 7. EPS 契约

前端 EPS bootstrap 会遍历 `eps.service`，如果某个节点存在 `namespace`，则为该节点的每个 `{ path, method }` 项生成 request 方法。

生成规则：

1. `method` 默认值是 `get`。
2. `method.toLowerCase() == "post"` 时，请求体使用 `data`。
3. 其他 method 使用 `params`。
4. 生成后的 service 会合并到全局 `service` 对象。

Go 版 `/admin/base/open/eps` 必须提供足够信息，让前端生成：

```ts
service.base.open.login
service.base.open.refreshToken
service.base.open.captcha
service.base.open.eps
service.base.comm.person
service.base.comm.permmenu
service.base.sys.user.page
service.base.sys.user.add
service.base.sys.user.update
service.base.sys.user.delete
service.base.sys.user.info
service.base.sys.user.list
```

admin EPS 中单个 Controller 建议结构：

```json
{
  "module": "base",
  "name": "BaseSysUserEntity",
  "prefix": "/admin/base/sys/user",
  "info": {
    "type": {
      "name": "user",
      "description": "用户管理"
    }
  },
  "api": [],
  "columns": [],
  "pageQueryOp": {
    "keyWordLikeFields": [],
    "fieldEq": [],
    "fieldLike": []
  },
  "pageColumns": []
}
```

Column 字段：

| 字段 | 说明 |
|---|---|
| `propertyName` | 前端字段名，使用 camelCase |
| `type` | 字段类型 |
| `length` | 字段长度 |
| `comment` | 字段注释 |
| `nullable` | 是否可空 |
| `defaultValue` | 默认值 |
| `dict` | 字典数组 |
| `source` | 前端查询源字段，使用 `a.camelCaseName` |

Go 内部 DB 字段可以使用 snake_case，但 EPS 必须输出前端 camelCase。

EPS 按 Node 规则排除 `tenantId`，并将 `createTime`、`updateTime` 放在字段列表末尾。Base 时间列的实体元数据类型为 `varchar`。

## 8. 菜单与权限契约

前端调用：

```ts
service.base.comm.permmenu()
```

前端期望返回：

```json
{
  "menus": [],
  "perms": []
}
```

`menus` 规则：

1. 前端会过滤 `type != 2` 的项生成路由和菜单。
2. 前端使用 `router || String(id)` 计算路径。
3. `isShow === undefined` 时视为 `true`。
4. 前端使用 `name`、`id` 生成路由名称：`${name}-${id}`。
5. 前端读取 `meta`，并补充 `meta.label = name`、`meta.keepAlive = keepAlive || 0`。
6. 前端使用 `orderNum` 排序菜单树。

`perms` 规则：

1. `perms` 是字符串数组。
2. 前端会把权限字符串中的 `:` 替换为 `/` 后匹配 EPS service namespace。
3. Go 后端必须返回与 Node 版一致的权限字符串格式。

## 9. 表结构契约

第一阶段至少需要兼容这些 base 表：

| 表 | 关键字段 | 说明 |
|---|---|---|
| `base_sys_user` | `id`, `departmentId`, `userId`, `name`, `username`, `password`, `passwordV`, `nickName`, `headImg`, `phone`, `email`, `remark`, `status`, `socketId`, `createTime`, `updateTime`, `tenantId` | 系统用户 |
| `base_sys_role` | `id`, `userId`, `name`, `label`, `remark`, `relevance`, `menuIdList`, `departmentIdList`, `createTime`, `updateTime`, `tenantId` | 系统角色 |
| `base_sys_menu` | `id`, `parentId`, `name`, `router`, `perms`, `type`, `icon`, `orderNum`, `viewPath`, `keepAlive`, `isShow`, `createTime`, `updateTime`, `tenantId` | 菜单和按钮权限 |
| `base_sys_department` | `id`, `name`, `userId`, `parentId`, `orderNum`, `createTime`, `updateTime`, `tenantId` | 部门 |
| `base_sys_param` | `id`, `keyName`, `name`, `data`, `dataType`, `remark`, `createTime`, `updateTime`, `tenantId` | 系统参数 |
| `base_sys_log` | `id`, `userId`, `action`, `ip`, `params`, `createTime`, `updateTime`, `tenantId` | 系统日志 |
| `base_sys_conf` | `id`, `cKey`, `cValue`, `createTime`, `updateTime`, `tenantId` | 系统配置和初始化标记 |
| `base_sys_user_role` | `userId`, `roleId` | 用户角色关联，来自初始化数据和权限逻辑 |

字段命名兼容要求：

1. HTTP JSON 和 EPS 字段使用 Node camelCase，例如 `createTime`、`departmentId`、`orderNum`。
2. Go 内部数据库字段可使用 snake_case，但必须通过 metadata 映射回 camelCase。
3. `BaseEntity` 字段 `id/createTime/updateTime/tenantId` 不应在业务 model 中重复定义。
4. `password` 不应出现在用户 info/page 的默认返回中。

## 10. 初始化数据契约

Go 版第一阶段沿用 Node 风格初始化文件：

```text
modules/base/db.json
modules/base/menu.json
```

导入规则：

1. 按模块顺序导入。
2. 使用 `base_sys_conf` 写入 `init_db_base` 和 `init_menu_base` 标记，避免重复导入。
3. 支持 `@childDatas` 子数据。
4. 支持子数据字段引用父级字段，例如字符串值以 `@` 开头时读取父级对象字段。
5. 导入完成后，数据库中必须存在可登录的 `admin` 用户、基础角色、基础菜单。

Node 初始化数据确认：

1. `base_sys_user` 默认 admin 密码为 `e10adc3949ba59abbe56e057f20f883e`，对应 `md5("123456")`。
2. `base_sys_role` 默认超管角色 `label` 为 `admin`。
3. `base_sys_conf` 默认包含 `logKeep` 和 `recycleKeep`。
4. `base_sys_department` 默认包含 `COOL`、`开发`、`测试`、`游客`。
5. `menu.json` 使用树结构 `childMenus` 表达父子菜单，按钮权限 `type` 为 `2`。

## 11. 错误码契约

已确认前端响应拦截器只把 `code === 1000` 当成功，其它 code 进入 rejected promise。

当前必须兼容：

| 场景 | HTTP Status | Body code | Body message |
|---|---:|---:|---|
| 成功 | 200 | 1000 | `success` |
| 通用业务失败 | 200 | 1001 | 业务错误消息 |
| refreshToken 失效 | 401 | 1001 | `登录失效~` |
| HTTP 未授权 | 401 | 可为空 | 前端执行 logout |
| HTTP 禁止访问 | 403 | 可为空 | 非开发环境跳转 `/403` |
| HTTP 服务错误 | 500 | 可为空 | 非开发环境跳转 `/500` |
| HTTP 网关错误 | 502 | 可为空 | 非开发环境跳转 `/502` |

## 12. Go 版实现约束

认证兼容约束：

1. 密码校验必须兼容 Node 版 `md5(password)`。
2. 登录成功必须返回 `token`、`expire`、`refreshToken`、`refreshExpire`。
3. JWT payload 必须包含 `isRefresh`、`roleIds`、`username`、`userId`、`passwordVersion`、`tenantId`。
4. 前端请求 header 使用 `Authorization: <token>`，Go 版不能只支持 `Bearer <token>`。
5. refreshToken 接口失败时必须返回 HTTP `401`，否则前端不会立即 logout。
6. `admin` 用户必须拥有全部菜单和权限。
7. 普通用户权限通过角色、菜单关系计算。
8. 权限菜单接口必须返回 `{ perms, menus }`。

Go 版后端必须兼容本文档记录的路径、HTTP 方法、请求参数、响应字段、Token header、菜单结构、权限结构和 EPS 结构。
