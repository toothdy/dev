# 21 QueryPlan Compilation And Query Execution Implementation Plan

**Goal:** 在 `cool-next/crud` 将模块 20 的安全查询 AST 编译为不可变 `QueryPlan`，通过 Descriptor 在数据库访问前完成解析和校验，并把计划参数化应用到克隆后的 GoFrame `gdb.Model`。

**Architecture:** `crud` 定义公开但字段私有的 `QueryPlan` 与 `DescriptorResolver`，包内 `compileQueryPlan` 负责执行 Extend、解析实体和字段、规范化节点、校验类型及固定上限；包内 `applyQueryPlan` 使用 `Model.Clone`、`Model.QuoteWord` 和现有链式 API 应用计划。模块 21 不实现 `ActionPlan`，模块 25 后续只委托这两个包内函数。

**Tech Stack:** Go 1.26、标准库 `context` / `errors` / `reflect` / `strings` / `sync`、现有 `core/entity` / `core/exception` / `db/driver` / `db/schema`、GoFrame gdb 2.10.2、Go `testing`

## Preconditions

- 模块 04 的 `entity.Metadata` / `entity.Field` 只读契约和模块 20 的 QueryRequest / 查询 AST 已完成。
- 开发前阅读本地 GoFrame 2.10.2 文档：`go doc gdb.Model.Clone`、`QuoteWord`、`As`、`Ctx`、`LeftJoin`、`InnerJoin`、`Where`、`Fields`、`Group`、`Having`、`Order`、`AllAndCount`。
- 开发前阅读上述方法对应的本地模块源码，特别确认链式方法的 Model 副本行为、Join 参数形状和 `AllAndCount(false)` 的 Count 语义。
- 开发前阅读现有 `core/entity`、`core/exception`、`db/driver`、`db/schema` 和 `test/integration/run.sh`，复用现有 Descriptor、异常、方言探测与三数据库环境，不自建平行基础设施。
- 运行模块 20 专项测试和 `go run ./cmd/cool check`，确认前置基线通过后再开始编码。

## File Structure

- Create: `cool-next/crud/plan.go` - DescriptorResolver、QueryPlan、规范化节点、包内编译器、类型校验和固定上限
- Create: `cool-next/crud/plan_test.go` - Resolver、Join、Select、请求匹配、条件、限制、错误与不可变性测试
- Create: `cool-next/crud/apply.go` - 将 QueryPlan 参数化应用到克隆的 `gdb.Model`
- Create: `cool-next/crud/apply_test.go` - Model 应用、引用、参数化、原对象不变与 SQLite 行为测试
- Modify: `cool-next/crud/fuzz_test.go` - 增加请求值、IN、别名和节点边界的最小模糊测试
- Create: `cool-next/crud/plan_integration_test.go` - MySQL、PostgreSQL、SQLite 复杂查询和 Page/Count 矩阵

本模块不修改 `ast.go`、`query.go`、`request.go`、`core/controller`、`core/service`、`test/integration/run.sh`、生成器、README、Makefile 或 go.mod。

### Task 1: QueryPlan Boundary And Root Resolution

**Tracks:** 21.1、21.2

**Write set:** `cool-next/crud/plan.go`、`cool-next/crud/plan_test.go`

- [ ] 先写失败测试，覆盖 nil Context、nil Resolver、nil 根实体、解析失败、typed-nil Metadata 和无主键 Metadata。
- [ ] 写成功测试，确认根实体固定别名为 `a`，无别名字段与显式 `.Of("a")` 等价，nil QueryRequest 等价于空请求。
- [ ] 定义 `DescriptorResolver.Resolve(entity any) (entity.Metadata, bool)` 和字段私有的 `QueryPlan`；不增加节点 getter、序列化、修改或重新绑定 Resolver 的 API。
- [ ] 定义包内 `compileQueryPlan(ctx, resolver, entity, QueryOp, request)`，供本模块同包测试和模块 25 后续装配调用；不导出 `CompileQuery`。
- [ ] 规范化列只保存表名、表别名、数据库列名、逻辑名、JSON 名、Go 类型、逻辑类型和 nullable 等执行所需数据。
- [ ] 使用 `reflect.TypeOf(entity)` 固定 Resolver 输入实体类型；根字段必须由 `Metadata.Field` 查逻辑名，再读取 `Field.Column`，不接受调用方数据库列字符串。
- [ ] 对输入和计划使用包内固定常量：8 个 Join、128 个规范化节点、1000 个绑定值；不定义公开 `QueryLimits` 或配置入口。
- [ ] 复用 `newCoreError` / `newValidateError`，错误只包含实体、字段、别名或请求参数名，不包含请求值、RawWhere 参数或连接信息。
- [ ] 保持当前简短中文导出注释风格；不为内部节点添加叙述性注释。

Run:

```bash
go test ./cool-next/crud -run 'Test(QueryPlanBoundary|RootResolution|NilQueryRequest)' -count=1
```

### Task 2: Join And Projection Compilation

**Tracks:** 21.2、21.4

**Write set:** `cool-next/crud/plan.go`、`cool-next/crud/plan_test.go`

- [ ] 先写 Join 表驱动测试，覆盖 Left/Inner、别名重复、保留别名 `a`、未知实体、向前引用、悬空 On、左右字段未知、实体类型与别名不一致、字段类型不一致。
- [ ] 按 QueryOp Join 后接 Extend Join 的声明顺序解析；每个新 Join 必须一侧指向当前别名，另一侧只指向根实体或此前 Join。
- [ ] Join 字段比较去除一层 nullable 指针后必须是相同 Go 类型；不做命名类型、数字或字符串隐式转换。
- [ ] 先写 Select 测试，覆盖未配置时只展开 `All("a")`、显式 All、As、未知别名/字段、输出名重复和相同物理列使用不同输出名。
- [ ] `All(alias)` 按 Metadata.Fields 顺序展开持久化字段，输出名使用 JSON 名；Join 永不隐式展开到结果。
- [ ] 写 Group/Having/Order 结构测试，确认 Group/Having 只来自 Extend，Order 为 QueryOp 后接 Extend，方向和列在编译期验证。
- [ ] 规范化节点计数在 All 展开后进行，覆盖 127、128、129 边界以及 Join 7、8、9 边界；静态/Extend 结构超限返回 Core。
- [ ] 不实现隐藏字段、只读字段、客户端排序白名单或默认排序，这些继续归模块 25。

Run:

```bash
go test ./cool-next/crud -run 'Test(QueryPlanJoin|QueryPlanSelect|QueryPlanGroup|QueryPlanOrder|QueryPlanNodeLimit)' -count=1
```

### Task 3: Condition, Request And Value Compilation

**Tracks:** 21.3、21.4、21.8

**Write set:** `cool-next/crud/plan.go`、`cool-next/crud/plan_test.go`

- [ ] 先写 Extend 测试，确认恰好执行一次、接收原始 request、同类节点保持追加顺序；Cool 异常原样传播，普通错误保留 Cause 并包装为 Core。
- [ ] 写 FieldEq 测试，固定缺失跳过、零值/false 标量 Eq、非空 slice/array IN、nullable null 转 IS NULL、非 nullable null 和空集合 Validate 错误。
- [ ] 写 FieldLike 与 keyWord 测试，固定字符串类型、`%value%`、显式空字符串、缺失、null、关键词多字段 OR 分组以及与其他 Where 的 AND 关系。
- [ ] 写静态条件测试，覆盖 Eq/Ne nil、In 元素、LikeValue 原始模式、GT/GTE/LT/LTE 可用逻辑类型、RawWhere 表达式和参数顺序。
- [ ] 字段为指针时只去除一层指针进行类型匹配；非 nil 值必须可直接赋值给目标类型，不做任何弱类型转换。
- [ ] In 每个元素单独校验且不得为 nil；静态 Eq/Ne 中的 `[]byte` 是字节字段标量，静态字节集合使用 `[][]byte`，FieldEq 仍严格遵守“slice/array 转 IN”，不增加字节字段特例。
- [ ] 编译顺序固定为静态 Where、keyWord、FieldLike、FieldEq、Extend Where/Having；每类内部保持输入顺序。
- [ ] 对计划保存的 slice、array 和嵌套 `[]byte` 绑定数据做独立副本；编译完成后修改 QueryOp、Builder 或 QueryRequest 来源不得影响计划。
- [ ] 绑定值计数覆盖 999、1000、1001 边界；请求集合导致超限返回 Validate，静态/Extend 参数导致超限返回 Core。
- [ ] 不 recover DSL panic，不增加 Or/Not DSL、Visitor、计划缓存或第二套值转换器。

Run:

```bash
go test ./cool-next/crud -run 'Test(QueryPlanExtend|QueryPlanFieldEq|QueryPlanFieldLike|QueryPlanKeyword|QueryPlanCondition|QueryPlanBindingLimit|QueryPlanValueCopy)' -count=1
```

### Task 4: Parameterized GoFrame Model Application

**Tracks:** 21.5、21.7

**Write set:** `cool-next/crud/apply.go`、`cool-next/crud/apply_test.go`

- [ ] 先写失败测试，覆盖 nil Context、nil Model、nil Plan 和内部非法节点，均返回 Core 而不是 panic。
- [ ] 定义包内 `applyQueryPlan(ctx, model, plan)`；第一步调用 `model.Clone()`，再依次 Ctx、As(`a`)、Join、Where、Fields、Group、Having、Order。
- [ ] 用 `Model.QuoteWord` 分别引用可信 Descriptor 表别名、列名和输出名；只拼接固定操作符、点号、括号、逗号和占位符，不手写数据库引号或方言分支。
- [ ] Join 只把已解析表名、别名和字段等值表达式传给 `LeftJoin` / `InnerJoin`，不开放动态表名或任意 Join On SQL。
- [ ] 普通条件、关键词 OR 组、Having 和 RawWhere 参数全部通过 `?` 与 args 传给 GoFrame；请求字符串不得进入表达式文本。
- [ ] Having 条件合并成一次调用，避免链式覆盖；Order 按规范化 slice 连续应用，不通过 map 保存顺序。
- [ ] 每次 Apply 再复制 slice/array 绑定数据；不修改 QueryPlan，也不调用 `Model.Raw`、`Model.DB`、`Model.TX`、`Unscoped` 或任何执行 I/O 的方法。
- [ ] 用 SQLite 行为测试验证 Join、Where、Select、Group、Having、Order 的组合结果和注入形状字符串；断言原 Model 仍可独立查询且 fixture 表未受影响。
- [ ] 不断言 GoFrame 私有 SQL 格式，不建立 SQL Golden Test 或自定义 SQL Builder。

Run:

```bash
go test ./cool-next/crud -run 'TestApplyQueryPlan' -count=1
```

### Task 5: Immutability, Concurrency And Fuzz Boundaries

**Tracks:** 21.1、21.7、21.8

**Write set:** `cool-next/crud/plan_test.go`、`cool-next/crud/apply_test.go`、`cool-next/crud/fuzz_test.go`

- [ ] 写同一 Plan 重复应用测试，确认结果、字段顺序和参数顺序稳定。
- [ ] 写并发应用测试，让多个 goroutine 把同一 Plan 应用到独立 Model，确认没有共享可变状态；用 Race Test 验证。
- [ ] 在现有 fuzz 文件增加请求标量/集合入口，断言任意值只会生成合法规范化节点或返回确定的 Validate/Core 错误。
- [ ] 增加别名和 Join 可达性 seed，覆盖根别名、未知别名、向前引用和重复别名，不构建大型随机图。
- [ ] 增加节点/绑定边界 seed，重点验证计数不溢出、错误不泄露输入值；默认测试只运行 seed，长时间 fuzz 不进入 make check。
- [ ] 审查 QueryPlan 不持有 Context、Resolver、Model、闭包、SQL、执行结果或数据库方言，也不需要锁。

Run:

```bash
go test -race ./cool-next/crud -run 'Test(QueryPlanConcurrent|ApplyQueryPlanConcurrent)' -count=1
go test ./cool-next/crud -run '^$' -fuzz '^FuzzQueryPlanRequestValue$' -fuzztime=10s
go test ./cool-next/crud -run '^$' -fuzz '^FuzzQueryPlanAlias$' -fuzztime=10s
go test ./cool-next/crud -run '^$' -fuzz '^FuzzQueryPlanLimits$' -fuzztime=10s
```

### Task 6: Three-Database Query Matrix

**Tracks:** 21.5、21.6、21.8

**Write set:** `cool-next/crud/plan_integration_test.go`

- [ ] 测试名包含 `Integration`；`COOL_INTEGRATION != 1` 时跳过，不在普通单测中要求 Docker 或外部数据库。
- [ ] 在同包测试中读取现有 `COOL_TEST_MYSQL_*`、`COOL_TEST_POSTGRES_*`、`COOL_TEST_SQLITE_PATH`，并导入仓库已安装的三种 GoFrame 驱动；不修改集成脚本。
- [ ] 为 MySQL、PostgreSQL、SQLite 使用同一组 Descriptor 和 fixture；复用 `driver.Probe` 与 `schema.Manager.Apply(Sync, ...)` 建表，不复制生产方言 DDL。
- [ ] 每个数据库使用隔离表并注册 cleanup，避免失败后污染后续用例；错误输出不得包含密码或完整连接配置。
- [ ] 同一测试矩阵覆盖 camelCase 列、Left/Inner Join、标量 Eq、数组 IN、FieldLike、keyWord、Select As、Group/Having 和多字段 Order。
- [ ] 分页路径固定为 `applyQueryPlan -> Page -> AllAndCount(false)`，断言 list 与 total 共享过滤条件、排序不影响 total、Group/Having 统计分组结果行数。
- [ ] 注入形状请求字符串只按值匹配，三库结果一致；不比较完整 SQL 文本。

Run:

```bash
test/integration/run.sh -- go test ./cool-next/crud -run Integration -count=1 -v
```

### Task 7: Full Verification And Review

- [ ] 仅对本计划列出的 Go 文件执行 gofmt；不格式化或修改其他模块。
- [ ] 运行 crud 专项单测、Race Test、三数据库矩阵、`cool check`、全仓单测、vet、make check 和 diff check。
- [ ] 审查 21.1-21.8 每项均有对应测试，非平凡错误分支至少有一个失败用例。
- [ ] 审查本模块新增的公开 API 只有 `QueryPlan` 与 `DescriptorResolver`；没有提前实现 `CompilePlan`、`ActionPlan`、FieldPolicy 或公开 QueryPlan 编译/执行入口。
- [ ] 审查所有表和列来自 Descriptor、所有业务值使用绑定参数、所有错误都不泄露请求值或连接信息。
- [ ] 审查实现没有 SQL Builder、Visitor、缓存、插件、全局可变配置或新依赖。
- [ ] 检查工作区差异只包含本计划列出的 crud 文件以及本轮规格勘误和实施计划文档。

Run:

```bash
gofmt -w cool-next/crud/plan.go cool-next/crud/plan_test.go cool-next/crud/apply.go cool-next/crud/apply_test.go cool-next/crud/fuzz_test.go cool-next/crud/plan_integration_test.go
env GOCACHE=/private/tmp/cool-admin-go-build go test ./cool-next/crud -count=1
env GOCACHE=/private/tmp/cool-admin-go-build go test -race ./cool-next/crud -count=1
test/integration/run.sh -- go test ./cool-next/crud -run Integration -count=1 -v
go run ./cmd/cool check
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
go vet ./...
make check
git diff --check
rg -n 'CompileQuery|QueryLimits|type SQLBuilder|type Visitor|Model\.Raw|Model\.DB|Model\.TX|Unscoped' cool-next/crud --glob '*.go'
```

## Completion Criteria

- [ ] 21.1-21.8 均由对应任务和可运行测试覆盖。
- [ ] QueryPlan 只包含私有、规范化、可复制的数据节点，同一计划可安全重复和并发应用。
- [ ] DescriptorResolver 在 Model 应用前拒绝未知实体、字段、别名、不可达 Join、类型错误和结构超限。
- [ ] FieldEq、FieldLike、keyWord、null、数组和静态条件语义与批准规格一致。
- [ ] `applyQueryPlan` 克隆原 Model，使用 GoFrame 当前数据库引用能力，并只参数化传递业务值。
- [ ] MySQL、PostgreSQL、SQLite 的复杂查询和 `AllAndCount(false)` Page/total 结果一致。
- [ ] 未新增公开 `CompileQuery`、`QueryLimits`、SQL Builder、缓存、插件、新依赖或模块 22-57 能力。
