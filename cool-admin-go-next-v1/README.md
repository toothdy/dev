# cool-admin-go-next

Go 版 cool-admin next，目标是用 GoFrame v2 实现与 Node 版 cool-admin-midway 兼容的后端体验。

## 当前阶段

当前仓库处于阶段 6C：EPS 与图片验证码运行时。

已完成：

1. Go module 初始化。
2. GoFrame v2 依赖。
3. `cool/module` 模块注册骨架。
4. `modules/base` 模块骨架。
5. `cool/response` Node 兼容响应结构。
6. `/health` 健康检查。
7. `cool/model` 模型元数据。
8. `modules/base/model` base 表定义。
9. MySQL schema sync：创建表、新增字段、新增索引。
10. `cool/seed` 初始化数据导入器。
11. `modules/base/db.json` 默认初始化数据。
12. `modules/base/menu.json` 默认菜单和权限数据。
13. app 启动 seed/menu hook。
14. `cool/crud` metadata-driven CRUD runtime。
15. base 模块核心 CRUD 路由。
16. `cool/auth` 密码、token、上下文和 middleware。
17. base login / refreshToken / 图片验证码接口。
18. base person / logout / program 接口。
19. auth middleware 保护非放行路由。
20. `GET /admin/base/comm/permmenu`。
21. admin 全菜单和权限码。
22. 普通用户角色菜单权限。
23. base CRUD 权限 middleware。

未完成：

1. Vue 前端联调。

## Auth 验收

执行 auth 相关验收测试：

```bash
go test ./cool/auth ./modules/base ./cool/app -count=1
go test ./...
COOL_AUTH_INTEGRATION=1 go test ./modules/base -run AuthIntegration -count=1
```

手工启动应用后，先请求验证码并从图片读取 4 位数字；随后使用返回的 `captchaId` 与该验证码验证登录和当前用户接口：

```bash
curl 'http://127.0.0.1:8001/admin/base/open/captcha?height=45&width=150&color=%232c3142'

curl -X POST http://127.0.0.1:8001/admin/base/open/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"123456","captchaId":"<captchaId>","verifyCode":"<图片中的4位验证码>"}'

curl http://127.0.0.1:8001/admin/base/comm/person \
  -H 'Authorization: <token>'
```

## 权限菜单和 CRUD 权限验收

执行权限相关验收测试：

```bash
go test ./modules/base -count=1
go test ./cool/auth ./cool/app ./modules/base -count=1
go test ./...
COOL_PERMISSION_INTEGRATION=1 go test ./modules/base -run PermissionIntegration -count=1
```

手工启动应用后，可使用以下命令验证权限菜单和 CRUD 权限：

```bash
curl http://127.0.0.1:8001/admin/base/comm/permmenu \
  -H 'Authorization: <token>'

curl -X POST http://127.0.0.1:8001/admin/base/sys/user/page \
  -H 'Content-Type: application/json' \
  -H 'Authorization: <token>' \
  -d '{"page":1,"size":15}'
```

## EPS Bootstrap 验收

EPS 是匿名全量 bootstrap 描述，不按登录用户、角色或权限裁剪：

```bash
go test ./cool/eps ./modules/base -count=1
curl http://127.0.0.1:8001/admin/base/open/eps
```

期望响应为 `code: 1000`，`data.base` 包含 base open、comm 和 sys CRUD Controller；每个 API 使用相对 `path` 与独立 `prefix`。

## 首批自定义 API 验收

启动应用后，已登录用户可使用个人资料和本地上传接口；sys 管理接口还需要对应菜单权限：

```bash
go test ./modules/base/service ./modules/base/controller ./cool/app -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run TestCustomAPIIntegration -count=1
curl http://127.0.0.1:8001/admin/base/comm/uploadMode -H 'Authorization: <token>'
```

`uploadMode` 返回 `{"mode":"local","type":"local"}`。本地上传仅接受 multipart 字段 `file`，单文件最大 10MB，成功返回 `/uploads/YYYYMMDD/<随机文件名>`；上传目录不会提供目录列表。`user/move`、`department/order`、日志三个管理功能缺权限时返回 HTTP 403 与 `登录失效或无权限访问~`。

`COOL_CUSTOM_API_INTEGRATION=1` 只能指向专用测试 MySQL 数据库，不能在默认开发库或共享库执行。该显式集成测试会重置 base fixture（包含账号、菜单、权限配置）并清空 `base_sys_log`，以验证日志 `clear` 的全表契约。

## 字典模块验收

`dict` 模块对齐 Node `dict` 模块，提供 `dict_type` 与 `dict_info` 的 CRUD、`GET /admin/dict/info/types`（免登录，供前端 `cool-eps` 生成 `DictKey` 类型）与 `POST /admin/dict/info/data`（登录即放行，无细粒度权限）。字典菜单与权限码 `dict:info:*`、`dict:type:*` 已由 `modules/base/menu.json` 提供。

```bash
go test ./modules/dict ./modules ./cool/app -count=1
COOL_DICT_INTEGRATION=1 go test ./modules/dict -run TestDictIntegrationTypesDataAndCRUD -count=1
```

`/data` 接收可选 `{types: ["key"]}`，返回 `{<key>: [{id,typeId,name,parentId,orderNum,value}]}`，按 `orderNum`、`createTime` 升序，`value` 能转数字则转 number。删除 `dict_type` 事务级联清理其 `dict_info`；删除 `dict_info` 递归清理 `parentId` 子项。

`COOL_DICT_INTEGRATION=1` 只能指向专用测试 MySQL 数据库，会重置 base 与 dict fixture。

## 图片验证码验收

验证码为匿名接口。先启动应用：

```bash
go run ./cmd/cool run
```

请求图片验证码：

```bash
curl 'http://127.0.0.1:8001/admin/base/open/captcha?height=45&width=150&color=%232c3142'
```

期望 `code: 1000`，内层 `data.captchaId` 非空，`data.data` 以 `data:image/svg+xml;base64,` 开头。登录时必须提交该 `captchaId` 与图片中的 4 位 `verifyCode`；验证码 30 分钟后失效，正确使用一次即消费。

## 协议契约

Go 版第一阶段必须以以下文档为准兼容现有前端：

```text
docs/protocol/base-api-contract.md
```

代表性响应 fixture 位于：

```text
docs/protocol/fixtures/
```

## 启动

环境要求：Go 1.26 或更高版本。CI 始终使用最新的 Go 1.26.x 补丁版本。

```bash
go run ./cmd/cool run
```

默认监听：

```text
:8001
```

健康检查：

```bash
curl http://127.0.0.1:8001/health
```

正式构建入口会先校验并更新目录约定生成的模块装配代码：

```bash
go run ./cmd/cool generate
go run ./cmd/cool check
go run ./cmd/cool build
```

原生 `go build` 和 `go test` 直接使用仓库中已提交的 `modules_gen.go`，不会自动扫描源码。根 `config.go` 声明模块、`module.<key>` 配置驱动集中生成。开发模块时应先运行 `cool generate`，CI 使用 `cool check` 拒绝过期输出。详细目录协议见 `docs/module-development.md`。

## 模块目录协议

`modules` 的每个直接子目录是一个模块。生成器仅依赖目录、导出函数名和 Go 类型签名，不要求 `register.go`、`entity/models.go`、`module.yaml` 或注释指令。

| 目录 | 组件 |
| --- | --- |
| `entity/**` | 模型定义 |
| `service/**` | 业务服务、普通 Provider 和 Task HandlerDefinition |
| `controller/**` | Controller |
| `middleware/global/**` | 全局中间件 |
| 其他 `middleware/**` | 当前模块路由中间件 |
| `event/**` | 事件与 Runtime |
| `schedule/**` | 定时任务 Runtime |
| `queue/**` | 队列 |
| `dto/**` | 数据传输对象 |
| 模块根 `config.go` | `ModuleConfig() module.Declaration[Config]` |
| `db.json` / `menu.json` | 可选初始化数据 |

模块内部可以任意深度嵌套。`_test.go`、`testdata`、隐藏目录和标准生成文件不参与发现。自动组件不得放在 GOOS/GOARCH 专属文件中。

最小模块只需创建模块目录和符合签名的组件：

```text
modules/report/
├── controller/admin/report.go
├── entity/report.go
└── service/report.go
```

执行 `go run ./cmd/cool generate` 后，生成器会在 `modules/` 下创建 `modules_gen.go`（集中生成全部模块的装配代码），并将新模块加入 `modules.Specs()`。生成文件必须提交，但不得手工修改。

## MySQL 自动建表验收

Plan2 使用真实 MySQL 验收，默认连接：

```text
mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4
```

如果数据库不存在，先执行：

```sql
CREATE DATABASE `cool-go` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

执行 schema sync 集成测试：

```bash
COOL_SCHEMA_INTEGRATION=1 go test ./cool/db/schema -run TestSyncerCreatesTableAndIsIdempotent -count=1
```

## seed/menu 初始化验收

启动应用时会按配置自动执行初始化：

```yaml
cool:
  schema:
    autoSync: true
  initDB: true
  initMenu: true
```

执行 seed/menu 真实 MySQL 集成测试：

```bash
COOL_SEED_INTEGRATION=1 go test ./cool/seed -count=1
```

该测试会：

1. 同步 base 表结构。
2. 清理 seed 相关 base 数据。
3. 导入 `modules/base/db.json`。
4. 导入 `modules/base/menu.json`，其中包含“数据管理”下的回收站菜单。
5. 验证 admin 用户、admin 角色、初始化标记、角色菜单和角色部门绑定。
6. 重复导入并验证第二次会按初始化标记跳过。

关键数据：

```text
admin username: admin
admin login password: 123456
admin password storage: bcrypt
init db marker: init_db_base
init menu marker: init_menu_base
```

启动应用时会按 `cool.schema.autoSync`、`cool.initDB`、`cool.initMenu` 自动同步 base 表结构并导入初始化数据：

```bash
go run ./cmd/cool run
```

期望响应：

```json
{
  "code": 1000,
  "message": "success",
  "data": {
    "status": "ok"
  }
}
```

## CRUD runtime 验收

当前已注册 base 模块核心 CRUD 路由：

```text
/admin/base/sys/user
/admin/base/sys/role
/admin/base/sys/menu
/admin/base/sys/department
/admin/base/sys/param
/admin/base/sys/log
```

普通测试不连接 MySQL：

```bash
go test ./cool/crud ./modules/base -count=1
go test ./...
```

启动应用并验证 user page：

```bash
go run ./cmd/cool run
curl -X POST http://127.0.0.1:8001/admin/base/sys/user/page \
  -H 'Content-Type: application/json' \
  -d '{"page":1,"size":15}'
```

期望响应仍使用 Node 兼容包络，分页 data 为 `{ list, pagination }`，且用户列表不返回 `password`。

## 测试

```bash
go test ./...
```

## 远端仓库

```text
https://github.com/toothdy/cool-admin-go-next
```
