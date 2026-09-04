# 权限自动推导设计

> 日期：2026-08-21
> 状态：已实现（commit 492b21b）
> 范围：后台权限标识的来源；不改变鉴权判定逻辑、不改变超管判定、不动 App 侧鉴权

## 1. 结论

后台权限标识由最终路由路径推导，业务源码不再声明任何权限。

```text
业务源码 Controller（无权限声明）
        ↓
codegen 只渲染路径与 ignoreToken 标签
        ↓
apphttp.NewContextMiddleware(authService, "/admin/base/sys/user/move", ignoreToken)
        ↓
构造时调用 auth.DerivePermission 推导一次，推导失败即启动失败
        ↓
AuthenticateHTTP → 空权限只校验登录 / 非空走 Authorize
        ↓
PermissionService.Authorize → 超管旁路 → 菜单权限集合比对
```

采用 Node 的设计、Go 的管道：可观察行为与 `cool-admin-midway` 完全一致，但保留 Go next 已有的每路由中间件绑定，不引入每请求 URL 解析。

## 2. 背景与根因

`cool-admin-midway` 的 `@cool-midway/core` 中不存在权限概念（grep `perms|permission` 零结果）。权限标识只以一处形态存在：`base_sys_menu.perms` 字段，由管理员在菜单管理界面填写。

`BaseAuthorityMiddleware`（`cool-admin-midway/src/modules/base/config.ts:18` 注册为全局中间件）在请求期反向比对：

```js
perms = perms.map(e => e.replace(/:/g, '/'));
if (!perms.includes(url.split('?')[0].replace('/admin/', ''))) statusCode = 403;
```

即 Node 从不回答"这条路由需要什么权限"，只回答"这个 URL 是否已被授权"。

Go v1 与当前 Go next 只对 CRUD 路由自动生成权限（`codegen/route_analysis.go:207` 的 `defaultCRUDPermissionPrefix`），自定义路由仍需手写 `Route.Permission`。核对结果：仓库现有 13 处手写值与路径推导值**逐条相同**，`CurdOption.PermissionPrefix` 在 `modules/` 下一次都未使用。这些是纯重复信息，且每新增一条自定义路由都要求作者手工重复一次。

## 3. 目标

1. 后台受保护路由的权限标识由最终路径推导；
2. 业务源码不再出现任何权限声明；
3. 保留 `ignoreToken`、`/comm/`、App 路由的既有豁免语义；
4. 删除因手写权限而存在的字段、校验与重复值；
5. 对现有任何用户，鉴权结果保持不变；
6. 不新增第三方依赖。

## 4. 非目标

1. 不引入权限缓存。Node 有 `admin:perms:${userId}`（`cool-admin-midway` 的 `service/sys/login.ts:94` 写、`service/sys/perms.ts:44` 写、`login.ts:159` 删），Go 每请求查库。加缓存需连带处理授权变更的失效点，属独立改动。
2. 不实现 Node 的 `/admin/dict/info/data` 硬编码特例，dict 模块尚未迁移。需要记录其后果：Node 中间件把该 URL 硬编码为只校验登录，而 `modules/base/menu.json` 中同时存在 `dict:info:data` 权限码。本设计的推导会得出 `dict:info:data` 并实际比对该权限码，因此 dict 模块迁移后，未被授予该菜单的角色会收到 403，而 Node 放行。这是一处**已知的、迁移时必须重新决策**的行为差异，不是遗漏。
3. 不改超管判定。Go 使用 `role.label == "admin"`（`service/permission.go:15`），比 Node 硬编码 `username == 'admin'` 与 `getPerms` 里的 `roleIds.includes(1)` 干净，保持不变。
4. 不改 App 侧鉴权。
5. 不改 gRPC 鉴权。

## 5. 方案比较

### 5.1 中间件构造期推导，采用

HTTP 中间件在构造时按自己的静态路径推导一次。业务源码、codegen 产物中均不再出现权限标识。

优点：与 Node 可观察行为逐字节一致；保留 Go 的每路由绑定，不引入每请求 URL 解析；`auth.Rule` 与 `AuthenticateHTTP` 签名不变；顺带清掉 EPS 重构后将无消费方的 `coreroute.Route.Permission`（与第 9 节保留的 `auth.Rule.Permission` 是不同类型，勿混）。

### 5.2 codegen 期静态推导，拒绝

在 `route_analysis.go` 推导后渲染进 `modules_gen.go`。

优点：权限标识成为可 diff 的静态产物，写错在 `go generate` 期即失败。

拒绝理由：保留了 Node 不存在的"路由声明权限"这一中间概念，与"按 Node 的设计"的目标不符。

### 5.3 `coreroute.New` 构建期填充，拒绝

不成立。鉴权使用的 `auth.Rule` 是 `codegen/render.go:1895` 直接渲染的字面量，不经过 `coreroute.New`；该处填充只能影响 Graph 一支。要生效必须再改 render 从 Graph 反向读取，形成 codegen 与 Graph 的反向耦合。

## 6. 推导规则

`auth` 包新增纯函数：

```go
func DerivePermission(fullPath string, ignoreToken bool) (string, error)
```

判定顺序：

| # | 条件 | 结果 |
| --- | --- | --- |
| 1 | `ignoreToken == true` | `""` |
| 2 | 路径不以 `/admin/` 开头 | `""` |
| 3 | 去掉 `/admin/` 后存在某段等于 `comm` 且该段不是最后一段 | `""` |
| 4 | 任一段不匹配 `^[A-Za-z_][A-Za-z0-9_]*$` | error，启动失败 |
| 5 | 其余 | 各段以 `:` 连接 |

空串统一表示"只校验登录"，复用 `auth/service.go:129` 的既有语义。

第 4 条**不得使用 `go/token.IsIdentifier`**。该函数拒绝 Go 关键字，而真实路由中存在关键字段：当前仓库的 `/admin/base/sys/menu/import`，以及 Node 全量集中的 `/admin/dict/type/*` 与 `/admin/space/type/*`（共 12 条）。权限标识只作为 map 键与字符串字面量使用，从不成为 Go 标识符，因此按字符形状校验即可。

现有 `codegen/route_analysis.go:207` 的 `defaultCRUDPermissionPrefix` 与 `core/route/route.go:509`、`core/controller/controller.go:398` 的 `validatePermission` 均使用 `token.IsIdentifier`，属既有过度约束——当前未暴露仅因带关键字段的路由恰好都没有权限标识。这三处随第 8 节一并删除或替换。

第 3 条的"不是最后一段"与 Node 正则 `^/admin/?.*/comm/` 精确等价——该正则要求 `/comm/` 带尾斜杠，因此 `comm` 之后必须还有内容。当前无此边界路由，规则按 Node 写死以免后续分歧。

推导与 HTTP Method 无关。同路径不同 Method 得到同一权限标识，与 Node 一致。

### 6.1 与 Node 的一处有意偏离

含路径参数的 `/admin/` 路由，Node 会静默永久 403（字面比对不可能匹配 `{date}`），本设计第 4 条改为启动失败。启动期报错优于静默 403。

该规则不会误伤：对 Node 完整 `/admin` EPS 的 118 条路由执行推导，失败 0 条（见 7.2）。这一验证覆盖了 dict、plugin、recycle、space、task、user、demo 等 Go 尚未迁移的模块，因此后续模块迁移也不会触发此路径。

### 6.2 顺带修正的潜在分歧

现有 `defaultCRUDPermissionPrefix` 的做法是**剥离** `admin`/`app` 前缀后拼接剩余段，而非**以** `/admin/` 为准入条件。这带来两处与 Node 不一致的行为：

1. App 侧 CRUD 控制器会得到真实权限标识并被强制校验，而 Node 的 `UserMiddleware` 对 `/app/` 从不做权限比对；
2. 使用 `IgnoreGlobalPrefix` 让路径脱离 `/admin/` 与 `/app/` 的控制器，其全部路径段会被直接拼成权限标识（例如 `/upload` 得到 `upload:add`），而 Node 的 `BaseAuthorityMiddleware` 对非 `/admin/` 路径完全放行。

当前仓库既无 App CRUD 控制器，也无脱离前缀的 CRUD 控制器，故两者均未暴露。新规则第 2 条以 `/admin/` 前缀为准入条件，同时堵住这两个口。

## 7. 全路由验证

对仓库现有 57 条路由逐条比对，下表列出覆盖各条推导规则的代表性结果，完整比对由第 12 节验证第 2 条的回归测试锁定：

| 路径 | 推导结果 | 现状 | 判定 |
| --- | --- | --- | --- |
| `/admin/base/sys/user/move` | `base:sys:user:move` | 手写同值 | 一致 |
| `/admin/base/sys/log/getKeep` | `base:sys:log:getKeep` | 手写同值 | 一致 |
| `/admin/base/sys/department/add` | `base:sys:department:add` | CRUD 自动同值 | 一致 |
| `/admin/base/comm/person` | `""` | `""` | 一致 |
| `/admin/base/open/captcha` | `""`（ignoreToken） | `""` | 一致 |
| `/app/base/comm/upload` | `""`（非 admin） | `""` | 一致 |
| `/upload/{date}/{name}` | `""`（非 admin） | `""` | 一致 |
| `/admin/base/coding/createCode` | `base:coding:createCode` | `""` + `requireAdmin` | 见 7.1 |
| `/admin/base/coding/getModuleTree` | `base:coding:getModuleTree` | `""` + `requireAdmin` | 见 7.1 |
| `/admin/base/sys/menu/parse` | `base:sys:menu:parse` | `""` + `requireAdmin` | 见 7.1 |
| `/admin/base/sys/menu/create` | `base:sys:menu:create` | `""` + `requireAdmin` | 见 7.1 |
| `/admin/base/sys/menu/export` | `base:sys:menu:export` | `""` + `requireAdmin` | 见 7.1 |
| `/admin/base/sys/menu/import` | `base:sys:menu:import` | `""` + `requireAdmin` | 见 7.1 |

13 处手写值与全部 CRUD 路由推导结果逐条相同，确认为可删除的重复信息。

### 7.2 Node 全量路由集验证

以 Node 后端 `/admin/base/open/eps` 的实际返回（118 条 `/admin` 路由，覆盖 base、demo、dict、plugin、recycle、space、task、user 全部模块）执行推导：

| 项 | 数量 |
| --- | --- |
| 路由总数 | 118 |
| 推导失败 | 0 |
| 权限码为空 | 14（comm 豁免 7、ignoreToken 7） |
| 推导出权限码 | 104 |
| 与 `modules/base/menu.json` 逐字符命中 | 82 |

82 条独立权限码的路径推导值与管理员手填的菜单权限码完全相同，是推导规则正确性的直接证据。

### 7.3 两个差集

**`menu.json` 中存在、Node 无对应路由的 4 条**：

| 权限码 | 原因 |
| --- | --- |
| `base:sys:param:list` | param 控制器无 `/list` 接口 |
| `space:info:getConfig` | 无对应路由 |
| `task:info:list` | task 控制器无 `/list` 接口 |
| `plugin:info:install` | 路由存在但标记 `ignoreToken: true`，永不进入权限比对 |

这四条权限码永远不生效，Node 不产生任何报错。这是第 2 节所述"缺少编译期导致静默失效"的实证。本设计不改变这一点——菜单侧权限码仍由管理员自由填写，推导只作用于路由侧。

**Node 有路由、`menu.json` 无权限码的 22 条**，分三类：

- `base:coding:*`（2 条）与 `base:sys:menu:{create,export,import,parse}`（4 条）：与 7.1 所列六条工具路由完全吻合；
- `base:sys:param:html`（1 条）：Go 现有手写值与推导值相同，Node 菜单侧同样缺失，两边均只有超管可用，改动后行为一致；
- `demo:*`（9 条）与 `user:address:*`（6 条）：示例模块与 Go 尚未迁移的模块。

### 7.1 六条工具路由

这六条当前权限标识为空（仅校验登录），改为在 handler 内调用 `permission.IsAdmin` 做超管校验（`controller/admin/coding.go:72`、`controller/admin/sys/menu.go:165`）。

Node 对应的 `AdminCodingController` 与 `BaseSysMenuController` 的 `parse`/`create`/`export`/`import` 无任何守卫，依靠推导出的权限标识不在任何菜单记录中来挡住非超管。

对现有任何用户，本次改动后行为一致：

- 普通角色：现在被 `requireAdmin` 挡下 403；改后因 `modules/base/menu.json` 无对应权限标识同样 403。
- 超管：现在经 `requireAdmin` 放行；改后经 `PermissionService.Authorize` 的超管旁路放行。

实质差别是管理员从此可在后台菜单中把这些接口授权给其他角色。`requireAdmin` 把这条路径堵死，是在"当前无权限标识"这一临时状态下打的补丁，随本次改动移除。

## 8. 删除清单

### 8.1 字段与业务值

- `core/controller.Route.Permission` 及业务源码中 13 处手写值；
- `core/controller.CurdOption.PermissionPrefix`；
- `core/route.RouteDefinition.Permission` 与 `Route.permission`；
- `codegen/route_analysis.go` 中读取手写 `Permission`（第 282 行）与 `PermissionPrefix`（第 158 行）的分支；
- `controller/admin/coding.go` 与 `controller/admin/sys/menu.go` 中的 `requireAdmin` 及其 `adminRoleChecker`/`menuAdminChecker` 依赖。

### 8.2 因手写权限而存在的校验

| 位置 | 内容 |
| --- | --- |
| `core/route/route.go:367` | ignoreToken 与 Permission 冲突校验 |
| `core/route/route.go:509` | `validatePermission` |
| `core/controller/controller.go:307` | ignoreToken 与 Permission 冲突 panic |
| `core/controller/controller.go:398` | `validatePermission` |
| `core/app/http/context.go:24` | Rule 的 IgnoreToken/Permission 冲突校验 |

这些校验的前提是作者可能写错权限标识；推导之后无从写错。

`grpc/context.go:99` 的同类校验**保留**：该处 `Rule` 由调用方经 `RuleResolver` 提供。

### 8.3 连带影响

`eps/eps.go:576` 是 `Route.Permission()` 的全仓库唯一消费方，随字段一并删除。这与 EPS 对齐设计第 7.2 节"不输出内部 Bind、Permission、完整路径"的要求一致，两份设计不冲突。

## 9. 保留边界

`auth.Rule.Permission` **不删除**。`grpc/context.go:21` 的 `RuleResolver` 由调用方提供，gRPC 权限不来自 `Route.Permission`，且 gRPC 无 `/admin/` 路径约定、无从推导。删除该字段等于让 gRPC 失去权限能力。

因此本次只改 HTTP 侧入口签名：

```go
apphttp.NewContextMiddleware(authenticator Authenticator, requestPath string, ignoreToken bool)
```

中间件构造时调用 `DerivePermission` 得到权限标识，再组装内部 `Rule`。`auth.Rule` 与 `AuthenticateHTTP` 的签名保持不变。

`modules_gen.go:1292` 的 gRPC registrar 目前 `return nil`，仓库无 gRPC 服务，该链路本次仅保留能力。

## 10. 文件边界

- `cool-next/auth/`：新增 `DerivePermission` 纯函数及其测试；
- `cool-next/core/app/http/context.go`：入口签名改为接收 `ignoreToken`，内部推导；
- `cool-next/codegen/route_analysis.go`：删除两处读取分支；
- `cool-next/codegen/render.go`：不再渲染 `Permission`；
- `cool-next/core/route/route.go`、`cool-next/core/controller/controller.go`：删除字段与校验；
- `cool-next/eps/eps.go`：删除 `Permission` 输出；
- `modules/base/controller/admin/`：删除手写权限值与 `requireAdmin`。

不新增 Provider、Registry、工厂或配置开关。

## 11. 错误处理

推导失败时 `NewContextMiddleware` 返回 error，生成代码中已有的 `if err != nil { return exception.WrapCore(...) }` 使启动失败。不新增错误类型，复用 `exception.Core`。

请求期不做推导、不做降级猜测。

## 12. 验证

1. **`DerivePermission` 表驱动测试**：admin CRUD、admin 自定义、`comm` 位于中段、`comm` 位于末段、`ignoreToken`、`/app/`、非法路径段、含路径参数。
2. **全路由回归测试**：以 `generatedGraph()` 遍历真实 57 条路由，断言推导结果与第 7 节表格逐条相同。这是本次改动唯一的安全网，必须先落地并通过，再执行第 8 节的删除。
3. **鉴权行为测试**：超管旁路、普通角色命中菜单权限返回 200、未命中返回 403、`/comm/` 只校验登录、`ignoreToken` 放行、App 路由不校验权限。
4. **codegen 测试**：断言 `modules_gen.go` 中不再出现 `Permission:` 字面量。
5. `go build ./...`、`go vet ./...`、`go test ./...` 全部通过。

## 13. 验收标准

1. 业务源码中不存在任何权限声明；
2. 新增自定义路由无需编写权限标识；
3. 对现有任何用户，鉴权结果与改动前逐例一致；
4. `ignoreToken`、`/comm/`、App 路由豁免语义不变；
5. 第 8 节所列字段、校验与重复值全部删除；
6. `auth.Rule` 与 `AuthenticateHTTP` 签名未变，gRPC 鉴权能力未受影响；
7. 无新增第三方依赖。

## 14. 与 EPS 对齐设计的关系

EPS 对齐设计（`2026-08-21-node-eps-alignment-design.md`）第 12 节将权限自动推导记为独立后续改动，并要求单独提交、单独验证。本设计遵守该约定：本次不修改 EPS 契约形状，仅删除 `eps.go:576` 一行 `Permission` 输出。

两份改动的实施顺序不构成依赖。本设计先行落地时，EPS 重构后续无需再处理 `Route.Permission`。

## 15. 实施结果

实现见 commit `492b21b`。第 8 节删除清单全部执行，第 6 节推导规则以 `cool-next/auth/permission.go` 的 `DerivePermission` 落地，HTTP 入口按第 9 节改为 `NewContextMiddleware(authenticator, requestPath, ignoreToken)`。

### 15.1 实施中修正的设计错误

第 6 节规则 4 原写"任一段不是合法 Go 标识符"。按字面使用 `go/token.IsIdentifier` 实现会使当前仓库启动失败——该函数拒绝 Go 关键字，而 `/admin/base/sys/menu/import` 的 `import` 段正是关键字，Node 全量集中 `/admin/dict/type/*` 与 `/admin/space/type/*` 的 `type` 段同理，共 13 条。规则已改为字符形状校验并在第 6 节写明禁用 `token.IsIdentifier`。

第 7.2 节的 118 条验证使用正则 `[A-Za-z_][A-Za-z0-9_]*`，结论本身正确；错误仅存在于第 6 节的措辞。

### 15.2 验证执行情况

第 12 节五项全部完成：

| 项 | 覆盖 |
| --- | --- |
| 1 `DerivePermission` 表驱动 | 23 例，含关键字段、comm 各位置、路径参数、非法字符 |
| 2 全路由回归 | 57 条，推导值与改动前快照逐条相同；快照不依赖已删除的 `Route.Permission()` |
| 3 鉴权行为 — 传递链 | 真实 `ghttp` server 端到端，确认推导值送达认证内核；comm、ignoreToken、App 三类均得到空权限标识 |
| 3 鉴权行为 — 判定结果 | sqlite 临时库上验证 `PermissionService.Authorize`：超管旁路未授权接口、普通角色命中菜单权限、未命中被拒、无角色被拒、App 身份被拒 |
| 4 codegen 断言 | `modules_gen.go` 零个 `Permission:` 字面量、零个 `auth.Rule` |
| 5 工具链 | `go build`、`go vet`、`go test ./...`、`gofmt` 全通过 |

第 3 项后半直接验证了第 7.1 节的核心主张：运维角色被授予 `base:coding:createCode` 后可以调用该接口——这是 `requireAdmin` 时代无法做到的；未获授权时仍被拒绝，与改动前一致。

### 15.3 测试不入库

仓库 `.gitignore:14` 排除 `*_test.go`，本次新增的四个测试文件均无法提交：

- `cool-next/auth/permission_test.go`
- `cool-next/core/app/http/permission_wiring_test.go`
- `modules/permission_regression_test.go`
- `modules/base/service/permission_authorize_test.go`

上述验证在本地全部通过，但不进入仓库，评审者无法查看或重跑，后续也不构成回归保护。是否调整该忽略规则属仓库约定变更，不在本设计范围内。

### 15.4 与本次改动无关的既有问题

以下两项在改动前即存在，`go.mod` 与 `go.sum` 本次未修改：

- `go mod tidy -diff` 报告 `google.golang.org/protobuf` 应为 indirect 依赖；
- `Makefile` 的 `check-architecture` 与 `test-integration` 指向的 `test/architecture`、`test/integration` 目录不存在。
