# Base Service 行为保持简化实施计划

> 日期：2026-08-29
> 依据：`docs/superpowers/specs/2026-08-29-base-service-simplification-design.md`
> 状态：已实施

## 约束

1. 保留事务、锁顺序、关系快照、最后管理员保护和 Session 撤销。
2. 保留数据库、哈希、权限和外部输入边界的错误处理。
3. 相信构造流程和 `Mutable` 已建立的字段类型约束。
4. CRUD 与控制器入口名称保持稳定，避免无收益的生成文件变更。
5. 每次完整修改一个文件，再格式化并运行相关测试。
6. 不覆盖工作区已有修改，所有简化基于当前内容继续进行。

## 任务 1：建立基线

运行 Base 模块测试，记录当前失败；检查五个目标文件和相关测试的现有 diff。

```bash
go test ./modules/base/... -count=1
```

## 任务 2：简化 user.go

- receiver 改为 `s`，局部变量使用 `req`、`roles`、`deptIDs`、`oldRoles`、`newRoles`。
- 删除 nil receiver 检查、死代码和单调用状态辅助函数。
- 简化角色、部门字段读取，删除不可能的类型 error 通道。
- 直接赋值个人资料字段，不使用闭包表。
- 按 GoFrame 官方建议删除 `sql.ErrNoRows` 判断，只保留 nil 结果判断。
- 同步调整已有相关测试。

验证：

```bash
go test ./modules/base/service -run 'TestUser' -count=1
```

## 任务 3：简化 role.go

- receiver 改为 `s`，权限变量统一为 `perms`。
- 合并 `AddWithPermissions` 到 `Add`、`UpdateWithPermissions` 到 `Update`。
- 删除权限解包、更新项和 DTO 转换的单调用包装。
- 简化 `Mutable` 权限字段读取。
- 删除 `sql.ErrNoRows` 判断，保持 nil 结果语义。

验证：

```bash
go test ./modules/base/service -run 'TestRole' -count=1
```

## 任务 4：简化 permission.go

- receiver 改为 `s`，缩短角色、菜单和快照局部变量。
- 删除构造成功后各方法重复执行的内部依赖校验。
- 删除仅由测试调用的 `LockUserRoleChanges` 和 `flatVisibleMenus`。
- 同步测试，使其直接覆盖保留的底层行为。
- 保留所有授权、快照、管理员保护和 Session 撤销顺序。

验证：

```bash
go test ./modules/base/service -run 'TestPermission' -count=1
```

## 任务 5：简化 department.go

- receiver 改为 `s`，部门变量缩短为 `deptID`、`deptIDs`。
- 内联单调用的更新项与树 ID 包装。
- 将父级字段读取简化为 `parentID`，删除不可能的类型分支。
- 删除重复 service 初始化检查和 `sql.ErrNoRows` 判断。
- 保留树删除、锁、用户迁移和关系清理。

验证：

```bash
go test ./modules/base/service -run 'TestDepartment' -count=1
```

## 任务 6：简化 menu.go

- receiver 改为 `s`，菜单和父级变量使用惯用短名。
- 内联单调用的更新项包装。
- 简化父级字段读取，删除不可能的类型分支。
- 删除重复 service 初始化检查和 `sql.ErrNoRows` 判断。
- 保留树循环检测、后代锁、导入导出和 Session 撤销。

验证：

```bash
go test ./modules/base/service -run 'TestMenu' -count=1
```

## 任务 7：全量验收

```bash
gofmt -w modules/base/service/user.go modules/base/service/role.go modules/base/service/permission.go modules/base/service/department.go modules/base/service/menu.go
go test ./modules/base/... -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

验收时确认没有新增依赖、没有新增辅助层、目标生产代码净减少，并且已有未提交修改仍然存在。

## 验收结果

- `go test ./modules/base/... -count=1`：通过。
- `go vet ./...`：通过。
- `git diff --check`：通过。
- `go test ./... -count=1`：除 `cool-next/codegen` 的既有组件拓扑顺序失败外，其余包通过；失败涉及工作区原有的生成/拓扑改动，与本次 Base Service 简化无关。
