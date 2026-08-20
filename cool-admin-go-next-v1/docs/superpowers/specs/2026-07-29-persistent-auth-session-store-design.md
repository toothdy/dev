# Cool Admin Go Next 认证会话持久化设计

## 1. 背景

当前默认应用在每次启动时创建 `MemorySessionStore`。登录后的 JWT 虽然仍在有效期内，但鉴权中间件还会根据 token 中的 session ID 查询服务端 session。进程重启会清空内存 session，因此旧 token 会被拒绝。

项目已有 `RedisSessionStore` 实现和 `redis.default` 配置，但默认应用装配流程尚未使用它。JWT 签名密钥已持久化到配置文件，不是本问题的原因。

## 2. 目标

1. `redis.default` 存在时，默认使用 Redis 保存认证 session。
2. Redis session 在服务正常重启后保留，未过期且未被撤销的 token 继续有效。
3. 只有完全没有 `redis.default` 配置时才回退到内存 session。
4. Redis 已配置但配置无效或无法连接时，应用启动失败，不静默降级。
5. 保留 `Options.SessionStore` 显式注入能力，便于测试和嵌入式使用。

## 3. 非目标

1. 不改变 JWT 结构、签发方式、有效期或 refresh 轮换协议。
2. 不改变登录、退出、SSO 和授权变更后撤销 session 的现有语义。
3. 不引入数据库 session 表或第二套 Redis 客户端实现。
4. 不在 Redis 故障时自动切换为内存 session。

## 4. 设计

### 4.1 责任边界

默认 session store 的选择放在 `cool/app` 应用装配层。`cool/auth` 继续只提供 `SessionStore` 接口、内存实现和 Redis 实现，不直接读取全局配置。

新增一个默认 session store 工厂，职责仅包括：

1. 读取 `redis.default`。
2. 配置不存在时返回 `MemorySessionStore`。
3. 配置存在时获取 GoFrame 的默认 Redis 单例。
4. 在启动期执行 `PING` 验证配置和连接可用性。
5. 使用现有 `auth.NewRedisSessionStore` 构造 session store。

GoFrame Redis 单例由框架管理，应用不为认证 session 单独创建或关闭连接池。
GoFrame 官方 Redis adapter 在应用包中注册，adapter 与项目现有 Task Redis 路径共用 `go-redis/v9` 实现。

### 4.2 装配顺序

`BuildWithContext` 按以下顺序确定 session store：

1. `Options.SessionStore` 非空时直接使用，不读取或连接 Redis。
2. 未显式注入时调用默认 session store 工厂。
3. 工厂根据 `redis.default` 是否存在选择 Redis 或内存实现。
4. 工厂返回错误时，`BuildWithContext` 立即终止应用构建。

这个顺序保证现有单元测试、集成测试和嵌入方可以继续注入独立 store，不受本地 Redis 配置影响。

### 4.3 数据流

首次登录时，登录服务继续将 session ID、用户 ID、token JTI 哈希、密码版本和 refresh 过期时间写入应用级 `SessionStore`。Redis 实现使用 refresh 剩余有效期作为 TTL。

服务重启后，应用重新装配到同一 Redis namespace。鉴权中间件使用 JWT 中的 session ID 读取原 session，并按现有规则校验用户 ID、密码版本和 SSO access JTI。不需要迁移或重新签发旧 token。

### 4.4 错误处理

1. 读取 `redis.default` 失败：返回启动错误。
2. `redis.default` 存在但配置不完整：由 GoFrame Redis 客户端初始化或 `PING` 返回错误，应用启动失败。
3. Redis 无法连接或认证失败：`PING` 返回错误，应用启动失败。
4. `RedisSessionStore` 构造失败：返回启动错误。
5. 错误信息说明是默认认证 session store 初始化失败，保留底层错误链，不输出 Redis 密码。

### 4.5 配置语义

`redis.default` 的存在性是唯一选择开关，不新增 `cool.auth.session.mode` 等重复配置。

- 存在 `redis.default`：Redis session。
- 不存在 `redis.default`：内存 session，并接受重启后 token 失效的本地单进程语义。
- 存在但不可用：启动失败，不回退内存。

Redis key prefix 继续使用 `RedisSessionStore` 的现有默认值 `cool-admin-go-next:auth:v1`，避免与 Task 和其他缓存 key 冲突。

## 5. 测试

### 5.1 单元测试

1. 配置不包含 `redis.default` 时，工厂返回 `MemorySessionStore`。
2. 配置包含 `redis.default` 且 Redis 可用时，工厂执行连通性检查并返回 `RedisSessionStore`。
3. 配置包含 `redis.default` 但连通性检查失败时，工厂返回错误，不构造内存 store。
4. 显式注入 `Options.SessionStore` 时，不创建默认 Redis 客户端，不执行 `PING`。
5. 现有内存和 Redis session 的保存、读取、轮换和撤销测试继续通过。

### 5.2 集成测试

设置 `COOL_AUTH_REDIS_INTEGRATION=1` 后，在可用 Redis 环境中执行集成测试：

1. 用第一个应用实例保存 session。
2. 构建使用同一 Redis 配置的第二个应用实例，模拟服务重启。
3. 第二个实例能读取原 session，且原 access token 通过鉴权。
4. 测试结束后只清理该测试的独立 Redis namespace。

## 6. 验收标准

1. 使用当前 `manifest/config/config.yaml` 中的 `redis.default` 启动时，应用的默认 session store 为 Redis。
2. 用户登录后重启服务，在 token 未过期、session 未撤销且 Redis 数据仍存在时，原 token 仍可访问受保护接口。
3. 删除 `redis.default` 后，应用可使用内存 session 启动。
4. 保留 `redis.default` 但停止 Redis 时，应用启动失败并返回明确错误。
5. 与认证、应用装配和 Base 模块相关的现有测试通过。
