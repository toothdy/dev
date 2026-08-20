# cool-admin-go-next Plan3 Seed/Menu Design

日期：2026-07-15

## 1. 目标

Plan3 目标是实现 `cool-admin-go-next` 的初始化数据导入能力，让 Plan2 已创建的 base 表具备可登录、可加载菜单、可计算权限的基础数据。

本阶段完成后，数据库至少应包含：

1. 默认 `admin` 用户。
2. 默认超管角色。
3. 默认部门树。
4. 默认系统参数和系统配置。
5. 默认菜单和按钮权限。
6. 用户角色、角色菜单、角色部门关联数据。
7. `base_sys_conf` 中的初始化标记。

Plan3 只解决初始化数据和菜单导入，不实现登录接口、权限计算接口、CRUD runtime 或 EPS runtime；但导入结果必须为后续 Plan4/Plan5/Plan6 提供可用数据基础。

## 2. 当前上下文

当前仓库已完成：

1. Plan0 协议勘察文档。
2. Plan1 GoFrame v2 项目骨架。
3. `cool/app` 最小 runtime。
4. `cool/module` 模块注册。
5. `cool/response` Node 兼容响应。
6. `modules/base` 模块骨架。
7. `/health` 健康检查。
8. `cool/model` 模型元数据。
9. `modules/base/model` base 表定义。
10. `cool/db/schema` MySQL schema sync。
11. `cool/app` schema sync 启动 hook。
12. 真实 MySQL 自动建表验收。

当前仓库尚未实现：

1. `cool/seed`。
2. `db.json` 真实初始化数据。
3. `menu.json` 真实菜单数据。
4. `cool/app` seed/menu 启动 hook。
5. 初始化数据幂等导入。
6. 登录、JWT、权限菜单、CRUD、EPS。

Plan3 验收数据库连接沿用 Plan2：

```text
user: root
password: 123456
database: cool-go
host: 127.0.0.1
port: 3306
```

## 3. 范围

Plan3 包含：

1. 定义 `cool/seed` 初始化导入边界。
2. 扩展模块接口，声明 seed 文件路径和开关。
3. 读取模块顺序，按模块执行初始化导入。
4. 解析 `modules/base/db.json`。
5. 解析 `modules/base/menu.json`。
6. 支持 `@childDatas` 子数据。
7. 支持子数据引用父级字段。
8. 支持 camelCase JSON 字段到 snake_case DB 字段映射。
9. 使用 `base_sys_conf` 写入 `init_db_base` 和 `init_menu_base` 标记。
10. 保证重复启动不会重复插入初始化数据。
11. 在 `cool/app` 中接入 seed/menu hook，并受配置控制。
12. 使用真实 MySQL 验证导入结果。

Plan3 不包含：

1. 登录接口实现。
2. JWT 生成和校验。
3. token middleware。
4. 权限树计算接口。
5. CRUD runtime。
6. EPS runtime。
7. 文件上传。
8. 前端联调。
9. 数据更新型迁移。
10. 删除或覆盖用户已有业务数据。

## 4. 架构

Plan3 增加两个主要边界：

```text
modules/base/db.json
modules/base/menu.json
   ↓
cool/module seed metadata
   ↓
cool/seed parser + importer
   ↓
cool/model field mapping
   ↓
GoFrame gdb transaction
   ↓
MySQL base tables
```

### 4.1 `cool/seed`

`cool/seed` 负责初始化数据导入，不负责业务 CRUD。

职责：

1. 定义 seed 配置和模块 seed 元数据。
2. 读取 JSON 文件。
3. 校验目标表是否存在于已注册 model metadata。
4. 将 JSON camelCase 字段映射为 DB snake_case 字段。
5. 构建参数化 INSERT SQL。
6. 处理父子数据。
7. 写入初始化标记。
8. 保证导入过程在事务中完成。
9. 返回导入结果用于日志和测试断言。

### 4.2 `cool/module`

模块需要声明 seed 文件：

```text
base module
   ├── db.json
   └── menu.json
```

建议模块接口在 Plan3 扩展：

```go
type SeedDefinition struct {
   DBPath   string
   MenuPath string
}
```

模块定义需要提供：

```text
ModuleSeeds() seed.Definition
```

`modules/base` 返回：

```text
DBPath: modules/base/db.json
MenuPath: modules/base/menu.json
```

### 4.3 `cool/app`

`cool/app` 启动顺序在 Plan3 后应为：

```text
加载配置
   ↓
注册模块
   ↓
收集 models
   ↓
按配置执行 schema sync
   ↓
按配置执行 seed db 导入
   ↓
按配置执行 seed menu 导入
   ↓
注册路由
   ↓
启动 HTTP server
```

关键约束：

1. seed 必须在 schema sync 之后执行。
2. seed 必须在后续 auth/CRUD/EPS 初始化前执行。
3. 测试环境必须能注入 fake seed runner。
4. 启动 HTTP server 仍可关闭，便于单元测试。

## 5. 配置

沿用现有 `manifest/config/config.yaml` 的 cool 配置：

```yaml
cool:
  initDB: true
  initMenu: true
  initJudge: "db"
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
```

Plan3 规则：

1. `cool.initDB = true` 时允许导入 `db.json`。
2. `cool.initMenu = true` 时允许导入 `menu.json`。
3. `cool.initJudge = "db"` 时使用 `base_sys_conf` 标记判断是否已初始化。
4. 其他 `initJudge` 值本阶段不扩展；遇到未知值应返回清晰错误。
5. 测试可以通过 options 显式覆盖配置读取，避免真实导入。

## 6. 初始化标记

Plan3 使用 `base_sys_conf` 作为幂等标记表。

| 模块 | 数据类型 | c_key | c_value |
|---|---|---|---|
| base | db | `init_db_base` | `time consuming: <duration>` |
| base | menu | `init_menu_base` | `time consuming: <duration>` |

规则：

1. 导入前检查 `base_sys_conf.c_key` 是否存在。
2. 标记存在则跳过对应导入。
3. 标记不存在则执行导入。
4. 导入成功后写入标记。
5. 导入失败必须回滚标记和本次数据。
6. DB 导入和 menu 导入分别独立判断、独立事务。

## 7. `db.json` 设计

### 7.1 文件位置

```text
modules/base/db.json
```

### 7.2 推荐结构

Plan3 推荐采用表名分组结构，便于和 `cool/model` 对齐：

```json
{
  "base_sys_department": [
    {
      "id": 1,
      "name": "COOL",
      "parentId": null,
      "orderNum": 0,
      "@childDatas": {
        "base_sys_department": [
          {
            "id": 2,
            "name": "开发",
            "parentId": "@id",
            "orderNum": 1
          }
        ]
      }
    }
  ],
  "base_sys_role": [
    {
      "id": 1,
      "name": "超管",
      "label": "admin",
      "relevance": 1
    }
  ],
  "base_sys_user": [
    {
      "id": 1,
      "departmentId": 1,
      "username": "admin",
      "password": "e10adc3949ba59abbe56e057f20f883e",
      "passwordV": 1,
      "name": "管理员",
      "nickName": "管理员",
      "status": 1
    }
  ],
  "base_sys_user_role": [
    {
      "userId": 1,
      "roleId": 1
    }
  ]
}
```

### 7.3 必备数据

`db.json` 至少需要包含：

1. `base_sys_user`
   - `username = admin`
   - `password = e10adc3949ba59abbe56e057f20f883e`
   - `passwordV = 1`
   - `status = 1`
2. `base_sys_role`
   - `label = admin`
3. `base_sys_user_role`
   - 绑定 admin 用户和 admin 角色。
4. `base_sys_department`
   - 至少包含 `COOL`、`开发`、`测试`、`游客`。
5. `base_sys_param`
   - 后续前端或系统默认参数依赖项。
6. `base_sys_conf`
   - `logKeep`
   - `recycleKeep`

### 7.4 字段规则

1. JSON 字段使用 Node 兼容 camelCase。
2. DB 插入前通过 model metadata 转为 snake_case。
3. 未出现在 model metadata 中的字段默认报错，不静默忽略。
4. `@childDatas` 是导入控制字段，不写入数据库。
5. `null` 值允许写入可空字段。
6. 未提供 `createTime/updateTime/tenantId` 时允许使用数据库默认或空值策略。
7. Plan3 不引入 GoFrame 自动生成 DO 文件；动态 seed 插入使用 metadata 构建参数化 SQL。
8. 禁止使用字符串拼接写入用户可控值，只允许字段名来自 model metadata，字段值走参数绑定。

## 8. `menu.json` 设计

### 8.1 文件位置

```text
modules/base/menu.json
```

### 8.2 推荐结构

`menu.json` 使用树结构，父子菜单字段为 `childMenus`：

```json
[
  {
    "id": 1,
    "parentId": null,
    "name": "系统管理",
    "router": "/system",
    "perms": null,
    "type": 0,
    "icon": "icon-settings",
    "orderNum": 1,
    "viewPath": null,
    "keepAlive": 0,
    "isShow": 1,
    "childMenus": [
      {
        "id": 2,
        "parentId": "@id",
        "name": "用户管理",
        "router": "/system/user",
        "perms": null,
        "type": 1,
        "icon": "icon-user",
        "orderNum": 1,
        "viewPath": "modules/base/views/user/index.vue",
        "keepAlive": 1,
        "isShow": 1,
        "childMenus": [
          {
            "id": 3,
            "parentId": "@id",
            "name": "用户新增",
            "router": null,
            "perms": "base:sys:user:add",
            "type": 2,
            "orderNum": 1,
            "isShow": 0
          }
        ]
      }
    ]
  }
]
```

### 8.3 必备菜单

`menu.json` 至少需要覆盖 `docs/protocol/base-api-contract.md` 中第一阶段 base CRUD prefixes 对应的后台页面：

1. 用户管理：`base:sys:user:*`
2. 角色管理：`base:sys:role:*`
3. 菜单管理：`base:sys:menu:*`
4. 部门管理：`base:sys:department:*`
5. 参数管理：`base:sys:param:*`
6. 操作日志：`base:sys:log:*`

按钮权限至少覆盖通用 CRUD：

1. `add`
2. `delete`
3. `update`
4. `info`
5. `list`
6. `page`

如 Node/Vue 实际还依赖 `move`、`parse`、`create`、`export`、`import`、`order`、`clear`、`setKeep`、`getKeep`、`html`，Plan3 数据应预留对应按钮权限，后续接口实现可在 Plan4/Plan5 补齐。

### 8.4 菜单字段规则

1. `type = 0` 表示目录。
2. `type = 1` 表示菜单页面。
3. `type = 2` 表示按钮权限。
4. `router` 用于前端路由。
5. `perms` 用于权限码。
6. `orderNum` 用于菜单排序。
7. `isShow` 缺省时前端视为显示，但导入数据应显式写 `1/0`。
8. `childMenus` 是导入控制字段，不写入数据库。
9. 子菜单 `parentId = "@id"` 表示引用父菜单真实 ID。

## 9. 父子数据和引用规则

Plan3 需要支持两类父子结构：

1. `db.json` 中的 `@childDatas`。
2. `menu.json` 中的 `childMenus`。

### 9.1 `@childDatas`

`@childDatas` 的值是一个对象：

```json
{
  "@childDatas": {
    "base_sys_department": [
      {
        "parentId": "@id",
        "name": "开发"
      }
    ]
  }
}
```

规则：

1. key 是目标表名。
2. value 是子记录数组。
3. 父记录先插入。
4. 子记录后插入。
5. 子记录可以用 `@字段名` 引用父记录字段值。
6. 如父记录未显式提供 `id`，需要使用插入后返回 ID 支持 `@id`。

### 9.2 `childMenus`

`childMenus` 只用于 `menu.json`：

1. 父菜单先插入。
2. 子菜单递归插入。
3. `parentId = "@id"` 自动替换为父菜单 ID。
4. 插入后的菜单 ID 用于后续 `base_sys_role_menu` 授权绑定。

### 9.3 引用限制

1. Plan3 只支持引用直接父级字段。
2. 引用格式仅支持完整字符串 `"@fieldName"`。
3. 不支持字符串模板，例如 `"prefix-@id"`。
4. 引用不存在字段必须报错。
5. 跨兄弟记录引用不支持。
6. 跨文件引用不支持。

## 10. 插入和事务规则

### 10.1 执行顺序

推荐 DB 初始化顺序：

1. `base_sys_department`
2. `base_sys_role`
3. `base_sys_user`
4. `base_sys_user_role`
5. `base_sys_param`
6. `base_sys_conf`
7. 其他表

推荐 menu 初始化顺序：

1. 插入 `base_sys_menu` 树。
2. 收集所有菜单 ID。
3. 为 admin 角色插入 `base_sys_role_menu`。
4. 如需要部门权限，插入 `base_sys_role_department`。
5. 写入 `init_menu_base` 标记。

### 10.2 事务

1. DB 初始化使用一个事务。
2. Menu 初始化使用一个事务。
3. 任意一条记录失败则回滚本次事务。
4. DB 初始化成功但 Menu 初始化失败时，DB 标记保留，Menu 下次可继续重试。
5. 事务使用 GoFrame `gdb.Transaction` 或等价事务闭包。

### 10.3 SQL 安全

1. 表名必须来自 `model.Definition.TableName`。
2. 字段名必须来自 `model.Field.ColumnName`。
3. 值必须使用参数绑定。
4. JSON 文件中的未知表名必须报错。
5. JSON 文件中的未知字段必须报错。
6. 不执行来自 JSON 的任意 SQL。

## 11. 幂等性

Plan3 的幂等性以初始化标记为准：

1. 第一次执行：导入数据并写入标记。
2. 第二次执行：发现标记后跳过。
3. 跳过不应报错。
4. 跳过应返回结果，例如 `SkippedDB = true` 或日志项。
5. 如果用户手动删除标记但保留数据，本阶段允许因唯一键冲突报错；这属于人工干预后的数据状态问题。
6. 后续如需要更强幂等，可在 Plan4+ 增加按唯一键 upsert，但 Plan3 不默认覆盖已有数据。

## 12. 错误处理

错误必须使用 GoFrame `gerror.Wrap` 增加上下文。

常见错误：

| 场景 | 处理 |
|---|---|
| seed 文件不存在 | 返回包含模块名和文件路径的错误 |
| JSON 格式错误 | 返回包含文件路径的解析错误 |
| 未知表名 | 返回表名和模块名 |
| 未知字段 | 返回表名、字段名和文件路径 |
| `@` 引用不存在 | 返回引用名和父记录摘要 |
| 目标表不存在 | 提示先执行 schema sync |
| MySQL 连接失败 | 保留 GoFrame 错误栈 |
| 插入失败 | 返回表名、字段列表和原始错误 |
| 写初始化标记失败 | 回滚事务并返回错误 |

## 13. `cool/app` 接入规则

Plan3 建议扩展 `app.Options`：

```text
AutoInitDB
AutoInitMenu
UseConfigInit
SeedRunner
```

或者保持最小变更，使用一个统一 runner：

```text
SeedRunner(ctx, modules, models) error
```

测试要求：

1. 能创建 app 但不启动 HTTP server。
2. 能注入 fake schema runner。
3. 能注入 fake seed runner。
4. 能断言 schema sync 在 seed 前执行。
5. 能断言 `cool.initDB/initMenu` 关闭时不执行 seed。

启动阶段默认行为：

1. `app.Run(ctx)` 读取配置。
2. `cool.schema.autoSync` 为 true 时先同步表结构。
3. `cool.initDB` 为 true 时导入 DB seed。
4. `cool.initMenu` 为 true 时导入 menu seed。
5. 任一启动初始化错误应阻止应用继续启动。

## 14. 验收标准

Plan3 完成时必须满足：

1. `go test ./...` 通过。
2. 真实 MySQL 中存在 `cool-go` 数据库。
3. schema sync 后 10 张 base 表存在。
4. 第一次 seed 导入后 `base_sys_user` 存在 `username = admin`。
5. admin 密码为 `e10adc3949ba59abbe56e057f20f883e`。
6. `base_sys_role` 存在 `label = admin`。
7. `base_sys_user_role` 存在 admin 用户和 admin 角色关联。
8. `base_sys_department` 至少存在 `COOL`、`开发`、`测试`、`游客`。
9. `base_sys_conf` 存在 `logKeep` 和 `recycleKeep`。
10. `base_sys_conf` 存在 `init_db_base` 标记。
11. `base_sys_conf` 存在 `init_menu_base` 标记。
12. `base_sys_menu` 存在用户、角色、菜单、部门、参数、日志相关菜单。
13. `base_sys_menu` 存在对应按钮权限记录。
14. `base_sys_role_menu` 为 admin 角色绑定全部初始化菜单。
15. 连续执行两次初始化不会重复插入数据。
16. `/health` 仍返回 Node 兼容成功响应。
17. 工作区只包含 Plan3 相关文件变更。

## 15. 测试策略

### 15.1 单元测试

覆盖：

1. seed 文件路径解析。
2. JSON 解析。
3. 表名校验。
4. 字段名 camelCase 到 snake_case 映射。
5. 未知字段报错。
6. `@childDatas` 递归展开。
7. `childMenus` 递归展开。
8. `@id` 父字段引用。
9. 初始化标记 key 生成。
10. 配置关闭时跳过 runner。

### 15.2 集成测试

集成测试要求真实 MySQL：

1. 使用 `root:123456@tcp(127.0.0.1:3306)/cool-go`。
2. 先执行 schema sync。
3. 再执行 DB seed。
4. 再执行 Menu seed。
5. 查询关键表和关键记录。
6. 重复执行验证跳过。

建议集成测试通过环境变量显式开启：

```bash
COOL_SEED_INTEGRATION=1 go test ./cool/seed ./cool/app -count=1
```

### 15.3 运行验证

运行：

```bash
go test ./...
go run .
curl -s http://127.0.0.1:8001/health
```

并执行只读 SQL 验证：

```sql
SELECT username, password, status FROM base_sys_user WHERE username = 'admin';
SELECT label FROM base_sys_role WHERE label = 'admin';
SELECT c_key FROM base_sys_conf WHERE c_key IN ('init_db_base', 'init_menu_base');
SELECT COUNT(*) FROM base_sys_menu;
SELECT COUNT(*) FROM base_sys_role_menu;
```

## 16. 风险与处理

| 风险 | 处理 |
|---|---|
| 初始化文件结构与 Node 版存在差异 | 以 `docs/protocol/base-api-contract.md` 和前端实际消费为准，必要时更新 fixtures |
| 菜单权限不完整导致后续页面无权限 | Plan3 先覆盖 base CRUD 和已知自定义 API 权限 |
| 手动删除初始化标记后重复导入冲突 | 报清晰唯一键冲突，不自动覆盖已有数据 |
| `@childDatas` 引用过度复杂 | Plan3 只支持直接父级字段引用，避免实现过度复杂 |
| GoFrame DO 文件尚未生成 | Plan3 动态 seed 使用 metadata + 参数化 SQL，不手写 `dao/do/entity` |
| schema sync 未先执行 | app 启动顺序固定为 schema sync → seed；单独执行 seed 时检测表存在 |
| menu 导入失败导致 DB seed 已完成 | DB 和 Menu 分别事务、分别标记，下次只重试 Menu |

## 17. 后续衔接

Plan3 产出的 seed/menu 数据将被后续阶段复用：

1. Plan4 CRUD runtime 使用初始化数据验证 page/list/info/add/update/delete。
2. Plan5 auth 使用 admin 用户、角色、菜单和权限数据完成登录与权限菜单。
3. Plan6 EPS runtime 使用菜单权限和 controller metadata 生成前端可消费 EPS。
4. Plan7 前端联调使用真实初始化菜单验证页面显示和权限按钮。
