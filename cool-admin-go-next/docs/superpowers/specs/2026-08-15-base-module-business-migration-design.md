# Base 业务模块迁移设计

> 日期：2026-08-15
> 状态：实施中
> 目标：将 `cool-admin-midway/src/modules/base` 的业务能力迁移到 `cool-admin-go-next`
> 方案：契约优先、纵向迁移

## 1. 文档定位

本文档是 Base 业务模块的迁移设计，不是 Go v2 框架架构文档。它回答以下问题：

1. Midway Base 模块的哪些业务功能必须保留；
2. 这些功能在 Go v2 中的实体、Service、Controller 和运行组件边界；
3. 哪些通用能力必须先在 `cool-next` 中最小补齐；
4. 如何验证 HTTP 契约、权限、数据库和安全边界。

事实来源按以下顺序解释：

1. Midway `src/modules/base` 的 Controller、Service、Entity、`db.json` 和 `menu.json` 实际行为；
2. `cool-admin-vue` 对 Base 接口的实际调用，用于确认前端正在依赖的参数名、响应字段和单位；
3. 本设计明确列出的 Go 版有意差异；
4. Go v2 已批准的架构、身份上下文、密码边界和静态装配文档；
5. `cool-admin-go-next-v1/modules/base` 的验证码实现及已验证的安全经验。

当文档描述与 Midway Base 源码冲突时，默认以源码为准；只有“Go 版有意差异”中明确列出的项目可以改变外部行为。不得根据 Midway 的类型声明孤立推断契约，例如 `LoginDTO.verifyCode` 声明为 `number`，但前端提交字符串且登录服务调用 `toLowerCase()`，Go DTO 必须按实际运行行为使用字符串。

v1 不是架构源。不复制 v1 的旧 Controller DSL、运行时注册器、多租户逻辑或旧异常体系。

## 2. 目标与非目标

### 2.1 目标

1. 保留 Midway Base 模块的后台和 App 业务功能；
2. 保留前端可见的路径、HTTP 方法、参数名、响应字段和 EPS 信息；
3. 使用 Go v2 已有的 Entity Descriptor、CRUD、Auth、Session、HTTP 和静态装配能力；
4. 支持 MySQL、PostgreSQL 和 SQLite，业务层不编写单一数据库专用 SQL；
5. 保持系统日志与操作日志完全分离；
6. 修复已确认的安全问题和明显缺陷，不复制 Node 实现错误。

### 2.2 非目标

1. 不实现多租户；
2. 不增加 `tenantId` 字段、配置、过滤器、索引或兼容分支；
3. 不重写 Go v2 已有的异常、认证、响应、CRUD 或事务框架；
4. 不同时迁移 Plugin、Task、Dict 等独立业务模块；
5. 不为未确认的云存储、分布式验证码或通用代码工作台预留抽象。

### 2.3 Go 版有意差异

以下差异不复制 Midway 的缺陷，并必须由测试固定：

1. 密码从 MD5 改为 bcrypt cost 12，初始密码仍为 `123456`；
2. `verifyCode` 使用字符串 DTO，验证码校验采用进程内互斥的一次性消费；
3. Refresh Token 原子轮换，登出、密码或权限变化可靠撤销 Session；
4. 平台超级管理员以 `role.label == "admin"` 判定，不再依赖 `username == "admin"` 或固定 ID；
5. 修复 Midway 用户分页把 `userId` 写入 `roleIds` 的错误，返回真实角色 ID；
6. 个人资料更新改用字段白名单，禁止 Midway Entity 整体入参造成的越权字段写入；
7. `/admin/base/sys/menu/parse`、`/create` 和 `/admin/base/coding/**` 只在开发环境安装，只静态解析或生成 Go 源码，不执行客户端源码；
8. 本地上传保留现有参数和返回值形状，但增加大小、路径、原子落盘和安全响应头校验；
9. App 上传必须使用 App 身份，不复制 Midway Base 权限中间件只检查 `/admin/**` 而使 App 上传缺少身份校验的行为；
10. 菜单和部门增加隐藏的 `seedKey` 维护字段，用于在不依赖固定数据库 ID 的情况下幂等对齐初始化数据；
11. HTML 缓存未命中时查库并回填；操作日志增加路由过滤、大小限制和敏感字段脱敏，不复制 Midway 的缓存未命中和原始请求体记录缺陷；
12. `/admin/base/comm/program` 返回 `Go`，不返回 Midway 源码中的 `Node`。

## 3. 总体方案

采用“契约优先、纵向迁移”：先补齐 Base 运行必需的最小通用缺口，再按可独立验收的业务闭环逐步迁移。

```text
Midway HTTP 契约
        |
        v
Go v2 Controller / DTO / EPS
        |
        v
Base Service 业务规则
        |
        +--> cool-next Auth / Session / Exception
        +--> cool-next CRUD / DB Runtime / Transaction
        +--> GoFrame gcache / gcron / ghttp / glog
        |
        v
base_sys_* 业务表
```

纵向闭环顺序为：

1. 通用框架缺口与 Base 模块骨架；
2. 数据模型、Schema 和幂等初始化；
3. 登录、验证码、Session 和权限闭环；
4. 用户、角色、菜单和部门管理；
5. 参数、HTML、上传和操作日志；
6. EPS、i18n、菜单工具和开发环境代码生成；
7. 三数据库、全量回归和 Midway 契约对照。

## 4. 架构与边界

### 4.1 `cool-next` 职责

`cool-next` 只提供可被其他模块复用的通用能力：

1. 静态生成的 HTTP Installer 真正安装业务路由；
2. Handler 支持有 DTO 和无 DTO 两种签名；
3. HTTP 层支持 HTML 和文件等原始响应；
4. Entity Descriptor 支持 JSON slice/map 字段；
5. 公共 Session 端口支持按身份种类和用户 ID 撤销全部 Session；
6. 生成装配接入数据库、Auth、HTTP、模块初始化和生命周期；
7. 通用的 `db.json` / `menu.json` 初始化入口。

上述能力不知道 Base 的角色、菜单、操作日志或初始账号规则。

### 4.2 `modules/base` 职责

Base 模块拥有：

1. 10 张 `base_sys_*` 表的 Entity Descriptor；
2. Base DTO、Controller、Service 和权限 Authorizer；
3. 登录、验证码、个人资料、权限菜单和数据权限规则；
4. `db.json`、`menu.json` 的 Base 业务初始数据；
5. 参数缓存、本地上传及公开文件读取路由、操作日志、i18n 和开发代码生成；
6. 密码、管理员保护、Session 撤销和关系表一致性规则。

业务逻辑放在 `service/`，不新建 `logic/`。不手工修改生成文件，所有静态注册与装配代码由 `cool generate` 生成。

## 5. 数据模型

### 5.1 表名

Base 模块仅使用以下 10 张表，全部带 `base_sys_` 前缀：

| 表 | 用途 |
| --- | --- |
| `base_sys_user` | 后台用户 |
| `base_sys_role` | 角色与数据权限配置 |
| `base_sys_menu` | 目录、菜单、按钮与权限标识；初始化记录带隐藏的 `seedKey` |
| `base_sys_department` | 部门树；初始化记录带隐藏的 `seedKey` |
| `base_sys_param` | 公开参数、富文本和文件参数 |
| `base_sys_conf` | Base 内部配置，如操作日志保留天数 |
| `base_sys_log` | 后台操作日志，不是系统运行日志 |
| `base_sys_user_role` | 用户与角色关系 |
| `base_sys_role_menu` | 角色与菜单关系 |
| `base_sys_role_department` | 角色与部门关系 |

所有表使用 Go v2 的 `id/createTime/updateTime` 基础字段，不存在 `tenantId`。

### 5.2 关系权威性

1. `base_sys_user_role` 是用户角色的唯一权威来源；
2. `base_sys_role_menu` 是角色菜单的唯一权威来源；
3. `base_sys_role_department` 是角色部门的唯一权威来源；
4. `base_sys_role.menuIdList` 和 `departmentIdList` 仅用于保持 Node/EPS 的请求与响应契约，不参与授权判定；
5. 更新角色时，关系表与兼容字段在同一事务内同步。

### 5.3 字段与查询契约

Entity 保留 Midway 业务字段及默认值。`seedKey` 是 Go 版新增的 nullable unique 系统维护字段，只用于识别初始化记录，不接受普通 CRUD、菜单导入或导出写入。由于现有 EPS 会编译 Descriptor 的全部字段，`seedKey` 必须在 Entity Descriptor 中标记为 hidden 和 readonly；它会作为隐藏维护字段存在于 EPS 元数据中，但不进入普通业务响应。用户自行创建的菜单和部门保持 `seedKey == null`。

虚拟字段不写入数据库，并按接口保持以下响应契约：

- 用户 `page` 的每条记录补充 `departmentName`、`roleIds` 和 `roleName`，其中 `roleIds` 必须是真实角色 ID，`roleName` 是按角色 ID 稳定排序后以逗号连接的角色名称；
- 用户 `info` 补充 `departmentName` 和 `roleIdList`；
- 菜单查询补充 `parentName` 和 `childMenus`；
- 部门查询补充 `parentName`；
- 操作日志分页左连用户表补充用户 `name`。

密码字段只允许写入，任何用户列表、详情、个人信息和 EPS 响应都不得返回密码摘要。

### 5.4 初始数据

1. 首次安装时完整载入 `db.json` 和 `menu.json`，后续启动只执行下述有明确对齐键的幂等校验与补齐，不按固定数据库 ID 重建业务数据；
2. 用户、角色、参数和内部配置分别以 `username`、`label`、`keyName` 和 `cKey` 对齐；初始化部门和菜单在 JSON 中携带显式、非空且各表内唯一的 `seedKey`，禁止从名称、排序位置或数据库 ID 推导；
3. 初始化器按部门 `seedKey` 建立源部门 ID 到实际数据库 ID 的映射，解析部门父子关系和初始账号部门；菜单按嵌套树从父到子写入并使用父节点实际 ID；用户角色关系按 `username` 和 `label` 解析，不假设任何自增 ID 在新数据库中固定；
4. 已有 `seedKey` 记录以初始化 JSON 为权威，同步名称、父级、路由、权限、类型、显示和排序等种子字段，因此对种子记录的本地修改会在下次初始化时恢复；用户创建且 `seedKey == null` 的记录不受影响；从初始化 JSON 删除种子项不触发启动期自动删除，删除旧菜单、部门及其关系必须通过显式数据库迁移完成；
5. `menu.json` 保留完整菜单树，包括对后续业务模块的菜单引用；
6. 导入旧 `db.json` 时将 JSON 字段中的字符串 `"null"` 规范化为空列表，不把字符串写入 JSON 列；
7. 平台超级管理员以 `base_sys_role.label == "admin"` 判定，不依赖用户名或固定 ID；
8. 初始账号保持 `admin/123456`，仅在账号不存在并首次插入时生成 bcrypt cost 12 摘要；后续启动不得重置已有账号的密码、`passwordV` 或个人资料；
9. 参数、内部配置、用户和角色已有记录不由启动初始化覆盖管理员运行期修改，只补齐缺失记录和必要关系；
10. 初始管理员、`admin` 角色和用户角色关系必须同时存在。

## 6. 认证、Session 与授权

### 6.1 登录流程

```text
校验登录 DTO
-> 校验并消费验证码
-> 查询用户
-> 校验状态和 bcrypt 密码
-> 读取权威用户角色关系
-> 构造 Go v2 Identity
-> 创建 Session 和 Token Pair
```

登录请求保持 `username/password/captchaId/verifyCode` 参数名，四个字段均为必填字符串。账号不存在、密码错误或账号禁用使用同一对外提示，不泄露账号状态。未配置任何角色的用户不允许登录。

登录和刷新不得直接序列化 `auth.TokenPair`。两者共用专用 HTTP 响应 DTO，统一响应的业务数据固定为：

```json
{
  "token": "<access-token>",
  "expire": 7200,
  "refreshToken": "<refresh-token>",
  "refreshExpire": 1296000
}
```

`expire` 和 `refreshExpire` 是从响应时刻开始计算的 TTL 秒数，不是 Unix 时间戳或 Go `time.Time`。刷新请求体固定为 `{ "refreshToken": "..." }`，刷新成功返回相同结构。

### 6.2 验证码

验证码直接迁移 v1 已验证的业务实现：

1. 4 位安全随机字符；
2. 默认宽 150、高 50、颜色 `#fff`；
3. 校验宽高和颜色，非法值回退默认值；
4. 使用安全随机 `captchaId`；
5. 返回 `{ captchaId, data }`，`data` 为 `data:image/svg+xml;base64,...`；
6. 使用 GoFrame `gcache`，缓存键为 `captcha:{captchaId}`，有效期 30 分钟；
7. 校验时忽略大小写；
8. 读取、比较和成功删除必须在登录服务的同一进程内互斥区完成，同一验证码并发校验最多一次成功；
9. 匹配成功后立即删除，同一验证码只能成功使用一次；
10. 匹配失败时保留缓存，允许用户重新输入。

仅适配 Go v2 的模块装配、缓存和异常接口，不带入 v1 旧架构。首版 `gcache` 验证码是单实例状态；多实例部署必须保证验证码生成与登录请求落到同一实例，本次迁移不实现分布式验证码。

### 6.3 Session 规则

1. 使用 Go v2 现有 Session Store 和 JWT 能力；
2. Refresh Token 必须原子轮换，旧 Refresh Token 成功使用后立即失效；
3. 登出撤销当前 Session；
4. 用户禁用、密码修改、用户角色变化、角色菜单变化后，撤销所有受影响 Session；
5. 修改密码必须校验旧密码，成功后递增 `passwordV`；
6. 公共 Session 端口提供 `RevokeUser(context.Context, Kind, uint64) error` 和 `RevokeUsers(context.Context, Kind, []uint64) error`；单用户方法复用批量方法，多用户权限变化必须一次批量撤销，不直接依赖 Redis 或 Memory Store 具体实现；
7. Session 撤销失败时，与之相关的权限或密码数据库变更不得提交；
8. Session Store 与业务数据库不声明分布式原子性：Session 撤销成功后若数据库提交失败，允许用户被额外登出；不允许数据库权限已提交但旧 Session 仍有效。
9. 登录和刷新在数据库事务内按用户 ID 锁定 `base_sys_user` 行，重新读取用户、状态、`passwordV` 和权威角色关系，并在释放锁前完成 Session 保存或轮换；
10. 用户角色变化、角色菜单或部门变化、菜单权限变化等授权写操作必须与登录和刷新使用同一套数据库锁边界：先锁定被修改的角色或菜单记录以稳定关系集合，再计算受影响用户，按用户 ID 升序锁定用户行，完成数据库写入和 Session 撤销后再提交；所有批量锁都先去重并按 ID 升序，避免死锁；
11. Redis 的 `RevokeUsers` 对全部目标用户构造集合，只扫描一次 Session Key，并批量删除匹配项；禁止在数据库事务内为每个用户分别扫描全部 Session。

### 6.4 授权与数据权限

Base `PermissionService` 实现现有 `auth.Authorizer`：

1. `role.label == "admin"` 拥有平台级权限；
2. 普通用户的接口权限来自角色关联菜单的 `perms`；
3. 逗号分隔的权限标识解析为独立权限项；
4. `/admin/base/comm/**` 只需后台身份，不进行菜单权限拦截；
5. `/admin/base/sys/**` 与 `/admin/base/coding/**` 按静态路由权限规则校验；
6. App 上传使用 App 身份，不接受后台 Session 代替；
7. 首版数据权限只取 `base_sys_role_department` 直接关联部门的并集，不自动加入父部门或子部门；`relevance` 继续保留读写和 EPS 契约，但本次迁移不赋予新行为；
8. 非管理员的用户分页和部门列表使用相同范围：直接关联部门内的数据，或 `userId` 等于当前用户 ID 的自建数据；没有直接关联部门时只能看到自建数据；
9. `admin` 角色可查看全部部门和用户；
10. 角色可见性依据权威管理员角色和创建关系，不以 `username == "admin"` 判定超管。

### 6.5 管理员保护

1. 不允许删除或禁用最后一个绑定 `admin` 角色的有效用户；
2. 不允许删除、改名或使 `admin` 角色失去平台管理能力；
3. 判定和更新使用数据库事务与必要的锁或条件写，防止并发请求同时绕过“最后一个管理员”校验。

## 7. HTTP 契约

### 7.1 开放接口

| 路径 | 方法 | 认证 | 功能 |
| --- | --- | --- | --- |
| `/admin/base/open/eps` | GET | 公开 | 后台 EPS |
| `/admin/base/open/html` | GET | 公开 | 按参数键返回原始 HTML |
| `/admin/base/open/login` | POST | 公开 | 后台登录 |
| `/admin/base/open/captcha` | GET | 公开 | v1 图形验证码 |
| `/admin/base/open/refreshToken` | POST | 公开 | 原子刷新 Token Pair |
| `/upload/{date}/{name}` | GET | 公开 | 受控读取本地上传文件 |

### 7.2 后台通用接口

| 路径 | 方法 | 认证 | 功能 |
| --- | --- | --- | --- |
| `/admin/base/comm/person` | GET | 后台身份 | 当前用户信息 |
| `/admin/base/comm/personUpdate` | POST | 后台身份 | 修改个人资料或密码 |
| `/admin/base/comm/permmenu` | GET | 后台身份 | 当前权限与菜单树 |
| `/admin/base/comm/upload` | POST | 后台身份 | 文件上传 |
| `/admin/base/comm/uploadMode` | GET | 后台身份 | 上传模式 |
| `/admin/base/comm/logout` | POST | 后台身份 | 撤销当前 Session |
| `/admin/base/comm/program` | GET | 公开 | 返回 `Go` |

`personUpdate` 使用业务 DTO 白名单，只接受 `name`、`nickName`、`headImg`、`phone`、`email`、`password` 和 `oldPassword`。`password` 非空时 `oldPassword` 必填并校验旧密码；两者不持久化为普通资料字段。不接受角色、状态、部门、创建者或 `passwordV` 等管理字段。

用户查询响应不得混用字段：`page.list[]` 使用 `roleIds/roleName`，`info` 使用表单回填字段 `roleIdList`。修复 Midway 将用户 ID 错写进 `roleIds` 的缺陷不视为契约破坏。

### 7.3 App 接口

| 路径 | 方法 | 认证 | 功能 |
| --- | --- | --- | --- |
| `/app/base/comm/param` | GET | 公开 | 只读 `allowKeys` 允许的参数 |
| `/app/base/comm/eps` | GET | 公开 | App EPS |
| `/app/base/comm/upload` | POST | App 身份 | 文件上传 |
| `/app/base/comm/uploadMode` | GET | App 身份 | 上传模式 |

### 7.4 管理 CRUD 与自定义接口

| 前缀 | CRUD | 自定义接口 |
| --- | --- | --- |
| `/admin/base/sys/user` | add/delete/update/info/list/page | `POST /move` |
| `/admin/base/sys/role` | add/delete/update/info/list/page | 无 |
| `/admin/base/sys/menu` | add/delete/update/info/list/page | `POST /parse`、`/create`、`/export`、`/import` |
| `/admin/base/sys/department` | add/delete/update/list | `POST /order` |
| `/admin/base/sys/param` | add/delete/update/info/page | `GET /html` |
| `/admin/base/sys/log` | page | `POST /clear`、`/setKeep`，`GET /getKeep` |

CRUD 和已有菜单按钮使用对应的 `base:sys:<resource>:<action>` 权限标识。Midway `menu.json` 没有为菜单 `parse/create/export/import` 配置普通权限，因此四个自定义接口仅允许 `admin` 角色访问，不为普通角色虚构新权限标识。动态查询必须使用现有 CRUD Query AST 和参数化条件，不拼接客户端字符串。

菜单导入请求为 `{ "menus": [...] }`，导出请求为 `{ "ids": [...] }`；导出树不包含数据库 `id/createTime/updateTime/parentId`，也不包含 Go 版维护字段 `seedKey`，导入忽略这些字段并重新建立实际父子 ID。

菜单解析保持 `entity/controller/module` 参数名，`entity` 和可选的 `controller` 在 Go 版中是待静态解析的 Go 源码。响应保持 Midway Service 返回的 `columns/className/tableName/fileName/path` 字段组合。菜单代码创建保持 `module/entity/controller/service/fileName` 参数名，只生成经过解析、格式化和路径校验的 Go 文件。`parse/create` 仅在开发环境安装；生产环境请求必须得到 404，而不是仅在 Service 内返回错误。

### 7.5 开发环境代码生成

| 路径 | 方法 | 功能 |
| --- | --- | --- |
| `/admin/base/coding/getModuleTree` | GET | 获取允许模块的平铺名称数组 |
| `/admin/base/coding/createCode` | POST | 批量生成已校验的 Go 文件 |

这组路由仅在开发环境安装，且只允许 `admin` 角色访问。`getModuleTree` 保持 Midway 与当前 Vue 依赖的平铺 `string[]` 响应，稳定排序并仅返回含合法 `config.go` 的模块。服务使用 `go/ast`、`go/parser`、`go/token` 和 `go/format`，所有输出路径规范化后必须位于 Base 配置声明的可信项目工作区内。拒绝绝对路径、`..` 越界、符号链接越界和非 Go 目标文件；批量请求先完成全量预检，再通过同目录临时文件和原子无覆盖发布写入，不能以普通 `os.Rename` 覆盖既有文件。

当前 Vue AI 页只对 Node 和 Java 调用 `createCode`；`/admin/base/comm/program` 改为返回 `Go` 后，前端还必须补齐 Go 文件路径映射和调用分支，后端工具接口本身不会触发文件生成。

## 8. 辅助业务能力

### 8.1 参数与 HTML

1. `base_sys_param` 按 `keyName` 唯一；
2. 使用 GoFrame `gcache` 缓存按键查询结果；
3. 增加、修改和删除参数后主动失效相关缓存；
4. `dataType == 0` 时先尝试 JSON 解析，可返回对象、数组、布尔或数字，解析失败则返回原字符串；
5. `dataType == 1` 时返回富文本字符串，`dataType == 2` 时按逗号返回文件列表；
6. HTML 接口返回原始 `text/html` 内容，不包裹统一 JSON 响应；HTML 缓存未命中时必须查库并回填缓存；
7. App 参数接口只允许读取模块配置 `allowKeys` 中的键。

### 8.2 本地上传

1. 优先使用 GoFrame `ghttp.UploadFile`；
2. 首版只实现 Midway Base 运行所需的本地模式，`uploadMode` 的业务数据固定为 `{ "mode": "local", "type": "local" }`；
3. 上传成功的业务数据是 URL 字符串，保持 Midway 的 `<配置公开基础 URL>/upload/<YYYYMMDD>/<文件名>` 结构；公开基础 URL 去除末尾斜杠，响应不返回本地绝对路径；
4. Base Controller 使用现有 `IgnoreGlobalPrefix` 静态元数据声明公开的 `GET /upload/{date}/{name}`，由生成的 HTTP Installer 按普通业务路由安装；Base 上传 Service 持有上传根目录并负责日期目录、basename、根目录边界、目录访问和符号链接校验，不建立通用静态目录映射；
5. 默认单文件上限为 100 MiB；multipart 总请求体上限为 101 MiB，用于容纳文件及表单边界等开销；普通 JSON、Form CRUD 请求仍保持 8 MiB 上限；
6. 公开文件读取使用 `cool-next/core/controller` 提供的通用文件原始响应。只允许已通过内容探测的 JPEG、PNG、GIF、WebP、MP3、WAV、MP4 和 WebM 使用对应 `Content-Type` 内联响应，其余文件统一使用 `application/octet-stream` 和 `Content-Disposition: attachment`；所有文件响应增加 `X-Content-Type-Options: nosniff`；客户端声明 MIME 只作为提示，不作为授权依据。对允许内联的媒体类型，扩展名必须与内容探测结果匹配；其他文件保持 Midway 可上传任意业务附件的能力，不要求扩展名、声明 MIME 和内容探测结果机械相等；
7. 保留前端固定提交的可选 `key` 参数。本地模式只接受不含目录的 basename，拒绝绝对路径、路径分隔符、`..`、NUL 和已有目标；未提供 `key` 时由服务端生成安全随机文件名。即使 `key` 相同也不得覆盖已有文件；
8. 保存文件采用临时文件加同目录原子重命名，失败时不留下可访问的半文件；
9. 操作日志不保存文件字节、表单文件内容或本地绝对路径；审计中间件遇到 multipart 请求时只读取 URL 查询参数，不解析 multipart 请求体。

### 8.3 i18n

Base 翻译中间件只负责 Base 业务消息和菜单字段翻译。它读取请求上下文中的语言，不改写底层数据，不成为框架级全局国际化实现。

### 8.4 EPS

EPS 使用 Go v2 现有静态 Graph、Entity Descriptor 和 Controller Definition 生成，保持后台/App 分组及前端所需字段、CRUD 动作、自定义路由和权限元数据。不建立独立的 Base EPS 扫描器。

## 9. 操作日志与系统日志

### 9.1 操作日志

`base_sys_log` 只保存后台业务操作日志：

1. 只处理符合记录规则的 `/admin/**` 业务请求；
2. `userId` 记录已认证后台用户，`action` 记录去除查询串的路径，`ip` 记录客户端 IP；
3. `params` 使用 JSON 字段保存已脱敏的查询或请求参数；
4. 密码、旧密码、Token、Refresh Token、Authorization、验证码、`captchaId` 和文件内容必须排除或替换为 `[REDACTED]`；
5. 对参数大小设置上限，超限只保存截断标识和安全预览；
6. 不记录 `/app/**`、公开上传文件读取、健康检查、系统内部任务、`/admin/base/open/eps`、`/admin/base/open/captcha` 和 `/admin/base/comm/program`；
7. 操作日志写入失败不改写业务响应，失败原因通过 `glog` 记录；
8. `page`、`clear`、`setKeep`、`getKeep` 保持 Midway 契约。

操作日志不保存 panic 堆栈、SQL、数据库连接信息、系统启动信息或定时任务调试文本。

### 9.2 操作日志清理

1. `base_sys_conf.cKey == "logKeep"` 保存保留天数，默认 31 天；
2. 使用 GoFrame `gcron` 每日执行清理；
3. 按 `createTime` 删除保留窗口之前的操作日志；
4. 清理开始、结束、耗时、删除数量和异常通过 `g.Log()` 输出，不回写 `base_sys_log`。

### 9.3 系统日志

系统日志完全使用 GoFrame `glog` / `g.Log()`：

- 应用启动、停止与组件生命周期；
- HTTP 未预期异常与 panic；
- 数据库连接、Schema 和初始化异常；
- 定时任务运行过程；
- 操作日志自身的写入或清理失败。

日志使用请求 `context.Context` 和现有 Trace ID，由 GoFrame 日志配置控制级别、路径、文件名、轮转和标准输出。Base 不再实现一套系统日志存储器。

## 10. 事务与一致性

### 10.1 事务边界

以下操作必须在 Go v2 现有事务边界内完成：

1. 新建用户及其角色关系；
2. 更新用户及替换用户角色；
3. 新建或更新角色及替换菜单、部门关系；
4. 删除用户或角色及清理关系表；
5. 修改密码、递增 `passwordV` 并撤销 Session；
6. 禁用用户或变更用户/角色/菜单权限并撤销受影响 Session；
7. 管理员最小数量保护检查与实际写入。

数据库写入使用 Descriptor/DO/gdb 和现有事务 Runtime，不使用 `g.Map` 作为数据库写入模型。角色和菜单变更在写入前读取旧关系、在写入后读取新关系，以两者影响用户的并集作为撤销范围。

Session 撤销在数据库事务回调返回前执行；撤销失败时回调返回错误并回滚数据库变更。Session Store 不参与数据库事务：撤销成功后数据库提交失败时，已撤销 Session 不恢复，用户重新登录即可；这是为避免旧权限继续有效而接受的可用性降级，不引入 Outbox、补偿事务或分布式事务。

### 10.2 删除与引用

1. 删除用户前清理用户角色关系并撤销 Session；
2. 删除角色前执行管理员保护，然后清理用户、菜单和部门关系；
3. 删除菜单或部门前处理子节点及关系引用，不产生孤立关系；
4. 群组删除在事务开始后统一验证，不部分成功。

## 11. 异常处理

Go v2 框架已封装异常类型、安全解析、HTTP/gRPC 映射、堆栈脱敏和统一响应。Base 不新增异常框架，只遵守以下使用规则：

1. 可预期业务失败使用 `exception.Comm(...)`；
2. 参数校验失败使用 `exception.Validate(...)`；
3. 内部依赖失败使用 `exception.WrapCore(...)`；
4. 未登录和无权限由现有 Auth 能力返回 401/403；
5. HTTP 层继续由 `NewResponseMiddleware` 和 `exception.Resolve` 生成统一响应；
6. 未知异常由现有响应中间件记录至 `glog`，响应不暴露堆栈、SQL、凭据或本地路径；
7. 数据库底层错误继续由现有 CRUD/DB 能力处理；只有“用户名已存在”、“不能删除最后一个管理员”等 Base 业务语义在 Service 中转换为业务异常。

## 12. 框架前置改动

以下是 Base 运行所需的最小通用改动：

1. 修复 `cool-next/codegen/render.go` 中生成 HTTP Installer 固定返回 `nil` 的缺口；
2. 在现有 Controller/Route 类型体系中增加无 DTO Handler 支持，不引入反射调度容器；
3. 在统一响应中间件中增加明确的原始响应通道，已提交的 HTML/文件响应不再包装 JSON；
4. 扩展 Entity Descriptor 支持受限的 JSON slice/map 字段，由现有方言层处理 MySQL、PostgreSQL 和 SQLite 差异；
5. 在公共 Session Store 端口及适配器中补齐按身份种类批量撤销用户 Session 的能力，单用户撤销委托给批量实现，Redis 多用户撤销只扫描一次；
6. 在静态装配链中构造 Base 所需的 DB/Auth/HTTP 运行链，并安装模块 Initializer 与生命周期组件；
7. 通用初始化装配依次处理 Descriptor Schema、`db.json` 和 `menu.json`，业务对齐键由 Base 提供。

所有生成文件由 `cool generate` 重新生成，不手改生成产物。每项通用改动必须先有框架单元测试，再由 Base 集成测试验证实际装配。

## 13. 测试设计

### 13.1 框架回归

1. 生成的 HTTP Installer 实际安装路由；
2. Handler 同时支持有 DTO 和无 DTO 签名；
3. HTML 和文件响应不进入 JSON 包装，验证码保持 JSON 中的 SVG Data URL 契约；
4. JSON slice/map 字段在三种数据库中读写一致；
5. 公共 Session Adapter 能按 Admin/App 身份隔离地撤销指定用户的全部 Session；
6. Schema 与初始数据重复执行幂等；
7. 现有异常、Auth、CRUD、HTTP/gRPC 与其他模块测试不回归。

### 13.2 Base 单元与服务测试

1. v1 验证码默认值、非法输入回退、SVG Data URL、缓存、忽略大小写和一次性消费，同一验证码并发校验最多一次成功；
2. bcrypt cost 12 加密与校验；
3. 登录、账号禁用、无角色用户、固定 Token 响应 DTO、TTL 秒单位、刷新令牌重放和登出；
4. 用户、角色、菜单、部门关系替换与事务回滚；
5. 最后一个平台管理员保护与并发场景；
6. 个人资料字段白名单、密码摘要隐藏以及 `page`、`info` 各自的角色字段契约；
7. 权限菜单、路由权限和直接关联部门数据权限，确认 `relevance` 不扩展范围；
8. 参数类型解析、HTML 缓存未命中和变更失效；
9. 上传大小、可信媒体内容探测、附件强制下载、可选 basename `key`、禁止覆盖、原子落盘、配置公开基础 URL，以及 Base 公开文件路由的日期目录、basename、根目录边界、目录访问、符号链接和路径穿越校验；
10. 操作日志脱敏、截断、路由过滤、分页、清理和保留天数；
11. 菜单解析、导入、导出与代码生成路径安全，确认生产环境不安装 `menu/parse`、`menu/create` 和 `coding/**`。

### 13.3 集成与契约测试

1. 对 Midway Base 的每个公开路由建立方法、路径、认证、参数和响应契约测试；
2. 验证六组 CRUD、所有自定义接口和 EPS 元数据；
3. 验证密码、角色、菜单或用户状态变更后旧 Session 失效；并发执行登录或刷新与权限变更，确认不能在撤销后创建携带旧权限的 Session；模拟 Session 撤销失败时数据库回滚，模拟数据库提交失败时 Session 保持撤销；
4. 验证全部表名使用 `base_sys_` 前缀，且任何表、DTO、配置和查询都不包含 `tenantId`；
5. SQLite 作为快速集成基线，MySQL 和 PostgreSQL 使用项目现有容器集成测试链路；
6. 重复启动两次，验证 Schema、`db.json` 和 `menu.json` 幂等；修改初始化菜单或部门名称后再次启动，确认仍按 `seedKey` 对齐、恢复种子值且不产生重复记录；确认启动不会重置管理员密码、`passwordV`、个人资料、参数或日志保留天数；
7. 验证 `seedKey` 仅作为 hidden、readonly EPS 字段出现，普通 CRUD、菜单导入导出和业务响应均不可读写；从 JSON 删除种子项时确认启动不自动删除，显式迁移负责清理；
8. Redis 集成测试为多个用户撤销 Session 时只执行一次 Key 扫描；
9. 契约测试以 Midway Base 源码和当前前端调用为基线，并为第 2.3 节的每项有意差异建立独立断言。

### 13.4 日志专项测试

1. 后台业务操作写入 `base_sys_log`；
2. 密码、Token、验证码和文件内容不进入操作日志；
3. `/app/**`、公开上传文件读取、健康检查和系统事件不产生操作日志；
4. 启动、异常、数据库和定时任务日志只由 `glog` 输出；
5. 系统日志不写入 `base_sys_log`；
6. `gcron` 清理任务只删除过期操作日志，其运行过程写入 `glog`。

### 13.5 工程检查

最终必须通过：

1. `cool generate` 后工作区不产生非预期生成差异；
2. `gofmt` 检查；
3. `go test ./...`；
4. `go vet ./...`；
5. MySQL、PostgreSQL 和 SQLite 集成测试。

## 14. 验收标准

以下条件全部满足时，Base 迁移才算完成：

1. Midway Base 的全部目标路由在适用环境的 Go v2 中可用，开发专用路由在生产环境不存在，前端契约测试通过；
2. 10 张表全部以 `base_sys_` 开头，无任何多租户字段或分支；
3. 用户、角色、菜单、部门、参数和操作日志功能可用；
4. v1 验证码实现和行为测试已迁移；
5. 权限变更可靠撤销受影响 Session，最后一个平台管理员受保护；
6. 用户接口不泄露密码摘要，操作日志不泄露敏感参数；
7. 用户分页保留 `departmentName/roleIds/roleName`，用户详情保留 `departmentName/roleIdList`；
8. 本地上传接受兼容的可选 `key`，返回配置公开基础 URL 下可由 Base 公开文件路由直接访问的 `/upload/**` URL，上传目录不能被覆盖、遍历或列目录；
9. 系统日志仅使用 GoFrame `glog`，`base_sys_log` 仅保存操作日志；
10. 三种数据库的 Schema、初始化、CRUD 和业务集成测试通过；
11. 现有框架异常封装和其他模块无回归；
12. 登录和刷新严格返回 `token/expire/refreshToken/refreshExpire`，两个过期字段均为 TTL 秒数；
13. 除第 2.3 节列出的有意差异外，文档、生成文件和代码实现与 Midway Base 源码及当前前端调用一致。

## 15. 实施边界备忘

1. 业务规则进 `modules/base/service`，通用机制才进 `cool-next`；
2. 复用 GoFrame v2.10.2 的 `glog`、`gcron`、`gcache`、`ghttp.UploadFile`、`gerror` 和 gdb；
3. 继续复用 Go v2 现有 Auth、Session、Exception、CRUD、Descriptor 和 Transaction 边界；
4. 实施时按单个文件完整修改，不夹带无关重构；
5. 新增第三方 API 调用前必须先核对官方文档或当前版本源码；
6. 实施计划必须给出逐文件修改顺序、单测、集成测试和每个纵向闭环的停止条件。
