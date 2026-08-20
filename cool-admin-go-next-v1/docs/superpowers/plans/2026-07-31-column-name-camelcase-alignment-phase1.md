# 列名 camelCase 对齐 · 阶段一实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把项目里 738 处 snake_case 数据库列名字面量全部替换为 camelCase，与 cool-admin-midway 契约对齐，删库重建后表结构与 Node 版一致。

**Architecture:** 纯字面量替换，不动任何业务逻辑、不改任何函数签名。利用 GoFrame v2.10.2 `mappingAndFilterData` 的模糊匹配（`MapPossibleItemByKey`）兜底——替换未完成时仍可工作但不被任何测试发现，故必须以静态扫描为准。按模块切分、每步独立 build + test。

**Tech Stack:** GoFrame v2.10.2、Go 1.21+、MySQL 8.0、yq (yaml 处理)、grep + sed (字面量替换)。

## 全局约束

- 替换集合严格限定为设计文档 §6.1 的 42 个列名，禁止通配
- 替换后必须跑 grep 验证该模块命中数归零
- 每步必须 `go build ./...` + `go test ./...` 通过方可进入下一步
- 数据库可删，删库后 `autoSync` 重建
- GoFrame 模糊匹配特性会在漏改时**静默通过**——这意味着 grep 清零是唯一可信的验收
- 严格大小写：替换目标列名必须按设计文档 §6.1 大小写（如 `departmentId` 而非 `departmentid`）——MySQL 结果集 key 取自 SQL 书写形式，写错大小写会污染 API 输出

## 替换映射表（42 项）

```
branch_key       → branchKey
c_key            → cKey
c_value          → cValue
create_time      → createTime
data_type        → dataType
department_id    → departmentId
department_id_list → departmentIdList
end_date         → endDate
entity_info      → entityInfo
head_img         → headImg
is_show          → isShow
job_id           → jobId
keep_alive       → keepAlive
key_name         → keyName
last_execute_time → lastExecuteTime
lock_expire_time → lockExpireTime
lock_owner       → lockOwner
menu_id          → menuId
menu_id_list     → menuIdList
next_run_time    → nextRunTime
nick_name        → nickName
order_num        → orderNum
parent_id        → parentId
parent_item_id   → parentItemId
password_v       → passwordV
primary_key      → primaryKey
recycle_id       → recycleId
remaining_count  → remainingCount
repeat_conf      → repeatConf
restore_order    → restoreOrder
restore_status   → restoreStatus
role_id          → roleId
socket_id        → socketId
start_date       → startDate
table_name       → tableName
task_id          → taskId
task_type        → taskType
tenant_id        → tenantId
type_id          → typeId
update_time      → updateTime
user_id          → userId
view_path        → viewPath
```

---

## Task 1: 准备工具

**Files:**
- Create: `scripts/replace-columns.sh`（shell 脚本，用 sed 批量替换字面量）
- Create: `scripts/verify-columns.sh`（验证脚本，按模块统计残留数）

**Interfaces:**
- 产生: `scripts/replace-columns.sh <mapping-file>`，对 `*.go`/`*.yaml`/`*.yml`/`*.json`/`*.md` 做精确字面量替换
- 产生: `scripts/verify-columns.sh <module-path>`，返回该路径下未替换的列名及命中数

### 步骤

- [ ] **Step 1: 创建映射文件 `scripts/column-mapping.txt`**

按设计文档 §6.1 的 42 项，每行 `旧名→新名` 格式：

```
branch_key→branchKey
c_key→cKey
c_value→cValue
create_time→createTime
data_type→dataType
department_id→departmentId
... （42 行全列）
view_path→viewPath
```

- [ ] **Step 2: 写替换脚本 `scripts/replace-columns.sh`**

要求：
1. 接收一个模块路径参数（默认 `.` 全项目）
2. 读取 `scripts/column-mapping.txt`
3. 对每个文件类型 `*.go`/`*.yaml`/`*.yml`/`*.json`/`*.md`，逐项做精确匹配替换
4. 精确匹配带引号的完整字面量（如 `"user_id"`），不做前缀/子串匹配
5. 跳过 `.git/`、`node_modules/`、`dist/`、`*.min.*`
6. 输出每个文件的替换数

```bash
#!/usr/bin/env bash
set -euo pipefail

SCOPE="${1:-.}"
MAPPING="$(dirname "$0")/column-mapping.txt"

if [ ! -f "$MAPPING" ]; then
  echo "Mapping file not found: $MAPPING" >&2
  exit 1
fi

total=0
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  # 精确匹配带双引号的旧字面量
  pattern="\"$old\""
  replacement="\"$new\""
  while IFS= read -r -d '' file; do
    before=$(grep -c "$pattern" "$file" 2>/dev/null || true)
    [ "$before" = "0" ] && continue
    sed -i.tmp "s|$pattern|$replacement|g" "$file"
    rm -f "$file.tmp"
    echo "  $file: $old → $new × $before"
    total=$((total + before))
  done < <(find "$SCOPE" \( -name '*.go' -o -name '*.yaml' -o -name '*.yml' -o -name '*.json' -o -name '*.md' \) \
           -not -path '*/\.git/*' -not -path '*/node_modules/*' -not -path '*/dist/*' -print0)
done < "$MAPPING"

echo "Total replacements: $total"
```

- [ ] **Step 3: 写验证脚本 `scripts/verify-columns.sh`**

要求：统计指定路径下未替换的列名及命中数（应全为 0）

```bash
#!/usr/bin/env bash
set -euo pipefail

SCOPE="${1:-.}"
MAPPING="$(dirname "$0")/column-mapping.txt"

remaining=0
while IFS='→' read -r old new; do
  [ -z "$old" ] && continue
  pattern="\"$old\""
  count=$(grep -rE "$pattern" "$SCOPE" \
    --include='*.go' --include='*.yaml' --include='*.yml' --include='*.json' --include='*.md' \
    --exclude-dir=.git --exclude-dir=node_modules --exclude-dir=dist 2>/dev/null | wc -l | tr -d ' ')
  if [ "$count" -gt 0 ]; then
    echo "  $old → $new: $count remaining"
    remaining=$((remaining + count))
  fi
done < "$MAPPING"

echo "---"
echo "Total remaining: $remaining"
exit $remaining
```

- [ ] **Step 4: 加执行权限并跑全项目基线**

```bash
chmod +x scripts/replace-columns.sh scripts/verify-columns.sh
./scripts/verify-columns.sh .
```

期望：`Total remaining: 738`（基线值，允许 ±1，因 grep 统计可能因转义差 1）

- [ ] **Step 5: 跑 build + test 确认基线干净**

```bash
go build ./...
go test ./... 2>&1 | tail -20
```

期望：build 成功，test 可能有失败（基线允许）但不应因本步操作引入新失败。

- [ ] **Step 6: 提交**

```bash
git add scripts/column-mapping.txt scripts/replace-columns.sh scripts/verify-columns.sh
git commit -m "chore: add column-name replacement tools"
```

---

## Task 2: 公共模块替换（cool/）

**Files:**
- Modify: `cool/entity/entity.go`（`BaseFields()` 中 3 个 `NewField`）
- Modify: `cool/db/tenant/metadata.go:12`（`tenantColumn` 常量）
- Modify: `cool/module/seed/parser.go:146`（`ParentColumn` 字段）
- Modify: `manifest/config/config.yaml:26-27`（时间字段配置）

**Interfaces:**
- Consumes: `scripts/replace-columns.sh`（任务 0 产生）
- 产生: `entity.NewField("createTime", "createTime", "datetime")` 形式的双参数签名（待阶段 3a 收尾，本阶段只改 ColumnName 字面量）
- 产生: `tenantColumn = "tenantId"`（**租户隔离安全边界**）
- 产生: `mapped.ParentColumn = "parentId"`（**seed 菜单树父子关系**）

### 步骤

- [ ] **Step 1: 跑 `cool/` 模块验证基线**

```bash
./scripts/verify-columns.sh cool/ manifest/
```

期望：`Total remaining: 4`（3 个 BaseFields + tenantColumn + ParentColumn + config 2 处，合计 4-7，按实际数记录为基线）

- [ ] **Step 2: 替换 `cool/entity/entity.go` 中 3 个 BaseFields**

定位 `NewField("create_time", ...)`、`NewField("update_time", ...)`、`NewField("tenant_id", ...)`，将第一个参数改为 `"createTime"`/`"updateTime"`/`"tenantId"`，ColumnName 与 JSONName 一致。

- [ ] **Step 3: 替换 `cool/db/tenant/metadata.go:12`**

```go
tenantColumn    = "tenant_id"   // 改前
tenantColumn    = "tenantId"    // 改后
```

- [ ] **Step 4: 替换 `cool/module/seed/parser.go:146`**

```go
mapped.ParentColumn = "parent_id"   // 改前
mapped.ParentColumn = "parentId"    // 改后
```

- [ ] **Step 5: 替换 `manifest/config/config.yaml` 时间字段配置**

```yaml
createdAt: create_time   # 改前
createdAt: createTime    # 改后
updatedAt: update_time   # 改前
updatedAt: updateTime    # 改后
```

- [ ] **Step 6: 跑 cool/ 模块验证**

```bash
./scripts/verify-columns.sh cool/ manifest/
```

期望：`Total remaining: 0`（或仅剩任务 0 映射表本身里的 42 个）

- [ ] **Step 7: 跑 build + test**

```bash
go build ./... && go test ./... 2>&1 | tail -30
```

期望：build 成功，test 全部通过。租户相关测试（`tenant_*.go`、`recycle_runtime_test.go`）必须绿——它们覆盖 `tenantColumn` 修改。

- [ ] **Step 8: 提交**

```bash
git add cool/entity/entity.go cool/db/tenant/metadata.go cool/module/seed/parser.go manifest/config/config.yaml
git commit -m "refactor: align cool/ public module column names to camelCase"
```

---

## Task 3: modules/base 模块替换

**Files:**
- Modify: `modules/base/entity/sys/*.go`（8 个文件，16 个 NewField）
- Modify: `modules/base/service/sys/*.go`（7 个 service，约 150 处字面量）

**Interfaces:**
- 产生: 16 个 `NewField(jsonName, columnName, dataType)` 中的 columnName 与 jsonName 一致
- 产生: service 内的 `Where("user_id", ...)` 等字面量全部改为 `Where("userId", ...)`

### 步骤

- [ ] **Step 1: 跑 modules/base 基线**

```bash
./scripts/verify-columns.sh modules/base/
```

记录基线值（预期约 200+ 处）。

- [ ] **Step 2: 替换 entity 文件**

```bash
./scripts/replace-columns.sh modules/base/entity/
```

脚本会替换 entity 文件里的所有 NewField 字面量。打开 `user.go`、`role.go`、`param.go`、`menu.go`、`department.go`、`conf.go`、`log.go`、`user_role.go`、`role_menu.go`、`role_department.go`，人工核对每个 NewField 调用——ColumnName 应已变为与 JSONName 一致。

- [ ] **Step 3: 替换 service 文件**

```bash
./scripts/replace-columns.sh modules/base/service/
```

服务文件里有大量 `Where("user_id", ...)`、`Fields("user_id")` 等会一并替换。

- [ ] **Step 4: 跑 modules/base 验证**

```bash
./scripts/verify-columns.sh modules/base/
```

期望：`Total remaining: 0`

- [ ] **Step 5: 跑 build + test**

```bash
go build ./... && go test ./modules/base/... 2>&1 | tail -30
```

期望：build 成功，base 模块全部测试通过。

- [ ] **Step 6: 启动运行时验收（任务 0 的 verify 技能）**

```bash
cat >/tmp/cool-go-verify.yaml <<'YAML'
server:
  address: ":18001"
  openapiPath: "/api.json"
  swaggerPath: "/swagger"
logger:
  level: "all"
  stdout: true
database:
  default:
    link: "mysql:root:123456@tcp(127.0.0.1:3306)/cool-go?loc=Local&parseTime=true&charset=utf8mb4"
    debug: true
cool:
  initDB: true
  initMenu: true
  initJudge: "db"
  schema:
    autoSync: true
    safeMode: true
    logDiff: true
  eps:
    enable: true
  auth:
    jwtSecret: "cool-admin-go-next-dev-secret"
    tokenExpire: 7200
    refreshExpire: 604800
YAML

# 先删库（已确认数据库可删）
docker exec MySQL mysql -uroot -p123456 -e "DROP DATABASE IF EXISTS cool-go; CREATE DATABASE cool-go;"

# 后台启动
GF_GCFG_FILE=/tmp/cool-go-verify.yaml go run . > /tmp/go-server.log 2>&1 &
SERVER_PID=$!
echo "Server PID: $SERVER_PID"

# 等待启动
for i in {1..30}; do
  if curl -sS http://127.0.0.1:18001/health > /dev/null 2>&1; then
    echo "Server ready after ${i}s"
    break
  fi
  sleep 1
done

# 4 条验收探测
echo "--- 探测 1: health ---"
curl -sS http://127.0.0.1:18001/health | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['code']==1000 and d['data']['status']=='ok', d; print('OK')"

echo "--- 探测 2: user page (首登录需 token) ---"
TOKEN=$(curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin","captchaId":"","verifyCode":""}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")
curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"page":1,"size":15}' | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['code']==1000, d; assert 'list' in d['data'] and 'pagination' in d['data']; assert not any('password' in u for u in d['data']['list']), 'password leaked'; print('OK')"

echo "--- 探测 3: API 字段名大小写比对 ---"
curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"page":1,"size":15}' | python3 -c "
import sys, json
d = json.load(sys.stdin)
expected = {'id','createTime','updateTime','tenantId','departmentId','userId','name','username','nickName','headImg','phone','email','remark','status','socketId'}
actual = set(d['data']['list'][0].keys()) if d['data']['list'] else set()
mismatch = expected - actual
extra = actual - expected
assert not mismatch, f'missing fields: {mismatch}'
print('Field names OK')
"

echo "--- 探测 4: 注入式 sort 应被拒 ---"
curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"sort":"username desc; drop table"}' | python3 -c "import sys,json; d=json.load(sys.stdin); assert d['code'] != 1000, 'injection succeeded'; print('OK, blocked')"

# 关停
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
```

期望：四条探测全 OK。**第 3 条「字段名大小写」是本阶段最关键的验收**——见设计文档 §7「大小写写错污染 API 输出」。

- [ ] **Step 7: 提交**

```bash
git add modules/base/
git commit -m "refactor: align modules/base column names to camelCase"
```

---

## Task 4: modules/recycle 模块替换

**Files:**
- Modify: `modules/recycle/entity/{item,data}.go`（约 20 处字面量）
- Modify: `modules/recycle/service/data.go`、`modules/recycle/event/data.go`（约 50 处字面量）

**Interfaces:**
- 产生: 涉及 `recycle_item` / `recycle_data` 表的 9 个独有列名（`recycleId`、`tableName`、`primaryKey`、`branchKey`、`parentItemId`、`restoreOrder`、`entityInfo`、`restoreStatus`、`remainingCount`）全部 camelCase

### 步骤

- [ ] **Step 1: 跑 modules/recycle 基线**

```bash
./scripts/verify-columns.sh modules/recycle/
```

记录基线。

- [ ] **Step 2: 替换 entity + service + event 文件**

```bash
./scripts/replace-columns.sh modules/recycle/
```

- [ ] **Step 3: 跑 modules/recycle 验证**

```bash
./scripts/verify-columns.sh modules/recycle/
```

期望：`Total remaining: 0`

- [ ] **Step 4: 跑 build + test**

```bash
go build ./... && go test ./modules/recycle/... 2>&1 | tail -30
```

期望：build 成功，recycle 测试全绿（特别是 `restore_test.go` 覆盖字段还原逻辑）。

- [ ] **Step 5: 提交**

```bash
git add modules/recycle/
git commit -m "refactor: align modules/recycle column names to camelCase"
```

---

## Task 5: modules/task 模块替换

**Files:**
- Modify: `modules/task/entity/{info,log}.go`（约 15 处）
- Modify: `modules/task/service/store.go`（41 处，是单文件命中最高）

**Interfaces:**
- 产生: `task_info` 表的 9 个独有列名（`jobId`、`repeatConf`、`startDate`、`endDate`、`nextRunTime`、`taskType`、`lastExecuteTime`、`lockExpireTime`、`lockOwner`）全部 camelCase

### 步骤

- [ ] **Step 1: 跑 modules/task 基线**

```bash
./scripts/verify-columns.sh modules/task/
```

记录基线。

- [ ] **Step 2: 替换 entity + service 文件**

```bash
./scripts/replace-columns.sh modules/task/
```

- [ ] **Step 3: 跑 modules/task 验证**

```bash
./scripts/verify-columns.sh modules/task/
```

期望：`Total remaining: 0`

- [ ] **Step 4: 跑 build + test**

```bash
go build ./... && go test ./modules/task/... 2>&1 | tail -30
```

期望：build 成功，task 测试全绿（特别是 `recycle_integration_test.go` 覆盖回收站级联还原）。

- [ ] **Step 5: 提交**

```bash
git add modules/task/
git commit -m "refactor: align modules/task column names to camelCase"
```

---

## Task 6: modules/dict 模块替换

**Files:**
- Modify: `modules/dict/entity/{type,info}.go`（约 7 处）
- Modify: `modules/dict/service/{dict_type,dict_info}.go`（约 9 处）

**Interfaces:**
- 产生: `dict_info` 表 3 个独有列名（`typeId`、`orderNum`、`parentId`）全部 camelCase

### 步骤

- [ ] **Step 1: 跑 modules/dict 基线**

```bash
./scripts/verify-columns.sh modules/dict/
```

- [ ] **Step 2: 替换 entity + service 文件**

```bash
./scripts/replace-columns.sh modules/dict/
```

- [ ] **Step 3: 跑 modules/dict 验证**

```bash
./scripts/verify-columns.sh modules/dict/
```

期望：`Total remaining: 0`

- [ ] **Step 4: 跑 build + test**

```bash
go build ./... && go test ./modules/dict/... 2>&1 | tail -30
```

期望：build 成功，dict 测试全绿。

- [ ] **Step 5: 提交**

```bash
git add modules/dict/
git commit -m "refactor: align modules/dict column names to camelCase"
```

---

## Task 7: 全项目最终验收

**Files:** 无新文件，全项目验收。

### 步骤

- [ ] **Step 1: 全项目 grep 清零**

```bash
./scripts/verify-columns.sh .
```

期望：`Total remaining: 0`（允许任务 0 映射表本身命中 42 次，可加 `--exclude=scripts/` 排除）

- [ ] **Step 2: 全量 build + test**

```bash
go build ./... && go test ./... 2>&1 | tee /tmp/test-final.log | tail -20
```

期望：全绿。失败则**先看测试是否因列名引用了错误的字面量**，回查 Step 1 验证是否漏改。

- [ ] **Step 3: 删库重建 + 完整运行时验收**

```bash
docker exec MySQL mysql -uroot -p123456 -e "DROP DATABASE IF EXISTS cool-go; CREATE DATABASE cool-go;"

GF_GCFG_FILE=/tmp/cool-go-verify.yaml go run . > /tmp/go-server.log 2>&1 &
SERVER_PID=$!

# 等待启动
for i in {1..30}; do
  if curl -sS http://127.0.0.1:18001/health > /dev/null 2>&1; then
    echo "Server ready after ${i}s"
    break
  fi
  sleep 1
done

# 健康检查
curl -sS http://127.0.0.1:18001/health

# 登录获取 token
TOKEN=$(curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"admin","captchaId":"","verifyCode":""}' | python3 -c "import sys,json; print(json.load(sys.stdin)['data']['token'])")

# 关键探测：列名是否与 Node 版契约一致
docker exec MySQL mysql -uroot -p123456 -e "USE cool-go; SHOW COLUMNS FROM base_sys_user;" 2>&1 | grep -v "Using a password"

# 期望列名: id, createTime, updateTime, tenantId, departmentId, userId, name, username, password, passwordV, nickName, headImg, phone, email, remark, status, socketId

# 字段名 API 比对（设计文档 §8 第 6 项）
curl -sS -X POST http://127.0.0.1:18001/admin/base/sys/user/page -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{"page":1,"size":15}' | python3 -c "
import sys, json
d = json.load(sys.stdin)
if not d['data']['list']:
    print('No data')
    sys.exit(0)
keys = set(d['data']['list'][0].keys())
# 期望至少包含 Node 版契约的 camelCase 字段名
expected = {'id','createTime','updateTime','tenantId','departmentId','userId','name','username','nickName','headImg','phone','email','remark','status','socketId'}
missing = expected - keys
assert not missing, f'字段名大小写或缺失: {missing}'
print('All field names match Node contract: OK')
"

# 关闭
kill $SERVER_PID
wait $SERVER_PID 2>/dev/null
```

- [ ] **Step 4: 修复「顺带发现的现存缺陷」（user.go username trim bug，可选）**

若决定修复，参考设计文档 §5 阶段二末尾说明：
- `user.go` Update 路径 `username` 查重后回写 trim 值到 data
- `user.go:563`、`user.go:569` 同类 trim 已就地清洗，无需回写
- 修完跑 `go test ./modules/base/service/sys/...` 验证

- [ ] **Step 5: 更新设计文档第 94/151 行**

修改 `docs/superpowers/specs/2026-07-14-cool-admin-go-next-schema-sync-design.md` 第 94、151 行：

```diff
- 保留 MySQL snake_case 字段名。
+ 字段名以 base-api-contract.md 的表结构契为准（camelCase）。早期临时采用 snake_case，现已对齐。
```

```diff
- DB 字段使用 snake_case。
+ DB 字段使用 camelCase（与 HTTP JSON / EPS 一致，与 Node 版共用）。
```

- [ ] **Step 6: 提交验收产物**

```bash
git add docs/superpowers/specs/2026-07-14-cool-admin-go-next-schema-sync-design.md
git commit -m "docs: align schema-sync spec with camelCase contract"
```

---

## 范围外（下一阶段做）

- **阶段二**：删除 5 个 service 的 `xxxUpdateData` / `xxxMutationRow` / `xxxRowFromData`
- **阶段 3a**：`NewField(jsonName, columnName, dataType)` 收敛为 `NewField(name, dataType)`
- **阶段 3b**：`Field.ColumnName` 与 `JSONName` 合并为单一 `Name`

这些**不依赖阶段一完成即可独立做**，也**不阻塞**阶段一收益。单独排期。
