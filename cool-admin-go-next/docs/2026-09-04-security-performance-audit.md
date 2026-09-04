# cool-admin-go-next 安全与性能审计报告

审计日期：2026-09-04

审计范围：`cool-admin-go-next` Go 服务端源码、模块配置、数据库访问、认证授权、HTTP/gRPC 传输、文件上传、任务调度、Session 存储和 Go 依赖。

## 1. 结论

本次审计原确认 4 项应优先处理的安全问题，其中验证码泄漏已在当前工作区修复：

1. 验证码答案包含在客户端 SVG 响应中，验证码可被直接绕过（已修复）；
2. 公开 HTML 接口原样输出数据库内容，存在存储型 XSS 风险；
3. 登录和验证码公开接口没有应用层资源限制，存在撞库与内存消耗风险；
4. 当前 Go 工具链及依赖存在 11 个由 `govulncheck` 确认可达的已知漏洞。

原有 2 项中长期容量和并发风险，其中 Redis 用户 Session 撤销全量扫描已在当前工作区修复，尚余任务执行租约过期后可能重复执行。

未发现外部输入直接拼接 SQL、运行时命令注入、明显 SSRF 或应用代码造成的上传目录穿越。生产传输安全是否成立取决于反向代理、TLS、网络隔离和超时配置。

开发环境中的 `admin/123456`、本地 MySQL `root:123456`、`initDB: true` 和 `initMenu: true` 符合当前开发用途，不作为漏洞。前提是生产环境不加载 `manifest/config/config.local.yaml`。

## 2. 已确认问题

### SEC-01：验证码答案随响应泄漏（已修复）

严重级别：高

修复状态：当前工作区已将文本型 SVG 替换为不含明文的 SVG Path，保留一条弱化干扰线并移除密集噪点；同时增加 MIME、尺寸、颜色、缓存和一次性消费回归测试。以下证据描述修复前的根因。

证据：

- `modules/base/service/captcha.go:256` 将验证码写入 `<text visibility="hidden">...</text>`；
- `modules/base/service/captcha_test.go:17` 使用正则从返回的 SVG 中提取验证码；
- `modules/base/controller/admin/open.go:113` 将验证码接口注册为无需 Token 的公开接口。

根因：验证码答案作为隐藏文本嵌入 SVG。`visibility="hidden"` 只影响显示，不会从响应内容中移除文本。

攻击方式：请求 `/admin/base/open/captcha`，解码响应中的 Base64 SVG，读取隐藏 `<text>` 内容，再使用该验证码调用登录接口。整个过程无需图像识别。

影响：验证码不能阻止自动化登录尝试，所有依赖验证码抵抗机器请求的保护均失效。

修复建议：

1. 删除隐藏答案节点；
2. 测试通过注入固定随机源或检查缓存中的预期值验证逻辑，不从响应提取答案；
3. 增加回归测试，断言 SVG 不包含明文验证码及可机器读取的等价元数据。

### SEC-02：公开原始 HTML 存在存储型 XSS 风险

严重级别：高，若参数只能由完全可信管理员维护且页面不在认证主域使用，可下调

证据：

- `modules/base/controller/admin/open.go:95` 将 `/admin/base/open/html` 注册为忽略 Token 的公开接口；
- `modules/base/service/param.go:104` 将数据库中的 `record.Name` 和 `record.Data` 直接替换进 HTML；
- `cool-next/core/gnctrl/response.go:143` 以 `text/html` 原样写入响应；
- 该公开 HTML 路径未使用 `ParamService.AppDataByKey` 的 `allowKeys` 检查。

根因：数据库内容被当作可信 HTML 输出，公开路由又没有键名允许列表、HTML 清洗或浏览器隔离策略。

攻击方式：具备参数新增或修改权限的账号写入 `<script>`、事件属性或危险链接。当管理员在同源环境访问对应公开 URL 时，脚本在站点来源下执行。

影响：可能读取同源数据、代替管理员发起请求、篡改页面或诱导用户执行操作。实际影响取决于 Token 存储方式和浏览器安全策略。

修复建议：

1. 不需要富文本时返回纯文本或 JSON，并对标题和正文统一转义；
2. 需要富文本时，对标签、属性、协议和 URL 使用严格允许列表清洗；
3. 公开接口复用 `allowKeys`，只允许显式声明的参数键；
4. 增加严格 CSP；高风险富文本优先部署到不携带后台认证信息的独立来源。

### SEC-03：登录无速率限制，验证码缓存无容量上限

严重级别：中高

证据：

- `modules/base/controller/admin/open.go:104` 和 `modules/base/controller/admin/open.go:113` 暴露登录及验证码接口；
- `modules/base/service/captcha.go:49` 使用进程内 `gcache.New()`；
- 验证码设置 TTL，但没有总条目数、客户端配额或生成频率限制；
- 全仓未发现登录限流器、账号退避或 `429 Too Many Requests` 处理。

根因：公开认证入口只校验单次请求是否合法，没有控制单位时间内的请求数量和服务端状态增长。

攻击方式：攻击者可持续生成验证码增加内存占用；修复 SEC-01 后，仍可通过并发请求进行密码猜测或撞库。

影响：账号攻击、CPU 消耗、缓存膨胀和服务可用性下降。

修复建议：

1. 登录同时按账号和可信客户端 IP 限流，连续失败使用指数退避；
2. 验证码接口限制单位时间生成次数和未消费数量；
3. 设置验证码缓存容量上限，并在达到上限时拒绝或淘汰旧条目；
4. 应用直接暴露公网时在应用层实现；已有可信网关时也应保留账号维度限制；
5. 只有在正确配置可信代理后才能使用转发 IP 请求头作为限流依据。

### SEC-04：Go 工具链和依赖存在已知可达漏洞

严重级别：高；部分条目受功能开关或本地攻击前提限制

当前版本：

- `go.mod:5`：Go toolchain `1.26.4`；
- `go.mod:19`：`google.golang.org/grpc v1.64.1`；
- `go.mod:52`：`go.opentelemetry.io/otel v1.38.0`；
- `go.mod:54`：`go.opentelemetry.io/otel/sdk v1.38.0`。

`govulncheck ./...` 确认代码调用链触达以下 11 个漏洞：

| 漏洞 | 组件 | 主要风险 | 修复版本 |
| --- | --- | --- | --- |
| GO-2026-6091 | `html/template` | JavaScript 正则上下文跟踪错误 | Go 1.26.6 |
| GO-2026-6090 | `crypto/tls` | TLS 握手消息资源消耗 | Go 1.26.6 |
| GO-2026-6089 | `net/http` | 未加密 HTTP/2 检查未正确应用请求头超时 | Go 1.26.6 |
| GO-2026-6088 | `encoding/xml` | XML 深度递归导致资源耗尽 | Go 1.26.6 |
| GO-2026-6061 | gRPC-Go | xDS RBAC 和 HTTP/2 服务端问题 | gRPC 1.82.1 |
| GO-2026-5972 | `encoding/asn1` | ASN.1 深度递归导致资源耗尽 | Go 1.26.6 |
| GO-2026-5856 | `crypto/tls` | ECH 隐私泄漏 | Go 1.26.5 |
| GO-2026-5506 | OpenTelemetry | 多值 baggage 头导致过量内存分配 | OTel 1.41.0 |
| GO-2026-4970 | `os` | `os.Root` 在特定符号链接和尾斜杠组合下路径逃逸 | Go 1.26.5 |
| GO-2026-4762 | gRPC-Go | 缺少前导斜杠的 `:path` 可造成授权绕过 | gRPC 1.79.3 |
| GO-2026-4394 | OpenTelemetry SDK | 特定 PATH 劫持条件下执行任意程序 | OTel SDK 1.40.0 |

风险边界：

- 主配置当前关闭 gRPC，因此 gRPC 远程漏洞在该配置下不可达；启用 gRPC 后风险成立；
- OpenTelemetry PATH 劫持通常需要本地环境或进程启动环境控制能力；
- 标准库漏洞的符号调用链已被扫描器确认，但每项能否被远程利用仍取决于协议、输入和部署方式；
- `modules/base/service/upload.go` 对 Web 输入执行了文件名和路径校验，这会降低 `GO-2026-4970` 在当前上传接口上的可利用性，但升级标准库仍是正确修复方式。

修复建议：

1. Go toolchain 升级到 `1.26.6` 或更高补丁版本；
2. gRPC-Go 升级到 `1.82.1` 或更高兼容版本；
3. OpenTelemetry API 和 SDK 统一升级到 `1.41.0` 或更高兼容版本；
4. 升级后执行 `go mod tidy`、完整测试、`go vet ./...` 和 `govulncheck ./...`；
5. 在完整测试恢复通过前，不应只更新依赖后直接发布。

## 3. 并发与性能问题

### PERF-01：固定任务租约可能造成重复执行

严重级别：中；仅单实例且任务始终短于 5 分钟时较低

证据：

- `modules/task/service/executor.go:17` 将执行租约固定为 5 分钟；
- `modules/task/service/executor.go:106` 在取得租约后同步调用任务；
- `modules/task/service/executor.go:164` 写入固定过期时间；
- 执行期间没有续租，任务调用也没有独立执行超时；
- `modules/task/service/executor.go:203` 释放租约时没有校验租约所有者或本次执行令牌。

根因：租约只记录过期时间，没有持有者、执行令牌和心跳续租机制。

影响：多实例环境中，任务执行超过 5 分钟后可能被其他实例或手动执行入口再次抢占。存在外部副作用的任务可能重复扣款、重复发货或重复通知。

修复建议：

1. 明确规定任务最大执行时间，并通过 Context 超时传给 Callable；
2. 长任务使用执行令牌和周期性续租；
3. 释放租约和写入结果时校验执行令牌，旧执行者不得清除新租约；
4. 所有具有外部副作用的任务使用业务幂等键。

### PERF-02：Redis 用户 Session 撤销扫描全部 Session（已修复）

严重级别：中低，随在线 Session 数量增加而上升

修复状态：当前工作区已将 Redis Session 存储切换到 `v2` 命名空间，使用 Session 定位 Key 和按用户组织的 Hash。`RevokeUsers` 现在只 `UNLINK` 目标用户 Hash，不再执行 `SCAN`，前台复杂度由 O(全部 Session) 降为 O(目标用户数)，目标用户 Session 的内存由 Redis 后台释放。保存、刷新和单 Session 撤销通过 Lua 原子维护两类 Key；旧 Session 统一失效并按原 TTL 自然清理。

以下证据描述修复前的根因：

- `cool-next/auth/redis.go:240` 的 `RevokeUsers` 用于按用户撤销 Session；
- `cool-next/auth/redis.go:249` 先调用 `scanKeys`；
- `cool-next/auth/redis.go:269` 使用 `SCAN` 遍历全部 Session Key；
- 所有 Key 被累计到 Go 切片后，再每 100 条执行批量检查和撤销。

根因：Session 仅按 Session ID 存储，没有维护用户到 Session ID 的反向索引。

影响：撤销少量用户权限的成本仍接近 O(全部 Session)，会增加 Redis CPU、网络流量、应用内存和权限管理接口延迟。

验证：鉴权定向测试、竞态测试和真实 Redis 保存、刷新、读取、按用户撤销验证均通过。测试同时覆盖 `v2` 命名空间隔离、用户 Hash 最晚过期时间、跨用户隔离和残留定位 Key 惰性清理。

## 4. 条件性部署风险

### OPS-01：传输安全依赖外部基础设施

证据：

- `cool-next/core/gnhttp/config.go:14` 的 HTTP 默认监听地址是 `0.0.0.0`；
- `cool-next/grpc/config.go:14` 的 gRPC 默认监听地址是 `0.0.0.0`；
- `cool-next/grpc/transport.go:198` 创建 gRPC Server 时未配置 TLS credentials；
- `cool-next/core/gnhttp/transport.go:71` 只显式设置客户端请求体大小，没有显式设置写超时。

这不一定是代码漏洞。如果生产环境由可信反向代理终止 TLS、限制请求速率和连接时间，且 gRPC 只在可信内网或 mTLS 网络开放，则风险已经在部署层控制。

上线检查项：

1. 外网入口强制 HTTPS，并正确配置 HSTS；
2. 后端端口不直接暴露公网；
3. 代理设置请求头、读、写、空闲和最大请求体超时；
4. gRPC 启用时使用 mTLS 或严格网络访问控制；
5. 为 gRPC 设置消息大小、并发流和连接年龄限制；
6. 只信任来自已配置代理地址的转发 IP 请求头。

## 5. 未发现的高风险模式

### SQL 注入

未发现外部输入直接拼接 SQL。查询值使用参数绑定，排序字段经过实体元数据和允许字段校验，业务代码没有调用动态 `RawWhere`。唯一业务原生 SQL 使用固定语句和绑定参数。

### 命令注入和 SSRF

未发现 HTTP 业务接口调用 `os/exec` 或使用用户 URL 发起服务器端请求。`cmd/cool` 中的命令执行属于本地 CLI 代码，不是远程业务入口。

### 文件上传和目录穿越

上传文件使用随机名称、大小限制、`os.OpenRoot`、常规文件检查和受控响应类型。文件响应包含 `X-Content-Type-Options: nosniff`，不可信类型强制下载。除应升级标准库修复 `GO-2026-4970` 外，未发现应用层路径穿越。

### 认证和权限

JWT 固定验证签名算法，并检查 issuer、audience、有效期和 key ID；签名密钥要求至少 32 字节。Refresh Token 采用原子轮换并处理重放撤销，密码和权限变化会撤销 Session。错误响应经过脱敏，不直接输出内部堆栈或连接凭据。

### 数据库索引

用户角色、角色菜单等关联表已经包含正向唯一索引和反向查询索引。所有业务实体自动包含 `createTime` 和 `updateTime` 索引。未发现需要作为安全问题报告的明显缺失索引。

## 6. 验证结果

| 检查 | 结果 |
| --- | --- |
| Go 文件数量 | 328 |
| 测试文件数量 | 110 |
| `go vet ./...` | 审计时通过；当前复验被未完成的 `cmd/cool-plugin` 构建错误阻断 |
| Base、认证、CRUD 定向测试 | 通过；Redis Session 反向索引另通过竞态测试和真实 Redis 验证 |
| Task Service 测试 | 当前没有测试文件 |
| `govulncheck ./...` | 发现 11 个代码调用链可达漏洞 |
| `go test ./... -count=1` | 未通过，失败集中在 `cool-next/codegen` 和未完成的 `cmd/cool-plugin` |

完整测试失败不是本次修复造成。`cool-next/codegen` 测试样例仍使用旧别名 `gnmodule`、`gnentity`、`gnauth`、`gnoutbox` 和 `gncrud`，而对应导入已改为默认包名，造成 `undefined` 与未使用导入错误；当前工作区的 `cmd/cool-plugin` 还引用尚未定义的 `artifactProject` 和 `newProjectCommands`。在依赖升级前建议先修复这些测试和构建回归，恢复可靠的发布验证基线。

## 7. 建议修复顺序

1. 已完成：将明文 SVG 替换为 SVG Path 并增加防泄漏回归测试；
2. 限制公开 HTML 键名并完成 HTML 清洗或输出隔离；
3. 为登录和验证码增加账号、IP、缓存容量三层资源限制；
4. 修复 Codegen 测试回归；
5. 升级 Go、gRPC 和 OpenTelemetry，重新执行完整安全扫描；
6. 根据实际任务时长决定采用执行超时还是租约续期；
7. 已完成：使用 Session 定位 Key 和用户 Hash 替代 Redis Session 全量扫描；
8. 将传输层上线检查项固化到部署配置和验收流程。

## 8. 审计边界

本报告基于源码静态检查、Go 工具链检查和本地测试。未执行针对真实环境的渗透测试，也未验证生产数据库权限、Redis ACL、网络安全组、反向代理、TLS 证书、容器权限和密钥管理系统。条件性部署风险应结合实际基础设施配置复核。
