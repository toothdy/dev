# 菜单与部门局部更新修复实施计划

**Goal:** 修复菜单和部门更新将未提交列写成 `NULL` 的问题，同时兼容 Vue 完整表单中的只读字段，并保留严格校验、租户作用域和现有性能特征。

**Architecture:** 菜单和部门各自使用一个纯转换函数，根据请求 key 是否存在同时生成 mutation row 和数据库字段列表。更新继续走现有事务、行锁和 `tenant.ScopedModel`，但使用 GoFrame `Fields(fields).Data(row).Update()` 限制 `SET` 列。

**Tech Stack:** Go 1.23、GoFrame v2.10.2、MySQL、现有 CRUD/租户/模型元数据。

**Design:** `docs/superpowers/specs/2026-07-29-partial-update-design.md`

**Status:** 设计已批准，待实施。

## 约束

- 同时修复菜单和部门，不扩展到通用 CRUD Runtime。
- 静默忽略 `tenantId`、`createTime`、`updateTime`；部门额外忽略 `userId`。
- 其他未知字段继续拒绝。
- 缺失字段不更新，可空字段的显式 `nil` 写入 `NULL`，`false` 和 `0` 必须保留。
- 不使用 `OmitEmptyData` 或 `OmitNilData`。
- 不引入第三方依赖，不修改 Vue 和 Midway 项目。
- 不增加数据库查询、事务范围或锁范围。

## Task 1：先用单元测试锁定局部更新契约

**Files:**

- Modify: `modules/base/service/sys/menu_test.go`
- Modify: `modules/base/service/sys/department_test.go`

1. 为菜单更新转换函数先添加失败测试，覆盖：
   - `isShow: false` 输出 `is_show`，row 值保持 `false`。
   - `orderNum: 0` 输出 `order_num`，row 值保持 `0`。
   - `parentId: nil` 输出 `parent_id`，row 值保持 `nil`。
   - 未提交的 `name`、`router`、`perms` 等不出现在 fields。
   - 非空字段明确为 `nil` 返回 Validate 错误。
2. 为部门更新转换函数添加对称测试，覆盖 `orderNum: 0`、`parentId: nil`、缺失字段和非空 `nil`。
3. 用确定性字段顺序断言 fields，避免 map 遍历顺序导致测试波动。
4. 运行聚焦测试，确认因转换函数尚未实现而失败。

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./modules/base/service/sys -run 'Test(Menu|Department)UpdateMutation' -count=1
```

## Task 2：实现菜单局部更新数据转换

**Files:**

- Modify: `modules/base/service/sys/menu.go`
- Modify: `modules/base/service/sys/menu_test.go`

1. 定义确定顺序的菜单 JSON 字段到数据库列的映射：
   - `parentId -> parent_id`
   - `name -> name`
   - `router -> router`
   - `perms -> perms`
   - `type -> type`
   - `icon -> icon`
   - `orderNum -> order_num`
   - `viewPath -> view_path`
   - `keepAlive -> keep_alive`
   - `isShow -> is_show`
2. 实现纯转换函数，仅在请求中存在 key 时设置 row 并加入 fields。
3. 明确校验菜单非空字段：`name`、`type`、`orderNum`、`keepAlive`、`isShow`。
4. `name` 如果提交，要求为非空字符串并使用去除首尾空白后的值。
5. 保持 `parentId == id` 的现有校验；`parentId: nil` 不进行父节点存在性查询。
6. 在未知字段校验前删除 `tenantId`、`createTime`、`updateTime`。
7. 设置服务端 `UpdateTime`，在 fields 末尾加入 `update_time`。
8. 将更新语句改为 `Fields(fields).Where("id", id).Data(row).Update()`，不改动事务、目标行锁、父节点锁和租户作用域。
9. 运行菜单聚焦测试，确认 Task 1 的菜单用例通过。

## Task 3：实现部门局部更新数据转换

**Files:**

- Modify: `modules/base/service/sys/department.go`
- Modify: `modules/base/service/sys/department_test.go`

1. 定义确定顺序的部门字段映射：`name -> name`、`parentId -> parent_id`、`orderNum -> order_num`。
2. 实现部门纯转换函数，使用 key 存在性区分缺失与显式 `nil`。
3. 校验 `name` 和 `orderNum` 不得明确为 `nil`，`name` 如果出现必须是非空字符串。
4. 在未知字段校验前删除 `tenantId`、`createTime`、`updateTime`、`userId`。
5. 设置服务端 `UpdateTime` 并追加 `update_time`。
6. 使用 `Fields(fields).Where("id", id).Data(row).Update()`，保留现有关系作用域、事务、目标/父节点锁和错误包装。
7. 运行部门聚焦测试，确认 Task 1 的部门用例通过。

## Task 4：增加 SQL 结构回归测试

**Files:**

- Modify: `modules/base/service/sys/param_test.go`
- Modify: `modules/base/service/sys/menu_test.go`
- Modify: `modules/base/service/sys/department_test.go`

1. 扩展 `newBaseTenantServiceTestDB` 的预置表字段，包含菜单和部门的更新列及 `base_sys_department` 表。
2. 使用 `gdb.ToSQL` 和 `tenant.ScopedModel` 生成局部更新 SQL，不连接真实 MySQL。
3. 对菜单 `isShow: false` 断言：
   - `SET` 包含 `is_show` 和 `update_time`。
   - `SET` 不包含 `create_time`、`tenant_id`、`name` 等缺失列。
   - `WHERE` 包含当前租户谓词。
4. 对菜单 `parentId: nil` 断言 SQL 包含 `parent_id = NULL`。
5. 对部门 `orderNum: 0` 和 `parentId: nil` 做对称断言。
6. 确认 DryRun SQL 不增加任何额外读查询。

## Task 5：扩展真实 MySQL 关系作用域测试

**Files:**

- Modify: `modules/base/service/sys/relation_scope_integration_test.go`

1. 在现有租户关系集成场景中构建 `MenuService`，复用已有菜单和部门 fixture。
2. 执行菜单开关局部更新，断言 `is_show` 改变，`name`、`create_time`、`tenant_id` 保持不变。
3. 执行带 `createTime`、`updateTime`、`tenantId` 的完整菜单表单更新，断言请求成功且只读列不变。
4. 执行部门 `orderNum: 0` 和 `parentId: nil` 局部更新，断言其他列保持不变。
5. 保留并重跑现有跨租户父节点拒绝用例，确认失败更新不会部分写入。
6. 运行真实 MySQL 聚焦用例：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build COOL_CUSTOM_API_INTEGRATION=1 go test -p=1 ./modules/base/service/sys -run TestRelationScopeMySQLIntegration -count=1
```

## Task 6：综合验证与实施提交

1. 格式化受影响 Go 文件并检查 diff：

```bash
gofmt -w modules/base/service/sys/menu.go modules/base/service/sys/menu_test.go modules/base/service/sys/department.go modules/base/service/sys/department_test.go modules/base/service/sys/param_test.go modules/base/service/sys/relation_scope_integration_test.go
git diff --check
```

2. 运行 Base Service 聚焦测试：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./modules/base/service/sys -count=1
```

3. 运行受影响 Base 测试：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./modules/base -count=1
```

4. 运行全量单元测试和静态检查：

```bash
env GOCACHE=/private/tmp/cool-admin-go-build go test ./... -count=1
go vet ./...
```

5. 核对最终 SQL 和 Git diff，确认：
   - 菜单开关更新仅写 `is_show`、`update_time`。
   - 部门局部更新不写未提交列。
   - 不存在 `OmitEmptyData`、`OmitNilData` 或新依赖。
   - 没有修改 Vue、Midway 或通用 CRUD Runtime。
6. 单独提交实施文件，提交信息建议为 `fix(base): preserve partial update fields`。

## 验收输出

- 菜单和部门局部更新实现。
- 纯函数、DryRun SQL 和真实 MySQL 三层回归结果。
- 实际开关请求和完整表单请求的 SQL 列集合。
- 全量测试、`go vet` 和未执行外部依赖测试的边界说明。
