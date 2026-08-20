# cool-admin-go-next Recycle 模块设计

- 日期：2026-07-28
- 项目：`cool-admin-go-next`
- 对照实现：`cool-admin-midway` 8.x
- 目录规范：`cool-admin-midway/.cursor/rules/module.mdc`
- 状态：已批准，待实施

## 1. 背景

Go 版已具备模块注册、模型元数据、Schema Sync、Seed、通用 CRUD、租户作用域、认证权限和 Task 运行时，但当前所有删除最终都是物理删除。`base_sys_conf` 已预置 `recycleKeep`，Base 菜单也已包含数据回收站入口，后端却没有对应模块和统一删除生命周期。

Midway Recycle 的外部协议简单：删除前保存实体快照，随后物理删除原数据；回收站提供分页、详情和恢复接口。但 Node 实现存在以下缺陷：

1. `softDelete(ids)` 未等待完成，归档失败不能阻止物理删除。
2. 归档、删除和恢复不在统一事务中。
3. 自定义 Service 的直接 Repository 删除容易绕过回收站。
4. 关联数据通常直接删除，恢复主实体后关联数据永久丢失。
5. `save()` 恢复可能覆盖已占用原主键的当前记录。
6. 缺少明确的租户、模型白名单、依赖顺序和并发恢复边界。

本设计保持 Node 的接口、字段、配置键和“快照归档后物理删除”语义，但使用同步事务协调器修复一致性问题。明确不采用 GoFrame `deleted_at` Soft Time 逻辑删除。

## 2. 目标

1. 现有 Vue Recycle 页面无需修改即可分页、查看和恢复数据。
2. `cool.crud.softDelete=true` 时，业务删除必须先成功归档，再物理删除。
3. 通用 CRUD 默认只归档当前主实体，不自动推断表关系。
4. 自定义 Service 可以显式把关联数据加入同一删除批次并声明恢复依赖。
5. 覆盖当前 `base`、`dict` 和 `task` 的全部管理端删除路径。
6. 归档、业务删除和关系删除处于同一 MySQL 事务。
7. 恢复不覆盖当前数据；普通冲突静默跳过并保留归档项。
8. 所有查询、归档、恢复和清理保持租户隔离。
9. 内部维护和自动清理删除必须显式绕过 Recycle。
10. 模块目录严格遵循 `module.mdc` 的 Controller、DTO、Entity、Event、Schedule、Service 和 Config 职责。

## 3. 非目标

1. 不增加 `deleted_at`，不启用 GoFrame Soft Time。
2. 不使用数据库 Trigger、Binlog 或异步消息捕获删除。
3. 不自动扫描外键或猜测聚合关系。
4. 不恢复已经撤销的登录 Session。
5. 不自动逆转部门删除过程中已经完成的用户迁移。
6. 不兼容任意数据源；第一阶段只恢复当前 GoFrame `default` MySQL 数据源。
7. 不修改现有 Vue Recycle 页面交互。
8. 不对归档 JSON 做脱敏；这是经确认保留的 Node 兼容行为。

## 4. 核心决策

### 4.1 快照归档后物理删除

删除流程保留 Node 语义：读取完整记录快照，写入回收表，再物理删除原数据。归档记录是恢复事实来源，原业务表不保留删除标记。

该选择避免软删除记录继续占用唯一值，也避免为所有现有表增加 `deleted_at` 和修改全部查询条件。

### 4.2 同步事务协调器

不采用 Node 的异步删除事件。`cool/recycle.Manager` 统一拥有删除事务，归档与业务删除共享同一个 `gdb.TX`。任何阶段失败都会回滚归档和业务变更。

### 4.3 默认主实体，自定义关联批次

通用 CRUD 只归档当前资源。关联数据是否删除、迁移、禁止删除或加入同一回收批次，由业务 Service 显式决定。

例如：

- 删除 Dict 类型时，`DictTypeService` 把类型和全部 Dict 信息放入同一批次。
- 删除部门且 `deleteUser=false` 时，用户只迁移，不归档。
- 删除部门且 `deleteUser=true` 时，用户和用户角色关系加入部门批次。
- 删除文件分类时，未来 Space Service 可以选择迁移文件，而不是由框架自动删除。

### 4.4 依赖感知的部分恢复

恢复不覆盖已存在数据。主键、唯一键或可兼容结构冲突时，静默跳过冲突分支，恢复其他独立分支。冲突项继续保留，允许后续重试。

如果父项冲突，其后代不尝试恢复；子项冲突不撤销已经恢复的父项和其他独立子项。

### 4.5 内部删除显式绕过

Base 日志、Task 日志、Recycle 过期数据、Seed、测试清理、Session 和临时数据不进入回收站。调用方必须使用 `recycle.WithBypass(ctx)` 表达该意图。

缺少 HTTP 操作人上下文且没有 Bypass 的删除失败关闭，防止新增内部调用静默绕过审计。

## 5. 配置

配置键与 Midway `src/config/config.default.ts` 保持一致：

```yaml
cool:
  crud:
    # 是否开启删除数据回收记录。
    softDelete: true
```

语义：

- `true`：先归档，再物理删除；默认值为 `true`。
- `false`：按原 Service 逻辑直接物理删除，不新增回收记录。
- 配置在应用启动时读取，修改后重启生效。
- 关闭开关不隐藏 Recycle 模块，已有记录仍可查看、恢复和过期清理。
- 自定义 Service 不读取配置，由 Manager 统一决定是否创建归档。
- 开关开启但 Recycle Store 无法构建时，应用启动失败，不能降级成直接删除。

保留期继续使用 `base_sys_conf.recycleKeep`：

- 正整数表示保留天数。
- `0` 表示永久保留，不自动清理。
- 配置缺失、负数或格式错误时跳过清理并记录错误。
- 修改后在下一次每日清理时生效。

## 6. 目录与所有权

### 6.1 通用框架

```text
cool/recycle/
├── manager.go
├── batch.go
├── store.go
├── restore.go
└── context.go
```

职责：

- `manager.go`：配置判断、事务边界、归档删除协调和提交后动作。
- `batch.go`：删除批次、归档项、依赖关系、拓扑排序和安全上限。
- `store.go`：模块 Store 接口，不依赖 `modules/recycle`。
- `restore.go`：依赖感知恢复、Savepoint 和冲突分类。
- `context.go`：HTTP 删除元数据和显式 Bypass。

### 6.2 业务模块

```text
modules/recycle/
├── controller/
│   └── admin/
│       └── data.go
├── dto/
│   └── data.go
├── entity/
│   ├── data.go
│   ├── item.go
│   └── models.go
├── event/
│   └── data.go
├── schedule/
│   └── data.go
├── service/
│   └── data.go
├── config.go
├── register.go
├── db.json
└── menu.json
```

职责：

- Controller 只负责 Node 兼容路由与 DTO 绑定。
- DTO 定义 Restore 请求。
- Entity 定义 `recycle_data` 和 `recycle_item`。
- Event 是同步 Store 适配器，不启动异步归档任务。
- Schedule 负责每日清理和生命周期停止。
- Service 负责分页、详情、恢复和清理。
- `config.go` 读取并校验 Recycle 配置。
- `register.go` 是 Go 模块注册入口。
- `menu.json` 接管当前 Base Seed 中的数据回收站菜单，避免模块菜单继续混在 `base/menu.json`。

不创建空的 App 或 Middleware 目录。`base`、`dict`、`task` 只依赖 `cool/recycle`，不导入 `modules/recycle`。

## 7. 框架接线

Recycle 模块 Runtime 构建 Store 和 Manager。Application 在构建所有 Controller 与 CRUD Runtime 前取得该 Manager，并通过依赖对象注入各模块。

需要扩展现有注册依赖：

- `registry.RuntimeDeps` 可获得冻结后的模型列表或模型注册表。
- `registry.Deps` 和需要自定义删除的模块构造参数获得 `*recycle.Manager`。
- `crud.Runtime` 构造函数获得 Manager。
- Application 只接受一个 Recycle Provider；重复 Provider 导致启动失败。

模块之间不通过包级全局变量共享 Manager。测试可以注入假 Store、假时钟和关闭状态的 Manager。

应用顺序：

```text
编译模块和模型
→ Schema Sync
→ DB/Menu Seed
→ 构建 Recycle Store 和 Manager
→ 注入 CRUD 与各模块 Service
→ 启动 Recycle Schedule
→ 启动 HTTP Server
```

关闭时先停止 Schedule 接收新清理任务，等待当前清理批次结束，再释放模块 Runtime。

## 8. 数据模型

### 8.1 recycle_data

`recycle_data` 表示一次用户删除操作，保持 Node 前端字段：

| 字段 | 类型 | 语义 |
| --- | --- | --- |
| `id` | bigint | 批次 ID |
| `entity_info` | json | 根资源的数据源名、实体名和稳定资源名 |
| `user_id` | bigint nullable | 删除操作人 |
| `data` | json | 根实体的完整原始记录数组，不脱敏 |
| `url` | varchar nullable | 删除请求 URL |
| `params` | json nullable | 删除请求参数 |
| `count` | int | 本批次原始删除总记录数 |
| `restore_status` | varchar | `pending` 或 `partial` |
| `remaining_count` | int | 尚未成功恢复的 Item 数量 |
| `create_time` | varchar | 删除时间 |
| `update_time` | varchar | 最近恢复尝试时间 |
| `tenant_id` | bigint nullable | 租户归属 |

索引至少包含：

- `(create_time, id)`
- `(tenant_id, create_time)`
- `(tenant_id, user_id)`
- `user_id`

`entityInfo` 对外继续包含 Node 字段：

```json
{
  "dataSourceName": "default",
  "entity": "DictTypeEntity",
  "resource": "dict.type"
}
```

恢复使用稳定 `resource` 查找冻结模型，不能使用归档中的任意表名直接执行 SQL。

### 8.2 recycle_item

`recycle_item` 表示一条可恢复记录：

| 字段 | 类型 | 语义 |
| --- | --- | --- |
| `id` | bigint | Item ID |
| `recycle_id` | bigint | 所属回收批次 |
| `resource` | varchar | 冻结模型资源名 |
| `table_name` | varchar | 审计用表名，必须与模型一致 |
| `primary_key` | json | 原记录身份；单主键或联合唯一键的有序字段和值 |
| `data` | json | 完整单行快照 |
| `branch_key` | varchar | 独立恢复分支 |
| `parent_item_id` | bigint nullable | 父依赖 Item |
| `restore_order` | int | 同层稳定恢复顺序 |
| `status` | varchar | `pending`、`restored`、`conflict` |
| `error` | text nullable | 最近冲突原因，仅内部可见 |
| `create_time` | varchar | 创建时间 |
| `update_time` | varchar | 状态更新时间 |
| `tenant_id` | bigint nullable | 租户归属 |

索引至少包含：

- `(recycle_id, status, restore_order)`
- `(recycle_id, branch_key)`
- `(tenant_id, recycle_id)`

`recycle_data.data` 是 Node 页面展示所需的根记录快照；`recycle_item` 是恢复事实来源。主记录在部分恢复期间保持原始审计内容不变，Item 状态和 `remaining_count` 表达恢复进度。

JSON 解码必须使用无损数字语义，不能把 MySQL `bigint` 经 `float64` 转换后再恢复。

Item 身份按以下顺序编译：

1. 模型存在主键时使用主键。
2. 没有主键但恰好存在可用联合唯一索引时，使用该索引的有序字段和值。
3. 存在多个候选唯一索引或没有稳定身份时，Service 必须显式指定身份字段；否则该模型不能归档恢复。

`base_sys_user_role`、`base_sys_role_menu` 和 `base_sys_role_department` 使用各自现有联合唯一索引作为身份。恢复前按完整身份检查冲突，不把关系表错误地当成带 `id` 的实体。

## 9. 删除批次模型

一个 Batch 包含：

- 删除操作元数据：用户、租户、URL、方法和请求参数。
- 根资源和根 ID。
- 一个或多个独立 `branchKey`。
- 每个分支内的父子依赖。
- 提交后动作。

批量删除多个 Dict 类型时，每个类型形成独立分支。类型 Item 是分支根，对应 Dict 信息以类型 Item 为父依赖。一个类型恢复冲突时，不影响另一个类型分支。

单次根实体数量沿用 `crud.MaxBatchSize=500`。主实体与关联数据总 Item 超过 `10000` 时拒绝删除，防止超大事务和归档载荷耗尽内存。关联数据分块读取和批量写入，但仍处于同一个事务。

依赖图必须满足：

- Item 只能依赖同一 Batch、同一租户的 Item。
- 不允许环。
- 父项恢复顺序必须早于子项。
- 分支根必须属于根资源或由 Service 显式声明。
- 每个 Item 的模型和字段必须通过冻结模型注册表校验。

## 10. Manager 与 Service 契约

Manager 统一提供删除事务，概念调用为：

```text
Manager.RunDelete(ctx, rootResource, rootIDs, callback)
```

Manager 固定负责：

1. 读取启动期冻结的 `softDelete` 开关。
2. 校验 HTTP 操作人、租户和 Bypass。
3. 开启事务。
4. 锁定并归档根实体。
5. 调用 Service 回调。
6. 校验批次和影响行数。
7. 在同一事务写入 `recycle_data/recycle_item`。
8. 提交事务。
9. 运行提交后动作。

Service 回调负责：

1. 使用 Manager 提供的 `gdb.TX` 查询和锁定关联数据。
2. 把需要恢复的关联记录追加到 Batch。
3. 声明分支、父依赖和恢复顺序。
4. 按业务规则删除、迁移或拒绝操作。
5. 返回实际删除数量和提交后动作。

当 `softDelete=false` 或 Context 带 Bypass 时，Manager 仍提供同一事务和回调，但不读取快照、不写回收表。Service 始终只有一套业务删除代码。

自定义 Service 不再在 Manager 外部开启嵌套事务。已经拥有内部 Store 事务的 Task 删除需要改为接受 Manager 提供的 TX。

## 11. 删除流程

业务删除顺序：

1. Controller 规范化 ID 和删除附加参数。
2. HTTP 层把用户、租户、URL、方法和原请求参数放入 Context。
3. Manager 校验作用域和配置。
4. 事务内按主键稳定排序锁定根记录。
5. Manager 保存根快照并创建分支。
6. 自定义 Service 锁定、归档并删除关联记录，或迁移引用记录。
7. 删除主记录。
8. 校验锁定数、归档数和实际删除数一致。
9. 写回收批次和 Item。
10. 提交事务。
11. 运行 Session 撤销、权限刷新或 Task Engine 移除等提交后动作。

归档写入和业务删除任一步失败都会回滚。不存在“请求失败但删除已提交”或“数据删除但归档缺失”的成功路径。

## 12. 当前模块覆盖矩阵

| 删除入口 | 根记录 | 同批关联记录 | 不归档动作 |
| --- | --- | --- | --- |
| Base 参数 | `base_sys_param` | 无 | 无 |
| Base 用户 | `base_sys_user` | `base_sys_user_role` | Session 撤销 |
| Base 角色 | `base_sys_role` | `base_sys_user_role`、`base_sys_role_menu`、`base_sys_role_department` | Session 撤销 |
| Base 菜单 | 选中菜单 | 后代菜单、`base_sys_role_menu` | 权限状态刷新 |
| Base 部门 | 选中部门 | `base_sys_role_department` | `deleteUser=false` 时迁移用户 |
| Base 部门删除用户 | 部门 | 用户、用户角色、角色部门关系 | Session 撤销 |
| Dict 类型 | `dict_type` | 该类型全部 `dict_info` | 无 |
| Dict 信息 | 选中信息 | 全部后代 `dict_info` | 无 |
| Task 任务 | `task_info` | 该任务全部 `task_log` | Engine 移除计划 |

部门保持现有 `deleteUser` 协议：

- `deleteUser=false`：用户迁移到现有 Service 选择的顶级部门；没有可用部门则置空。用户不归档，恢复部门也不自动迁回用户。
- `deleteUser=true`：用户和用户角色关系进入同一部门批次，恢复时按依赖顺序恢复。

权限、Session 和外部调度状态不是归档数据。数据库恢复成功后由资源 Hook 对账，历史 Session 不恢复。

## 13. 明确绕过的删除

以下路径必须使用 `recycle.WithBypass(ctx)`：

- Base 日志 `/clear` 和未来日志保留期任务。
- Task 日志保留期清理。
- Recycle 过期清理。
- Schema/Seed/Test 清理。
- Session、缓存、临时文件和队列运行态清理。
- 业务事务中明确不需要独立恢复的临时关系维护。

Bypass 是类型化 Context 状态，不接受 HTTP 参数控制。管理端客户端不能提交 `bypass=true` 绕过归档。

## 14. 恢复流程

接口保持 Node 格式：

```http
POST /admin/recycle/data/restore
Content-Type: application/json

{"ids":[1,2]}
```

每个 Recycle 批次独立执行恢复事务：

1. 按当前租户锁定 `recycle_data`。
2. 加载该批次全部 Item 和依赖图；只对 `pending/conflict` Item 执行恢复，`restored` Item 用于验证父依赖和幂等状态。
3. 校验数据源、模型、字段、租户和依赖图。
4. 按拓扑顺序和 `restore_order` 处理分支。
5. 每个分支使用 MySQL Savepoint 隔离普通冲突。
6. 使用显式 `INSERT` 恢复原主键和原字段值，不使用 Upsert 或 Save。
7. 成功项标记 `restored`，普通冲突项标记 `conflict`。
8. 更新 `restore_status`、`remaining_count` 和 Item 内部错误。
9. 所有 Item 成功后删除整个回收批次。
10. 提交后执行资源恢复 Hook。

恢复模型只能来自应用启动时冻结的模型注册表。归档表名只用于一致性校验，不参与未经验证的 SQL 拼接。

### 14.1 冲突语义

普通冲突包括：

- 原主键已存在。
- 唯一索引冲突。
- 当前模型新增必填字段且归档无法满足。
- 归档字段发生可识别但不可自动迁移的结构漂移。

处理规则：

- 不覆盖当前数据。
- 父 Item 冲突时静默跳过其后代。
- 子 Item 冲突不影响已恢复父项和其他独立子项。
- 接口不返回冲突列表或原因，不抛普通冲突业务错误。
- HTTP 返回既有成功包络，现有 Vue 继续显示“数据恢复成功”。
- 冲突项保留在回收站并允许后续重试。
- 冲突原因只保存在 `recycle_item.error` 和服务端日志。

归档结构损坏、依赖成环、租户不一致或模型白名单校验失败属于安全错误，不作为普通冲突静默处理；该批次停止恢复并返回错误。

### 14.2 并发和幂等

- `SELECT ... FOR UPDATE` 锁定 Recycle 主记录，串行化同一批次恢复。
- `restored` Item 不重复插入。
- 重复请求已完全恢复并删除的批次按不存在处理，不影响其他批次。
- Savepoint 回滚只撤销当前分支尝试，不撤销之前成功的独立分支。

## 15. 恢复 Hook

恢复 Hook 按稳定资源名注册并在应用构建期冻结。重复名称和未知资源导致启动失败。

第一阶段 Hook：

- `task.info`：提交后通知 Engine 全量或单任务对账；已停止任务保持停止。
- `base.user`、`base.role`、`base.menu`：刷新权限相关状态；不创建或恢复 Session。
- `base.department`：刷新部门权限关系；不逆转删除后的用户迁移。
- `dict.type`、`dict.info`、`base.param`：无需外部副作用。

Hook 在数据库恢复提交后运行，因此 Hook 失败不能回滚已提交数据。失败写日志，由现有 Engine 或后续请求对账修复。

## 16. HTTP、权限与 EPS

必须提供：

```text
GET  /admin/recycle/data/info
POST /admin/recycle/data/page
POST /admin/recycle/data/restore
```

权限保持：

```text
recycle:data:info
recycle:data:page
recycle:data:restore
```

分页行为：

- 关键字匹配操作人名称和请求 URL。
- 默认按创建时间或 ID 倒序。
- 联表返回 `userName`。
- 返回 Node 兼容 `entityInfo`、`data`、`url`、`params`、`count` 和基础字段。
- `data` 和 `params` 返回完整 JSON，不脱敏。
- 不提供 Add、Update 或 Delete 接口。

Restore 使用既有成功响应包络，不返回普通冲突详情。配置关闭不会隐藏这些接口或菜单，因为历史归档仍需管理。

## 17. 保留期和清理

`schedule/data.go` 每日执行：

1. 读取 `recycleKeep`。
2. `0` 时直接结束。
3. 非法值时写错误日志并结束。
4. 正整数时计算本地时区下的保留边界。
5. 获取 MySQL 命名锁，未获得则说明其他实例正在清理，本实例跳过。
6. 使用 Bypass 按小批次删除过期 `recycle_item` 和 `recycle_data`。
7. 释放命名锁。

清理以整个 Recycle 批次的 `create_time` 为准。部分恢复且仍有冲突 Item 的批次到期后同样永久删除。由于原业务数据在删除时已经物理移除，过期清理只清除归档表，不再操作业务表。

应用关闭时 Schedule 停止接收新任务，并等待当前数据库批次完成。Context 取消会在当前安全边界后结束后续循环。

## 18. 错误处理

删除阶段失败关闭：

- 任一根 ID 不存在或不属于当前租户，整次删除失败。
- 快照序列化、归档写入或物理删除失败，事务回滚。
- 实际删除行数与归档行数不一致，事务回滚。
- 模型未知、字段不匹配、依赖成环或跨租户依赖，拒绝删除。
- 总 Item 超过安全上限，拒绝删除并提示拆分或先清理关联数据。

恢复阶段区分普通冲突和安全错误。普通冲突静默保留；安全错误停止批次并返回统一错误。

日志记录：

- Recycle ID、资源名、用户 ID、租户、数量和错误摘要。
- 不把完整快照再次写入服务端日志。
- 不输出拼接 SQL 或凭据。

## 19. 安全边界

1. 租户用户只能看到和恢复本租户批次；平台数据使用 `tenant_id IS NULL`。
2. 客户端提交的 `tenantId`、资源名、表名和 Bypass 均不可信。
3. 恢复前同时校验主记录、Item，以及目标模型存在时快照内的规范租户字段；无租户字段的关系表通过同批父实体和关系端点验证作用域。
4. 表名和字段名只能来自冻结模型元数据。
5. 所有值通过参数化 SQL 写入。
6. 完整归档 JSON 可能包含密码摘要、手机号等敏感字段；按已确认决策，仅由现有 Recycle 权限保护，不额外脱敏或增加二次权限。
7. Recycle Controller 不暴露新增、修改或删除归档记录的通用 CRUD。

## 20. 菜单和 Seed

当前 `modules/base/menu.json` 中的数据回收站菜单迁移到 `modules/recycle/menu.json`，保持以下值不变：

- Router：`/recycle/data`
- ViewPath：`modules/recycle/views/data.vue`
- 权限：`recycle:data:page`、`recycle:data:info`、`recycle:data:restore`

迁移后 Base Seed 不再拥有 Recycle 菜单，避免重复导入。`modules/recycle/db.json` 不预置业务归档数据，只保留合法模块 Seed 结构。

## 21. 测试设计

### 21.1 单元测试

- `softDelete` 开启、关闭和缺省为 `true`。
- Bypass 不生成回收记录。
- 缺少操作者且无 Bypass 时失败关闭。
- 快照、归档、删除和影响行数异常触发回滚。
- 批次分支、拓扑排序、环检测和安全上限。
- `bigint` JSON 无损编码与恢复。
- 父冲突传播、子冲突隔离和 Savepoint 行为。
- 普通冲突不进入 HTTP 响应。
- `recycleKeep=0`、正整数、缺失、负数和非法字符串。
- MySQL 命名锁单持有者语义。
- 配置关闭后历史记录仍可查询和恢复。

### 21.2 MySQL 集成测试

使用：

```text
COOL_RECYCLE_INTEGRATION=1
```

覆盖：

- Base 参数删除、分页、详情和恢复。
- 用户与用户角色关系恢复。
- 角色及三类关系恢复。
- 菜单树及角色菜单关系恢复。
- 部门用户迁移模式和 `deleteUser` 模式。
- Dict 类型与信息整批删除和恢复。
- Dict 多分支部分冲突，其他分支正常恢复。
- Task 与 Task Log 恢复，并触发 Engine 对账。
- 不同租户不能查看、恢复或影响对方归档。
- 并发删除、并发恢复和重复恢复。
- `softDelete=false` 直接物理删除。
- 日志、Task 日志和 Recycle 清理正确绕过。
- 归档写入失败时业务数据仍存在。

### 21.3 HTTP、权限和 EPS

- 路由、方法、权限标识和响应包络与 Node 一致。
- Page 返回操作人联表和完整 JSON。
- 无权限用户不能读取或恢复。
- 租户不能通过 ID 探测其他租户批次。
- EPS 中只暴露 Info、Page 和 Restore。
- 现有 Vue Recycle 页面无需修改。

### 21.4 静态删除审计

扩展现有数据库入口审计：`base`、`dict` 和 `task` 新增直接 Delete 或原始 `DELETE SQL` 时，必须满足以下之一：

- 位于 Manager 管理的删除回调中。
- 使用明确的 Recycle Bypass。
- 位于测试清理文件。

未分类的新增删除入口使测试失败，防止自定义 Service 再次漏接 Recycle。

## 22. 验收标准

1. `cool.crud.softDelete=true` 时，受管业务删除不存在“已删除但无归档”的成功状态。
2. `false` 时保留现有物理删除行为且不生成归档。
3. Dict 类型与信息可以作为同一批次完整恢复。
4. Base、Dict 和 Task 当前全部管理端删除路径完成分类和接入。
5. 普通恢复冲突不覆盖当前数据、不提示客户端，并保留冲突项。
6. 维护删除和定时清理不会产生回收记录。
7. 回收站严格按租户隔离。
8. `recycleKeep=0` 永不自动清理。
9. 现有 Vue Recycle 页面无需修改即可使用。
10. 普通 `go test ./...`、Recycle MySQL 集成测试及受影响模块集成测试全部通过。

## 23. 实施顺序

1. 实现 `cool/recycle` 类型、Context、批次和 Store 契约。
2. 实现 Recycle Entity、Store、Service、Controller 和模块注册。
3. 接入 Application、Registry 和通用 CRUD。
4. 接入 Base 自定义删除和恢复 Hook。
5. 接入 Dict 聚合删除。
6. 接入 Task 删除与 Engine 恢复 Hook。
7. 实现 Schedule、配置和多实例清理锁。
8. 迁移菜单 Seed，增加静态删除审计。
9. 完成单元、MySQL、HTTP、权限、EPS 和并发验收。
