# cool-admin-go-next 安全与性能审计报告

- 审计日期：2026-07-23
- 审计范围：`cool/` 和 `modules/base/`
- 审计基准：当前工作区内容（包含未提交变更）
- 审计方式：主审与 3 个子代理并行只读审计，并复核路由、中间件、Controller、Service 和 ORM/SQL 完整调用链

## 结论摘要

当前最紧急的风险不是 SQL 注入，而是身份信任边界、角色关系授权和租户隔离。已确认两条普通登录用户可获得超级管理员权限的链路，以及多处可从已注册 HTTP 路由触发的跨租户 IDOR。

| 等级 | 数量 | 主要类型 |
| --- | ---: | --- |
| 严重 | 3 | JWT 重签提权、跨租户绑定 admin 角色、默认超管凭据 |
| 高 | 7 | 跨租户 IDOR、同源 XSS、密码泄露、MD5、远程源码写入、授权撤销失败 |
| 中 | 8 | 会话撤销、无界查询、权限查询性能、内存 session、schema 漂移、事务一致性、限流和文件覆盖 |

## 已确认漏洞

### 1. [严重] 默认 JWT 密钥可让普通用户伪造超管身份

**状态（2026-07-27）：已修复并通过单元、race 和真实 MySQL HTTP 集成测试。** 默认启动会将精确占位符原子替换为 UUID v4，空值、历史默认值和弱密钥会拒绝启动；超管权限改为按用户 ID 和数据库中的全局 `admin` 角色关系判定。直接篡改 `username`、`roleIds` 或 `tenantId` 都返回 401。

**证据**

- `manifest/config/config.yaml:28` 提供公开固定的 JWT 密钥 `cool-admin-go-next-jwt-secret-key`。
- `cool/app/app.go:154-155` 仅校验密钥长度，该默认密钥能通过检查。
- `manifest/config/config.yaml:31` 默认 `sso: false`。
- `cool/auth/middleware.go:88-103` 仅将 session 中的 `UserID` 和 `PasswordVersion` 与 JWT 比对，然后直接信任 JWT 中的 `Username`、`RoleIds` 和 `TenantId`。
- `modules/base/service/sys/perms.go:72-80` 对 `Username == "admin"` 直接授予超管权限。

**攻击链**

1. 普通用户正常登录，获得自己的 `sid`/`userId`/`passwordVersion`。
2. 修改 JWT payload，保留上述会话字段，将 `username` 改为 `admin`，必要时将 `tenantId` 改为 `0`。
3. 使用仓库公开的默认 HS256 密钥重签。
4. 认证中间件会通过 session 校验，权限服务将该请求视为超管。

**关于用户名唯一性**

数据库中的 `username` 确实不允许重复，`modules/base/service/sys/user.go:230` 和 `:289` 会检查重复用户名。但此攻击不会新增或修改数据库用户名，只会篡改 JWT claim；当前中间件没有从数据库重新确认 `Username`，因此用户名唯一约束不能阻止该提权。

**修复建议**

- 删除生产可用的默认密钥，从环境变量或 Secret Manager 注入。
- 启动时拒绝已知默认/占位密钥，不能只检查长度。
- 不从 JWT 中的 `Username` 决定管理员权限；应按 `user_id` 从服务端权威数据查询。
- session 保存并校验授权版本或权威身份快照，角色变更时使其失效。

### 2. [严重] 租户用户可绑定全局 admin 角色

**状态（2026-07-27）：已修复并通过真实 MySQL HTTP 集成测试。** 全局 `admin` 角色不能通过任何新增或更新接口分配；普通角色会校验目标作用域和调用者可授权范围。平台超管用户及全局 `admin` 角色的管理端 Update/Delete 统一返回 403。

**证据**

- `modules/base/service/sys/user.go:226` 和 `:310-323` 接受客户端 `roleIdList`。
- `modules/base/service/sys/user.go:492-506` 不检查角色租户或调用者可授权范围，直接写入 `base_sys_user_role`。
- `modules/base/db.json:69-77` 中的全局角色 `id=1` 具有 `label=admin`。
- `modules/base/service/sys/perms.go:76` 查询 admin 角色时没有租户条件。

具有 `base:sys:user:update` 或 `base:sys:user:add` 权限的租户管理员可为自己或新用户指定 `roleIdList:[1]`，随后被视为平台超管。该攻击同样不需要创建重复的 `admin` 用户名。

**修复建议**

- 在同一事务内校验 `roleIdList`、`menuIdList`、`departmentIdList` 的租户归属。
- 校验调用者的可授权范围，禁止租户用户分配平台 admin 角色。
- `IsAdmin` 必须同时校验用户、角色和角色作用域。
- 关系表应通过租户字段或可验证的复合外键保证关系图完整性。

### 3. [严重] 首次部署自动创建已知超管凭据

**状态（2026-07-27）：已缓解，尚未彻底修复。** 默认配置已关闭 `initDB`，不会在普通启动时自动导入已知凭据；但 `modules/base/db.json` 仍包含兼容初始化账号，显式执行 seed 导入后仍必须立即修改密码。彻底修复仍需改为部署变量注入或一次性随机初始凭据。

**证据**

- `manifest/config/config.yaml:18` 默认 `initDB: true`。
- `modules/base/db.json:80-93` 创建用户 `admin`。
- `modules/base/db.json:86` 的密码摘要 `e10adc3949ba59abbe56e057f20f883e` 是 MD5(`123456`)。
- `modules/base/db.json:96-100` 将其绑定到角色 1。

未在首次启动后立即改密的实例可被使用公开凭据直接接管。验证码只会提高自动化成本，不是对已知密码的有效修复。

**修复建议**

- 不提交固定生产超管密码。
- 首次启动时强制通过安全变量设置，或生成仅显示一次的高熵凭据。
- 首次登录强制改密；检测到默认摘要时拒绝对外启动。

### 4. [高] 多处租户行级授权缺失

**状态（2026-07-28）：已完成通用修复。** `cool/tenant` 已接入模型编译、通用 CRUD Runtime 和自定义 ORM 路径。包含标准 `tenant_id` 字段的资源默认启用隔离，Missing 作用域失败关闭；客户端 `tenantId` 不再决定写入归属。

当前安全边界：

- 平台数据规范化为 `tenant_id IS NULL`，正数表示具体租户；legacy `0` 只用于迁移兼容并会转换为 `NULL`。
- Tenant、Platform、Bypass 和 Missing 是不同作用域。Platform 保留经批准的跨租户管理语义；Bypass 只能由受审计内部代码显式派生。
- Add/AddMany 强制覆盖租户值；读取追加参数化谓词；Update/Delete 及批量操作在事务内校验完整命中，零命中和部分命中不会提交或执行 After Hook。
- 公开参数和字典读取使用显式 GlobalOnly，只读取 `tenant_id IS NULL`，缺失认证不会自动变成 Bypass。
- Base 与 Dict 的 raw SQL、JOIN、递归删除和无租户列关系表已完成作用域审计。用户、角色、菜单、部门关系污染不会泄露另一租户数据或产生部分写入。
- AST guard 扫描模块 Service 的直接 `GetOne/GetAll/GetCount/Exec/Model`；任何新增或变化的 raw access 都必须提供精确用途并重新审计。

真实 MySQL 双租户矩阵覆盖通用 CRUD、Dict Type/Info、公开读取、伪造 tenantId、跨租户父子和关系污染、权限菜单、递归级联及原子回滚。代表性 Page 使用 tenant 索引，点 Update/Delete 使用主键并保留 tenant 谓词，无全表扫描和额外租户查询。

详细运行约束和验证命令见 `docs/tenant-scope.md`。

### 5. [高] 任意主动内容上传并在应用同源公开

**状态（2026-07-27）：已修复。** 上传只允许图片、PDF 和文本类白名单扩展，服务端同时校验内容嗅探结果；HTML、SVG、JS 和伪装图片会被拒绝。静态上传响应增加 `X-Content-Type-Options: nosniff` 和沙箱 CSP，并通过单元与 HTTP 集成测试。

**证据**

- `modules/base/service/upload.go:74-105` 仅检查文件大小和路径，保留 `.html`、`.svg`、`.js` 等扩展名，不验证 MIME 或文件 magic bytes。
- `modules/base/controller/app/comm.go:41-48` 的 app 上传路由只要登录即可使用，没有细粒度 Permission。
- `cool/app/app.go:493-496` 将上传目录挂载到同源 `/upload` 和 `/uploads`。

攻击者可上传包含脚本的 HTML/SVG，再诱导管理员访问返回的同源 URL，形成持久型 XSS 或恶意内容托管。

**修复建议**

- 同时校验扩展名、声明 MIME 和 magic bytes，使用最小允许白名单。
- 拒绝 HTML、SVG、JS 等主动内容，图片建议解码后重新编码。
- 上传资源放到独立无 Cookie/无凭据域名，设置 `X-Content-Type-Options: nosniff` 和隔离 CSP，或强制 attachment。

### 6. [高] 公开 HTML 接口可执行数据库中的任意脚本

**状态（2026-07-27）：已修复。** 已移除免认证 `/admin/base/open/html`，保留的管理端参数 HTML 路由需要认证和权限，标题与内容均进行 HTML 转义。

- `modules/base/controller/admin/open.go:81-85` 将 `/admin/base/open/html` 设为 `IgnoreAuth: true`。
- `modules/base/service/sys/param.go:294-310` 将参数名称和内容不转义地拼入 HTML。
- 该公开入口没有 app param 接口使用的 `allowKeys` 白名单。

结合参数跨租户更新，可存入 `<script>` 或事件处理器，再诱导管理员访问公开 URL，从而窃取前端 token 或以管理员身份操作。

修复时应移除公开原始 HTML 渲染；必须保留时，使用严格 key 白名单、可靠 HTML sanitizer、标题转义、独立域名和严格 CSP。

### 7. [高] 操作日志明文记录旧密码

**状态（2026-07-27）：新写入路径已修复。** 登录、刷新和个人资料/密码修改路由完全不记录请求 body，通用递归脱敏也覆盖 `oldPassword`。历史日志不会由应用自动修改，升级时仍应执行一次性清理。

- `modules/base/middleware/log.go:22-29` 的敏感字段列表包含 `password`，但不包含 `oldPassword`。
- `modules/base/middleware/log.go:61-85` 将 JSON body 记录到日志。
- `modules/base/service/sys/user.go:51-60` 的个人改密 DTO 使用 `oldPassword`。

调用 `/admin/base/comm/personUpdate` 改密时，当前有效旧密码会写入 `base_sys_log.params`。应对认证、改密和 token 路由禁止记录 body，并使用允许字段清单而不是有限的拒绝清单。还需清理已有日志中的密码。

### 8. [高] 密码使用无盐单轮 MD5

**状态（2026-07-27）：已修复。** 新密码统一使用 bcrypt；旧 MD5 摘要只用于兼容验证，并在成功登录持有用户行锁的事务中原子升级为 bcrypt，同时递增密码版本。

- `cool/auth/password.go:4-16` 使用 MD5 生成和校验密码摘要。
- `modules/base/service/sys/login.go:136` 登录时使用该校验。
- `modules/base/service/sys/user.go:242`、`:305` 和 `:541` 在创建/修改密码时写入 MD5。

数据库或备份泄露后，常见密码可被彩虹表或 GPU 迅速破解，相同密码的摘要也完全相同。应迁移到 Argon2id 或 bcrypt；登录时兼容识别旧 MD5，成功验证后原子升级摘要。

### 9. [高] 菜单“创建代码”接口可覆盖项目 Go 源码

**状态（2026-07-27）：已修复。** 管理端不再注册 `/admin/base/sys/menu/create`，EPS 和权限映射中也不再暴露该能力；离线代码生成逻辑不再可由 HTTP 调用。

- `modules/base/controller/admin/sys/menu.go:39-43` 注册了 `base:sys:menu:create` HTTP 路由。
- `modules/base/service/sys/menu.go:567-625` 接收 Entity/Controller/Service 源码，只检查 Go 语法。
- `modules/base/service/sys/menu.go:621` 使用 `os.WriteFile`，已存在的普通文件会被覆盖。

具有该权限的账号可写入含恶意 `init()` 的语法有效 Go 文件，在后续构建或部署时执行，也可覆盖已有业务文件。生产环境应禁用此开发端点，并将代码生成移到离线 CLI 或受控 CI。

### 10. [高] 移除用户全部角色时撤销失败

**状态（2026-07-27）：已修复并通过真实 MySQL HTTP 集成测试。** `roleIdList` 缺失时保持原关系，字段存在且为空数组时删除全部角色关系，并在事务提交前撤销该用户的 access/refresh session。

`modules/base/service/sys/user.go:310-323` 只在 `roleIdList` 存在且解析结果非空时调用 `replaceUserRoles`。管理员提交 `roleIdList:[]` 试图移除最后一个角色时，接口返回成功，但原关系和权限保留。

修复时只要请求中出现 `roleIdList`，就必须执行替换，包括空集合，并添加撤销最后角色的集成测试。

## 中等风险与性能问题

### 11. 删除用户未撤销现有 session

**状态（2026-07-27）：已修复并通过单元、race 和真实 MySQL HTTP 集成测试。** 用户删除、密码/状态/用户名/角色变更及角色权限变更都会在事务提交前批量撤销 session。登录、刷新和授权写入共用按用户 ID 排序的 `FOR UPDATE` 协议；Redis refresh 轮换会原子校验用户 generation。

`modules/base/service/sys/user.go:343-374` 删除用户及角色关系后没有调用 `Sessions.DeleteUser`。旧 access token 在过期前仍可通过认证中间件，并继续使用只要登录、不需要细粒度 Permission 的接口。

### 12. 分页、导出、列表和批量写无服务端硬上限

**状态（2026-07-27）：已修复。** 普通分页最大 200 条、同步导出最大 10000 条、无分页列表最大 1000 条、批量写最大 500 条；客户端缺失或提交更大限制时统一收敛到服务端上限。

- `cool/crud/request.go:38-52` 直接信任客户端 `size`、`isExport` 和 `maxExportLimit`。
- `cool/crud/query.go:418-425` 只修正非正数，不限制最大 size。
- `cool/crud/query.go:167-185` 的通用 list 没有 `LIMIT`。
- `modules/base/service/sys/page.go:28-35` 在 `isExport=true` 且 `maxExportLimit<=0` 时返回无 `LIMIT` SQL，用户和日志 Page 会调用该实现。
- `cool/crud/runtime.go:76-90` 和 `:154-161` 对数组逐条执行 SQL，没有条数上限。

攻击者可请求巨大页面或全表导出，导致数据库扫描、Go map 分配和响应放大。应使用服务端固定的分页/导出/批量上限，大导出改为异步流式任务。

### 13. 权限检查在每个请求上多次查库并加载全量菜单

`modules/base/service/sys/perms.go:130-150` 的 `HasPermission` 先查超管角色，再构建用户权限菜单。`modules/base/service/sys/perms.go:176-213` 会查询用户角色菜单，再读取全表菜单以补齐父节点。

这使每个受保护请求产生多次 DB 访问和 O(全部菜单) 处理。建议直接查询权限码或缓存用户权限集合，并在角色/菜单变更时失效。

### 14. 默认内存 session store 不适合生产多实例

- `cool/app/app.go:157-160` 未注入 SessionStore 时使用 `MemorySessionStore`。
- `cool/auth/session.go:66-95` 只在访问同一 SID 时清理过期 session。
- `cool/auth/session.go:112-137` 没有定时清理、容量限制或每用户会话上限。

多实例会出现随机 401、注销/SSO 只在单实例生效和重启全部登出；不再访问的过期 session 会长期占用内存。生产应强制 Redis/DB 实现，支持原子 Rotate 和 TTL。

### 15. Schema 同步会静默接受已有结构漂移

- `cool/db/schema/sync.go:58-75` 对已存在列只检查名称，不比对类型、nullable、default、unsigned 和 auto increment。
- `cool/db/schema/sync.go:78-98` 对已存在索引只检查名称，不比对唯一性和列顺序。
- `cool/db/schema/sync.go:194-256` 的定义和查询不读取/校验主键。

旧库或手工迁移存在同名但定义错误的列/索引时，启动仍会报同步成功。这可使用户名、seed marker 等唯一约束失效。Safe mode 应在发现漂移时拒绝启动并输出迁移建议。

### 16. 默认 CRUD 和批量写入缺少原子性

**状态（2026-07-27）：已修复默认 CRUD 路径。** 默认新增、更新、删除及批量新增/更新均在 GoFrame 事务中执行，before/after hook 与数据库写入处于同一事务；after hook 或任意批次失败会回滚。显式业务 Service 重写仍负责自身事务边界。

- `cool/crud/runtime.go:59-71`、`:117-121` 和 `:145-149` 先提交数据库写入，再调用 after hook。after hook 失败时接口返回失败，但数据已提交。
- `cool/crud/runtime.go:76-90` 和 `:154-161` 的批量新增/更新逐项执行，第 N 项失败时前 N-1 项保留。

这会产生部分提交或“已提交但返回失败”，客户端重试可重复写入。需要原子性的批量和 hook 应在同一事务中执行；若 after 是 after-commit 通知，它的失败不应伪装成主写入失败。

### 17. 公开验证码和同步日志可被用于存储放大

- `modules/base/controller/admin/open.go:65-85` 的 captcha/login/eps/html 等入口免认证。
- `modules/base/service/sys/login.go:23-30` 将 captcha TTL 设为 30 分钟。
- `modules/base/service/sys/login.go:93-113` 每次 captcha 请求都生成多个加密随机数并写入缓存。
- `modules/base/middleware/log.go:31-45` 在处理器之前对所有 `/admin/` 请求同步写入日志。

未发现 IP、账号或全局限流。匿名请求可同时放大 CPU、内存缓存和日志表写入。应对登录/验证码设置多维度令牌桶，缩短 TTL，限制每 IP 活动 captcha，并排除 captcha/eps 等低价值日志或使用有界异步日志。

### 18. 客户端 upload key 可覆盖当天的同名文件

**状态（2026-07-27）：已修复。** 客户端 key 不再决定落盘路径；服务端生成 32 位随机文件名，按租户和用户分目录，并使用 `O_EXCL` 排他创建，无法通过重复 key 覆盖既有文件。

`modules/base/service/upload.go:83-99` 接受客户端 `key`，生成目标路径后使用 `file.Save(targetDir, false)`。路径穿越已被拒绝，但没有对象所有权、租户命名空间或排他创建。知道 key 的登录用户可替换其他用户的头像、附件或已信任 URL 内容。

应由服务端生成不可预测、不可变 key，按租户/用户隔离，创建时使用排他写入；替换必须走单独授权接口。

## 风险假设与加固建议

### 自定义 admin 路由 Permission 默认为空时 fail-open

`cool/controller/permission.go:38-42` 对没有 PermissionMap 记录的路由直接放行。base comm 中某些路由可能是有意设计为“任意登录用户”，但新增敏感路由如果忘记填写 `Permission`，会静默开放给所有登录用户。

建议对 admin/sys 自定义路由默认要求 Permission，需要仅认证的路由使用显式 `AuthenticatedOnly` 元数据，并在启动期 lint。

### Seed 首次导入存在多实例 TOCTOU

`cool/seed/importer.go:58-105` 在事务外查询 marker，事务内再使用普通 `COUNT` 查询，然后先导入数据、最后写 marker。多实例同时启动时可同时观测到 marker 不存在，导致重复导入、唯一约束冲突或某个实例启动失败。

建议先使用唯一 marker、MySQL advisory lock 或专用 migration lock 抢占导入权，再执行导入。

## SQL 注入审计结论

未发现当前 HTTP 输入可直接触发的 SQL 注入：

- CRUD 和 base service 中的用户输入值普遍使用占位符绑定。
- 排序字段和方向通过 `ResolveSortTerms` 映射到启动期白名单。
- CRUD 标识符来自模型元数据，并使用标识符引用。
- Seed/Schema 中的动态标识符主要来自本地模型和 seed 定义，未发现 HTTP 可控调用链。

上述结论不代表动态 SQL 永远安全；新增 raw SQL、动态表/列名或自定义排序时仍需保持白名单和参数绑定。

## 已检查且未发现明确绕过的区域

- Access token 和 refresh token 类型已分离。
- HMAC 签名比较使用常量时间比较。
- Session ID/JTI 使用加密随机数，refresh 轮换在内存 store 中是原子的。
- 验证码正确验证后会被一次性删除。
- 动态排序字段和方向有白名单校验。
- 上传 key 对 `..`、绝对路径、Windows drive 和根目录逃逸有明确阻断，未发现可确认的路径穿越。
- 内部数据库错误默认不会直接回显给客户端。
- 用户详情和分页响应不返回密码字段。

## 验证结果

2026-07-27 对身份、授权及剩余安全整改运行：

```text
go vet ./cool/... ./modules/base/...
go vet ./...
go test ./... -count=1
go test -race ./cool/crud ./cool/app ./modules/base/service ./modules/base/middleware ./modules/base/service/sys -count=1
go test -race ./cool/auth ./cool/app ./modules/base/service/sys -run 'JWTSecret|Session|Middleware|Admin|Assignable|Protected|User|Department|Role|Refresh|Person' -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run 'Auth(Security)?Integration|AuthIntegration.*Concurrent' -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run 'Test(CustomAPIServiceIntegration|TenantBoundaryServiceIntegration|CustomAPIIntegration)$' -count=1
```

上述检查全部通过。真实 MySQL 用例覆盖 JWT claim 篡改、全局 admin 分配与保护、空角色清权、用户/角色变更后会话撤销、授权事务与登录/刷新的双连接并发协议，以及参数/日志双租户隔离和安全上传 HTTP 流程。

现有测试的主要缺口：

- 两个租户之间对所有模块每个 CRUD 动作的完整 HTTP 矩阵。
- Schema 已有列、索引和主键定义漂移的真实 MySQL 集成用例。

## 剩余建议顺序

1. 取消 seed 中的 admin/123456，改为部署变量或一次性随机初始凭据。
2. 对升级前的敏感操作日志执行一次性清理。
3. 为所有模块补齐双租户 HTTP CRUD 矩阵，并为新资源建立默认 tenant scope 约束。
4. 优化权限查询缓存，生产多实例强制使用 Redis session store。
5. 修复 schema 漂移检测、seed 多实例锁和登录/验证码限流。
