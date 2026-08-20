# cool-admin-go-next 身份与授权边界修复设计

- 日期：2026-07-27
- 输入：`docs/security-audit-2026-07-23.md`
- 覆盖审计项：#1、#2、#10、#11

## 1. 背景

安全审计将 JWT 默认密钥、超管判定、角色分配和 session 撤销串成了两条提权链路。复核 Node 版后，需要修正其中一项风险前提：Node 版的 `pnpm dev` 会执行 `cool check`，将 JWT secret 占位符替换为 UUID。因此，“公开默认密钥可直接重签”只在部署仍使用已知默认值时成立，不是所有生产环境的无条件漏洞。

Go Next 当前已提交版本没有对等的首次生成机制，且会接受已知默认值启动。同时，当前工作区的未提交修复草稿将用户名、角色和租户复制到 session，可以防止签名密钥泄露后篡改 claims，但也引入了两份身份快照、session 数据迁移和更强的服务端状态依赖。在部署安全注入 secret 的前提下，这些复杂度没有必要。

## 2. 目标

1. 复刻 Node 版的本地首次 secret 生成体验，并保留部署配置覆盖能力。
2. 保持现有 JWT claims 和登录、刷新响应协议兼容。
3. 删除 `username == "admin"` 的超管授权捷径，使平台超管只由数据库权威关系决定。
4. 禁止所有 HTTP 接口新增、移除或修改唯一平台超管身份，同时保留超管修改自己资料和密码的能力。
5. 授权信息变化时立即撤销受影响用户的全部 session。
6. 修复空 `roleIdList` 无法撤销最后一个角色的问题。
7. 通过单元测试和 MySQL HTTP 集成测试防止回归。

## 3. 非目标

1. 不在本规格中实现通用 CRUD 租户行级隔离；审计 #4 单独设计。
2. 不处理首次管理员密码引导和 MD5 迁移；审计 #3 和 #8 单独设计。
3. 不在本规格中强制生产 Redis session store；多实例部署约束属于审计 #14。
4. 不新增平台管理员管理 API。唯一平台超管后续变更只能通过离线数据库运维完成。

## 4. 核心决策

### 4.1 JWT 和 session 的边界

JWT 是服务端签发的身份快照，继续包含：

- `userId`
- `username`
- `roleIds`
- `tenantId`
- `passwordVersion`
- `sid`
- `jti`
- token 类型和过期时间

中间件只在签名、有效期和 token 类型校验成功后使用 claims。session 不再保存 `Username`、`RoleIDs` 和 `TenantID`，只保存会话有效性所需的数据：

- session ID 和用户 ID
- access/refresh JTI 摘要
- 密码版本
- refresh token 过期时间

session 负责退出、SSO、refresh 轮换、重放防护和授权变更后的立即撤销。它不承担第二份身份快照。

### 4.2 平台超管定义

平台超管必须同时满足：

1. 用户记录存在且状态启用。
2. 用户属于平台作用域，即 `tenant_id` 为 `NULL` 或 `0`。
3. 用户绑定标签为 `admin` 的全局角色。
4. 该角色的 `tenant_id` 为 `NULL` 或 `0`。

`IsAdmin` 每次按 `userId` 查询上述权威关系。用户名不参与授权判定。实现不硬编码角色 ID `1`，而是使用角色作用域和 `label = "admin"` 识别受保护角色。

### 4.3 唯一平台超管保护

任何 HTTP 路径都不得：

1. 为其他用户绑定全局 `admin` 角色。
2. 从唯一平台超管移除全局 `admin` 角色。
3. 通过管理端用户 Update/Delete 修改或删除唯一平台超管用户。
4. 修改、删除或改变全局 `admin` 角色作用域。
5. 通过修改请求中的 `username`、`tenantId` 或其他描述字段绕过上述保护。

保护判定必须根据数据库中的目标 ID 和实际关系完成，不使用客户端提交的用户名或角色标签。

超管仍可以通过 `personUpdate` 修改自己的个人资料和密码。该接口不得修改用户名、状态、租户或角色绑定；密码修改成功后按通用规则撤销全部 session。

## 5. 组件设计

### 5.1 JWT Secret Bootstrap

Go Next 使用明确的仓库占位符，例如 `cool-admin-go-next-xxxxxx`。默认启动流程在创建 JWT Manager 之前执行一次 secret bootstrap：

1. 读取有效的 `cool.auth.jwtSecret`。
2. 部署层已提供非占位 secret 时直接使用，不修改仓库文件。
3. 有效值仍是占位符时，使用加密安全随机源生成 UUID v4。
4. 只替换 `manifest/config/config.yaml` 中 `cool.auth.jwtSecret` 的占位值，不做全文本任意替换。
5. 通过同目录临时文件和原子 rename 写回，并保留原文件权限。
6. 重新读取生效配置，然后创建 JWT Manager。

若占位符无法写回，或重新读取仍得到占位符，应用拒绝启动。已知历史默认值、空值和长度不足的非占位值也拒绝启动。日志只记录配置路径和错误原因，不记录 secret。

### 5.2 Auth Middleware

受保护请求按以下顺序验证：

1. 解析并验证 JWT 签名和过期时间。
2. 拒绝将 refresh token 用作 access token。
3. 根据 `sid` 读取 session；session store 未注入或不可用时失败关闭。
4. 校验 session 与 claims 的 `userId` 和 `passwordVersion`。
5. `sso = true` 时校验 access JTI。
6. 使用已验签 claims 构造 `UserContext`。

当前草稿中将 `Username`、`RoleIDs` 和 `TenantID` 写入 session 并从 session 构造上下文的改动不纳入本设计。

### 5.3 SessionStore 批量撤销

`SessionStore` 增加按用户 ID 集合撤销的能力。语义是调用返回成功后，所有目标用户的旧 session 都不再可用。

- 内存实现在同一把锁下删除目标用户 session。
- Redis 实现批量更新用户 session generation，不使用 `KEYS` 或扫描 session key。
- 输入用户 ID 先去重和排序，空集合是成功的空操作。

### 5.4 用户授权变更

用户 Add/Update/Delete 服务必须：

1. 从数据库读取调用者、目标用户和目标角色的实际作用域。
2. 拒绝任何包含全局 `admin` 角色的新增或替换请求。
3. 平台超管可以为目标用户分配与目标用户租户作用域一致的非 `admin` 角色。
4. 非平台超管只能操作同租户用户，且只能分配同租户中由自己创建或自己当前持有的角色。租户用户不能分配任何全局角色。
5. 拒绝通过管理端用户 Update/Delete 修改或删除已绑定全局 `admin` 角色的用户。
6. `roleIdList` 缺失表示保持原关系；字段存在且为空数组表示删除全部关系。
7. 角色、状态、租户、用户名或密码变化时撤销目标用户的全部 session。
8. 用户删除时在删除提交前撤销全部 session。

### 5.5 角色授权变更

角色 Update/Delete 服务必须：

1. 拒绝修改或删除全局 `admin` 角色。
2. 更新角色菜单或部门权限前，收集所有绑定该角色的用户 ID。
3. 角色关系变化成功后，撤销全部受影响用户 session。

## 6. 数据流

### 6.1 登录

1. 按用户名查询用户，并在数据库事务中锁定该用户行。
2. 校验用户状态、密码和角色集合。无角色用户不能登录。
3. 使用当前用户和角色快照生成 token pair。
4. 在释放用户行锁前保存 session。`SSO = true` 时替换该用户旧 session，否则新增 session。
5. 提交事务并返回现有 Node 兼容响应。

### 6.2 刷新

1. 验证 refresh token 签名、类型、有效期和 session。
2. 在事务中锁定用户行，重新读取用户状态、角色和租户，不直接复制旧 refresh claims。
3. 用户不存在、被禁用或没有角色时拒绝刷新。
4. 使用最新身份快照生成 token pair，原子轮换 refresh JTI。
5. 轮换成功后释放用户行锁并返回新 token pair。

### 6.3 授权变更

1. 将受影响用户 ID 去重、升序排列。
2. 在数据库事务中按相同顺序锁定用户行，避免多用户变更产生锁顺序死锁。
3. 根据数据库中的真实目标执行平台超管保护校验。
4. 执行用户、角色或关系写入。
5. 保持用户行锁期间批量撤销 session。
6. session 撤销成功后提交数据库事务。

登录、刷新和授权变更使用相同的用户行锁协议，因此不能在“session 已撤销、授权变更未提交”的窗口签发旧权限 session。

## 7. 错误处理与一致性

1. token 无效、session 不存在、用户失效或 refresh 重放统一返回 HTTP 401。
2. 通过管理端修改或删除唯一平台超管、全局 `admin` 角色或其绑定统一返回 HTTP 403 和通用“非法操作”消息。
3. 普通参数错误继续使用现有业务错误结构。
4. session store 撤销失败时回滚数据库事务，客户端收到通用内部错误。
5. session 已撤销但数据库提交最终失败时，用户可能需要重新登录，但不会保留过期权限。这是有意的安全侧降级。
6. secret 生成、原子写回或配置重载失败时拒绝启动，错误中不包含 secret。
7. 数据库和 session store 详细错误只写入内部日志。

## 8. 协议兼容

以下协议保持不变：

- 登录响应的 `token`、`expire`、`refreshToken`、`refreshExpire`
- access 和 refresh JWT 的现有业务 claims
- `Authorization` header 的现有格式
- 现有 401/403 响应主体
- `roleIdList` 的请求字段名

唯一有意收紧的行为是：过去可能返回成功的平台超管变更现在返回 403；授权变更后受影响用户需要重新登录。

## 9. 当前修复草稿的处理

### 9.1 保留方向

- 启动时拒绝空值、已知默认值和弱 secret。
- 中间件在 session store 缺失时失败关闭。
- `IsAdmin` 改为查询数据库权威关系。
- 请求中出现空 `roleIdList` 时执行关系替换。
- 用户删除、密码和授权变化后撤销 session。
- Redis session generation 机制作为批量撤销的基础。

### 9.2 需要调整

- 将空 `jwtSecret` 改为明确占位符，增加 Node 兼容的首次生成和写回流程。
- 从 session 结构和 Redis payload 删除 `Username`、`RoleIDs` 和 `TenantID`，中间件恢复从已验签 claims 构造用户上下文。
- 当前的角色分配校验仍依赖调用者 `Username`，必须改为只根据数据库目标和作用域校验。
- 不仅禁止新分配 `admin` 角色，还必须保护现有平台超管用户、角色和绑定不被管理端 HTTP CRUD 修改或删除，但保留超管的个人资料和密码更新能力。
- 授权变更和 session 撤销纳入统一用户行锁协议，避免并发登录签发旧权限 session。
- 增加本规格定义的安全回归测试。

当前草稿中的 bcrypt、通用租户隔离、上传、CRUD 上限和事务等其他改动不属于本规格，不在本设计中评价或回退。

## 10. 测试设计

### 10.1 单元测试

1. secret 占位符首次替换为 UUID，第二次保持不变。
2. 已有部署覆盖值时不修改配置文件。
3. secret 写回、rename 或重载失败时拒绝启动，错误不包含 secret。
4. JWT payload 被直接篡改后验签失败。
5. session 缺失、用户 ID 不匹配、密码版本不匹配和 refresh 重放被拒绝。
6. 用户名为 `admin` 但没有全局 admin 绑定时不是平台超管。
7. 租户用户绑定同名或错误作用域角色时不是平台超管。
8. 平台超管只能分配与目标用户作用域一致的非 `admin` 角色。
9. 非平台超管不能分配跨租户、全局或自己无权分配的角色。
10. `roleIdList` 缺失保持关系，空数组清空关系。
11. 用户和角色授权变化批量撤销受影响 session。
12. session 撤销失败时数据库事务回滚。

### 10.2 MySQL HTTP 集成测试

1. 普通用户篡改 JWT 的 `username`、`roleIds` 或 `tenantId` 后返回 401。
2. 新增或修改用户时提交全局 `admin` 角色返回 403，关系表不变。
3. 租户调用者分配跨租户或全局角色返回 403，关系表不变。
4. 唯一平台超管用户不能通过管理端用户 CRUD 更新、删除或移除绑定，但可以通过 `personUpdate` 修改自己的资料和密码。
5. 全局 `admin` 角色不能更新或删除。
6. 清空普通用户最后一个角色后，旧 access 和 refresh token 立即失效。
7. 禁用、修改角色、修改租户、修改用户名、修改密码和删除用户后，旧 session 失效。
8. 修改角色菜单或部门权限后，该角色的所有用户被登出。
9. 授权变更与并发登录不会产生携带旧权限的新 session。
10. 登录、刷新和用户信息响应继续符合现有 Node 协议 fixture。

集成测试只能在显式环境开关下连接专用测试库，不得使用默认开发库或共享库。

### 10.3 验收

实施计划必须包含：

```text
go test ./cool/auth ./cool/app ./modules/base/service/sys ./modules/base -count=1
go test ./... -count=1
go test -race ./cool/auth ./cool/app ./modules/base/service/sys -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
```

若安全集成用例不纳入现有 `AuthIntegration` 测试族，必须使用另一个名称明确、同样要求专用 MySQL 的显式开关。

## 11. 成功标准

1. 新克隆项目首次本地启动会将 JWT secret 占位符替换为 UUID，后续重启不变。
2. 部署注入 secret 时不修改本地配置文件。
3. 已验签 JWT 可继续作为身份快照，但用户名不再能单独授予超管权限。
4. 任何 HTTP 请求都无法新增、修改、移除或删除唯一平台超管身份；个人资料和密码更新不改变该身份。
5. 空 `roleIdList` 可以撤销最后一个普通角色。
6. 所有授权变更在成功响应前撤销受影响 session，且不存在并发登录窗口。
7. 上述单元、集成和 race 测试全部通过。
