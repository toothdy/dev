# Base 业务模块迁移实施计划

> 日期：2026-08-15
> 依据：`docs/superpowers/specs/2026-08-15-base-module-business-migration-design.md`
> 源码基线：`cool-admin-midway/src/modules/base` 与 `cool-admin-vue/src/modules/base`
> 状态：实施中

## 1. 目标

在现有 Go v2 静态模块、Descriptor、CRUD、Auth、Session、HTTP 和事务体系上迁移 Base 业务模块。除设计第 2.3 节列出的有意差异外，HTTP 路径、方法、参数名、响应字段、EPS 和业务数据以 Midway Base 源码及当前前端调用为准。

最终交付：

1. 10 张 `base_sys_*` 表及三数据库 Schema；
2. Base 全部目标 Controller、Service、初始化、权限、上传、日志和开发工具；
3. 可运行的生成 HTTP Installer 和模块装配；
4. Midway 契约测试、并发安全测试和 MySQL/PostgreSQL/SQLite 集成测试。

## 2. 实施约束

1. 不运行 `gf init`，不改变现有自定义工程结构；
2. 不新增第三方依赖，优先使用标准库、GoFrame v2.10.2 和仓库已有组件；
3. 不创建 `logic/`，业务逻辑只放 `modules/base/service/`；
4. 数据库写入只使用生成 DO、Descriptor、`gdb.Model` 和现有事务 Runtime，不使用 `g.Map`；
5. 不手写 DAO、DO、Descriptor 或 `modules/modules_gen.go`，统一由 `cool generate` 生成；
6. 每个任务先写失败测试，再做最小实现；每次完整修改一个文件后再处理下一个文件；
7. 每个任务的专项测试失败时立即停止，不带着失败进入下一任务；
8. 每个任务单独提交，提交只包含该任务列出的文件及必要生成文件。

## 3. GoFrame 使用决策

1. 事务统一使用 `DB.Transaction(ctx, func(context.Context, gdb.TX) error)`，通过闭包返回错误触发回滚；
2. 授权并发边界使用事务内 `LockUpdate()`，批量 ID 先去重并升序排列；不使用 `LockUpdateSkipLocked()`，因为权限写入必须等待而不是跳过用户；
3. 验证码和参数缓存各自持有 `gcache.New()` 实例，避免全局缓存键污染；缓存过期使用 `time.Duration`；
4. 日志清理使用模块自有 `gcron.Cron` 和具名 `AddSingleton` 任务，停止时移除任务并等待正在执行的清理结束；
5. 上传文件不直接开放通用目录浏览。Base 受控 Handler 对可信媒体使用内联文件响应，其他文件使用附件响应并设置 `nosniff`；
6. 所有日志调用传递请求或任务的 `context.Context`，复用 GoFrame Trace ID。

## 4. 任务清单

### 任务 1：生成 HTTP 路由真正可执行

文件：

- 修改：`cool-next/codegen/route_analysis.go`
- 修改：`cool-next/codegen/render.go`
- 修改：`cool-next/codegen/route_analysis_test.go`
- 修改：`cool-next/codegen/render_test.go`
- 修改：`cool-next/core/controller/controller.go`
- 修改：`cool-next/core/controller/controller_test.go`
- 修改：`cool-next/core/route/route.go`
- 修改：`cool-next/core/route/route_test.go`
- 创建：`cool-next/core/controller/http.go`
- 创建：`cool-next/core/controller/http_test.go`
- 生成：`modules/modules_gen.go`

步骤：

1. 为自定义 Handler 增加无 DTO 签名支持：`func(context.Context) error` 和 `func(context.Context) (T, error)`；保留现有带 `*DTO` 签名；
2. 在 Controller HTTP Adapter 中统一完成 DTO 绑定、CRUD Plan、事务 Dispatcher、Handler 调用和响应值写回；
3. 让 codegen 生成闭包式 `generatedHTTPInstaller`，捕获已构造的 Binder、Dispatcher、Auth 和 Controller Service；
4. 按静态 Route Table 安装 CRUD、自定义路由、模块/Controller/Route 中间件和权限元数据；
5. Controller 和 Route 增加明确的 `DevelopmentOnly` 静态元数据；Installer 使用 GoFrame `gmode.IsDevelop()` 决定是否绑定开发路由，生产环境不注册这些 Handler；
6. 禁止反射路由发现和运行时容器，只允许生成代码调用已分析的具体函数；
7. 生成失败、DTO 绑定失败或中间件依赖缺失时返回稳定诊断，不产生半成品生成文件。

验证：

```text
go test ./cool-next/core/controller ./cool-next/codegen -count=1
go test -race ./cool-next/core/controller ./cool-next/codegen -count=1
go run ./cmd/cool generate
go run ./cmd/cool check
```

停止条件：Golden Server 中至少一条 CRUD、一条带 DTO 和一条无 DTO 路由可真实请求，且旧的固定 `return nil` Installer 不再存在。

### 任务 2：通用原始响应

文件：

- 修改：`cool-next/core/controller/response.go`
- 修改：`cool-next/core/controller/response_test.go`

步骤：

1. 增加明确的 `HTMLResponse` 和 `FileResponse` 原始响应结果，不进入统一 JSON 包装；
2. `FileResponse` 只承载调用方已经决定的文件、文件名、`Content-Type`、内联或附件方式及安全响应头，不知道 `/upload`、上传根目录或 Base 的 MIME 策略；
3. 响应适配器拒绝无效文件响应，错误不泄露本地路径。

验证：

```text
go test ./cool-next/core/controller -count=1
```

停止条件：HTML 和文件响应按调用方提供的元数据原样写出，且都不进入 JSON 包装。

### 任务 3：JSON 字段和批量 Session 撤销

文件：

- 修改：`cool-next/core/entity/field.go`
- 修改：`cool-next/core/entity/types.go`
- 修改：`cool-next/core/entity/compile.go`
- 修改：`cool-next/core/entity/validate.go`
- 修改：`cool-next/codegen/entity_validate.go`
- 修改：`cool-next/codegen/do_emit.go`
- 修改：`cool-next/db/driver/type_mapping.go`
- 修改：`cool-next/db/driver/types.go`
- 修改：`cool-next/db/driver/ddl.go`
- 修改：`cool-next/db/schema/expected.go`
- 修改：`cool-next/db/schema/inspect.go`
- 修改：`cool-next/db/schema/diff.go`
- 修改：上述包对应 `_test.go`
- 修改：`cool-next/auth/token.go`
- 修改：`cool-next/auth/session/session.go`
- 修改：`cool-next/auth/session/adapter.go`
- 修改：`cool-next/auth/session/memory.go`
- 修改：`cool-next/auth/session/redis.go`
- 修改：上述 Session 对应 `_test.go`

步骤：

1. 仅为显式标记的 slice/map Entity 字段生成 JSON Descriptor 和强类型 DO 支持；普通 slice/map 仍拒绝；
2. 三个方言分别映射到已有 JSON 能力，读写使用结构化值，不在业务 Service 手工拼 JSON 字符串；
3. Session 端口增加 `RevokeUser` 和 `RevokeUsers`，单用户方法委托批量方法；
4. Memory Store 在一次持锁遍历中删除目标用户 Session；
5. Redis Store 只执行一次 `SCAN`，使用目标用户集合匹配并分批删除；
6. 批量撤销允许部分用户被额外登出，但任何存储错误必须返回，供数据库事务回滚。

验证：

```text
go test ./cool-next/core/entity ./cool-next/codegen ./cool-next/db/driver ./cool-next/auth/... -count=1
go test -race ./cool-next/auth/... -count=1
```

停止条件：JSON 字段 Descriptor/DO 测试通过，Redis 多用户撤销测试断言只有一次 Key 扫描。

### 任务 4：Base 配置、实体和 Schema

文件：

- 创建：`modules/base/config.go`
- 创建：`modules/base/entity/base.go`
- 创建：`modules/base/entity/user.go`
- 创建：`modules/base/entity/role.go`
- 创建：`modules/base/entity/menu.go`
- 创建：`modules/base/entity/department.go`
- 创建：`modules/base/entity/param.go`
- 创建：`modules/base/entity/conf.go`
- 创建：`modules/base/entity/log.go`
- 创建：`modules/base/entity/user_role.go`
- 创建：`modules/base/entity/role_menu.go`
- 创建：`modules/base/entity/role_department.go`
- 创建：`modules/base/entity/entity_test.go`
- 修改：`manifest/config/config.yaml`
- 生成：`modules/modules_gen.go`

步骤：

1. 配置只包含已使用字段：JWT TTL、本地上传根目录和公开基础 URL、10 MB 上限、App 参数 `allowKeys`、验证码和日志清理设置；
2. 建立 10 张 `base_sys_*` Entity，保持 Midway 字段名和默认值，不增加 `tenantId`；
3. 菜单和部门加入 nullable unique `seedKey`；关系表建立必要的唯一索引和查询索引，引用完整性由 Base 事务维护，不扩展当前不具备的通用外键 DSL；
4. 角色 JSON 兼容字段使用任务 3 的 JSON Descriptor，不参与授权判定；
5. 在 Entity 测试中固定密码和 `seedKey` 为敏感维护字段，任务 11 的全部 Controller 必须分别应用 hidden 或 hidden/readonly 策略；
6. 生成代码并检查唯一生成文件，不手写 DO 或 Descriptor。

验证：

```text
go test ./modules/base/entity ./cool-next/codegen -count=1
go run ./cmd/cool generate
go run ./cmd/cool check
```

停止条件：10 张表的期望 Schema 在三方言编译测试中稳定，无 `tenantId`、手写 DAO 或额外表。

### 任务 5：初始数据与幂等初始化

文件：

- 创建：`modules/base/data/db.json`
- 创建：`modules/base/data/menu.json`
- 创建：`modules/base/service/initializer.go`
- 创建：`modules/base/service/initializer_test.go`

步骤：

1. 从 Midway Base 原始 JSON 复制完整业务数据，只增加显式 `seedKey`，不改菜单、参数和初始账号业务字段；
2. 通过 `username/label/keyName/cKey/seedKey` 对齐，不依赖固定自增 ID；
3. 在一个事务中按父子顺序建立部门、菜单和关系的实际 ID 映射；
4. 初始管理员缺失时生成 bcrypt cost 12 的 `123456` 密码；已有用户不得重置密码、`passwordV` 或资料；
5. 启动时补齐缺失记录、同步 seed 菜单和部门字段，不覆盖现有参数、配置、用户和角色；
6. JSON 删除 seed 项不自动删库，清理必须由显式迁移完成；
7. 初始化器只实现 `module.Initializer`，不增加通用迁移框架。

验证：

```text
go test ./modules/base/service -run 'TestInitializer' -count=1
```

停止条件：空库初始化、重复初始化、已有业务数据和 JSON 删除四组测试通过，管理员密码不会在重启时变化。

### 任务 6：认证、验证码和授权边界

文件：

- 创建：`modules/base/dto/login.go`
- 创建：`modules/base/dto/token.go`
- 创建：`modules/base/service/captcha.go`
- 创建：`modules/base/service/login.go`
- 创建：`modules/base/service/permission.go`
- 创建：`modules/base/service/auth_boundary.go`
- 创建：上述文件对应 `_test.go`

步骤：

1. 登录 DTO 固定四个字符串字段，刷新 DTO 固定 `refreshToken`；响应 DTO 固定 `token/expire/refreshToken/refreshExpire` 和 TTL 秒单位；
2. 验证码使用 `crypto/rand`、模块私有 `gcache.Cache` 和进程内互斥，成功最多消费一次；
3. 登录按用户名定位候选用户后，在事务内 `LockUpdate()` 用户行并重新读取状态、密码、`passwordV` 和角色；
4. 刷新按 Token 用户 ID 锁同一用户行，在锁内重新构造 Identity 并原子轮换 Session；
5. 授权只读取权威关系表，`role.label == "admin"` 全局放行，普通权限按逗号拆分；
6. 授权变更统一复用 `auth_boundary.go`：先稳定角色/菜单关系，再按用户 ID 升序锁用户，批量撤销 Session 后提交；
7. 不复制 Midway 用户名超管、MD5、非原子验证码和可重放刷新逻辑。

验证：

```text
go test ./modules/base/service -run 'TestCaptcha|TestLogin|TestRefresh|TestPermission|TestAuthorization' -count=1
go test -race ./modules/base/service -run 'TestCaptcha|TestAuthorization' -count=1
```

停止条件：并发验证码最多一次成功；登录/刷新与权限变更并发测试不能产生携带旧权限的新 Session。

### 任务 7：用户、角色、菜单和部门业务

文件：

- 创建：`modules/base/dto/user.go`
- 创建：`modules/base/dto/role.go`
- 创建：`modules/base/dto/menu.go`
- 创建：`modules/base/dto/department.go`
- 创建：`modules/base/service/user.go`
- 创建：`modules/base/service/role.go`
- 创建：`modules/base/service/menu.go`
- 创建：`modules/base/service/department.go`
- 创建：上述 Service 对应 `_test.go`

步骤：

1. 用现有 `service.Base` 承担普通 CRUD，只覆写关系替换、虚拟字段、数据权限和保护规则；
2. 用户新增/更新与用户角色关系同事务，密码只在非空时 bcrypt，响应永不返回摘要；
3. 用户 `page` 返回真实 `roleIds/roleName`，`info` 返回 `roleIdList/departmentName`；
4. 角色菜单、部门关系和兼容 JSON 字段同事务更新，授权读取只信关系表；
5. 部门列表和用户分页使用相同的直接部门范围与自建数据规则；
6. 菜单和部门递归删除先锁定并验证完整引用范围，不留下孤立关系；
7. 最后一个有效管理员保护在事务锁内完成，禁止删除、禁用或破坏 `admin` 角色；
8. 所有权限变化调用任务 6 的统一边界并批量撤销 Session。

验证：

```text
go test ./modules/base/service -run 'TestUser|TestRole|TestMenu|TestDepartment|TestLastAdmin' -count=1
go test -race ./modules/base/service -run 'TestLastAdmin|TestAuthorization' -count=1
```

停止条件：四类关系回滚、数据权限、字段响应和管理员并发保护测试全部通过。

### 任务 8：参数、HTML 和本地上传

文件：

- 创建：`modules/base/service/param.go`
- 创建：`modules/base/service/upload.go`
- 创建：`modules/base/service/param_test.go`
- 创建：`modules/base/service/upload_test.go`

步骤：

1. 参数缓存使用模块私有 `gcache.Cache`，变更后删除旧键并回填新键；
2. `dataType` 按 Midway 返回 JSON 值、原字符串、HTML 或逗号文件列表；
3. HTML 缓存未命中查库回填，返回 `text/html` 原始响应；
4. 上传 DTO 保留 `file` 和可选 basename `key`，默认上限 10 MB；
5. 使用 `crypto/rand` 生成缺省文件名，临时文件与目标同目录，原子重命名且禁止覆盖；
6. 返回 `<publicBaseURL>/upload/<YYYYMMDD>/<name>`，路径和错误不暴露上传根目录；
7. 文件读取只接受严格的 `YYYYMMDD` 日期目录和 basename 文件名，拒绝绝对路径、路径分隔符、`.`、`..`、NUL、目录和目录索引；
8. 解析目标路径后再次验证上传根目录边界，拒绝目标或任一现有父路径通过符号链接越界；
9. 读取文件内容并探测实际类型，只有扩展名与内容匹配的 JPEG/PNG/GIF/WebP/MP3/WAV/MP4/WebM 使用对应 `Content-Type` 内联；其他文件使用 `application/octet-stream` 和附件方式；
10. 使用任务 2 的通用 `FileResponse` 返回文件并设置 `X-Content-Type-Options: nosniff`，不建立通用静态文件服务。

验证：

```text
go test ./modules/base/service -run 'TestParam|TestHTML|TestUpload' -count=1
```

停止条件：前端提交的 `key` 可用，分域 URL 正确；覆盖、半文件、超限、目录、符号链接和路径越界测试全部失败且无残留；可信媒体内联，其他文件强制下载。

### 任务 9：操作日志、清理任务和翻译

文件：

- 创建：`modules/base/middleware/log.go`
- 创建：`modules/base/middleware/translate.go`
- 创建：`modules/base/service/log.go`
- 创建：`modules/base/service/conf.go`
- 创建：`modules/base/service/translate.go`
- 创建：`modules/base/service/log_job.go`
- 创建：上述文件对应 `_test.go`

步骤：

1. 操作日志中间件只记录设计允许的 `/admin/**` 业务请求；
2. 使用结构化字段白名单递归脱敏密码、Token、验证码、Authorization 和文件内容，并限制保存大小；
3. 日志写库失败只通过 `g.Log()` 记录，不改变业务响应；
4. 日志分页左连用户并返回 `name`，`clear/setKeep/getKeep` 保持 Midway 契约；
5. 使用模块自有 `gcron.Cron.AddSingleton` 注册每日清理，生命周期停止时移除任务并等待退出；
6. 系统运行信息只写 `glog`，不写 `base_sys_log`；
7. 翻译中间件只处理 Base 消息和菜单字段，不建立全局 i18n 框架。

验证：

```text
go test ./modules/base/middleware ./modules/base/service -run 'TestLog|TestTranslate' -count=1
go test -race ./modules/base/service -run 'TestLogJob' -count=1
```

停止条件：敏感数据和系统日志不会进入 `base_sys_log`，清理任务不可重入且可干净停止。

### 任务 10：菜单导入导出与开发代码工具

文件：

- 创建：`modules/base/service/menu_tool.go`
- 创建：`modules/base/service/coding.go`
- 创建：`modules/base/service/menu_tool_test.go`
- 创建：`modules/base/service/coding_test.go`

步骤：

1. 菜单导出递归生成不含 `id/createTime/updateTime/parentId/seedKey` 的树；导入在事务中重新建立父子 ID；
2. 菜单 `parse` 使用 `go/parser`、`go/ast` 和 `go/token` 静态提取 Entity/Controller 信息，禁止编译或执行输入；
3. 菜单 `create` 与 Coding `createCode` 共用一个包内私有的安全 Go 文件写入函数，不增加公共代码工作台抽象；
4. `getModuleTree` 保持当前 Vue 依赖的平铺 `string[]`，稳定排序且只列出含合法 `config.go` 的模块；输出只允许可信工作区内 `modules/**/*.go`，拒绝绝对路径、`..`、符号链接越界和已有文件覆盖；
5. 写入前 `go/format`，批量请求先全量预检，再使用同目录临时文件和原子无覆盖发布；普通 `os.Rename` 在 Unix 会覆盖目标，不能用于这一约束；
6. Service 不读取运行环境或决定路由可见性，开发专用和 `admin` 限制统一由任务 11 的静态 Controller 元数据处理。

验证：

```text
go test ./modules/base/service -run 'TestMenuTool|TestCoding' -count=1
```

停止条件：源码输入永不执行，路径攻击和覆盖已有文件全部失败且无临时文件残留。

### 任务 11：Base HTTP Controller、装配与前端契约

文件：

- 创建：`modules/base/controller/admin/open.go`
- 创建：`modules/base/controller/admin/comm.go`
- 创建：`modules/base/controller/admin/upload.go`
- 创建：`modules/base/controller/admin/coding.go`
- 创建：`modules/base/controller/admin/sys/user.go`
- 创建：`modules/base/controller/admin/sys/role.go`
- 创建：`modules/base/controller/admin/sys/menu.go`
- 创建：`modules/base/controller/admin/sys/department.go`
- 创建：`modules/base/controller/admin/sys/param.go`
- 创建：`modules/base/controller/admin/sys/log.go`
- 创建：`modules/base/controller/app/comm.go`
- 创建：`modules/base/controller/contract_test.go`
- 修改：`cool-next/eps/eps.go`
- 修改：`cool-next/eps/eps_test.go`
- 生成：`modules/modules_gen.go`

步骤：

1. 按设计第 7 节完整声明开放、后台通用、App、CRUD 和自定义路由；公开文件 Controller 使用 `controller.Admin("upload")` 和现有 `IgnoreGlobalPrefix` 静态元数据声明 `GET /upload/{date}/{name}`，不新增 Public DSL；
2. `/upload/{date}/{name}` 公开访问，`/admin/base/comm/**` 只要求后台身份，`/app/base/comm/upload*` 要求 App 身份；
3. CRUD 权限使用 `base:sys:<resource>:<action>`，菜单四个工具接口只允许 `admin` 角色；
4. `personUpdate` 只绑定白名单 DTO；`program` 返回 `Go`；HTML 使用原始响应；
5. `menu/parse`、`menu/create` 和 `/admin/base/coding/**` 标记 `DevelopmentOnly`；生产 Installer 不绑定这些 Handler，实际请求得到 404；
6. 以静态表驱动测试逐条对照 Midway 路径、方法、参数名和响应字段，并单列设计第 2.3 节差异；
7. 运行生成器装配 Base 配置、Descriptor、Service、中间件、Initializer、Cron、Controller 和 HTTP Transport；
8. 确认 EPS 中密码不可见，`seedKey` 仅 hidden/readonly；开发环境显示开发路由，生产 EPS 响应过滤 `DevelopmentOnly` 路由。

验证：

```text
go test ./modules/base/controller/... ./cool-next/eps -count=1
go run ./cmd/cool generate
go run ./cmd/cool check
```

停止条件：全部目标路由真实安装，`GET /upload/{date}/{name}` 位于根路径且不带 `/admin`，生产开发路由为 404，Token、用户、权限菜单、上传读取、上传模式和 EPS 契约测试通过。

### 任务 12：三数据库 HTTP 集成与契约回归

文件：

- 修改：`modules/modules_gen.go`（只通过生成器）
- 创建：`test/integration/base/base_test.go`
- 创建：`test/integration/base/contracts_test.go`
- 修改：`test/integration/run.sh`
- 修改：`test/integration/compose.yaml`（仅在现有服务不能复用时）

步骤：

1. 检查任务 11 生成图已完整装配 Base 配置、10 个 Descriptor、Base Service、Auth/Session、Controller、中间件、Initializer、Cron 和 HTTP Transport；
2. 启动 SQLite HTTP 测试服务，执行验证码、登录、刷新、个人信息、权限菜单和 CRUD 冒烟；
3. 对 Token DTO、用户虚拟字段、上传 URL、公开文件读取的内联/附件响应及越界拒绝、HTML、EPS 和错误状态做端到端断言；
4. MySQL、PostgreSQL、SQLite 分别验证 Schema、初始化两次、关系事务、JSON 字段和管理员保护；
5. 并发执行登录/刷新与角色或菜单权限变更，验证旧权限 Session 不可产生；
6. Redis Store 使用现有测试替身验证批量撤销一次扫描，不在 Base 集成测试引入真实 Redis 容器；
7. 对照设计第 2.3 节逐项确认有意差异，其他行为以 Midway 源码契约表为准。

验证：

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./test/integration/base -count=1
test/integration/run.sh
```

停止条件：三数据库全部通过，同一生成结果连续执行两次无差异，开发/生产路由集合符合设计。

### 任务 13：全量质量门禁

步骤：

1. 只对本计划修改的 Go 文件执行 `gofmt -w`；
2. 运行 Base、框架、Race、Vet、生成新鲜度和三数据库测试；
3. 检查工作区只有计划内文件，无手写生成文件、无 `tenantId`、无 MD5、无 `eval`、无 `g.Map` 数据库写入；
4. 将设计和本计划状态更新为已完成；
5. 输出 Midway 路由对照结果和第 2.3 节有意差异结果。

验证：

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
make check
test/integration/run.sh
git diff --check
rg -n 'tenantId|md5\(|eval\(|g\.Map' modules/base
```

最终停止条件：所有命令成功；任何失败都修复在所属任务，不通过跳过测试、降低断言或增加兼容分支完成迁移。

## 5. 提交顺序

建议提交顺序与任务一致：

```text
feat(http): install generated controller routes
feat(http): support raw html and file responses
feat(core): support base json fields and session revocation
feat(base): add entities and schema
feat(base): initialize seed data
feat(base): implement authentication and authorization
feat(base): implement system management services
feat(base): implement params html and upload
feat(base): implement operation logs and translation
feat(base): implement menu and coding tools
feat(base): expose midway-compatible controllers
test(base): cover assembly and database contracts
chore(base): complete migration verification
```

每次提交前运行该任务专项命令；任务 4 起额外运行 `go run ./cmd/cool check`，禁止提交过期的 `modules/modules_gen.go`。
