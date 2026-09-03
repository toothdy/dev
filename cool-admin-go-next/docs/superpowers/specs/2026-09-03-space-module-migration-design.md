# 文件空间模块迁移设计

> 日期：2026-09-03  
> 状态：已自审，待复核
> 源模块：`cool-admin-midway/src/modules/space`  
> 目标模块：`cool-admin-go-next/modules/space`

## 1. 目标

在 `cool-admin-go-next` 中实现 Midway 文件空间模块，使现有 `cool-admin-vue` 无需修改即可使用 Go 后端管理文件元数据与分类。

Midway 模块是业务行为的首要事实来源。Go 版保持相同的 HTTP 路径、请求字段、响应结构、CRUD 能力、筛选规则、权限语义和分类级联删除结果，同时遵循 Go v2 现有模块、实体、Service、Controller、事务、删除归档与静态生成架构。

在兼容行为之外，Go 版增加可选的本地真实文件删除能力。该能力默认关闭，只由 Space 模块的删除流程触发，不增加公开删除接口，也不向其他业务模块提供文件删除能力。

`cool-admin-go-next-v1` 不在本次范围内。

## 2. 方案与边界

采用以下职责划分：

1. Space 模块负责文件元数据、分类、级联关系和是否删除真实文件的策略；
2. 真实文件删除代码只位于 Space 模块；
3. Base 上传服务继续负责上传和读取，仅向 Space 提供受管文件的只读定位信息，避免 Space 重复上传根目录与公开地址配置；
4. 不抽象通用文件删除服务，不增加异步清理队列或分布式事务；
5. 不修改前端，不新增对象存储驱动，不迁移没有消费者的 WPS 测试配置；
6. 不增加 `tenantId` 或多租户过滤，沿用 Go v2 当前统一边界。

未采用的方案：

- Space 重复保存上传根目录和公开地址：会产生两份可能漂移的配置；
- Base 提供通用删除服务：当前只有 Space 存在该业务需求，会扩大删除能力的可见范围；
- 使用任务或 Outbox 异步清理：会改变同步错误语义并引入当前需求不需要的复杂度。

## 3. 模块结构

```text
modules/space/
├── config.go
├── entity/
│   ├── info.go
│   └── type.go
├── service/
│   ├── info.go
│   └── type.go
└── controller/
    ├── contract_test.go
    └── admin/
        ├── info.go
        └── type.go
```

实体与 Service 测试放在对应包的 `_test.go` 文件中。`modules/modules_gen.go` 由现有 `cool generate` 流水线更新，不手工维护生成内容。

模块声明保持 Midway 元数据：

- 名称：`文件空间`；
- 描述：`上传和管理文件资源`；
- 加载顺序：`0`；
- 无模块中间件和全局中间件；
- Midway 中未被任何业务代码读取的 WPS 测试 `appId` 不迁移。

Space 配置增加布尔字段 `ShouldDeletePhysicalFile`，JSON/YAML 名称为 `shouldDeletePhysicalFile`，默认值为 `false`。覆盖配置的完整路径为 `modules.space.shouldDeletePhysicalFile`：

```yaml
modules:
  space:
    shouldDeletePhysicalFile: false
```

## 4. 数据模型

### 4.1 文件信息

表名固定为 `space_info`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` | `*gtime.Time` | 框架维护 |
| `updateTime` | `*gtime.Time` | 框架维护 |
| `url` | `string` | 非空字符串 |
| `type` | `string` | 非空字符串 |
| `classifyId` | `*uint64` | 可空 |
| `fileId` | `string` | 非空字符串，普通索引 |
| `name` | `string` | 非空字符串 |
| `size` | `int32` | 非空，与 TypeORM 默认 `int` 对齐 |
| `version` | `int32` | 默认 `1` |
| `key` | `string` | 非空字符串 |

字符串字段沿用当前 Go 模块的默认兼容长度。`fileId` 索引名称固定，确保 MySQL、PostgreSQL 和 SQLite 的期望 Schema 一致。

### 4.2 文件分类

表名固定为 `space_type`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` | `*gtime.Time` | 框架维护 |
| `updateTime` | `*gtime.Time` | 框架维护 |
| `name` | `string` | 非空字符串 |
| `parentId` | `*uint64` | 可空 |

不增加数据库外键。分类与文件信息的级联删除由 Service 在同一数据库事务中完成，以兼容现有删除归档机制。

## 5. HTTP 契约

### 5.1 文件信息

前缀：`/admin/space/info`

| 路径 | 方法 | 权限 | 行为 |
| --- | --- | --- | --- |
| `/add` | POST | `space:info:add` | 新增文件元数据 |
| `/delete` | POST | `space:info:delete` | 删除文件元数据，并按配置删除本地真实文件 |
| `/update` | POST | `space:info:update` | 更新文件元数据 |
| `/info` | GET | `space:info:info` | 查询详情 |
| `/list` | POST | `space:info:list` | 查询列表 |
| `/page` | POST | `space:info:page` | 分页，支持 `type`、`classifyId` 等值筛选 |

### 5.2 文件分类

前缀：`/admin/space/type`

| 路径 | 方法 | 权限 | 行为 |
| --- | --- | --- | --- |
| `/add` | POST | `space:type:add` | 新增分类 |
| `/delete` | POST | `space:type:delete` | 删除分类及该分类下的文件记录 |
| `/update` | POST | `space:type:update` | 更新分类 |
| `/info` | GET | `space:type:info` | 查询详情 |
| `/list` | POST | `space:type:list` | 查询列表 |
| `/page` | POST | `space:type:page` | 分页查询 |

全部路由均要求后台 Token 和对应菜单权限，不添加 `ignoreToken` 标签。现有 `modules/base/menu.json` 已声明上述权限，本模块不新增 `menu.json`。

## 6. Service 行为

### 6.1 新增文件信息

文件信息新增流程：

1. 通过 Base 上传服务的只读定位方法检查提交的 `url` 是否属于当前本地上传公开地址；
2. 属于受管本地文件时，从定位结果生成 `/upload/<YYYYMMDD>/<name>` 形式的规范 `key` 并覆盖请求值，保持 Midway 本地模式由服务端推导 `key` 的行为；
3. 外部 URL 或未来对象存储 URL 不执行本地路径推导，保留请求中的 `key`；
4. 将规范化后的输入交给基础 Service 新增。

Base 上传服务新增只读定位方法，输入完整 URL，输出上传根目录、安全相对路径和是否属于受管文件。该方法必须按结构比较 URL 的 scheme、host 和公开路径前缀，拒绝 user info、query、fragment、非法转义、非法日期、多层文件名、绝对路径与目录穿越；不能使用字符串前缀替换判断归属。Base 不打开或删除目标文件，也不增加删除路由。

### 6.2 删除文件信息

文件信息删除流程：

1. 在当前 CRUD 事务中按请求 ID 查询待删记录的 `url`；
2. 调用基础 Service 删除数据库记录并进入现有删除归档流程；
3. `shouldDeletePhysicalFile=false` 时结束，不访问文件系统；
4. `shouldDeletePhysicalFile=true` 时，仅使用 Base 从持久化 `url` 解析出的受管文件位置，不读取客户端可独立控制的 `key` 作为删除目标；
5. 使用受根目录约束的文件 API 删除普通文件，不跟随符号链接，不接受绝对路径、目录穿越、非法日期或嵌套文件名；
6. 对规范化后的相对路径去重，避免多条元数据重复操作同一个文件；
7. 外部文件和非受管路径跳过物理删除；
8. 上传根目录或目标文件已经不存在视为成功；目录、符号链接及其他文件系统错误返回 Core 异常，由 CRUD Dispatcher 回滚数据库事务。

数据库事务无法和文件系统形成真正的原子事务。上述顺序保证数据库删除失败时不会触碰文件；物理删除失败时数据库会回滚。但批量删除到一半发生文件系统错误，或所有文件删除后数据库提交失败时，已删除文件无法恢复。本次不为这一低概率边界引入文件暂存、异步补偿或分布式事务。

### 6.3 删除分类

分类删除流程：

1. 删除请求中的分类记录；
2. 查询 `classifyId` 位于请求 ID 集合中的全部文件信息；
3. 构造文件删除输入并调用 `SpaceInfoService.Delete`；
4. 文件元数据级联删除和可选物理删除均复用文件信息删除流程；
5. 分类与文件元数据删除使用 CRUD Dispatcher 建立的同一数据库事务。

行为与 Midway 一致，只删除直接归属于所删分类的文件记录，不递归删除子分类，也不删除子分类记录。

## 7. 初始化、菜单与装配

Midway Space 模块没有 `db.json`，Go 模块不新增初始化数据。Space 菜单与权限已经存在于 `modules/base/menu.json`，不重复声明。

执行现有生成命令后，静态生成文件负责：

1. 注册 Space 模块与两个实体 Descriptor；
2. 构造两个基础 Service 和两个业务 Service；
3. 注入 Space 配置及 Base 上传定位依赖；
4. 注册两个 Admin Controller；
5. 发布 CRUD 路由、权限和 EPS 元数据。

## 8. 错误与安全

1. 输入绑定、ID 校验、未知字段、分页参数和数据库错误沿用现有 Controller、Service 与 `exception` 机制；
2. 不吞掉数据库和有效受管文件的删除错误，不向前端暴露磁盘绝对路径、SQL 或堆栈；
3. 所有数据库查询使用 GoFrame 参数化 ORM 条件；
4. 物理删除开关默认关闭，升级后不会改变现有数据与文件；
5. 路径是否受管必须由 Base 上传配置和结构化 URL 解析结果决定；`key` 不参与物理删除，持久化 `url` 仍必须重新校验；
6. 目录、符号链接、外部 URL、编码后的路径穿越和上传根目录之外的目标均不得删除；
7. 更新接口仍保持 Midway 全量 CRUD 契约，但删除前必须重新校验当前持久化值，不能因历史或篡改数据扩大删除范围；
8. 启用物理删除后，回收站恢复文件元数据不恢复真实文件，这是管理员显式开启该配置后的预期限制。

## 9. 验证

### 9.1 单元与契约测试

至少覆盖：

1. 两个 Controller 的路径、方法、CRUD、查询配置、权限和 EPS 投影；
2. 两个实体的表名、字段、可空性、默认值与 `fileId` 索引；
3. 本地上传 URL 的 `key` 推导与外部 URL 保留，包括 host 前缀混淆、query、fragment 和非法转义；
4. `modules.space.shouldDeletePhysicalFile` 默认关闭和显式开启；
5. 单个、批量及分类级联删除；
6. 文件不存在的幂等行为；
7. 外部 URL、绝对路径、目录穿越、非法日期、嵌套文件名、目录和符号链接保护，以及重复文件路径去重；
8. 文件系统删除失败时返回错误并回滚数据库事务；
9. 删除分类不递归删除子分类及其文件；
10. 生成后的模块装配、路由与 EPS 契约。

### 9.2 工程门禁

实现后依次执行：

```text
go doc os.Root
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./modules/space/... ./modules/base/service -count=1
go test ./... -count=1
go vet ./...
test -z "$(gofmt -l $(rg --files modules/space -g '*.go') modules/base/service/upload.go)"
git diff --check
```

如本地数据库环境可用，再启动 Go 服务，使用现有前端或 HTTP 请求验证登录、EPS、Space CRUD、分类筛选、分类级联和配置开启后的真实文件删除。

## 10. 验收标准

1. `cool-admin-vue` 不需要任何代码或配置修改；
2. Midway Space 模块现有全部路由在 Go 后端可用；
3. 请求字段、成功响应、错误响应和权限语义符合当前 Go v2 已对齐的协议；
4. 文件空间页面可完成分类和文件元数据的增删改查及筛选；
5. 删除分类会删除直接归属该分类的文件记录；
6. 默认不删除真实文件；开启配置后，只删除本地受管普通文件；
7. 非受管路径无法触发磁盘删除，错误信息不泄露服务端路径；
8. EPS 和静态模块装配包含 Space 模块；
9. MySQL、PostgreSQL 和 SQLite 使用相同业务代码，不引入方言 SQL；
10. 生成检查、全量测试、静态检查与格式检查通过。
