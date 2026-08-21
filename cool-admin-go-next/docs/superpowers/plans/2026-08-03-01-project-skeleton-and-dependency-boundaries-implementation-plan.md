# 模块 01：工程骨架与依赖边界实施计划

> 日期：2026-08-03  
> 状态：active  
> 需求：`docs/superpowers/specs/2026-08-03-01-project-skeleton-and-dependency-boundaries-requirements.md`  
> 范围：M01-FR-001-M01-FR-018、M01-AC-001-M01-AC-018

## 1. 实施目标

在不实现后续框架能力的前提下，交付根 Go Module、目录骨架、依赖守卫、统一质量门禁、三数据库测试 Harness 和 CI。

实施完成后，以下命令成为模块 01 的验收入口：

```text
make check
make test-race
make test-integration
make check-full
```

## 2. 技术决策

### 2.1 不使用 `gf init`

仓库已有自定义架构和非空文档。`gf init` 会生成 `main.go`、标准业务目录和应用代码，违反 M01-FR-004 及模块 46 的所有权。本模块只使用 Go Module、GoFrame v2.10.2 官方组件和官方数据库驱动。

### 2.2 测试代码归属

- 依赖守卫放在 `test/architecture/`，不新增顶级工具目录；
- 数据库 Harness 放在 `test/integration/database/`；
- 数据库编排放在 `test/integration/compose.yaml`；
- 本地统一入口使用根 `Makefile`；
- 生产代码不得导入任何 `test/**` 包。

### 2.3 GoFrame 数据库接入

Smoke Test 使用 `gdb.New(gdb.ConfigNode{...})` 创建隔离连接，不修改应用全局配置。数据库驱动使用与框架一致的 v2.10.2：

```text
github.com/gogf/gf/v2 v2.10.2
github.com/gogf/gf/contrib/drivers/mysql/v2 v2.10.2
github.com/gogf/gf/contrib/drivers/pgsql/v2 v2.10.2
github.com/gogf/gf/contrib/drivers/sqlite/v2 v2.10.2
```

事务测试使用 `DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error)`；写入使用结构化 DO，不使用 `map[string]any`。

### 2.4 数据库版本策略

- 合并门禁固定使用 `mysql:8.4.0` 与 `postgres:16.3`；
- 定时和发布门禁增加 `mysql:8.0.36` 与 `postgres:9.5.25` 最低版本任务；
- SQLite 引擎由 GoFrame SQLite 驱动嵌入，运行时必须探测实际版本且不低于 3.24；版本解析单测覆盖 3.23 拒绝和 3.24 接受；
- 镜像和 Go 依赖必须使用明确版本，不使用 `latest`。

### 2.5 实施纪律

- 每个任务先写失败测试或失败检查，再补最小实现；
- 每个任务结束执行所列验证命令；
- 不创建 `main.go`、`cmd/cool`、业务模块或后续框架包；
- 开始编码时先把需求文档状态从 `reviewed` 改为 `active`；
- 当前目录不是 Git 工作树，计划不包含 commit 步骤。

## 3. 任务清单

### 任务 1：激活需求并建立 Go Module

关联需求：M01-FR-001-M01-FR-003、M01-AC-001-M01-AC-002。

文件：

- 修改：`docs/superpowers/specs/2026-08-03-01-project-skeleton-and-dependency-boundaries-requirements.md`
- 创建：`go.mod`
- 创建：`go.sum`（加入真实依赖后生成）

步骤：

1. 将需求状态从 `reviewed` 改为 `active`；
2. 在仓库根执行 `go mod init github.com/toothdy/cool-admin-go-next`；
3. 设置 `go 1.26.0` 和 `toolchain go1.26.4`，CI 同样固定使用 Go 1.26.4；
4. 检查仓库中不存在第二个 `go.mod`；
5. 暂不添加未被源码引用的依赖，避免 `go mod tidy` 删除形式化依赖。

验证：

```text
go env GOMOD
go list -m
find . -mindepth 2 -name go.mod
```

预期：`GOMOD` 指向仓库根，Module Path 正确，子目录无 `go.mod`。

### 任务 2：建立目录骨架和仓库卫生规则

关联需求：M01-FR-004-M01-FR-005、M01-AC-003、M01-NFR-003。

文件：

- 创建：`.gitignore`
- 创建：`cool-next/.gitkeep`
- 创建：`cmd/.gitkeep`
- 创建：`modules/.gitkeep`
- 创建：`manifest/config/.gitkeep`
- 创建：`test/integration/.gitignore`

步骤：

1. 创建需求指定的顶级目录；
2. 占位文件保持为空，不定义 Go Package 或 API；
3. 根 `.gitignore` 排除本地配置、`.env`、数据库文件、覆盖率、测试报告和构建产物；
4. `test/integration/.gitignore` 只允许提交模板和编排文件，排除运行时资源；
5. 确认没有创建 `main.go` 和 `cmd/cool`。

验证：

```text
find cool-next cmd modules manifest test -maxdepth 3 -print
find . -name '*.db' -o -name '.env' -o -name 'coverage.out'
```

预期：目录完整，无运行代码和测试残留。

### 任务 3：测试驱动实现依赖守卫

关联需求：M01-FR-006-M01-FR-009、M01-AC-004-M01-AC-007。

文件：

- 创建：`test/architecture/guard.go`
- 创建：`test/architecture/guard_test.go`
- 创建：`test/architecture/repository_test.go`
- 创建：`test/architecture/testdata/valid/`
- 创建：`test/architecture/testdata/direct/`
- 创建：`test/architecture/testdata/indirect/`
- 创建：`test/architecture/testdata/testfile/`
- 创建：`test/architecture/testdata/buildtag/`
- 创建：`test/architecture/testdata/alias/`

步骤：

1. 先写表驱动测试，覆盖合法、直接、间接、测试文件、Build Tag 和别名导入；
2. 运行测试并确认因守卫未实现而失败；
3. 使用 `go/parser` 和 `go/ast` 解析所有 `.go` 文件的结构化 import；
4. 将仓库内 import 映射到目录，建立本地依赖图并检测禁止链路；
5. 扫描时包含 `_test.go` 和所有 Build Tag 文件，排除 `.git`、隐藏缓存、`vendor` 和测试夹具之外的非源码产物；
6. 错误返回导入方、目标、规则和可取得的依赖链；
7. 增加真实仓库检查：`cool-next/**` 依赖闭包不得到达 `modules/**`；
8. 当前无框架 Go Package 时输出 `skeleton-only` 诊断，同时由夹具测试证明规则有效。

验证：

```text
go test ./test/architecture -run TestGuardFixtures -count=1
go test ./test/architecture -run TestRepositoryBoundaries -count=1 -v
```

预期：所有正反夹具通过，真实仓库检查报告骨架状态。

### 任务 4：建立快速门禁和 Race Test

关联需求：M01-FR-010-M01-FR-012、M01-AC-008-M01-AC-010。

文件：

- 创建：`Makefile`

Make Target：

| Target | 行为 |
|---|---|
| `check-mod` | `go mod tidy -diff`、`go mod verify` |
| `check-format` | 检查 `gofmt -l` 输出为空 |
| `check-architecture` | 运行依赖守卫 |
| `test-unit` | `go test ./...` |
| `check` | Module、格式、vet、依赖守卫、单测 |
| `test-race` | `CGO_ENABLED=1 go test -race ./...` |
| `test-integration` | 调用数据库集成测试 Runner |
| `check-full` | `check`、`test-race`、`test-integration` |

步骤：

1. 依次临时创建未格式化文件、vet 失败测试和 `test/architecture/race_probe_test.go` 数据竞争探针，确认对应命令失败后立即删除，再验证门禁恢复成功；
2. 实现 Make Target，并为每个阶段输出稳定名称；
3. 使用 `go mod tidy -diff` 检查 Module，不修改工作区；
4. Race 夹具默认验证共享状态实现无竞争；负向验证只在开发验收时临时注入，不提交故意失败测试；
5. 确认不存在 `cool check` 或 `cool build` 占位命令。

验证：

```text
make check
make test-race
```

预期：快速门禁和 Race Test 成功；任一子阶段失败时 Make 返回非零。

### 任务 5：测试驱动实现数据库配置和版本校验

关联需求：M01-FR-014-M01-FR-017、M01-AC-013、M01-AC-016。

文件：

- 创建：`test/integration/database/config.go`
- 创建：`test/integration/database/config_test.go`
- 创建：`test/integration/database/version.go`
- 创建：`test/integration/database/version_test.go`
- 创建：`test/integration/database/drivers.go`

步骤：

1. 先写配置测试：普通单测模式允许无外部环境；集成模式缺少必需配置必须报错；
2. 先写版本表测试：MySQL 8.x、PostgreSQL 9.5+、SQLite 3.24+ 通过，低版本或无法解析的版本失败；
3. 运行测试并确认失败；
4. 实现结构化环境变量读取，错误只包含数据库类型和缺失键，不回显值；
5. 单测向配置注入可识别的假密码和 DSN 片段，并断言错误与日志不包含原值；
6. 实现数据库类型与版本比较，不使用字符串字典序；
7. 在 `drivers.go` 空白导入三个 GoFrame 官方驱动；
8. 使用以下命令添加 GoFrame 和三个驱动，再执行 `go mod tidy`：

```text
go get github.com/gogf/gf/v2@v2.10.2
go get github.com/gogf/gf/contrib/drivers/mysql/v2@v2.10.2
go get github.com/gogf/gf/contrib/drivers/pgsql/v2@v2.10.2
go get github.com/gogf/gf/contrib/drivers/sqlite/v2@v2.10.2
go mod tidy
```

环境变量契约：

```text
COOL_INTEGRATION
COOL_TEST_MYSQL_HOST
COOL_TEST_MYSQL_PORT
COOL_TEST_MYSQL_USER
COOL_TEST_MYSQL_PASSWORD
COOL_TEST_MYSQL_DATABASE
COOL_TEST_POSTGRES_HOST
COOL_TEST_POSTGRES_PORT
COOL_TEST_POSTGRES_USER
COOL_TEST_POSTGRES_PASSWORD
COOL_TEST_POSTGRES_DATABASE
COOL_TEST_SQLITE_PATH
```

验证：

```text
go test ./test/integration/database -run 'TestLoadConfig|TestVersion' -count=1
go mod tidy -diff
go mod verify
```

预期：边界版本测试通过，缺失配置错误不包含敏感值，依赖无差异。

### 任务 6：建立可重复的数据库编排 Runner

关联需求：M01-FR-014-M01-FR-017、M01-NFR-001-M01-NFR-007。

文件：

- 创建：`test/integration/compose.yaml`
- 创建：`test/integration/run.sh`
- 创建：`test/integration/images.env`

步骤：

1. Compose 定义 MySQL 和 PostgreSQL 服务、健康检查、临时 Volume 与随机宿主端口；
2. `images.env` 固定代表镜像为 `mysql:8.4.0` 和 `postgres:16.3`，并允许兼容性 Job 覆盖为 `mysql:8.0.36` 和 `postgres:9.5.25`；
3. Runner 使用唯一 Compose Project Name，支持并行运行；
4. Runner 在运行时生成测试密码并注入 Compose，不提交 `.env`；
5. 使用 `docker compose up --wait` 或等价健康状态等待，不使用固定 `sleep`；
6. 读取随机映射端口，设置第 5 项定义的环境变量；
7. 为 SQLite 创建唯一临时目录和数据库文件；
8. 使用 `trap` 保证成功、失败和中断后执行 `docker compose down -v` 并删除临时目录；
9. 提供镜像覆盖参数供最低版本和当前批准版本矩阵使用；
10. Runner 支持 `--` 后传入测试命令，默认执行数据库集成测试；
11. Runner 依次输出 MySQL、PostgreSQL、SQLite 的独立测试结果，不输出密码和完整 DSN。

验证：

```text
docker compose --env-file test/integration/images.env -f test/integration/compose.yaml config
test/integration/run.sh -- go test ./test/integration/database -run 'TestLoadConfig|TestVersion' -count=1
```

预期：编排配置有效，三数据库就绪后执行命令，结束时容器、Volume 和 SQLite 文件无残留。

### 任务 7：实现 GoFrame 三数据库 Smoke Test

关联需求：M01-FR-014-M01-FR-016、M01-AC-012-M01-AC-016。

文件：

- 创建：`test/integration/database/harness.go`
- 创建：`test/integration/database/smoke_test.go`
- 创建：`test/integration/database/isolation_test.go`

步骤：

1. 先写同一套表驱动用例，数据库差异只放在连接和版本探测适配器；
2. 集成模式未启用时只 Skip 外部 Smoke Test，配置和版本单测照常执行；
3. 使用 `gdb.New` 和结构化 `gdb.ConfigNode` 建立连接；
4. 健康检查后读取类型、版本、MySQL InnoDB、PostgreSQL UTF-8 和 SQLite 文件模式；
5. 使用随机且受限的表名创建测试表；DDL 保持三库共同可执行，不实现方言层；
6. 使用私有结构化 DO 写入测试记录；
7. 使用 GoFrame Transaction 闭包验证提交可见；
8. 返回测试哨兵错误触发回滚，验证回滚记录不可见；
9. `t.Cleanup` 删除测试表和 SQLite 临时文件；GoFrame 连接池不要求测试显式关闭连接对象；
10. 并行运行两组用例，验证表名、数据库和清理互不干扰；
11. 日志只记录数据库类型、实际版本和测试名。

验证：

```text
make test-integration
make test-integration
```

预期：连续两次三数据库均通过；任一数据库缺失或版本不符时失败而不是 Skip，且无资源残留。

### 任务 8：建立 CI 工作流

关联需求：M01-FR-010-M01-FR-013、M01-FR-017、M01-AC-011、M01-AC-017-M01-AC-018。

文件：

- 创建：`.github/workflows/go.yml`

工作流：

| Job | 触发 | 命令 |
|---|---|---|
| `quality` | PR、push | `make check` |
| `race` | PR、push | `make test-race` |
| `database` | PR、push | `make test-integration` |
| `compatibility` | schedule、workflow_dispatch、release | 最低版本与当前批准版本矩阵 |

步骤：

1. 固定 Go 1.26.4，权限设为 `contents: read`；
2. 缓存 Go Module 与 Build Cache，不缓存测试结果；
3. PR 和主分支三个必需 Job 独立报告；
4. Compatibility Job 通过环境变量覆盖数据库镜像；
5. SQLite 报告实际嵌入版本并执行边界版本单测；
6. 设置合理 Job Timeout 和并发取消策略；
7. 保证日志不打印环境变量集合、密码或完整连接串；
8. 不调用尚未实现的 `cool check/build`。

验证：

```text
make check-full
```

预期：本地完整门禁通过；工作流语法检查通过，四类 Job 与触发器齐全。

### 任务 9：更新使用说明

关联需求：M01-FR-018、M01-D-008、M01-NFR-004、M01-NFR-008。

文件：

- 修改：`README.md`
- 创建：`test/integration/README.md`

步骤：

1. 将 README 当前状态更新为模块 01 已实施；
2. 记录 Module Path、Go/GoFrame 版本和实际目录；
3. 记录 `make check`、`make test-race`、`make test-integration`、`make check-full`；
4. 说明普通单测不需要数据库，完整门禁要求 Docker 与三个数据库全部成功；
5. 记录环境变量名称、镜像覆盖方法、失败定位和清理方式；
6. 明确禁止提交 `.env`、数据库文件和真实凭据；
7. 保留 `cool check/build/run` 尚未实现的事实，不提前宣称可用。

验证：

```text
rg -n 'make check|make test-race|make test-integration|make check-full' README.md test/integration/README.md
rg -n 'cool check|cool build' README.md
```

预期：入口齐全，目标 CLI 仍标记为未实现。

### 任务 10：执行最终验收并收口

关联需求：全部。

文件：

- 修改：`docs/superpowers/plans/2026-08-03-01-project-skeleton-and-dependency-boundaries-implementation-plan.md`
- 必要时修改：模块 01 需求文档中的实施证据，不改变已评审需求行为

步骤：

1. 从干净依赖缓存执行快速门禁；
2. 执行 Race Test；
3. 执行三数据库完整门禁；
4. 执行架构守卫的全部正反夹具；
5. 检查子 Module、绝对路径、凭据、数据库文件和测试残留；
6. 按 M01-AC-001-M01-AC-018 记录通过证据；
7. 确认未实现任何第 3.2 节排除项；
8. 自审 README、需求和实现一致性；
9. 将本计划状态更新为 `completed`。

最终命令：

```text
go mod tidy -diff
go mod verify
make check
make test-race
make test-integration
make check-full
go test ./test/architecture -count=1 -v
find . -mindepth 2 -name go.mod
```

## 4. 需求覆盖

| 任务 | 覆盖需求 | 主要验收 |
|---|---|---|
| 1 | FR-001-003 | AC-001-002 |
| 2 | FR-004-005 | AC-003 |
| 3 | FR-006-009 | AC-004-007 |
| 4 | FR-010-012 | AC-008-010 |
| 5 | FR-014-017 | AC-013、016 |
| 6 | FR-014-017 | AC-012、015-017 |
| 7 | FR-014-016 | AC-012-016 |
| 8 | FR-010-013、017 | AC-011、017-018 |
| 9 | FR-018 | 文档与入口证据 |
| 10 | 全部 | AC-001-018 |

## 5. 完成条件

- 10 个任务全部完成；
- M01-D-001-M01-D-008 均有实际文件或自动化证据；
- M01-AC-001-M01-AC-018 全部通过；
- 快速、Race、三数据库和完整门禁均成功；
- 无 Secret、绝对路径、测试残留和子 Module；
- 需求文档保持 `active`，实施计划状态为 `completed`；
- 后续模块可以直接复用依赖守卫、质量入口和数据库 Harness。

## 6. 当前实施状态

> 最后检查：2026-08-03

- 任务 1-9：已完成；
- 任务 10：进行中；
- `make check`：通过；
- `make test-race`：通过；
- `go test ./test/architecture -count=1 -v`：全部正反夹具通过，真实仓库报告 `skeleton-only`；
- 普通测试模式下数据库配置、版本、驱动注册与 Harness 测试：通过；
- 本机 MySQL `127.0.0.1:3306` 单库 Smoke Test：通过，实际版本为 `8.0.43`，连接、InnoDB、事务提交和回滚均通过；
- 子 Module、数据库文件、SQLite 文件、`.env` 和覆盖率文件残留检查：通过；
- `make test-integration` 与 `make check-full`：未完成；PostgreSQL/SQLite 实跑按用户指示暂缓；
- 在 MySQL、PostgreSQL、SQLite 三数据库实跑通过前，本计划保持 `active`。
