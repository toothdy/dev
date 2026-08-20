# Base 模块设计（v2）

| 项 | 值 |
|---|---|
| 状态 | 草案，待评审 |
| 日期 | 2026-08-20 |
| 适用范围 | `modules/base/` 及其依赖的 `cool-next/` 框架能力 |
| 上位文档 | `2026-07-31-cool-admin-go-next-architecture-design.md`（总架构，冲突时以其为准） |
| 行为参考 | `cool-admin-midway/src/modules/base/`（Node 业务） |
| 框架参考 | `cool-admin-midway-packages/core/src/`（Node 框架公共能力） |

本文档回答三件事：Node 版 base 到底有哪些功能；Go v2 已经做到哪一步；**哪些能力不该留在 base 里**。第三项是本次重构的主线——当前 base 的多处偏离并非写错，而是框架层缺能力后业务模块自行兜底的结果。

---

## 1. Node 版 base 功能清单

### 1.1 HTTP 接口面

| 控制器 | 接口 | 说明 |
|---|---|---|
| `admin/open` | `GET /eps` | 实体与路径信息，免 Token |
| | `GET /html?key=` | 富文本参数内容，免 Token |
| | `POST /login` | 登录，免 Token |
| | `GET /captcha` | 图形验证码，免 Token |
| | `POST /refreshToken` | 刷新令牌，失败返回 401 |
| `admin/comm` | `GET /person` | 个人信息 |
| | `POST /personUpdate` | 修改个人信息（含改密） |
| | `GET /permmenu` | 权限 + 菜单树 |
| | `POST /upload`、`GET /uploadMode` | 文件上传（走 plugin） |
| | `POST /logout` | 退出 |
| | `GET /program` | 返回运行时标识，免 Token |
| `admin/sys/user` | CRUD + `POST /move` | 移动部门 |
| `admin/sys/role` | CRUD | 分页带数据范围过滤（非超管仅见自建或自有角色，且隐藏 `admin` 角色） |
| `admin/sys/menu` | CRUD + `parse` / `create` / `export` / `import` | 代码解析与菜单导入导出 |
| `admin/sys/department` | add/delete/update/list + `POST /order` | 部门排序 |
| `admin/sys/param` | CRUD + `GET /html` | 参数配置 |
| `admin/sys/log` | page + `clear` / `setKeep` / `getKeep` | 操作日志 |
| `admin/coding` | `getModuleTree` / `createCode` | 仅 local 环境可用 |
| `app/comm` | `GET /param`（受 `allowKeys` 白名单约束）、`GET /eps`、`upload`、`uploadMode` | App 端 |

### 1.2 Service 能力

- **login**：登录、验证码生成与校验、令牌签发与刷新、登出（`sso` 开关决定是否单点）
- **user**：分页（联查部门/角色）、`move`、`person`、`personUpdate`、`add`/`update`（保护 `admin` 账号）、`forbidden`（禁用即踢下线）、角色关系维护
- **role**：`getByUser`、`updatePerms`（角色菜单 + 部门范围）、info/list/page 数据范围过滤
- **menu**：list、`getPerms`、`getMenus`、delete（级联子菜单）、`refreshPerms`（刷新在线用户权限）、`parse`、`create`、`export`、`import`
- **perms**：`refreshPerms`、`isAdmin`、`permmenu`、`departmentIds`（**基于缓存的数据权限范围**）
- **department**：list（按角色过滤）、`getByRoleIds`、`order`、delete（含用户转移/清理）
- **param**：`dataByKey`、`htmlByKey`、`addOrUpdate`
- **conf**：`getValue` / `updateVaule`（键值配置，也承载导入锁）
- **log**：`record`、`clear`（按 `logKeep` 保留天数）
- **coding**：模块树、生成代码（仅 local）
- **translate**：多语言加载与翻译（i18n）

### 1.3 事件与任务

- `event/menu.ts`：监听 `onMenuImport` → 调 `menuService.import()` 写库；监听 `onServerReady` → 加载翻译
- `event/app.ts`：监听 `onServerReady` → 打包模式下打开浏览器
- `job/log.ts`：日志清理定时任务

### 1.4 框架侧支撑（关键）

Node 把下面两项放在 **`@cool-midway/core`**，不是 base：

| 框架组件 | 位置 | 职责 |
|---|---|---|
| `CoolModuleImport` | `core/src/module/import.ts:141` | 读 `<modulePath>/db.json`，按表名分组逐条插入；支持 `@childDatas` 递归子表、`@字段名` 引用父记录字段；PostgreSQL 插入后修正序列；用 lock 文件或 `base_sys_conf` 的 `init_db_<module>` 行做幂等守卫；受 `cool.initDB` / `cool.initJudge` 控制 |
| `CoolModuleMenu` | `core/src/module/menu.ts:92` | 读 `<modulePath>/menu.json`，**只负责读取与幂等守卫**，随后 `emit('onMenuImport', datas)`，真正写库由 base 的事件处理器完成 |

**结论：种子数据与菜单的「发现 + 读取 + 幂等守卫」是框架能力；「写成业务表」才是业务能力。** 这条边界是本次整改的核心依据。

---

## 2. Go v2 现状比对

### 2.1 已对齐

实体、Service、Controller 三层与 Node 基本一一对应，字段命名约定（`orm` lowerCamelCase、`json`、`description`、`cool` 标签、`PasswordV`↔`passwordV`）完全达标。Go 侧另有 Node 没有的收敛：`AuthorizationBoundary` 统一了授权变更的加锁与 Session 撤销顺序，避免 Node 里分散的竞态。

### 2.2 缺口

| Node 能力 | Go 现状 | 处置 |
|---|---|---|
| `perms.departmentIds` 数据权限范围 | **无对应实现** | 需补；`role.updatePerms` 已写入部门范围，但查询侧未消费 |
| `user.forbidden` 禁用即踢下线 | 无独立方法（`UpdateWithRoles` 内含 Session 撤销） | 确认语义是否已覆盖 |
| `menu.refreshPerms` 刷新在线用户权限 | 无对应实现 | 需补或明确不做 |
| `jwt.sso` 单点登录开关 | 无 | 需确认是否纳入 |
| `cool.initDB` / `initMenu` / `initJudge` | 无 | 随种子导入下沉一并设计 |

### 2.3 越界（README 明示不在 v2 范围）

| 能力 | Go 实现 |
|---|---|
| i18n / 翻译中间件 | `middleware/translate.go`、`service/translate.go` |
| 上传业务接口 | `service/upload.go`、`controller/admin/upload.go` |

这两项与 README 的范围表述冲突。鉴于 base 模块本身也被 README 列为「不在 v2 范围」而实际已开发，此处判定为 **README 范围表述已过期**，需要单独定夺是保留实现还是移除；本文档不预设结论。

---

## 3. 职责划分

### 3.1 原则

> 业务模块只表达「这个系统有什么业务」；凡是「模块通用机制」一律属框架层。

判定法：**换一个业务模块还需要吗？** 需要 → 框架层。

按此重审当前 base 的四处偏离，全部指向同一根因——框架欠账：

| 现象 | 根因 | 归属 |
|---|---|---|
| 自建 `data/` 目录 + `go:embed` + 549 行 `initializer.go` | 框架无种子导入能力 | **框架** |
| `service/log_job.go` 直连 `gcron` | 框架 `schedule` 能力未落地 | **框架** |
| `auth_boundary.go` 自造行锁并用 `Unscoped()` 规避 `updateTime` | 框架无行锁原语 | **框架**（已修，见 §5.1） |
| 全局中间件放在 `middleware/` 而非 `middleware/global/` | 纯目录违规 | 业务 |

### 3.2 种子数据导入下沉（重点）

**现状问题**：`modules/base/service/initializer.go` 把「读 JSON、解析、建树、写库、保证幂等」全部实现在业务模块内，共 549 行。其中只有极小部分是 base 特有的（管理员初始口令、菜单树结构），其余对任何模块都通用。同时它依赖的 `data/` 目录不在模块目录白名单内，导致该目录下的 Go 文件被 `codegen/analyze.go:196` 静默跳过，**逃逸了 CG098/CG099 等全部 AST 级校验**（校验器 `query_validate.go:21` 遍历的正是白名单过滤后的集合）。

**Go 的约束**：v2 不做运行时扫描或反射，因此不能照搬 Node 的 `fs.readFileSync(modulePath)`。种子文件必须在**编译期**进入产物。

**方案**：

1. **发现与嵌入**——`cool generate` 扫描模块根的 `db.json` / `menu.json`（README 模块目录协议已规定此位置，Node 亦同），将内容作为字节切片写入 `modules/modules_gen.go`，挂到该模块的 `ModuleDefinition` 上。`cool-next/core/module/graph.go:46` 的 `ModuleDefinition` 需新增字段：

   ```go
   type ModuleDefinition struct {
       // ... 现有字段
       DBSeed   []byte // 模块根 db.json，无则为 nil
       MenuSeed []byte // 模块根 menu.json，无则为 nil
   }
   ```

   这样业务模块不再需要任何 `go:embed` 代码，`data/` 目录随之消失，`config.go` 也不必承载种子访问器。

2. **导入执行**——新增框架包 `cool-next/seed`，提供：
   - 通用表驱动导入：按表名分组插入，对齐 Node 的 `@childDatas` 递归与 `@字段` 父引用语义
   - 幂等守卫：框架自建内部表 `cool_seed_lock`，按模块记录 `init_db_<module>` / `init_menu_<module>`。**不复用 base 的 `base_sys_conf`**——那会让框架层反向依赖业务表，违反 `cool-next/*` 不得 import `modules/*` 的方向约束，也会使种子导入能力绑死在 base 存在与否上。Node 的 file lock 模式（`initJudge=file`）**不予对齐**：容器重建即丢失、多副本各写各的会导致重复导入，v2 只保留数据库守卫
   - 事务：整个模块的导入在单个框架事务内完成，失败整体回滚
   - 配置开关：对应 Node 的 `cool.initDB` / `cool.initMenu`（`initJudge` 不引入，见上）

3. **业务保留部分**——base 只保留无法通用化的逻辑：管理员初始口令的 bcrypt 哈希、菜单树写入（复用已有 `MenuToolService.Import`）。预计 `initializer.go` 可从 549 行降到百行以内。

**待定**：幂等守卫所用的表由框架自建（如 `cool_seed_lock`）还是复用 base 的 `base_sys_conf`。前者更干净且不让框架依赖业务表，倾向前者；需评审确认。

### 3.3 调度下沉

`service/log_job.go` 直连 `gcron` 是对未落地框架能力的兜底。总架构已冻结 Schedule 的职责边界但未定义 Definition/Handler 协议。**在 Schedule 专项设计落地前，不强行迁移**；仅先将其移入协议规定的 `schedule/` 目录，待框架能力就绪后再改造为框架调度组件。

---

## 4. 目录协议整改

`modules/base` 对 README「模块目录协议 (v2)」的偏离及处置：

| 偏离 | 处置 |
|---|---|
| `data/` 不在白名单，且逃逸静态校验 | 随 §3.2 消除；同时硬化 `isAllowedDirectory`，未知目录改为报错（新诊断码 CG111）而非静默跳过 |
| 定时任务位于 `service/` 而非 `schedule/` | 迁至 `schedule/`（见 §3.3） |
| 全局中间件位于 `middleware/` 而非 `middleware/global/` | 迁至 `middleware/global/`，`ModuleConfig` 引用改为点号路径 |
| 无 `event/` 目录 | 待框架 Event 能力落地后再评估 |

**README 自身需修正的两处**（文档错、代码对）：

1. `config.go` 模板中的 `module.Ref("middleware/global.NewTrace")` 会被 `cool-next/core/module/declaration.go:83` 的 `validateRef` 拒绝——该函数按 `.` 分段并要求每段是合法 Go 标识符，`middleware/global` 含斜杠不合法。而 `codegen/module.go:196-199` 用**点号**拼接包路径。正确写法为 `module.Ref("middleware.global.NewTrace")`。
2. 模块目录协议表缺 `grpc`，但 `codegen/analyze.go:210` 的白名单含之。

---

## 5. 实施计划

### 5.1 已完成

**框架行锁原语**——新增 `cool-next/db/lock.go`，提供 `Runtime.LockRows(ctx, table, ids)`：MySQL/PostgreSQL 走 `SELECT ... FOR UPDATE`，SQLite 无行级锁则以不触碰业务列的空更新提升为写事务再回读；`Unscoped()` 收在框架内，用于阻止空更新把 `updateTime` 顶掉（`gdb_model_update.go:55` 证实该语义）。base 侧 `auth_boundary.go` 改为调用该原语，四处 `lockAuthorizationRows` 调用点统一，并顺带消除了 `base_sys_department` 表名硬编码（改从 `Descriptor().Table()` 取）。

结果：`cool check` 首次全绿（此前被 CG099 阻塞，导致 `cool generate` 无法执行）。

### 5.2 待执行（按依赖顺序）

| # | 事项 | 依赖 |
|---|---|---|
| 1 | `ModuleDefinition` 增加种子字段，`cool generate` 发现并嵌入模块根 JSON | — |
| 2 | 新增 `cool-next/seed`：表驱动导入 + 幂等守卫 + 事务 | 1 |
| 3 | `initializer.go` 瘦身，删除 `data/` 相关残留 | 2 |
| 4 | `isAllowedDirectory` 硬化为报错（CG111） | 3（否则自身会先报错） |
| 5 | 全局中间件迁 `middleware/global/` | — |
| 6 | 定时任务迁 `schedule/` | — |
| 7 | README 修正两处矛盾 | — |
| 8 | 补 §2.2 功能缺口 | 评审确认范围后 |

### 5.3 验收

每步均须 `make check` 全绿（含 `cool check` 的生成新鲜度与静态契约校验）。注意仓库当前 `.gitignore` 排除了 `*_test.go` 与 `/test/`，本地无单测可依赖，因此静态门禁是唯一自动化保障——不得以「改动简单」为由跳过。

---

## 6. 已决事项

| 事项 | 决定 | 日期 |
|---|---|---|
| 种子幂等守卫存放位置 | 框架自建 `cool_seed_lock`，不复用 `base_sys_conf` | 2026-08-20 |
| Node 的 `initJudge=file` lock 模式 | 不对齐，只做数据库守卫 | 2026-08-20 |
| 设计文档是否入库 | 放开 `.gitignore` 的 `/docs/` 规则，文档随代码版本化 | 2026-08-20 |

## 7. 未决事项

1. i18n 与上传接口是否保留（§2.3）
2. `jwt.sso` 单点登录是否纳入 v2
3. 数据权限范围（`departmentIds`）的实现形态——Node 依赖缓存，Go 侧需确定是否引入等价机制
4. `event/` 目录与框架 Event 能力的落地时机（§4）
