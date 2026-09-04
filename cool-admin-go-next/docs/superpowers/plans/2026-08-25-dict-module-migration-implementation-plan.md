# 字典模块迁移实施计划

> 日期：2026-08-25  
> 依据：`docs/superpowers/specs/2026-08-25-dict-module-migration-design.md`  
> 状态：实施中

## 1. 目标

在 Go v2 现有静态模块、Descriptor、CRUD、事务、Auth、Seed 和 HTTP Transport 上实现 Dict 业务模块，使 `cool-admin-vue` 无需修改即可复用 Node 字典模块的接口和行为。

## 2. 实施约束

1. 不新增第三方依赖，不运行 `gf init`；
2. 不手写 Descriptor、DO 或 `modules/modules_gen.go`，统一使用 `cool generate`；
3. 数据库查询使用 GoFrame 参数化 ORM，写入和删除复用现有基础 Service 与删除归档；
4. 不增加多租户、完整 i18n 工具链、通用树抽象或数据库方言 SQL；
5. 每次完整修改一个文件，再处理下一个文件；
6. 当前仓库忽略 `*_test.go`，测试文件仍保留在工作区并作为本次验证依据；
7. 不覆盖 `cool-admin-vue/build/cool/eps.d.ts` 的现有用户修改。

## 3. 任务

### 任务 1：模块、DTO、实体与种子

文件：

- 创建：`modules/dict/config.go`
- 创建：`modules/dict/dto/info.go`
- 创建：`modules/dict/entity/type.go`
- 创建：`modules/dict/entity/info.go`
- 创建：`modules/dict/db.json`
- 创建：`modules/dict/entity/entity_test.go`

步骤：

1. 保持 Node 模块名称、描述和顺序；
2. 建立 `dict_type`、`dict_info` 实体，不增加唯一索引、外键或租户字段；
3. 建立共享 `types` 请求 DTO；
4. 原样迁移 Node 字典初始数据；
5. 编译 Descriptor 并验证字段、默认值和可空性。

验证：

```text
go test ./modules/dict/entity -count=1
```

### 任务 2：字典查询、解析与级联删除

文件：

- 创建：`modules/dict/service/info.go`
- 创建：`modules/dict/service/type.go`
- 创建：`modules/dict/service/service_test.go`

步骤：

1. 实现全量或按 key 聚合、字段白名单和稳定排序；
2. 用标准库实现 Node `Number` 与 `parseInt` 所需的兼容转换；
3. 实现单值和批量字典名称解析；
4. `InfoService.Delete` 收集完整后代后委托基础删除；
5. `TypeService.Delete` 删除类型并委托信息基础 Service 清理全部关联项；
6. 两个删除 override 直接委托各自嵌入的 `Base.Delete`，使生成器选择 Delegate 模式并保留 CRUD ActionPlan 与同一事务。

验证：

```text
go test ./modules/dict/service -count=1
```

### 任务 3：Controller、权限与契约

文件：

- 创建：`modules/dict/controller/admin/type.go`
- 创建：`modules/dict/controller/admin/info.go`
- 创建：`modules/dict/controller/app/info.go`
- 创建：`modules/dict/controller/contract_test.go`
- 修改：`cool-next/auth/permission.go`
- 修改：`cool-next/auth/auth_test.go`

步骤：

1. 声明两个 Admin CRUD Controller 和一个 App Controller；
2. 保持 List 查询字段与追加排序；
3. Admin `types`、App `data/types` 标记公开；
4. 在权限推导根因位置为 Admin `data` 增加精确的“仅登录”路径；
5. 固定全部路由的方法、路径、绑定来源、权限和公开标签。

验证：

```text
go test ./cool-next/auth ./modules/dict/controller -count=1
```

### 任务 4：静态生成与工程验收

文件：

- 生成：`modules/modules_gen.go`

步骤：

1. 执行 `cool generate`，检查 Dict Descriptor、Service、Controller、Seed 和 EPS 装配；
2. 执行 `cool check`；
3. 跑 Dict 专项、全量测试、`go vet` 和 `gofmt`；
4. 启动本地服务，核验 EPS 与字典 HTTP 链路；
5. 检查最终 diff 只包含本次模块与必要权限兼容点。

验证：

```text
go run ./cmd/cool generate
go run ./cmd/cool check
go test ./modules/dict/... ./cool-next/auth -count=1
go test ./... -count=1
go vet ./...
```

## 4. 完成条件

1. Node 字典模块全部路由可由同一前端调用；
2. 聚合、数值转换、名称解析和两类级联删除行为一致；
3. 权限公开范围不扩大；
4. Seed、EPS 和静态装配包含 Dict；
5. 专项与全量质量门禁通过。
