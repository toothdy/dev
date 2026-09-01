# cool-next 包名统一 gn 前缀重构计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** cool-next 框架层全部包目录名与包声明名统一加 `gn` 前缀(学 GoFrame 的 `g+领域词` 模式),使业务代码引用框架包时**零别名**。

**Architecture:** 单 Go 模块内机械重命名:git mv 目录 → 改 package 声明 → 用一次性 Python 脚本批量重写 import 路径、别名与标识符前缀 → 同步 codegen 生成器(路径常量、别名表)→ 重新生成 modules_gen.go 验证幂等 → 同步文档。编译器与现有测试链(`make check`)兜底。

**Tech Stack:** Go 1.26 单模块 `github.com/toothdy/cool-admin-go-next`;git mv 保留历史;Python3(macOS 自带)做批量重写;Makefile 检查链。

**Spec:** 无既有 spec。本次决策来自用户对话拍板:前缀 `gn`(g 沿袭 gf 血统、n 取自 cool-admin-go-**next**),范围全部统一,业务模块分层包名(`modules/*/entity|service|controller`)保持不变,文档随重构同步修改(用户明确:"不用管文档,文档不合理的时候是需要去改的")。Task 4 产出设计文档 `docs/superpowers/specs/2026-09-01-gn-package-prefix-rename-design.md` 作为本次的 spec 存档。

## 命名映射总表(所有任务共用)

### 目录与包名(24 组,目录名 = 包声明名)

| 旧目录 | 新目录 | 旧 package | 新 package |
|---|---|---|---|
| `cool-next/auth` | `cool-next/gnauth` | `auth` | `gnauth` |
| `cool-next/auth/bcrypt` | `cool-next/gnauth/gnbcrypt` | `bcrypt` | `gnbcrypt` |
| `cool-next/auth/session` | `cool-next/gnauth/gnsession` | `session` | `gnsession` |
| `cool-next/codegen` | `cool-next/gncodegen` | `codegen` | `gncodegen` |
| `cool-next/core/app` | `cool-next/core/gnapp` | `app` | `gnapp` |
| `cool-next/core/config` | `cool-next/core/gnconfig` | `config` | `gnconfig` |
| `cool-next/core/controller` | `cool-next/core/gncontroller` | `controller` | `gncontroller` |
| `cool-next/core/entity` | `cool-next/core/gnentity` | `entity` | `gnentity` |
| `cool-next/core/exception` | `cool-next/core/gnexception` | `exception` | `gnexception` |
| `cool-next/core/http` | `cool-next/core/gnhttp` | `apphttp` | `gnhttp` |
| `cool-next/core/module` | `cool-next/core/gnmodule` | `module` | `gnmodule` |
| `cool-next/core/route` | `cool-next/core/gnroute` | `route` | `gnroute` |
| `cool-next/core/service` | `cool-next/core/gnservice` | `service` | `gnservice` |
| `cool-next/crud` | `cool-next/gncrud` | `crud` | `gncrud` |
| `cool-next/db` | `cool-next/gndb` | `db` | `gndb` |
| `cool-next/db/driver` | `cool-next/gndb/gndriver` | `driver` | `gndriver` |
| `cool-next/db/recycle` | `cool-next/gndb/gnrecycle` | `recycle` | `gnrecycle` |
| `cool-next/db/schema` | `cool-next/gndb/gnschema` | `schema` | `gnschema` |
| `cool-next/db/tx` | `cool-next/gndb/gntx` | `tx` | `gntx` |
| `cool-next/eps` | `cool-next/gneps` | `eps` | `gneps` |
| `cool-next/grpc` | `cool-next/gngrpc` | `grpc` | `gngrpc` |
| `cool-next/outbox` | `cool-next/gnoutbox` | `outbox` | `gnoutbox` |
| `cool-next/outbox/store` | `cool-next/gnoutbox/gnstore` | `store` | `gnstore` |
| `cool-next/seed` | `cool-next/gnseed` | `seed` | `gnseed` |

### import 路径映射(24 条,长路径优先)

```
github.com/toothdy/cool-admin-go-next/cool-next/auth/bcrypt   → …/gnauth/gnbcrypt
github.com/toothdy/cool-admin-go-next/cool-next/auth/session  → …/gnauth/gnsession
github.com/toothdy/cool-admin-go-next/cool-next/auth          → …/gnauth
github.com/toothdy/cool-admin-go-next/cool-next/core/app      → …/core/gnapp
github.com/toothdy/cool-admin-go-next/cool-next/core/config   → …/core/gnconfig
github.com/toothdy/cool-admin-go-next/cool-next/core/controller → …/core/gncontroller
github.com/toothdy/cool-admin-go-next/cool-next/core/entity   → …/core/gnentity
github.com/toothdy/cool-admin-go-next/cool-next/core/exception → …/core/gnexception
github.com/toothdy/cool-admin-go-next/cool-next/core/http     → …/core/gnhttp
github.com/toothdy/cool-admin-go-next/cool-next/core/module   → …/core/gnmodule
github.com/toothdy/cool-admin-go-next/cool-next/core/route    → …/core/gnroute
github.com/toothdy/cool-admin-go-next/cool-next/core/service  → …/core/gnservice
github.com/toothdy/cool-admin-go-next/cool-next/codegen       → …/gncodegen
github.com/toothdy/cool-admin-go-next/cool-next/crud          → …/gncrud
github.com/toothdy/cool-admin-go-next/cool-next/db/driver     → …/gndb/gndriver
github.com/toothdy/cool-admin-go-next/cool-next/db/recycle    → …/gndb/gnrecycle
github.com/toothdy/cool-admin-go-next/cool-next/db/schema     → …/gndb/gnschema
github.com/toothdy/cool-admin-go-next/cool-next/db/tx         → …/gndb/gntx
github.com/toothdy/cool-admin-go-next/cool-next/db            → …/gndb
github.com/toothdy/cool-admin-go-next/cool-next/eps           → …/gneps
github.com/toothdy/cool-admin-go-next/cool-next/grpc          → …/gngrpc
github.com/toothdy/cool-admin-go-next/cool-next/outbox/store  → …/gnoutbox/gnstore
github.com/toothdy/cool-admin-go-next/cool-next/outbox        → …/gnoutbox
github.com/toothdy/cool-admin-go-next/cool-next/seed          → …/gnseed
```

(表中 `…` = `github.com/toothdy/cool-admin-go-next`)

### 标识符前缀映射(旧引用前缀 → 新)

既有显式别名:`coreentity→gnentity`、`coreservice→gnservice`、`corecontroller→gncontroller`、`coreroute→gnroute`、`coredb→gndb`、`corerecycle→gnrecycle`、`dbschema→gnschema`、`dbtx→gntx`、`outboxstore→gnstore`、`apphttp→gnhttp`、`coolgrpc→gngrpc`、`authbcrypt→gnbcrypt`、`cooloutbox→gnoutbox`、`query→gncrud`、`q→gncrud`、`dto→保留(见 Task 2 说明)`。

无别名导入的默认名 = 旧包声明名,映射同目录表;`core/http` 特例:默认名 `apphttp` → `gnhttp`。

## Global Constraints

- 业务模块包名不动:`modules/*/entity|service|controller|dto|middleware|schedule` 保持现有命名(cool-admin 生态分层惯例)。
- 不改 `go.mod` 模块路径;`cool-next/` 顶层目录名保留(框架层顶级,README 语义)。
- gf(`github.com/gogf/gf/v2/...` 全部)引用不动;`g`、`gdb`、`ghttp` 等 gf 包导入不变。
- 仓库根执行所有命令;macOS sed 用 `sed -i ''`。
- 本机 Bash 的 grep 是 ugrep,会漏 gitignored 文件:一律用 `command grep`。
- 每个任务结束提交一次 commit,中文、带 scope,风格随仓库:`refactor(cool-next): …`。
- 提交信息尾行:`Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。
- 验证基线:`test/integration` 存在**预存失败**(outbox 拓扑 panic,见 memory cool-next-baseline-e2e-test-fails),本次要求"不新增失败",不要求修复它。
- 历史计划文档 `docs/superpowers/plans/*.md` 不改(历史记录);specs 与 README 必须同步。

---

### Task 1: 框架层目录与包名迁移

**Files:**
- Rename: 24 组目录(见映射总表)
- Modify: `cool-next/**` 全部 `.go` 文件的 package 声明、import、标识符前缀
- Create: `/tmp/gn_rewrite.py`(一次性工具,不进仓库)

**Interfaces:**
- Consumes: 无(首个任务)
- Produces: `cool-next/` 内部自洽(gn* 包名),Task 2 的 gncodegen 可编译可测试

- [ ] **Step 1: git mv 目录(嵌套先父后子)**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git mv cool-next/auth cool-next/gnauth
git mv cool-next/gnauth/bcrypt cool-next/gnauth/gnbcrypt
git mv cool-next/gnauth/session cool-next/gnauth/gnsession
for d in app config controller entity exception http module route service; do
  git mv cool-next/core/$d cool-next/core/gn$d
done
git mv cool-next/codegen cool-next/gncodegen
git mv cool-next/crud cool-next/gncrud
git mv cool-next/db cool-next/gndb
git mv cool-next/gndb/driver cool-next/gndb/gndriver
git mv cool-next/gndb/recycle cool-next/gndb/gnrecycle
git mv cool-next/gndb/schema cool-next/gndb/gnschema
git mv cool-next/gndb/tx cool-next/gndb/gntx
git mv cool-next/eps cool-next/gneps
git mv cool-next/grpc cool-next/gngrpc
git mv cool-next/outbox cool-next/gnoutbox
git mv cool-next/gnoutbox/store cool-next/gnoutbox/gnstore
git mv cool-next/seed cool-next/gnseed
```

- [ ] **Step 2: 批量改 package 声明**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
sed -i '' 's/^package auth$/package gnauth/' cool-next/gnauth/*.go
sed -i '' 's/^package bcrypt$/package gnbcrypt/' cool-next/gnauth/gnbcrypt/*.go
sed -i '' 's/^package session$/package gnsession/' cool-next/gnauth/gnsession/*.go
for d in app config controller entity exception module route service; do
  sed -i '' "s/^package $d\$/package gn$d/" cool-next/core/gn$d/*.go
done
sed -i '' 's/^package apphttp$/package gnhttp/' cool-next/core/gnhttp/*.go
sed -i '' 's/^package codegen$/package gncodegen/' cool-next/gncodegen/*.go
sed -i '' 's/^package crud$/package gncrud/' cool-next/gncrud/*.go
sed -i '' 's/^package db$/package gndb/' cool-next/gndb/*.go
for d in driver recycle schema tx; do
  sed -i '' "s/^package $d\$/package gn$d/" cool-next/gndb/gn$d/*.go
done
sed -i '' 's/^package eps$/package gneps/' cool-next/gneps/*.go
sed -i '' 's/^package grpc$/package gngrpc/' cool-next/gngrpc/*.go
sed -i '' 's/^package outbox$/package gnoutbox/' cool-next/gnoutbox/*.go
sed -i '' 's/^package store$/package gnstore/' cool-next/gnoutbox/gnstore/*.go
sed -i '' 's/^package seed$/package gnseed/' cool-next/gnseed/*.go
```

- [ ] **Step 3: 写批量重写脚本 /tmp/gn_rewrite.py**

完整脚本(原样保存):

```python
#!/usr/bin/env python3
"""cool-next gn 前缀包改名批量重写工具。

用法: python3 /tmp/gn_rewrite.py <目录或文件>...
对目标内所有 .go 文件:
1. import 行(含字符串字面量中的合成源码行):旧路径→新路径、别名映射、同名别名省略
2. 其余行中的旧路径字符串(如 PackagePath 字段值、测试合成源码)同步替换
3. 文件内旧包标识符前缀 → 新包前缀(仅替换该文件实际导入的框架包)
幂等:可重复执行。
"""
import os
import re
import sys

OLD_BASE = "github.com/toothdy/cool-admin-go-next/"
# (旧路径后缀, 新路径后缀, 旧默认包名/别名集合, 新包名)  长路径优先
MAP = [
    ("cool-next/auth/bcrypt", "cool-next/gnauth/gnbcrypt", ["bcrypt", "authbcrypt"], "gnbcrypt"),
    ("cool-next/auth/session", "cool-next/gnauth/gnsession", ["session"], "gnsession"),
    ("cool-next/auth", "cool-next/gnauth", ["auth"], "gnauth"),
    ("cool-next/core/app", "cool-next/core/gnapp", ["app"], "gnapp"),
    ("cool-next/core/config", "cool-next/core/gnconfig", ["config"], "gnconfig"),
    ("cool-next/core/controller", "cool-next/core/gncontroller", ["controller", "corecontroller"], "gncontroller"),
    ("cool-next/core/entity", "cool-next/core/gnentity", ["entity", "coreentity"], "gnentity"),
    ("cool-next/core/exception", "cool-next/core/gnexception", ["exception"], "gnexception"),
    ("cool-next/core/http", "cool-next/core/gnhttp", ["http", "apphttp"], "gnhttp"),
    ("cool-next/core/module", "cool-next/core/gnmodule", ["module"], "gnmodule"),
    ("cool-next/core/route", "cool-next/core/gnroute", ["route", "coreroute"], "gnroute"),
    ("cool-next/core/service", "cool-next/core/gnservice", ["service", "coreservice"], "gnservice"),
    ("cool-next/codegen", "cool-next/gncodegen", ["codegen"], "gncodegen"),
    ("cool-next/crud", "cool-next/gncrud", ["crud", "query", "q"], "gncrud"),
    ("cool-next/db/driver", "cool-next/gndb/gndriver", ["driver"], "gndriver"),
    ("cool-next/db/recycle", "cool-next/gndb/gnrecycle", ["recycle", "corerecycle"], "gnrecycle"),
    ("cool-next/db/schema", "cool-next/gndb/gnschema", ["schema", "dbschema"], "gnschema"),
    ("cool-next/db/tx", "cool-next/gndb/gntx", ["tx", "dbtx"], "gntx"),
    ("cool-next/db", "cool-next/gndb", ["db", "coredb"], "gndb"),
    ("cool-next/eps", "cool-next/gneps", ["eps"], "gneps"),
    ("cool-next/grpc", "cool-next/gngrpc", ["grpc", "coolgrpc"], "gngrpc"),
    ("cool-next/outbox/store", "cool-next/gnoutbox/gnstore", ["store", "outboxstore"], "gnstore"),
    ("cool-next/outbox", "cool-next/gnoutbox", ["outbox", "cooloutbox"], "gnoutbox"),
    ("cool-next/seed", "cool-next/gnseed", ["seed"], "gnseed"),
]

# import 行:行首空白 + 可选别名 + 引号路径(覆盖 import 块行与单行/合成 import)
IMPORT_LINE = re.compile(r'^(\s*)((?:import\s+)?)(\w+\s+)?"([^"]+)"\s*$')
PREFIX_RE = {old: re.compile(r"\b" + re.escape(old) + r"\.") for entry in MAP for old in entry[2]}


def find_entry(path):
    for suffix, new_suffix, olds, new_name in MAP:
        if path == OLD_BASE + suffix or path.endswith(suffix):
            return suffix, new_suffix, olds, new_name
    return None


def rewrite_import_line(match):
    indent, keyword, alias, path = match.group(1), match.group(2), match.group(3), match.group(4)
    entry = find_entry(path)
    if entry is None:
        return None
    _, new_suffix, olds, new_name = entry
    new_path = OLD_BASE + new_suffix
    # 已是新路径时,alias 若恰为新包名则视为无别名
    current = (alias or "").strip()
    if current and current in olds:
        current = new_name
    if current == new_name:
        current = ""
    tail = ' "%s"' % new_path
    if keyword.strip() == "import" or keyword:
        # 单行 import / 合成源码 import:保留关键字
        if current:
            return indent + keyword + current + tail
        return indent + keyword + tail.lstrip()
    if current:
        return indent + current + tail
    return indent + '"%s"' % new_path


def collect_old_locals(match):
    alias = (match.group(3) or "").strip()
    path = match.group(4)
    entry = find_entry(path)
    if entry is None:
        return []
    _, _, olds, new_name = entry
    if alias:
        return [(alias, new_name)] if alias in olds or alias == new_name else []
    return [(old, new_name) for old in olds]


def process_file(file_path):
    with open(file_path, "r", encoding="utf-8") as handle:
        lines = handle.read().split("\n")
    changed = False
    prefix_map = {}
    for index, line in enumerate(lines):
        match = IMPORT_LINE.match(line)
        if match and OLD_BASE in match.group(4):
            entry = find_entry(match.group(4))
            if entry is not None:
                new_line = rewrite_import_line(match)
                if new_line is not None and new_line != line:
                    lines[index] = new_line
                    changed = True
                for old, new in collect_old_locals(match):
                    prefix_map[old] = new
                continue
        # 非行匹配但含旧路径(字段值/合成源码/注释):纯字符串替换
        new_line = line
        for suffix, new_suffix, _, _ in MAP:
            if OLD_BASE + suffix in new_line:
                new_line = new_line.replace(OLD_BASE + suffix, OLD_BASE + new_suffix)
        if new_line != line:
            lines[index] = new_line
            changed = True
    body = "\n".join(lines)
    for old, new in prefix_map.items():
        body = PREFIX_RE[old].sub(new + ".", body)
    if changed or body != "\n".join(lines):
        with open(file_path, "w", encoding="utf-8") as handle:
            handle.write(body)
        return True
    return False


def main():
    targets = []
    for arg in sys.argv[1:]:
        if os.path.isfile(arg):
            targets.append(arg)
        else:
            for root, _, files in os.walk(arg):
                if ".git" in root.split(os.sep):
                    continue
                targets.extend(os.path.join(root, name) for name in files if name.endswith(".go"))
    touched = 0
    for target in targets:
        if process_file(target):
            touched += 1
    print("rewritten: %d / %d files" % (touched, len(targets)))


if __name__ == "__main__":
    main()
```

- [ ] **Step 4: 对框架层跑脚本 + gofmt**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
python3 /tmp/gn_rewrite.py cool-next/
gofmt -w cool-next/
```

- [ ] **Step 5: 残留自检**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
command grep -rn "cool-next/core/entity\|cool-next/core/service\|cool-next/core/controller\|cool-next/core/config\|cool-next/core/exception\|cool-next/core/http\|cool-next/core/module\|cool-next/core/route\|cool-next/core/app\"\|cool-next/auth/bcrypt\|cool-next/auth/session\|cool-next/auth\"\|cool-next/db\b\|cool-next/crud\|cool-next/eps\|cool-next/grpc\"\|cool-next/outbox\|cool-next/seed\|cool-next/codegen" cool-next/ --include="*.go" | command grep -v "gnauth\|gnapp\|gnconfig\|gncontroller\|gnentity\|gnexception\|gnhttp\|gnmodule\|gnroute\|gnservice\|gncodegen\|gncrud\|gndb\|gneps\|gngrpc\|gnoutbox\|gnseed"
```

预期:无输出(旧路径零残留)。

```bash
command grep -rn "coreentity\.\|coreservice\.\|corecontroller\.\|coreroute\.\|coredb\.\|corerecycle\.\|dbschema\.\|dbtx\.\|outboxstore\.\|apphttp\.\|coolgrpc\.\|authbcrypt\.\|cooloutbox\." cool-next/ --include="*.go"
```

预期:无输出。

- [ ] **Step 6: 编译与框架层测试**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go build ./cool-next/... && go test ./cool-next/... -count=1
```

预期:build 成功。`gncodegen` 包测试**允许失败**(它断言旧生成输出,Task 2 修复);其余包测试应全绿。若出现"undefined: X"类错误,多为局部变量与包前缀同名被误替换(`config`/`db`/`app` 是高风险名),按编译错误逐个把误替换的标识符改回原局部变量名。

- [ ] **Step 7: 提交**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add -A
git commit -m "refactor(cool-next): 框架包统一 gn 前缀(目录与包名迁移)

学 GoFrame 的 g+领域词模式,cool-next 全部包改名 gn*,业务侧将零别名引用框架包。
本提交仅覆盖框架层;业务侧与生成器在后续提交同步。

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: codegen 生成器同步

**Files:**
- Modify: `cool-next/gncodegen/render.go:79-157`(imports.add 别名参数)
- Modify: `cool-next/gncodegen/imports.go:27-72`(删除 fixed 别名表)
- Modify: `cool-next/gncodegen/render.go:659-669`(writeImports 同名别名省略)
- Test: `go test ./cool-next/gncodegen/...`

**Interfaces:**
- Consumes: Task 1 的 gn* 包名与新路径常量值(脚本已替换常量字符串)
- Produces: 生成代码使用 gn* 包名且无同名冗余别名;Task 3 依赖 `go run ./cmd/cool generate` 输出可编译代码

- [ ] **Step 1: 更新 render.go 的 imports.add 调用别名(22 处)**

用 Edit 逐处替换(render.go,行号为 Task 1 后大致位置,以内容定位):

| 内容定位(旧) | 改为 |
|---|---|
| `imports.add(modulePackagePath, "module")` | `imports.add(modulePackagePath, "gnmodule")` |
| `imports.add(appPackagePath, "app")` | `imports.add(appPackagePath, "gnapp")` |
| `imports.add(appHTTPPackagePath, "apphttp")` | `imports.add(appHTTPPackagePath, "gnhttp")` |
| `imports.add(configPackagePath, "config")` | `imports.add(configPackagePath, "gnconfig")` |
| `imports.add(exceptionPackagePath, "exception")`(3 处) | `imports.add(exceptionPackagePath, "gnexception")` |
| `imports.add(grpcPackagePath, "coolgrpc")` | `imports.add(grpcPackagePath, "gngrpc")` |
| `imports.add(queryPackagePath, "crud")` | `imports.add(queryPackagePath, "gncrud")` |
| `imports.add(outboxPackagePath, "outbox")` | `imports.add(outboxPackagePath, "gnoutbox")` |
| `imports.add(seedPackagePath, "seed")` | `imports.add(seedPackagePath, "gnseed")` |
| `imports.add(entityPackagePath, "coreentity")`(3 处) | `imports.add(entityPackagePath, "gnentity")` |
| `imports.add(servicePackagePath, "coreservice")`(2 处) | `imports.add(servicePackagePath, "gnservice")` |
| `imports.add(databasePackagePath, "coredb")` | `imports.add(databasePackagePath, "gndb")` |
| `imports.add(recyclePackagePath, "corerecycle")` | `imports.add(recyclePackagePath, "gnrecycle")` |
| `imports.add(schemaPackagePath, "dbschema")` | `imports.add(schemaPackagePath, "gnschema")` |
| `imports.add(outboxStorePackagePath, "outboxstore")` | `imports.add(outboxStorePackagePath, "gnstore")` |
| `imports.add(controllerPackagePath, "corecontroller")` | `imports.add(controllerPackagePath, "gncontroller")` |
| `imports.add(epsPackagePath, "eps")` | `imports.add(epsPackagePath, "gneps")` |
| `imports.add(routePackagePath, "coreroute")` | `imports.add(routePackagePath, "gnroute")` |
| `imports.add(authPackagePath, "auth")` | `imports.add(authPackagePath, "gnauth")` |
| `imports.add(authBcryptPackagePath, "authbcrypt")` | `imports.add(authBcryptPackagePath, "gnbcrypt")` |

注意:`imports.add(route.handler.RequestPackagePath, "dto")` **不改**(dto 是对业务 DTO 包的生成别名策略,与框架无关)。

- [ ] **Step 2: 删除 imports.go 的 fixed 别名表**

Task 1 后 imports.go 的 `finalize()` 中 `fixed := []struct{...}{...}` 块(含 module/corecontroller/coreroute/coreentity/coreservice/coredb/corerecycle/gdb/g 条目)整体删除,gdb/g 的别名已由 `imports.add(path, "gdb"|"g")` 的 preferred 提供。删除后 `finalize()` 仅保留通用 preferred + 数字后缀逻辑。同步删除因此不再使用的 import(若有)。

- [ ] **Step 3: writeImports 同名别名省略**

`render.go` 的 `writeImports` 改为:别名与路径末段相同时不输出别名(消除 `config "…/gnconfig"` 式噪音):

```go
func writeImports(source *strings.Builder, imports *importManager) {
	paths := imports.pathsInOrder()
	if len(paths) == 0 {
		return
	}
	source.WriteString("import (\n")
	for _, path := range paths {
		name := imports.alias(path)
		if name != "" && name != path[strings.LastIndex(path, "/")+1:] {
			fmt.Fprintf(source, "\t%s %q\n", name, path)
		} else {
			fmt.Fprintf(source, "\t%q\n", path)
		}
	}
	source.WriteString(")\n\n")
}
```

- [ ] **Step 4: 生成器残留自检**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
command grep -rn "coreentity\|coreservice\|corecontroller\|coreroute\|coredb\|corerecycle\|dbschema\|outboxstore\|apphttp\|coolgrpc\|authbcrypt" cool-next/gncodegen/ --include="*.go"
```

预期:无输出。若有输出,判断语义:测试期望字符串里的旧别名说明 Task 1 脚本漏了该字符串形式,手工按映射总表替换。

- [ ] **Step 5: 跑 codegen 测试**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go test ./cool-next/gncodegen/... -count=1
```

预期:全绿。失败时看失败断言:多为 golden/期望字符串中仍含旧别名,按映射总表修订测试期望(生成逻辑语义未变,只是名字变了)。

- [ ] **Step 6: 提交**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add -A
git commit -m "refactor(gncodegen): 生成器别名表同步 gn 前缀,同名别名不再输出

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 业务侧与入口迁移 + 重生成

**Files:**
- Modify: `modules/**`、`cmd/**`、`main.go`、`manifest/**`、`test/**` 中 `.go` 文件
- Regenerate: `modules/modules_gen.go`

**Interfaces:**
- Consumes: Task 1 的 gn* 包、Task 2 的生成器
- Produces: 全仓库编译通过、`modules_gen.go` 由新生成器产出且与脚本改写结果一致

- [ ] **Step 1: 对业务侧跑脚本 + gofmt**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
python3 /tmp/gn_rewrite.py modules/ cmd/ main.go manifest/ test/
gofmt -w modules/ cmd/ main.go test/
```

- [ ] **Step 2: 全仓库编译**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go build ./...
```

预期:成功。失败处理同 Task 1 Step 6(局部变量误替换)。

- [ ] **Step 3: 重新生成 modules_gen.go 并验证幂等**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
go run ./cmd/cool generate
git diff --stat modules/modules_gen.go
go run ./cmd/cool generate
git diff modules/modules_gen.go | command grep -c "^[+-]" || true
```

预期:第一次 generate 后 diff 极小或为零(生成器输出 ≈ 脚本改写结果);第二次 generate 后 diff 为 0(幂等)。若第一次 diff 大,检查 diff 内容是否仍含旧别名——是则回 Task 2 补生成器遗漏。

- [ ] **Step 4: 静态检查链**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
make check
```

预期:check-mod/check-format/check-vet/check-architecture/test-unit/check-build 全绿。`check-architecture`(test/architecture)若失败,看断言内容:多为依赖边界断言里的旧包路径,按映射总表修订 `test/architecture/*.go` 后重跑。

- [ ] **Step 5: 集成测试(对比基线)**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
make test-integration
```

预期:与 memory 基线一致(outbox 拓扑 panic 为预存失败,非本次引入);不出现新的失败用例。

- [ ] **Step 6: 提交**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add -A
git commit -m "refactor(modules): 业务侧与入口同步 gn 包名,重生成模块注册表

业务实体/服务文件不再需要 coreentity/coreservice 等别名,直接引用 gnentity/gnservice。

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 文档同步

**Files:**
- Modify: `README.md`(目录树 + 正文)
- Modify: `docs/superpowers/specs/*.md`(44 个文件中约 30 个含旧路径)
- Create: `docs/superpowers/specs/2026-09-01-gn-package-prefix-rename-design.md`

**Interfaces:**
- Consumes: 映射总表
- Produces: 文档与新结构一致;后续开发者可从 spec 查到命名约定

- [ ] **Step 1: specs 与 README 批量替换确定性映射**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
sed -i '' \
  -e 's#cool-next/core/entity#cool-next/core/gnentity#g' \
  -e 's#cool-next/core/service#cool-next/core/gnservice#g' \
  -e 's#cool-next/core/controller#cool-next/core/gncontroller#g' \
  -e 's#cool-next/core/config#cool-next/core/gnconfig#g' \
  -e 's#cool-next/core/exception#cool-next/core/gnexception#g' \
  -e 's#cool-next/core/http#cool-next/core/gnhttp#g' \
  -e 's#cool-next/core/module#cool-next/core/gnmodule#g' \
  -e 's#cool-next/core/route#cool-next/core/gnroute#g' \
  -e 's#cool-next/core/app#cool-next/core/gnapp#g' \
  -e 's#cool-next/auth/bcrypt#cool-next/gnauth/gnbcrypt#g' \
  -e 's#cool-next/auth/session#cool-next/gnauth/gnsession#g' \
  -e 's#cool-next/auth#cool-next/gnauth#g' \
  -e 's#cool-next/db/driver#cool-next/gndb/gndriver#g' \
  -e 's#cool-next/db/recycle#cool-next/gndb/gnrecycle#g' \
  -e 's#cool-next/db/schema#cool-next/gndb/gnschema#g' \
  -e 's#cool-next/db/tx#cool-next/gndb/gntx#g' \
  -e 's#cool-next/db#cool-next/gndb#g' \
  -e 's#cool-next/crud#cool-next/gncrud#g' \
  -e 's#cool-next/codegen#cool-next/gncodegen#g' \
  -e 's#cool-next/eps#cool-next/gneps#g' \
  -e 's#cool-next/grpc#cool-next/gngrpc#g' \
  -e 's#cool-next/outbox/store#cool-next/gnoutbox/gnstore#g' \
  -e 's#cool-next/outbox#cool-next/gnoutbox#g' \
  -e 's#cool-next/seed#cool-next/gnseed#g' \
  README.md docs/superpowers/specs/*.md
```

再替换唯一性别名提及(文档散文中的 `coreentity.Descriptor` 等写法):

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
sed -i '' \
  -e 's/coreentity\./gnentity./g' -e 's/coreservice\./gnservice./g' \
  -e 's/corecontroller\./gncontroller./g' -e 's/coreroute\./gnroute./g' \
  -e 's/coredb\./gndb./g' -e 's/corerecycle\./gnrecycle./g' \
  -e 's/dbschema\./gnschema./g' -e 's/outboxstore\./gnstore./g' \
  -e 's/apphttp/gnhttp/g' -e 's/coolgrpc/gngrpc/g' -e 's/authbcrypt/gnbcrypt/g' \
  README.md docs/superpowers/specs/*.md
```

- [ ] **Step 2: README 目录树手工核对**

打开 README.md「完整目录结构」段,核对 24 个新目录名、`core/` 注释(11 子目录数量)、依赖方向图中的包引用。逐行与磁盘 `ls cool-next` 对照。

- [ ] **Step 3: 裸包名残留人工巡检**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
command grep -rn "package entity\b\|package service\b\|package controller\b\|core/entity\|core/service\|apphttp\|coredb\|coreentity" README.md docs/superpowers/specs/ | command grep -v plans | command grep -v gn
```

对残存行逐条判断:描述旧结构的叙述改为新名;业务模块语境(`modules/*/entity`)保留。

- [ ] **Step 4: 写设计文档**

创建 `docs/superpowers/specs/2026-09-01-gn-package-prefix-rename-design.md`,内容至少包含:背景(框架包与业务包同名导致调用方别名,modules_gen.go 出现 entity2/dto4 式编号别名)、决策(学 GoFrame g+领域词,前缀 gn = g 沿袭 gf 血统 + n 取自 cool-admin-go-next)、映射总表(从本计划复制)、约定(cool-next 下所有包 = gn + 领域词;业务模块分层包名保持领域词;框架引用零别名)、与 gf 的已知取舍(gndb/gnhttp 与 gdb/ghttp 一字之差,接受)。

- [ ] **Step 5: 提交**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
git add README.md docs/
git commit -m "docs: 同步 gn 包名重构,新增命名约定设计文档

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 全量验证与收尾

**Files:**
- 无代码改动;更新 memory

**Interfaces:**
- Consumes: Task 1-4 全部完成
- Produces: 验证结论、memory 记录

- [ ] **Step 1: 全量检查链**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
make check && make test-race
```

预期:全绿。

- [ ] **Step 2: 集成测试基线对比**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
make test-integration
```

预期:仅 memory 记录的预存 outbox 拓扑 panic;无新增失败。若有新增失败,回 Task 3 定位。

- [ ] **Step 3: 旧名终检**

```bash
cd /Users/n/数据/cool-admin/cool-admin-go-next
command grep -rn "coreentity\|coreservice\|corecontroller\|coreroute\|coredb\|corerecycle\|dbschema\|dbtx\|outboxstore\|apphttp\|coolgrpc\|authbcrypt" . --include="*.go" 2>/dev/null | command grep -v "^./docs/superpowers/plans/"
command grep -rn "cool-admin-go-next/cool-next/core/\|cool-admin-go-next/cool-next/auth\"\|cool-admin-go-next/cool-next/db\|cool-admin-go-next/cool-next/crud\|cool-admin-go-next/cool-next/eps\|cool-admin-go-next/cool-next/grpc\"\|cool-admin-go-next/cool-next/outbox\|cool-admin-go-next/cool-next/seed\|cool-admin-go-next/cool-next/codegen" . --include="*.go" 2>/dev/null | command grep -v gn
```

预期:均无输出(plans 历史文档除外)。

- [ ] **Step 4: 更新 memory**

1. 修订 `/Users/n/.claude/projects/-Users-n----cool-admin/memory/cool-next-docs-before-delete.md`:把"目录结构受文档冻结"修正为"文档是结构基线但可随合理重构同步修改(用户 2026-09-01 明确)"。
2. 新建 `cool-next-gn-package-prefix.md`(type: project):cool-next 框架包统一 gn 前缀(gnentity/gnservice/…),业务模块保持 entity/service 分层,框架引用零别名;约定出处 spec `2026-09-01-gn-package-prefix-rename-design.md`。
3. 更新 `MEMORY.md` 索引。

- [ ] **Step 5: 汇报**

向用户汇报:5 个 commit、make check/test-race 结果、集成测试与基线对比、生成幂等验证结果。

---

## Self-Review 记录

- **覆盖检查**:用户四点诉求(零别名/学 gf/文档可改/业务不动)分别由 Task 1-3、映射总表、Task 4、Global Constraints 覆盖。无缺口。
- **占位符扫描**:脚本全文、映射表、sed/Edit 内容均完整给出;Task 4 设计文档给出必含要素清单(因正文需执行时按最终映射誊写,要素已列全)。
- **一致性**:标识符前缀映射在 Task 1 脚本、Task 2 表格、Task 4 sed、Task 3 验证中一致;`core/http` 的 apphttp→gnhttp 特例在四处均已处理。
- **已知风险**(执行者注意):`config`/`db`/`app`/`entity` 等既是旧包名又是常见局部变量名,脚本按"文件实际导入"限定替换,仍可能有变量遮蔽误替换,由 `go build` 兜底人工修正;`gncodegen` golden 测试期望串若含旧别名由 Task 2 Step 5 修订。
