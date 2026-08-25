# permmenu 接口返回嵌套树导致前端菜单不可点击

> 日期：2026-08-25
> 状态：Bug 已定位，未修复
> 修复范围：仅 Go 后端（前端不动）

## 1. 现象

前端管理端在 Go 版本后端（端口 8001）下：

- 侧边栏只显示菜单树的根节点（`type=0` 目录），子菜单（`type=1`）缺失
- 点击根节点下的空白处无反应
- 登录后首页重定向失效（找不到 `type=1` 节点）

切到 Node 版本后端（端口 8002 `cool-admin-midway`）恢复正常。

## 2. 结论

`PermissionService.PermissionMenu` 在 Go 侧返回**嵌套树**（顶层数组只含根节点，子节点嵌在 `childMenus`），而前端 `useMenuStore.get()` 用 `deepTree` 工具按 `parentId` 重建树，**只遍历顶层数组**。Node 侧返回**扁平列表**，`deepTree` 才能正确组装出树形。两侧契约不对齐是直接原因。

## 3. 根因

### 3.1 后端返回结构差异

**Go**（`cool-admin-go-next/modules/base/service/menu.go:616-641` `buildMenuItems`）：

```go
func buildMenuItems(rows []menuRow) []dto.MenuListItem {
    items := make(map[uint64]dto.MenuListItem, len(rows))
    children := make(map[uint64][]uint64)
    roots := make([]uint64, 0)
    for _, row := range rows {
        items[row.ID] = menuListItem(row)
        if row.ParentID == nil || *row.ParentID == 0 {
            roots = append(roots, row.ID)
            continue
        }
        children[*row.ParentID] = append(children[*row.ParentID], row.ID)
    }
    var build func(uint64) dto.MenuListItem
    build = func(id uint64) dto.MenuListItem {
        item := items[id]
        for _, childID := range children[id] {
            item.ChildMenus = append(item.ChildMenus, build(childID))
        }
        return item
    }
    result := make([]dto.MenuListItem, 0, len(roots))
    for _, root := range roots {
        result = append(result, build(root))
    }
    return result
}
```

只把 `roots` 放进顶层结果，每个根节点递归挂载 `ChildMenus`。JSON 形如：

```json
{
  "perms": ["..."],
  "menus": [
    { "id": 1, "parentId": null, "type": 0,
      "childMenus": [
        { "id": 2, "parentId": 1, "type": 0,
          "childMenus": [
            { "id": 4, "parentId": 2, "type": 1, "childMenus": null }
          ]
        }
      ]
    }
  ]
}
```

**Node**（`cool-admin-midway/src/modules/base/service/sys/menu.ts:114-127` `getMenus`）：

```typescript
async getMenus(roleIds, isAdmin) {
  const find = this.baseSysMenuEntity.createQueryBuilder('a');
  if (!isAdmin) {
    find.innerJoinAndSelect(
      BaseSysRoleMenuEntity, 'b',
      'a.id = b.menuId AND b.roleId in (:...roleIds)', { roleIds }
    );
  }
  find.orderBy('a.orderNum', 'ASC');
  const list = await find.getMany();
  return _.uniqBy(list, 'id');
}
```

`getMany()` 返回全部菜单行的扁平数组，`childMenus` 字段不被赋值。JSON 形如：

```json
{
  "perms": ["..."],
  "menus": [
    { "id": 1, "parentId": null, "type": 0 },
    { "id": 2, "parentId": 1, "type": 0 },
    { "id": 3, "parentId": 1, "type": 0 },
    { "id": 4, "parentId": 2, "type": 1, "router": "/admin/user/list" },
    { "id": 5, "parentId": 3, "type": 1, "router": "/admin/menu/list" }
  ]
}
```

### 3.2 前端消费方式

`cool-admin-vue/src/modules/base/store/menu.ts:114-153` `get()`：

```typescript
const list = res.menus
    ?.filter(e => e.type != 2)
    .map(e => ({
        ...e,                                 // ← childMenus 跟随展开
        path: revisePath(e.router || String(e.id)),
        isShow: e.isShow === undefined ? true : e.isShow,
        meta: { ...e.meta, label: e.name, keepAlive: e.keepAlive || 0 },
        name: `${e.name}-${e.id}`,
        children: []                          // ← 新增 children: []
    }));
setGroup(list);                              // → deepTree(list)
```

`cool-admin-vue/src/cool/utils/index.ts:199-219` `deepTree`：

```typescript
export function deepTree(list: any[], sort?: 'desc' | 'asc'): any[] {
    const newList: any[] = [];
    const map: any = {};

    orderBy(list, 'orderNum', sort)
        .map(e => { map[e.id] = e; return e; })
        .forEach(e => {
            const parent = map[e.parentId];
            if (parent) {
                (parent.children || (parent.children = [])).push(e);
            } else {
                newList.push(e);
            }
        });

    return newList;
}
```

`deepTree` 只遍历传入数组的顶层元素，按 `parentId` 查找父节点挂到 `parent.children`。**不会下钻到 `childMenus`**。

### 3.3 失配点

| 后端 | `list` 顶层元素数 | `deepTree` 后 `group` | 侧栏可见 |
|---|---|---|---|
| Node 扁平 | 全部菜单行 | `[{root, children:[mid, mid2, ...]}]` | 正常 |
| Go 嵌套 | 仅根节点 | `[{root, children:[]}]` | 只剩根目录，子菜单不可见 |

Go 嵌套版本中，4 个子菜单被埋在 `root.childMenus` 里，根本没有进入 `list`，`deepTree` 永远不会遍历到它们，所以侧栏为空、点不到、`getPath(group)` 返回空串导致首页重定向失效。

## 4. 证据

1. `cool-admin-go-next/modules/base/service/menu.go:616-641` `buildMenuItems` —— 顶层数组仅含 `roots`，子树挂在 `ChildMenus`
2. `cool-admin-midway/src/modules/base/service/sys/menu.ts:114-127` `getMenus` —— 返回 `getMany()` 扁平结果
3. `cool-admin-vue/src/cool/utils/index.ts:199-219` `deepTree` —— 单层遍历，按 `parentId` 重建
4. `cool-admin-vue/src/modules/base/store/menu.ts:114-153` `get()` —— `filter(e => e.type != 2)` 后直接交给 `deepTree`，未做扁平化
5. 在 `/tmp/menu_sim/main.go` 复现 Go `buildMenuItems` 等价逻辑，构造 5 条菜单（1 根 + 2 中间 + 2 叶）输出 JSON，确认顶层数组只有 1 条根节点，叶子嵌在 `childMenus`

## 5. 修复方案（前端不动）

### 5.1 范围约束

`dto.MenuListItem` 同时被多个接口复用：

| 接口 | 期望结构 | 消费方 |
|---|---|---|
| `/admin/base/comm/permmenu` | 扁平 | 前端 `useMenuStore.get()` `deepTree` |
| `/admin/base/menu/list`、`/page` | 待确认 | 菜单管理 CRUD 页（`cool-admin-vue/src/modules/base/views/menu/index.vue`），CRUD 框架通过 `childMenus`/`children` 渲染树形表格 |

`dto.MenuTree.ChildMenus` 被菜单导入导出两端共同依赖（`cool-admin-midway/src/modules/base/service/sys/menu.ts:400,417,437` 与 Go `seed/runtime.go:174` 的种子递归解析），不能动。

### 5.2 改 `PermissionMenu`，不动 `MenuService.List`

`PermissionService.PermissionMenu`（`modules/base/service/permission.go:194-226`）当前直接复用 `menuService.List(ctx)`。改为单独调一个返回扁平结果的查询，避开 CRUD 接口对树形结构的依赖。

最小改动方案 —— 新增私有方法 `flatVisibleMenus` 给 `PermissionService` 用，复用 `service.Base.Model` + `WhereIn` 逻辑（与 `MenuService.List` 同构），但返回扁平切片。`MenuService.List` 维持原样。

```go
// 在 PermissionService 上新增
func (service *PermissionService) flatVisibleMenus(ctx context.Context) ([]dto.MenuListItem, error) {
    identity, err := auth.Admin(ctx)
    if err != nil { return nil, err }
    model, err := service.menu.Model(ctx)
    if err != nil { return nil, err }
    if isAdmin, err := service.IsAdmin(ctx, identity.RoleIDs()); err != nil {
        return nil, err
    } else if !isAdmin {
        menuIDs, err := service.menuIDsByRoles(ctx, identity.RoleIDs())
        if err != nil { return nil, err }
        if len(menuIDs) == 0 { return []dto.MenuListItem{}, nil }
        model = model.WhereIn("id", menuIDs)
    }
    var rows []menuRow
    if err = model.OrderAsc("orderNum").OrderAsc("id").Scan(&rows); err != nil {
        return nil, exception.WrapCore(err, "查询菜单列表失败")
    }
    result := make([]dto.MenuListItem, 0, len(rows))
    for _, row := range rows {
        item := menuListItem(row)
        result = append(result, item)
    }
    return result, nil
}
```

把 `PermissionMenu` 内的 `menus, err := service.menuService.List(ctx)` 换成 `menus, err := service.flatVisibleMenus(ctx)`。

### 5.3 同步动作

- 给 `flatVisibleMenus` 加单测：扁平结构、`orderNum` 排序、空 `roleIDs`、空 `menuIDs`、超管场景
- 验证 `PermissionMenu` 返回结构与 Node `getMenus` 一致：`menus[*]` 全部包含 `parentId`，顶层数组长度等于可见菜单数
- `dto.MenuListItem.ChildMenus` 与 `dto.MenuTree.ChildMenus` **保留**，导入导出和 CRUD 树表还在用
- 顺手把 `MenuService.buildMenuItems` 的逻辑（递归建树）保留在 `MenuService.List` 里，菜单管理页继续可用

## 6. 顺带观察

`menuListItem`（`menu.go:644`）不填 `ParentName`，只有 `Info` 单查时填（`menu.go:205-207`），Node 端 `permmenu` 也不填 `parentName`，两边一致，不在本次修复范围内。

## 7. 不修的相邻问题

- `cool-admin-go-next/modules/base/service/menu.go:82-99` `Add`、`:102-155` `Update`、`:158-187` `Delete` 中的 `if service == nil || service.runtime == nil` 守卫属于死代码，`NewMenu` 已保证依赖非 nil，本次不顺手清理
