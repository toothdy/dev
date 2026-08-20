# cool-admin-go-next 与 cool-admin-midway 差距分析

- 分析日期：2026-07-27
- Go 项目：`cool-admin-go-next`
- 对照基准：`cool-admin-midway` 8.x
- 分析范围：模块覆盖、基础设施、Base 协议、生产化能力与验证边界

## 1. 结论

`cool-admin-go-next` 已完成核心框架、`base`、`dict` 和 `task` 模块，但距离完整替代 `cool-admin-midway` 仍缺少较多业务模块与通用基础设施。

项目 README 中“只剩 Vue 前端联调”的描述仅适用于当前阶段范围，不表示已经与 Midway 全量对齐。当前 Go 版更准确的定位是：

1. 核心运行时已经建立，包括模块注册、模型元数据、Schema Sync、Seed、CRUD、认证、权限、中间件、统一响应和 EPS。
2. 后台管理的 `base` 主流程和 `dict` 模块已经具备较高完成度。
3. Midway 的其他业务模块、插件体系和事件机制尚未完整迁移；Task 队列使用 Go 专属 Asynq 协议，不与 Bull/BullMQ Redis 数据互通。

## 2. 已完成的核心能力

Go 版当前已经具备以下基础：

- GoFrame v2 应用启动和路由注册。
- 模块注册与加载顺序。
- 模型元数据和 MySQL Schema Sync。
- 模块 `db.json`、`menu.json` 初始化导入。
- 元数据驱动的通用 CRUD Runtime。
- Controller 构建、路由派生和权限映射。
- Node 兼容的响应包络和分页结构。
- 后台登录、验证码、Access Token、Refresh Token 和 Session。
- 用户、角色、菜单、部门、参数、日志等 Base 管理接口。
- 权限菜单、细粒度接口权限和 EPS。
- 请求日志、翻译和统一错误处理中间件。
- 本地文件上传。
- `dict_type`、`dict_info` 及字典自定义接口。
- Task 本地 Cron、Asynq/Redis 分布式队列、任务日志、租户隔离和应用 Runtime 生命周期。

当前实际注册的业务模块只有：

```text
modules/base
modules/dict
modules/task
```

注册入口位于 `modules/modules.go`。

## 3. 业务模块差距

| Midway 模块 | Go 当前状态 | 主要缺口 |
|---|---|---|
| `base` | 大部分完成 | 日志定时清理、AI Coding、云存储上传 |
| `dict` | 基本完成 | 仍需真实前端和数据库联调 |
| `user` | 完全缺失 | APP 用户、微信登录、短信、地址、独立用户 JWT |
| `plugin` | 完全缺失 | 插件安装、启停、Hook、动态实例调用 |
| `task` | 已完成 | 本地 Cron 与 Asynq/Redis 分布式队列已实现；不兼容 Bull/BullMQ Redis 数据格式 |
| `space` | 完全缺失 | 文件分类、文件资源管理、WPS 配置 |
| `recycle` | 完全缺失 | 删除记录、回收站查询和数据恢复 |
| `swagger` | 部分替代 | 有 GoFrame OpenAPI/Swagger，没有 Midway 自定义构建模块 |
| `demo` | 缺失 | 缓存、事务、队列、RPC、SSE 示例；非生产必需 |

## 4. 基础设施差距

### 4.1 通用多租户隔离

Midway 使用 TypeORM Subscriber 在查询、插入、更新和删除阶段自动加入租户约束。

Go 版已通过 `cool/tenant` 建立默认启用的通用租户作用域。包含规范 `tenant_id` 字段的 BaseFields-backed 模型会在启动期编译租户元数据，通用 CRUD 在结构化 SQL 规划阶段强制写入租户值并追加参数化谓词；平台数据使用 `tenant_id IS NULL`，legacy `tenant_id = 0` 仅用于迁移兼容。

运行时区分 Tenant、Platform、Bypass 和 Missing：缺失身份对租户资源失败关闭，平台身份保留经批准的跨租户管理语义，内部跨租户路径必须显式派生 Bypass。公开参数和字典读取使用 GlobalOnly，只返回平台数据，客户端提交的 `tenantId` 不能选择作用域。

自定义 GoFrame ORM 查询使用 scoped Model；复杂 JOIN 和 raw SQL 必须为每个租户表别名追加参数化谓词，或在同一事务中先验证主实体。AST guard 会扫描模块 Service 的直接数据库入口，新增入口必须重新审计。该能力已覆盖 Base 和 Dict，详细边界及验证矩阵见 `docs/tenant-scope.md`。

### 4.2 软删除与回收站

Midway 默认启用 `cool.crud.softDelete`，删除事件可以被 `recycle` 模块记录并恢复。

Go 版尚未形成统一的软删除字段、CRUD 删除语义、删除事件和回收数据结构，也没有 `recycle` 模块。该能力不能仅靠增加一个回收站 Controller 完成，需要先确定通用删除生命周期。

### 4.3 事件、任务与队列

Go 版尚未提供与以下 Midway 能力对等的通用机制：

- 进程内事件和跨进程全局事件。
- 通用服务启动生命周期事件；Task 已具备模块 Runtime 启停边界。
- RPC 和分布式事务示例能力。

当前模块注册结构主要覆盖模型、Controller、中间件和 Seed，还没有 Event、Job、Queue、Hook 等模块级扩展点。

### 4.4 通用缓存

现有验证码使用 GoFrame `gcache`，认证层提供 Memory/Redis Session Store，但尚未形成 Midway `cacheManager` 对等的通用缓存能力。

仍缺少：

- 统一缓存客户端配置和注入。
- 内存、文件、Redis 等后端切换。
- 业务 Service 通用缓存 API。
- 方法级缓存和失效策略。
- 权限缓存及角色、菜单变化后的刷新机制。

### 4.5 Session 生产接线

Redis Session Store 已经实现，但默认应用启动仍使用 Memory Session Store。多实例部署时会出现登录状态不一致、注销只在单实例生效以及应用重启后全部登出等问题。

需要补充：

1. Redis 客户端配置。
2. 从运行配置构建 Redis Session Store。
3. 生产模式下禁止回退到内存 Session。
4. Redis 不可用时的启动或降级策略。

### 4.6 APP 用户认证域

Go 版认证中间件已经保护 `/admin/**` 和 `/app/**`，但当前没有 Midway `user` 模块对应的独立 APP 用户体系。

尚缺：

- APP 用户实体和上下文。
- 独立 JWT Secret、过期时间和 Refresh Token。
- 小程序、公众号、微信 APP 登录。
- 手机号、短信验证码和一键登录。
- 用户地址与个人资料接口。

因此，现有 `/app/base/**` 路由并不代表 APP 用户登录闭环已经完成。

## 5. Base 模块剩余差异

### 5.1 日志定时清理

Midway 通过每日 Job 按日志保留天数清理历史数据。Go 已实现日志保留天数配置接口，但没有对应的自动清理任务。

### 5.2 AI Coding

以下接口尚未实现：

```text
GET  /admin/base/coding/getModuleTree
POST /admin/base/coding/createCode
```

菜单 `/parse`、`export`、`import` 已有实现。远程 HTTP `/create` 因可以写入项目源码而被安全修复主动移除，不应为了机械兼容直接恢复；代码生成更适合迁移到离线 CLI 或受控 CI。

### 5.3 文件存储

当前仅支持本地上传，没有 Midway 插件体系提供的 OSS、COS、Qiniu 等云存储适配器，也没有 `space` 模块提供的文件资源管理能力。

### 5.4 Swagger

GoFrame 已配置：

```text
/api.json
/swagger
```

这可以满足基础 OpenAPI 浏览，但没有 Midway `swagger` 模块基于 Cool Controller、实体和运行时事件构建文档的完整行为。是否继续补齐，应以现有前端和接口文档消费需求为准。

### 5.5 有意保留的协议差异

以下差异属于安全增强，不建议为追求一致而回退：

- 用户列表不返回密码摘要。
- 平台超级管理员及全局 `admin` 角色受到额外保护。
- Refresh Token 轮换并校验服务端 Session。
- 授权变化后主动撤销相关 Session。
- 上传限制主动内容并增加静态资源安全响应头。
- 移除匿名原始 HTML 输出。
- 移除可以远程写入 Go 源码的菜单 `/create` 接口。

## 6. 生产化差距

当前 `manifest/config/config.yaml` 仍偏向开发环境：

- 数据库连接包含固定用户名和密码。
- 数据库 Debug 默认开启。
- Schema Auto Sync 默认开启。
- EPS 和 Swagger 默认开启。
- 缺少明确的生产环境覆盖文件和部署变量约束。
- 默认 Session Store 为内存实现。
- 初始化数据中仍保留兼容用已知管理员账号。

此外还需要关注：

1. Schema Sync 对已存在字段类型、Nullable、默认值、索引列顺序等漂移的校验仍不完整。
2. 权限校验主要在请求时查询数据库，缺少权限缓存，菜单规模增大后会增加数据库压力。
3. 真实 MySQL 集成测试依赖显式环境变量，普通 `go test ./...` 不会覆盖所有数据库行为。

## 7. 推荐实施顺序

### P0：保证核心边界

1. 在通用 CRUD/Repository 层实现默认租户作用域和显式绕过能力。
2. 定义统一软删除生命周期，为 `recycle` 模块提供基础。
3. 接入生产 Redis Session，并增加多实例约束。
4. 增加生产配置覆盖，关闭 Debug、自动同步、EPS 和 Swagger。
5. 完成 `base + dict` 与 Vue 前端、专用 MySQL 的真实联调。

### P1：补齐主要业务模块

建议按以下顺序实施：

```text
space -> user -> recycle -> plugin
```

- `space`：后台常见文件管理能力，且能承接现有本地上传。
- `user`：形成 APP、小程序和公众号用户闭环。
- `recycle`：依赖前置软删除生命周期。
- `plugin`：动态加载和安全边界最复杂，适合最后实施。

### P2：补齐扩展基础设施

1. 通用事件总线和应用生命周期事件。
2. 通用缓存客户端和权限缓存。
3. 通用任务监控；Task 已具备专用分布式队列和重试。
4. 云存储插件接口。
5. 按实际需要补充 RPC、SSE 和 Demo 示例。

## 8. 验证结果与边界

已执行：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
```

结果：全部通过。

本次没有执行依赖真实 MySQL 或 Redis 的显式集成测试，例如：

```text
COOL_AUTH_INTEGRATION=1
COOL_PERMISSION_INTEGRATION=1
COOL_CUSTOM_API_INTEGRATION=1
COOL_DICT_INTEGRATION=1
```

因此，单元测试通过只能证明当前无外部依赖测试集正常，不能代替完整的数据库、Redis、多租户和前端协议验收。

## 9. 分析方法说明

Midway 项目已有 `.codegraph` 索引，架构基线来自该索引和关键源码调用链。

`cool-admin-go-next` 当前没有 `.codegraph` 索引。按照 Codegraph 的使用约束，本次没有自动为项目初始化索引；Go 侧结论来自实际源码、模块注册、协议对照文档、安全审计和全量 Go 测试。
