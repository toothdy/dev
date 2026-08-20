# cool-admin-go-next Auth Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现阶段 5A Auth 最小登录链路，让 admin 可以登录、刷新 token，并通过 `Authorization: <token>` 获取当前用户信息。

**Architecture:** 新增 `cool/auth` 认证核心包，负责 md5 密码、JWT token、用户上下文和 middleware。base 模块新增 auth service 与 open/comm 路由，复用现有 schema sync、seed、response、errors、model 与 CRUD runtime。阶段 5A 不实现权限菜单、权限码、EPS、文件上传或完整操作日志；阶段 5B 单独处理权限菜单。

**Tech Stack:** Go 1.23+、GoFrame v2.10.2、GoFrame gdb/ghttp/gcfg/gerror、JWT HMAC-SHA256 标准库实现、MySQL 8.x、标准库 `crypto/md5` / `crypto/hmac` / `crypto/sha256` / `encoding/base64` / `encoding/json` / `time`。

## Global Constraints

- 始终用中文编写说明文档和代码注释。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL。
- 阶段 5A 只实现 auth/login/person 最小闭环，不实现权限菜单和 EPS。
- 密码校验必须兼容 Node 版 `md5(password)`。
- 前端请求 header 使用 `Authorization: <token>`，Go 版不能只支持 `Bearer <token>`。
- refreshToken 接口失败时必须返回 HTTP `401` 和 `{ "code": 1001, "message": "登录失效~" }`。
- 不使用 `git add -A`；提交时只显式 stage 本计划创建或修改的文件。
- GoFrame 自动生成文件后续必须由工具生成，不手写、不手改；本计划不创建 `dao/`、`internal/model/do/`、`internal/model/entity/`。
- 不使用 `logic/` 目录，业务逻辑直接放在 `cool/auth` 和 `modules/base`。
- Go 代码错误处理使用 GoFrame `gerror` 包装上下文。
- Go 文件内如果有 3 个及以上相关变量声明，使用 `var (...)` 分组。
- 为控制主 agent 上下文，实施阶段每个 Task 使用 fresh subagent，子代理只返回摘要、测试结果和关键 diff 说明。

---

## Scope Check

本计划只覆盖设计文档 `docs/superpowers/specs/2026-07-15-cool-admin-go-next-auth-login-design.md` 的阶段 5A。

包含：

1. `cool/auth` 密码、token、上下文、middleware。
2. base 登录、刷新、captcha 占位、person、logout、program。
3. app 接入 auth manager、auth routes、auth middleware。
4. 单元测试和真实 MySQL 集成测试。
5. README 阶段说明更新。

不包含：

1. `GET /admin/base/comm/permmenu`。
2. 菜单树和权限码。
3. CRUD 权限校验。
4. EPS runtime。
5. 文件上传。
6. `personUpdate`。
7. 真实验证码图片和校验。
8. 操作日志完整记录。

---

## File Structure

### 创建文件

- `cool/auth/password.go`  
  Node 兼容 md5 密码工具。

- `cool/auth/password_test.go`  
  测试 md5 输出和密码校验。

- `cool/auth/token.go`  
  HMAC-SHA256 JWT 生成和解析，不新增第三方 JWT 依赖。

- `cool/auth/token_test.go`  
  测试 access token、refresh token、过期和类型校验。

- `cool/auth/context.go`  
  当前登录用户上下文读写。

- `cool/auth/context_test.go`  
  测试上下文写入和读取。

- `cool/auth/middleware.go`  
  GoFrame token middleware 和 Authorization header 解析。

- `cool/auth/middleware_test.go`  
  测试裸 token / Bearer token 解析、放行路径。

- `modules/base/auth.go`  
  base auth service：登录、刷新、person 查询。

- `modules/base/auth_test.go`  
  测试请求校验、用户字段过滤、ignore paths。

- `modules/base/auth_routes.go`  
  注册 open/comm auth 路由。

- `modules/base/auth_integration_test.go`  
  `COOL_AUTH_INTEGRATION=1` 时执行真实 MySQL 登录链路测试。

### 修改文件

- `cool/app/app.go`  
  增加 auth manager runner/options，注册 auth routes 和 middleware。

- `cool/app/app_test.go`  
  测试 auth 初始化和 run 顺序不破坏 schema/seed。

- `modules/base/routes.go`  
  保持 CRUD 路由不变；如需共享 ignore paths，不在此文件混入 auth 逻辑。

- `README.md`  
  更新当前阶段和 auth 验收命令。

### 不创建文件

- 不创建 `dao/`。
- 不创建 `internal/model/do/`。
- 不创建 `internal/model/entity/`。
- 不创建 `logic/`。
- 不新增第三方 JWT 依赖。

---

## Implementation Tasks

### Task 1: 增加 `cool/auth` 密码和 token 核心

**Files:**
- Create: `cool/auth/password.go`
- Create: `cool/auth/password_test.go`
- Create: `cool/auth/token.go`
- Create: `cool/auth/token_test.go`
- Test: `go test ./cool/auth -run 'Test(MD5|Token|Refresh|Expired)' -count=1`

**Interfaces:**
- Consumes: none.
- Produces:
  - `func MD5Password(password string) string`
  - `func VerifyMD5Password(password string, hashed string) bool`
  - `type Claims struct`
  - `type TokenPair struct`
  - `type Manager struct`
  - `func NewManager(secret string, expire int64, refreshExpire int64) *Manager`
  - `func (m *Manager) GenerateTokenPair(claims Claims) (TokenPair, error)`
  - `func (m *Manager) ParseAccessToken(token string) (Claims, error)`
  - `func (m *Manager) ParseRefreshToken(token string) (Claims, error)`

- [ ] **Step 1: Create auth directory and password tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/auth
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/password_test.go`:

```go
package auth_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

func TestMD5PasswordMatchesNodeSeed(t *testing.T) {
   hashed := auth.MD5Password("123456")
   if hashed != "e10adc3949ba59abbe56e057f20f883e" {
      t.Fatalf("expected node md5 hash, got %s", hashed)
   }
}

func TestVerifyMD5Password(t *testing.T) {
   hashed := "e10adc3949ba59abbe56e057f20f883e"
   if !auth.VerifyMD5Password("123456", hashed) {
      t.Fatal("expected correct password to pass")
   }
   if auth.VerifyMD5Password("wrong", hashed) {
      t.Fatal("expected wrong password to fail")
   }
}
```

- [ ] **Step 2: Run failing password tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth -run TestMD5 -count=1
```

Expected: FAIL because `MD5Password` does not exist.

- [ ] **Step 3: Implement password helpers**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/password.go`:

```go
package auth

import (
   "crypto/md5"
   "encoding/hex"
)

/**
 * 生成 Node 兼容 md5 密码
 * @param password 明文密码
 * @returns string
 */
func MD5Password(password string) string {
   sum := md5.Sum([]byte(password))
   return hex.EncodeToString(sum[:])
}

/**
 * 校验 Node 兼容 md5 密码
 * @param password 明文密码
 * @param hashed 已保存 hash
 * @returns bool
 */
func VerifyMD5Password(password string, hashed string) bool {
   return MD5Password(password) == hashed
}
```

- [ ] **Step 4: Add token tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/token_test.go`:

```go
package auth_test

import (
   "testing"
   "time"

   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

func testClaims() auth.Claims {
   return auth.Claims{
      RoleIds:         []int64{1, 2},
      Username:        "admin",
      UserId:          1,
      PasswordVersion: 1,
      TenantId:        0,
   }
}

func TestTokenPairCanBeParsed(t *testing.T) {
   manager := auth.NewManager("secret", 7200, 604800)
   pair, err := manager.GenerateTokenPair(testClaims())
   if err != nil {
      t.Fatalf("generate token pair failed: %v", err)
   }
   if pair.Token == "" || pair.RefreshToken == "" {
      t.Fatalf("expected token pair, got %#v", pair)
   }
   if pair.Expire != 7200 || pair.RefreshExpire != 604800 {
      t.Fatalf("unexpected expires: %#v", pair)
   }

   claims, err := manager.ParseAccessToken(pair.Token)
   if err != nil {
      t.Fatalf("parse access token failed: %v", err)
   }
   if claims.IsRefresh {
      t.Fatal("expected access token isRefresh=false")
   }
   if claims.Username != "admin" || claims.UserId != 1 {
      t.Fatalf("unexpected claims: %#v", claims)
   }

   refreshClaims, err := manager.ParseRefreshToken(pair.RefreshToken)
   if err != nil {
      t.Fatalf("parse refresh token failed: %v", err)
   }
   if !refreshClaims.IsRefresh {
      t.Fatal("expected refresh token isRefresh=true")
   }
}

func TestTokenTypeMismatchFails(t *testing.T) {
   manager := auth.NewManager("secret", 7200, 604800)
   pair, err := manager.GenerateTokenPair(testClaims())
   if err != nil {
      t.Fatalf("generate token pair failed: %v", err)
   }
   if _, err = manager.ParseAccessToken(pair.RefreshToken); err == nil {
      t.Fatal("expected refresh token rejected as access token")
   }
   if _, err = manager.ParseRefreshToken(pair.Token); err == nil {
      t.Fatal("expected access token rejected as refresh token")
   }
}

func TestExpiredTokenFails(t *testing.T) {
   manager := auth.NewManager("secret", -1, -1)
   pair, err := manager.GenerateTokenPair(testClaims())
   if err != nil {
      t.Fatalf("generate token pair failed: %v", err)
   }
   time.Sleep(1100 * time.Millisecond)
   if _, err = manager.ParseAccessToken(pair.Token); err == nil {
      t.Fatal("expected expired access token error")
   }
}
```

- [ ] **Step 5: Run failing token tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth -run 'Test(Token|Expired)' -count=1
```

Expected: FAIL because token types and manager do not exist.

- [ ] **Step 6: Implement token manager**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/token.go`:

```go
package auth

import (
   "crypto/hmac"
   "crypto/sha256"
   "encoding/base64"
   "encoding/json"
   "strings"
   "time"

   "github.com/gogf/gf/v2/errors/gerror"
)

// Claims 是 Node 兼容 JWT payload。
type Claims struct {
   IsRefresh       bool    `json:"isRefresh"`
   RoleIds         []int64 `json:"roleIds"`
   Username        string  `json:"username"`
   UserId          int64   `json:"userId"`
   PasswordVersion int64   `json:"passwordVersion"`
   TenantId        int64   `json:"tenantId"`
   ExpiresAt       int64   `json:"exp"`
}

// TokenPair 是登录和刷新接口返回结构。
type TokenPair struct {
   Token         string `json:"token"`
   Expire        int64  `json:"expire"`
   RefreshToken  string `json:"refreshToken"`
   RefreshExpire int64  `json:"refreshExpire"`
}

// Manager 是 JWT token 管理器。
type Manager struct {
   Secret        []byte
   Expire        int64
   RefreshExpire int64
}

/**
 * 创建 token 管理器
 * @param secret JWT 密钥
 * @param expire access token 有效秒数
 * @param refreshExpire refresh token 有效秒数
 * @returns *Manager
 */
func NewManager(secret string, expire int64, refreshExpire int64) *Manager {
   return &Manager{
      Secret:        []byte(secret),
      Expire:        expire,
      RefreshExpire: refreshExpire,
   }
}

/**
 * 生成 token pair
 * @param claims 业务 claims
 * @returns TokenPair
 */
func (m *Manager) GenerateTokenPair(claims Claims) (TokenPair, error) {
   now := time.Now().Unix()
   accessClaims := claims
   accessClaims.IsRefresh = false
   accessClaims.ExpiresAt = now + m.Expire

   refreshClaims := claims
   refreshClaims.IsRefresh = true
   refreshClaims.ExpiresAt = now + m.RefreshExpire

   token, err := m.sign(accessClaims)
   if err != nil {
      return TokenPair{}, gerror.Wrap(err, "生成 access token 失败")
   }
   refreshToken, err := m.sign(refreshClaims)
   if err != nil {
      return TokenPair{}, gerror.Wrap(err, "生成 refresh token 失败")
   }

   return TokenPair{
      Token:         token,
      Expire:        m.Expire,
      RefreshToken:  refreshToken,
      RefreshExpire: m.RefreshExpire,
   }, nil
}

/**
 * 解析 access token
 * @param token JWT
 * @returns Claims
 */
func (m *Manager) ParseAccessToken(token string) (Claims, error) {
   claims, err := m.parse(token)
   if err != nil {
      return Claims{}, err
   }
   if claims.IsRefresh {
      return Claims{}, gerror.New("refresh token 不能作为 access token 使用")
   }
   return claims, nil
}

/**
 * 解析 refresh token
 * @param token JWT
 * @returns Claims
 */
func (m *Manager) ParseRefreshToken(token string) (Claims, error) {
   claims, err := m.parse(token)
   if err != nil {
      return Claims{}, err
   }
   if !claims.IsRefresh {
      return Claims{}, gerror.New("access token 不能作为 refresh token 使用")
   }
   return claims, nil
}

func (m *Manager) sign(claims Claims) (string, error) {
   header := map[string]string{
      "alg": "HS256",
      "typ": "JWT",
   }
   headerData, err := json.Marshal(header)
   if err != nil {
      return "", err
   }
   payloadData, err := json.Marshal(claims)
   if err != nil {
      return "", err
   }

   signingInput := base64.RawURLEncoding.EncodeToString(headerData) + "." + base64.RawURLEncoding.EncodeToString(payloadData)
   signature := m.signature(signingInput)
   return signingInput + "." + signature, nil
}

func (m *Manager) parse(token string) (Claims, error) {
   parts := strings.Split(token, ".")
   if len(parts) != 3 {
      return Claims{}, gerror.New("token 格式错误")
   }
   signingInput := parts[0] + "." + parts[1]
   if !hmac.Equal([]byte(parts[2]), []byte(m.signature(signingInput))) {
      return Claims{}, gerror.New("token 签名无效")
   }

   payloadData, err := base64.RawURLEncoding.DecodeString(parts[1])
   if err != nil {
      return Claims{}, gerror.Wrap(err, "解析 token payload 失败")
   }
   claims := Claims{}
   if err = json.Unmarshal(payloadData, &claims); err != nil {
      return Claims{}, gerror.Wrap(err, "反序列化 token payload 失败")
   }
   if claims.ExpiresAt <= time.Now().Unix() {
      return Claims{}, gerror.New("token 已过期")
   }
   return claims, nil
}

func (m *Manager) signature(signingInput string) string {
   mac := hmac.New(sha256.New, m.Secret)
   mac.Write([]byte(signingInput))
   return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
```

- [ ] **Step 7: Run auth token tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth -run 'Test(MD5|Verify|Token|Expired)' -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/auth/password.go cool/auth/password_test.go cool/auth/token.go cool/auth/token_test.go
git commit -m "feat: add auth token core" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 增加 auth context 和 middleware

**Files:**
- Create: `cool/auth/context.go`
- Create: `cool/auth/context_test.go`
- Create: `cool/auth/middleware.go`
- Create: `cool/auth/middleware_test.go`
- Test: `go test ./cool/auth -run 'Test(Context|Authorization|Middleware)' -count=1`

**Interfaces:**
- Consumes:
  - `type Manager struct`
  - `func (m *Manager) ParseAccessToken(token string) (Claims, error)`
- Produces:
  - `type UserContext struct`
  - `func ContextWithUser(ctx context.Context, user UserContext) context.Context`
  - `func UserFromContext(ctx context.Context) (UserContext, bool)`
  - `type MiddlewareOptions struct`
  - `func NewMiddleware(options MiddlewareOptions) ghttp.HandlerFunc`
  - `func AuthorizationToken(r *ghttp.Request) string`
  - `func Unauthorized(r *ghttp.Request)`

- [ ] **Step 1: Write context tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/context_test.go`:

```go
package auth_test

import (
   "context"
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

func TestContextWithUser(t *testing.T) {
   user := auth.UserContext{
      UserId:          1,
      Username:        "admin",
      RoleIds:         []int64{1},
      PasswordVersion: 1,
      TenantId:        0,
   }
   ctx := auth.ContextWithUser(context.Background(), user)
   got, ok := auth.UserFromContext(ctx)
   if !ok {
      t.Fatal("expected user from context")
   }
   if got.UserId != 1 || got.Username != "admin" {
      t.Fatalf("unexpected user context: %#v", got)
   }
}

func TestUserFromEmptyContext(t *testing.T) {
   if _, ok := auth.UserFromContext(context.Background()); ok {
      t.Fatal("expected no user from empty context")
   }
}
```

- [ ] **Step 2: Implement auth context**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/context.go`:

```go
package auth

import "context"

type userContextKey struct{}

// UserContext 是当前登录用户上下文。
type UserContext struct {
   UserId          int64
   Username        string
   RoleIds         []int64
   PasswordVersion int64
   TenantId        int64
}

/**
 * 写入当前用户上下文
 * @param ctx 上下文
 * @param user 当前用户
 * @returns context.Context
 */
func ContextWithUser(ctx context.Context, user UserContext) context.Context {
   return context.WithValue(ctx, userContextKey{}, user)
}

/**
 * 从上下文读取当前用户
 * @param ctx 上下文
 * @returns UserContext 和是否存在
 */
func UserFromContext(ctx context.Context) (UserContext, bool) {
   user, ok := ctx.Value(userContextKey{}).(UserContext)
   return user, ok
}
```

- [ ] **Step 3: Write middleware tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/middleware_test.go`:

```go
package auth_test

import (
   "net/http/httptest"
   "testing"

   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

func TestAuthorizationTokenSupportsRawToken(t *testing.T) {
   request := httptest.NewRequest("GET", "/", nil)
   request.Header.Set("Authorization", "raw.token.value")
   r := &ghttp.Request{Request: request}

   if token := auth.AuthorizationToken(r); token != "raw.token.value" {
      t.Fatalf("expected raw token, got %q", token)
   }
}

func TestAuthorizationTokenSupportsBearerToken(t *testing.T) {
   request := httptest.NewRequest("GET", "/", nil)
   request.Header.Set("Authorization", "Bearer raw.token.value")
   r := &ghttp.Request{Request: request}

   if token := auth.AuthorizationToken(r); token != "raw.token.value" {
      t.Fatalf("expected bearer token stripped, got %q", token)
   }
}

func TestMiddlewareOptionsIgnorePath(t *testing.T) {
   options := auth.MiddlewareOptions{
      Manager:     auth.NewManager("secret", 7200, 604800),
      IgnorePaths: []string{"/admin/base/open/login"},
   }
   if !options.IsIgnored("/admin/base/open/login") {
      t.Fatal("expected login path ignored")
   }
   if options.IsIgnored("/admin/base/comm/person") {
      t.Fatal("expected person path protected")
   }
}
```

- [ ] **Step 4: Implement middleware**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/auth/middleware.go`:

```go
package auth

import (
   "net/http"
   "strings"

   "github.com/gogf/gf/v2/net/ghttp"
   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

// MiddlewareOptions 是 auth middleware 配置。
type MiddlewareOptions struct {
   Manager     *Manager
   IgnorePaths []string
}

/**
 * 路径是否放行
 * @param path 请求路径
 * @returns bool
 */
func (o MiddlewareOptions) IsIgnored(path string) bool {
   for _, item := range o.IgnorePaths {
      if item == path {
         return true
      }
   }
   return false
}

/**
 * 创建 token middleware
 * @param options middleware 配置
 * @returns ghttp.HandlerFunc
 */
func NewMiddleware(options MiddlewareOptions) ghttp.HandlerFunc {
   return func(r *ghttp.Request) {
      if options.IsIgnored(r.URL.Path) {
         r.Middleware.Next()
         return
      }
      token := AuthorizationToken(r)
      if token == "" || options.Manager == nil {
         Unauthorized(r)
         return
      }
      claims, err := options.Manager.ParseAccessToken(token)
      if err != nil {
         Unauthorized(r)
         return
      }
      r.SetCtx(ContextWithUser(r.Context(), UserContext{
         UserId:          claims.UserId,
         Username:        claims.Username,
         RoleIds:         claims.RoleIds,
         PasswordVersion: claims.PasswordVersion,
         TenantId:        claims.TenantId,
      }))
      r.Middleware.Next()
   }
}

/**
 * 从请求读取 Authorization token
 * @param r HTTP 请求
 * @returns string
 */
func AuthorizationToken(r *ghttp.Request) string {
   value := strings.TrimSpace(r.Header.Get("Authorization"))
   if value == "" {
      return ""
   }
   if strings.HasPrefix(strings.ToLower(value), "bearer ") {
      return strings.TrimSpace(value[7:])
   }
   return value
}

/**
 * 写入未授权响应
 * @param r HTTP 请求
 * @returns null
 */
func Unauthorized(r *ghttp.Request) {
   r.Response.WriteStatus(http.StatusUnauthorized)
   r.Response.WriteJson(response.Body{
      Code:    coolErrors.CodeCommFail,
      Message: "登录失效~",
   })
}
```

- [ ] **Step 5: Run auth middleware tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth -run 'Test(Context|Authorization|Middleware)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Run all auth tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 2**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/auth/context.go cool/auth/context_test.go cool/auth/middleware.go cool/auth/middleware_test.go
git commit -m "feat: add auth middleware" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 实现 base auth service

**Files:**
- Create: `modules/base/auth.go`
- Create: `modules/base/auth_test.go`
- Test: `go test ./modules/base -run 'Test(Auth|Login|Person|Refresh)' -count=1`

**Interfaces:**
- Consumes:
  - `auth.Manager`
  - `auth.Claims`
  - `auth.TokenPair`
  - `auth.VerifyMD5Password(password string, hashed string) bool`
- Produces:
  - `type AuthService struct`
  - `type LoginRequest struct`
  - `func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService`
  - `func (s *AuthService) Login(ctx context.Context, request LoginRequest) (auth.TokenPair, error)`
  - `func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (auth.TokenPair, error)`
  - `func (s *AuthService) Person(ctx context.Context, userId int64) (map[string]interface{}, error)`
  - `func FilterUserPassword(user map[string]interface{}) map[string]interface{}`

- [ ] **Step 1: Write base auth unit tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/auth_test.go`:

```go
package base_test

import (
   "testing"

   baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
)

func TestFilterUserPassword(t *testing.T) {
   user := map[string]interface{}{
      "id":       int64(1),
      "username": "admin",
      "password": "secret",
   }
   filtered := baseModule.FilterUserPassword(user)
   if _, ok := filtered["password"]; ok {
      t.Fatal("expected password removed")
   }
   if filtered["username"] != "admin" {
      t.Fatalf("expected username preserved, got %#v", filtered)
   }
   if _, ok := user["password"]; !ok {
      t.Fatal("expected original map unchanged")
   }
}

func TestLoginRequestValidate(t *testing.T) {
   if err := (baseModule.LoginRequest{}).Validate(); err == nil {
      t.Fatal("expected empty request validation error")
   }
   request := baseModule.LoginRequest{Username: "admin", Password: "123456"}
   if err := request.Validate(); err != nil {
      t.Fatalf("expected valid request, got %v", err)
   }
}
```

- [ ] **Step 2: Run failing base auth tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(FilterUserPassword|LoginRequest)' -count=1
```

Expected: FAIL because `FilterUserPassword` and `LoginRequest` do not exist.

- [ ] **Step 3: Implement base auth service**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/auth.go`:

```go
package base

import (
   "context"
   "fmt"

   "github.com/gogf/gf/v2/database/gdb"
   "github.com/gogf/gf/v2/errors/gerror"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
)

// AuthService 是 base 认证服务。
type AuthService struct {
   DB      gdb.DB
   Manager *auth.Manager
}

// LoginRequest 是登录请求。
type LoginRequest struct {
   Username   string `json:"username"`
   Password   string `json:"password"`
   CaptchaId  string `json:"captchaId"`
   VerifyCode string `json:"verifyCode"`
}

/**
 * 校验登录请求
 * @returns error
 */
func (r LoginRequest) Validate() error {
   if r.Username == "" || r.Password == "" {
      return gerror.New("用户名和密码不能为空")
   }
   return nil
}

/**
 * 创建认证服务
 * @param db 数据库实例
 * @param manager token 管理器
 * @returns *AuthService
 */
func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService {
   return &AuthService{
      DB:      db,
      Manager: manager,
   }
}

/**
 * 登录
 * @param ctx 上下文
 * @param request 登录请求
 * @returns auth.TokenPair
 */
func (s *AuthService) Login(ctx context.Context, request LoginRequest) (auth.TokenPair, error) {
   if err := request.Validate(); err != nil {
      return auth.TokenPair{}, err
   }
   user, err := s.userByUsername(ctx, request.Username)
   if err != nil {
      return auth.TokenPair{}, err
   }
   if len(user) == 0 {
      return auth.TokenPair{}, gerror.New("账户或密码不正确~")
   }
   if int64Value(user["status"]) != 1 {
      return auth.TokenPair{}, gerror.New("账户已被禁用~")
   }
   if !auth.VerifyMD5Password(request.Password, stringValue(user["password"])) {
      return auth.TokenPair{}, gerror.New("账户或密码不正确~")
   }

   roleIds, err := s.roleIdsByUserID(ctx, int64Value(user["id"]))
   if err != nil {
      return auth.TokenPair{}, err
   }
   return s.Manager.GenerateTokenPair(auth.Claims{
      RoleIds:         roleIds,
      Username:        stringValue(user["username"]),
      UserId:          int64Value(user["id"]),
      PasswordVersion: int64Value(user["passwordV"]),
      TenantId:        int64Value(user["tenantId"]),
   })
}

/**
 * 刷新 token
 * @param ctx 上下文
 * @param refreshToken refresh token
 * @returns auth.TokenPair
 */
func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (auth.TokenPair, error) {
   claims, err := s.Manager.ParseRefreshToken(refreshToken)
   if err != nil {
      return auth.TokenPair{}, gerror.Wrap(err, "解析 refresh token 失败")
   }
   user, err := s.userByID(ctx, claims.UserId)
   if err != nil {
      return auth.TokenPair{}, err
   }
   if len(user) == 0 || int64Value(user["status"]) != 1 {
      return auth.TokenPair{}, gerror.New("登录失效~")
   }
   roleIds, err := s.roleIdsByUserID(ctx, claims.UserId)
   if err != nil {
      return auth.TokenPair{}, err
   }
   return s.Manager.GenerateTokenPair(auth.Claims{
      RoleIds:         roleIds,
      Username:        stringValue(user["username"]),
      UserId:          int64Value(user["id"]),
      PasswordVersion: int64Value(user["passwordV"]),
      TenantId:        int64Value(user["tenantId"]),
   })
}

/**
 * 当前用户信息
 * @param ctx 上下文
 * @param userId 用户 ID
 * @returns map[string]interface{}
 */
func (s *AuthService) Person(ctx context.Context, userId int64) (map[string]interface{}, error) {
   user, err := s.userByID(ctx, userId)
   if err != nil {
      return nil, err
   }
   if len(user) == 0 {
      return nil, gerror.New("登录失效~")
   }
   return FilterUserPassword(user), nil
}

/**
 * 过滤用户密码
 * @param user 用户 map
 * @returns map[string]interface{}
 */
func FilterUserPassword(user map[string]interface{}) map[string]interface{} {
   filtered := map[string]interface{}{}
   for key, value := range user {
      if key != "password" {
         filtered[key] = value
      }
   }
   return filtered
}

func (s *AuthService) userByUsername(ctx context.Context, username string) (map[string]interface{}, error) {
   record, err := s.DB.GetOne(ctx, "SELECT id, department_id AS departmentId, user_id AS userId, name, username, password, password_v AS passwordV, nick_name AS nickName, head_img AS headImg, phone, email, remark, status, socket_id AS socketId, create_time AS createTime, update_time AS updateTime, tenant_id AS tenantId FROM base_sys_user WHERE username = ? LIMIT 1", username)
   if err != nil {
      return nil, gerror.Wrap(err, "查询登录用户失败")
   }
   return record.Map(), nil
}

func (s *AuthService) userByID(ctx context.Context, userId int64) (map[string]interface{}, error) {
   record, err := s.DB.GetOne(ctx, "SELECT id, department_id AS departmentId, user_id AS userId, name, username, password, password_v AS passwordV, nick_name AS nickName, head_img AS headImg, phone, email, remark, status, socket_id AS socketId, create_time AS createTime, update_time AS updateTime, tenant_id AS tenantId FROM base_sys_user WHERE id = ? LIMIT 1", userId)
   if err != nil {
      return nil, gerror.Wrap(err, "查询当前用户失败")
   }
   return record.Map(), nil
}

func (s *AuthService) roleIdsByUserID(ctx context.Context, userId int64) ([]int64, error) {
   result, err := s.DB.GetAll(ctx, "SELECT role_id AS roleId FROM base_sys_user_role WHERE user_id = ?", userId)
   if err != nil {
      return nil, gerror.Wrap(err, "查询用户角色失败")
   }
   roleIds := make([]int64, 0, len(result))
   for _, record := range result {
      roleIds = append(roleIds, int64Value(record.Map()["roleId"]))
   }
   return roleIds, nil
}

func stringValue(value interface{}) string {
   if value == nil {
      return ""
   }
   return fmt.Sprintf("%v", value)
}

func int64Value(value interface{}) int64 {
   switch item := value.(type) {
   case int64:
      return item
   case int:
      return int64(item)
   case uint64:
      return int64(item)
   case []byte:
      var result int64
      _, _ = fmt.Sscan(string(item), &result)
      return result
   default:
      var result int64
      _, _ = fmt.Sscan(fmt.Sprintf("%v", item), &result)
      return result
   }
}
```

- [ ] **Step 4: Run base auth tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run 'Test(FilterUserPassword|LoginRequest)' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run related package tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/auth.go modules/base/auth_test.go
git commit -m "feat: add base auth service" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 注册 base auth routes

**Files:**
- Create: `modules/base/auth_routes.go`
- Modify: `modules/base/auth_test.go`
- Test: `go test ./modules/base -run 'Test(AuthIgnore|RegisterAuth|Captcha)' -count=1`

**Interfaces:**
- Consumes:
  - `type AuthService struct`
  - `type LoginRequest struct`
  - `auth.UserFromContext(ctx context.Context) (auth.UserContext, bool)`
  - `auth.Unauthorized(r *ghttp.Request)`
- Produces:
  - `func AuthIgnorePaths() []string`
  - `func RegisterAuthRoutes(server *ghttp.Server, service *AuthService)`

- [ ] **Step 1: Append auth route tests**

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/auth_test.go`:

```go
func TestAuthIgnorePaths(t *testing.T) {
   paths := baseModule.AuthIgnorePaths()
   for _, path := range []string{
      "/health",
      "/admin/base/open/login",
      "/admin/base/open/refreshToken",
      "/admin/base/open/captcha",
      "/admin/base/comm/program",
   } {
      if !containsString(paths, path) {
         t.Fatalf("expected ignore path %s in %#v", path, paths)
      }
   }
}

func containsString(items []string, target string) bool {
   for _, item := range items {
      if item == target {
         return true
      }
   }
   return false
}
```

- [ ] **Step 2: Run failing route tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run TestAuthIgnorePaths -count=1
```

Expected: FAIL because `AuthIgnorePaths` does not exist.

- [ ] **Step 3: Implement auth routes**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/auth_routes.go`:

```go
package base

import (
   "net/http"

   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

/**
 * auth middleware 放行路径
 * @returns []string
 */
func AuthIgnorePaths() []string {
   return []string{
      "/health",
      "/admin/base/open/login",
      "/admin/base/open/refreshToken",
      "/admin/base/open/captcha",
      "/admin/base/comm/program",
   }
}

/**
 * 注册 base auth 路由
 * @param server HTTP 服务
 * @param service auth 服务
 * @returns null
 */
func RegisterAuthRoutes(server *ghttp.Server, service *AuthService) {
   server.BindHandler("POST:/admin/base/open/login", func(r *ghttp.Request) {
      request := LoginRequest{}
      if err := r.Parse(&request); err != nil {
         r.Response.WriteJson(response.Fail("参数错误", coolErrors.CodeValidateFail))
         return
      }
      pair, err := service.Login(r.Context(), request)
      if err != nil {
         r.Response.WriteJson(response.Fail(err.Error()))
         return
      }
      r.Response.WriteJson(response.OK(pair))
   })

   server.BindHandler("POST:/admin/base/open/refreshToken", func(r *ghttp.Request) {
      token := r.Get("refreshToken").String()
      pair, err := service.RefreshToken(r.Context(), token)
      if err != nil {
         r.Response.WriteStatus(http.StatusUnauthorized)
         r.Response.WriteJson(response.Body{Code: coolErrors.CodeCommFail, Message: "登录失效~"})
         return
      }
      r.Response.WriteJson(response.OK(pair))
   })

   server.BindHandler("GET:/admin/base/open/captcha", func(r *ghttp.Request) {
      r.Response.WriteJson(response.OK(map[string]string{
         "captchaId": "disabled",
         "data":      "",
      }))
   })

   server.BindHandler("GET:/admin/base/comm/person", func(r *ghttp.Request) {
      user, ok := auth.UserFromContext(r.Context())
      if !ok {
         auth.Unauthorized(r)
         return
      }
      data, err := service.Person(r.Context(), user.UserId)
      if err != nil {
         auth.Unauthorized(r)
         return
      }
      r.Response.WriteJson(response.OK(data))
   })

   server.BindHandler("POST:/admin/base/comm/logout", func(r *ghttp.Request) {
      r.Response.WriteJson(response.OK(map[string]interface{}{}))
   })

   server.BindHandler("GET:/admin/base/comm/program", func(r *ghttp.Request) {
      r.Response.WriteJson(response.OK("Go"))
   })
}
```

- [ ] **Step 4: Run route tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run TestAuthIgnorePaths -count=1
```

Expected: PASS.

- [ ] **Step 5: Run base package tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 4**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/auth_routes.go modules/base/auth_test.go
git commit -m "feat: add base auth routes" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 接入 app auth middleware

**Files:**
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Test: `go test ./cool/app ./modules/base ./cool/auth -count=1`

**Interfaces:**
- Consumes:
  - `auth.NewManager(secret string, expire int64, refreshExpire int64) *auth.Manager`
  - `auth.NewMiddleware(options auth.MiddlewareOptions) ghttp.HandlerFunc`
  - `base.NewAuthService(db gdb.DB, manager *auth.Manager) *base.AuthService`
  - `base.RegisterAuthRoutes(server *ghttp.Server, service *base.AuthService)`
  - `base.AuthIgnorePaths() []string`
- Produces:
  - `type AuthManagerFactory func(ctx context.Context) *auth.Manager`
  - `Options.AuthManagerFactory AuthManagerFactory`
  - app route registration with auth middleware protecting CRUD routes.

- [ ] **Step 1: Append app auth option tests**

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app_test.go`:

```go
func TestAuthManagerFactoryUsesConfig(t *testing.T) {
   adapter, err := gcfg.NewAdapterContent(`cool:
  auth:
    jwtSecret: "test-secret"
    tokenExpire: 11
    refreshExpire: 22`)
   if err != nil {
      t.Fatalf("create config adapter failed: %v", err)
   }
   config := g.Cfg()
   previousAdapter := config.GetAdapter()
   config.SetAdapter(adapter)
   t.Cleanup(func() {
      config.SetAdapter(previousAdapter)
   })

   manager := app.DefaultAuthManagerFactory(context.Background())
   if string(manager.Secret) != "test-secret" {
      t.Fatalf("unexpected secret: %s", string(manager.Secret))
   }
   if manager.Expire != 11 || manager.RefreshExpire != 22 {
      t.Fatalf("unexpected expires: %#v", manager)
   }
}
```

Also add import:

```go
"github.com/toothdy/cool-admin-go-next/cool/auth"
```

Only keep the import if later tests use `auth` directly; otherwise let `go test` compile feedback guide cleanup.

- [ ] **Step 2: Run failing app auth test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app -run TestAuthManagerFactoryUsesConfig -count=1
```

Expected: FAIL because `DefaultAuthManagerFactory` does not exist.

- [ ] **Step 3: Modify app runtime for auth**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app.go`:

1. Add imports:

```go
"github.com/toothdy/cool-admin-go-next/cool/auth"
baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
```

2. Add type:

```go
// AuthManagerFactory 是 auth manager 工厂。
type AuthManagerFactory func(ctx context.Context) *auth.Manager
```

3. Extend `Options`:

```go
AuthManagerFactory AuthManagerFactory
```

4. Extend `Application`:

```go
authManagerFactory AuthManagerFactory
authManager        *auth.Manager
```

5. In `NewWithContext`, default factory:

```go
authFactory := options.AuthManagerFactory
if authFactory == nil {
   authFactory = DefaultAuthManagerFactory
}
```

6. Set fields in `Application`:

```go
authManagerFactory: authFactory,
authManager:        authFactory(ctx),
```

7. Add exported factory:

```go
/**
 * 默认 auth manager 工厂
 * @param ctx 上下文
 * @returns *auth.Manager
 */
func DefaultAuthManagerFactory(ctx context.Context) *auth.Manager {
   return auth.NewManager(
      g.Cfg().MustGet(ctx, "cool.auth.jwtSecret", "cool-admin-go-next-dev-secret").String(),
      g.Cfg().MustGet(ctx, "cool.auth.tokenExpire", 7200).Int64(),
      g.Cfg().MustGet(ctx, "cool.auth.refreshExpire", 604800).Int64(),
   )
}
```

8. Update `registerRoutes()` order after `/health` and before CRUD routes:

```go
authService := baseModule.NewAuthService(g.DB(), a.authManager)
baseModule.RegisterAuthRoutes(a.server, authService)
a.server.Use(auth.NewMiddleware(auth.MiddlewareOptions{
   Manager:     a.authManager,
   IgnorePaths: baseModule.AuthIgnorePaths(),
}))
```

Keep existing CRUD registry code after middleware registration.

- [ ] **Step 4: Run app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app -count=1
```

Expected: PASS.

- [ ] **Step 5: Run related tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go
git commit -m "feat: wire auth into app" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 增加真实 MySQL auth 集成测试

**Files:**
- Create: `modules/base/auth_integration_test.go`
- Test: `go test ./modules/base -run AuthIntegration -count=1`
- Integration Test: `COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1`

**Interfaces:**
- Consumes:
  - `schema.NewSyncer(g.DB()).Sync(ctx, baseModel.Register())`
  - `seed.NewImporter(g.DB(), baseModel.Register())`
  - `NewAuthService(g.DB(), manager)`
  - `AuthService.Login/RefreshToken/Person`
- Produces: skipped-by-default integration coverage for admin login chain.

- [ ] **Step 1: Write integration test**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/auth_integration_test.go`:

```go
package base_test

import (
   "context"
   "os"
   "testing"

   _ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/toothdy/cool-admin-go-next/cool/auth"
   "github.com/toothdy/cool-admin-go-next/cool/db/schema"
   "github.com/toothdy/cool-admin-go-next/cool/seed"
   baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
   baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
)

func TestAuthIntegrationAdminLoginRefreshAndPerson(t *testing.T) {
   if os.Getenv("COOL_AUTH_INTEGRATION") != "1" {
      t.Skip("set COOL_AUTH_INTEGRATION=1 to run real MySQL auth integration test")
   }

   ctx := context.Background()
   definitions := baseModel.Register()
   if _, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions); err != nil {
      t.Fatalf("schema sync failed: %v", err)
   }
   cleanupAuthSeedData(t, ctx)

   importer := seed.NewImporter(g.DB(), definitions)
   if _, err := importer.ImportDB(ctx, "base", "modules/base/db.json"); err != nil {
      t.Fatalf("import db seed failed: %v", err)
   }
   if _, err := importer.ImportMenu(ctx, "base", "modules/base/menu.json"); err != nil {
      t.Fatalf("import menu seed failed: %v", err)
   }

   manager := auth.NewManager("integration-secret", 7200, 604800)
   service := baseModule.NewAuthService(g.DB(), manager)

   pair, err := service.Login(ctx, baseModule.LoginRequest{Username: "admin", Password: "123456"})
   if err != nil {
      t.Fatalf("admin login failed: %v", err)
   }
   if pair.Token == "" || pair.RefreshToken == "" {
      t.Fatalf("expected token pair, got %#v", pair)
   }

   if _, err = service.Login(ctx, baseModule.LoginRequest{Username: "admin", Password: "wrong"}); err == nil {
      t.Fatal("expected wrong password login to fail")
   }

   refreshed, err := service.RefreshToken(ctx, pair.RefreshToken)
   if err != nil {
      t.Fatalf("refresh token failed: %v", err)
   }
   if refreshed.Token == "" || refreshed.RefreshToken == "" {
      t.Fatalf("expected refreshed token pair, got %#v", refreshed)
   }

   claims, err := manager.ParseAccessToken(refreshed.Token)
   if err != nil {
      t.Fatalf("parse refreshed token failed: %v", err)
   }
   person, err := service.Person(ctx, claims.UserId)
   if err != nil {
      t.Fatalf("person failed: %v", err)
   }
   if person["username"] != "admin" {
      t.Fatalf("expected admin person, got %#v", person)
   }
   if _, ok := person["password"]; ok {
      t.Fatal("expected person response without password")
   }
}

func cleanupAuthSeedData(t *testing.T, ctx context.Context) {
   t.Helper()
   statements := []string{
      "DELETE FROM base_sys_role_menu",
      "DELETE FROM base_sys_role_department",
      "DELETE FROM base_sys_user_role",
      "DELETE FROM base_sys_menu",
      "DELETE FROM base_sys_user",
      "DELETE FROM base_sys_role",
      "DELETE FROM base_sys_department",
      "DELETE FROM base_sys_param",
      "DELETE FROM base_sys_conf WHERE c_key IN ('init_db_base', 'init_menu_base', 'logKeep', 'recycleKeep')",
   }
   for _, statement := range statements {
      if _, err := g.DB().Exec(ctx, statement); err != nil {
         t.Fatalf("cleanup failed for %s: %v", statement, err)
      }
   }
}
```

- [ ] **Step 2: Run skipped integration test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run AuthIntegration -count=1
```

Expected: PASS with skip message unless `COOL_AUTH_INTEGRATION=1` is set.

- [ ] **Step 3: Run real MySQL integration test**

Ensure database exists:

```sql
CREATE DATABASE `cool-go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
```

Expected: PASS.

- [ ] **Step 4: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: PASS; integration tests skipped unless env flags are set.

- [ ] **Step 5: Commit Task 6**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/auth_integration_test.go
git commit -m "test: add auth integration coverage" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: README、最终验证和上下文交接记录

**Files:**
- Modify: `README.md`
- Modify: `todo.md` only if maintaining local task notes for this phase
- Test: `go test ./...`
- Integration Test: `COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1`

**Interfaces:**
- Consumes: all previous tasks.
- Produces: documented and verified Plan5A state.

- [ ] **Step 1: Update README current stage**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/README.md`:

1. 当前阶段改为：`阶段 5A：Auth 最小登录链路`。
2. 已完成列表加入：
   - `cool/auth` 密码、token、上下文和 middleware。
   - base login / refreshToken / captcha 占位接口。
   - base person / logout / program 接口。
   - auth middleware 保护非放行路由。
3. 未完成列表保留：
   - 权限菜单 / 权限码。
   - EPS runtime。
   - Vue 前端联调。
4. 新增 auth 验收命令：

```bash
go test ./cool/auth ./modules/base ./cool/app -count=1
go test ./...
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
```

5. 新增手工验证示例：

```bash
curl -X POST http://127.0.0.1:8001/admin/base/open/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"123456"}'

curl http://127.0.0.1:8001/admin/base/comm/person \
  -H 'Authorization: <token>'
```

- [ ] **Step 2: Run unit tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/auth ./cool/app ./modules/base -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: PASS.

- [ ] **Step 4: Run real MySQL auth integration**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
```

Expected: PASS.

- [ ] **Step 5: Verify forbidden directories absent**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
test ! -d logic
test ! -d dao
test ! -d internal/model/do
test ! -d internal/model/entity
```

Expected: all commands exit `0`.

- [ ] **Step 6: Check git status**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
```

Expected: only README or local task notes changed before final commit.

- [ ] **Step 7: Commit Task 7**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md
git commit -m "docs: document auth login phase" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

If `todo.md` was updated and should be committed, stage it explicitly:

```bash
git add todo.md
```

Do not use `git add -A`.

---

## Final Plan5A Acceptance Checklist

- [ ] `go test ./cool/auth ./cool/app ./modules/base -count=1` passes.
- [ ] `go test ./...` passes.
- [ ] `COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1` passes against real MySQL.
- [ ] `POST /admin/base/open/login` supports admin / 123456.
- [ ] Login response includes `token`, `expire`, `refreshToken`, `refreshExpire`.
- [ ] `POST /admin/base/open/refreshToken` returns a new token pair.
- [ ] `GET /admin/base/comm/person` with `Authorization: <token>` returns admin user.
- [ ] person response does not include `password`.
- [ ] Missing or invalid token returns HTTP 401 with `登录失效~`.
- [ ] `/admin/base/comm/program` remains accessible without token.
- [ ] Existing CRUD routes are protected by auth middleware.
- [ ] No `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/` directories were created.
- [ ] Main agent context contains only task summaries and review results; implementation details live in this plan and subagent reports.

---

## Self-Review

### Spec coverage

This plan covers the approved Stage 5A design:

1. md5 password compatibility: Task 1.
2. access/refresh token pair: Task 1.
3. token context and middleware: Task 2.
4. base login/refresh/person service: Task 3.
5. open/comm auth routes: Task 4.
6. app route and middleware integration: Task 5.
7. real MySQL admin login chain: Task 6.
8. README and final verification: Task 7.
9. context-control requirement: Global Constraints and final checklist.

Stage 5B items are intentionally excluded: permmenu, permission codes, menu tree, CRUD permission checks.

### Placeholder scan

The plan contains no unresolved placeholder markers. Every task has concrete files, interfaces, commands, and expected outcomes.

### Type consistency

The produced interfaces are consistent across tasks:

1. `auth.Manager` is created in Task 1 and consumed by Tasks 2, 3, 5, 6.
2. `auth.UserContext` is created in Task 2 and consumed by Task 4.
3. `base.AuthService` is created in Task 3 and consumed by Tasks 4, 5, 6.
4. `base.AuthIgnorePaths` and `base.RegisterAuthRoutes` are created in Task 4 and consumed by Task 5.
5. `app.DefaultAuthManagerFactory` is created in Task 5 and documented in Task 7.
