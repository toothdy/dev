# cool-admin-go-next Go Plugin 系统设计

- 日期：2026-07-28
- 项目：`cool-admin-go-next`
- 配套仓库：`cool-admin-go-plugin`
- 配套 ABI：`docs/superpowers/specs/2026-07-29-cool-plugin-v1-abi-design.md`
- 对照：`cool-admin-midway/.cursor/rules/module.mdc`、`cool-admin-midway/src/modules/plugin`
- 状态：总体设计与优化决策已批准，已完成规范自检，等待用户书面审核

## 1. 背景

`cool-admin-go-next` 需要支持后台上传插件、立即安装并在不重启主应用的情况下调用。现有 Node 版插件将 JavaScript 代码从数据库或用户数据目录加载后执行，调用方既可以使用 `invoke(key, method, ...params)`，也可以通过 `getInstance(key)` 获得活的 Node 对象并继续调用对象方法。

Go 主程序不能安全热加载普通 Go 动态库，也不能跨平台稳定替换已加载的原生代码。本设计选择 Extism 作为插件调用模型，使用 wazero 在 Go 进程内执行 WASM，插件使用 Go/TinyGo 开发。系统不兼容 Node `.cool` JavaScript 插件，也不复制 Node 的任意对象返回和 `eval` 行为。

插件配置继续由后台维护。系统采用受信任管理员插件模型：插件包不做开发者签名，配置以明文 JSON 保存，插件可以通过受控 Host HTTP 访问公网、内网、localhost 和云元数据地址。

## 2. 目标

1. 后台上传单个插件文件后立即完成校验、预编译、初始化和热切换。
2. 主应用不重启，不启动插件子进程，也不要求插件独立部署。
3. 插件使用 Go/TinyGo 开发，并通过稳定、版本化的 WASM ABI 与宿主通信。
4. 普通业务代码使用 CLI 生成的强类型 Client，不手写 JSON、插件方法字符串或响应断言。
5. 未知插件和插件间调用仍可使用受控的动态 `Invoke` 协议。
6. 插件实例、内存、并发、调用时间、输出和网络响应均有固定上限。
7. 升级失败时保持当前版本继续运行；成功切换后安全排空旧实例。
8. 目录和代码职责遵守现有 `cool/**` 框架层与 `modules/**` 业务模块边界。

## 3. 非目标

- 不兼容 Node `.cool` 包、JavaScript ABI、`getInstance()` 或插件对象链式返回。
- 不使用 Go 原生 `plugin` 包、CGO 动态库或运行时重新编译主应用。
- 不支持插件独立进程、Sidecar、容器或远程插件服务。
- 不支持多应用实例，也不使用 Redis 同步安装、配置或运行状态。
- 不提供插件历史版本列表或后台一键回滚。
- 不支持租户级插件启停或配置覆盖；插件和配置均为平台全局。
- 不开放通用 HTTP 插件调用端点。
- 不向插件开放数据库、系统命令、宿主环境变量或宿主文件系统。
- 不承诺阻止受信任管理员安装的恶意插件发起 SSRF、内网扫描或配置外传。

## 4. 核心决策

### 4.1 Extism + wazero

宿主使用 Extism 的插件模型和 wazero Runtime。Extism 提供稳定的字节输入输出和 Host Function 边界，wazero 保持纯 Go、跨平台和进程内运行，不依赖系统动态链接器。

Extism、wazero 和 TinyGo 在实施计划中固定具体版本，依赖升级必须先通过 ABI Golden Fixture、取消、内存和实例生命周期回归测试。`cool-plugin-v1` 的规范性线协议由配套 ABI 文档定义。

WASM 负责内存隔离，但不单独提供业务权限。所有宿主能力必须通过显式 Host Function 暴露；未暴露的数据库、命令、环境和文件能力默认不可用。

WASI 实例不预开放目录、不注入宿主环境变量和命令行参数，也不开放直接网络 Socket。插件可以使用受限的时钟和随机数能力；所有网络访问必须经过 `host.http`。

### 4.2 无状态调用边界

跨 WASM 边界只传递一次调用所需的数据，不返回可继续调用的宿主对象、插件对象或 Handle。Node 的：

```text
getInstance("wx") -> OfficialAccount() -> getAccessToken()
```

在 Go 插件中收敛为一个声明的方法：

```text
officialAccount.getAccessToken
```

该约束使任意空闲实例都可以处理调用，避免 Handle 与具体实例绑定造成的泄漏、过期、并发和热升级问题。

### 4.3 强类型 Client，JSON 仅作为 ABI

插件开发者以 Go 请求类型、响应类型和 Service 契约作为唯一源定义。CLI 解析 Go AST 并生成：

- 插件端方法分发代码。
- 宿主端强类型 Client。
- 请求与响应 JSON 编解码代码。
- 参数校验代码。
- `plugin.json` 方法清单、内嵌 Schema 和契约哈希。

业务代码调用生成的 Client，例如：

```go
wxClient := wxplugin.NewClient(pluginService)
token, err := wxClient.OfficialAccount().GetAccessToken(ctx)
```

`OfficialAccount()` 是编译进宿主的本地 Client 门面，不是 WASM 返回的远程对象。Client 内部调用带预期 ABI、Manifest Schema Version 和契约哈希的 `InvokeContract`，运行时负责结构体与 JSON 字节之间的转换。

强类型 Client 必须在宿主构建期作为普通 Go 依赖引入。后台热安装不会把新的 Go Client 动态注入已经运行的主程序。宿主未编译对应 Client 时，内部代码只能使用动态 `Invoke`；这不影响插件安装和运行。

### 4.4 单实例与本地事实状态

系统只考虑一个应用实例。MySQL 保存插件元数据、平台配置、文件哈希和当前路径，`modules/plugin/plugins` 保存当前插件文件。运行中的实例池、熔断器和编译缓存只存在于进程内，不通过 Redis 或其他节点协调。

## 5. 仓库与目录边界

### 5.1 `cool-admin-go-next`

框架级运行能力位于 `cool/plugin`：

```text
cool/plugin/
├── api.go
├── runtime.go
├── pool.go
├── host.go
└── manifest.go
```

职责如下：

- `api.go`：共享 `contract.Invoker` 的 Runtime 实现、原始请求响应和调用错误；不得在主应用仓库重新定义生成 Client 依赖的接口。
- `runtime.go`：插件代际、注册表、预编译、热切换、熔断和生命周期。
- `pool.go`：有界实例池、获取、归还、Trap 淘汰、补池和排空。
- `host.go`：`host.log`、`host.http`、`host.invoke`、`host.context`。
- `manifest.go`：Manifest、方法、Schema、ABI 和哈希校验。

业务模块严格按模块角色组织：

```text
modules/plugin/
├── controller/
│   ├── controllers.go
│   └── admin/
│       └── info.go
├── entity/
│   ├── info.go
│   └── models.go
├── service/
│   ├── info.go
│   ├── installer.go
│   └── repository.go
├── plugins/
│   ├── staging/
│   └── active/
├── config.go
├── register.go
└── menu.json
```

模块职责如下：

- Controller 只处理后台协议、权限、上传绑定和响应。
- Info Service 负责插件查询、配置、启停和删除业务。
- Installer 负责包校验、落盘、预热和安装事务。
- Repository 封装 MySQL 操作，不向 `cool/plugin` 泄漏 Entity。
- Entity 只定义 `plugin_info` 模型和模型注册。
- Register 构建唯一 Runtime，将同一个 Runtime/Service 注入 Controller。
- `plugins` 是运行数据目录，不参与 Go 编译，也不作为静态 HTTP 目录暴露。

`modules/plugin/plugins` 的运行内容必须加入 `.gitignore`，不能提交安装包、WASM、配置或 staging 文件。目录由 Plugin Runtime 启动时创建。

`hooks` 只在未来确实需要“随主程序编译的内置 Hook”时增加，不与后台热安装插件混用。

### 5.2 `cool-admin-go-plugin`

独立仓库负责插件开发工具链：

```text
cool-admin-go-plugin/
├── cmd/cool-plugin/
├── sdk/
├── contract/
│   ├── invoke.go
│   └── abi/v1/
├── internal/generator/
├── internal/packager/
├── templates/basic/
├── testhost/
└── examples/
```

该仓库提供：

- TinyGo 兼容 SDK 和四个 ABI 入口包装。
- `contract/invoke.go` 定义最小 `Invoker` 和 `ContractExpectation`；生成 Client 只依赖该包和自身契约类型，不能导入 `cool-admin-go-next`。
- 宿主、SDK 和 testhost 共用的版本化 ABI 类型、严格编解码器和 Golden Fixture；共享契约包不得依赖 GoFrame、数据库或 `cool-admin-go-next`。
- Go 契约解析、Dispatcher、Client、Schema 和契约哈希生成。
- `init`、`generate`、`test`、`build`、`pack` 命令。
- 与正式宿主使用同一 ABI 和 Host Function 语义的本地测试宿主。
- 最小插件模板和可验收示例。

## 6. 插件包

构建结果是一个可上传文件：

```text
<key>-<version>.coolwasm
```

它是 ZIP，第一版固定包含：

```text
plugin.json
plugin.wasm
config.default.json
README.md
assets/logo.png
checksums.json
```

规则如下：

1. 所有路径必须使用 ZIP 根目录相对路径。
2. `checksums.json` 保存除自身外每个文件的 SHA-256。
3. 不允许额外可执行文件、符号链接、硬链接、绝对路径或归一化后重复路径。
4. `config.default.json` 必须是 JSON Object；空配置使用 `{}`。
5. README 和 Logo 只用于后台展示，不会映射为公共静态资源。README 必须按不可信 Markdown 处理：渲染时禁用原始 HTML，或使用不允许脚本、事件属性和危险 URL Scheme 的白名单清洗器处理结果 HTML。
6. 上传包安装成功或失败后均从临时上传位置删除；已安装目录保存解包后的固定文件。
7. `plugin.json` 最多 2 MiB，`config.default.json` 和 `checksums.json` 各最多 1 MiB，README 最多 2 MiB。
8. Logo 只接受有效 PNG，原文件最多 256 KiB，像素宽高均不得超过 1024；安装时编码为 Base64 保存到数据库。
9. Manifest 和配置 JSON 必须是有效 UTF-8，拒绝重复 Key、尾随 Token 和当前 Schema 未声明的字段。

`plugin.json` 至少包含：

```json
{
  "schemaVersion": 1,
  "key": "wx",
  "name": "微信插件",
  "description": "微信服务端能力",
  "version": "1.0.0",
  "author": "cool-team",
  "abi": "cool-plugin-v1",
  "contractHash": "sha256:<hex>",
  "configSchema": {},
  "methods": [
    {
      "name": "officialAccount.getAccessToken",
      "requestSchema": {},
      "responseSchema": {}
    }
  ]
}
```

约束如下：

- `key` 长度为 2 至 64，并匹配 `^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`。
- `name` 和 `author` 各最多 255 个 Unicode 字符，`description` 编码后最多 4096 字节。
- `version` 必须是规范 SemVer，编码后最多 255 字节。
- `schemaVersion` 和 `abi` 必须由宿主明确支持，未知版本直接拒绝。
- `methods` 最多包含 1024 个方法。
- 方法名必须唯一并匹配 `^[a-z][A-Za-z0-9]*(?:\.[a-z][A-Za-z0-9]*)*$`，不允许保留 ABI 名或动态通配符。
- 方法的请求和响应 Schema 由 CLI 从 Go 契约生成，不能在打包阶段独立手改。
- `configSchema` 由配置 Go Struct 生成；没有配置时固定为只接受空 Object 的 Schema。
- `contractHash` 对 ABI、Manifest Schema Version、规范化后的方法名、请求 Schema 和响应 Schema 计算，不包含 README、Logo、插件版本、默认配置和业务错误 Details。

第一版契约生成只接受确定的 JSON 数据类型：布尔值、字符串、各宽度整数和浮点数、这些类型组成的 Struct、Slice、Array、`map[string]T`，以及用于表达可选字段的 Pointer。每个方法的顶层请求和响应必须是命名 Struct；空请求和响应使用空 Struct。Struct 字段必须导出并有明确 JSON Tag，不允许匿名嵌入；`omitempty` 只允许用于 Pointer，Pointer 表示字段可省略但存在时不得为 JSON `null`。Slice 和 Map 的 nil 值由生成编解码器规范化为 `[]` 和 `{}`。拒绝 `interface{}`、`any`、函数、Channel、复数、非字符串 Map Key、递归类型和带自定义 Marshal 行为的类型。时间、金额、UUID 和枚举必须由插件契约显式定义为字符串或整数表示，不能依赖宿主 Go 类型的私有编码。

Schema 方言固定为 JSON Schema Draft 2020-12。契约哈希输入按方法名排序，并使用 RFC 8785 JSON Canonicalization Scheme 规范化；哈希文本固定为 `sha256:` 加 64 位小写十六进制。CLI、测试宿主和正式 Runtime 必须使用共享 ABI Contract 中的同一规范化实现。

JSON Schema 只在生成、打包和安装阶段使用。调用期不运行 JSON Schema 引擎；Runtime 仍执行大小限制、严格信封解析、方法存在性、Client 兼容性和目标 Go 类型解码。动态 `Invoke` 的调用方承担业务 Payload 语义不匹配的风险。

## 7. ABI 与调用协议

本节只描述宿主调用模型；四个入口、四个 Host Function、JSON 信封、错误分类、HTTP 二进制帧和 Golden Fixture 的规范性定义见 `2026-07-29-cool-plugin-v1-abi-design.md`。共享参考实现位于 `cool-admin-go-plugin/contract/abi/v1`。

### 7.1 稳定入口

每个插件固定导出四个 Extism 入口：

- `init`
- `validate_config`
- `invoke`
- `health`

入口均使用 UTF-8 JSON 字节作为输入输出。空请求使用 `{}`，不使用空指针或语言私有编码。固定信封拒绝重复 Key、尾随 Token 和未知字段。

### 7.2 初始化和配置

`validate_config` 接收候选平台配置并返回成功或声明式业务错误。安装、升级、启用和配置更新都必须先调用它。

`init` 在每个新实例进入池前调用，输入包含已校验的配置快照、插件 Key、插件版本、运行代际和宿主 ABI。实例初始化失败时不得进入可用池。

配置更新不会原地修改正在使用的实例。Runtime 使用同一 WASM 编译结果和新配置构建新代际，全部实例初始化和健康检查成功后原子切换，再排空旧代际。

### 7.3 方法调用

底层提供两个调用入口：

```text
Invoke(ctx, pluginKey, method, request, response) error
InvokeContract(ctx, pluginKey, method, expectation, request, response) error
```

`Invoker` 接口和 `ContractExpectation` 由 `cool-admin-go-plugin/contract/invoke.go` 定义；`cool/plugin` 的 Runtime 实现该接口。`expectation` 是包含 `abi`、`schemaVersion` 和 `contractHash` 的不可变结构。生成 Client 固定使用 `InvokeContract`；未知插件和框架调试代码使用 `Invoke`。两者共用同一执行路径，区别仅是前者在取得实例前要求当前 Manifest 的三项兼容标识与 Client 完全一致。

调用流程：

1. 根据 Key 原子读取当前启用代际。
2. 确认方法存在于 Manifest，并在生成 Client 提供预期契约时校验 ABI、Manifest Schema Version 和契约哈希。
3. 将请求编码为 JSON，构造 `method + payload` 调用信封。
4. 在调用 Context 截止时间内从实例池取得实例。
5. 调用 WASM `invoke`。
6. 校验输出大小和响应信封，将 `data` 严格解码到强类型响应；调用期不执行 JSON Schema 校验。
7. 按结果归还实例、淘汰故障实例或更新熔断器。

成功响应为：

```json
{
  "ok": true,
  "data": {}
}
```

插件业务错误为：

```json
{
  "ok": false,
  "error": {
    "kind": "business",
    "code": "WX_TOKEN_REJECTED",
    "message": "获取 Access Token 失败",
    "details": {}
  }
}
```

`code` 和 `message` 必须是可公开的业务信息；`details` 可省略，存在时必须是最大 64 KiB 的 JSON Object，不进入契约哈希。插件只能主动返回 `kind: business`；Trap、非法 JSON、伪造系统错误、ABI 错误、路径和配置内容由宿主转换为系统错误，不透传内部诊断。

### 7.4 强类型与动态调用边界

- 已知插件的业务模块必须使用生成 Client。
- 生成 Client 固化插件 Key、方法名、请求类型、响应类型、预期 ABI、Manifest Schema Version 和契约哈希。
- Runtime 仍在每次调用前检查当前安装插件是否声明该方法。
- 未知插件或框架工具可直接使用动态 `Invoke`，但必须提供可编码请求和明确响应目标。
- 第一版不向普通业务公开 Raw/Binary 方法；大文件通过 URL、存储 Key 或 Host HTTP 传递，不嵌入 JSON Base64。

## 8. Host Functions

### 8.1 `host.log`

插件可以写 Debug、Info、Warn、Error 日志。宿主自动附加插件 Key、版本、代际、方法、调用 ID 和 Trace ID。完整请求最多 64 KiB：SDK 先校验字段预算，字段超限直接拒绝；仅消息超限时在 UTF-8 字符边界截断并追加 ASCII `...`。宿主对仍超限的请求直接返回稳定错误，不做第二次截断。配置和调用负载不会自动进入日志。

ABI 逻辑名 `host.log` 对应 WASM 导入 `cool_log`。其他 Host Function 使用相同规则：插件业务代码只调用 SDK，不能直接操作 Extism PTR 或手写协议帧。

### 8.2 `host.http`

插件可以发起 HTTP/HTTPS 请求。第一版按已确认的受信任管理员模型允许访问：

- 公网地址。
- 内网地址。
- localhost。
- 云元数据地址。

SDK 使用 ABI 文档定义的 `CPH1` 长度前缀二进制帧，将 Method、URL、Headers JSON 和原始 Body 放在同一 Extism 内存块中，避免 Base64 放大。帧固定使用 16 字节头、无符号 32 位大端长度和严格的总长度校验。插件开发者只使用 SDK 的强类型 HTTP Client，不手写该帧。

宿主不承诺 SSRF 防护。稳定性边界仍强制执行：

- 单次请求最长 30 秒，并受上层插件调用剩余时间约束。
- 请求正文最多 8 MiB，请求头编码后最多 64 KiB。
- 响应正文和响应头分别最多 8 MiB、64 KiB；压缩响应按解压后正文计算。
- 重定向次数最多 5 次。
- Context 取消后立即停止读取响应并关闭 Body。

### 8.3 `host.invoke`

插件可以调用另一个已启用插件。SDK 对插件开发者暴露与宿主 Client 相同的强类型包装，底层传递目标 Key、方法和 JSON 负载。

规则如下：

- 根调用深度为 0，子调用深度等于父调用深度加 1；第一版默认最大深度为 8，子调用深度大于该值时拒绝。
- 调用链中出现重复插件 Key 即判定循环并拒绝，包括 `A -> B -> A`。
- Runtime 必须在取得目标插件实例前完成重复 Key 和深度检查。
- 子调用继承父调用的截止时间、调用 ID 链和取消信号。
- 子调用使用目标插件自己的实例池、熔断器和方法声明。
- 插件声明的业务错误原样返回调用插件，不计入系统熔断。

### 8.4 `host.context`

插件只能读取固定白名单调用元数据：调用 ID、Trace ID、当前插件 Key、方法、调用方插件 Key、调用深度和截止时间。它不提供原始 HTTP Request、Cookie、Authorization、进程环境变量或数据库连接。

需要处理 HTTP 回调的插件由业务模块将 Method、URL、允许的 Headers 和 Body 转换成明确 DTO 后调用插件；插件返回 Status、Headers 和 Body DTO。不得将 GoFrame Request、Response 或其他 Go 对象跨 WASM 边界传递。

## 9. Runtime、实例池与并发

### 9.1 插件代际

每个启用插件在注册表中对应一个不可变代际，包含：

- Manifest 和方法索引。
- WASM 文件哈希与预编译模块。
- 配置快照与配置哈希。
- 有界实例池。
- 熔断器。
- 接单状态、在途调用计数和排空 Context。

Registry 使用原子指针切换整个代际。调用热路径不读取 MySQL、不读取文件、不重新解析 Manifest，也不重新编译 WASM。

预编译缓存按 `wasmHash + abi + runtimeProfileHash` 键控并使用引用计数，避免未来 ABI 或 Runtime 配置变化时错误复用。`runtimeProfileHash` 至少覆盖内存 Page 上限、WASI 能力、Host Import 集合和 wazero 编译特性。代际创建时增加引用，代际排空关闭后减少引用；引用归零即关闭并移除预编译模块。应用关闭时关闭剩余缓存，防止持续升级造成编译缓存增长。

### 9.2 默认限制

第一版默认值：

| 限制 | 默认值 |
| --- | --- |
| 每插件实例数 | 4 |
| 单次调用总超时 | 30 秒 |
| 每实例线性内存 | 64 MiB |
| 单次 WASM 输入 | 8 MiB |
| 单次 WASM 输出 | 8 MiB |
| Host HTTP 超时 | 30 秒且不超过调用剩余时间 |
| Host HTTP 请求正文 | 8 MiB |
| Host HTTP 请求头 | 64 KiB |
| Host HTTP 响应头 | 64 KiB |
| Host HTTP 响应 | 8 MiB |
| 单条插件日志 | 64 KiB |
| 插件调用深度 | 8 |
| 旧代际排空时间 | 30 秒 |
| 熔断阈值 | 30 秒内 5 次系统故障 |
| 熔断时间 | 30 秒 |

所有等待都必须响应 Context，不创建无界 goroutine 或无界排队。调用超时包含等待实例、WASM 执行、Host Function 和子插件调用的全部时间。根调用使用调用方 Deadline 与配置总超时中较早者。

Runtime 必须显式启用 wazero 的 Context 取消终止能力，并将 64 MiB 严格配置为 1024 个 64 KiB WASM Page，不能只在业务配置中记录限制。非 64 KiB 整数倍的自定义内存配置在启动时拒绝。

上述边界只约束单插件、单实例和单调用。第一版按已确认决策不限制全局启用插件数和总实例数；进程总资源由部署容量和受信任管理员控制，系统只提供总实例、编译缓存、调用、内存和故障指标及告警，不能宣称进程总资源固定有界。

### 9.3 实例状态

- 正常成功或插件业务错误后，实例归还原池。
- Trap、非法 ABI 输出、实例内存错误或无法恢复的 Host Function 协议错误后，实例立即关闭并异步补充。
- 调用超时或取消后，无论 Extism/wazero 返回何种可复用状态，实例一律关闭并异步补池。
- 旧代际开始排空后拒绝新调用，正在执行的调用最多等待 30 秒，随后取消并关闭全部实例。
- 应用关闭时先从全局入口拒绝新 Invoke，再按相同规则排空所有插件。

### 9.4 熔断

Trap、非法 JSON、系统超时、补池失败和 Host Function 系统错误计入熔断；插件声明的业务错误不计入。

同一插件在滚动 30 秒内达到 5 次系统故障后打开熔断 30 秒。熔断到期只允许一个探测调用；成功关闭熔断，系统失败重新打开。调用方取消且插件未开始执行不计入插件故障。

## 10. 安装目录与文件生命周期

最终目录固定为：

```text
modules/plugin/plugins/
├── staging/
│   └── <install-id>/
│       ├── package.coolwasm
│       └── content/
└── active/
    └── <key>/
        └── <sha256>/
            ├── plugin.wasm
            ├── plugin.json
            ├── config.default.json
            ├── README.md
            ├── assets/logo.png
            └── checksums.json
```

其中 `<sha256>` 是完整 `.coolwasm` 上传包的 SHA-256。目录从应用项目根解析，必须是普通本地目录且对当前进程可写。它不注册为静态资源目录。

文件生命周期：

1. 上传流写入新的随机 `staging/<install-id>/package.coolwasm`，不直接使用客户端文件名作为路径。
2. 校验和解包只在 `staging/<install-id>/content` 内完成。
3. 预编译和候选实例池从 content 内容构建。
4. 候选通过后，将 content 原子重命名为 `active/<key>/<sha256>`，再删除 staging 外层目录和原始上传包。
5. 数据库提交新元数据和当前路径后，Registry 原子切换候选代际。
6. 旧代际排空后删除旧哈希目录，不保留历史回滚版本。
7. 安装任一步失败时关闭候选资源、删除 staging，并继续使用旧代际和旧数据库记录。

确定性目标目录已经存在时必须先对账，不能直接覆盖：

- 目录完整且包哈希、Manifest 和全部 Checksum 一致时安全复用；它可能是上次在数据库提交前崩溃留下的有效目录。
- 目录损坏且未被数据库引用时作为孤儿清理，再重新原子移动候选目录。
- 目录损坏且正在被数据库引用时返回稳定的安装状态冲突错误，不原地覆盖；管理员必须先删除记录和目录，再重新安装。

相同包哈希只有在数据库记录和 active 目录均重新验证通过后才算幂等成功。数据库提交失败时关闭候选并尽力删除新 active 目录；删除失败留下的目录由启动恢复按孤儿处理。

启动恢复：

- 清理创建超过 1 小时且无进行中操作的 staging 目录。
- 按数据库逐个验证启用插件的路径、Manifest 和哈希。
- 数据库未引用的 active 哈希目录视为孤儿并清理。
- 单个插件缺失、损坏或启动失败时写入 `runtimeError` 并保持不可调用，但不阻止主应用和其他插件启动。

Plugin Runtime 构造函数只保存依赖和配置，不查询 `plugin_info`。当前应用先同步 Schema、再调用模块 Runtime 的 `Start`，因此目录创建、staging 清理、数据库读取和逐插件恢复全部在 `Start` 中执行。全局配置非法或插件根目录不可用时 `Start` 返回错误并终止应用启动；单个插件恢复失败由 Runtime 吸收、写入安全 `runtime_error` 后继续恢复其他插件。

## 11. 安装与热切换流程

### 11.1 新装或升级

同一 `key` 不存在时是新装，存在时是升级。相同包哈希按文件生命周期一节重新验证后才视为幂等成功；不同包只接受高于当前版本的 SemVer，相同或更低版本直接拒绝。系统不兼容旧版本迁移脚本，也不保留文件用于自动回滚。

流程如下：

1. 校验管理员身份、权限和上传大小。
2. 写入 staging，同时计算包 SHA-256。
3. 安全读取 ZIP，验证条目数、压缩前后大小和路径。
4. 读取并严格校验 Manifest、SemVer、ABI、方法 Schema、契约哈希和全文件 Checksum。
5. 读取候选配置：升级保留后台现有配置，新装使用 `config.default.json`。
6. 预编译 WASM；相同 `wasmHash + abi + runtimeProfileHash` 组合只执行一次并复用编译结果。
7. 使用临时候选实例调用 `validate_config`。
8. 新装插件默认启用；启用插件的新装或升级构建完整实例池，每个实例执行 `init` 和 `health`。升级前已禁用的插件只使用临时实例完成 `init` 和 `health`，不创建运行池。
9. 按目标目录对账规则原子移动或复用文件，再提交数据库元数据。
10. 插件应启用时原子切换 Registry，排空并删除旧代际；插件保持禁用时关闭临时实例，不注册代际。

在第 10 步前发生的任何失败均不得改变当前可调用版本。Registry 原子指针提交设计为无失败返回的内存操作；数据库提交后到 Registry 切换之间不再执行插件代码、文件 I/O 或其他可失败业务步骤。若进程在该窗口崩溃，重启以数据库记录恢复新版本。

Installer 和 Info Service 对每个插件 Key 使用进程内互斥锁。同一 Key 的安装、升级、配置、启用、禁用和删除必须串行；不同 Key 可以并行。安装在解析并验证 Key 后取得锁，再重新读取数据库状态和版本，不能使用加锁前的检查结果。互斥锁不阻塞普通 Invoke，运行代际仍通过 Registry 原子切换。

### 11.2 配置更新

后台只接受最大 1 MiB 的完整 JSON Object，不支持局部路径表达式。流程为：读取当前插件与配置，校验 JSON 和 Manifest 配置 Schema 并调用 `validate_config`。插件已启用时，基于新配置构建候选代际，数据库保存配置后原子切换并排空旧代际；插件已禁用时，仅通过临时实例执行 `init` 和 `health`，保存配置后关闭临时实例，不创建运行池。

候选失败时数据库和当前代际均不改变。配置明文存储，不对字段做自动加密或脱敏。

`update` 同时提交 `status` 和 `config` 时以请求中的完整配置构建目标状态：目标为启用则只在候选代际全部就绪后提交配置和状态；目标为禁用则先验证并保存配置，再按禁用流程排空当前代际。整个操作持有同一 Key 的互斥锁，不拆成两个可并发请求。

### 11.3 启用、禁用与删除

- 启用：从当前 active 路径重新验证并构建完整代际，成功后在数据库保存启用状态并原子注册。
- 禁用：先将当前代际标记为不接新调用，再在数据库保存禁用状态；提交失败时恢复接单，提交成功后从 Registry 移除并排空旧代际。文件和配置保留。
- 删除：先将当前代际标记为不接新调用，在数据库保存禁用状态；提交失败时恢复接单。随后移除并排空代际、删除数据库记录，最后删除该 Key 的 active 文件目录。进程在删除记录后中断时，启动恢复将文件识别为孤儿并清理。

禁用和删除期间的新调用返回稳定的不可用错误，不等待排空完成。

## 12. 数据模型

`plugin_info` 是平台全局表，模型必须显式使用 `TenantModeDisabled`，不得复用包含 `tenant_id` 的通用 `BaseFields()`。第一版固定字段：

| 字段 | 存储类型 | 语义 |
| --- | --- | --- |
| `id` | `BIGINT UNSIGNED` | 主键 |
| `name` | `VARCHAR(255)` | Manifest 名称 |
| `description` | `TEXT` | Manifest 描述 |
| `key_name` | `VARCHAR(64)` | 唯一插件 Key，唯一索引 |
| `version` | `VARCHAR(255)` | 当前规范 SemVer |
| `author` | `VARCHAR(255)` | 作者 |
| `status` | `TINYINT` | `0` 禁用，`1` 启用 |
| `readme` | `MEDIUMTEXT` | README 展示内容 |
| `logo` | `MEDIUMTEXT` | 经过校验的 PNG 原始 RFC 4648 Base64 文本，不带 Data URI 前缀或换行 |
| `manifest` | `JSON` | 完整 Manifest JSON |
| `config` | `JSON` | 后台维护的明文 JSON Object |
| `package_hash` | `CHAR(64)` | `.coolwasm` SHA-256 小写 hex |
| `wasm_hash` | `CHAR(64)` | `plugin.wasm` SHA-256 小写 hex |
| `contract_hash` | `CHAR(71)` | `sha256:` 加 64 位小写 hex |
| `active_path` | `VARCHAR(512)` | 相对 `modules/plugin/plugins` 的当前目录 |
| `runtime_error` | `VARCHAR(1024)` | 最近启动或热切换系统错误摘要，正常时为空 |
| `create_time` | 项目规范时间类型 | GoFrame 自动维护创建时间 |
| `update_time` | 项目规范时间类型 | GoFrame 自动维护更新时间 |

数据库不保存 WASM 字节、完整上传包或历史版本。`active_path` 必须是受控相对路径，读取前再次确认解析结果位于 `modules/plugin/plugins/active` 内。

`runtime_error` 只保存安全摘要，不保存 Trap 栈、主机绝对路径、配置内容或网络凭据。Runtime 内存状态不写数据库。

成功安装、升级、配置、启用或启动恢复后清空 `runtime_error`。调用期偶发业务错误和被熔断器处理的系统错误只写运行日志，不持续写 MySQL 热路径。

## 13. 后台 API、权限与 EPS

第一版提供：

```text
POST /admin/plugin/info/page
POST /admin/plugin/info/info
POST /admin/plugin/info/install
POST /admin/plugin/info/update
POST /admin/plugin/info/delete
```

语义如下：

- `page`：分页查询平台插件元数据、状态、配置、Manifest、安全运行错误摘要和 Base64 Logo；每页最多 20 条，按 Logo 上限计算最坏约包含 6.7 MiB Logo 文本。
- `info`：查询单个插件详情、README 和 Base64 Logo。
- `install`：multipart 上传 `.coolwasm` 并执行新装或升级。
- `update`：只允许修改 `status` 和完整 `config`；出现未知字段或名称、Key、版本、哈希、路径等只读字段时直接拒绝，不静默忽略。
- `delete`：禁用、排空并删除插件记录和文件。

不提供通用 `invoke` HTTP API。需要公开插件能力时，由具体业务模块定义明确 Controller、DTO、权限和响应协议，再通过生成 Client 调用插件。

所有管理接口依次要求有效登录、平台身份和 `plugin:info:*` 权限：租户身份一律拒绝；超级管理员沿用现有权限服务的自动放行；其他平台用户必须具有对应权限。安装接口不能像当前 Node 实现一样忽略 Token。接口遵循现有统一响应、错误边界、EPS 和审计日志规则。

第一版不提供 `add` 和 `list`：插件只能通过 `install` 创建，分页场景统一使用 `page`。`modules/plugin/menu.json` 只声明 `page`、`info`、`install`、`update`、`delete` 五项权限；当前误放在 `modules/base/menu.json` 的插件菜单在实施时迁移到插件模块，不能重复导入。

## 14. 配置

主配置增加：

```yaml
plugin:
  # 热安装插件根目录，相对项目根目录解析。
  path: modules/plugin/plugins
  package:
    # 单个上传包上限。
    maxSize: 32mb
    # 解压后全部文件总上限。
    maxUnpackedSize: 128mb
    # ZIP 条目数量上限。
    maxEntries: 64
    # PNG Logo 原文件上限。
    logoMaxSize: 256kb
    # PNG Logo 最大宽高像素。
    logoMaxDimension: 1024
    # 启动恢复可清理的 staging 最小年龄。
    stagingTTL: 1h
  runtime:
    # 每个启用插件的 WASM 实例数。
    poolSize: 4
    # 包含排队、WASM、Host Function 和子调用的总超时。
    callTimeout: 30s
    # 每个 WASM 实例线性内存上限。
    memoryLimit: 64mb
    # 单次插件输入上限。
    inputLimit: 8mb
    # 单次插件输出上限。
    outputLimit: 8mb
    # 热切换或关闭时等待在途调用的时间。
    drainTimeout: 30s
    # 插件调用另一个插件的最大深度。
    maxCallDepth: 8
  http:
    # Host HTTP 最长时间，实际还受调用剩余时间限制。
    timeout: 30s
    # Host HTTP 响应正文上限。
    responseLimit: 8mb
    # Host HTTP 请求正文上限。
    requestLimit: 8mb
    # Host HTTP 请求头编码后上限。
    headerLimit: 64kb
    # Host HTTP 响应头编码后上限。
    responseHeaderLimit: 64kb
    # Host HTTP 最大重定向次数。
    maxRedirects: 5
  circuit:
    # 窗口内系统故障达到该数量后熔断。
    failures: 5
    # 故障统计滚动窗口。
    window: 30s
    # 熔断保持时间。
    openTimeout: 30s
```

所有配置在 Plugin Runtime 启动前读取和校验。大小、时长、池容量和深度必须为正；内存上限必须是 64 KiB WASM Page 的整数倍；包解压上限必须不小于上传包上限；Host HTTP 超时不得绕过总调用截止时间。

当前主配置的 `server.clientMaxBodySize` 是 12 MiB，无法承载 32 MiB multipart 插件包。实施时默认值同步提高到 40 MiB，并在启动时确认它大于 `plugin.package.maxSize` 加 1 MiB multipart 余量。安装请求必须恰好包含一个名为 `file` 的文件 Part，不允许额外文件 Part 或普通字段。安装 Action 不声明 multipart DTO，而是从 Context 取得原始 GoFrame Request，使用 `MultipartReader` 单次流式写入 staging；不得触发通用 `r.Parse` / `ParseMultipartForm`、先进入普通上传目录或完整读入内存。

## 15. 安全边界

### 15.1 明确接受的风险

- 插件包不做开发者签名、证书链或来源认证。
- 配置以明文 JSON 保存到 MySQL。
- Host HTTP 允许访问公网、内网、localhost 和云元数据。
- 因此系统不承诺防止 SSRF、敏感配置外传、内网扫描或恶意网络访问。

这些风险只在“能够管理插件的平台管理员是受信任主体”的前提下接受。

### 15.2 强制防线

- 只有平台管理员和对应权限可以安装、升级、配置、启停或删除插件。
- 上传包默认最大 32 MiB，解压后最大 128 MiB，最多 64 个条目。
- 拒绝路径穿越、绝对路径、符号链接、硬链接和重复归一化路径。
- 严格校验 Manifest、ABI、Key、SemVer、方法清单、契约哈希和全文件 SHA-256。
- WASM 内存、实例数、调用时间、输出、HTTP 响应、调用深度和日志大小有界。
- 插件不能直接访问数据库、系统命令、宿主环境变量和宿主文件系统。
- 后台和调用错误不返回 Trap、Go 栈、绝对路径、配置内容、SQL 或凭据。

## 16. 可观测性

Runtime 至少提供以下带插件 Key、版本和代际标签的指标：调用总数、调用耗时、业务错误、系统错误、实例池占用、实例等待、Trap、超时、取消、熔断状态、补池失败和排空耗时。

进程级指标至少包含启用插件数、总实例数、预编译缓存条目、缓存引用数和当前在途插件调用。第一版不基于这些总量拒绝启用或安装，只提供容量告警；告警阈值属于部署配置，不属于 ABI。

日志继续使用现有 GoFrame Logger，并自动附加调用 ID 和 Trace ID。调用负载、完整配置、HTTP Body 和凭据不得自动记录；内部 Trap 和网络诊断只进入受控服务端日志，不进入 API 或 `runtime_error`。

## 17. 错误处理

宿主对调用方提供稳定分类：

| 错误 | 语义 |
| --- | --- |
| `PLUGIN_NOT_FOUND` | Key 不存在 |
| `PLUGIN_DISABLED` | 插件已禁用或正在排空 |
| `PLUGIN_METHOD_NOT_FOUND` | Manifest 未声明方法 |
| `PLUGIN_CONTRACT_MISMATCH` | 生成 Client 与安装插件契约不一致 |
| `PLUGIN_INSTALL_STATE_CONFLICT` | DB 正在引用的 active 目录缺失或损坏，拒绝原地覆盖 |
| `PLUGIN_BUSY` | 实例池在截止时间内无可用实例 |
| `PLUGIN_TIMEOUT` | 总调用时间耗尽 |
| `PLUGIN_CANCELED` | 调用方主动取消调用 |
| `PLUGIN_CALL_LOOP` | 插件调用链出现重复插件 Key |
| `PLUGIN_CALL_DEPTH_EXCEEDED` | 子调用深度超过配置上限 |
| `PLUGIN_CIRCUIT_OPEN` | 插件系统故障已触发熔断 |
| `PLUGIN_RUNTIME_ERROR` | Trap、非法 ABI 或 Host Function 系统错误 |
| `PLUGIN_BUSINESS_ERROR` | 插件主动声明的业务错误，保留其业务 Code |

错误行为：

- 插件业务错误返回公开 Code、Message 和有界 Details，不计入熔断。
- Trap、非法 JSON 和不可恢复实例错误淘汰实例并记录内部诊断。
- Pool 满或超时返回稳定错误，不无限排队。
- 安装、升级和配置候选失败时返回安全摘要并保留当前代际。
- 启动时单个插件失败只标记该插件不可调用，不阻止主应用启动。

## 18. 测试策略

### 18.1 包与 Manifest

使用恶意 Fixture 覆盖：

- Zip Bomb、超条目、超解压大小。
- `../`、绝对路径、Windows 路径、符号链接、硬链接和归一化重复路径。
- 缺文件、额外非法文件、Checksum 不一致。
- 非法 Key、SemVer、ABI、Schema、方法名、重复方法和契约哈希。
- 超过 1024 个方法。
- 非 PNG、超 256 KiB、像素尺寸超 1024 或解码炸弹 Logo。

所有恶意包必须在执行 WASM 前拒绝。

### 18.2 Runtime

覆盖：

- 每个相同 `wasmHash + abi + runtimeProfileHash` 组合只预编译一次。
- 实例池上限、Context 取消、Pool 耗尽和无界 goroutine 检查。
- wazero Context 取消终止和 1024 Page 内存上限实际生效；超时或取消实例永不回池。
- `init`、配置校验、健康检查和配置代际切换。
- Trap 淘汰补池、输出超限、熔断打开、单探测和恢复。
- `A -> A`、`A -> B -> A` 循环与深度 8。
- 热升级原子切换、旧池排空、超时取消和应用关闭。

### 18.3 SDK、CLI 与契约

覆盖：

- `init`、`generate`、`test`、`build`、`pack` 完整流程。
- Go 契约稳定生成 Dispatcher、Client、Schema、方法清单和相同契约哈希。
- 生成 Client 的方法、字段和响应具备编译期类型检查。
- 契约不匹配在调用前返回 `PLUGIN_CONTRACT_MISMATCH`。
- 示例源码能生成单个 `.coolwasm` 并通过正式宿主契约测试。
- 普通 Go、TinyGo SDK、testhost 和正式宿主对共享 ABI Golden Fixture 产生逐字节一致结果。
- `CPH1` HTTP 帧的 Magic、版本、Kind、Reserved、大小端长度、溢出、超限和尾随字节恶意 Fixture 全部在网络请求前拒绝。

### 18.4 HTTP 与 MySQL 集成

使用真实 MySQL，并由 `COOL_PLUGIN_INTEGRATION=1` 显式开启，覆盖：

- 匿名、Refresh Token 和租户身份不能管理插件；平台普通用户按权限码放行，超级管理员沿用现有自动放行。
- 32 MiB 包可流式上传，超过插件上限或服务器请求上限时在解包前拒绝。
- 安装 Action 只接受一个名为 `file` 的文件 Part，拒绝额外文件 Part 和普通字段，不触发 GoFrame `ParseMultipartForm`，上传内容只写入受控 staging。
- 新装、升级、配置、启用、禁用和删除。
- 安装或升级失败保持旧版本、旧配置和旧文件可调用。
- 数据库相对路径校验、损坏文件和启动恢复。
- EPS、菜单、统一响应和错误不泄漏内部信息。
- README 作为不可信 Markdown 展示，原始 HTML 或清洗后的危险脚本、事件属性和 URL Scheme 无法执行。
- `page` 每页最多 20 条并返回已校验的 Base64 Logo；`update` 拒绝未知和只读字段。

### 18.5 并发与性能

覆盖：

- `go test -race` 下安装、配置切换、启停和持续 Invoke。
- 100 个并发 Invoke、池耗尽、取消、Trap 补池和熔断竞争。
- 调用热路径零 MySQL 查询、零文件读取、零 Manifest 解析。
- 相同 `wasmHash + abi + runtimeProfileHash` 只编译一次；切换后旧代际最终全部释放。
- 长时间压测无持续 goroutine、实例、编译缓存或线性内存增长。

JSON 编解码保留基准测试，但第一版不设与具体硬件绑定的延迟 SLA。大文件必须通过引用或 Host HTTP 传递，基准不得用 Base64 大负载掩盖协议问题。

### 18.6 恢复

覆盖进程在以下阶段中断：写 staging、解包完成、原子移动后、数据库提交后和 Registry 切换后。另覆盖已存在完整目标目录、未引用损坏目录和数据库正在引用的损坏目录。重启后必须满足：主应用可用、数据库引用的完整插件可恢复、损坏插件不可调用、staging 和孤儿 active 目录可安全清理。

## 19. 验收标准

1. Go/TinyGo 示例插件可通过 CLI 生成一个 `.coolwasm`，后台上传后无需重启即可调用。
2. 业务模块通过生成的强类型 Client 调用已知插件，不出现手写 JSON 或方法字符串。
3. Runtime 不提供 `getInstance()`、远程对象或 Handle 协议。
4. 热安装文件只位于 `modules/plugin/plugins/staging|active`，且不会被静态 HTTP 暴露。
5. 升级、配置或启用候选失败时，当前插件继续成功处理调用。
6. 热路径不读取 MySQL 和文件；每个相同 WASM、ABI 和 Runtime 配置组合只预编译一次。
7. 单插件、单实例和单调用的实例数、内存、调用时间、输入、输出、HTTP Header/Body、调用深度和排空时间均有界；进程总资源不承诺固定上限。
8. 插件间循环调用、未声明方法、契约不匹配和恶意 ZIP 在规定阶段被拒绝。
9. 单个插件损坏或启动失败不阻止主应用和其他插件运行。
10. 聚焦单元测试、真实 MySQL 集成、竞态检测、全仓测试、`go vet` 和 `git diff --check` 通过。
11. ABI、Schema Version 和契约哈希在取得实例前同时匹配，共享 Golden Fixture 在宿主和 TinyGo SDK 中逐字节通过。
12. 匿名、Refresh Token 和租户身份无法访问任何插件管理接口，安装接口不再忽略 Token。

## 20. 后续实施顺序

本文档批准后仅进入详细实施计划，不直接开始编码。计划应按以下依赖顺序拆分：

1. `cool-admin-go-plugin/contract/abi/v1`、ABI Golden Fixture、SDK、契约生成和最小示例。
2. `cool/plugin` Manifest、安全包读取、Runtime 与 Host Functions。
3. `modules/plugin` Entity、Repository、Installer、Service、Controller、配置和菜单。
4. 热安装事务、启动恢复、配置代际、启停和删除。
5. 强类型 Client 端到端示例、集成测试、竞态与压力验证。
