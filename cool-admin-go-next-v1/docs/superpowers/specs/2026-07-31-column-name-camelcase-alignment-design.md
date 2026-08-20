# 数据库列名 camelCase 对齐设计

日期：2026-07-31

## 1. 问题

`modules/base/service/sys/user.go` 等 service 里存在大量字段映射代码，例如 `userUpdateData` 用一张 `columns` 表把 `departmentId` 翻译成 `department_id`。同类代码在 `user.go`、`role.go`、`department.go`、`menu.go`、`param.go` 五个 service 重复出现，新增一个字段要改四处。

这些映射不是偶然写出来的，它们是一个更上游缺陷的产物。

## 2. 根因

`cool-admin` 生态的数据库列名本来就是 camelCase，而 `cool-admin-go-next` 单方面用了 snake_case，于是每一层都得翻译一遍。

证据链：

**Node 版（cool-admin-midway）** —— TypeORM 默认命名策略，列名直接取属性名：

```ts
// src/modules/base/entity/sys/user.ts
@Column({ comment: '部门ID', nullable: true })
departmentId: number;
```

`BaseEntity` 同样是 `id` / `createTime` / `updateTime` / `tenantId`。

**老 Go 版（cool-admin-go）** —— 显式指定 camelCase 与 Node 对齐：

```go
DepartmentID uint `gorm:"column:departmentId;type:bigint;index"`
```

**本项目的契约文档** —— `docs/protocol/base-api-contract.md:271` 的表结构契约：

> `base_sys_user` | `id`, `departmentId`, `userId`, `name`, `username`, `password`, `passwordV`, `nickName`, `headImg`, `phone`, `email`, `remark`, `status`, `socketId`, `createTime`, `updateTime`, `tenantId`

**冲突点**：`docs/superpowers/specs/2026-07-14-cool-admin-go-next-schema-sync-design.md` 第 127 行写「字段以 base-api-contract.md 的表结构契约为准」，但第 94、151 行又写「DB 字段使用 snake_case」。契约本身是 camelCase，后者与之矛盾，推测是照搬 GoFrame 惯例时未与契约核对。

**后果**：go-next 建出来的表与 Node 版不是同一张表，同库共用不可能。前端契约虽对得上，数据库层已分叉。

## 3. 现状量化

全量比对 Node 22 张表与 go-next 16 张表的结果：

| 项 | 数量 |
|---|---|
| `entity.NewField` 调用总数 | 98 |
| 其中 JSONName ≠ ColumnName | 51 |
| 去重后待改列名 | 42 |
| **待替换字面量合计** | **738 处** |
| ├─ 生产代码 `.go` | 383 处 / 35 个文件 |
| ├─ 测试代码 `.go` | 255 处 |
| └─ yaml / json / md | 100 处 |
| 测试文件总数 / 含 snake_case 断言 | 150 / 55 |

注：全项目 snake_case 字面量共约 976 处，但其中相当一部分是**表名**（`base_sys_user` 45 处、`base_sys_department` 15 处、`demo_goods` 14 处等）。表名两边一致，不在替换范围。真正待替换的是上表 738 处。

**关键可行性依据**：全项目**不存在动态列名转换**——`grep` 确认无 `CaseSnake` / `CaseCamel` / `ToSnake` 调用，也无字符串拼接列名的写法。所有列名均为带引号的硬编码字面量，因此 §6.1 的 42 项精确替换可做到完全覆盖，不存在运行期动态生成的漏网情况。

**字段集合本身是一致的**，13 张共有表字段完全对齐。真实差异仅三处，均为 go-next 主动增强而非缺陷：`recycle_item` 整表、`recycle_data.restoreStatus` 与 `.remainingCount`、`task_info.lockOwner`。

Node 有而 go-next 无的 7 张表（`demo_goods`、`plugin_info`、`space_info`、`space_type`、`user_address`、`user_info`、`user_wx`）属 demo / plugin / space / user 四个尚未开发的模块，不在本次范围。

## 4. 目标与非目标

**目标**

1. 数据库列名与 Node 版对齐，恢复同库共用的可能
2. 消除因命名分叉产生的全部映射代码
3. 通过 API 约束保证未来新模块无法再引入 snake_case 列名

**非目标**

1. 不补齐未开发模块的 7 张表
2. 不改动 HTTP JSON / EPS 对外字段（本来就是 camelCase，不变）
3. 不改表名（`base_sys_user` 等表名两边一致，无需动）
4. 不改索引名（`idx_base_sys_user_department_id` 只是名字，不影响兼容；但索引定义引用的**列名参数**要改）
5. 不重构 `mutationTimestamp()` 手动写时间的问题（另案）

## 5. 方案：三阶段推进

字面量替换面达 738 处，一次性全改若测试失败将难以区分是列名改错还是代码删漏。故切分为四个可独立验证的步骤（阶段一、二与阶段三的 3a、3b），每步结束时系统均处于可运行、可测试的完整状态。

### 阶段一 · 列名对齐

改动内容：

1. `cool/entity/entity.go` 的 `BaseFields()`：`create_time` → `createTime`、`update_time` → `updateTime`、`tenant_id` → `tenantId`
2. 16 个 entity 定义文件中 51 处 `NewField` 的 ColumnName 参数改为与 JSONName 一致
3. `manifest/config/config.yaml:26-27` 时间字段配置：

   ```yaml
   createdAt: create_time   →  createTime
   updatedAt: update_time   →  updateTime
   ```

   遗漏此项会导致 GoFrame 自动时间维护失效。
4. **两处硬编码列名常量**（不在 entity 定义中，机械替换虽能覆盖，但语义特殊须专项验证）：

   | 位置 | 现值 | 漏改后果 |
   |---|---|---|
   | `cool/db/tenant/metadata.go:12` | `tenantColumn = "tenant_id"` | `FieldByColumn("tenant_id")` 查无此字段，`CompileMetadata` 抛「模型租户字段不规范」，所有启用多租户的模型初始化失败 |
   | `cool/module/seed/parser.go:146` | `mapped.ParentColumn = "parent_id"` | seed 菜单树父子关系断裂，菜单层级错乱 |

   前者涉及租户隔离，属安全边界，须有专项测试覆盖（见 §8）。
5. 全项目字面量替换，738 处（详见 §6 替换清单与安全约束）
6. 索引定义中引用的列名参数
7. 删库重建，跑全量测试

此阶段结束后，所有映射逻辑变成恒等映射，系统行为不变，但表结构已对齐 Node。

**下游无需改动**：`seed/mapper.go`、`db/tenant/metadata.go`、`db/recycle/{batch,manager,catalog,restore}.go`、`db/schema/sync.go`、`crud/metadata.go`、`crud/query.go` 共 70 处 `JSONName`/`ColumnName` 配对逻辑，在对齐后仍然正确，无需修改。

其中 `crud/query.go:420` 的排序机制值得单独说明：`buildOrderClause` 以 `SortFields` 构建「JSON 名 → 列名」映射，前端传入的排序字段必须命中该白名单才会拼入 `ORDER BY`。对齐后映射退化为恒等，但白名单机制本身不依赖两名是否相同，故排序功能与注入防护均不受影响。


需要强调的是，这些配对逻辑**不是等待清理的冗余代码**。以 `recycle/manager.go:393` 为例：

```go
fmt.Sprintf("%s AS %s", quoteIdentifier(field.ColumnName), quoteIdentifier(field.JSONName))
```

回收站快照以 JSON 存盘、key 必须是 JSON 名，故查询时用 `AS` 完成「从列 X 取值、以名 Y 输出」的映射。对齐后 `AS` 子句在**字面上**成为多余字符，但它编码的映射意图依旧成立，且**不依赖**「X 恰好等于 Y」这一偶然事实。

若在此阶段顺手删掉 `AS`，等于把「`ColumnName` 必然等于 `JSONName`」硬编码进代码，而在双字段结构下没有任何机制保证该假设——将来任一字段传入不同的两个名字，此处便会静默产出错误的 JSON key。因此这类改写必须等到 3b 把两字段合并、由类型系统保证相等之后再做（届时也必须做，因为 `field.JSONName` 已不存在）。


### 阶段二 · 删除失效的映射代码

五个 service 目前存在**两种不同写法**，不可一概而论：

| 写法 | service | 形态 | 白名单机制 |
|---|---|---|---|
| A · map 式 | `user`、`role`、`param` | `xxxUpdateData(data) map[string]interface{}`，内含 `columns` 映射表 | `xxxMutationFields` 变量 + `validateXxxMutation` |
| B · struct + fields 式 | `menu`、`department` | `xxxUpdateMutation(data) (xxxMutationRow, []string, error)`，同时构造 struct 与显式列名 slice，配合 `Fields(fields...)` 限定更新列 | Update 内联 `switch` 判断 |

删除：

- 写法 A 的 3 个 `columns` 映射表（`userUpdateData`、`roleUpdateData`、`paramUpdateData`）
- 写法 B 的 2 处显式列名 slice（`menuUpdateMutation`、`departmentUpdateMutation` 中的 `fields = append(fields, "parent_id")` 之类）
- 5 个 `xxxMutationRow` 结构体与对应的 `xxxRowFromData`

不动：`userRoleMutationRow`、`roleMenuMutationRow`、`roleDepartmentMutationRow` 三个关联表结构体——它们字段均为 `int64` 且列名本就无 camelCase 差异，与本次改动无关。

保留：

- **白名单校验**。写法 A 的 `xxxMutationFields` + `validateXxxMutation`，以及写法 B 的内联 `switch`，都属 API 契约层，负责对未知字段返回「未知字段: xxx」，同时挡住 `createTime` 等不应由前端写入的列。这是整套里唯一必要的字段声明。两种写法是否统一为一种，属独立议题，不在本次范围。
- **业务转换逻辑**：`password` 哈希与 `password_v` 自增、`role.menuIdList` / `departmentIdList` 的 JSON 序列化、`role.label` 的 `gdb.Raw("NULL")` 处理、`param.data` 按 `dataType` 的三种序列化、`dataType` 范围校验、`menu` / `department` 的 `name` 非空与 `parentId` 自引用校验。

改动后 service 直接使用前端传入的 map，由 GoFrame 完成列匹配与非表字段过滤：

```go
values := request.Data
if _, ok := values["menuIdList"]; ok {
   encoded, err := roleIDsJSON(menuIDs)
   if err != nil {
      return nil, err
   }
   values["menuIdList"] = encoded
}
// 主键用 FieldsEx("id") 排除，避免 UPDATE ... SET id=?
```

依据（均经 GoFrame v2.10.2 源码核实）：

1. `gdb_model.go:140` 中 `filter: true` 是 Model 构造默认值，故 `gdb_core_structure.go:474` 的 `mappingAndFilterData` 会自动过滤非表字段（如 `roleIdList`）
2. `FieldsEx` 的排除动作发生在 `gdb_model_utility.go:170` 的 `doMappingAndFilterForInsertOrUpdateDataMap` 内部，作用于 **Insert/Update 的数据 map**（而非查询字段），且在字段名映射**之后**执行，故参数应传列名


**Insert 路径的行为变化**：现用结构体时未传字段为 `nil`，会显式写入 `NULL`；改用 map 后该字段不出现在 SQL 中，走 DB 默认值。对 NOT NULL 且无默认值的列，行为从「报错」变为「取默认值」。**动手前须逐列核对表结构的 NOT NULL 与默认值定义，确认无列因此改变行为。**

#### 写法 B 的改造范例（以 `menuUpdateMutation` 为例）

该函数 60 行，其中仅约 10 行是真实业务逻辑。每个字段的处理都混了三件事：

```go
if value, ok := data["parentId"]; ok {   // ① 判断字段是否传入
    row.ParentID = value                  // ② 字段名第二遍
    fields = append(fields, "parent_id")  // ③ 字段名第三遍（列名）
}
```

一个字段名书写三遍，11 个字段共 33 遍，新增字段须同时改 struct、map key、列名 slice 三处；调用方还需续拼 `fields = append(fields, "update_time")`。

①②③ 均为 GoFrame 已提供的能力：map 中不存在的 key 天然不出现在 `SET` 子句（对应①）、camelCase 自动映射到列（对应③）、`row` 是多余的中间层（对应②）。`fields` slice 作为 `Fields(...)` 白名单，与 Update 开头的内联 `switch` 白名单功能重复。

改造后仅保留真实校验：

```go
func validateMenuMutation(data map[string]interface{}) error {
   if value, ok := data["name"]; ok {
      name, valid := value.(string)
      if !valid || strings.TrimSpace(name) == "" {
         return exception.Validate("name不能为空")
      }
      data["name"] = strings.TrimSpace(name)   // 见下方陷阱说明
   }
   for _, field := range []string{"type", "orderNum", "keepAlive", "isShow"} {
      if value, ok := data[field]; ok && value == nil {
         return exception.Validate(field + "不能为空")
      }
   }
   return nil
}
```

调用方：

```go
if err := validateMenuMutation(request.Data); err != nil {
   return nil, err
}
if v, ok := request.Data["parentId"]; ok && int64Value(v) == id {
   return nil, exception.Validate("上级菜单不能是自身")
}
values := request.Data
values["updateTime"] = mutationTimestamp()
// query.FieldsEx("id").Data(values).Update()
```

约 60 行降至 20 行，且后续新增字段无需改动此处，仅需维护 Update 开头的白名单 `switch`。

**陷阱：顺手做的数据清洗不能丢。** 原代码 `row.Name = name` 赋入的是 `TrimSpace` 之后的值，即校验的同时完成了清洗。若改写时只校验而不回写 map，入库值将变为带空格的原始值——属静默行为变化。

改造前须逐一确认的清洗回写点：

| 位置 | 现状 |
|---|---|
| `menu.go:113` | `name` trim 后赋给 `row.Name`，须回写 |
| `department.go:61` | `name` trim 后赋给 row，须回写 |
| `user.go:320` | `password` trim 后参与哈希，结果已回写 `values`，安全 |
| `user.go:569`、`role.go:633` | 数组元素 `values[index] = TrimSpace(item)`，就地清洗，安全 |

其余 `TrimSpace` 调用属「仅校验、不回写」（形如 `if strings.TrimSpace(x) == ""`），改造不受影响。

#### 顺带发现的现存缺陷（范围外，待决策）

`user.go:303-316` 的 Update 路径存在校验与入库取值不一致：

```go
username := strings.TrimSpace(fmt.Sprint(usernameValue))   // trim 后的值
count := ...Where("username", username).WhereNot("id", id).Count()   // 用 trim 后的值查重
...
values := userUpdateData(request.Data)   // 取 data["username"] 原始值，未 trim
```

即**查重用 trim 后的值，入库用未 trim 的原值**。前端传 `"  admin  "` 时，以 `"admin"` 查重（可能误判重复而被拒），若通过则入库为带空格的 `"  admin  "`，该用户后续以 `admin` 登录将无法匹配。

Add 路径（`user.go:259`）为 `row.Username = username`，使用 trim 后的值，不存在此问题——缺陷仅限 Update。

改造阶段二时必然经过这段代码，顺手回写 trim 值即可修复。但这属本次范围外的行为变更（会改变既有输入的处理结果），是否纳入需单独决策；若不纳入，应另立 issue 记录。



### 阶段三 · 收敛 entity API（防复发）

尚有 4 个模块待开发（demo / plugin / space / user）。若 `NewField` 仍保留 ColumnName 参数，新模块极可能再次写入 snake_case，缺陷复发。此阶段的价值在于让规范由 API 强制保证，而非依赖人记住。

拆为 3a、3b 两步执行，两步均在计划内，风险递增：

**3a** —— `NewField(jsonName, columnName, dataType)` 收敛为 `NewField(name, dataType)`，构造时同时赋给 `JSONName` 与 `ColumnName`。`Field` 结构保留两个字段名，下游 70 处引用一行不用改。双写在定义层消失，新模块无从写错。改动 98 个调用点，机械且可由编译器全量校验。

**3b** —— 合并 `Field.ColumnName` 与 `JSONName` 为单一 `Name`。

```go
type Field struct {
   JSONName   string     ┐
   ColumnName string     ┘  →  Name string
   ...
}
```

「双名」这一概念随之消失，所有为维护它而存在的校验、索引与拼接一并失去存在理由。**3b 的净效果是删代码**：

| 位置 | 3b 后 |
|---|---|
| `db/recycle/manager.go:393` | 删掉 `AS` 拼接，`SELECT` 列表直接用列名 |
| `db/recycle/restore.go:216,234` | 删掉两处双字段一致性校验 |
| `db/recycle/batch.go:20,286,323` | `IdentityField` 双字段收敛为一，删掉双名匹配 |
| `db/tenant/metadata.go:34-42` | 删掉 `jsonField` / `columnField` 双向查找与比较 |
| `rest/crud/metadata.go:128-129` | `FieldsByJSON` / `FieldsByColumn` 双索引合一 |

在 3b 之前，上述代码全部是**正确且必要**的写法，不得提前改动——详见 §5 阶段一末尾关于 `AS` 别名的说明。

3b 须独立成阶段、独立验收，原因是 70 处引用中有两块出错后果较重：

- **`recycle` 还原逻辑**：快照以 JSON 存盘，还原时需把 JSON key 映射回列名。双字段收敛若搞错映射方向，表现为「还原后字段错位」，且仅在执行还原时暴露，常规 CRUD 测试无法覆盖。
- **`tenant` 校验重写**：该校验是租户字段的守门人，重写后须确认越权路径仍被堵住。

关于「列名 ≠ JSON 名」这一能力的取舍：其唯一真实用途是对接列名不可控的外部表（如列名为 `DEPTNO` 的遗留库）。本项目表结构全部自建、列名完全可控，该场景不适用，故不作为保留双字段的理由。




## 6. 替换清单与安全约束

### 6.1 完整列名映射（42 项）

括号内为该字面量在全项目 `.go` 文件中的出现次数。

| 现列名 | 目标列名 | 处数 | | 现列名 | 目标列名 | 处数 |
|---|---|---|---|---|---|---|
| `tenant_id` | `tenantId` | 113 | | `menu_id` | `menuId` | 8 |
| `parent_id` | `parentId` | 48 | | `view_path` | `viewPath` | 8 |
| `create_time` | `createTime` | 47 | | `lock_owner` | `lockOwner` | 7 |
| `user_id` | `userId` | 47 | | `table_name` | `tableName` | 6 |
| `update_time` | `updateTime` | 45 | | `restore_order` | `restoreOrder` | 6 |
| `role_id` | `roleId` | 31 | | `menu_id_list` | `menuIdList` | 5 |
| `order_num` | `orderNum` | 30 | | `socket_id` | `socketId` | 5 |
| `department_id` | `departmentId` | 24 | | `parent_item_id` | `parentItemId` | 5 |
| `password_v` | `passwordV` | 19 | | `remaining_count` | `remainingCount` | 5 |
| `key_name` | `keyName` | 16 | | `restore_status` | `restoreStatus` | 5 |
| `nick_name` | `nickName` | 16 | | `branch_key` | `branchKey` | 4 |
| `type_id` | `typeId` | 15 | | `department_id_list` | `departmentIdList` | 4 |
| `recycle_id` | `recycleId` | 12 | | `entity_info` | `entityInfo` | 4 |
| `data_type` | `dataType` | 11 | | `last_execute_time` | `lastExecuteTime` | 4 |
| `is_show` | `isShow` | 11 | | `next_run_time` | `nextRunTime` | 4 |
| `task_id` | `taskId` | 11 | | `end_date` | `endDate` | 3 |
| `job_id` | `jobId` | 10 | | `lock_expire_time` | `lockExpireTime` | 3 |
| `head_img` | `headImg` | 9 | | `primary_key` | `primaryKey` | 3 |
| `keep_alive` | `keepAlive` | 9 | | `repeat_conf` | `repeatConf` | 3 |
| `c_key` | `cKey` | 8 | | `start_date` | `startDate` | 3 |
| `c_value` | `cValue` | 8 | | `task_type` | `taskType` | 3 |

### 6.2 替换安全约束

**必须用白名单驱动，禁止正则通配。** 项目中大量 snake_case 字面量是**表名**而非列名（`base_sys_user` 45 处、`base_sys_department` 15 处、`demo_goods` 14 处、`task_info` 12 处、`dict_info` 12 处、`task_log` 11 处、`base_sys_log` 11 处、`recycle_data` 10 处、`recycle_item` 9 处、`base_sys_menu` 9 处等），表名两边一致，改了反而破坏兼容。

因此替换须满足：

1. 替换集合严格限定为 §6.1 的 42 项，逐项精确匹配带引号的完整字面量（如 `"user_id"`），不做前缀 / 子串匹配
2. 注意 `table_name` 一项——它是 `recycle_item` 的**列名**，与「表名」概念无关，属替换范围
3. 替换后须复查表名类字面量未被误伤
4. `.go` 之外还需检查 `.yaml` 配置与 `docs/` 下的文档（合计 100 处）。`docs/protocol/fixtures/` 下 6 个 JSON 已确认无 snake_case（均为 camelCase 的 API 响应样本），无需改动
5. **公共模块 `cool/` 下仅 4 处命中，全部人工复核而非机械替换**：

   | 位置 | 内容 |
   |---|---|
   | `cool/entity/entity.go` | 3 处，即 `BaseFields()` 的三个公共字段 |
   | `cool/db/tenant/metadata.go:12` | `tenantColumn` 常量，见 §5 阶段一第 4 项 |
   | `cool/module/seed/parser.go:146` | `ParentColumn` 常量，同上 |
   | `cool/db/schema/sync.go` | 1 处 |

   其余 379 处生产代码命中集中在 `modules/` 下 31 个文件，以 `task/service/store.go`（41）、`recycle/event/data.go`（39）、`base/service/sys/user.go`（39）、`role.go`（33）、`menu.go`（32）为多。

### 6.3 待同步修正的文档

- `docs/superpowers/specs/2026-07-14-cool-admin-go-next-schema-sync-design.md` 第 94、151 行：删除或改写「DB 字段使用 snake_case」的记述，与第 127 行的契约优先原则保持一致
- `docs/protocol/source-map.md`、`base-api-comparison.md`：如含列名描述需同步

## 7. 风险

| 风险 | 影响 | 应对 |
|---|---|---|
| **GoFrame 模糊匹配会掩盖漏改** | 漏改的 `"parent_id"` 在列名为 `parentId` 时仍被 `MapPossibleItemByKey` 匹配成功，功能照常、测试全绿，但代码留下不一致 | **测试通过不能作为替换完整的证据**，必须以 grep 清零为准（§8 第 4 项） |
| `tenantColumn` 常量漏改 | **租户隔离失效**，多租户模型初始化报错；若与其他改动叠加，存在越权读写风险 | §5 阶段一第 4 项 + §8 专项验证 |
| `seed.ParentColumn` 常量漏改 | seed 菜单树父子断裂，层级错乱 | 同上 |
| 字面量替换误伤表名 | SQL 运行期报错 | 白名单精确匹配 + 替换后复查（§6.2） |
| `autoSync` 不会重命名列，只会新增列 | 旧库残留 snake_case 列，新列全空 | 删库重建（已确认数据库可删） |
| 时间字段配置漏改 | GoFrame 自动时间维护失效，`createTime` 不再自动写入 | 阶段一第 3 项，配合断言创建时间非空的测试 |
| 阶段二 Insert 改 map 后 NULL 语义变化 | NOT NULL 无默认值的列行为改变 | 动手前逐列核对表结构 |
| **大小写写错污染 API 输出** | SQL 照常执行不报错，但结果集 key 大小写错误 → JSON 字段名错误 → 前端读不到值，**破坏本次改动要修复的 API 契约** | 42 项的目标大小写严格以 entity 定义的 JSONName 为准，不凭手感书写；§8 增加 API 响应字段名比对 |
| 255 处测试内字面量 | 测试大面积失败 | 与生产代码同批替换，全量测试作为验收门槛 |

第一条是本次改动最反直觉的风险：`Data(map)`、`Fields()`、`FieldsEx()` 三者均通过 `gutil.MapPossibleItemByKey` 做「忽略大小写与符号」的模糊匹配（`gdb_model_utility.go:128`、`gdb_core_structure.go:487`）。这带来两个后果——好的一面是阶段一具备容错性，漏改不会导致系统崩溃；坏的一面是**漏改不会被任何测试发现**，只能靠静态扫描兜住。

### MySQL 列名大小写行为（已于 MySQL 8.0 实测）

| 探测 | 结果 |
|---|---|
| 以 `DEPARTMENTID` / `departmentid` / `DePaRtMeNtId` 插入、查询 `departmentId` 列 | 全部成功——列名**引用**不区分大小写 |
| 建立仅大小写不同的两列 `departmentId` + `departmentid` | `ERROR 1060 Duplicate column name`——印证标识符大小写不敏感 |
| `SELECT DePaRtMeNtId` 的结果集列标题 | 返回 `DePaRtMeNtId`，**按查询时书写的大小写，而非表定义的大小写** |

前两项说明列名**匹配**层面无风险，无需担心跨平台差异（表名的大小写敏感性另论，但本次不改表名）。

第三项则构成一个独立风险，即上表「大小写写错污染 API 输出」一条：由于结果集 key 取自 SQL 中书写的形式，`SELECT departmentid`（错误大小写）会让 GoFrame 扫出 key 为 `departmentid` 的记录，最终输出到前端的 JSON 字段名即为 `departmentid`。全过程无任何报错。

风险敞口取决于 SELECT 的构造方式：凡取值于 entity 定义 `field.ColumnName` 的（如 `recycle/manager.go` 的快照查询、`crud` 的字段列表）均安全，因大小写有单一来源；仅手写字符串字面量处存在敞口。故 §6.1 的 42 项替换，目标列名的大小写必须逐字对照 entity 定义中的 JSONName。



## 8. 验证

每个阶段独立验收，全部满足方可进入下一阶段：

1. `go build ./...` 通过
2. `go test ./...` 全量通过（150 个测试文件）
3. 删库后 `autoSync` 重建，用 `SHOW COLUMNS` 逐表比对，确认列名与 §6.1 目标列名及 Node 版一致
4. **静态扫描清零**：全局 grep 结果中不应再出现 §6.1 的任何一项（注意排除表名，见 §6.2）。**此项不可省略、不可用测试结果替代**——GoFrame 的模糊匹配会让漏改的字面量继续正常工作，测试全绿并不能证明替换完整（见 §7 第一条）
5. **运行时验收**，按 `.claude/skills/verify` 的方式启动与探测：

   ```bash
   GF_GCFG_FILE=/tmp/cool-go-verify.yaml go run .   # 绑定 :18001，避开占用
   ```

   | 探测 | 期望 |
   |---|---|
   | `GET /health` | `code:1000`，`data.status:"ok"` |
   | `POST /admin/base/sys/user/page` | `code:1000`，含 `data.list` 与 `data.pagination`，且行内**不含 `password`** |
   | `POST .../page` 传 `{"sort":"username desc; drop table"}` | 返回业务失败，不执行任意 SQL（验证 `SortFields` 白名单在对齐后仍生效） |
   | `GET .../page`（错误方法） | 按既有约定拒绝 |

   该配置含 `initDB: true` / `initMenu: true`，故启动过程本身即验证了 seed 链路。验证完毕须停掉后台进程。
6. **API 字段名大小写比对**（防 §7 「大小写写错污染 API 输出」）：取 `user/page`、`menu/list`、`param/page` 等响应，将其字段名与 `docs/protocol/base-api-contract.md` 及 `docs/protocol/fixtures/` 下的样本逐字比对，确认大小写完全一致。此项无法由 SQL 报错或单元测试代替——错误大小写在数据库侧完全合法。
7. **租户隔离专项**（安全边界，不可省）：
   - 启用 `TenantModeRequired` 的模型能正常 `CompileMetadata`，不报「模型租户字段不规范」
   - 跨租户读写隔离生效，A 租户查不到 B 租户数据
   - `sanitizeHookUpdateData` 仍能拦住前端篡改租户列的尝试（该逻辑运行在 GoFrame 标准化**之后**，此时 map key 已是列名，故对齐后 `tenantColumn` 与标准化后的 key 均为 `tenantId`，比较仍成立）
8. **时间字段专项**：新增记录后 `createTime` / `updateTime` 自动写入非空，验证 `config.yaml` 配置生效
9. **seed 专项**：seed 导入的菜单树父子层级正确，验证 `ParentColumn` 已改
10. 阶段二额外验证：CRUD 增删改查在真实请求下行为不变，未传字段不被写入 NULL
11. 阶段 3a 额外验证：`NewField` 调用点全部为两参数形式
12. **阶段 3b 额外验证**（两项均为专项，不可由常规 CRUD 测试替代）：
    - **回收站往返**：删除 → 还原 → 逐字段比对原始记录，确认无字段错位。需覆盖含父子关系的多表级联场景（`recycle_item` 的 `parentItemId` / `branchKey` 链路）
    - **租户守门**：`tenant/metadata.go` 校验重写后重跑租户隔离测试，确认 `TenantModeRequired` 模型缺字段仍报错、跨租户越权路径仍被堵住

