# Controller 瘦身：业务逻辑回归 Service 层

> 状态：待实施
> 关联：`2026-08-20-base-module-design.md`（职责划分原则）
> 实施者：另一 Agent；审核者：本文档作者
> 范围：P1 菜单导出/导入、P2 权限菜单编排、P3 用户 DTO 转换。**不含** `config.go` 的
> `go:embed` 问题——那项由 [`2026-08-21-seed-provider-codegen.md`](2026-08-21-seed-provider-codegen.md) 覆盖。

---

## 0. 依据与共同原则

### 0.1 Node 的边界（`.cursor/rules/controller.mdc` + 实际源码）

- `controller.mdc` 中即便是"重写 CRUD 实现"这种最复杂的定制场景，业务逻辑也写在
  `BaseSysMenuService` 里，Controller 只用 `@CoolController({service: ...})` 声明式接入，类体为空。
- `BaseCommController.permmenu()` 是**一行**：`this.baseSysPermsService.permmenu(roleIds)`，
  编排在 `BaseSysPermsService` 里（它自己注入了 `BaseSysMenuService`——**Service 之间互相依赖
  在 Node 是常规做法**，这一点直接决定了 P2 的做法）。
- `BaseSysUserController` 类体只有一个 `/move` 一行转发；`add`/`update`/`page` 全部由框架
  CRUD 委派给 `BaseSysUserService` 的同名覆盖方法。

### 0.2 本次遵循的判定标准

> **Controller 只做三件事**：取身份/参数、转发给 Service、返回。
> 出现以下任一情况即判定越界：直接访问数据库（`Model()`/`Scan()`）、多步跨 Service 编排、
> DTO 与领域对象之间的字段映射。

### 0.3 全局约束

- **不得改变任何对外 HTTP 契约**：路径、方法、请求/响应 JSON 字段名与结构、错误文案全部保持
  逐字节一致。前端 `cool-admin-vue` 已依赖这些协议。
- **纯搬迁，不做顺手优化**。搬迁过程中发现的其他问题记录下来单独提，不要混在同一次改动里。
- 仓库 `.gitignore` 排除 `*_test.go` 与 `/test/`，**无单元测试**。静态门禁是唯一保障。

### 0.4 共同验收标准（三项每项都要过）

1. `go build ./...` 通过
2. `go vet ./...` 无输出
3. `gofmt -l .` 无输出
4. `go run ./cmd/cool generate` 成功
5. `go run ./cmd/cool check` 输出「检查通过」
6. 幂等：连续两次 `generate`，第二次后 `modules_gen.go` 无新增改动
7. **路由零漂移**：`git diff modules/modules_gen.go` 中，凡是 `Path:` 字段的值必须与改动前
   完全一致。P2/P3 会改构造器签名，`modules_gen.go` 必然有 diff，但 diff **只应出现在**
   依赖注入相关的行，不应出现在任何 `Path:`/`Method:`/`Summary:` 上。改动前先
   `git show HEAD:modules/modules_gen.go | grep -o 'Path: "[^"]*"' | sort > /tmp/before.txt`
   存档，改完同样导出比对。

---

## 1. P1：菜单导出/导入回归 `MenuService`

### 1.1 问题

`modules/base/controller/admin/sys/menu.go` 的 `ExportMenu` 直接在 handler 里执行
`model.Fields(...).WhereIn(...).Scan(&rows)` + 排序 + 建树；`ImportMenu` 直接开事务写库。
对照 Node 的 `BaseSysMenuController.export` —— 只有 `return this.ok(await this.baseSysMenuService.export(ids))` 一行。

**这是上一轮重构引入的回归**，优先级最高。

### 1.2 同时删除 `codegen.BuildTree` / `codegen.InsertTree`

上一轮把树形组装/插入抽成了 `cool-next/codegen` 的两个泛型函数。现在判定这是**过度抽象**，
应当删除并内联回 `MenuService`：

| 理由 | 说明 |
|---|---|
| 各自只有一个调用方 | 典型的 speculative generality |
| 语义错位 | `codegen` 是 `cool generate` 的 AST 编译器包，放运行期数据库树操作不合理 |
| 会传染 | P1 若保留它们，`MenuService` 就得 `import cool-next/codegen`，读起来是"菜单服务依赖代码生成器" |
| 挪到第三处更差 | 为两个单调用方函数新建 `core/tree` 包，不划算 |

内联后菜单字段名回到业务代码里（它们本来就属于那里），框架层不再有任何菜单概念泄漏。

> 说明：这两个函数是本文档作者上一轮的过度设计，本次一并纠正。

### 1.3 目标形态

**`modules/base/service/menu.go` 新增：**

```go
// Export 导出选中的菜单树，不含维护字段。
func (service *MenuService) Export(ctx context.Context, ids []uint64) ([]dto.MenuTree, error)

// Import 在调用方事务中插入菜单树，并用实际新 ID 重建父子关系。
func (service *MenuService) Import(ctx context.Context, menus []dto.MenuTree) error
```

**从 `controller/admin/sys/menu.go` 迁入 `service/menu.go` 的符号**（原样搬，不改逻辑）：

- `menuExportRow`（结构体）
- `buildMenuTreeNode` → 内联进新的递归函数
- `menuTreeValues` / `menuTreeChildren` → 内联
- `menuExportRowID` / `menuExportRowParentID` → 删除（内联后不再需要函数形式）
- `stringValue` → 搬进 service 包。**已确认 service 包当前无同名符号**，不会冲突

**递归实现**：把 `codegen.BuildTree`/`InsertTree` 的泛型逻辑特化为菜单专用的两个私有函数，
建议命名 `buildMenuTree` / `importMenuTree`（这也是上一轮重构前的原始命名，便于对照
`git show a509d43` 校验行为一致）。

**关键：`importMenuTree` 必须保留显式设置 `createTime`/`updateTime` 的行为。**
当前经由 `codegen.InsertTree` → `seed.NewDO(descriptor, fields, true)` 完成。内联后可以继续
调用 `seed.NewDO`（零行为变化，推荐），也可以用 service 包已有的 `businessDO` +
显式时间字段。**若选后者必须确认时间字段确实被写入**——`businessDO`（user.go:812）
不会自动补时间戳。首选前者。

**环检测**：`codegen.BuildTree` 用 `ancestors map[uint64]bool` 做环截断，
`buildMenuTree` 必须保留这个保护，否则脏数据（父子成环）会导致无限递归打爆栈。

### 1.4 `MenuToolHandler` 改造后

```go
func (handler *MenuToolHandler) ExportMenu(ctx context.Context, request *dto.MenuExportReq) ([]dto.MenuTree, error) {
	if err := handler.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, exception.Validate("菜单导出请求不能为空")
	}

	return handler.menu.Export(ctx, request.IDs)
}
```

`ImportMenu` 同理。

**依赖调整**：`MenuToolHandler` 当前持有 `menu *coreservice.Base[entity.Menu, uint64]`
（框架泛型 Base），改为持有 `menu *service.MenuService`。注意 `sys` 包已经 import 了
`service` 包，无新增依赖。

`NewMenuToolHandler` 的参数相应从 `*coreservice.Base[entity.Menu, uint64]` 改为
`*service.MenuService`。**`MenuController` 的第一个参数已经是 `*service.MenuService`**，
`cool generate` 会把同一个实例注入两处，无需额外配置。

### 1.5 删除清单

- `cool-next/codegen/scaffold_tree.go` —— **整个文件删除**（`BuildTree`/`InsertTree` 是它
  仅有的内容）
- 确认删除后 `cool-next/codegen` 不再 import `cool-next/seed`（若 `scaffold.go`/
  `scaffold_write.go` 未用到），避免留下无用依赖

---

## 2. P2：权限菜单编排下沉 `PermissionService`

### 2.1 问题

`modules/base/controller/admin/comm.go` 的 `CommHandler.PermissionMenu` 内有四步编排：
取身份 → `permission.RoleIDs` → `permission.Permissions` → map 转有序 slice → `menu.List`。

### 2.2 目标形态

严格对齐 Node 的 `BaseSysPermsService.permmenu(roleIds)`。

**`modules/base/dto/menu.go` 新增**（放这里而非 `user.go`，因为它主要是菜单结构）：

```go
// PermissionMenuResult 是当前用户的权限标识与菜单树。
type PermissionMenuResult struct {
	Perms []string       `json:"perms"`
	Menus []MenuListItem `json:"menus"`
}
```

原 `controller/admin/comm.go` 中的 `PermissionMenuResult` 删除。**注意 JSON 字段名
`perms`/`menus` 必须保持不变。**

**`modules/base/service/permission.go` 新增：**

```go
// PermissionMenu 返回当前管理员的权限标识与可见菜单树，对齐 Node 的 permmenu。
func (service *PermissionService) PermissionMenu(ctx context.Context) (dto.PermissionMenuResult, error)
```

方法体 = 现 `CommHandler.PermissionMenu` 的 L61-84 原样搬入，把 `handler.permission.` 换成
`service.`、`handler.menu.List(ctx)` 换成 `service.menu.List(ctx)`。

**权限标识排序必须保留 `sort.Strings(perms)`** —— `Permissions` 返回的是
`map[string]struct{}`，Go 的 map 遍历顺序随机，去掉排序会导致同一用户每次请求拿到的
`perms` 数组顺序不同。

### 2.3 依赖注入改造（本项唯一有风险的部分）

`PermissionService` 需要新增 `*MenuService` 依赖：

```go
type PermissionService struct {
	userRole *coreservice.Base[entity.UserRole, uint64]
	role     *coreservice.Base[entity.Role, uint64]
	roleMenu *coreservice.Base[entity.RoleMenu, uint64]
	menu     *coreservice.Base[entity.Menu, uint64]
	menuService *MenuService   // 新增
}
```

`NewPermission` 增加末位参数 `menuService *MenuService`，并加入 nil 校验。

**已核对无循环依赖**：`MenuService` 的构造依赖是
`runtime / menu(Base) / role(Base) / roleMenu(Base) / userRole(Base) / sessions`，
不含 `PermissionService`。实施者**仍须自行复核一遍**，因为 P1 会改 `MenuService`，
且 `cool generate` 对构造器循环依赖的报错信息不够直观。

> 命名说明：结构体里同时有 `menu`（Base）和 `menuService`（业务 Service）两个字段，
> 名字接近容易混淆。可以把已有的 `menu` 字段改名为 `menuBase` 以示区分——但这会波及
> `Permissions` 方法内的引用，属于"顺手优化"，**建议单独一次提交**，不要混进 P2。

### 2.4 `CommHandler` 改造后

```go
func (handler *CommHandler) PermissionMenu(ctx context.Context) (dto.PermissionMenuResult, error) {
	if handler == nil || handler.permission == nil {
		return dto.PermissionMenuResult{}, exception.Core("Base 权限菜单接口未初始化")
	}

	return handler.permission.PermissionMenu(ctx)
}
```

`CommHandler` 的 `menu *service.MenuService` 字段随之**不再需要**——检查
`NewCommHandler` 是否还有其他地方用到它，无则一并从结构体和构造器参数中删除
（`modules_gen.go` 会相应少一条注入边）。

---

## 3. P3：用户 DTO 转换下沉 `UserService`

### 3.1 问题

`controller/admin/sys/user.go` 有约 100 行 DTO→领域对象映射：
`UserAddReq.input()`、`UserUpdateReq.input()`、`appendPresentUserFields()`、`appendUserField()`。

### 3.2 **本项最关键的语义：三态字段更新**

`UserUpdateReq` 通过自定义 `UnmarshalJSON` 维护一个 `submitted map[string]bool`，实现
**三态**语义：

| 请求 JSON | `submitted["phone"]` | 行为 |
|---|---|---|
| 不含 `phone` 键 | `false` | **不更新该列** |
| `"phone": null` | `true`，指针为 nil | **更新为 NULL**（`coreservice.Null`） |
| `"phone": "123"` | `true`，指针非 nil | 更新为 `"123"`（`coreservice.Value`） |

**这套语义必须逐字保留。** 任何简化（比如只判断指针是否为 nil）都会让"清空手机号"
与"不修改手机号"变得无法区分，属于静默数据错误，且没有测试能发现。

### 3.3 类型迁移

`UserPageReq` / `UserAddReq` / `UserUpdateReq` 从 `controller/admin/sys/user.go` 迁到
`modules/base/dto/user.go`（与已有的 `dto.UserMoveReq` / `dto.UserRoleInput` 一致）。

`UnmarshalJSON` 跟随类型一起迁移——它是 HTTP 解码关注点，留在 DTO 上是正确的。

**跨包访问 `submitted` 的问题**：`submitted` 是私有字段，迁到 `dto` 包后 `service` 包读不到。
解决方式是在 `dto` 包给 `UserUpdateReq` 加一个导出访问器：

```go
// HasField 报告请求 JSON 是否显式提交了该字段，用于区分「不更新」与「更新为 NULL」。
func (request *UserUpdateReq) HasField(name string) bool {
	return request != nil && request.submitted[name]
}
```

**不要把 `submitted` 改成导出字段**——那会暴露可变 map，调用方能篡改三态判定。

### 3.4 转换逻辑迁移

`input()` 两个方法 + `appendPresentUserFields` + `appendUserField` 全部迁入
`modules/base/service/user.go`，改为包内私有函数（不再是 DTO 的方法）：

```go
func userAddInput(descriptor coreentity.Descriptor[entity.User, uint64], request dto.UserAddReq, userID uint64) (coreservice.AddInput[entity.User], error)
func userUpdateInput(descriptor coreentity.Descriptor[entity.User, uint64], request *dto.UserUpdateReq) (coreservice.UpdateInput[entity.User, uint64], *[]uint64, error)
```

`userUpdateInput` 内部把原来的 `request.submitted[x]` 改为 `request.HasField(x)`。

### 3.5 `UserService` 新增入口方法

```go
// Add 新增用户并写入角色关系。
func (service *UserService) Add(ctx context.Context, request dto.UserAddReq, operatorID uint64) (coreservice.AddResult[uint64], error)

// Update 更新用户并按提交状态替换角色关系。
func (service *UserService) Update(ctx context.Context, request *dto.UserUpdateReq) error
```

内部调用 `userAddInput`/`userUpdateInput` 得到领域输入，再走**现有的**
`AddWithRoles`/`UpdateWithRoles`。

> **不要删除或改签名 `AddWithRoles`/`UpdateWithRoles`**。它们承载事务、加锁、管理员保护等
> 全部核心逻辑，本次只在其上加一层 DTO 适配。

**`operatorID` 为什么由 Controller 传入**：`userAddInput` 需要把当前登录用户 ID 写进
`userId` 列。取身份（`auth.Admin(ctx)`）是 Controller 的职责（见 §0.2），所以由 Controller
取出后作为参数传入，Service 不直接调 `auth.Admin`。这与仓库现有惯例一致——
`sys/role.go`/`sys/department.go` 的 `InsertParam` 闭包也是在 Controller 层取身份。

### 3.6 `UserHandler` 改造后

```go
func (handler *UserHandler) Add(ctx context.Context, request *dto.UserAddReq) (coreservice.AddResult[uint64], error) {
	identity, err := auth.Admin(ctx)
	if err != nil {
		return coreservice.AddResult[uint64]{}, err
	}

	return handler.user.Add(ctx, *request, identity.UserID)
}

func (handler *UserHandler) Update(ctx context.Context, request *dto.UserUpdateReq) error {
	return handler.user.Update(ctx, request)
}
```

`Page` 方法里的默认值兜底（`page == 0 → 1`、`size == 0 → 15`）**保留在 Controller**——
那是 HTTP 参数默认值，不是业务规则。

### 3.7 迁移后校验

改完用以下方式确认三态语义未破坏（无测试，只能靠代码比对）：

```bash
git show HEAD:cool-admin-go-next/modules/base/controller/admin/sys/user.go > /tmp/user_before.go
```

把 `/tmp/user_before.go` 里的 `input()`/`appendPresentUserFields`/`appendUserField` 与迁移后
`service/user.go` 中的对应函数**逐行对读**，确认：

- `appendUserField` 的三个 `case`（`*uint64`/`*int32`/`*string`）分支完全一致，
  nil → `coreservice.Null(name)`，非 nil → `coreservice.Value(name, *current)`
- `appendPresentUserFields` 的 8 个字段列表（departmentId/name/nickName/headImg/phone/
  email/remark/status）顺序与内容一致
- `UserAddReq.input` 中 `userId`/`username`/`password` 三个**无条件**字段仍然无条件写入
- `UserUpdateReq.input` 中 password 的特殊处理仍在：
  `submitted["password"] && request.Password != nil && strings.TrimSpace(*request.Password) != ""`
  ——**三个条件缺一不可**，少一个会导致空密码被写库
- `roleIdList` 的 `*[]uint64` 三态（未提交 → nil；提交为 null/空 → 空切片指针）保持

---

## 4. 执行顺序与提交粒度

**一项一个提交**，每项独立跑完 §0.4 全部七条验收再进入下一项。

| 顺序 | 项 | 风险 | 说明 |
|---|---|---|---|
| 1 | P1 | 低 | 纯搬迁 + 删一个文件，不动 DI 图签名 |
| 2 | P2 | 中 | 动 `NewPermission` 签名，新增一条 Service→Service 注入边 |
| 3 | P3 | 中高 | 三态语义最容易被无意破坏 |

P3 放最后，是为了让前两项先验证流程与门禁都跑得通。

**若任一项做完 `cool check` 不过且原因不明，停下来把现状报告出来，不要继续叠加下一项改动。**

---

## 5. 风险与陷阱

| 风险 | 后果 | 对策 |
|---|---|---|
| P3 三态语义被简化 | 「清空字段」与「不修改字段」混淆，静默数据错误 | §3.2 + §3.7 逐行对读 |
| P3 password 三条件少一个 | 空密码或空白密码被写入 | §3.7 明确列出 |
| P2 忘记 `sort.Strings(perms)` | 同一用户每次请求 `perms` 顺序随机 | §2.2 |
| P1 丢掉环检测 `ancestors` | 脏数据导致递归爆栈 | §1.3 |
| P1 丢掉 `createTime`/`updateTime` 显式设置 | 导入的菜单时间字段为空 | §1.3，首选继续用 `seed.NewDO` |
| P2 循环依赖 | `cool generate` 报错且信息不直观 | §2.3 先复核 |
| 响应字段名漂移 | 前端 `cool-admin-vue` 静默失效 | §0.3 + §0.4 第 7 条路由存档比对 |
| `stringValue` 命名冲突 | 编译失败（易发现，低危） | 已确认 service 包无同名符号 |

---

## 6. 审核关注点（供审核者）

1. `git diff modules/modules_gen.go` 里**没有任何 `Path:` 行发生变化**
2. P3 的 `appendUserField` 三个 case 分支与迁移前逐字一致
3. P3 的 password 三条件、`roleIdList` 三态完整保留
4. `dto.UserUpdateReq.submitted` 仍是私有字段，只通过 `HasField` 暴露
5. P2 的 `sort.Strings(perms)` 还在
6. P1 的 `ancestors` 环检测还在；`createTime`/`updateTime` 仍被显式设置
7. `cool-next/codegen/scaffold_tree.go` 已删除，且 `codegen` 包无遗留的 `seed` 无用 import
8. `CommHandler` 里已无用的 `menu` 字段是否清理干净
9. 三个 Controller 文件里**不再出现** `Model(`/`Scan(`/`.Tx(` 等直接数据库调用
