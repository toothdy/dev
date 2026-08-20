# Node 对齐模块配置改造进度

- 日期：2026-07-30
- 对应计划：`docs/superpowers/plans/2026-07-30-node-aligned-module-config.md`
- 对应设计：`docs/superpowers/specs/2026-07-30-node-aligned-module-config-design.md`
- 当前状态：Task 1-14 已完成；Task 15、16 部分完成；Task 17 未开始

## 1. 重要恢复说明

上轮 Task 15、16 的执行 agent 被用户主动停止。停止前已经写入了部分改动，因此当前工作树不是 Task 14 的干净结束状态，也不是 Task 15、16 的完整完成状态。

后续接手时必须：

1. 先审计现有 diff 和文件内容，确认哪些改动已经落地。
2. 在现有改动上继续完成，不要重新执行整套迁移。
3. 不要回滚、覆盖或清理无法确认归属的修改。
4. 在 Task 15、16 收口前不要宣称整体改造完成。
5. 不要执行 `git add`、`git commit` 或破坏性 Git 命令。

建议先执行：

```bash
git status --short
git diff --check
git diff -- manifest/config/config.yaml modules/modules_gen.go cool/app/app.go
git diff -- README.md docs/module-development.md .github/workflows/go.yml
git diff -- cool/codegen/module/guards_test.go cool/codegen/module/layout_guard_test.go
```

## 2. 已完成：Task 1-14

核心框架、codegen 和四个业务模块迁移已经完成。

已完成的主要内容：

- 新增强类型 `module.Declaration[T]`、`ComponentRef` 和泛型配置加载器。
- 模块配置统一从 `module.<key>` 读取。
- Application 在组件工厂和副作用之前准备模块配置。
- Scanner 使用模块根目录 allowlist。
- `ModuleConfig()`、Config 类型和 Middleware 引用由 codegen 静态分析。
- Task HandlerDefinition 只从 `service/**` 发现，并与实现共置。
- codegen 使用项目级依赖图和集中 Renderer/Writer。
- CLI 已移除 `--module`，所有命令按完整项目分析。
- Base、Dict、Recycle、Task 已迁移到 Node 对齐目录。
- 四个模块根目录只保留 `config.go`、可选 JSON 和允许的业务目录。
- 已删除模块内 `config/`、`handler/`、`providers.go`、`module_gen.go`。
- 根级测试已迁入对应业务目录。

Task 1-14 已通过的主要验证：

```bash
go test ./cool/module ./cool/registry ./cool/codegen/module ./cmd/cool -count=1
git diff --check
```

四模块的聚焦测试和主要 race 测试已通过。Base 中需要监听 `127.0.0.1:0` 的测试受当前沙箱限制，不能据此判断为代码回归。

## 3. 部分完成：Task 15

### 已经写入的内容

- `manifest/config/config.yaml` 中原顶层 `task` 配置已迁移到 `module.task`。
- 已运行过一次集中生成，`modules/modules_gen.go` 已更新为全局静态装配文件。
- 模块内旧 `module_gen.go` 已清理。
- `modules/modules_test.go` 已按真实依赖顺序调整断言。
- 生成后的真实依赖顺序当前为：`recycle`、`base`、`dict`、`task`。
- 修复了 Application 过早深拷贝 `ModuleSpec.TaskHandlers` 的问题：配置阶段完成后再建立最终快照，避免生成器预分配的 HandlerDefinition 被切断。
- 已增加 Task Handler 快照相关回归测试。

### 尚未完成或尚未确认

- Task 15 agent 在最终验证阶段被停止，尚无完整的通过报告。
- 需要确认 `modules/modules_gen.go` 与当前源码完全同步且二次生成无差异。
- 需要确认所有模块内旧生成文件均已删除。
- 需要完整复跑 modules、Application 和 Task 相关聚焦测试。
- 需要确认 `cool check` 返回成功，而不是仅完成 Overlay 后报告生成文件过期。
- 需要确认 Task Handler 快照修复不会改变显式传入 ModuleSpec 的隔离语义。
- 需要区分真实失败和沙箱禁止监听、Redis/MySQL 不可用造成的环境失败。

### Task 15 后续动作

1. 审计 manifest，确保顶层不再存在 `task`，且 `module.task` 内容完整。
2. 审计 `modules/modules_gen.go`，确保没有 `Module()`、反射、运行时 DI、`init()` 或空白注册导入。
3. 确认不存在模块内 `module_gen.go`。
4. 运行一次 `go run ./cmd/cool generate`。
5. 紧接着运行 `go run ./cmd/cool check`，确认生成结果稳定。
6. 复跑 Task 15 聚焦测试和 race 测试。

建议命令：

```bash
rg -n '^task:' manifest/config/config.yaml
rg -n '^  task:' manifest/config/config.yaml
find modules -mindepth 2 -name module_gen.go -print
rg -n 'func Module\(|reflect|init\(\)|google/wire|uber.org/fx' modules/modules_gen.go

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go run ./cmd/cool generate

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go run ./cmd/cool check

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go test ./modules/... ./cool/app ./cmd/cool -count=1

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go test -race ./cool/app ./modules/task/service ./modules/task/event ./modules/task/queue -count=1
```

## 4. 部分完成：Task 16

### 已经写入的内容

- 已新增或扩展模块布局与生成物 guard 测试。
- Guard 已覆盖以下约束：
  - 模块根目录 allowlist。
  - 禁止 `config/`、`handler/`、`db/`、`hooks/`、`job/`。
  - 禁止模块内 `providers.go`、`module_gen.go`、`register.go`。
  - 只允许唯一的 `modules/modules_gen.go` 生成文件。
  - 禁止模块业务代码直接调用 `g.Cfg()`。
  - manifest 禁止顶层 `task`，必须存在 `module.task`。
  - 生成代码禁止反射、运行时 DI、`init()` 和旧 `Module()` 聚合调用。

停止前的聚焦结果显示：布局、配置和生成物 AST guard 已通过；文档协议 guard 因 README 和模块开发文档仍包含旧约定而失败。

### 尚未完成

- `README.md` 尚未完成最终更新和审校。
- `docs/module-development.md` 仍包含旧的局部生成示例，例如 `generate --module dict`。
- 文档仍可能把模块内 `module_gen.go` 描述为合法结构。
- `.github/workflows/go.yml` 尚未完成最终更新和验证。
- Task 16 guard 测试尚未在文档、CI 更新后复跑至全绿。
- 需要检查 guard 是否误扫测试夹具、临时目录或非模块源码。

### Task 16 后续动作

1. 更新 README 的模块目录和生成命令说明。
2. 更新模块开发文档，明确唯一根 `config.go`、`module.<key>` 和集中生成。
3. 删除所有 `--module` 使用说明。
4. 明确 Task HandlerDefinition 必须与实现共置于 `service/**`。
5. 更新 CI：先执行 `cool check` 和 guard，再执行全量 Test、Race、Vet、Build。
6. 运行 Task 16 聚焦测试并修复误报。

建议命令：

```bash
rg -n -- '--module|module_gen.go|providers.go|handler/' README.md docs .github

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go test ./cool/codegen/module \
  -run 'Guard|Layout|GeneratedWiring|MainApplicationDependencies' \
  -count=1

git diff --check
```

## 5. 未开始：Task 17 最终验收

Task 17 必须在 Task 15、16 全部完成后执行。目前不能开始最终验收，也不能使用此前旧阶段的验收结论。

最终验收至少需要覆盖：

```bash
GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go run ./cmd/cool check

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go test ./... -count=1

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go test -race ./cool/module ./cool/registry ./cool/codegen/module ./cool/app ./modules/... -count=1

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go vet ./...

GOCACHE=/tmp/cool-admin-go-next-gocache \
GOTMPDIR=/tmp/cool-admin-go-next-gotmp \
go build -o /tmp/cool-admin-go-next-final ./

go mod tidy -diff
git diff --check
```

还需要进行静态审计：

```bash
find modules -mindepth 2 -type d \
  \( -name config -o -name handler -o -name db -o -name hooks -o -name job \) \
  -print

find modules -mindepth 2 -type f \
  \( -name providers.go -o -name module_gen.go -o -name register.go \) \
  -print

rg -n -- '--module|RegisterModule|registry\.Modules\(\)|func Module\(' .
rg -n 'reflect|google/wire|uber.org/fx|sarulabs/di' modules/modules_gen.go
```

## 6. 已知环境限制

- 当前可用 Go 工具链为 1.26.4；计划中的 1.26.5 在当前环境不可用。
- 沙箱禁止监听 `127.0.0.1:0` 或 `:0`，部分 App/Base HTTP 测试会因此失败。
- Redis `127.0.0.1:6379`、MySQL 和 Auth 外部集成环境不可用。
- 不应为了适配沙箱而修改生产行为或降低测试断言。
- 外部集成矩阵应在具备 MySQL、Redis 和监听权限的 CI 或开发环境补跑。

## 7. 推荐恢复顺序

严格按以下顺序继续：

1. 审计被中止的 Task 15、16 部分改动。
2. 完成 Task 15，并确保 `generate` 后立即 `check` 通过。
3. 完成 Task 16 的 README、模块开发文档、CI 和 guard。
4. 复跑 Task 15、16 的全部聚焦测试。
5. 执行 Task 17 全量 Test、Race、Vet、Build 和静态审计。
6. 将环境限制与真实代码失败分开记录。
7. 所有验收完成后，才能将总体状态标记为完成。
