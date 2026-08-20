# 首批自定义 API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为已可登录的 Vue 前端补齐个人资料、本地上传、用户移动、部门排序和日志配置的 8 条 base 自定义 API，并保持 Node 权限、EPS 和响应契约兼容。

**Architecture:** 所有接口由既有 `controller.Definition` 的 `RouteOptions` 声明，运行时会从相同 metadata 自动注册 HTTP 路由、派生 sys 权限映射并输出 EPS。Controller 只负责提取 HTTP/认证上下文并调用领域 Service；`UserService`、`DepartmentService`、`LogService` 和新建的 `UploadService` 负责输入校验、事务、持久化或文件保存。应用启动时以 GoFrame `AddStaticPath` 仅映射上传目录到 `/uploads/`。

**Tech Stack:** Go 1.x、GoFrame v2.10.2（`ghttp`、`gdb`）、现有 `cool/controller`、`cool/response`、MySQL、标准库 `os`/`path/filepath`/`time`。

## Global Constraints

- 所有项目面向的说明和代码注释使用中文。
- 保持 Controller metadata 为路由、权限和 EPS 的唯一接口来源；不得手写 EPS 或前端 service。
- 不修改已有 `ModifyBefore/ModifyAfter` CRUD Hook，不引入通用 action 框架。
- Comm 新接口只要求认证；sys 新接口必须使用 `menu.json` 中已经存在的显式权限码。
- 已登录但缺少 sys 权限时，必须返回 HTTP 403 和精确 body `{"code":1001,"message":"登录失效或无权限访问~"}`。
- 未登录或 token 无效时保持当前 HTTP 401 认证行为。
- 所有可预期的输入、文件和数据错误必须使用 HTTP 200、`code:1001` 和中文业务消息；不得向客户端泄露 SQL、绝对路径或内部错误。
- `personUpdate` 仅允许更新当前用户的 `nickName`、`headImg`、`phone`、`email`、`remark`；本阶段不允许改密码。
- `uploadMode` 固定返回 `{"mode":"local","type":"local"}`；`upload` 仅接受 multipart `file`，成功 data 是 `/uploads/YYYYMMDD/<随机文件名>.<扩展名>` 格式字符串。
- 上传仅开放应用配置的上传目录至 `/uploads/`；禁止目录列表、禁止将项目根目录设为静态根目录；单文件上限是 `10 * 1024 * 1024` 字节。
- `user/move` 接收 `{departmentId, userIds}`；`department/order` 接收 `[]DepartmentOrderItem`，每项只含 `id`、`parentId`、`orderNum`。
- 日志 `clear` 严格对齐 Node `clear(true)`，清空整张 `base_sys_log` 表；`logKeep` 使用 `base_sys_conf.c_key = 'logKeep'`，缺失时回退初始化值 `31`。
- 新增或修改 Go 文件必须执行 `gofmt`；包管理只使用 Yarn（本计划不增加前端依赖）；不得使用 `git add -A`。
- 每个引入或变更生产行为的任务必须先观察到失败测试，再写最小实现并执行任务聚焦测试。

---

## File Structure

### New files

- `modules/base/service/custom_api.go` — 首批接口的请求 DTO、纯输入校验、`UploadService` 与上传保存逻辑。
- `modules/base/service/custom_api_test.go` — DTO 白名单、校验及临时目录上传单元测试。
- `modules/base/custom_api_integration_test.go` — 以 `COOL_CUSTOM_API_INTEGRATION=1` 启用的真实 MySQL/HTTP 验收。

### Modified files

- `cool/controller/permission.go`、`cool/controller/permission_test.go` — 对齐 Node 的 403 消息与精确响应测试。
- `modules/base/service/sys_user.go` — 当前用户资料白名单更新和批量部门移动。
- `modules/base/service/sys_department.go` — 部门树排序的存在性校验与单事务更新。
- `modules/base/service/sys_log.go` — 清空日志、读写 `logKeep`。
- `modules/base/controller/comm/comm.go` — personUpdate、uploadMode、upload 路由及 handler。
- `modules/base/controller/admin/sys_user.go` — move metadata route。
- `modules/base/controller/admin/sys_department.go` — order metadata route。
- `modules/base/controller/admin/sys_log.go` — clear、setKeep、getKeep metadata route。
- `modules/base/controller/controllers.go` — 接收由 app 构造的 `UploadService` 并将它注入 Comm Controller；向 Comm Controller 传入 user service。
- `modules/base/controller/controllers_test.go` — 8 条新 route 的 metadata、权限和 EPS 输入结构断言。
- `modules/base/eps_test.go` — EPS 实际输出包含新增 API。
- `cool/app/app.go`、`cool/app/app_test.go` — `Options.UploadDir`、上传目录的静态 URI 映射与独立 server 验证。
- `modules/base/permission_integration_test.go` — 更新既有 403 契约断言为 Node 消息。
- `README.md` — 首批自定义 API 验收命令与上传约束。

---

### Task 1: 对齐 Node 的权限拒绝响应

**Files:**
- Modify: `cool/controller/permission.go`
- Modify: `cool/controller/permission_test.go`
- Modify: `modules/base/permission_integration_test.go`

**Interfaces:**
- Produces: `controller.WriteForbidden(r *ghttp.Request)` 始终写入 HTTP 403 和 `response.Body{Code: 1001, Message: "登录失效或无权限访问~"}`。
- Consumed by: 所有现有及新增 sys 权限路由。

- [ ] **Step 1: 写出 Node 兼容 403 的失败单测**

在 `cool/controller/permission_test.go` 将现有 `TestRegisterPermissionMiddlewareReturnsForbiddenOnCheckerError` 的精确断言替换为：

```go
if rec.Code != http.StatusForbidden {
    t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
}
if rec.Body.String() != `{"code":1001,"message":"登录失效或无权限访问~"}` {
    t.Fatalf("unexpected forbidden response: %s", rec.Body.String())
}
```

- [ ] **Step 2: 运行测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/controller -run TestRegisterPermissionMiddlewareReturnsForbiddenOnCheckerError -count=1
```

Expected: FAIL，实际 body 的 message 仍为 `权限不足~`。

- [ ] **Step 3: 最小化修改统一 403 常量**

在 `cool/controller/permission.go` 将：

```go
const permissionDeniedMessage = "权限不足~"
```

替换为：

```go
const permissionDeniedMessage = "登录失效或无权限访问~"
```

不要在 Comm Controller 保留第二套同名常量或 `writeForbidden`；后续 Task 4 删除它并改用 `coolController.WriteForbidden`。

- [ ] **Step 4: 更新真实权限集成测试的预期**

在 `modules/base/permission_integration_test.go` 的受限用户断言中替换：

```go
if limitedPage.Body != `{"code":1001,"message":"登录失效或无权限访问~"}` {
    t.Fatalf("unexpected forbidden body: %s", limitedPage.Body)
}
```

- [ ] **Step 5: 运行聚焦测试，确认 Green**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/controller/permission.go cool/controller/permission_test.go modules/base/permission_integration_test.go
go test ./cool/controller -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交权限契约修正**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/controller/permission.go cool/controller/permission_test.go modules/base/permission_integration_test.go
git commit -m $'fix: align forbidden response with Node\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 2: 建立首批接口 DTO、校验与本地上传服务

**Files:**
- Create: `modules/base/service/custom_api.go`
- Create: `modules/base/service/custom_api_test.go`

**Interfaces:**
- Produces:

```go
const DefaultLogKeep int64 = 31
const MaxUploadSize int64 = 10 * 1024 * 1024

type PersonUpdateRequest struct {
    NickName string `json:"nickName"`
    HeadImg  string `json:"headImg"`
    Phone    string `json:"phone"`
    Email    string `json:"email"`
    Remark   string `json:"remark"`
}
func (r PersonUpdateRequest) Values() map[string]interface{}

type MoveRequest struct {
    DepartmentID int64   `json:"departmentId"`
    UserIDs      []int64 `json:"userIds"`
}
func (r MoveRequest) Validate() error

type DepartmentOrderItem struct {
    ID        int64  `json:"id"`
    ParentID  *int64 `json:"parentId"`
    OrderNum  int64  `json:"orderNum"`
}
func ValidateDepartmentOrder(items []DepartmentOrderItem) error

type LogKeepRequest struct { Value int64 `json:"value"` }
func (r LogKeepRequest) Validate() error

type UploadService struct { RootDir string }
func NewUploadService(rootDir string) *UploadService
func (s *UploadService) Upload(ctx context.Context, file *ghttp.UploadFile) (string, error)
func (s *UploadService) StaticDir() string
```

- Consumed by: Task 3 的领域 Service 和 Task 4 的 Comm handler。

- [ ] **Step 1: 写 DTO 和上传规则的失败测试**

创建 `modules/base/service/custom_api_test.go`。测试必须直接覆盖白名单、输入规则与实际文件保存：

```go
func TestPersonUpdateRequestValuesUsesOnlyAllowedFields(t *testing.T) {
    values := PersonUpdateRequest{
        NickName: "昵称", HeadImg: "/old.png", Phone: "13800000000", Email: "n@example.com", Remark: "备注",
    }.Values()
    if !reflect.DeepEqual(values, map[string]interface{}{
        "nick_name": "昵称", "head_img": "/old.png", "phone": "13800000000", "email": "n@example.com", "remark": "备注",
    }) {
        t.Fatalf("unexpected update values: %#v", values)
    }
    for _, prohibited := range []string{"id", "username", "password", "password_v", "status", "tenant_id"} {
        if _, ok := values[prohibited]; ok {
            t.Fatalf("forbidden field %s leaked into update values", prohibited)
        }
    }
}

func TestCustomAPIRequestsRejectInvalidInput(t *testing.T) {
    cases := []struct { name string; err error }{
        {"empty user IDs", MoveRequest{DepartmentID: 1}.Validate()},
        {"invalid department", MoveRequest{DepartmentID: 0, UserIDs: []int64{1}}.Validate()},
        {"duplicate user IDs", MoveRequest{DepartmentID: 1, UserIDs: []int64{1, 1}}.Validate()},
        {"empty order", ValidateDepartmentOrder(nil)},
        {"duplicate order ID", ValidateDepartmentOrder([]DepartmentOrderItem{{ID: 1, OrderNum: 0}, {ID: 1, OrderNum: 1}})},
        {"negative order", ValidateDepartmentOrder([]DepartmentOrderItem{{ID: 1, OrderNum: -1}})},
        {"invalid keep", LogKeepRequest{Value: 0}.Validate()},
    }
    for _, item := range cases {
        if item.err == nil { t.Fatalf("expected %s validation error", item.name) }
    }
}
```

使用 `httptest.NewRequest`、`multipart.NewWriter` 和 `ghttp.Request` 构造含 `file` 的请求，新增上传测试：

```go
func TestUploadServiceSavesRandomFileUnderDailyDirectory(t *testing.T) {
    root := t.TempDir()
    service := NewUploadService(root)
    request, uploadFile := uploadRequest(t, "../avatar.png", []byte("PNG"))
    defer request.Body.Close()

    savedURL, err := service.Upload(context.Background(), uploadFile)
    if err != nil { t.Fatalf("upload failed: %v", err) }
    if !regexp.MustCompile(`^/uploads/[0-9]{8}/[a-z0-9]+\\.png$`).MatchString(savedURL) {
        t.Fatalf("unexpected upload URL: %s", savedURL)
    }
    relative := strings.TrimPrefix(savedURL, "/uploads/")
    content, err := os.ReadFile(filepath.Join(root, relative))
    if err != nil || string(content) != "PNG" {
        t.Fatalf("expected saved content, got %q, %v", content, err)
    }
}
```

同文件验证 `nil` 文件返回 `文件不能为空`，大于 `MaxUploadSize` 的 `multipart.FileHeader.Size` 返回 `文件大小不能超过10MB`。

- [ ] **Step 2: 运行测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/service -run 'Test(PersonUpdateRequestValuesUsesOnlyAllowedFields|CustomAPIRequestsRejectInvalidInput|UploadServiceSavesRandomFileUnderDailyDirectory)' -count=1
```

Expected: FAIL，提示 `PersonUpdateRequest`、`UploadService` 或校验函数未定义。

- [ ] **Step 3: 实现 DTO、校验与上传服务**

创建 `modules/base/service/custom_api.go`，实现以下规则：

```go
const (
    DefaultLogKeep int64 = 31
    MaxUploadSize  int64 = 10 * 1024 * 1024
)

func (r PersonUpdateRequest) Values() map[string]interface{} {
    return map[string]interface{}{
        "nick_name": r.NickName,
        "head_img":  r.HeadImg,
        "phone":     r.Phone,
        "email":     r.Email,
        "remark":    r.Remark,
    }
}
```

- `MoveRequest.Validate`：`DepartmentID <= 0` 返回 `部门参数错误`；空 user IDs 返回 `用户不能为空`；非正或重复 ID 返回 `用户参数错误`。
- `ValidateDepartmentOrder`：空数组返回 `排序数据不能为空`；ID 非正、重复 ID、`OrderNum < 0` 均返回 `排序数据错误`。`ParentID` 为 nil 合法。
- `LogKeepRequest.Validate`：`Value <= 0` 返回 `日志保留天数必须大于0`。
- `NewUploadService`：将空 root 防御性替换为 `filepath.Join("resource", "public", "uploads")`。
- `StaticDir` 返回 `RootDir`。
- `Upload`：file nil 返回 `文件不能为空`；`file.Size > MaxUploadSize` 返回 `文件大小不能超过10MB`；以 `time.Now().Format("20060102")` 建立日期子目录；调用 GoFrame `file.Save(targetDir, true)` 生成随机文件名；只以 `filepath.Ext(file.Filename)` 保留扩展名；保存失败记录包装错误但对 controller 通过后续 handler 映射成 `文件上传失败`；返回 `"/uploads/" + date + "/" + filename`。

上传实现不得拼接或信任 `file.Filename` 的目录部分，且不得接受客户端 `key` 覆盖文件位置。

- [ ] **Step 4: 运行聚焦测试，确认 Green**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/service/custom_api.go modules/base/service/custom_api_test.go
go test ./modules/base/service -run 'Test(PersonUpdateRequestValuesUsesOnlyAllowedFields|CustomAPIRequestsRejectInvalidInput|UploadServiceSavesRandomFileUnderDailyDirectory)' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交 DTO 和上传服务**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/service/custom_api.go modules/base/service/custom_api_test.go
git commit -m $'feat: add custom API request validation and uploads\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 3: 实现用户、部门和日志领域操作

**Files:**
- Modify: `modules/base/service/sys_user.go`
- Modify: `modules/base/service/sys_department.go`
- Modify: `modules/base/service/sys_log.go`
- Modify: `modules/base/service/custom_api_test.go`

**Interfaces:**
- Consumes: Task 2 的 `PersonUpdateRequest`、`MoveRequest`、`DepartmentOrderItem`、`LogKeepRequest`、`DefaultLogKeep`。
- Produces:

```go
func (s *UserService) PersonUpdate(ctx context.Context, userID int64, request PersonUpdateRequest) error
func (s *UserService) Move(ctx context.Context, request MoveRequest) error
func (s *DepartmentService) Order(ctx context.Context, items []DepartmentOrderItem) error
func (s *LogService) Clear(ctx context.Context) error
func (s *LogService) SetKeep(ctx context.Context, request LogKeepRequest) error
func (s *LogService) GetKeep(ctx context.Context) (int64, error)
```

- Consumed by: Task 4 的 HTTP handlers。

- [ ] **Step 1: 为 Service 入口写失败测试**

在 `modules/base/service/custom_api_test.go` 增加不依赖 MySQL 的快速前置校验测试，确保 Service 在 DB 为 nil 前就拒绝不合法请求：

```go
func TestResourceServicesValidateBeforeDatabaseAccess(t *testing.T) {
    user := NewUserService(nil, baseModel.BaseSysUser())
    if err := user.Move(context.Background(), MoveRequest{}); err == nil || err.Error() != "部门参数错误" {
        t.Fatalf("expected department validation error, got %v", err)
    }

    department := NewDepartmentService(nil, baseModel.BaseSysDepartment())
    if err := department.Order(context.Background(), nil); err == nil || err.Error() != "排序数据不能为空" {
        t.Fatalf("expected order validation error, got %v", err)
    }

    logService := NewLogService(nil, baseModel.BaseSysLog())
    if err := logService.SetKeep(context.Background(), LogKeepRequest{}); err == nil || err.Error() != "日志保留天数必须大于0" {
        t.Fatalf("expected keep validation error, got %v", err)
    }
}
```

- [ ] **Step 2: 运行测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/service -run TestResourceServicesValidateBeforeDatabaseAccess -count=1
```

Expected: FAIL，提示 `Move`、`Order` 或 `SetKeep` 未定义。

- [ ] **Step 3: 实现最小领域 Service 方法**

在 `sys_user.go`：

```go
func (s *UserService) PersonUpdate(ctx context.Context, userID int64, request PersonUpdateRequest) error {
    if userID <= 0 { return gerror.New("登录失效~") }
    _, err := s.DB.Model(s.Model.TableName).Ctx(ctx).Where("id", userID).Data(request.Values()).Update()
    if err != nil { return gerror.Wrap(err, "更新个人信息失败") }
    return nil
}

func (s *UserService) Move(ctx context.Context, request MoveRequest) error {
    if err := request.Validate(); err != nil { return err }
    exists, err := s.DB.Model("base_sys_department").Ctx(ctx).Where("id", request.DepartmentID).Count()
    if err != nil { return gerror.Wrap(err, "查询部门失败") }
    if exists == 0 { return gerror.New("部门不存在") }
    return s.DB.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
        _, err := tx.Model(s.Model.TableName).WhereIn("id", request.UserIDs).Data(g.Map{"department_id": request.DepartmentID}).Update()
        return err
    })
}
```

在 `sys_department.go`：先执行 `ValidateDepartmentOrder`，再用 `WHERE id IN (...)` 查询并比较计数确认所有 ID 存在；随后用 `s.DB.Transaction` 逐项执行：

```go
_, err := tx.Model(s.Model.TableName).Where("id", item.ID).Data(g.Map{
    "parent_id": item.ParentID,
    "order_num": item.OrderNum,
}).Update()
```

任何查询或 update 错误使用 `gerror.Wrap` 返回；事务回调直接传播错误以触发回滚。

在 `sys_log.go`：

- `Clear` 执行 `s.DB.Model(s.Model.TableName).Ctx(ctx).Delete()`，不得按 tenant 过滤。
- `SetKeep` 先 `request.Validate()`，再按 `c_key = 'logKeep'` 查询；存在则更新 `c_value`，不存在则插入 `g.Map{"c_key":"logKeep", "c_value": strconv.FormatInt(request.Value, 10)}`。
- `GetKeep` 查询 `base_sys_conf` 的 `c_value`；无记录、空值或 `strconv.ParseInt` 失败时返回 `DefaultLogKeep, nil`；数据库查询本身失败时包装返回错误。

所有数据库错误的 wrapper 只供日志记录；Task 4 的 handler 不得把 `err.Error()` 原样返回客户端。

- [ ] **Step 4: 运行聚焦测试，确认 Green**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/service/sys_user.go modules/base/service/sys_department.go modules/base/service/sys_log.go modules/base/service/custom_api_test.go
go test ./modules/base/service -run 'Test(ResourceServicesValidateBeforeDatabaseAccess|PersonUpdateRequestValuesUsesOnlyAllowedFields|CustomAPIRequestsRejectInvalidInput)' -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交领域 Service 操作**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/service/sys_user.go modules/base/service/sys_department.go modules/base/service/sys_log.go modules/base/service/custom_api_test.go
git commit -m $'feat: add base custom resource operations\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 4: 声明自定义 metadata 路由并接入 HTTP handler

**Files:**
- Modify: `modules/base/controller/comm/comm.go`
- Modify: `modules/base/controller/admin/sys_user.go`
- Modify: `modules/base/controller/admin/sys_department.go`
- Modify: `modules/base/controller/admin/sys_log.go`
- Modify: `modules/base/controller/controllers.go`
- Modify: `modules/base/controller/controllers_test.go`

**Interfaces:**
- Consumes: Task 2/3 Service 方法，`cool/auth.UserFromContext`，`cool/controller.WriteForbidden`。
- Produces: 8 个 Controller metadata route，名字分别为 `personUpdate`、`uploadMode`、`upload`、`move`、`order`、`clear`、`setKeep`、`getKeep`。

- [ ] **Step 1: 写 8 条路由 metadata 的失败测试**

在 `modules/base/controller/controllers_test.go` 加入：

```go
func TestBaseCustomAPIMetadata(t *testing.T) {
    expected := map[string]struct {
        method, path, permission string
    }{
        "personUpdate": {http.MethodPost, "/admin/base/comm/personUpdate", ""},
        "uploadMode":   {http.MethodGet,  "/admin/base/comm/uploadMode", ""},
        "upload":       {http.MethodPost, "/admin/base/comm/upload", ""},
        "move":          {http.MethodPost, "/admin/base/sys/user/move", "base:sys:user:move"},
        "order":         {http.MethodPost, "/admin/base/sys/department/order", "base:sys:department:order"},
        "clear":         {http.MethodPost, "/admin/base/sys/log/clear", "base:sys:log:clear"},
        "setKeep":       {http.MethodPost, "/admin/base/sys/log/setKeep", "base:sys:log:setKeep"},
        "getKeep":       {http.MethodGet,  "/admin/base/sys/log/getKeep", "base:sys:log:getKeep"},
    }
    seen := map[string]bool{}
    for _, definition := range Controllers(nil, nil, nil) {
        for _, route := range definition.Routes {
            want, ok := expected[route.Name]
            if !ok { continue }
            seen[route.Name] = true
            if route.Method != want.method || route.FullPath != want.path || route.Permission != want.permission || route.IgnoreAuth || route.Handler == nil {
                t.Fatalf("unexpected route %s: %#v", route.Name, route)
            }
        }
    }
    if !reflect.DeepEqual(seen, map[string]bool{"personUpdate":true,"uploadMode":true,"upload":true,"move":true,"order":true,"clear":true,"setKeep":true,"getKeep":true}) {
        t.Fatalf("missing custom API routes: %#v", seen)
    }
}
```

同文件加入 `PermissionMap` 断言：5 条 sys key 的 permission 必须正确，三条 Comm 路径不得出现。

- [ ] **Step 2: 运行测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base/controller -run TestBaseCustomAPIMetadata -count=1
```

Expected: FAIL，8 条 route 尚不存在。

- [ ] **Step 3: 连接 Service 构造与 Controller metadata**

将 `CommController` 改为接收：

```go
func CommController(authService *baseService.AuthService, userService *baseService.UserService, permissionService *baseService.PermissionService, uploadService *baseService.UploadService) coolController.Definition
```

将 `Controllers` 改为接收由应用层构造的 uploader：

```go
func Controllers(db gdb.DB, manager *auth.Manager, epsService *openController.EPSService, uploadService *baseService.UploadService) []coolController.Definition
```

在 `controllers.go` 将 `userService` 和 `uploadService` 传入 `CommController`。所有 `Controllers(nil, nil, nil)` 调用点改为 `Controllers(nil, nil, nil, nil)`；nil DB 和 nil upload service 的 handlers 可以注册，但被调用时以通用业务失败响应返回，不能 panic。

追加 metadata：

```go
.Route(coolController.RouteOptions{Name: "personUpdate", Method: http.MethodPost, Path: "/personUpdate", Description: "修改个人信息", Handler: personUpdateHandler(userService)})
.Route(coolController.RouteOptions{Name: "uploadMode", Method: http.MethodGet, Path: "/uploadMode", Description: "文件上传模式", Handler: uploadModeHandler})
.Route(coolController.RouteOptions{Name: "upload", Method: http.MethodPost, Path: "/upload", Description: "文件上传", Handler: uploadHandler(uploadService)})
```

在三个 admin Controller 分别追加：

```go
.Route(coolController.RouteOptions{Name: "move", Method: http.MethodPost, Path: "/move", Description: "移动部门", Permission: "base:sys:user:move", Handler: moveHandler(userService)})
.Route(coolController.RouteOptions{Name: "order", Method: http.MethodPost, Path: "/order", Description: "排序", Permission: "base:sys:department:order", Handler: orderHandler(departmentService)})
.Route(coolController.RouteOptions{Name: "clear", Method: http.MethodPost, Path: "/clear", Description: "清理", Permission: "base:sys:log:clear", Handler: clearHandler(logService)})
.Route(coolController.RouteOptions{Name: "setKeep", Method: http.MethodPost, Path: "/setKeep", Description: "日志保存时间", Permission: "base:sys:log:setKeep", Handler: setKeepHandler(logService)})
.Route(coolController.RouteOptions{Name: "getKeep", Method: http.MethodGet, Path: "/getKeep", Description: "获得日志保存时间", Permission: "base:sys:log:getKeep", Handler: getKeepHandler(logService)})
```

每个 handler 使用 `r.Parse` 读取 JSON；对于 `personUpdate`、`move`，先用 `coolAuth.UserFromContext` 获得当前用户，无用户时调用 `coolAuth.Unauthorized(r)` 并返回。成功直接返回 `map[string]interface{}{}` 或上传 URL；预期业务错误用 `response.Fail(<稳定中文消息>)`，数据库/保存错误记录 `g.Log().Error` 后返回 `response.Fail("操作失败")` 或 `response.Fail("文件上传失败")`。

删除 Comm Controller 内重复的 `permissionDeniedMessage` 和 `writeForbidden`，`permmenuHandler` 的错误分支改为 `coolController.WriteForbidden(r)`。

- [ ] **Step 4: 运行 metadata 与原有 handler 测试，确认 Green**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w modules/base/controller/comm/comm.go modules/base/controller/admin/sys_user.go modules/base/controller/admin/sys_department.go modules/base/controller/admin/sys_log.go modules/base/controller/controllers.go modules/base/controller/controllers_test.go
go test ./modules/base/controller ./cool/controller -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交路由和 handlers**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/controller/comm/comm.go modules/base/controller/admin/sys_user.go modules/base/controller/admin/sys_department.go modules/base/controller/admin/sys_log.go modules/base/controller/controllers.go modules/base/controller/controllers_test.go
git commit -m $'feat: declare base custom API routes\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 5: 注入上传目录并验证静态 URL

**Files:**
- Modify: `cool/app/app.go`
- Modify: `cool/app/app_test.go`
- Modify: `modules/base/controller/controllers.go`

**Interfaces:**
- Produces:

```go
type Options struct {
    // existing fields...
    UploadDir string
}
```

- `Application` 保存解析后的上传目录，并在 `registerRoutes` 之前调用 `a.server.AddStaticPath("/uploads", a.uploadDir)`。
- 默认上传目录：`resource/public/uploads`；测试可通过 `Options.UploadDir` 注入 `t.TempDir()`。

- [ ] **Step 1: 写静态路径映射的失败测试**

在 `cool/app/app_test.go` 创建临时目录和文件：

```go
func TestNewServesOnlyInjectedUploadDirectory(t *testing.T) {
    uploadDir := t.TempDir()
    if err := os.MkdirAll(filepath.Join(uploadDir, "20260721"), 0o755); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(uploadDir, "20260721", "proof.txt"), []byte("uploaded"), 0o644); err != nil { t.Fatal(err) }

    server := ghttp.GetServer("app-upload-static-test")
    server.SetPort(0)
    server.SetDumpRouterMap(false)
    defer server.Shutdown()
    New(Options{StartServer: true, Server: server, UploadDir: uploadDir})

    request := httptest.NewRequest(http.MethodGet, "/uploads/20260721/proof.txt", nil)
    recorder := httptest.NewRecorder()
    server.ServeHTTP(recorder, request)
    if recorder.Code != http.StatusOK || recorder.Body.String() != "uploaded" {
        t.Fatalf("unexpected static upload response: %d %q", recorder.Code, recorder.Body.String())
    }
}
```

- [ ] **Step 2: 运行测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool/app -run TestNewServesOnlyInjectedUploadDirectory -count=1
```

Expected: FAIL，返回 404，因为应用尚未配置 `/uploads` 静态映射。

- [ ] **Step 3: 添加显式上传目录配置与 server 映射**

在 `Options` 增加 `UploadDir string`，在 `Application` 增加 `uploadDir string`。构造时：

```go
uploadDir := options.UploadDir
if uploadDir == "" {
    uploadDir = filepath.Join("resource", "public", "uploads")
}
```

`bindRuntimeControllers` 使用 `baseService.NewUploadService(a.uploadDir)`，并把 service 传给 `baseController.Controllers(g.DB(), a.authManager, epsService, uploadService)`。更新该 factory 的参数及全部四个旧 compatibility shim 调用点，使它们显式传 `nil` upload service。

在 `registerRoutes` 的最前面、health 路由之前写入：

```go
a.server.SetIndexFolder(false)
a.server.AddStaticPath("/uploads", a.uploadDir)
```

不得调用 `SetServerRoot`，不得添加项目根目录到 `AddSearchPath`。

- [ ] **Step 4: 运行 app 和 Controller 测试，确认 Green**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/app/app.go cool/app/app_test.go modules/base/controller/controllers.go modules/base/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/permission.go
go test ./cool/app ./modules/base/controller -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交应用上传配置**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add cool/app/app.go cool/app/app_test.go modules/base/controller/controllers.go modules/base/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/permission.go
git commit -m $'feat: serve local upload files\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

### Task 6: 验证 EPS、HTTP 契约和真实 MySQL 行为

**Files:**
- Modify: `modules/base/eps_test.go`
- Create: `modules/base/custom_api_integration_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1 至 Task 5 的所有路由、Service、上传和权限行为。
- Produces: 非数据库 EPS 回归测试及 `COOL_CUSTOM_API_INTEGRATION=1` 可复制验收。

- [ ] **Step 1: 写 EPS 失败断言**

在 `modules/base/eps_test.go` 的实际 `data["base"]` 断言后添加：

```go
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseCommController").API, http.MethodPost, "/personUpdate", false)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseCommController").API, http.MethodGet, "/uploadMode", false)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseCommController").API, http.MethodPost, "/upload", false)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysUserEntity").API, http.MethodPost, "/move", false)
assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysDepartmentEntity").API, http.MethodPost, "/order", false)
for _, path := range []string{"/clear", "/setKeep", "/getKeep"} {
    assertEPSAPI(t, findEPSController(t, baseControllers, "BaseSysLogEntity").API, map[string]string{"/getKeep": http.MethodGet}[path], path, false)
}
```

将循环改为显式三条断言，避免 map 缺省 method：

```go
assertEPSAPI(t, log.API, http.MethodPost, "/clear", false)
assertEPSAPI(t, log.API, http.MethodPost, "/setKeep", false)
assertEPSAPI(t, log.API, http.MethodGet, "/getKeep", false)
```

- [ ] **Step 2: 运行 EPS 测试，确认 Red**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./modules/base -run TestEPSRouteReturnsAnonymousFullBootstrap -count=1
```

Expected: 在 Task 4 前 FAIL，缺失新增 API；Task 4 后应 PASS。

- [ ] **Step 3: 新建真实 MySQL 自定义 API 集成测试**

创建 `modules/base/custom_api_integration_test.go`，采用既有 auth/permission 集成测试相同模式：

1. 若 `COOL_CUSTOM_API_INTEGRATION != "1"`，`t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run custom API integration test")`。
2. schema sync 后清理并重新导入 `modules/base/db.json`、`modules/base/menu.json`；清理列表必须包含 `DELETE FROM base_sys_log`。
3. 使用 `g.Server(guid.S())`、`app.New(app.Options{StartServer:true, Server:server, UploadDir:t.TempDir()})` 启动独立 listener。
4. 通过验证码辅助函数登录 admin，创建并登录无菜单权限的 `limited` 用户。
5. 对 admin 依次断言：
   - `GET /admin/base/comm/uploadMode` 的 data 严格为 `{"mode":"local","type":"local"}`；
   - multipart `POST /admin/base/comm/upload` 返回 `/uploads/<date>/<random>.txt`，随后 GET 该 URL 返回上传内容；
   - `POST /admin/base/comm/personUpdate` 后 `GET /person` 显示新 `nickName`，且 username/status 未改变；
   - 插入两个部门、两个用户后 `POST /admin/base/sys/user/move` 使两用户 `department_id` 更新；
   - `POST /admin/base/sys/department/order` 更新 parent_id 和 order_num；
   - `POST /admin/base/sys/log/setKeep` 后 GET `getKeep` 返回 `45`，插入日志后 POST `clear` 使 `base_sys_log` 计数为 0。
6. 对 limited 用户逐一请求 `move`、`order`、`clear`、`setKeep`、`getKeep`，每次精确断言 HTTP 403 与 `{"code":1001,"message":"登录失效或无权限访问~"}`。
7. 未带 Authorization 请求 `/admin/base/comm/uploadMode` 与 `/admin/base/sys/log/getKeep`，两者必须 HTTP 401。
8. GET `/admin/base/open/eps`，解码为 `map[string][]eps.Controller` 并检查 Task 1 的 8 条 API。

HTTP multipart helper 使用：

```go
func postMultipart(t *testing.T, url, token, filename string, content []byte) testHTTPResponse {
    t.Helper()
    var body bytes.Buffer
    writer := multipart.NewWriter(&body)
    part, err := writer.CreateFormFile("file", filename)
    if err != nil { t.Fatal(err) }
    if _, err = part.Write(content); err != nil { t.Fatal(err) }
    if err = writer.Close(); err != nil { t.Fatal(err) }
    request, err := http.NewRequest(http.MethodPost, url, &body)
    if err != nil { t.Fatal(err) }
    request.Header.Set("Content-Type", writer.FormDataContentType())
    request.Header.Set("Authorization", token)
    return executeRequest(t, request)
}
```

- [ ] **Step 4: 运行集成测试，确认 Red/Green**

在 Task 4/5 之前运行应失败；所有实现完成后运行：

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run TestCustomAPIIntegration -count=1
```

Expected: PASS。若本地 MySQL 未运行，报告连接失败，不得把它表述为测试通过。

- [ ] **Step 5: 补充 README 验收说明**

在 `README.md` 的 EPS Bootstrap 验收章节后添加：

```markdown
## 首批自定义 API 验收

启动应用后，已登录用户可使用个人资料和本地上传接口；sys 管理接口还需要对应菜单权限：

```bash
go test ./modules/base/service ./modules/base/controller ./cool/app -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run TestCustomAPIIntegration -count=1
curl http://127.0.0.1:8001/admin/base/comm/uploadMode -H 'Authorization: <token>'
```

`uploadMode` 返回 `{"mode":"local","type":"local"}`。本地上传仅接受 multipart 字段 `file`，单文件最大 10MB，成功返回 `/uploads/YYYYMMDD/<随机文件名>`；上传目录不会提供目录列表。`user/move`、`department/order`、日志三个管理功能缺权限时返回 HTTP 403 与 `登录失效或无权限访问~`。
```

- [ ] **Step 6: 执行最终验证**

Run:

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
gofmt -w cool/controller/permission.go cool/controller/permission_test.go cool/app/app.go cool/app/app_test.go modules/base/service/custom_api.go modules/base/service/custom_api_test.go modules/base/service/sys_user.go modules/base/service/sys_department.go modules/base/service/sys_log.go modules/base/controller/comm/comm.go modules/base/controller/admin/sys_user.go modules/base/controller/admin/sys_department.go modules/base/controller/admin/sys_log.go modules/base/controller/controllers.go modules/base/controller/controllers_test.go modules/base/eps_test.go modules/base/custom_api_integration_test.go modules/base/permission_integration_test.go modules/base/routes.go modules/base/auth_routes.go modules/base/permission_routes.go modules/base/permission.go
go test ./cool/controller ./cool/eps ./cool/crud ./modules/base/service ./modules/base/controller ./modules/base ./cool/app -count=1
COOL_CUSTOM_API_INTEGRATION=1 go test ./modules/base -run TestCustomAPIIntegration -count=1
go test ./...
go vet ./...
git diff --check
```

Expected: 所有普通测试、全包测试、vet 和 diff 检查通过；真实 MySQL 集成测试通过。不得跳过受限用户的五条 403 精确 body 断言。

- [ ] **Step 7: 提交验收与文档**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add modules/base/eps_test.go modules/base/custom_api_integration_test.go README.md
git commit -m $'test: verify first custom API batch\n\nCo-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>'
```

---

## Plan Self-Review

- **Spec coverage:** Task 1 对齐 Node 403 状态和消息；Task 2 定义所有请求类型、白名单和本地上传；Task 3 实现用户移动、部门排序、日志配置和清空；Task 4 将八项能力绑定为 metadata route，因此权限与 EPS 来自同一来源；Task 5 限制静态暴露面；Task 6 覆盖 EPS、认证、权限、真实 HTTP/MySQL 和 README。
- **Placeholder scan:** 本计划不含 TBD、TODO、"适当处理"、"类似 Task" 或未指定的验收步骤。所有生产改动均有失败测试、最小实现、聚焦验证与显式暂存文件。
- **Type consistency:** `MoveRequest`、`DepartmentOrderItem`、`LogKeepRequest` 和 `UploadService` 均先于消费者定义；路由 name/path/permission 在 Task 4、EPS 和集成测试一致；403 消息在 Task 1 后由所有权限 route 复用。
- **Scope check:** 菜单 parse/create/export/import、param/html、密码更新、云存储、数据权限和定时清理均被排除。本计划只交付可以独立验收的首批八个 API。
