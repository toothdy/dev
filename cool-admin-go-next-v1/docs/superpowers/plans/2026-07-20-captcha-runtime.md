# 图片验证码闭环实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以原版 cool-admin-go 的 SVG/Data URL 协议实现图片验证码生成、30 分钟缓存、一次性登录校验，使 Vue 登录页可显示并消费验证码。

**Architecture:** `AuthService` 持有可注入的 GoFrame `gcache.Cache`，生成密码学随机的 4 位验证码和不透明 ID，生成 SVG 后编码为 Data URL；验证码按 `captcha:<id>` 保存 30 分钟。登录在查询数据库前于服务内互斥地读取、比较并删除正确验证码。Open Controller 仅负责解析 query 参数并写入现有标准响应。

**Tech Stack:** Go 1.x、GoFrame v2 (`gcache`/`ghttp`)、标准库 `crypto/rand`、`encoding/base64`、`encoding/hex`、`strings`、`sync`、`time`。

## Global Constraints

- 所有项目面向的说明和代码注释使用中文。
- 必须保持标准成功包络 `{ code: 1000, message: "success", data: ... }`。
- `GET /admin/base/open/captcha` 与 `POST /admin/base/open/login` 必须保持匿名访问。
- 验证码内层响应必须是 `{ captchaId, data }`，其中 `data` 为非空 `data:image/svg+xml;base64,...` 字符串。
- 验证码为 4 位数字、有效期 30 分钟，忽略大小写比较；正确匹配后立即删除，禁止重放；错误匹配不删除。
- 不新增 Redis、数据库表、第三方依赖、前端改动、文件上传、限流或验证码开关。
- 仅使用当前项目已安装的 GoFrame v2 `gcache.Cache`；缓存是进程内实现，进程重启验证码失效是本阶段预期限制。
- `AuthService.Login` 必须在任何数据库访问前拒绝错误、过期或缺失验证码。
- 随机验证码和 ID 使用 `crypto/rand`，不得使用 `math/rand`。
- color query 参数不得未经验证直接写入 SVG XML 属性；只接受 `#RRGGBB`，其他值回退 `#2c3142`。
- 任意 query 尺寸值都不得导致 SVG 生成 panic：`height <= 1` 或 `width < 30` 时回退默认值；其他正值按请求尺寸生成。
- 保持既有 EPS、refresh token、person、permmenu 与 CRUD 行为不变。
- 新增 Go 文件必须执行 `gofmt`；不得使用 `git add -A`。
- 每个引入或变更生产行为的任务必须先观察到失败测试，再编写最小实现并运行任务聚焦测试；仅文档和最终回归验证复用此前的 TDD 测试。

---

## File Structure

### Modified files

- `modules/base/service/auth.go` — 缓存依赖、验证码 DTO、SVG/Data URL 生成、验证码校验、登录流程集成。
- `modules/base/service/auth_test.go` — 验证码单元测试与登录数据库访问前拒绝测试。
- `modules/base/auth.go` — 保留 root compatibility constructor，并补充可注入 cache 的 wrapper。
- `modules/base/auth_test.go` — 更新 root compatibility constructor 和登录请求校验测试。
- `modules/base/controller/open/open.go` — 将 `/captcha` 的 query 参数传给认证服务并返回服务数据。
- `modules/base/controller/controllers_test.go` — 使用 JSON 结构断言真实验证码响应而非旧的 disabled 占位内容。
- `docs/protocol/base-api-contract.md` — 明确实现后的验证码 TTL、Data URL、大小写和一次性消费规则。
- `README.md` — 更新当前阶段和图片验证码手工验收步骤。

---

### Task 1: 在认证服务实现验证码生成与一次性校验

**Files:**
- Modify: `modules/base/service/auth.go`
- Modify: `modules/base/service/auth_test.go`
- Modify: `modules/base/auth.go`
- Modify: `modules/base/auth_test.go`

**Interfaces:**
- Consumes: GoFrame `gcache.New()`、`Cache.Set(ctx, key, value, duration)`、`Cache.Get(ctx, key)`、`Cache.Remove(ctx, key)`；现有 `LoginRequest` 与 `auth.TokenPair`。
- Produces:

```go
type Captcha struct {
	CaptchaID string `json:"captchaId"`
	Data      string `json:"data"`
}

func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService
func NewAuthServiceWithCache(db gdb.DB, manager *auth.Manager, cache *gcache.Cache) *AuthService
func (s *AuthService) Captcha(ctx context.Context, height int, width int, color string) (Captcha, error)
func (s *AuthService) Login(ctx context.Context, request LoginRequest) (auth.TokenPair, error)
```

- [ ] **Step 1: 写服务层失败测试**

在 `modules/base/service/auth_test.go` 添加 imports：`encoding/base64`、`strings`、`time`、`github.com/gogf/gf/v2/os/gcache`。新增 helper：

```go
func newCaptchaService() (*AuthService, *gcache.Cache) {
	cache := gcache.New()
	return NewAuthServiceWithCache(nil, nil, cache), cache
}
```

添加验证码生成、参数回退与 Data URL 测试：

```go
func TestCaptchaBuildsRenderableDataURLAndCachesCode(t *testing.T) {
	service, cache := newCaptchaService()
	captcha, err := service.Captcha(context.Background(), 45, 150, "#2c3142")
	if err != nil {
		t.Fatalf("create captcha failed: %v", err)
	}
	if captcha.CaptchaID == "" || !strings.HasPrefix(captcha.Data, "data:image/svg+xml;base64,") {
		t.Fatalf("unexpected captcha: %#v", captcha)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(captcha.Data, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatalf("decode SVG failed: %v", err)
	}
	if !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="45"`) || !strings.Contains(string(svg), `fill="#2c3142"`) {
		t.Fatalf("unexpected SVG: %s", svg)
	}
	cached, err := cache.Get(context.Background(), captchaCacheKey(captcha.CaptchaID))
	if err != nil || cached == nil || len(cached.String()) != 4 {
		t.Fatalf("expected cached four-digit code, got %#v, %v", cached, err)
	}
}

func TestCaptchaFallsBackForInvalidDimensionsAndColor(t *testing.T) {
	service, _ := newCaptchaService()
	captcha, err := service.Captcha(context.Background(), 0, -1, `"/><script>`)
	if err != nil {
		t.Fatalf("create fallback captcha failed: %v", err)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(captcha.Data, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatalf("decode SVG failed: %v", err)
	}
	if !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="50"`) || !strings.Contains(string(svg), `fill="#2c3142"`) || strings.Contains(string(svg), "<script>") {
		t.Fatalf("unexpected fallback SVG: %s", svg)
	}
}
```

添加验证码验证、一次性消费与错误保留测试：

```go
func TestVerifyCaptchaIgnoresCaseAndConsumesMatchedCode(t *testing.T) {
	service, cache := newCaptchaService()
	if err := cache.Set(context.Background(), captchaCacheKey("captcha-id"), "aB12", time.Minute); err != nil {
		t.Fatalf("seed captcha failed: %v", err)
	}
	if err := service.verifyCaptcha(context.Background(), "captcha-id", "Ab12"); err != nil {
		t.Fatalf("expected matching captcha, got %v", err)
	}
	value, err := cache.Get(context.Background(), captchaCacheKey("captcha-id"))
	if err != nil || value != nil {
		t.Fatalf("expected consumed captcha, got %#v, %v", value, err)
	}
	if err = service.verifyCaptcha(context.Background(), "captcha-id", "aB12"); err == nil || err.Error() != "验证码错误" {
		t.Fatalf("expected replay rejected, got %v", err)
	}
}

func TestVerifyCaptchaRejectsInvalidCodeWithoutDeletingIt(t *testing.T) {
	service, cache := newCaptchaService()
	if err := cache.Set(context.Background(), captchaCacheKey("captcha-id"), "1234", time.Minute); err != nil {
		t.Fatalf("seed captcha failed: %v", err)
	}
	if err := service.verifyCaptcha(context.Background(), "captcha-id", "4321"); err == nil || err.Error() != "验证码错误" {
		t.Fatalf("expected invalid captcha error, got %v", err)
	}
	value, err := cache.Get(context.Background(), captchaCacheKey("captcha-id"))
	if err != nil || value == nil || value.String() != "1234" {
		t.Fatalf("expected original captcha retained, got %#v, %v", value, err)
	}
}

func TestLoginRejectsInvalidCaptchaBeforeDatabaseAccess(t *testing.T) {
	service, _ := newCaptchaService()
	_, err := service.Login(context.Background(), LoginRequest{
		Username: "admin", Password: "123456", CaptchaId: "missing", VerifyCode: "1234",
	})
	if err == nil || err.Error() != "验证码错误" {
		t.Fatalf("expected captcha error before database access, got %v", err)
	}
}
```

在 `modules/base/auth_test.go` 调整 `TestLoginRequestValidate` 的有效请求为：

```go
request := LoginRequest{
	Username: "admin", Password: "123456", CaptchaId: "captcha-id", VerifyCode: "1234",
}
```

并为 root wrapper 添加可注入 cache 的断言：

```go
cache := gcache.New()
service := NewAuthServiceWithCache(nil, manager, cache)
if service.Cache != cache {
	t.Fatal("expected root constructor to keep injected cache")
}
```

- [ ] **Step 2: 运行服务测试，确认 Red**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/service ./modules/base -run 'Test(Captcha|VerifyCaptcha|LoginRejectsInvalidCaptchaBeforeDatabaseAccess|LoginRequestValidate|AuthServiceCompatibilityAliasAndConstructor)' -count=1
```

预期：失败，提示 `Captcha`、`NewAuthServiceWithCache`、`captchaCacheKey` 或 `verifyCaptcha` 未定义；旧的登录验证测试也会与新 captcha 字段契约不符。

- [ ] **Step 3: 实现 AuthService 缓存依赖与验证码 DTO**

在 `modules/base/service/auth.go` 添加 imports：

```go
"crypto/rand"
"encoding/base64"
"encoding/hex"
"fmt"
"math/big"
"strconv"
"strings"
"sync"
"time"

"github.com/gogf/gf/v2/os/gcache"
```

保留已有 `fmt`，不要重复导入。将 `AuthService` 改为：

```go
type AuthService struct {
	DB       gdb.DB
	Manager  *auth.Manager
	Cache    *gcache.Cache
	captchaMu sync.Mutex
}

type Captcha struct {
	CaptchaID string `json:"captchaId"`
	Data      string `json:"data"`
}
```

将原构造函数改为委托，并增加注入构造函数：

```go
func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService {
	return NewAuthServiceWithCache(db, manager, gcache.New())
}

func NewAuthServiceWithCache(db gdb.DB, manager *auth.Manager, cache *gcache.Cache) *AuthService {
	if cache == nil {
		cache = gcache.New()
	}
	return &AuthService{DB: db, Manager: manager, Cache: cache}
}
```

在 `modules/base/auth.go` 增加对应 wrapper：

```go
func NewAuthServiceWithCache(db gdb.DB, manager *auth.Manager, cache *gcache.Cache) *AuthService {
	return baseService.NewAuthServiceWithCache(db, manager, cache)
}
```

- [ ] **Step 4: 实现随机生成、SVG 和缓存写入**

在 `auth.go` 增加常量：

```go
const (
	captchaCodeLength = 4
	captchaTTL        = 30 * time.Minute
	captchaKeyPrefix  = "captcha:"
	defaultCaptchaWidth = 150
	defaultCaptchaHeight = 50
	defaultCaptchaColor = "#2c3142"
)
```

实现下列私有 helper：

```go
func captchaCacheKey(captchaID string) string
func normalizeCaptchaOptions(height int, width int, color string) (int, int, string)
func isHexColor(color string) bool
func randomCaptchaCode(length int) (string, error)
func randomCaptchaID() (string, error)
func randomInt(max int) (int, error)
func buildCaptchaSVG(text string, width int, height int, color string) (string, error)
func randomGreyColor() (string, error)
```

精确行为：

- `captchaCacheKey` 返回 `captcha:` 加 ID。
- `randomInt` 必须在 `max <= 0` 时返回 error；否则调用 `rand.Int(rand.Reader, big.NewInt(int64(max)))`。
- `randomCaptchaCode` 每位从 `"0123456789"` 选取，长度为 `captchaCodeLength`。
- `randomCaptchaID` 读取 16 个随机字节并使用 `hex.EncodeToString`；不得暴露验证码文本。
- `normalizeCaptchaOptions` 对 `height <= 1` 或 `width < 30` 回退默认值；color 只接受 `#` 后六位十六进制，其他值回退默认颜色。
- `buildCaptchaSVG` 使用 `strings.Builder` 构造 `<svg xmlns="http://www.w3.org/2000/svg" width="..." height="..." viewBox="0 0 ...">`；加入 3 条灰色 `path` 干扰线；每个数字输出带 `rotate` 的 `<text>`，字体大小为 `height*7/10`；每个数字添加 2 个灰色噪点 path。所有随机值经 `randomInt` 获得并向上传递 error。

实现 `Captcha`：

```go
func (s *AuthService) Captcha(ctx context.Context, height int, width int, color string) (Captcha, error) {
	width, height, color = normalizeCaptchaOptions(height, width, color)
	code, err := randomCaptchaCode(captchaCodeLength)
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码失败")
	}
	captchaID, err := randomCaptchaID()
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码标识失败")
	}
	svg, err := buildCaptchaSVG(code, width, height, color)
	if err != nil {
		return Captcha{}, gerror.Wrap(err, "生成验证码图片失败")
	}
	if err = s.Cache.Set(ctx, captchaCacheKey(captchaID), strings.ToLower(code), captchaTTL); err != nil {
		return Captcha{}, gerror.Wrap(err, "保存验证码失败")
	}
	return Captcha{
		CaptchaID: captchaID,
		Data:      "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(svg)),
	}, nil
}
```

注意 `normalizeCaptchaOptions` 参数和返回值在 plan 中是 `(height, width, color)`，调用 `buildCaptchaSVG` 时传入 `(width, height, color)`。

- [ ] **Step 5: 实现登录校验与错误映射**

在 `Login` 的用户名密码校验后、`s.userByUsername` 前插入：

```go
if err := s.verifyCaptcha(ctx, request.CaptchaId, request.VerifyCode); err != nil {
	return auth.TokenPair{}, err
}
```

实现：

```go
func (s *AuthService) verifyCaptcha(ctx context.Context, captchaID string, verifyCode string) error {
	if captchaID == "" || verifyCode == "" {
		return gerror.New("验证码错误")
	}
	s.captchaMu.Lock()
	defer s.captchaMu.Unlock()

	value, err := s.Cache.Get(ctx, captchaCacheKey(captchaID))
	if err != nil {
		return gerror.Wrap(err, "读取验证码失败")
	}
	if value == nil || !strings.EqualFold(value.String(), verifyCode) {
		return gerror.New("验证码错误")
	}
	if _, err = s.Cache.Remove(ctx, captchaCacheKey(captchaID)); err != nil {
		return gerror.Wrap(err, "删除验证码失败")
	}
	return nil
}
```

`captchaMu` 覆盖同一服务实例内的 Get/compare/Remove，防止并发请求以同一验证码同时通过。错误验证码不调用 `Remove`。

在 `modules/base/controller/open/open.go` 的 `loginErrorMessage` switch 中增加：

```go
"验证码错误":
	return err.Error()
```

- [ ] **Step 6: 运行服务测试，确认 Green**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/service/auth.go modules/base/service/auth_test.go modules/base/auth.go modules/base/auth_test.go modules/base/controller/open/open.go
go test ./modules/base/service ./modules/base -run 'Test(Captcha|VerifyCaptcha|LoginRejectsInvalidCaptchaBeforeDatabaseAccess|LoginRequestValidate|AuthServiceCompatibilityAliasAndConstructor)' -count=1
```

预期：PASS；错误验证码在 `DB == nil` 时返回 `验证码错误` 而不会 panic。

- [ ] **Step 7: 提交认证服务验证码闭环**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/service/auth.go modules/base/service/auth_test.go modules/base/auth.go modules/base/auth_test.go modules/base/controller/open/open.go
git commit -m $'feat: add one-time image captcha authentication\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 2: 接入验证码 HTTP 参数并固定前端响应契约

**Files:**
- Modify: `modules/base/controller/open/open.go`
- Modify: `modules/base/controller/controllers_test.go`

**Interfaces:**
- Consumes: `func (s *AuthService) Captcha(ctx context.Context, height int, width int, color string) (Captcha, error)`。
- Produces: `GET /admin/base/open/captcha` 返回标准响应包络与可渲染的验证码 Data URL。

- [ ] **Step 1: 写 controller route 的失败测试**

在 `modules/base/controller/controllers_test.go` 添加 imports：

```go
"encoding/base64"
"encoding/json"
"strings"
```

保留既有 `TestBaseCustomRouteHandlersPreserveLegacyResponses` 的 program/logout/login cases，但删除其固定 captcha case。添加：

```go
func TestCaptchaRouteReturnsRenderableImage(t *testing.T) {
	server := ghttp.GetServer("base-captcha-handler-test")
	server.SetPort(0)
	if err := server.Start(); err != nil {
		t.Fatalf("start test server failed: %v", err)
	}
	defer server.Shutdown()
	if err := coolRuntimeController.RegisterRoutes(server, nil, Controllers(nil, nil, nil)); err != nil {
		t.Fatalf("register controller routes failed: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/admin/base/open/captcha?height=45&width=150&color=%232c3142", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := struct {
		Code int `json:"code"`
		Data struct {
			CaptchaID string `json:"captchaId"`
			Data      string `json:"data"`
		} `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode captcha response failed: %v", err)
	}
	if body.Code != coolErrors.CodeSuccess || body.Data.CaptchaID == "" || !strings.HasPrefix(body.Data.Data, "data:image/svg+xml;base64,") {
		t.Fatalf("unexpected captcha response: %#v", body)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(body.Data.Data, "data:image/svg+xml;base64,"))
	if err != nil || !strings.Contains(string(svg), `width="150"`) || !strings.Contains(string(svg), `height="45"`) {
		t.Fatalf("unexpected captcha SVG: %q, %v", svg, err)
	}
}
```

- [ ] **Step 2: 运行 controller 测试，确认 Red**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/controller -run TestCaptchaRouteReturnsRenderableImage -count=1
```

预期：失败：当前 `captchaHandler` 仍返回 `captchaId: "disabled"`、空 `data`，或尚未调用 `AuthService.Captcha`。

- [ ] **Step 3: 实现 query 参数到 AuthService 的转发**

将 `captchaHandler` 改为 service closure：

```go
func captchaHandler(service *baseService.AuthService) func(*ghttp.Request) {
	return func(r *ghttp.Request) {
		captcha, err := service.Captcha(
			r.Context(),
			r.Get("height").Int(),
			r.Get("width").Int(),
			r.Get("color").String(),
		)
		if err != nil {
			g.Log().Error(r.Context(), err)
			r.Response.WriteJson(response.Fail("验证码获取失败"))
			return
		}
		r.Response.WriteJson(response.OK(captcha))
	}
}
```

在 `OpenController` 的 captcha route 中将：

```go
Handler: captchaHandler,
```

改为：

```go
Handler: captchaHandler(authService),
```

不改变 route 的 `Name`、`Method`、`Path`、`IgnoreAuth`。

- [ ] **Step 4: 运行 controller 测试，确认 Green**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/controller/open/open.go modules/base/controller/controllers_test.go
go test ./modules/base/controller -run 'Test(CaptchaRouteReturnsRenderableImage|BaseCustomRouteHandlersPreserveLegacyResponses)' -count=1
```

预期：PASS；captcha route 返回 HTTP 200、code 1000、非空 ID 和可解码 SVG Data URL；其他 custom route 响应未回退。

- [ ] **Step 5: 提交验证码 HTTP 接入**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/controller/open/open.go modules/base/controller/controllers_test.go
git commit -m $'feat: expose renderable captcha endpoint\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 3: 记录验证码协议并完成回归验收

**Files:**
- Modify: `docs/protocol/base-api-contract.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: 已实现的 `/admin/base/open/captcha` 与登录验证码校验。
- Produces: 可复现的协议与手工验收说明。

- [ ] **Step 1: 更新协议契约和 README**

在 `docs/protocol/base-api-contract.md` 的 Open 接口表下新增“图片验证码实现规则”小节，精确写明：

```markdown
图片验证码实现规则：

1. `GET /admin/base/open/captcha` 使用 `height`、`width`、`color` query 参数；默认分别为 `50`、`150`、`#2c3142`，无效尺寸或颜色回退默认值。
2. 成功 data 是 `{ captchaId, data }`；`data` 是以 `data:image/svg+xml;base64,` 开头的非空 SVG Data URL。
3. 验证码为 4 位数字，服务端以 `captcha:<captchaId>` 在进程内缓存 30 分钟。
4. 登录必须传入 `captchaId` 与 `verifyCode`；比较不区分大小写。缺失、过期或错误时返回 `验证码错误`。
5. 正确匹配会在密码校验前立即消费验证码；错误匹配保留验证码以便用户重试。
```

更新 `README.md`：

1. 将当前阶段标题从“阶段 5B：权限菜单和 CRUD 权限校验”改为“阶段 6C：EPS 与图片验证码运行时”。
2. 将“未完成”列表中的 `EPS runtime` 删除，保留 `Vue 前端联调`。
3. 在 EPS Bootstrap 验收后添加：

```markdown
## 图片验证码验收

验证码为匿名接口。先启动应用：

```bash
go run .
```

请求图片验证码：

```bash
curl 'http://127.0.0.1:8001/admin/base/open/captcha?height=45&width=150&color=%232c3142'
```

期望 `code: 1000`，内层 `data.captchaId` 非空，`data.data` 以 `data:image/svg+xml;base64,` 开头。登录时必须提交该 `captchaId` 与图片中的 4 位 `verifyCode`；验证码 30 分钟后失效，正确使用一次即消费。
```

- [ ] **Step 2: 运行完整验证**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/service ./modules/base/controller ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
go vet ./...
git diff --check
```

预期：全部通过；现有 EPS、auth、permission 和 CRUD 行为无回退。

- [ ] **Step 3: 提交文档与验收结果**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add docs/protocol/base-api-contract.md README.md
git commit -m $'docs: document image captcha contract\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

## Plan Self-Review

- **Spec coverage:** Task 1 覆盖原版 SVG/Data URL 协议、4 位随机码、30 分钟 GoFrame cache、颜色与尺寸回退、忽略大小写、一次性消费、登录数据库访问前校验和安全错误映射；Task 2 覆盖 query 参数、匿名 HTTP route 和 Vue 所需 JSON 响应；Task 3 覆盖协议、README 和全量回归。
- **Scope check:** 计划不引入 Redis、数据库表、依赖、前端修改、限流、审计或其他 base 业务接口。进程内 cache 的单实例限制已在设计中明确。
- **Placeholder scan:** 没有 TBD/TODO、模糊的“适当处理”或未定义 helper；每处新增私有 helper、公开接口、固定错误文本与测试断言都已给出。尺寸归一化的下限同时保证 SVG 随机坐标的 `randomInt` 参数始终大于零。
- **Type consistency:** `Captcha` 的 `captchaId`/`data` JSON tags 与 Vue 解构字段一致；`NewAuthService` 保持已有签名；`NewAuthServiceWithCache` 提供明确注入；handler 用 `r.Get(...).Int/String` 传递可选 query；`gcache.Get` 允许 nil value，`Remove` 返回值被忽略但 error 必须处理；登录错误通过既有 `loginErrorMessage` 白名单暴露 `验证码错误`。
