# cool-admin-go-next 框架增强实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 增强 Go 版 cool-admin 框架层，使其具备 Node 版 `@cool-midway/core` 的核心能力，消除 modules 中大量冗余代码。

**架构:** 在 `cool/entity` 新增级联删除声明，在 `cool/rest/crud` 扩展 Runtime 支持级联删除、JOIN 查询、动态 Where 条件、Before hook、serviceApis，在 `cool/controller` 同步新增 Builder 接口和路由注册。

**Tech Stack:** Go, GoFrame v2, 无新依赖

**全局约束:**
- 所有改动在 `cool/` 框架层，不涉及 `modules/` 业务层（除 Entity 声明级联外）
- 向后兼容：现有接口不改变
- 任何新增字段都用 `CascadeDefs` 等新字段名，不影响现有 JSON 序列化

---

## 文件结构

### 需修改的文件

| 文件 | 改动 |
|------|------|
| `cool/entity/entity.go` | 新增 `Cascade` 结构体、`Definition.Cascades` 字段、`CascadeDelete()` 方法 |
| `cool/controller/definition.go` | 新增 `ServiceAPI`、`JoinOp` 结构体；`CRUDOptions`/`CRUDDefinition` 新增 `ServiceApis`、`Before`；`QueryOptions` 新增 `Joins`、`Select`、`AddOrderBy`、`Where` |
| `cool/controller/builder.go` | 新增 `ServiceAPI()` 方法；`CRUD()` 方法传递新字段 |
| `cool/controller/derive.go` | `CRUDResourceSpecs` 传递 `ServiceApis`、`Before`、`QueryOptions` 新增字段 |
| `cool/controller/register.go` | `CompileRoutePlan` 中为 `ServiceApi` 生成路由 |
| `cool/rest/crud/types.go` | `ResourceSpec` 新增 `ServiceApis`、`Before`、`CascadeDefs`；`QuerySpec` 新增 `Joins`、`Select`、`AddOrderBy`、`Where` |
| `cool/rest/crud/metadata.go` | `Resource` 新增对应字段；`buildResource` 填充；编译 JOIN 元数据 |
| `cool/rest/crud/runtime.go` | 默认 Delete 增加 `cascadeDelete`；默认 Add/Update 调用 `Before`；新增 `ServiceAPI` 方法 |
| `cool/rest/crud/query.go` | `buildListQuery`/`buildPageQuery` 支持 JOIN；`buildWhereClause` 支持 `Where` 回调 |

---

### Task 1: Entity 层级联删除声明

**Files:**
- Modify: `cool/entity/entity.go`

**Interfaces:**
- Produces: `entity.Cascade` 结构体, `entity.Definition.Cascades` 字段, `entity.Definition.CascadeDelete()` 方法

- [ ] **Step 1: 新增 Cascade 结构体和 Definition 字段**

```go
// cool/entity/entity.go

// Cascade 表示级联删除关系
type Cascade struct {
    Entity     Definition // 被级联删除的目标实体
    ForeignKey string     // 目标实体的外键字段名（JSON 名）
}

// Definition 新增字段
type Definition struct {
    // ... 现有字段
    Cascades []Cascade
}

// CascadeDelete 设置级联删除关系
func (d Definition) CascadeDelete(cascades ...Cascade) Definition {
    d.Cascades = append(d.Cascades, cascades...)
    return d
}
```

- [ ] **Step 2: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/entity/
```

### Task 2: InfoIgnoreFields 传递链路打通

**Files:**
- Modify: `cool/controller/derive.go`（确认 `InfoIgnoreFields` 传入 `ResourceSpec`）
- Modify: `cool/rest/crud/metadata.go`（确认 `buildResource` 中已正确处理）

**Interfaces:**
- Consumes: `CRUDDefinition.InfoIgnoreFields`
- Produces: `Resource.InfoIgnoreFields` 正确设置

- [ ] **Step 1: 检查 derive.go 传递链路**

```go
// 在 CRUDResourceSpecs 中确认已有：
InfoIgnoreFields: cloneStrings(definition.CRUD.InfoIgnoreFields),
```

- [ ] **Step 2: 检查 metadata.go 处理链路**

```go
// 在 buildResource 中确认已有：
for _, fieldName := range spec.InfoIgnoreFields {
    resource.InfoIgnoreFields[fieldName] = true
}
```

- [ ] **Step 3: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### Task 3: before hook 请求预处理

**Files:**
- Modify: `cool/controller/definition.go`
- Modify: `cool/controller/builder.go`
- Modify: `cool/controller/derive.go`
- Modify: `cool/rest/crud/types.go`
- Modify: `cool/rest/crud/metadata.go`
- Modify: `cool/rest/crud/runtime.go`

**Interfaces:**
- Consumes: `CRUDDefinition.Before`
- Produces: 默认 Add/Delete/Update 在操作前调用 `Before` 回调

- [ ] **Step 1: definition.go — 新增 Before 字段**

```go
// CRUDOptions 新增
type CRUDOptions struct {
    // ... 现有字段
    Before func(ctx context.Context, action string, data map[string]interface{}) error
}

// CRUDDefinition 新增
type CRUDDefinition struct {
    // ... 现有字段
    Before func(ctx context.Context, action string, data map[string]interface{}) error
}
```

- [ ] **Step 2: builder.go — CRUD() 传递 Before**

```go
func (b *Builder) CRUD(options CRUDOptions) *Builder {
    b.definition.CRUD = &CRUDDefinition{
        // ... 现有字段
        Before: options.Before,
    }
    return b
}
```

- [ ] **Step 3: derive.go — 传递 Before 到 ResourceSpec**

```go
// CRUDResourceSpecs 中增加：
Before: definition.CRUD.Before,
```

- [ ] **Step 4: types.go — ResourceSpec 新增 Before 字段**

```go
type ResourceSpec struct {
    // ... 现有字段
    Before func(ctx context.Context, action string, data map[string]interface{}) error
}
```

- [ ] **Step 5: metadata.go — Resource 新增 Before 字段，buildResource 填充**

```go
type Resource struct {
    // ... 现有字段
    Before func(ctx context.Context, action string, data map[string]interface{}) error
}

// buildResource 中：
resource.Before = spec.Before
```

- [ ] **Step 6: runtime.go — 默认 Add/Delete/Update 调用 Before**

```go
// 在 addDefault 中，runModifyBefore 之前：
if resource.Before != nil {
    if err := resource.Before(ctx, "add", input); err != nil {
        return nil, err
    }
}

// 在 updateDefault 中类似
// 在 DeleteWithData 的默认 deleteWork 中类似
```

- [ ] **Step 7: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### Task 4: pageQueryOp.where 动态查询条件

**Files:**
- Modify: `cool/controller/definition.go`
- Modify: `cool/controller/builder.go`
- Modify: `cool/controller/derive.go`
- Modify: `cool/rest/crud/types.go`
- Modify: `cool/rest/crud/metadata.go`
- Modify: `cool/rest/crud/query.go`

**Interfaces:**
- Consumes: `QueryOptions.Where`
- Produces: `buildWhereClause` 在构建 WHERE 时调用 `Where` 回调

- [ ] **Step 1: definition.go — QueryOptions 新增 Where 字段**

```go
type QueryOptions struct {
    // ... 现有字段
    Where func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error)
}
```

- [ ] **Step 2: builder.go — cloneQueryOptions 处理 Where**

```go
func cloneQueryOptions(options QueryOptions) QueryOptions {
    return QueryOptions{
        // ... 现有字段
        Where: options.Where,
    }
}
```

- [ ] **Step 3: derive.go — 传递 Where**

```go
// CRUDResourceSpecs 中：
ListQuery: crud.QuerySpec{
    // ... 现有字段
    Where: definition.CRUD.ListQuery.Where,
},
PageQuery: crud.QuerySpec{
    // ... 现有字段
    Where: definition.CRUD.PageQuery.Where,
},
```

- [ ] **Step 4: types.go — QuerySpec 新增 Where 字段**

```go
type QuerySpec struct {
    // ... 现有字段
    Where func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error)
}
```

- [ ] **Step 5: metadata.go — QueryMetadata 新增 Where 字段，fillQueryMetadata 处理**

```go
type QueryMetadata struct {
    // ... 现有字段
    Where func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error)
}

// fillQueryMetadata 中：
target.Where = query.Where
```

- [ ] **Step 6: query.go — buildWhereClause 调用 Where 回调**

```go
func buildWhereClause(resource Resource, request QueryRequest, tenantCondition tenant.Condition) (string, []interface{}, error) {
    // ... 现有逻辑

    // 在最后追加 Where 回调生成的 SQL
    if resource.Where != nil {
        sql, args, err := resource.Where(ctx, request.Raw)
        if err != nil {
            return "", nil, err
        }
        if sql != "" {
            parts = append(parts, sql)
            args = append(args, args...)
        }
    }
}
```

注意：`buildWhereClause` 当前没有 `ctx` 参数，需要修改签名。由于 `buildListQuery`/`buildPageQuery` 中有 `ctx`，需要从调用方传递。

- [ ] **Step 7: 修改 buildWhereClause 签名，增加 ctx 参数**

```go
func buildWhereClause(ctx context.Context, resource Resource, request QueryRequest, tenantCondition tenant.Condition) (string, []interface{}, error) {
```

- [ ] **Step 8: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### Task 5: pageQueryOp.join 关联表查询

**Files:**
- Modify: `cool/controller/definition.go`
- Modify: `cool/controller/builder.go`
- Modify: `cool/controller/derive.go`
- Modify: `cool/rest/crud/types.go`
- Modify: `cool/rest/crud/metadata.go`
- Modify: `cool/rest/crud/query.go`

**Interfaces:**
- Consumes: `QueryOptions.Joins`, `QueryOptions.Select`, `QueryOptions.AddOrderBy`
- Produces: `buildListQuery`/`buildPageQuery` 支持 JOIN、自定义字段选择、额外排序

- [ ] **Step 1: definition.go — 新增 JoinOp 结构体，QueryOptions 新增字段**

```go
// JoinOp 关联表查询配置
type JoinOp struct {
    Entity    entity.Definition // 关联实体
    Alias     string            // 表别名
    Condition string            // JOIN 条件
    Type      string            // "leftJoin" 或 "innerJoin"
}

type QueryOptions struct {
    // ... 现有字段
    Joins      []JoinOp
    Select     []string // 指定查询字段（多表时必需）
    AddOrderBy map[string]string // 额外排序，如 {"price": "desc"}
}
```

- [ ] **Step 2: builder.go — cloneQueryOptions 处理新字段**

```go
func cloneQueryOptions(options QueryOptions) QueryOptions {
    return QueryOptions{
        // ... 现有字段
        Joins:      append([]JoinOp{}, options.Joins...),
        Select:     cloneStrings(options.Select),
        AddOrderBy: cloneStringMap(options.AddOrderBy),
    }
}

func cloneStringMap(m map[string]string) map[string]string {
    if m == nil {
        return nil
    }
    cloned := make(map[string]string, len(m))
    for k, v := range m {
        cloned[k] = v
    }
    return cloned
}
```

- [ ] **Step 3: derive.go — 传递 Joins/Select/AddOrderBy**

```go
// CRUDResourceSpecs 中：
ListQuery: crud.QuerySpec{
    // ... 现有字段
    Joins:      convertJoins(definition.CRUD.ListQuery.Joins),
    Select:     cloneStrings(definition.CRUD.ListQuery.Select),
    AddOrderBy: cloneStringMap(definition.CRUD.ListQuery.AddOrderBy),
},
PageQuery: crud.QuerySpec{
    // ... 现有字段
    Joins:      convertJoins(definition.CRUD.PageQuery.Joins),
    Select:     cloneStrings(definition.CRUD.PageQuery.Select),
    AddOrderBy: cloneStringMap(definition.CRUD.PageQuery.AddOrderBy),
},
```

- [ ] **Step 4: types.go — QuerySpec 新增 Joins/Select/AddOrderBy**

```go
type JoinSpec struct {
    TableName string
    Alias     string
    Condition string
    Type      string // "LEFT JOIN" 或 "INNER JOIN"
}

type QuerySpec struct {
    // ... 现有字段
    Joins      []JoinSpec
    Select     []string
    AddOrderBy map[string]string
    Where      func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error)
}
```

- [ ] **Step 5: metadata.go — QueryMetadata 新增字段，填充逻辑**

```go
type QueryMetadata struct {
    // ... 现有字段
    Joins      []JoinSpec
    Select     []string
    AddOrderBy map[string]string
    Where      func(ctx context.Context, request map[string]interface{}) (string, []interface{}, error)
}
```

- [ ] **Step 6: query.go — selectColumns 支持自定义 Select**

```go
func selectColumns(resource Resource, infoOnly ...bool) string {
    // 如果指定了 Select，使用自定义字段
    if len(resource.Select) > 0 {
        return strings.Join(resource.Select, ", ")
    }
    // ... 原有逻辑
}
```

- [ ] **Step 7: query.go — buildListQuery/buildPageQuery 支持 JOIN**

```go
// 构建 FROM 子句时加入 JOIN
func buildFromClause(resource Resource, metadata QueryMetadata) string {
    from := quoteIdentifier(resource.Spec.Model.TableName) + " a"
    for _, join := range metadata.Joins {
        from += " " + join.Type + " " + quoteIdentifier(join.TableName) + " " + join.Alias + " ON " + join.Condition
    }
    return from
}
```

- [ ] **Step 8: query.go — buildOrderClause 支持 AddOrderBy**

```go
// 在 buildOrderClause 中，追加 AddOrderBy 的排序项
func buildOrderClause(resource Resource, request QueryRequest) (string, error) {
    // ... 现有逻辑
    // 追加 AddOrderBy
    for field, order := range resource.AddOrderBy {
        if _, exists := columns[field]; !exists {
            parts = append(parts, fmt.Sprintf("%s %s", field, order))
        }
    }
}
```

- [ ] **Step 9: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### Task 6: serviceApis 直接注册 Service 方法为 API

**Files:**
- Modify: `cool/controller/definition.go`
- Modify: `cool/controller/builder.go`
- Modify: `cool/controller/derive.go`
- Modify: `cool/controller/register.go`
- Modify: `cool/rest/crud/types.go`
- Modify: `cool/rest/crud/metadata.go`
- Modify: `cool/rest/crud/runtime.go`

**Interfaces:**
- Consumes: `CRUDDefinition.ServiceApis`
- Produces: 为 Service 方法生成 POST 路由，Controller 声明即可

- [ ] **Step 1: definition.go — 新增 ServiceAPI 结构体，CRUDDefinition 新增字段**

```go
// ServiceAPI 将 Service 方法注册为 API
type ServiceAPI struct {
    Method     string // Service 方法名
    Summary    string // 接口描述
    Permission string // 可选权限标识
}

type CRUDDefinition struct {
    // ... 现有字段
    ServiceApis []ServiceAPI
}
```

- [ ] **Step 2: builder.go — 新增 ServiceAPI() 方法**

```go
func (b *Builder) ServiceAPI(serviceApis ...ServiceAPI) *Builder {
    b.definition.CRUD.ServiceApis = append(b.definition.CRUD.ServiceApis, serviceApis...)
    return b
}
```

- [ ] **Step 3: derive.go — 传递 ServiceApis**

```go
// CRUDResourceSpecs 中：
ServiceApis: append([]ServiceAPI{}, definition.CRUD.ServiceApis...),
```

- [ ] **Step 4: types.go — ResourceSpec 新增 ServiceApis**

```go
type ServiceAPI struct {
    Method     string
    Summary    string
    Permission string
}

type ResourceSpec struct {
    // ... 现有字段
    ServiceApis []ServiceAPI
}
```

- [ ] **Step 5: metadata.go — Resource 新增 ServiceApis**

```go
type Resource struct {
    // ... 现有字段
    ServiceApis []ServiceAPI
}

// buildResource 中：
resource.ServiceApis = append([]ServiceAPI{}, spec.ServiceApis...)
```

- [ ] **Step 6: runtime.go — 新增 ServiceAPI 方法**

```go
// ServiceAPI 通过反射调用 Service 方法
func (r *Runtime) ServiceAPI(ctx context.Context, resource Resource, method string, input map[string]interface{}) (interface{}, error) {
    svc := resourceService(resource)
    if svc == nil {
        return nil, exception.Internal(nil, "服务不可用")
    }
    svcValue := reflect.ValueOf(svc)
    methodValue := svcValue.MethodByName(method)
    if !methodValue.IsValid() {
        return nil, exception.Validate(fmt.Sprintf("方法不存在: %s", method))
    }
    // 调用方法，传入 input 作为参数
    args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(input)}
    outputs := methodValue.Call(args)
    // 处理返回值
    if len(outputs) == 0 {
        return nil, nil
    }
    if len(outputs) == 1 {
        if err, ok := outputs[0].Interface().(error); ok {
            return nil, err
        }
        return outputs[0].Interface(), nil
    }
    // 两个返回值: (data, error)
    if err, ok := outputs[1].Interface().(error); ok && err != nil {
        return nil, err
    }
    return outputs[0].Interface(), nil
}
```

- [ ] **Step 7: register.go — CompileRoutePlan 为 ServiceApi 生成路由**

```go
// 在 CompileRoutePlan 中，CRUD 路由注册后：
for _, serviceAPI := range resource.ServiceApis {
    fullPath := resource.Spec.Prefix + "/" + serviceAPI.Method
    method := http.MethodPost
    key, canonicalMethod, canonicalPath, err := canonicalRoute(method, fullPath)
    if err != nil {
        return nil, exception.Core(fmt.Sprintf("ServiceAPI %s: %v", serviceAPI.Method, err))
    }
    if previous, exists := seen[key]; exists {
        return nil, exception.Core(fmt.Sprintf("路由冲突 %s: %s 与 ServiceAPI %s", key, previous, serviceAPI.Method))
    }
    seen[key] = location
    plan.routes = append(plan.routes, compiledRoute{
        method:  canonicalMethod,
        path:    canonicalPath,
        key:     key,
        module:  definition.Module,
        handler: serviceAPIRouteHandler(runtime, resource, serviceAPI),
    })
}
```

- [ ] **Step 8: register.go 或 controller 包新增 serviceAPIRouteHandler**

```go
func serviceAPIRouteHandler(runtime *crud.Runtime, resource crud.Resource, serviceAPI crud.ServiceAPI) ghttp.HandlerFunc {
    return func(r *ghttp.Request) {
        input := map[string]interface{}{}
        if err := bindJSON(r, &input, true); err != nil {
            r.SetError(exception.Validate(err.Error()))
            return
        }
        data, err := runtime.ServiceAPI(r.Context(), resource, serviceAPI.Method, input)
        if err != nil {
            r.SetError(err)
            return
        }
        writeJSONSuccess(r, data)
    }
}
```

- [ ] **Step 9: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### Task 7: 级联删除框架层实现

**Files:**
- Modify: `cool/rest/crud/types.go`
- Modify: `cool/rest/crud/metadata.go`
- Modify: `cool/rest/crud/runtime.go`

**Interfaces:**
- Consumes: `entity.Cascade`（Task 1 产出）
- Produces: 默认 Delete 递归处理级联删除

- [ ] **Step 1: types.go — ResourceSpec 新增 CascadeDefs**

```go
type ResourceSpec struct {
    // ... 现有字段
    CascadeDefs []entity.Cascade
}
```

- [ ] **Step 2: metadata.go — Resource 新增 CascadeDefs，buildResource 填充**

```go
type Resource struct {
    // ... 现有字段
    CascadeDefs []entity.Cascade
}

// buildResource 中：
resource.CascadeDefs = append([]entity.Cascade{}, spec.Model.Cascades...)
```

- [ ] **Step 3: runtime.go — 新增 cascadeDelete 方法**

```go
// cascadeDelete 递归处理级联删除
func (r *Runtime) cascadeDelete(ctx context.Context, tx gdb.TX, scope *recycle.DeleteScope, resource Resource, ids []interface{}) error {
    for _, cascade := range resource.CascadeDefs {
        // 获取级联目标实体的表名
        targetTable := cascade.Entity.TableName
        fkColumn := cascade.ForeignKey
        
        // 查询关联行
        query := fmt.Sprintf("SELECT * FROM %s WHERE %s IN (?)", quoteIdentifier(targetTable), quoteIdentifier(fkColumn))
        rows, err := tx.Ctx(ctx).GetAll(query, ids)
        if err != nil {
            return gerror.Wrapf(err, "级联查询 %s 失败", targetTable)
        }
        if len(rows) == 0 {
            continue
        }
        
        // 收集关联行的 ID
        childIDs := make([]interface{}, 0, len(rows))
        primaryField, _ := cascade.Entity.PrimaryField()
        for _, row := range rows {
            childIDs = append(childIDs, row[primaryField.ColumnName].Val())
        }
        
        // 递归处理子级联（自引用支持）
        childResource := Resource{Spec: ResourceSpec{Model: cascade.Entity}}
        // 需要从 registry 获取完整的 Resource 定义
        // 如果 cascade.Entity 有 Cascades，递归处理
        if len(cascade.Entity.Cascades) > 0 {
            if childRes, ok := r.registry.resources[cascade.Entity.ResourceKey()]; ok {
                if err := r.cascadeDelete(ctx, tx, scope, childRes, childIDs); err != nil {
                    return err
                }
            } else {
                // 未注册到 registry 的实体，直接处理
                if err := r.cascadeDelete(ctx, tx, scope, childResource, childIDs); err != nil {
                    return err
                }
            }
        }
        
        // 归档到回收站
        if scope != nil && scope.IsArchiving() {
            if _, err := scope.AddResult(cascade.Entity, rows, recycle.ItemOptions{
                BranchKey: "...", ParentKey: "...", RestoreOrder: 10,
            }); err != nil {
                return err
            }
        }
        
        // 删除关联行
        deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE %s IN (?)", quoteIdentifier(targetTable), quoteIdentifier(fkColumn))
        result, err := tx.Ctx(ctx).Exec(deleteSQL, childIDs)
        if err != nil {
            return gerror.Wrapf(err, "级联删除 %s 失败", targetTable)
        }
        if scope != nil && scope.IsArchiving() {
            affected, _ := result.RowsAffected()
            if err := scope.MarkDeleted(affected); err != nil {
                return err
            }
        }
    }
    return nil
}
```

- [ ] **Step 4: runtime.go — DeleteWithData 默认分支调用 cascadeDelete**

```go
// 在 deleteWork 中，requireVisibleMutationRows 之后，runModifyBefore 之前：
if len(resource.CascadeDefs) > 0 {
    if err := r.cascadeDelete(ctx, tx, deleteScope, resource, ids); err != nil {
        return err
    }
}
```

- [ ] **Step 5: 验证编译通过**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./cool/...
```

### 验证

- [ ] **Step 1: 全量编译**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go build ./...
```

- [ ] **Step 2: 运行框架层测试**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go test ./cool/... -count=1
```

- [ ] **Step 3: 运行模块层测试**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next && go test ./modules/... -count=1
```