# cool-admin-go-next

`cool-admin-go-next` 的 v2 重写：在 GoFrame v2 上重新实现 Cool Admin 框架。  
v2 替换 `cool-admin-go-next-v1` 的内部实现，对外遵循 Node 版 `cool-admin-midway` 的 HTTP/CRUD 行为与协议。

| 项 | 值 |
|---|---|
| 仓库位置 | 当前仓库根目录 |
| 远端仓库 | `https://github.com/toothdy/cool-admin-go-next` |
| 包路径 | `github.com/toothdy/cool-admin-go-next` |
| Go 模块 | 仓库根 `go.mod` |
| Go 版本 | Go 1.26.0 / toolchain 1.26.4 |
| 底层框架 | GoFrame v2（`gdb` / `ghttp` / `grpcx`） |
| 框架层顶级目录 | `cool-next/` |
| 命名约定 | 包名 = 目录短名（单词），Go 风格 |

---

## 角色与参考来源

| 来源 | 角色 | 约束 |
|---|---|---|
| `cool-admin-midway` | Node 行为参考 | HTTP 契约、CRUD 语义和鉴权行为的首要参考 |
| `cool-admin-midway-packages` | Node 框架公共 API | Controller、Service、Exception 等开发体验的首要参考 |
| `cool-admin-go-next-v1` | v1 实现 | 只用于识别遗漏与缺陷，禁止复制其 DSL、运行注册或自定义 SQL 编译器 |
| 本仓库 `docs/superpowers/specs/2026-07-31-cool-admin-go-next-architecture-design.md` | 唯一权威架构设计 | 一切实现以该文档为准 |

---

## 当前状态

仓库已进入 v2 实施阶段。应用入口、Application Host、HTTP/gRPC Transport、静态生成流水线和基础质量门禁已经可用；后续业务与异步能力仍按模块需求逐项交付。

未完成：

1. `modules/` 业务模块。
2. `cool outbox` 运维子命令。
3. Event、Schedule 和 Queue 专项能力。

`cool generate/check/build/run` 已可通过 `go run ./cmd/cool <command>` 执行。当前完整门禁以根 `Makefile` 为准。

---

## 完整目录结构

```text
cool-admin-go-next/
├── README.md                     ← 本文档
├── .gitignore
├── .github/                       ← CI workflow
├── go.mod                          ← module github.com/toothdy/cool-admin-go-next
├── go.sum
├── main.go                        ← 入口（app.Run 装配）
│
├── cool-next/                     ← 框架层顶级
│   │
│   ├── core/                      ← 协议无关核心 (11 子目录)
│   │   ├── controller/            ← Builder / Definition / CurdOption / Router
│   │   ├── entity/                ← Descriptor / Field / Index / DOValue
│   │   ├── service/               ← Base[E, ID] 泛型 + Hook 适配器
│   │   ├── exception/             ← Comm / Validate / Core 异常
│   │   ├── module/                ← Declaration / Declaration[T] / ComponentRef
│   │   ├── event/                 ← 进程内事件
│   │   ├── tag/                   ← URL Tag 注册
│   │   ├── configuration/         ← 启动配置入口
│   │   ├── app/                   ← Application Host / Transport 装配
│   │   ├── hooks/                 ← ModifyBefore / ModifyAfter 接口
│   │   └── route/                 ← 路由规范化
│   │
│   ├── auth/                      ← L1 鉴权
│   │   ├── bcrypt/                ← bcrypt 包装
│   │   ├── jwt/                   ← JWT
│   │   ├── session/               ← SessionStore (Redis/Memory)
│   │   ├── context.go             ← Admin / App 已验证身份访问
│   │   └── middleware.go          ← Security 中间件
│   │
│   ├── crud/                      ← L3 协议无关 CRUD runtime
│   │   ├── types.go               ← Action / ActionPlan / QueryPlan
│   │   ├── metadata.go            ← 资源编译
│   │   ├── selector.go            ← SELECT 编译
│   │   └── executor.go            ← Add/Delete/Update/Info/List/Page
│   │
│   ├── eps/                       ← EPS / OpenAPI bootstrap
│   │
│   ├── outbox/                    ← Transactional Outbox / Inbox / Worker
│   │   └── store/                 ← 三数据库领取、Lease、去重 DML Adapter
│   │
│   ├── task/                      ← 本地调度 + 分布式队列
│   │
│   ├── grpc/                      ← gRPC 拦截器 / 异常映射 / 注册桥接
│   │
│   ├── codegen/                   ← AST 编译器
│   │
│   └── db/                        ← gdb 桥接
│       ├── tx/                    ← 统一事务 Scope / Runner
│       ├── driver/                ← MySQL/PostgreSQL/SQLite DDL 差异
│       ├── recycle/               ← 回收站
│       └── schema/                ← DDL 同步 / validate
│
├── cmd/
│   └── cool/                      ← CLI (cool 是产品名)
│       └── main.go                ← cool generate / check / build / run
│
├── modules/                       ← 业务模块（v2 阶段为空骨架）
│   └── modules_gen.go
│
├── manifest/config/               ← 部署配置
│   ├── config.yaml
│   └── config.local.yaml          ← gitignored
│
├── resource/                      ← 静态资源
│   └── public/
│       └── uploads/
│
└── docs/
    └── superpowers/
        └── specs/
            └── 2026-07-31-cool-admin-go-next-architecture-design.md
```

依赖方向：

```text
main / cmd
    -> core/app
        -> generated module graph
            -> controller + service + crud + outbox + task
                -> core/entity + db
                    -> GoFrame v2
```

强约束：

- `cool-next/*` 不得 import `modules/*`。
- 框架的协议无关层不得 import `ghttp` 或 gRPC 生成的协议类型。
- `db/driver` 只处理 DDL 方言和能力差异，不接管 DML。
- `crud` 不依赖具体业务 Service。
- 模块之间不靠隐式扫描或全局变量通信；依赖由构造函数参数类型形成有向图。

`CurdOption` 沿用 Node 公共 API 的历史拼写以保持概念兼容，不是 `CRUDOption` 的笔误。

---

## 模块目录协议 (v2)

`modules/<module>/` 内允许的子目录（生成器仅依赖此列表）：

| 目录 | 组件 |
|---|---|
| `contract/**` | 跨模块稳定接口 |
| `entity/**` | 模型定义 |
| `service/**` | 业务服务 |
| `controller/**` | Controller |
| `middleware/global/**` | 全局中间件 |
| `middleware/**` | 模块路由中间件 |
| `event/**` | 事件 |
| `schedule/**` | 定时任务 |
| `queue/**` | 队列 |
| `consumer/**` | Outbox/Inbox 可靠消息 Consumer |
| `dto/**` | 数据传输对象 |
| 模块根 `config.go` | `ModuleConfig() module.Declaration[Config]` |
| `db.json` / `menu.json` | 可选初始化数据 |

目录可以任意深度嵌套。`_test.go`、`testdata`、隐藏目录和标准生成文件不参与发现。  
`middleware/global/**` 目录本身不构成隐式注册，必须由 `ModuleConfig.GlobalMiddlewares` 显式引用构造器。

### 模块 `config.go` 模板

```go
package demo

import module "github.com/toothdy/cool-admin-go-next/cool-next/core/module"

type Config struct {
   Enabled bool `json:"enabled"`
   Limit   int  `json:"limit" v:"min:1#处理数量必须大于 0"`
}

func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{
      Name:        "示例模块",
      Description: "演示模块开发方式",
      Order:       0,
      Middlewares: []module.ComponentRef{
         module.Ref("middleware.New"),
      },
      GlobalMiddlewares: []module.ComponentRef{
         module.Ref("middleware/global.NewTrace"),
      },
      Defaults: Config{
         Enabled: true,
         Limit:   100,
      },
   }
}
```

`Declaration` 公开字段是 `Name`、`Description`、`Order`、`Middlewares`、`GlobalMiddlewares`、`Defaults`。  
框架按 “默认值 -> 主配置 -> 环境覆盖” 合并配置，然后统一调用 GoFrame `gvalid` 校验 `v` 标签。  
`module.Ref("middleware.New")` 只是供 AST 编译器解析和类型检查的符号引用，运行时不按字符串查找组件。

---

## 字段命名约定

| 字段语义 | 字段名（Go + DB） | 说明 |
|---|---|---|
| 创建时间 | `createTime` | 驼峰；`manifest/config/config.yaml` 的 `database.default.createdAt: createTime` |
| 更新时间 | `updateTime` | 驼峰；同上 `updatedAt: updateTime` |
| **密码版本** | **`passwordV`** | Go 字段统一为 `PasswordV`；DB、JSON、Redis Session 和 JWT Claim 统一为 `passwordV` |

> **前端 = 后端 = 数据库 = 驼峰**。不出现下划线，避免后端↔前端数据转换时的字段重命名（snake_case → camelCase）造成各种兼容问题。

Go struct 字段与 DB 列名映射：

- 业务实体必须用 struct tag 显式声明，例如 ``UserID uint64 `json:"userId" orm:"userId" description:"用户ID"` ``；框架不依赖 GoFrame 的默认字段名推导。
- `orm` 是数据库列名，必须为 lowerCamelCase；`json` 是外部字段名，通常与列名一致。
- `cool generate` 从 Go 类型和 `orm/json/description/cool` 标签生成 Descriptor；不存在 `entity.Field{ColumnName/JSONName}` 这套 v1 fluent API。

删除归档与 gdb 时序：

- `manifest/config/config.yaml` 显式声明字段映射（`createdAt: createTime` / `updatedAt: updateTime`）。
- gdb 启动期按 entity 字段自动维护 `createTime` / `updateTime`。
- `cool.crud.softDelete` 对齐 Node 配置名，省略时默认 `true`。开启后，默认 Base 和显式 Base 委托的 `Delete` 在同一事务中先写 `cool_recycle` 快照，再从业务表物理删除；关闭后直接物理删除。
- `coreentity.Base` 和业务实体都不包含 `deletedAt/deletedTime/deleteTime`，`Info/List/Page` 不注入逻辑删除过滤，也不启用 GoFrame/ORM 的另一套逻辑删除模式。
- 归档与删除、恢复与移除回收记录分别原子提交；失败整体回滚，不能用未等待的进程内事件异步补写回收记录。完整契约见架构文档 §4.7。

Node 对照：当前 `BaseService.delete` 在 `cool.crud.softDelete=true` 时先读取原记录并发送 `SOFT_DELETE` 回收事件，随后仍调用 `entity.delete(ids)`。因此这里对齐的是“回收站归档后硬删除”的功能语义，不是 TypeORM `@DeleteDateColumn` 逻辑删除；Go 版把两步收进同一数据库事务以消除丢归档窗口。

---

## CLI 工具

自建 `cool` CLI 位于 `cmd/cool/main.go`：

```text
cool generate      # 重生成 modules/modules_gen.go
cool check         # 校验 codegen 是否过期 / 静态检查
cool build         # go build
cool run           # go run (本地启动)
cool outbox list   # 查询可运维的 Outbox 记录
cool outbox show   # 按 messageId 检查记录
cool outbox replay # 审计后将单条 dead 记录转回 retry
```

前四个命令已经实现；`outbox` 子命令在对应模块交付后启用。`cool generate` 是模块发现、路由装配、Descriptor 编译、CRUD 适配的唯一入口；运行时不再扫描或反射。`cool build` 与 `cool run` 都会先执行生成文件新鲜度和当前静态契约检查。

应用默认读取 `manifest/config/config.yaml`，也可通过 `COOL_CONFIG_FILE` 指向另一份明确配置。业务模块配置统一放在顶层 `modules` 对象中，并以模块目录身份作为直接键：

```yaml
modules:
  system/user:
    enabled: true
```

`outbox` 子命令使用应用的正常配置和 `cool.outbox.databaseGroup`。重放必须提供 Operator/Reason，只将单条 `dead` 记录受控转回 `retry`；它不直接调用 Publisher，也不更换原 `messageId`。完整参数、审计、并发保护和脱敏规则见架构文档 §11.6。

---

## 开发命令

```bash
# Module、格式、vet、依赖边界和单元测试
make check

# Race Test
make test-race

# MySQL、PostgreSQL、SQLite Smoke Test
make test-integration

# 完整门禁
make check-full
```

普通 `go test ./...` 不需要外部数据库。`make test-integration` 通过 Docker Compose 启动隔离数据库，三库任一失败都会返回非零；环境、镜像覆盖和清理规则见 `test/integration/README.md`。

---

## 架构层次

```text
应用 (main.go)
  └─ Generated Module Graph (cool-next/codegen + modules/*)
       ├─ Interface (cool-next/core/controller + cool-next/grpc)
       ├─ Application (cool-next/core/service + cool-next/crud)
       ├─ Reliable Side Effects (cool-next/outbox + cool-next/task)
       ├─ Domain/Data (cool-next/core/entity + cool-next/db/{tx,driver,recycle,schema})
       ├─ Cross-Cutting (cool-next/auth + cool-next/core/{exception,module,event,tag,hooks,route,configuration,app})
       └─ Infrastructure (GoFrame v2 + Broker Adapter)
```

Application Host 通过协议无关的 `Transport` 接口统一协调 HTTP（`ghttp`）和 gRPC（`grpcx`），Ready Gate 与生命周期逆序停止是 v2 的硬约束。可观察的 Transport 错误由 Host 统一回滚；GoFrame HTTP 内部无法上报的 Serve 错误按进程级快速失败处理，整个应用非零退出并由生产部署监督器重启，不能在 HTTP 失效后继续保持 Ready。生产部署必须配置 Docker、Kubernetes、systemd 等进程监督器，并同时配置覆盖实际 HTTP/gRPC 服务的健康检查；缺少任一项都不属于受支持的生产运行方式。CRUD、Inbox、默认事务 Route 和事务任务共用 `cool.outbox.databaseGroup` 对应的唯一 Framework Database Group 与 `dbtx` Scope，不提供 Entity/Route 级换组。数据库提交后的可靠副作用使用 Transactional Outbox，交付语义固定为 at-least-once，并通过稳定 UUIDv7 `messageId` 和 Inbox 唯一键实现幂等消费；可靠 Consumer Adapter 必须提供持久 Ack、跨重启重试计数、延迟重试、DLQ 和 Message ID 保留能力。详细分层、依赖规则、不变量、生命周期回滚与异常映射见架构文档。

JWT 的 `sessionId/jti/subject/isRefresh/iss/aud/iat/nbf/exp` 为必填 Claim，Access/Refresh 都必须匹配服务端 Session；刷新原子轮换两个 JTI，旧 Refresh Token 重放会撤销 Session。Event、Schedule 和 Queue 保留为框架层能力，当前总架构只冻结职责与可靠性边界，具体 Definition、Handler、并发和失败协议由后续专项设计确定后再实现。

数据库列名统一使用 lowerCamelCase（如 `createTime`、`updateTime`、`messageId`）；表名和索引名仍可使用 snake_case。PostgreSQL DDL/DML 必须按方言引用列标识符以保留大小写。

数据库基线为 MySQL 8.x、PostgreSQL 9.5+ 和 SQLite 3.24+。MySQL 的默认存储引擎、所有参与框架事务的业务表及 Recycle/Outbox/Inbox 内部表必须为 InnoDB；应用启动时校验实际表引擎，`schema.mode=off` 也不跳过已启用基础设施的事务能力检查。

---

## 与 Node/v1 的边界

v2 替换 v1 的内部实现，除架构文档明确排除的能力外，对外协议以 Node 为准。明确边界：

| 维度 | Node (`cool-admin-midway`) | v1 (`cool-admin-go-next-v1`) | v2 (`cool-admin-go-next`) |
|---|---|---|---|
| 实体写法 | TypeORM Decorator | fluent Definition | Go struct + Descriptor |
| CRUD 输入 | plain object | `map[string]interface{}` | 私有 `AddInput/DeleteInput/UpdateInput/Mutable` 容器 |
| 批量 Add | 顶层数组 | `AddMany` 独立 API | `Add` 自身接收单对象和数组 |
| Hook 签名 | `(ctx, action, data)` | `(ctx, action, data interface{})` | `(ctx, *Mutation[E, ID])` |
| 模块发现 | 装饰器扫描 | 运行时注册表 | `cool generate` 编译期生成 |
| Service → API | `serviceApis` 自动映射 | 未实现（仅计划文档出现） | 不提供；显式 `Route{Handler}` |
| 数据库 | MySQL/PostgreSQL/SQLite | 主要 MySQL | MySQL 8.x / PostgreSQL 9.5+ / SQLite 3.24+ 同行为 |
| gRPC | `@cool-midway/rpc` | 无 | `grpcx` + 协议无关 Transport |
| Session | Cache + JWT | `SessionStore`（Redis 默认） | `SessionStore`（Redis 默认） |
| 多租户 | TypeORM Subscriber + URL 配置 | 框架 Scope | 不提供 |
| 提交后可靠副作用 | EventEmitter / PM2 / BullMQ 直接调用，无事务型 Outbox | 无统一可靠投递协议 | Transactional Outbox + Inbox，at-least-once + 幂等消费 |
| 软删除 | `cool.crud.softDelete` 开启时通过事件归档后硬删除 | recycle 抽象 | 同名全局配置；默认 `true`；Base 在同一事务中归档到 `cool_recycle` 后硬删除，不增加删除时间字段 |

下列能力**不在 v2 范围**：

- i18n / 翻译中间件；
- 文件空间（`cool.file`）业务模块；
- 插件市场（`@cool-midway/plugin`）；
- Swagger UI；
- 上传业务接口；
- 任何具体业务模块（`base` / `dict` / `task` / `recycle` / `user` 等）。

这些是 Node 主仓或后续业务模块的责任，由后续设计文档单独确认。

---

## 参考与入口

| 文档 | 路径 |
|---|---|
| v2 架构设计（权威） | `docs/superpowers/specs/2026-07-31-cool-admin-go-next-architecture-design.md` |
| Node 行为参考 | `../cool-admin-midway/docs/` |
| Node 公共包 | `../cool-admin-midway-packages/core/` |
| v1 实现 | `../cool-admin-go-next-v1/` |

实现期变更必须先反映在架构文档，然后由 `cool generate` 重新生成，最后提交 `modules/modules_gen.go`。  
`cool check` 在 CI 中统一验证：生成文件最新且字节稳定；构造器依赖图、生命周期图和 Producer/Consumer 图合法；路由、别名、权限和 Consumer Name 无冲突；实体、字段、索引、配置和消息版本元数据合法；业务代码未绕过事务、Descriptor、受控查询、Outbox 或基础设施依赖边界。它不替代 `gofmt`、`go vet`、`go test` 或 `go test -race`，这些命令仍由 CI 独立执行。
