# Redis Session 用户反向索引设计

## 目标

将 Redis 用户 Session 撤销从扫描全部 Session Key 改为按目标用户定向查询，使撤销成本由 O(全部 Session) 降为 O(目标用户 Session)，同时保持现有鉴权 Store 接口和 Token 协议不变。

## 数据结构

- Session Key：`<prefix>v2:<sessionId>`，值继续使用现有版本化 Session JSON，并按 Session 到期时间过期。
- 用户索引 Key：`<prefix>v2:user:<subject>:<userId>`，使用 Redis Set 保存该用户的 Session ID。
- 用户索引的到期时间取集合内 Session 的最晚到期时间。个别已过期成员允许暂时残留，索引整体到期或用户撤销时统一清理。

`v2` 命名空间使升级前创建且没有反向索引的 Session 立即失效。旧 Key 不主动删除，继续按原 TTL 自然过期，避免一次性全量扫描和删除。发布后已登录用户需要重新登录一次。

## 写入与轮换

保存 Session 时，通过 Lua 脚本原子完成以下操作：

1. 按绝对到期时间写入 Session；
2. 将 Session ID 加入用户索引；
3. 仅当新 Session 到期时间更晚时延长索引有效期。

刷新 Token 不改变 Session ID、身份类型或用户 ID。轮换脚本在校验原 Session 内容未变化后更新 Session，并再次写入相同用户索引及有效期，保证索引可自愈。

## 撤销

- 单 Session 撤销：先读取并校验当前 Session，再通过比较原始内容的 Lua 脚本原子删除 Session Key 和对应索引成员。若 Session 已不存在，按幂等成功处理。
- 用户 Session 撤销：逐个读取目标用户索引，由 Lua 脚本遍历该索引中的 Session ID，删除对应 Session Key，最后删除索引 Key。
- 用户撤销不再调用 `SCAN`，也不再把全部 Session Key 累积到 Go 切片。

当前 Redis Store 使用单 Redis 命名空间，脚本按现有部署模型操作相关 Key。本次不扩展 Redis Cluster 分片策略。

## 错误处理

- Redis 脚本或命令失败时沿用现有核心异常包装，不静默降级到全量扫描。
- Session JSON 无效时返回现有解析错误，避免根据不可信内容构造索引 Key。
- 构造索引前继续校验身份类型、用户 ID 和 Session ID。

## 测试

- 校验新 Session 只写入 `v2` 命名空间并建立正确的用户索引。
- 校验多个 Session 会共享用户索引，且索引有效期不会被较早到期的 Session 缩短。
- 校验 Token 轮换同时维护索引，并保留原有重放检测语义。
- 校验单 Session 撤销会移除对应索引成员，且并发更新后不会误删新内容。
- 校验按用户撤销只访问目标用户索引，不执行 `SCAN`，其他用户和身份类型的 Session 不受影响。
- 校验升级前旧命名空间中的 Session 不可读取，并由原 TTL 自然清理。

## 范围

仅修改 Redis Session Store 及其测试。Memory Store、鉴权 Store 接口、JWT 内容、业务调用方和 Redis 配置字段保持不变，不新增第三方依赖。
