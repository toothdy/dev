# cool-admin-go-next core 框架层增强设计

## 命名对齐原则

本次 core 重写在类型与字段名上以**对齐 Node 版 `@cool-midway/core`** 为原则。具体对齐参照：

| 概念 | Node 版 | Go 新版 |
|----|----|----|
| CRUD 配置类型 | `CurdOption` | `CurdOption`（**保留 Node 拼写，含连字符 d**） |
| 查询配置类型 | `QueryOp` | `QueryOp` |
| 关联表配置类型 | `JoinOp` | `JoinOp` |
| ServiceApi 元素类型 | `ServiceApis` | `ServiceApis`（复数，与 Node 一致） |
| 等值字段对象 | `FieldEq` | `FieldEq` |

字段命名：保留 Go 的 PascalCase 首字母规则，但后缀与 Node 一致（例：Node `pageQueryOp` → Go `PageQueryOp`；Node `infoIgnoreProperty` → Go `InfoIgnoreProperty`）。

> 历史包袱清理：
> - 原 `CRUDOptions` / `QueryOptions` 与 Node 命名不一致，重命名为 `CurdOption` / `QueryOp`
> - 原 `CRUDDefinition` 字段与 `CRUDOptions` 完全重复，**本次合并清理到 `CurdOption`**
> - 原 `PageSelectOptions` 是旧 controller 路径下的简化联表配置，与 Node `JoinOp` 无对应；新 core 路径以 `JoinOp` 为主，旧字段保留在原路径不动

## 背景

Go 版 cool-admin 框架层 `cool/rest/crud/runtime.go` 已提供基本 CRUD 能力（Add/Delete/Update/Info/List/Page），但对比 Node 版 `@cool-midway/core`，缺少多个关键能力，导致 `modules/` 中的 Service 层必须手写大量样板代码。

**核心问题**：框架能力不足 → Service 覆盖默认实现 → 代码膨胀

| 缺失能力 | 后果 | 典型冗余量 |
|---------|------|----------|
| 级联删除声明式 | Service 手写 Delete 回收站+级联+归档 | ~500 行（6 个 Service） |
| 关联表查询 | Service 重写 List/Page | ~120 行（menu.go 的 List+Page） |
| 动态 Where 条件 | 无法灵活扩展查询 | 间接导致重写 List/Page |
| before hook | 无法在 Controller 层预处理 | 间接导致重写 Add/Update |
| InfoIgnoreProperty 链路 | 未打通 | 框架 bug → **当前已修复，本 spec 仅记录** |

> 注：`serviceApis`（反射注册 Service 方法为 API）因当前模块迭代优先级，本次**不实现**。CurdOption 中**不**包含 `serviceApis` 字段。

**实施位置**：本次框架层重写在 `cool/core/` 新目录进行，**不修改** `cool/controller/`、`cool/rest/crud/`、`cool/entity/` 等历史路径下的代码，避免对存量模块造成回归。

## 设计

### 1. 级联删除声明式

**现状**：框架默认 Delete 只删当前表。Service 为实现级联删除必须实现 `DeleteHandler` 接口，手写 `deleteLegacy`/`deleteManaged`/`archiveXxx` 三件套。

**设计**：在 Entity 定义上声明级联关系，框架默认 Delete 自动处理。

```go
// core 层新增
type Cascade struct {
    Entity     Definition // 被级联删除的实体
    ForeignKey string     // 外键字段名（JSON 名）
}

// 在 Entity 定义中声明
func DictType() Definition {
    return New("dict", "DictType", "dict_type").
        Fields(fields).
        CascadeDelete(Cascade{Entity: DictInfo(), ForeignKey: "typeId"})
}
```

> **命名变更**：`entity.NewDefinition(module, name, tableName)` 因与返回类型 `entity.Definition` 命名高度冗余，本次 core 重写统一改为 `entity.New(module, name, tableName) Definition`。

**Runtime 行为**：默认 Delete 在锁定主表行后，递归处理级联：
1. 查询关联表中外键匹配的行
2. 递归处理子 Cascade（支持自引用如 `parentId`）
3. 归档到回收站
4. 删除关联行
5. 删除主表行

**约束**：级联删除仅在 Service 未实现 `DeleteHandler` 时生效。Service 实现 DeleteHandler 后完全接管。

### 2. 关联表查询（pageQueryOp.join）

**现状**：默认 List/Page 只查询单表。需要 LEFT JOIN 时 Service 必须重写整个方法。

**设计**：在 Controller 声明 JOIN 配置，框架自动构建带 JOIN 的查询。字段命名严格对齐 Node `QueryOp.join`。

```go
// Controller 声明
controller.Admin("base/sys/menu").
    Curd(controller.CurdOption{
        PageQueryOp: controller.QueryOp{
            Join: []controller.JoinOp{
                {Entity: BaseSysMenuDefinition(), Alias: "p", Condition: "a.parentId = p.id", Type: "leftJoin"},
            },
            Select: []string{"a.*", "p.name AS parentName"},
            AddOrderBy: map[string]string{"a.orderNum": "asc"},
        },
    })
```

**Runtime 行为**：
- `buildListQuery` / `buildPageQuery` 检测 `Join`，构建 `FROM table a LEFT JOIN other b ON condition`
- 如果 `Select` 不为空，使用自定义字段列表替代默认的 `selectColumns`
- `AddOrderBy` 追加到 ORDER BY 子句

> **字段映射到 Node**：`Join`（单数而不是 `Joins`）、`AddOrderBy`、`Select`、`Where` 都与 Node `QueryOp` 接口对齐。

### 3. 动态 Where 条件（pageQueryOp.where）

**现状**：`KeyWordLikeFields` / `FieldEq` / `FieldLike` 只能在编译期静态配置，无法根据运行时条件动态生成 SQL。

**设计**：Controller 声明 `Where` 回调函数，在查询时执行。对齐 Node `QueryOp.where`。

```go
controller.Admin("base/sys/menu").
    Curd(controller.CurdOption{
        PageQueryOp: controller.QueryOp{
            Where: func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error) {
                return "a.status = ?", []interface{}{1}, nil
            },
        },
    })
```

### 4. before hook — 请求预处理

**现状**：`ModifyBeforeHook` 在 Service 层，需要实现接口。缺少 Controller 层直接声明的方式。

**设计**：对齐 Node `CurdOption.before?: Function`，签名仅接受 ctx。

```go
// core 层类型
type BeforeFunc func(ctx context.Context) error

// Controller 声明
controller.Admin("base/sys/menu").
    Curd(controller.CurdOption{
        Before: func(ctx context.Context) error {
            // 用户自行从 ctx 提取请求数据
            data := core.MustGetBody(ctx)
            data["userId"] = security.UserFromContext(ctx).UserId
            return nil
        },
    })
```

> **与原 spec 的差异**：原 spec 章节 5 的 `Before(ctx, action, data) error` 签名（含 action / data 参数）偏离 Node 版 `(ctx) => {}`，本次重写统一为 Node 风格。需要在 ctx 提取数据的用户调用 `core.MustGetBody(ctx)` 框架辅助函数。

### 5. InfoIgnoreProperty 传递链路 — 已实现，本次仅记录

经过核查 `cool/rest/crud/metadata.go:140` 与 `cool/rest/crud/query.go:512`，原字段名 `InfoIgnoreFields`（Go 版拼写）已构成完整链路。本次 core 重写统一为 Node 拼写 `InfoIgnoreProperty`，作为对外暴露字段名；但历史路径 `cool/controller/` 下的 `InfoIgnoreFields` 不动。

## 文件清单（位于 `cool/core/`）

| 文件 | 改动 |
|------|------|
| `cool/core/entity/entity.go` | 新增 `Cascade` 结构体、`Definition.Cascades` 字段、`CascadeDelete()` 方法；**`NewDefinition` 重命名为 `New`**；类型 `Definition` 保留 |
| `cool/core/controller/definition.go` | 新增 `JoinOp` / `ServiceApis` / `FieldEq` 结构体；`CRUDOptions` / `CRUDDefinition` 合并为 `CurdOption`；`QueryOptions` 重命名为 `QueryOp`；CurdOption 新增 `Before`；QueryOp 新增 `Join`、`Select`、`AddOrderBy`、`Where`，新增 `KeyWordLikeFields`、`FieldEq`、`FieldLike` 字段 |
| `cool/core/controller/builder.go` | `Curd()` 处理新字段；`cloneQueryOp` 处理新字段 |
| `cool/core/controller/derive.go` | `CRUDResourceSpecs` 传递所有新字段 |
| `cool/core/rest/crud/types.go` | `ResourceSpec` 新增 `CascadeDefs`、`Before`；`QuerySpec` 新增 `Join`、`Select`、`AddOrderBy`、`Where` |
| `cool/core/rest/crud/metadata.go` | `Resource` 新增对应字段；`buildResource` 填充；`QueryMetadata` 新增字段 |
| `cool/core/rest/crud/query.go` | `buildListQuery` / `buildPageQuery` 支持 JOIN；`buildWhereClause` 支持 `Where` 回调；`buildOrderClause` 支持 `AddOrderBy` |
| `cool/core/rest/crud/runtime.go` | 默认 Delete 增加 `cascadeDelete`；默认 Add / Update 调用 `Before` |

## 向后兼容

- 所有新增字段为可选，不修改现有 API
- **路径隔离**：core 层代码全部位于 `cool/core/` 下，旧路径 `cool/controller/`、`cool/rest/crud/`、`cool/entity/` 维持现状不受影响；Controller 通过显式选择 core 入口后才进入新路径
- 级联删除仅在 Service 未实现 `DeleteHandler` 时生效
- JOIN 查询仅在 `Join` 不为空时启用
- Before hook 仅在 `Before` 不为 nil 时调用
- **命名迁移**：`entity.NewDefinition` → `entity.New`，旧名以 `// Deprecated:` 注释形式保留以供过渡期调用
- **类型重命名（core 内）**：`CRUDOptions` → `CurdOption`、`QueryOptions` → `QueryOp`、`CRUDDefinition` 并入 `CurdOption`。core 路径不暴露旧类型名
