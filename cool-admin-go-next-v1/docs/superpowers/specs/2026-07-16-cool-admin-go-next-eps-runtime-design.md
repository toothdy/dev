# cool-admin-go-next 阶段 6C EPS Runtime 设计

日期：2026-07-16

## 1. 背景

阶段 6A 设计了 controller/service 源码形态对齐，阶段 6B 设计了 metadata 驱动路由和权限。阶段 6C 在这些 metadata 基础上实现 EPS runtime，让现有 `cool-admin-vue` 前端启动时可以通过 `/admin/base/open/eps` 获取接口、字段和查询元数据。

协议文档已经记录前端 EPS bootstrap 行为：

1. 前端读取 EPS 响应中的 service/controller 信息。
2. 遍历接口 `api` 列表。
3. 根据 `method` 判断请求使用 `data` 还是 `params`。
4. 生成 `service.base.open.login`、`service.base.comm.permmenu`、`service.base.sys.user.page` 等方法。

阶段 6C 的目标是从 controller metadata 和 model metadata 生成 Node 兼容 EPS，避免手写 EPS JSON。EPS 采用匿名、全量 bootstrap 契约：登录前可访问，不按用户、角色或权限裁剪。

## 2. 目标

阶段 6C 完成后应满足：

1. 实现 `GET /admin/base/open/eps`。
2. EPS 数据从 controller metadata 和 model metadata 生成。
3. EPS 为全量 bootstrap 描述：登录前可匿名访问，不按用户、角色或权限裁剪。
4. EPS 输出结构对齐 `docs/protocol/fixtures/eps-admin-success.json`。
5. 至少生成 base open、comm、sys CRUD controllers。
6. 前端能生成当前已实现接口的 service 方法。
7. EPS 输出使用 Node/Vue 兼容 camelCase 字段。
8. EPS route 忽略 auth，登录前可访问。

## 3. 非目标

阶段 6C 不做：

1. Vue 真实联调中的全部缺口修复。
2. user move、menu parse/create/export/import、department order、param html、log clear/setKeep/getKeep 的业务实现。
3. CRUD hooks。
4. 文件上传。
5. 数据权限。
6. 插件系统或动态模块加载。
7. TypeScript `.d.ts` 文件生成。

## 4. EPS 响应结构

HTTP 接口：

```text
GET /admin/base/open/eps
```

响应外层使用标准成功包络：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "base": []
  }
}
```

`data` 按 module key 分组：

```json
{
  "base": [
    {
      "module": "base",
      "name": "BaseSysUserEntity",
      "prefix": "/admin/base/sys/user",
      "info": {},
      "api": [],
      "columns": [],
      "pageQueryOp": {},
      "pageColumns": []
    }
  ]
}
```

阶段 6C 不引入额外顶层字段，避免前端解析差异。

## 5. EPS Controller 对象

```go
type Controller struct {
   Module      string                 `json:"module"`
   Name        string                 `json:"name"`
   Prefix      string                 `json:"prefix"`
   Info        map[string]interface{} `json:"info"`
   API         []API                  `json:"api"`
   Columns     []Column               `json:"columns"`
   PageQueryOp PageQueryOp            `json:"pageQueryOp"`
   PageColumns []Column               `json:"pageColumns"`
}
```

字段来源：

| EPS 字段 | 来源 |
|---|---|
| `module` | controller metadata `Module` |
| `name` | controller metadata `Name` |
| `prefix` | controller metadata `Prefix` |
| `info.type.name` | controller path 最后一段或 metadata type name |
| `info.type.description` | controller metadata `Description` |
| `api` | CRUD APIs + custom routes |
| `columns` | model metadata |
| `pageQueryOp` | controller CRUD query options |
| `pageColumns` | 第一阶段空数组 |

## 6. API 生成规则

### 6.1 API 类型

```go
type API struct {
   Method      string                 `json:"method"`
   Path        string                 `json:"path"`
   Summary     string                 `json:"summary"`
   DTS         map[string]interface{} `json:"dts"`
   Tag         string                 `json:"tag"`
   Prefix      string                 `json:"prefix"`
   IgnoreToken bool                   `json:"ignoreToken"`
}
```

### 6.2 CRUD API

对 controller 的 `CRUD.APIs` 生成：

| CRUD API | method | path | summary |
|---|---|---|---|
| add | POST | `/add` | 新增 |
| delete | POST | `/delete` | 删除 |
| update | POST | `/update` | 修改 |
| info | GET | `/info` | 单个信息 |
| list | POST | `/list` | 列表查询 |
| page | POST | `/page` | 分页查询 |

`prefix` 为 controller prefix。

`ignoreToken` 默认 `false`。

### 6.3 Custom route API

对 controller `Routes` 生成：

| EPS 字段 | 来源 |
|---|---|
| `method` | route method |
| `path` | route path，例如 `/login` |
| `summary` | route description，空时使用 route name |
| `prefix` | controller prefix |
| `ignoreToken` | route `IgnoreAuth` |

示例：

```json
{
  "method": "POST",
  "path": "/login",
  "summary": "登录",
  "dts": {},
  "tag": "",
  "prefix": "/admin/base/open",
  "ignoreToken": true
}
```

## 7. Column 生成规则

### 7.1 Column 类型

```go
type Column struct {
   PropertyName string      `json:"propertyName"`
   Type         string      `json:"type"`
   Length       string      `json:"length"`
   Comment      string      `json:"comment"`
   Nullable     bool        `json:"nullable"`
   DefaultValue interface{} `json:"defaultValue"`
   Dict         interface{} `json:"dict"`
   Source       string      `json:"source"`
}
```

### 7.2 字段来源

字段来自 `cool/model.Definition`。

映射规则：

| Column 字段 | 来源 |
|---|---|
| `propertyName` | model field JSON/camelCase 名称 |
| `type` | DB/type metadata 映射成 Node 兼容类型 |
| `length` | 字段长度，缺省为空字符串 |
| `comment` | 字段注释 |
| `nullable` | 是否可空 |
| `defaultValue` | 默认值，无默认值为 `null` |
| `dict` | 字典，无字典为 `null` |
| `source` | `a.` + propertyName |

### 7.3 类型映射

| Go/model 类型 | EPS type |
|---|---|
| int / int64 / uint64 | int |
| string / varchar / text | varchar |
| bool | tinyint |
| time / datetime | datetime |
| json | json |

如果现有 model metadata 无法表达 length/comment/default/dict，阶段 6C 应补充 model metadata，而不是在 EPS 层硬编码字段。

## 8. pageQueryOp 生成规则

```go
type PageQueryOp struct {
   KeyWordLikeFields []string `json:"keyWordLikeFields"`
   FieldEq           []string `json:"fieldEq"`
   FieldLike         []string `json:"fieldLike"`
}
```

来源于 controller CRUD query options。

输出规则：

1. 每个字段加 `a.` 前缀。
2. 输入已经有 `a.` 前缀时不重复添加。
3. 保持字段顺序。
4. 空项输出空数组，不省略字段。

示例：

```json
{
  "keyWordLikeFields": ["a.name", "a.username", "a.nickName"],
  "fieldEq": ["a.status", "a.departmentId"],
  "fieldLike": []
}
```

## 9. 生成服务设计

新增包：

```text
cool/eps/
```

核心接口：

```go
type Generator struct{}

func NewGenerator() *Generator
func (g *Generator) Generate(controllers []controller.Definition) map[string][]Controller
```

或者无状态函数：

```go
func Generate(controllers []controller.Definition) map[string][]Controller
```

推荐先用无状态函数，简单直接。EPS metadata 在当前阶段随应用启动固定，单次生成开销很小；不引入缓存、刷新或权限裁剪状态。

### 9.1 生成流程

```text
controllers
  ↓
filter EPS-visible controllers
  ↓
controller -> EPS Controller
  ↓
CRUD APIs + custom Routes -> api[]
  ↓
model fields -> columns[]
  ↓
CRUD query options -> pageQueryOp
  ↓
group by module
```

### 9.2 EPS-visible controller

默认规则：

1. 有 CRUD 的 controller 进入 EPS。
2. 有 Routes 的 controller 进入 EPS。
3. EPS 始终生成所有可见 controller，不按用户、角色或权限过滤。
4. 如果 controller 显式标记 `HideFromEPS`，不进入 EPS。

阶段 6C 可预留：

```go
HideFromEPS bool
```

但第一阶段不需要隐藏 base controller。

## 10. `/admin/base/open/eps` 接入

阶段 6C 在 open controller 中接入 EPS handler。

handler 行为：

```go
func (s *EPSService) Admin(ctx context.Context) (map[string][]eps.Controller, error) {
   return eps.Generate(s.Controllers), nil
}
```

HTTP response：

```go
r.Response.WriteJson(response.OK(data))
```

路由 metadata：

```go
Route(controller.RouteOptions{
   Name:        "eps",
   Method:      http.MethodGet,
   Path:        "/eps",
   Description: "EPS",
   IgnoreAuth:  true,
   Handler:     epsService.Admin,
})
```

## 11. 和阶段 6A/6B 的依赖

阶段 6C 依赖：

1. controller metadata 中有完整 Prefix、Name、Description、CRUD、Routes、Model。
2. app 能收集所有 module controllers。
3. open EPS handler 接收 app 收集的全量 controller metadata，并调用无状态 `eps.Generate`。
4. `/admin/base/open/eps` 由 metadata 驱动注册。
5. IgnoreAuth paths 能包含 `/admin/base/open/eps`。
6. EPS 不依赖当前登录用户，也不查询权限服务。

如果 6A/6B 尚未实现，6C 实施计划必须先把必要 metadata 能力补齐，不能手写独立 EPS 清单。

## 12. 测试策略

### 12.1 EPS generator 单元测试

新增：

```text
cool/eps/eps_test.go
```

覆盖：

1. CRUD controller 生成 add/delete/update/info/list/page。
2. open controller 生成 login/refreshToken/captcha/eps。
3. comm controller 生成 person/permmenu/logout/program。
4. columns 使用 camelCase propertyName 和 `a.` source。
5. pageQueryOp 加 `a.` 前缀。
6. 输出按 module 分组。

### 12.2 fixture 对齐测试

新增或扩展：

```text
modules/base/eps_test.go
```

覆盖：

1. base EPS 包含 `BaseSysUserEntity`。
2. user api 包含 `/page`、`/add`、`/info`。
3. user columns 至少包含 `id`、`username`、`status`、`createTime`、`updateTime`。
4. pageQueryOp 包含 `a.name`、`a.username`、`a.nickName`、`a.status`、`a.departmentId`。

### 12.3 HTTP 集成测试

新增：

```text
modules/base/eps_integration_test.go
```

环境变量：

```text
COOL_EPS_INTEGRATION=1
```

流程：

1. 启动 app HTTP routes。
2. 未登录请求 `/admin/base/open/eps`。
3. 断言 HTTP 200。
4. 断言 body code 1000。
5. 断言 data.base 非空。
6. 断言包含 user controller 和 page API。

必跑命令：

```bash
go test ./cool/eps ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_EPS_INTEGRATION=1 go test ./modules/base -run EPSIntegration -count=1
```

## 13. 风险和约束

1. 前端 EPS parser 可能依赖更多 Node 字段，阶段 6C 要以真实 fixture 和前端行为为准。
2. model metadata 如果缺少 comment/length/default/dict，需要补 model 层，不要在 EPS 生成器中写 base 专用硬编码。
3. open/eps 必须 IgnoreAuth，否则前端登录前无法 bootstrap。
4. EPS API path 应为相对 path，例如 `/page`，prefix 单独输出。
5. method 大小写应与 fixture 一致，使用 `GET` / `POST`。
6. pageQueryOp 字段必须是 `a.camelCase`，不是 DB snake_case。
7. 阶段 6C 不应改变已有 auth/permmenu/CRUD 行为。

## 14. 验收标准

阶段 6C 完成后必须满足：

1. `GET /admin/base/open/eps` 未登录可访问。
2. EPS 响应 code 为 1000。
3. EPS `data.base` 非空。
4. EPS 包含 open login/refreshToken/captcha/eps。
5. EPS 包含 comm person/permmenu/logout/program。
6. EPS 包含 sys user/role/menu/department/param/log CRUD controller。
7. user controller 包含 page/add/update/delete/info/list API。
8. user columns 至少包含 fixture 中代表字段。
9. user pageQueryOp 与 controller metadata 一致。
10. `go test ./...` 通过。
11. `COOL_EPS_INTEGRATION=1 go test ./modules/base -run EPSIntegration -count=1` 通过。
12. 未创建 `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/`。

## 15. 自检

本设计只覆盖 EPS runtime。它依赖 6A/6B 的 controller metadata 和 metadata 驱动注册，不允许重新手写一份 EPS 接口清单。自定义 API 和 hooks 留到 6D。
