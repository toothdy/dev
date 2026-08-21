# 模块 04：实体与 Descriptor 元数据实施计划

> 状态：completed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `cool-next/core/entity` 交付从明确 Go 实体类型编译不可变 Descriptor，并严格校验字段、索引和跨实体物理名称冲突的能力。

**Architecture:** `Compile[E, ID]` 通过一次显式反射读取已知 struct，不扫描目录、不注册全局状态。私有不可变值实现公开 `Field/Metadata/Descriptor` 接口，三个字段索引提供精确查询，`ValidateSet` 在调用方提交的集合上执行稳定冲突检查。

**Tech Stack:** Go 1.26.4、GoFrame v2.10.2 `g.Meta/gtime/gerror`、模块 02 Core 异常、标准库 reflection/regexp/strconv/testing

---

## 1. 实施约束

- 事实来源：`docs/superpowers/specs/2026-08-04-04-entity-and-descriptor-metadata-design.md`；
- 严格 TDD，每个行为先写测试并观察预期失败；
- 不实现 `DOValue/NewDO`、数据库类型、DDL、AST 扫描或生成文件；
- 不引入全局注册表；
- 当前仓库没有初始提交且基线全部未跟踪，无法创建 worktree 或有意义的分步提交；保留用户现有状态，不创建异常 root commit。

## 2. 文件结构

| 文件 | 职责 |
|---|---|
| `cool-next/core/entity/base.go` | 固定 Base 字段 |
| `cool-next/core/entity/types.go` | LogicalType、Constraints、Field/Metadata/Descriptor 接口 |
| `cool-next/core/entity/index.go` | Schema、Index 和两个防御性构造器 |
| `cool-next/core/entity/compile.go` | Compile 编排、实体骨架和 g.Meta/Base 解析 |
| `cool-next/core/entity/field.go` | 业务字段标签、逻辑类型和 cool 约束解析 |
| `cool-next/core/entity/descriptor.go` | 私有不可变 Field/Descriptor 实现和查询索引 |
| `cool-next/core/entity/validate.go` | 单 Descriptor 与 Descriptor 集合冲突校验 |
| `cool-next/core/entity/errors.go` | Core 异常包装 |
| `cool-next/core/entity/base_test.go` | Base 和公开契约测试 |
| `cool-next/core/entity/compile_test.go` | 实体、Meta、字段标签和逻辑类型测试 |
| `cool-next/core/entity/index_test.go` | Schema、索引、系统字段和不可变性测试 |
| `cool-next/core/entity/validate_test.go` | 集合冲突和错误语义测试 |

## 3. 任务清单

### 任务 1：Base、公开契约与 Index 构造器

**Files:** `base.go`、`types.go`、`index.go`、`base_test.go`

- [x] 写 `TestBaseShape`，验证 Base 只有三个固定字段、类型和标签：

```go
typ := reflect.TypeFor[Base]()
if typ.NumField() != 3 {
   t.Fatalf("Base fields = %d", typ.NumField())
}
if typ.Field(0).Tag.Get("orm") != "id" {
   t.Fatalf("ID orm tag = %q", typ.Field(0).Tag.Get("orm"))
}
```

- [x] 写 `TestIndexConstructorsCopyFields`，修改传入和返回的 `Fields` 后，原始声明保持不变。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'Test(BaseShape|IndexConstructorsCopyFields)' -count=1
```

预期因 package/API 不存在失败。

- [x] 实现固定 `Base`、七种 `LogicalType`、`Constraints`、`Field/Metadata/Descriptor`、`Schema/Index` 和两个索引构造器：

```go
func IndexOf(name string, fields ...string) Index {
   return Index{Name: name, Fields: append([]string(nil), fields...)}
}

func UniqueIndexOf(name string, fields ...string) Index {
   index := IndexOf(name, fields...)
   index.Unique = true
   return index
}
```

- [x] 运行定向测试，确认任务 1 转绿。

### 任务 2：实体骨架、g.Meta 与 Base 编译

**Files:** `compile.go`、`descriptor.go`、`errors.go`、`compile_test.go`

- [x] 定义合法测试实体并写 `TestCompileReadsMetaAndBase`：

```go
type goodsEntity struct {
   g.Meta `orm:"table:demo_goods" description:"商品信息"`
   Base
   Title string `json:"title" orm:"title" description:"标题"`
}

descriptor, err := Compile[goodsEntity, uint64](Schema{})
if err != nil {
   t.Fatal(err)
}
if descriptor.Table() != "demo_goods" || descriptor.Description() != "商品信息" {
   t.Fatalf("metadata = %s/%s", descriptor.Table(), descriptor.Description())
}
```

- [x] 写表驱动 RED 测试，拒绝指针根、缺失/重复 Meta、缺失/重复 Base、其他匿名字段、未导出字段、错误 ID 类型、空/非法/重复 `table:` 指令和空表描述。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'TestCompile(ReadsMetaAndBase|RejectsInvalidEntityShape)' -count=1
```

- [x] 实现 `Compile[E, ID]` 的实体类型门禁、严格 Meta 标签解析、Base 展开及 Core 错误包装；不使用全局缓存或注册表。
- [x] 构造固定 Base 字段元数据：`id` 主键/自增，两个时间字段系统维护且数据库非空。
- [x] 运行定向测试和本包测试，确认任务 2 转绿。

### 任务 3：业务字段标签、类型与 cool 约束

**Files:** `field.go`、`compile.go`、`compile_test.go`

- [x] 写 `TestCompileInfersFieldMetadata`，覆盖 bool、全部整数族、float、string、`[]byte`、`time.Time`、`gtime.Time`、named type 和一层 pointer，断言逻辑类型、Go 类型及 nullable。
- [x] 写 `TestCompileParsesFieldConstraints`：

```go
type pricedEntity struct {
   g.Meta `orm:"table:priced_goods" description:"商品"`
   Base
   Name  string  `json:"name" orm:"name" description:"名称" cool:"size=50"`
   Price float64 `json:"price" orm:"price" description:"价格" cool:"precision=10,scale=2,default=0"`
}
```

断言 `size/default/precision/scale` 的值和 Has 标志。

- [x] 写表驱动 RED 测试，拒绝缺失/空/非法 `json`、`orm`、`description`，snake_case 列名，名称重复，双重指针和不支持类型。
- [x] 写表驱动 RED 测试，拒绝 cool 空项、重复键、未知键、缺少等号、非法数值、错误适用类型、scale 缺 precision 或大于 precision。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'TestCompile(InfersFieldMetadata|ParsesFieldConstraints|RejectsInvalid)' -count=1
```

- [x] 实现严格标签解析、lowerCamelCase 校验、逻辑类型推导和 cool 约束解析；默认值只保存非空原始文本，不在本模块翻译为数据库表达式。
- [x] 运行定向测试和本包测试，确认任务 3 转绿。

### 任务 4：Descriptor 查询、Schema 索引与不可变性

**Files:** `descriptor.go`、`index.go`、`validate.go`、`index_test.go`

- [x] 写 `TestDescriptorIndexesFieldsByEveryName`，分别通过逻辑名、JSON 名和列名取得同一 Field，并验证不存在名称返回 `false`。
- [x] 写 `TestCompileMergesSystemAndDeclaredIndexes`，确认索引顺序为两个系统索引，再按 Schema 顺序追加普通/唯一索引。
- [x] 写错误表，拒绝空/非法/重复索引名、空字段、重复字段和不存在逻辑字段，并验证系统索引不可覆盖。
- [x] 写 `TestDescriptorCollectionsAreImmutable`，编译后修改 `Schema.Indexes`、输入 `Fields`、`Fields()` 和 `Indexes()` 的返回结果，后续读取保持不变。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'Test(Descriptor|CompileMerges|CompileRejectsInvalidIndex)' -count=1
```

- [x] 实现私有 Field/Descriptor 值、三份名称索引、稳定字段/索引顺序、系统索引生成和逐层防御性复制。
- [x] 运行定向测试和 Race Test，确认任务 4 转绿。

### 任务 5：Descriptor 集合冲突与仓库门禁

**Files:** `validate.go`、`validate_test.go`、任务 4 全部文件

- [x] 写 `TestValidateSetRejectsPhysicalConflicts`，覆盖 nil Descriptor、重复表名和跨表重复物理索引名，并断言错误包含冲突双方表名。
- [x] 写 `TestValidateSetAcceptsIndependentDescriptors`，确认同名列在不同表允许、不同表和索引集合成功。
- [x] 写 `TestEntityErrorsAreCoreExceptions`，对实体、字段、索引和集合错误执行：

```go
var coreException *exception.BaseException
if !errors.As(err, &coreException) || coreException.Code != exception.CoreFail {
   t.Fatalf("error = %v, want CoreFail", err)
}
```

- [x] 实现无状态 `ValidateSet`，按输入顺序稳定报告首个冲突，不缓存 Descriptor。
- [x] 运行格式化和模块验证：

```bash
gofmt -w cool-next/core/entity/*.go
go test ./cool-next/core/entity -count=1
go test -race ./cool-next/core/entity -count=1
```

- [x] 运行仓库门禁：

```bash
go vet ./...
make check
```

- [x] 对照 04.1-04.12、专项设计完成标准和依赖边界逐项复核；扫描 TODO/TBD/占位实现；更新本计划状态为 `completed`。

## 4. 计划自检

- 04.1、04.2、04.8：任务 1、2；
- 04.3-04.5：任务 3；
- 04.6、04.7、04.11：任务 1、4；
- 04.9、04.10：任务 1、4；
- 04.12：任务 5；
- `DOValue/NewDO` 明确留给模块 05；
- 数据库方言、DDL 与生成器分别留给模块 06、07、13；
- 无 TBD、TODO、占位类型或隐式全局注册。
