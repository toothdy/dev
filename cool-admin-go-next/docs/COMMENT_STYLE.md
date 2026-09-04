# 注释规范

> 本规范适用于 `cool-admin-go-next` 仓库的所有 Go 代码。

## 核心原则

注释应当**补充信息**,而不是**重述标识符**。

标识符本身的拼写、类型签名、调用现场已经告诉读者大部分信息;注释存在的唯一理由,是表达**代码读不出来**的内容——角色、意图、归类。任何把名字翻成中文、把动词堆成分号从句、把实现契约塞进注释的行为,都属于噪音。

## 写作规则

### 1. 行尾 vs 行首:按场景选位置

| 场景 | 位置 | 示例 |
|---|---|---|
| 常量 | 行尾 | `Success = 1000 // 操作成功` |
| 结构体字段 | 行尾 | `Name string // 异常类型名称` |
| 类型 / 函数文档 | 行首 | `// 业务异常基类型` + `type ...` |

理由:行尾注释节省垂直空间,值与注释左右对照,`gofmt` 自动对齐 `=` 和注释列;行首注释用于 godoc 必须的导出符号。

### 2. 不重述标识符

```go
// ❌ 错:把标识符翻成中文
// BaseException 定义基础业务异常。
type BaseException struct { ... }

// ❌ 错:标识符 + 动词
// Error 返回异常消息。
func (e *BaseException) Error() string { ... }

// ✅ 对:直接说是什么 / 做什么
// 业务异常基类型
type BaseException struct { ... }

// 返回异常消息
func (e *BaseException) Error() string { ... }
```

### 3. 用名词短语,不用"xxx 是 sss"标签句

```go
// ❌ 错:系动词标签
// MsgSuccess 是操作成功消息。
// CommException 是通用业务异常类型名称。

// ✅ 对:纯名词短语
MsgSuccess      = "success"          // 操作成功的消息
CommException   = "CoolCommException" // 通用业务异常
```

### 4. 注释末尾不带句号

短句不需要标点,光秃秃读起来更紧凑:

```go
// ❌ 错
// 返回异常消息。
func (e *BaseException) Error() string { ... }

// ✅ 对
// 返回异常消息
func (e *BaseException) Error() string { ... }
```

> 注意:这与 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments) "Comment Sentences" 的官方建议不同。官方建议 doc comment 以句号结尾以兼容 godoc 工具链;本规范在测试已确认 `golint` / `go vet` / `gofmt` 不会报警的前提下,省略句号以换取视觉简洁。如果后续接入强 lint,需要重新评估。

### 5. 不在注释里写实现细节

panic、nil 接收者、空字符串 fallback、cause 为 nil 早返回——这些是**实现契约**或**边界行为**,不是文档语义。写到注释里既冗余又容易随代码漂移。处理方式:

| 边界行为 | 落点 |
|---|---|
| panic | 专门的测试钉住(`TestConstructorsRejectMultipleStatusCodes` 断言 panic 消息) |
| nil 接收者安全 | Go 标准库惯例,无需说明 |
| 空字符串 fallback | 函数体 `if message == "" { ... }` 一眼可见 |
| cause-nil 早返回 | 同上,函数体有 `if cause == nil { return nil }` |

```go
// ❌ 错:把契约塞进注释
// WrapComm 包装通用业务异常；cause 为 nil 时返回 nil，否则传入多个 statusCode 时会 panic。
func WrapComm(cause error, message string, statusCode ...int) error { ... }

// ✅ 对:只说意图
// 包装通用业务异常
func WrapComm(cause error, message string, statusCode ...int) error { ... }
```

### 6. godoc 兼容性

虽然我们不写"标识符 + 动词"开头,但**保留行首注释**以兼容:

- IDE hover 文档
- pkg.go.dev 索引
- `go doc` 命令行输出

如果导出符号完全没有行首注释,`golint` / `staticcheck` 会报警。

## 完整示例

### 常量块(尾部注释)

```go
const (
    Success      = 1000 // 操作成功
    CommFail     = 1001 // 通用失败
    ValidateFail = 1002 // 参数校验失败
    CoreFail     = 1003 // 核心服务失败
)
```

### 结构体(行首文档 + 字段尾部注释)

```go
// 业务异常基类型
type BaseException struct {
    Name       string // 异常类型名称
    Code       int    // 业务错误码
    Message    string // 错误描述
    StatusCode int    // 响应状态码
    Cause      error  // 原始错误
}
```

### 函数(行首短句)

```go
// 返回异常消息
func (e *BaseException) Error() string { ... }

// 包装核心服务异常
func WrapCore(cause error, message string, statusCode ...int) error { ... }
```

## 反例对照

| 反例 | 为什么差 | 改法 |
|---|---|---|
| `// Success 表示操作成功。` | 标识符 + "表示"+ 句号 | `Success = 1000 // 操作成功` |
| `// BaseException 定义基础业务异常。` | 把名字翻成中文 | `// 业务异常基类型` |
| `// WrapCore 包装核心服务异常；cause 为 nil 时返回 nil，否则传入多个 statusCode 时会 panic。` | 把契约、panic 条件全塞进注释 | `// 包装核心服务异常` |
| `// Error 返回异常消息；接收者为 nil 时返回空字符串。` | 重述函数 + 加 nil 边界 | `// 返回异常消息` |
| `// MsgSuccess 是操作成功消息。` | 系动词标签 | `MsgSuccess = "success" // 操作成功消息` |

## 自检清单

写完注释后,过一遍:

- [ ] 没有"xxx 是 sss" / "xxx 表示 sss"的标签句
- [ ] 没有把标识符翻译成中文
- [ ] 没有 panic / nil / 空字符串 fallback 等实现细节
- [ ] 行尾 / 行首的位置选对(常量/字段 → 行尾,类型/函数 → 行首)
- [ ] 末尾没有句号
- [ ] 导出符号都有行首注释(避免 golint 报警)

## 参考

- [Go Code Review Comments - Comment Sentences](https://github.com/golang/go/wiki/CodeReviewComments)
- [Go Doc Comment Spec (go.dev/doc/comment)](https://go.dev/doc/comment)
- [Effective Go - Commentary](https://go.dev/doc/effective_go)