# Controller 装配入口统一设计

## 背景

当前 `base` 与 `dict` 模块通过 `controller/controllers.go` 聚合 Controller，`task` 与 `recycle` 模块则直接在模块 `register.go` 的 `ModuleSpec.Controllers` 回调中完成装配。两种方式行为等价，但增加了不必要的目录结构差异。

## 目标

- 删除 `modules/base/controller/controllers.go`。
- 删除 `modules/dict/controller/controllers.go`。
- 将两个文件中的 Service 创建、模型查找和 Controller 列表构建迁移到对应模块的 `register.go`。
- 保持路由、权限、Controller 顺序、依赖生命周期和模块注册行为不变。

## 设计

### Base 模块

`modules/base/register.go` 的 `Controllers` 回调继续加载 Base 配置，并在回调内完成以下装配：

1. 从模块模型定义建立按表名索引的映射。
2. 创建会话存储、认证服务、权限服务和各系统业务 Service。
3. 注入 SSO、回收站、上传目录及 EPS Provider 等现有依赖。
4. 按当前顺序返回 9 个 Controller 定义。

现有 `Controllers` 与 `ControllersWithOptions` 聚合函数不再保留。

### Dict 模块

`modules/dict/register.go` 的 `Controllers` 回调直接基于模块模型定义创建字典类型和字典信息 Service，并返回两个 Controller 定义。

### 模型索引

两个模块都需要按表名读取模型。为避免在 Controller 包中保留仅服务于装配的辅助函数，各模块在自己的 `register.go` 中保留私有模型索引函数。该函数只负责将 `[]model.Definition` 转换为以 `TableName` 为键的映射。

## 测试调整

- 将 Base Controller 聚合测试改为通过 Base 模块装配入口获得 Controller 定义，继续覆盖数量、顺序、路由、权限和依赖绑定。
- 为 Dict 模块保留或补充装配结果验证，确保两个 Controller 均被注册。
- 运行 `go test ./modules/base ./modules/dict`。
- 运行 `go test ./... -count=1` 验证模块注册与路由收集没有回归。

## 非目标

- 不修改单个 Controller 的实现。
- 不改变 `registry.ModuleSpec` 或应用启动流程。
- 不调整 Service、实体或数据库定义。
- 不重构 `task`、`recycle` 模块。

## 风险控制

主要风险是迁移时遗漏依赖或改变 Controller 顺序。通过复用现有构造参数、保持返回列表顺序，并运行现有 Controller 与模块级测试控制风险。
