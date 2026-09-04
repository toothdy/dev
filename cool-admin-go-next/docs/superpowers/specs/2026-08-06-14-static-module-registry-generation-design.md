# 模块 14：静态模块注册表生成设计

> 日期：2026-08-06
> 状态：待复核
> 模块：14 静态模块注册表生成
> 对应拆分项：14.1-14.9
> 前置模块：12 Provider 图、13 Descriptor 与 DO 代码生成

## 1. 目标

模块 14 将模块 11-13 的静态模型合并为唯一 `modules/modules_gen.go` 的内存源码，并在 `core/module` 定义最小、不可变的静态图契约。生成文件提供已验证的模块描述、Descriptor 及其 Provider、普通 Provider、依赖边和稳定拓扑顺序。

`module.Graph` 是静态、不可执行的蓝图，不保存 `Factory any`，不通过反射检查或调用构造器，不按字符串查找组件，也不在运行期完成依赖注入。模块 40 在生命周期和 Application Graph 契约冻结后，再扩展生成器产出直接、强类型的实例装配代码。

模块 14 不写文件、不执行 `ModuleConfig`、不解码配置、不调用构造器、不连接数据库，也不实现尚未冻结的 Host、HTTP、生命周期、gRPC、Outbox、Event、Schedule 或 Queue 契约。模块 15 负责格式化、类型检查和原子写入。

## 2. 最小静态图契约

`core/module` 提供受控的 Graph 构建输入和只读访问器。Graph 内部状态私有，所有集合访问器返回防御性副本。构建器只校验静态集合的完整性、唯一性、依赖参数顺序和拓扑一致性，不执行任何业务函数。

组件身份固定为 `(module, packagePath, constructorName)`。同一模块的不同包可以合法声明同名构造器；三元组完全相同才是重复组件。Provider 元数据至少包含类别、模块、包路径、名称和类型文本。Dependency 元数据关联消费组件、提供者与构造器参数序号；参数序号必须从 `0` 连续、无重复，生成和读取均保持原始参数顺序。组件顺序是已验证的拓扑结果，所有组件 Provider 必须先于其 Consumer。

模块静态描述包含 Identity、展示名称、说明、Order、原始默认配置工厂和已声明的 Middleware 引用。配置合并、环境覆盖、`gvalid` 与 `module.Compile` 仍由后续 Application Host 按统一 Source 执行。默认配置工厂是模块配置边界，不是组件 DI 工厂；Graph 构建期不调用它。

Descriptor 以 `entity.Metadata` 登记，并关联其类型化 Descriptor Provider 元数据，使普通构造器的 Descriptor 依赖与其他 Provider 使用同一依赖模型。Graph 不以 `any` 保存泛型 Descriptor 构造函数。

## 3. 生成文件

渲染入口只接受已完成的 `Model`、Provider `Graph` 与 `DescriptorSet`，并返回内存字节。输出固定为 `package modules`，带 `// Code generated ... DO NOT EDIT.` 标记和单一 `Generated() module.Graph` 入口。

生成器按模块 Identity、实体包路径/名称和 Provider 拓扑顺序输出。导入按包路径排序并分配稳定、无冲突别名。业务实体包常用 `entity` 名称，因此核心 `entity` 包使用固定可区分别名，不依赖默认包名。私有 DO、强类型 Descriptor helper、模块描述、Provider、Dependency 和拓扑元数据合并到唯一文件，不向业务模块目录写入分散生成文件。

每个普通构造器生成一个强类型 `build...` 包装函数。包装函数按原构造器参数顺序直接调用其符号：返回 `(*T, error)` 时保留原错误返回，只返回 `*T` 时包装为空错误。该函数只用于让 Go 编译器证明构造器符号、参数和返回值仍与分析结果一致，不登记到 Graph，也不在 `Generated()` 中执行。

Descriptor helper 保留真实泛型签名 `entity.Descriptor[E, uint64]`，并由 DescriptorDefinition 登记其 Metadata 与 Provider 元数据。构造器对 Descriptor 的强类型依赖必须通过生成文件编译。

## 4. 错误与边界

输入为 nil、Provider 图与源码模型不一致、Descriptor 归属未知模块、组件完整身份重复、生成标识符/导入别名冲突、依赖 Provider 不存在、参数序号不连续或拓扑顺序与依赖边冲突时，渲染器或 Graph 构建器返回稳定诊断。生成过程不调用任意业务函数。

Graph 不保存可执行组件函数，因此不承诺在模块 14 传播构造器运行错误。构造器实际执行、实例保存、启动失败回滚和生命周期管理属于模块 40 及后续 Host 模块。运行期配置、数据库、路由和生命周期错误不提前伪造为模块 14 校验规则。模块 15 在完成格式化和类型检查前不得替换旧 `modules/modules_gen.go`。

## 5. 验收

单元测试覆盖空图、单/多模块、Descriptor 及其 Provider 登记、三类 Provider、依赖参数顺序、拓扑先后关系、跨包同名构造器、完整身份冲突、带/不带 error 的强类型包装函数、导入别名冲突、稳定输出和 Graph 不可变访问。测试必须证明 Graph 没有 `Factory any`、不依赖反射，且渲染不执行 `ModuleConfig`、Schema 或构造器。

端到端测试在临时 Go workspace 中完整执行：

```text
Analyze
-> CompileDescriptors
-> BuildGraphWithDescriptors
-> Render
-> 写入临时 modules/modules_gen.go
-> go test -mod=mod ./...
```

测试模块同时包含强类型 Config、Entity/Schema、Descriptor 依赖、普通组件依赖、不同包的同名构造器和返回 error 的构造器。生成包必须通过真实 Go 编译，不仅是 AST 语法解析。

生成内容只覆盖已冻结的模块、实体和普通 Provider，禁止出现运行期目录扫描、字符串组件定位、反射 DI 或未批准的 Transport/生命周期注册。

门禁为 `go test ./cool-next/core/module ./cool-next/codegen -count=1`、两个包的竞态测试、`go test ./... -count=1`、`go vet ./...`、`make check` 与 `git diff --check`。
