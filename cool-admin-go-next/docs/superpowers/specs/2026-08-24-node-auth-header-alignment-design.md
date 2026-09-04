# Node Authorization 行为对齐设计

## 目标

让 Go 版后台认证行为与 Node 版一致：客户端通过 `Authorization: <token>` 发送裸 token，认证层直接校验该值。

## 根因

Node 版将完整的 `Authorization` 请求头直接传给 token 校验。Go 版的协议认证入口要求 `Bearer <token>` 并提取第二段，导致现有前端发送的裸 token 无法认证。

## 方案

HTTP 和 gRPC 认证入口不再解析认证 scheme，直接将完整的 `Authorization` 值交给现有认证内核。空值提前返回现有无效凭证错误，其他无效值由 token 校验自然处理，不增加格式分支。

认证失败继续使用现有错误边界：路由认证中间件将错误交给统一响应中间件，响应中间件在调用 Handler 前发现错误并立即返回。HTTP 状态码为 `401`，响应体业务码为 `1001`，受保护 Handler 不执行。

## 修改范围

- 修改 `cool-next/auth/transport.go` 中的协议认证入口。
- 修改 `cool-next/auth/service_test.go`，验证裸 token 可用于 HTTP 和 gRPC，并验证无效 token 返回业务码 `1001`、HTTP 状态码 `401`。
- 修改 HTTP 上下文和响应中间件，确保认证错误进入统一 JSON 响应且不会执行 Handler。
- 不修改前端；前端当前已经发送裸 token。

## 验证

- 运行 `cool-next/auth` 单元测试。
- 运行 HTTP 上下文和响应中间件测试。
- 重启当前 `cool-admin-go-next` 服务并验证 `8001` 端口。
