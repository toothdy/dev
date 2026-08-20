# cool-admin-go-next Plan2 Schema Sync Design

日期：2026-07-14

## 1. 目标

Plan2 目标是实现 `cool-admin-go-next` 的 Model metadata 与 MySQL 自动建表能力，让后续 seed、CRUD、auth、EPS 都能复用同一份模型元数据。

本阶段必须真实连接 MySQL，并实际创建 base 模块所需表。验收数据库连接固定为：

```text
user: root
password: 123456
database: cool-go
host: 127.0.0.1
port: 3306
```

## 2. 当前上下文

当前仓库已完成：

1. Plan0 协议勘察文档。
2. Plan1 GoFrame v2 项目骨架。
3. `cool/app` 最小 runtime。
4. `cool/module` 模块注册。
5. `cool/response` Node 兼容响应。
6. `modules/base` 模块骨架。
7. `/health` 健康检查。

当前仓库尚未实现：

1. `cool/model`。
2. `cool/db` 或 schema sync。
3. base 表模型元数据。
4. MySQL 真实连接验证。
5. seed、CRUD、auth、EPS。

## 3. 范围

Plan2 包含：

1. 定义 Model metadata 数据结构。
2. 定义字段、索引、表注释、默认值、nullable、主键、自增等元数据。
3. 定义 base 模块第一批表模型。
4. 生成 MySQL DDL。
5. 读取 `information_schema`。
6. 表不存在时创建表。
7. 字段不存在时新增字段。
8. 索引不存在时创建索引。
9. 将 schema sync 接入 `cool/app` 启动编排。
10. 使用真实 MySQL 验证建表结果和幂等性。

Plan2 不包含：

1. `db.json` / `menu.json` 初始化导入。
2. CRUD runtime。
3. 登录、JWT、权限。
4. EPS runtime。
5. 自动删除字段。
6. 自动重命名字段。
7. 自动缩短字段长度。
8. 自动修改主键。
9. 自动删除索引。
10. 数据迁移。

## 4. 架构

Plan2 增加三个主要边界：

```text
modules/base/model
   ↓ 定义 base 表
cool/model
   ↓ 统一元数据
cool/db/schema
   ↓ MySQL DDL + information_schema + sync
cool/app
   ↓ 启动时按配置调用 schema sync
MySQL
```

### 4.1 `cool/model`

`cool/model` 只描述模型，不访问数据库。

职责：

1. 定义 `Definition` 表元数据。
2. 定义 `Field` 字段元数据。
3. 定义 `Index` 索引元数据。
4. 提供基础字段 `id/createTime/updateTime/tenantId`。
5. 保留 JSON/EPS camelCase 字段名。
6. 字段名以 base-api-contract.md 的表结构契为准（camelCase）。早期临时采用 snake_case，现已对齐。
7. 为后续 CRUD 和 EPS 提供统一读取接口。

### 4.2 `cool/db/schema`

`cool/db/schema` 负责真实 MySQL schema 同步。

职责：

1. 把 `model.Definition` 转为 MySQL DDL。
2. 查询 `information_schema.tables` 判断表是否存在。
3. 查询 `information_schema.columns` 判断字段是否存在。
4. 查询 `information_schema.statistics` 判断索引是否存在。
5. 创建缺失表。
6. 新增缺失字段。
7. 新增缺失索引。
8. 对危险差异只返回日志项，不执行破坏性 DDL。

### 4.3 `modules/base/model`

`modules/base/model` 定义第一批 base 表：

1. `base_sys_user`
2. `base_sys_role`
3. `base_sys_menu`
4. `base_sys_department`
5. `base_sys_param`
6. `base_sys_log`
7. `base_sys_conf`
8. `base_sys_user_role`
9. `base_sys_role_menu`
10. `base_sys_role_department`

字段以 `docs/protocol/base-api-contract.md` 的表结构契约为准。HTTP JSON / EPS / DB 字段一律使用 camelCase（与 Node 版共用）。

### 4.4 `cool/app`

`cool/app` 在应用初始化时收集模块模型。如果 `cool.schema.autoSync` 为 `true`，则执行 schema sync。

测试环境需要能显式关闭自动启动 HTTP server，同时单独调用 schema sync。

## 5. 数据模型规则

### 5.1 基础字段

除纯关联表外，base 业务表默认包含：

| JSON/EPS 字段 | DB 字段 | MySQL 类型 | 说明 |
|---|---|---|---|
| `id` | `id` | `bigint unsigned` | 主键，自增 |
| `createTime` | `createTime` | `datetime` | 创建时间 |
| `updateTime` | `updateTime` | `datetime` | 更新时间 |
| `tenantId` | `tenantId` | `bigint unsigned` | 租户 ID，可空，普通索引 |

### 5.2 命名规则

1. JSON/EPS 字段使用 Node 兼容 camelCase。
2. DB 字段使用 camelCase（与 HTTP JSON / EPS 一致，与 Node 版共用）。
3. `source` 字段后续 EPS 生成时使用 `a.camelCaseName`。
4. metadata 中必须同时保留 JSON 字段名和 DB 字段名。

### 5.3 MySQL 类型

Plan2 至少支持：

1. `bigint unsigned`
2. `int`
3. `tinyint`
4. `varchar(n)`
5. `text`
6. `longtext`
7. `datetime`
8. `json`

### 5.4 默认值

Plan2 支持：

1. 字符串默认值。
2. 数字默认值。
3. 布尔默认值映射为 `0/1`。
4. `CURRENT_TIMESTAMP` 原始表达式。
5. 无默认值。

## 6. 自动同步规则

### 6.1 允许的操作

1. `CREATE TABLE`。
2. `ALTER TABLE ADD COLUMN`。
3. `CREATE INDEX`。
4. `CREATE UNIQUE INDEX`。

### 6.2 禁止的操作

1. 删除表。
2. 删除字段。
3. 重命名字段。
4. 缩短字段长度。
5. 修改主键。
6. 删除索引。
7. 数据迁移。

### 6.3 幂等性

同一批模型连续执行两次 sync：

1. 第一次创建缺失表、字段、索引。
2. 第二次不应执行重复 DDL。
3. 第二次不应报错。

## 7. GoFrame 使用约束

1. 使用 GoFrame v2 的数据库组件连接 MySQL。
2. 使用 `g.DB()` 或可注入的 `gdb.DB` 执行 SQL。
3. 原生 SQL 用于 DDL 和 `information_schema` 查询。
4. 错误使用 GoFrame `gerror.Wrap` 增加上下文。
5. 不手写 GoFrame 自动生成的 `dao/do/entity` 文件。
6. 不使用 `logic/` 目录。
7. 业务逻辑后续直接放入 `service/`，本阶段不创建业务 service。

## 8. 配置

`manifest/config/config.yaml` 的数据库连接需要更新为 Plan2 验收库：

```yaml
database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
    debug: true
```

`cool.schema.autoSync` 继续控制是否自动同步：

```yaml
cool:
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
```

## 9. 验收标准

Plan2 完成时必须满足：

1. `go test ./...` 通过。
2. 真实 MySQL 中存在 `cool-go` 数据库。
3. 执行 schema sync 后，10 张 base 表存在。
4. `base_sys_user` 至少包含 `id`、`department_id`、`user_id`、`name`、`username`、`password`、`password_v`、`nick_name`、`head_img`、`phone`、`email`、`remark`、`status`、`socket_id`、`create_time`、`update_time`、`tenant_id`。
5. `base_sys_menu` 至少包含 `id`、`parent_id`、`name`、`router`、`perms`、`type`、`icon`、`order_num`、`view_path`、`keep_alive`、`is_show`、`create_time`、`update_time`、`tenant_id`。
6. `base_sys_conf` 至少包含 `id`、`c_key`、`c_value`、`create_time`、`update_time`、`tenant_id`。
7. `base_sys_user_role`、`base_sys_role_menu`、`base_sys_role_department` 关联表存在。
8. 连续执行两次 schema sync 不报错。
9. `/health` 仍返回 Node 兼容成功响应。
10. 工作区只包含 Plan2 相关文件变更。

## 10. 测试策略

### 10.1 单元测试

覆盖：

1. `cool/model` metadata 构造。
2. 基础字段生成。
3. base model 数量。
4. 关键字段 camelCase 与 snake_case 映射。
5. MySQL DDL 生成。
6. 索引 DDL 生成。

### 10.2 集成测试

集成测试要求真实 MySQL：

1. 使用 `root:123456@tcp(127.0.0.1:3306)/cool-go`。
2. 若数据库不存在，测试或验收命令需要提示用户创建数据库。
3. sync 后查询 `information_schema.tables`。
4. sync 后查询 `information_schema.columns`。
5. sync 后查询 `information_schema.statistics`。
6. 重复执行 sync 验证幂等。

### 10.3 运行验证

运行：

```bash
go test ./...
go run .
curl -s http://127.0.0.1:8001/health
```

并执行专门的 schema sync 验收命令或测试。

## 11. 风险与处理

| 风险 | 处理 |
|---|---|
| 本地 MySQL 未启动 | 明确报错，要求启动 MySQL 后重跑 |
| `cool-go` 数据库不存在 | 明确提示执行 `CREATE DATABASE \`cool-go\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;` |
| DDL 与已有表冲突 | 只做安全新增，危险差异记录为 diff，不自动修改 |
| GoFrame 自动生成目录误创建 | Plan2 禁止创建 `dao/do/entity` |
| 后续 EPS 需要 camelCase | metadata 同时保留 JSON 字段名和 DB 字段名 |

## 12. 后续衔接

Plan2 产出的 model metadata 将被后续阶段复用：

1. Plan3 seed/menu 导入使用表名和字段映射。
2. Plan4 CRUD runtime 使用字段、主键、忽略字段和查询字段。
3. Plan6 EPS runtime 使用字段注释、类型、默认值、dict 和 camelCase 字段名。
