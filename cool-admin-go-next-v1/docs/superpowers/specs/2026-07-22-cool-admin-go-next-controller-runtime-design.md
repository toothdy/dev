# cool-admin-go-next Controller Runtime 设计

日期：2026-07-22

修订：2026-07-23

## 1. 背景

`go-next` 的目标是通用、快速开发的后台管理框架。当前标准 CRUD 已经可以由 controller metadata 自动生成，但自定义接口仍普遍使用 `func(*ghttp.Request)`：

```go
func move(service *UserService) func(*ghttp.Request) {
    return func(r *ghttp.Request) {
        request := MoveRequest{}
        if err := r.Parse(&request); err != nil {
            r.Response.WriteJson(response.Fail("参数错误"))
            return
        }
        if err := service.Move(r.Context(), request); err != nil {
            g.Log().Error(r.Context(), err)
            r.Response.WriteJson(response.Fail("操作失败"))
            return
        }
        r.Response.WriteJson(response.OK(map[string]interface{}{}))
    }
}
```

这使每个模块重复承担以下框架职责：

1. 请求解析和校验。
2. 成功响应包装。
3. 错误分类和 HTTP status 映射。
4. 服务端错误日志。
5. 内部错误信息脱敏。

`cool/controller/register.go` 已经支持反射调用并自动包装返回值，但当前实现存在三个根本缺口：

1. 只识别 `context.Context` 和 `*ghttp.Request`，其他参数直接传零值，无法作为类型化 DTO runtime。
2. handler 签名在请求时才检查，配置错误不能在启动阶段失败。
3. `error` 直接以 `err.Error()` 返回客户端，无法区分业务错误和内部错误。

因此模块只能绕过 runtime，退回低层 GoFrame handler。

## 2. Node 版基准

Node 版的实际职责划分如下：

| 能力 | Node 位置 | 行为 |
|---|---|---|
| 标准 CRUD | `@CoolController` + `BaseController` | 根据 entity、service 和 api metadata 自动生成 |
| 自定义接口 | controller method | 只读取参数、调用 service、`this.ok()`/`this.fail()` |
| 参数绑定 | Midway decorator | `@Body`、`@Query`、`@Validate` |
| 业务错误 | service | 抛出 `CoolCommException`/`CoolValidateException` |
| 通用异常 | core `CoolExceptionFilter` | 全局记录日志并生成错误响应 |
| 翻译 | base translate middleware | 对响应消息和指定业务数据翻译 |
| 特殊响应 | controller context | HTML、文件上传等少量接口直接使用 Context |

Go 版不复制 decorator 和 DI 语法，但必须对齐职责边界。

Go 版允许 action 直接返回数据，由 runtime 自动包装 `response.OK`。这比 Node 的每个方法调用 `this.ok()` 更符合 Go 的 `(value, error)` 习惯，但对外协议保持一致。

## 3. 设计目标

1. 普通模块 controller 不出现 `g.Log()`、`response.OK/Fail()` 或 `r.Response.WriteJson()`。
2. 普通 action 只接收 `context.Context` 和可选的类型化请求 DTO，只返回数据和/或错误。
3. action 签名、路由冲突和绑定配置在应用启动时完成校验。
4. 参数绑定、验证、成功响应和错误响应由 `cool` 核心统一处理。
5. CRUD 与自定义 action 使用同一套结果和错误管线。
6. 内部错误保留 `gerror` stack，但不向客户端暴露 SQL、路径或 stack。
7. authority、permission、CRUD 和 action 的错误行为一致。
8. HTML、文件流等特殊响应有明确的 Result 抽象，不迫使普通 action 使用 `*ghttp.Request`。

## 4. 非目标

1. 不复制 Midway decorator、文件路径扫描和请求级 DI。
2. 不引入 `logic/` 目录；业务逻辑继续位于 `service/`。
3. 不在本次设计中重写 ORM、schema model 或 CRUD SQL builder。
4. 不要求将所有 CRUD 动态 map 立即改为静态 entity DTO。
5. 不通过 panic 模拟 Node throw；Go service 正常返回 `error`。
6. 不让业务模块注册或关闭核心错误边界。

## 5. 目标开发体验

### 5.1 标准 CRUD

CRUD 继续使用已有声明式 metadata：

```go
func TypeController(service *DictTypeService, definition model.Definition) controller.Definition {
    return controller.Admin("dict/type").
        Model(definition).
        Service(service).
        CRUD(controller.CRUDOptions{
            APIs: []string{"add", "delete", "update", "info", "list", "page"},
        }).
        Build()
}
```

controller 不需要为六个标准接口编写 handler。

### 5.2 自定义 action

```go
type MoveRequest struct {
    DepartmentID int64   `json:"departmentId" v:"required|min:1#部门参数错误#部门参数错误"`
    UserIDs      []int64 `json:"userIds" v:"required#用户不能为空"`
}

func move(service *UserService) func(context.Context, *MoveRequest) error {
    return func(ctx context.Context, request *MoveRequest) error {
        return service.Move(ctx, *request)
    }
}

controller.Admin("base/sys/user").
    Route(controller.RouteOptions{
        Name:       "move",
        Method:     http.MethodPost,
        Path:       "/move",
        Permission: "base:sys:user:move",
        Action:     move(service),
    })
```

action 不解析 JSON、不写响应、不记录日志。

### 5.3 返回数据

```go
func program() string {
    return "Go"
}

func person(service *UserService) func(context.Context) (map[string]interface{}, error) {
    return func(ctx context.Context) (map[string]interface{}, error) {
        user, err := auth.RequireUser(ctx)
        if err != nil {
            return nil, err
        }
        return service.Person(ctx, user.UserId)
    }
}
```

runtime 自动转换为：

```json
{"code":1000,"message":"success","data":{}}
```

只返回 `error` 且错误为空时，成功响应不包含 `data`，与 Node `this.ok()` 一致。确实需要空对象时，action 显式返回空对象。

### 5.4 文件上传

GoFrame 原生支持将上传文件绑定到 DTO：

```go
type UploadRequest struct {
    File *ghttp.UploadFile `type:"file" v:"required#文件不能为空"`
}

func upload(service *UploadService) func(context.Context, *UploadRequest) (string, error) {
    return func(ctx context.Context, request *UploadRequest) (string, error) {
        return service.Upload(ctx, request.File)
    }
}
```

上传接口仍属于普通 action，不需要直接写 response。

### 5.5 HTML 和文件流

特殊响应通过 `controller.Result` 表达：

```go
func htmlByKey(service *ParamService) func(context.Context, *HTMLRequest) (controller.Result, error) {
    return func(ctx context.Context, request *HTMLRequest) (controller.Result, error) {
        html, err := service.HTMLByKey(ctx, request.Key)
        if err != nil {
            return nil, err
        }
        return controller.HTML(html), nil
    }
}
```

首批 Result：

1. `controller.HTML(content string)`。
2. `controller.File(path string)`。
3. `controller.Stream(contentType string, reader io.Reader)`。
4. `controller.Redirect(location string, status ...int)`。
5. `controller.NoContent()`。

Result 只负责成功响应的输出格式，错误仍进入统一错误管线。

## 6. Route Metadata

### 6.1 新定义

```go
type BindSource string

const (
    BindAuto  BindSource = "auto"
    BindQuery BindSource = "query"
    BindForm  BindSource = "form"
    BindJSON  BindSource = "json"
)

type RouteOptions struct {
    Name               string
    Method             string
    Path               string
    Description        string
    IgnoreAuth         bool
    Permission         string
    Action             interface{}
    Bind               BindSource
    AllowUnknownFields bool
}
```

`RouteDefinition` 只保存可复制、可输出的声明 metadata。启动阶段由 compiler 生成内部 `compiledRoute` 和 invoker；编译产物不进入 EPS、不放入导出的 metadata，也不允许模块直接构造。

### 6.2 路由规范化

controller build、RoutePlan compiler、IgnoreAuth、permission、EPS 和最终 BindHandler 必须复用同一个 canonical route helper：

1. method 转为大写并校验为明确支持的 HTTP method。
2. path 必须以 `/` 开头，使用 `path.Clean` 消除重复斜杠和 `.`，根路径以外移除尾斜杠。
3. 拒绝 `..`、query、fragment、控制字符和空 route name。
4. canonical key 固定为 `METHOD:normalizedPath`，禁止各包自行拼接另一种 key。
5. 首批 metadata route 不支持动态 path 参数或 wildcard；后续支持时必须基于 GoFrame matched route template 派生权限键，不能直接使用实际 URL path。

### 6.3 Handler 处理

`Handler interface{}` 从正常 Route API 中移除，不作为新旧协议并存的长期兼容字段。

迁移期间可在内部保留一次性 legacy adapter，但完成 base、dict 迁移后必须删除：

1. `RouteOptions.Handler`。
2. `RouteDefinition.Handler`。
3. `handleCustomRoute` 的任意函数/零值注入逻辑。
4. controller 中直接返回 `ghttp.HandlerFunc` 的普通接口。

底层 escape hatch 如确有需要，单独设计为显式 `RawAction`，不能混入普通 Action：

```go
type RawAction func(ctx context.Context, request *ghttp.Request) error
```

首批迁移使用 Result 和上传 DTO 后，预计现有 base、dict 不需要 RawAction。

## 7. Action Runtime

### 7.1 支持的签名

启动时只接受以下签名：

| 输入 | 输出 | 说明 |
|---|---|---|
| `()` | `T` | 无失败路径的常量/纯读取 action |
| `()` | `error` | 无返回数据 |
| `()` | `(T, error)` | 返回数据 |
| `(context.Context)` | 上述任一输出 | 上下文 action |
| `(*Request)` | 上述任一输出 | 类型化输入 |
| `(context.Context, *Request)` | 上述任一输出 | 推荐形式 |

约束：

1. 最多一个请求 DTO。
2. DTO 必须是 struct 指针。
3. `context.Context` 必须位于 DTO 之前。
4. 最多两个返回值。
5. 两个返回值时第二个必须是 `error`。
6. 不支持的签名在 RoutePlan compile 阶段返回错误，应用不得启动。
7. runtime 将反射函数预编译为 invoker，每个请求不重复分析类型。
8. `T` 不得实现 `error`，避免 `T` 与 error-only 签名产生歧义。
9. 返回 `controller.Result` 时，nil interface 或 typed nil Result 视为内部错误，不得静默写入 `null`。

签名实际在 RoutePlan compile 阶段校验；`RegisterRoutes` 只提交已经验证的 plan，不再承担反射签名分析。

### 7.2 请求绑定

默认 `BindAuto` 规则：

| 请求 | 绑定方式 |
|---|---|
| GET/HEAD | `Request.ParseQuery` |
| `application/json` | 严格 JSON 解码后执行 GoFrame validation |
| multipart/form-data | `Request.ParseForm`，支持 `*ghttp.UploadFile` |
| form-urlencoded | `Request.ParseForm` |
| 其他 | `Request.Parse` |

JSON 默认拒绝未知字段，避免管理接口静默接受拼写错误或越权字段。兼容第三方 payload 的接口必须显式设置 `AllowUnknownFields=true`。

绑定契约补充如下：

1. 使用 `mime.ParseMediaType` 解析 Content-Type；`application/json` 和 `application/*+json` 进入严格 JSON 分支。
2. 有 body 的 API 请求缺少或使用不支持的 Content-Type 时返回 HTTP 415，不允许通过伪装为 `text/plain` 绕过严格 JSON。
3. 严格 JSON 必须拒绝未知字段、重复字段、尾随第二个 JSON value 和字段类型错误；BindJSON + DTO 的空 body 按空对象绑定，再由 required validation 决定是否允许。
4. `Request.ParseQuery`/`Request.ParseForm` 已执行 GoFrame `v` tag 校验，binder 不得对同一 DTO 重复运行 validation；严格 JSON 分支只手工运行一次 GoFrame validation。
5. 全局 `server.clientMaxBodySize` 必须显式配置，并大于允许的最大上传文件加 multipart 开销；超限统一映射为 HTTP 413 和 validate error。
6. 首批不增加 route 级 body limit；如后续需要，扩展独立 `MaxBodyBytes` metadata，不能在 action 内自行读取无限 body。
7. `BindJSON` 只允许 POST、PUT、PATCH、DELETE；GET/HEAD 使用 BindJSON 在 compile 阶段失败。

绑定完成后：

1. 执行 GoFrame `v` tag 校验。
2. 如果 DTO 实现 `Validate() error`，继续执行领域级输入校验。
3. 任一绑定/校验错误统一转换为 validate error，业务码 `1002`。
4. `Validate() error` 应是无 I/O 的纯领域校验；其中明确返回的 typed internal error 必须保留，不能被 binder 改写成 validate error。

需要区分“未提供”和“显式空值”的 DTO 使用指针字段，不再在 controller 手工解析 raw JSON 记录字段存在性。

### 7.3 成功结果

| Action 返回 | HTTP 响应 |
|---|---|
| `T` | `response.OK(T)` |
| `(T, nil)` | `response.OK(T)` |
| `nil error` | `response.OK(nil)`，省略 `data` |
| `controller.Result` | Result 直接写成功响应 |
| 非 nil error | 不写成功响应，交给 error boundary |

普通 action 禁止返回 `response.Body`，避免业务 controller 再次依赖传输响应结构。

`T` 为 nil interface 时省略 `data`；`T` 为 typed nil pointer/map/slice 时固定输出 `"data":null`。该差异属于协议，必须由字节级测试锁定。

### 7.4 Result 契约

```go
type Result interface {
    Write(r *ghttp.Request) error
}
```

1. Result 是一次性 writer；runtime 不得在 `Write` 成功后再追加 JSON。
2. `Stream` 接收 `io.Reader`，若同时实现 `io.Closer`，由 Result 在请求结束时关闭；关闭错误仅记录，不能在已提交后追加 JSON。
3. `File` 必须在提交 header 前完成路径存在性和可读性检查；下载文件名、inline/attachment 行为通过显式参数扩展，不能从不可信 header 直接拼接。
4. `Redirect` 只接受 301、302、303、307、308，Location 必须拒绝 CR/LF。
5. `NoContent` 固定返回 204 且无 body；HEAD 执行与 GET 相同的校验和 header 生成，但不写 body。
6. Result 在提交前失败时进入统一 error boundary；提交后失败只记录一次完整错误并终止输出。

## 8. 错误模型

### 8.1 使用 GoFrame gerror/gcode

不创建与 GoFrame 平行的 error 基类。`cool/errors` 基于以下原生能力构建：

1. `gerror.NewCode/NewCodef` 创建带 code 的错误。
2. `gerror.WrapCode/WrapCodef` 包装底层错误并保留 stack。
3. `gerror.Code(err)` 在边界读取分类。
4. `gcode.Code.Detail()` 保存私有、不可变的 typed detail，包含 error kind、HTTP status、是否公开消息和日志级别。

wire 业务码与内部分类分离：resolver 不只按 `Code()` 整数判断 unauthorized/forbidden/internal，而是读取 `cool/errors` 创建的 typed detail。未知 detail、第三方 gcode 和无 code error 一律降级为 internal，禁止模块自行伪造公开错误元数据。

`cool/errors` 对模块提供稳定构造函数：

```go
func Comm(message string) error
func Validate(message string) error
func Unauthorized(message ...string) error
func Forbidden(message ...string) error
func Core(message string) error
func Internal(err error, operation string) error
```

模块 service 不直接依赖 HTTP request，但可以返回业务含义明确的 `Comm`/`Validate` 错误。

数据库、缓存、文件系统等基础设施错误继续使用 `gerror.Wrap` 或 `errors.Internal` 保留 stack。未携带公开 code 的错误一律视为 internal，不能将 `err.Error()` 返回客户端。

依赖调用必须区分业务否定与系统故障：Session 不存在、权限检查返回 `(false, nil)` 分别映射 unauthorized/forbidden；SessionStore 或 PermissionService 返回 error 时必须映射 internal，不能伪装成 401/403。

### 8.2 对外映射

| 类型 | HTTP | 业务码 | 客户端消息 | 日志 |
|---|---:|---:|---|---|
| success | 200 | 1000 | success | 无 |
| comm/business | 200 | 1001 | error 公共消息 | Warn |
| validate | 200 | 1002 | 校验消息 | Debug/Warn |
| core | 200 | 1003 | 安全的 core 消息 | Error |
| unauthorized | 401 | 1001 | 登录失效~ | Info/Warn |
| forbidden | 403 | 1001 | 登录失效或无权限访问~ | Info/Warn |
| unclassified/internal | 200 | 1001 | 操作失败 | Error + stack |
| panic | 200 | 1001 | 操作失败 | Error + stack |

Node 当前对部分未知异常返回 `error.message`。Go 版明确不复制这一信息泄露行为。

internal/panic 继续返回 HTTP 200 是首批迁移的显式 Node 协议兼容决定，不代表请求成功。监控必须同时按业务码和 resolved error kind 统计；未来若切换为 HTTP 500，必须作为独立协议变更发布。配置和无效 action 等启动期 Core error 直接由 Build/Start 返回，不生成 HTTP 响应。

### 8.3 迁移要求

现有 service 中的裸 `gerror.New` 同时表示业务错误和内部错误，不能依赖字符串白名单推断。

迁移时必须逐项分类：

1. 用户可修正的业务条件改为 `coolErrors.Comm`。
2. 输入结构和字段问题改为 `coolErrors.Validate`。
3. 登录状态改为 `coolErrors.Unauthorized`。
4. 权限不足改为 `coolErrors.Forbidden`。
5. DB/cache/file 错误保持 `gerror.Wrap`，由边界视为 internal。
6. 框架配置和无效 action 签名在启动时返回 `Core` error，不进入 HTTP runtime。

切换新 error boundary 前必须完成 base、dict 的业务错误分类，禁止用“裸错误暂时公开”作为兼容方案。

## 9. 核心错误边界

### 9.1 归属

新增 `cool/middleware/error.go`，提供每个 Application 必装、不可由模块配置关闭的两层核心保护：

1. 最外层 `cool.recovery` 是最后一道 panic guard，覆盖 translate 和所有模块中间件。
2. `cool.error` 位于 translate 内层，处理 authority、permission、log、action 和 CRUD 产生的 error/panic，使 translate 能后置翻译正常错误响应。

两层共享同一个 resolver、logger 和 renderer，不得复制错误映射逻辑。`cool.recovery` 只处理逃逸出常规边界的 panic；其响应使用安全的默认消息，不再次调用已经 panic 的 translator。

它负责：

1. 捕获 panic。
2. 读取 `r.GetError()`。
3. 解析 `gerror.Code` 和 detail。
4. 按错误类型记录一次日志。
5. 清理未提交的部分 buffer。
6. 写入统一 HTTP status 和 `response.Body`。
7. 清除 request error，避免 GoFrame 再次处理。
8. 将 panic 转为带 `runtime/debug.Stack` 和 request trace ID 的 internal error，再交给共享 logger。

普通 action、CRUD、authority、permission 只设置 error 并返回，不直接写失败响应。

### 9.2 与翻译中间件的顺序

目标顺序：

| Order | 名称 | 责任 |
|---:|---|---|
| 0 | `cool.recovery` | 全链最后 panic guard |
| 100 | `base.translate` | 后置翻译最终 JSON，包括 error boundary 生成的消息 |
| 150 | `cool.error` | 通用异常和 panic 边界 |
| 200 | `base.authority` | admin 登录校验 |
| 300 | `base.permission` | 路由权限校验 |
| 400 | `base.log` | 请求操作日志 |
| - | action/CRUD | 业务执行 |

translate 先调用 `Next()`，因此可以在 `cool.error` 写入响应后翻译最终 body。

`0-199` 为核心保留 order 区间，除 `base.translate` adapter 外，模块中间件必须使用 `>=200`。这样普通模块始终位于 `cool.error` 内层。

`modules/base/middleware/translate.go` 删除 panic、`r.GetError()` 和日志处理，只保留响应翻译。

没有 base 模块时，`cool.recovery` 和 `cool.error` 仍由 Application 注册并正常工作；仅缺少可选翻译能力。

### 9.3 已提交响应

GoFrame Response 默认带 buffer。仅设置 status、header 或写入尚未 flush 的 buffer 不算已提交，error boundary 应清空 buffer、重置 status 和相关 content header 后覆盖响应。只有底层 header/body 已写出、response 已 flush、hijack 或 raw writer 已使用时才算已提交。

如果 Result/RawAction 已真正提交响应后发生错误，error boundary 不能安全覆盖响应：

1. 记录完整错误。
2. 不追加第二份 JSON。
3. 中止后续写入。

普通 Action 在成功前不得直接操作 Response，因此不会产生这一状态。

## 10. CRUD 统一

当前 `cool/crud/handler.go` 同时负责 HTTP 解析、错误写入和成功响应，造成第二套 controller runtime。

调整后：

1. `cool/crud.Runtime`、query builder、resource metadata 保持不变。
2. CRUD HTTP 路由绑定移动到 `cool/controller/crud.go`。
3. CRUD route 构造内部 action，调用 `Runtime.Add/Delete/Update/Info/List/Page`。
4. CRUD 输入解析错误使用 validate error。
5. Runtime/service 错误原样进入 `cool.error`。
6. 成功结果通过同一个 result writer 包装。
7. 删除 `cool/crud/handler.go` 中的 `writeValidateError`、`writeCommError` 和直接 `WriteJson`。

这样 controller 是唯一 HTTP transport owner，crud 保持可独立测试的业务 runtime，也避免 `crud -> controller` 的循环依赖。

## 11. Authentication 与 Permission

authority 和 permission 不再直接写响应：

```go
if tokenInvalid {
    r.SetError(coolErrors.Unauthorized())
    return
}
```

```go
if !allowed {
    r.SetError(coolErrors.Forbidden())
    return
}
```

新增：

```go
func RequireUser(ctx context.Context) (UserContext, error)
```

action 使用该函数读取登录用户；缺失时得到统一 unauthorized error，不调用 `coolAuth.Unauthorized(r)`。

IgnoreAuth 和 permission map 仍只从 Route/CRUD metadata 派生，不增加配置白名单。

## 12. 模块迁移

### 12.1 base admin/open/comm/app

| 当前接口 | 目标 |
|---|---|
| person、permmenu、EPS、program、uploadMode | 无 DTO action，返回数据/error |
| personUpdate、move、order、setKeep、menu parse/create/import | 类型化 JSON DTO action |
| captcha、param、getKeep | query DTO action |
| login、refreshToken | 类型化 DTO + typed auth/business error |
| upload | `UploadRequest` 绑定 `*ghttp.UploadFile` |
| html/htmlByKey | 返回 `controller.HTML` Result |
| logout、clear、menu export | error-only 或 data action |

### 12.2 dict

dict `data` 和 `types` 改为普通 action。dict 不声明自己的 global middleware，继续由 base authority、permission、translate 依据全局路径 metadata 处理。

### 12.3 Service 错误

controller 中现有的错误字符串 switch（上传、登录、移动、菜单导入等）移动到错误产生位置：

1. 可公开业务失败由 service 返回 typed public error。
2. 基础设施错误由 service `gerror.Wrap`。
3. controller 不根据 `err.Error()` 决定响应。

## 13. 包结构

```text
cool-admin-go-next/
├── cool/
│   ├── controller/
│   │   ├── action.go       # action 签名编译与 invoker
│   │   ├── bind.go         # DTO 绑定与 validation
│   │   ├── result.go       # JSON success 与 HTML/File/Stream Result
│   │   ├── crud.go         # CRUD HTTP 路由适配
│   │   ├── plan.go         # RoutePlan 编译、全局冲突校验与一次性注册
│   │   ├── definition.go   # Action/Bind metadata
│   │   └── register.go     # 统一注册
│   ├── errors/
│   │   ├── code.go         # 1000-1003 业务码
│   │   ├── error.go        # gcode 与构造函数
│   │   └── resolve.go      # 边界映射
│   └── middleware/
│       └── error.go        # 核心错误边界
└── modules/
    ├── base/controller/    # 只声明 route/action
    └── dict/controller/    # 只声明 route/action
```

请求 DTO 优先放在 `modules/<module>/dto`。service 专用的领域输入可以保留在 service 包，但不得依赖 `ghttp.Request` 或 response 类型。

## 14. Application 组装

```text
build modules/controllers
  -> compile every Action/CRUD route into an immutable RoutePlan
  -> validate all custom/CRUD/core route conflicts without mutating Server
  -> collect module middleware definitions
  -> append mandatory cool.recovery and cool.error definitions
  -> validate names and stable-sort middleware
  -> register middleware once
  -> bind the validated RoutePlan once
```

编译阶段返回 `(*RoutePlan, error)`；Application 的 Build/Start 路径向调用者返回 error，不得在库代码中使用 `g.Log().Fatal` 终止进程。只有 `main` 可以决定是否 fatal。

`Application.Options.MiddlewareOverride` 按 middleware 设计中的显式 `append`/`replace-modules` mode 处理模块中间件，不能替换掉 `cool.recovery`/`cool.error`。测试需要替换行为时，注入 `ErrorRenderer`/`ErrorLogger`，而不是移除核心保护。

新增可注入接口：

```go
type ErrorLogger interface {
    Log(ctx context.Context, resolved errors.Resolved, err error)
}

type ErrorRenderer interface {
    Write(r *ghttp.Request, resolved errors.Resolved)
}
```

默认实现使用 `g.Log()` 和 `response.Body`；测试可以使用 recorder。

## 15. 启动期校验

以下问题必须阻止启动：

1. Action 为 nil 或不是函数。
2. Action 输入/输出签名不受支持。
3. DTO 不是 struct 指针。
4. 自定义、CRUD、health 和其他 core route 的规范化 method + full path 重复。
5. Action 与 Result 类型不合法。
6. BindJSON 用于不支持 body 的 method。
7. CRUD api 不受支持或资源缺失。
8. middleware name 重复。
9. 模块 middleware 使用核心保留 order，或试图注册 `cool.*` 保留名称。

错误必须包含 controller name、route name、method 和 full path，便于模块作者直接定位。

## 16. 测试设计

### 16.1 Action 编译

1. 支持全部合法签名。
2. 非函数、nil、多个 DTO、DTO 非指针、错误返回位置错误均在启动时失败。
3. invoker 只编译一次。
4. route 错误包含完整定位信息。
5. 任一 compile error 发生时 Server 不包含部分 middleware 或 route 注册。
6. 同一路径不同 method 合法；IgnoreAuth 和 permission 均按 method + path 派生。
7. 重复斜杠、尾斜杠、非法 `..`、query/fragment 和不支持 method 的规范化行为锁定。

### 16.2 DTO 绑定

1. GET query、JSON、form、multipart 分别绑定正确。
2. GoFrame `v` tag 和 `Validate()` 均执行。
3. malformed JSON、未知字段和类型错误返回 code 1002。
4. `AllowUnknownFields` 显式放行。
5. 指针字段保留 absent 与 zero value 区别。
6. 上传文件无需 action 读取 raw request。
7. JSON Content-Type 缺失/错误返回 415，body 超限返回 413。
8. JSON 重复字段、尾随 value 和 `application/*+json` 行为锁定。
9. 每个绑定分支只执行一次 GoFrame `v` validation。

### 16.3 Result

1. data、零值、nil、error-only 的 JSON 字节与 Node 协议一致。
2. HTML、File、Stream、Redirect 不被 JSON 包装。
3. Result 写入失败进入 error boundary。
4. action 返回错误时绝不写 success body。
5. nil interface、typed nil data 和 typed nil Result 行为固定。
6. Stream Closer、HEAD、204、redirect header 注入和文件提交前失败行为固定。

### 16.4 Error boundary

1. comm、validate、core、401、403 映射正确。
2. internal 和 panic 不泄露内部文本。
3. `gerror.Wrap` stack 被 logger 接收。
4. 每个错误只记录一次。
5. 已提交响应不追加第二份 body。
6. 没有 base 模块时仍能处理错误。
7. translate 能处理 error boundary 生成的 message。
8. translate 或外层 middleware panic 由 `cool.recovery` 兜底且只记录一次。
9. 未 flush buffer 可被错误响应覆盖，已 flush/raw/hijack 响应不可覆盖。
10. SessionStore/PermissionService 故障返回 internal，不返回 401/403。

### 16.5 CRUD

1. 六个 CRUD route 全部复用统一 result/error pipeline。
2. CRUD override service 的 public/internal error 分类正确。
3. 请求解析失败返回 1002，不进入 Runtime。
4. permission metadata 和路由地址不改变。

### 16.6 模块回归

1. base 和 dict 所有现有 endpoint 的 method/path/EPS 不变。
2. controller 中不存在 `g.Log()`。
3. 普通 controller 中不存在 `r.Response.WriteJson()`。
4. 普通 controller 中不存在 `response.OK/Fail()`。
5. 登录、刷新、上传、HTML、字典、菜单导入导出响应锁定。

## 17. 分阶段实施

### 阶段 A：错误基础设施

1. 增加 1003 core code、typed gcode detail 和 resolver。
2. 盘点并分类 base、dict、CRUD 的全部 public/internal errors。
3. authority/permission 改为设置 typed error，并区分业务否定与依赖故障。
4. 将 translate 缩减为纯翻译。
5. 分类迁移完成后，一次性启用 mandatory `cool.recovery` 和 `cool.error`。

### 阶段 B：Action Runtime

1. 增加 Action 签名编译器。
2. 增加 DTO binder/validator。
3. 增加统一 success writer 和 Result。
4. `RouteOptions` 从 Handler 切换到 Action。
5. 增加启动期 route/action 校验。

### 阶段 C：CRUD 统一

1. 将 CRUD HTTP binding 移到 controller。
2. 复用 action result/error pipeline。
3. 删除 CRUD 内重复响应函数。

### 阶段 D：模块迁移

1. 先迁移 dict，验证简单 action。
2. 迁移 base app/comm/open。
3. 迁移 base sys custom actions。
4. 删除 controller 字符串错误 switch，并检查迁移期间没有新增裸 public error。

### 阶段 E：删除 legacy

1. 删除 Handler metadata 和 legacy adapter。
2. 删除 controller 手工 JSON/parser helpers。
3. 删除 base translate 的异常职责。
4. 增加静态 `rg` 回归检查。

每个阶段必须保持 `go test -p 1 ./...` 通过，不能长期保留两套对外错误协议。

## 18. 验收标准

1. 新增普通自定义接口只需要 DTO、service 方法和一个 Action 声明。
2. controller 不重复写日志、解析错误和通用响应。
3. 标准 CRUD 继续零 handler 开发。
4. 所有不支持的 Action 在启动时失败，不在首个请求时暴露。
5. 业务错误可公开，内部错误永不泄露。
6. authority、permission、CRUD、自定义 action 使用同一错误响应协议。
7. base translate 只负责翻译，cool.recovery/cool.error 独立于模块存在。
8. base、dict 的 route、permission、EPS 与 Node 约定保持一致。
9. `Handler:` 从模块 controller metadata 中完全移除。
10. 全仓测试、vet 和格式检查通过。
