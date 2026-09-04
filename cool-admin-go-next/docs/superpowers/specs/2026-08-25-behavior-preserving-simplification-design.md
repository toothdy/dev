# 行为等价精简设计

## 背景

当前分支完成了静态装配、CRUD 查询计划、Outbox 和任务模块迁移。生产代码中存在一批可以删除的转发函数、重复遍历和手写标准库功能。本设计在不改变现有功能的前提下精简这些实现。

本次的“行为等价”包括：

- 保持导出 Go API 的类型、方法和函数签名。
- 保持合法输入的返回值、数据库写入和 SQL 结构。
- 保持错误类型、错误文本及错误发生阶段。
- 保持组件构造、初始化、启动、监督、停止和回滚顺序。
- 保持任务参数解析、Seed 时间字段和 Outbox 导出接口的当前能力。
- 保持业务层的构造器、Controller DSL、Service 调用、Entity/DTO、配置和任务协议用法。

### 业务层零改造原则

本次精简不得要求业务模块适配框架内部变化。`modules/**` 中手写的 Controller、Service、Entity、DTO、Middleware、Schedule 和模块配置保持不变；唯一允许变化的业务目录文件是由 Codegen 全量重生成的 `modules/modules_gen.go`。

若某项精简需要业务代码更改构造器签名、替换 API、调整 DSL、更换配置键或改变请求/响应结构，该项不实施。

## 目标

1. 删除 Codegen 生成的 34 个纯构造器转发函数。
2. 在编译器不变量完全覆盖的前提下，删除 CRUD 执行阶段的重复计划校验。
3. 将 Assembly 的完整校验、前缀校验和合法前缀提取合并为一次遍历。
4. 用 Go 标准库替代单用途 `contains*` 函数。
5. 删除 Go 1.26 `crypto/rand.Read` 直接调用后不可达的错误分支。

预计净删除 300–350 行。净行数是结果指标，不作为改变行为的理由。

## 非目标

- 不删除或改动 `Store.Claim`。
- 不缩减 `ConsumerRuntime` 重复初始化、启动和停止的状态检查。
- 不改变任务调用字符串对嵌套 JSON、引号内逗号和括号的支持。
- 不删除 `TaskJob`、`DemoService` 及其导出构造器。
- 不依赖所有部署都配置 GoFrame 自动时间维护，因此不删除 Seed 显式时间写入。
- 不精简文档、测试文件或测试夹具。测试仅在生产实现调整导致断言需同步时修改。
- 不为精简框架内部代码而修改手写业务层文件。

## 设计

### 1. 直接调用组件构造器

`writeComponentDeclarations` 当前为每个组件生成一个私有 `build*` 函数，函数体只调用原构造器。生成的 `assemble` 再调用 `build*`。

调整后，`writeComponentConstruction` 根据构造器包别名、函数名和已解析参数，直接生成 `package.New*(...)`。保留现有的返回值接收、`error` 判断和 `exception.WrapCore` 错误文本。

同步删除：

- `writeComponentDeclarations`。
- `componentFunctionName`。
- `validateGeneratedIdentifiers` 中对 `build*` 标识符的占用检查。
- `modules_gen.go` 中由生成器产生的 34 个 `build*` 函数。

构造器排序、Provider 图、依赖选择和组件局部变量名不变。`modules_gen.go` 只通过项目现有生成命令重新生成，不手工编辑。

业务构造器的声明和调用规约不变：业务层仍使用现有 `New*` 签名声明依赖，只是生成的装配代码从间接调用改为直接调用。

### 2. 去除 CRUD 计划的重复校验

`QueryPlan` 的字段全部为包内私有，生产路径只能由 `queryPlanCompiler` 构造。在删除 `applyQueryPlan` 中的 `validatePlan` 之前，必须建立以下对应表：

| 执行期校验 | 编译期不变量来源 |
|---|---|
| 根表和根别名 | Descriptor 解析与固定 `rootQueryAlias` |
| Join 表、别名、类型和字段 | `resolveEntity`、`compileJoin` |
| Select 字段和输出别名 | `compileSelects`、`appendSelect` |
| Where/Having 操作符、值和参数 | `compileCondition` |
| Group 字段 | `compileGroups` |
| Order 字段和方向 | `compileOrders`、`applyRequestOrder` |

对应表中任意一条无法由实际代码证明时，保留该校验，不为达到行数目标而删除。

完全覆盖后，删除 `applyQueryPlan` 的二次校验调用和已无调用的校验函数。编译失败的错误仍在 `CompilePlan` 阶段返回。

Controller 和 Service 层仍使用现有 CRUD DSL、`CompilePlan`、`ActionPlan` 和 Dispatcher 调用方式，不增加新参数、新构造步骤或业务层校验。

### 3. 合并 Assembly 校验遍历

现有三个函数重复检查组件顺序、Lifecycle 能力和 Transport 标记。合并后使用一个包内扫描函数，一次遍历同时生成：

- 已校验的组件前缀。
- 前缀中的 Transport 列表。
- 第一个不变量错误。
- Assembly 是否与 Graph 完整匹配。

`StartDefinition` 保持当前分支：

- 装配函数返回错误时，只回滚已确认合法的前缀。
- 装配成功但结果不完整时，返回现有错误并回滚合法前缀。
- 完整匹配时，按现有顺序初始化和启动。

合并不得改变错误文本或回滚时纳入 Stopper/Transport 的条件。

### 4. 使用 `slices`

对等值比较使用 `slices.Contains`，需要字段或 `types.Identical` 判定时使用 `slices.ContainsFunc`。仅替换单一调用方的包装函数，不改动具有额外业务语义的方法。

范围为：

- Schema 表、字段和索引查找。
- Controller API 类型查找。
- Codegen 路由 Tag 和 Entity 类型查找。

### 5. 删除不可达的随机数错误分支

Go 1.26 的 `crypto/rand.Read` 保证填满切片且不返回错误。仅对直接调用 `crypto/rand.Read` 的生产实现删除不可达分支。

兼容规则：

- `NewTraceID` 等导出函数保留现有返回签名。
- Auth/JWT 中可注入的 `func() (string, error)` 保留签名，调用方继续处理注入实现返回的错误。
- `io.Reader`、`rand.Int` 和 `io.ReadFull` 的错误处理不删除，因为这些路径仍可返回错误。
- 私有且只使用 `crypto/rand.Read` 的辅助函数可改为无 `error` 返回，同步删除其私有调用方的不可达分支。

## 实施顺序

1. 用 `slices` 替换单用途查找函数。
2. 精简直接 `crypto/rand.Read` 路径。
3. 调整 Codegen 构造器调用并重新生成 `modules_gen.go`。
4. 合并 Assembly 校验遍历。
5. 建立 CRUD 不变量对应并删除已证明重复的校验。

每一步只修改相关文件，单个文件的所有调整一次完成，然后运行该包的定向测试。任一步发现需要改变导出 API 或已有边界行为时，停止该项精简并保留现状。

## 验证

定向验证：

- Codegen：生成结果不再包含 `build*`，组件图、构造顺序和错误包装不变。
- CRUD：每类非法输入仍在编译阶段失败，合法计划生成的 SQL 不变。
- Application：完整 Assembly、组件数量不符、顺序不符、Lifecycle 不符、Transport 不符和中途构造失败的错误及回滚结果不变。
- 随机数：输出长度和编码不变，可注入 Reader/函数的失败路径仍可观测。

全量验证：

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l cool-next modules cmd main.go)"
```

## 成功标准

- 导出 Go API 不变。
- `modules/**` 中手写业务文件无差异；仅 `modules/modules_gen.go` 可因重新生成而变化。
- 现有全部测试通过，`go vet ./...` 通过。
- 生成文件可重复生成，再次生成无差异。
- 组件图、路由、CRUD SQL、任务协议、Seed 和 Outbox 行为不变。
- 生产 Go 代码预计净减少 300–350 行；若 CRUD 某项校验无法完成来源证明，允许净减少低于该范围。
