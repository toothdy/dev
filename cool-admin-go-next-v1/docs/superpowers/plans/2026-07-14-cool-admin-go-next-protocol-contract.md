# cool-admin-go-next Protocol Contract Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 固化 `cool-admin-vue` 与 Node 版 `cool-admin-midway` 的真实 base 协议契约，为 Go 版 `cool-admin-go-next` 后续实现提供不可猜测的接口、表结构、EPS、Token、错误码依据。

**Architecture:** 本计划只做协议勘察和文档/fixture 产出，不实现 Go 后端功能。执行者从前端消费代码和 Node 版 base 实现双向提取契约，写入 `docs/protocol/base-api-contract.md`，并保存代表性 JSON fixtures 供后续 Go 版协议测试使用。

**Tech Stack:** Markdown、JSON、Python 3 标准库校验脚本、现有 `cool-admin-vue` TypeScript/Vue 源码、现有 `cool-admin-midway` Midway/TypeScript 源码。

## Global Constraints

- 始终用中文编写说明文档。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL。
- 第一阶段采用运行时自动建表、运行时 EPS、`db.json` / `menu.json` 初始化导入。
- 第一阶段插件系统不实现，只预留扩展点。
- 目标本地目录是 `/Users/n/数据/cool-admin/cool-admin-go-next`。
- 后续远端仓库是 `https://github.com/toothdy/cool-admin-go-next`。
- 当前本地目录可能还不是 git 仓库；执行前必须检查 git 状态。
- 不使用 `git add -A`；如需要提交，只显式 stage 本计划创建或修改的文件。
- 本计划不新增 npm/pnpm/yarn/go 依赖。

---

## Scope Check

这份计划只覆盖设计文档中的“阶段 0：协议勘察”。它不实现 GoFrame 项目骨架、自动建表、seed、CRUD、auth、EPS runtime。完成本计划后，再分别创建后续实现计划：

1. Plan 1：GoFrame 项目骨架与 `cool/app` runtime。
2. Plan 2：Model metadata 与 MySQL 自动建表。
3. Plan 3：`db.json` / `menu.json` 导入。
4. Plan 4：CRUD runtime。
5. Plan 5：auth 与 base 协议实现。
6. Plan 6：EPS runtime。
7. Plan 7：Vue 前端联调。

---

## File Structure

### 创建文件

- `docs/protocol/base-api-contract.md`  
  base 协议主文档，记录前端调用、Node 路由、请求参数、响应结构、Token、权限、EPS、表结构、初始化数据。

- `docs/protocol/fixtures/login-success.json`  
  登录成功响应的代表性 fixture。

- `docs/protocol/fixtures/refresh-token-success.json`  
  刷新 Token 成功响应的代表性 fixture。

- `docs/protocol/fixtures/person-success.json`  
  个人信息接口响应的代表性 fixture。

- `docs/protocol/fixtures/permmenu-success.json`  
  权限菜单接口响应的代表性 fixture。

- `docs/protocol/fixtures/eps-admin-success.json`  
  admin EPS 接口响应的代表性 fixture。

- `docs/protocol/fixtures/crud-page-success.json`  
  通用分页接口响应的代表性 fixture。

- `docs/protocol/source-map.md`  
  协议来源映射表，记录每个契约字段来自哪个前端/Node 文件。

### 读取来源文件

前端：

- `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/service/request.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/user.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/menu.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/pages/login/index.vue`
- `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/bootstrap/eps.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/packages/vite-plugin/src/eps/index.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/packages/vite-plugin/src/virtual.ts`
- `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/config.ts`

Node：

- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/open.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/comm.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/user.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/role.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/menu.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/department.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/param.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/log.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/login.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/perms.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/user.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/role.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/menu.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/department.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/user.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/role.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/menu.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/department.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/param.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/log.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/conf.ts`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/db.json`
- `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/menu.json`

---

### Task 1: 初始化协议文档目录并确认仓库状态

**Files:**
- Create: `docs/protocol/base-api-contract.md`
- Create: `docs/protocol/source-map.md`
- Create directory: `docs/protocol/fixtures/`

**Interfaces:**
- Consumes: 设计文档 `docs/superpowers/specs/2026-07-14-cool-admin-go-next-design.md`
- Produces: 协议文档目录结构，后续任务直接写入这些文件。

- [ ] **Step 1: 检查目标目录和 git 状态**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
pwd
git rev-parse --show-toplevel
```

Expected if git is not initialized:

```text
/Users/n/数据/cool-admin/cool-admin-go-next
fatal: not a git repository (or any of the parent directories): .git
```

Expected if git is initialized:

```text
/Users/n/数据/cool-admin/cool-admin-go-next
/Users/n/数据/cool-admin/cool-admin-go-next
```

If git is not initialized, continue without committing in this plan. Do not run `git init` in this task.

- [ ] **Step 2: Create protocol directories**

Run:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures
```

Expected: command exits with code `0` and no output.

- [ ] **Step 3: Create initial `base-api-contract.md`**

Write this exact initial content to `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md`:

```markdown
# cool-admin-go-next Base API Contract

日期：2026-07-14

## 1. 契约目标

本文档记录 `cool-admin-vue` 前端与 Node 版 `cool-admin-midway` base 模块之间的真实协议。Go 版 `cool-admin-go-next` 第一阶段必须以本文档为准实现兼容后端。

## 2. 全局响应规则

成功响应外层结构：

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

前端 axios 响应拦截器在 `code === 1000` 时返回 `data` 字段；非 `1000` 时按 `{ code, message }` 进入 rejected promise。

## 3. Token 规则

请求头字段：`Authorization`。

前端发送方式：如果本地存在 token，则设置 `req.headers["Authorization"] = user.token`，不自动添加 `Bearer ` 前缀。

刷新 Token 触发条件：本地 `token` 过期且 `refreshToken` 未过期时，前端调用 `service.base.open.refreshToken({ refreshToken })`。

## 4. Open 接口

## 5. Comm 接口

## 6. Base CRUD 接口

## 7. EPS 契约

## 8. 菜单与权限契约

## 9. 表结构契约

## 10. 初始化数据契约

## 11. 错误码契约

## 12. Go 版实现约束

Go 版后端必须兼容本文档记录的路径、HTTP 方法、请求参数、响应字段、Token header、菜单结构、权限结构和 EPS 结构。
```

- [ ] **Step 4: Create initial `source-map.md`**

Write this exact initial content to `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/source-map.md`:

```markdown
# Base Protocol Source Map

日期：2026-07-14

## 前端来源

| 契约 | 文件 |
|---|---|
| axios 响应解包、错误处理、Authorization header | `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/service/request.ts` |
| token / refreshToken 本地存储与刷新调用 | `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/user.ts` |
| 权限菜单消费结构 | `/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/menu.ts` |
| EPS service 注入方式 | `/Users/n/数据/cool-admin/cool-admin-vue/src/cool/bootstrap/eps.ts` |

## Node 来源

| 契约 | 文件 |
|---|---|
| open 接口：eps/html/login/captcha/refreshToken | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/open.ts` |
| comm 接口：person/personUpdate/permmenu/upload/uploadMode/logout/program | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/comm.ts` |
| 登录、验证码、刷新 Token、JWT payload、密码 md5 | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/login.ts` |
| 权限菜单返回 `{ perms, menus }` | `/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/perms.ts` |
```

- [ ] **Step 5: Validate created files exist**

Run:

```bash
test -f /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
test -f /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/source-map.md
test -d /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures
```

Expected: all commands exit with code `0` and no output.

---

### Task 2: 固化前端 request、Token、菜单消费契约

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `docs/protocol/source-map.md`

**Interfaces:**
- Consumes: Task 1 创建的文档。
- Produces: 后续 auth、response、menu 实现必须遵守的前端消费契约。

- [ ] **Step 1: Read frontend request/token/menu files**

Read these files:

```text
/Users/n/数据/cool-admin/cool-admin-vue/src/cool/service/request.ts
/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/user.ts
/Users/n/数据/cool-admin/cool-admin-vue/src/modules/base/store/menu.ts
```

Record these exact facts in `base-api-contract.md`:

```markdown
## 2. 全局响应规则

成功响应外层结构：

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

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
  "refreshExpire": 604800
}
```

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
```

- [ ] **Step 2: Verify contract mentions Authorization without Bearer**

Run:

```bash
grep -n "不会自动添加.*Bearer" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
```

Expected output includes one matching line.

- [ ] **Step 3: Verify menu contract mentions `menus` and `perms`**

Run:

```bash
grep -n "\"menus\"" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
grep -n "\"perms\"" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
```

Expected: both commands print at least one matching line.

---

### Task 3: 固化 Node open/comm/auth 路由契约

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `docs/protocol/source-map.md`
- Create: `docs/protocol/fixtures/login-success.json`
- Create: `docs/protocol/fixtures/refresh-token-success.json`
- Create: `docs/protocol/fixtures/person-success.json`
- Create: `docs/protocol/fixtures/permmenu-success.json`

**Interfaces:**
- Consumes: Task 2 的 Token 和菜单前端契约。
- Produces: 后续 Go auth/base controller 需要实现的 open/comm 路由清单。

- [ ] **Step 1: Read Node open/comm/login/perms files**

Read these files:

```text
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/open.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/comm.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/login.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/service/sys/perms.ts
```

Record this exact route table in `base-api-contract.md`:

```markdown
## 4. Open 接口

Controller prefix：`/admin/base/open`。

| Method | Path | 忽略 Token | 请求参数 | 成功 data | 说明 |
|---|---|---:|---|---|---|
| GET | `/admin/base/open/eps` | 是 | 无 | admin EPS object | 实体信息与路径 |
| GET | `/admin/base/open/html` | 是 | query: `key` | 原始 HTML body | 根据配置参数 key 获得网页内容 |
| POST | `/admin/base/open/login` | 是 | body: `username`, `password`, `captchaId`, `verifyCode` | `{ token, expire, refreshToken, refreshExpire }` | 登录 |
| GET | `/admin/base/open/captcha` | 是 | query: `width`, `height`, `color` | `{ captchaId, data }` | 验证码，`data` 是 base64 SVG data URL |
| POST | `/admin/base/open/refreshToken` | 是 | body: `refreshToken` | `{ token, expire, refreshToken, refreshExpire }` | 刷新 token |

登录密码规则：Node 版使用 `md5(password)` 与 `base_sys_user.password` 比较。

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
| POST | `/admin/base/comm/upload` | 否 | multipart file | upload result | 文件上传 |
| GET | `/admin/base/comm/uploadMode` | 否 | 无 | string | 文件上传模式 |
| POST | `/admin/base/comm/logout` | 否 | 无 | 无 | 退出 |
| GET | `/admin/base/comm/program` | 是 | 无 | `"Node"` | 编程标识；Go 版可返回 `"Go"`，但如前端依赖该值则必须复核 |
```

- [ ] **Step 2: Create `login-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/login-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "expire": 7200,
    "token": "access.jwt.token",
    "refreshExpire": 604800,
    "refreshToken": "refresh.jwt.token"
  }
}
```

- [ ] **Step 3: Create `refresh-token-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/refresh-token-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "expire": 7200,
    "token": "new.access.jwt.token",
    "refreshExpire": 604800,
    "refreshToken": "new.refresh.jwt.token"
  }
}
```

- [ ] **Step 4: Create `person-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/person-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "id": 1,
    "username": "admin",
    "name": "管理员",
    "nickName": "管理员",
    "headImg": "",
    "phone": "",
    "email": "",
    "status": 1,
    "departmentId": 1,
    "createTime": "2026-07-14 00:00:00",
    "updateTime": "2026-07-14 00:00:00"
  }
}
```

- [ ] **Step 5: Create `permmenu-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/permmenu-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "perms": [
      "base:sys:user:page",
      "base:sys:user:add",
      "base:sys:user:update",
      "base:sys:user:delete"
    ],
    "menus": [
      {
        "id": 1,
        "parentId": null,
        "name": "系统管理",
        "router": "/system",
        "perms": null,
        "type": 0,
        "icon": "setting",
        "orderNum": 1,
        "viewPath": null,
        "keepAlive": 0,
        "isShow": true,
        "status": 1,
        "meta": {}
      },
      {
        "id": 2,
        "parentId": 1,
        "name": "用户管理",
        "router": "/system/user",
        "perms": "base:sys:user:page",
        "type": 1,
        "icon": "user",
        "orderNum": 1,
        "viewPath": "modules/base/views/user/index.vue",
        "keepAlive": 1,
        "isShow": true,
        "status": 1,
        "meta": {}
      }
    ]
  }
}
```

- [ ] **Step 6: Validate JSON fixtures**

Run:

```bash
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/login-success.json >/dev/null
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/refresh-token-success.json >/dev/null
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/person-success.json >/dev/null
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/permmenu-success.json >/dev/null
```

Expected: all commands exit with code `0` and no output.

---

### Task 4: 固化 EPS 生成与消费契约

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `docs/protocol/source-map.md`
- Create: `docs/protocol/fixtures/eps-admin-success.json`

**Interfaces:**
- Consumes: Task 2 的前端 EPS 消费方式、Task 3 的 `/admin/base/open/eps` 路由。
- Produces: 后续 Go EPS runtime 的输出结构契约。

- [ ] **Step 1: Read frontend EPS source files**

Read these files:

```text
/Users/n/数据/cool-admin/cool-admin-vue/src/cool/bootstrap/eps.ts
/Users/n/数据/cool-admin/cool-admin-vue/packages/vite-plugin/src/eps/index.ts
/Users/n/数据/cool-admin/cool-admin-vue/packages/vite-plugin/src/virtual.ts
```

Record this exact section in `base-api-contract.md`:

```markdown
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
    "keyWordLikeFields": ["a.name", "a.username"],
    "fieldEq": ["a.status", "a.departmentId"],
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
```

- [ ] **Step 2: Create `eps-admin-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/eps-admin-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "base": [
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
        "api": [
          {
            "method": "POST",
            "path": "/add",
            "summary": "新增",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          },
          {
            "method": "POST",
            "path": "/delete",
            "summary": "删除",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          },
          {
            "method": "POST",
            "path": "/update",
            "summary": "修改",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          },
          {
            "method": "GET",
            "path": "/info",
            "summary": "单个信息",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          },
          {
            "method": "POST",
            "path": "/list",
            "summary": "列表查询",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          },
          {
            "method": "POST",
            "path": "/page",
            "summary": "分页查询",
            "dts": {},
            "tag": "",
            "prefix": "/admin/base/sys/user",
            "ignoreToken": false
          }
        ],
        "columns": [
          {
            "propertyName": "id",
            "type": "int",
            "length": "",
            "comment": "ID",
            "nullable": false,
            "defaultValue": null,
            "dict": null,
            "source": "a.id"
          },
          {
            "propertyName": "username",
            "type": "varchar",
            "length": "100",
            "comment": "用户名",
            "nullable": false,
            "defaultValue": null,
            "dict": null,
            "source": "a.username"
          },
          {
            "propertyName": "status",
            "type": "int",
            "length": "",
            "comment": "状态",
            "nullable": false,
            "defaultValue": 1,
            "dict": ["禁用", "启用"],
            "source": "a.status"
          },
          {
            "propertyName": "createTime",
            "type": "datetime",
            "length": "",
            "comment": "创建时间",
            "nullable": false,
            "defaultValue": null,
            "dict": null,
            "source": "a.createTime"
          },
          {
            "propertyName": "updateTime",
            "type": "datetime",
            "length": "",
            "comment": "更新时间",
            "nullable": false,
            "defaultValue": null,
            "dict": null,
            "source": "a.updateTime"
          }
        ],
        "pageQueryOp": {
          "keyWordLikeFields": ["a.name", "a.username", "a.nickName"],
          "fieldEq": ["a.status", "a.departmentId"],
          "fieldLike": []
        },
        "pageColumns": []
      }
    ]
  }
}
```

- [ ] **Step 3: Validate EPS fixture JSON**

Run:

```bash
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/eps-admin-success.json >/dev/null
```

Expected: command exits with code `0` and no output.

- [ ] **Step 4: Verify EPS contract contains service method list**

Run:

```bash
grep -n "service.base.sys.user.page" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
```

Expected: one matching line.

---

### Task 5: 固化 base CRUD、实体、表结构和初始化数据契约

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `docs/protocol/source-map.md`
- Create: `docs/protocol/fixtures/crud-page-success.json`

**Interfaces:**
- Consumes: Task 4 EPS Column 字段规则。
- Produces: 后续 Go Model、自动建表、CRUD runtime 的字段级契约。

- [ ] **Step 1: Read Node sys controllers and entities**

Read these files:

```text
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/user.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/role.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/menu.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/department.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/param.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/controller/admin/sys/log.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/user.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/role.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/menu.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/department.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/param.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/log.ts
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/entity/sys/conf.ts
```

Append this base CRUD structure section to `base-api-contract.md`:

```markdown
## 6. Base CRUD 接口

Go 版必须实现以下 CRUD prefixes：

| 模块 | Controller prefix | 说明 | CRUD |
|---|---|---|---|
| base | `/admin/base/sys/user` | 用户管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/role` | 角色管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/menu` | 菜单管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/department` | 部门管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/param` | 参数管理 | add/delete/update/info/list/page |
| base | `/admin/base/sys/log` | 日志管理 | delete/info/list/page，具体 API 以 Node Controller 为准 |

CRUD HTTP 方法：

| API | Method | 请求位置 | 响应 |
|---|---|---|---|
| add | POST | body | 新增结果，至少包含 `id` |
| delete | POST | body: `{ ids }` | 空 data |
| update | POST | body | 空 data |
| info | GET | query: `id` | 单条数据 |
| list | POST | body | 数组 |
| page | POST | body | `{ list, pagination }` 或 Node 实际分页结构 |

Go 版必须在协议勘察中继续确认 Node 版分页返回字段名，并在 CRUD runtime 计划中固定。
```

- [ ] **Step 2: Read Node seed files**

Read:

```text
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/db.json
/Users/n/数据/cool-admin/cool-admin-midway/src/modules/base/menu.json
```

Append this initialization contract to `base-api-contract.md`:

```markdown
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
```

- [ ] **Step 3: Create `crud-page-success.json` fixture**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/crud-page-success.json`:

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "list": [
      {
        "id": 1,
        "username": "admin",
        "name": "管理员",
        "nickName": "管理员",
        "status": 1,
        "departmentId": 1,
        "createTime": "2026-07-14 00:00:00",
        "updateTime": "2026-07-14 00:00:00"
      }
    ],
    "pagination": {
      "page": 1,
      "size": 15,
      "total": 1
    }
  }
}
```

- [ ] **Step 4: Validate CRUD fixture JSON**

Run:

```bash
python3 -m json.tool /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures/crud-page-success.json >/dev/null
```

Expected: command exits with code `0` and no output.

- [ ] **Step 5: Mark pagination structure as requiring exact Node confirmation**

Add this warning line under the CRUD table in `base-api-contract.md`:

```markdown
分页 fixture 使用 `{ list, pagination }` 作为初始代表结构；实现 Go CRUD 前必须从 Node `BaseMysqlService.entityRenderPage/sqlRenderPage` 和前端表格消费代码确认真实分页字段名。如果真实字段不同，以 Node/Vue 为准更新本节和 fixture。
```

This is not an implementation placeholder; it is a protocol risk marker that blocks CRUD implementation until resolved.

---

### Task 6: 固化错误码、密码、JWT、权限风险清单

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `docs/protocol/source-map.md`

**Interfaces:**
- Consumes: Task 3 的 auth route contract。
- Produces: 后续 Go auth 和 middleware 的兼容约束。

- [ ] **Step 1: Record error code contract**

Append this section to `base-api-contract.md`:

```markdown
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
```

- [ ] **Step 2: Record password/JWT compatibility contract**

Append this section to `base-api-contract.md`:

```markdown
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
```

- [ ] **Step 3: Verify auth constraints are present**

Run:

```bash
grep -n "md5(password)" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
grep -n "Authorization: <token>" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
grep -n "refreshToken 接口失败" /Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md
```

Expected: each command prints one matching line.

---

### Task 7: 自检协议文档和 fixtures

**Files:**
- Modify: `docs/protocol/base-api-contract.md` if issues are found
- Modify: `docs/protocol/source-map.md` if missing sources are found
- Modify: `docs/protocol/fixtures/*.json` if JSON validation fails

**Interfaces:**
- Consumes: Tasks 1-6 outputs.
- Produces: Reviewed protocol contract ready for Plan 1+ implementation planning.

- [ ] **Step 1: Placeholder scan**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
root = Path('/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol')
bad_words = ['TBD', 'TODO', '填充', '待补充', '随便', '以后再说']
failed = False
for path in root.rglob('*'):
    if path.is_file() and path.suffix in {'.md', '.json'}:
        text = path.read_text(encoding='utf-8')
        for word in bad_words:
            if word in text:
                print(f'{path}: contains {word}')
                failed = True
if failed:
    raise SystemExit(1)
print('placeholder scan passed')
PY
```

Expected:

```text
placeholder scan passed
```

- [ ] **Step 2: JSON validation**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
import json
root = Path('/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/fixtures')
for path in sorted(root.glob('*.json')):
    json.loads(path.read_text(encoding='utf-8'))
    print(f'valid json: {path.name}')
PY
```

Expected output includes:

```text
valid json: crud-page-success.json
valid json: eps-admin-success.json
valid json: login-success.json
valid json: permmenu-success.json
valid json: person-success.json
valid json: refresh-token-success.json
```

- [ ] **Step 3: Required section scan**

Run:

```bash
python3 - <<'PY'
from pathlib import Path
path = Path('/Users/n/数据/cool-admin/cool-admin-go-next/docs/protocol/base-api-contract.md')
text = path.read_text(encoding='utf-8')
sections = [
    '## 1. 契约目标',
    '## 2. 全局响应规则',
    '## 3. Token 规则',
    '## 4. Open 接口',
    '## 5. Comm 接口',
    '## 6. Base CRUD 接口',
    '## 7. EPS 契约',
    '## 8. 菜单与权限契约',
    '## 10. 初始化数据契约',
    '## 11. 错误码契约',
    '## 12. Go 版实现约束',
]
missing = [s for s in sections if s not in text]
if missing:
    print('missing sections:')
    for item in missing:
        print(item)
    raise SystemExit(1)
print('required section scan passed')
PY
```

Expected:

```text
required section scan passed
```

- [ ] **Step 4: Git status and explicit staging guidance**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
```

If git is not initialized, expected:

```text
fatal: not a git repository (or any of the parent directories): .git
```

If git is initialized, expected changed files include only:

```text
?? docs/protocol/base-api-contract.md
?? docs/protocol/source-map.md
?? docs/protocol/fixtures/login-success.json
?? docs/protocol/fixtures/refresh-token-success.json
?? docs/protocol/fixtures/person-success.json
?? docs/protocol/fixtures/permmenu-success.json
?? docs/protocol/fixtures/eps-admin-success.json
?? docs/protocol/fixtures/crud-page-success.json
```

If git is initialized and the user explicitly asks to commit, stage explicitly:

```bash
git add docs/protocol/base-api-contract.md docs/protocol/source-map.md docs/protocol/fixtures/login-success.json docs/protocol/fixtures/refresh-token-success.json docs/protocol/fixtures/person-success.json docs/protocol/fixtures/permmenu-success.json docs/protocol/fixtures/eps-admin-success.json docs/protocol/fixtures/crud-page-success.json
git commit -m "docs: add base protocol contract"
```

Commit body must include:

```text
Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>
```

---

## Self-Review

### Spec coverage

This plan covers the design doc's Stage 0 requirements:

1. Read `cool-admin-vue` API consumption.
2. Read Node base Controller/Service/Entity sources.
3. Document routes, methods, params, responses.
4. Document EPS contract.
5. Document Token/header/refresh behavior.
6. Document seed/menu import assumptions.
7. Produce fixtures for protocol tests.

Implementation stages are intentionally not covered in this plan and require follow-up plans.

### Placeholder scan

The plan uses no `TBD` or `TODO` markers. The pagination warning is an explicit blocking risk marker tied to exact source files, not a placeholder.

### Type consistency

Fixture names and document paths are consistent across tasks:

1. `login-success.json`
2. `refresh-token-success.json`
3. `person-success.json`
4. `permmenu-success.json`
5. `eps-admin-success.json`
6. `crud-page-success.json`
