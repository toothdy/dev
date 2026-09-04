# 13 Descriptor And DO Code Generation Implementation Plan

**Goal:** 将源码实体和 Schema 编译为确定性的 Descriptor/私有 DO 源码片段与类型化 Provider 候选，不写入业务模块或数据库。

**Architecture:** 复用模块 11 的实体与 Schema 类型/位置模型，在 codegen 中完成生成前元数据校验；产物只描述对 `entity.Compile[E, uint64](schema)` 的静态调用和 GoFrame DO struct，模块 14 再统一拼装生成文件。

**Tech Stack:** Go 1.26、标准库 `go/ast` / `go/types` / `go/format`、GoFrame `g.Meta`、模块 04/05 entity、模块 11/12 codegen、Go `testing`

## File Structure

- Create: `cool-next/codegen/descriptor.go` - 实体/Schema 编译入口与不可变片段模型
- Create: `cool-next/codegen/entity_validate.go` - 元数据、标签、表名、列名、索引冲突校验
- Create: `cool-next/codegen/do_emit.go` - 私有 GoFrame DO struct 与 Descriptor 调用片段渲染
- Create: `cool-next/codegen/descriptor_test.go` - 临时模块实体的正反例、DO 形状与稳定性测试
- Modify: `cool-next/codegen/model.go` - 仅增加同包 Descriptor 编译所需的受控内部实体/Schema 类型信息
- Modify: `cool-next/codegen/entity.go` - 保留实体和 Schema 的类型对象、函数对象与字段位置

### Task 1: Descriptor Fragment Model And Entity Metadata

- [ ] 写测试，覆盖单实体、无 Schema、重复 Schema 及不可变访问器。
- [ ] 扩展模块 11 的内部模型，保留实体类型、Schema 函数、包路径和字段位置，不改变公开访问器契约。
- [ ] 定义 DescriptorFragment、EntityProviderCandidate 与确定性排序。
- [ ] 为每个实体建立对应 Schema 查找，不执行函数体或数据库操作。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestCompileDescriptors(Model|Schema)' -count=1`。

### Task 2: Static Metadata Validation

- [ ] 写失败测试，覆盖 g.Meta table/description、Base、json/orm/description、lowerCamel 列名、未知 cool 属性、重复表与物理索引。
- [ ] 从 `go/types` 与 AST 标签解析实体字段和 `cool` 属性，只接受 size/default/precision/scale。
- [ ] 复用 entity 的字段/索引语义，产生带字段/声明位置的稳定 DiagnosticError。
- [ ] 保持方言长度、SQL 转义和数据库能力在 db/driver 边界，不在此处扩展。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestCompileDescriptors(Validates|Rejects)' -count=1`。

### Task 3: Static DO And Descriptor Emission

- [ ] 写 Golden 测试，验证 g.Meta DO 标记、导出 any 字段、Descriptor 泛型调用和稳定 import/声明顺序。
- [ ] 生成每实体私有 DO struct 与 Descriptor 构造片段，不生成 DAO、Columns 或业务模块文件。
- [ ] 将 Descriptor 类型作为 Provider 候选交给后续图扩展，但不在模块 13 修改构造顺序或生成注册表。
- [ ] 确保片段经 `go/format` 格式化后重复生成字节一致。
- [ ] 运行 `go test ./cool-next/codegen -run 'TestCompileDescriptors(Emits|Deterministic)' -count=1`。

### Task 4: Full Verification

- [ ] 增加并发读取 Fragment 测试，确认不执行 Schema/ModuleConfig、不连接数据库、不写文件。
- [ ] 运行 `gofmt -w cool-next/codegen/*.go`。
- [ ] 运行 `go test ./cool-next/codegen -count=1`、`go test -race ./cool-next/codegen -count=1`、`go test ./... -count=1`。
- [ ] 运行 `go vet ./...`、`make check`、`git diff --check`，避免纳入 `module.go` 的既有外部修改。
