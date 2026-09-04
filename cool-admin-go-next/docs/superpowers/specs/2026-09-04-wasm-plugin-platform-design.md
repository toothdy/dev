# WASM 插件平台设计

> 日期：2026-09-04
> 状态：已批准
> Node 宿主参考：`cool-admin-midway/src/modules/plugin`
> Node 开发脚手架参考：`https://github.com/cool-team-official/cool-admin-midway-plugin`
> 目标工程：`cool-admin-go-next`

## 1. 背景

Node 版插件不是预先枚举上传、短信、支付等类型，而是允许插件继承统一 `BasePlugin`，自由公开方法，再由宿主通过插件 `key` 或 `hook` 加方法名调用。插件包由 `plugin.json` 描述，JavaScript 构建产物安装后通过 `eval` 装载，因此能够在不重启 Node 服务的情况下启用、禁用和升级。

Go 不能直接复制该机制：

- Go 源码必须先编译，不能像 JavaScript 一样直接 `eval`；
- 标准库 `plugin` 只支持部分操作系统，不能卸载，并严格依赖相同 Go 工具链、构建参数和依赖源码；
- 当前 Go v2 架构要求业务模块、Provider、路由和生命周期由 `cool generate` 静态确定，运行期不扫描 Go 源码、不使用字符串 DI，也不反射调用业务 Service；
- 插件由超管安装并被视为完全信任，但插件错误仍不能破坏宿主进程的可用性。

因此，Go 版使用标准 Go 编译的 WASI WebAssembly 模块作为插件载体，在主服务内通过 WASM Runtime 热加载。插件调用保持开放，宿主静态装配原则保持不变。

## 2. 目标

本设计必须实现以下能力：

1. 插件只使用 Go 开发，通过独立 SDK 和脚手架构建；
2. 超管在后台上传 `.cool` 文件后，无需重新编译或重启 Go 主服务即可启用；
3. 插件可以声明任意方法，不要求宿主提前知道插件属于上传、短信、支付、AI 或其他类型；
4. 保留 Node 的 `key`、`hook`、`singleton`、版本、配置、就绪回调和插件间调用概念；
5. 支持安装、校验、启用、禁用、配置更新、升级和卸载；
6. 单次插件异常、Panic、WASM Trap、超时或内存越限不能终止宿主服务；
7. 多实例部署最终加载相同插件版本和配置；
8. 插件数据在 MySQL、PostgreSQL 和 SQLite 上保持一致行为；
9. 插件平台作为普通静态组件接入现有 Application Graph，不建立第二套业务模块 DI 系统。

## 3. 信任边界

产品信任模型固定为：超管安装插件即代表明确授予该插件完全信任，插件来源、代码和行为风险由超管承担。首期不提供安装时权限清单、逐项授权或发布者签名验证。

完全信任不等于无限资源。宿主仍强制限制包大小、解压大小、单次调用输入输出、执行时间、实例数量和 WASM 内存，用于阻止程序错误或失控计算拖垮服务。SHA-256 只证明制品在保存和分发过程中未损坏，不证明发布者身份或代码安全。

WASM 不继承宿主环境变量、数据库连接、Go 对象或应用容器。插件需要的能力通过稳定 Host API 获取。这是 WASM 调用边界，不是权限审批系统。

## 4. 范围

### 4.1 首期包含

- 任意插件方法注册和 JSON 请求、响应；
- 插件配置、缓存、日志、HTTP、文件和插件间调用 Host API；
- 宿主显式开放业务能力的 `host.call` 扩展点；
- Node 风格的 `key` 直接调用和 `hook` 实现替换；
- 单例与按调用实例两种生命周期；
- 后台管理接口和现有插件管理页面所需数据；
- 单节点原子热切换与多节点最终一致同步；
- 标准 Go 1.26 WASI 构建链路；
- `wazero` 纯 Go Runtime。

### 4.2 首期不包含

- 插件动态新增宿主 Controller、HTTP 路由或 gRPC 方法；
- 插件声明实体、创建数据库表或运行数据库迁移；
- 向插件直接暴露数据库连接和任意 SQL；
- 插件自行启动常驻 goroutine、监听端口或后台调度循环；
- TypeScript、Rust 或其他插件开发语言；
- Node 插件源码或现有 `.cool` 制品的二进制兼容；
- 插件市场下载、计费、许可证和发布审核；
- 细粒度权限清单和发布者签名体系；
- 同一 Hook 的流量分配、灰度比例或负载均衡。

需要定时执行插件时，由宿主任务模块按配置调用插件方法。需要公开 HTTP 能力时，由静态 Controller 完成认证和请求校验，再调用插件，不提供公共万能调用路由。

## 5. 方案选择

### 5.1 采用：WASM 插件

插件使用 Go 1.26 的 `GOOS=wasip1 GOARCH=wasm` 和 `-buildmode=c-shared` 构建为 WASI reactor。宿主使用 `wazero` 编译和实例化插件，并通过稳定 ABI 交换 JSON。

选择理由：

- 制品跨 Linux、macOS、amd64 和 arm64，不需要按宿主平台分别构建；
- 安装后可以在主服务内热加载、关闭和替换；
- WASM Trap 和线性内存与宿主 Go 堆隔离；
- Runtime 为纯 Go，不引入 CGO 和系统动态库；
- ABI 可以独立版本化，不绑定宿主内部 Go 类型和依赖版本。

首期固定使用 `github.com/tetratelabs/wazero` v1 系列。实现前必须根据项目锁定版本阅读其官方 Runtime、WASI、Host Module、内存限制和 Context 取消文档，并由 `go.mod` 固定具体版本。

### 5.2 未采用：独立插件进程

独立进程加 gRPC 也支持热更新和任意方法，并能提供更强的故障隔离，但插件必须按操作系统和 CPU 架构分别发布，部署、进程监督和端口管理更复杂。它不是首期实现。

### 5.3 未采用：Go 标准库 `plugin`

标准库插件不能关闭或卸载，平台覆盖有限，Race Detector 支持不足，并要求宿主和插件使用完全一致的工具链、构建参数及公共依赖源码。它不满足跨平台后台上传和可靠升级要求。

### 5.4 未采用：嵌入 JavaScript Runtime

嵌入 JavaScript 看似最接近 Node，但无法完整兼容 npm、Node 内建模块和 Midway 容器；死循环和内存失控也更容易影响主进程。为了兼容一部分旧代码引入第二种运行时，不符合首期只支持 Go 的边界。

## 6. 总体架构

```text
后台上传 .cool
      |
      v
Plugin Install Service
  校验 ZIP、Manifest、SHA-256、ABI 和 WASM
      |
      v
plugin_info + plugin_artifact
      |
      v
Plugin Manager
  +-- Artifact Store 与本机缓存
  +-- wazero Runtime 与 CompiledModule 缓存
  +-- key / hook Resolver
  +-- Instance 生命周期与热切换
  +-- Invocation Context 与资源限制
      |
      +------------------------+
      |                        |
      v                        v
WASM Plugin               Host API Registry
  cool_invoke               log / cache / http / file
                            plugin.call / context / host.call
```

职责划分如下：

| 组件 | 职责 |
| --- | --- |
| `modules/plugin` | 插件实体、管理接口、安装与状态业务规则 |
| Plugin Manager | 编译缓存、解析目标、实例管理、调用和热切换 |
| Artifact Store | 保存不可变 `.cool` 制品并按 SHA-256 读取 |
| WASM Runtime | 实例化 WASI reactor、执行 ABI、限制资源和关闭实例 |
| Host API Registry | 将固定操作名映射到宿主显式实现的 JSON 适配器 |
| Guest SDK | 类型化方法注册、JSON 编解码、错误封装和 Host API 客户端 |
| `cool-plugin` CLI | 初始化脚手架、检查、测试、构建和打包插件 |

Plugin Manager 是现有静态模块图中的唯一普通组件。业务组件通过构造函数依赖它或插件调用 Contract。动态插件不会成为 Module Graph Provider，也不会修改 Graph、路由表或生成装配函数。

## 7. 插件包协议

### 7.1 目录

`.cool` 是 ZIP 文件，结构固定为：

```text
aliyun-sms_v1.0.0.cool
├── plugin.json
├── plugin.wasm
├── README.md
└── assets/
    └── logo.png
```

`plugin.json`、`plugin.wasm` 必须存在且只能出现一次。README、Logo 和其他普通资源可选，但 Manifest 引用的文件必须存在。ZIP 中禁止绝对路径、父目录跳转、反斜杠路径、符号链接、设备文件、重复规范路径和超出限制的文件。

### 7.2 Manifest

```json
{
  "schemaVersion": 1,
  "name": "阿里云短信",
  "key": "aliyun-sms",
  "hook": "sms",
  "singleton": true,
  "version": "1.0.0",
  "description": "阿里云短信发送插件",
  "author": "COOL",
  "logo": "assets/logo.png",
  "readme": "README.md",
  "runtime": {
    "abi": "cool.plugin/v1",
    "module": "plugin.wasm",
    "minHostVersion": "2.0.0"
  },
  "config": {
    "accessKeyId": "",
    "accessKeySecret": "",
    "signName": ""
  }
}
```

字段规则：

| 字段 | 规则 |
| --- | --- |
| `schemaVersion` | 首期只接受整数 `1` |
| `name` | 非空展示名称，最长 100 个 Unicode 字符 |
| `key` | 唯一标识，匹配 `^[a-z][a-z0-9-]{0,63}$`，不得为 `plugin` |
| `hook` | 可为空；非空时使用与 `key` 相同的格式 |
| `singleton` | `true` 为持久实例，`false` 为按调用实例 |
| `version` | 不带 `v` 前缀的 SemVer，例如 `1.2.3` |
| `description` | 最长 500 个 Unicode 字符 |
| `author` | 非空，最长 100 个 Unicode 字符 |
| `logo`、`readme` | 可选的包内规范相对路径 |
| `runtime.abi` | 首期固定为 `cool.plugin/v1` |
| `runtime.module` | 首期固定为 `plugin.wasm` |
| `runtime.minHostVersion` | 宿主版本不得低于该 SemVer |
| `config` | JSON object，作为默认配置 |

`schemaVersion: 1` 下拒绝未知字段，避免拼写错误被静默忽略。后续 Manifest 变化必须增加 `schemaVersion`，旧宿主对未知版本明确报不兼容。

### 7.3 配置合并

全新安装使用 Manifest 默认配置。相同 `key` 升级时按 Node 行为保留管理员配置：新默认配置作为底层，旧配置在顶层覆盖，不执行递归合并。已删除的顶层默认键如果仍存在于旧配置中会继续保留，管理员可以在配置更新时显式删除。

配置支持 Node 风格的 `@baseDir` 字符串替换，值为宿主应用工作目录。若配置顶层存在环境分支，则按 GoFrame 运行模式选择：

| GoFrame 模式 | 首选键 | 兼容键 |
| --- | --- | --- |
| `develop` | `@develop` | `@local` |
| `testing` | `@testing` | `@unittest` |
| `staging` | `@staging` | 无 |
| `product` | `@product` | `@prod` |

存在环境分支但没有当前模式对应对象时，配置校验失败，不静默使用空对象。替换和环境选择只产生运行时副本，不修改数据库中的原始配置。

### 7.4 制品完整性

宿主对完整上传字节计算 SHA-256。数据库记录 SHA-256、压缩大小和 Manifest。相同 SHA-256 的制品可复用，不重复保存；相同 `key` 和 `version` 但 SHA-256 不同的上传必须显式强制覆盖，否则拒绝，避免同版本内容漂移。

## 8. Go 插件 SDK

### 8.1 开发接口

插件作者只编写普通 Go 请求、响应和 Handler：

```go
type SendRequest struct {
   Mobile  string `json:"mobile"`
   Content string `json:"content"`
}

type SendResult struct {
   RequestID string `json:"requestId"`
}

func Send(ctx context.Context, request SendRequest) (SendResult, error) {
   config, err := coolplugin.Config[Config](ctx)
   if err != nil {
      return SendResult{}, err
   }
   _ = config

   return SendResult{RequestID: "example"}, nil
}

func Plugin() coolplugin.Definition {
   return coolplugin.Define(
      coolplugin.Method("send", Send),
   )
}
```

`Method` 使用泛型把 `func(context.Context, Req) (Res, error)` 转换为内部 JSON Handler。插件可以注册任意合法方法名，方法名匹配 `^[a-z][A-Za-z0-9]{0,63}$` 且在单个插件内唯一。SDK 不使用反射定位宿主 Service。

SDK 同时提供 `RawMethod`，用于输入或输出必须保持 `json.RawMessage` 的场景。业务插件优先使用类型化 `Method`。

插件可以注册一个 `Ready` 回调和一个 `Shutdown` 回调。`Ready` 在实例进入注册表前执行；`Shutdown` 在停止接收新调用并完成排空后执行。回调失败遵循生命周期错误规则。

### 8.2 脚手架和构建

`cool-plugin` CLI 提供：

```text
cool-plugin init
cool-plugin check
cool-plugin test
cool-plugin build
cool-plugin pack
```

脚手架包含插件业务包和一个由 CLI 维护的 WASM bridge。Bridge 导出固定 ABI 并调用业务包的 `Plugin()`，插件作者无需编写指针、线性内存或 `//go:wasmexport` 代码。

构建固定使用项目 Go 1.26 工具链：

```bash
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -trimpath -o plugin.wasm ./cmd/wasm
```

CLI 在打包前执行 Manifest 校验、Go 测试、WASM 构建、导入导出检查和本地 Runtime 冒烟测试。最终按稳定文件顺序和时间戳生成可复现 `.cool` ZIP。

### 8.3 宿主调用 API

底层 Manager 只接受 JSON：

```text
InvokeJSON(ctx, target, method, inputJSON) -> outputJSON, error
```

SDK 为宿主业务代码提供泛型包装：

```go
result, err := coolplugin.Invoke[SendRequest, SendResult](
   ctx,
   manager,
   "sms",
   "send",
   request,
)
```

`target` 先按插件 `key` 精确匹配；不存在该 `key` 时再按 `hook` 解析。禁止同一个字符串同时作为已安装插件 key 和其他插件的 hook，以消除调用歧义。

不提供公开的 `/invoke` HTTP 或 gRPC 接口。静态业务 Controller 负责身份、权限、DTO 和事务边界，再调用 Manager。

## 9. ABI

### 9.1 版本

ABI 名称固定为 `cool.plugin/v1`。Manifest 版本负责安装前兼容判断，WASM 导出的 `cool_abi_version` 再提供运行时交叉校验。ABI v1 的整数值为 `1`。

ABI 只使用 Wasm 标量、线性内存地址、长度和句柄，不传递 Go 指针、接口、字符串结果、数据库对象或宿主请求对象。

### 9.2 Guest 导出

WASM 模块必须导出：

```text
cool_abi_version() -> i32
cool_alloc(size: i32) -> i32
cool_free(pointer: i32, size: i32)
cool_init(invocationID: i64, configPointer: i32, configLength: i32) -> i64
cool_invoke(invocationID: i64,
            methodPointer: i32, methodLength: i32,
            inputPointer: i32, inputLength: i32) -> i64
cool_shutdown(invocationID: i64) -> i64
cool_response_pointer(responseHandle: i64) -> i32
cool_response_length(responseHandle: i64) -> i32
cool_response_drop(responseHandle: i64)
```

`cool_init`、`cool_invoke` 和 `cool_shutdown` 返回 Guest 响应句柄。宿主读取指针和长度后复制响应，并始终调用 `cool_response_drop`。输入由宿主调用 `cool_alloc` 分配、写入，并在调用结束后调用 `cool_free`。Guest 必须在响应释放前保持响应字节存活。

WASI reactor 实例化后，宿主先调用 `_initialize`，再调用任何 `cool_*` 导出。缺失导出、签名不符、重复初始化或无效内存范围均视为 ABI 错误。

### 9.3 Host 导入

Guest 从 `cool_host` 模块导入：

```text
call(invocationID: i64,
     operationPointer: i32, operationLength: i32,
     inputPointer: i32, inputLength: i32) -> i64
response_length(responseHandle: i64) -> i32
response_read(responseHandle: i64, targetPointer: i32, targetLength: i32) -> i32
response_drop(responseHandle: i64)
```

Host `call` 返回宿主响应句柄。Guest 先查询长度，分配自己的字节区，再调用 `response_read`，最后调用 `response_drop`。无论业务成功或失败，正常调用都返回统一 JSON Envelope；无效句柄、越界内存和 Runtime 已关闭属于 Trap 或 ABI 错误。

### 9.4 Envelope

成功响应：

```json
{
  "ok": true,
  "data": {}
}
```

失败响应：

```json
{
  "ok": false,
  "error": {
    "code": "PLUGIN_INVALID_INPUT",
    "message": "参数无效"
  }
}
```

异常堆栈、宿主路径、配置内容和内部错误不进入返回 Envelope，只写入脱敏日志并关联 Trace ID、插件 key、版本和调用 ID。

## 10. Host API

Host API 使用稳定操作名和 JSON Schema。首期提供：

| 操作前缀 | 能力 |
| --- | --- |
| `config.get` | 读取当前实例的运行时配置副本 |
| `cache.get/set/delete` | 使用插件 key 自动命名空间隔离的缓存 |
| `http.do` | 发起带超时和响应大小限制的 HTTP/HTTPS 请求 |
| `file.read/write/delete/list` | 操作插件数据目录或配置指定路径 |
| `log.write` | 写入统一宿主日志 |
| `plugin.call` | 按 key 或 hook 调用其他插件方法 |
| `context.get` | 读取 Trace ID、调用来源和已认证身份摘要 |
| `host.call` | 调用宿主显式登记的业务适配器 |

WASI 只预打开插件自己的持久数据目录并挂载为 `/data`，不继承宿主环境变量、标准输入或宿主根目录。网络访问通过 `http.do`；标准 Go 插件不能依赖 WASI Socket。

`host.call` 不是 Service Locator。每个宿主适配器必须由 Go 代码显式实现固定操作名、请求和响应，作为 Plugin 模块的静态构造依赖装配。适配器不得用反射或字符串从 Application Graph 查找任意组件。

插件间调用继承原 `context.Context` 截止时间、取消、Trace ID 和身份摘要。调用链记录插件 key；检测到环路立即返回 `PLUGIN_CALL_CYCLE`。默认最大嵌套深度为 8，超过后返回 `PLUGIN_CALL_DEPTH_EXCEEDED`，避免递归耗尽实例和内存。

## 11. 实例模型

### 11.1 编译缓存

每个 SHA-256 制品只调用一次 `CompileModule`，结果由 Manager 缓存并引用计数。没有活跃版本或调用引用时关闭 CompiledModule。缓存键不能只使用插件 key 或版本。

### 11.2 非单例

`singleton: false` 时，每次调用从已编译模块创建独立实例，依次执行 `_initialize`、`cool_init`、`cool_invoke`、`cool_shutdown` 和关闭实例。初始化配置来自该调用开始时解析到的版本快照。

不同调用可以并发，但受每插件和全局实例上限约束。实例不跨调用保留 Guest 内存状态；持久状态应使用缓存、插件数据目录或宿主业务 API。

### 11.3 单例

`singleton: true` 时，版本加载阶段创建一个实例并执行 `_initialize` 和 `cool_init`。同一实例的调用串行执行，匹配 Node 单线程单例的状态语义，也避免并发进入标准 Go WASM Runtime。

升级、配置更新、禁用或关闭时停止接收新调用，等待旧调用完成后执行 `cool_shutdown`。单例调用其他插件时不得形成调用环，否则会被调用链检查拒绝，不等待实例锁。

### 11.4 调用快照

每次调用在开始时取得不可变目标快照，包含插件 ID、key、hook、版本、SHA-256、配置和实例引用。热切换只替换新调用使用的快照；旧调用持有的快照完成后才释放，避免升级中断正在执行的业务。

## 12. 生命周期和热切换

### 12.1 宿主生命周期

Plugin Manager 实现现有组件生命周期：

- `OnInit`：创建 Runtime、注册 WASI 和 Host Module、读取插件状态；
- `OnStart`：加载全部期望启用的插件，启动数据库状态同步循环；
- `OnStop`：停止新调用，取消同步循环，排空并逆序关闭实例、编译缓存和 Runtime。

任一启动时启用插件加载失败不会让整个宿主启动失败。该插件本节点状态为 `error`，调用返回不可用错误并记录诊断；其他插件与主服务继续启动。Runtime 自身或插件表读取失败属于 Plugin Manager 启动失败，由 Application Host 按现有生命周期规则回滚。

### 12.2 本地状态

数据库 `status` 表示期望状态，节点内存状态使用：

```text
loading -> enabled
        -> error
enabled -> draining -> disabled
enabled -> replacing -> enabled
```

本地状态不覆盖数据库期望状态。加载失败后节点按同步周期重试；相同 revision 和相同确定性错误使用有上限退避，避免持续高频编译。

### 12.3 安装

安装流程固定为：

1. 认证超管并将 Multipart 文件流式写入临时文件；
2. 校验压缩大小、ZIP 路径、条目数量和累计解压大小；
3. 严格解析 Manifest，校验 key、hook、版本和宿主版本；
4. 计算完整制品 SHA-256；
5. 读取并检查 `plugin.wasm` 导入、导出和 ABI；
6. 编译并使用候选配置创建暂存实例；
7. 执行 `_initialize` 和 `cool_init`；期望启用的单例候选保持存活，非单例或期望禁用的候选验证成功后执行 `cool_shutdown` 并关闭；
8. 在数据库事务中保存制品和插件信息，递增该插件 revision；
9. 当前节点按新 revision 原子发布快照；期望启用的单例直接提升步骤 7 的候选实例，不再次执行 `_initialize`、`cool_init` 或 `Ready`，期望启用的非单例发布已验证的编译模块，期望禁用时只更新本地版本元数据；
10. 删除临时文件。

步骤 1 至 7 失败不修改数据库和当前运行版本。步骤 8 失败关闭候选并保持旧版本。数据库提交后若当前节点发布失败，则保留旧快照、进入 `error` 并按 revision 重试；其他节点不受影响。

全新插件默认 `status = 1`，与 Node 安装行为一致。相同 key 安装视为升级，并保留旧 status 和管理员配置。

### 12.4 启用和 Hook 切换

启用前必须完成候选实例初始化。普通插件初始化成功后更新 status 和 revision，再发布快照。

Hook 插件启用时，在同一数据库事务中把其他相同 hook 的插件设为禁用，再启用目标并递增涉及记录的 revision。单节点快照替换在事务提交后完成。短暂多节点同步窗口内，各节点可能仍使用旧实现，但单个节点的调用始终解析到一个完整版本，不出现空实现或半初始化实例。

内置默认能力不是数据库插件。只有宿主为某个 Hook 显式注册了静态默认实现，Resolver 才在该 Hook 没有启用插件时回退；例如宿主为 `upload` 注册本地上传实现后，未启用对应插件时使用本地上传，启用后使用 WASM 实现。没有显式默认实现的 Hook 在未解析到插件时返回 `PLUGIN_NOT_FOUND`。

### 12.5 配置更新

配置必须是 JSON object。更新时使用新配置创建候选实例并执行初始化；成功后在事务中保存配置、递增 revision 并原子替换快照。失败时数据库配置和当前实例均不改变。

配置日志和错误响应必须脱敏。`page` 和 `list` 不返回配置值；`info` 只有超管可以获得完整配置，其他角色省略配置字段或只返回不可逆脱敏值。普通日志、EPS 和健康检查不得返回配置值。

### 12.6 升级

升级先完成新制品的全部安装校验和候选初始化，再提交新的 `activeArtifactID`、Manifest、版本、配置和 revision。提交后新调用使用新快照，旧调用结束后旧实例执行 shutdown。

新版本在提交前失败时继续使用旧版本。提交后单个节点加载失败时该节点继续保留旧快照并报告新 revision 加载错误，后续同步重试。旧 Artifact 不立即删除，避免其他节点尚未读取和便于审计；制品清理由单独的超管维护动作处理，不属于安装热路径。

### 12.7 禁用和卸载

禁用先提交 status 和 revision，再让节点停止把新调用路由到该插件，排空活跃调用并关闭实例。超过关闭期限时取消实例 Context 并强制关闭。

卸载删除插件信息和全部制品记录，节点检测到记录消失后执行相同排空。插件 `/data` 业务数据默认保留；清理业务数据必须是超管明确选择的独立动作，不能随普通卸载隐式删除。

## 13. 多实例一致性

数据库是插件期望状态和制品的事实来源，本机文件只缓存按 SHA-256 命名的制品。每个 Plugin Manager 默认每 5 秒读取全部插件的 `id`、`keyName`、`status`、`activeArtifactID` 和 `revision`，与本地快照比较：

- revision 增加时拉取制品、验证 SHA-256 并热切换；
- status 变为禁用时排空并关闭；
- 记录消失时卸载本地实例；
- 节点重启时从数据库恢复全部期望启用插件；
- 加载失败时保留该节点旧快照并记录目标 revision 与错误。

该协议保证最终一致，不承诺所有节点同时切换。事务提交、共享制品和单节点原子快照保证任何节点只使用完整旧版本或完整新版本。已有 Event 或 Outbox 后续可以发送即时变更通知以缩短延迟，但数据库轮询仍是恢复和一致性兜底，不能依赖单次通知正确性。

## 14. 数据模型

### 14.1 `plugin_info`

表名固定为 `plugin_info`，使用框架通用 `id/createTime/updateTime`：

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `name` | `string` | 非空，最长 100 |
| `description` | `string` | 非空，最长 500 |
| `keyName` | `string` | 非空，最长 64，唯一索引 |
| `hook` | `*string` | 可空，最长 64，普通索引 |
| `singleton` | `bool` | 非空 |
| `readme` | `*string` | 可空文本 |
| `version` | `string` | 非空，最长 64 |
| `logo` | `[]byte` | 可空字节内容 |
| `author` | `string` | 非空，最长 100 |
| `status` | `int32` | `0` 禁用，`1` 启用，默认 `1` |
| `manifest` | `map[string]any` | JSON，当前 Manifest 快照 |
| `config` | `map[string]any` | JSON，管理员原始配置 |
| `runtimeABI` | `string` | 非空，最长 64 |
| `activeArtifactID` | `uint64` | 当前制品 ID，普通索引 |
| `revision` | `uint64` | 每次期望状态变化递增 |

不使用数据库外键，保持现有模块跨数据库做法。Service 在事务中维护 Info 与 Artifact 一致性。

### 14.2 `plugin_artifact`

| JSON/列名 | Go 类型 | 约束 |
| --- | --- | --- |
| `pluginId` | `uint64` | 所属插件，普通索引 |
| `version` | `string` | 非空，最长 64 |
| `sha256` | `string` | 64 位小写十六进制，唯一索引 |
| `size` | `int64` | 原始 `.cool` 字节数 |
| `manifest` | `map[string]any` | JSON，不可变 Manifest |
| `packageData` | `[]byte` | 完整 `.cool` 制品 |

Artifact 创建后不可更新。升级只新增 Artifact 并切换 `activeArtifactID`。普通插件分页查询不得读取 `packageData`。

## 15. HTTP 契约

管理接口前缀为 `/admin/plugin/info`：

| 路径 | 方法 | 权限 | 行为 |
| --- | --- | --- | --- |
| `/install` | POST Multipart | 仅超管 | 校验并安装或升级 `.cool` |
| `/page` | POST | `plugin:info:page` | 查询插件分页，不返回制品字节和配置值 |
| `/list` | POST | `plugin:info:list` | 查询插件列表，不返回制品字节和配置值 |
| `/info` | GET | `plugin:info:info` | 查询详情；仅超管返回完整配置，其他角色省略或脱敏 |
| `/update` | POST | 仅超管 | 只更新 `status` 或 `config` |
| `/delete` | POST | 仅超管 | 卸载插件，默认保留数据目录 |

安装接口不复制 Node 当前的 `IGNORE_TOKEN` 行为。服务端必须从认证上下文再次判断超管身份，不能只依赖菜单权限。`update` 不允许修改 key、hook、version、Manifest、Artifact 或作者元数据；这些值只能来自经过校验的安装包。

接口响应继续使用现有 Cool 成功和错误 Envelope。安装覆盖提示保留 Node 的交互语义：同 key 或同版本不同内容时，首次返回需要确认的检查结果；超管携带 `force=true` 后才继续。`force` 不能绕过 ZIP、Manifest、ABI、宿主版本或候选初始化校验。

## 16. 资源限制

Plugin 模块配置提供以下默认值：

| 配置 | 默认值 | 说明 |
| --- | --- | --- |
| `maxPackageBytes` | 32 MiB | Multipart 压缩包上限 |
| `maxUnpackedBytes` | 64 MiB | ZIP 累计解压上限 |
| `maxEntries` | 256 | ZIP 条目上限 |
| `maxPayloadBytes` | 4 MiB | 单次 ABI 输入或输出上限 |
| `callTimeout` | 30 秒 | 没有更短上游期限时的调用期限 |
| `initTimeout` | 15 秒 | 初始化期限 |
| `shutdownTimeout` | 10 秒 | 单实例排空和关闭期限 |
| `memoryLimitPages` | 4096 | 每实例 256 MiB Wasm 线性内存上限 |
| `maxInstancesPerPlugin` | 8 | 非单例并发实例上限 |
| `maxInstances` | 64 | 单节点全部插件实例上限 |
| `syncInterval` | 5 秒 | 多节点数据库同步周期 |
| `maxCallDepth` | 8 | 插件调用链深度上限 |

上游 Context 截止时间早于模块默认值时使用更短期限。等待实例配额也计入同一调用期限。配置值必须为正且满足内部关系，非法配置使 Plugin 模块构造失败。

## 17. 错误模型

稳定插件错误码至少包括：

| 错误码 | 含义 |
| --- | --- |
| `PLUGIN_NOT_FOUND` | key 或 hook 不存在 |
| `PLUGIN_DISABLED` | 插件期望或本地状态不可调用 |
| `PLUGIN_METHOD_NOT_FOUND` | Guest 未注册方法 |
| `PLUGIN_INVALID_INPUT` | 请求 JSON 或 DTO 无效 |
| `PLUGIN_INVALID_OUTPUT` | 响应 JSON 或目标 DTO 无效 |
| `PLUGIN_ABI_UNSUPPORTED` | ABI 名称、版本或导出签名不兼容 |
| `PLUGIN_INIT_FAILED` | 初始化失败 |
| `PLUGIN_TIMEOUT` | 等待实例、调用或宿主能力超时 |
| `PLUGIN_TRAP` | WASM Trap、Panic 或非法内存访问 |
| `PLUGIN_RESOURCE_EXHAUSTED` | 内存、实例或负载大小超限 |
| `PLUGIN_HOST_CALL_FAILED` | Host API 执行失败 |
| `PLUGIN_CALL_CYCLE` | 插件间调用形成环路 |
| `PLUGIN_CALL_DEPTH_EXCEEDED` | 插件间调用超过最大深度 |

调用方可识别错误码，不依赖错误消息文本。宿主使用现有 `exception` 模型映射 HTTP/gRPC 错误；底层错误保留堆栈用于日志，但不向客户端泄露。

## 18. Node 行为对应

| Node 插件能力 | Go WASM 对应 |
| --- | --- |
| `Plugin` 固定导出 | 固定 `cool.plugin/v1` ABI 导出 |
| 任意实例方法 | `Method` / `RawMethod` 任意注册 |
| `pluginService.invoke` | `InvokeJSON` 和泛型 `Invoke` |
| `key` | key Resolver |
| `hook` | 单活 Hook Resolver，仅在宿主显式注册默认实现时回退 |
| `singleton` | 持久串行实例或按调用实例 |
| `ready()` | Guest `Ready`，由 `cool_init` 执行 |
| `pluginInfo.config` | `config.get` 和类型化 `Config[T]` |
| Midway Cache | 带插件命名空间的 `cache.*` |
| `pluginService` 调其他插件 | `plugin.call` |
| `ctx` / `app` | `context.get` 和显式 Host API |
| `@baseDir` | 运行时配置替换 |
| TypeScript `.d.ts` | Go 请求、响应类型和泛型 Handler |
| `eval` 热加载 | wazero 编译、实例化和快照热切换 |
| PM2 全局事件 | 数据库 revision 同步，通知只作加速 |

Go 版不兼容 Node 源码，但保持插件作者和调用方可观察的核心能力。Node 可以绕过正式契约直接访问 Midway 容器，Go 版不复制这一内部耦合；需要的宿主能力必须成为显式 Host API。

## 19. 测试与验收

### 19.1 SDK 和 ABI

- 示例 Go 插件可由标准 Go 1.26 编译为 WASI reactor；
- CLI 生成的 Bridge 导出完整且签名正确；
- 类型化 Method、RawMethod、Ready、Shutdown 和错误 Envelope 正确；
- 输入、输出内存严格遵守 alloc/free 与 response/drop 所有权；
- Host 响应句柄在成功、错误、取消和 Trap 后均不泄漏；
- ABI 版本、缺失导出、额外非法导入和越界内存测试完整。

### 19.2 安装和包安全

- 合法 `.cool` 可安装并立即调用，无需重启；
- 缺失文件、重复文件、绝对路径、路径穿越、符号链接、ZIP Bomb、超大包和错误 SHA 被拒绝；
- Manifest 未知字段、非法 key/hook、错误 SemVer、宿主版本不足和 ABI 不兼容被拒绝；
- 强制安装只绕过覆盖确认，不绕过结构和运行校验；
- 相同 key 升级保留 status 和顶层管理员配置。

### 19.3 生命周期

- 启用、禁用、配置更新、Hook 替换和卸载即时生效；
- 新版本初始化失败时旧版本继续服务；
- 热切换后新调用使用新版本，已开始调用完成于旧版本；
- 单例串行且保留内存状态，非单例调用互相隔离；
- Shutdown 超时会取消并关闭实例；
- 调用环路和超深调用链稳定失败且不死锁。

### 19.4 故障和资源

- Guest Panic、WASM Trap、无限循环、取消、内存增长失败和超大响应不终止宿主；
- 实例配额在并发和取消下不泄漏；
- Host API 错误携带 Trace ID 且日志脱敏；
- Race Test 覆盖安装、调用、热切换和关闭并发。

### 19.5 多实例和数据库

- 两个 Manager 共享数据库时最终加载相同 revision 和 SHA-256；
- 单节点加载失败不改变其他节点运行版本；
- 节点离线后重新启动可恢复当前期望状态；
- 删除、禁用和 Hook 切换在轮询后收敛；
- MySQL、PostgreSQL 和 SQLite 的 Schema、JSON、字节制品和事务行为一致。

### 19.6 门禁

```bash
go test ./... -count=1
go test -race ./cool-next/plugin/... ./modules/plugin/... -count=1
go vet ./...
make check
git diff --check
```

端到端验收必须真实执行以下链路，不允许只 Mock Runtime：

```text
构建示例 Go 插件
-> 打包 .cool
-> 调用后台安装服务
-> 无重启调用插件方法
-> 上传新版本并保持旧请求
-> 验证新请求切换
-> 禁用并确认调用失败
```

## 20. 实现分解

该平台按四个可独立验收的阶段实现：

1. ABI、Guest SDK、`cool-plugin` CLI 和最小 wazero Host，证明标准 Go WASI reactor 可调用；
2. `modules/plugin` 数据模型、制品存储、安装校验和管理 HTTP 契约；
3. 实例模型、Host API、配置更新、热切换、资源限制和多节点同步；
4. 将现有本地上传作为 `upload` 静态默认实现，并增加一个 WASM 示例 Hook，验证真实业务替换链路。

每阶段完成后运行对应单元、Race 和端到端测试。不得先把现有静态 Provider 图改造成通用运行时容器，也不得在 ABI 验证完成前迁移现有业务调用。

## 21. 完成标准

1. 第三方开发者只使用 Go、Guest SDK 和 CLI 即可构建合法 `.cool`；
2. 超管上传插件后不重启主服务即可调用任意注册方法；
3. key、hook、singleton、配置、就绪、插件间调用与 Node 核心语义对应；
4. 安装、升级、配置更新和 Hook 替换不会向调用方暴露半初始化版本；
5. 插件错误和资源耗尽不会终止宿主进程；
6. 多节点可从数据库事实来源自动收敛；
7. 不引入运行时业务模块扫描、字符串 DI、反射 Service Locator 或动态路由；
8. 三种数据库、Race Test、Vet、生成检查和完整项目测试通过。
