# Node 风格 CRUD Service 重写与业务层减量设计

> 日期：2026-08-26
> 状态：已确认，待实施计划
> 范围：`cool-next` CRUD 扩展能力与 `modules/base` 用户业务迁移

## 1. 背景

`cool-admin-midway` 由 Controller 一次声明 `add/delete/update/info/list/page` 六个基础接口。业务 Service 声明同名方法时覆盖默认行为，并可通过 `super` 调用 Base；未覆盖的方法继续使用框架实现。用户 Controller 约 35 行，用户 Service 约 237 行。

Go 版虽然已有 Base、Descriptor、Binder、CRUD Dispatcher 和 codegen，但用户模块为了接收 `roleIdList` 和自定义分页结果，将 `add/update/page` 重新声明为自定义路由。随后业务 Service 又手工完成：

```text
DTO
-> FieldValue
-> Mutable
-> AddInput / UpdateInput
-> Descriptor
-> DO
-> Model
```

当前相关代码量为：

| 文件 | 行数 |
| --- | ---: |
| `modules/base/service/user.go` | 1023 |
| `modules/base/dto/user.go` | 140 |
| `modules/base/controller/admin/sys/user.go` | 108 |
| 合计 | 1271 |

Go 版需要保留 Node 未完整提供的 bcrypt、严格字段边界、事务、行锁、并发快照、最后管理员保护和 Session 集中撤销，但这些安全语义不能成为业务层重复实现框架输入、权限查询和部门查询的理由。

## 2. 目标

1. Controller 一次声明六个基础 CRUD，业务 Service 通过同名方法重写差异。
2. 未重写的动作自动调用 Base；重写方法可以显式委托 Base。
3. 请求只绑定一次，UserService 和 Controller 不构造 `FieldValue`、`Mutable`、`AddInput`、`UpdateInput` 或 DO。
4. 支持参与 CRUD 输入但不进入数据库的 transient 字段。
5. PermissionService 统一角色关系、管理员判断和最后管理员保护。
6. DepartmentService 统一部门范围、校验和批量名称查询。
7. `auth.Boundary` 统一 ID、锁、并发快照和 Session 撤销原语。
8. 用户分页以固定批量查询替代逐用户查询。
9. 用户相关 Controller、DTO、Service 总量降至约 500 行，且不削弱安全语义。

## 3. 非目标

1. 不照搬 Node 的动态 `any` 对象、MD5、宽松字段边界或非原子关系写入。
2. 不通过压缩排版、移动原代码或机械拆文件达到行数目标。
3. 不新建第二套 CRUD、Repository、事务或查询框架。
4. 不要求 dict、task 等其他模块在本轮同步迁移。
5. 不改变现有 HTTP 路径、方法、请求字段和响应 JSON。
6. 不引入新第三方依赖。

## 4. 方案选择

### 4.1 同名 Service 重写与 transient 字段，采用

复用现有 Base、Mutable、CRUD Dispatcher、Controller DSL 和 codegen。框架补充 transient 字段及更完整的重写识别，用户模块作为首个迁移样板。

优点：最接近 Node 的开发体验；一次绑定；安全输入模型仍由框架持有；改动集中在现有扩展点。

### 4.2 只增加 Before/After Hook，拒绝

Hook 可覆盖简单写操作，但用户分页、业务 DTO、管理员保护和角色关系会把 Hook 变成携带大量动作状态的万能入口，不能解决业务复杂度。

### 4.3 每个动作使用独立 DTO 并生成映射，拒绝

该方案仍保留 Entity、DTO、Mutable 之间的重复表达，只是把手写映射改成生成映射。生成代码和错误定位成本增加，业务边界没有真正变短。

## 5. 六个 CRUD 的重写契约

Controller 始终通过 `controller.AllAPI()` 声明完整 CRUD。Codegen 对每个动作进行静态选择：

```text
Service 声明合法同名方法 -> 生成业务 Service Adapter
Service 未声明同名方法   -> 生成 Base Adapter
```

Go 没有类的虚方法，因此这里的“重写”是 codegen 静态分派，不依赖反射动态调用。

动作契约如下：

| 动作 | 默认输入 | 重写结果 |
| --- | --- | --- |
| Add | `AddInput[E]` | 保持 `AddResult[ID], error` |
| Delete | `DeleteInput[ID]` | 保持 `error` |
| Update | `UpdateInput[E, ID]` | 保持 `error` |
| Info | 主键 ID | 允许业务结果 `T, error` |
| List | `Query` 或强类型查询 DTO | 允许业务结果 `T, error` |
| Page | `Query` 或强类型查询 DTO | 允许业务结果 `T, error` |

Add、Delete、Update 保持固定结果契约，确保默认 HTTP 响应和 Base 委托稳定。Info、List、Page 的 Controller 执行器已经以 `any` 承载结果，codegen 可在静态检查输入与末尾 `error` 的前提下生成强类型调用。

业务重写需要默认行为时显式调用 `service.Base.Add`、`service.Base.Update` 等方法。CRUD Dispatcher 负责创建或复用事务、写入 ActionPlan 和执行统一外壳；业务重写不得再次创建相同事务。

## 6. Transient 字段

### 6.1 声明

参与 CRUD 输入但不属于数据库列的实体字段使用：

```go
RoleIDList *[]uint64 `json:"roleIdList" cool:"transient"`
```

不使用 `orm:"-"` 作为项目契约。GoFrame v2.10.2 当前文档和所用 ORM 写入源码没有为本项目的 Descriptor/DO 管线提供足够明确的该标签忽略保证；本项目本来也由 Descriptor 决定 Schema 和数据库投影，因此使用明确的 `cool` 元数据。

### 6.2 元数据与行为

Descriptor Field 增加是否持久化的只读元数据。Transient 字段：

- 必须有合法且唯一的 JSON 名；
- 不要求数据库列名；
- 参与 Binder 的名称、类型、null 和字段提交状态校验；
- 保存在现有 Mutable 中，不新增第二个扩展字段容器；
- 不进入 DO、Schema、索引、Select、Where、Order 或 Group；
- 仍受 HiddenFields 和 ReadonlyFields 策略约束；
- 不放宽未知 JSON 字段校验。

Transient 除现有持久化字段类型外，至少支持标量切片及其指针，并在 Value、Set、Get 时进行防御性复制；本轮只实现用户角色所需的 `[]uint64`，不扩展任意 map 或递归对象。

更新时必须区分：

| 请求状态 | 角色语义 |
| --- | --- |
| 未提交 `roleIdList` | 不修改角色 |
| 提交 `roleIdList: null` | 清空角色 |
| 提交 `roleIdList: []` | 清空角色 |
| 提交 `roleIdList: [1, 2]` | 替换角色 |

Base 从 Mutable 生成 DO 时只遍历持久化字段。业务 Service 可以读取或改写 transient 字段，但不负责删除它。

新增用户保持现有校验：必须提交至少一个角色。该动作差异由 UserService.Add 校验，不把 Add 专属规则写进同时服务 Update 的 Entity 标签。

## 7. 领域职责

### 7.1 PermissionService

统一负责：

- 查询单个或批量用户的角色；
- 判断角色集合或用户是否为管理员；
- 校验并锁定角色；
- 替换用户角色关系；
- 校验角色关系并发快照；
- 防止删除、禁用或移除最后一个管理员；
- 批量返回用户角色 ID 和名称。

最后管理员是后台权限领域规则，不下沉到通用 `auth` 包。DepartmentService 删除部门用户时也调用 PermissionService，不再维护独立 SQL 实现。

### 7.2 DepartmentService

统一负责：

- 校验并锁定部门；
- 按角色返回部门范围；
- 返回当前用户可见部门范围；
- 单个或批量返回部门名称。

### 7.3 auth.Boundary

只负责通用安全原语：

- ID 过滤、排序和去重；
- 表记录和用户锁；
- 并发快照比较；
- Session 撤销。

### 7.4 UserService

依赖收缩为：

```text
User Base
PermissionService
DepartmentService
bcrypt Verifier
auth.Boundary
```

UserService 不再直接持有 UserRole、Role、Department Base，不直接查询角色或部门关系。

PermissionService 和 DepartmentService 可以在各自领域边界内使用 Base、Model 和 Descriptor 完成关系存储，但这些类型不得通过公开方法泄漏给 UserService。迁移后 `modules/base/service` 的生产代码总量必须净减少，不能把 user.go 的实现原样搬到其他文件。

## 8. UserService 动作流程

### 8.1 Add

1. 读取已绑定输入中的 `roleIdList`。
2. 加密密码。
3. 校验并按统一顺序锁定角色、部门。
4. 调用 `Base.Add`。
5. 调用 PermissionService 写入角色关系。

用户新增继续只支持单对象；批量用户与角色关系的协议不在本轮扩展。

### 8.2 Update

1. 从 Mutable presence 判断角色、状态和密码是否提交。
2. 按角色、部门、用户顺序加锁。
3. 读取锁定后的当前用户及角色快照。
4. 校验最后管理员和并发快照。
5. 密码存在时执行 bcrypt 并递增 `passwordV`。
6. 调用 `Base.Update`。
7. 角色已提交时替换关系。
8. 密码、状态或角色变化时撤销 Session。

### 8.3 Delete

1. 规范化并锁定目标用户及相关角色。
2. 调用 PermissionService 保护最后管理员。
3. 清理角色关系。
4. 调用 `Base.Delete`。
5. 撤销目标用户 Session。

### 8.4 Info、List、Page

- Info 调用 Base 读取用户，并补充角色和部门字段；密码始终隐藏。
- List 未重写时直接使用 Base.List。
- Page 使用强类型 `UserPageReq`，由 codegen 自动选择现有 `HandleCRUDDTO` 管线，Controller 不重新声明 `/page`。
- Page 先查询用户分页，再按本页用户和部门 ID 各执行一次批量查询。

分页查询阶段固定为：

```text
1 个用户分页阶段（底层可包含 COUNT 和数据读取）
1 次本页角色
1 次本页部门名称
```

验收关注 SQL 总数为常数且不随本页用户数量增长，不把分页 COUNT 与数据读取误计为一条 SQL。

### 8.5 Move 与个人资料

Move、Person、PersonUpdate 是六个 CRUD 之外的独立业务接口，保留强类型 DTO。Move 可以直接绑定 `UserService.Move`，不再创建只做参数转发的 UserHandler。

## 9. 事务、锁与 Session

CRUD Dispatcher 是六个基础动作的唯一事务外壳。所有领域 Service 通过传入的 `ctx` 使用同一事务。

跨领域写操作统一按以下顺序加锁：

```text
角色 -> 部门 -> 用户 -> 关系写入
```

该顺序同时用于用户修改、用户删除和部门删除，避免当前不同方法交叉加锁造成死锁。

数据库写入和关系写入成功后、Dispatcher 提交前执行 Session 撤销。Session 后端错误会使数据库事务回滚；若最终数据库提交失败，Session 可能被额外撤销，但不会留下旧授权继续有效，安全性优先。

## 10. Controller 最终形态

用户 Controller：

- 使用 `controller.AllAPI()`；
- 声明 Entity、Service、InsertParam、HiddenFields 和 ReadonlyFields；
- 只额外声明 `/move`；
- 删除自定义 `/add`、`/update`、`/page`；
- 删除 UserHandler 的 Add、Update、Page、Move 转发方法。

Codegen 的最终分派为：

| 路由 | 实现 |
| --- | --- |
| add | UserService.Add |
| delete | UserService.Delete |
| update | UserService.Update |
| info | UserService.Info |
| list | Base.List |
| page | UserService.Page |
| move | UserService.Move |

## 11. 错误边界

- JSON、字段名、字段类型、null 和提交状态错误由 Binder 返回 Validate 异常。
- 用户、角色和部门业务条件由对应领域 Service 返回明确异常。
- 最后管理员保护返回 Comm 异常。
- 数据库、锁、bcrypt 和 Session 后端错误保留 cause 并包装为 Core 异常。
- 错误消息不得包含密码、摘要、Token 或 Session 标识。
- 密码由 HiddenFields 与业务响应 DTO 双重隔离。

## 12. 实施范围与顺序

1. 为 Descriptor、Binder、DO 投影和 Schema 增加 transient 支持及测试。
2. 扩展 codegen 的六动作重写识别、强类型查询 DTO 和业务返回结果支持。
3. 扩充 PermissionService 的角色关系、管理员保护和批量角色能力。
4. 扩充 DepartmentService 的范围、校验和批量名称能力。
5. 按新边界重写 UserService。
6. 恢复用户 Controller 的 `AllAPI()`，删除纯转发 Handler。
7. 删除已无调用方的用户 Add/Update DTO、转换函数和重复查询。
8. 重新生成 `modules/modules_gen.go`，确保二次生成无差异。

每个文件的修改一次完成，不改无关模块。框架新能力保持增量兼容，其他业务模块无需在本轮迁移。

## 13. 测试与验证

### 13.1 框架测试

- transient 可绑定并保留 presence；
- transient 不进入 Add/Update SQL、DO、Schema、索引和查询计划；
- 未知字段、非法类型和非法 null 仍被拒绝；
- 默认 Base 与六种 Service 重写静态分派正确；
- Info/List/Page 的业务返回类型正确生成 Adapter；
- List/Page 强类型 DTO 使用 CRUD Dispatcher，而非普通自定义路由；
- codegen 二次生成无差异。

### 13.2 业务测试

- 用户新增、更新、删除、详情、分页和移动；
- 角色未提交、清空和替换；
- 密码加密、密码版本和旧密码校验；
- 密码、状态和角色变化后的 Session 撤销；
- 删除、禁用和移除最后管理员；
- 用户与部门并发变更的锁顺序和快照校验；
- Page 响应字段、排序、筛选和数据权限不变；
- 分页 SQL 次数不随本页用户数量增长；
- MySQL、PostgreSQL、SQLite 行为一致。

### 13.3 全量门禁

```bash
go test ./...
go vet ./...
test -z "$(gofmt -l cool-next modules cmd main.go)"
go run ./cmd/cool generate
git diff --exit-code -- modules/modules_gen.go
```

## 14. 成功标准

| 指标 | 目标 |
| --- | ---: |
| UserService | 250-350 行 |
| 用户 Controller | 40-60 行 |
| 用户专用 DTO | 40-70 行 |
| 用户相关 Controller、DTO、Service 总量 | 约 500 行 |
| 用户分页查询 | 固定 3 个阶段，SQL 总数不随用户数量增长 |
| UserService/Controller 构造 Mutable/Input/DO | 0 处 |
| UserService 直接查询角色或部门 | 0 处 |
| `modules/base/service` 生产代码总量 | 相对迁移前净减少 |

行数用于证明框架确实降低了业务开发成本，不得以删减安全语义、压缩可读性或移动重复代码达标。最终还必须满足：普通单表模块只声明 Entity 和 Controller 即可获得六个基础接口。
