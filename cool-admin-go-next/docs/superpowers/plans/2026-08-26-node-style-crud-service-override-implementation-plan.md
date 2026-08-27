# Node 风格 CRUD Service 重写与用户业务减量实施计划

> 日期：2026-08-26
> 依据：`docs/superpowers/specs/2026-08-26-node-style-crud-service-override-design.md`
> 状态：待实施

## 1. 目标

在保留 Go 版 bcrypt、严格字段边界、事务、行锁、并发快照、最后管理员保护和 Session 集中撤销的前提下，让 Controller 一次声明六个 CRUD，codegen 静态选择同名业务 Service 方法或 Base 方法，并把用户相关 Controller、DTO、Service 从当前 1271 行降至约 500 行。

本轮必须解决两个根因：

1. 框架内部的 `FieldValue -> Mutable -> Input -> DO` 管线不得泄漏到用户 Controller 和 UserService；
2. 角色、管理员和部门能力必须由 PermissionService、DepartmentService 复用，用户分页不得逐用户查询。

## 2. 硬性约束

1. 不改变 `/admin/base/sys/user` 现有路径、HTTP 方法、请求字段和响应 JSON；
2. 不削弱 bcrypt、事务、锁、并发快照、最后管理员保护、Session 撤销和未知字段拒绝；
3. 不新增第三方依赖，不使用 `orm:"-"`，不新建第二套 CRUD、Repository 或事务框架；
4. `roleIdList` 必须保持四态：未提交不修改，`null` 和 `[]` 清空，非空数组替换；
5. `Fields()` 表示全部可绑定字段，新增的持久化字段视图才允许进入 SQL、DO、Schema、索引和查询计划；
6. EPS `Columns` 保留 transient 请求字段，EPS `PageColumns` 和查询条件只允许持久化字段；
7. 生成文件只通过 `go run ./cmd/cool generate` 更新，不手工编辑 `modules/modules_gen.go`；
8. 不修改 `cool-admin-vue/build/cool/eps.d.ts` 和 `cool-admin-vue/build/cool/eps.json` 的现有未提交内容；
9. 每次完整修改一个文件后再处理下一个文件，不顺手重构无关模块；
10. `modules/base/service` 生产代码必须净减少，禁止把 `user.go` 原样搬到新文件；
11. 普通单表模块的现有 Controller 和 Service 行为保持兼容；
12. 任一阶段无法保持上述约束时，在该阶段停止并保留上一阶段已通过的能力，不扩大范围。

## 3. 字段模型定案

为避免 `transient` 被不同消费者误解，Descriptor 明确提供两类字段集合：

| 视图 | 包含 transient | 用途 |
| --- | --- | --- |
| `Fields()`、`Field()`、`JSON()` | 是 | Binder、Mutable、字段策略、EPS 请求列 |
| `PersistentFields()`、`Column()` | 否 | DO、DDL、Schema、Recycle、Select、Where、Group、Order、PageColumns |

`Field` 增加 `Persistent() bool`。持久化字段必须有 `orm` 列名；`cool:"transient"` 字段不得声明数据库列，不得被 Schema 索引引用。`RoleIDList` 使用：

```go
RoleIDList *[]uint64 `json:"roleIdList" description:"角色ID列表" cool:"transient"`
```

本轮只为 transient 增加标量切片及其指针支持，满足 `[]uint64`；不扩展任意 map、嵌套对象或递归 DTO。

## 4. 任务与顺序

### 任务 0：记录基线和保护工作区

文件：无。

步骤：

1. 记录 `git status --short`，确认两份 Vue EPS 文件是用户已有改动；
2. 运行生成器静态检查、框架专项测试、Base 模块测试和全量测试；
3. 记录用户相关文件及 `modules/base/service` 生产 Go 行数；
4. 保存当前用户路由、请求和响应契约测试结果，作为迁移后的比较基线。

验证：

```bash
go run ./cmd/cool check
go test ./cool-next/core/entity ./cool-next/core/service ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen ./cool-next/eps -count=1
go test ./modules/base/... -count=1
go test ./... -count=1
wc -l modules/base/service/user.go modules/base/dto/user.go modules/base/controller/admin/sys/user.go
find modules/base/service -name '*.go' ! -name '*_test.go' -print0 | xargs -0 wc -l
```

### 任务 1：让 Descriptor 表达 transient 与持久化字段

生产文件：

- 修改：`cool-next/core/entity/types.go`
- 修改：`cool-next/core/entity/descriptor.go`
- 修改：`cool-next/core/entity/field.go`
- 修改：`cool-next/core/entity/compile.go`
- 修改：`cool-next/core/entity/do.go`
- 修改：`cool-next/codegen/entity_validate.go`

测试文件：

- 修改：`cool-next/core/entity/compile_test.go`
- 修改：`cool-next/core/entity/index_test.go`
- 修改：`cool-next/core/entity/do_test.go`
- 修改：`cool-next/core/entity/do_goframe_test.go`
- 修改：`cool-next/codegen/analyze_test.go`

步骤：

1. 先增加失败用例，固定合法 transient、重复 JSON 名、缺少 `orm` 的持久化字段、transient 声明 `orm`、transient 索引和非法 `cool` 标签的结果；
2. `Field` 增加 `Persistent()`，`Metadata` 增加返回防御性副本的 `PersistentFields()`；
3. `parseBusinessField` 先解析 `cool:"transient"`，再决定是否要求 `orm`；持久化字段保持原校验，transient 的 `Column()` 返回空字符串且不进入 `byColumn`；
4. `cool` 标签解析器接受无值的 `transient` 标志，继续拒绝未知、重复和与字段类型冲突的约束；
5. transient 支持 `[]uint64` 和 `*[]uint64` 的逻辑类型、可空性与防御性复制，不扩大到任意对象；
6. Descriptor 的 `byName`、`byJSON` 和 `Fields()` 收录 transient，`byColumn`、`PersistentFields()` 和索引字段集合排除 transient；
7. DO shape 只从 `PersistentFields()` 编译，`SetColumn("roleIdList", ...)` 必须失败；
8. codegen 静态校验与运行时 Compile 使用同一规则，避免 `cool check` 通过但启动时失败。

验证：

```bash
go test ./cool-next/core/entity ./cool-next/codegen -count=1
go test ./cool-next/core/entity -run 'Transient|DO|Index' -count=1
```

### 任务 2：把所有数据库与查询消费者切到持久化字段视图

生产文件：

- 修改：`cool-next/core/service/base.go`
- 修改：`cool-next/core/service/input.go`
- 修改：`cool-next/crud/plan.go`
- 修改：`cool-next/crud/action_plan.go`
- 修改：`cool-next/crud/projection.go`
- 修改：`cool-next/db/driver/ddl.go`
- 修改：`cool-next/db/schema/expected.go`
- 修改：`cool-next/db/recycle/store.go`
- 修改：`cool-next/seed/record.go`
- 修改：`cool-next/eps/eps.go`

测试文件：

- 修改：`cool-next/core/service/input_test.go`
- 修改：`cool-next/core/service/base_test.go`
- 创建：`cool-next/core/controller/binder_test.go`
- 修改：`cool-next/crud/plan_test.go`
- 修改：`cool-next/crud/action_plan_test.go`
- 修改：`cool-next/crud/projection_test.go`
- 修改：`cool-next/db/driver/ddl_test.go`
- 修改：`cool-next/db/schema/schema_test.go`
- 修改：`cool-next/db/recycle/store_test.go`
- 创建：`cool-next/seed/record_test.go`
- 修改：`cool-next/eps/eps_test.go`

步骤：

1. 先建立一个同时含持久化字段和 `roleIdList` transient 的测试实体；
2. 验证 Binder/NewMutable 能保留 `Has`、`IsNull`、`Get` 四态，`Get`/`Set` 对切片做防御性复制；
3. Mutable 增加 `Unset(field)`，只删除本次输入中的字段及 presence，供业务重写忽略更新时的空密码，不改变 Descriptor；
4. Base 的 Add/Update 投影只遍历 `PersistentFields()`，transient 留在 Mutable 中供业务方法读取，但不计入“更新字段不能为空”的数据库字段数；
5. HiddenFields 继续拒绝未经业务处理的客户端写入，但业务 Service 通过 `Set` 改写为服务端值后允许 Base 落库；Controller ReadonlyFields 继续忽略客户端值，但允许业务 Service 设置 passwordV；主键和系统维护字段规则不放宽；
6. CRUD 默认 Select、自动排序白名单和字段投影只遍历 `PersistentFields()`；显式把 transient 用于 Select、Where、Join、Group、Order 或 SortFields 时返回 Core 配置错误；
7. DDL、Schema 期望表、Recycle 快照和归档列只遍历 `PersistentFields()`；
8. Seed 输入显式拒绝 transient，防止启动时接受一个最终不会落库的字段；
9. EPS `Columns` 继续使用 `Fields()`，使 `roleIdList` 出现在 Add/Update 请求类型；`PageColumns` 和 PageQueryOp 只来自持久化查询投影；
10. 验证未知字段、非法数组元素、非法 null 和隐藏/只读策略没有放宽。

验证：

```bash
go test ./cool-next/core/service ./cool-next/crud ./cool-next/db/driver ./cool-next/db/schema ./cool-next/db/recycle ./cool-next/seed ./cool-next/eps -count=1
go test ./cool-next/core/service ./cool-next/crud ./cool-next/eps -run Transient -count=1
```

### 任务 3：让 CRUD Dispatcher 支持业务重写后调用 Base

生产文件：

- 修改：`cool-next/crud/dispatcher.go`
- 修改：`cool-next/core/controller/pipeline.go`

测试文件：

- 修改：`cool-next/crud/dispatcher_test.go`
- 修改：`cool-next/core/controller/pipeline_test.go`
- 修改：`cool-next/core/service/base_test.go`

步骤：

1. 先增加失败用例：业务 override 在同一 Dispatcher 事务中调用 `Base.Add/Update/Info` 时必须拿到匹配的 ActionPlan；
2. Base、Delegate、Override 三种模式都完成绑定和 ActionPlan 编译，并在 Dispatcher 派生 Context 中写入 OperationScope；
3. Override 执行 InsertParam 等已声明输入增强，但仍跳过 Base 专用 ModifyBefore/ModifyAfter 自动钩子，只有业务方法显式调用 Base 时才进入 Base；
4. 保留同一事务 Runner，业务 Service 内不得再次 `Within`；
5. `HandleCRUDDTO` 继续只绑定一次 DTO，并编译静态动作计划；不得退化成普通自定义 Route；
6. 更新 `validateDispatchPlan`，要求三种合法模式都携带动作匹配的计划，继续在事务开始前拒绝 nil、动作不匹配和非法模式。

验证：

```bash
go test ./cool-next/crud ./cool-next/core/controller ./cool-next/core/service -count=1
go test -race ./cool-next/crud ./cool-next/core/controller -count=1
```

### 任务 4：扩展 codegen 的六动作静态分派

生产文件：

- 修改：`cool-next/codegen/service_analysis.go`
- 修改：`cool-next/codegen/route_analysis.go`

测试文件：

- 修改：`cool-next/codegen/analyze_test.go`
- 修改：`cool-next/codegen/route_analysis_test.go`
- 修改：`cool-next/codegen/route_render_test.go`
- 修改：`cool-next/codegen/render_integration_test.go`

步骤：

1. 建立包含六个动作组合的测试 workspace：无重写、固定输入重写、直接 Base 委托、Info 业务结果、List/Page `Query` 结果、List/Page 强类型 DTO 结果；
2. Add/Delete/Update 只接受设计文档中的固定输入和返回签名，发现同名但非法签名时给出生成期诊断，不静默回退；
3. Info 固定接收主键 ID，允许任意非 `error` 业务结果加末尾 `error`；
4. List/Page 接受 `coreservice.Query` 或 `*具名 struct DTO`，允许业务结果加末尾 `error`；
5. 区分方法是业务 Service 直接声明还是嵌入 Base 提升：直接声明生成业务 Adapter，未声明生成 Base Adapter；
6. 强类型 List/Page 生成 `HandleCRUDDTO`，其余查询生成 `HandleQuery`，六个动作始终保持 `KindCRUD`；
7. 保留 Delegate 识别用于显式 `service.Base.<Action>` 的可观测元数据，但不得依赖 AST 猜测来决定 ActionPlan 是否存在；
8. 生成结果断言每个动作只绑定一次、只进入一次 Dispatcher，Info/List/Page 的业务返回值原样交给响应层；
9. 其他模块现有 Service 和 Controller 无需迁移即可通过生成与编译。

验证：

```bash
go test ./cool-next/codegen -count=1
go test ./cool-next/codegen -run 'ServiceAction|StaticRoutes|Render' -count=1
```

### 任务 5：把角色关系与最后管理员规则集中到 PermissionService

生产文件：

- 修改：`modules/base/service/permission.go`

测试文件：

- 修改：`modules/base/service/permission_test.go`

步骤：

1. 扩充构造依赖，使 PermissionService 能在自身领域内访问 User、UserRole、Role 及 `auth.Boundary`，不把 Base、Model、Descriptor 暴露给调用方；
2. 复用 `auth.NormalizeIDs`，删除角色领域内重复的过滤、排序和去重；
3. 保留单用户 `RoleIDs` 和 `IsAdmin`，新增一次查询返回多用户角色 ID/名称的批量能力；
4. 提供角色存在性校验、按稳定 ID 顺序锁角色、读取关系快照、校验快照和替换关系能力；
5. 集中实现删除、禁用或移除管理员角色时的最后管理员保护；
6. 关系替换使用一次删除和批量写入，不按角色逐条构造 Descriptor DO；
7. 错误保持 Validate/Comm/Core 边界，不暴露 Token、Session 或密码信息；
8. 用测试固定空角色、重复角色、缺失角色、关系清空/替换、并发快照变化和最后管理员场景。

验证：

```bash
go test ./modules/base/service -run 'Permission|Admin|Role' -count=1
go test -race ./modules/base/service -run 'Permission|Admin|Role' -count=1
```

### 任务 6：让 DepartmentService 只负责部门领域并复用 PermissionService

生产文件：

- 修改：`modules/base/service/department.go`

测试文件：

- 创建：`modules/base/service/department_test.go`

步骤：

1. 构造依赖改为 Department Base、User Base、RoleDepartment Base、PermissionService 和 `auth.Boundary`，移除直接 Role/UserRole 查询；
2. `isAdmin` 调用 PermissionService，按角色的部门范围只由 DepartmentService 查询 RoleDepartment；
3. 增加部门存在性校验与稳定顺序锁、当前用户可见范围、单个名称和批量名称查询；
4. 部门删除用户时调用 PermissionService 的最后管理员保护，不保留第二套原生 SQL；
5. 部门删除遵守“角色 -> 部门 -> 用户 -> 关系写入”顺序：锁前可预读候选集，锁后必须重读并做快照校验；
6. 在 DepartmentService 内使用现有 Descriptor DO 或 Base 输入完成部门用户迁移，不向其他领域暴露 DO，也不再调用定义在 `user.go` 的 `businessDO`；
7. 部门列表现有父名称行为保持不变，本轮不把无关的父名称查询扩成新树框架；
8. 测试部门范围、批量名称、删除迁移/删除用户、最后管理员和并发快照。

验证：

```bash
go test ./modules/base/service -run 'Department' -count=1
go test -race ./modules/base/service -run 'Department' -count=1
```

### 任务 7：按新边界一次性重写用户实体、DTO、Service 和 Controller

按以下文件顺序逐个完成，每个文件只进行一次集中修改。

#### 7.1 用户实体

生产文件：

- 修改：`modules/base/entity/user.go`

步骤：

1. 增加 `RoleIDList *[]uint64` transient 字段；
2. 不增加数据库列和索引，不改变现有持久化字段、默认值或表结构；
3. 运行 Descriptor、Schema 和 EPS 测试，确认数据库无 `roleIdList`，EPS 请求列包含它。

#### 7.2 用户 DTO

生产文件：

- 修改：`modules/base/dto/user.go`

步骤：

1. 删除 `UserRoleInput`、`UserAddReq`、`UserUpdateReq`、自定义 `UnmarshalJSON` 和 `HasField`；
2. 保留并精简 `UserMoveReq`、`UserPageReq`、`PersonUpdateReq` 和稳定响应 DTO；
3. Page/Info 响应字段保持与现有 JSON 一致；
4. DTO 目标控制在 40–70 行，不用压缩排版达标。

#### 7.3 UserService

生产文件：

- 修改：`modules/base/service/user.go`

测试文件：

- 修改：`modules/base/service/user_test.go`
- 创建：`modules/base/service/user_integration_test.go`

步骤：

1. 构造依赖收缩为 User Base、PermissionService、DepartmentService、bcrypt Verifier 和 `auth.Boundary` 所需依赖；
2. Add 签名改为框架 `AddInput`：只允许单对象，读取 transient 角色，要求至少一个角色，通过 `Set` 把明文密码替换为服务端摘要，按角色/部门顺序锁定，调用 `Base.Add` 后写关系；
3. Update 签名改为框架 `UpdateInput`：从 Mutable 读取角色、状态、密码 presence；空密码通过 `Unset` 保持现有“忽略”语义，非空密码通过 `Set` 写摘要和 passwordV；随后按角色 -> 部门 -> 用户顺序锁定，校验关系快照和最后管理员，调用 `Base.Update` 后替换关系并按需撤销 Session；
4. Delete 保持框架 `DeleteInput`：规范化 ID、锁定并保护最后管理员，清理关系、调用 `Base.Delete`、撤销 Session；
5. Info 调用 `Base.Info`，映射到无密码业务 DTO，再从 PermissionService 和 DepartmentService 补充角色与部门；
6. 不声明 List，让 codegen 自动使用 `Base.List`；
7. Page 签名使用 `*dto.UserPageReq`，保留页码默认值、筛选、排序和当前管理员数据范围；用户分页阶段完成后，只调用一次批量角色和一次批量部门名称；
8. Move、Person、PersonUpdate 保留独立业务 DTO，并复用 DepartmentService、PermissionService 和 Boundary；
9. 删除 `userAddInput`、`userUpdateInput`、`appendPresentUserFields`、`businessDO`、`businessUniqueIDs`、重复角色/管理员/部门查询及逐用户 `pageItems` 查询；
10. UserService/Controller 不出现 `FieldValue`、`NewMutable`、`NewAddObject`、`NewUpdateObject` 或 `NewDO`；
11. UserService 目标 250–350 行，不通过拆文件转移原实现。

#### 7.4 用户 Controller

生产文件：

- 修改：`modules/base/controller/admin/sys/user.go`

测试文件：

- 修改：`modules/base/controller/contract_test.go`
- 修改：`modules/base/controller/admin/contract_test.go`

步骤：

1. 删除 UserHandler 和 NewUserHandler；
2. `CurdOption.API` 改为 `controller.AllAPI()`；
3. 使用 `InsertParam` 从认证上下文设置 `userId`，保留 password HiddenFields 及 passwordV/socketId ReadonlyFields；
4. `/move` 直接绑定 `controller.Handle(user.Move)`；
5. 删除自定义 `/add`、`/update`、`/page`，这些路由必须变为 `KindCRUD`；
6. Controller 工厂只依赖 UserService，目标 40–60 行。

阶段验证：

```bash
go test ./modules/base/entity ./modules/base/dto ./modules/base/service ./modules/base/controller/... -count=1
go test -race ./modules/base/service -count=1
rg -n 'FieldValue|NewMutable|NewAddObject|NewUpdateObject|NewDO|businessDO|businessUniqueIDs' modules/base/service/user.go modules/base/controller/admin/sys/user.go
wc -l modules/base/service/user.go modules/base/dto/user.go modules/base/controller/admin/sys/user.go
find modules/base/service -name '*.go' ! -name '*_test.go' -print0 | xargs -0 wc -l
```

### 任务 8：业务行为、并发和查询数验收

测试文件：

- 完善：`modules/base/service/user_integration_test.go`
- 创建：`test/integration/database/user_test.go`

步骤：

1. 在 SQLite 专项测试中覆盖用户新增、更新、删除、详情、分页、移动和个人资料；
2. 分别提交缺失、`null`、`[]`、重复 ID 和非空 `roleIdList`，固定四态语义；
3. 验证明文密码不落库、不出响应，密码修改递增 passwordV，错误旧密码不写入；
4. 验证密码、状态或角色变化撤销 Session，普通资料变化不额外撤销；
5. 验证删除、禁用和移除最后一个管理员均失败，仍有其他有效管理员时成功；
6. 用数据库 Hook 统计 Page SQL：用户分页阶段可包含 COUNT 和数据读取，之后恰好一次批量角色、一次批量部门名称；比较 1 个用户和多用户页面，SQL 总量必须相同；
7. 并发测试固定角色 -> 部门 -> 用户的加锁顺序，并验证锁前后关系变化返回“已变更，请重试”；
8. 通过现有数据库集成脚本在 MySQL、PostgreSQL、SQLite 上验证 transient 不建列、CRUD 和用户核心流程一致。

验证：

```bash
go test ./modules/base/service -run 'User|Permission|Department' -count=1
go test -race ./modules/base/service -run 'User|Permission|Department' -count=1
test/integration/run.sh
```

### 任务 9：重新生成、二次生成和全量门禁

生成文件：

- 重新生成：`modules/modules_gen.go`

步骤：

1. 运行生成器，检查 UserHandler Provider 被删除，NewUser/NewPermission/NewDepartment 依赖图正确；
2. 检查用户六个 CRUD 均生成 `KindCRUD`，Add/Update/Delete/Info/Page 调用 UserService，List 调用 Base 提升方法，Page 使用 `HandleCRUDDTO[dto.UserPageReq]`；
3. 检查所有用户 CRUD 都只进入一次 Dispatcher，生成代码没有手工 DTO -> Mutable 适配；
4. 再次运行 generate/check，确认 `modules/modules_gen.go` 字节不变；
5. 运行 gofmt、专项测试、全量测试、race、vet 和 `make check`；
6. 检查 Vue 两份 EPS 文件未被本轮修改；
7. 统计生产代码净变化并核对成功指标。

验证：

```bash
test -z "$(find cool-next modules/base -name '*.go' -print0 | xargs -0 gofmt -l)"
go run ./cmd/cool generate
go run ./cmd/cool check
cp modules/modules_gen.go /private/tmp/node-style-crud-modules-gen.go
go run ./cmd/cool generate
cmp /private/tmp/node-style-crud-modules-gen.go modules/modules_gen.go
go run ./cmd/cool check
go test ./... -count=1
go test -race ./cool-next/core/entity ./cool-next/core/service ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen ./modules/base/service -count=1
go vet ./...
make check
git diff --check
```

## 5. 分阶段提交建议

| 提交 | 内容 | 可独立回退 |
| --- | --- | --- |
| 1 | Descriptor transient 与 codegen 校验 | 是 |
| 2 | 持久化消费者、Binder/Mutable、EPS | 是 |
| 3 | Dispatcher 与六动作 codegen 分派 | 是 |
| 4 | PermissionService、DepartmentService 领域收口 | 是 |
| 5 | User Entity/DTO/Service/Controller 迁移及生成文件 | 是，依赖前四项 |
| 6 | 集成测试和最终精简 | 是 |

每个提交前运行对应专项测试；任务 7 不拆成“旧代码和新代码同时存在”的中间提交，避免产生两条用户写入路径。

## 6. 风险与中止条件

| 风险 | 中止或回退条件 |
| --- | --- |
| transient 泄漏进数据库 | DDL、Schema、DO、Recycle、Seed 或 SQL 任一路径出现 `roleIdList` 即停止业务迁移 |
| EPS 丢失角色输入 | Add/Update 请求类型不含 `roleIdList`，或 PageColumns 含该字段，即回退 EPS 字段视图选择 |
| override 无法调用 Base | 业务方法内 `Base.*` 取不到同一事务 ActionPlan，即停止 UserService 迁移 |
| codegen 静默选择错误方法 | 同名非法签名未在生成期失败，或未声明动作调用业务 Adapter，即停止生成文件更新 |
| 锁顺序不一致 | User/Department 任一路径先锁用户再锁角色或部门，即停止领域迁移 |
| N+1 仍存在 | Page SQL 数量随页内用户数增长，即不接受 UserService 减量结果 |
| 代码只是搬家 | `modules/base/service` 生产代码不减反增，或重复 SQL 转移到新 helper，即回退对应领域拆分 |
| 安全语义变化 | bcrypt、最后管理员、Session、未知字段或事务用例任一回归，即停止并修复根因 |
| 生成不稳定 | 连续两次 generate 有差异，即不提交生成文件 |

## 7. 完成标准

| 指标 | 目标 |
| --- | ---: |
| UserService | 250–350 行 |
| 用户 Controller | 40–60 行 |
| 用户专用 DTO | 40–70 行 |
| 用户 Controller、DTO、Service 合计 | 约 500 行 |
| UserService/Controller 构造 Mutable/Input/DO | 0 处 |
| UserService 直接查询角色或部门 | 0 处 |
| 用户分页 | 固定 3 个阶段，SQL 总数不随页内用户数增长 |
| 用户六个基础路由 | 全部 `KindCRUD` |
| `modules/base/service` 生产代码 | 相对任务 0 基线净减少 |
| 新第三方依赖 | 0 |

最终必须同时满足：普通单表模块仍只需 Entity 和 Controller 即可获得六个基础接口；复杂业务只在同名 Service 方法中编排差异；所有专项测试、全量测试、race、vet、集成数据库和生成稳定性门禁通过。
