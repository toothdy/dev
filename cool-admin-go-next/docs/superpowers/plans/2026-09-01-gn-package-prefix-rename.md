# Cool Next 冲突包名简化实施计划

> 日期：2026-09-01
> 状态：已完成

## 目标

仅调整会与业务分层重名的框架包，并让目录名与 package 名一致，使 `gopls/goimports` 保存时无需添加显式 import 别名。

```go
import "github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"

var descriptor gnentity.Descriptor[User, uint64]
```

## 约束

1. 仅重命名 `controller`、`entity`、`service`、`http` 四个目录。
2. 其他框架目录、package 和 import 路径保持不变。
3. 不修改 `go.mod` 模块路径。
4. 业务包 `modules/*/entity|service|controller|dto|middleware|schedule` 的包名保持不变。
5. 仅在存在真实包名冲突时生成 import 别名；`_` 和 `.` import 保持原语义。
6. 不改变公共 API、配置字段、数据库字段、错误码和业务逻辑。

## 最终映射

| 旧目录 / package | 新目录 / package |
| --- | --- |
| `core/controller` / `controller` | `core/gnctrl` / `gnctrl` |
| `core/entity` / `entity` | `core/gnentity` / `gnentity` |
| `core/service` / `service` | `core/gnservice` / `gnservice` |
| `core/http` / `apphttp` | `core/gnhttp` / `gnhttp` |

## 实施阶段

### 1. 框架包声明与内部引用

- 按上表移动四个目录并修改前三个 package 声明。
- 删除框架包之间的显式 import 别名。
- 将代码限定符同步为对应 `gn*` 包名。
- 保留测试夹具中模拟业务包的 `package entity/service/controller` 声明。

### 2. Codegen 输出策略

- `cool-next/codegen` 保持原目录和 `codegen` 包名。
- import manager 使用包的真实声明名作为 preferred name。
- preferred name 未冲突时，生成无别名 import。
- preferred name 冲突时，仅为冲突包生成数字后缀别名。
- `modules/modules_gen.go` 必须由 `go run ./cmd/cool generate` 重新生成，不手工修改。

### 3. 业务侧和入口

- 同步 `cmd/**`、`main.go`、`modules/**`、`test/**` 中的框架包限定符。
- 删除 `coreentity`、`coreservice`、`coreroute`、`coredb`、`outboxstore` 等显式别名。
- 业务包自身的 `entity`、`service`、`controller`、`dto` 引用不改。

### 4. 文档

- README 若包含上述目录树或 import 示例，按最终四目录同步。

## 验收

```bash
go test ./cool-next/... -run '^$'
go run ./cmd/cool generate
go run ./cmd/cool generate
make check
go test ./... -count=1
go vet ./...
git diff --check
```

终检要求：

- 目录变更仅包含上述四个 rename。
- Go 源码中不再引用四个旧目录。
- 框架包 import 没有无必要的显式别名。
- 第二次生成 `modules/modules_gen.go` 无 diff。
- 已知 outbox 拓扑顺序用例若仍失败，应与改名前基线一致，不得出现新失败。

## 验收结果

- `go test ./... -run '^$'` 通过。
- `go run ./cmd/cool generate` 连续执行两次，`modules/modules_gen.go` 哈希一致。
- `go vet ./...`、`gofmt -l .`、`git diff --check` 通过。
- 目录变更仅包含四个目标 rename；其他目录均保持原名，无多余框架 import 别名。
- `make check` 与 `go test ./... -count=1` 仅有既有 `TestRenderEndToEndGeneratedRegistryCompiles` 失败：Outbox 拓扑要求 `NewPublisher` 先于 `NewWorker`；其余包通过。
