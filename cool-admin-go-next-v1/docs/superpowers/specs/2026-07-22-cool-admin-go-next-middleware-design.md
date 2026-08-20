# cool-admin-go-next 中间件与配置设计

日期：2026-07-22

修订：2026-07-23

## 1. 背景

`go-next` 当前在 `cool/app` 中无条件注册 token 认证和权限中间件：

```go
a.server.Use(auth.NewMiddleware(...))
controller.RegisterPermissionMiddleware(...)
```

这与两个参考实现都不一致：

1. Node 版由 base 模块通过 `globalMiddlewares` 声明 `translate`、`authority`、`log` 及其顺序。
2. 老 Go 版通过 `modules.base.middleware.authority.enable` 和 `log.enable` 控制注册。

当前还存在行为缺口：

1. token 中间件全局作用，会保护 Node 版不由 base authority 保护的 `/app/**`。
2. 未实现请求日志中间件。
3. 未实现翻译和统一异常中间件。
4. token 只校验签名和过期时间，注销、密码版本和 SSO 状态尚未进入中间件链。

## 2. 设计来源和取舍

| 问题 | 基准 | 设计选择 |
|---|---|---|
| 中间件归属 | Node | 归 base 模块声明，不由 `cool/app` 硬编码 |
| 配置开关 | 老 Go | authority 和 log 使用显式 `enable` |
| 执行顺序 | Node + 核心安全边界 | recovery -> translate -> error -> authority -> permission -> log -> handler |
| authority 范围 | Node | 仅处理 `/admin/**` |
| 配置根节点 | go-next 现状 | 继续使用单数 `module.base`，不引入第二套 `modules.base` |
| 中间件注册 | GoFrame | 在一个入口统一注册，以稳定顺序 |
| 日志参数 | 安全约束 | 兼容 Node 数据结构，但必须脱敏和限长 |

## 3. 目标

1. 中间件是模块 metadata 的一部分，与 controller、model、seed 同级。
2. authority 和 log 由 YAML 配置控制，默认值与老 Go 版一致。
3. authority 只影响 `/admin/**`，`/app/**` 由 app 模块自己的认证策略决定。
4. `IgnoreAuth` 和权限映射仍由 controller metadata 派生，不建第二份路由白名单。
5. 补齐 Node 版请求日志、会话校验和可选翻译行为。
6. 中间件可独立测试，应用测试可注入配置和依赖。

## 4. 非目标

1. 本设计不改变 controller route path、HTTP method 和 EPS 协议。
2. 不引入 `logic/` 目录。
3. 第一个实现不要求 Redis；会话存储通过接口预留外部实现。
4. 不复制 Node 的 Midway decorator/DI 形式，只对齐对外行为和模块责任。
5. 不在日志中保存密码、token、验证码或文件内容。

## 5. 配置模型

### 5.1 YAML

```yaml
server:
  clientMaxBodySize: "12mb"

cool:
  auth:
    jwtSecret: "" # 必须由本地或环境配置覆盖
    tokenExpire: 7200
    refreshExpire: 604800
    sso: false
  i18n:
    enable: false
    languages: ["zh-cn", "zh-tw", "en"]

module:
  base:
    allowKeys: []
    middleware:
      authority:
        enable: true
      log:
        enable: true
```

### 5.2 默认值

| 配置 | 默认值 | 说明 |
|---|---:|---|
| `module.base.middleware.authority.enable` | `true` | 同时控制 token 认证和路由权限校验 |
| `module.base.middleware.log.enable` | `true` | 记录 `/admin/**` 动态请求 |
| `cool.i18n.enable` | `false` | 只控制翻译，不关闭统一异常处理 |
| `cool.auth.sso` | `false` | 开启时只允许最新 access token |
| `cool.auth.jwtSecret` | 无 | dev/test 可显式注入测试值；其他环境缺失时启动失败 |

### 5.3 配置读取

新增 `modules/base/config` 包，使用类型化结构统一读取 base 配置：

```go
type Config struct {
    AllowKeys  []string
    Middleware MiddlewareConfig
}

type MiddlewareConfig struct {
    Authority Switch
    Log       Switch
}

type Switch struct {
    Enable bool
}

func Load(ctx context.Context) Config
```

`cool.auth` 和 `cool.i18n` 同样在 Application 组装边界读取为 typed config。禁止 controller、service 和 middleware 各自使用字符串路径读配置；配置只读取一次，然后通过构造参数注入。

生产配置约束：

1. `StartServer=true` 时必须提供至少 32 个随机字节等价熵的 `jwtSecret`；单元测试通过注入 AuthManager 使用显式测试密钥，不走隐式默认值。
2. 日志不得输出 secret 原文；错误只说明配置缺失或不符合强度要求。
3. 如未来支持密钥轮换，JWT header 使用 `kid`，解析期同时接受当前和上一把密钥，签发只使用当前密钥。
4. `server.clientMaxBodySize` 必须显式配置，并与上传文件上限及 multipart 开销保持一致。

## 6. 模块中间件 metadata

### 6.1 核心定义

新增 `cool/middleware` 包：

```go
type Definition struct {
    Name    string
    Order   int
    Handler ghttp.HandlerFunc
}

func Register(server *ghttp.Server, definitions []Definition) error
```

规则：

1. `Name` 全局唯一，重复时启动失败。
2. `Handler` 不能为 `nil`。
3. 按 `Order` 升序稳定排序。
4. 排序后一次调用 `server.Use(handlers...)`，使所有中间件处于同一路由深度，避免 GoFrame 深度优先改变顺序。
5. 路径范围由中间件内部快速过滤。

### 6.2 模块扩展

`registry.ModuleSpec` 和运行时 `module.Module` 增加中间件声明能力：

```go
type MiddlewareDeps struct {
    Context      context.Context
    DB           gdb.DB
    AuthManager  *auth.Manager
    SessionStore auth.SessionStore
    Translator   middleware.Translator
    Controllers  []controller.Definition
}

type ModuleSpec struct {
    // existing fields...
    Middlewares func(MiddlewareDeps) ([]middleware.Definition, error)
}
```

base 模块负责创建 base middleware definitions；`cool/app` 只负责收集、排序和注册，不再直接 import base service 来组装权限中间件。

`0-199` 是核心保留 order 区间。除框架认可的 `base.translate=100` adapter 外，模块 middleware 必须使用 `>=200`，并且不能注册 `cool.*` 保留名称。违反规则时启动失败。

## 7. 目录和职责

```text
cool-admin-go-next/
├── cool/
│   ├── auth/
│   │   ├── middleware.go       # JWT 解析与用户上下文
│   │   └── session.go          # 会话存储接口
│   └── middleware/
│       ├── definition.go       # 通用 metadata
│       ├── register.go         # 排序与统一注册
│       └── error.go            # recovery/error 两层核心边界
└── modules/base/
    ├── config/
    │   └── config.go           # base 类型化配置
    └── middleware/
        ├── middleware.go       # definitions 组装
        ├── authority.go        # `/admin/**` 认证
        ├── permission.go       # metadata 权限校验适配
        ├── log.go              # base_sys_log 记录
        └── translate.go        # 纯响应翻译
```

`cool/auth` 保持模块无关；`modules/base/middleware` 负责 Node base 语义和配置。

## 8. 执行顺序

| Order | 名称 | 类型 | 职责 |
|---:|---|---|---|
| 0 | `cool.recovery` | 核心前置 + recovery | 覆盖包括 translate 在内的全链最后 panic guard |
| 100 | `base.translate` | 后置 | 对最终 JSON 响应做无损翻译 |
| 150 | `cool.error` | 核心前置 + 后置 | 捕获下游 panic，解析 request error 并写统一失败响应 |
| 200 | `base.authority` | 前置 | 仅对 `/admin/**` 解析和校验 access token |
| 300 | `base.permission` | 前置 | 对存在 permission metadata 的路由校验权限 |
| 400 | `base.log` | 前置 | 记录通过 authority 的 `/admin/**` 请求 |
| - | route handler | - | controller/CRUD 执行 |

`cool.recovery`、`cool.error` 共享 resolver/logger/renderer。translate 调用 `Next()` 后能看到 `cool.error` 生成的失败体并进行后置翻译；translate 自身或其他外层框架 adapter panic 时由 `cool.recovery` 使用安全默认消息兜底，不再次调用已经失败的 translator。

## 9. Authority 设计

### 9.1 路径规则

1. 非 `/admin/` 前缀：直接 `Next()`。
2. 当前 `METHOD:path` route metadata 标记 `IgnoreAuth=true`：直接 `Next()`。
3. 其他 `/admin/**`：必须提供有效 access token。
4. refresh token 不能作为 access token。
5. authority 关闭时，permission 也不注册，避免出现“关闭认证后仍因缺少用户上下文拒绝”。

IgnoreAuth 与 permission 必须调用 controller RoutePlan 使用的同一个 canonical route helper，route key 固定为 `METHOD:normalizedPath`。禁止只按 path 生成白名单或在 middleware 包重复实现路径清理；同一路径的 GET 和 POST 可以拥有不同认证策略。

### 9.2 会话状态

新增可注入接口：

```go
type SessionStore interface {
    Save(ctx context.Context, session Session) error
    ReplaceUser(ctx context.Context, userID int64, session Session) error
    Get(ctx context.Context, sessionID string) (Session, bool, error)
    Rotate(ctx context.Context, sessionID string, oldRefreshJTIHash string, next Session) (bool, error)
    Delete(ctx context.Context, sessionID string) error
    DeleteUser(ctx context.Context, userID int64) error
}

type Session struct {
    ID                  string
    UserID              int64
    AccessJTIHash       string
    RefreshJTIHash      string
    PasswordVersion     int64
    RefreshTokenExpires time.Time
}
```

登录、刷新和注销必须与该 store 共用同一实例：

1. login 为每个登录会话生成不可预测的 `sid`，为 access/refresh token 生成不同 `jti`；JWT claims 必须明确包含 `sid`、`jti` 和 token type，session 只保存 JTI 的 SHA-256 hash，不保存 token 明文。
2. refresh 必须通过 `Rotate` 原子比较旧 refresh JTI hash 并写入新 session；比较失败表示旧 token 已使用或已撤销，返回 unauthorized。
3. logout 删除当前 `sid`；显式“退出全部设备”或密码变更使用 `DeleteUser`。
4. authority 校验 session 存在，且 `session.UserID` 必须与已验签 claims 的 user ID 一致。
5. token 中 `passwordVersion` 必须与 session 一致。
6. `sso=true` 时，登录通过 `ReplaceUser` 原子撤销该用户其他 session 并写入新 session，且请求 access JTI hash 必须与 session 中最新值一致。
7. session 生命周期覆盖 refresh token 有效期；access token 自身仍按 JWT `exp` 到期。
8. refresh 前必须确认 session 存在、用户可用、密码版本一致并成功原子轮换，因此 logout 后或已轮换的 refresh token 不能重建会话。
9. SessionStore 返回 error 是基础设施故障，映射为 internal；只有 `ok=false`、版本/JTI 不匹配才映射 unauthorized。

进程内 adapter 仅用于测试、开发和明确的单实例部署；多实例生产必须注入共享、支持 TTL 和原子 Rotate 的 store，否则启动失败。authority 和 AuthService 仅依赖接口，不依赖具体 cache。

`sso=false` 表示同一用户可以有多个独立 `sid`，刷新或注销一个 session 不影响其他设备；密码变更和管理员禁用用户必须撤销该用户全部 session。

### 9.3 权限

permission 继续使用 controller metadata 产生的 `METHOD:path -> permission` 映射。

1. 无 permission metadata 的 comm/open route 不做权限校验。
2. CRUD 和显式 `Permission` route 调用 `PermissionService.HasPermission`。
3. `admin` 账号和 admin 角色由 service 统一判定为超管。
4. 无 token 返回 HTTP 401；无权限返回 HTTP 403。
5. `HasPermission` 返回 `(false, nil)` 才是 forbidden；返回 error 必须进入 internal error boundary，不能伪装为 403。

## 10. Log 设计

### 10.1 记录范围

1. 只记录 `/admin/**` 动态请求。
2. authority 拒绝的请求不记录，与 Node 中间件顺序一致。
3. 日志写入失败只记录服务端 error，不中断业务请求。

### 10.2 数据映射

| 字段 | 来源 |
|---|---|
| `user_id` | `auth.UserFromContext`，无用户时为 `NULL` |
| `action` | 不含 query 的 URL path |
| `ip` | GoFrame 的 client IP 解析结果 |
| `params` | GET 使用 query，其他方法使用 body/form 的 JSON |
| `tenant_id` | 当前用户 tenant ID |

### 10.3 安全规则

1. 递归脱敏 `password`、`oldPassword`、`newPassword`、`token`、`accessToken`、`refreshToken`、`authorization`、`captchaId`、`verifyCode` 等凭据字段。
2. key 先去除 `-`、`_` 并转小写再比较；同时支持可注入的额外敏感字段集合，禁止只匹配固定六个精确字符串。
3. multipart 只保存字段和文件名，不读取文件内容。
4. 序列化后最大 64 KiB，超出时截断并标记。
5. 落库使用类型化 DO 数据对象，不使用 `g.Map` 或 `map[string]interface{}` 组装数据库写入。
6. 数据库错误使用 `gerror.Wrap` 保留完整错误栈。

## 11. Translate 与核心异常设计

base 模块存在时 translate middleware 始终注册，`cool.i18n.enable` 只控制翻译分支；没有 base 模块时核心 error/recovery 仍独立工作。

核心异常职责：

1. `cool.error` 调用 `Next()`，捕获下游 panic，并在返回后检查 `r.GetError()`。
2. `cool.recovery` 捕获逃逸出 translate/常规边界的 panic。
3. 两者共用统一 renderer/logger；内部错误记录完整 stack，客户端不返回 SQL、文件路径和 stack。
4. 未 flush 的 GoFrame response buffer 可清空覆盖；已 flush、raw write 或 hijack 的响应只记录错误，不追加第二份 JSON。

后置翻译职责：

1. 仅处理 JSON `response.Body`，不修改 HTML、文件和空响应。
2. 语言从 `language` header 读取，并限制在 `cool.i18n.languages` 内。
3. 第一批对齐 Node 的菜单和字典路径：`permmenu`、menu list、dict info list/data、dict type page。
4. translator 使用接口注入；当 i18n 关闭时不创建外部 translator。
5. JSON 必须使用 `json.Decoder.UseNumber`、`json.RawMessage` 或等价无损方式处理，禁止把 int64 ID 经 `float64` 往返。
6. 翻译后的响应设置 `Vary: language`，避免共享缓存跨语言复用。

translate 不再捕获 panic、读取/清除 `r.GetError()`、记录通用错误或生成失败响应。

## 12. 应用组装流程

```text
load core config
  -> build registered modules/controllers
  -> compile all Action/CRUD routes and validate every route conflict
  -> load each module's typed config
  -> build middleware definitions with all controller metadata
  -> append mandatory cool.recovery/cool.error
  -> validate names and sort by Order
  -> server.Use(all middleware handlers)
  -> bind the precompiled RoutePlan
```

`Application.Options` 增加可选的 module middleware definitions/session store/translator 注入点，测试不得依赖全局单例。显式 Options 优先于 YAML，与现有 schema/seed 配置测试规则一致。

Options 对 module middleware 的覆盖语义使用显式类型，不能再用 nil/non-nil slice 隐式表达：

```go
type MiddlewareMode string

const (
    MiddlewareAppend         MiddlewareMode = "append"
    MiddlewareReplaceModules MiddlewareMode = "replace-modules"
)

type MiddlewareOverride struct {
    Mode        MiddlewareMode
    Definitions []middleware.Definition
}
```

无论哪种模式，都不能删除、替换或重命名 `cool.recovery`/`cool.error`；核心行为只能通过 ErrorRenderer/ErrorLogger 注入测试替身。未知 mode 在启动阶段失败。

所有 compile/config/middleware validation error 由 Application Build/Start 返回；库代码不得调用 `g.Log().Fatal`，且失败前不得对 Server 产生部分注册。

## 13. 测试设计

### 13.1 配置

1. authority/log 缺省时默认开启。
2. YAML 显式 `false` 能关闭对应 definition。
3. 显式 Application Options 覆盖 YAML。
4. `module.base` 是唯一 base 配置根。
5. 生产环境缺少/弱 jwtSecret 启动失败，错误不包含 secret。
6. clientMaxBodySize 与上传上限配置一致。

### 13.2 注册与顺序

1. 重复 middleware name 返回启动错误。
2. 验证 recovery -> translate -> error -> authority -> permission -> log -> handler 顺序。
3. 关闭 authority 时 permission 不注册。
4. 无 base 模块时应用仍可启动。
5. 模块占用核心 order/name 返回启动错误。
6. 替换 module middleware 不会移除核心边界。

### 13.3 Authority

1. `/app/**` 和 `/health` 不要求 admin token。
2. `IgnoreAuth` admin route 可匿名访问。
3. 受保护 admin route 无 token 返回 401。
4. refresh token、过期 token、已注销 session 返回 401。
5. 密码版本变更使旧 token 失效。
6. SSO 开启时旧 token 失效。
7. 无权限 route 返回 403。
8. 同 path 不同 method 的 IgnoreAuth 不互相放行。
9. SessionStore 故障返回 internal，PermissionService 故障不返回 403。
10. 旧 refresh token 重放和并发双 refresh 最多一个成功。
11. `sso=false` 多 session 相互独立，密码变更撤销全部 session。

### 13.4 Log

1. 正常 admin 请求写入一条 `base_sys_log`。
2. `/app/**`、静态文件和 authority 拒绝的请求不写入。
3. 用户、tenant、action、IP 和 params 映射正确。
4. 敏感字段不出现在存储内容中。
5. 日志库失败不改变业务响应。
6. old/new password、access token、不同大小写/分隔符和嵌套数组均完成脱敏。
7. multipart 只记录字段和安全文件名，body 上限在解析前生效。

### 13.5 Translate

1. i18n 关闭时响应字节不变。
2. i18n 开启时只翻译白名单路径和字段。
3. 不支持的语言回退到默认语言。
4. HTML 和文件响应不变。
5. 未处理错误不泄露内部细节。
6. 大于 `2^53` 的整数、`json.Number`、null 和嵌套分页数据翻译前后保持数值不变。
7. translate panic 由 `cool.recovery` 兜底且错误只记录一次。

## 14. 分阶段实施

### 阶段 A：配置和注册骨架

1. 增加类型化 base config。
2. 增加 middleware metadata 和 module 扩展点。
3. 将当前 auth/permission 迁移为 base definitions。
4. 修正 authority 仅处理 `/admin/**`。

### 阶段 B：会话对齐

1. 增加带 sid/jti 和原子 Rotate 的 SessionStore。
2. login/refresh/logout/password change 维护 session 生命周期。
3. 实现 password version、refresh replay protection、多 session 和 SSO 校验。
4. 增加生产 shared store 与 jwtSecret 启动约束。

### 阶段 C：请求日志

1. 增加 `LogService.Record`。
2. 增加脱敏、限长和 multipart 处理。
3. 接入 log middleware 开关。

### 阶段 D：异常和翻译

1. 完成 public/internal error 分类后启用 `cool.recovery`/`cool.error`。
2. 将 translate 缩减为纯翻译并实现无损 JSON 处理。
3. 定义 Translator 接口和 no-op 实现。
4. 实现 Node 首批菜单/字典翻译路径。

## 15. 验收标准

1. YAML 可独立关闭 authority 或 log，且有自动化测试。
2. 默认配置下 `/admin/**` 需要 token，`/app/**` 不会被 base authority 误拦截。
3. IgnoreAuth、CRUD permission 和 custom permission 仍只从 controller metadata 派生。
4. 注销后原 access token 立即失效。
5. refresh token 单次轮换且不可重放，多实例生产使用共享 SessionStore。
6. log 开启时写入 Node 兼容字段，关闭时不产生记录，任何凭据不进入 params。
7. i18n 关闭时无响应回归，开启时指定菜单/字典响应完成无损翻译。
8. 核心边界不可关闭，translate 或任意模块中间件 panic 不逃逸应用。
9. 中间件顺序、错误状态和 Node 响应体通过 HTTP 集成测试锁定。
