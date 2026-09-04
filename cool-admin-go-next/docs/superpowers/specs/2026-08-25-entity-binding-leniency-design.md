# 实体绑定宽容度设计

> 日期：2026-08-25  
> 状态：已实现  
> 影响面：`cool-next/core/controller/binder.go`、`cool-next/core/service/base.go`

## 1. 背景

`cool-admin-vue` 的多个页面用「取回整行 → 挂派生字段 → 编辑后整行回传」的写法。以任务页为例，`src/modules/task/views/list.vue:121` 有：

```js
list.value = res.list.map(e => { if (e.every) { e._every = parseInt(String(e.every / 1000)) } return e })
```

编辑表单以 `form: { ...item }` 打开，提交时把整行连同 `_every`、`createTime`、`updateTime` 一起发回。

Node 侧 TypeORM 静默忽略多余键，因此这套写法一直成立。Go 侧有两处拒绝：

| 位置 | 行为 | 被挡的字段 |
| --- | --- | --- |
| `binder.go` `decodeMutable` | 未知 JSON 字段报错 | `_every` |
| `base.go` `mutableData` | Update 拒绝只读字段 | `createTime`、`updateTime` |

前端是与 Node 后端共用的既有客户端，不在本仓库的修改范围内，因此收敛点在后端。

## 2. 决策

### 2.1 视图字段按前缀放行

`decodeMutable` 遇到实体不存在的 JSON 字段时，键名以 `_` 开头则丢弃，其余仍然报错。

保留严格性的部分是有价值的那一半：字段名拼错（`nmae`）依旧在绑定阶段暴露。放开的只是前端约定的视图字段命名空间。

不采用「未知字段一律忽略」：那会让所有拼写错误变成静默丢弃，用一个真实缺陷类别换一个命名约定，不划算。

### 2.2 只读字段在 Add 与 Update 上行为一致

`mutableData` 原本：

```go
if action == crud.ActionAdd && item.source == fieldSourceClient {
    continue                                   // Add：忽略
}
if action == crud.ActionUpdate {
    return ... 只读字段不允许更新              // Update：报错
}
```

改为对 `fieldSourceClient` 来源一律忽略，`fieldSourceServer`（业务代码显式写入）在 Update 上仍然报错。

这不是放宽策略，是修不对称：客户端回传只读字段在 Add 上已经被认定为无害噪音，Update 没有理由更严。真正需要拦的是业务代码误写只读字段，那条护栏原样保留。

主键不受影响——`decodeEntityItems` 在构造 `Mutable` 前就把主键从原始 map 里摘走，交给 `UpdateItem.id`，从不进入 `mutableData` 的只读判定。

## 3. 影响

1. 任务模块删除自带的 `BodyNormalizer`（108 行 + 一个 DI 组件 + 一个 `Before` 钩子）；
2. 后续迁移的模块不必各自复制一份请求体裁剪；
3. DTO 绑定路径（`BindJSON` 的 `DisallowUnknownFields`、`bindDTOMap`）不变——DTO 是后端自己定义的契约，没有前端整行回传的问题。

## 4. 验证

| 测试 | 钉住的行为 |
| --- | --- |
| `TestDecodeMutableDropsViewFields` | `_` 前缀未知字段被丢弃，实体字段正常绑定 |
| `TestDecodeMutableStillRejectsUnknownFields` | 拼错的字段名仍然报错 |
| `TestMutableDataIgnoresClientReadonlyFields` | 客户端回传的只读字段在 Add 与 Update 上都被忽略 |
| `TestMutableDataRejectsServerReadonlyUpdate` | 业务代码更新只读字段仍然报错 |

工程门禁：`cool generate`、`cool check`、`go test ./...`、`go vet ./...`、`gofmt -l`。
