# cool-admin-go-next Skeleton Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 初始化 `cool-admin-go-next` 的 GoFrame v2 项目骨架和最小 `cool/app` runtime，让服务可以启动、注册 base 模块骨架，并提供 `/health` 健康检查。

**Architecture:** 使用 GoFrame v2 作为 HTTP/config/logging 底座，但不采用完整业务实现。创建 `cool` 兼容层的最小边界：`app` 负责启动编排，`module` 负责模块注册，`response` 负责 Node 兼容响应，`errors` 负责错误码常量，`modules/base` 提供第一个模块定义。后续 Plan 2+ 在这个骨架上增加自动建表、seed、CRUD、auth、EPS。

**Tech Stack:** Go 1.23+、GoFrame v2、标准库 `context`、Markdown/JSON 协议文档。包管理使用 Go modules；不新增 npm/pnpm/yarn 依赖。

## Global Constraints

- 始终用中文编写说明文档和代码注释。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL；本计划只建立配置占位，不连接真实 MySQL。
- 第一阶段采用运行时自动建表、运行时 EPS、`db.json` / `menu.json` 初始化导入；这些能力不在本计划实现，只预留启动阶段 hook。
- 第一阶段插件系统不实现，只预留扩展点。
- 目标本地目录是 `/Users/n/数据/cool-admin/cool-admin-go-next`。
- 后续远端仓库是 `https://github.com/toothdy/cool-admin-go-next`。
- 不使用 `git add -A`；提交时只显式 stage 本计划创建或修改的文件。
- GoFrame 自动生成文件后续必须由工具生成，不手写、不手改；本计划不创建 `dao/do/entity` 生成目录。
- 不使用 `logic/` 目录，业务逻辑后续直接放在 `service/`。
- Go 代码错误处理后续使用 GoFrame `gerror`；本计划的最小错误常量先放在 `cool/errors`。
- Go 文件内如果有 3 个及以上相关变量声明，使用 `var (...)` 分组。

---

## Scope Check

本计划只覆盖设计文档中的“阶段 1：项目骨架和 core runtime”。

包含：

1. Go module 初始化。
2. GoFrame 最小 HTTP 服务。
3. `/health` 路由。
4. `cool/app` 启动编排骨架。
5. `cool/module` 模块注册骨架。
6. `cool/response` Node 兼容响应结构。
7. `modules/base` 模块骨架。
8. 配置文件骨架。
9. 最小单元测试和 HTTP 测试。

不包含：

1. MySQL 连接实现。
2. 自动建表。
3. `db.json` / `menu.json` 导入。
4. CRUD runtime。
5. 登录/JWT/权限。
6. EPS runtime。
7. Vue 联调。

---

## File Structure

### 创建文件

- `go.mod`  
  Go module 定义，module path 为 `github.com/toothdy/cool-admin-go-next`。

- `main.go`  
  应用入口，只调用 `app.Run(context.Background())`。

- `manifest/config/config.yaml`  
  默认配置，包含 server、logger、database 占位、cool 配置占位。

- `manifest/config/config.local.yaml`  
  本地覆盖配置示例，默认不包含真实密码。

- `cool/app/app.go`  
  启动编排：创建 app、注册模块、注册 health route、启动 GoFrame server。

- `cool/app/app_test.go`  
  测试 app 初始化时模块注册和 health handler 行为。

- `cool/errors/code.go`  
  Node 兼容错误码常量。

- `cool/response/response.go`  
  Node 兼容响应结构和 `OK/Fail` helper。

- `cool/response/response_test.go`  
  测试 `OK/Fail` JSON 字段符合 Plan0 契约。

- `cool/module/module.go`  
  模块接口、配置结构、registry。

- `cool/module/module_test.go`  
  测试模块注册顺序、重复注册行为、清空 registry 的测试 helper。

- `modules/modules.go`  
  项目模块统一注册入口。

- `modules/base/base.go`  
  base 模块定义。

- `modules/base/config.go`  
  base 模块配置骨架。

- `modules/base/db.json`  
  空初始化数据占位，Plan3 再填充真实数据。

- `modules/base/menu.json`  
  空菜单数据占位，Plan3 再填充真实菜单。

- `README.md`  
  项目说明、当前阶段、启动命令。

### 修改文件

- `.gitignore`  
  补充 Go 构建产物、临时文件忽略规则，保留 `.superpowers/` 忽略。

### 不创建文件

- 不创建 `dao/`。
- 不创建 `internal/model/do/`。
- 不创建 `internal/model/entity/`。
- 不创建 `logic/`。
- 不创建业务 Controller/Service 实现。

---

### Task 1: 初始化 Go module 和基础配置文件

**Files:**
- Create: `go.mod`
- Create: `manifest/config/config.yaml`
- Create: `manifest/config/config.local.yaml`
- Modify: `.gitignore`
- Test: `go list ./...`

**Interfaces:**
- Consumes: Plan0 protocol contract path `docs/protocol/base-api-contract.md`。
- Produces: Go module `github.com/toothdy/cool-admin-go-next`，后续所有 Go import 使用该路径。

- [ ] **Step 1: Write failing verification for missing module**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go list ./...
```

Expected before implementation: FAIL with message similar to:

```text
pattern ./...: directory prefix . does not contain main module
```

If it already passes, inspect `go.mod`; it must contain `module github.com/toothdy/cool-admin-go-next`.

- [ ] **Step 2: Create `go.mod`**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/go.mod`:

```go
module github.com/toothdy/cool-admin-go-next

go 1.23

require github.com/gogf/gf/v2 v2.10.2
```

- [ ] **Step 3: Create config directory and `config.yaml`**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/manifest/config
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/manifest/config/config.yaml`:

```yaml
server:
  address: ":8001"
  openapiPath: "/api.json"
  swaggerPath: "/swagger"

logger:
  level: "all"
  stdout: true

# 第一阶段只支持 MySQL；Plan1 不连接数据库，只保留配置结构。
database:
  default:
    link: "mysql:root:password@tcp(127.0.0.1:3306)/cool_admin_go_next?loc=Local&parseTime=true"
    debug: true

cool:
  initDB: true
  initMenu: true
  initJudge: "db"
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
  eps:
    enable: true
  auth:
    jwtSecret: "cool-admin-go-next-dev-secret"
    tokenExpire: 7200
    refreshExpire: 604800
```

- [ ] **Step 4: Create `config.local.yaml`**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/manifest/config/config.local.yaml`:

```yaml
# 本地配置覆盖示例。不要提交真实生产密码。
server:
  address: ":8001"

database:
  default:
    link: "mysql:root:password@tcp(127.0.0.1:3306)/cool_admin_go_next?loc=Local&parseTime=true"
```

- [ ] **Step 5: Update `.gitignore`**

Ensure `/Users/n/数据/cool-admin/cool-admin-go-next/.gitignore` contains exactly these lines or a superset:

```gitignore
.superpowers/
/bin/
/tmp/
*.test
*.out
.DS_Store
.env
```

If the file only contains `.superpowers/`, replace it with the above content.

- [ ] **Step 6: Download modules**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go mod tidy
```

Expected: command exits `0` and may create `go.sum`.

- [ ] **Step 7: Verify module exists**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go list ./...
```

Expected after Task 1 only: command exits `0` and may print warning/no packages. If no Go files exist yet, acceptable output is no packages listed with exit `0`.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
git add go.mod go.sum .gitignore manifest/config/config.yaml manifest/config/config.local.yaml
git commit -m "chore: initialize go module and config" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

If `go.sum` does not exist, omit it from `git add`.

---

### Task 2: 实现 Node 兼容响应和错误码基础包

**Files:**
- Create: `cool/errors/code.go`
- Create: `cool/response/response.go`
- Create: `cool/response/response_test.go`
- Test: `go test ./cool/response ./cool/errors`

**Interfaces:**
- Consumes: Plan0 response contract: success code `1000`, common fail code `1001`。
- Produces:
  - `errors.CodeSuccess int`
  - `errors.CodeCommFail int`
  - `errors.CodeValidateFail int`
  - `response.Body` struct
  - `response.OK(data interface{}) response.Body`
  - `response.Fail(message string, code ...int) response.Body`

- [ ] **Step 1: Write failing response tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/response
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/response/response_test.go`:

```go
package response_test

import (
   "encoding/json"
   "testing"

   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/response"
)

func TestOKWithDataMatchesNodeContract(t *testing.T) {
   body := response.OK(map[string]string{
      "token": "access.jwt.token",
   })

   if body.Code != coolErrors.CodeSuccess {
      t.Fatalf("expected success code %d, got %d", coolErrors.CodeSuccess, body.Code)
   }
   if body.Message != "success" {
      t.Fatalf("expected success message, got %q", body.Message)
   }
   if body.Data == nil {
      t.Fatal("expected data to be present")
   }

   data, err := json.Marshal(body)
   if err != nil {
      t.Fatalf("marshal response: %v", err)
   }

   var decoded map[string]interface{}
   if err := json.Unmarshal(data, &decoded); err != nil {
      t.Fatalf("unmarshal response: %v", err)
   }
   if decoded["code"].(float64) != float64(coolErrors.CodeSuccess) {
      t.Fatalf("expected json code %d, got %v", coolErrors.CodeSuccess, decoded["code"])
   }
   if decoded["message"].(string) != "success" {
      t.Fatalf("expected json message success, got %v", decoded["message"])
   }
}

func TestFailDefaultsToCommonFail(t *testing.T) {
   body := response.Fail("账户或密码不正确~")

   if body.Code != coolErrors.CodeCommFail {
      t.Fatalf("expected common fail code %d, got %d", coolErrors.CodeCommFail, body.Code)
   }
   if body.Message != "账户或密码不正确~" {
      t.Fatalf("unexpected message: %q", body.Message)
   }
   if body.Data != nil {
      t.Fatalf("expected nil data, got %#v", body.Data)
   }
}

func TestFailSupportsCustomCode(t *testing.T) {
   body := response.Fail("参数错误", coolErrors.CodeValidateFail)

   if body.Code != coolErrors.CodeValidateFail {
      t.Fatalf("expected validate fail code %d, got %d", coolErrors.CodeValidateFail, body.Code)
   }
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/response ./cool/errors
```

Expected: FAIL because `cool/errors` and `cool/response` are not implemented.

- [ ] **Step 3: Implement error codes**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/errors
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/errors/code.go`:

```go
package errors

const (
   // CodeSuccess 是 Node 兼容成功码。
   CodeSuccess = 1000
   // CodeCommFail 是 Node 兼容通用失败码。
   CodeCommFail = 1001
   // CodeValidateFail 是参数校验失败码。
   CodeValidateFail = 1002
)
```

- [ ] **Step 4: Implement response helpers**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/response/response.go`:

```go
package response

import coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"

// Body 是兼容 Node 版的响应体。
type Body struct {
   Code    int         `json:"code"`
   Message string      `json:"message"`
   Data    interface{} `json:"data,omitempty"`
}

/**
 * 成功响应
 * @param data 响应数据
 * @returns Body
 */
func OK(data interface{}) Body {
   return Body{
      Code:    coolErrors.CodeSuccess,
      Message: "success",
      Data:    data,
   }
}

/**
 * 失败响应
 * @param message 错误消息
 * @param code 错误码
 * @returns Body
 */
func Fail(message string, code ...int) Body {
   failCode := coolErrors.CodeCommFail
   if len(code) > 0 {
      failCode = code[0]
   }

   return Body{
      Code:    failCode,
      Message: message,
   }
}
```

- [ ] **Step 5: Run response tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/response ./cool/errors
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/cool/response
?   	github.com/toothdy/cool-admin-go-next/cool/errors	[no test files]
```

- [ ] **Step 6: Commit Task 2**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/errors/code.go cool/response/response.go cool/response/response_test.go
git commit -m "feat: add node compatible response helpers" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 实现模块注册 registry

**Files:**
- Create: `cool/module/module.go`
- Create: `cool/module/module_test.go`
- Test: `go test ./cool/module`

**Interfaces:**
- Consumes: none from earlier implementation except Go module path.
- Produces:
  - `type Module interface`
  - `type Config struct`
  - `type Definition struct`
  - `func New(key string) *Definition`
  - `func Register(mod Module)`
  - `func List() []Module`
  - `func ResetForTesting()`

- [ ] **Step 1: Write failing module registry tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/module
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/module/module_test.go`:

```go
package module_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestRegisterOrdersModulesByOrderDesc(t *testing.T) {
   module.ResetForTesting()

   low := module.New("low").Name("低优先级").Order(1)
   high := module.New("high").Name("高优先级").Order(100)

   module.Register(low)
   module.Register(high)

   list := module.List()
   if len(list) != 2 {
      t.Fatalf("expected 2 modules, got %d", len(list))
   }
   if list[0].Key() != "high" {
      t.Fatalf("expected high module first, got %s", list[0].Key())
   }
   if list[1].Key() != "low" {
      t.Fatalf("expected low module second, got %s", list[1].Key())
   }
}

func TestRegisterReplacesDuplicateKey(t *testing.T) {
   module.ResetForTesting()

   module.Register(module.New("base").Name("旧 base").Order(1))
   module.Register(module.New("base").Name("新 base").Order(2))

   list := module.List()
   if len(list) != 1 {
      t.Fatalf("expected duplicate key to be replaced, got %d modules", len(list))
   }
   if list[0].NameText() != "新 base" {
      t.Fatalf("expected new module name, got %s", list[0].NameText())
   }
}

func TestDefinitionConfig(t *testing.T) {
   cfg := module.Config{
      Description: "系统基础能力",
      Order:       100,
   }

   mod := module.New("base").Name("基础模块").Config(cfg)

   if mod.Key() != "base" {
      t.Fatalf("unexpected key: %s", mod.Key())
   }
   if mod.NameText() != "基础模块" {
      t.Fatalf("unexpected name: %s", mod.NameText())
   }
   if mod.ModuleConfig().Description != "系统基础能力" {
      t.Fatalf("unexpected description: %s", mod.ModuleConfig().Description)
   }
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/module
```

Expected: FAIL because `cool/module` implementation is missing.

- [ ] **Step 3: Implement module registry**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/module/module.go`:

```go
package module

import "sort"

// Config 是模块配置骨架。
type Config struct {
   Description string
   Order       int
}

// Module 是 cool 模块接口。
type Module interface {
   Key() string
   NameText() string
   OrderValue() int
   ModuleConfig() Config
}

// Definition 是默认模块定义。
type Definition struct {
   key    string
   name   string
   config Config
}

var modules = map[string]Module{}

/**
 * 创建模块定义
 * @param key 模块标识
 * @returns *Definition
 */
func New(key string) *Definition {
   return &Definition{
      key: key,
      config: Config{
         Order: 0,
      },
   }
}

/**
 * 设置模块名称
 * @param name 模块名称
 * @returns *Definition
 */
func (d *Definition) Name(name string) *Definition {
   d.name = name
   return d
}

/**
 * 设置模块排序
 * @param order 排序值
 * @returns *Definition
 */
func (d *Definition) Order(order int) *Definition {
   d.config.Order = order
   return d
}

/**
 * 设置模块配置
 * @param config 模块配置
 * @returns *Definition
 */
func (d *Definition) Config(config Config) *Definition {
   d.config = config
   return d
}

/**
 * 模块标识
 * @returns string
 */
func (d *Definition) Key() string {
   return d.key
}

/**
 * 模块名称
 * @returns string
 */
func (d *Definition) NameText() string {
   return d.name
}

/**
 * 模块排序
 * @returns int
 */
func (d *Definition) OrderValue() int {
   return d.config.Order
}

/**
 * 模块配置
 * @returns Config
 */
func (d *Definition) ModuleConfig() Config {
   return d.config
}

/**
 * 注册模块
 * @param mod 模块定义
 * @returns null
 */
func Register(mod Module) {
   modules[mod.Key()] = mod
}

/**
 * 获取模块列表
 * @returns []Module
 */
func List() []Module {
   list := make([]Module, 0, len(modules))
   for _, mod := range modules {
      list = append(list, mod)
   }

   sort.SliceStable(list, func(i, j int) bool {
      return list[i].OrderValue() > list[j].OrderValue()
   })

   return list
}

/**
 * 重置测试模块注册表
 * @returns null
 */
func ResetForTesting() {
   modules = map[string]Module{}
}
```

- [ ] **Step 4: Run module tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/module
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/cool/module
```

- [ ] **Step 5: Commit Task 3**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/module/module.go cool/module/module_test.go
git commit -m "feat: add module registry" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 创建 base 模块骨架和统一模块注册入口

**Files:**
- Create: `modules/base/base.go`
- Create: `modules/base/config.go`
- Create: `modules/base/db.json`
- Create: `modules/base/menu.json`
- Create: `modules/modules.go`
- Create: `modules/base/base_test.go`
- Test: `go test ./modules/...`

**Interfaces:**
- Consumes:
  - `module.New(key string) *module.Definition`
  - `module.Register(mod module.Module)`
  - `module.List() []module.Module`
- Produces:
  - `base.NewModule() module.Module`
  - `modules.Register() []module.Module`

- [ ] **Step 1: Write failing base module tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/modules/base
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base_test.go`:

```go
package base_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/modules/base"
)

func TestNewModule(t *testing.T) {
   mod := base.NewModule()

   if mod.Key() != "base" {
      t.Fatalf("expected base key, got %s", mod.Key())
   }
   if mod.NameText() != "基础模块" {
      t.Fatalf("expected 基础模块, got %s", mod.NameText())
   }
   if mod.OrderValue() != 100 {
      t.Fatalf("expected order 100, got %d", mod.OrderValue())
   }
   if mod.ModuleConfig().Description != "系统基础能力" {
      t.Fatalf("unexpected description: %s", mod.ModuleConfig().Description)
   }
}
```

Create `/Users/n/数据/cool-admin/cool-admin-go-next/modules/modules_test.go`:

```go
package modules_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/module"
   "github.com/toothdy/cool-admin-go-next/modules"
)

func TestRegister(t *testing.T) {
   module.ResetForTesting()

   list := modules.Register()

   if len(list) != 1 {
      t.Fatalf("expected 1 module, got %d", len(list))
   }
   if list[0].Key() != "base" {
      t.Fatalf("expected base module, got %s", list[0].Key())
   }
}
```

- [ ] **Step 2: Run failing module tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/...
```

Expected: FAIL because `modules/base` and `modules.Register` are not implemented.

- [ ] **Step 3: Implement base config**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/config.go`:

```go
package base

import "github.com/toothdy/cool-admin-go-next/cool/module"

/**
 * 创建 base 模块配置
 * @returns module.Config
 */
func NewConfig() module.Config {
   return module.Config{
      Description: "系统基础能力",
      Order:       100,
   }
}
```

- [ ] **Step 4: Implement base module**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base.go`:

```go
package base

import "github.com/toothdy/cool-admin-go-next/cool/module"

/**
 * 创建 base 模块
 * @returns module.Module
 */
func NewModule() module.Module {
   return module.New("base").
      Name("基础模块").
      Config(NewConfig())
}
```

- [ ] **Step 5: Create seed placeholders**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/db.json`:

```json
{}
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/menu.json`:

```json
[]
```

- [ ] **Step 6: Implement global modules register**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/modules.go`:

```go
package modules

import (
   "github.com/toothdy/cool-admin-go-next/cool/module"
   "github.com/toothdy/cool-admin-go-next/modules/base"
)

/**
 * 注册全部模块
 * @returns []module.Module
 */
func Register() []module.Module {
   module.Register(base.NewModule())
   return module.List()
}
```

- [ ] **Step 7: Run module tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/...
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/modules
ok  	github.com/toothdy/cool-admin-go-next/modules/base
```

- [ ] **Step 8: Commit Task 4**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/modules.go modules/modules_test.go modules/base/base.go modules/base/base_test.go modules/base/config.go modules/base/db.json modules/base/menu.json
git commit -m "feat: add base module skeleton" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 实现最小 `cool/app` runtime 和 `/health`

**Files:**
- Create: `cool/app/app.go`
- Create: `cool/app/app_test.go`
- Create: `main.go`
- Test: `go test ./cool/app`

**Interfaces:**
- Consumes:
  - `modules.Register() []module.Module`
  - `response.OK(data interface{}) response.Body`
- Produces:
  - `app.Options`
  - `app.Application`
  - `app.New(options Options) *Application`
  - `(*Application).Modules() []module.Module`
  - `(*Application).Health(ctx context.Context) response.Body`
  - `(*Application).Run(ctx context.Context) error`
  - `app.Run(ctx context.Context) error`

- [ ] **Step 1: Write failing app tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/app
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app_test.go`:

```go
package app_test

import (
   "context"
   "testing"

   coolErrors "github.com/toothdy/cool-admin-go-next/cool/errors"
   "github.com/toothdy/cool-admin-go-next/cool/app"
)

func TestNewRegistersBaseModule(t *testing.T) {
   application := app.New(app.Options{
      StartServer: false,
   })

   mods := application.Modules()
   if len(mods) != 1 {
      t.Fatalf("expected 1 module, got %d", len(mods))
   }
   if mods[0].Key() != "base" {
      t.Fatalf("expected base module, got %s", mods[0].Key())
   }
}

func TestHealthMatchesNodeResponse(t *testing.T) {
   application := app.New(app.Options{
      StartServer: false,
   })

   body := application.Health(context.Background())
   if body.Code != coolErrors.CodeSuccess {
      t.Fatalf("expected success code %d, got %d", coolErrors.CodeSuccess, body.Code)
   }
   if body.Message != "success" {
      t.Fatalf("expected success message, got %q", body.Message)
   }
}
```

- [ ] **Step 2: Run failing app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app
```

Expected: FAIL because `cool/app` does not exist.

- [ ] **Step 3: Implement app runtime**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app.go`:

```go
package app

import (
   "context"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/gogf/gf/v2/net/ghttp"
   "github.com/toothdy/cool-admin-go-next/cool/module"
   "github.com/toothdy/cool-admin-go-next/cool/response"
   "github.com/toothdy/cool-admin-go-next/modules"
)

// Options 是应用启动选项。
type Options struct {
   StartServer bool
}

// Application 是 cool 应用实例。
type Application struct {
   server  *ghttp.Server
   modules []module.Module
}

/**
 * 创建应用实例
 * @param options 启动选项
 * @returns *Application
 */
func New(options Options) *Application {
   registeredModules := modules.Register()
   server := g.Server()

   application := &Application{
      server:  server,
      modules: registeredModules,
   }
   application.registerRoutes()

   return application
}

/**
 * 启动默认应用
 * @param ctx 上下文
 * @returns error
 */
func Run(ctx context.Context) error {
   return New(Options{StartServer: true}).Run(ctx)
}

/**
 * 当前模块列表
 * @returns []module.Module
 */
func (a *Application) Modules() []module.Module {
   return a.modules
}

/**
 * 健康检查
 * @param ctx 上下文
 * @returns response.Body
 */
func (a *Application) Health(ctx context.Context) response.Body {
   return response.OK(map[string]string{
      "status": "ok",
   })
}

/**
 * 运行应用
 * @param ctx 上下文
 * @returns error
 */
func (a *Application) Run(ctx context.Context) error {
   if a.server == nil {
      return nil
   }

   a.server.Run()
   return nil
}

/**
 * 注册基础路由
 * @returns null
 */
func (a *Application) registerRoutes() {
   a.server.BindHandler("/health", func(r *ghttp.Request) {
      r.Response.WriteJson(a.Health(r.Context()))
   })
}
```

- [ ] **Step 4: Implement main entry**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/main.go`:

```go
package main

import (
   "context"

   "github.com/gogf/gf/v2/frame/g"
   "github.com/toothdy/cool-admin-go-next/cool/app"
)

/**
 * 应用入口
 * @returns null
 */
func main() {
   if err := app.Run(context.Background()); err != nil {
      g.Log().Fatal(context.Background(), err)
   }
}
```

- [ ] **Step 5: Run app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/cool/app
```

- [ ] **Step 6: Run full Go tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 7: Commit Task 5**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go main.go
git commit -m "feat: add minimal app runtime" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 添加 README 和运行说明

**Files:**
- Create: `README.md`
- Test: `go test ./...`

**Interfaces:**
- Consumes: `main.go`, `/health`, config path, Plan0 protocol doc path.
- Produces: 人类可读启动说明。

- [ ] **Step 1: Write README**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/README.md`:

```markdown
# cool-admin-go-next

Go 版 cool-admin next，目标是用 GoFrame v2 实现与 Node 版 cool-admin-midway 兼容的后端体验。

## 当前阶段

当前仓库处于阶段 1：项目骨架和 `cool/app` runtime。

已完成：

1. Go module 初始化。
2. GoFrame v2 依赖。
3. `cool/module` 模块注册骨架。
4. `modules/base` 模块骨架。
5. `cool/response` Node 兼容响应结构。
6. `/health` 健康检查。

未完成：

1. MySQL 连接。
2. 自动建表。
3. `db.json` / `menu.json` 导入。
4. CRUD runtime。
5. 登录/JWT/权限。
6. EPS runtime。
7. Vue 前端联调。

## 协议契约

Go 版第一阶段必须以以下文档为准兼容现有前端：

```text
docs/protocol/base-api-contract.md
```

代表性响应 fixture 位于：

```text
docs/protocol/fixtures/
```

## 启动

```bash
go run .
```

默认监听：

```text
:8001
```

健康检查：

```bash
curl http://127.0.0.1:8001/health
```

期望响应：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "status": "ok"
  }
}
```

## 测试

```bash
go test ./...
```

## 远端仓库

```text
https://github.com/toothdy/cool-admin-go-next
```
```

- [ ] **Step 2: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Commit Task 6**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md
git commit -m "docs: add skeleton runtime readme" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 最终验证 Plan 1 产物

**Files:**
- No new files expected.
- Verify all files from Tasks 1-6.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: verified Plan 1 completion.

- [ ] **Step 1: Run full Go tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 2: Verify `/health` response by running server briefly**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
(go run . > /tmp/cool-admin-go-next.log 2>&1 & echo $! > /tmp/cool-admin-go-next.pid)
sleep 2
curl -s http://127.0.0.1:8001/health
kill $(cat /tmp/cool-admin-go-next.pid)
```

Expected curl output:

```json
{"code":1000,"message":"success","data":{"status":"ok"}}
```

If the JSON contains spaces or newlines, it is acceptable as long as fields match exactly.

- [ ] **Step 3: Verify no forbidden directories were created**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
test ! -d logic
test ! -d dao
test ! -d internal/model/do
test ! -d internal/model/entity
```

Expected: all commands exit `0`.

- [ ] **Step 4: Verify git status**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
```

Expected: empty output, except ignored `.superpowers/` must not appear.

- [ ] **Step 5: Update SDD ledger**

Append this line to `/Users/n/数据/cool-admin/cool-admin-go-next/.superpowers/sdd/progress.md`:

```text
Plan1: complete (GoFrame skeleton runtime, health check verified)
```

This file is ignored and must not be committed.

---

## Self-Review

### Spec coverage

This plan covers Plan 1 from the design doc:

1. GoFrame v2 project base: Task 1.
2. `cool/app` startup orchestration: Task 5.
3. `cool/module` module registry: Task 3.
4. `cool/response` unified response: Task 2.
5. Config loading structure: Task 1 creates config files; actual config reads happen in later plans.
6. MySQL config placeholder: Task 1.
7. Base module skeleton: Task 4.
8. `/health` route: Task 5 and Task 7.
9. README docs: Task 6.

It intentionally does not implement database, CRUD, auth, seed, or EPS runtime.

### Placeholder scan

The plan contains no TBD/TODO/fill-later markers. Empty `db.json` and `menu.json` are explicit placeholders for Plan 3 and are valid JSON files, not incomplete plan steps.

### Type consistency

The task interfaces consistently use:

1. `module.Module`.
2. `module.Config`.
3. `module.New`.
4. `module.Register`.
5. `module.List`.
6. `response.Body`.
7. `response.OK`.
8. `response.Fail`.
9. `app.Options`.
10. `app.Application`.
11. `app.New`.
12. `app.Run`.
