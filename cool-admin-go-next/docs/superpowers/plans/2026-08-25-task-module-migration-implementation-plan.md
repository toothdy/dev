# 任务调度模块迁移实施计划

> 日期：2026-08-25  
> 依据：`docs/superpowers/specs/2026-08-25-task-module-migration-design.md`  
> 状态：已完成

## 1. 目标

在 Go v2 现有静态模块、Descriptor、CRUD、事务、生命周期、Seed 和 HTTP Transport 上实现 Task 业务模块，使 `cool-admin-vue` 无需修改即可复用 Node 任务模块的接口和行为。

## 2. 实施约束

1. 不新增第三方依赖，定时触发统一使用 GoFrame `gcron`；
2. 不手写 Descriptor、DO 或 `modules/modules_gen.go`，统一使用 `cool generate`；
3. 数据库查询使用 GoFrame 参数化 ORM，写入和删除复用现有基础 Service 与删除归档；
4. 不引入队列、反射调用容器或多租户；
5. 每次完整修改一个文件，再处理下一个文件；
6. 当前仓库忽略 `*_test.go`，测试文件仍保留在工作区并作为本次验证依据。

## 3. 任务

### 任务 1：模块、实体、DTO 与种子

文件：

- 创建：`modules/task/config.go`
- 创建：`modules/task/entity/info.go`
- 创建：`modules/task/entity/log.go`
- 创建：`modules/task/dto/info.go`
- 创建：`modules/task/db.json`
- 创建：`modules/task/entity/entity_test.go`

步骤：

1. 保持 Node 模块名称、描述、顺序和 `log.keepDays` 默认值；
2. 按设计逐列建立 `task_info`、`task_log`，`task_log.taskId` 建索引；
3. 建立启停、单次执行和日志分页请求 DTO；
4. 原样迁移 Node 两条示例任务；
5. 编译 Descriptor 并验证字段、默认值和可空性。

验证：

```text
go test ./modules/task/entity -count=1
```

### 任务 2：定时规则与调用注册表

文件：

- 创建：`modules/task/service/pattern.go`
- 创建：`modules/task/service/registry.go`
- 创建：`modules/task/service/demo.go`
- 创建：`modules/task/service/pattern_test.go`
- 创建：`modules/task/service/registry_test.go`

步骤：

1. 把 `taskType`、`cron`、`every` 编译成 `gcron` 模式，5 段 cron 补秒位，间隔任务用 `@every`；
2. 实现 6 段模式的前向扫描以计算下次执行时间；
3. 实现顶层感知的 `目标.方法(参数)` 解析；
4. 建立静态调用注册表和演示任务目标。

验证：

```text
go test ./modules/task/service -count=1
```

### 任务 3：执行器与调度器

文件：

- 创建：`modules/task/service/executor.go`
- 创建：`modules/task/service/scheduler.go`
- 创建：`modules/task/schedule/task_job.go`
- 创建：`modules/task/service/integration_test.go`

步骤：

1. 执行器按 ID 重新读取任务、判定起止时间，用框架原语 `db.Runtime.LockRows` 抢占执行租约；
2. 调用注册表目标，成功和失败分别落库，清理超期日志，最后释放锁；
3. 调度器实现 `OnStart` 装载、`OnStop` 优雅停止、`Sync` 单条同步；
4. 定时器以后台 Context 注册，回调内重新读取任务并在缺失或停止时自注销；
5. 生命周期组件按目录协议放在 `schedule/`，只负责 `OnStart` 装载与 `OnStop` 停机。

验证：

```text
go test ./modules/task/service -count=1
```

### 任务 4：任务 Service 与 Controller

文件：

- 创建：`modules/task/service/info.go`
- 创建：`modules/task/controller/admin/info.go`
- 创建：`modules/task/controller/admin/normalizer.go`
- 创建：`modules/task/controller/admin/controller_test.go`
- 创建：`modules/task/service/prepare_test.go`

步骤：

1. 覆写 `Add`、`Update`、`Delete`，在委托基础 Service 后归一化定时字段并同步调度器；
2. 实现 `Start`、`Stop`、`Once` 和日志分页；
3. 按 Node 开放 `add/delete/update/info/page` 与四条自定义路由，`page` 支持 `status`、`type` 等值过滤；
4. 用 `CurdOption.Before` 挂请求体裁剪，兼容前端回传整条记录的写入请求。

验证：

```text
go test ./modules/task/... -count=1
```

### 任务 5：生成、装配与全量门禁

文件：

- 更新：`modules/modules_gen.go`（由生成器产出）
- 更新：`modules/permission_regression_test.go`（补齐 dict 与 task 路由快照）

步骤：

1. 运行生成与静态检查；
2. 补齐权限快照，使其覆盖真实路由图全量路由；
3. 运行全量测试、`go vet` 和格式检查。

验证：

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./... -count=1
go vet ./...
gofmt -l modules/task
```

## 4. 风险与对策

| 风险 | 对策 |
| --- | --- |
| 定时器在事务回滚后残留 | 执行入口重新读取任务行，缺失或停止时自注销 |
| 定时器复用请求事务 Context | 一律以后台 Context 注册条目 |
| `limit` 是 MySQL 保留字 | 只经 GoFrame ORM 访问，不拼接裸 SQL |
| 多实例重复执行 | `lockExpireTime` 条件更新抢锁 |
| 任务目标 panic 逃逸 | 执行器回调内恢复并按失败日志落库 |
| `gdb.Model` 链式可变，读取的 `Fields` 会限制随后的写入列 | 读写各取一次 `Model`，并由集成测试钉住租约列确实落库 |
| 前端回传整条记录导致更新被 Binder 拒绝 | Controller 层 `Before` 裁剪写入请求体 |
