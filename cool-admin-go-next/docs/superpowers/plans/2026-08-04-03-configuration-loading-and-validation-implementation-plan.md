# 模块 03：配置加载与基础校验实施计划

> 状态：completed

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `cool-next/core/configuration` 交付“代码默认值 -> YAML 覆盖 -> 环境变量叶子替换”的强类型、严格、确定且不可变的配置加载能力。

**Architecture:** 反射 Schema 冻结合法字段和类型，`yaml.Node` 保留 YAML 节点类型、重复键和位置，私有类型化配置树保留 null/presence 并执行递归合并。最终值使用 GoFrame `gvalid` 校验，错误包装为模块 02 Core 异常，Result 只暴露深拷贝和规范化 JSON 副本。

**Tech Stack:** Go 1.26.4、GoFrame v2.10.2 `gvalid`、`gopkg.in/yaml.v3`、标准库 reflection/json/time/testing/fuzz

---

## 1. 实施约束

- 事实来源：`docs/superpowers/specs/2026-08-04-03-configuration-loading-and-validation-design.md`；
- 严格遵循 TDD，每个行为先写测试并观察预期失败；
- 不修改模块 02 行为，不实现数据库配置、模块声明或 Application Host；
- `gopkg.in/yaml.v3` 从间接依赖提升为直接依赖，不引入新版本；
- 当前仓库无初始提交且全部基线文件未跟踪，无法建立 worktree 或有意义的局部提交；本计划保留用户现有状态，不创建只含模块 03 的异常 root commit。

## 2. 文件结构

| 文件 | 职责 |
|---|---|
| `cool-next/core/configuration/configuration.go` | 公开 API、加载编排、文件读取、Result |
| `cool-next/core/configuration/schema.go` | 反射 Schema、JSON Tag 和支持类型检查 |
| `cool-next/core/configuration/node.go` | 私有配置树、默认值转换、YAML 严格解析 |
| `cool-next/core/configuration/merge.go` | object/map/pointer 递归合并和集合替换 |
| `cool-next/core/configuration/environment.go` | 占位符与目标类型转换 |
| `cool-next/core/configuration/decode.go` | 目标值构造、规范化 JSON、gvalid |
| `cool-next/core/configuration/configuration_test.go` | 公开流程、校验、错误和不可变性 |
| `cool-next/core/configuration/merge_test.go` | presence、null 和合并矩阵 |
| `cool-next/core/configuration/environment_test.go` | 环境变量、duration 和严格标量 |
| `cool-next/core/configuration/fuzz_test.go` | 不 panic、确定性和秘密不泄漏 |
| `go.mod`、`go.sum` | 将 yaml.v3 标记为直接依赖并保持依赖一致 |

## 3. 任务清单

### 任务 1：公开加载入口与代码默认值

**Files:** `configuration_test.go`、`configuration.go`、`schema.go`、`node.go`

- [x] 写 `TestLoadUsesCodeDefaults`，以包含 bool、int、string、nested struct、map、slice、pointer 的配置调用：

```go
result, err := Load(context.Background(), defaults, Source{})
if err != nil {
   t.Fatal(err)
}
if got := result.Value(); !reflect.DeepEqual(got, defaults) {
   t.Fatalf("Value() = %#v, want %#v", got, defaults)
}
```

- [x] 运行 `go test ./cool-next/core/configuration -run TestLoadUsesCodeDefaults -count=1`，确认因 API 不存在失败。
- [x] 实现 `LookupEnv`、`Source`、`Result[T]`、`Load[T]` 的最小骨架；建立仅支持设计允许类型的反射 Schema，把全部代码默认字段转换为类型化节点。
- [x] 增加表驱动失败测试，拒绝非 struct 根、匿名字段、重复 JSON 名、interface/func/chan/complex、非 string map key 和递归类型。
- [x] 实现 JSON Tag 规则：`-` 排除、空名称回退 Go 名、`omitempty` 忽略、其他选项拒绝。
- [x] 运行本包测试，确认任务 1 通过。

### 任务 2：严格 YAML、Presence 与合并

**Files:** `merge_test.go`、`node.go`、`merge.go`

- [x] 写 `TestLoadMergesMainConfiguration`：nested struct/map 递归覆盖，scalar/slice/array 整体替换，省略字段保留默认值，显式 zero/false 覆盖默认值。
- [x] 运行定向测试，确认主配置尚未生效而失败。
- [x] 用 `yaml.Decoder` 解析唯一文档，检查 mapping/sequence/scalar、标准 Tag、重复键、未知字段、锚点/别名/合并键和额外文档。
- [x] 根据 Schema 把 YAML 转为类型化覆盖树；主配置根必须为 object，空文档等价于空覆盖。
- [x] 实现合并：struct/map/pointer-child 递归合并，scalar/slice/array 整体替换。
- [x] 写 `TestLoadHandlesExplicitNull`，验证 pointer/map/slice 可清空，struct/array/scalar 拒绝 null，map value 的 null 不删除 key。
- [x] 写严格错误表，覆盖未知字段、重复键、类型不匹配、整数溢出、array 长度、自定义 Tag、多文档和非法 YAML。
- [x] 运行 `go test ./cool-next/core/configuration -run 'TestLoad(Merges|Handles|Rejects)' -count=1`，确认任务 2 通过。

### 任务 3：环境变量与扩展标量

**Files:** `environment_test.go`、`environment.go`、`node.go`、`schema.go`

- [x] 写 `TestLoadResolvesEnvironmentLeaves`：string、bool、int、uint、float、`time.Duration` 和 pointer scalar 使用 `${NAME}` 覆盖。
- [x] 运行定向测试，确认占位符仍是普通字符串或类型错误。
- [x] 实现完整占位符识别；nil LookupEnv 使用 `os.LookupEnv`，自定义 LookupEnv 用于确定性测试。
- [x] 按目标类型严格解析环境值；string 保留原文，非 string 空值失败，duration 使用 `time.ParseDuration`。
- [x] 写错误表，覆盖缺失变量、非法变量名、非叶子位置、数值范围、非法 bool/duration，以及错误文本不包含秘密值。
- [x] 验证代码默认字符串 `${NAME}` 不展开，`prefix-${NAME}` 和环境值中的 `${OTHER}` 不递归展开。
- [x] 实现同时满足 TextMarshaler/TextUnmarshaler 且底层为安全值类型的扩展标量；添加 round-trip 测试。
- [x] 运行环境变量定向测试和本包全测。

### 任务 4：gvalid、规范化和不可变 Result

**Files:** `configuration_test.go`、`decode.go`、`configuration.go`

- [x] 写 `TestLoadValidatesFinalValue`，使用 `v:"min:1"` 验证默认值和覆盖后的最终值；确认失败可 `errors.As` 为 `BaseException` 且 `Code == CoreFail`。
- [x] 运行定向测试，确认尚未执行 gvalid。
- [x] 用 `gvalid.New().Data(value).Run(ctx)` 校验最终强类型值并通过 `exception.WrapCore` 保留 Cause。
- [x] 写 `TestResultIsImmutable`：修改两次 `Value()` 返回的 map/slice/pointer 和 `CanonicalJSON()` 字节，后续读取保持不变。
- [x] 实现从私有节点构造新 `T`，并用稳定排序 writer 输出规范化 JSON；区分 nil 与非 nil 空集合，duration 输出规范字符串。
- [x] 写 `TestCanonicalJSONIsDeterministic`，随机化 map 插入顺序并重复加载 100 次，断言字节一致且 uint64 最大值不失真。
- [x] 写错误安全测试，确保文件值、环境值和秘密标记不出现在公开 `err.Error()`。
- [x] 运行本包全测和 Race Test。

### 任务 5：文件读取、Fuzz 与仓库门禁

**Files:** `configuration_test.go`、`fuzz_test.go`、`configuration.go`、`go.mod`、`go.sum`

- [x] 写 `TestLoadFile`，验证显式路径成功、文件不存在失败、文件错误统一为 Core 异常。
- [x] 运行定向测试，确认 `LoadFile` 尚不存在或未工作。
- [x] 实现 `LoadFile`：使用 `os.ReadFile` 读取明确路径并委托 `Load`，不绑定工作目录、不静默忽略缺失文件。
- [x] 添加 `FuzzLoad`，种子覆盖合法配置、null、重复键、占位符和秘密标记；断言不 panic、成功结果可重复、错误不泄漏。
- [x] 将 `gopkg.in/yaml.v3 v3.0.1` 调整为直接依赖，执行 `go mod tidy`。
- [x] 运行格式化和定向验证：

```bash
gofmt -w cool-next/core/configuration/*.go
go test ./cool-next/core/configuration -count=1
go test ./cool-next/core/configuration -run=^$ -fuzz=FuzzLoad -fuzztime=10s
go test -race ./cool-next/core/configuration -count=1
```

- [x] 运行仓库验证：

```bash
go vet ./...
make check
```

- [x] 对照 03.1-03.9 和本设计完成标准逐项复核，更新本计划状态为 `completed`。

## 4. 计划自检

- 03.1：任务 1、2、3、5；
- 03.2-03.4：任务 1、2；
- 03.5：任务 3；
- 03.6：任务 1、2、3；
- 03.7：任务 4；
- 03.8：任务 4；
- 03.9：任务 2-5；
- 无数据库连接、模块声明、Host、热更新或远程配置越界；
- 无 TBD、TODO、占位实现或依赖后续补全的步骤。
