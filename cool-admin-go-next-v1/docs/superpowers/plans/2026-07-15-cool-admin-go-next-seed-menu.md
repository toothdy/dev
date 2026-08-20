# cool-admin-go-next Seed/Menu Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Plan3 seed/menu 初始化导入，让 `cool-admin-go-next` 在 Plan2 自动建表后，可以导入 base 模块默认用户、角色、部门、系统参数、菜单和权限关系数据。

**Architecture:** 使用 `cool/module` 声明模块 seed 文件，使用 `cool/seed` 读取 `db.json` / `menu.json`，通过 `cool/model` 完成 camelCase → snake_case 字段映射，使用 GoFrame gdb 事务和参数化 SQL 写入 MySQL，并通过 `base_sys_conf` 的 `init_db_base` / `init_menu_base` 标记实现幂等。

**Tech Stack:** Go 1.23+、GoFrame v2.10.2、GoFrame MySQL driver、MySQL 8.x、标准库 `context` / `encoding/json` / `os` / `path/filepath` / `time` / `strings` / `fmt`。

## Global Constraints

- 始终用中文编写说明文档和代码注释。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL。
- Plan3 必须建立在 Plan2 已完成的 model metadata 与 schema sync 上。
- Plan3 验收数据库连接：`root` / `123456` / `cool-go` / `127.0.0.1:3306`。
- 初始化导入必须在 schema sync 之后执行。
- 初始化导入必须使用事务；失败时回滚本次导入和初始化标记。
- 初始化导入必须通过 `base_sys_conf` 标记幂等，不能重复插入。
- 动态 SQL 的表名和字段名必须来自 `cool/model` 元数据，值必须参数绑定。
- 不使用 `git add -A`；提交时只显式 stage 本计划创建或修改的文件。
- GoFrame 自动生成文件后续必须由工具生成，不手写、不手改；本计划不创建 `dao/`、`internal/model/do/`、`internal/model/entity/`。
- 不使用 `logic/` 目录，业务逻辑直接放在 `cool/seed` 和后续 `service/`。
- Go 代码错误处理使用 GoFrame `gerror` 包装上下文。
- Go 文件内如果有 3 个及以上相关变量声明，使用 `var (...)` 分组。
- GoFrame 事务优先使用 `db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error { ... })` 或当前版本等价闭包写法。

---

## Scope Check

本计划只覆盖设计文档中的“阶段 3：seed/menu 导入”。

包含：

1. `cool/seed` seed 元数据、解析、映射、导入器。
2. `cool/module` 扩展 seed 声明。
3. `modules/base/db.json` 真实初始化数据。
4. `modules/base/menu.json` 真实菜单和按钮权限数据。
5. `cool/app` seed/menu 启动 hook。
6. 真实 MySQL seed/menu 验收命令和 README 更新。

不包含：

1. 登录接口。
2. JWT。
3. token middleware。
4. 权限菜单接口。
5. CRUD runtime。
6. EPS runtime。
7. Vue 前端联调。
8. 自动覆盖已有业务数据。
9. 删除或更新用户已有数据。

---

## File Structure

### 创建文件

- `cool/seed/definition.go`  
  seed 文件声明、导入选项、结果类型、标记 key helper。

- `cool/seed/definition_test.go`  
  测试 seed 定义、标记 key、默认配置。

- `cool/seed/mapper.go`  
  根据 `model.Definition` 将 JSON 记录映射为 DB 字段和值。

- `cool/seed/mapper_test.go`  
  测试字段映射、未知表、未知字段、控制字段过滤、父级引用。

- `cool/seed/parser.go`  
  解析 `db.json` 和 `menu.json`，展开 `@childDatas` / `childMenus`。

- `cool/seed/parser_test.go`  
  测试 DB seed 和 menu seed 的递归展开。

- `cool/seed/importer.go`  
  MySQL seed importer：检查标记、事务插入、写入标记。

- `cool/seed/importer_test.go`  
  不依赖真实 MySQL 的 importer helper 测试；真实 MySQL 集成测试默认跳过。

- `cool/seed/integration_test.go`  
  `COOL_SEED_INTEGRATION=1` 时执行真实 MySQL seed/menu 验收。

### 修改文件

- `cool/module/module.go`  
  扩展模块接口，增加 `ModuleSeeds() seed.Definition`。

- `cool/module/module_test.go`  
  测试模块 seed 文件声明。

- `modules/base/base.go`  
  base 模块挂载 `Seeds("modules/base/db.json", "modules/base/menu.json")`。

- `modules/base/base_test.go`  
  验证 base 模块 seed 文件路径。

- `modules/base/db.json`  
  替换占位内容，写入 base 默认初始化数据。

- `modules/base/menu.json`  
  替换占位内容，写入 base 默认菜单和按钮权限。

- `cool/app/app.go`  
  增加 seed runner、配置读取和启动阶段 seed hook。

- `cool/app/app_test.go`  
  测试 seed runner 调用、配置关闭跳过、schema 在 seed 前执行。

- `README.md`  
  更新当前阶段和 seed/menu 验收说明。

### 不创建文件

- 不创建 `dao/`。
- 不创建 `internal/model/do/`。
- 不创建 `internal/model/entity/`。
- 不创建 `logic/`。
- 不创建 auth、CRUD、EPS 实现。

---

## Implementation Tasks

### Task 1: 增加 `cool/seed` 定义并扩展 module seed 声明

**Files:**
- Create: `cool/seed/definition.go`
- Create: `cool/seed/definition_test.go`
- Modify: `cool/module/module.go`
- Modify: `cool/module/module_test.go`
- Modify: `modules/base/base.go`
- Modify: `modules/base/base_test.go`
- Test: `go test ./cool/seed ./cool/module ./modules/base`

**Interfaces:**
- Consumes:
  - `module.New(key string) *module.Definition`
- Produces:
  - `type seed.Definition struct`
  - `type seed.Kind string`
  - `const seed.KindDB seed.Kind`
  - `const seed.KindMenu seed.Kind`
  - `func NewDefinition(dbPath string, menuPath string) Definition`
  - `func MarkerKey(kind Kind, moduleName string) string`
  - `func (d *module.Definition) Seeds(dbPath string, menuPath string) *module.Definition`
  - `func (d *module.Definition) ModuleSeeds() seed.Definition`

- [ ] **Step 1: Write failing seed definition tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/seed
```

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/definition_test.go`:

```go
package seed_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/seed"
)

func TestNewDefinition(t *testing.T) {
   definition := seed.NewDefinition("modules/base/db.json", "modules/base/menu.json")
   if definition.DBPath != "modules/base/db.json" {
      t.Fatalf("unexpected db path: %s", definition.DBPath)
   }
   if definition.MenuPath != "modules/base/menu.json" {
      t.Fatalf("unexpected menu path: %s", definition.MenuPath)
   }
}

func TestMarkerKey(t *testing.T) {
   if seed.MarkerKey(seed.KindDB, "base") != "init_db_base" {
      t.Fatalf("unexpected db marker key")
   }
   if seed.MarkerKey(seed.KindMenu, "base") != "init_menu_base" {
      t.Fatalf("unexpected menu marker key")
   }
}
```

- [ ] **Step 2: Run failing seed tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed
```

Expected: FAIL because `cool/seed` implementation does not exist.

- [ ] **Step 3: Implement seed definition**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/definition.go`:

```go
package seed

import "fmt"

// Kind 是初始化数据类型。
type Kind string

const (
   // KindDB 表示 db.json 初始化数据。
   KindDB Kind = "db"
   // KindMenu 表示 menu.json 初始化数据。
   KindMenu Kind = "menu"
)

// Definition 是模块 seed 文件声明。
type Definition struct {
   DBPath   string
   MenuPath string
}

/**
 * 创建 seed 定义
 * @param dbPath DB 初始化文件路径
 * @param menuPath 菜单初始化文件路径
 * @returns Definition
 */
func NewDefinition(dbPath string, menuPath string) Definition {
   return Definition{
      DBPath:   dbPath,
      MenuPath: menuPath,
   }
}

/**
 * 生成初始化标记键
 * @param kind 初始化类型
 * @param moduleName 模块名
 * @returns string
 */
func MarkerKey(kind Kind, moduleName string) string {
   return fmt.Sprintf("init_%s_%s", kind, moduleName)
}
```

- [ ] **Step 4: Extend module tests**

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/module/module_test.go`:

```go
func TestDefinitionSeeds(t *testing.T) {
   mod := module.New("base").Seeds("modules/base/db.json", "modules/base/menu.json")
   seeds := mod.ModuleSeeds()

   if seeds.DBPath != "modules/base/db.json" {
      t.Fatalf("unexpected db seed path: %s", seeds.DBPath)
   }
   if seeds.MenuPath != "modules/base/menu.json" {
      t.Fatalf("unexpected menu seed path: %s", seeds.MenuPath)
   }
}
```

Add import:

```go
"github.com/toothdy/cool-admin-go-next/cool/seed"
```

Only keep the import if the file references `seed` directly; otherwise let `goimports` remove it.

- [ ] **Step 5: Modify `cool/module/module.go`**

Update `cool/module/module.go`:

1. Import `github.com/toothdy/cool-admin-go-next/cool/seed`.
2. Add `ModuleSeeds() seed.Definition` to `Module` interface.
3. Add `seeds seed.Definition` field to `Definition`.
4. Add method:

```go
/**
 * 设置模块 seed 文件
 * @param dbPath DB 初始化文件路径
 * @param menuPath 菜单初始化文件路径
 * @returns *Definition
 */
func (d *Definition) Seeds(dbPath string, menuPath string) *Definition {
   d.seeds = seed.NewDefinition(dbPath, menuPath)
   return d
}

/**
 * 模块 seed 定义
 * @returns seed.Definition
 */
func (d *Definition) ModuleSeeds() seed.Definition {
   return d.seeds
}
```

- [ ] **Step 6: Attach seeds to base module**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base.go`:

```go
return module.New("base").
   Name("基础模块").
   Config(NewConfig()).
   Models(baseModel.Register()).
   Seeds("modules/base/db.json", "modules/base/menu.json")
```

Append to `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base_test.go`:

```go
func TestNewModuleSeeds(t *testing.T) {
   mod := base.NewModule()
   seeds := mod.ModuleSeeds()

   if seeds.DBPath != "modules/base/db.json" {
      t.Fatalf("unexpected db seed path: %s", seeds.DBPath)
   }
   if seeds.MenuPath != "modules/base/menu.json" {
      t.Fatalf("unexpected menu seed path: %s", seeds.MenuPath)
   }
}
```

- [ ] **Step 7: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed ./cool/module ./modules/base
```

Expected: all packages pass.

- [ ] **Step 8: Commit Task 1**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/seed/definition.go cool/seed/definition_test.go cool/module/module.go cool/module/module_test.go modules/base/base.go modules/base/base_test.go
git commit -m "feat: add seed metadata" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 实现 seed 字段映射和 JSON 解析

**Files:**
- Create: `cool/seed/mapper.go`
- Create: `cool/seed/mapper_test.go`
- Create: `cool/seed/parser.go`
- Create: `cool/seed/parser_test.go`
- Test: `go test ./cool/seed`

**Interfaces:**
- Consumes:
  - `model.Definition`
  - `model.Field`
- Produces:
  - `type RawRecord map[string]interface{}`
  - `type MappedRecord struct`
  - `type ModelMap map[string]model.Definition`
  - `func NewModelMap(definitions []model.Definition) ModelMap`
  - `func MapRecord(models ModelMap, tableName string, record RawRecord, parent RawRecord) (MappedRecord, error)`
  - `func ParseDBContent(data []byte, models ModelMap) ([]MappedRecord, error)`
  - `func ParseMenuContent(data []byte, menuDefinition model.Definition) ([]MappedRecord, error)`
  - `func LoadDBFile(path string, models ModelMap) ([]MappedRecord, error)`
  - `func LoadMenuFile(path string, menuDefinition model.Definition) ([]MappedRecord, error)`

- [ ] **Step 1: Write failing mapper tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/mapper_test.go`:

```go
package seed_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/model"
   "github.com/toothdy/cool-admin-go-next/cool/seed"
)

func testUserDefinition() model.Definition {
   return model.NewDefinition("base", "BaseSysUser", "base_sys_user").
      Fields([]model.Field{
         model.NewField("id", "id", "bigint"),
         model.NewField("username", "username", "varchar"),
         model.NewField("passwordV", "password_v", "int"),
         model.NewField("parentId", "parent_id", "bigint"),
      })
}

func TestMapRecordConvertsJsonFieldsToColumns(t *testing.T) {
   models := seed.NewModelMap([]model.Definition{testUserDefinition()})
   mapped, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
      "id":        float64(1),
      "username":  "admin",
      "passwordV": float64(1),
   }, nil)
   if err != nil {
      t.Fatalf("map record failed: %v", err)
   }

   if mapped.TableName != "base_sys_user" {
      t.Fatalf("unexpected table: %s", mapped.TableName)
   }
   if mapped.Values["password_v"] != float64(1) {
      t.Fatalf("expected password_v to be mapped, got %#v", mapped.Values)
   }
}

func TestMapRecordRejectsUnknownField(t *testing.T) {
   models := seed.NewModelMap([]model.Definition{testUserDefinition()})
   _, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
      "unknown": "value",
   }, nil)
   if err == nil {
      t.Fatal("expected unknown field error")
   }
}

func TestMapRecordResolvesParentReference(t *testing.T) {
   models := seed.NewModelMap([]model.Definition{testUserDefinition()})
   mapped, err := seed.MapRecord(models, "base_sys_user", seed.RawRecord{
      "parentId": "@id",
   }, seed.RawRecord{
      "id": float64(9),
   })
   if err != nil {
      t.Fatalf("map record failed: %v", err)
   }
   if mapped.Values["parent_id"] != float64(9) {
      t.Fatalf("expected parent reference value 9, got %#v", mapped.Values["parent_id"])
   }
}
```

- [ ] **Step 2: Implement mapper**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/mapper.go`.

Required behavior:

1. `NewModelMap` maps `definition.TableName` to definition.
2. `MapRecord` rejects unknown table.
3. `MapRecord` skips control keys `@childDatas` and `childMenus`.
4. `MapRecord` resolves string values in format `@fieldName` from direct parent record.
5. `MapRecord` maps JSON field names to DB column names using `definition.FieldsValue`.
6. Unknown JSON field returns `gerror.Newf` or wrapped error with table and field.
7. `MappedRecord` stores:

```go
type MappedRecord struct {
   TableName string
   Values    map[string]interface{}
}
```

- [ ] **Step 3: Write failing parser tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/parser_test.go`:

```go
package seed_test

import (
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/model"
   "github.com/toothdy/cool-admin-go-next/cool/seed"
)

func TestParseDBContentExpandsChildDatas(t *testing.T) {
   definition := model.NewDefinition("base", "BaseSysDepartment", "base_sys_department").
      Fields([]model.Field{
         model.NewField("id", "id", "bigint"),
         model.NewField("name", "name", "varchar"),
         model.NewField("parentId", "parent_id", "bigint"),
      })
   models := seed.NewModelMap([]model.Definition{definition})

   records, err := seed.ParseDBContent([]byte(`{
      "base_sys_department": [
         {
            "id": 1,
            "name": "COOL",
            "@childDatas": {
               "base_sys_department": [
                  {"id": 2, "name": "开发", "parentId": "@id"}
               ]
            }
         }
      ]
   }`), models)
   if err != nil {
      t.Fatalf("parse db content failed: %v", err)
   }
   if len(records) != 2 {
      t.Fatalf("expected 2 records, got %d", len(records))
   }
   if records[1].Values["parent_id"] != float64(1) {
      t.Fatalf("expected child parent_id 1, got %#v", records[1].Values["parent_id"])
   }
}

func TestParseMenuContentExpandsChildMenus(t *testing.T) {
   menuDefinition := model.NewDefinition("base", "BaseSysMenu", "base_sys_menu").
      Fields([]model.Field{
         model.NewField("id", "id", "bigint"),
         model.NewField("name", "name", "varchar"),
         model.NewField("parentId", "parent_id", "bigint"),
         model.NewField("type", "type", "tinyint"),
      })

   records, err := seed.ParseMenuContent([]byte(`[
      {
         "id": 1,
         "name": "系统管理",
         "type": 0,
         "childMenus": [
            {"id": 2, "name": "用户管理", "parentId": "@id", "type": 1}
         ]
      }
   ]`), menuDefinition)
   if err != nil {
      t.Fatalf("parse menu content failed: %v", err)
   }
   if len(records) != 2 {
      t.Fatalf("expected 2 records, got %d", len(records))
   }
   if records[1].Values["parent_id"] != float64(1) {
      t.Fatalf("expected menu parent_id 1, got %#v", records[1].Values["parent_id"])
   }
}
```

- [ ] **Step 4: Implement parser**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/parser.go`.

Required behavior:

1. `ParseDBContent` unmarshals root object `map[string][]RawRecord`.
2. For each table group, recursively append parent record before child records.
3. `@childDatas` type must be `map[string]interface{}` containing table arrays.
4. `ParseMenuContent` unmarshals root array `[]RawRecord`.
5. `ParseMenuContent` treats all records as table `base_sys_menu`.
6. `childMenus` type must be array.
7. `LoadDBFile` / `LoadMenuFile` use `os.ReadFile` and wrap path errors.

- [ ] **Step 5: Run seed parser tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed
```

Expected: all seed unit tests pass.

- [ ] **Step 6: Commit Task 2**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/seed/mapper.go cool/seed/mapper_test.go cool/seed/parser.go cool/seed/parser_test.go
git commit -m "feat: parse seed files" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 实现 seed importer 和幂等标记

**Files:**
- Create: `cool/seed/importer.go`
- Create: `cool/seed/importer_test.go`
- Create: `cool/seed/integration_test.go`
- Test: `go test ./cool/seed`

**Interfaces:**
- Consumes:
  - `seed.MappedRecord`
  - `seed.LoadDBFile`
  - `seed.LoadMenuFile`
  - `seed.MarkerKey`
  - `model.Definition`
- Produces:
  - `type Importer struct`
  - `type ImportOptions struct`
  - `type ImportResult struct`
  - `func NewImporter(db gdb.DB, definitions []model.Definition) *Importer`
  - `func (i *Importer) ImportDB(ctx context.Context, moduleName string, path string) (ImportResult, error)`
  - `func (i *Importer) ImportMenu(ctx context.Context, moduleName string, path string) (ImportResult, error)`
  - `func InsertSQL(record MappedRecord) (string, []interface{})`

- [ ] **Step 1: Write SQL builder tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/importer_test.go`:

```go
package seed_test

import (
   "strings"
   "testing"

   "github.com/toothdy/cool-admin-go-next/cool/seed"
)

func TestInsertSQLUsesParameters(t *testing.T) {
   sql, args := seed.InsertSQL(seed.MappedRecord{
      TableName: "base_sys_user",
      Values: map[string]interface{}{
         "id":       float64(1),
         "username": "admin",
      },
   })

   if !strings.HasPrefix(sql, "INSERT INTO `base_sys_user`") {
      t.Fatalf("unexpected insert sql: %s", sql)
   }
   if !strings.Contains(sql, "VALUES (?, ?)") {
      t.Fatalf("expected parameter placeholders, got %s", sql)
   }
   if len(args) != 2 {
      t.Fatalf("expected 2 args, got %d", len(args))
   }
}
```

- [ ] **Step 2: Implement importer SQL helper**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/importer.go` with at least:

1. `InsertSQL(record MappedRecord) (string, []interface{})`.
2. Stable column ordering by sorting column names to keep tests deterministic.
3. Identifier quoting implemented inside `cool/seed` and only called for metadata-derived names.

- [ ] **Step 3: Implement Importer**

Extend `importer.go`:

```go
type ImportResult struct {
   ModuleName      string
   Kind            Kind
   InsertedRecords int
   Skipped         bool
   MarkerKey       string
}

type Importer struct {
   db          gdb.DB
   models      ModelMap
   definitions []model.Definition
}
```

Required methods:

1. `NewImporter(db gdb.DB, definitions []model.Definition) *Importer`.
2. `ImportDB(ctx, moduleName, path)`:
   - marker key = `init_db_<module>`。
   - marker exists → return skipped。
   - parse db file。
   - transaction insert all records。
   - write marker。
3. `ImportMenu(ctx, moduleName, path)`:
   - marker key = `init_menu_<module>`。
   - marker exists → return skipped。
   - find `base_sys_menu` definition。
   - parse menu file。
   - transaction insert menu records。
   - grant all menu IDs to admin role in `base_sys_role_menu`。
   - optionally grant all departments to admin role in `base_sys_role_department` when departments exist。
   - write marker。
4. `markerExists(ctx, txOrDB, markerKey)` uses `base_sys_conf`.
5. `writeMarker(ctx, txOrDB, markerKey, duration)` inserts into `base_sys_conf`.

Notes:

- If GoFrame `gdb.TX` interface shape differs in this version, adapt to the actual v2.10.2 API after checking docs/source.
- Use `gerror.Wrap` around file parse, marker check, insert and marker write errors.
- Do not use GoFrame generated DO files.

- [ ] **Step 4: Add integration tests**

Create `/Users/n/数据/cool-admin/cool-admin-go-next/cool/seed/integration_test.go`.

Required tests:

1. If `COOL_SEED_INTEGRATION != "1"`, skip.
2. Use `_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"`.
3. Use `g.DB()`.
4. Run schema sync for base models before seed.
5. Clean only Plan3-owned tables or use a dedicated cleanup helper:
   - Delete `base_sys_role_menu`。
   - Delete `base_sys_role_department`。
   - Delete `base_sys_user_role`。
   - Delete `base_sys_menu`。
   - Delete `base_sys_user`。
   - Delete `base_sys_role`。
   - Delete `base_sys_department`。
   - Delete `base_sys_param`。
   - Delete seed-related `base_sys_conf` keys。
6. Import DB and Menu.
7. Assert key records exist.
8. Import DB and Menu again.
9. Assert second run is skipped.

Keep integration tests behind env flag so `go test ./...` passes without MySQL.

- [ ] **Step 5: Run unit tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed
```

Expected: unit tests pass; integration tests skipped.

- [ ] **Step 6: Commit Task 3**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/seed/importer.go cool/seed/importer_test.go cool/seed/integration_test.go
git commit -m "feat: add seed importer" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 填充 base `db.json` 默认初始化数据

**Files:**
- Modify: `modules/base/db.json`
- Modify: `cool/seed/parser_test.go` or add `modules/base/base_seed_test.go`
- Test: `go test ./cool/seed ./modules/base`

**Interfaces:**
- Consumes:
  - `seed.LoadDBFile`
  - `modules/base/model.Register()`
- Produces:
  - Real `modules/base/db.json`

- [ ] **Step 1: Replace `modules/base/db.json`**

Replace the current placeholder with table-grouped JSON.

Minimum required data:

```json
{
  "base_sys_department": [
    {
      "id": 1,
      "name": "COOL",
      "parentId": null,
      "orderNum": 0,
      "@childDatas": {
        "base_sys_department": [
          { "id": 2, "name": "开发", "parentId": "@id", "orderNum": 1 },
          { "id": 3, "name": "测试", "parentId": "@id", "orderNum": 2 },
          { "id": 4, "name": "游客", "parentId": "@id", "orderNum": 3 }
        ]
      }
    }
  ],
  "base_sys_role": [
    {
      "id": 1,
      "userId": 1,
      "name": "超管",
      "label": "admin",
      "remark": "最高权限角色",
      "relevance": 1,
      "menuIdList": null,
      "departmentIdList": null
    }
  ],
  "base_sys_user": [
    {
      "id": 1,
      "departmentId": 1,
      "userId": null,
      "name": "管理员",
      "username": "admin",
      "password": "e10adc3949ba59abbe56e057f20f883e",
      "passwordV": 1,
      "nickName": "管理员",
      "headImg": "",
      "phone": "",
      "email": "",
      "remark": "系统默认管理员",
      "status": 1,
      "socketId": ""
    }
  ],
  "base_sys_user_role": [
    { "userId": 1, "roleId": 1 }
  ],
  "base_sys_param": [
    {
      "id": 1,
      "keyName": "siteName",
      "name": "站点名称",
      "data": "cool-admin-go-next",
      "dataType": 0,
      "remark": "默认站点名称"
    }
  ],
  "base_sys_conf": [
    { "id": 1, "cKey": "logKeep", "cValue": "31" },
    { "id": 2, "cKey": "recycleKeep", "cValue": "31" }
  ]
}
```

If Node 版真实初始化数据已可查到，应优先以 Node 版为准补齐更多字段和默认参数。

- [ ] **Step 2: Add base seed fixture test**

Create or append a test that loads actual `modules/base/db.json` with `modules/base/model.Register()` and asserts:

1. File parses successfully.
2. At least 12 mapped records exist.
3. `base_sys_user` contains `username = admin`.
4. `base_sys_role` contains `label = admin`.
5. No unknown field errors.

- [ ] **Step 3: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed ./modules/base
```

Expected: pass.

- [ ] **Step 4: Commit Task 4**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/db.json cool/seed/parser_test.go
git commit -m "feat: add base db seed data" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

If the fixture test is placed in a different file, stage that file explicitly instead of `cool/seed/parser_test.go`.

---

### Task 5: 填充 base `menu.json` 默认菜单和权限数据

**Files:**
- Modify: `modules/base/menu.json`
- Modify: `cool/seed/parser_test.go` or add `modules/base/base_seed_test.go`
- Test: `go test ./cool/seed ./modules/base`

**Interfaces:**
- Consumes:
  - `seed.LoadMenuFile`
  - `modules/base/model.BaseSysMenu()`
- Produces:
  - Real `modules/base/menu.json`

- [ ] **Step 1: Replace `modules/base/menu.json`**

Replace placeholder with menu tree.

Minimum required menu groups:

1. 系统管理目录。
2. 用户管理页面。
3. 角色管理页面。
4. 菜单管理页面。
5. 部门管理页面。
6. 参数管理页面。
7. 操作日志页面。

Each page should include button permission records with `type = 2`.

Required permission strings:

```text
base:sys:user:add
base:sys:user:delete
base:sys:user:update
base:sys:user:info
base:sys:user:list
base:sys:user:page
base:sys:user:move
base:sys:role:add
base:sys:role:delete
base:sys:role:update
base:sys:role:info
base:sys:role:list
base:sys:role:page
base:sys:menu:add
base:sys:menu:delete
base:sys:menu:update
base:sys:menu:info
base:sys:menu:list
base:sys:menu:page
base:sys:menu:parse
base:sys:menu:create
base:sys:menu:export
base:sys:menu:import
base:sys:department:add
base:sys:department:delete
base:sys:department:update
base:sys:department:list
base:sys:department:order
base:sys:param:add
base:sys:param:delete
base:sys:param:update
base:sys:param:info
base:sys:param:page
base:sys:param:html
base:sys:log:page
base:sys:log:clear
base:sys:log:setKeep
base:sys:log:getKeep
```

Recommended ID allocation:

- `1`: 系统管理。
- `100-199`: 用户管理及按钮。
- `200-299`: 角色管理及按钮。
- `300-399`: 菜单管理及按钮。
- `400-499`: 部门管理及按钮。
- `500-599`: 参数管理及按钮。
- `600-699`: 操作日志及按钮。

- [ ] **Step 2: Add menu fixture test**

Add test that loads actual `modules/base/menu.json` and asserts:

1. File parses successfully.
2. Records count is greater than 30.
3. There is at least one `type = 0` directory.
4. There is at least one `type = 1` page.
5. There are `type = 2` button permissions.
6. Required permissions above are present.
7. No unknown field errors.

- [ ] **Step 3: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/seed ./modules/base
```

Expected: pass.

- [ ] **Step 4: Commit Task 5**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/menu.json cool/seed/parser_test.go
git commit -m "feat: add base menu seed data" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

If the fixture test is placed in a different file, stage that file explicitly instead of `cool/seed/parser_test.go`.

---

### Task 6: 接入 app seed/menu 启动 hook

**Files:**
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Test: `go test ./cool/app`

**Interfaces:**
- Consumes:
  - `module.Module.ModuleSeeds() seed.Definition`
  - `seed.NewImporter(g.DB(), definitions)`
- Produces:
  - `type SeedRunner func(ctx context.Context, modules []module.Module, definitions []model.Definition) error`
  - `Options.AutoInitDB bool`
  - `Options.AutoInitMenu bool`
  - `Options.UseConfigInit bool`
  - `Options.SeedRunner SeedRunner`
  - `func (a *Application) InitSeed(ctx context.Context) error`
  - `func (a *Application) InitDBEnabled(ctx context.Context) bool`
  - `func (a *Application) InitMenuEnabled(ctx context.Context) bool`

- [ ] **Step 1: Write failing app tests**

Append tests to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app_test.go`:

```go
func TestRunCallsSchemaBeforeSeed(t *testing.T) {
   calls := []string{}
   application := app.New(app.Options{
      StartServer:    false,
      AutoSyncSchema: true,
      AutoInitDB:     true,
      AutoInitMenu:   true,
      SchemaSyncRunner: func(ctx context.Context, definitions []model.Definition) error {
         calls = append(calls, "schema")
         return nil
      },
      SeedRunner: func(ctx context.Context, modules []module.Module, definitions []model.Definition) error {
         calls = append(calls, "seed")
         return nil
      },
   })

   if err := application.Run(context.Background()); err != nil {
      t.Fatalf("run failed: %v", err)
   }
   if len(calls) != 2 || calls[0] != "schema" || calls[1] != "seed" {
      t.Fatalf("expected schema then seed, got %#v", calls)
   }
}

func TestRunSkipsSeedWhenConfigDisabled(t *testing.T) {
   adapter, err := gcfg.NewAdapterContent(`cool:
  initDB: false
  initMenu: false
  schema:
    autoSync: false`)
   if err != nil {
      t.Fatalf("create config adapter failed: %v", err)
   }
   config := g.Cfg()
   previousAdapter := config.GetAdapter()
   config.SetAdapter(adapter)
   t.Cleanup(func() {
      config.SetAdapter(previousAdapter)
   })

   called := false
   application := app.New(app.Options{
      StartServer:       false,
      UseConfigAutoSync: true,
      UseConfigInit:     true,
      SeedRunner: func(ctx context.Context, modules []module.Module, definitions []model.Definition) error {
         called = true
         return nil
      },
   })

   if err := application.Run(context.Background()); err != nil {
      t.Fatalf("run failed: %v", err)
   }
   if called {
      t.Fatal("expected seed runner not to be called")
   }
}
```

Add import:

```go
"github.com/toothdy/cool-admin-go-next/cool/module"
```

- [ ] **Step 2: Modify app options and runtime**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app.go`:

1. Import `github.com/toothdy/cool-admin-go-next/cool/seed`.
2. Add type:

```go
type SeedRunner func(ctx context.Context, modules []module.Module, definitions []model.Definition) error
```

3. Extend `Options`:

```go
AutoInitDB    bool
AutoInitMenu  bool
UseConfigInit bool
SeedRunner    SeedRunner
```

4. Extend `Application`:

```go
seedRunner    SeedRunner
autoInitDB    bool
autoInitMenu  bool
useConfigInit bool
seedInitialized bool
```

5. In `NewWithContext`, default `SeedRunner` to `defaultSeedRunner`.
6. In `Run`, execute schema sync before seed, and seed before server run.
7. Avoid duplicate seed execution using `seedInitialized`.
8. Add config helpers reading:
   - `cool.initDB`
   - `cool.initMenu`

- [ ] **Step 3: Implement default seed runner**

Add:

```go
func defaultSeedRunner(ctx context.Context, modules []module.Module, definitions []model.Definition) error {
   importer := seed.NewImporter(g.DB(), definitions)
   for _, mod := range modules {
      seeds := mod.ModuleSeeds()
      if seeds.DBPath != "" {
         if _, err := importer.ImportDB(ctx, mod.Key(), seeds.DBPath); err != nil {
            return err
         }
      }
      if seeds.MenuPath != "" {
         if _, err := importer.ImportMenu(ctx, mod.Key(), seeds.MenuPath); err != nil {
            return err
         }
      }
   }
   return nil
}
```

If config disables only DB or only Menu, either:

1. Pass flags into importer via app-level runner, or
2. Implement `InitSeed(ctx)` in app and call ImportDB/ImportMenu conditionally.

Prefer option 2 so `cool.initDB` and `cool.initMenu` can independently control each file.

- [ ] **Step 4: Run app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app
```

Expected: pass.

- [ ] **Step 5: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: pass; integration tests skipped.

- [ ] **Step 6: Commit Task 6**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go
git commit -m "feat: wire seed import into app" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 真实 MySQL 验收、README 更新和最终验证

**Files:**
- Modify: `README.md`
- Modify: `todo.md` only if maintaining local task notes for this phase
- Test: `go test ./...`
- Integration Test: `COOL_SEED_INTEGRATION=1 go test ./cool/seed -count=1`
- Runtime Test: `go run .` + `/health`

**Interfaces:**
- Consumes:
  - All previous Plan3 tasks.
- Produces:
  - README seed/menu 验收说明。
  - Verified Plan3 state.

- [ ] **Step 1: Update README current stage**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/README.md`:

1. 当前阶段改为阶段 3：seed/menu 初始化导入。
2. 已完成列表加入：
   - `cool/seed` 初始化导入器。
   - `modules/base/db.json` 默认数据。
   - `modules/base/menu.json` 默认菜单。
   - app 启动 seed hook。
3. 未完成列表移除 `db.json / menu.json 导入`。
4. 增加 seed/menu 验收命令。

- [ ] **Step 2: Run unit tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: pass.

- [ ] **Step 3: Run real MySQL seed integration test**

Ensure database exists:

```sql
CREATE DATABASE `cool-go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1
COOL_SEED_INTEGRATION=1 go test ./cool/seed -count=1
```

Expected: pass.

- [ ] **Step 4: Run app and verify `/health`**

Run app:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go run .
```

In another shell:

```bash
curl -s http://127.0.0.1:8001/health
```

Expected:

```json
{"code":1000,"message":"success","data":{"status":"ok"}}
```

Stop the app after verification.

- [ ] **Step 5: Verify database records**

Use Go integration test assertions or a read-only SQL client to verify:

```sql
SELECT username, password, status FROM base_sys_user WHERE username = 'admin';
SELECT label FROM base_sys_role WHERE label = 'admin';
SELECT COUNT(*) FROM base_sys_user_role WHERE user_id = 1 AND role_id = 1;
SELECT name FROM base_sys_department WHERE name IN ('COOL', '开发', '测试', '游客');
SELECT c_key FROM base_sys_conf WHERE c_key IN ('logKeep', 'recycleKeep', 'init_db_base', 'init_menu_base');
SELECT COUNT(*) FROM base_sys_menu;
SELECT COUNT(*) FROM base_sys_role_menu WHERE role_id = 1;
```

Expected:

1. admin user exists.
2. admin password equals `e10adc3949ba59abbe56e057f20f883e`.
3. admin role exists.
4. user-role relation exists.
5. required departments exist.
6. required config and init markers exist.
7. menu count is greater than 30.
8. admin role has menu bindings.

- [ ] **Step 6: Run final status check**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git status --short
```

Expected: only Plan3 files changed.

- [ ] **Step 7: Commit Task 7**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md
git commit -m "docs: document seed import" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

If `todo.md` was updated and should be committed, stage it explicitly:

```bash
git add todo.md
```

Do not use `git add -A`.

---

## Final Plan3 Acceptance Checklist

- [ ] `go test ./...` passes.
- [ ] `COOL_SEED_INTEGRATION=1 go test ./cool/seed -count=1` passes against real MySQL.
- [ ] App startup runs schema sync before seed/menu.
- [ ] `base_sys_user.username = admin` exists.
- [ ] admin password is `e10adc3949ba59abbe56e057f20f883e`.
- [ ] `base_sys_role.label = admin` exists.
- [ ] admin user-role relation exists.
- [ ] base departments exist: `COOL`、`开发`、`测试`、`游客`。
- [ ] `base_sys_conf` contains `logKeep` and `recycleKeep`.
- [ ] `base_sys_conf` contains `init_db_base` and `init_menu_base`.
- [ ] `base_sys_menu` contains base module page and button permissions.
- [ ] `base_sys_role_menu` grants admin role all initialized menus.
- [ ] Running seed twice skips the second run without duplicate insert.
- [ ] `/health` still returns Node-compatible success body.
- [ ] No `dao/`、`internal/model/do/`、`internal/model/entity/`、`logic/` directories were created.
- [ ] Working tree only contains Plan3-related changes before final commits.
