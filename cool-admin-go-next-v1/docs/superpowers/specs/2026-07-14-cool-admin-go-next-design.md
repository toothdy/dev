# cool-admin-go-next 设计文档

日期：2026-07-14

## 1. 背景与目标

`cool-admin-go-next` 是面向长期演进的 Go 版 cool-admin 后端。它的第一目标不是做一个 Go 风格的新后台接口，而是让现有 `cool-admin-vue` 前端在尽量不改代码的情况下接入 Go 后端。

第一阶段目标：

1. 底层采用 GoFrame v2。
2. 上层封装 `cool` 兼容层。
3. 严格兼容 Node 版 base 模块接口协议。
4. 支持 MySQL。
5. 支持运行时自动建表。
6. 支持 `db.json` / `menu.json` 初始化导入。
7. 支持运行时自动生成 EPS。
8. 实现 base 基础后台模块。
9. 让现有 Vue 前端不改即可登录、加载菜单、加载权限并使用基础 CRUD 页面。

第一阶段不做：

1. 插件系统。
2. PostgreSQL / SQLite。
3. RPC。
4. 完整队列任务系统。
5. 全量 Node 内置模块迁移。
6. 运行时模块卸载。

但架构需要为后续插件、数据库适配、CLI 生成器、更多模块扩展预留接口。

## 2. 技术路线

推荐路线为：

```text
cool-admin-vue
      ↓ Node-compatible HTTP protocol
cool-admin-go-next
      ↓
cool runtime compatibility layer
      ↓
GoFrame v2
      ↓
MySQL
```

GoFrame v2 负责：

1. HTTP Server。
2. Router Group。
3. Middleware。
4. Config。
5. Logging。
6. MySQL 连接。
7. ORM 执行能力。
8. `gerror` 错误栈。
9. CLI/代码生成的长期基础。

`cool` 兼容层负责：

1. 模块注册。
2. Controller 注册。
3. CRUD 注册。
4. Service 基类。
5. Model 元数据。
6. 自动建表。
7. EPS 生成。
8. 统一响应。
9. Token/权限。
10. 菜单/初始化数据导入。
11. Node 协议兼容。

## 3. 目录结构

建议初始目录：

```text
cool-admin-go-next/
├── go.mod
├── main.go
├── manifest/
│   └── config/
│       ├── config.yaml
│       └── config.local.yaml
├── cool/
│   ├── app/
│   ├── auth/
│   ├── cache/
│   ├── config/
│   ├── controller/
│   ├── crud/
│   ├── db/
│   ├── eps/
│   ├── errors/
│   ├── module/
│   ├── model/
│   ├── response/
│   ├── seed/
│   └── service/
├── modules/
│   ├── modules.go
│   └── base/
│       ├── base.go
│       ├── config.go
│       ├── db.json
│       ├── menu.json
│       ├── controller/
│       │   ├── admin/
│       │   ├── app/
│       │   └── open/
│       ├── model/
│       └── service/
├── resource/
│   └── public/
├── utility/
└── docs/
```

说明：

1. `cool/` 是 Go 版核心框架层，对齐 Node `@cool-midway/core`。
2. `modules/base` 是第一阶段重点模块。
3. `modules/modules.go` 统一注册模块，替代 Node 运行时自动 `require`。
4. `manifest/config` 放 GoFrame 配置。
5. 不使用 `logic/` 目录，业务逻辑直接放在 `service/`。
6. GoFrame 自动生成的 `dao`、`do`、`entity` 文件后续如引入，必须由工具生成，不手写、不手改。

## 4. 核心包职责

### 4.1 `cool/app`

应用启动编排层，类似 Node 版 `configuration.ts`。

负责：

1. 加载配置。
2. 初始化数据库。
3. 注册模块。
4. 自动建表。
5. 导入 `db.json` / `menu.json`。
6. 注册路由。
7. 生成 EPS。
8. 启动 HTTP 服务。

### 4.2 `cool/module`

模块系统。

建议模块接口包含：

```go
type Module interface {
   Name() string
   Order() int
   Config() ModuleConfig
   Models() []model.Definition
   Controllers() []controller.Definition
   Services() []service.Definition
   Seeds() seed.Definition
}
```

第一阶段采用静态注册：

```go
module.Register(base.NewModule())
```

后续 CLI 可以自动维护 `modules/modules.go`，降低人工维护成本。

### 4.3 `cool/controller`

Controller 注册和路由映射层。

目标是模拟 Node 版 `@CoolController`，例如：

```go
controller.Admin("base/sys/user").
   Description("用户管理").
   CRUD(crud.Options{
      APIs: []crud.API{
         crud.APIAdd,
         crud.APIDelete,
         crud.APIUpdate,
         crud.APIInfo,
         crud.APIList,
         crud.APIPage,
      },
      Model:   model.BaseSysUserDefinition(),
      Service: baseService.NewBaseSysUserService(),
      PageQuery: crud.QueryOp{
         KeyWordLikeFields: []string{
            "name",
            "username",
            "nickName",
         },
         FieldEq: []string{
            "status",
            "departmentId",
         },
      },
      InfoIgnoreProperty: []string{
         "password",
      },
   })
```

生成路由：

```text
POST /admin/base/sys/user/add
POST /admin/base/sys/user/delete
POST /admin/base/sys/user/update
GET  /admin/base/sys/user/info
POST /admin/base/sys/user/list
POST /admin/base/sys/user/page
```

### 4.4 `cool/crud`

CRUD 通用执行层。

负责：

1. `Add`。
2. `Delete`。
3. `Update`。
4. `Info`。
5. `List`。
6. `Page`。
7. `ServiceApis`。
8. `Before/After` hooks。
9. `InsertParam`。
10. `InfoIgnoreProperty`。

### 4.5 `cool/service`

Service 基类和业务服务注册。

第一阶段只支持 MySQL，但接口预留数据库适配：

```go
type BaseService[T any] struct {
   DB    db.Adapter
   Model model.Definition
   Hooks ServiceHooks
}
```

业务服务通过 hooks 对齐 Node 版 `modifyBefore` / `modifyAfter`。

### 4.6 `cool/model`

Model 元数据层。

Go 版用 struct tag 和注册函数表达 Node TypeORM Entity metadata。

职责：

1. 解析表名。
2. 解析字段名。
3. 解析字段类型。
4. 解析长度。
5. 解析注释。
6. 解析默认值。
7. 解析索引。
8. 解析 nullable。
9. 解析 dict。
10. 为自动建表、CRUD、EPS 提供统一元数据。

### 4.7 `cool/db`

MySQL adapter。

第一阶段负责：

1. 连接 MySQL。
2. 自动建表。
3. 自动补字段。
4. CRUD SQL/ORM 执行。
5. 分页。
6. 排序。
7. 模糊查询。
8. 等值查询。
9. 软删除。

安全边界：

1. 自动创建不存在的表。
2. 自动新增缺失字段。
3. 不自动删除字段。
4. 不自动做危险类型收窄。
5. 表结构差异只记录日志。
6. 生产环境可关闭 auto sync。

### 4.8 `cool/eps`

EPS 生成层。

运行时从模块、Controller、CRUD、Model、路由 metadata 生成 Node 兼容 EPS snapshot。

### 4.9 `cool/seed`

初始化数据导入层。

对齐 Node 风格：

```text
modules/base/db.json
modules/base/menu.json
```

支持：

1. 按模块顺序导入。
2. DB 标记避免重复导入。
3. 文件锁可选。
4. 父子数据引用。
5. 菜单初始化。
6. `base_sys_conf` 写入初始化标记。

### 4.10 `cool/auth`

认证与权限层。

负责：

1. 登录。
2. Token 生成。
3. Token 解析。
4. admin/app 用户上下文。
5. 权限码。
6. 菜单权限。
7. 忽略 token 路由。
8. 超管逻辑。

### 4.11 `cool/response`

统一响应层。

对齐 Node：

```json
{
  "code": 1000,
  "message": "success",
  "data": {}
}
```

错误响应也必须兼容 Node/Vue。

## 5. 模块注册设计

模块目录对齐 Node 版：

```text
modules/base/
├── base.go
├── config.go
├── db.json
├── menu.json
├── controller/
│   ├── admin/
│   ├── app/
│   └── open/
├── model/
└── service/
```

Go 不能像 Node 一样动态 `require` 未引用代码，所以第一阶段使用显式注册：

```go
package modules

import (
   "cool-admin-go-next/modules/base"

   "cool-admin-go-next/cool/module"
)

/**
 * 注册全部模块
 * @returns null
 */
func RegisterModules() {
   module.Register(base.NewModule())
}
```

`base.NewModule()` 返回模块定义：

```go
package base

import (
   "cool-admin-go-next/cool/module"
)

/**
 * 创建 base 模块
 * @returns module.Module
 */
func NewModule() module.Module {
   return module.New("base").
      Name("基础模块").
      Description("系统基础能力").
      Order(100).
      Config(NewConfig()).
      Models(RegisterModels()).
      Services(RegisterServices()).
      Controllers(RegisterControllers()).
      Seeds("db.json", "menu.json")
}
```

## 6. Model 元数据与自动建表

### 6.1 BaseModel

Node 版默认字段：

1. `id`。
2. `createTime`。
3. `updateTime`。
4. `tenantId`。

Go 版建议 DB 使用 snake_case，API/EPS 使用 camelCase：

```go
type BaseModel struct {
   Id         uint64      `json:"id" cool:"column:id;type:bigint;primary;autoIncrement;comment:ID"`
   CreateTime *gtime.Time `json:"createTime" cool:"column:create_time;type:datetime;comment:创建时间;autoCreateTime"`
   UpdateTime *gtime.Time `json:"updateTime" cool:"column:update_time;type:datetime;comment:更新时间;autoUpdateTime"`
   TenantId   uint64      `json:"tenantId" cool:"column:tenant_id;type:bigint;nullable;index;comment:租户ID"`
}
```

前端和 EPS 只看到 camelCase，DB 内部使用 snake_case。

### 6.2 Model 示例

```go
type BaseSysUser struct {
   coolModel.BaseModel

   Username string `json:"username" cool:"column:username;type:varchar;size:100;notNull;index;comment:用户名"`
   Password string `json:"password" cool:"column:password;type:varchar;size:255;notNull;comment:密码"`
   Name     string `json:"name" cool:"column:name;type:varchar;size:100;comment:姓名"`
   NickName string `json:"nickName" cool:"column:nick_name;type:varchar;size:100;comment:昵称"`
   HeadImg  string `json:"headImg" cool:"column:head_img;type:varchar;size:255;comment:头像"`
   Phone    string `json:"phone" cool:"column:phone;type:varchar;size:20;comment:手机号"`
   Email    string `json:"email" cool:"column:email;type:varchar;size:100;comment:邮箱"`
   Status   int    `json:"status" cool:"column:status;type:int;default:1;dict:禁用,启用;comment:状态"`
}
```

Model 注册：

```go
func BaseSysUserDefinition() model.Definition {
   return model.Define[BaseSysUser]().
      Name("BaseSysUser").
      Table("base_sys_user").
      Comment("系统用户").
      Module("base")
}
```

### 6.3 自动建表流程

```text
读取已注册模块
   ↓
收集所有 model.Definition
   ↓
查询 information_schema
   ↓
表不存在 → CREATE TABLE
   ↓
表存在 → 对比字段
   ↓
字段不存在 → ALTER TABLE ADD COLUMN
   ↓
索引不存在 → CREATE INDEX
   ↓
危险差异 → 只日志提示，不自动修改
```

第一阶段自动处理：

1. 创建表。
2. 新增字段。
3. 新增普通索引。
4. 新增唯一索引。
5. 设置默认值。
6. 设置 nullable / not null。
7. 设置 comment。

第一阶段不自动处理：

1. 删除字段。
2. 重命名字段。
3. 字段类型收窄。
4. 字段长度缩短。
5. 删除索引。
6. 修改主键。
7. 数据迁移。

配置：

```yaml
cool:
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
```

## 7. 初始化数据导入

### 7.1 `db.json`

沿用 Node 版结构：

```text
modules/base/db.json
```

导入流程：

```text
启动
   ↓
读取模块顺序
   ↓
判断 initDB / initJudge
   ↓
读取 db.json
   ↓
按 tableName 分组导入
   ↓
处理 @childDatas
   ↓
处理父子字段引用
   ↓
写入 init_db_{module} 标记
```

重复导入判断优先使用 DB 标记：

```text
base_sys_conf:
  cKey = init_db_base
  cValue = time consuming: xxxms
```

### 7.2 `menu.json`

沿用 Node 版结构：

```text
modules/base/menu.json
```

导入到：

```text
base_sys_menu
```

导入标记：

```text
base_sys_conf:
  cKey = init_menu_base
```

## 8. EPS 设计

EPS 是 Node 协议兼容层，不做 Go 风格重新设计。

数据来源：

```text
module registry
   ↓
controller registry
   ↓
route registry
   ↓
crud options
   ↓
model metadata
   ↓
auth tag metadata
   ↓
eps snapshot
```

建议对外接口：

```text
GET /admin/base/open/eps
```

实际路径以 `cool-admin-vue` 当前调用为准。

响应：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "admin": {},
    "app": {},
    "module": {}
  }
}
```

每个 Controller EPS 包含：

1. `module`。
2. `name`。
3. `prefix`。
4. `info`。
5. `api`。
6. `columns`。
7. `pageQueryOp`。
8. `pageColumns`。

CRUD 默认路由：

| API | Method | Path | Summary |
|---|---|---|---|
| add | POST | `/add` | 新增 |
| delete | POST | `/delete` | 删除 |
| update | POST | `/update` | 修改 |
| page | POST | `/page` | 分页查询 |
| list | POST | `/list` | 列表查询 |
| info | GET | `/info` | 单个信息 |

Column EPS 示例：

```json
{
  "propertyName": "status",
  "type": "int",
  "length": "",
  "comment": "状态",
  "nullable": false,
  "defaultValue": 1,
  "dict": ["禁用", "启用"],
  "source": "a.status"
}
```

字段名输出 Node camelCase，DB 字段通过 metadata 映射。

## 9. base 模块协议

第一阶段 base 至少包含：

1. 登录。
2. 获取用户信息。
3. 获取权限菜单。
4. 获取权限码。
5. 系统菜单。
6. 用户管理。
7. 角色管理。
8. 部门管理。
9. 参数配置。
10. 操作日志。
11. EPS。
12. 文件上传基础接口，若前端启动即依赖。

建议表：

| 表 | 说明 |
|---|---|
| `base_sys_user` | 系统用户 |
| `base_sys_role` | 角色 |
| `base_sys_menu` | 菜单/权限 |
| `base_sys_department` | 部门 |
| `base_sys_user_role` | 用户角色关联 |
| `base_sys_role_menu` | 角色菜单关联 |
| `base_sys_role_department` | 角色部门关联 |
| `base_sys_param` | 系统参数 |
| `base_sys_log` | 操作日志 |
| `base_sys_conf` | 初始化标记/系统配置 |
| `base_eps_admin` | admin EPS 缓存，视 Node/Vue 需要 |
| `base_eps_app` | app EPS 缓存，视 Node/Vue 需要 |

路径、HTTP method、参数名、响应字段必须以现有 Vue 调用为准。

## 10. 登录鉴权

Go 版使用 JWT，但协议兼容 Node/Vue。

中间件职责：

1. 判断路由是否 `IgnoreToken`。
2. 从 header 读取 token。
3. 解析 JWT。
4. 写入请求上下文。
5. 查询权限缓存或数据库。
6. 权限不通过返回兼容错误。

登录流程：

```text
接收 username/password/captcha?
   ↓
校验验证码，第一阶段可配置关闭
   ↓
查询 base_sys_user
   ↓
校验状态
   ↓
校验密码
   ↓
生成 token
   ↓
记录登录日志
   ↓
返回前端需要结构
```

密码算法必须兼容 Node 版初始化数据。不能在第一阶段随意更换 hash 规则。

## 11. 权限菜单

菜单来源：

```text
base_sys_menu
```

权限逻辑：

1. `admin` 超管拥有全部权限。
2. 普通用户通过 `user_role → role_menu → menu` 获取菜单和权限码。
3. 禁用菜单不返回。
4. 按 `orderNum` 排序。
5. 返回树结构或平铺结构以 Vue 需求为准。

## 12. 错误处理

内部错误使用 GoFrame `gerror`，外部响应兼容 Node 版。

原则：

1. 所有业务错误带清晰 message。
2. 关键错误保留 stack。
3. DB、鉴权、参数错误分层包装。
4. HTTP 响应不泄露内部堆栈。
5. 日志记录完整错误，前端只收到兼容结构。

统一转换：

```text
error
   ↓
cool/errors classify
   ↓
cool/response Fail
   ↓
Node-compatible JSON
```

## 13. 中间件

第一阶段核心中间件：

1. 响应中间件：panic recover、error 转统一响应、日志记录。
2. 鉴权中间件：IgnoreToken、Token 解析、上下文注入、权限检查。
3. 日志中间件：操作日志、请求耗时、IP、用户 ID、敏感字段脱敏。
4. CORS/静态资源：方便本地 Vue 开发。

## 14. 测试策略

### 14.1 单元测试

覆盖：

1. Model tag 解析。
2. 字段名 camelCase/snake_case 映射。
3. SQL schema 生成。
4. QueryOp 构建。
5. EPS 生成。
6. token 生成/解析。
7. 权限树构建。
8. `db.json` / `menu.json` 导入解析。

### 14.2 集成测试

使用 MySQL 测试库，覆盖：

1. 启动自动建表。
2. 导入 base 初始化数据。
3. 登录成功。
4. 登录失败。
5. 获取用户信息。
6. 获取菜单。
7. 获取权限码。
8. EPS 返回结构。
9. 用户/角色/部门 CRUD。
10. 软删除行为。

### 14.3 前端兼容测试

使用现有 `cool-admin-vue`：

1. 不改前端配置之外的代码。
2. 指向 Go 后端。
3. 登录。
4. 首页加载。
5. 菜单显示。
6. 用户管理页面可分页。
7. 角色/菜单/部门页面可用。
8. 新增/编辑/删除基础数据。
9. 刷新页面 token 仍可用。
10. 无权限/Token 失效表现正常。

### 14.4 协议快照测试

保存 Node 版接口响应 fixture，对 Go 版响应做结构对比。

重点接口：

1. login。
2. person/user info。
3. permmenu。
4. perms。
5. eps。
6. page/list/info。

## 15. 阶段计划

### 阶段 0：协议勘察

目标：确认前端和 Node base 的真实契约。

内容：

1. 读取 `cool-admin-vue` API 定义。
2. 读取 Node 版 base Controller/Service/Entity。
3. 整理接口路径、方法、参数、响应。
4. 整理 base 表结构。
5. 整理 EPS 实际格式。
6. 整理密码算法和 token 规则。

产出：

```text
docs/protocol/base-api-contract.md
```

### 阶段 1：项目骨架和 core runtime

目标：初始化 `cool-admin-go-next` 骨架。

内容：

1. GoFrame v2 项目基础。
2. `cool/app` 启动编排。
3. `cool/module` 模块注册。
4. `cool/controller` 路由注册。
5. `cool/response` 统一响应。
6. 配置加载。
7. MySQL 连接。

产出：

1. 可以启动空服务。
2. `/health` 可访问。
3. 模块注册生效。

### 阶段 2：Model metadata 和自动建表

目标：完成表结构自动同步。

内容：

1. `cool/model` struct tag 解析。
2. MySQL type mapping。
3. information_schema 读取。
4. CREATE TABLE。
5. ALTER TABLE ADD COLUMN。
6. index 创建。
7. base 表 model 定义。

产出：

1. 启动自动创建 base 所需表。

### 阶段 3：seed/menu 导入

目标：初始化数据可用。

内容：

1. `db.json` 解析。
2. `menu.json` 解析。
3. 父子数据引用。
4. 写入 `base_sys_conf` 初始化标记。
5. 初始化 admin 用户、角色、菜单。

产出：

1. 数据库初始化后可登录。

### 阶段 4：CRUD runtime

目标：通用 CRUD 跑通。

内容：

1. Add/Delete/Update/Info/List/Page。
2. QueryOp。
3. 分页排序。
4. 模糊查询。
5. 等值查询。
6. infoIgnoreProperty。
7. service hooks。

产出：

1. base 用户/角色/部门/菜单/参数/日志 CRUD 可用。

### 阶段 5：auth 和 base 协议

目标：前端可登录并加载后台。

内容：

1. 登录。
2. JWT。
3. token middleware。
4. admin context。
5. 权限菜单。
6. 权限码。
7. 用户信息。
8. 操作日志。

产出：

1. Vue 前端不改可登录和显示菜单。

### 阶段 6：EPS runtime

目标：前端动态 API 元数据可用。

内容：

1. EPS snapshot。
2. Admin/App grouping。
3. columns。
4. pageQueryOp。
5. route metadata。
6. ignoreToken。
7. EPS 接口。

产出：

1. Vue EPS 消费正常。

### 阶段 7：前端联调和兼容修正

目标：用 Vue 端验证真实可用。

内容：

1. 登录联调。
2. 菜单联调。
3. 权限联调。
4. CRUD 页面联调。
5. EPS 联调。
6. 修正字段差异。

产出：

1. 第一阶段 MVP 完成。

## 16. 后续预留扩展点

第一阶段不实现插件系统，但保留：

1. DB adapter。
2. Module lifecycle。
3. Module registry。
4. EPS provider。
5. Seed provider。
6. CLI generator。
7. Plugin hook。

后续仓库地址：

```text
https://github.com/toothdy/cool-admin-go-next
```

## 17. 验收标准

第一阶段完成的验收标准：

1. Go 后端能连接 MySQL 并自动创建 base 表。
2. Go 后端能导入 base 初始化数据和菜单。
3. 现有 Vue 前端不改业务代码即可登录。
4. 登录后能加载用户信息、菜单、权限码。
5. EPS 接口返回结构被前端正常消费。
6. 用户、角色、菜单、部门、参数、日志基础页面可访问。
7. 通用 CRUD 的 add/delete/update/info/list/page 行为与 Node 版兼容。
8. 错误响应结构与 Node 版兼容。
9. 前端刷新页面后 token 仍然可用。
10. 未登录、无权限、token 失效场景表现正常。
