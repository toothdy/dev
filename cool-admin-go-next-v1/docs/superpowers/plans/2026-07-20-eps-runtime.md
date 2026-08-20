# EPS Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 从已注册的 Controller 与 Model metadata 生成 Node/Vue 兼容 EPS，并通过匿名 `GET /admin/base/open/eps` 输出全量 bootstrap 描述。

**Architecture:** 新增无状态 `cool/eps` 纯生成器，将 `[]controller.Definition` 映射为按模块分组的 EPS Controller 数据。EPS HTTP handler 作为 base open Controller 的 metadata route 注入；应用在构造运行时 Controller 后提供一个读取当前全量模块 Controller 的 getter，因此 EPS 始终来自与路由、认证、权限、CRUD 相同的 metadata 来源。

**Tech Stack:** Go 1.x、GoFrame v2、现有 `cool/controller`、`cool/model`、`cool/module`、`cool/response`、标准库 `context`/`strings`。

## Global Constraints

- 所有项目面向的说明和代码注释使用中文。
- EPS 是匿名全量 bootstrap：`GET /admin/base/open/eps` 登录前可访问，且不按用户、角色或权限裁剪。
- EPS 必须由 controller/model metadata 派生；不得维护 base 专用手写 EPS 清单。
- 保持已有 HTTP 路径、方法、认证、权限与 CRUD 行为不变。
- EPS API 的 `path` 必须是相对 Controller prefix 的路径，`prefix` 单独输出。
- EPS API method 使用大写 `GET`/`POST`；字段命名使用 Node/Vue 兼容 camelCase。
- `pageQueryOp` 的字段必须保持声明顺序并使用 `a.<camelCase>`；空字段输出空数组。
- 不引入 EPS 缓存、刷新机制、权限裁剪、TypeScript 文件生成、`dao/`、`internal/model/do/`、`internal/model/entity/` 或 `logic/`。
- 不在本阶段加入 `HideFromEPS`；当前所有具有 CRUD 或自定义 Route 的 Controller 都是 EPS 可见对象。
- 新增 Go 文件必须执行 `gofmt`；不得使用 `git add -A`。
- 每个引入或变更生产行为的任务必须先观察到失败测试，再编写最小实现并运行任务聚焦测试；仅文档与最终回归验证任务复用此前已完成的 TDD 测试。

---

## File Structure

### New files

- `cool/eps/eps.go` — EPS public DTO、无状态 metadata 生成器、字段/API/query 映射。
- `cool/eps/eps_test.go` — generator 单元测试：CRUD、custom routes、columns、模块分组、查询字段前缀。
- `modules/base/eps_test.go` — base metadata 的 EPS fixture 级结构断言与匿名 HTTP endpoint 测试。
- `modules/base/eps_integration_test.go` — 通过真实 GoFrame listener 验证未登录 EPS endpoint。

### Modified files

- `modules/base/controller/open/open.go` — 在 open Controller metadata 中声明 `GET /eps`，由 EPS handler 返回全量生成结果。
- `modules/base/controller/controllers.go` — 为 Open Controller 注入 EPS service；保留所有现有 Controller/Service 绑定。
- `modules/base/routes.go`、`modules/base/auth_routes.go`、`modules/base/permission_routes.go`、`modules/base/permission.go` — 更新兼容 shim 对 `Controllers` 的调用，显式传入 `nil` EPS service。
- `cool/app/app.go` — 在绑定运行时 Controller 时构造一个读取当前模块 Controller metadata 的 EPS getter，并允许 `Options.Server` 注入独立测试 server。
- `cool/app/app_test.go` — 覆盖注入 server 的完整 route 注册路径。
- `modules/base/controller/controllers_test.go` — 更新 `Controllers` 调用参数，并断言 open metadata 包含匿名 EPS route。

---

### Task 1: 实现无状态 EPS metadata 生成器

**Files:**
- Create: `cool/eps/eps.go`
- Create: `cool/eps/eps_test.go`

**Interfaces:**
- Consumes: `controller.Definition`、`controller.CRUDDefinition`、`controller.RouteDefinition`、`model.Definition`、`crud.RouteMethod`。
- Produces:

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

type API struct {
	Method      string                 `json:"method"`
	Path        string                 `json:"path"`
	Summary     string                 `json:"summary"`
	DTS         map[string]interface{} `json:"dts"`
	Tag         string                 `json:"tag"`
	Prefix      string                 `json:"prefix"`
	IgnoreToken bool                   `json:"ignoreToken"`
}

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

type PageQueryOp struct {
	KeyWordLikeFields []string `json:"keyWordLikeFields"`
	FieldEq           []string `json:"fieldEq"`
	FieldLike         []string `json:"fieldLike"`
}

func Generate(controllers []controller.Definition) map[string][]Controller
```

- [ ] **Step 1: 写 generator 的失败测试**

创建 `cool/eps/eps_test.go`，用一个 CRUD Controller 和一个 open Route Controller 直接描述 EPS 合约：

```go
func TestGenerateBuildsCRUDAndCustomAPIs(t *testing.T) {
   definition := controller.Admin("base/sys/user").
      Name("BaseSysUserEntity").
      Description("用户管理").
      Model(testUserModel()).
      CRUD(controller.CRUDOptions{
         APIs: []string{crud.APIAdd, crud.APIInfo, crud.APIPage},
         PageQuery: controller.QueryOptions{
            KeyWordLikeFields: []string{"username"},
            FieldEq:           []string{"status"},
         },
      }).
      Build()
   open := controller.Open("base/open").
      Name("BaseOpenController").
      Route(controller.RouteOptions{
         Name: "login", Method: http.MethodPost, Path: "/login",
         Description: "登录", IgnoreAuth: true,
      }).
      Build()

   data := Generate([]controller.Definition{definition, open})
   if len(data["base"]) != 2 {
      t.Fatalf("expected two base EPS controllers, got %#v", data)
   }
   user := data["base"][0]
   if user.API[0].Method != http.MethodPost || user.API[0].Path != "/add" || user.API[0].Prefix != "/admin/base/sys/user" {
      t.Fatalf("unexpected CRUD api: %#v", user.API[0])
   }
   if user.PageQueryOp.KeyWordLikeFields[0] != "a.username" || user.PageQueryOp.FieldEq[0] != "a.status" {
      t.Fatalf("unexpected page query op: %#v", user.PageQueryOp)
   }
   if data["base"][1].API[0].IgnoreToken != true || data["base"][1].API[0].Path != "/login" {
      t.Fatalf("unexpected custom API: %#v", data["base"][1].API[0])
   }
}
```

在同一文件添加字段和空 query 规则测试：

```go
func TestGenerateMapsModelFieldsAndKeepsEmptyQueryArrays(t *testing.T) {
	definition := controller.Admin("base/sys/user").
		Model(testUserModel()).
		CRUD(controller.CRUDOptions{}).
		Build()

	item := Generate([]controller.Definition{definition})["base"][0]
	if item.Columns[0].PropertyName != "id" || item.Columns[0].Type != "int" || item.Columns[0].Source != "a.id" {
		t.Fatalf("unexpected id column: %#v", item.Columns[0])
	}
	if item.Columns[1].PropertyName != "username" || item.Columns[1].Type != "varchar" || item.Columns[1].Length != "100" {
		t.Fatalf("unexpected username column: %#v", item.Columns[1])
	}
	if item.Columns[2].PropertyName != "status" || item.Columns[2].Type != "int" || item.Columns[2].DefaultValue != int64(1) || !reflect.DeepEqual(item.Columns[2].Dict, []string{"禁用", "启用"}) {
		t.Fatalf("unexpected status column: %#v", item.Columns[2])
	}
	if item.PageQueryOp.KeyWordLikeFields == nil || item.PageQueryOp.FieldEq == nil || item.PageQueryOp.FieldLike == nil {
		t.Fatalf("expected non-nil empty page query arrays: %#v", item.PageQueryOp)
	}
}
```

`testUserModel` 必须使用 `model.NewDefinition` 生成 `id bigint`、`username varchar`（长度 100）、`status tinyint`（默认 `1` 且 dict 为 `禁用`、`启用`）字段，以验证已有 model metadata 的真实映射：

```go
func testUserModel() model.Definition {
	return model.NewDefinition("base", "BaseSysUser", "base_sys_user").Fields([]model.Field{
		model.NewField("id", "id", "bigint").Primary().Comment("ID"),
		model.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		model.NewField("status", "status", "tinyint").NotNull().Default("1").Comment("状态").WithDict("禁用", "启用"),
	})
}
```

并在字段测试中断言 `status` 映射为 `Type: "int"`、`DefaultValue: int64(1)`，且 `Dict` 为 `[]string{"禁用", "启用"}`。

- [ ] **Step 2: 运行测试，确认 Red**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/eps -count=1
```

预期：失败，提示 package 或 `Generate` 未定义。

- [ ] **Step 3: 实现 EPS DTO 与生成器**

创建 `cool/eps/eps.go`，添加 `strconv`、`strings`、`cool/controller` 和 `cool/crud` imports，并实现以下固定映射：

```go
package eps

// Generate 从全量 Controller metadata 生成按模块分组的 EPS。
func Generate(definitions []controller.Definition) map[string][]Controller {
   result := map[string][]Controller{}
   for _, definition := range definitions {
      if definition.CRUD == nil && len(definition.Routes) == 0 {
         continue
      }
      result[definition.Module] = append(result[definition.Module], buildController(definition))
   }
   return result
}
```

`buildController` 的精确行为：

- `Info` 固定为 `map[string]interface{}{"type": map[string]string{"name": controllerTypeName(definition.Prefix), "description": definition.Description}}`；`controllerTypeName` 去除 `/admin/` 后取最后一个路径段。
- CRUD API 按 `definition.CRUD.APIs` 声明顺序生成，使用 `crud.RouteMethod(api)`；未知 API 跳过，因为 Builder metadata 已由 runtime 注册阶段校验。
- CRUD summary 使用固定映射：`add: 新增`、`delete: 删除`、`update: 修改`、`info: 单个信息`、`list: 列表查询`、`page: 分页查询`。
- 自定义 route 按 `definition.Routes` 声明顺序追加；summary 使用 `route.Description`，为空时使用 `route.Name`；`IgnoreToken` 等于 `route.IgnoreAuth`。
- CRUD API 的 `Path` 为 `"/" + api`，`IgnoreToken` 为 false；所有 API 的 `DTS` 必须是非 nil 空 map、`Tag` 为 `""`、`Prefix` 为 definition.Prefix。
- `Columns` 按 `definition.Model.FieldsValue` 声明顺序映射；`Length` 用 `strconv.Itoa(field.Length)`，长度 0 时 `""`；默认值的具体 JSON 标量转换遵循本列表最后一项；`Dict` 为字段字典的 slice 副本（字典为空时 `nil`）；`Source` 是 `"a." + field.JSONName`。
- EPS type 映射采用 `strings.ToLower(field.DataType)`：`bigint`、`int`、`integer`、`uint64`、`tinyint` 映射 `int`；`varchar`、`string`、`text` 映射 `varchar`；`bool`、`boolean` 映射 `tinyint`；`time`、`datetime`、`timestamp` 映射 `datetime`；`json` 映射 `json`；其他值保留小写原值。`tinyint` 必须映射为 `int`，因为现有 `status` metadata 为 `tinyint`，而 fixture 契约要求 `status.type` 是 `int`。
- `PageQueryOp` 只使用 `definition.CRUD.PageQuery`；CRUD 不存在时三个字段都为非 nil 空 slice。用 `withAlias` 保持顺序，为未带 `a.` 的字段加一次前缀，已带 `a.` 的不重复添加。
- `PageColumns` 始终是非 nil 空 slice。
- `DefaultValue` 必须保留 fixture 的 JSON 标量类型：无默认值为 `nil`；`bigint`、`int`、`integer`、`uint64`、`tinyint` 默认值用 `strconv.ParseInt(..., 10, 64)` 转为 `int64`，解析失败时保留原始字符串；`bool`、`boolean` 用 `strconv.ParseBool` 转为 bool，解析失败时保留原始字符串；其余类型保留 `field.DefaultValue` 字符串。因此真实 user `status` 的默认值 `"1"` 输出 JSON 数字 `1`。

- [ ] **Step 4: 运行 generator 测试，确认 Green**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/eps/eps.go cool/eps/eps_test.go
go test ./cool/eps -count=1
```

预期：PASS。

- [ ] **Step 5: 提交 generator**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/eps/eps.go cool/eps/eps_test.go
git commit -m $'feat: generate EPS from controller metadata\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 2: 将匿名 EPS route 接入 base Open Controller 与应用 metadata

**Files:**
- Modify: `modules/base/controller/open/open.go`
- Modify: `modules/base/controller/controllers.go`
- Modify: `modules/base/routes.go`
- Modify: `modules/base/auth_routes.go`
- Modify: `modules/base/permission_routes.go`
- Modify: `modules/base/permission.go`
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Modify: `modules/base/controller/controllers_test.go`
- Create: `modules/base/eps_test.go`

**Interfaces:**
- Consumes: `eps.Generate([]controller.Definition) map[string][]eps.Controller`、`module.CollectControllers([]module.Module) []controller.Definition`。
- Produces:
  - `type EPSService struct { controllers func() []controller.Definition }`
  - `func NewEPSService(controllers func() []controller.Definition) *EPSService`
  - `func (s *EPSService) Admin(ctx context.Context) (map[string][]eps.Controller, error)`
  - `func OpenController(authService *baseService.AuthService, paramService *baseService.ParamService, epsService *EPSService) controller.Definition`
  - `func Controllers(db gdb.DB, manager *auth.Manager, epsService *openController.EPSService) []controller.Definition`
  - `Options.Server *ghttp.Server`，仅在 `StartServer: true` 时替代 `g.Server()`，使 endpoint 测试能使用唯一 server。
  - 匿名 `GET /admin/base/open/eps` route metadata。

- [ ] **Step 1: 写失败的 base Controller 与 HTTP 测试**

在 `modules/base/controller/controllers_test.go` 增加 metadata 断言，并先将文件中每个 `Controllers(nil, nil)` 调用改为 `Controllers(nil, nil, nil)`：

```go
func TestControllersDeclareAnonymousEPSRoute(t *testing.T) {
	definitions := Controllers(nil, nil, openController.NewEPSService(func() []coolController.Definition {
		return nil
	}))
	var open coolController.Definition
	for _, definition := range definitions {
		if definition.Prefix == "/admin/base/open" {
			open = definition
			break
		}
	}
	for _, route := range open.Routes {
		if route.Path == "/eps" {
			if route.Method != http.MethodGet || !route.IgnoreAuth || route.Handler == nil {
				t.Fatalf("expected anonymous GET eps route, got %#v", route)
			}
			return
		}
	}
	t.Fatal("expected EPS route")
}
```

新建 `modules/base/eps_test.go`，通过新增的 `Options.Server` 使用独立 GoFrame server；这个生产选项只注入 server，不新增测试专用 route 注册 API：

```go
func TestEPSRouteReturnsAnonymousFullBootstrap(t *testing.T) {
	server := ghttp.GetServer("base-eps-test")
	server.SetPort(0)
	server.SetDumpRouterMap(false)
	defer server.Shutdown()

	app.New(app.Options{StartServer: true, Server: server})
	request := httptest.NewRequest(http.MethodGet, "/admin/base/open/eps", nil)
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	body := struct {
		Code int                           `json:"code"`
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode EPS response failed: %v", err)
	}
	if body.Code != coolErrors.CodeSuccess {
		t.Fatalf("expected success response, got %#v", body)
	}
	if len(body.Data["base"]) != 8 {
		t.Fatalf("expected eight base controllers, got %#v", body.Data)
	}
}
```

测试请求不得设置 `Authorization` header。

- [ ] **Step 2: 运行测试，确认 Red**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/controller ./modules/base -run 'TestControllersDeclareAnonymousEPSRoute|TestEPSRouteReturnsAnonymousFullBootstrap' -count=1
```

预期：失败，`EPSService`、第三个 `OpenController` 参数或 `/eps` route 不存在。

- [ ] **Step 3: 实现 EPS service 和 Open route**

在 `modules/base/controller/open/open.go`：

1. 导入 `context`、`cool/eps` 与 `cool/controller`。
2. 新增：

```go
// EPSService 是 base EPS bootstrap 服务。
type EPSService struct {
   controllers func() []coolController.Definition
}

// NewEPSService 创建 EPS bootstrap 服务。
func NewEPSService(controllers func() []coolController.Definition) *EPSService {
   return &EPSService{controllers: controllers}
}

// Admin 返回当前全量 Controller metadata 的 EPS。
func (s *EPSService) Admin(ctx context.Context) (map[string][]eps.Controller, error) {
   _ = ctx
   if s == nil || s.controllers == nil {
      return map[string][]eps.Controller{}, nil
   }
   return eps.Generate(s.controllers()), nil
}
```

3. 将 `OpenController` 签名改为：

```go
func OpenController(authService *baseService.AuthService, paramService *baseService.ParamService, epsService *EPSService) coolController.Definition
```

4. 在 captcha route 后追加固定 metadata。由于兼容 shim 会传入 nil，先构造安全 handler，再把它放入 route：

```go
epsHandler := func(ctx context.Context) (map[string][]eps.Controller, error) {
	return map[string][]eps.Controller{}, nil
}
if epsService != nil {
	epsHandler = epsService.Admin
}
```

```go
.Route(coolController.RouteOptions{
	Name:        "eps",
	Method:      http.MethodGet,
	Path:        "/eps",
	Description: "EPS",
	IgnoreAuth:  true,
	Handler:     epsHandler,
})
```

在 `modules/base/controller/controllers.go`：

- 将 `Controllers` 签名更新为：

```go
func Controllers(db gdb.DB, manager *auth.Manager, epsService *openController.EPSService) []coolController.Definition
```

- 调用 `openController.OpenController(authService, paramService, epsService)`。

在以下兼容 shim 中，将每个 `baseController.Controllers(nil, nil)` 更新为 `baseController.Controllers(nil, nil, nil)`；这些路径不提供 app runtime EPS getter，传 nil 后 `/eps` 仍安全地返回空 map：

```text
modules/base/routes.go
modules/base/auth_routes.go
modules/base/permission_routes.go
modules/base/permission.go
```

在 `cool/app/app.go`：

1. 为 `Options` 增加：

```go
Server *ghttp.Server
```

2. 将 server 初始化改为：

```go
if options.StartServer {
	application.server = options.Server
	if application.server == nil {
		application.server = g.Server()
	}
	application.registerRoutes()
}
```

3. 在 `bindRuntimeControllers` 构造 getter 并传给 base Controller factory：

```go
func (a *Application) bindRuntimeControllers() {
	epsService := openController.NewEPSService(func() []coolController.Definition {
		return module.CollectControllers(a.modules)
	})
	baseControllers := baseController.Controllers(g.DB(), a.authManager, epsService)
	// 保留现有找到 base module 并替换 Definition 的循环。
}
```

若 `cool/app/app.go` 尚未导入 `modules/base/controller/open`，以 `openController` 别名导入。这个 getter 在请求时读取 `a.modules`，因此它在 `definition.Controllers(baseControllers)` 完成后会生成当前模块全量 metadata，不会出现初始化时空 EPS，也不需缓存。

在 `cool/app/app_test.go` 增加 `TestNewUsesInjectedServer`：使用 `ghttp.GetServer("app-injected-server-test")` 和 `Options{StartServer: true, Server: server}` 创建 app，随后以 `server.ServeHTTP` 请求 `/health` 并断言 HTTP 200，证明 endpoint 测试所依赖的注入 server 已被 route 初始化路径实际使用。

- [ ] **Step 4: 扩展 base EPS 内容断言**

在 `modules/base/eps_test.go` 将响应 `data` 解码为 `map[string][]eps.Controller` 并断言：

```go
baseControllers := data["base"]
if len(baseControllers) != 8 {
   t.Fatalf("expected 8 base controllers, got %#v", baseControllers)
}
user := findEPSController(t, baseControllers, "BaseSysUserEntity")
assertEPSAPI(t, user.API, http.MethodPost, "/page", false)
assertEPSAPI(t, user.API, http.MethodPost, "/add", false)
assertEPSAPI(t, user.API, http.MethodGet, "/info", false)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseOpenController").API, http.MethodGet, "/eps", true)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseCommController").API, http.MethodGet, "/program", true)
```

同时断言 user `Columns` 至少有 `id`、`username`、`status`、`createTime`、`updateTime`，且 `PageQueryOp` 等于：

```go
PageQueryOp{
   KeyWordLikeFields: []string{"a.name", "a.username", "a.nickName"},
   FieldEq:           []string{"a.status", "a.departmentId"},
   FieldLike:          []string{},
}
```

- [ ] **Step 5: 运行 task 聚焦测试，确认 Green**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/app/app.go cool/app/app_test.go modules/base/controller/open/open.go modules/base/controller/controllers.go modules/base/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/permission.go modules/base/controller/controllers_test.go modules/base/eps_test.go
go test ./cool/eps ./modules/base/controller ./modules/base ./cool/app -count=1
```

预期：PASS；未认证 `/admin/base/open/eps` 返回 HTTP 200、`code: 1000`，且 data.base 中包含 8 个 base Controller。

- [ ] **Step 6: 提交 EPS route 接入**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go modules/base/controller/open/open.go modules/base/controller/controllers.go modules/base/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/permission.go modules/base/controller/controllers_test.go modules/base/eps_test.go
git commit -m $'feat: expose anonymous EPS bootstrap endpoint\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 3: 用 fixture 契约、真实 listener 验收与文档防止 EPS 回退

**Files:**
- Modify: `cool/eps/eps_test.go`
- Modify: `modules/base/eps_test.go`
- Create: `modules/base/eps_integration_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: `eps.Generate`、`GET /admin/base/open/eps`、`docs/protocol/fixtures/eps-admin-success.json`。
- Produces: fixture 关键字段兼容回归保护与可复制的 EPS 验收命令。

- [ ] **Step 1: 写 fixture 对照与顺序性的失败测试**

在 `modules/base/eps_test.go` 读取 fixture，并只比较稳定、当前阶段承诺的 JSON 字段，而非要求完整字节相等。复用 `TestEPSRouteReturnsAnonymousFullBootstrap` 中解码的 `map[string][]eps.Controller` 的方式，新增以下 helper 和测试；fixture 使用相对 `modules/base` package 目录的路径 `../../docs/protocol/fixtures/eps-admin-success.json`：

```go
func loadEPSFixture(t *testing.T) map[string][]eps.Controller {
	t.Helper()
	contents, err := os.ReadFile("../../docs/protocol/fixtures/eps-admin-success.json")
	if err != nil {
		t.Fatalf("read EPS fixture failed: %v", err)
	}
	body := struct {
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err = json.Unmarshal(contents, &body); err != nil {
		t.Fatalf("decode EPS fixture failed: %v", err)
	}
	return body.Data
}

func findEPSController(t *testing.T, controllers []eps.Controller, name string) eps.Controller {
	t.Helper()
	for _, definition := range controllers {
		if definition.Name == name {
			return definition
		}
	}
	t.Fatalf("expected EPS controller %s", name)
	return eps.Controller{}
}

func assertEPSAPI(t *testing.T, apis []eps.API, method string, path string, ignoreToken bool) {
	t.Helper()
	for _, api := range apis {
		if api.Method == method && api.Path == path && api.IgnoreToken == ignoreToken {
			return
		}
	}
	t.Fatalf("expected API %s %s ignoreToken=%t, got %#v", method, path, ignoreToken, apis)
}

func assertEPSColumn(t *testing.T, columns []eps.Column, propertyName string, fieldType string, source string) {
	t.Helper()
	for _, column := range columns {
		if column.PropertyName == propertyName && column.Type == fieldType && column.Source == source {
			return
		}
	}
	t.Fatalf("expected column %s %s %s, got %#v", propertyName, fieldType, source, columns)
}

func TestBaseEPSMatchesFixtureContract(t *testing.T) {
	fixture := loadEPSFixture(t)
	actual := eps.Generate(module.CollectControllers(app.New(app.Options{StartServer: false}).Modules()))

	fixtureUser := findEPSController(t, fixture["base"], "BaseSysUserEntity")
	actualUser := findEPSController(t, actual["base"], "BaseSysUserEntity")
	if actualUser.Prefix != fixtureUser.Prefix {
		t.Fatalf("expected user prefix %s, got %s", fixtureUser.Prefix, actualUser.Prefix)
	}
	assertEPSAPI(t, actualUser.API, http.MethodPost, "/page", false)
	assertEPSColumn(t, actualUser.Columns, "username", "varchar", "a.username")
	if !reflect.DeepEqual(actualUser.PageQueryOp, fixtureUser.PageQueryOp) {
		t.Fatalf("expected page query %#v, got %#v", fixtureUser.PageQueryOp, actualUser.PageQueryOp)
	}
}
```

在 `cool/eps/eps_test.go` 新增顺序回归测试；同一 module 的 Controller 输出顺序必须等于 input definition 顺序，同一 Controller 的 API 输出顺序必须为 CRUD `APIs` 后跟 `Routes` 声明顺序：

```go
func TestGeneratePreservesControllerAndAPIOrder(t *testing.T) {
	first := controller.Admin("base/sys/first").
		Name("First").
		CRUD(controller.CRUDOptions{APIs: []string{crud.APIPage, crud.APIAdd}}).
		Route(controller.RouteOptions{Name: "custom", Method: http.MethodGet, Path: "/custom"}).
		Build()
	second := controller.Open("base/open").
		Name("Second").
		Route(controller.RouteOptions{Name: "login", Method: http.MethodPost, Path: "/login"}).
		Build()

	controllers := Generate([]controller.Definition{first, second})["base"]
	if controllers[0].Name != "First" || controllers[1].Name != "Second" {
		t.Fatalf("unexpected controller order: %#v", controllers)
	}
	apis := controllers[0].API
	if apis[0].Path != "/page" || apis[1].Path != "/add" || apis[2].Path != "/custom" {
		t.Fatalf("unexpected API order: %#v", apis)
	}
}
```

- [ ] **Step 2: 运行 fixture 与顺序测试，确认 Red**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/eps ./modules/base -run 'Test(BaseEPSMatchesFixtureContract|GeneratePreservesControllerAndAPIOrder)' -count=1
```

预期：在 Task 1 和 Task 2 完成前失败，分别提示 `Generate` 或 `/eps` contract 不存在；Task 2 完成后的最终运行 PASS。

- [ ] **Step 3: 写真实 listener 的失败集成测试**

新建 `modules/base/eps_integration_test.go`。它不访问 MySQL，且必须以 `COOL_EPS_INTEGRATION=1` 显式启用；`app.New` 使用 Task 2 引入的独立 server 注入，避免污染全局 `g.Server()`：

```go
func TestEPSIntegrationAnonymousBootstrap(t *testing.T) {
	if os.Getenv("COOL_EPS_INTEGRATION") != "1" {
		t.Skip("set COOL_EPS_INTEGRATION=1 to run EPS HTTP integration test")
	}

	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	app.New(app.Options{StartServer: true, Server: server})
	if err := server.Start(); err != nil {
		t.Fatalf("start EPS test server failed: %v", err)
	}
	defer server.Shutdown()

	response, err := http.Get("http://127.0.0.1:" + strconv.Itoa(server.GetListenedPort()) + "/admin/base/open/eps")
	if err != nil {
		t.Fatalf("request EPS failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read EPS response failed: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.StatusCode, body)
	}
	decoded := struct {
		Code int                           `json:"code"`
		Data map[string][]eps.Controller `json:"data"`
	}{}
	if err = json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode EPS response failed: %v", err)
	}
	if decoded.Code != coolErrors.CodeSuccess || len(decoded.Data["base"]) != 8 {
		t.Fatalf("unexpected EPS response: %#v", decoded)
	}
}
```

- [ ] **Step 4: 运行真实 listener 测试，确认 Red**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_EPS_INTEGRATION=1 go test ./modules/base -run TestEPSIntegrationAnonymousBootstrap -count=1
```

预期：在 Task 2 尚未接入 `/eps` 时失败，响应为 401 或 404；Task 2 完成后 PASS。

- [ ] **Step 5: 补充 README EPS 验收说明**

在 `README.md` 的权限验收章节后新增：

```markdown
## EPS Bootstrap 验收

EPS 是匿名全量 bootstrap 描述，不按登录用户、角色或权限裁剪：

```bash
go test ./cool/eps ./modules/base -count=1
curl http://127.0.0.1:8001/admin/base/open/eps
```

期望响应为 `code: 1000`，`data.base` 包含 base open、comm 和 sys CRUD Controller；每个 API 使用相对 `path` 与独立 `prefix`。
```

- [ ] **Step 6: 运行最终验证**

运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/eps/eps_test.go modules/base/eps_test.go modules/base/eps_integration_test.go
go test ./cool/eps ./modules/base/controller ./modules/base ./cool/app -count=1
COOL_EPS_INTEGRATION=1 go test ./modules/base -run TestEPSIntegrationAnonymousBootstrap -count=1
go test ./...
go vet ./...
git diff --check
```

预期：全部通过；现有 auth、permission、CRUD 测试无回退。

- [ ] **Step 7: 提交 fixture 契约、listener 验收与文档**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/eps/eps_test.go modules/base/eps_test.go modules/base/eps_integration_test.go README.md
git commit -m $'test: verify EPS fixture contract\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

## Plan Self-Review

- **Spec coverage:** Task 1 实现带完整 JSON tags 的 EPS DTO、CRUD/custom API、model columns、`a.` pageQueryOp、module 分组、`tinyint → int` 与默认值 JSON 标量映射；Task 2 实现匿名 `/open/eps` metadata route、所有 `Controllers` 调用点的签名迁移、注入 server 的可测试 app 初始化和全量 controller getter；Task 3 覆盖 fixture 契约、输出顺序、真实 listener 匿名验收、README 命令与全包验证。
- **Scope check:** 计划未引入缓存、权限裁剪、EPS hide flag、业务自定义 API 补齐、TypeScript 文件生成或高级 CRUD 查询，符合设计非目标；`Options.Server` 只解决现有 `Application` 无法使用独立 server 做 endpoint 测试的缺口，且仅在 `StartServer` 时生效。
- **Placeholder scan:** 已逐项核实 `response.Body` 使用 `coolErrors.CodeSuccess`、`Application` 当前没有 `RegisterRoutesForTest` 也没有 server 注入、base 模型 `status` 是 `tinyint`、以及四个兼容 shim 的 `Controllers` 调用点；计划不再引用未定义 helper 或测试专用生产 API。没有 TBD/TODO 或“适当处理”类未定义步骤；每个生产改动步骤均给出路径、签名、规则和测试命令。
- **Type consistency:** `eps.Generate` 只依赖 `controller.Definition`；所有 DTO 提供 fixture 所需 JSON tags；`EPSService.Admin` 返回 `map[string][]eps.Controller, error`，符合现有反射 handler 的 `(data, error)` 支持；Open Controller 对 nil EPS service 安全；app closure 在 module Definition 保存 runtime Controller 后读取全量 metadata；fixture、内存 HTTP 与真实 listener 测试均解码同一 `map[string][]eps.Controller` 结构。
