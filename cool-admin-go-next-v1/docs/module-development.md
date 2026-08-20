# 模块目录与自动装配

`modules` 的每个直接子目录是一个模块。新模块不需要 `register.go`、`entity/models.go`、`module.yaml` 或注释指令；生成器仅依赖目录、导出函数名和 Go 类型签名。

## 目录协议

| 目录 | 组件 |
| --- | --- |
| `entity/**` | 模型定义 |
| `service/**` | 业务服务、普通 Provider 和 Task HandlerDefinition |
| `controller/**` | Controller |
| `middleware/global/**` | 全局中间件 |
| 其他 `middleware/**` | 当前模块路由中间件 |
| `event/**` | 事件与 Runtime |
| `schedule/**` | 定时任务 Runtime |
| `queue/**` | 队列 |
| `dto/**` | 数据传输对象 |
| 模块根 `config.go` | `ModuleConfig() module.Declaration[Config]` |
| `db.json` / `menu.json` | 可选初始化数据 |

模块内部可以任意深度嵌套。`_test.go`、`testdata`、隐藏目录和标准生成文件不参与发现。自动组件不得放在 GOOS/GOARCH 专属文件中。

## 最小模块

只需创建模块目录和符合签名的组件，无需修改任何全局清单：

```text
modules/report/
├── controller/admin/report.go
├── entity/report.go
└── service/report.go
```

执行 `go run ./cmd/cool generate` 后，生成器会在 `modules/` 下创建 `modules_gen.go`（集中生成全部模块的装配代码），并将 `report` 加入 `modules.Specs()`。生成文件必须提交，但不得手工修改。

## 构造签名

Provider 必须返回 `T` 或 `(T, error)`，不能使用可变参数。同一输出类型只能有一个标准 `New<Type>`；`Build*`、`New<Type>With*` 等非标准辅助构造函数不参与发现。

Controller 工厂返回 `controller.Definition` 或 `(controller.Definition, error)`。Controller 及其 Provider 的构造错误会由 Application 构建流程返回，不会在生成代码中触发 panic。

模型参数通过精确名称绑定。例如 `BaseSysUser()` 对应 `baseSysUserModel`，`TaskLog()` 对应 `taskLogModel`。`string`、`int`、`[]string` 等原始标量不会自动注入，应放入强类型 `Config` 或专用类型。

Controller 集合的延迟依赖只允许使用 `registry.ControllerProvider`。其他循环依赖会在生成期失败。

## 命令

```bash
# 分析全部模块并更新生成文件
go run ./cmd/cool generate

# 只比较内存候选输出，不写盘
go run ./cmd/cool check

# 生成后构建或运行
go run ./cmd/cool build
go run ./cmd/cool run
```

所有候选输出先经过 `go/format` 和 Overlay 类型检查；任何错误都不会覆盖上一版可编译输出。

## 常见错误

```text
标量依赖 rootDir string 不参与自动注入，请使用强类型 Config
模型参数 definition 无法唯一绑定，期望名称 definitionModel
接口依赖 store example.Store 存在多个实现
Provider 循环依赖: module/service.NewA -> module/service.NewB
```

错误会同时包含模块、包、符号和源码位置，应修正构造签名或明确的业务适配 Provider，不得引入运行时 Service Locator。
