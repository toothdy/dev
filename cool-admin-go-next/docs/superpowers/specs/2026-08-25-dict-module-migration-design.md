# 字典模块迁移设计

> 日期：2026-08-25  
> 状态：已批准
> 源模块：`cool-admin-midway/src/modules/dict`  
> 目标模块：`cool-admin-go-next/modules/dict`

## 1. 目标

在 `cool-admin-go-next` 中实现 Node 字典模块，使现有 `cool-admin-vue` 无需修改即可切换到 Go 后端。

Node 版是字典业务行为的首要事实来源。Go 版保持相同的 HTTP 路径、请求字段、响应数据、CRUD 能力、权限语义、EPS 元数据、初始化数据、排序、数值转换和级联删除结果，同时遵循 Go v2 已有的模块、实体、Service、Controller、事务、种子和静态生成架构。

## 2. 范围

本次实现包含：

1. `dict_type` 和 `dict_info` 两张表及其 CRUD；
2. Admin 与 App 字典查询接口；
3. 字典类型删除时级联删除其全部字典项；
4. 字典项删除时递归删除全部后代；
5. 按字典 key 聚合数据和数字值转换；
6. 按值或 ID 解析字典名称的内部 Service 能力；
7. Node `db.json` 初始数据；
8. 权限、EPS、模块装配和契约测试。

以下能力沿用 Go v2 已批准的全局架构边界，不在本次扩建：

1. 不增加 `tenantId`、租户过滤或租户绕过逻辑；
2. 不迁移 Node 本地环境中扫描源码、调用外部翻译服务并生成 locale 文件的完整 i18n 工具链；
3. 不修改前端；
4. 不抽象通用树服务、级联删除框架或新的依赖。

## 3. 模块结构

目标模块使用现有业务模块布局：

```text
modules/dict/
├── config.go
├── db.json
├── dto/
│   └── info.go
├── entity/
│   ├── info.go
│   └── type.go
├── service/
│   ├── info.go
│   └── type.go
└── controller/
    ├── admin/
    │   ├── info.go
    │   └── type.go
    └── app/
        └── info.go
```

测试与被测包放在同级 `_test.go` 文件中。`modules/modules_gen.go` 由现有 `cool generate` 流水线更新，不手工维护生成内容。

模块声明保持 Node 元数据：

- 名称：`字典管理`；
- 描述：`数据字典等`；
- 加载顺序：`0`；
- 无模块中间件和全局中间件。

## 4. 数据模型

### 4.1 字典类型

表名固定为 `dict_type`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` | `*gtime.Time` | 框架维护 |
| `updateTime` | `*gtime.Time` | 框架维护 |
| `name` | `string` | 非空字符串 |
| `key` | `string` | 非空字符串 |

不额外增加唯一索引。Node 版没有声明 `key` 唯一约束，Go 版不擅自收紧写入行为。

### 4.2 字典信息

表名固定为 `dict_info`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `id` | `uint64` | 自增主键 |
| `createTime` | `*gtime.Time` | 框架维护 |
| `updateTime` | `*gtime.Time` | 框架维护 |
| `typeId` | `uint64` | 非空 |
| `name` | `string` | 非空字符串 |
| `value` | `*string` | 可空 |
| `orderNum` | `int32` | 默认 `0` |
| `remark` | `*string` | 可空 |
| `parentId` | `*uint64` | 可空，默认 `null` |

不声明数据库外键。类型与树节点的级联行为由 Service 在同一业务事务中完成，以兼容当前跨数据库和删除归档机制。

## 5. HTTP 契约

### 5.1 Admin 字典类型

前缀：`/admin/dict/type`

| 路径 | 方法 | 认证与权限 | 行为 |
| --- | --- | --- | --- |
| `/add` | POST | `dict:type:add` | 新增 |
| `/delete` | POST | `dict:type:delete` | 删除并级联字典项 |
| `/update` | POST | `dict:type:update` | 更新 |
| `/info` | GET | `dict:type:info` | 详情 |
| `/list` | POST | `dict:type:list` | 列表，`keyWord` 模糊匹配 `name` |
| `/page` | POST | `dict:type:page` | 分页 |

### 5.2 Admin 字典信息

前缀：`/admin/dict/info`

| 路径 | 方法 | 认证与权限 | 行为 |
| --- | --- | --- | --- |
| `/add` | POST | `dict:info:add` | 新增 |
| `/delete` | POST | `dict:info:delete` | 删除节点及其全部后代 |
| `/update` | POST | `dict:info:update` | 更新 |
| `/info` | GET | `dict:info:info` | 详情 |
| `/list` | POST | `dict:info:list` | `typeId` 等值、`keyWord` 模糊匹配 `name`，追加 `createTime ASC` |
| `/page` | POST | `dict:info:page` | 分页 |
| `/data` | POST | 仅后台登录 | 按 `types` 聚合字典数据 |
| `/types` | GET | 公开 | 返回全部字典类型 |

`/admin/dict/info/data` 与 Node 一致：必须持有有效后台身份，但不要求菜单权限。它不能标为 `ignoreToken`，否则会错误地变成公开接口。权限推导层对这一条精确路径返回空权限标识，继续由认证层校验登录状态。

### 5.3 App 字典信息

前缀：`/app/dict/info`

| 路径 | 方法 | 认证 | 行为 |
| --- | --- | --- | --- |
| `/data` | POST | 公开 | 按 `types` 聚合字典数据 |
| `/types` | GET | 公开 | 返回全部字典类型 |

两个 App 路由均使用现有 `ignoreToken` 标签。Admin 与 App 共用同一 `InfoService`，不复制查询逻辑。

## 6. Service 行为

### 6.1 聚合数据

`Data(ctx, types)` 遵循以下规则：

1. `types` 为空或未提交时查询全部字典类型；
2. `types` 非空时只查询 `key` 位于集合中的类型；
3. 没有匹配类型时返回空对象 `{}`；
4. 一次查询全部匹配类型的字典项，只选择 `id/name/typeId/parentId/orderNum/value`；
5. 字典项固定按 `orderNum ASC, createTime ASC` 排序；
6. 结果按类型 `key` 分组，每个已匹配类型都返回对应数组；
7. 精确复现源代码的 `value ? Number(value) : value` 和 `isNaN` 判断：`null` 与空字符串保持原值；其他字符串按 ECMAScript `Number` 语义转换，`NaN` 保持原字符串，有限数返回 JSON 数字，正负无穷在 JSON 中返回 `null`；
8. 不在后端构造树。前端继续使用现有 `deepTree` 按 `parentId` 组装。

响应示例：

```json
{
  "occupation": [
    {
      "id": 27,
      "name": "射手",
      "typeId": 20,
      "parentId": null,
      "orderNum": 5,
      "value": 0
    }
  ]
}
```

### 6.2 类型列表

`Types(ctx)` 返回 `dict_type` 全部记录，包含基础时间字段，与 Node Repository `find()` 的数据形状一致。该方法同时服务 Admin 和 App 的 `/types`。

### 6.3 字典名称解析

Service 提供类型安全的单值和批量解析方法：

1. 先按字典类型 `key` 查找类型；不存在时返回未命中结果；
2. 再加载该类型全部字典项；
3. 优先按 `value` 完全匹配；
4. 未匹配时按 ECMAScript `parseInt(value)` 的十进制前缀和 `0x` 十六进制前缀规则解析 ID 后查找；
5. 未命中返回 `nil`；
6. 批量输入保持输入顺序和结果数量。

这对应 Node `getValues` 与 `findValueInDictValues` 的内部能力，不新增 HTTP 路由。

### 6.4 递归删除

字典项删除流程：

1. 读取请求中的根节点 ID 并去重；
2. 分批按 `parentId` 查询直接子节点；
3. 使用已访问集合收集完整后代闭包，避免异常环形数据导致死循环；
4. 将根节点与后代交给现有基础 Service 一次删除；
5. 整个流程运行在 CRUD Dispatcher 已建立的同一数据库事务中。

类型删除流程：

1. 删除请求中的字典类型；
2. 查询这些类型对应的全部字典项 ID；
3. 通过字典信息基础 Service 删除全部匹配项；
4. 类型和字典项删除属于同一事务，任一步失败全部回滚。

Go 版允许内部使用更强的事务一致性，但成功响应、最终数据和前端行为与 Node 一致。

## 7. 初始化、菜单与装配

`modules/dict/db.json` 原样迁移 Node 的两种字典类型和八条字典信息，保留显式 ID、父子关系、排序、空字段和值。

模块不新增 `menu.json`。字典菜单和全部权限项已经位于 `modules/base/menu.json`，重复声明会造成种子冲突。

执行现有生成命令后，静态生成文件负责：

1. 注册 Dict 模块与两个实体 Descriptor；
2. 构造两个基础 Service 和两个业务 Service；
3. 注册三个 Controller；
4. 嵌入 Dict `db.json`；
5. 将 Admin/App 路由和 EPS 元数据发布到现有 Transport。

## 8. 错误与安全

1. 输入绑定、ID 校验、未知字段、分页参数和数据库错误继续使用现有 Controller、Service 和 `exception` 机制；
2. 不吞掉数据库错误，不把内部 SQL 或堆栈暴露给前端；
3. 所有查询使用 GoFrame 参数化 ORM 条件，不拼接用户输入；
4. 空 `types` 是合法请求，不作为校验错误；
5. 删除不存在的节点沿用基础删除行为，不额外引入业务异常；
6. 公开范围严格限定为三个 Node 已公开路由：Admin `types`、App `data`、App `types`。

## 9. 验证

### 9.1 单元与契约测试

至少覆盖：

1. 三个 Controller 的路径、方法、CRUD、查询配置、标签与 EPS 投影；
2. Admin `data` 只免菜单权限而不免登录；
3. 空 `types`、指定类型、未知类型和多类型聚合；
4. 排序和响应字段白名单；
5. `"0"`、整数、浮点数、空字符串、普通字符串和 `null` 的值转换；
6. 单值与批量名称解析，包括 value 命中、ID 回退和未命中；
7. 删除单节点、多根节点、深层后代、异常环形数据；
8. 删除类型时清除其全部字典项且不影响其他类型；
9. 事务中任一步失败时不产生部分删除；
10. 种子 JSON 能由 Descriptor 校验并导入。

### 9.2 工程门禁

实现后依次执行：

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./modules/dict/... -count=1
go test ./... -count=1
go vet ./...
test -z "$(gofmt -l $(rg --files modules/dict -g '*.go') cool-next/auth/permission.go)"
```

如本地数据库环境可用，再启动 Go 服务并用现有前端或 HTTP 请求核验登录、EPS、CRUD、字典刷新和递归删除完整链路。

## 10. 验收标准

1. `cool-admin-vue` 不需要任何代码或配置修改；
2. Node 字典模块现有全部路由在 Go 后端可用；
3. 请求字段、成功响应和错误响应遵循当前 Go v2 已对齐的 Node 协议；
4. 前端字典管理页面可以完成类型和树形字典项的增删改查；
5. 前端启动后的全量字典刷新和按类型刷新结果正确；
6. 普通后台用户可以调用 Admin `data`，但无菜单权限时不能调用受保护 CRUD；
7. 公开路由无需 Token，其他路由不扩大访问范围；
8. EPS 同时发布 Admin 与 App 字典契约；
9. MySQL、PostgreSQL 和 SQLite 使用相同业务代码，不引入方言 SQL；
10. 全量测试、静态检查和格式检查通过。
