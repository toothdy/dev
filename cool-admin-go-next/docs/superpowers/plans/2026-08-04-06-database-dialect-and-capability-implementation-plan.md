# 06 Database Dialect And Capability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 MySQL 8.x、PostgreSQL 9.5+ 和 SQLite 3.24+ 提供确定性 DDL 编译与真实数据库能力探测

**Architecture:** `Dialect` 作为无连接状态的值对象，负责标识符、类型和 DDL 差异。`Probe` 使用 GoFrame `gdb.DB` 在随机内部表上验证版本、事务、条件写入和锁，并单独校验 MySQL InnoDB。

**Tech Stack:** Go 1.26、GoFrame v2.10.2 `gdb`、Go `testing`、Docker Compose 三数据库集成环境

---

## File Structure

- Create: `cool-next/db/driver/types.go` - 方言、版本、能力和 DDL 公开模型
- Create: `cool-next/db/driver/version.go` - 驱动识别、版本解析与基线校验
- Create: `cool-next/db/driver/quote.go` - 单段标识符校验与引用
- Create: `cool-next/db/driver/type_mapping.go` - Go 类型、precision 和 size 到 SQL 类型映射
- Create: `cool-next/db/driver/ddl.go` - 列约束、默认值、注释和索引编译
- Create: `cool-next/db/driver/probe.go` - 真实数据库能力探测
- Create: `cool-next/db/driver/*_test.go` - 与上述责任对应的单元测试
- Modify: `test/integration/database/harness.go` - 将原有重复 Smoke 逻辑替换为 06 公开契约
- Create: `test/integration/database/driver_test.go` - 三库 DDL 执行、能力报告与 InnoDB 反例

### Task 1: Public Model And Version Contract

**Files:**
- Create: `cool-next/db/driver/types.go`
- Create: `cool-next/db/driver/version.go`
- Test: `cool-next/db/driver/version_test.go`

- [ ] **Step 1: Write the failing version and kind tests**

```go
func TestValidateVersionBaselines(t *testing.T) {
    tests := []struct {
        kind Kind
        raw string
        want Version
    }{
        {MySQL, "8.4.1", Version{Major: 8, Minor: 4, Patch: 1}},
        {PostgreSQL, "9.5", Version{Major: 9, Minor: 5}},
        {SQLite, "3.24.0", Version{Major: 3, Minor: 24}},
    }
    for _, test := range tests {
        got, err := ValidateVersion(test.kind, test.raw)
        if err != nil || got != test.want {
            t.Fatalf("ValidateVersion(%q, %q) = %#v, %v", test.kind, test.raw, got, err)
        }
    }
}

func TestValidateVersionRejectsUnsupportedVersions(t *testing.T) {
    for _, test := range []struct{ kind Kind; raw string }{
        {MySQL, "5.7.44"}, {MySQL, "9.0.0"},
        {PostgreSQL, "9.4.26"}, {SQLite, "3.23.2"},
    } {
        if _, err := ValidateVersion(test.kind, test.raw); err == nil {
            t.Fatalf("ValidateVersion(%q, %q) should fail", test.kind, test.raw)
        }
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./cool-next/db/driver -run 'TestValidateVersion' -count=1`

Expected: FAIL because package/API does not exist

- [ ] **Step 3: Implement public values and strict version validation**

```go
type Kind string

const (
    MySQL Kind = "mysql"
    PostgreSQL Kind = "postgresql"
    SQLite Kind = "sqlite"
)

type Version struct { Major, Minor, Patch int }

func ValidateVersion(kind Kind, raw string) (Version, error) {
    version, err := ParseVersion(raw)
    if err != nil { return Version{}, err }
    if !supports(kind, version) {
        return Version{}, gerror.Newf("数据库版本不满足基线: %s %s", kind, version)
    }
    return version, nil
}
```

- [ ] **Step 4: Verify GREEN**

Run: `go test ./cool-next/db/driver -run 'TestValidateVersion|TestParseVersion' -count=1`

Expected: PASS

### Task 2: Identifier Quoting

**Files:**
- Create: `cool-next/db/driver/quote.go`
- Test: `cool-next/db/driver/quote_test.go`

- [ ] **Step 1: Write failing quoting tests**

```go
func TestDialectQuote(t *testing.T) {
    tests := []struct{ kind Kind; want string }{
        {MySQL, "`createTime`"},
        {PostgreSQL, `"createTime"`},
        {SQLite, `"createTime"`},
    }
    for _, test := range tests {
        dialect, _ := New(test.kind)
        got, err := dialect.Quote("createTime")
        if err != nil || got != test.want { t.Fatalf("Quote = %q, %v", got, err) }
    }
}

func TestDialectQuoteRejectsSQLFragments(t *testing.T) {
    dialect, _ := New(PostgreSQL)
    for _, value := range []string{"", "public.goods", `"goods"`, "goods;DROP TABLE users"} {
        if _, err := dialect.Quote(value); err == nil { t.Fatalf("Quote(%q) should fail", value) }
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./cool-next/db/driver -run 'TestDialectQuote' -count=1`

Expected: FAIL because `New` and `Quote` do not exist

- [ ] **Step 3: Implement immutable dialect and validated quoting**

```go
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Dialect struct { kind Kind }

func New(kind Kind) (Dialect, error) {
    if !kind.valid() { return Dialect{}, gerror.Newf("不支持的数据库类型: %s", kind) }
    return Dialect{kind: kind}, nil
}

func (d Dialect) Quote(identifier string) (string, error) {
    if !identifierPattern.MatchString(identifier) { return "", gerror.Newf("数据库标识符无效: %q", identifier) }
    if d.kind == MySQL { return "`" + identifier + "`", nil }
    return `"` + identifier + `"`, nil
}
```

- [ ] **Step 4: Verify GREEN**

Run: `go test ./cool-next/db/driver -run 'TestDialectQuote|TestNew' -count=1`

Expected: PASS

### Task 3: Go Type Mapping

**Files:**
- Create: `cool-next/db/driver/type_mapping.go`
- Test: `cool-next/db/driver/type_mapping_test.go`

- [ ] **Step 1: Write failing table-driven mapping tests**

```go
func TestDialectColumnType(t *testing.T) {
    descriptor := compileTypeFixture(t)
    tests := []struct{ kind Kind; field string; want string }{
        {MySQL, "uint64Value", "BIGINT UNSIGNED"},
        {PostgreSQL, "uint64Value", "NUMERIC(20,0)"},
        {SQLite, "uint64Value", "INTEGER"},
        {MySQL, "price", "DECIMAL(10,2)"},
        {PostgreSQL, "payload", "BYTEA"},
        {SQLite, "createdAt", "DATETIME"},
    }
    for _, test := range tests {
        dialect, _ := New(test.kind)
        field, _ := descriptor.Field(test.field)
        got, err := dialect.ColumnType(field)
        if err != nil || got != test.want { t.Fatalf("ColumnType = %q, %v", got, err) }
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./cool-next/db/driver -run 'TestDialectColumnType' -count=1`

Expected: FAIL because `ColumnType` does not exist

- [ ] **Step 3: Implement mapping by logical type and underlying Go kind**

```go
func (d Dialect) ColumnType(field entity.Field) (string, error) {
    if field == nil { return "", gerror.New("字段元数据不能为 nil") }
    typ := field.GoType()
    if typ.Kind() == reflect.Pointer { typ = typ.Elem() }
    switch field.LogicalType() {
    case entity.LogicalBool:
        return d.boolType(), nil
    case entity.LogicalInt, entity.LogicalUint:
        return d.integerType(typ.Kind(), field.LogicalType() == entity.LogicalUint)
    case entity.LogicalFloat:
        return d.floatType(typ.Kind(), field.Constraints())
    case entity.LogicalString:
        return d.stringType(field.Constraints())
    case entity.LogicalBytes:
        return d.bytesType(field.Constraints())
    case entity.LogicalTime:
        return d.timeType(), nil
    default:
        return "", gerror.Newf("不支持的逻辑类型: %s", field.LogicalType())
    }
}
```

- [ ] **Step 4: Verify GREEN and constraint boundaries**

Run: `go test ./cool-next/db/driver -run 'TestDialectColumnType|TestPrecisionLimits' -count=1`

Expected: PASS

### Task 4: Deterministic Table And Index DDL

**Files:**
- Create: `cool-next/db/driver/ddl.go`
- Test: `cool-next/db/driver/ddl_test.go`

- [ ] **Step 1: Write failing full-DDL golden tests**

```go
func TestDialectCompilePostgreSQL(t *testing.T) {
    metadata := compileDDLFixture(t)
    dialect, _ := New(PostgreSQL)
    ddl, err := dialect.Compile(metadata)
    if err != nil { t.Fatal(err) }
    wantCreate := `CREATE TABLE "driver_goods" ("id" BIGSERIAL NOT NULL PRIMARY KEY, "createTime" TIMESTAMP(6) WITHOUT TIME ZONE NOT NULL, "updateTime" TIMESTAMP(6) WITHOUT TIME ZONE NOT NULL, "displayName" VARCHAR(50) NOT NULL DEFAULT 'guest', "price" NUMERIC(10,2) NOT NULL DEFAULT 0)`
    if ddl.CreateTable != wantCreate { t.Fatalf("CreateTable:\n%s", ddl.CreateTable) }
    if !slices.Contains(ddl.Comments, `COMMENT ON COLUMN "driver_goods"."displayName" IS '显示名称'`) {
        t.Fatalf("comments = %#v", ddl.Comments)
    }
    if !slices.Contains(ddl.Indexes, `CREATE UNIQUE INDEX "idx_driver_goods_display_name" ON "driver_goods" ("displayName")`) {
        t.Fatalf("indexes = %#v", ddl.Indexes)
    }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./cool-next/db/driver -run 'TestDialectCompile' -count=1`

Expected: FAIL because `Compile` does not exist

- [ ] **Step 3: Implement column/default/comment/index compilation**

```go
func (d Dialect) Compile(metadata entity.Metadata) (DDL, error) {
    if metadata == nil { return DDL{}, gerror.New("实体元数据不能为 nil") }
    table, err := d.Quote(metadata.Table())
    if err != nil { return DDL{}, err }
    columns := make([]string, 0, len(metadata.Fields()))
    for _, field := range metadata.Fields() {
        column, columnComments, err := d.compileColumn(table, field)
        if err != nil { return DDL{}, err }
        columns = append(columns, column)
        comments = append(comments, columnComments...)
    }
    // 表注释、InnoDB 选项和索引由方言分支组装
    return ddl, nil
}
```

Default parsing must use `strconv.ParseBool`, `ParseInt`, `ParseUint`, `ParseFloat`; strings use doubled single quotes; bytes defaults fail; time only accepts `CURRENT_TIMESTAMP`.

- [ ] **Step 4: Verify GREEN for all dialects and invalid defaults**

Run: `go test ./cool-next/db/driver -run 'TestDialectCompile|TestCompileRejects' -count=1`

Expected: PASS

- [ ] **Step 5: Run complete package tests**

Run: `go test ./cool-next/db/driver -count=1`

Expected: PASS

### Task 5: Live Capability Probe

**Files:**
- Create: `cool-next/db/driver/probe.go`
- Test: `cool-next/db/driver/probe_test.go`

- [ ] **Step 1: Write failing SQLite probe test against a real temporary database**

```go
func TestProbeSQLite(t *testing.T) {
    database, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Link: "sqlite::@file(:memory:)"})
    if err != nil { t.Fatal(err) }
    report, err := Probe(context.Background(), database)
    if err != nil { t.Fatal(err) }
    if report.Dialect.Kind() != SQLite { t.Fatalf("kind = %s", report.Dialect.Kind()) }
    want := Capabilities{Transactions: true, ConditionalWrite: true}
    if report.Capabilities != want { t.Fatalf("capabilities = %#v", report.Capabilities) }
}
```

- [ ] **Step 2: Verify RED**

Run: `go test ./cool-next/db/driver -run 'TestProbeSQLite' -count=1`

Expected: FAIL because `Probe` does not exist

- [ ] **Step 3: Implement ordered probe with guaranteed cleanup**

```go
func Probe(ctx context.Context, database gdb.DB, transactionTables ...string) (report Report, err error) {
    dialect, err := dialectFromDatabase(database)
    if err != nil { return Report{}, err }
    version, err := readAndValidateVersion(ctx, database, dialect.kind)
    if err != nil { return Report{}, err }
    table, err := newProbeTableName()
    if err != nil { return Report{}, err }
    if err = createProbeTable(ctx, database, dialect, table); err != nil { return Report{}, err }
    defer func() { err = joinCleanupError(err, dropProbeTable(ctx, database, dialect, table)) }()
    if err = probeTransaction(ctx, database, dialect, table); err != nil { return Report{}, err }
    if err = probeConditionalWrite(ctx, database, dialect, table); err != nil { return Report{}, err }
    if dialect.kind != SQLite {
        if err = probeSkipLocked(ctx, database, dialect, table); err != nil { return Report{}, err }
    }
    if dialect.kind == MySQL {
        if err = validateInnoDB(ctx, database, transactionTables); err != nil { return Report{}, err }
    }
    return Report{Dialect: dialect, Version: version, Capabilities: dialect.capabilities()}, nil
}
```

- [ ] **Step 4: Verify GREEN and no residual probe tables**

Run: `go test ./cool-next/db/driver -run 'TestProbeSQLite|TestProbeCleansUp' -count=1`

Expected: PASS

- [ ] **Step 5: Run Race Test**

Run: `CGO_ENABLED=1 go test -race ./cool-next/db/driver -count=1`

Expected: PASS

### Task 6: Three-Database Integration Contract

**Files:**
- Modify: `test/integration/database/harness.go`
- Create: `test/integration/database/driver_test.go`

- [ ] **Step 1: Write failing shared Probe integration test**

```go
func TestDriverProbe(t *testing.T) {
    requireIntegration(t)
    config := loadIntegrationConfig(t)
    for _, testCase := range smokeCases(config) {
        t.Run(string(testCase.kind), func(t *testing.T) {
            database, err := gdb.New(testCase.node)
            if err != nil { t.Fatal(err) }
            report, err := driver.Probe(context.Background(), database)
            if err != nil { t.Fatal(err) }
            if !report.Capabilities.Transactions || !report.Capabilities.ConditionalWrite {
                t.Fatalf("capabilities = %#v", report.Capabilities)
            }
        })
    }
}
```

- [ ] **Step 2: Verify RED against integration environment**

Run: `test/integration/run.sh`

Expected: FAIL until the harness uses `driver.Probe` and DDL execution coverage exists

- [ ] **Step 3: Execute compiled fixture DDL and add MySQL InnoDB rejection**

```go
ddl, err := report.Dialect.Compile(metadata)
if err != nil { t.Fatal(err) }
for _, statement := range ddl.Statements() {
    if _, err := database.Exec(ctx, statement); err != nil { t.Fatal(err) }
}

// MySQL subtest creates a MyISAM table and requires Probe(..., table) to fail
```

- [ ] **Step 4: Verify GREEN on MySQL, PostgreSQL and SQLite**

Run: `test/integration/run.sh`

Expected: PASS for all three database sections

### Task 7: Repository Verification And Documentation Audit

**Files:**
- Modify only files found defective by verification

- [ ] **Step 1: Format production and test files**

Run: `gofmt -w cool-next/db/driver/*.go test/integration/database/*.go`

Expected: no output

- [ ] **Step 2: Audit comments against COMMENT_STYLE**

Run: `go doc ./cool-next/db/driver`

Expected: all exported symbols have concise Chinese documentation; constants and fields use trailing comments where applicable; no trailing periods

- [ ] **Step 3: Run package and Race tests fresh**

Run: `go test ./cool-next/db/driver -count=1`

Run: `CGO_ENABLED=1 go test -race ./cool-next/db/driver -count=1`

Expected: PASS

- [ ] **Step 4: Run static and repository checks**

Run: `go vet ./...`

Run: `make check`

Expected: PASS

- [ ] **Step 5: Run fresh integration suite**

Run: `test/integration/run.sh`

Expected: MySQL, PostgreSQL and SQLite all PASS with no residual `cool_probe_*` tables

- [ ] **Step 6: Review scope and worktree**

Run: `git status --short`

Expected: pre-existing module 05 changes remain intact; implementation changes are limited to module 06 code and integration coverage; ignored tests/docs are listed separately with `git status --short --ignored`
