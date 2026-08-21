# Node EPS 契约对齐设计

> 日期：2026-08-21
> 状态：设计已批准，待实现
> 范围：EPS 直接契约；权限自动推导仅记录为独立后续改动

## 1. 结论

Go next 的 EPS 只服务当前 `cool-admin-vue` 实际消费的 Node 契约，不再保留 OpenAPI、通用文档模型或未来消费方扩展点。

实现采用直接投影：

```text
已校验 Graph + 运行时 Controller Definition + Entity Descriptor
                           ↓
              Admin/App Node EPS 契约
                           ↓
                    启动时发布一次
                           ↓
                  /admin、/app EPS 接口
```

不再经过 `Document -> LegacyView` 二次转换。业务 Controller DSL 保持不变，不增加 EPS 注解、配置或重复字段。

## 2. 背景与根因

Node `CoolEps` 只有一个职责：聚合 Midway 路由、Controller 配置、TypeORM Entity 和分页查询配置，生成前端代码生成器需要的数据。Swagger 是独立模块读取 EPS 后再转换，不属于 EPS 核心。

Go v1 同样直接从 Controller Definition 生成最终契约。Go next 曾同时编译内部 EPS 文档和 OpenAPI，因此建立了 `Document`、`Module`、`Entity`、`Field`、`API` 等中间模型。OpenAPI 已删除，但中间模型和 `LegacyView` 仍然保留，造成以下问题：

1. 同一份数据先扩充为内部文档，再缩减回 Node 契约；
2. 大量字段没有消费方；
3. Graph 已校验后，EPS 又执行部分重复校验；
4. `pageQueryOp` 和 `pageColumns` 在最终响应中被固定为空；
5. 代码量明显高于 Node 和 Go v1，实际前端能力反而不完整。

根因不是 GoFrame，也不是文件数量，而是已经失去第二消费方的多阶段编译架构仍然存在。

## 3. 目标

本次实现必须：

1. EPS HTTP 响应对齐 Node `CoolEps` 契约；
2. 直接生成 Admin/App 两份按模块分组的数据；
3. 恢复静态 `PageQueryOp` 的 `pageQueryOp` 与 `pageColumns`；
4. 保留 Admin/App、开发路由和 `ignoreToken` 过滤语义；
5. 保持现有 Controller 写法不变；
6. 删除没有消费方的内部模型、字段、常量和转换；
7. 不增加第三方依赖；
8. 启动时生成一次，请求时只读取已发布快照。

## 4. 非目标

本次不实现：

- OpenAPI、Swagger JSON 或 Swagger UI；
- DTO 参数和响应的完整 DTS 推导；
- 新的 Controller/EPS DSL；
- 为未来未知消费方保留中间文档；
- 请求时重新编译 EPS；
- 执行依赖请求上下文的动态查询提供器；
- 修改权限校验行为。

权限自动推导在第 12 节记录，但必须作为独立改动实现和验证。

## 5. 方案比较

### 5.1 直接生成 Node 契约，采用

生成器将已经构造好的 `controller.Definition` 与 Descriptor 一起交给 EPS。EPS 结合 Graph 中的最终路由信息，一次生成最终契约。

优点：数据流最短；动态运行时配置不会在 codegen 中重复解析；删除中间模型；最接近 Node 和 v1。

### 5.2 扩展现有 Document，拒绝

继续给 `QuerySchema`、`Document` 和 `LegacyView` 补字段。

缺点：保留两套类型和两次转换；仍需维护无人直接消费的内部模型；继续增加代码。

### 5.3 完全在 codegen 中静态生成契约，拒绝

生成器解析 Controller AST 并直接写出所有 EPS 元数据。

缺点：重复解释运行时 Controller 配置；难以正确处理 QueryProvider；把业务契约逻辑塞进生成器，维护成本最高。

## 6. 最终契约

EPS 只保留 Node/前端实际使用的类型：

```go
type Views struct {
   Admin map[string][]Controller
   App   map[string][]Controller
}

type Controller struct {
   Module      string
   Name        string
   Prefix      string
   Info        Info
   API         []API
   Columns     []Column
   PageQueryOp PageQueryOp
   PageColumns []Column
}

type API struct {
   Method      string
   Path        string
   Summary     string
   DTS         map[string]interface{}
   Tag         string
   Prefix      string
   IgnoreToken bool
}

type Column struct {
   PropertyName string
   Type         string
   Length       string
   Comment      string
   Nullable     bool
   DefaultValue interface{}
   Dict         interface{}
   Source       string
}

type PageQueryOp struct {
   KeyWordLikeFields []string
   FieldEq           []string
   FieldLike         []string
}
```

字段 JSON 名称与 Node 现有响应完全一致。`DTS` 当前保持空对象，`Tag` 保持空字符串；没有真实数据来源的 `Dict` 保持 `null`，不为这些字段建立推导系统。

## 7. 数据来源与生成规则

### 7.1 模块与 Controller

- Graph 提供模块 Key、最终 Controller 路径、路由、开发环境标记和标签；
- 运行时 `controller.Definition` 提供 CurdOption 和 QueryProvider；
- Descriptor 提供实体名与字段元数据；
- Admin/App 按 Graph 中的最终 Controller 路径分区；
- 只输出至少包含一个可用 API 的 Controller；
- 最终结果按模块 Key 分组。

### 7.2 API

- `method`、`summary`、认证标签来自 Graph Route；
- `path` 必须是相对 Controller Prefix 的路径，例如 `/page`；
- `prefix` 使用 Controller 最终完整前缀；
- `ignoreToken` 由路由 `ignoreToken` 标签决定；
- 含 `{}` 路径参数且不能生成合法前端 service 方法的路由继续排除；
- 不输出内部 `Bind`、`Permission`、完整路径或 OpenAPI Schema。

### 7.3 Columns

- CRUD Controller 使用 Descriptor 的持久化字段；
- 非 CRUD Controller 的 `name` 为空、`columns` 为空，与 Node 行为一致；
- 排除 `tenantId`；
- `createTime`、`updateTime` 保持原顺序并移动到末尾；
- `source` 固定为 `a.<jsonName>`；
- 类型映射沿用 Node/v1 前端可识别名称；
- 长度、描述、可空、默认值直接来自 Descriptor；
- 不把 Hidden、Readonly、Sortable 等内部策略塞进前端 Column。

### 7.4 PageQueryOp

只读取 `CurdOption.PageQueryOp`，不把 ListQueryOp 混入分页契约：

- `KeyWordLikeFields` 输出字段来源，例如 `a.name`；
- `FieldEq` 输出参与等值匹配的字段来源；
- `FieldLike` 输出参与模糊匹配的字段来源；
- 显式请求参数名不改变 Column `source`；
- 所有切片必须输出 `[]`，不能输出 `null`。

### 7.5 PageColumns

静态 PageQueryOp 的 Select 决定分页额外字段：

- `All(alias)` 展开对应实体的全部字段；
- `As(column, alias)` 使用输出别名作为 `propertyName`；
- `source` 保留真实查询来源；
- 按 `source` 去重；
- `createTime`、`updateTime` 移到末尾。

Query AST 的具体节点属于 `crud`。EPS 不使用反射读取私有字段，也不复制查询编译器。`crud` 只暴露生成 EPS 所需的最小只读查询投影，复用现有字段、Join、Select 和 Descriptor 解析规则。

## 8. 静态与动态查询

`StaticQuery` 有稳定结构，完整生成 `pageQueryOp` 和 `pageColumns`。

`DynamicQuery` 可能依赖请求身份、租户或其他 Context 数据，不能在启动时用空 Context 擅自执行。动态查询保持运行时行为不变，EPS 为其输出空 `pageQueryOp` 和 `pageColumns`。

如果未来确实需要动态查询的静态前端描述，应先出现真实业务需求，再设计显式的稳定元数据；本次不增加第二套配置。

## 9. Controller 兼容性

业务 Controller 继续使用现有写法：

```go
controller.Admin().
   Options(...).
   Curd(controller.CurdOption{
      Entity:      entity.User{},
      Service:     service,
      PageQueryOp: controller.StaticQuery(...),
   }).
   Route(...).
   Build()
```

不要求业务模块：

- 增加 EPS 配置；
- 重复声明实体字段；
- 修改路由路径；
- 修改 StaticQuery/DynamicQuery 写法；
- 手动发布 EPS。

改动只发生在框架 EPS、必要的只读查询投影、codegen 装配和两个 EPS HTTP 返回入口。

## 10. 文件边界

目标是减少而不是增加文件：

- `cool-next/eps/eps.go`：最终契约、直接编译和已发布快照；
- 删除 `cool-next/eps/legacy.go`；
- 删除 `cool-next/eps/views.go`，其少量发布逻辑并入 `eps.go`；
- codegen 只负责把现有 Controller Definition 和 Descriptor 传入 EPS；
- `crud`/`core/controller` 只增加无法由现有公开 API 获得的最小只读入口。

不创建 Provider、Registry 接口、工厂、插件机制或配置开关。

## 11. 错误处理与验证

### 11.1 错误边界

- 未校验 Graph、缺失 Controller Definition 或 Descriptor：启动失败；
- 静态 Query 引用未知字段、实体或别名：启动失败；
- DynamicQuery：不执行，因此不会因为 EPS 产生额外副作用或启动错误；
- EPS 尚未发布：HTTP 入口返回现有 Core 错误；
- 请求阶段不做重新编译或降级猜测。

Graph 已经保证的模块重复、路由冲突、HTTP Method 合法性不在 EPS 重复校验。

### 11.2 最小验证集

1. Node 契约测试：固定 Admin/App 分组、API 相对路径、列顺序和空切片；
2. 查询测试：覆盖关键词、FieldEq、FieldLike、Join、All、As 和动态查询空投影；
3. codegen 测试：确认生成代码传入运行时 Controller Definition；
4. HTTP 契约测试：两个 EPS 路由返回最终 map，不再调用 LegacyView；
5. 全量 `go test ./...` 与 `go vet ./...`。

不恢复无人消费的 OpenAPI 哈希基线，不为简单字段映射建立大型 golden 框架。

## 12. 独立后续：权限自动推导

权限不是 EPS 职责，本次实现不修改鉴权。但已确认后续应单独对齐 Node：

1. Node 不声明 Route Permission；后台权限由完整 URL 自动推导；
2. `/admin/base/sys/user/move` 自动对应 `base:sys:user:move`；
3. Node 对 `ignoreToken` 路由跳过认证；
4. Node 对 `/admin/**/comm/**` 只校验登录，不校验菜单权限；
5. Node 不对 App 路由应用后台菜单权限。

Go v1 和当前 Go next 只对 CRUD 自动生成权限，自定义路由仍手写。当前仓库中的手写权限均与路径推导值相同，是可删除的重复信息。

后续权限改动固定为独立提交：后台受保护的非 Comm 路由按最终路径自动生成权限码，公开路由和 App 路由不生成后台权限；随后删除冗余的 `Route.Permission`、`CurdOption.PermissionPrefix` 及业务 Controller 手写值。该改动必须单独执行权限映射、管理员绕过、普通角色 403、Comm、App 和 ignoreToken 测试，不能与 EPS 重构共用一次行为变更。

## 13. 验收标准

完成实现后必须满足：

1. Controller 业务源码不因 EPS 对齐而修改；
2. `/admin/base/open/eps` 和 `/app/base/comm/eps` 可被当前 Vite 插件直接消费；
3. 静态 PageQueryOp 正确产生搜索字段与分页联表列；
4. EPS 包不再存在 OpenAPI 和内部 Document 遗留；
5. EPS 目录只保留必要实现；
6. 没有新增第三方依赖；
7. 权限行为在本次 EPS 提交中保持不变。
