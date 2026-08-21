# 模块 30：Controller DSL 与 CurdOption 实施计划

> 日期：2026-08-11
> 依据：`docs/superpowers/specs/2026-08-11-30-controller-dsl-curd-option-design.md`
> 范围：30.1-30.11
> 状态：已完成

## 任务 1：查询响应形状策略

修改 `cool-next/crud/query.go`、`plan.go` 及对应测试：

1. 为 QueryOp 增加包内私有形状策略；
2. 提供 Controller 可调用的静态/动态 QueryOp 标记入口；
3. Static Extend 只允许替换外层 Select 已声明的单字段别名；
4. Dynamic Query 禁止自定义输出别名和 Extend AddSelect；
5. 保持现有字段、Join、节点和重复输出校验不变；
6. 先写失败测试，再实现最小合并逻辑。

验收：`go test ./cool-next/crud -count=1`。

## 任务 2：Controller DSL

创建 `cool-next/core/controller`，职责按真实边界拆分为少量文件：

1. Definition、Builder、Admin/App、路径基础校验；
2. CurdOption、APIType、URLTag、防御性复制和用户字段策略；
3. crud 类型别名及同名构造包装；
4. StaticQuery、DynamicQuery 密封实现；
5. Before、InsertParam 及强类型顺序执行；
6. CompilePlan 转换到唯一 `crud.CompilePlan`；
7. 测试不可伪造、复制隔离、Page/List 独立、错误传播和字段策略。

验收：`go test ./cool-next/core/controller ./cool-next/crud -count=1`。

## 任务 3：Controller 静态分析

修改 `cool-next/codegen` 的分析模型和专用分析文件：

1. 发现 admin/app Controller 工厂及源码位置；
2. 推导空路径并校验显式路径、Prefix 和区域；
3. 允许零个 Curd，拒绝重复 Curd；
4. 从 CurdOption 取得 Entity、Service、InsertParam 静态类型；
5. 校验 Service 直接匿名 Base 的 Entity/ID 泛型；
6. 只保存 ControllerDeclaration，不渲染路由或注册表；
7. 用 overlay fixture 覆盖成功和诊断位置。

验收：`go test ./cool-next/codegen -count=1`。

## 任务 4：全量门禁

依次运行：

```bash
gofmt -w <本模块修改的 Go 文件>
go test ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen -count=1
go test -race ./cool-next/core/controller ./cool-next/crud ./cool-next/codegen -count=1
go test ./... -count=1
go vet ./...
make check
git diff --check
```

最终逐项核对 30.1-30.11，不修改模块 31/32 的 Route、Binder、Dispatcher Adapter 或生成注册表。
