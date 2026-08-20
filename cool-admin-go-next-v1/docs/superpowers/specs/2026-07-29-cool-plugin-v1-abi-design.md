# cool-plugin-v1 ABI 规范

- 日期：2026-07-29
- 适用仓库：`cool-admin-go-plugin`、`cool-admin-go-next`
- ABI 标识：`cool-plugin-v1`
- Manifest Schema：`1`
- 状态：设计决策已批准，已完成规范自检，等待用户书面审核

## 1. 目的与规范等级

本文是 `cool-plugin-v1` 的规范性定义，完整规定插件与宿主之间的线协议。总体架构、安装事务、后台接口和运行生命周期由 `2026-07-28-go-plugin-system-design.md` 定义；两份文档冲突时，ABI 字节格式、字段、错误和版本行为以本文为准。

`cool-admin-go-plugin/contract/abi/v1` 是本文的 Go 参考实现，提供无 GoFrame、数据库和业务 Runtime 依赖的类型、严格编解码器、常量和 Golden Fixture。正式宿主、插件 SDK 和 testhost 必须复用该包或通过相同 Fixture 证明字节兼容，不能各自定义协议变体。

本文中的“必须”“不得”“固定”是兼容性要求；违反要求的输入必须被接收方拒绝，不能猜测、修复或静默忽略。

## 2. 基础编码规则

### 2.1 JSON

除 `host.http` Body 外，ABI 数据均使用 UTF-8 JSON。规则如下：

1. 顶层必须是一个 JSON Object。
2. 不允许 UTF-8 BOM、非法 UTF-8、重复 Key、尾随 Token、注释、`NaN`、`Infinity` 或 `-Infinity`。
3. ABI 信封和固定元数据对象拒绝未知字段。
4. 业务 `payload`、成功 `data`、配置和业务错误 `details` 内部字段由插件契约决定；宿主调用期不执行 JSON Schema 校验。
5. 空请求和空结果固定编码为 `{}`，不使用空字节、`null` 或语言私有零值。
6. JSON 编码不得依赖 Map 遍历顺序。只有契约哈希输入要求 RFC 8785 规范化，普通调用不要求字段顺序。

### 2.2 大小计算

所有 JSON 上限按 UTF-8 编码后的字节数计算，二进制帧上限按完整帧或对应字段的原始字节数计算。接收方必须先执行有界读取，再解析或分配目标缓冲区。

固定上限如下：

| 数据 | 上限 |
| --- | --- |
| 单次 `invoke` 请求信封 | 8 MiB |
| 单次插件入口输出 | 8 MiB |
| 配置 Object | 1 MiB |
| 业务错误 `details` | 64 KiB |
| `host.log` 完整请求 | 64 KiB |
| `host.http` 请求 Headers JSON | 64 KiB |
| `host.http` 响应 Headers JSON | 64 KiB |
| `host.http` 请求 Body | 8 MiB |
| `host.http` 响应 Body | 8 MiB |
| `host.invoke` 请求和响应 | 各 8 MiB |

### 2.3 标识与时间

- 插件 Key、方法名、SemVer 和契约哈希沿用总体设计的格式。
- 调用 ID、Trace ID 和代际 ID 均为非空 UTF-8 字符串，最长 128 字节。
- Deadline 固定为 UTC RFC 3339 Nano 字符串，例如 `2026-07-29T12:00:00.123456789Z`。
- 调用深度使用 JSON 非负整数，根调用为 `0`，最大值由宿主配置且第一版默认为 `8`。

## 3. 共享类型与依赖边界

共享包固定放置于：

```text
cool-admin-go-plugin/contract/abi/v1/
├── constants.go
├── envelope.go
├── entries.go
├── host.go
├── http_frame.go
├── strict_json.go
└── testdata/
```

其父包 `cool-admin-go-plugin/contract/invoke.go` 定义宿主 Client 所需的最小 `Invoker` 接口和 `ContractExpectation`。生成 Client 只导入该父包、ABI 包和自身契约类型；正式 Runtime 负责实现该接口，因此共享工具链不得反向导入 `cool-admin-go-next`。

共享接口的 Go 形状固定为：

```go
type ContractExpectation struct {
    ABI           string
    SchemaVersion uint32
    ContractHash  string
}

type Invoker interface {
    Invoke(ctx context.Context, pluginKey, method string, request, response any) error
    InvokeContract(ctx context.Context, pluginKey, method string, expectation ContractExpectation, request, response any) error
}
```

`ContractExpectation` 的字段在构造后不得修改；Runtime 必须把传入值作为一次调用的只读快照。

边界如下：

- 可以依赖 Go 标准库和经 TinyGo 兼容性验证的纯 Go 小型依赖。
- 不得导入 `cool-admin-go-next`、GoFrame、数据库驱动、Extism Runtime 或业务模块。
- `cool-admin-go-next/cool/plugin` 可以导入共享 ABI 包。
- 插件 SDK 可以包装共享类型，但插件开发者不直接拼接 ABI 信封或 HTTP 帧。
- 生成的宿主 Client 只依赖共享 `contract.Invoker` 接口和自身契约类型，不依赖 Runtime、Installer、数据库或主应用仓库。

## 4. Extism 边界

### 4.1 插件导出入口

每个插件必须导出以下四个 Extism Plugin Function：

| 导出名 | 用途 |
| --- | --- |
| `init` | 初始化一个新插件实例 |
| `validate_config` | 验证候选平台配置 |
| `invoke` | 执行声明的业务方法 |
| `health` | 验证初始化后的实例可用 |

它们均通过 Extism PDK 的单段字节输入输出调用。SDK 可以使用不同的 Go 函数名生成这些 WASM 导出别名，但最终 WASM 导出名必须完全一致。

### 4.2 Host Function 导入

WASM 导入模块固定为 Extism 用户 Host Function 命名空间 `extism:host/user`，函数名固定为：

| ABI 逻辑名 | WASM 导入名 | Extism 类型 |
| --- | --- | --- |
| `host.log` | `cool_log` | `PTR -> PTR` |
| `host.http` | `cool_http` | `PTR -> PTR` |
| `host.invoke` | `cool_invoke` | `PTR -> PTR` |
| `host.context` | `cool_context` | `PTR -> PTR` |

`PTR` 表示由 Extism 管理并带长度的内存块。插件 SDK 负责读取返回块并遵守 Extism 的内存生命周期；插件业务代码不得保存跨调用 PTR、宿主对象或实例 Handle。

## 5. 通用结果信封

### 5.1 成功

所有 JSON 型入口和 Host Function 成功时返回：

```json
{
  "ok": true,
  "data": {}
}
```

`ok` 和 `data` 必须存在。`data` 的具体结构由对应操作定义。

### 5.2 失败

失败信封固定为：

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

规则如下：

- `kind` 只能是 `business` 或 `system`。
- 插件导出入口只能主动返回 `business`；插件伪造 `system` 时宿主按非法 ABI 输出处理。
- Host Function 可以返回 `system`；`host.invoke` 还可以转发目标插件的 `business`。
- `code` 必须匹配 `^[A-Z][A-Z0-9_]{1,63}$`。
- `message` 必须是可公开 UTF-8 文本，最长 1024 字节。
- `details` 可省略；存在时必须是 JSON Object，编码后最多 64 KiB，不纳入契约哈希。
- `business` 不计入熔断；`system` 由当前插件调用转换为宿主系统错误并计入相应故障统计。

## 6. 插件导出入口

### 6.1 `init`

请求：

```json
{
  "abi": "cool-plugin-v1",
  "schemaVersion": 1,
  "pluginKey": "wx",
  "pluginVersion": "1.0.0",
  "generationId": "wx-42",
  "config": {}
}
```

规则如下：

- 所有字段必须存在。
- `abi` 和 `schemaVersion` 必须与已安装 Manifest 一致。
- `config` 是已通过 Manifest 配置 Schema 和 `validate_config` 的快照。
- 成功 `data` 固定为 `{}`。
- 每个进入运行池或临时健康检查的实例只调用一次 `init`。

### 6.2 `validate_config`

请求：

```json
{
  "config": {}
}
```

成功 `data` 固定为 `{}`。配置不被插件接受时返回 `business`，建议 Code 为 `PLUGIN_CONFIG_INVALID`；Trap、非法输出和 Host Function 故障属于系统失败。

该调用在临时实例上执行。完成后临时实例必须关闭，不能因为验证成功直接放入正式池。

### 6.3 `invoke`

请求：

```json
{
  "method": "officialAccount.getAccessToken",
  "payload": {}
}
```

规则如下：

- `method` 必须已在当前 Manifest 中声明。
- `payload` 必须是 JSON Object；空请求使用 `{}`。
- 成功 `data` 是方法的响应数据，必须是 JSON Object。
- 宿主严格解析信封并解码到调用方目标类型，但调用期不执行请求或响应 JSON Schema 校验。

### 6.4 `health`

请求固定为 `{}`。成功响应固定为：

```json
{
  "ok": true,
  "data": {
    "status": "ok"
  }
}
```

`status` 只允许字面值 `ok`。任何失败、Trap、非法字段或其他状态均表示候选实例不可用。

## 7. Host Functions

### 7.1 `host.log` / `cool_log`

请求：

```json
{
  "level": "info",
  "message": "request completed",
  "fields": {
    "remoteCode": "200"
  }
}
```

规则如下：

- `level` 只允许 `debug`、`info`、`warn`、`error`。
- `message` 必须是字符串。
- `fields` 可省略；存在时必须是 JSON Object，值只允许布尔值、字符串或 JSON Number。
- 完整请求最大 64 KiB。SDK 先编码 `fields` 并为消息 JSON 开销和 ASCII `...` 预留空间；字段已耗尽预算时拒绝请求。仅 `message` 超限时，SDK 截取可容纳的最长 UTF-8 字符前缀并追加 `...`。宿主仍独立检查上限，对超限请求返回错误，不执行第二次截断。
- 成功 `data` 固定为 `{}`。
- 宿主自动添加插件 Key、版本、代际、方法、调用 ID、Trace ID 和调用深度，插件不能覆盖这些字段。

### 7.2 `host.context` / `cool_context`

请求固定为 `{}`。成功 `data`：

```json
{
  "callId": "call-123",
  "traceId": "trace-456",
  "pluginKey": "wx",
  "method": "officialAccount.getAccessToken",
  "callerPluginKey": "",
  "callDepth": 0,
  "deadline": "2026-07-29T12:00:00Z"
}
```

`callerPluginKey` 在宿主业务代码直接调用时固定为空字符串。该接口不得返回原始 HTTP Request、Cookie、Authorization、环境变量、数据库连接或配置内容。

### 7.3 `host.invoke` / `cool_invoke`

请求：

```json
{
  "pluginKey": "storage",
  "method": "object.put",
  "payload": {}
}
```

成功 `data` 是目标方法的响应 Object。目标插件业务错误按通用失败信封转发并保持 `kind: business`；目标不存在、禁用、超时、熔断、循环、深度超限和 ABI 故障返回 `kind: system`。

根调用深度固定为 `0`。子调用深度固定为父调用深度加 `1`；子调用深度大于宿主配置的最大值时拒绝，等于最大值时仍允许。

宿主在取得目标实例前必须：

1. 继承父调用的 Context、Deadline、Trace ID 和调用链。
2. 拒绝调用链中已经出现的目标插件 Key。
3. 计算子调用深度并拒绝超过配置上限的调用。
4. 应用目标插件自己的方法清单、实例池和熔断器。

### 7.4 `host.http` / `cool_http`

`host.http` 使用第 8 节的二进制帧，不使用通用 JSON 结果信封。插件业务代码只能使用 SDK HTTP Client，不能手写帧。

### 7.5 JSON Host Function 稳定系统错误

三个 JSON Host Function 的 `kind: system` Code 固定为下表。ABI v1 实现不得新增同义 Code；未能归入更具体类别的目标 Runtime 故障统一使用 `HOST_INVOKE_TARGET_ERROR`。

| Host Function | Code | 语义 |
| --- | --- | --- |
| `host.log` | `HOST_LOG_INVALID_REQUEST` | Level、Message、Fields 或 JSON 结构非法 |
| `host.log` | `HOST_LOG_REQUEST_TOO_LARGE` | 完整日志请求超过 64 KiB |
| `host.context` | `HOST_CONTEXT_INVALID_REQUEST` | 请求不是严格的空 Object |
| `host.context` | `HOST_CONTEXT_UNAVAILABLE` | 宿主无法取得完整调用上下文 |
| `host.invoke` | `HOST_INVOKE_INVALID_REQUEST` | Key、方法、Payload 或 JSON 结构非法 |
| `host.invoke` | `HOST_INVOKE_TARGET_NOT_FOUND` | 目标插件不存在 |
| `host.invoke` | `HOST_INVOKE_TARGET_DISABLED` | 目标插件未启用或正在排空 |
| `host.invoke` | `HOST_INVOKE_METHOD_NOT_FOUND` | 目标方法未声明 |
| `host.invoke` | `HOST_INVOKE_TARGET_BUSY` | 目标实例池在截止时间内无可用实例 |
| `host.invoke` | `HOST_INVOKE_TARGET_TIMEOUT` | 目标调用耗尽 Deadline |
| `host.invoke` | `HOST_INVOKE_TARGET_CANCELED` | 父调用已取消 |
| `host.invoke` | `HOST_INVOKE_TARGET_CIRCUIT_OPEN` | 目标插件熔断器已打开 |
| `host.invoke` | `HOST_INVOKE_CALL_LOOP` | 调用链出现重复插件 Key |
| `host.invoke` | `HOST_INVOKE_CALL_DEPTH_EXCEEDED` | 子调用深度超过配置上限 |
| `host.invoke` | `HOST_INVOKE_RESPONSE_TOO_LARGE` | 目标响应超过 8 MiB |
| `host.invoke` | `HOST_INVOKE_TARGET_ERROR` | 目标 Trap、非法 ABI、非法输出或其他 Runtime 故障 |

## 8. Host HTTP 二进制帧

### 8.1 帧头

完整帧由 16 字节固定头、JSON 元数据和原始 Body 顺序拼接：

| 偏移 | 长度 | 字段 | 编码 |
| --- | --- | --- | --- |
| `0` | 4 | Magic | ASCII `CPH1` |
| `4` | 1 | Frame Version | `0x01` |
| `5` | 1 | Frame Kind | `0x01` 请求，`0x02` 响应 |
| `6` | 2 | Reserved | 必须为 `0x0000` |
| `8` | 4 | Metadata Length | 无符号 32 位大端整数 |
| `12` | 4 | Body Length | 无符号 32 位大端整数 |
| `16` | N | Metadata | UTF-8 JSON Object |
| `16+N` | M | Body | 原始字节，可为空 |

接收方必须在分配 N 或 M 字节前检查：Magic、版本、Kind、Reserved、整数溢出、元数据上限、Body 上限，以及 `16 + N + M` 是否与实际帧长度完全一致。帧不得包含尾随字节。

### 8.2 HTTP 请求元数据

请求帧 Metadata：

```json
{
  "method": "POST",
  "url": "https://api.example.com/token",
  "headers": {
    "Content-Type": ["application/json"]
  }
}
```

规则如下：

- `method` 必须是合法 HTTP Token，SDK 默认转为大写。
- `url` 只允许绝对 `http` 或 `https` URL，不允许 UserInfo 中包含凭据。
- `headers` 必须是 `map[string][]string` 语义，以保留重复响应头。
- Header 名和值不得包含 CR、LF、NUL 或其他非法控制字符。
- `Host`、`Content-Length`、`Connection`、`Transfer-Encoding` 和其他 Hop-by-Hop Header 由宿主管理，插件设置时拒绝请求。
- Body 长度由帧头决定，不能再由元数据声明。

### 8.3 HTTP 成功响应元数据

响应帧成功 Metadata：

```json
{
  "ok": true,
  "status": 200,
  "url": "https://api.example.com/token",
  "headers": {
    "Content-Type": ["application/json"]
  }
}
```

`status` 是最终 HTTP 状态码，任何 HTTP 状态码都属于成功完成传输，不自动变成 Host Function 系统错误。`url` 是重定向后的最终 URL。响应 Body 是完成内容解码后的有界原始字节；压缩响应必须在解压过程中执行 8 MiB 上限。

### 8.4 HTTP 系统失败响应元数据

系统失败响应 Body 必须为空，Metadata 固定为：

```json
{
  "ok": false,
  "error": {
    "kind": "system",
    "code": "HOST_HTTP_TIMEOUT",
    "message": "HTTP request timed out"
  }
}
```

ABI v1 允许的稳定 Code 固定为：

| Code | 语义 |
| --- | --- |
| `HOST_HTTP_INVALID_REQUEST` | URL、Method、Header 或帧非法 |
| `HOST_HTTP_TIMEOUT` | HTTP 或父调用 Deadline 耗尽 |
| `HOST_HTTP_CANCELED` | 父调用被取消 |
| `HOST_HTTP_REQUEST_TOO_LARGE` | 请求 Header 或 Body 超限 |
| `HOST_HTTP_RESPONSE_TOO_LARGE` | 响应 Header、解压后 Body 或帧超限 |
| `HOST_HTTP_REDIRECT_LIMIT` | 超过 5 次重定向 |
| `HOST_HTTP_TRANSPORT_ERROR` | DNS、连接、TLS 或读取失败 |

内部网络地址、凭据、绝对路径和底层错误文本不得进入公开 `message`。

### 8.5 HTTP 执行边界

- HTTP 总时间不超过 30 秒，且不得超过父插件调用剩余时间。
- 最多跟随 5 次重定向，每次重定向继续执行相同协议和 Context 限制。
- Context 取消后立即停止写请求或读响应，并关闭 Body。
- 第一版允许公网、内网、localhost 和云元数据地址，不做 SSRF 地址过滤。
- Transport 必须设置响应 Header 上限，不能只在完整读取后检查。

## 9. 契约哈希与 Client 兼容性

### 9.1 哈希输入

契约哈希输入固定为：

```json
{
  "abi": "cool-plugin-v1",
  "schemaVersion": 1,
  "methods": [
    {
      "name": "officialAccount.getAccessToken",
      "requestSchema": {},
      "responseSchema": {}
    }
  ]
}
```

方法按 `name` 的 UTF-8 字节升序排列；每个 Schema 和完整对象使用 RFC 8785 JSON Canonicalization Scheme 规范化，最后计算 SHA-256。文本固定为 `sha256:` 加 64 位小写十六进制。

配置 Schema、默认配置、版本、README、Logo 和业务错误 `details` 不进入契约哈希。

### 9.2 Client 期望

生成 Client 固化以下四项：

- 插件 Key。
- `abi`。
- `schemaVersion`。
- `contractHash`。

宿主在取得实例前必须逐项完全匹配，不能只比较契约哈希。总体设计中的 `InvokeContract` 使用一个结构化 `ContractExpectation` 传递后三项。

### 9.3 调用期 Schema 行为

JSON Schema 只用于：

- CLI 生成契约和 Client。
- 打包阶段计算契约哈希。
- 安装阶段验证 Schema 方言、结构和哈希。

调用期不运行 JSON Schema 引擎。宿主仍必须执行：信封严格解码、大小限制、方法存在性检查、Client 兼容性检查，以及目标 Go 类型解码。动态 `Invoke` 调用方承担业务 Payload 语义不匹配的风险。

## 10. Context、取消与实例状态

宿主对每次根调用创建有效 Deadline，取调用方 Deadline 与 `runtime.callTimeout` 的较早值。所有 Host Function 和子调用使用同一 Context 链。

Runtime 必须在 wazero 配置中启用 Context 取消时终止执行的能力。WASM 调用发生超时或取消后，无论 SDK 是否报告正常终止，该实例都必须关闭并异步补池，不得重新进入可用池。

调用方在实例开始执行前主动取消，不计入插件故障；开始执行后的系统超时、Trap、非法输出和 Host Function 系统错误计入熔断。业务错误不计入。

## 11. 错误映射

插件导出入口与 Host Function 的内部错误最终映射为宿主稳定错误：

| ABI 结果 | 宿主结果 |
| --- | --- |
| 插件 `kind: business` | `PLUGIN_BUSINESS_ERROR`，保留 Code、Message、Details |
| Host Function `kind: system` | `PLUGIN_RUNTIME_ERROR` 或更具体稳定分类 |
| 根调用 Deadline 耗尽 | `PLUGIN_TIMEOUT` |
| 等实例时 Deadline 耗尽 | `PLUGIN_BUSY` |
| Context 主动取消 | `PLUGIN_CANCELED` |
| `HOST_INVOKE_CALL_LOOP` | `PLUGIN_CALL_LOOP` |
| `HOST_INVOKE_CALL_DEPTH_EXCEEDED` | `PLUGIN_CALL_DEPTH_EXCEEDED` |
| Trap、非法 JSON、非法帧 | `PLUGIN_RUNTIME_ERROR`，关闭实例 |
| ABI、Schema Version、契约哈希不匹配 | `PLUGIN_CONTRACT_MISMATCH`，调用前拒绝 |

公开错误不包含 Trap 栈、Go 栈、绝对路径、配置、SQL、URL 凭据或底层网络诊断。完整诊断只进入受控内部日志。

## 12. Golden Fixture 与兼容性测试

共享 `testdata` 至少包含：

```text
testdata/
├── entries/
│   ├── init-request.json
│   ├── validate-config-request.json
│   ├── invoke-request.json
│   ├── health-success.json
│   ├── business-error.json
│   └── invalid/
├── host/
│   ├── log-request.json
│   ├── context-success.json
│   ├── invoke-request.json
│   └── invalid/
├── http/
│   ├── request.bin
│   ├── response.bin
│   ├── error.bin
│   └── invalid/
└── contract/
    ├── canonical.json
    └── hash.txt
```

验收要求：

1. 共享 Go Contract、TinyGo SDK、testhost 和正式宿主读取相同 Fixture。
2. 正常编码必须逐字节等于 Fixture，不能只比较解码结果。
3. 所有 invalid Fixture 必须在进入插件业务逻辑或发送网络请求前拒绝。
4. Extism、wazero、TinyGo 或 JSON Canonicalization 依赖升级前必须重新通过全部 Fixture。
5. ABI v1 已发布后不修改既有字段语义、帧布局或错误分类；不兼容变化创建 `cool-plugin-v2` 和新的版本化包。

## 13. ABI v1 验收标准

1. 示例 TinyGo 插件与正式 Go 宿主使用同一共享 Contract 和 Fixture 完成四个入口调用。
2. 生成 Client 在调用前同时校验 ABI、Schema Version 和契约哈希。
3. 插件不能返回宿主对象、实例对象、Handle 或跨调用 PTR。
4. HTTP 帧对 Magic、版本、长度、字节序、Header 和 Body 上限执行先验校验。
5. 超时或取消的 WASM 实例一律关闭，不再复用。
6. 插件业务错误与宿主系统错误可稳定区分，业务错误不触发熔断。
7. 调用期不执行 JSON Schema 校验的限制被文档、测试和 API 注释明确说明。
8. 两个仓库的 ABI 回归、TinyGo 编译和 Golden Fixture 测试全部通过。
