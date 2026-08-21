# 模块 34 Identity、Context 与密码边界设计

## 1. 范围

本模块只建立协议无关的已验证身份、授权调用边界和 bcrypt 密码适配器。Session、JWT、中间件、角色权限存储和超级管理员规则由后续模块实现。

代码归属固定为 `cool-next/auth` 和 `cool-next/auth/bcrypt`。

## 2. Identity

身份种类固定为 `AdminKind` 和 `AppKind`。Admin 与 App 使用独立值类型，不共享可产生无效字段组合的通用结构体：

```go
type Kind string

const (
   AdminKind Kind = "admin"
   AppKind   Kind = "app"
)

type AdminIdentity struct {
   UserID    uint64
   Username  string
   PasswordV int
   roleIDs   []uint64
}

func (identity AdminIdentity) RoleIDs() []uint64

type AppIdentity struct {
   ID uint64
}
```

只有 `auth` 包内的可信鉴权流程能构造包含私有角色数据的完整 Admin Identity，并将身份写入 Context。构造时复制角色切片，`RoleIDs` 每次再返回副本，调用方不能修改 Context 中保存的身份。Identity 不包含密码、Token、Session、租户或超级管理员标记。

## 3. Context

已验证身份使用标准 `context.Context` 传播。Context key、Identity 构造器和写入函数均保持包私有；后续 JWT 验证内核位于同一个 `auth` 包，通过私有入口写入。业务包只有以下读取入口：

```go
func Admin(context.Context) (AdminIdentity, error)
func App(context.Context) (AppIdentity, error)
```

Context 为 nil、未携带身份或 Subject 不匹配时，访问器返回安全的 `Comm` 异常和 HTTP 401，不返回可误用的零值，也不泄露另一类身份。Context 只保存一种已验证身份。

## 4. Authorizer

框架只定义协议无关的授权调用边界：

```go
type Authorization struct {
   Subject    Kind
   SubjectID  uint64
   Permission string
   Resource   string
}

type Authorizer interface {
   Authorize(context.Context, Authorization) (bool, error)
}

func Authorize(context.Context, Authorizer, permission, resource string) error
```

统一入口只接受权限和资源，并从私有 Context 中读取已验证身份后构造 `Authorization`，调用方不能提交任意 Subject 或 SubjectID。HTTP Adapter 将规范化的 `method + path` 作为 Resource，gRPC Adapter 使用完整 Method。`Permission` 和 `Resource` 必须非空，Context 中的 Subject 只允许 Admin/App，SubjectID 必须非零。

返回值语义固定如下：

- `true, nil`：允许访问；
- `false, nil`：明确权限不足，统一转换为安全 `Comm` 异常和 HTTP 403；
- 非 nil error：权限服务或存储失败，统一包装为安全 `Core` 异常并保留 cause。

框架不解释角色、菜单或超级管理员规则，也不根据用户名猜测绕过策略。业务模块实现 Authorizer，并以数据库权威关系决定授权结果。

## 5. bcrypt

`auth/bcrypt` 封装 `golang.org/x/crypto/bcrypt`，配置只有 Cost：

```go
type Config struct {
   Cost int
}

type Verifier struct {
   // 私有配置
}

type VerifyResult struct {
   Valid       bool
   NeedsRehash bool
}

func New(Config) (*Verifier, error)
func (verifier *Verifier) Hash(password string) (string, error)
func (verifier *Verifier) Verify(password, encoded string) (VerifyResult, error)
```

Cost 默认值和推荐值固定为 12，合法范围直接采用 bcrypt 的 4 到 31。Cost 为零时使用默认值；其他越界值在构造时返回 Core 异常。首期只接受 bcrypt 摘要，不兼容 MD5、明文或其他算法。

`Hash` 使用配置 Cost。超过 bcrypt 72 字节限制或生成失败时返回 Core 异常，不在错误或日志中包含明文密码。

`Verify` 的结果固定如下：

- bcrypt 摘要合法且密码匹配：`Valid=true`；
- 密码不匹配：`Valid=false` 且 error 为 nil；
- 摘要损坏、算法不支持或 Cost 非法：返回 Core 异常；
- 匹配成功且摘要 Cost 低于当前 Cost：`NeedsRehash=true`；
- 摘要 Cost 等于或高于当前 Cost：`NeedsRehash=false`，不得降级。

登录服务收到 `NeedsRehash=true` 后，在自己的登录事务内调用 `Hash` 并更新密码摘要。适配器不依赖数据库，也不自行写库。渐进式 Rehash 不修改 `PasswordV`；该字段只在用户主动修改或重置密码时递增，用于撤销旧登录态。

## 6. 安全边界

- 明文密码只作为 `Hash` 和 `Verify` 的瞬时参数，不进入 Identity、Context、Session、JWT 或日志；
- 任何 bcrypt 错误均不得拼接明文密码或完整摘要；
- 业务代码不能构造带私有角色数据的完整 Identity，也不能调用 Context 写入口；
- 缺失和错误 Subject 只返回统一未认证消息；
- 明确拒绝、无效凭证和基础设施失败分别映射为 HTTP 403、401 和 Core；
- 框架不提供超级管理员默认实现或字符串规则。

## 7. 文件职责

| 文件 | 职责 |
|---|---|
| `cool-next/auth/identity.go` | Subject、Admin/App Identity 和防御性副本 |
| `cool-next/auth/context.go` | 私有 Context 写入和公开身份访问器 |
| `cool-next/auth/authorizer.go` | 授权请求、接口和错误分类 |
| `cool-next/auth/bcrypt/bcrypt.go` | Cost 校验、Hash、Verify 和 Rehash 判断 |
| 对应 `_test.go` | 不可变性、错误分类、bcrypt 兼容和敏感信息测试 |

## 8. 验收

1. Admin/App Identity 正确读取，缺失、nil Context 和 Subject 不匹配稳定返回 HTTP 401；
2. 构造输入、`RoleIDs` 返回值的修改均不能改变已保存 Identity；
3. `auth` 包外不存在 Identity 构造或 Context 写入 API；
4. Authorizer 的允许、明确拒绝和基础设施失败分别得到 nil、HTTP 403 Comm 和 Core；
5. bcrypt Cost 12 可生成并验证摘要，错误密码是普通不匹配；
6. Cost 4 到 31 合法，零值取 12，其余越界配置拒绝；
7. 低 Cost 摘要匹配后要求 Rehash，同 Cost 和高 Cost 不要求且不降级；
8. 已有 bcrypt 摘要保持当前 bcrypt 库的解析兼容行为，非 bcrypt 或损坏摘要返回 Core；
9. 超过 72 字节密码失败，错误文本和异常日志不包含明文密码或完整摘要；
10. `go test ./cool-next/auth/... -count=1`、`go test -race ./cool-next/auth/... -count=1`、`go test ./... -count=1`、`go vet ./...` 和 `gofmt` 全部通过。
