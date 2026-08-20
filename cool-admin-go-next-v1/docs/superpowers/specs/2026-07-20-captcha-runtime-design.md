# cool-admin-go-next 图片验证码闭环设计

日期：2026-07-20

## 1. 背景

当前 `GET /admin/base/open/captcha` 是占位实现，固定返回：

```json
{
  "captchaId": "disabled",
  "data": ""
}
```

`cool-admin-vue` 的登录验证码组件只在内层 `data` 为非空字符串时渲染图像并通过 `v-model` 回写 `captchaId`。空字符串会触发“验证码获取失败”，因此前端无法显示验证码，也不会把 `captchaId` 带入登录请求。

同时，当前 `AuthService.Login` 只校验用户名和密码，没有验证 `captchaId` 与 `verifyCode`。验证码即使可见也不参与认证，和原版 `cool-admin-go`、Node 版的认证行为不一致。

## 2. 目标

本阶段完成可用的图片验证码登录闭环：

1. `GET /admin/base/open/captcha` 根据 query 参数生成随机 4 位数字验证码。
2. 响应内层 data 与 Vue 登录组件兼容：

   ```json
   {
     "captchaId": "随机标识",
     "data": "data:image/svg+xml;base64,..."
   }
   ```

3. 登录接口校验 `captchaId` 和 `verifyCode`，不区分大小写。
4. 验证码有效期为 30 分钟；验证成功后立即删除，禁止重放。
5. 保持现有登录、JWT、密码、权限和 EPS 行为不变。

## 3. 非目标

本阶段不做：

1. Redis 或跨进程验证码共享。
2. 短信、邮箱、滑块或行为验证码。
3. 管理后台验证码开关、失败次数限制、IP 限流或审计日志。
4. 前端 Vue 组件改造。
5. 上传、`personUpdate`、富文本 HTML 等其他 base 接口补齐。

## 4. 兼容契约

### 4.1 请求

前端经 EPS 发出：

```text
GET /admin/base/open/captcha?height=45&width=150&color=%232c3142
```

`height`、`width`、`color` 均为可选 query 参数。

默认值遵循原版：

| 参数 | 默认值 | 无效值处理 |
|---|---:|---|
| `width` | `150` | 小于等于 0 时回退默认值 |
| `height` | `50` | 小于等于 0 时回退默认值 |
| `color` | `#2c3142` | 空字符串时回退默认值 |

### 4.2 响应

HTTP 外层继续使用标准成功包络：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "captchaId": "随机标识",
    "data": "data:image/svg+xml;base64,PHN2Zy..."
  }
}
```

- `captchaId` 为不透明、唯一的随机标识。
- `data` 必须为非空 SVG Data URL，前缀固定为 `data:image/svg+xml;base64,`。
- SVG 中含 4 位数字、干扰线与噪点；文字颜色使用请求 `color`。

### 4.3 登录校验

登录请求继续使用已有字段：

```json
{
  "username": "admin",
  "password": "123456",
  "captchaId": "...",
  "verifyCode": "1234"
}
```

校验顺序固定为：

1. 用户名、密码非空；
2. `captchaId`、`verifyCode` 非空；
3. 读取缓存内验证码；缓存缺失、过期或忽略大小写比较不一致时返回 `验证码错误`；
4. 比较成功后立即删除缓存项；
5. 执行既有用户状态、MD5 密码、角色与 JWT 流程。

验证码错误不能访问数据库或生成 token。错误验证码不会删除缓存，使用户可在有效期内重新输入；匹配成功的验证码即使后续密码错误也已经消费，符合一次性验证码语义。

## 5. 架构

```text
Vue pic-captcha
  ↓ GET /admin/base/open/captcha
OpenController.captchaHandler
  ↓ query 参数
AuthService.Captcha
  ├─ 随机 4 位数字
  ├─ 构造 SVG + base64 Data URL
  └─ GoFrame gcache 保存 captchaId → 小写文本（30 分钟）
  ↓
response.OK({ captchaId, data })

Vue 登录表单
  ↓ POST /admin/base/open/login
AuthService.Login
  ├─ gcache 读取 captchaId
  ├─ strings.EqualFold 校验 verifyCode
  ├─ 成功即 Remove(captchaId)
  └─ 既有数据库密码、角色、JWT 流程
```

### 5.1 缓存边界

使用 GoFrame v2 内置 `gcache.Cache` 的进程内缓存实例，由 `AuthService` 显式持有并通过构造函数注入。缓存值只保存验证码文本，key 使用统一前缀：

```text
captcha:<captchaId>
```

缓存默认使用 `gcache.New()`；生产中应用进程重启会使验证码失效，这是本阶段无外部缓存的预期限制。测试注入独立 cache，避免测试之间共享状态。

### 5.2 服务接口

新增公开 DTO：

```go
type Captcha struct {
   CaptchaID string `json:"captchaId"`
   Data      string `json:"data"`
}
```

`AuthService` 增加：

```go
func NewAuthService(db gdb.DB, manager *auth.Manager) *AuthService
func NewAuthServiceWithCache(db gdb.DB, manager *auth.Manager, cache *gcache.Cache) *AuthService
func (s *AuthService) Captcha(ctx context.Context, height int, width int, color string) (Captcha, error)
```

- 现有 `NewAuthService` 保持签名不变，构造默认 cache，避免调用方回归。
- `NewAuthServiceWithCache` 仅用于可控依赖注入与测试。
- `Login` 内部调用私有 `verifyCaptcha(ctx, captchaID, verifyCode)`。

### 5.3 SVG 生成

复用原版 `cool-admin-go` 的最小实现策略：

- `crypto/rand` 生成安全的随机数字和随机 ID；
- `strings.Builder` 构造 SVG；
- 3 条灰色贝塞尔干扰线；
- 每个数字有轻微位置与旋转扰动；
- 每个数字附近生成噪点；
- `encoding/base64` 编码 SVG；
- SVG 文本只接收经颜色格式验证后的 `#RRGGBB` 颜色，非法颜色回退默认值，避免把 query 字符串直接写入 XML 属性。

## 6. 文件与职责

| 文件 | 变更 |
|---|---|
| `modules/base/service/auth.go` | 验证码 DTO、cache 依赖、SVG 生成、验证码写入与登录校验。 |
| `modules/base/service/auth_test.go` | 验证码响应、默认参数、错误/过期/一次性/忽略大小写校验的单元测试。 |
| `modules/base/controller/open/open.go` | 读取 captcha query 参数，调用 `AuthService.Captcha` 并返回标准响应。 |
| `modules/base/controller/controllers_test.go` | 用真实 route 断言 `/captcha` 返回非空可渲染 Data URL。 |
| `modules/base/auth_integration_test.go` | 扩展可选 MySQL 登录集成：先请求验证码、从测试 cache 注入或受控验证码流程完成登录；若无法在 HTTP server 外读取验证码，不在本阶段强行覆盖该场景。 |
| `docs/protocol/base-api-contract.md` | 把验证码从“契约描述”补充为已实现的大小写、TTL、一次性消费规则。 |
| `README.md` | 更新当前阶段与手工验证码验收命令。 |

## 7. 测试策略

### 7.1 服务单元测试

使用独立 `gcache.New()`：

1. `Captcha` 返回非空 `captchaId` 与以 Data URL 前缀开头的非空 data。
2. 默认和非法尺寸/颜色回退不会输出非法 SVG 属性。
3. 正确验证码大小写不同仍通过；随后 cache key 已删除。
4. 错误验证码返回 `验证码错误` 且保留 cache key。
5. 缓存不存在、验证码为空、ID 为空都返回 `验证码错误`。
6. 登录的验证码校验在数据库查询之前执行，因此使用 nil DB 和无效验证码时返回验证码错误而不是 panic。

### 7.2 Controller 路由测试

请求：

```text
GET /admin/base/open/captcha?height=45&width=150&color=%232c3142
```

断言：

- HTTP 200；
- `code` 为 1000；
- 内层 data 的 `captchaId` 非空；
- `data` 以 `data:image/svg+xml;base64,` 开头；
- base64 解码后是带请求宽高的 SVG。

### 7.3 回归验证

```bash
go test ./modules/base/service ./modules/base/controller ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
go vet ./...
git diff --check
```

## 8. 风险与限制

1. 进程内缓存不支持多实例共享，负载均衡环境需在后续阶段切换为共享 cache；当前单机开发与本地联调不受影响。
2. GoFrame cache 操作错误必须向调用者返回，不能悄悄放行登录。
3. 原版 `cool-admin-go` 的 `math/rand` 不是加密安全随机源；本实现使用 `crypto/rand`，保持协议而改进随机性。
4. 一次性验证码在正确输入后、密码验证前消费；用户若输错密码需刷新验证码重新登录。

## 9. 验收标准

1. Vue 登录页不再显示“验证码获取失败”，而是显示可点击刷新的验证码图片。
2. 前端表单得到非空 `captchaId`。
3. 正确用户名、密码与验证码可以登录。
4. 错误、过期或被消费的验证码返回 `验证码错误`，且不产生 token。
5. 同一个验证码不能成功登录两次。
6. `/admin/base/open/captcha` 与 `/admin/base/open/login` 均保持匿名访问。
7. 既有 EPS、refresh token、person、permmenu 与 CRUD 测试不回退。
