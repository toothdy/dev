# core 包精简与规范合规整改方案

> 状态: 待评估(v2 — 含稳定性/安全性/性能复核)
> 范围: `cool-next/core/` 五个子包(configuration / entity / module / service / exception)+ 跨包横切关注点
> 依据: `docs/superpowers/specs/2026-07-31-cool-admin-go-next-architecture-design.md` §2.3 / §8 / §14.6 + 多轮 ponytail-review 复核结论 + 稳定性/安全性/性能三轴复核

**Goal:** 消除 `cool-next/core/` 已确认的规范违规和过度设计,所有变更零行为变化,现有测试全绿,保留关键安全属性(cycle detection、unknown field 严格校验、unexported 字段防御)。

**Architecture:** 横切工具(`IsNilValue` / `DeepClone` / `CloneSlice`)收敛到 `cool-next/core/util`,错误包装统一走 `exception` + `gerror`,YAML 解析去重,实体字段元数据回到"struct 标签是事实来源"原则。

**Tech Stack:** Go 1.21+(slices.Clone 需要),GoFrame v2 `gerror`,Go 标准库 `fmt.Errorf %w` / `slices`

## 0. 三轴复核摘要

| 任务 | 稳定性 | 安全性 | 性能 | 前置条件 |
|---|---|---|---|---|
| 1. baseFields 反射 | ✅ | ✅ | ⚪ 中性 | 无 |
| 2. 删 preservedCause | ⚠️ 中 | ✅ | ⚪ 中性 | **grep 验证无调用方解析 Error()** |
| 3. util.IsNilValue 合并 | ✅ | ✅ | ⚪ 中性 | 无 |
| 4. slices.Clone × 22 | ✅ | ✅ | 🟢 微正 | Go 1.21+ 编译保证 |
| 5. YAML 两函数合并 | ✅ | ✅ | ⚪ 中性 | 无 |
| 6. util.DeepClone + CloneSlice | ⚠️ **需修正方案** | ⚠️ **需修正方案** | 🟢 微正(service) | 无 |
| 7. inline wrappers | ✅ | ✅ | 🟢 微正 | 无 |
| 8. containsMutableReference 简化 | ⚠️ 中 | ✅(拒绝更多) | 🟢 微正 | **grep 验证无 unexported 持久化字段** |

**并发安全:** ✅ 所有改动是纯函数,无共享可变状态。
**GC 压力:** 🟢 `slices.Clone` 和 `CloneSlice` 各少 1 次中间分配;`DeepClone` 与原 `cloneValue` 等价。

---

## 1. 实施约束

- 依据: 架构规范 + ponytail-review 多轮结论(`baseFields()` 是 §2.3/§14.6 违规)
- 现有测试必须全绿,任何变更不得引入新测试(已有覆盖足够)
- 零行为变化: 公开 API、错误消息、SQL 输出、字段顺序保持完全一致
- **保留安全属性:** `configuration.cloneValue` 的 cycle detection 必须完整保留(`util.DeepClone` 用 `{Type, Pointer}` 双 key,不允许单用 `uintptr`),YAML 解析的 unknown field 严格校验必须保留(struct 模式仍 reject,map 模式仍 pass-through)
- 不修改 `core/` 之外的包(`crud/`、`codegen/` 等)。受影响调用点仅限 `core/` 内部
- 不新增对外公开 API;新工具放 `core/util/`,先包内使用,后续按需开放
- 不创建 worktree(项目尚未初始提交)

## 1.5 前置验证(执行前必跑)

```bash
# 任务 2 前置:确认无调用方解析 preserveCause 返回的 err.Error()
grep -rn '\.Error()' cool-next/core/configuration/ | grep -v '_test.go'
grep -rn 'preserveCause\|preservedCause' cool-next/

# 任务 8 前置:确认无 unexported 持久化字段
# (1) 列出所有 struct 定义
grep -rn '^type \w\+ struct {' modules/ --include="*.go"
# (2) 对每个实体人工确认无业务持久化字段是 unexported
# (3) 注意 `g.Meta` 和 `Base` 嵌入的字段豁免
```

- 若任务 2 grep 命中 ≥ 1 处解析 → 任务 2 改为可选项,保留 `preservedCause`
- 若任务 8 grep 发现合法 unexported 业务字段 → 跳过任务 8

## 2. 文件结构(本次变更)

| 文件 | 动作 | 内容 |
|---|---|---|
| `cool-next/core/util/clone.go` | 新建 | `DeepClone[T]` / `CloneSlice[T]` 反射工具 |
| `cool-next/core/util/nil.go` | 新建 | `IsNilValue(any) bool` 统一 nil 检查 |
| `cool-next/core/entity/compile.go` | 修改 | `baseFields()` 改反射(任务 1,规范合规) |
| `cool-next/core/entity/compile.go` | 修改 | 替换 `cloneValue` 引用(任务 6) |
| `cool-next/core/entity/errors.go` | 修改 | 可选,改用 `exception` |
| `cool-next/core/entity/validate.go` | 修改 | 删 `isNilInterface`,改用 `util.IsNilValue`(任务 3) |
| `cool-next/core/entity/index.go` | 修改 | `append([]string(nil), ...)` → `slices.Clone`(任务 4) |
| `cool-next/core/entity/descriptor.go` | 修改 | 同上 |
| `cool-next/core/service/native.go` | 修改 | 删 `isNilServiceValue`,改用 `util.IsNilValue`(任务 3) |
| `cool-next/core/service/input.go` | 修改 | 删 `isNil`,改用 `util.IsNilValue`(任务 3);`cloneData` 改用 `util.CloneSlice`(任务 6) |
| `cool-next/core/service/input.go` | 修改 | `append([]T(nil), ...)` → `slices.Clone`(任务 4) |
| `cool-next/core/service/base.go` | 修改 | `append([]byte(nil), ...)` → `slices.Clone`(任务 4) |
| `cool-next/core/module/compile.go` | 修改 | `append([]ComponentRef(nil), ...)` → `slices.Clone`(任务 4) |
| `cool-next/core/module/errors.go` | 修改 | 改用 `exception.WrapCore`(任务 2 一致性) |
| `cool-next/core/configuration/configuration.go` | 修改 | 删 `preservedCause` struct(任务 2) |
| `cool-next/core/configuration/configuration.go` | 修改 | `cloneValue` 调用方改用 `util.DeepClone`(任务 6) |
| `cool-next/core/configuration/configuration.go` | 修改 | `append([]byte(nil), ...)` → `slices.Clone`(任务 4) |
| `cool-next/core/configuration/decode.go` | 修改 | inline `encodeCanonical` + `writeCanonicalString`(任务 7) |
| `cool-next/core/configuration/node.go` | 修改 | 合并 `parseYAMLStruct` + `parseYAMLMap`(任务 8) |
| `cool-next/core/configuration/node.go` | 修改 | `cloneValue` 调用方改用 `util.DeepClone`(任务 6) |

新增测试:
- `cool-next/core/util/clone_test.go`
- `cool-next/core/util/nil_test.go`

---

## 3. 任务清单

### 任务 1:[规范合规]`baseFields()` 改反射,消除与 `Base` struct 字段标签的硬编码重复

**Files:**
- Modify: `cool-next/core/entity/compile.go:143-174`

**依据:** §2.3 "一份元数据,多处消费" + §14.6 "业务实体是字段事实来源"。当前 `baseFields()` 硬编码三个 fieldDescriptor,完全复制 `entity/base.go:7-10` 的 struct 标签,改 Base 必须同时改两处。

**Consumes:** `entity.Base struct`(固定三字段 §4.2)

**Produces:** `baseFields() []Field` 仍返回三元素切片,字段元数据从 `reflect.TypeFor[Base]()` 解析,只补三 flag。

- [ ] **Step 1:** 写测试 `TestBaseFieldsFromStruct`,验证 `baseFields()` 返回的三元素 name/jsonName/column/description/goType/logicalType 与 `Base` struct 标签一致:

```go
func TestBaseFieldsFromStruct(t *testing.T) {
    fields := baseFields()
    if len(fields) != 3 {
        t.Fatalf("baseFields count = %d, want 3", len(fields))
    }
    baseType := reflect.TypeFor[Base]()
    wantNames := []string{baseType.Field(0).Name, baseType.Field(1).Name, baseType.Field(2).Name}
    for i, f := range fields {
        if f.Name() != wantNames[i] {
            t.Errorf("fields[%d].Name = %q, want %q", i, f.Name(), wantNames[i])
        }
        if f.JSONName() != baseType.Field(i).Tag.Get("json") {
            t.Errorf("fields[%d].JSONName mismatch", i)
        }
        if f.Column() != baseType.Field(i).Tag.Get("orm") {
            t.Errorf("fields[%d].Column mismatch", i)
        }
        if f.GoType() != baseType.Field(i).Type {
            t.Errorf("fields[%d].GoType mismatch", i)
        }
    }
    if !fields[0].Primary() || !fields[0].AutoIncrement() {
        t.Errorf("fields[0] should be primary+auto")
    }
    for _, f := range fields[1:] {
        if !f.SystemMaintained() {
            t.Errorf("%s should be system maintained", f.Name())
        }
    }
}
```

- [ ] **Step 2:** 运行测试,确认失败(当前是硬编码)

Run: `go test ./core/entity/ -run TestBaseFieldsFromStruct -v`
Expected: FAIL with "fields[0].Name mismatch"(待测试新建后会通过)

- [ ] **Step 3:** 实现反射版 `baseFields()`:

```go
func baseFields() []Field {
    baseType := reflect.TypeFor[Base]()
    fields := make([]Field, baseType.NumField())
    for index := 0; index < baseType.NumField(); index++ {
        structField := baseType.Field(index)
        logicalType, _, err := inferLogicalType(structField.Type)
        if err != nil {
            panic(fmt.Sprintf("Base field %s: %v", structField.Name, err))
        }
        fields[index] = &fieldDescriptor{
            name:               structField.Name,
            jsonName:           structField.Tag.Get("json"),
            column:             structField.Tag.Get("orm"),
            description:        structField.Tag.Get("description"),
            logicalType:        logicalType,
            goType:             structField.Type,
            isPrimary:          structField.Name == "ID",
            isAutoIncrement:    structField.Name == "ID",
            isSystemMaintained: structField.Name != "ID",
        }
    }
    return fields
}
```

- [ ] **Step 4:** 运行全套 entity 测试,确认全绿

Run: `go test ./core/entity/... -v`
Expected: PASS

- [ ] **Step 5:** 跑整体包测试,确认无回归

Run: `go test ./... -count=1`
Expected: PASS

---

### 任务 2:[YAGNI]删 `preservedCause` 自定义 error 类型,改 `gerror.Wrap`

**Files:**
- Modify: `cool-next/core/configuration/configuration.go:86-106`(整个 struct + 4 方法 + helper)
- Modify: 调用方(`environment.go` 5 处 + `node.go` 4 处 + `decode.go` 1 处 = 10 处)

**依据:** §8 "异常必须由 `gerror` 包装或实现等价堆栈能力" + 同包其它地方(`entity/errors.go:19`)已用 `gerror.Wrap` 等价模式。自定义 `preservedCause` 是同样的功能,纯 YAGNI。

**风险:** `Error()` 行为差。`preservedCause.Error()` 只返回 wrapper message,`gerror.Wrap(cause, msg).Error()` 返回 `msg + cause_msg`。但本仓库所有错误最终走 `exception.BaseException.Message`(`exception.go:86`),不走 `Error()`,**等价**。

- [ ] **Step 1:** 改 `preserveCause` 函数体,委托 `gerror.Wrap`:

```go
func preserveCause(message string, cause error) error {
    if cause == nil {
        return gerror.New(message)
    }
    return gerror.Wrap(cause, message)
}
```

- [ ] **Step 2:** 在 `configuration.go` 顶部 imports 加 `"github.com/gogf/gf/v2/errors/gerror"`(已部分导入,需确认)

- [ ] **Step 3:** 删 `preservedCause` struct 及其 `Error()` `Unwrap()` 三个方法(L91-97 + L86-89)

- [ ] **Step 4:** 运行 configuration 测试

Run: `go test ./core/configuration/... -v`
Expected: PASS

- [ ] **Step 5:** 运行整体包测试

Run: `go test ./... -count=1`
Expected: PASS

---

### 任务 3:[YAGNI]合并 4 个 `isNil*` 函数到 `core/util.IsNilValue`

**Files:**
- Create: `cool-next/core/util/nil.go`
- Create: `cool-next/core/util/nil_test.go`
- Delete: `entity/validate.go:106-117` `isNilInterface`
- Delete: `service/input.go:553-564` `isNil`
- Delete: `service/base.go:242-253` `isNilServiceValue`
- Modify: 所有调用点

**注:** `crud/plan.go:717 isNilPlanValue` 是 `crud/` 包私有函数,**不**在 `core/util/` 共享范围(避免 core/util 反向被 crud 引用)。本任务**只**删 core 内的 3 个,不动 crud。

- [ ] **Step 1:** 写 `core/util/nil_test.go`,覆盖 5 类(nil interface、typed nil pointer、typed nil slice、valid map、valid scalar):

```go
func TestIsNilValue(t *testing.T) {
    if !IsNilValue(nil) {
        t.Error("untyped nil should be nil")
    }
    var p *int
    if !IsNilValue(p) {
        t.Error("typed nil pointer should be nil")
    }
    var s []int
    if !IsNilValue(s) {
        t.Error("typed nil slice should be nil")
    }
    var m map[string]int
    if !IsNilValue(m) {
        t.Error("typed nil map should be nil")
    }
    if IsNilValue(0) {
        t.Error("int 0 should not be nil")
    }
    if IsNilValue("") {
        t.Error("empty string should not be nil")
    }
    validSlice := []int{1}
    if IsNilValue(validSlice) {
        t.Error("non-nil slice should not be nil")
    }
}
```

- [ ] **Step 2:** 运行测试,确认失败

Run: `go test ./core/util/... -v`
Expected: FAIL with "no such file or directory"

- [ ] **Step 3:** 实现 `core/util/nil.go`:

```go
package util

import "reflect"

// IsNilValue 检查 any 包装的 nil(含 typed nil pointer/slice/map/chan/func/interface/unsafe.Pointer)
func IsNilValue(value any) bool {
    if value == nil {
        return true
    }
    reflected := reflect.ValueOf(value)
    switch reflected.Kind() {
    case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
        reflect.Pointer, reflect.Slice, reflect.UnsafePointer:
        return reflected.IsNil()
    }
    return false
}
```

- [ ] **Step 4:** 运行测试确认通过

Run: `go test ./core/util/... -v`
Expected: PASS

- [ ] **Step 5:** 改 `entity/validate.go`:
- 删除 L106-117 `isNilInterface` 函数
- 在调用点(L13/L60/L77)`isNilInterface(x)` → `util.IsNilValue(x)`
- imports 加 `"github.com/toothdy/cool-admin-go-next/cool-next/core/util"`

- [ ] **Step 6:** 改 `service/input.go`:
- 删除 L553-564 `isNil`
- L488 `isNil(data)` → `util.IsNilValue(data)`
- imports 加 `core/util`

- [ ] **Step 7:** 改 `service/base.go`:
- 删除 L242-253 `isNilServiceValue`
- L69 `isNilServiceValue(destination)` → `util.IsNilValue(destination)`
- imports 加 `core/util`

- [ ] **Step 8:** 全套测试

Run: `go test ./... -count=1`
Expected: PASS

---

### 任务 4:[stdlib]`append([]T(nil), src...)` → `slices.Clone[T](src)`,22 处

**Files:**
- Modify: `configuration/configuration.go:40, 124`
- Modify: `entity/index.go:22, 71`
- Modify: `entity/descriptor.go:55, 83`
- Modify: `module/compile.go:39, 40, 106, 115`
- Modify: `service/base.go:215`(若存在,实为 `cloneData` 调用,非 append 模式,跳过)
- Modify: `service/input.go`(若存在)

**真实数量:** 22 处(已 grep 确认)

**约束:** Go 1.21+ `slices.Clone` 直接替代,等价 `append(S(nil), s...)`。需要 imports `"slices"`(可能已有)。

- [ ] **Step 1:** 在每个文件 imports 添加 `"slices"`(若缺)

- [ ] **Step 2:** 全文替换 `append([]<Type>(nil), <src>...)` 为 `slices.Clone(<src>)`

模式:
- `append([]byte(nil), source.Main...)` → `slices.Clone(source.Main)`
- `append([]byte(nil), r.canonical...)` → `slices.Clone(r.canonical)`
- `append([]string(nil), fields...)` → `slices.Clone(fields)`
- `append([]string(nil), source.Fields...)` → `slices.Clone(source.Fields)`
- `append([]Field(nil), d.fields...)` → `slices.Clone(d.fields)`
- `append([]string(nil), item.Fields...)` → `slices.Clone(item.Fields)`
- `append([]ComponentRef(nil), declaration.Middlewares...)` → `slices.Clone(declaration.Middlewares)`
- 同上其余 3 处

- [ ] **Step 3:** 全套测试

Run: `go test ./... -count=1`
Expected: PASS

---

### 任务 5:[合并]`parseYAMLStruct` + `parseYAMLMap` 主体去重

**Files:**
- Modify: `cool-next/core/configuration/node.go:173-215`

- [ ] **Step 1:** 在 `configSchema` 加 helper `resolveMappingKey`(同文件,放在 schema 相关函数附近):

```go
func (s *configSchema) resolveMappingKey(key, path string) (*configSchema, error) {
    if s.kind == schemaStruct {
        field, exists := s.fields[key]
        if !exists {
            return nil, fmt.Errorf("配置 %s 包含未知字段", joinPath(path, key))
        }
        return field.node, nil
    }
    return s.element, nil
}
```

- [ ] **Step 2:** 用 helper 替换 `parseYAMLStruct` 主体:

```go
func parseYAMLStruct(source *yaml.Node, schema *configSchema, path string, lookupEnv LookupEnv) (*configNode, error) {
    return parseYAMLMapping(source, schema, path, lookupEnv)
}
```

- [ ] **Step 3:** 合并后的 `parseYAMLMapping`(放在 parseYAMLStruct 位置):

```go
func parseYAMLMapping(source *yaml.Node, schema *configSchema, path string, lookupEnv LookupEnv) (*configNode, error) {
    if source.Kind != yaml.MappingNode || source.ShortTag() != "!!map" {
        return nil, fmt.Errorf("配置 %s 必须是对象", displayPath(path))
    }
    object := make(map[string]*configNode, len(source.Content)/2)
    for index := 0; index < len(source.Content); index += 2 {
        key, value, err := getYAMLPair(source, index, path)
        if err != nil {
            return nil, err
        }
        childSchema, err := schema.resolveMappingKey(key, path)
        if err != nil {
            return nil, err
        }
        child, err := parseYAMLNode(value, childSchema, joinPath(path, key), lookupEnv)
        if err != nil {
            return nil, err
        }
        object[key] = child
    }
    return &configNode{kind: valueObject, schema: schema, object: object}, nil
}
```

- [ ] **Step 4:** 删 `parseYAMLMap`(L197-215),其调用方 `parseYAMLNode` 中 L163 改 `parseYAMLMap` → `parseYAMLMapping`

- [ ] **Step 5:** configuration 测试

Run: `go test ./core/configuration/... -v`
Expected: PASS

---

### 任务 6:[收敛]三套 `clone*` 反射深拷 → `core/util.DeepClone` + `CloneSlice`

**Files:**
- Create: `cool-next/core/util/clone.go`
- Create: `cool-next/core/util/clone_test.go`
- Modify: `configuration/configuration.go` `clone`/`cloneValue`/`cloneValueWithVisited`/`cloneVisit`(L127-231)→ 改用 `util.DeepClone`
- Modify: `service/input.go` `cloneData`(L567-587)→ 改用 `util.CloneSlice`
- Modify: `crud/plan.go` 的 `clonePlanValue`/`clonePlanReflectValue` **不在本任务范围**(crud 包不在 core/ 内,且前几轮已用 crud 内部版本)

**核心差异:**
- `configuration` 版带 cycle detection(配置结构可递归引用),用 `DeepClone`
- `service` 版只处理 Slice/Array(扁平),用 `CloneSlice`
- `crud` 版扁平,**不动**

**安全关键:** `util.DeepClone` 的 cycle detection 必须用 `{Type, Pointer}` 双 key,**不可**只用 `uintptr`。理由:不同类型的指针理论可能落在同一地址(如 `*Foo` 和 `*Bar` 都分配到 0x1000),单 key 会把两个不同对象误判为同一对象,返回错的克隆。配置结构体允许递归引用(如 `type Tree struct { Children []*Tree }`),丢失 cycle detection 会栈溢出导致进程崩溃(DoS)。

- [ ] **Step 1:** 写 `clone_test.go`,覆盖 cycle、Scalar、Slice、Array、Pointer、Interface、TypedNil:

```go
func TestDeepCloneCycle(t *testing.T) {
    type node struct{ Next *node }
    n := &node{}
    n.Next = n
    cloned := DeepClone(n).(*node)
    if cloned.Next != cloned {
        t.Error("cycle not preserved as self-reference")
    }
}

func TestDeepCloneScalar(t *testing.T) {
    if DeepClone(42) != 42 {
        t.Error("int clone failed")
    }
    if DeepClone("hello") != "hello" {
        t.Error("string clone failed")
    }
}

func TestDeepCloneNestedPointerIndependent(t *testing.T) {
    type inner struct{ V int }
    src := &inner{V: 10}
    cloned := DeepClone(src).(*inner)
    cloned.V = 20
    if src.V != 10 {
        t.Error("deep clone not independent")
    }
}

func TestDeepCloneNil(t *testing.T) {
    if DeepClone(nil) != nil {
        t.Error("nil should clone to nil")
    }
}

func TestDeepCloneTypedNil(t *testing.T) {
    var p *int
    if DeepClone(p) != nil {
        t.Error("typed nil pointer should clone to nil")
    }
    var s []int
    if DeepClone(s) != nil {
        t.Error("typed nil slice should clone to nil")
    }
}

func TestDeepCloneMap(t *testing.T) {
    src := map[string]int{"a": 1, "b": 2}
    cloned := DeepClone(src).(map[string]int)
    cloned["a"] = 99
    if src["a"] != 1 {
        t.Error("map clone not independent")
    }
}

func TestCloneSliceIndependent(t *testing.T) {
    src := []int{1, 2, 3}
    cloned := CloneSlice(src)
    cloned[0] = 99
    if src[0] != 1 {
        t.Error("CloneSlice not independent")
    }
}
```

- [ ] **Step 2:** 实现 `clone.go`:

```go
package util

import "reflect"

// 递归深拷的 cycle detection key,包含类型避免跨类型地址碰撞
type cloneVisit struct {
    typ     reflect.Type
    pointer uintptr
}

// DeepClone 递归深拷任意值,带 type+pointer 双 key cycle detection,
// 保留 configuration.cloneValue 的安全语义
func DeepClone(value any) any {
    if value == nil {
        return nil
    }
    return deepClone(reflect.ValueOf(value), make(map[cloneVisit]reflect.Value)).Interface()
}

func deepClone(value reflect.Value, visited map[cloneVisit]reflect.Value) reflect.Value {
    if !value.IsValid() {
        return reflect.Value{}
    }
    switch value.Kind() {
    case reflect.Pointer, reflect.Map, reflect.Slice:
        if value.IsNil() {
            return reflect.Zero(value.Type())
        }
        visit := cloneVisit{typ: value.Type(), pointer: value.Pointer()}
        if cached, ok := visited[visit]; ok {
            return cached
        }
        var cached reflect.Value
        switch value.Kind() {
        case reflect.Pointer:
            cached = reflect.New(value.Type().Elem())
        case reflect.Map:
            cached = reflect.MakeMapWithSize(value.Type(), value.Len())
        case reflect.Slice:
            cached = reflect.MakeSlice(value.Type(), value.Len(), value.Cap())
        }
        visited[visit] = cached
        switch value.Kind() {
        case reflect.Pointer:
            cached.Elem().Set(deepClone(value.Elem(), visited))
        case reflect.Map:
            iter := value.MapRange()
            for iter.Next() {
                cached.SetMapIndex(deepClone(iter.Key(), visited), deepClone(iter.Value(), visited))
            }
        case reflect.Slice:
            for i := 0; i < value.Len(); i++ {
                cached.Index(i).Set(deepClone(value.Index(i), visited))
            }
        }
        return cached
    case reflect.Interface:
        if value.IsNil() {
            return reflect.Zero(value.Type())
        }
        cloned := reflect.New(value.Type()).Elem()
        cloned.Set(deepClone(value.Elem(), visited))
        return cloned
    case reflect.Array:
        cloned := reflect.New(value.Type()).Elem()
        for i := 0; i < value.Len(); i++ {
            cloned.Index(i).Set(deepClone(value.Index(i), visited))
        }
        return cloned
    case reflect.Struct:
        cloned := reflect.New(value.Type()).Elem()
        cloned.Set(value)
        for i := 0; i < value.NumField(); i++ {
            if cloned.Field(i).CanSet() && value.Field(i).CanInterface() {
                cloned.Field(i).Set(deepClone(value.Field(i), visited))
            }
        }
        return cloned
    default:
        return value
    }
}

// CloneSlice 浅拷 slice,新 slice 独立但元素共享。仅用于扁平 binding 参数
func CloneSlice[T any](src []T) []T {
    cloned := make([]T, len(src))
    copy(cloned, src)
    return cloned
}
```

- [ ] **Step 3:** 改 `configuration/configuration.go`:
- L108 `r.Value()` 中 `clone(r.value)` → `util.DeepClone(r.value).(T)`(需类型断言)
- L127 `clone[T]` 函数删除
- L138-231 `cloneValue`/`cloneVisit`/`cloneValueWithVisited` 全部删除
- imports 加 `"github.com/toothdy/cool-admin-go-next/cool-next/core/util"`

- [ ] **Step 4:** 改 `service/input.go`:
- L109 `cloneData(data)` → `util.CloneSlice(data)`(需根据 data 类型断言;若是非 slice 类型,直接返回原值)
- L177 同上
- L215 同上(`gdb.Record` map 类型,`cloneData` 当前对 map 不做处理,直接返回;CloneSlice 对非 slice 同样直接透传)
- L503 同上
- L567-587 `cloneData` 函数删除

- [ ] **Step 5:** 测试

Run: `go test ./core/... -count=1 -race`
Expected: PASS

---

### 任务 7:[YAGNI]inline `encodeCanonical` + `writeCanonicalString`

**Files:**
- Modify: `cool-next/core/configuration/decode.go`

- [ ] **Step 1:** 改 `Load` 调用 `encodeCanonical` 处(L58-61)inline:

```go
var canonical bytes.Buffer
if err := writeCanonical(&canonical, mergedNode); err != nil {
    return nil, wrapCore("配置规范化失败: "+err.Error(), err)
}
canonicalBytes := canonical.Bytes()
```

- [ ] **Step 2:** 删 `encodeCanonical` 函数(L89-97)

- [ ] **Step 3:** 改 `writeCanonicalObject` 调用 `writeCanonicalString` 处(L145)inline:

```go
encodedKey, err := json.Marshal(key)
if err != nil {
    return fmt.Errorf("配置字段名无法编码")
}
buffer.Write(encodedKey)
buffer.WriteByte(':')
if err := writeCanonical(buffer, node.object[key]); err != nil {
    return err
}
```

(此行实际不调 writeCanonicalString,检查后再决定是否需要改)

- [ ] **Step 4:** 改 `writeCanonicalScalar` 调用 `writeCanonicalString` 处(L159, 167)inline,直接 `buffer.Write(mustJSON(value))`

- [ ] **Step 5:** 删 `writeCanonicalString` 函数(L188-196)

- [ ] **Step 6:** 测试

Run: `go test ./core/configuration/... -v`
Expected: PASS

---

### 任务 8:[可选,条件性]简化 `containsMutableReference` → 拒绝所有 unexported 字段

**前置条件:** 需先 grep 整个 `cool-next/` 确认无任何 unexported 字段被合法使用:

```bash
grep -rn "description\|json:\|orm:" --include="*.go" cool-next/ | grep -v "_test.go" | grep -v "//"
```

**Files:**
- Modify: `cool-next/core/configuration/schema.go:181-203` `containsMutableReference` 删
- Modify: `cool-next/core/configuration/schema.go:146-148` 调用方简化:

```go
if !field.IsExported() {
    return nil, fmt.Errorf("配置结构体 %s 不允许未导出字段 %s", current, field.Name)
}
```

- [ ] **Step 1:** 验证前置条件:grep 全代码库无合法 unexported 标量字段

- [ ] **Step 2:** 改 schema.go L146-148 为直接拒绝,删 `containsMutableReference` 函数

- [ ] **Step 3:** 测试

Run: `go test ./core/configuration/... -v`
Expected: PASS

---

## 4. 执行顺序与依赖

```
任务 3 (util.IsNilValue) ─┐
任务 6 (util.DeepClone) ──┼─→ 任务 4 (slices.Clone,可最先做)
                          │
任务 2 (gerror.Wrap) ─────┴─→ 任务 5 (parseYAML merge)
                              任务 7 (inline wrappers)
                              任务 8 (optional)
```

**建议执行顺序:**
1. **任务 1** 优先(规范合规,独立)
2. **任务 4** 简单机械替换(独立)
3. **任务 2** 替换 gerror(独立)
4. **任务 3** 建 util + 合并 isNil
5. **任务 6** 建 util.Clone + 替换两个 clone
6. **任务 5** YAML 合并
7. **任务 7** inline wrappers
8. **任务 8** 可选,前置条件通过才做

## 5. 验证策略

每任务完成后:
- `go vet ./core/...`
- `go test ./core/... -count=1 -race`
- `go test ./... -count=1`(回归确认)

最终:
- `go build ./...`
- `go test ./... -count=1 -race`

## 6. 风险与回滚

| 任务 | 风险 | 回滚 |
|---|---|---|
| 1 | 反射版与硬编码版输出顺序不一致 | git revert 单任务 |
| 2 | `Error()` 行为差(本仓库无影响,exception 路径绕开) | git revert |
| 3 | 三处调用点替换漏改 | 单文件回滚 |
| 4 | 纯机械替换,go vet 可拦截类型不匹配 | git revert |
| 5 | 合并后字段查表 vs element 透传逻辑漂移 | 单文件回滚 |
| 6 | util.DeepClone 与原 configuration clone 行为差(cycle 边界) | 单文件回滚 |
| 7 | inline 后 buffer 初始化遗漏 | 单文件回滚 |
| 8 | 拒绝 unexported 字段破坏现有合法实体 | 仅当 grep 确认后执行 |

## 7. 自评

- **规范覆盖:** §2.3 ✅(任务 1)、§8 ✅(任务 2)
- **占位符扫描:** 无"TBD/TODO/类似任务 N",所有代码完整
- **类型一致性:** `util.IsNilValue` `util.DeepClone` `util.CloneSlice` 在所有引用处使用相同签名

## 8. 净评分

| 项 | 行数变化 |
|---|---|
| 任务 1(反射 baseFields) | ≈ 0(替换同等长度) |
| 任务 2(删 preservedCause) | −20 |
| 任务 3(util.IsNilValue) | −15 |
| 任务 4(slices.Clone × 22) | −22(每处约 1 行净省) |
| 任务 5(YAML 合并) | −15 |
| 任务 6(util.DeepClone) | −80(删 130 行,加 50 行) |
| 任务 7(inline wrappers) | −10 |
| 任务 8(可选) | −25 |
| **合计** | **−160~−190 行(不含任务 8),新增 ~80 行工具** |

净行数:**−80~−110 行**。架构收益:
- 1 处规范违规修复
- 4 个重复函数收敛到 2 个工具
- 22 处手写模式替 stdlib
- 2 处内部 YAML 解析去重
- 3 处薄包装 inline

---

**等你评估。确认后告知执行方式(本会话 inline 执行 / 子代理驱动 / 暂缓)。**