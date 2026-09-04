# 模块 13：Descriptor 与 DO 代码生成设计

> 日期：2026-08-05
> 状态：待复核
> 模块：13 Descriptor 与 DO 代码生成
> 对应拆分项：13.1-13.11
> 前置模块：05 DO 值与字段状态、11 源码分析、12 Provider 图

## 1. 目标

模块 13 将模块 11 的实体/Schema 源码模型编译为静态 Descriptor 与私有 GoFrame DO 适配器片段，供模块 14 汇入唯一 `modules/modules_gen.go`。它复用 `core/entity.Compile`、`Descriptor` 与 `DOValue` 的既有运行期契约，不再复制字段类型映射、四态 DO 值或 DDL 规则。

本模块只在内存中产出确定性 Go 源码片段和类型化 Descriptor Provider 信息；不写文件、不执行数据库操作、不注册组件或构造业务 Service。

## 2. 实体编译

每个模块 11 发现的实体必须有同目录、同名 `XxxSchema()`。生成器静态校验实体 `g.Meta` 的 table 与 description、业务字段的 json/orm/description/cool 标签、Base 三字段、Schema 索引和跨实体物理冲突。规则与 `core/entity.Compile` 的公开契约一致，错误使用源码位置诊断。

`cool` 仅接受 `size`、`default`、`precision`、`scale`；列名必须 lowerCamelCase，表名与索引名允许既有 snake_case。表名、物理索引名和字段声明冲突在生成前失败。方言长度和 DDL 能力仍由 `db/driver` 处理，不在 codegen 重做 SQL 编译。

## 3. 生成片段与 Provider

每个实体生成一个私有 DO struct 和一个静态 Descriptor 构造表达式，DO 采用 GoFrame 标准形状：嵌入 `g.Meta`，业务字段为导出的 `any`，并带 `orm:"table:<table>,do:true"`。Descriptor 继续由 `entity.Compile[E, uint64](schema)` 构造，生成代码只负责提供实体类型和已声明 Schema。

片段不会写入实体目录，也不生成 DAO、Columns 或每实体文件。模块 13 把 `Descriptor[E, uint64]` 作为后续模块可登记的类型化 Provider 候选；Base Service Provider 留待模块 23。

## 4. 确定性与边界

实体按模块 Identity、包路径和实体名称排序；字段按源码声明顺序；索引按声明顺序并以名称打破并列。相同输入的片段字节等价。模块 14 负责把片段与模块图合并为唯一生成文件，模块 15 负责格式化、类型检查与原子写入。

## 5. 验收

测试覆盖实体/Schema 配对、字段和 cool 标签错误、表/索引冲突、DO 形状、Descriptor Provider 类型、确定性及无数据库生成。门禁为 `go test ./cool-next/codegen -count=1`、竞态测试、`go vet ./...` 与 `make check`。
