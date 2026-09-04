# Cool Next 命名精简设计

## 目标

精简 `cool-next` 全部生产 Go 文件中过长、重复上下文和过度描述的函数名、receiver 与局部变量名，使名称依赖包和类型上下文表达语义，同时保持现有行为、协议和生成语义不变。

## 范围

- 处理 `cool-next` 下非测试 Go 文件。
- 同步测试文件中因生产符号改名而必须调整的调用点，不优化测试自身命名。
- 同步模块、控制器和生成器模板中的生产调用点。
- 不手工编辑标记为生成产物的文件；需要变化时修改生成源并重新生成或验证。

## 命名规则

| 类别 | 规则 | 示例 |
| --- | --- | --- |
| 包内函数 | 删除包名、类型名等重复上下文 | `validateModuleDirectories` → `checkDirs` |
| 校验函数 | `validate*` 优先改为 `check*` | `validateGeneratedIdentifiers` → `checkNames` |
| 转换函数 | 使用对象名或简短的 `to*` 动词 | `normalizeRequestPlanValue` → `planValue` |
| 生成器函数 | 保留 `write`、`render` 等动作，缩短宾语 | `writeCompiledDescriptorDeclaration` → `writeDescriptor` |
| receiver | 使用 `s`、`r`、`g`、`c`、`t` 等惯用短名 | `transport` → `t` |
| 局部变量 | 使用 `req`、`cfg`、`def`、`pos`、`deps` 等上下文明确的短名 | `candidateIDs` → `ids` |
| 公共 API | 只删除包上下文已经表达的词 | `NewUnaryContextInterceptor` → `NewUnaryInterceptor` |
| 协议名称 | 保持稳定 | JSON 字段、错误码、数据库列名和配置键不改 |

## 保留边界

- 不按字符数强制截断名称。
- 不使用无法从上下文判断含义的单字母名称；receiver、循环索引和惯用缩写除外。
- 不删除安全、事务、并发、数据库和外部输入边界校验。
- 不改变导出类型的职责、函数签名参数和返回值，仅在确认全部调用点可同步时修改导出名称。
- 不为旧私有名称保留转发函数。
- 不新增依赖或辅助抽象。

## 实施顺序

1. `auth`、`core`、`crud`、`db` 等运行时基础包。
2. `grpc`、`outbox`、`seed`、`eps` 等功能包。
3. `codegen`，包括分析、图构建、校验和渲染代码。
4. 同步模块生产调用点以及测试中的必要引用。

每次完整处理一个包，再执行格式化和该包测试，避免跨包改名长期处于不可编译状态。

## 验证

```bash
go test ./cool-next/... -count=1
go test ./modules/base/... -count=1
go test ./... -count=1
go vet ./...
git diff --check
```

全仓测试若存在与工作区原有修改相关的失败，需要记录失败包、错误和与本次命名改动的关系。
