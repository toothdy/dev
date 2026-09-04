# 行为等价精简实施计划

> 日期：2026-08-25
> 依据：`docs/superpowers/specs/2026-08-25-behavior-preserving-simplification-design.md`
> 状态：待实施

## 1. 目标

在不改变导出 Go API、业务层用法、请求响应、SQL、错误语义和生命周期顺序的前提下，删除 Codegen 构造器转发、CRUD 重复校验、Assembly 重复遍历、单用途 `contains*` 和不可达的 `crypto/rand.Read` 错误分支。

预计生产 Go 代码净减少 300–350 行。CRUD 校验只在不变量证明闭合时删除，因此实际减少量可以低于目标。

## 2. 硬性约束

1. `modules/**` 中手写的 Controller、Service、Entity、DTO、Middleware、Schedule、配置和 Seed 数据不修改；
2. 业务层的构造器签名、Controller DSL、CRUD 入口、Service 调用和任务协议不变；
3. 业务目录中仅允许 `modules/modules_gen.go` 通过 `cool generate` 重新生成；
4. 导出类型、函数、方法和接口签名不变；
5. 不删除 `Store.Claim`、`ConsumerRuntime` 状态检查、任务参数解析能力、`TaskJob`、`DemoService` 或 Seed 显式时间写入；
6. 生成文件不手工编辑，只由项目现有生成器写入；
7. 每次完整修改一个文件后再处理下一个文件；
8. 任一项需要业务层迁移或改变可观测行为时，跳过该项而不扩大范围。

## 3. 变更概览

| 任务 | 生产文件 | 测试文件 | 预计净减少 |
|---|---|---|---:|
| 1. `slices` 替换 | 5 个 | 不新增 | 约 30 行 |
| 2. 随机数错误分支 | 8 个 | 不新增 | 约 15–20 行 |
| 3. Codegen 构造器直调 | `render.go` + 生成文件 | 2 个 | 约 158 行 |
| 4. Assembly 校验合并 | 1 个 | 原则上不改 | 约 45 行 |
| 5. CRUD 重复校验 | 1–2 个 | 1–2 个 | 最多 100 行 |

## 4. 实施任务

### 任务 0：建立基线与业务层门禁

文件：无。

步骤：

1. 确认工作区只有本计划已知的变更；
2. 运行 Codegen 静态检查，确认当前 `modules_gen.go` 新鲜；
3. 运行相关包测试和全量 Go 测试，记录基线；
4. 在 `/private/tmp` 保存生成文件中的路由、Provider、Component、Lifecycle 和 Transport 段落摘要，用于任务 3 后比较；
5. 每个后续任务结束时运行业务层零差异检查。

验证：

```bash
go run ./cmd/cool check
go test ./cool-next/codegen ./cool-next/crud ./cool-next/core/app -count=1
go test ./... -count=1
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 1：用 `slices` 替换单用途查找函数

文件：

- 修改：`cool-next/db/schema/inspect.go`
- 修改：`cool-next/db/schema/diff.go`
- 修改：`cool-next/core/controller/plan.go`
- 修改：`cool-next/codegen/route_analysis.go`
- 修改：`cool-next/codegen/controller_analysis.go`

步骤：

1. `containsString([]string, string)` 和 `containsAPI([]APIType, APIType)` 直接替换为 `slices.Contains`；
2. `containsColumn`、`containsIndex` 和 `containsControllerEntity` 的唯一调用点内联为 `slices.ContainsFunc`；
3. 删除六个已无调用的 `contains*` 函数，补充或清理 `slices` import；
4. 不改动 `containsAdminRole`、`containsMutableReference`、`containsProtobufMessage` 等具有额外业务或递归语义的函数。

验证：

```bash
go test ./cool-next/db/schema ./cool-next/core/controller ./cool-next/codegen -count=1
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 2：删除直接 `crypto/rand.Read` 的不可达错误分支

文件：

- 修改：`cool-next/core/app/request.go`
- 修改：`cool-next/auth/service.go`
- 修改：`cool-next/auth/jwt/service.go`
- 修改：`cool-next/db/driver/probe.go`
- 修改：`cool-next/outbox/store/probe.go`
- 修改：`cool-next/outbox/store/database.go`
- 修改：`cool-next/outbox/worker.go`
- 修改：`cool-next/codegen/scaffold_write.go`

步骤：

1. `NewTraceID`、`randomID`、`randomJTI` 保留现有 `(string, error)` 签名，实现中直接 `rand.Read` 并返回 `nil` 错误；
2. 私有 `newProbeTableName`、`newProbeMessageID`、`newClaimToken`、`newWorkerID` 可移除无意义的 `error` 返回，同步精简包内调用方；
3. `createTemporaryCodeFile` 保留文件创建错误和重试，只删除 `rand.Read` 分支；
4. Auth/JWT 的可注入 ID/JTI 函数类型不变，调用方继续传播注入实现的错误；
5. 不改动 `io.Reader`、`io.ReadFull`、`rand.Int` 或随机数注入路径的错误处理。

验证：

```bash
go test ./cool-next/core/app ./cool-next/auth ./cool-next/auth/jwt ./cool-next/db/driver -count=1
go test ./cool-next/outbox ./cool-next/outbox/store ./cool-next/codegen -count=1
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 3：让 Codegen 直接生成构造器调用

文件：

- 修改：`cool-next/codegen/render.go`
- 修改：`cool-next/codegen/render_test.go`
- 修改：`cool-next/codegen/render_integration_test.go`
- 重新生成：`modules/modules_gen.go`

步骤：

1. 先把测试期望从 `build*` 声明/调用改为原构造器选择器调用，同时增加“生成结果不得出现 `func build*`”断言；
2. 调整 `writeComponentConstruction`，通过已有 import manager 别名直接拼出 `package.New*(arguments...)`；
3. 删除 `writeComponentDeclarations`、`componentFunctionName` 和对转发函数标识符的重复占用检查；
4. 保持现有四种构造器生成形态：有/无返回值、有/无 `error`；
5. 保持 `exception.WrapCore` 文本、组件局部变量、Hooks、Transport 和 Graph 定义不变；
6. 运行生成器更新 `modules/modules_gen.go`，再次生成必须字节不变；
7. 检查生成差异只包含 `build*` 函数删除和 `assemble` 调用目标替换，路由、Provider、Component、Lifecycle 和 Transport 内容不变。

验证：

```bash
go test ./cool-next/codegen ./cmd/cool -count=1
go run ./cmd/cool generate
go run ./cmd/cool check
go run ./cmd/cool generate
go run ./cmd/cool check
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 4：合并 Assembly 完整校验和前缀扫描

文件：

- 修改：`cool-next/core/app/lifecycle.go`
- 仅当现有用例不足以锁定行为时修改：`cool-next/core/app/lifecycle_test.go`

步骤：

1. 保留当前生命周期测试作为行为基线，不更改断言来迁就新实现；
2. 实现一个包内 Assembly 扫描函数，按当前 Graph 顺序生成合法组件前缀和 Transport 列表；
3. 前缀模式只校验组件数量上限、顺序、Hooks 和 Transport 标记，保持现有 `validateAssemblyPrefix` 语义；
4. 完整模式额外校验组件数量精确匹配、Transport 名称/重复和 Transport 数量；
5. 遇到仅在完整模式中失败的 Transport 名称时，合法回滚前缀必须仍包含当前结构合法组件，与当前二次扫描结果一致；
6. `StartDefinition` 分别处理装配函数失败和完整装配校验失败，保持错误文本、`errors.Join` 时机和回滚顺序；
7. 删除被单次扫描取代的 `validateAssembly`、`validateAssemblyPrefix`、`validAssemblyPrefix` 重复实现。

验证：

```bash
go test ./cool-next/core/app -count=1
go test -race ./cool-next/core/app -count=1
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 5：证明并删除 CRUD 执行期重复校验

文件：

- 修改：`cool-next/crud/apply.go`
- 必要时修改：`cool-next/crud/plan.go`
- 同步内部不变量测试：`cool-next/crud/apply_test.go`
- 必要时补充编译入口用例：`cool-next/crud/plan_test.go`

步骤：

1. 对 `validatePlan`/`validateCondition` 每个分支建立实际代码映射，不接受只基于类型设计的推测；
2. 根表、根别名、Join、Select、Where、Having、Group 和 Order 分别对应 `resolveEntity`、`compileJoin`、`compileSelects`、`compileCondition`、`compileGroups`、`compileOrders` 及 `applyRequestOrder`；
3. 对 `operatorRaw` 额外确认唯一公开构造入口 `RawWhere` 已拒绝空表达式，并确认 Codegen 不允许绕过构造器；
4. 对 `operatorKeyword` 确认列数、参数数量和参数类型由 `compileKeyword` 同时产生；
5. 某个分支若能通过任意现有生产入口构造出无效计划，保留对应执行期检查；
6. 仅在所有分支都闭合时，删除 `applyQueryPlan` 中的二次校验及已无调用的 `validatePlan`/`validateCondition`；
7. `TestApplyQueryPlanRejectsInvalidNodes` 中直接篡改包内私有字段的用例，只能在等价的非法状态已被编译入口用例覆盖后同步移除；不得删除公开输入拒绝用例；
8. 不修改 Controller、Service、CRUD DSL、`CompilePlan`、`ActionPlan` 或 Dispatcher 调用方式。

验证：

```bash
go test ./cool-next/crud ./cool-next/core/controller -count=1
go test -race ./cool-next/crud -count=1
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

### 任务 6：全量验收

文件：无额外修改。

步骤：

1. 运行 `gofmt` 并确认所有 Go 文件格式正确；
2. 连续运行生成和静态检查，确认生成结果稳定；
3. 运行全量测试、`go vet` 和项目 `make check`；
4. 检查手写业务层零差异；
5. 检查导出 API、生成 Graph 段落、路由、SQL 和错误断言无漂移；
6. 统计生产 Go 代码净减少行数，但不因未达 300 行而放宽行为约束。

验证：

```bash
gofmt -w cool-next/db/schema/inspect.go cool-next/db/schema/diff.go cool-next/core/controller/plan.go cool-next/codegen/route_analysis.go cool-next/codegen/controller_analysis.go
gofmt -w cool-next/core/app/request.go cool-next/auth/service.go cool-next/auth/jwt/service.go cool-next/db/driver/probe.go cool-next/outbox/store/probe.go cool-next/outbox/store/database.go cool-next/outbox/worker.go cool-next/codegen/scaffold_write.go
gofmt -w cool-next/codegen/render.go cool-next/codegen/render_test.go cool-next/codegen/render_integration_test.go cool-next/core/app/lifecycle.go cool-next/core/app/lifecycle_test.go
gofmt -w cool-next/crud/apply.go cool-next/crud/plan.go cool-next/crud/apply_test.go cool-next/crud/plan_test.go
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./... -count=1
go vet ./...
make check
git diff --check
git diff --exit-code -- modules ':(exclude)modules/modules_gen.go'
```

## 5. 风险与中止条件

| 风险 | 中止或回退条件 |
|---|---|
| 生成器直调改变组件顺序 | Graph/Component/Lifecycle/Transport 摘要任意一项变化即中止任务 3 |
| Assembly 扫描改变回滚前缀 | 现有 lifecycle 用例任意失败即保留原三函数 |
| CRUD 编译器不变量有缺口 | 保留有缺口的校验；无法分割时整个任务 5 跳过 |
| 随机数精简触及可注入 Reader/函数 | 该路径保留原错误处理 |
| 需要修改手写业务层 | 对应优化项立即中止 |
| 导出 API 签名变化 | 对应优化项立即中止 |

## 6. 完成标准

- `modules/**` 手写业务文件零差异；
- `modules/modules_gen.go` 可重复生成且仅出现计划内差异；
- 导出 Go API、业务层用法、错误语义和生命周期顺序不变；
- `go run ./cmd/cool check`、`go test ./... -count=1`、`go vet ./...` 和 `make check` 全部通过；
- 没有新增第三方依赖；
- 净减少行数不作为放宽上述条件的理由。
