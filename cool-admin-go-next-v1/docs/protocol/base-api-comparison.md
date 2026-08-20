# Base API Node 基准对照

日期：2026-07-23

## 1. 基准与范围

- 协议基准：`cool-admin-midway/src/modules/base` 与 `cool-admin-midway-packages/core`
- Go 实现：`cool-admin-go-next/modules/base`
- 调用方：`cool-admin-vue`
- 对齐范围：路由、HTTP 方法、请求字段、字段类型、成功响应和主要业务结果
- 本轮不实现 AI Coding：`getModuleTree`、`createCode`

除上述两个 AI Coding 接口外，Go 对外协议以 Node 实际实现为准。Go 可以保留事务、租户校验、会话轮换、敏感信息过滤等增强，但不得改变正常请求的 Node 协议形状。

## 2. 全局响应

Node `BaseController.ok(data)` 的实际规则：

```json
{"code":1000,"message":"success"}
```

仅当 `data` 为真值或严格等于 `0` 时包含 `data`。因此：

| 服务返回值 | HTTP JSON |
|---|---|
| `undefined` / `null` | 不包含 `data` |
| `0` | `data: 0` |
| `{}` / `[]` / 非空字符串 | 包含 `data` |

Go 已按该规则处理成功响应。通用及自定义 CRUD 的 `update`、`delete` 成功后均省略 `data`；`add` 保留 Node 对应返回值。

查询不存在的详情时，Node 通用 `info` 和菜单 `info` 返回 `null`，因此响应不包含 `data`；角色自定义 `info` 返回空对象，因此响应包含 `data: {}`。Go 已分别按这三种返回语义对齐。

通用 `add`、`update` 同时接受单个 object 和顶层 object[]。批量新增返回 `{id: number[]}`，批量更新仍省略 `data`。

分页统一为：

```json
{
  "list": [],
  "pagination": {"page": 1, "size": 15, "total": 0}
}
```

HTTP JSON 字段统一使用 camelCase。

分页不限制调用方传入的正数 `size`，并支持 Node 的 `order`、`sort`、逗号分隔多字段排序、`isExport`、`maxExportLimit`。配置为 `fieldEq` 的字段收到数组时使用 `IN` 查询。

## 3. Node 字段类型

| 资源 | 字段 | Node 类型 | Go 状态 |
|---|---|---|---|
| menu | `keepAlive`, `isShow` | info/list 为 boolean，page 原始 SQL为 0/1 | 已按接口分别对齐 |
| role | `relevance` | info/list 为 boolean，page 原始 SQL为 0/1 | 已按接口分别对齐 |
| role | `userId` | string | 已对齐实体与 EPS 类型 |
| role | `menuIdList`, `departmentIdList` | number[] | 已统一 info/list/page |
| user | `roleIdList`, `roleIds` | number[] | 已对齐数组形状 |
| 通用实体 | `id`, `tenantId`, 外键 ID | number / null | 已对齐 |
| 通用实体 | `createTime`, `updateTime` | varchar datetime string | 已对齐实体与 EPS 元数据 |

菜单新增省略字段时采用 Node 默认值：`type=0`、`orderNum=0`、`keepAlive=true`、`isShow=true`。角色新增省略 `relevance` 时采用 Node 默认值 `false`。

EPS 排除 `tenantId`，将 `createTime`、`updateTime` 排到末尾，并对齐 Node 的 nullable、length、defaultValue 与 text/json 类型。日志 EPS 的 `pageColumns` 同时包含 `a.*` 与联表用户字段 `b.name`。

## 4. Open

Prefix：`/admin/base/open`，均匿名访问。

| Method | Path | Node 请求 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| GET | `/eps` | 无 | admin EPS object | 已对齐；不含未实现的 AI Coding |
| GET | `/html` | query: `key` | 原始 HTML body，不套 JSON | 已对齐输出形状 |
| POST | `/login` | `username`, `password`, `captchaId`, `verifyCode` | `{token,expire,refreshToken,refreshExpire}` | 已对齐 |
| GET | `/captcha` | query: `width`, `height`, `color` | `{captchaId,data}` | 已对齐 |
| POST | `/refreshToken` | `refreshToken` | `{token,expire,refreshToken,refreshExpire}` | 已对齐输出形状 |

验证码基准：默认 `width=150`、`height=50`、`color=#fff`，4 位字母数字组合，`data` 为 `data:image/svg+xml;base64,...`，缓存 30 分钟，比较不区分大小写，成功后消费。验证码失败消息为 `验证码不正确`。

登录基准：用户不存在、密码错误或 `status===0` 返回 `账户或密码不正确~`；没有任何角色返回 `该用户未设置任何角色，无法登录~`。

## 5. Admin Comm

Prefix：`/admin/base/comm`。

| Method | Path | Node 请求 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| GET | `/person` | 无 | 当前 user object，不含 `password` | 已对齐 |
| POST | `/personUpdate` | user fields；改密用 `oldPassword`, `password` | 无 `data` | 已对齐 |
| GET | `/permmenu` | 无 | `{perms: string[], menus: object[]}` | 已对齐 |
| POST | `/upload` | multipart: `file`，可选 `key` | `<domain>/upload/YYYYMMDD/<name>` | 已对齐本地模式 |
| GET | `/uploadMode` | 无 | `{mode:"local",type:"local"}` | 已对齐 |
| POST | `/logout` | 无 | 无 `data` | 已对齐响应 |
| GET | `/program` | 无 | `"Node"` | Go 返回 `"Go"`，用于标识实际后端运行时 |

上传的 `key` 支持安全相对路径；路径遍历输入返回 `非法的文件路径`。Go 当前只提供 Node 的 local 模式，不实现云存储插件。

## 6. User

Prefix：`/admin/base/sys/user`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/add` | user fields, `roleIdList` | 新用户 ID number | 已对齐 |
| POST | `/delete` | `ids` | 无 `data` | 已对齐响应 |
| POST | `/update` | `id` + user fields；可选 `roleIdList` | 无 `data` | 已对齐；空角色数组不清空关系 |
| GET | `/info` | query: `id` | user object + `roleIdList`, `departmentName` | 已对齐且不返回密码 |
| POST | `/list` | 通用 list 查询字段 | user object[] | 结构已对齐；Go 额外隐藏密码哈希 |
| POST | `/page` | `page`, `size`, `keyWord`, `status`, `departmentIds` | user page result | 已对齐 |
| POST | `/move` | `departmentId`, `userIds` | 无 `data` | 已对齐 |

Node 当前 page 中 `roleIds` 错误地映射成 `userId`；Go 保留真实角色 ID，不复制该数据错误。

## 7. Role

Prefix：`/admin/base/sys/role`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/add` | `name`, `label`, `remark`, `relevance`, `menuIdList`, `departmentIdList` | `{id}` | 已对齐 |
| POST | `/delete` | `ids` | 无 `data` | 已对齐响应 |
| POST | `/update` | `id` + 可修改字段 | 无 `data` | 已对齐 |
| GET | `/info` | query: `id` | role object，两个权限字段为 number[] | 已对齐 |
| POST | `/list` | body 被 Node 自定义 list 忽略 | role object[] | 已对齐字段类型与权限范围 |
| POST | `/page` | `page`, `size`, `keyWord` 及分页排序/导出字段 | role page result | 已对齐权限范围；`relevance` 为 0/1 |

## 8. Menu

Prefix：`/admin/base/sys/menu`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/add` | menu fields | `{id}` | 已对齐默认值与返回 |
| POST | `/delete` | `ids` | 无 `data` | 已对齐响应及递归删除 |
| POST | `/update` | `id` + menu fields | 无 `data` | 已对齐响应 |
| GET | `/info` | query: `id` | menu object | 已对齐布尔类型 |
| POST | `/list` | body 被 Node 自定义 list 忽略 | menu object[]，含 `parentName` | 已对齐，boolean 字段为 boolean |
| POST | `/page` | 分页排序/导出字段；无菜单筛选 | menu page result | 已对齐，boolean 字段为 0/1 |
| POST | `/parse` | `entity`, `controller`, `module` | parse result object | 字段形状已对齐 |
| POST | `/create` | `module`, `entity`, `controller`, `service`, `fileName` | 无 `data` | 请求/响应已对齐；Go 生成 Go 文件 |
| POST | `/export` | `ids` | `childMenus` 菜单树 | 已对齐 |
| POST | `/import` | `menus` | 无 `data` | 已对齐 |

## 9. Department

Prefix：`/admin/base/sys/department`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/add` | `name`, `parentId`, `orderNum` | `{id}` | 已对齐；`userId` 取当前用户 |
| POST | `/delete` | `ids`, `deleteUser` | 无 `data` | 已对齐删除/迁移分支 |
| POST | `/update` | `id` + department fields | 无 `data` | 已对齐响应 |
| POST | `/list` | body 被 Node 自定义 list 忽略 | department object[]，含 `parentName` | 已对齐 |
| POST | `/order` | department order item[] | 无 `data` | 已对齐响应 |

## 10. Param

Prefix：`/admin/base/sys/param`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/add` | `keyName`, `name`, `data`, `dataType`, `remark` | `{id}` | 已对齐 |
| POST | `/delete` | `ids` | 无 `data` | 已对齐响应 |
| POST | `/update` | `id` + param fields | 无 `data` | 已对齐响应 |
| GET | `/info` | query: `id` | param object | 已复刻 Node 的 `{}` 替换后 JSON 解析规则 |
| POST | `/page` | `page`, `size`, `keyWord`, `dataType` | param page result | 已对齐 |
| GET | `/html` | query: `key` | 原始 HTML body | 已对齐输出形状 |

`dataByKey` 的 Node 类型规则：`dataType=0` 尝试 JSON 解析，`1` 返回字符串，`2` 按逗号拆分为字符串数组。

## 11. Log

Prefix：`/admin/base/sys/log`。

| Method | Path | Node 请求关键字段 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| POST | `/page` | `page`, `size`, `keyWord` 及分页排序/导出字段 | log page result，列表项含用户名 `name`，`params` 为 JSON 值 | 已对齐 |
| POST | `/clear` | 无 | 无 `data` | 已对齐响应 |
| POST | `/setKeep` | `value` | 无 `data` | 已对齐响应 |
| GET | `/getKeep` | 无 | `logKeep` 值 | 已对齐 |

Node 另有每日定时清理任务；它不是 HTTP API，Go 尚未补齐。

## 12. App Comm

Prefix：`/app/base/comm`。

| Method | Path | Node 请求 | Node 成功 data | Go 状态 |
|---|---|---|---|---|
| GET | `/param` | query: `key` | `dataByKey` 动态值 | 已对齐；仅 allowKeys 匿名 |
| GET | `/eps` | 无 | app EPS object | 已对齐 |
| POST | `/upload` | multipart: `file`，可选 `key` | `<domain>/upload/YYYYMMDD/<name>` | 已对齐本地模式，需 App Token |
| GET | `/uploadMode` | 无 | `{mode:"local",type:"local"}` | 已对齐，需 App Token |

## 13. AI Coding（本轮不做）

| Method | Path | 状态 |
|---|---|---|
| GET | `/admin/base/coding/getModuleTree` | Go 未实现，本轮明确跳过 |
| POST | `/admin/base/coding/createCode` | Go 未实现，本轮明确跳过 |

## 14. 保留的非协议安全增强

以下差异不会改变正常前端请求的字段和成功响应形状：

1. user list 不返回密码哈希。
2. admin 用户和 admin 角色具有额外修改、删除保护。
3. 用户、角色、菜单、部门关系修改使用事务并清理关联数据。
4. 客户端提交的 `tenantId` 不可信，写入时使用认证上下文。
5. 日志中的密码、Token、验证码等敏感参数会脱敏。
6. refresh token 校验会话、用户状态和密码版本并执行轮换。
7. 菜单源码解析不执行请求中的源码，并限制生成路径。

## 15. 验证要求

```bash
GOCACHE=/tmp/cool-admin-go-cache go test ./...
GOCACHE=/tmp/cool-admin-go-cache go vet ./...
```

涉及真实 HTTP listener 的测试在受限沙箱中需要允许本地临时端口。
