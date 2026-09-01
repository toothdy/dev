# Cool Next 命名精简实施计划

> 日期：2026-08-29
> 依据：`docs/superpowers/specs/2026-08-29-cool-next-naming-simplification-design.md`
> 状态：已完成（2026-09-01）

## 约束

1. 只修改生产代码中重复上下文、过长或过度描述的名称。
2. 不改变函数参数、返回值、协议字段、错误码、数据库列名和业务顺序。
3. 测试文件只同步编译所需的符号引用，不做风格重构。
4. 不覆盖工作区已有修改，不手工修改生成产物。
5. 每次完整处理一个包，再格式化并运行该包测试。

## 阶段

1. 完成 `modules/base/service` 已确认的权限方法短命名。
2. 精简 `cool-next/auth`、`core`、`crud`、`db` 的私有名称和重复上下文公共名称。
3. 精简 `cool-next/grpc`、`outbox`、`seed`、`eps`。
4. 精简 `cool-next/codegen` 的分析、图、校验和渲染名称。
5. 同步生产调用点与测试中的必要引用。
6. 运行包级测试、全仓测试、`go vet`、`gofmt` 和 `git diff --check`。

## 验收命令

```bash
go test ./cool-next/... -count=1
go test ./modules/base/... -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

## 验证结果

- `go test ./modules/base/... -count=1` 通过。
- `go test ./cool-next/codegen -count=1 -skip '^TestRenderEndToEndGeneratedRegistryCompiles$'` 通过。
- `go vet ./...`、`gofmt -l` 和 `git diff --check` 通过。
- `go test ./cool-next/... -count=1` 和 `go test ./... -count=1` 仅有已知的 `TestRenderEndToEndGeneratedRegistryCompiles` 失败：生成图中 `NewPublisher` 必须先于 `NewWorker`，与本次纯命名修改无关。
