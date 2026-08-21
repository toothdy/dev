# 模块 02：核心异常模型实施计划

> 状态：completed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `cool-next/core/exception` 中交付协议无关的 Cool 异常类型、固定业务码、构造器、错误包装、错误链和 GoFrame 堆栈能力。

**Architecture:** `BaseException` 保存可公开的分类信息和内部 Cause，`Error()` 只返回安全业务消息，`Unwrap()` 接入标准库错误链。公共构造器统一经过私有构建函数，并用 GoFrame `gerror.WrapCodeSkip(gcode.CodeNil, 2, ...)` 在业务调用点记录堆栈（等价于 `WrapSkip` 因为 `BaseException` 不实现 `gcode.Code`）；HTTP 和 gRPC 映射留给模块 33、43。

**Tech Stack:** Go 1.26、GoFrame v2.10.2 `gerror`、标准库 `errors`、`testing`

---

## 1. 事实来源与边界

- `docs/superpowers/specs/2026-07-31-cool-admin-go-next-architecture-design.md` 第 8 节；
- `docs/superpowers/specs/2026-08-03-cool-admin-go-next-module-decomposition-design.md` 的 02.1-02.7；
- Node 兼容来源：`cool-admin-midway-packages/core/src/exception`；
- 不实现 HTTP Filter、gRPC Code、日志组件、Trace、配置加载或新的异常分类；
- 空消息回退到固定默认消息；`statusCode` 省略时为 0，传入两个以上值视为调用方编程错误并 panic；
- `Wrap*` 的 Cause 为 `nil` 时返回 `nil`，与 GoFrame `gerror.Wrap` 语义一致；
- 未知 `error` 不伪装成 Cool 异常，后续协议层通过 `errors.As` 区分并使用安全默认常量。

## 2. 文件结构

| 文件 | 职责 |
|---|---|
| `cool-next/core/exception/code.go` | 固定业务码、默认消息和异常名称 |
| `cool-next/core/exception/exception.go` | `BaseException`、构造器、Wrap 和私有构建逻辑 |
| `cool-next/core/exception/exception_test.go` | 分类、默认值、状态码、错误链、敏感信息和堆栈验收 |

## 3. 任务清单

### 任务 1：错误码与基础异常

**Files:**
- Create: `cool-next/core/exception/code.go`
- Create: `cool-next/core/exception/exception.go`
- Test: `cool-next/core/exception/exception_test.go`

- [x] **Step 1: 写失败测试**

测试固定常量、`BaseException.Error()`、`Unwrap()` 和 nil receiver：

```go
func TestBaseException(t *testing.T) {
   cause := errors.New("database unavailable")
   exception := &BaseException{
      Name:       "CoolCoreException",
      Code:       CoreFail,
      Message:    "core fail",
      StatusCode: 503,
      Cause:      cause,
   }

   if exception.Error() != MsgCoreFail {
      t.Fatalf("Error() = %q, want %q", exception.Error(), MsgCoreFail)
   }
   if !errors.Is(exception, cause) {
      t.Fatal("BaseException 未保留 Cause")
   }

   var nilException *BaseException
   if nilException.Error() != "" || nilException.Unwrap() != nil {
      t.Fatal("nil BaseException 必须安全返回零值")
   }
}
```

- [x] **Step 2: 确认测试失败**

Run: `go test ./cool-next/core/exception -run TestBaseException -count=1`

Expected: FAIL，包或 `BaseException` 尚不存在。

- [x] **Step 3: 实现固定常量和基础类型**

`code.go`：

```go
package exception

const (
   Success      = 1000
   CommFail     = 1001
   ValidateFail = 1002
   CoreFail     = 1003
)

const (
   MsgSuccess      = "success"
   MsgCommFail     = "comm fail"
   MsgValidateFail = "validate fail"
   MsgCoreFail     = "core fail"
)

const (
   CommException     = "CoolCommException"
   ValidateException = "CoolValidateException"
   CoreException     = "CoolCoreException"
)
```

`exception.go` 首段：

```go
package exception

type BaseException struct {
   Name       string
   Code       int
   Message    string
   StatusCode int
   Cause      error
}

func (exception *BaseException) Error() string {
   if exception == nil {
      return ""
   }
   return exception.Message
}

func (exception *BaseException) Unwrap() error {
   if exception == nil {
      return nil
   }
   return exception.Cause
}
```

- [x] **Step 4: 确认测试通过**

Run: `go test ./cool-next/core/exception -run TestBaseException -count=1`

Expected: PASS。

### 任务 2：Comm、Validate、Core 构造器

**Files:**
- Modify: `cool-next/core/exception/exception.go`
- Test: `cool-next/core/exception/exception_test.go`

- [x] **Step 1: 写构造器表驱动失败测试**

覆盖三类名称、业务码、消息、默认消息、显式状态码和 GoFrame 堆栈，并使用 `errors.As` 取得 `BaseException`：

```go
func TestConstructors(t *testing.T) {
   tests := []struct {
      name       string
      create     func() error
      wantName   string
      wantCode   int
      wantMessage string
      wantStatus int
   }{
      {"comm", func() error { return Comm("业务异常") }, CommException, CommFail, "业务异常", 0},
      {"validate", func() error { return Validate("") }, ValidateException, ValidateFail, MsgValidateFail, 0},
      {"core", func() error { return Core("框架异常", 503) }, CoreException, CoreFail, "框架异常", 503},
   }

   for _, test := range tests {
      t.Run(test.name, func(t *testing.T) {
         err := test.create()
         var exception *BaseException
         if !errors.As(err, &exception) {
            t.Fatalf("%T 不是 BaseException", err)
         }
         if exception.Name != test.wantName || exception.Code != test.wantCode ||
            exception.Message != test.wantMessage || exception.StatusCode != test.wantStatus {
            t.Fatalf("异常内容错误: %#v", exception)
         }
         if !gerror.HasStack(err) {
            t.Fatal("异常缺少 GoFrame 堆栈")
         }
      })
   }
}
```

再增加 `TestConstructorsRejectMultipleStatusCodes`，断言 `Comm("失败", 400, 500)` panic。

- [x] **Step 2: 确认构造器测试失败**

Run: `go test ./cool-next/core/exception -run 'TestConstructors' -count=1`

Expected: FAIL，构造器尚不存在。

- [x] **Step 3: 实现构造器和私有构建函数**

```go
func Comm(message string, statusCode ...int) error {
   return newException(CommException, CommFail, message, MsgCommFail, nil, statusCode)
}

func Validate(message string, statusCode ...int) error {
   return newException(ValidateException, ValidateFail, message, MsgValidateFail, nil, statusCode)
}

func Core(message string, statusCode ...int) error {
   return newException(CoreException, CoreFail, message, MsgCoreFail, nil, statusCode)
}

func newException(name string, code int, message string, fallback string, cause error, statusCode []int) error {
   if message == "" {
      message = fallback
   }
   exception := &BaseException{
      Name:       name,
      Code:       code,
      Message:    message,
      StatusCode: getStatusCode(statusCode),
      Cause:      cause,
   }
   return gerror.WrapCodeSkip(gcode.CodeNil, 2, exception)
}

func getStatusCode(statusCode []int) int {
   if len(statusCode) > 1 {
      panic("exception: statusCode 只能省略或传入一个值")
   }
   if len(statusCode) == 1 {
      return statusCode[0]
   }
   return 0
}
```

- [x] **Step 4: 确认构造器测试通过**

Run: `go test ./cool-next/core/exception -run 'TestConstructors' -count=1`

Expected: PASS。

### 任务 3：Wrap、错误链与安全消息

**Files:**
- Modify: `cool-next/core/exception/exception.go`
- Test: `cool-next/core/exception/exception_test.go`

- [x] **Step 1: 写 Wrap 失败测试**

表驱动覆盖三类 Wrap，断言：

```go
cause := errors.New("sql: password=secret")
err := WrapCore(cause, "读取数据失败")

var exception *BaseException
if !errors.As(err, &exception) {
   t.Fatal("WrapCore 未生成 BaseException")
}
if !errors.Is(err, cause) || exception.Cause != cause {
   t.Fatal("WrapCore 未保留原始 Cause")
}
if err.Error() != "读取数据失败" || strings.Contains(err.Error(), "password=secret") {
   t.Fatalf("公开错误消息泄漏内部原因: %q", err.Error())
}
if !gerror.HasStack(err) {
   t.Fatal("WrapCore 缺少 GoFrame 堆栈")
}
```

增加 `TestWrapNilCauseReturnsNil`，断言三个 `Wrap*` 在 Cause 为 nil 时全部返回 nil；增加未知标准库错误的 `errors.As` 反向测试，证明未知错误不会被误分类。

- [x] **Step 2: 确认 Wrap 测试失败**

Run: `go test ./cool-next/core/exception -run 'TestWrap|TestUnknown' -count=1`

Expected: FAIL，Wrap 函数尚不存在。

- [x] **Step 3: 实现三个 Wrap 函数**

```go
func WrapComm(cause error, message string, statusCode ...int) error {
   if cause == nil {
      return nil
   }
   return newException(CommException, CommFail, message, MsgCommFail, cause, statusCode)
}

func WrapValidate(cause error, message string, statusCode ...int) error {
   if cause == nil {
      return nil
   }
   return newException(ValidateException, ValidateFail, message, MsgValidateFail, cause, statusCode)
}

func WrapCore(cause error, message string, statusCode ...int) error {
   if cause == nil {
      return nil
   }
   return newException(CoreException, CoreFail, message, MsgCoreFail, cause, statusCode)
}
```

- [x] **Step 4: 确认全部包测试通过**

Run: `go test ./cool-next/core/exception -count=1 -v`

Expected: PASS，且公开消息不包含测试 Cause 中的敏感片段。

### 任务 4：架构与质量门禁

**Files:**
- Modify: `docs/superpowers/plans/2026-08-03-02-core-exception-model-implementation-plan.md`

- [x] **Step 1: 格式化实现文件**

Run: `gofmt -w cool-next/core/exception/code.go cool-next/core/exception/exception.go cool-next/core/exception/exception_test.go`

Expected: 命令成功且 `gofmt -l cool-next/core/exception` 无输出。

- [x] **Step 2: 执行模块 02 定向验收**

Run: `go test ./cool-next/core/exception -count=1 -v`

Expected: 所有分类、默认值、状态码、Cause、错误链、敏感信息和堆栈测试通过。

- [x] **Step 3: 执行仓库快速门禁**

Run: `make check`

Expected: Module、格式、vet、架构守卫和全部单测通过；真实架构检查不再报告 `skeleton-only`。

- [x] **Step 4: 执行 Race Test**

Run: `make test-race`

Expected: PASS。

- [x] **Step 5: 检查范围并更新计划状态**

Run: `find cool-next -type f -print && rg -n 'ghttp|grpc|http\.Status' cool-next/core/exception || true`

Expected: 只有异常包文件，没有引入 HTTP、gRPC、配置、数据库或业务模块依赖。全部验收通过后将本文状态记录为 `completed`。

## 4. 完成标准

- `BaseException` 的字段、`Error()` 和 `Unwrap()` 与上位设计一致；
- 三类构造器和三类 Wrap 的名称、业务码、默认消息与状态码一致；
- `errors.As` 能识别 Cool 异常，`errors.Is` 能到达原始 Cause；
- `gerror.HasStack` 为 true，公开 `Error()` 不包含 Cause；
- 未知标准库错误不会被识别为 Cool 异常；
- 包不依赖 HTTP、gRPC、数据库、配置或 `modules/**`；
- `make check` 与 `make test-race` 通过。

## 5. 实施结果

- 完成日期：2026-08-03。
- 交付文件：`cool-next/core/exception/code.go`、`exception.go`、`exception_test.go`；已删除不再需要的 `cool-next/.gitkeep`。
- 验收证据：`gofmt -l` 无输出；异常包定向测试、`make check`、`TestRepositoryBoundaries` 和 `make test-race` 均通过，架构检查不再报告 `skeleton-only`。
- 安全回归：覆盖带 GoFrame code 的敏感 Cause，确认公开 `Error()` 不泄漏内部 code message，同时 `errors.Is` 与 `gerror.HasCode` 仍可沿错误链访问 Cause。
- 范围：仅交付协议无关的核心异常模型及测试，未引入 HTTP、gRPC、数据库、配置或 `modules/**` 依赖。
