# 种子数据 Provider：`cool generate` 自动嵌入并注入模块根 JSON

> 状态：待实施
> 前置：`2026-08-20-base-module-design.md` §7 第 5 项（本文档是该待办的完整方案）
> 实施者：另一 Agent；审核者：本文档作者

---

## 1. 目标

让业务模块**不再编写任何 `go:embed` 代码**即可使用模块根的 `db.json` / `menu.json`。

改造后 `modules/base/config.go` 只剩 `Config` 类型树和 `ModuleConfig()`，与 Node 的 `config.ts`
（纯配置工厂，无辅助函数）对齐；种子数据由 `cool generate` 嵌入 `modules/modules_gen.go`，
按模块作用域注入给该模块的构造器。

### 为什么必须由框架做

`go:embed` 不能引用 `../`，所以嵌入模块根 JSON 的 `.go` 文件**必须**位于模块根。而
`analyze.go` 的 `validateModuleDirectories`（CG111）规定模块根只允许 `config.go`。两条约束
夹击的结果就是当前 `config.go` 里混进了 `//go:embed` 和 `DBSeed()`/`MenuSeed()`。

本方案让**生成文件**承担嵌入职责，从而：

- `config.go` 恢复为纯配置
- **CG111 不需要放宽**，模块根仍然只有 `config.go` + 两个 JSON
- 业务模块之间零重复（每加一个模块就手写一遍 embed 的问题消失）

---

## 2. 已验证的技术前提

**`go:embed` 可以从生成文件引用子目录。** 已用最小工程实测通过：

```
embedtest/
  go.mod
  main.go
  modules/
    gen.go          //go:embed base/db.json  →  编译并运行成功
    base/db.json
```

`modules/modules_gen.go`（package `modules`）嵌入 `base/db.json` 合法，因为目标是**子目录**
而非 `..`。这是整个方案的支点，实施者无需再验证。

**注意反面情况**：`//go:embed` 指向不存在的文件是**编译错误**，不是空值。所以生成器必须
先探测文件是否存在，只为存在的文件产出指令（见 §5.4）。

---

## 3. 设计

### 3.1 注入类型：`cool-next/seed.Data`

新增到现有的 `cool-next/seed` 包（该包已经承载种子导入的通用原语，语义吻合）：

```go
// Data 是 cool generate 从模块根嵌入的种子数据。字段可能为空，
// 表示该模块没有提供对应的 JSON 文件。
type Data struct {
	db   []byte
	menu []byte
}

// NewData 由生成代码调用，构造模块种子数据。
func NewData(db, menu []byte) Data {
	return Data{db: db, menu: menu}
}

// DB 返回 db.json 内容副本，无该文件时返回 nil。
func (d Data) DB() []byte { return cloneSeedBytes(d.db) }

// Menu 返回 menu.json 内容副本，无该文件时返回 nil。
func (d Data) Menu() []byte { return cloneSeedBytes(d.menu) }

func cloneSeedBytes(value []byte) []byte {
	if value == nil {
		return nil
	}
	return append([]byte(nil), value...)
}
```

**必须返回副本**——保持与现状 `base.DBSeed()` 相同的防御性拷贝语义。嵌入变量是包级
`[]byte`，调用方若能拿到底层数组并修改，会污染整个进程生命周期内的种子数据。

字段设为非导出 + 构造函数，避免业务代码绕过拷贝直接读字段。

### 3.2 Provider 种类：`ProviderKindSeed`

完全照抄 `ProviderKindConfig` 的模式——**每个模块注册一个属于自己的 Provider**，靠
`matchingProviders` 里既有的模块作用域逻辑保证 A 模块的构造器只能拿到 A 模块的种子数据。

与 Config 的**唯一差异**：Config 每个模块是不同类型（`base.Config` vs `demo.Config`），
而 Seed 所有模块共享 `seed.Data` 一个类型。这不构成问题，因为
`provider.go: matchingProviders` 的匹配条件是

```go
isLocalDependency && provider.provider.module == consumer.component.module
```

模块键相等是硬条件；跨模块引用会落进 `illegal` 分支并产出诊断，正是期望行为。

### 3.3 注册策略：所有模块一律注册

即使模块没有任何 JSON 文件，也注册 Provider，此时 `seed.Data` 两个字段均为 nil。

理由：生成逻辑更简单；"没有种子数据"是合法状态，业务代码用 `len(data.DB()) == 0` 判断即可；
避免"先加了构造器参数、还没建 JSON 文件"时冒出难以理解的依赖不满足错误。

### 3.4 生成产物长什么样

`modules/modules_gen.go` 中新增（示例为 `base` 模块同时具备两个 JSON 的情况）：

```go
import (
	_ "embed"
	// ...其余 import
	seed "github.com/toothdy/cool-admin-go-next/cool-next/seed"
)

//go:embed base/db.json
var seedDB_base []byte

//go:embed base/menu.json
var seedMenu_base []byte
```

构造器实参处直接使用表达式 `seed.NewData(seedDB_base, seedMenu_base)`。

模块缺少某个 JSON 时，对应变量**不生成**，表达式该位置填字面量 `nil`。两个都缺则表达式为
`seed.NewData(nil, nil)`。

---

## 4. 影响面清单

| 文件 | 改动 |
|---|---|
| `cool-next/seed/data.go` | **新建**，§3.1 的 `Data` 类型 |
| `cool-next/codegen/graph.go` | 新增 `ProviderKindSeed` 常量；`buildGraph` 注册 Provider |
| `cool-next/codegen/model.go` | `Module` 增加种子文件存在性字段 |
| `cool-next/codegen/analyze.go` | `analyzeModule` 探测模块根 JSON |
| `cool-next/codegen/provider.go` | `isCrossModuleContract` 对新 Kind 返回 false |
| `cool-next/codegen/render.go` | 嵌入变量声明、`providerExpression`、`moduleProviderKind`、`embed` 空白导入 |
| `cool-next/core/module/graph.go` | 运行时 `ProviderKindSeed` 常量 + 校验白名单 |
| `modules/base/config.go` | **删除** embed 与 `DBSeed`/`MenuSeed` |
| `modules/base/service/initializer.go` | 构造器接收 `seed.Data` |
| `README.md` | 模块目录协议表更新 |
| `docs/.../2026-08-20-base-module-design.md` | §7 第 5 项标记完成 |

---

## 5. 实现细节

### 5.1 `cool-next/core/module/graph.go`（运行时）

约 L59 常量组增加：

```go
ProviderKindSeed ProviderKind = "seed" // 模块种子数据
```

`validateProviderDefinition`（约 L452）的合法 Kind switch 增加 `ProviderKindSeed`。
**漏掉这一处会导致生成的注册表在运行时被拒绝**，且错误信息（"Provider 类别无效"）
不会指向 codegen，排查成本高。

### 5.2 `cool-next/codegen/graph.go`

约 L23 常量组增加同名 `ProviderKindSeed ProviderKind = "seed"`（codegen 侧与 runtime 侧
是两套独立常量，字符串值必须一致）。

`buildGraph` 中现有的模块遍历循环（约 L184-192）里，紧随 Config Provider 之后追加：

```go
providers = append(providers, graphProvider{
	componentIndex: -1,
	provider: Provider{
		kind:        ProviderKindSeed,
		name:        "Data",
		module:      key,
		packagePath: seedPackagePath,
		typ:         seedDataType,
	},
	typ: seedDataType,
})
```

`seedPackagePath` 常量加到 `entity.go` 的常量组（那里已有
`databasePackagePath`/`entityPackagePath` 等同类常量）：

```go
seedPackagePath = "github.com/toothdy/cool-admin-go-next/cool-next/seed"
```

**`seedDataType` 怎么拿**：需要一个 `types.Type` 对象参与
`types.AssignableTo` 匹配。参照 `findRuntimeProviderType`（graph.go 约 L391）的既有做法
——它从任意构造器参数里搜出 `*coredb.Runtime` 的 `types.Type`。同理实现
`findSeedDataType(model)`：遍历全部构造器参数，找到类型全名为
`github.com/toothdy/cool-admin-go-next/cool-next/seed.Data` 的那个并返回。

若全工程没有任何构造器请求 `seed.Data`（`findSeedDataType` 返回 nil），则**跳过全部
Seed Provider 注册**，也不生成任何 embed 代码。这保证了"没人用就零产出"，`modules_gen.go`
不会平白多出无用的嵌入数据。

### 5.3 `model.go` + `analyze.go`：探测 JSON 是否存在

`Module` 结构（model.go 约 L352）增加两个字段：

```go
seedDB   bool // 模块根存在 db.json
seedMenu bool // 模块根存在 menu.json
```

配套导出方法（供 render 使用，与既有 `Config()`/`Entities()` 风格一致）：

```go
func (m Module) HasSeedDB() bool   { return m.seedDB }
func (m Module) HasSeedMenu() bool { return m.seedMenu }
```

`analyzeModule`（analyze.go 约 L278）在构造 `result` 时填充：

```go
result.seedDB = regularFileExists(filepath.Join(root, "db.json"))
result.seedMenu = regularFileExists(filepath.Join(root, "menu.json"))
```

`regularFileExists` 用 `os.Lstat` 并要求 `Mode().IsRegular()`——**不要用 `os.Stat`**，
符号链接必须排除，理由与 `codegen/scaffold_write.go` 里 `validateCodeTarget` 拒绝符号链接
一致：嵌入内容会进二进制，不能让链接指向工作区外的文件。

### 5.4 `render.go`

**(a) 嵌入变量声明。** 新增 `writeSeedDeclarations(source, modules, imports)`，在
`writeDescriptorDeclarations` 之前调用（约 L164 那一串 `write*Declarations` 里）。
只为 `HasSeedDB()`/`HasSeedMenu()` 为真的模块产出：

```go
//go:embed base/db.json
var seedDB_base []byte
```

变量名用 `"seedDB_" + generatedIdentifier(moduleKey)`。`generatedIdentifier`
（`do_emit.go:106`）把非字母数字字符替换为 `_`，所以嵌套模块键 `system/user` 会变成
`system_user`，不会产出非法标识符。

embed 路径用**模块相对 `modules/` 根的路径 + `/db.json`**，正斜杠，不能用
`filepath.Join`（Windows 下会产出反斜杠，`go:embed` 只接受正斜杠）。`Module.root` 字段
存的是工作区相对路径（如 `modules/base`），需要剥掉 `modules/` 前缀。建议直接用
`module.Identity().Key()`——它就是 `base` / `system/user` 这种形式，与目录结构一一对应。

**(b) 空白导入 `embed`。** `importManager.add(path, preferred)` 的 `preferred` 会成为别名，
`writeImports` 输出 `%s %q`。因此调用 `imports.add("embed", "_")` 即可产出 `_ "embed"`，
无需改 `writeImports`。**仅在实际产出了至少一条 embed 指令时才 add**，否则会留下未使用的
导入导致编译失败。

同时需要 `imports.add(seedPackagePath, "seed")`，供 `seed.NewData(...)` 表达式使用。

**(c) `providerExpression`（约 L1352）** 增加分支：

```go
case ProviderKindSeed:
	for _, current := range modules {
		if current.key == provider.module {
			return fmt.Sprintf("seed.NewData(%s, %s)", seedVarOrNil(current, "DB"), seedVarOrNil(current, "Menu")), true
		}
	}
```

`seedVarOrNil` 在模块有对应文件时返回变量名，否则返回字面量 `"nil"`。

注意 `renderModule` 结构（render.go 约 L179）当前是否携带 `HasSeedDB` 信息——若没有，
需要在 `renderModel` 里一并带上，不要在 `providerExpression` 里回查 `*Model`。

**(d) `moduleProviderKind`（约 L2052）** 增加：

```go
case ProviderKindSeed:
	return "module.ProviderKindSeed"
```

**(e) 不需要 `_ = xxx` 兜底。** Config 因为是函数内局部变量，未使用会编译失败，所以
render.go L1000 有 `configUsed` 判断。Seed 是**包级变量**，Go 允许包级变量未被使用，
无需类似处理。

### 5.5 `provider.go`

`isCrossModuleContract`（约 L95）开头的 Config 短路增加 Seed：

```go
if provider.provider.kind == ProviderKindConfig || provider.provider.kind == ProviderKindSeed {
	return false
}
```

语义：种子数据是模块私有资源，不构成跨模块契约，不允许被别的模块注入。

---

## 6. `base` 模块迁移

### 6.1 `modules/base/config.go`

删除以下内容，文件恢复为纯配置：

```go
import _ "embed"           // 删

var (                       // 整块删
	//go:embed db.json
	dbSeed []byte
	//go:embed menu.json
	menuSeed []byte
)

func DBSeed() []byte { ... }   // 删
func MenuSeed() []byte { ... } // 删
```

### 6.2 `modules/base/service/initializer.go`

`NewInitializer` 增加参数（放在 `runtime` 之后、Descriptor 之前）：

```go
func NewInitializer(
	runtime *coredb.Runtime,
	data seed.Data,
	conf coreentity.Descriptor[entity.Conf, uint64],
	// ...其余不变
) (*Initializer, error)
```

`Initializer` 结构增加 `data seed.Data` 字段。`OnInit` 里：

```go
seeds, err := parseInitialSeeds(initializer.data.DB(), initializer.data.Menu())
```

替换现有的 `parseInitialSeeds(base.DBSeed(), base.MenuSeed())`。

**随之删除 `initializer.go` 对 `modules/base` 的 import**——检查该文件是否还有其他地方用
到 `base.` 前缀（当前只有这两处），若无则整条 import 删掉。

### 6.3 空数据的行为

`base` 两个 JSON 都存在，不触发空路径。但实现者需确认 `parseInitialSeeds` 在收到
`nil` 时的行为是安全的（当前实现走 `json.Unmarshal(nil, ...)` 会报错）。若要让"无种子文件"
成为合法状态，`OnInit` 应在两者皆空时直接 return nil。**建议加这个短路**，否则
§3.3 的"一律注册"策略对无 JSON 的模块会变成运行期报错。

---

## 7. 文档更新

**README.md** 模块目录协议表中 `db.json` / `menu.json` 一行的说明改为：

> 可选初始化数据，由 `cool generate` 自动嵌入生成文件并作为 `seed.Data` 注入本模块构造器，
> 业务模块无需编写 `go:embed`

并在「字段命名约定」之后、「删除归档与 gdb 时序」之前补一小节说明用法：构造器声明
`data seed.Data` 参数即可拿到本模块的种子数据。

**`2026-08-20-base-module-design.md`**：§7 第 5 项标记完成并引用本文档；§5 增加一节记录
实际交付范围与偏差。

---

## 8. 验收标准

仓库 `.gitignore` 排除了 `*_test.go` 与 `/test/`，**没有单元测试可依赖**，因此以下静态门禁
是唯一保障，一项都不能跳：

1. `go build ./...` 通过
2. `go vet ./...` 无输出
3. `gofmt -l .` 无输出
4. `go run ./cmd/cool generate` 成功
5. `go run ./cmd/cool check` 输出「检查通过」
6. **幂等性**：连续两次 `cool generate`，第二次后 `git status` 中 `modules_gen.go` 无新增改动
7. **嵌入内容正确性**（必须实测，不能靠推理）：写一个临时 `main` 或临时导出函数，打印
   注入给 `base` 的 `seed.Data.DB()` 与 `Menu()`，与 `modules/base/db.json`、
   `modules/base/menu.json` 做 `bytes.Equal` 比对，确认逐字节一致。验证完删除临时代码。
8. **拷贝语义**：确认修改 `DB()` 返回值不影响后续调用的返回值（防止漏掉
   `cloneSeedBytes`）。同样用临时代码验证后删除。
9. `modules_gen.go` 中确实出现了 `//go:embed base/db.json` 与 `_ "embed"`，且
   `modules/base/config.go` 中不再有任何 `embed` 字样。

---

## 9. 明确不做

- **不放宽 CG111**。模块根仍然只允许 `config.go`（加两个 JSON 数据文件）。本方案的价值之一
  就是在不削弱该约束的前提下解决问题。
- **不引入配置开关**。Node 有 `cool.initDB` / `cool.initMenu` 开关，v2 当前没有对应配置，
  本轮不新增——嵌入是编译期行为，是否执行导入由业务模块的 `OnInit` 决定。
- **不改动 `cool-next/seed` 已有的 `Store`/`Guard`/`SyncTree`/`InsertMissing`**。本方案只
  新增 `Data` 类型，不碰既有的幂等守卫与导入原语。
- **不做 `db.json` 内容格式的校验**（如 `@childDatas`/`@id` 语法检查）。嵌入阶段只管字节，
  解析仍由业务模块的 `parseInitialSeeds` 负责。

---

## 10. 风险与陷阱

| 风险 | 说明 | 对策 |
|---|---|---|
| 漏改 runtime 侧 Kind 白名单 | `core/module/graph.go` 的 `validateProviderDefinition` 不认新 Kind，`cool check` 可能通过但运行时装配失败 | §5.1 必做；验收第 5 项覆盖不到运行期，靠代码审查 |
| `embed` 导入未使用 | 无任何模块有 JSON 时仍 add 了 `_ "embed"`，编译失败 | §5.4(b) 条件化 add |
| 路径分隔符 | 用 `filepath.Join` 拼 embed 路径在 Windows 产出反斜杠 | 用 `Identity().Key()` + 正斜杠字面量 |
| 符号链接 | `os.Stat` 会跟随链接，可能把工作区外文件嵌进二进制 | 用 `os.Lstat` + `Mode().IsRegular()` |
| 丢失拷贝语义 | 直接返回嵌入变量，调用方修改会污染全局 | §3.1 强制 `cloneSeedBytes`，验收第 8 项实测 |
| `renderModule` 缺字段 | 在 `providerExpression` 里回查 `*Model` 会破坏现有渲染层的数据流 | §5.4(c) 在 `renderModel` 阶段带上 |
| 空种子数据运行期报错 | 无 JSON 模块注入空 `Data`，`json.Unmarshal(nil)` 报错 | §6.3 在 `OnInit` 加空值短路 |

---

## 11. 审核关注点（供审核者）

提交后我会重点看这几处，实施者可自查：

1. `seed.Data` 是否真的返回副本，字段是否非导出
2. `findSeedDataType` 在无人使用时是否真的做到零产出（`modules_gen.go` 不含任何 seed 痕迹）
3. embed 路径是否用了 `Identity().Key()` 而非 `filepath.Join(Module.root)`
4. `os.Lstat` 而非 `os.Stat`
5. runtime 侧 `validateProviderDefinition` 是否同步
6. `modules/base/config.go` 是否彻底干净（无 `embed`、无 `DBSeed`/`MenuSeed`、无多余 import）
7. `cool generate` 幂等性与嵌入内容的逐字节实测证据
