# Recycle 模块实施计划

**Goal:** 为 `cool-admin-go-next` 实现 Node 兼容的快照式数据回收站，并把 Base、Dict、Task 的管理端删除纳入同步事务归档和可恢复批次。

**Architecture:** `cool/recycle.Manager` 统一管理删除事务、归档批次、依赖图、Bypass 和恢复 Hook；`modules/recycle` 按标准模块目录提供实体、Store、Service、Controller、Schedule 和菜单。通用 CRUD 默认归档主实体，自定义 Service 显式追加关联数据。MySQL 是归档与恢复的唯一事实来源。

**Tech Stack:** Go 1.23、GoFrame v2.10.2、MySQL、现有模型元数据/Schema Sync/CRUD/租户/模块 Runtime。

**Status:** 2026-07-29 已完成实现、自审和验收。实际验收使用现有 Base、Dict、Task MySQL 开关及 Task Redis 开关；Task 另增加聚合归档恢复的真实 MySQL 用例。

## 约束

- 遵循 `docs/superpowers/specs/2026-07-28-recycle-module-design.md`。
- 配置键使用 Node 同名 `cool.crud.softDelete`，默认 `true`。
- 不增加 `deleted_at`，不启用 GoFrame Soft Time。
- 不修改现有 Vue 页面或 Node 项目。
- 保持 `/admin/recycle/data/info|page|restore`、权限和 EPS 协议。
- 普通恢复冲突静默跳过并保留归档，不向客户端返回冲突详情。
- 当前工作区包含用户未提交的 Task 与 EPS 修改；所有实现必须在其基础上增量编辑，禁止回退或覆盖。
- 只提交本计划和后续 Recycle 明确涉及的文件。

## Agent 分工

- **Core Agent:** `cool/recycle`、`cool/crud`、`cool/registry`、`cool/app` 的框架契约与单元测试。
- **Module Agent:** `modules/recycle`、配置、菜单、Schedule、HTTP/EPS 和模块级测试。
- **Integration Agent:** Base、Dict、Task 自定义删除接入、恢复 Hook 与聚焦测试。
- **Root Agent:** API 边界裁决、冲突集成、静态审计、全量验收和最终修复。

Agent 不创建独立 worktree；共享工作区中按以上文件所有权编辑。跨所有权改动先由 Root Agent 协调。

## Task 1：Recycle 核心契约

- [ ] 创建 `cool/recycle/context.go`，实现删除请求元数据、操作人和类型化 Bypass。
- [ ] 创建 `cool/recycle/store.go`，定义事务内批次持久化、分页/详情、恢复状态和清理所需接口。
- [ ] 创建 `cool/recycle/batch.go`，实现 Item、单主键/联合唯一键身份、分支、父依赖、恢复顺序和 10000 Item 上限。
- [ ] 创建 `cool/recycle/manager.go`，实现配置开关、事务所有权、根快照、Service 回调和提交后动作。
- [ ] 创建 `cool/recycle/restore.go`，实现全 Item 依赖校验、拓扑顺序、Savepoint、普通冲突分类和幂等状态。
- [ ] 为配置开关、Bypass、缺少操作者、依赖环、联合身份、无损 bigint 和回滚语义增加单元测试。

## Task 2：模型元数据和 Registry 接线

- [ ] 为模型定义增加稳定资源键或建立由 Module/Name 派生的冻结索引，兼容无主键关系模型的联合唯一身份。
- [ ] 扩展 `registry.RuntimeDeps`、`registry.Deps` 和必要的 MiddlewareDeps，使 Recycle Provider/Manager 可安全注入。
- [ ] Application 只接受一个 Recycle Provider，重复或开启配置但缺失 Store 时启动失败。
- [ ] CRUD Runtime 接收 Manager；默认 Delete 在 Manager 事务中归档根实体并物理删除。
- [ ] `softDelete=false` 和 Bypass 走相同业务事务但不写归档。
- [ ] 保持现有 CRUD Hook、影响行数、租户谓词和 DeleteHandler 优先级。
- [ ] 增加 Application、Registry、CRUD 默认删除的回归测试。

## Task 3：Recycle 实体与配置

- [ ] 创建 `modules/recycle/entity/data.go`，定义 Node 字段、恢复状态、剩余数量和租户索引。
- [ ] 创建 `modules/recycle/entity/item.go`，定义资源、身份、快照、分支、依赖、顺序、状态和内部错误。
- [ ] 创建 `modules/recycle/entity/models.go`，注册两张模型并校验字段/索引。
- [ ] 创建 `modules/recycle/config.go`，读取 `cool.crud.softDelete`，缺省为 `true`。
- [ ] 在 `manifest/config/config.yaml` 增加 Node 同名配置及中文注释。
- [ ] 创建合法空 Seed `modules/recycle/db.json`。
- [ ] 增加模型、配置和 Schema Sync 单元测试。

## Task 4：Store、Service 和恢复

- [ ] 创建 `modules/recycle/event/data.go`，实现同步 Store 适配器，不使用异步事件。
- [ ] 创建 `modules/recycle/service/data.go`，实现租户分页、详情、操作人联表、批次恢复和过期清理。
- [ ] 恢复只使用冻结模型和参数化 INSERT，不使用 Save/Upsert。
- [ ] 对每个分支使用 MySQL Savepoint；父冲突跳过后代，子冲突不影响其他项。
- [ ] 普通冲突写 `recycle_item.error` 和服务端日志，HTTP 仍返回成功。
- [ ] 安全错误停止当前批次并返回统一错误。
- [ ] 所有 Item 恢复后删除 Item 与主记录；部分恢复更新状态和剩余数。
- [ ] 增加联合关系身份、部分恢复、重复恢复、并发恢复和租户隔离测试。

## Task 5：Controller、EPS 和菜单

- [ ] 创建 `modules/recycle/dto/data.go`，严格绑定 `ids` 且限制批量大小。
- [ ] 创建 `modules/recycle/controller/admin/data.go`，提供 Info、Page、Restore。
- [ ] 创建 `modules/recycle/register.go`，注册模型、Runtime、Controller、Schedule 和 Seed。
- [ ] EPS 只暴露 `info`、`page`、`restore`，不暴露 Add/Update/Delete。
- [ ] 从 `modules/base/menu.json` 精确迁移数据回收站菜单到 `modules/recycle/menu.json`，保持 ID、Router、ViewPath 和权限不变。
- [ ] 在 `modules/modules.go` 注册 Recycle 模块。
- [ ] 增加路由、权限、EPS、Node 响应字段和菜单 Seed 测试。

## Task 6：Schedule 和保留期

- [ ] 创建 `modules/recycle/schedule/data.go`，按本地时区每日执行清理。
- [ ] 读取 `base_sys_conf.recycleKeep`；`0` 不清理，非法值只记录错误。
- [ ] 使用 MySQL 命名锁保证多实例单执行者。
- [ ] 使用 Recycle Bypass 按小事务先删 Item 再删主记录。
- [ ] Runtime Stop 停止新任务并等待当前批次完成。
- [ ] 增加 0/正数/非法值、过期边界、命名锁和优雅停止测试。

## Task 7：Base 删除接入

- [ ] 参数：归档并删除 `base_sys_param`。
- [ ] 用户：同批归档用户和 `base_sys_user_role`，提交后撤销 Session。
- [ ] 角色：同批归档角色、用户角色、角色菜单、角色部门关系，保持保护规则和 Session 撤销。
- [ ] 菜单：同批归档选中菜单、全部后代和角色菜单关系，保持递归与租户规则。
- [ ] 部门 `deleteUser=false`：归档部门和角色部门关系，用户按现有逻辑迁移且不归档。
- [ ] 部门 `deleteUser=true`：用户及用户角色加入部门批次，提交后撤销 Session。
- [ ] Base 日志 `/clear` 使用 Bypass，不生成回收记录。
- [ ] 注册 Base 用户、角色、菜单、部门恢复 Hook。
- [ ] 扩展 Base 单元和 MySQL 关系作用域测试。

## Task 8：Dict 删除接入

- [ ] Dict 类型：每个类型建立独立分支，类型为根，全部 `dict_info` 为子项。
- [ ] Dict 信息：选中项和递归后代建立依赖树并同批归档。
- [ ] 保持公开字典读取、GlobalOnly 和租户作用域不变。
- [ ] 测试完整恢复、父冲突、单子项冲突、多类型独立分支和跨租户拒绝。

## Task 9：Task 删除接入

- [ ] 调整 Task Store Delete 接受 Manager 提供的 TX，避免嵌套事务。
- [ ] 同批归档 `task_info` 和该任务全部 `task_log`。
- [ ] 数据库提交后移除 Scheduler 计划。
- [ ] 恢复 Hook 通知 Engine 对账；停止任务不自动启动。
- [ ] Task 日志保留期清理使用 Bypass。
- [ ] 在当前未提交 Task 修改基础上增量实现并保留其行为。
- [ ] 扩展 Task 单元、MySQL 集成和现有 Redis 测试。

## Task 10：静态删除审计

- [ ] 扩展现有 Service 数据库入口审计或新增 Recycle 删除审计测试。
- [ ] 分类 `base/dict/task` 的直接 Delete 和原始 DELETE SQL：Manager 回调、Bypass 或测试清理。
- [ ] 新增未分类删除入口时测试失败。
- [ ] 审计不误报 Session、缓存、文件和明确的关系替换操作。

## Task 11：综合验收

- [ ] 运行 `gofmt` 和 `git diff --check`。
- [ ] 运行所有 Recycle/Core/Base/Dict/Task 聚焦单元测试。
- [ ] 运行 `env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1`。
- [ ] 运行 `env GOCACHE=/private/tmp/cool-admin-go-build COOL_RECYCLE_INTEGRATION=1 go test -p=1 ./... -count=1`。
- [ ] 运行受影响的现有 Base、Dict、Task 显式 MySQL 集成测试。
- [ ] 在 Redis 可用时运行现有 Task Redis 集成测试；不可用时明确记录边界。
- [ ] 运行 `go vet ./...`。
- [ ] 核对 Git diff，确认没有回退用户原有 Task/EPS 修改，也没有修改 Vue 和 Midway 项目。
- [ ] 更新设计和计划状态，提交 Recycle 实现。

## 验收输出

- 实现文件与关键架构说明。
- 测试命令及通过结果。
- 未执行的外部依赖测试及原因。
- 配置和数据库迁移注意事项。
- 用户原有未提交修改的保护说明。

## 实施结果

- Recycle 核心、模块目录、配置、菜单、定时清理和静态删除审计已接入。
- Base、Dict、Task 自定义删除已按聚合边界归档；默认 CRUD 只归档根实体。
- 恢复冲突静默保留，父冲突跳过后代；租户快照恢复前 fail-closed 校验。
- 过期清理使用专用 MySQL 连接持有命名锁，每批独立事务提交，`recycleKeep=0` 永不清理。
- `go test ./... -count=1`、`go vet ./...`、Dict/Base/Task MySQL 集成和 Task Redis 集成全部通过。
