# 回收站模块迁移设计

> 日期：2026-09-02  
> 状态：待实现  
> 源模块：`cool-admin-midway/src/modules/recycle`  
> 目标模块：`cool-admin-go-next/modules/recycle`

## 1. 目标

在 `cool-admin-go-next` 中补齐回收站业务模块，使现有 `cool-admin-vue` 无需修改即可查询、查看和恢复 Go 后端归档的数据，并按保留期清理过期回收记录。

本次实现遵循框架架构文档 §4.7 已确定的边界：`cool-next/db/recycle` 独占 `cool_recycle` 内部表、归档事务和恢复事务；业务模块只负责接口、权限、展示适配和清理调度。Node 版提供外部行为基准，但不照搬其 `recycle_data` 表和异步归档事件。

## 2. 现状与根因

当前框架层已经实现：

1. `cool.crud.softDelete=true` 时，同一事务内读取快照、写入 `cool_recycle` 并物理删除业务记录；
2. 快照按主键稳定排序并保持强类型数据精度；
3. 恢复时锁定回收记录，普通 `INSERT` 全量写回后删除回收记录；
4. 主键冲突、字段不兼容和并发恢复均整体回滚；
5. `source/params/operatorType/operatorId` 的存储与敏感键过滤能力。

缺口不是归档算法，而是 `modules/recycle` 尚未迁移：当前没有 `/admin/recycle/data/page`、`/info`、`/restore`，没有过期清理生命周期，HTTP 删除请求也没有把已验证身份和脱敏来源写入 `recycle.Audit`。因此前端菜单已经存在，但没有对应服务；真实归档记录的审计字段始终为空。

## 3. 设计原则

1. 不新增 `recycle_data` 或其他业务回收表；唯一事实来源是 `cool_recycle`。
2. 不把 `cool_recycle` 注册成普通业务 Entity，避免重复 Schema、Descriptor 注册以及回收自身。
3. 不在业务模块重写快照解析、事务恢复或并发控制，统一调用核心 Store。
4. 对外路径、请求和前端所需字段与 Node 版保持兼容；内部字段仍使用 Go 框架契约。
5. 不增加第三方依赖，查询、调度和时间处理复用 GoFrame 与标准库。

## 4. 模块结构

```text
modules/recycle/
├── config.go
├── dto/
│   └── data.go
├── service/
│   └── data.go
├── controller/
│   └── admin/
│       └── data.go
└── schedule/
    └── data_job.go
```

模块元信息与 Node 保持一致：

- 名称：`数据回收`；
- 描述：`收集被删除的数据，管理和恢复`；
- 加载顺序：`0`；
- 清理表达式默认 `@daily`；
- 单次清理超时默认 `30m`。

模块不包含 `entity/` 和 `db.json`。`cool_recycle` 的结构由 `cool-next/db/recycle` 内部 Descriptor 管理，`recycleKeep=31` 已存在于 `modules/base/db.json`。

## 5. 核心 Store 扩展

在 `cool-next/db/recycle` 增加业务模块所需的最小公开能力：

1. 按 ID 查询单条回收记录；
2. 按页查询回收记录，支持来源、操作人 ID 集合的过滤以及受控排序；
3. 物理删除指定截止时间前的回收记录；
4. 判断记录不存在，使业务层批量恢复可以保持 Node 的幂等跳过语义。

查询和清理始终限定 `cool_recycle`，排序字段使用固定白名单 `id/createTime/updateTime/count`，不接受任意列名。清理直接操作内部表，不经过 CRUD Delete，因此不会为回收记录再次创建回收记录。

`Store.Restore` 的既有事务契约保持不变。业务层只逐个调用它，不解析和写入快照。

## 6. HTTP 契约

前缀固定为 `/admin/recycle/data`：

| 路径 | 方法 | 权限 | 行为 |
| --- | --- | --- | --- |
| `/page` | POST | `recycle:data:page` | 分页查询回收记录 |
| `/info` | GET | `recycle:data:info` | 查询单条回收记录 |
| `/restore` | POST | `recycle:data:restore` | 恢复一个或多个回收批次 |

三条路由均要求后台身份，不声明 `ignoreToken`。权限由现有路径推导规则生成，并与 `modules/base/menu.json` 已有权限完全一致。

### 6.1 请求

分页请求接受现有前端发送的字段：

```json
{
   "page": 1,
   "size": 15,
   "keyWord": "用户或接口关键字",
   "order": "count",
   "sort": "desc"
}
```

`page/size` 缺省时分别取 `1/15`，`size` 不得超过全局 CRUD `pageLimit`。`order` 只允许 `id/createTime/updateTime/count`，`sort` 只允许 `asc/desc`，两者必须同时出现。

详情请求为 `GET /info?id=<uint64>`。恢复请求为：

```json
{
   "ids": [1, 2]
}
```

`ids` 必填、去重且最多 500 项；零值拒绝。已不存在的回收记录幂等跳过。每条记录沿用 Store 的独立原子恢复事务；前序恢复成功而后序记录冲突时，保留前序结果并返回当前错误，与 Node 逐条恢复行为一致。

### 6.2 响应适配

分页响应保持 `{ list, pagination }`。列表和详情项提供：

| 外部字段 | 来源 |
| --- | --- |
| `id/createTime/updateTime/count/data` | `cool_recycle` 同名字段 |
| `url` | `source` |
| `params` | 已脱敏的 `params`；可解析查询串转换为对象，否则返回空对象 |
| `userId` | `operatorType=admin` 时解析 `operatorId` |
| `userName` | 按 `userId` 批量查询 `base_sys_user.name` |
| `entityInfo.dataSourceName` | `databaseGroup` |
| `entityInfo.entity` | `tableName` |

同时保留 `databaseGroup/tableName/operatorType/operatorId`，便于 Go 客户端识别真实目标；现有前端会忽略额外字段。`data` 以 `json.RawMessage` 返回，不先解码为 `map[string]any`，避免大整数精度损失，也不修改可恢复快照。

关键字匹配 `source` 和后台用户名称。服务先从 `base_sys_user` 查询匹配用户 ID，再把 ID 集合作为受控条件交给 Store；不在核心回收包引入 Base 业务表依赖。

## 7. HTTP 删除审计

认证成功后、进入 Handler 前，HTTP Adapter 根据当前请求构造 `recycle.Audit` 并替换请求 Context：

1. `source` 取请求路径，查询串由 `NewAudit` 移除；
2. `params` 只收集 URL 查询参数，并继续经过敏感键过滤；不保存 Authorization、Cookie 或未经结构化脱敏的完整 JSON 请求体；
3. 后台身份写入 `operatorType=admin` 和十进制 `operatorId`；
4. 应用端身份写入 `operatorType=app` 和十进制 `operatorId`；
5. 公开路由或无身份内部调用允许审计字段为空，不能因此跳过归档。

审计接入放在 `cool-next/core/gnhttp`，因为架构文档明确由 HTTP Adapter 提供协议来源；业务 Service 和核心 Store 均不依赖 `ghttp.Request`。

## 8. 过期清理

`DataService.ClearExpired` 从现有 `base.ConfService` 读取 `recycleKeep`：

- 值必须是大于 0 的整数天数；
- 配置不存在或非法时返回错误并保留全部数据，不采用 Node 版“缺配置即清空”的危险行为；
- 截止时间取本地当天零点向前推保留天数；
- 删除条件为 `createTime < cutoff`。

`DataJob` 使用项目已有 `gcron.Cron.AddSingleton` 注册每日任务，防止单进程重入；任务使用独立超时 Context，记录删除数量与耗时。`OnStop` 移除任务并等待正在运行的清理结束。跨实例不增加新的分布式锁：相同条件的幂等物理删除已经足够，额外锁不会改善正确性。

## 9. 错误处理

1. 请求字段错误返回 `exception.Validate`；
2. 回收记录不存在：详情返回 `null`，批量恢复跳过；
3. 快照冲突、目标表缺失、字段不兼容或数据库错误原样保留核心堆栈并补充业务上下文；
4. 恢复失败时核心事务回滚并保留对应回收记录；
5. 清理配置或数据库失败只记录任务错误，不清空、不重试死循环，也不影响应用存活。

## 10. 测试与验收

至少覆盖：

1. Store 单条查询、分页、关键字/操作人过滤、排序白名单和过期清理；
2. Service 字段映射、用户名称批量补全、JSON 快照精度和 `params` 转换；
3. 批量恢复的去重、缺失跳过、冲突保留和成功移除；
4. HTTP 审计写入后台/应用身份、移除查询串中的敏感字段，以及无身份兼容；
5. 清理保留天数、当天零点边界、非法配置保护和生命周期幂等；
6. 路由方法、路径、权限与 EPS 契约；
7. `cool generate` 后静态装配包含 recycle Service、Controller 和 Schedule；
8. `go test ./...`、`go vet ./...` 和修改文件 `gofmt` 检查通过。

## 11. 非目标

1. 不修改前端页面；
2. 不增加手动清空接口或彻底删除按钮；
3. 不支持跨数据库组恢复；
4. 不支持修改快照、部分恢复或 Upsert 恢复；
5. 不引入新的队列、缓存、锁服务或第三方依赖；
6. 不迁移旧 Go v1 的 `recycle_item`、部分恢复状态、多租户和 MySQL 专用锁设计。

## 12. 完成标准

现有回收站前端可以直接调用 Go 后端完成分页、详情和批量恢复；删除生成的记录带有可用且脱敏的来源和操作者信息；过期数据按 `recycleKeep` 每日清理；所有恢复继续满足框架 §4.7 的强类型、原子性和并发安全约束，且系统中只有 `cool_recycle` 一套回收数据。
