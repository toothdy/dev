# cool-admin-go-next Auth Login Design

日期：2026-07-15

## 1. 目标

阶段 5A 只实现认证最小闭环，让 `cool-admin-go-next` 能兼容现有 `cool-admin-vue` 的基础登录链路：

1. admin 用户可以用初始化数据登录。
2. 登录成功返回 `token`、`expire`、`refreshToken`、`refreshExpire`。
3. 前端使用 `Authorization: <token>` 访问受保护接口。
4. access token 可解析为当前用户上下文。
5. refresh token 可换取新的 token pair。
6. 当前用户可以访问 `person`。

本设计不实现权限菜单、权限码、EPS runtime 和 Vue 全量联调。它们拆到阶段 5B 与阶段 6。

## 2. 范围

### 2.1 本期包含

Open 接口：

| Method | Path | 说明 |
|---|---|---|
| POST | `/admin/base/open/login` | 登录，返回 token pair |
| POST | `/admin/base/open/refreshToken` | 刷新 token pair |
| GET | `/admin/base/open/captcha` | 返回兼容结构，第一版不做真实验证码校验 |

Comm 接口：

| Method | Path | 说明 |
|---|---|---|
| GET | `/admin/base/comm/person` | 当前登录用户信息 |
| POST | `/admin/base/comm/logout` | 退出，第一版返回成功响应 |
| GET | `/admin/base/comm/program` | 返回 Go 程序标识 |

核心能力：

1. md5 密码兼容。
2. JWT access token / refresh token。
3. token middleware。
4. 当前用户上下文。
5. admin 登录集成测试。

### 2.2 本期不包含

1. `GET /admin/base/comm/permmenu`。
2. 菜单树构建。
3. 权限码计算。
4. CRUD 权限校验。
5. EPS runtime。
6. 文件上传。
7. `personUpdate`。
8. 真实图片验证码校验。
9. 操作日志完整记录。

## 3. 已有基础

当前仓库已经完成：

1. GoFrame v2 skeleton runtime。
2. Node 兼容响应结构 `cool/response`。
3. 错误码 `cool/errors`。
4. model metadata。
5. MySQL schema sync。
6. seed/menu 初始化导入。
7. metadata-driven CRUD runtime。
8. base 核心 CRUD 路由。

阶段 5A 必须复用这些能力，不引入 GoFrame 自动生成的 `dao/`、`internal/model/do/`、`internal/model/entity/`，也不创建 `logic/` 目录。

## 4. 架构

新增 `cool/auth` 作为认证核心包，负责 token、密码、上下文和 middleware。`cool/auth` 不依赖 base 模块，避免认证核心与业务模块互相耦合。

base 模块新增 auth/comm 路由注册文件，负责数据库查询和 HTTP handler。它依赖 `cool/auth`、`cool/response`、`cool/errors`、GoFrame `gdb`。

`cool/app.registerRoutes()` 的注册顺序：

```text
/health
  ↓
base open/comm auth routes
  ↓
auth middleware
  ↓
base CRUD routes
```

这样 open 登录接口可以放行，已有 CRUD 默认被 token middleware 保护。

## 5. 文件与职责

### 5.1 `cool/auth/password.go`

职责：Node 兼容密码算法。

接口：

```go
func MD5Password(password string) string
func VerifyMD5Password(password string, hashed string) bool
```

规则：

1. `MD5Password("123456")` 必须等于 `e10adc3949ba59abbe56e057f20f883e`。
2. 不引入 bcrypt/argon2；第一阶段必须兼容 Node 初始化数据。

### 5.2 `cool/auth/token.go`

职责：生成和解析 access token / refresh token。

核心类型：

```go
type Claims struct {
   IsRefresh       bool    `json:"isRefresh"`
   RoleIds         []int64 `json:"roleIds"`
   Username        string  `json:"username"`
   UserId          int64   `json:"userId"`
   PasswordVersion int64   `json:"passwordVersion"`
   TenantId        int64   `json:"tenantId"`
}

type TokenPair struct {
   Token         string `json:"token"`
   Expire        int64  `json:"expire"`
   RefreshToken  string `json:"refreshToken"`
   RefreshExpire int64  `json:"refreshExpire"`
}

type Manager struct {
   Secret        []byte
   Expire        int64
   RefreshExpire int64
}
```

接口：

```go
func NewManager(secret string, expire int64, refreshExpire int64) *Manager
func (m *Manager) GenerateTokenPair(claims Claims) (TokenPair, error)
func (m *Manager) ParseAccessToken(token string) (Claims, error)
func (m *Manager) ParseRefreshToken(token string) (Claims, error)
```

规则：

1. access token 中 `isRefresh` 为 `false`。
2. refresh token 中 `isRefresh` 为 `true`。
3. `ParseAccessToken` 遇到 refresh token 必须报错。
4. `ParseRefreshToken` 遇到 access token 必须报错。
5. token payload 字段必须包含协议文档要求的字段。

### 5.3 `cool/auth/context.go`

职责：把已解析用户写入 request context。

接口：

```go
type UserContext struct {
   UserId          int64
   Username        string
   RoleIds         []int64
   PasswordVersion int64
   TenantId        int64
}

func ContextWithUser(ctx context.Context, user UserContext) context.Context
func UserFromContext(ctx context.Context) (UserContext, bool)
```

规则：

1. handler 只能通过 `UserFromContext` 获取当前用户。
2. 未登录时返回 `false`。

### 5.4 `cool/auth/middleware.go`

职责：HTTP token middleware。

接口：

```go
type MiddlewareOptions struct {
   Manager     *Manager
   IgnorePaths []string
}

func NewMiddleware(options MiddlewareOptions) ghttp.HandlerFunc
func AuthorizationToken(r *ghttp.Request) string
```

规则：

1. 从 header `Authorization` 读取 token。
2. 不自动要求 `Bearer ` 前缀。
3. 如果用户传了 `Bearer xxx`，可以兼容性去掉前缀，但不能只支持 Bearer。
4. `IgnorePaths` 使用精确 path 匹配即可，不做复杂 pattern。
5. token 缺失或无效时返回 HTTP 401：

```json
{
  "code": 1001,
  "message": "登录失效~"
}
```

### 5.5 `modules/base/auth.go`

职责：base 登录与 comm handler。

建议类型：

```go
type AuthService struct {
   DB      gdb.DB
   Manager *auth.Manager
}

type LoginRequest struct {
   Username   string `json:"username"`
   Password   string `json:"password"`
   CaptchaId  string `json:"captchaId"`
   VerifyCode string `json:"verifyCode"`
}
```

核心方法：

```go
func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService
func (s *AuthService) Login(ctx context.Context, request LoginRequest) (auth.TokenPair, error)
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (auth.TokenPair, error)
func (s *AuthService) Person(ctx context.Context, userId int64) (map[string]interface{}, error)
```

数据库读取规则：

1. 从 `base_sys_user` 按 `username` 查询用户。
2. `status` 必须等于 `1`。
3. 使用 `md5(password)` 比对 `password` 字段。
4. 从 `base_sys_user_role` 查询当前用户角色 ID。
5. person 返回必须移除 `password`。
6. 字段输出使用 camelCase。

### 5.6 `modules/base/auth_routes.go`

职责：注册阶段 5A open/comm 路由。

接口：

```go
func RegisterAuthRoutes(server *ghttp.Server, service *AuthService)
func AuthIgnorePaths() []string
```

`AuthIgnorePaths()` 返回：

```text
/health
/admin/base/open/login
/admin/base/open/refreshToken
/admin/base/open/captcha
/admin/base/comm/program
```

路由响应：

1. login 成功：`response.OK(tokenPair)`。
2. refresh 成功：`response.OK(tokenPair)`。
3. refresh 失败：HTTP 401 + `response.Fail("登录失效~")`。
4. person 成功：`response.OK(user)`。
5. logout 成功：`response.OK(nil)`。
6. program 成功：`response.OK("Go")`。
7. captcha 成功：返回兼容结构：

```json
{
  "captchaId": "disabled",
  "data": ""
}
```

## 6. 配置

复用现有配置：

```yaml
cool:
  auth:
    jwtSecret: "cool-admin-go-next-dev-secret"
    tokenExpire: 7200
    refreshExpire: 604800
```

读取规则：

1. `jwtSecret` 默认值为 `cool-admin-go-next-dev-secret`。
2. `tokenExpire` 默认值为 `7200`。
3. `refreshExpire` 默认值为 `604800`。
4. 测试中可以直接注入 `auth.Manager`，避免依赖全局配置。

## 7. 错误处理

业务错误统一返回 Node 兼容结构。

| 场景 | HTTP Status | Body code | Body message |
|---|---:|---:|---|
| 登录成功 | 200 | 1000 | `success` |
| 用户名或密码错误 | 200 | 1001 | `账户或密码不正确~` |
| 用户禁用 | 200 | 1001 | `账户已被禁用~` |
| 缺少 access token | 401 | 1001 | `登录失效~` |
| access token 无效 | 401 | 1001 | `登录失效~` |
| refresh token 无效 | 401 | 1001 | `登录失效~` |
| 当前用户不存在 | 401 | 1001 | `登录失效~` |

内部错误必须用 `gerror.Wrap` 或 `gerror.Newf` 添加上下文。HTTP 响应不能暴露内部堆栈。

## 8. 测试策略

### 8.1 单元测试

新增：

```text
cool/auth/password_test.go
cool/auth/token_test.go
cool/auth/context_test.go
cool/auth/middleware_test.go
```

覆盖：

1. `MD5Password("123456")` 输出固定值。
2. 正确密码校验通过。
3. 错误密码校验失败。
4. access token 可解析。
5. refresh token 可解析。
6. refresh token 不能通过 `ParseAccessToken`。
7. access token 不能通过 `ParseRefreshToken`。
8. `ContextWithUser` / `UserFromContext` 可读写当前用户。
9. `AuthorizationToken` 支持裸 token。
10. `AuthorizationToken` 兼容 `Bearer token`。

### 8.2 base auth 单元测试

新增：

```text
modules/base/auth_test.go
modules/base/auth_routes_test.go
```

覆盖：

1. `AuthIgnorePaths` 包含 open 登录、刷新、验证码、program 和 health。
2. person 输出过滤 password。
3. 登录请求缺少 username/password 返回业务错误。

### 8.3 集成测试

新增：

```text
modules/base/auth_integration_test.go
```

环境变量：

```text
COOL_AUTH_INTEGRATION=1
```

测试流程：

```text
schema sync
  ↓
seed db/menu
  ↓
POST /admin/base/open/login admin/123456
  ↓
POST /admin/base/open/refreshToken
  ↓
GET /admin/base/comm/person with Authorization
  ↓
GET /admin/base/comm/person without Authorization
```

验收：

1. admin 登录成功。
2. 登录响应包含 `token`、`expire`、`refreshToken`、`refreshExpire`。
3. 错误密码登录返回 `code=1001`。
4. refreshToken 成功返回新的 token pair。
5. person 返回 username 为 `admin`。
6. person 不返回 `password`。
7. 无 token 访问 person 返回 HTTP 401。

## 9. 上下文控制策略

阶段 5A 执行必须控制主 agent 上下文：

1. 主 agent 只保留 spec、plan、任务摘要和 review 结论。
2. 每个实现任务由 fresh subagent 执行。
3. 子代理只返回修改文件、测试命令、结果和关键设计说明，不粘贴完整文件。
4. 主 agent 审阅时只读取必要 diff 或小范围文件。
5. 阶段 5B 权限菜单单独写 spec/plan，不提前塞入 5A。

## 10. 验收标准

阶段 5A 完成后必须满足：

1. `go test ./cool/auth ./modules/base -count=1` 通过。
2. `go test ./...` 通过。
3. `COOL_AUTH_INTEGRATION=1 go test ./modules/base -run Auth -count=1` 在真实 MySQL 上通过。
4. `POST /admin/base/open/login` 使用 admin / 123456 成功。
5. 登录响应符合协议契约。
6. `POST /admin/base/open/refreshToken` 成功。
7. `GET /admin/base/comm/person` 带 token 成功且不返回 password。
8. `GET /admin/base/comm/person` 不带 token 返回 HTTP 401。
9. 现有 CRUD 路由默认受 token middleware 保护。
10. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 11. 后续阶段 5B

阶段 5B 单独处理：

1. `GET /admin/base/comm/permmenu`。
2. admin 全量权限菜单。
3. 普通用户按角色计算权限菜单。
4. `perms` 权限码数组。
5. 菜单树或前端所需菜单结构。
6. 可选 `personUpdate`。
7. 可选 CRUD 权限校验。
