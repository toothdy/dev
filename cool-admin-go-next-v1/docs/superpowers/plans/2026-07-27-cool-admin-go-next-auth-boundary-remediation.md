# cool-admin-go-next 身份与授权边界修复实施计划

> **For agentic workers:** 按任务顺序实施，每个生产行为都必须先观察到失败测试，再完成最小实现。当前工作区包含未提交审计修复草稿，不得回退本计划之外的改动。

**Goal:** 保持 Node/Vue 登录协议兼容，实现 JWT secret 首次生成、数据库权威超管判定、唯一平台超管保护、角色分配范围和并发安全的 session 撤销。

**Architecture:** JWT 继续保存签发时身份快照；session 只保存会话有效性、JTI 摘要和密码版本。平台超管按用户 ID、平台用户作用域和全局 `admin` 角色关系查库判定。登录、刷新和授权写入共用按用户 ID 排序的 MySQL `FOR UPDATE` 锁协议，并在事务提交前批量撤销 session。

**Tech Stack:** Go 1.23、GoFrame v2.10.2（`gdb`、`gcfg`、`ghttp`）、MySQL、现有 `cool/auth`、`cool/app`、`cool/controller`、`modules/base/service/sys`、`gopkg.in/yaml.v3`。

**Design:** `docs/superpowers/specs/2026-07-27-cool-admin-go-next-auth-boundary-remediation-design.md`

## 全局约束

- 保持登录、刷新、`Authorization` header、JWT 业务 claims 和 401/403 body 兼容。
- 不硬编码超管用户名或角色 ID；受保护角色由平台作用域和 `label = "admin"` 识别。
- 管理端 User/Role CRUD 不得修改或删除平台超管身份；`personUpdate` 仍允许超管修改资料和密码。
- 所有 session store 错误均失败关闭；授权数据写入不得在 session 撤销失败后提交。
- 用户 ID 集合操作必须去重、升序排列，数据库加锁和 session 撤销使用相同顺序。
- 使用结构化 YAML 解析器更新 `cool.auth.jwtSecret`，不用全文字符串替换。
- secret 只在默认启动且有效值是占位符时写回；测试注入 `AuthManagerFactory` 时不触碰真实配置。
- 新增或修改的 Go 文件必须执行 `gofmt`；包管理使用 `go mod`；不得使用 `git add -A`。
- 每次提交只暂存当前任务的精确文件，保留 bcrypt、租户 CRUD、上传、限制和其他已有草稿改动。
- 对 `app.go`、`go.mod`、`go.sum`、`config.yaml`、auth/base service 等已脏且与任务重叠的文件，提交前必须逐文件审阅 `git diff HEAD -- <path>`。只有文件全部差异都已纳入当前审阅范围时才能整体暂存；否则延后该任务提交，不得强行夹带或回退其他改动。

## 文件结构

### 新增

- `cool/app/jwt_secret.go` - JWT secret 占位符识别、UUID 生成、YAML 节点更新和原子写回。
- `cool/app/jwt_secret_test.go` - 首次替换、幂等、部署覆盖、弱 secret 和写入失败测试。
- `modules/base/service/sys/auth_boundary.go` - 用户行锁、平台作用域、受保护 admin 关系和可分配角色的共享策略。
- `modules/base/service/sys/auth_boundary_test.go` - 纯输入归一化和数据库策略测试。
- `modules/base/auth_security_integration_test.go` - 显式 MySQL/HTTP 安全验收。

### 修改

- `manifest/config/config.yaml`
- `cool/app/app.go`、`cool/app/middleware_test.go`
- `cool/auth/session.go`、`cool/auth/session_test.go`、`cool/auth/middleware.go`
- `cool/auth/session_redis.go`（当前工作区中为未跟踪修复草稿）
- `cool/auth/session_redis_test.go`
- `modules/base/service/sys/login.go`、`login_session_test.go`
- `modules/base/service/sys/perms.go`、`perms_test.go`
- `modules/base/service/sys/user.go`、`user_test.go`
- `modules/base/service/sys/role.go`、`role_test.go`
- `modules/base/service/sys/department.go`、`department_test.go`
- `modules/base/controller/controllers.go`、`modules/base/register.go`
- `modules/base/auth_integration_test.go`

---

## Task 1：复刻 Node 的 JWT secret 首次生成

**Files:**
- Create: `cool/app/jwt_secret.go`
- Create: `cool/app/jwt_secret_test.go`
- Modify: `cool/app/app.go`
- Modify: `cool/app/middleware_test.go`
- Modify: `manifest/config/config.yaml`
- Modify: `go.mod`、`go.sum`（将已有间接 `gopkg.in/yaml.v3` 提升为直接依赖）

- [ ] **Step 1: 写 secret bootstrap 失败测试**

用 `t.TempDir()` 创建包含 `cool.auth.jwtSecret: cool-admin-go-next-xxxxxx` 的 YAML，断言：首次返回 UUID v4 并写回目标节点；第二次值不变；其他 YAML 节点语义不变；文件权限保留。再覆盖非占位 secret、无效 YAML、缺失节点、只读目录和 rename 失败。

- [ ] **Step 2: 运行 Red**

```bash
go test ./cool/app -run 'TestJWTSecret|TestBuildRejectsWeakJWTSecret' -count=1
```

预期：失败，因为 bootstrap helper 和占位符行为尚未实现。

- [ ] **Step 3: 实现结构化写回**

实现可测试的内部函数，输入配置路径和已解析的有效 secret，输出当次启动应使用的 secret。用 `yaml.Node` 定位 `cool -> auth -> jwtSecret`，只更新该 scalar；用 `github.com/google/uuid.NewRandom()` 生成 UUID v4；临时文件必须与目标文件同目录，`Sync` 、`Chmod` 后原子 rename。任何错误都不包含 secret。

只在 `StartServer = true` 且未注入 `AuthManagerFactory` 时执行 bootstrap。部署层有效 secret 不是占位符时不读写 YAML。bootstrap 返回的新值直接供当次 JWT Manager 使用，避免依赖 GoFrame 配置缓存刷新时序。

- [ ] **Step 4: 更新占位符和弱值校验**

将 `manifest/config/config.yaml` 的 secret 设为 `cool-admin-go-next-xxxxxx`。`unsafeJWTSecret` 必须区分“允许 bootstrap 的精确占位符”和“必须拒绝的空值、历史默认值、其他占位值或长度不足值”。

- [ ] **Step 5: 运行 Green 并精确提交**

```bash
gofmt -w cool/app/jwt_secret.go cool/app/jwt_secret_test.go cool/app/app.go cool/app/middleware_test.go
go test ./cool/app -count=1
git add cool/app/jwt_secret.go cool/app/jwt_secret_test.go cool/app/app.go cool/app/middleware_test.go manifest/config/config.yaml go.mod go.sum
git commit -m "fix: bootstrap local JWT secret"
```

---

## Task 2：收窄 session 责任并增加批量撤销

**Files:**
- Modify: `cool/auth/session.go`
- Modify: `cool/auth/session_test.go`
- Modify: `cool/auth/middleware.go`
- Modify: `cool/auth/session_redis.go`
- Create/Modify: `cool/auth/session_redis_test.go`
- Modify: `modules/base/service/sys/login.go`
- Modify: `modules/base/service/sys/login_session_test.go`

- [ ] **Step 1: 写 session 语义失败测试**

增加以下断言：`Session` 和 Redis payload 不再需要用户名、角色、租户；中间件在 session 匹配后从已验签 JWT 构造上下文；`DeleteUsers` 对重复和无序 ID 生效；空集合成功；任意失败必须返回 error。

Redis 测试使用实现 `RedisCommander` 的可控 fake，断言批量撤销只更新 generation key，不发出 `KEYS`、`SCAN` 或按 session 删除。

- [ ] **Step 2: 运行 Red**

```bash
go test ./cool/auth -run 'TestMemorySessionStore|TestRedisSessionStore|TestMiddleware' -count=1
```

- [ ] **Step 3: 实现最小 session API**

向 `SessionStore` 添加 `DeleteUsers(ctx context.Context, userIDs []int64) error`，并让 `DeleteUser` 委托单元素批量方法。内存 store 在单次加锁内完成；Redis store 为每个用户生成新 generation，使用单个 Lua 或等价原子批量命令写入。

从 `Session`、`storedRedisSession`、`sessionFromPair`、`storedFromSession` 和 `sessionFromStored` 删除重复身份字段。中间件保留 session 的用户 ID、密码版本和 SSO JTI 校验，然后使用 claims 构造 `UserContext`。

- [ ] **Step 4: 补齐已有 fake 和构造器**

编译所有实现 `SessionStore` 的测试 fake，不得通过放宽接口或恢复重复字段规避编译错误。

- [ ] **Step 5: 运行 Green 并提交**

```bash
gofmt -w cool/auth/session.go cool/auth/session_test.go cool/auth/middleware.go cool/auth/session_redis.go cool/auth/session_redis_test.go modules/base/service/sys/login.go modules/base/service/sys/login_session_test.go
go test ./cool/auth ./modules/base/service/sys -run 'Session|Middleware|Logout|Refresh' -count=1
git add cool/auth/session.go cool/auth/session_test.go cool/auth/middleware.go cool/auth/session_redis.go cool/auth/session_redis_test.go modules/base/service/sys/login.go modules/base/service/sys/login_session_test.go
git commit -m "refactor: keep authorization claims in JWT"
```

---

## Task 3：建立权威超管和角色分配策略

**Files:**
- Create: `modules/base/service/sys/auth_boundary.go`
- Create: `modules/base/service/sys/auth_boundary_test.go`
- Modify: `modules/base/service/sys/perms.go`
- Modify: `modules/base/service/sys/perms_test.go`

- [ ] **Step 1: 写权限策略失败测试**

替换现有“用户名 admin 直接是超管”测试，覆盖：只有用户名不授权；用户或角色在租户作用域不授权；用户禁用不授权；只有启用平台用户绑定全局 `admin` 角色才授权。

对可分配角色策略覆盖：任何调用者都不能分配全局 `admin`；平台超管可分配与目标用户作用域一致的普通角色；非超管只能分配同租户中自己创建或自己当前持有的角色。

- [ ] **Step 2: 运行 Red**

```bash
go test ./modules/base/service/sys -run 'TestPermissionServiceIsAdmin|TestAssignableRole|TestProtectedPlatformAdmin' -count=1
```

- [ ] **Step 3: 实现共享边界 helper**

实现：租户作用域归一化（`NULL` 和 `0` 都是平台）；按 ID 升序去重；按真实数据库关系查询平台超管；查询目标用户是否受保护；校验角色的作用域、创建者和调用者现有绑定。

错误语义固定为 `coolErrors.Forbidden("非法操作")`，不返回“哪个角色是 admin”或其他可枚举细节。

- [ ] **Step 4: 使 `PermissionService.IsAdmin` 委托权威查询**

保留 `IsAdmin(ctx, user)` 公开签名，但只使用 `user.UserId`；删除所有用户名分支和测试特例。

- [ ] **Step 5: 运行 Green 并提交**

```bash
gofmt -w modules/base/service/sys/auth_boundary.go modules/base/service/sys/auth_boundary_test.go modules/base/service/sys/perms.go modules/base/service/sys/perms_test.go
go test ./modules/base/service/sys -run 'Admin|AssignableRole|Protected' -count=1
git add modules/base/service/sys/auth_boundary.go modules/base/service/sys/auth_boundary_test.go modules/base/service/sys/perms.go modules/base/service/sys/perms_test.go
git commit -m "fix: enforce platform admin authority"
```

---

## Task 4：让登录和刷新参与用户行锁协议

**Files:**
- Modify: `modules/base/service/sys/auth_boundary.go`
- Modify: `modules/base/service/sys/login.go`
- Modify: `modules/base/service/sys/login_session_test.go`
- Modify: `modules/base/auth_integration_test.go`

- [ ] **Step 1: 写登录/刷新并发失败测试**

在单元层断言刷新会重读用户、角色和租户，用户被禁用或角色为空时返回 401，且 session rotate 失败不返回 token pair。在 MySQL 集成测试中用两个独立连接验证授权事务持有用户行锁时，登录/刷新不能读取旧快照并保存新 session。

- [ ] **Step 2: 运行 Red**

```bash
go test ./modules/base/service/sys -run 'Login|Refresh|Session' -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run 'AuthIntegration.*Concurrent' -count=1
```

- [ ] **Step 3: 实现共享 `FOR UPDATE` 协议**

在 `auth_boundary.go` 提供只接受常量表名和占位参数的用户行锁 helper，生成 `SELECT id FROM base_sys_user WHERE id IN (...) ORDER BY id FOR UPDATE`。登录在验证码消费后进入事务，按 username 找到用户后锁定该 ID，再校验密码、读取角色、签发 token 并保存 session。

刷新先验证 refresh token 和当前 session，然后进入事务锁定 claims 中的用户 ID，重读最新用户/角色/租户快照，在锁释放前完成 refresh JTI 轮换。现有 MD5 -> bcrypt 登录升级必须使用同一事务，不得在行锁外再次写密码版本。

- [ ] **Step 4: 运行 Green 并提交**

```bash
gofmt -w modules/base/service/sys/auth_boundary.go modules/base/service/sys/login.go modules/base/service/sys/login_session_test.go modules/base/auth_integration_test.go
go test ./modules/base/service/sys -run 'Login|Refresh|Session' -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
git add modules/base/service/sys/auth_boundary.go modules/base/service/sys/login.go modules/base/service/sys/login_session_test.go modules/base/auth_integration_test.go
git commit -m "fix: serialize login with authorization changes"
```

---

## Task 5：保护用户授权变更并在事务内撤销 session

**Files:**
- Modify: `modules/base/service/sys/user.go`
- Modify: `modules/base/service/sys/user_test.go`
- Modify: `modules/base/service/sys/department.go`
- Modify: `modules/base/service/sys/department_test.go`

- [ ] **Step 1: 写 User/Department 失败测试**

覆盖：任何调用者都不能为新或现有用户提交全局 `admin` 角色；非超管不能操作跨租户用户或分配不可分配角色；目标用户是真实平台超管时 Update/Delete 返回 403，即使请求伪造或省略 username；缺失 `roleIdList` 保持关系，空数组清空；session 撤销失败回滚用户和关系写入。

补充部门 `deleteUser = true` 路径：受保护超管不能被级联删除，不以 username 识别；被删用户的 session 在事务提交前批量撤销。

- [ ] **Step 2: 运行 Red**

```bash
go test ./modules/base/service/sys -run 'User|Department|Assignable|Protected|Revoke' -count=1
```

- [ ] **Step 3: 收敛 UserService 写入流程**

将角色分配策略和目标用户保护校验移入同一数据库事务。Update/Delete 首先锁定真实目标用户行，所有影响授权的更改在写库后、提交前调用 `DeleteUsers`。`personUpdate` 只保留现有允许字段；超管自改密码允许，并在事务内撤销自己全部 session。

将现有基于 `current["username"] == "admin"` 和目标请求 username 的保护全部替换为数据库关系判定。

- [ ] **Step 4: 收敛 DepartmentService 级联删除**

按 ID 锁定所有部门用户，使用共享平台超管保护 helper，然后删除用户/关系并在事务中批量撤销 session。不删除用户、只迁移部门时不需要撤销，因为用户的 JWT 授权快照不包含部门关系。

- [ ] **Step 5: 运行 Green 并提交**

```bash
gofmt -w modules/base/service/sys/user.go modules/base/service/sys/user_test.go modules/base/service/sys/department.go modules/base/service/sys/department_test.go
go test ./modules/base/service/sys -run 'User|Department|Assignable|Protected|Revoke' -count=1
git add modules/base/service/sys/user.go modules/base/service/sys/user_test.go modules/base/service/sys/department.go modules/base/service/sys/department_test.go
git commit -m "fix: protect user authorization mutations"
```

---

## Task 6：保护角色变更并撤销受影响用户

**Files:**
- Modify: `modules/base/service/sys/role.go`
- Modify: `modules/base/service/sys/role_test.go`
- Modify: `modules/base/controller/controllers.go`
- Modify: `modules/base/register.go`

- [ ] **Step 1: 写 RoleService 失败测试**

覆盖：全局 `admin` 角色 Update/Delete 返回 403，不依赖 ID `1`；更新菜单或部门关系会收集该角色所有用户；删除普通角色会撤销原绑定用户；session 批量撤销失败回滚角色和关系写入。

- [ ] **Step 2: 运行 Red**

```bash
go test ./modules/base/service/sys -run 'Role|Protected|Revoke' -count=1
```

- [ ] **Step 3: 为 RoleService 注入共享 SessionStore**

添加 `NewRoleServiceWithSessions`，保留 `NewRoleService` 作为兼容构造器。在 `ControllersWithOptions` 中与 User/Department 使用同一个 store，不在 RoleService 内自建 memory store。

- [ ] **Step 4: 将保护、写入和撤销纳入同一事务**

用数据库真实 label 和作用域识别受保护角色。Update/Delete 在事务中先查询受影响用户 ID，按升序锁定用户行，再写入或删除角色关系，最后批量撤销 session 并提交。

- [ ] **Step 5: 运行 Green 并提交**

```bash
gofmt -w modules/base/service/sys/role.go modules/base/service/sys/role_test.go modules/base/controller/controllers.go modules/base/register.go
go test ./modules/base/service/sys ./modules/base/controller ./modules/base -run 'Role|Controllers' -count=1
git add modules/base/service/sys/role.go modules/base/service/sys/role_test.go modules/base/controller/controllers.go modules/base/register.go
git commit -m "fix: revoke sessions after role changes"
```

---

## Task 7：增加端到端安全回归

**Files:**
- Create: `modules/base/auth_security_integration_test.go`
- Modify: `modules/base/auth_integration_test.go`
- Modify: `cool/auth/token_test.go`

- [ ] **Step 1: 建立专用 fixture**

在已有 `COOL_AUTH_INTEGRATION=1` 专用库保护下，创建：唯一平台超管、平台普通用户、两个租户、各自普通角色和一个非超管但具有用户管理权限的租户调用者。每个 ID 使用与其他集成测试不重叠的固定范围。

- [ ] **Step 2: 写 HTTP 失败用例**

精确断言：

- 直接修改 JWT payload 中任一 `username/roleIds/tenantId` 而不重签时返回 401。
- 租户用户提交全局 `admin` 或另一租户角色时返回 403，关系表不变。
- 平台超管也不能向第二个用户分配全局 `admin`。
- 以真实 ID 更新/删除平台超管用户，或更新/删除全局 `admin` 角色时返回 403。
- 平台超管通过 `personUpdate` 更新资料成功，更改密码后旧 token pair 失效。
- 清空最后角色、禁用、修改用户名/租户/密码、删除用户后，旧 access 和 refresh token 均失效。
- 修改普通角色的菜单/部门关系后，该角色所有用户旧 token 失效。
- 授权变更与并发登录/刷新最终只能产生新授权快照或 401，不能产生仍可访问的旧权限 session。

- [ ] **Step 3: 运行 Red，完成 fixture/handler 所需最小调整后转 Green**

```bash
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run 'Auth(Security)?Integration' -count=1
```

- [ ] **Step 4: 验证 Node 协议不变**

复用已有 login/refresh/person fixture 断言，并在 `token_test.go` 保留 claims 字段、access/refresh 类型分离和签名篡改拒绝测试。

- [ ] **Step 5: 运行 Green 并提交**

```bash
gofmt -w modules/base/auth_security_integration_test.go modules/base/auth_integration_test.go cool/auth/token_test.go
go test ./cool/auth ./modules/base -count=1
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run 'Auth(Security)?Integration' -count=1
git add modules/base/auth_security_integration_test.go modules/base/auth_integration_test.go cool/auth/token_test.go
git commit -m "test: cover auth boundary regressions"
```

---

## Task 8：全量验证与审计收尾

**Files:**
- Modify when accurate: `docs/security-audit-2026-07-23.md`
- Modify when behavior changed: `README.md`

- [ ] **Step 1: 运行静态和单元验证**

```bash
go vet ./cool/... ./modules/base/...
go test ./cool/auth ./cool/app ./modules/base/service/sys ./modules/base -count=1
go test ./... -count=1
go test -race ./cool/auth ./cool/app ./modules/base/service/sys -count=1
```

- [ ] **Step 2: 运行专用 MySQL 验收**

```bash
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run 'Auth(Security)?Integration' -count=1
```

运行前必须确认连接的是专用测试库；默认开发库或共享库必须拒绝。

- [ ] **Step 3: 检查协议和安全敏感信息**

确认：配置写回和错误日志不输出 secret；JWT 响应字段未删减；非法操作为 403；普通登录失效为 401；不存在 `Username == "admin"` 或角色 ID `1` 授权分支。

- [ ] **Step 4: 更新文档**

只在上述验证全部通过后，在审计文档中将 #1/#2/#10/#11 标记为已修复，并明确 #1 的条件前提和 Node 首次 secret 生成机制。README 只补充首次本地启动会写回 secret 占位符、授权变更会导致重新登录的运行说明。

- [ ] **Step 5: 审查实际 diff 并提交文档**

```bash
git diff --check
git status --short
git diff -- docs/security-audit-2026-07-23.md README.md
git add docs/security-audit-2026-07-23.md README.md
git commit -m "docs: close auth boundary audit findings"
```

若某个文档没有实际改动，不得为了满足命令而产生空润色或元数据更改。

## 最终完成条件

- JWT secret 占位符只在首次本地启动替换，部署覆盖不写文件，弱值启动失败。
- JWT 保持现有 claims，session 不再保存重复身份快照。
- 超管只由启用平台用户与全局 `admin` 角色的数据库关系决定。
- 所有 HTTP 路径都无法新增或破坏唯一平台超管身份，但超管可以自改资料和密码。
- 角色分配符合平台/租户作用域和调用者可分配范围。
- 空 `roleIdList` 清空角色，用户/角色授权变更在提交前批量撤销 session。
- 并发登录/刷新不能在授权变更窗口签发可用的旧权限 session。
- 所有聚焦、全量、race 和显式 MySQL 安全集成测试通过。
