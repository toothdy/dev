# cool-admin-go-next Schema Sync Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Model metadata 与真实 MySQL 自动建表，让 `cool-admin-go-next` 能为 base 模块创建第一批表结构。

**Architecture:** 使用 `cool/model` 表达表、字段、索引元数据；使用 `cool/db/schema` 将元数据转换为 MySQL DDL，并通过 GoFrame gdb 查询 `information_schema` 与执行安全同步。`modules/base/model` 注册 base 表定义，`cool/app` 提供 schema sync 启动 hook。

**Tech Stack:** Go 1.23+、GoFrame v2.10.2、GoFrame MySQL driver、MySQL 8.x、标准库 `context` / `strings` / `fmt`。

## Global Constraints

- 始终用中文编写说明文档和代码注释。
- Go 版第一阶段必须做到现有 `cool-admin-vue` 前端不改业务代码即可接入。
- 第一阶段只支持 MySQL。
- Plan2 必须真实连接 MySQL 并实际创建 base 表。
- Plan2 验收数据库连接：`root` / `123456` / `cool-go` / `127.0.0.1:3306`。
- 第一阶段采用运行时自动建表、运行时 EPS、`db.json` / `menu.json` 初始化导入；Plan2 只实现自动建表。
- 第一阶段插件系统不实现，只预留扩展点。
- 目标本地目录是 `/Users/n/数据/cool-admin/cool-admin-go-next`。
- 后续远端仓库是 `https://github.com/toothdy/cool-admin-go-next`。
- 不使用 `git add -A`；提交时只显式 stage 本计划创建或修改的文件。
- GoFrame 自动生成文件后续必须由工具生成，不手写、不手改；本计划不创建 `dao/`、`internal/model/do/`、`internal/model/entity/`。
- 不使用 `logic/` 目录，业务逻辑后续直接放在 `service/`。
- Go 代码错误处理使用 GoFrame `gerror` 包装上下文。
- Go 文件内如果有 3 个及以上相关变量声明，使用 `var (...)` 分组。
- 不复制旧 `cool-admin-go/contrib/drivers`；只 blank import GoFrame 官方 MySQL driver。

---

## Scope Check

本计划只覆盖设计文档中的“阶段 2：Model metadata 和自动建表”。

包含：

1. `cool/model` metadata。
2. GoFrame MySQL driver 注册。
3. `cool/db/schema` DDL 生成与 schema sync。
4. base 模块第一批表定义。
5. app 层 schema sync hook。
6. 真实 MySQL 验收命令。

不包含：

1. seed/menu 导入。
2. CRUD runtime。
3. 登录/JWT/权限。
4. EPS runtime。
5. Vue 联调。
6. 自动删除字段或索引。
7. 数据迁移。

---

## File Structure

### 创建文件

- `cool/model/model.go`  
  Model、Field、Index、DefaultValue 等元数据类型和构造函数。

- `cool/model/model_test.go`  
  测试模型定义、基础字段、字段映射和索引配置。

- `cool/db/driver/mysql.go`  
  blank import GoFrame 官方 MySQL driver。

- `cool/db/schema/ddl.go`  
  根据 `model.Definition` 生成 MySQL CREATE TABLE、ADD COLUMN、CREATE INDEX SQL。

- `cool/db/schema/ddl_test.go`  
  测试 DDL 字符串生成。

- `cool/db/schema/sync.go`  
  查询 `information_schema` 并执行安全 schema sync。

- `cool/db/schema/sync_test.go`  
  真实 MySQL 集成测试；未显式开启时跳过，开启后必须连接 `cool-go`。

- `modules/base/model/models.go`  
  注册 base 模块全部表定义。

- `modules/base/model/models_test.go`  
  测试 base 表数量与关键字段。

### 修改文件

- `go.mod`  
  增加 `github.com/gogf/gf/contrib/drivers/mysql/v2 v2.10.2`。

- `manifest/config/config.yaml`  
  更新默认 MySQL 连接到 `root:123456@tcp(127.0.0.1:3306)/cool-go`。

- `manifest/config/config.local.yaml`  
  更新本地 MySQL 示例连接到 `root:123456@tcp(127.0.0.1:3306)/cool-go`。

- `cool/module/module.go`  
  扩展模块接口，增加 `Models() []model.Definition`。

- `cool/module/module_test.go`  
  增加模块模型列表测试。

- `modules/base/base.go`  
  base 模块挂载 `modules/base/model.Register()`。

- `modules/base/base_test.go`  
  验证 base 模块模型列表。

- `cool/app/app.go`  
  增加 `SyncSchema(ctx context.Context) error` 和可注入 schema sync runner。

- `cool/app/app_test.go`  
  测试 app 可以收集 base models 并调用 schema sync runner。

- `README.md`  
  更新当前阶段和 MySQL schema sync 验收说明。

### 不创建文件

- 不创建 `dao/`。
- 不创建 `internal/model/do/`。
- 不创建 `internal/model/entity/`。
- 不创建 `logic/`。
- 不创建 seed、CRUD、auth、EPS 实现。

---

### Task 1: 增加 `cool/model` 元数据包

**Files:**
- Create: `cool/model/model.go`
- Create: `cool/model/model_test.go`
- Test: `go test ./cool/model`

**Interfaces:**
- Consumes: none.
- Produces:
  - `type Field struct`
  - `type Index struct`
  - `type Definition struct`
  - `func NewField(jsonName string, columnName string, dataType string) Field`
  - `func NewIndex(name string, columns ...string) Index`
  - `func NewUniqueIndex(name string, columns ...string) Index`
  - `func BaseFields() []Field`
  - `func NewDefinition(module string, name string, tableName string) Definition`
  - `func (d Definition) FieldByColumn(columnName string) (Field, bool)`

- [ ] **Step 1: Create model directory and failing tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/model
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/model/model_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/model"
)

func TestBaseFields(t *testing.T) {
	fields := model.BaseFields()
	if len(fields) != 4 {
		t.Fatalf("expected 4 base fields, got %d", len(fields))
	}

	checks := map[string]string{
		"id":          "id",
		"create_time": "createTime",
		"update_time": "updateTime",
		"tenant_id":   "tenantId",
	}
	for columnName, jsonName := range checks {
		found := false
		for _, field := range fields {
			if field.ColumnName == columnName && field.JSONName == jsonName {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing base field %s/%s", columnName, jsonName)
		}
	}
}

func TestDefinitionFieldByColumn(t *testing.T) {
	definition := model.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Comment("系统用户").
		Fields([]model.Field{
			model.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
			model.NewField("nickName", "nick_name", "varchar").Size(100).Comment("昵称"),
		})

	field, ok := definition.FieldByColumn("nick_name")
	if !ok {
		t.Fatal("expected nick_name field")
	}
	if field.JSONName != "nickName" {
		t.Fatalf("expected json name nickName, got %s", field.JSONName)
	}
}

func TestIndexConstructors(t *testing.T) {
	index := model.NewIndex("idx_user_status", "status", "tenant_id")
	if index.Name != "idx_user_status" {
		t.Fatalf("unexpected index name: %s", index.Name)
	}
	if index.IsUnique {
		t.Fatal("expected normal index")
	}
	if len(index.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(index.Columns))
	}

	uniqueIndex := model.NewUniqueIndex("uk_user_username", "username")
	if !uniqueIndex.IsUnique {
		t.Fatal("expected unique index")
	}
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/model
```

Expected: FAIL because `cool/model` implementation does not exist.

- [ ] **Step 3: Implement metadata types**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/model/model.go`:

```go
package model

// Field 是表字段元数据。
type Field struct {
	JSONName       string
	ColumnName     string
	DataType       string
	Length         int
	CommentText    string
	DefaultValue   string
	HasDefault     bool
	IsNullable     bool
	IsPrimary      bool
	IsAutoIncrement bool
	IsUnsigned     bool
	Dict           []string
}

// Index 是表索引元数据。
type Index struct {
	Name     string
	Columns  []string
	IsUnique bool
}

// Definition 是表模型元数据。
type Definition struct {
	Module      string
	Name        string
	TableName   string
	CommentText string
	FieldsValue []Field
	Indexes     []Index
}

/**
 * 创建字段元数据
 * @param jsonName JSON/EPS 字段名
 * @param columnName 数据库字段名
 * @param dataType MySQL 类型
 * @returns Field
 */
func NewField(jsonName string, columnName string, dataType string) Field {
	return Field{
		JSONName:   jsonName,
		ColumnName: columnName,
		DataType:   dataType,
	}
}

/**
 * 设置字段长度
 * @param length 字段长度
 * @returns Field
 */
func (f Field) Size(length int) Field {
	f.Length = length
	return f
}

/**
 * 设置字段注释
 * @param comment 字段注释
 * @returns Field
 */
func (f Field) Comment(comment string) Field {
	f.CommentText = comment
	return f
}

/**
 * 设置字段不可空
 * @returns Field
 */
func (f Field) NotNull() Field {
	f.IsNullable = false
	return f
}

/**
 * 设置字段可空
 * @returns Field
 */
func (f Field) Nullable() Field {
	f.IsNullable = true
	return f
}

/**
 * 设置字段默认值
 * @param value 默认值 SQL 片段
 * @returns Field
 */
func (f Field) Default(value string) Field {
	f.DefaultValue = value
	f.HasDefault = true
	return f
}

/**
 * 设置字段为主键
 * @returns Field
 */
func (f Field) Primary() Field {
	f.IsPrimary = true
	f.IsNullable = false
	return f
}

/**
 * 设置字段自增
 * @returns Field
 */
func (f Field) AutoIncrement() Field {
	f.IsAutoIncrement = true
	return f
}

/**
 * 设置无符号数字字段
 * @returns Field
 */
func (f Field) Unsigned() Field {
	f.IsUnsigned = true
	return f
}

/**
 * 设置字段字典
 * @param items 字典项
 * @returns Field
 */
func (f Field) WithDict(items ...string) Field {
	f.Dict = append([]string{}, items...)
	return f
}

/**
 * 创建普通索引
 * @param name 索引名
 * @param columns 字段名
 * @returns Index
 */
func NewIndex(name string, columns ...string) Index {
	return Index{
		Name:    name,
		Columns: append([]string{}, columns...),
	}
}

/**
 * 创建唯一索引
 * @param name 索引名
 * @param columns 字段名
 * @returns Index
 */
func NewUniqueIndex(name string, columns ...string) Index {
	return Index{
		Name:     name,
		Columns:  append([]string{}, columns...),
		IsUnique: true,
	}
}

/**
 * 基础字段列表
 * @returns []Field
 */
func BaseFields() []Field {
	return []Field{
		NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
		NewField("createTime", "create_time", "datetime").NotNull().Comment("创建时间"),
		NewField("updateTime", "update_time", "datetime").NotNull().Comment("更新时间"),
		NewField("tenantId", "tenant_id", "bigint").Unsigned().Nullable().Comment("租户ID"),
	}
}

/**
 * 创建模型定义
 * @param module 模块名
 * @param name 模型名
 * @param tableName 表名
 * @returns Definition
 */
func NewDefinition(module string, name string, tableName string) Definition {
	return Definition{
		Module:    module,
		Name:      name,
		TableName: tableName,
	}
}

/**
 * 设置表注释
 * @param comment 表注释
 * @returns Definition
 */
func (d Definition) Comment(comment string) Definition {
	d.CommentText = comment
	return d
}

/**
 * 设置字段列表
 * @param fields 字段列表
 * @returns Definition
 */
func (d Definition) Fields(fields []Field) Definition {
	d.FieldsValue = append([]Field{}, fields...)
	return d
}

/**
 * 追加字段列表
 * @param fields 字段列表
 * @returns Definition
 */
func (d Definition) AppendFields(fields ...Field) Definition {
	d.FieldsValue = append(d.FieldsValue, fields...)
	return d
}

/**
 * 设置索引列表
 * @param indexes 索引列表
 * @returns Definition
 */
func (d Definition) WithIndexes(indexes ...Index) Definition {
	d.Indexes = append([]Index{}, indexes...)
	return d
}

/**
 * 按数据库字段名查找字段
 * @param columnName 数据库字段名
 * @returns Field 和是否存在
 */
func (d Definition) FieldByColumn(columnName string) (Field, bool) {
	for _, field := range d.FieldsValue {
		if field.ColumnName == columnName {
			return field, true
		}
	}
	return Field{}, false
}
```

- [ ] **Step 4: Run model tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/model
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/cool/model
```

- [ ] **Step 5: Commit Task 1**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/model/model.go cool/model/model_test.go
git commit -m "feat: add model metadata" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 扩展 module 接口并注册 base models

**Files:**
- Modify: `cool/module/module.go`
- Modify: `cool/module/module_test.go`
- Create: `modules/base/model/models.go`
- Create: `modules/base/model/models_test.go`
- Modify: `modules/base/base.go`
- Modify: `modules/base/base_test.go`
- Test: `go test ./cool/module ./modules/base`

**Interfaces:**
- Consumes:
  - `model.Definition`
  - `model.NewDefinition(module string, name string, tableName string) model.Definition`
  - `model.BaseFields() []model.Field`
- Produces:
  - `func (d *Definition) Models(models []model.Definition) *Definition`
  - `func (d *Definition) ModuleModels() []model.Definition`
  - `func Register() []model.Definition` in `modules/base/model`

- [ ] **Step 1: Update module tests first**

Append this test to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/module/module_test.go`:

```go
func TestDefinitionModels(t *testing.T) {
	userModel := model.NewDefinition("base", "BaseSysUser", "base_sys_user")
	roleModel := model.NewDefinition("base", "BaseSysRole", "base_sys_role")

	mod := module.New("base").Models([]model.Definition{userModel, roleModel})

	models := mod.ModuleModels()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].TableName != "base_sys_user" {
		t.Fatalf("expected first table base_sys_user, got %s", models[0].TableName)
	}
}
```

Also add this import to the same file:

```go
"github.com/toothdy/cool-admin-go-next/cool/model"
```

- [ ] **Step 2: Run failing module test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/module
```

Expected: FAIL because `Models` and `ModuleModels` do not exist.

- [ ] **Step 3: Modify `cool/module/module.go`**

Update `/Users/n/数据/cool-admin/cool-admin-go-next/cool/module/module.go` so it imports `cool/model`, extends the interface, stores models, and exposes setters/getters:

```go
package module

import (
	"sort"

	"github.com/toothdy/cool-admin-go-next/cool/model"
)

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
	ModuleModels() []model.Definition
}

// Definition 是默认模块定义。
type Definition struct {
	key    string
	name   string
	config Config
	models []model.Definition
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
 * 设置模块模型
 * @param models 模型列表
 * @returns *Definition
 */
func (d *Definition) Models(models []model.Definition) *Definition {
	d.models = append([]model.Definition{}, models...)
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
 * 模块模型
 * @returns []model.Definition
 */
func (d *Definition) ModuleModels() []model.Definition {
	return append([]model.Definition{}, d.models...)
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

- [ ] **Step 4: Create base model definitions**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/modules/base/model
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/model/models.go`:

```go
package model

import coolModel "github.com/toothdy/cool-admin-go-next/cool/model"

/**
 * 注册 base 模块模型
 * @returns []coolModel.Definition
 */
func Register() []coolModel.Definition {
	return []coolModel.Definition{
		BaseSysUser(),
		BaseSysRole(),
		BaseSysMenu(),
		BaseSysDepartment(),
		BaseSysParam(),
		BaseSysLog(),
		BaseSysConf(),
		BaseSysUserRole(),
		BaseSysRoleMenu(),
		BaseSysRoleDepartment(),
	}
}

/**
 * 系统用户表
 * @returns coolModel.Definition
 */
func BaseSysUser() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("departmentId", "department_id", "bigint").Unsigned().Nullable().Comment("部门ID"),
		coolModel.NewField("userId", "user_id", "bigint").Unsigned().Nullable().Comment("上级用户ID"),
		coolModel.NewField("name", "name", "varchar").Size(100).Nullable().Comment("姓名"),
		coolModel.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		coolModel.NewField("password", "password", "varchar").Size(255).NotNull().Comment("密码"),
		coolModel.NewField("passwordV", "password_v", "int").NotNull().Default("1").Comment("密码版本"),
		coolModel.NewField("nickName", "nick_name", "varchar").Size(100).Nullable().Comment("昵称"),
		coolModel.NewField("headImg", "head_img", "varchar").Size(255).Nullable().Comment("头像"),
		coolModel.NewField("phone", "phone", "varchar").Size(20).Nullable().Comment("手机号"),
		coolModel.NewField("email", "email", "varchar").Size(100).Nullable().Comment("邮箱"),
		coolModel.NewField("remark", "remark", "varchar").Size(255).Nullable().Comment("备注"),
		coolModel.NewField("status", "status", "tinyint").NotNull().Default("1").Comment("状态").WithDict("禁用", "启用"),
		coolModel.NewField("socketId", "socket_id", "varchar").Size(255).Nullable().Comment("Socket ID"),
	)

	return coolModel.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Comment("系统用户").
		Fields(fields).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_user_username", "username"),
			coolModel.NewIndex("idx_base_sys_user_department_id", "department_id"),
			coolModel.NewIndex("idx_base_sys_user_status", "status"),
			coolModel.NewIndex("idx_base_sys_user_tenant_id", "tenant_id"),
		)
}

/**
 * 系统角色表
 * @returns coolModel.Definition
 */
func BaseSysRole() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("userId", "user_id", "bigint").Unsigned().Nullable().Comment("用户ID"),
		coolModel.NewField("name", "name", "varchar").Size(100).NotNull().Comment("角色名称"),
		coolModel.NewField("label", "label", "varchar").Size(100).NotNull().Comment("角色标识"),
		coolModel.NewField("remark", "remark", "varchar").Size(255).Nullable().Comment("备注"),
		coolModel.NewField("relevance", "relevance", "tinyint").NotNull().Default("1").Comment("数据权限是否关联上下级"),
		coolModel.NewField("menuIdList", "menu_id_list", "json").Nullable().Comment("菜单ID列表"),
		coolModel.NewField("departmentIdList", "department_id_list", "json").Nullable().Comment("部门ID列表"),
	)

	return coolModel.NewDefinition("base", "BaseSysRole", "base_sys_role").
		Comment("系统角色").
		Fields(fields).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_role_label", "label"),
			coolModel.NewIndex("idx_base_sys_role_tenant_id", "tenant_id"),
		)
}

/**
 * 系统菜单表
 * @returns coolModel.Definition
 */
func BaseSysMenu() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("parentId", "parent_id", "bigint").Unsigned().Nullable().Comment("父菜单ID"),
		coolModel.NewField("name", "name", "varchar").Size(100).NotNull().Comment("菜单名称"),
		coolModel.NewField("router", "router", "varchar").Size(255).Nullable().Comment("路由地址"),
		coolModel.NewField("perms", "perms", "varchar").Size(255).Nullable().Comment("权限标识"),
		coolModel.NewField("type", "type", "tinyint").NotNull().Default("0").Comment("类型"),
		coolModel.NewField("icon", "icon", "varchar").Size(100).Nullable().Comment("图标"),
		coolModel.NewField("orderNum", "order_num", "int").NotNull().Default("0").Comment("排序"),
		coolModel.NewField("viewPath", "view_path", "varchar").Size(255).Nullable().Comment("视图路径"),
		coolModel.NewField("keepAlive", "keep_alive", "tinyint").NotNull().Default("0").Comment("路由缓存"),
		coolModel.NewField("isShow", "is_show", "tinyint").NotNull().Default("1").Comment("是否显示"),
	)

	return coolModel.NewDefinition("base", "BaseSysMenu", "base_sys_menu").
		Comment("系统菜单").
		Fields(fields).
		WithIndexes(
			coolModel.NewIndex("idx_base_sys_menu_parent_id", "parent_id"),
			coolModel.NewIndex("idx_base_sys_menu_type", "type"),
			coolModel.NewIndex("idx_base_sys_menu_tenant_id", "tenant_id"),
		)
}

/**
 * 系统部门表
 * @returns coolModel.Definition
 */
func BaseSysDepartment() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("name", "name", "varchar").Size(100).NotNull().Comment("部门名称"),
		coolModel.NewField("userId", "user_id", "bigint").Unsigned().Nullable().Comment("负责人ID"),
		coolModel.NewField("parentId", "parent_id", "bigint").Unsigned().Nullable().Comment("父部门ID"),
		coolModel.NewField("orderNum", "order_num", "int").NotNull().Default("0").Comment("排序"),
	)

	return coolModel.NewDefinition("base", "BaseSysDepartment", "base_sys_department").
		Comment("系统部门").
		Fields(fields).
		WithIndexes(
			coolModel.NewIndex("idx_base_sys_department_parent_id", "parent_id"),
			coolModel.NewIndex("idx_base_sys_department_tenant_id", "tenant_id"),
		)
}

/**
 * 系统参数表
 * @returns coolModel.Definition
 */
func BaseSysParam() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("keyName", "key_name", "varchar").Size(100).NotNull().Comment("参数键"),
		coolModel.NewField("name", "name", "varchar").Size(100).NotNull().Comment("参数名称"),
		coolModel.NewField("data", "data", "longtext").Nullable().Comment("参数数据"),
		coolModel.NewField("dataType", "data_type", "tinyint").NotNull().Default("0").Comment("数据类型"),
		coolModel.NewField("remark", "remark", "varchar").Size(255).Nullable().Comment("备注"),
	)

	return coolModel.NewDefinition("base", "BaseSysParam", "base_sys_param").
		Comment("系统参数").
		Fields(fields).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_param_key_name", "key_name"),
			coolModel.NewIndex("idx_base_sys_param_tenant_id", "tenant_id"),
		)
}

/**
 * 系统日志表
 * @returns coolModel.Definition
 */
func BaseSysLog() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("userId", "user_id", "bigint").Unsigned().Nullable().Comment("用户ID"),
		coolModel.NewField("action", "action", "varchar").Size(255).Nullable().Comment("操作"),
		coolModel.NewField("ip", "ip", "varchar").Size(50).Nullable().Comment("IP"),
		coolModel.NewField("params", "params", "longtext").Nullable().Comment("参数"),
	)

	return coolModel.NewDefinition("base", "BaseSysLog", "base_sys_log").
		Comment("系统日志").
		Fields(fields).
		WithIndexes(
			coolModel.NewIndex("idx_base_sys_log_user_id", "user_id"),
			coolModel.NewIndex("idx_base_sys_log_tenant_id", "tenant_id"),
		)
}

/**
 * 系统配置表
 * @returns coolModel.Definition
 */
func BaseSysConf() coolModel.Definition {
	fields := coolModel.BaseFields()
	fields = append(fields,
		coolModel.NewField("cKey", "c_key", "varchar").Size(100).NotNull().Comment("配置键"),
		coolModel.NewField("cValue", "c_value", "longtext").Nullable().Comment("配置值"),
	)

	return coolModel.NewDefinition("base", "BaseSysConf", "base_sys_conf").
		Comment("系统配置").
		Fields(fields).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_conf_c_key", "c_key"),
			coolModel.NewIndex("idx_base_sys_conf_tenant_id", "tenant_id"),
		)
}

/**
 * 用户角色关联表
 * @returns coolModel.Definition
 */
func BaseSysUserRole() coolModel.Definition {
	return coolModel.NewDefinition("base", "BaseSysUserRole", "base_sys_user_role").
		Comment("用户角色关联").
		Fields([]coolModel.Field{
			coolModel.NewField("userId", "user_id", "bigint").Unsigned().NotNull().Comment("用户ID"),
			coolModel.NewField("roleId", "role_id", "bigint").Unsigned().NotNull().Comment("角色ID"),
		}).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_user_role", "user_id", "role_id"),
			coolModel.NewIndex("idx_base_sys_user_role_role_id", "role_id"),
		)
}

/**
 * 角色菜单关联表
 * @returns coolModel.Definition
 */
func BaseSysRoleMenu() coolModel.Definition {
	return coolModel.NewDefinition("base", "BaseSysRoleMenu", "base_sys_role_menu").
		Comment("角色菜单关联").
		Fields([]coolModel.Field{
			coolModel.NewField("roleId", "role_id", "bigint").Unsigned().NotNull().Comment("角色ID"),
			coolModel.NewField("menuId", "menu_id", "bigint").Unsigned().NotNull().Comment("菜单ID"),
		}).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_role_menu", "role_id", "menu_id"),
			coolModel.NewIndex("idx_base_sys_role_menu_menu_id", "menu_id"),
		)
}

/**
 * 角色部门关联表
 * @returns coolModel.Definition
 */
func BaseSysRoleDepartment() coolModel.Definition {
	return coolModel.NewDefinition("base", "BaseSysRoleDepartment", "base_sys_role_department").
		Comment("角色部门关联").
		Fields([]coolModel.Field{
			coolModel.NewField("roleId", "role_id", "bigint").Unsigned().NotNull().Comment("角色ID"),
			coolModel.NewField("departmentId", "department_id", "bigint").Unsigned().NotNull().Comment("部门ID"),
		}).
		WithIndexes(
			coolModel.NewUniqueIndex("uk_base_sys_role_department", "role_id", "department_id"),
			coolModel.NewIndex("idx_base_sys_role_department_department_id", "department_id"),
		)
}
```

- [ ] **Step 5: Add base model tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/model/models_test.go`:

```go
package model_test

import (
	"testing"

	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
)

func TestRegisterBaseModels(t *testing.T) {
	models := baseModel.Register()
	if len(models) != 10 {
		t.Fatalf("expected 10 base models, got %d", len(models))
	}

	expectedTables := map[string]bool{
		"base_sys_user":            false,
		"base_sys_role":            false,
		"base_sys_menu":            false,
		"base_sys_department":      false,
		"base_sys_param":           false,
		"base_sys_log":             false,
		"base_sys_conf":            false,
		"base_sys_user_role":       false,
		"base_sys_role_menu":       false,
		"base_sys_role_department": false,
	}
	for _, definition := range models {
		if _, ok := expectedTables[definition.TableName]; ok {
			expectedTables[definition.TableName] = true
		}
	}
	for tableName, found := range expectedTables {
		if !found {
			t.Fatalf("missing table definition: %s", tableName)
		}
	}
}

func TestBaseSysUserFields(t *testing.T) {
	definition := baseModel.BaseSysUser()
	for _, columnName := range []string{"id", "department_id", "user_id", "name", "username", "password", "password_v", "nick_name", "head_img", "phone", "email", "remark", "status", "socket_id", "create_time", "update_time", "tenant_id"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing base_sys_user field: %s", columnName)
		}
	}
}

func TestBaseSysMenuFields(t *testing.T) {
	definition := baseModel.BaseSysMenu()
	for _, columnName := range []string{"id", "parent_id", "name", "router", "perms", "type", "icon", "order_num", "view_path", "keep_alive", "is_show", "create_time", "update_time", "tenant_id"} {
		if _, ok := definition.FieldByColumn(columnName); !ok {
			t.Fatalf("missing base_sys_menu field: %s", columnName)
		}
	}
}
```

- [ ] **Step 6: Attach models to base module**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base.go`:

```go
package base

import (
	"github.com/toothdy/cool-admin-go-next/cool/module"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/model"
)

/**
 * 创建 base 模块
 * @returns module.Module
 */
func NewModule() module.Module {
	return module.New("base").
		Name("基础模块").
		Config(NewConfig()).
		Models(baseModel.Register())
}
```

Append this test to `/Users/n/数据/cool-admin/cool-admin-go-next/modules/base/base_test.go`:

```go
func TestNewModuleModels(t *testing.T) {
	mod := base.NewModule()
	models := mod.ModuleModels()
	if len(models) != 10 {
		t.Fatalf("expected 10 models, got %d", len(models))
	}
}
```

- [ ] **Step 7: Run tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/module ./modules/base ./modules/base/model
```

Expected: all packages pass.

- [ ] **Step 8: Commit Task 2**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/module/module.go cool/module/module_test.go modules/base/base.go modules/base/base_test.go modules/base/model/models.go modules/base/model/models_test.go
git commit -m "feat: register base model metadata" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 增加 MySQL DDL 生成器

**Files:**
- Create: `cool/db/schema/ddl.go`
- Create: `cool/db/schema/ddl_test.go`
- Test: `go test ./cool/db/schema`

**Interfaces:**
- Consumes:
  - `model.Definition`
  - `model.Field`
  - `model.Index`
- Produces:
  - `func CreateTableSQL(definition model.Definition) string`
  - `func AddColumnSQL(tableName string, field model.Field) string`
  - `func CreateIndexSQL(tableName string, index model.Index) string`
  - `func ColumnSQL(field model.Field) string`

- [ ] **Step 1: Write failing DDL tests**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/db/schema
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/db/schema/ddl_test.go`:

```go
package schema_test

import (
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/model"
)

func TestColumnSQL(t *testing.T) {
	field := model.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名")
	sql := schema.ColumnSQL(field)

	checks := []string{"`username` varchar(100)", "NOT NULL", "COMMENT '用户名'"}
	for _, check := range checks {
		if !strings.Contains(sql, check) {
			t.Fatalf("expected column sql to contain %q, got %s", check, sql)
		}
	}
}

func TestCreateTableSQL(t *testing.T) {
	definition := model.NewDefinition("base", "BaseSysUser", "base_sys_user").
		Comment("系统用户").
		Fields([]model.Field{
			model.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			model.NewField("username", "username", "varchar").Size(100).NotNull().Comment("用户名"),
		}).
		WithIndexes(model.NewUniqueIndex("uk_base_sys_user_username", "username"))

	sql := schema.CreateTableSQL(definition)
	checks := []string{
		"CREATE TABLE IF NOT EXISTS `base_sys_user`",
		"`id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT 'ID'",
		"PRIMARY KEY (`id`)",
		"UNIQUE KEY `uk_base_sys_user_username` (`username`)",
		"ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统用户'",
	}
	for _, check := range checks {
		if !strings.Contains(sql, check) {
			t.Fatalf("expected create sql to contain %q, got %s", check, sql)
		}
	}
}

func TestAddColumnSQL(t *testing.T) {
	field := model.NewField("nickName", "nick_name", "varchar").Size(100).Nullable().Comment("昵称")
	sql := schema.AddColumnSQL("base_sys_user", field)
	if !strings.Contains(sql, "ALTER TABLE `base_sys_user` ADD COLUMN `nick_name` varchar(100) NULL COMMENT '昵称'") {
		t.Fatalf("unexpected add column sql: %s", sql)
	}
}

func TestCreateIndexSQL(t *testing.T) {
	index := model.NewUniqueIndex("uk_base_sys_user_username", "username")
	sql := schema.CreateIndexSQL("base_sys_user", index)
	if sql != "CREATE UNIQUE INDEX `uk_base_sys_user_username` ON `base_sys_user` (`username`)" {
		t.Fatalf("unexpected index sql: %s", sql)
	}
}
```

- [ ] **Step 2: Run failing DDL tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/db/schema
```

Expected: FAIL because `ddl.go` does not exist.

- [ ] **Step 3: Implement DDL generator**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/db/schema/ddl.go`:

```go
package schema

import (
	"fmt"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool/model"
)

/**
 * 生成创建表 SQL
 * @param definition 模型定义
 * @returns string
 */
func CreateTableSQL(definition model.Definition) string {
	parts := make([]string, 0, len(definition.FieldsValue)+len(definition.Indexes)+1)
	primaryColumns := make([]string, 0)

	for _, field := range definition.FieldsValue {
		parts = append(parts, ColumnSQL(field))
		if field.IsPrimary {
			primaryColumns = append(primaryColumns, quoteIdentifier(field.ColumnName))
		}
	}
	if len(primaryColumns) > 0 {
		parts = append(parts, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(primaryColumns, ", ")))
	}
	for _, index := range definition.Indexes {
		parts = append(parts, inlineIndexSQL(index))
	}

	return fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s (\n  %s\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='%s'",
		quoteIdentifier(definition.TableName),
		strings.Join(parts, ",\n  "),
		escapeSQLString(definition.CommentText),
	)
}

/**
 * 生成新增字段 SQL
 * @param tableName 表名
 * @param field 字段元数据
 * @returns string
 */
func AddColumnSQL(tableName string, field model.Field) string {
	return fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", quoteIdentifier(tableName), ColumnSQL(field))
}

/**
 * 生成创建索引 SQL
 * @param tableName 表名
 * @param index 索引元数据
 * @returns string
 */
func CreateIndexSQL(tableName string, index model.Index) string {
	kind := "INDEX"
	if index.IsUnique {
		kind = "UNIQUE INDEX"
	}
	return fmt.Sprintf(
		"CREATE %s %s ON %s (%s)",
		kind,
		quoteIdentifier(index.Name),
		quoteIdentifier(tableName),
		quoteIdentifiers(index.Columns),
	)
}

/**
 * 生成字段 SQL
 * @param field 字段元数据
 * @returns string
 */
func ColumnSQL(field model.Field) string {
	parts := []string{quoteIdentifier(field.ColumnName), columnTypeSQL(field)}
	if field.IsNullable {
		parts = append(parts, "NULL")
	} else {
		parts = append(parts, "NOT NULL")
	}
	if field.IsAutoIncrement {
		parts = append(parts, "AUTO_INCREMENT")
	}
	if field.HasDefault {
		parts = append(parts, fmt.Sprintf("DEFAULT %s", field.DefaultValue))
	}
	if field.CommentText != "" {
		parts = append(parts, fmt.Sprintf("COMMENT '%s'", escapeSQLString(field.CommentText)))
	}
	return strings.Join(parts, " ")
}

/**
 * 生成字段类型 SQL
 * @param field 字段元数据
 * @returns string
 */
func columnTypeSQL(field model.Field) string {
	dataType := field.DataType
	if field.Length > 0 && supportsLength(dataType) {
		dataType = fmt.Sprintf("%s(%d)", dataType, field.Length)
	}
	if field.IsUnsigned {
		dataType = fmt.Sprintf("%s unsigned", dataType)
	}
	return dataType
}

/**
 * 类型是否支持长度
 * @param dataType 字段类型
 * @returns bool
 */
func supportsLength(dataType string) bool {
	switch dataType {
	case "varchar", "char", "int", "tinyint", "bigint":
		return true
	default:
		return false
	}
}

/**
 * 生成内联索引 SQL
 * @param index 索引元数据
 * @returns string
 */
func inlineIndexSQL(index model.Index) string {
	kind := "KEY"
	if index.IsUnique {
		kind = "UNIQUE KEY"
	}
	return fmt.Sprintf("%s %s (%s)", kind, quoteIdentifier(index.Name), quoteIdentifiers(index.Columns))
}

/**
 * 引用标识符
 * @param name 标识符
 * @returns string
 */
func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

/**
 * 引用多个标识符
 * @param names 标识符列表
 * @returns string
 */
func quoteIdentifiers(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, quoteIdentifier(name))
	}
	return strings.Join(quoted, ", ")
}

/**
 * 转义 SQL 字符串
 * @param value 字符串
 * @returns string
 */
func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
```

- [ ] **Step 4: Run DDL tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/db/schema
```

Expected:

```text
ok  	github.com/toothdy/cool-admin-go-next/cool/db/schema
```

- [ ] **Step 5: Commit Task 3**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/db/schema/ddl.go cool/db/schema/ddl_test.go
git commit -m "feat: add mysql ddl generator" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 实现 GoFrame MySQL driver 和 schema sync

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `cool/db/driver/mysql.go`
- Create: `cool/db/schema/sync.go`
- Create: `cool/db/schema/sync_test.go`
- Modify: `manifest/config/config.yaml`
- Modify: `manifest/config/config.local.yaml`
- Test: `go test ./cool/db/schema`

**Interfaces:**
- Consumes:
  - `schema.CreateTableSQL(definition model.Definition) string`
  - `schema.AddColumnSQL(tableName string, field model.Field) string`
  - `schema.CreateIndexSQL(tableName string, index model.Index) string`
- Produces:
  - `type Syncer struct`
  - `type SyncResult struct`
  - `func NewSyncer(db gdb.DB) *Syncer`
  - `func (s *Syncer) Sync(ctx context.Context, definitions []model.Definition) (SyncResult, error)`

- [ ] **Step 1: Add GoFrame MySQL driver dependency**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go get github.com/gogf/gf/contrib/drivers/mysql/v2@v2.10.2
```

Expected: command exits `0` and updates `go.mod` / `go.sum`.

- [ ] **Step 2: Create driver registration file**

Create directory:

```bash
mkdir -p /Users/n/数据/cool-admin/cool-admin-go-next/cool/db/driver
```

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/db/driver/mysql.go`:

```go
// Package driver 注册 GoFrame 数据库驱动。
package driver

import _ "github.com/gogf/gf/contrib/drivers/mysql/v2"
```

- [ ] **Step 3: Update config files**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/manifest/config/config.yaml` database link to:

```yaml
database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
    debug: true
```

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/manifest/config/config.local.yaml` database link to:

```yaml
database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
```

- [ ] **Step 4: Write sync integration tests**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/db/schema/sync_test.go`:

```go
package schema_test

import (
	"context"
	"os"
	"testing"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/model"
)

func TestSyncerSkipsWithoutIntegrationFlag(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") == "1" {
		t.Skip("integration flag enabled")
	}
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "" {
		t.Fatalf("unexpected integration flag value")
	}
}

func TestSyncerCreatesTableAndIsIdempotent(t *testing.T) {
	if os.Getenv("COOL_SCHEMA_INTEGRATION") != "1" {
		t.Skip("set COOL_SCHEMA_INTEGRATION=1 to run real MySQL schema sync test")
	}

	ctx := context.Background()
	db := g.DB()
	definition := model.NewDefinition("test", "SchemaTest", "schema_sync_test").
		Comment("建表测试").
		Fields([]model.Field{
			model.NewField("id", "id", "bigint").Unsigned().Primary().AutoIncrement().Comment("ID"),
			model.NewField("name", "name", "varchar").Size(100).NotNull().Comment("名称"),
		}).
		WithIndexes(model.NewIndex("idx_schema_sync_test_name", "name"))

	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS `schema_sync_test`")

	syncer := schema.NewSyncer(db)
	first, err := syncer.Sync(ctx, []model.Definition{definition})
	if err != nil {
		t.Fatalf("first sync failed: %v", err)
	}
	if first.CreatedTables != 1 {
		t.Fatalf("expected 1 created table, got %d", first.CreatedTables)
	}

	second, err := syncer.Sync(ctx, []model.Definition{definition})
	if err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if second.CreatedTables != 0 || second.AddedColumns != 0 || second.CreatedIndexes != 0 {
		t.Fatalf("expected idempotent second sync, got %#v", second)
	}
}
```

- [ ] **Step 5: Run tests and observe integration skip**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/db/schema
```

Expected: PASS; integration test skipped unless `COOL_SCHEMA_INTEGRATION=1`.

- [ ] **Step 6: Implement schema sync**

Write `/Users/n/数据/cool-admin/cool-admin-go-next/cool/db/schema/sync.go`:

```go
package schema

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/model"
)

// SyncResult 是 schema 同步结果。
type SyncResult struct {
	CreatedTables  int
	AddedColumns   int
	CreatedIndexes int
}

// Syncer 是 MySQL schema 同步器。
type Syncer struct {
	db gdb.DB
}

/**
 * 创建 schema 同步器
 * @param db GoFrame 数据库实例
 * @returns *Syncer
 */
func NewSyncer(db gdb.DB) *Syncer {
	return &Syncer{
		db: db,
	}
}

/**
 * 同步表结构
 * @param ctx 上下文
 * @param definitions 模型定义列表
 * @returns SyncResult
 */
func (s *Syncer) Sync(ctx context.Context, definitions []model.Definition) (SyncResult, error) {
	result := SyncResult{}
	for _, definition := range definitions {
		isExists, err := s.tableExists(ctx, definition.TableName)
		if err != nil {
			return result, gerror.Wrap(err, fmt.Sprintf("检查表是否存在失败: %s", definition.TableName))
		}
		if !isExists {
			if _, err = s.db.Exec(ctx, CreateTableSQL(definition)); err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("创建表失败: %s", definition.TableName))
			}
			result.CreatedTables++
			continue
		}

		for _, field := range definition.FieldsValue {
			isColumnExists, err := s.columnExists(ctx, definition.TableName, field.ColumnName)
			if err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("检查字段是否存在失败: %s.%s", definition.TableName, field.ColumnName))
			}
			if !isColumnExists {
				if _, err = s.db.Exec(ctx, AddColumnSQL(definition.TableName, field)); err != nil {
					return result, gerror.Wrap(err, fmt.Sprintf("新增字段失败: %s.%s", definition.TableName, field.ColumnName))
				}
				result.AddedColumns++
			}
		}

		for _, index := range definition.Indexes {
			isIndexExists, err := s.indexExists(ctx, definition.TableName, index.Name)
			if err != nil {
				return result, gerror.Wrap(err, fmt.Sprintf("检查索引是否存在失败: %s.%s", definition.TableName, index.Name))
			}
			if !isIndexExists {
				if _, err = s.db.Exec(ctx, CreateIndexSQL(definition.TableName, index)); err != nil {
					return result, gerror.Wrap(err, fmt.Sprintf("创建索引失败: %s.%s", definition.TableName, index.Name))
				}
				result.CreatedIndexes++
			}
		}
	}
	return result, nil
}

/**
 * 表是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @returns bool
 */
func (s *Syncer) tableExists(ctx context.Context, tableName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", tableName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

/**
 * 字段是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @param columnName 字段名
 * @returns bool
 */
func (s *Syncer) columnExists(ctx context.Context, tableName string, columnName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", tableName, columnName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

/**
 * 索引是否存在
 * @param ctx 上下文
 * @param tableName 表名
 * @param indexName 索引名
 * @returns bool
 */
func (s *Syncer) indexExists(ctx context.Context, tableName string, indexName string) (bool, error) {
	count, err := s.db.GetCount(ctx, "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?", tableName, indexName)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
```

- [ ] **Step 7: Run schema tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/db/schema
```

Expected: PASS; integration test skipped.

- [ ] **Step 8: Commit Task 4**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add go.mod go.sum manifest/config/config.yaml manifest/config/config.local.yaml cool/db/driver/mysql.go cool/db/schema/sync.go cool/db/schema/sync_test.go
git commit -m "feat: add mysql schema syncer" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: 接入 app schema sync hook

**Files:**
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Test: `go test ./cool/app`

**Interfaces:**
- Consumes:
  - `module.Module.ModuleModels() []model.Definition`
  - `schema.NewSyncer(db gdb.DB) *schema.Syncer`
- Produces:
  - `type SchemaSyncRunner func(ctx context.Context, definitions []model.Definition) error`
  - `type Options struct { StartServer bool; AutoSyncSchema bool; SchemaSyncRunner SchemaSyncRunner }`
  - `func (a *Application) Models() []model.Definition`
  - `func (a *Application) SyncSchema(ctx context.Context) error`

- [ ] **Step 1: Write failing app tests**

Append tests to `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app_test.go`:

```go
func TestNewCollectsBaseModels(t *testing.T) {
	application := app.New(app.Options{
		StartServer: false,
	})

	models := application.Models()
	if len(models) != 10 {
		t.Fatalf("expected 10 models, got %d", len(models))
	}
}

func TestSyncSchemaUsesRunner(t *testing.T) {
	called := false
	application := app.New(app.Options{
		StartServer: false,
		SchemaSyncRunner: func(ctx context.Context, definitions []model.Definition) error {
			called = true
			if len(definitions) != 10 {
				t.Fatalf("expected 10 definitions, got %d", len(definitions))
			}
			return nil
		},
	})

	if err := application.SyncSchema(context.Background()); err != nil {
		t.Fatalf("sync schema failed: %v", err)
	}
	if !called {
		t.Fatal("expected schema sync runner to be called")
	}
}
```

Add this import to the same file:

```go
"github.com/toothdy/cool-admin-go-next/cool/model"
```

- [ ] **Step 2: Run failing app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app
```

Expected: FAIL because `Models`, `SyncSchema`, and `SchemaSyncRunner` do not exist.

- [ ] **Step 3: Modify app runtime**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/cool/app/app.go`:

```go
package app

import (
	"context"

	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/model"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	"github.com/toothdy/cool-admin-go-next/cool/response"
	"github.com/toothdy/cool-admin-go-next/modules"
)

// SchemaSyncRunner 是 schema 同步执行器。
type SchemaSyncRunner func(ctx context.Context, definitions []model.Definition) error

// Options 是应用启动选项。
type Options struct {
	StartServer      bool
	AutoSyncSchema   bool
	SchemaSyncRunner SchemaSyncRunner
}

// Application 是 cool 应用实例。
type Application struct {
	server           *ghttp.Server
	modules          []module.Module
	models           []model.Definition
	schemaSyncRunner SchemaSyncRunner
}

/**
 * 创建应用实例
 * @param options 启动选项
 * @returns *Application
 */
func New(options Options) *Application {
	registeredModules := modules.Register()
	registeredModels := collectModels(registeredModules)
	runner := options.SchemaSyncRunner
	if runner == nil {
		runner = defaultSchemaSyncRunner
	}

	application := &Application{
		modules:          registeredModules,
		models:           registeredModels,
		schemaSyncRunner: runner,
	}

	if options.StartServer {
		application.server = g.Server()
		application.registerRoutes()
	}
	if options.AutoSyncSchema {
		if err := application.SyncSchema(context.Background()); err != nil {
			g.Log().Fatal(context.Background(), err)
		}
	}

	return application
}

/**
 * 启动默认应用
 * @param ctx 上下文
 * @returns error
 */
func Run(ctx context.Context) error {
	return New(Options{StartServer: true, AutoSyncSchema: true}).Run(ctx)
}

/**
 * 当前模块列表
 * @returns []module.Module
 */
func (a *Application) Modules() []module.Module {
	return a.modules
}

/**
 * 当前模型列表
 * @returns []model.Definition
 */
func (a *Application) Models() []model.Definition {
	return append([]model.Definition{}, a.models...)
}

/**
 * 同步数据库结构
 * @param ctx 上下文
 * @returns error
 */
func (a *Application) SyncSchema(ctx context.Context) error {
	return a.schemaSyncRunner(ctx, a.models)
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
 * 收集模块模型
 * @param modules 模块列表
 * @returns []model.Definition
 */
func collectModels(modules []module.Module) []model.Definition {
	definitions := make([]model.Definition, 0)
	for _, mod := range modules {
		definitions = append(definitions, mod.ModuleModels()...)
	}
	return definitions
}

/**
 * 默认 schema 同步执行器
 * @param ctx 上下文
 * @param definitions 模型定义列表
 * @returns error
 */
func defaultSchemaSyncRunner(ctx context.Context, definitions []model.Definition) error {
	_, err := schema.NewSyncer(g.DB()).Sync(ctx, definitions)
	return err
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

- [ ] **Step 4: Run app tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app
```

Expected: PASS.

- [ ] **Step 5: Run full Go tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 6: Commit Task 5**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go
git commit -m "feat: wire schema sync into app" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: 真实 MySQL 验收和 README 更新

**Files:**
- Modify: `README.md`
- Test: `go test ./...`
- Test: `COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1`
- Test: `go run .` + `/health`

**Interfaces:**
- Consumes:
  - app schema sync hook。
  - base model metadata。
  - `cool-go` MySQL database。
- Produces: Plan2 验收文档与已验证数据库表结构。

- [ ] **Step 1: Update README**

Modify `/Users/n/数据/cool-admin/cool-admin-go-next/README.md` current stage section to include Plan2:

```markdown
## 当前阶段

当前仓库处于阶段 2：Model metadata 和 MySQL 自动建表。

已完成：

1. Go module 初始化。
2. GoFrame v2 依赖。
3. `cool/module` 模块注册骨架。
4. `modules/base` 模块骨架。
5. `cool/response` Node 兼容响应结构。
6. `/health` 健康检查。
7. `cool/model` 模型元数据。
8. `modules/base/model` base 表定义。
9. MySQL schema sync：创建表、新增字段、新增索引。

未完成：

1. `db.json` / `menu.json` 导入。
2. CRUD runtime。
3. 登录/JWT/权限。
4. EPS runtime。
5. Vue 前端联调。
```

Add this section after startup instructions:

```markdown
## MySQL 自动建表验收

Plan2 使用真实 MySQL 验收，默认连接：

```text
mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4
```

如果数据库不存在，先执行：

```sql
CREATE DATABASE `cool-go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

执行集成测试：

```bash
COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1
```

启动应用时会按 `cool.schema.autoSync` 自动同步 base 表结构：

```bash
go run .
```
```

- [ ] **Step 2: Run full tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Verify MySQL database exists or report blocker**

Run:

```bash
mysql -uroot -p123456 -e "CREATE DATABASE IF NOT EXISTS \`cool-go\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
```

Expected: command exits `0`. If command is missing or MySQL is not running, report BLOCKED with the exact command output.

- [ ] **Step 4: Run real MySQL schema integration test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1
```

Expected: PASS.

- [ ] **Step 5: Run app once to create base tables**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
(go run . > /tmp/cool-admin-go-next.log 2>&1 & echo $! > /tmp/cool-admin-go-next.pid)
sleep 3
curl -s http://127.0.0.1:8001/health
kill $(cat /tmp/cool-admin-go-next.pid)
```

Expected curl output:

```json
{"code":1000,"message":"success","data":{"status":"ok"}}
```

- [ ] **Step 6: Verify base tables and key fields**

Run:

```bash
mysql -uroot -p123456 cool-go -e "SELECT table_name FROM information_schema.tables WHERE table_schema = 'cool-go' AND table_name IN ('base_sys_user','base_sys_role','base_sys_menu','base_sys_department','base_sys_param','base_sys_log','base_sys_conf','base_sys_user_role','base_sys_role_menu','base_sys_role_department') ORDER BY table_name;"
mysql -uroot -p123456 cool-go -e "SELECT column_name FROM information_schema.columns WHERE table_schema = 'cool-go' AND table_name = 'base_sys_user' AND column_name IN ('id','department_id','user_id','name','username','password','password_v','nick_name','head_img','phone','email','remark','status','socket_id','create_time','update_time','tenant_id') ORDER BY column_name;"
mysql -uroot -p123456 cool-go -e "SELECT column_name FROM information_schema.columns WHERE table_schema = 'cool-go' AND table_name = 'base_sys_menu' AND column_name IN ('id','parent_id','name','router','perms','type','icon','order_num','view_path','keep_alive','is_show','create_time','update_time','tenant_id') ORDER BY column_name;"
```

Expected:

1. First command prints 10 table names.
2. Second command prints all listed `base_sys_user` columns.
3. Third command prints all listed `base_sys_menu` columns.

- [ ] **Step 7: Verify idempotent app startup**

Run the app startup and table checks one more time:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
(go run . > /tmp/cool-admin-go-next.log 2>&1 & echo $! > /tmp/cool-admin-go-next.pid)
sleep 3
curl -s http://127.0.0.1:8001/health
kill $(cat /tmp/cool-admin-go-next.pid)
```

Expected: no schema sync error in `/tmp/cool-admin-go-next.log`, and health response remains successful.

- [ ] **Step 8: Verify forbidden directories are absent**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
test ! -d dao
test ! -d internal/model/do
test ! -d internal/model/entity
test ! -d logic
```

Expected: all commands exit `0`.

- [ ] **Step 9: Commit Task 6**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md
git commit -m "docs: document schema sync" -m "Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: 最终验证 Plan2 产物

**Files:**
- No new files expected.
- Verify all files from Tasks 1-6.

**Interfaces:**
- Consumes: all previous tasks.
- Produces: verified Plan2 completion.

- [ ] **Step 1: Run full Go tests**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./...
```

Expected: all packages pass.

- [ ] **Step 2: Run real schema sync integration test**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1
```

Expected: PASS.

- [ ] **Step 3: Verify app creates and keeps base tables**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
(go run . > /tmp/cool-admin-go-next.log 2>&1 & echo $! > /tmp/cool-admin-go-next.pid)
sleep 3
curl -s http://127.0.0.1:8001/health
kill $(cat /tmp/cool-admin-go-next.pid)
mysql -uroot -p123456 cool-go -e "SELECT COUNT(*) AS table_count FROM information_schema.tables WHERE table_schema = 'cool-go' AND table_name LIKE 'base_sys_%';"
```

Expected:

1. curl output includes `{"code":1000,"message":"success"`.
2. `table_count` is at least `10`.

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
Plan2: complete (Model metadata and MySQL schema sync, real MySQL verified)
```

This file is ignored and must not be committed.

---

## Self-Review

### Spec coverage

This plan covers the Plan2 design spec:

1. `cool/model` metadata: Task 1.
2. base 表模型定义: Task 2.
3. MySQL DDL generation: Task 3.
4. GoFrame MySQL driver and schema sync: Task 4.
5. app startup hook: Task 5.
6. real MySQL verification against `root/123456/cool-go`: Task 6 and Task 7.
7. idempotency: Task 4 integration test, Task 6, Task 7.
8. forbidden generated directories: Task 6.

### Placeholder scan

Placeholder scan passed; no open-ended implementation placeholders remain.

### Type consistency

The task interfaces consistently use:

1. `model.Field`.
2. `model.Index`.
3. `model.Definition`.
4. `module.Module.ModuleModels()`.
5. `schema.NewSyncer(db)`.
6. `schema.Syncer.Sync(ctx, definitions)`.
7. `app.Application.Models()`.
8. `app.Application.SyncSchema(ctx)`.
