# Redis Session 用户反向索引设计

## 目标

将 Redis 用户 Session 撤销从扫描全部 Session Key 改为按目标用户定向异步删除，使前台撤销成本由 O(全部 Session) 降为 O(目标用户数)，同时保持现有鉴权 Store 接口和 Token 协议不变。

## 数据结构

- Session 定位 Key：`<prefix>v2:session:<sessionId>`，值记录该 Session 的身份类型和用户 ID，并按 Session 到期时间过期。
- 用户 Session Key：`<prefix>v2:user:<subject>:<userId>`，使用 Redis Hash，以 Session ID 为字段、现有版本化 Session JSON 为值。
- 用户 Hash 的到期时间取其中 Session 的最晚到期时间。个别已过期字段允许暂时残留，Hash 整体到期或用户撤销时统一清理。

`v2` 命名空间使升级前创建且没有反向索引的 Session 立即失效。旧 Key 不主动删除，继续按原 TTL 自然过期，避免一次性全量扫描和删除。发布后已登录用户需要重新登录一次。

## 写入与轮换

保存 Session 时，通过 Lua 脚本原子完成以下操作：

1. 按绝对到期时间写入 Session 定位 Key；
2. 将 Session JSON 写入用户 Hash 的 Session ID 字段；
3. 仅当新 Session 到期时间更晚时延长用户 Hash 有效期。

刷新 Token 不改变 Session ID、身份类型或用户 ID。轮换脚本在校验原 Session 内容未变化后更新定位 Key、用户 Hash 字段及有效期。

## 撤销

- 单 Session 撤销：通过定位 Key 找到用户 Hash，再由 Lua 脚本原子删除定位 Key 和对应 Hash 字段。若 Session 已不存在，按幂等成功处理。
- 用户 Session 撤销：使用 Redis `UNLINK` 让目标用户 Hash 立即不可见，并在后台释放 Hash 内存。残留定位 Key 最多保留到原 Session 到期，读取时发现 Hash 字段不存在即删除定位 Key，并按 Session 不存在处理。
- 用户撤销不再调用 `SCAN`，也不再读取或累计其他用户的 Session Key。

Lua 脚本访问的 Key 全部通过 `KEYS` 显式传入，不根据 Redis 数据动态生成 Key，符合 Redis `EVAL` 的 Key 声明约束。当前 Redis Store 使用单 Redis 命名空间，本次不扩展 Redis Cluster 分片策略。

## 错误处理

- Redis 脚本或命令失败时沿用现有核心异常包装，不静默降级到全量扫描。
- Session JSON 无效时返回现有解析错误，避免根据不可信内容构造索引 Key。
- 解析定位值和构造用户 Hash Key 前继续校验身份类型、用户 ID 和 Session ID。

## 测试

- 校验新 Session 只写入 `v2` 命名空间，并建立正确的定位 Key 和用户 Hash 字段。
- 校验多个 Session 会共享用户 Hash，且 Hash 有效期不会被较早到期的 Session 缩短。
- 校验 Token 轮换同时维护定位 Key 和用户 Hash，并保留原有重放检测语义。
- 校验单 Session 撤销会移除定位 Key 和对应 Hash 字段，并能撤销刚完成刷新的同一 Session。
- 校验按用户撤销只 `UNLINK` 目标用户 Hash，不执行 `SCAN`，其他用户和身份类型的 Session 不受影响。
- 校验升级前旧命名空间中的 Session 不可读取，并由原 TTL 自然清理。

## 范围

仅修改 Redis Session Store 及其测试。Memory Store、鉴权 Store 接口、JWT 内容、业务调用方和 Redis 配置字段保持不变，不新增第三方依赖。
