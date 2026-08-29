# Base Service 行为保持简化设计

## 目标

在完整保留现有业务、事务和并发语义的前提下，简化 Base 模块中五个主要 Service：

- `modules/base/service/user.go`
- `modules/base/service/role.go`
- `modules/base/service/permission.go`
- `modules/base/service/department.go`
- `modules/base/service/menu.go`

删除死代码、重复防御和无意义包装层，缩短函数名与变量名，使主流程更接近直接、清晰的 Go 代码。

## 非目标

- 不改接口响应、数据库结构和权限结果。
- 不删除事务锁、角色快照、最后管理员保护和 Session 撤销。
- 不重写 `Mutable`、CRUD、认证或数据库核心框架。
- 不为兼容旧私有名称保留转发函数。
- 不把文件系统和外部输入边界的必要校验视为过度防御。

## 简化原则

1. 只保留被复用或能明显隔离复杂业务的辅助函数。
2. 单次调用且只包装简单判断、字段复制或参数转发的函数直接内联。
3. 相信构造流程和 `Mutable` 已建立的类型约束，不重复检查理论上不可达的状态。
4. 数据库、哈希、事务和权限边界的错误处理全部保留。
5. 名称短而明确，不使用需要读上下文才能理解的随意缩写。
6. CRUD 和控制器直接依赖的方法保持稳定，避免无收益的生成文件修改。

## 命名约定

- Service receiver 使用 `s`。
- 请求使用 `req`，权限使用 `perms`。
- 部门 ID 使用 `deptID` 或 `deptIDs`。
- 当前和下一角色使用 `oldRoles`、`newRoles`。
- 授权变更使用 `authChanged`，撤销标记使用 `shouldRevoke`。
- 布尔值保留 `is`、`has`、`can`、`should` 前缀。
- `ctx`、`err`、`id`、`ids`、`row` 等沿用 Go 惯用短名。

## 文件设计

### user.go

- 删除无调用的转换函数。
- 内联只使用一次的状态和持久字段判断。
- 简化角色、部门字段读取，删除 `Mutable` 已保证类型后的 error 通道。
- 简化个人资料字段赋值，避免闭包表驱动。
- 保留密码哈希、角色锁、快照校验、管理员保护和 Session 撤销。

### role.go

- 将 `AddWithPermissions` 合并到 `Add`，将 `UpdateWithPermissions` 合并到 `Update`。
- 删除只负责转发或解包的权限辅助函数。
- 简化菜单和部门权限字段读取。
- 保留事务、资源锁、关系替换、角色可见范围和管理员角色保护。

### permission.go

- 删除没有生产调用、只被测试覆盖的公开包装方法。
- 删除构造成功后各方法重复执行的内部依赖校验。
- 缩短快照、角色和菜单相关局部名称。
- 将仅在 `service` 包内调用的领域方法改为非导出短名：

| 原名称 | 新名称 |
| --- | --- |
| `RolesByUsers` | `roles` |
| `RoleSnapshot` | `roleSnap` |
| `ValidateRoleSnapshot` | `checkSnap` |
| `PrepareRoleChange` | `prepRoles` |
| `LockRoles` | `lockRoles` |
| `LockUsers` | `lockUsers` |
| `ReplaceRoles` | `setRoles` |
| `DeleteUserRoles` | `delRoles` |
| `AdminRoleIDs` | `adminRoles` |
| `EnsureAdminTransition` | `checkAdmin` |
| `EnsureNotLastAdmin` | `keepAdmin` |
| `RevokeUsers` | `revoke` |

- 保持 `Authorize`、`RoleIDs`、`IsAdmin` 和 `PermissionMenu` 等实际对外入口不变。
- 保留授权判断、关系快照、最后管理员校验和 Session 撤销。

### department.go

- 内联更新项、树 ID 等单调用小函数。
- 简化父级字段读取，删除不可能的类型分支。
- 缩短树、部门和用户相关局部名称。
- 保留树删除、部门锁、用户迁移和角色关系清理。

### menu.go

- 内联更新项等单调用小函数。
- 简化父级字段读取，删除不可能的类型分支。
- 缩短树、父级和用户相关局部名称。
- 保留循环检测、后代锁、导入导出和受影响用户 Session 撤销。

## 数据与错误流

控制器和 CRUD 层继续产生现有 `AddInput`、`UpdateInput`、`DeleteInput` 和 `Query`。Service 仍在相同事务边界内读取字段、锁定资源、执行数据库变更、维护关系并撤销 Session。简化只减少中间包装，不改变调用顺序、提交字段语义或错误传播。

## 验证

每次只完成一个文件的修改，然后执行格式化和对应测试。五个文件完成后运行：

```bash
go test ./modules/base/...
go test ./...
go vet ./...
```

同时检查 `gofmt -l`、未使用符号、生产代码中仅由测试调用的辅助函数，以及现有未提交修改是否完整保留。
