# cool-admin-go-next 通用租户作用域实施计划

> **For agentic workers:** 严格按任务顺序实施。每项生产行为先写失败测试，再完成最小实现。不得回退或提交工作区中不属于本计划的改动。

**Goal:** 为所有 BaseFields-backed 通用 CRUD 资源提供默认启用、稳定且无额外数据库查询的租户隔离，同时保留平台 `tenant_id=NULL` 的 Midway 兼容行为，并将缺失上下文与显式跨租户操作分离。

**Architecture:** 认证层使用不可变的三态 TenantIdentity 表示 Missing、Platform 和 Tenant；`cool/tenant` 将认证身份和派生 override 解析为 Missing、Platform、Tenant、Bypass。CRUD Resource 在启动期编译租户元数据，Runtime 在结构化 SQL规划阶段注入租户值或谓词。自定义 ORM 使用 scoped Model 工厂和写 Hook；JOIN/raw SQL使用别名限定 helper，不重写最终 SQL。

**Tech Stack:** Go 1.23、GoFrame v2.10.2、MySQL、现有 `cool/auth`、`cool/model`、`cool/crud`、`modules/base`、`modules/dict`。

**Design:** `docs/superpowers/specs/2026-07-27-universal-tenant-scope-design.md`

## 全局约束

- 数据库存储以 `NULL` 表示平台作用域，正整数表示具体租户；新代码不得写入 `tenant_id=0`。
- legacy JWT 的 `tenantId:0` 解析为平台；新平台 JWT序列化为 `tenantId:null`；缺失 claim 必须拒绝。
- 平台与 Bypass 即使都不追加谓词，也必须保持不同类型和审计语义。
- 客户端字段不能决定写入租户；平台写租户数据必须显式使用 `ForTenant`。
- 不使用正则改写 SQL，不增加每请求反射、全局锁、租户授权查询或共享可变状态。
- GoFrame Select Hook 不改写 SQL；Select scope 在 Model 构建时用结构化 `Where` 应用。
- 通用写入继续使用现有参数化 SQL；自定义 ORM 不新增 `g.Map` 数据库写入。
- 每项只暂存精确文件，不使用 `git add -A`；不得提交未跟踪的 `docs/midway-gap-analysis.md`。
- 修改 Go 文件后执行 `gofmt`，先跑定向测试，再扩大测试范围。

## Task 1：建立 TenantIdentity 与 JWT null 兼容

**Files:** Create `cool/auth/tenant_identity.go`, `cool/auth/tenant_identity_test.go`; modify `cool/auth/token.go`, `token_test.go`, `context.go`, `context_test.go`, `middleware.go`, `middleware_test.go`.

- [ ] 写失败测试：JSON null、legacy 0、正整数、缺失、负数、小数、字符串、溢出；平台 marshal 必须为显式 null；Missing 不得签发或通过解析。
- [ ] 实现字段不导出的值类型 TenantIdentity，提供平台/正租户构造器和只读查询方法；Claims 与 UserContext 按值保存，不使用指针。
- [ ] `GenerateTokenPair` 和 parse 对 Missing 失败关闭；Middleware 只从已规范化 Claims 构造上下文。
- [ ] 验证并提交：

```bash
gofmt -w cool/auth
go test ./cool/auth -count=1
git add cool/auth/tenant_identity.go cool/auth/tenant_identity_test.go cool/auth/token.go cool/auth/token_test.go cool/auth/context.go cool/auth/context_test.go cool/auth/middleware.go cool/auth/middleware_test.go
git commit -m "feat: model nullable tenant identity"
```

## Task 2：迁移登录、刷新和 Base 租户身份调用点

**Files:** Modify `modules/base/service/sys/auth_boundary.go`, `auth_boundary_test.go`, `login.go`, `login_test.go`, `login_session_test.go`, `tenant.go`, affected Base middleware/service tests.

- [ ] 写失败测试：数据库 tenant 为 nil、0、正数和非法值；登录平台用户签发 null；刷新重读快照并覆盖 tenant A -> B、tenant -> platform、platform -> tenant。
- [ ] 新增严格数据库值转换：nil/0 -> Platform，正数 -> Tenant，负数或非数字报错；禁止 `int64Value(nil)` 压平语义。
- [ ] Session 不增加 tenant 字段；更新所有直接读取或构造 `UserContext.TenantId` 的调用点。
- [ ] 迁移期间保留授权 SQL中的 `IS NULL OR = 0` 兼容读取。
- [ ] 验证并精确提交：

```bash
gofmt -w modules/base/service/sys modules/base/middleware
go test ./cool/auth ./modules/base/service/sys ./modules/base -run 'Tenant|Login|Refresh|Middleware|Context' -count=1
git commit -m "refactor: propagate typed tenant identity"
```

## Task 3：编译模型和 CRUD 租户元数据

**Files:** Modify `cool/model/model.go`, `model_test.go`, `cool/crud/types.go`, `metadata.go`, `metadata_test.go`, `cool/controller/builder.go`, `builder_test.go`.

- [ ] 写失败测试：Auto、Required、Disabled；canonical `tenantId/tenant_id`；缺字段、类型错误、非 unsigned、非 nullable；Controller clone 保留模式。
- [ ] Definition 增加 TenantMode 和链式设置；Auto 识别 BaseFields，Required 强校验，Disabled 显式跳过。
- [ ] Resource 保存启动期编译的不可变 tenant.Metadata，请求阶段不扫描模型。
- [ ] Public policy 留在 Resource/Route 层，不放入模型定义。
- [ ] 验证并提交：

```bash
gofmt -w cool/model cool/crud/metadata.go cool/crud/metadata_test.go cool/crud/types.go cool/controller
go test ./cool/model ./cool/crud ./cool/controller -count=1
git commit -m "feat: compile tenant resource metadata"
```

## Task 4：实现中央 TenantScope、谓词和派生上下文

**Files:** Create `cool/tenant/scope.go`, `scope_test.go`, `predicate.go`, `predicate_test.go`, `metadata.go`, `metadata_test.go`.

- [ ] 写失败测试：Missing/Platform/Tenant/Bypass、非法 ID、嵌套 override、并发隔离、别名校验和参数化条件。
- [ ] Resolve 先读取派生 override，再读取 auth UserContext；没有两者时为 Missing。
- [ ] `ForTenant` 只接受正数；`WithoutTenant` 返回派生 context，不修改 UserContext。
- [ ] Tenant 返回 alias-qualified `tenant_id = ?`；Platform/Bypass 无条件；Missing 对 tenant-aware 资源返回未认证错误。
- [ ] 验证并提交：

```bash
gofmt -w cool/tenant
go test ./cool/tenant -count=1
go test -race ./cool/tenant -count=1
git add cool/tenant
git commit -m "feat: add central tenant scope"
```

## Task 5：在 CRUD SQL规划层注入租户作用域

**Files:** Modify `cool/crud/query.go`, `query_test.go`, `runtime.go`, `override_test.go`; create `cool/crud/tenant_runtime_test.go`.

- [ ] 写失败测试：Tenant Add/AddMany 强制覆盖；Info/List/Page/Count/Update/UpdateMany/Delete 加谓词；Platform 无谓词且 Add 写 NULL；ForTenant 写指定 ID；Missing 拒绝；非租户模型兼容。
- [ ] Runtime 每次操作只解析一次 Scope，并把编译后的 plan 传入 query builder。
- [ ] Insert 在 writable 映射后注入内部 tenant 列；读写条件在结构化 WHERE 中追加；tenant 值只进入 Args。
- [ ] 自定义 Handler 必须收到可解析 scope，但其 SQL保护由后续 Model/raw guard 与集成测试保证。
- [ ] 验证并提交：

```bash
gofmt -w cool/crud
go test ./cool/crud -count=1
git commit -m "feat: scope generic CRUD by tenant"
```

## Task 6：落实 affected rows 与批量原子性

**Files:** Modify `cool/crud/runtime.go`, `tenant_runtime_test.go`, `override_test.go`.

- [ ] 写失败测试：跨租户单条 Update/Delete 零命中；不调用 ModifyAfter；UpdateMany 任一零命中整批回滚；Delete 去重后部分命中整批回滚。
- [ ] 在事务内验证 `RowsAffected`，并在 After Hook 前失败。
- [ ] 批量删除先用相同 scope 锁定/统计可见 ID；数量不匹配时返回现有 not-found 契约且不泄漏外租户 ID。
- [ ] Platform/Bypass 同样执行一致性校验。
- [ ] 验证并提交：

```bash
gofmt -w cool/crud
go test ./cool/crud -count=1
git commit -m "fix: enforce atomic tenant mutations"
```

## Task 7：提供 scoped GoFrame Model 和写 Hook

**Files:** Create `cool/tenant/model.go`, `model_test.go`, `model_integration_test.go`.

- [ ] 写失败测试：DB/TX Model 的 Select/Count、Insert、Update、Delete、Platform/Bypass、事务回滚；证明 raw SQL不会触发 Model Hook。
- [ ] Model factory 接受兼容 DB/TX 的最小 provider 和模型 Definition。
- [ ] Select 在建模阶段结构化 Where；Insert/Update/Delete Hook 只做写防御并正确调用 `Next`。
- [ ] Hook 内处理 GoFrame 标准化输入，不向业务暴露基于 `g.Map` 的写入 API。
- [ ] 验证并提交：

```bash
gofmt -w cool/tenant
go test ./cool/tenant -count=1
git commit -m "feat: add tenant scoped models"
```

## Task 8：接入配置并提供 zero-to-NULL 迁移

**Files:** Modify `manifest/config/config.yaml`, `cool/app/app.go`, `app_test.go`; create `cool/tenant/migrate.go`, `migrate_test.go`.

- [ ] 写失败测试：enable 传播、required+disabled 启动失败、显式关闭兼容；迁移只处理编译为 tenant-aware 的表，重复执行幂等。
- [ ] 增加 `cool.tenant.enable` 和 `requireEnabled`，不使用 Midway URL 列表。
- [ ] 迁移从已编译定义派生安全表名，执行 `tenant_id=0 -> NULL`；不处理关系表，不隐藏在请求或 schema sync 中。
- [ ] 验证并提交：

```bash
gofmt -w cool/app cool/tenant
go test ./cool/app ./cool/tenant -count=1
git commit -m "feat: configure tenant enforcement"
```

## Task 9：完整迁移 Dict 自定义查询和递归删除

**Files:** Modify `modules/dict/service/dict_info.go`, `dict_info_test.go`, `dict_type.go`, `controller/admin/info.go`, `dict_integration_test.go`.

- [ ] 写双租户失败矩阵：Type/Info 通用 CRUD、Types/Data、自定义删除、伪造 tenantId、平台可见性、公开 GlobalOnly、跨租户父子关系和级联。
- [ ] Types/Data 使用显式 Scope 和别名条件；公开接口使用 GlobalOnly，不把缺失认证当 bypass。
- [ ] 递归每一层使用同一事务和 Scope；Type 删除先 scoped 验证全部 Type，再删除同租户 Info。
- [ ] 验证并提交：

```bash
gofmt -w modules/dict
go test ./modules/dict -count=1
COOL_DICT_INTEGRATION=1 go test ./modules/dict -run DictIntegration -count=1
git commit -m "fix: isolate dictionary tenants"
```

## Task 10：审计 Base 自定义路径并增加 raw-access guard

**Files:** Create `cool/tenant/raw_access_test.go`; modify tenant-sensitive `modules/base/service/sys` files and对应测试。

- [ ] 使用 Go AST 扫描模块服务中新引入的直接 `GetOne/GetAll/GetCount/Exec/Model`；allowlist 精确到文件和用途。
- [ ] 审计 Param、Log、Menu、Department、User、Role 的自定义 CRUD、JOIN、级联和关系表。
- [ ] 正确的现有别名条件可保留；缺失或关联表泄漏路径改用 scoped Model/raw helper。
- [ ] 运行单元和现有 MySQL 双租户测试后精确提交。

```bash
gofmt -w cool/tenant/raw_access_test.go modules/base/service/sys
go test ./cool/tenant ./modules/base/service/sys -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run TenantBoundary -count=1
git commit -m "fix: scope custom base queries"
```

## Task 11：全矩阵验证、性能检查和文档收口

- [ ] 运行普通测试与 race：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
env GOCACHE=/private/tmp/cool-admin-go-build go test -race ./cool/auth ./cool/tenant ./cool/crud ./modules/base/service/sys ./modules/dict -count=1
```

- [ ] 运行 `COOL_AUTH_INTEGRATION`、`COOL_CUSTOM_API_INTEGRATION`、`COOL_DICT_INTEGRATION` 对应的真实 MySQL测试。
- [ ] 对代表性 tenant Page、Update、Delete 执行 EXPLAIN，确认使用 tenant 索引且无额外查询。
- [ ] 增加 Scope/Predicate microbenchmark，只发现明显退化，不设置依赖机器速度的绝对阈值。
- [ ] 更新差距和安全文档，明确平台 NULL、公开 GlobalOnly、legacy zero 迁移、Bypass 及 raw SQL边界。
- [ ] 最终检查 `git diff --check`、无占位标记、无新 `tenant_id=0` 写入、无客户端 scope 选择和未审阅 raw tenant query。
