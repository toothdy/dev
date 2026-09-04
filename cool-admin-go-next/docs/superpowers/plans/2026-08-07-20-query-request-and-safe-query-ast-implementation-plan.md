# 20 QueryRequest And Safe Query AST Implementation Plan

**Goal:** 在 `cool-next/crud` 提供 presence-aware QueryRequest 和封闭、安全、有序的查询 AST，并让 `cool check` 静态拒绝动态字段名、别名、请求参数名和 RawWhere 表达式。

**Architecture:** `crud` 使用私有节点实现 `ColumnRef`、Condition、Select、Join、Order、QueryOp 与只追加的 QueryBuilder；外部请求错误返回 Validate 异常，开发者 DSL 错误 panic Core 异常。`codegen` 复用现有 `go/ast`、`go/types` 和源码位置模型检查查询构造器的常量参数，不增加生成模型、数据库执行或规则插件。

**Tech Stack:** Go 1.26、标准库 `context` / `reflect` / `regexp` / `go/ast` / `go/constant` / `go/types`、现有 `core/exception`、Go `testing`

## Preconditions

- 模块 04 的 Descriptor 和实体类型契约已完成，模块 20 不修改 `core/entity`。
- 模块 15 的 `Analyze` / `cool check` 共享静态分析流水线已完成，查询规则接入同一分析过程。
- 开发前阅读本地官方文档：`go doc reflect.TypeFor`、`go doc go/types.Info`、`go doc go/constant.Value`、`go doc go/ast.Inspect`。
- 开发前阅读现有 `core/exception` 与 `gerror` 源码，复用 Validate/Core 包装方式，不新增错误框架。
- 运行 `go test ./cool-next/core/entity ./cool-next/codegen -count=1` 和 `go run ./cmd/cool check`，确认前置基线通过。

## File Structure

- Create: `cool-next/crud/errors.go` - Validate/Core 错误包装和 DSL panic 边界
- Create: `cool-next/crud/request.go` - RequestValue、QueryRequest、presence 和强类型读取
- Create: `cool-next/crud/request_test.go` - presence、类型、重复字段、null 和副本测试
- Create: `cool-next/crud/ast.go` - ColumnRef、Condition、Where、Select、Join、Order 私有节点
- Create: `cool-next/crud/ast_test.go` - 名称、实体、条件、关联、排序和非法 DSL 测试
- Create: `cool-next/crud/query.go` - QueryOp、FieldMatch、QueryExtender 和 QueryBuilder
- Create: `cool-next/crud/query_test.go` - 字段匹配、追加顺序和封闭 Builder 测试
- Create: `cool-next/crud/fuzz_test.go` - 请求名、字段名和集合构造的最小模糊测试
- Create: `cool-next/codegen/query_validate.go` - 查询构造器常量参数与 RawWhere 静态校验
- Create: `cool-next/codegen/query_validate_test.go` - 常量、动态表达式、符号身份和稳定诊断测试
- Modify: `cool-next/codegen/analyze.go` - 在已加载模块源码上调用查询静态校验

本模块不修改 `core/entity`、`core/service`、`core/controller`、`db`、`modules/modules_gen.go`、README、Makefile 或 go.mod。

### Task 1: QueryRequest And Error Boundaries

**Tracks:** 20.1

**Write set:** `cool-next/crud/errors.go`、`cool-next/crud/request.go`、`cool-next/crud/request_test.go`

- [ ] 先写表驱动失败测试，覆盖空名、非 lowerCamelCase、重复名、`RequestField(name, nil)` 和 nil RequestValue 输入。
- [ ] 写 presence 测试，分别固定缺失、零值、`false`、空字符串、显式 null、类型匹配和类型不匹配的 `Has/Value/String/Bool/Strings` 结果。
- [ ] 写副本测试，确认构造后修改原 `[]string`、修改 `Strings` 返回值或修改构造参数切片都不影响 QueryRequest。
- [ ] 在 `errors.go` 复用 `gerror.Newf` 与 `exception.WrapValidate/WrapCore`；只提供包内最小 helper，不定义公开错误类型。
- [ ] 实现私有 RequestValue 状态和 `NewQueryRequest`，用 map 按名读取，不保留调用方可修改的内部切片。
- [ ] typed getter 的 bool 只表示存在、非 null 且类型匹配；不做字符串转换、逗号切分或弱类型解析。
- [ ] 所有新增导出符号使用 `COMMENT_STYLE.md` 规定的短中文意图注释，不写实现边界复述。

Run:

```bash
go test ./cool-next/crud -run 'Test(QueryRequest|NewQueryRequest|RequestValue)' -count=1
```

### Task 2: ColumnRef And Structural AST

**Tracks:** 20.2、20.3、20.4、20.7、20.8

**Write set:** `cool-next/crud/ast.go`、`cool-next/crud/ast_test.go`

- [ ] 先写字段测试，覆盖合法/非法逻辑名、`NewColumnRefOf[E]` 的实体类型、`Of` 返回新值、零值引用和非法别名。
- [ ] 写实体边界测试，拒绝指针、匿名 struct、interface、slice、map 和 nil Join 实体，只接受非指针具名 struct。
- [ ] 写条件测试，覆盖 Eq、Ne、In、Like、On、Where 的私有节点类型、输入顺序和封闭接口。
- [ ] 写 In 测试，覆盖非空 slice/array、输入复制、nil、标量和空集合；本任务不校验元素与 Descriptor 字段类型。
- [ ] 写 Select/Join/Order 测试，覆盖 All、As、Left/Inner、Asc/Desc、非法方向、非法 Join 条件和顺序。
- [ ] 实现共用 lowerCamelCase 名称校验；不要复制 `core/entity` 私有正则，也不要为一个正则建立新公共包。
- [ ] 无 error 返回的 DSL 构造器遇到动态非法输入时 panic Core 异常；静态可见错误由 Task 4 提前诊断。
- [ ] `Where` 固定 AND；不增加 Or、Not、子查询、visitor 或公开节点访问器。
- [ ] `LikeValue` 原样保存调用方模式；不在 AST 层自动添加 `%`。
- [ ] 不 import gdb、ghttp、grpc、codegen 或业务模块。

Run:

```bash
go test ./cool-next/crud -run 'Test(ColumnRef|Condition|Where|In|Select|Join|Order)' -count=1
```

### Task 3: QueryOp And Append-Only QueryBuilder

**Tracks:** 20.4、20.5、20.8

**Write set:** `cool-next/crud/query.go`、`cool-next/crud/query_test.go`

- [ ] 先写 FieldMatch 测试，固定 Eq/Like 默认使用字段逻辑名，EqFrom/LikeFrom 使用显式 lowerCamelCase 请求参数名。
- [ ] 写 QueryOp 顺序测试，确认关键词、Select、FieldEq、FieldLike、Order 和 Join 均使用有序 slice；切片冻结和复制明确留给模块 21 的 QueryPlan 编译边界。
- [ ] 写 Builder 测试，按调用顺序覆盖 Where、GT/GTE/LT/LTE、Select、Join、Group、Having、Order 的连续追加。
- [ ] 写 nil Extender、nil Builder、nil Condition/Select 和非法 Order/Join 的 Core panic 测试。
- [ ] 实现上位设计冻结的 QueryOp、FieldMatch、QueryExtender 和 QueryBuilder API，不增加 QueryPlan、Compile 或 ApplyQuery。
- [ ] Builder 内部使用少量有序私有 slice；每个方法返回同一 Builder，不提供清空、替换、读取节点或数据库访问方法。
- [ ] Extend 只接受非 nil 函数并原样返回；不在模块 20 执行函数或定义执行时序。
- [ ] Group 和 Having 仅通过 Builder 追加，不给 QueryOp 增加上位设计没有的字段。

Run:

```bash
go test ./cool-next/crud -run 'Test(FieldMatch|QueryOp|QueryBuilder|Extend)' -count=1
```

### Task 4: Static Query DSL Validation

**Tracks:** 20.2、20.6、20.7

**Write set:** `cool-next/codegen/query_validate.go`、`cool-next/codegen/query_validate_test.go`、`cool-next/codegen/analyze.go`

- [ ] 先写临时模块测试，覆盖 RawWhere 字面量、命名常量和常量拼接通过，变量、函数返回值、`fmt.Sprintf` 和运行时拼接失败。
- [ ] 写常量名称测试，覆盖 NewColumnRef、NewColumnRefOf、Of、EqFrom、LikeFrom、All、As、LeftJoin 和 InnerJoin 的名称/别名参数。
- [ ] 写符号身份测试，证明同名本地函数不触发规则，包别名导入和点导入仍按 `go/types` 对象正确识别。
- [ ] 写稳定诊断测试，固定相对文件、行列、排序和未占用错误码；诊断不得包含 RawWhere 绑定值。
- [ ] 复用 `loadedPackage.TypesInfo` 和 AST，不重新调用 packages.Load，不修改 Model、Render 或 Pipeline。
- [ ] 使用 `go/types.Info.Types[expr].Value` 判断编译期字符串常量，不手写 Go 常量表达式解析器。
- [ ] RawWhere 第一个参数必须是非空编译期字符串常量；后续参数允许任意运行时表达式并只作为绑定值。
- [ ] 对名称参数同时检查编译期常量和 lowerCamelCase；不分析动态表名，因为公开 DSL 没有表名入口。
- [ ] 在 `analysis.analyzeModule` 已有源码范围内调用规则，复用 eligible 文件过滤和 Position 转换。
- [ ] 不增加通用 rule registry、插件接口或未来 Controller/QueryPlan 检查占位。

Run:

```bash
go test ./cool-next/codegen -run 'TestQueryValidation' -count=1
go run ./cmd/cool check
```

### Task 5: Fuzz And Boundary Verification

**Tracks:** 20.1-20.8

**Write set:** `cool-next/crud/fuzz_test.go`

- [ ] 添加一个 QueryRequest 名称 fuzz 入口，断言任意输入只会成功构造合法名称或返回 Validate 异常，不泄露值。
- [ ] 添加一个 ColumnRef/alias fuzz 入口，捕获 Core panic 并确认非法输入不会产生可用引用。
- [ ] 添加一个 In 集合最小 fuzz 入口，只使用受控标量切片验证空/非空边界，不建立大型随机 AST。
- [ ] 运行 `go test` 普通 seed；仅在本地受控时间运行短 fuzz，不把长时间 fuzz 加进默认 make check。
- [ ] 用 `go list -deps ./cool-next/crud` 与源码搜索确认 crud 不依赖数据库、Transport、Controller、codegen 或 modules。

Run:

```bash
go test ./cool-next/crud -run '^$' -fuzz '^FuzzQueryRequestName$' -fuzztime=10s
go test ./cool-next/crud -run '^$' -fuzz '^FuzzColumnRef$' -fuzztime=10s
go test ./cool-next/crud -run '^$' -fuzz '^FuzzInValues$' -fuzztime=10s
go list -deps ./cool-next/crud
```

### Task 6: Full Verification And Review

- [ ] 仅对本计划列出的 Go 文件执行 gofmt；不格式化或修改其他模块。
- [ ] 运行 crud 和 codegen 专项测试、Race Test、`cool check`、全仓单测、vet、make check 与 diff check。
- [ ] 审查注释符合 `COMMENT_STYLE.md`，不存在标识符翻译、句号、实现细节注释或无注释导出符号。
- [ ] 审查公开 API 与批准规格逐项一致，没有额外错误类型、AST visitor、数据库 API、Controller 包装或 QueryPlan。
- [ ] 审查每个非平凡分支均有最小测试，RawWhere 动态表达式和 DSL panic 均有失败测试。
- [ ] 检查工作区差异只包含本计划列出的 crud/codegen 文件和本地忽略的模块 20 文档。

Run:

```bash
gofmt -w cool-next/crud/*.go cool-next/codegen/query_validate.go cool-next/codegen/query_validate_test.go cool-next/codegen/analyze.go
env GOCACHE=/private/tmp/cool-admin-go-build go test ./cool-next/crud ./cool-next/codegen -count=1
env GOCACHE=/private/tmp/cool-admin-go-build go test -race ./cool-next/crud ./cool-next/codegen -count=1
go run ./cmd/cool check
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
go vet ./...
make check
git diff --check
rg -n 'gdb|ghttp|grpc|core/controller|core/service|modules/' cool-next/crud --glob '*.go'
```

## Completion Criteria

- [ ] 20.1-20.8 均由对应任务和测试覆盖。
- [ ] QueryRequest 保留缺失、零值、false、空字符串和显式 null 的区别。
- [ ] 查询 AST 只能通过框架封闭节点或常量 RawWhere 加绑定参数构造。
- [ ] 动态字段名、别名、请求参数名和 RawWhere 表达式由 cool check 拒绝。
- [ ] QueryBuilder 只追加节点，不连接数据库或泄露内部节点。
- [ ] crud 不依赖数据库、Transport、Controller、codegen 或业务模块。
- [ ] 未实现模块 21 的 Descriptor 解析、QueryPlan、SQL、分页、Count 或查询执行。
