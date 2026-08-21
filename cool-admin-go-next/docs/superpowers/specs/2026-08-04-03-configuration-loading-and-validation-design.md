# 模块 03：配置加载与基础校验设计

> 日期：2026-08-04  
> 状态：active  
> 模块：03 配置加载与基础校验  
> 对应拆分项：03.1-03.9  
> 前置模块：01 工程骨架与依赖边界、02 核心异常模型

## 1. 文档定位

本文冻结模块 03 的配置来源、合并规则、类型校验、环境变量替换、规范化结果和错误语义，供实施计划、编码和验收引用。

事实来源按以下顺序解释：

1. 本模块设计；
2. `2026-08-03-cool-admin-go-next-module-decomposition-design.md`；
3. `2026-07-31-cool-admin-go-next-architecture-design.md`；
4. 仓库 `README.md`。

发现冲突时停止相关实施并复核上位设计。本文件通过用户评审后改为 `reviewed`；实施开始后改为 `active`。

## 2. 目标

模块 03 提供一套与业务模块、数据库和 Application Host 解耦的强类型配置引擎：

1. 不常修改的默认值直接声明在 Go 配置结构体中；
2. `config.yaml` 只写部署时需要覆盖的字段；
3. 数据库等部署配置由后续模块定义类型后直接通过本引擎读取；
4. 敏感值可在配置叶子位置使用 `${ENV_NAME}` 从环境变量取得；
5. 合并全程保留字段是否出现以及是否显式为 `null`；
6. 未知字段、重复字段、类型错误、非法 `null` 和 `gvalid` 失败均阻止启动；
7. 相同输入产生字节一致、调用方不可修改内部状态的规范化结果；
8. 所有失败统一进入模块 02 的 Core 异常模型，同时保留内部 Cause。

## 3. 非目标

本模块不实现：

- 数据库配置结构、连接组、连接测试或密码脱敏日志；这些属于模块 09；
- `ModuleConfig()`、模块配置路径和模块声明；这些属于模块 10；
- 配置文件监听、运行时热更新、远程配置中心或命令行覆盖；
- Application Host 的启动顺序和默认配置文件定位；这些属于模块 46；
- YAML 锚点、别名、合并键、自定义 Tag 或模板表达式；
- 自动把 `COOL_DATABASE_DEFAULT_HOST` 推导为配置路径；
- 将错误配置静默修正、忽略或降级为 Go 零值。

## 4. 对外接口

代码归属为 `cool-next/core/configuration`。公开能力保持最小：

```go
type LookupEnv func(name string) (value string, found bool)

type Source struct {
   Main      []byte
   LookupEnv LookupEnv
}

type Result[T any] struct {
   // 私有规范化状态
}

func Load[T any](ctx context.Context, defaults T, source Source) (*Result[T], error)

func LoadFile[T any](
   ctx context.Context,
   defaults T,
   path string,
   lookupEnv LookupEnv,
) (*Result[T], error)

func (result *Result[T]) Value() T

func (result *Result[T]) CanonicalJSON() []byte
```

约束如下：

- `T` 必须是非指针结构体；配置对象的字段由 `json` Tag 定义，未写 Tag 时使用 Go 字段名；
- `json:"-"` 字段不属于配置 Schema，主配置不得提交该字段；
- `json:",omitempty"` 的 `omitempty` 在配置引擎中不生效，代码默认值即使是零值也必须保留；其他改变标量编码的 JSON Tag 选项不支持；
- 匿名嵌入字段和重复 JSON 字段名会产生有歧义的配置路径，Schema 建立阶段必须拒绝；
- 未导出的字符串、布尔、数字等不可变字段保留代码默认值；未导出的 map、slice、pointer 等可变引用无法安全防御性复制，Schema 建立阶段必须拒绝；
- map 的键必须是 string 或以 string 为底层类型；
- `Source.Main` 为空时表示没有主配置覆盖；
- `Load` 在解析前复制 `Source.Main`，调用方后续修改原始字节不影响加载过程和结果；
- `LookupEnv` 为 nil 时使用 `os.LookupEnv`；测试和生成期工具可以注入无副作用实现；
- `LoadFile` 只负责安全读取调用方明确传入的路径，然后委托 `Load`；不存在的文件是启动错误，不提供隐式路径回退；
- `Value` 每次返回从私有规范化数据重新构造的值，map、slice 和指针不与内部状态共享；
- `CanonicalJSON` 每次返回字节副本。

模块 46 后续决定生产入口传入 `manifest/config/config.yaml` 还是其他显式路径。本模块不绑定工作目录。

## 5. 配置树与字段 Presence

引擎内部使用私有配置树，每个节点明确记录以下状态之一：

| 状态 | 含义 |
|---|---|
| absent | 当前覆盖层没有提交该字段 |
| null | 当前覆盖层显式提交 `null` |
| scalar | 字符串、布尔值或数字 |
| object | 由字符串键索引的对象或 map |
| list | slice 或 array |

Go 默认值先依据目标类型转换为配置树，主配置再从 `yaml.Node` 转换为同一结构。合并完成前不得把任一层提前解码为目标结构体，也不得用 Go 零值推断字段是否出现。

GoFrame `gyaml.Decode` 会直接产生 `map[string]any`，无法完整承担 presence、重复键、严格数字类型和 YAML 节点位置校验，因此本模块使用 GoFrame 已依赖的 `gopkg.in/yaml.v3` 读取 `yaml.Node`。该依赖在 `go.mod` 中登记为直接依赖；配置校验仍使用 GoFrame `gvalid`。

YAML 输入必须满足：

- 根节点是对象；空文档等价于没有覆盖；
- 一个输入只允许一个 YAML 文档，额外的 `---` 文档必须拒绝；
- 对象键必须是普通字符串且同一对象中不可重复；
- 仅允许 YAML Core Schema 的标准 Tag，拒绝锚点、别名、`<<` 合并键和自定义 Tag；
- 标量按照目标字段类型严格解释，不进行字符串到任意类型的宽松转换；
- 错误路径使用 JSON 风格字段路径，例如 `database.default.port`，不得包含秘密值。

## 6. 合并规则

合并顺序固定为：

```text
Go 代码默认值 -> config.yaml -> 环境变量叶子值
```

规则如下：

| 目标类型 | 主配置行为 |
|---|---|
| struct/object | 只递归覆盖明确出现的字段 |
| map | 按 key 递归合并；新 key 加入结果 |
| scalar | 明确出现时整体替换；扩展文本标量按目标类型解析 |
| slice | 明确出现时整体替换，不追加 |
| array | 明确出现时整体替换，并要求长度准确 |
| pointer | 非 null 值按元素类型校验后替换 |

显式 `null` 只允许作用于 pointer、map 或 slice。对 struct、array、字符串、布尔和数字提交 `null` 必须返回错误。map 中显式为 `null` 的 value 遵循 map value 类型的 nullable 规则，不表示删除 key。

主配置省略字段时完整保留代码默认值，包括零值、`false`、空字符串、空集合和 nil。主配置明确提交零值或 `false` 时必须覆盖非零默认值。

## 7. 环境变量

环境变量只通过配置叶子位置的完整占位符启用：

```yaml
database:
  password: ${DATABASE_PASSWORD}
```

固定规则：

1. 占位符必须占据整个字符串，`prefix-${NAME}` 不展开；
2. 变量名必须匹配 `[A-Za-z_][A-Za-z0-9_]*`；
3. 占位符只能位于 scalar 或 pointer-to-scalar 目标字段；
4. 环境变量未定义时返回 Core 启动错误；已定义的空字符串仍视为存在，仅能用于 string 类型；
5. 环境值按照目标类型严格转换，例如 `5432` 可进入整数，`true`/`false` 可进入布尔值；
6. string 字段保留环境值原文，值为 `null` 时仍是字符串 `"null"`；
7. 不递归展开环境变量内容，环境值中的 `${OTHER}` 只是普通文本；
8. 错误可以包含环境变量名和字段路径，但不得包含环境变量值。

只有主配置中显式出现的占位符才会触发环境变量读取。Go 代码默认值即使恰好符合占位符格式也保持普通字符串；配置中的其他普通字符串不会被当成环境变量名，操作系统环境也不会自动覆盖未声明占位符的字段。

## 8. 类型解码与校验

合并后的配置树按照 `T` 的静态类型执行严格检查：

- struct 拒绝不存在、未导出或被 `json:"-"` 排除的字段；
- scalar 必须属于目标类型允许的 YAML 类型，并检查整数范围、无符号数负值和 array 长度；
- 普通 named scalar 按其底层类型校验后保留目标类型；
- `time.Duration` 接受 Go duration 字符串，例如 `2h`、`30s`，并通过 `time.ParseDuration` 严格解析；规范化时输出标准 duration 字符串；
- 同时实现 `encoding.TextUnmarshaler` 和 `encoding.TextMarshaler` 的 named scalar 接受 YAML string，并用新值调用 `UnmarshalText`；规范化使用 `MarshalText`，任一实现失败都必须保留 Cause；
- pointer、map、slice 和嵌套结构递归应用相同规则；
- interface、func、chan、complex 和 unsafe pointer 不允许出现在配置 Schema；
- float 必须是有限值，拒绝 NaN 和正负无穷；
- 不经过 `map[string]any` 或浮点中间值，确保 uint64 和整数精度不丢失。

严格类型检查通过后一次性构造 `T`，再调用 `gvalid.New().Data(value).Run(ctx)` 校验 `v` Tag。校验使用调用方传入的 Context；多个校验错误以 GoFrame 的结构化错误顺序输出，不自行改变 `v` 规则语义。

## 9. 规范化与不可变性

成功结果保存一份私有、确定性的规范化 JSON：

- object 和 map 的键按字节序稳定排序；
- 数组保持输入顺序；
- 数字按目标 Go 类型输出，不经过 float64；duration 和扩展文本标量输出其规范文本；
- 字符串使用标准 JSON 转义；
- nil pointer、map、slice 统一输出 `null`；非 nil 空 map/slice 分别输出 `{}` 和 `[]`；
- 相同默认值、主配置字节语义和环境变量值必须产生字节等价结果。

成功结果私有保存类型化配置树和规范化 JSON。`Value` 从类型化配置树构造深拷贝，不把规范化 JSON 当作通用反序列化协议；调用方只能取得深拷贝的 `T` 或规范化字节副本。修改任一返回值不得影响后续 `Value`、`CanonicalJSON` 或其他调用方。

## 10. 错误语义

以下错误均阻止加载：

- 文件读取失败或 YAML 语法错误；
- 重复键、未知字段、非法 YAML 能力或非法配置类型；
- 合并、nullability、范围或 array 长度错误；
- 环境变量占位符非法、变量缺失或转换失败；
- 强类型解码失败；
- `gvalid` 校验失败；
- 规范化失败。

公开返回值必须能通过 `errors.As` 识别为模块 02 的 `BaseException`，分类为 `CoreFail`。安全消息包含阶段和字段路径，例如 `配置 database.default.port 类型错误`；原始解析或校验错误作为 Cause 保留。错误消息不得包含配置原值、环境变量值、密码、Token 或完整配置内容。

## 11. 文件职责

| 文件 | 职责 |
|---|---|
| `configuration.go` | 公开类型、`Load`、`LoadFile` 和结果不可变访问 |
| `schema.go` | 从目标 Go 类型建立字段 Schema 并校验受支持类型 |
| `node.go` | 私有 presence-aware 配置树和 YAML 转换 |
| `merge.go` | 默认值、主配置和 null 的递归合并 |
| `environment.go` | 完整占位符识别及目标类型转换 |
| `decode.go` | 强类型构造、`gvalid` 调用和规范化 JSON |
| `configuration_test.go` | 公开流程、错误分类和文件读取测试 |
| `merge_test.go` | 合并矩阵、presence、null 和集合测试 |
| `environment_test.go` | 环境变量语法、缺失、空值和类型转换测试 |
| `fuzz_test.go` | YAML 与合并输入不 panic、不泄漏及确定性测试 |

实现时可在不改变职责边界的前提下合并过小的私有文件，但不得把配置逻辑移入数据库、模块或 Host 包。

## 12. 测试与验收

### 12.1 单元测试

至少覆盖：

- 无主配置时完整保留代码默认值；
- 嵌套 struct 与 map 递归覆盖；
- scalar、slice 和 array 整体替换；
- 零值、`false`、空字符串和显式 `null` 的不同语义；
- nil 与非 nil 空 map/slice 的规范化差异；
- 未知字段、重复键、非法 null、类型和数值范围错误；
- `${ENV_NAME}` 成功、缺失、空字符串、非法语法和非叶子使用；
- `v` Tag 成功与失败；
- uint64 最大值、`time.Duration` 和扩展文本标量不失真；
- `Value` 与 `CanonicalJSON` 的防御性复制；
- 相同语义输入重复加载至少 100 次，规范化字节完全一致；
- 所有错误可识别为 Core 异常且不泄漏秘密值。

### 12.2 模糊测试

Fuzz 输入覆盖 YAML 字节和环境变量值，保证：

- 任意输入不 panic；
- 成功结果可重复解码为目标类型；
- 同一输入和环境快照产生相同规范化字节；
- 错误文本不包含测试注入的秘密标记。

### 12.3 门禁命令

```bash
go test ./cool-next/core/configuration -count=1
go test ./cool-next/core/configuration -run=^$ -fuzz=Fuzz -fuzztime=10s
go test -race ./cool-next/core/configuration -count=1
go vet ./...
make check
```

模块 03 不需要数据库集成测试。

## 13. 完成标准

以下条件全部满足后模块 03 才算完成：

1. 03.1-03.9 均有实现和测试证据；
2. 代码默认值、文件覆盖和环境变量覆盖顺序固定；
3. 合并保留 absent、零值、`false` 和显式 `null`；
4. 未知字段和类型不匹配无法进入强类型配置；
5. `gvalid` 在最终值上执行；
6. 结果确定、可复现且不暴露内部可变状态；
7. 错误统一为 Core 异常并保留 Cause，不泄漏秘密；
8. 单元、Fuzz、Race、Vet 和仓库快速门禁通过；
9. `cool-next/**` 未反向依赖 `modules/**`；
10. 未提前实现模块 09、10 或 46 的能力。

## 14. 上位设计覆盖表

| 拆分项 | 本文位置 |
|---|---|
| 03.1 默认、主配置和环境变量 | 第 4、6、7 节 |
| 03.2 presence-aware 配置树 | 第 5 节 |
| 03.3 合并规则 | 第 6 节 |
| 03.4 显式 null | 第 5、6 节 |
| 03.5 环境变量叶子覆盖 | 第 7 节 |
| 03.6 未知字段和类型错误 | 第 5、8 节 |
| 03.7 强类型解码和 gvalid | 第 8 节 |
| 03.8 不可变、可复现结果 | 第 9 节 |
| 03.9 Core 启动错误 | 第 10 节 |
