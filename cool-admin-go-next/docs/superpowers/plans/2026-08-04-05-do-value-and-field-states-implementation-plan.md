# 模块 05：DOValue 与字段四态实施计划

> 状态：completed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `cool-next/core/entity` 交付由 Descriptor 创建的结构化 DOValue，严格区分未提交、零值、`false` 和显式 `null`，并向 GoFrame ORM 返回具体 `do:true` struct。

**Architecture:** 模块 04 的 Descriptor 在编译时按稳定字段顺序构造一次私有 GoFrame DO struct 类型和逻辑字段位置映射，`NewDO` 为每次写入创建独立状态和值。`SetColumn` 先完成逻辑字段、typed nil、实际 Go 值类型和 nullable 校验，再原子更新状态；`DBData` 返回当前具体 struct 值快照，未提交字段为 nil，显式空值为 `gdb.Raw("NULL")`。

**Tech Stack:** Go 1.26.4、GoFrame v2.10.2 `g.Meta/gdb.Raw/gerror`、模块 02 Core 异常、模块 04 Descriptor、标准库 reflection/testing/time

---

## 1. 实施约束

- 事实来源：`docs/superpowers/specs/2026-08-04-05-do-value-and-field-states-design.md`；
- 严格 TDD，每个行为先写测试并观察预期失败；
- 不实现 AST/codegen、数据库方言、DDL、连接、Mutable、CRUD、Hook 或实际 DML；
- 不使用 `g.Map`、`map[string]any`、全局注册表或运行时目录扫描；
- `SetColumn` 不接受宽松类型转换或调用方提供的 SQL Raw；
- 执行期间仓库建立了初始提交，但当前 `compile.go`、`index.go`、`validate.go` 已有用户修改；本模块原地保留这些修改，不创建提交、不改写历史；
- 现有 `.gitignore` 忽略 `*_test.go` 和 `/docs/`，本文及新增测试保存在工作区并参与实际验证，但不会出现在普通 `git status` 中；本模块不擅自改变仓库忽略策略。

## 2. 文件结构

| 文件 | 职责 |
|---|---|
| `cool-next/core/entity/types.go` | 增加 DOValue 能力接口和 Descriptor.NewDO |
| `cool-next/core/entity/descriptor.go` | Descriptor 持有不可变 DO shape 并创建 DOValue |
| `cool-next/core/entity/compile.go` | 在实体和字段已校验后编译一次 DO shape |
| `cool-next/core/entity/do.go` | 私有 DO shape、字段绑定、四态、SetColumn 和 DBData |
| `cool-next/core/entity/do_test.go` | 公开行为、四态、类型、覆盖、隔离和错误测试 |
| `cool-next/core/entity/do_goframe_test.go` | 具体 struct、g.Meta、do:true、orm Tag 和快照测试 |

## 3. 任务清单

### 任务 1：公开接口与 GoFrame DO Shape

**Files:** `types.go`、`descriptor.go`、`compile.go`、`do.go`、`do_goframe_test.go`

- [x] 写 `TestDescriptorNewDOCreatesGoFrameStruct`，编译包含 nullable 和普通字段的实体并验证：

```go
descriptor, err := Compile[doGoodsEntity, uint64](Schema{})
if err != nil {
   t.Fatal(err)
}

data := descriptor.NewDO().DBData()
typ := reflect.TypeOf(data)
if typ.Kind() != reflect.Struct {
   t.Fatalf("DBData type = %s, want struct", typ)
}
meta := typ.Field(0)
if !meta.Anonymous || meta.Type != reflect.TypeFor[g.Meta]() {
   t.Fatalf("meta field = %#v", meta)
}
if got := meta.Tag.Get("orm"); got != "table:do_goods,do:true" {
   t.Fatalf("meta orm = %q", got)
}
for index, column := range []string{"id", "createTime", "updateTime", "title", "remark", "enabled"} {
   field := typ.Field(index + 1)
   if field.Type != reflect.TypeFor[any]() || field.Tag.Get("orm") != column {
      t.Fatalf("DO field %d = %s/%q", index, field.Type, field.Tag.Get("orm"))
   }
}
```

- [x] 运行：

```bash
go test ./cool-next/core/entity -run TestDescriptorNewDOCreatesGoFrameStruct -count=1
```

预期：编译失败，提示 `Descriptor` 没有 `NewDO` 或 `DOValue` 尚未定义。

- [x] 在 `types.go` 增加公开能力：

```go
type DOValue interface {
   Has(field string) bool
   IsNull(field string) bool
   SetColumn(field string, value any) error
   DBData() any
}

type Descriptor[E any, ID comparable] interface {
   Metadata
   EntityType() reflect.Type
   IDType() reflect.Type
   NewDO() DOValue
}
```

- [x] 在 `do.go` 定义不可变 shape 和字段绑定；运行时 struct 使用安全、稳定的内部导出字段名，物理列名始终来自 `orm` Tag：

```go
type doFieldBinding struct {
   field       Field
   structIndex int
   stateIndex  int
   valueType   reflect.Type
}

type doShape struct {
   entityType reflect.Type
   structType reflect.Type
   fields     map[string]doFieldBinding
   fieldCount int
}

func compileDOShape(entityType reflect.Type, table string, fields []Field) *doShape {
   structFields := []reflect.StructField{{
      Name:      "Meta",
      Type:      reflect.TypeFor[g.Meta](),
      Anonymous: true,
      Tag:       reflect.StructTag(fmt.Sprintf(`orm:"table:%s,do:true"`, table)),
   }}
   bindings := make(map[string]doFieldBinding, len(fields))
   for index, field := range fields {
      structFields = append(structFields, reflect.StructField{
         Name: fmt.Sprintf("Field%d", index),
         Type: reflect.TypeFor[any](),
         Tag:  reflect.StructTag(fmt.Sprintf(`orm:"%s"`, field.Column())),
      })
      valueType := field.GoType()
      if valueType.Kind() == reflect.Pointer {
         valueType = valueType.Elem()
      }
      bindings[field.Name()] = doFieldBinding{
         field:       field,
         structIndex: index + 1,
         stateIndex:  index,
         valueType:   valueType,
      }
   }
   return &doShape{
      entityType: entityType,
      structType: reflect.StructOf(structFields),
      fields:     bindings,
      fieldCount: len(fields),
   }
}
```

- [x] 在 `descriptorValue` 保存 `*doShape`，在 `Compile` 完成字段和索引校验后调用 `compileDOShape`，并实现：

```go
func (d *descriptorValue[E, ID]) NewDO() DOValue {
   return d.doShape.newValue()
}
```

- [x] 为 `doShape.newValue` 建立最小实例，使 `DBData()` 返回全 nil 的具体 struct 值；运行定向测试，确认任务 1 转绿。

### 任务 2：未提交、普通值、零值与 false

**Files:** `do.go`、`do_test.go`

- [x] 写 `TestDOValuePreservesSubmittedZeroValues`，覆盖 `0`、`false`、空字符串和空但非 nil 的 `[]byte{}`：

```go
value := descriptor.NewDO()
for field, input := range map[string]any{
   "count":   int64(0),
   "enabled": false,
   "title":   "",
   "payload": []byte{},
} {
   if err := value.SetColumn(field, input); err != nil {
      t.Fatalf("SetColumn(%s) error = %v", field, err)
   }
   if !value.Has(field) || value.IsNull(field) {
      t.Fatalf("state %s = has:%v null:%v", field, value.Has(field), value.IsNull(field))
   }
}
if value.Has("remark") || value.IsNull("remark") {
   t.Fatal("unsubmitted remark must stay absent")
}
```

- [x] 运行：

```bash
go test ./cool-next/core/entity -run TestDOValuePreservesSubmittedZeroValues -count=1
```

预期：失败，因为 `SetColumn/Has/IsNull` 尚未实现状态语义。

- [x] 在 `do.go` 定义私有状态和值：

```go
type doFieldState uint8

const (
   doFieldUnset doFieldState = iota
   doFieldValue
   doFieldNull
)

type doValue struct {
   shape  *doShape
   data   reflect.Value
   states []doFieldState
}

func (s *doShape) newValue() DOValue {
   return &doValue{
      shape:  s,
      data:   reflect.New(s.structType).Elem(),
      states: make([]doFieldState, s.fieldCount),
   }
}
```

- [x] 实现逻辑字段查询、严格普通值类型校验和成功后赋值；失败路径不得修改状态：

```go
func (v *doValue) SetColumn(name string, value any) error {
   binding, exists := v.shape.fields[name]
   if !exists {
      return newEntityError("实体 %s 不存在逻辑字段 %s", v.shape.entityType, name)
   }
   if value == nil {
      return v.setNull(binding)
   }
   actualType := reflect.TypeOf(value)
   if isTypedNil(value) {
      if !binding.acceptsNilType(actualType) {
         return v.typeError(name, binding.valueType, actualType)
      }
      return v.setNull(binding)
   }
   if actualType != binding.valueType {
      return v.typeError(name, binding.valueType, actualType)
   }
   v.data.Field(binding.structIndex).Set(reflect.ValueOf(value))
   v.states[binding.stateIndex] = doFieldValue
   return nil
}
```

- [x] 实现 `Has`、`IsNull` 和 `DBData`；未知字段查询返回 false，`DBData` 调用 `v.data.Interface()` 返回 struct 值副本。
- [x] 运行定向测试与本包全测，确认任务 2 转绿且模块 04 行为无回归。

### 任务 3：显式 null、typed nil 与失败原子性

**Files:** `do.go`、`do_test.go`

- [x] 写 `TestDOValueHandlesExplicitNull`，验证 nullable 字段的 untyped nil、匹配的 typed nil pointer/slice 均转换为 `gdb.Raw("NULL")`：

```go
value := descriptor.NewDO()
var nullString *string
if err := value.SetColumn("remark", nullString); err != nil {
   t.Fatal(err)
}
if !value.Has("remark") || !value.IsNull("remark") {
   t.Fatal("remark must be explicit null")
}

data := reflect.ValueOf(value.DBData())
raw, ok := data.Field(remarkStructIndex).Interface().(gdb.Raw)
if !ok || raw != gdb.Raw("NULL") {
   t.Fatalf("remark DB value = %#v", data.Field(remarkStructIndex).Interface())
}
```

- [x] 写表驱动 `TestDOValueRejectsInvalidNullAndTypes`，覆盖：unknown field、非 nullable nil、非 nullable typed nil bytes、nullable 字段的不相关 typed nil、`int/int64/uint64` 宽度错误、普通类型与 named type 混用、`time.Time/gtime.Time` 混用、非 nil pointer 以及调用方 `gdb.Raw`。
- [x] 写 `TestDOValueFailedSetKeepsPreviousState`：先提交普通值，再执行错误类型/null 赋值，断言 `Has/IsNull/DBData` 仍保留第一次成功结果。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'TestDOValue(HandlesExplicitNull|RejectsInvalidNullAndTypes|FailedSetKeepsPreviousState)' -count=1
```

预期：显式 null 或严格失败原子性尚未完整实现而失败。

- [x] 实现 typed nil 识别和兼容规则：

```go
func isTypedNil(value any) bool {
   reflected := reflect.ValueOf(value)
   switch reflected.Kind() {
   case reflect.Pointer, reflect.Slice:
      return reflected.IsNil()
   default:
      return false
   }
}

func (b doFieldBinding) acceptsNilType(actual reflect.Type) bool {
   return actual == b.field.GoType() || actual == b.valueType
}
```

- [x] 实现 `setNull`：先检查 `Nullable()`，失败只返回 Core 错误；成功后把对应 DO 字段设为 `gdb.Raw("NULL")` 并把状态更新为 `doFieldNull`。
- [x] 实现类型错误，仅输出实体、逻辑字段、期望类型和实际类型；不得格式化 value 或 Raw 内容。
- [x] 运行定向测试和本包全测，确认任务 3 转绿。

### 任务 4：类型保真、覆盖、隔离与 DBData 快照

**Files:** `do_test.go`、`do_goframe_test.go`、`do.go`

- [x] 写 `TestDOValuePreservesExactGoTypes`，表驱动覆盖 bool、全部 int/uint 宽度、`math.MaxUint64`、float32/64、string、`[]byte`、`time.Time`、`gtime.Time` 和至少一个 named scalar；从 DBData 对应 `any` 字段取值并断言 `reflect.TypeOf` 与 `reflect.DeepEqual` 均保持一致。
- [x] 写 `TestDOValueLastSuccessfulSetWins`，验证普通值 -> 普通值、普通值 -> null、null -> 普通值，最后状态和值准确。
- [x] 写 `TestDescriptorNewDOValuesAreIsolated`：两个 `NewDO` 实例设置不同字段，彼此 `Has/IsNull/DBData` 不受影响。
- [x] 写 `TestDOValueDBDataReturnsSnapshot`：取得第一次 DBData 后重新赋值，第一次 struct 中的 `any` 字段保持原值，第二次反映新值。
- [x] 写 `TestDOValueErrorsAreCoreExceptionsAndHideValues`，对 unknown/type/null 错误执行 `errors.As`，断言 `Code == exception.CoreFail`，并确保秘密字符串和 `gdb.Raw` 内容未出现在错误文本。
- [x] 运行：

```bash
go test ./cool-next/core/entity -run 'Test(DOValue|DescriptorNewDO)' -count=1
```

预期：若任务 1-3 实现完整则直接通过；否则只补最小缺口，不改变已冻结接口。

- [x] 运行 Race Test，确认 Descriptor 的共享 shape 只读、每个 DOValue 的状态独立：

```bash
go test -race ./cool-next/core/entity -count=1
```

预期：PASS，无 data race。

### 任务 5：格式化、回归与仓库门禁

**Files:** `types.go`、`descriptor.go`、`compile.go`、`do.go`、`do_test.go`、`do_goframe_test.go`、本计划

- [x] 运行格式化：

```bash
gofmt -w cool-next/core/entity/*.go
```

- [x] 运行模块验证：

```bash
go test ./cool-next/core/entity -count=1
go test -race ./cool-next/core/entity -count=1
```

预期：全部 PASS。

- [x] 运行仓库门禁：

```bash
go vet ./...
make check
```

预期：全部成功，模块 01-04 无回归。

- [x] 扫描范围和占位实现：

```bash
rg -n 'TODO|TBD|map\[string\]any|g\.Map|modules_gen|database/sql' cool-next/core/entity
```

预期：没有模块 05 新增的 TODO/TBD、map 写入对象、生成器或数据库实现；既有注释若命中需逐项说明而不是机械删除。

- [x] 对照 05.1-05.9、专项设计完成标准和依赖边界逐项复核；将本计划状态更新为 `completed`，并把所有已执行步骤更新为 `[x]`。

## 4. 计划自检

- 05.1：任务 2、3、4；
- 05.2：任务 1；
- 05.3：任务 2、3；
- 05.4：任务 2、3、4；
- 05.5：任务 3；
- 05.6：任务 1；
- 05.7：任务 1-4；
- 05.8：任务 1、5；
- 05.9：任务 2-4；
- 公开签名、私有类型名和测试调用保持一致；
- 无数据库连接、方言、DDL、codegen、Mutable、CRUD 或 Hook 越界；
- 无 TBD、TODO、占位实现或依赖后续补全的步骤。
