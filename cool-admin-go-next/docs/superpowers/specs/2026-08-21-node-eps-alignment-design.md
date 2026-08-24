# Node EPS 契约对齐设计

> 日期：2026-08-21
> 状态：已实现
> 范围：Go EPS 自动投影与 Node 返回契约对齐；前端和权限自动推导不在本次改动范围

## 1. 结论

Go next 的 EPS 服务当前 `cool-admin-vue` 代码生成与搜索元数据消费，不再保留 OpenAPI、通用文档模型或未来消费方扩展点。

核心原则：

> EPS 完全从已校验 Graph、运行时 Controller Definition、CurdOption、静态 Query AST 和 Entity Descriptor 自动生成；业务模块不得声明任何 EPS 专用元数据。

实现采用直接投影：

```text
已校验 Graph + Controller Definition + Entity Descriptor
                         ↓
              只读 Controller/Query 投影
                         ↓
                 按有效 Prefix 分桶
                         ↓
             Admin/App Vue EPS 兼容契约
                         ↓
                  启动时发布一次
                         ↓
                /admin、/app EPS 接口
```

不再经过 `Document -> LegacyView` 二次转换。业务 Controller DSL 保持不变，不增加 EPS 注解、配置、接口或重复字段。

## 2. 背景与根因

Node `CoolEps` 聚合 Midway 路由、Controller 配置、TypeORM Entity 和分页查询配置，生成前端代码生成器需要的数据。Swagger 是独立模块读取 EPS 后再转换，不属于 EPS 核心。

Go v1 同样直接从 Controller Definition 生成最终契约。Go next 曾同时编译内部 EPS 文档和 OpenAPI，因此建立了 `Document`、`Module`、`Entity`、`Field`、`API` 等中间模型。OpenAPI 已删除，但中间模型和 `LegacyView` 仍然保留，造成以下问题：

1. 同一份数据先扩充为内部文档，再缩减回 Vue 契约；
2. 大量字段没有消费方；
3. Graph 已校验后，EPS 又执行部分重复校验；
4. `pageQueryOp` 和 `pageColumns` 在最终响应中被固定为空；
5. 代码量明显高于 Node 和 Go v1，实际前端能力反而不完整。

同时，Go Controller DSL 比 Vue 当前消费的 EPS 契约表达力更强：

- `CurdOption.Prefix` 可以与 Controller 自定义路由前缀不同；
- `EqFrom`、`LikeFrom` 可以让查询字段名与请求参数名不同；
- Hidden、Readonly 等字段策略只存在于运行时 Controller 定义；
- DynamicQuery 可能依赖请求身份、租户或其他 Context 数据。

因此不能把一个 Controller Definition 机械映射成一个 EPS Controller，也不能丢弃已有 DSL 中的业务语义。根因不是 GoFrame 或文件数量，而是失去第二消费方的多阶段编译架构与过窄的前端查询元数据同时存在。

## 3. 目标

本次实现必须：

1. EPS HTTP 响应与 Node `CoolEps` 的返回契约一致；
2. 业务 Controller 不增加任何 EPS 专用配置；
3. 自动生成 Admin/App 两份按模块分组的数据；
4. 按路由有效 Prefix 自动拆分或合并 EPS Controller；
5. 恢复静态 `PageQueryOp` 的搜索字段与 `pageColumns`；
6. 复用已有 Hidden、Readonly 等 Controller 字段策略，不重复声明；
7. 兼容 `EqFrom`、`LikeFrom` 的显式请求参数名；
8. 保留开发路由与 `ignoreToken` 过滤语义；
9. 删除没有消费方的内部模型、字段、常量和转换；
10. 不增加第三方依赖；
11. 启动时生成一次，请求时只读取已发布快照。

## 4. 非目标

本次不实现：

- OpenAPI、Swagger JSON 或 Swagger UI；
- DTO 参数和响应的完整 DTS 推导；
- 新的 Controller/EPS DSL；
- 业务模块手写 EPS Prefix、Column 或 Query 配置；
- 为未来未知消费方保留中间文档；
- 请求时重新编译 EPS；
- 执行依赖请求上下文的 DynamicQuery；
- 修改 `cool-admin-vue` 或其他后端实现；
- 修改权限校验行为。

权限自动推导在第 13 节记录，但必须作为独立改动实现和验证。

## 5. 方案比较

### 5.1 框架自动投影并对齐 Node 契约，采用

EPS 从运行时 Definition 和 Descriptor 读取现有业务语义，按有效 Prefix 生成最终契约。`fieldEq`、`fieldLike` 原样对齐 Node 已有的字符串和 `{ column, requestParam }` 两种格式，不修改前端消费代码。

优点：业务零新增配置；数据流最短；保留现有 DSL 能力；Go 与 Node 返回格式一致；删除中间模型。

### 5.2 Go 后端丢弃无法表达的查询信息，拒绝

对 `EqFrom`、`LikeFrom` 只输出字段来源或直接忽略。

缺点：前一种做法会让前端提交错误参数，后一种做法会无声失去自动搜索能力；两者都不符合快速开发框架的目标。

### 5.3 增加 EPS 专用业务配置，拒绝

要求业务 Controller 重复声明 EPS Prefix、字段、请求参数或动态查询说明。

缺点：同一业务事实出现两个来源，容易漂移；增加样板代码；把框架兼容责任推给业务模块。

## 6. 最终契约

Go EPS 只保留 Vue 实际消费的最终类型：

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

type Info struct {
   Type InfoType
}

type InfoType struct {
   Name        string
   Description string
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
   FieldEq           []QueryField
   FieldLike         []QueryField
}

type QueryField struct {
   Column       string
   RequestParam string
}
```

`QueryField` 的 JSON 形式兼容 Node：

- 请求参数名等于字段名时输出字符串，例如 `"a.status"`；
- 请求参数名不同时输出 `{ "column": "a.status", "requestParam": "state" }`；
- `QueryField` 是 Go 内部只读值，通过受控 JSON 编码产生上述联合格式，不把通用 `interface{}` 切片扩散到编译逻辑；
- 本次只对齐后端返回格式，不修改前端如何消费这两种形式。

字段 JSON 名称与现有 Vue EPS 契约一致。`DTS` 当前保持空对象，`Tag` 保持空字符串；没有真实数据来源的 `Dict` 保持 `null`，不为这些字段建立新的推导系统。所有集合必须输出 `[]`，不能输出 `null`。

## 7. 数据来源与生成规则

### 7.1 只读 Definition 投影

运行时 `controller.Definition` 保持不可变和封闭。`core/controller` 提供仅供框架消费的只读快照，包含：

- Admin/App 区域；
- Controller 相对路径与 RouterOptions；
- CurdOption 的 Entity、PageQueryOp、字段策略和 Prefix；
- 自定义 Route 的方法、相对路径、标签和 IgnoreGlobalPrefix；
- 已复制的切片和 QueryProvider，不暴露可变内部状态。

业务构造函数仍然只返回现有 `controller.Definition`。codegen 负责把已经构造的 Definition 与对应 Descriptor 传入 EPS，不重新解析 Query AST，也不要求业务手动发布。

### 7.2 Prefix 分桶

一个 Definition 可以生成一个或多个 EPS Controller。EPS 先过滤当前环境不可用的路由和不支持的路径参数路由，再按每条路由的有效 Prefix 分桶：

- CRUD 路由的 Prefix 取 Graph 中已生成 CRUD 完整路径的公共父路径；
- 自定义路由的 Prefix 由 Graph 完整路径与 Definition 中的相对 Route Path 共同确定；
- Route 或 Controller 的 `IgnoreGlobalPrefix` 必须反映在有效 Prefix 中；
- 相同 Prefix 的 CRUD 和自定义 API 合并；
- 不同 Prefix 自动拆成多个 EPS Controller；
- CRUD 桶携带 Entity、Columns、PageQueryOp 和 PageColumns；
- 纯自定义桶的 `name` 为空，`columns`、`pageColumns` 和查询集合为空；
- 过滤后没有 API 的桶不输出；
- 最终结果按模块 Key 分组。

例如 Controller 路径为 `/app/public/items`，CurdOption Prefix 为 `/app/archive/items` 时，框架自动输出：

```text
/app/archive/items -> CRUD API、Entity、Columns、PageQueryOp
/app/public/items  -> 自定义 API、空 Columns
```

业务模块不需要拆 Controller，也不需要声明 EPS Prefix。

### 7.3 Admin/App 与环境

- Admin/App 使用 Definition 的区域语义分区，不从最终 URL 猜测；
- Controller `DevelopmentOnly` 在非开发环境排除整个 Definition；
- Route `DevelopmentOnly` 在非开发环境只排除对应路由；
- 开发环境包含上述 Controller 和 Route；
- 环境过滤发生在 Prefix 分桶之前。

### 7.4 API

- `method`、`summary` 和 `ignoreToken` 来自已校验 Graph Route；
- `path` 是相对当前桶 Prefix 的路径，例如 `/page`；
- `prefix` 等于当前桶的完整 Prefix；
- 含 `{`、`}` 或 `:` 路径参数的路由不进入 EPS，因为当前 Vue service 生成器不具备路径参数替换能力；
- 该过滤是面向当前 Vue 消费方的明确兼容差异，不宣称 Node 后端会过滤这些路由；
- 不输出内部 `Bind`、`Permission`、完整路径或 OpenAPI Schema。

### 7.5 Columns

- CRUD 桶使用 Descriptor 的持久化字段；
- 排除 `tenantId`；
- 排除 CurdOption `HiddenFields` 中的根实体字段；
- Readonly 但非 Hidden 的字段仍输出，因为它可能存在于查询响应；
- 不在前端 Column 中增加 Hidden、Readonly、Sortable 等内部策略字段；
- `createTime`、`updateTime` 保持原顺序并移动到末尾；
- `source` 固定为 `a.<jsonName>`；
- 类型映射沿用 Node/v1 前端可识别名称；
- 长度、描述、可空、默认值直接来自 Descriptor。

密码、`seedKey` 等字段只要已经由业务 CRUD 策略声明为 Hidden，EPS 就自动排除。EPS 不要求业务重复声明安全策略，也不按字段名称猜测业务敏感性。本规则取代 Base 迁移设计中“`seedKey` 作为 hidden/readonly EPS 字段出现”的旧要求：最终 Vue 契约没有 Hidden 标记，因此 Hidden 字段必须完全不出现。

### 7.6 PageQueryOp

只读取 `CurdOption.PageQueryOp`，不把 ListQueryOp 混入分页契约：

- `KeyWordLikeFields` 输出字段来源，例如 `a.name`；
- `Eq`、`Like` 输出字段来源字符串；
- `EqFrom`、`LikeFrom` 在请求参数名不同时输出 `{ column, requestParam }`；
- 字段引用、实体、别名和请求参数由现有 Query 编译规则校验；
- 所有切片必须输出 `[]`，不能输出 `null`。

### 7.7 PageColumns

静态 PageQueryOp 的 Select 决定分页响应的额外字段：

- `All(alias)` 展开对应实体的全部字段；
- 根实体 `All("a")` 同样排除 `tenantId` 和 HiddenFields；
- `As(column, alias)` 使用输出别名作为 `propertyName`；
- `source` 保留真实查询来源；
- 不按 `source` 去重，同一来源的不同输出别名全部保留；
- 重复 `propertyName` 由现有 Query 编译规则拒绝；
- `createTime`、`updateTime` 移到末尾。

Query AST 的具体节点属于 `crud`。EPS 不使用反射读取私有字段，也不复制查询编译器。`crud` 只暴露生成 EPS 所需的最小只读查询投影，复用现有字段、Join、Select 和 Descriptor 解析规则。

## 8. 静态与动态查询

`StaticQuery` 有稳定结构，完整生成 `pageQueryOp` 和 `pageColumns`。

`DynamicQuery` 可能依赖请求身份、租户或其他 Context 数据，不能在启动时用空 Context 执行。动态查询保持运行时行为不变，EPS 为其输出空 `pageQueryOp` 和 `pageColumns`。

这是相对 Node 启动期执行函数型 PageQueryOp 的有意安全差异。框架不要求业务为 DynamicQuery 再写一套静态 EPS 描述；未来只有出现真实业务需求时才单独设计。

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
- 重复声明实体字段或查询字段；
- 为 EPS 拆分 Controller；
- 修改路由路径；
- 修改 StaticQuery、DynamicQuery、EqFrom 或 LikeFrom 写法；
- 手动发布 EPS。

已有 Hidden、Readonly、PageQueryOp、Prefix 和 Route 都是业务运行时语义，EPS 只读取并复用，不建立第二事实来源。

## 10. 前端边界

`cool-admin-vue`、Node 和 Java 均保持原样。本次只让 Go EPS 返回 Node `CoolEps` 已有的契约，不在前端增加 Go 专用兼容、后端识别或版本分支。

## 11. 文件边界

目标是减少 Go EPS 文件和业务样板代码：

- `cool-next/eps/eps.go`：最终契约、Prefix 分桶、直接编译和已发布快照；
- 删除 `cool-next/eps/legacy.go`；
- 删除 `cool-next/eps/views.go`，其少量发布逻辑并入 `eps.go`；
- `cool-next/core/controller`：增加 Definition 的最小只读快照入口；
- `cool-next/crud`：增加静态 Query 的最小只读投影入口；
- codegen：只把现有 Controller Definition 和 Descriptor 传入 EPS；
- 两个 EPS HTTP 入口：直接返回最终 map；

不创建业务 Provider、Registry、工厂、插件机制、配置开关或 EPS DSL。

## 12. 错误处理与验证

### 12.1 错误边界

- 未校验 Graph：启动失败；
- Graph Controller 缺少对应运行时 Definition：启动失败；
- 声明 CRUD 的 Controller 缺少 Entity 或 Descriptor：启动失败；
- 非 CRUD Controller 不要求 Descriptor；
- 路由无法无歧义计算有效 Prefix 或相对 Path：启动失败；
- 静态 Query 引用未知字段、实体、别名或请求参数：启动失败；
- DynamicQuery 不执行，因此不会因为 EPS 产生额外副作用或启动错误；
- EPS 尚未发布：HTTP 入口返回现有 Core 错误；
- 请求阶段不做重新编译或降级猜测。

Graph 已经保证的模块重复、路由冲突和 HTTP Method 合法性不在 EPS 重复校验。

### 12.2 最小验证集

1. 契约测试：固定 Admin/App 分组、API 相对路径、列顺序、JSON 字段名和空切片；
2. Prefix 测试：覆盖 CurdOption Prefix 与 Controller Path 不同、自定义路由同桶与跨桶、IgnoreGlobalPrefix；
3. 字段测试：覆盖 tenantId、Hidden、Readonly、createTime 和 updateTime；
4. 查询测试：覆盖关键词、Eq、Like、EqFrom、LikeFrom、Join、All、As、同源不同别名和 DynamicQuery 空投影；
5. codegen 测试：确认生成代码传入运行时 Controller Definition 与 Descriptor；
6. HTTP 契约测试：两个 EPS 路由返回最终 map，不再调用 LegacyView；
7. Go 全量 `go test ./...` 与 `go vet ./...`。

不恢复无人消费的 OpenAPI 哈希基线，不为简单字段映射建立大型 golden 框架。

## 13. 独立后续：权限自动推导

权限不是 EPS 职责，本次实现不修改鉴权。但已确认后续应单独对齐 Node：

1. Node 不声明 Route Permission；后台权限由完整 URL 自动推导；
2. `/admin/base/sys/user/move` 自动对应 `base:sys:user:move`；
3. Node 对 `ignoreToken` 路由跳过认证；
4. Node 对 `/admin/**/comm/**` 只校验登录，不校验菜单权限；
5. Node 不对 App 路由应用后台菜单权限。

Go v1 和当前 Go next 只对 CRUD 自动生成权限，自定义路由仍手写。当前仓库中的手写权限均与路径推导值相同，是可删除的重复信息。

后续权限改动固定为独立提交：后台受保护的非 Comm 路由按最终路径自动生成权限码，公开路由和 App 路由不生成后台权限；随后删除冗余的 `Route.Permission`、`CurdOption.PermissionPrefix` 及业务 Controller 手写值。该改动必须单独执行权限映射、管理员绕过、普通角色 403、Comm、App 和 ignoreToken 测试，不能与 EPS 重构共用一次行为变更。

## 14. 验收标准

完成实现后必须满足：

1. 业务 Controller 源码不因 EPS 对齐而增加或修改配置；
2. `/admin/base/open/eps` 和 `/app/base/comm/eps` 可被更新后的 Vite 插件直接消费；
3. 不同有效 Prefix 自动拆桶，前端生成的每个请求 URL 都与 Graph Route 一致；
4. Hidden 字段不进入 EPS，Readonly 非 Hidden 字段仍可用于响应类型；
5. StaticQuery 正确产生搜索字段与分页联表列，EqFrom/LikeFrom 保留请求参数名；
6. DynamicQuery 不在 EPS 编译期执行；
7. EPS 包不再存在 OpenAPI 和内部 Document 遗留；
8. EPS 目录只保留必要实现；
9. 没有新增第三方依赖；
10. 权限行为在本次 EPS 提交中保持不变。
