# cool/ 目录结构优化方案

## 背景

`cool/` 目录当前有 18 个顶层目录，部分目录仅包含 1-2 个几行代码的文件，存在目录膨胀和文件分散问题。参考 Node 版 `cool-admin-midway-packages` 的命名规范，对 Go 版的 `cool/` 进行重组。

## 优化目标

1. 顶层目录从 18 个减少到 10 个
2. 工具类统一归 `util/`
3. 命名与 Node 版对齐，降低认知成本
4. 小文件合并，减少文件碎片

## 新旧目录映射

### 顶层目录

| 新版 | 旧版 | 说明 |
|------|------|------|
| `app/` | `app/` | 不变 |
| `security/` | `auth/` | 重命名，涵盖认证+会话+密码+token |
| `controller/` | `controller/` | 不变 |
| `rest/` | `crud/` + `eps/` + `response/` | 跟随 Node 命名，CRUD + 端点收集 + 响应格式 |
| `db/` | `db/` + `recycle/` + `tenant/` | 数据层，含 schema 同步、回收站、多租户 |
| `task/` | `task/` | 不变 |
| `middleware/` | `middleware/` | 不变 |
| `entity/` | `model/` | 重命名，跟随 Node |
| `exception/` | `errors/` | 重命名，跟随 Node |
| `module/` | `module/` + `registry/` + `seed/` | 合并模块声明、注册、种子数据 |
| `service/` | `service/` | 不变 |
| `util/` | `route/` + `codegen/` + `auth/token.go` + `auth/password.go` | 工具类统一收纳 |

### 文件级映射

#### security/（原 auth/）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `security/middleware.go` | `auth/middleware.go` | 认证中间件 |
| `security/session.go` | `auth/session.go` | Session 接口 + 内存实现 |
| `security/session_redis.go` | `auth/session_redis.go` | Redis 会话存储 |
| `security/context.go` | `auth/context.go` | 用户上下文 |
| `security/tenant_identity.go` | `auth/tenant_identity.go` | 租户身份类型 |

#### util/（新）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `util/token.go` | `auth/token.go` | JWT 签名/解析 |
| `util/password.go` | `auth/password.go` | bcrypt 加密 |
| `util/route/` | `route/` | 路由键规范化 |
| `util/codegen/` | `codegen/module/` | 代码生成器 |
| `util/crypto.go` | — | 通用加密工具（可选） |

#### rest/（新）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `rest/crud/` | `crud/` | CRUD 运行时 |
| `rest/eps.go` | `eps/eps.go` | 端点收集 |
| `rest/response.go` | `response/response.go` | 响应体格式 |

#### db/（扩展）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `db/driver/` | `db/driver/` | 数据库驱动 |
| `db/schema/` | `db/schema/` | schema 同步 |
| `db/recycle/` | `recycle/` | 回收站 |
| `db/tenant/` | `tenant/` | 多租户 |

#### entity/（原 model/）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `entity/entity.go` | `model/model.go` | 模型定义类型 |

#### exception/（原 errors/）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `exception/code.go` | `errors/code.go` | 错误码常量 |
| `exception/exception.go` | `errors/error.go` | 错误构造 + 解析 |
| `exception/resolve.go` | `errors/resolve.go` | 错误解析 |

#### module/（扩展）

| 新路径 | 旧路径 | 说明 |
|--------|--------|------|
| `module/module.go` | `module/module.go` | 模块定义 |
| `module/config_loader.go` | `module/config_loader.go` | 配置加载 |
| `module/declaration.go` | `module/declaration.go` | 模块声明 |
| `module/registry.go` | `registry/registry.go` | 模块注册 |
| `module/dependencies.go` | `registry/dependencies.go` | 依赖管理 |
| `module/runtime_group.go` | `registry/runtime_group.go` | 运行时分组 |
| `module/seed/` | `seed/` | 种子数据 |

## 变更影响

### 需要修改的文件

- `cool/` 内部所有交叉引用（约 30+ 个文件需改 import 路径）
- `modules/` 下各业务模块的 import（`cool/errors` → `cool/exception` 等）
- `main.go` 的 import
- `cmd/cool/main.go` 的 import

### 不变的内容

- 所有代码逻辑不变，纯文件移动
- 包内导出符号不变（函数名、类型名不变）
- 测试用例不变，只改 import 路径

## 迁移步骤

1. 创建新目录结构
2. 按映射移动文件
3. 批量更新所有 import 路径
4. 运行测试验证
5. 删除旧目录