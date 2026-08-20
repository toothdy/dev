package crud

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

type tenantRuntimeQuery struct {
	SQL  string
	Args []interface{}
}

type tenantRuntimeStubDB struct {
	gdb.DB
	queries          []tenantRuntimeQuery
	transactionCount int
	rollbackCount    int
	lockCounts       []int
	lockIndex        int
	execAffected     []int64
	execIndex        int
}

func (d *tenantRuntimeStubDB) Transaction(ctx context.Context, callback func(context.Context, gdb.TX) error) error {
	d.transactionCount++
	err := callback(ctx, &tenantRuntimeStubTX{db: d})
	if err != nil {
		d.rollbackCount++
	}
	return err
}

func (d *tenantRuntimeStubDB) GetOne(_ context.Context, query string, args ...interface{}) (gdb.Record, error) {
	d.capture(query, args)
	return gdb.Record{}, nil
}

func (d *tenantRuntimeStubDB) GetAll(_ context.Context, query string, args ...interface{}) (gdb.Result, error) {
	d.capture(query, args)
	return gdb.Result{}, nil
}

func (d *tenantRuntimeStubDB) GetCount(_ context.Context, query string, args ...interface{}) (int, error) {
	d.capture(query, args)
	return 0, nil
}

func (d *tenantRuntimeStubDB) capture(query string, args []interface{}) {
	d.queries = append(d.queries, tenantRuntimeQuery{
		SQL:  query,
		Args: append([]interface{}{}, args...),
	})
}

type tenantRuntimeStubTX struct {
	gdb.TX
	db *tenantRuntimeStubDB
}

func (tx *tenantRuntimeStubTX) Ctx(context.Context) gdb.TX {
	return tx
}

func (tx *tenantRuntimeStubTX) GetAll(query string, args ...interface{}) (gdb.Result, error) {
	tx.db.capture(query, args)
	count := mutationArgumentCount(query, args)
	if tx.db.lockIndex < len(tx.db.lockCounts) {
		count = tx.db.lockCounts[tx.db.lockIndex]
		tx.db.lockIndex++
	}
	rows := make(gdb.Result, count)
	for index := range rows {
		rows[index] = gdb.Record{}
	}
	return rows, nil
}

func (tx *tenantRuntimeStubTX) Exec(query string, args ...interface{}) (sql.Result, error) {
	tx.db.capture(query, args)
	affected := int64(1)
	if strings.HasPrefix(query, "DELETE ") {
		affected = int64(mutationArgumentCount(query, args))
	}
	if tx.db.execIndex < len(tx.db.execAffected) {
		affected = tx.db.execAffected[tx.db.execIndex]
		tx.db.execIndex++
	}
	return tenantRuntimeSQLResult{affected: affected}, nil
}

type tenantRuntimeSQLResult struct {
	affected int64
}

func (tenantRuntimeSQLResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (r tenantRuntimeSQLResult) RowsAffected() (int64, error) {
	return r.affected, nil
}

func mutationArgumentCount(query string, args []interface{}) int {
	count := len(args)
	if strings.Contains(query, "`tenantId` = ?") {
		count--
	}
	return count
}

func testTenantContext(t *testing.T, tenantID int64) context.Context {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
}

func TestRuntimeInfoResolvesTenantScopes(t *testing.T) {
	platformCtx := testPlatformContext()
	forTenantCtx, err := tenant.ForTenant(platformCtx, 21)
	if err != nil {
		t.Fatalf("derive tenant context failed: %v", err)
	}
	tests := []struct {
		name       string
		ctx        context.Context
		wantTenant interface{}
		wantError  bool
	}{
		{name: "tenant", ctx: testTenantContext(t, 12), wantTenant: int64(12)},
		{name: "platform", ctx: platformCtx},
		{name: "for tenant", ctx: forTenantCtx, wantTenant: int64(21)},
		{name: "missing", ctx: context.Background(), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &tenantRuntimeStubDB{}
			runtime := NewRuntime(db, nil)
			_, queryErr := runtime.Info(test.ctx, testUserResource(t), 5)
			if test.wantError {
				if queryErr == nil {
					t.Fatal("expected missing scope rejected")
				}
				if len(db.queries) != 0 {
					t.Fatalf("missing scope reached database: %#v", db.queries)
				}
				return
			}
			if queryErr != nil {
				t.Fatalf("runtime info failed: %v", queryErr)
			}
			if len(db.queries) != 1 {
				t.Fatalf("expected one info query, got %#v", db.queries)
			}
			query := db.queries[0]
			if test.wantTenant == nil {
				if strings.Contains(query.SQL, "`tenantId` = ?") || !reflect.DeepEqual(query.Args, []interface{}{5}) {
					t.Fatalf("platform scope must stay unfiltered: %s %#v", query.SQL, query.Args)
				}
				return
			}
			if !strings.Contains(query.SQL, "`tenantId` = ?") || !reflect.DeepEqual(query.Args, []interface{}{5, test.wantTenant}) {
				t.Fatalf("unexpected scoped info query: %s %#v", query.SQL, query.Args)
			}
		})
	}
}

func TestRuntimeTenantMutationsUseServerScope(t *testing.T) {
	db := &tenantRuntimeStubDB{}
	runtime := NewRuntime(db, nil)
	resource := testUserResource(t)
	ctx := testTenantContext(t, 7)
	input := map[string]interface{}{
		"username": "alice",
		"tenantId": int64(999),
	}

	if _, err := runtime.Add(ctx, resource, input); err != nil {
		t.Fatalf("runtime add failed: %v", err)
	}
	if _, err := runtime.Update(ctx, resource, map[string]interface{}{"id": 3, "nickName": "Alice"}); err != nil {
		t.Fatalf("runtime update failed: %v", err)
	}
	if _, err := runtime.Delete(ctx, resource, []interface{}{3}); err != nil {
		t.Fatalf("runtime delete failed: %v", err)
	}

	if len(db.queries) != 5 {
		t.Fatalf("expected insert and guarded mutation queries, got %#v", db.queries)
	}
	insertQuery := db.queries[0]
	if !strings.Contains(insertQuery.SQL, "`tenantId`") || len(insertQuery.Args) != 4 || insertQuery.Args[1] != int64(7) {
		t.Fatalf("runtime add did not override forged tenant: %s %#v", insertQuery.SQL, insertQuery.Args)
	}
	if input["tenantId"] != int64(999) {
		t.Fatalf("runtime add mutated caller input: %#v", input)
	}
	updateQuery := db.queries[2]
	if !strings.Contains(updateQuery.SQL, "WHERE `id` = ? AND `tenantId` = ?") || !reflect.DeepEqual(updateQuery.Args, []interface{}{"Alice", 3, int64(7)}) {
		t.Fatalf("unexpected runtime update query: %s %#v", updateQuery.SQL, updateQuery.Args)
	}
	deleteQuery := db.queries[4]
	if !strings.Contains(deleteQuery.SQL, "WHERE `id` IN (?) AND `tenantId` = ?") || !reflect.DeepEqual(deleteQuery.Args, []interface{}{3, int64(7)}) {
		t.Fatalf("unexpected runtime delete query: %s %#v", deleteQuery.SQL, deleteQuery.Args)
	}
	for _, query := range db.queries {
		if strings.Contains(query.SQL, "999") || strings.Contains(query.SQL, "tenantId` = 7") {
			t.Fatalf("tenant value must stay parameterized: %s", query.SQL)
		}
	}
}

func TestRuntimeTenantReadsUseParameterizedPredicates(t *testing.T) {
	db := &tenantRuntimeStubDB{}
	runtime := NewRuntime(db, nil)
	resource := testUserResource(t)
	ctx := testTenantContext(t, 8)

	if _, err := runtime.List(ctx, resource, QueryRequest{FieldEq: map[string]interface{}{"status": 1}}); err != nil {
		t.Fatalf("runtime list failed: %v", err)
	}
	if _, err := runtime.Page(ctx, resource, QueryRequest{Page: 1, Size: 10}); err != nil {
		t.Fatalf("runtime page failed: %v", err)
	}

	if len(db.queries) != 3 {
		t.Fatalf("expected list, count and page queries, got %#v", db.queries)
	}
	wants := [][]interface{}{
		{int64(8), 1, MaxListSize},
		{int64(8)},
		{int64(8), 10, 0},
	}
	for index, query := range db.queries {
		if !strings.Contains(query.SQL, "`tenantId` = ?") || !reflect.DeepEqual(query.Args, wants[index]) {
			t.Fatalf("unexpected scoped read query %d: %s %#v", index, query.SQL, query.Args)
		}
		if strings.Contains(query.SQL, "tenantId` = 8") {
			t.Fatalf("tenant value must stay parameterized: %s", query.SQL)
		}
	}
}

func TestRuntimeRejectsMissingScopeBeforeDatabaseAccess(t *testing.T) {
	resource := testUserResource(t)
	tests := []struct {
		name string
		run  func(*Runtime) error
	}{
		{name: "add", run: func(runtime *Runtime) error {
			_, err := runtime.Add(context.Background(), resource, map[string]interface{}{"username": "alice"})
			return err
		}},
		{name: "info", run: func(runtime *Runtime) error {
			_, err := runtime.Info(context.Background(), resource, 1)
			return err
		}},
		{name: "list", run: func(runtime *Runtime) error {
			_, err := runtime.List(context.Background(), resource, QueryRequest{})
			return err
		}},
		{name: "page", run: func(runtime *Runtime) error {
			_, err := runtime.Page(context.Background(), resource, QueryRequest{})
			return err
		}},
		{name: "update", run: func(runtime *Runtime) error {
			_, err := runtime.Update(context.Background(), resource, map[string]interface{}{"id": 1, "nickName": "Alice"})
			return err
		}},
		{name: "delete", run: func(runtime *Runtime) error {
			_, err := runtime.Delete(context.Background(), resource, []interface{}{1})
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &tenantRuntimeStubDB{}
			if err := test.run(NewRuntime(db, nil)); err == nil {
				t.Fatal("expected missing scope rejected")
			}
			if len(db.queries) != 0 {
				t.Fatalf("missing scope reached database: %#v", db.queries)
			}
		})
	}
}

type tenantRuntimeOverrideService struct {
	addCalls   int
	addData    map[string]interface{}
	updateData map[string]interface{}
	scope      tenant.Scope
}

func (s *tenantRuntimeOverrideService) Add(ctx context.Context, request AddRequest) (interface{}, error) {
	s.addCalls++
	s.addData = cloneMap(request.Data)
	s.scope = tenant.Resolve(ctx)
	return map[string]interface{}{"id": int64(s.addCalls)}, nil
}

func (s *tenantRuntimeOverrideService) Update(ctx context.Context, request UpdateRequest) (interface{}, error) {
	s.updateData = cloneMap(request.Data)
	s.scope = tenant.Resolve(ctx)
	return nil, nil
}

func TestRuntimeValidatesScopeBeforeCustomHandler(t *testing.T) {
	service := &tenantRuntimeOverrideService{}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(nil, nil)

	if _, err := runtime.Add(context.Background(), resource, map[string]interface{}{"username": "alice"}); err == nil {
		t.Fatal("expected missing tenant scope rejected")
	}
	if service.addCalls != 0 {
		t.Fatalf("missing scope reached custom handler: %d", service.addCalls)
	}

	ctx := testTenantContext(t, 13)
	addInput := map[string]interface{}{"username": "alice", "tenantId": int64(999)}
	if _, err := runtime.Add(ctx, resource, addInput); err != nil {
		t.Fatalf("custom handler rejected valid tenant scope: %v", err)
	}
	if tenantID, ok := service.scope.TenantID(); !ok || tenantID != 13 {
		t.Fatalf("custom handler received unexpected scope: %#v", service.scope)
	}
	if _, ok := service.addData["tenantId"]; ok || addInput["tenantId"] != int64(999) {
		t.Fatalf("custom add handler received untrusted tenant data: handler=%#v input=%#v", service.addData, addInput)
	}

	updateInput := map[string]interface{}{"id": 1, "nickName": "Alice", "tenantId": int64(999)}
	if _, err := runtime.Update(ctx, resource, updateInput); err != nil {
		t.Fatalf("custom update handler rejected valid tenant scope: %v", err)
	}
	if _, ok := service.updateData["tenantId"]; ok || updateInput["tenantId"] != int64(999) {
		t.Fatalf("custom update handler received untrusted tenant data: handler=%#v input=%#v", service.updateData, updateInput)
	}
}

func TestRuntimeTenantBatchesReuseScopeAndTransaction(t *testing.T) {
	db := &tenantRuntimeStubDB{}
	runtime := NewRuntime(db, nil)
	resource := testUserResource(t)
	ctx := testTenantContext(t, 17)

	_, err := runtime.AddMany(ctx, resource, []map[string]interface{}{
		{"username": "alice", "tenantId": int64(91)},
		{"username": "bob", "tenantId": int64(92)},
	})
	if err != nil {
		t.Fatalf("tenant batch add failed: %v", err)
	}
	if db.transactionCount != 1 || len(db.queries) != 2 {
		t.Fatalf("batch add must use one transaction: transactions=%d queries=%#v", db.transactionCount, db.queries)
	}
	for _, query := range db.queries {
		if !strings.Contains(query.SQL, "`tenantId`") || !containsTenantRuntimeArg(query.Args, int64(17)) || containsTenantRuntimeArg(query.Args, int64(91)) || containsTenantRuntimeArg(query.Args, int64(92)) {
			t.Fatalf("batch add did not enforce tenant scope: %s %#v", query.SQL, query.Args)
		}
	}

	db.transactionCount = 0
	db.queries = nil
	_, err = runtime.UpdateMany(ctx, resource, []map[string]interface{}{
		{"id": 1, "nickName": "Alice", "tenantId": int64(91)},
		{"id": 2, "nickName": "Bob", "tenantId": int64(92)},
	})
	if err != nil {
		t.Fatalf("tenant batch update failed: %v", err)
	}
	if db.transactionCount != 1 || len(db.queries) != 3 {
		t.Fatalf("batch update must use one transaction: transactions=%d queries=%#v", db.transactionCount, db.queries)
	}
	for _, query := range db.queries {
		if !strings.Contains(query.SQL, "AND `tenantId` = ?") || !containsTenantRuntimeArg(query.Args, int64(17)) || containsTenantRuntimeArg(query.Args, int64(91)) || containsTenantRuntimeArg(query.Args, int64(92)) {
			t.Fatalf("batch update did not enforce tenant scope: %s %#v", query.SQL, query.Args)
		}
	}
}

func TestRuntimeModifyHooksReceiveSanitizedTenantData(t *testing.T) {
	service := &hookCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(&hookStubDB{}, nil)
	input := map[string]interface{}{"username": "alice", "tenantId": int64(999)}

	if _, err := runtime.Add(testTenantContext(t, 23), resource, input); err != nil {
		t.Fatalf("default tenant add failed: %v", err)
	}
	before, ok := service.before.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected before hook data: %#v", service.before)
	}
	if _, exists := before["tenantId"]; exists || input["tenantId"] != int64(999) {
		t.Fatalf("modify hook received untrusted tenant data: hook=%#v input=%#v", before, input)
	}
}

func containsTenantRuntimeArg(args []interface{}, expected interface{}) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func TestRuntimeRejectsUnavailableMutationBeforeHooks(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Runtime, Resource) error
	}{
		{
			name: "update",
			run: func(runtime *Runtime, resource Resource) error {
				_, err := runtime.Update(testTenantContext(t, 31), resource, map[string]interface{}{"id": 9, "nickName": "Alice"})
				return err
			},
		},
		{
			name: "delete",
			run: func(runtime *Runtime, resource Resource) error {
				_, err := runtime.Delete(testTenantContext(t, 31), resource, []interface{}{9})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := &tenantRuntimeStubDB{lockCounts: []int{0}}
			service := &hookCaptureService{}
			resource := testUserResource(t)
			resource.Service = service
			if err := test.run(NewRuntime(db, nil), resource); err == nil {
				t.Fatal("expected unavailable mutation rejected")
			}
			if service.before != nil || service.after != nil {
				t.Fatalf("unavailable mutation reached hooks: before=%#v after=%#v", service.before, service.after)
			}
			if db.rollbackCount != 1 {
				t.Fatalf("unavailable mutation did not roll back: %#v", db)
			}
		})
	}
}

func TestRuntimeUpdateManyRollsBackWhenAnyRowIsUnavailable(t *testing.T) {
	db := &tenantRuntimeStubDB{lockCounts: []int{1}}
	runtime := NewRuntime(db, nil)
	resource := testUserResource(t)

	_, err := runtime.UpdateMany(testTenantContext(t, 41), resource, []map[string]interface{}{
		{"id": 1, "nickName": "Alice"},
		{"id": 2, "nickName": "Bob"},
	})
	if err == nil {
		t.Fatal("expected batch update rejected")
	}
	if db.transactionCount != 1 || db.rollbackCount != 1 {
		t.Fatalf("batch update must roll back one transaction: %#v", db)
	}
	if len(db.queries) != 1 || !reflect.DeepEqual(db.queries[0].Args, []interface{}{1, 2, int64(41)}) {
		t.Fatalf("batch update must validate all rows before writing: %#v", db.queries)
	}
}

func TestRuntimeDeleteNormalizesIDsAndRejectsPartialVisibility(t *testing.T) {
	ctx := testTenantContext(t, 51)
	resource := testUserResource(t)

	partialDB := &tenantRuntimeStubDB{lockCounts: []int{1}}
	if _, err := NewRuntime(partialDB, nil).Delete(ctx, resource, []interface{}{1, 1, 2}); err == nil {
		t.Fatal("expected partially visible delete rejected")
	}
	if partialDB.rollbackCount != 1 || len(partialDB.queries) != 1 {
		t.Fatalf("partial delete must stop after lock query: %#v", partialDB)
	}
	if !reflect.DeepEqual(partialDB.queries[0].Args, []interface{}{1, 2, int64(51)}) {
		t.Fatalf("delete IDs were not normalized: %#v", partialDB.queries[0].Args)
	}

	successDB := &tenantRuntimeStubDB{lockCounts: []int{2}}
	if _, err := NewRuntime(successDB, nil).Delete(ctx, resource, []interface{}{1, 1, 2}); err != nil {
		t.Fatalf("normalized delete failed: %v", err)
	}
	if len(successDB.queries) != 2 || !reflect.DeepEqual(successDB.queries[1].Args, []interface{}{1, 2, int64(51)}) {
		t.Fatalf("delete did not reuse normalized IDs: %#v", successDB.queries)
	}
}

func TestRuntimeRejectsInconsistentAffectedRowsBeforeAfterHook(t *testing.T) {
	db := &tenantRuntimeStubDB{
		lockCounts:   []int{2},
		execAffected: []int64{1},
	}
	service := &hookCaptureService{}
	resource := testUserResource(t)
	resource.Service = service

	if _, err := NewRuntime(db, nil).Delete(testPlatformContext(), resource, []interface{}{1, 2}); err == nil {
		t.Fatal("expected inconsistent affected rows rejected")
	}
	if service.after != nil {
		t.Fatalf("inconsistent mutation reached after hook: %#v", service.after)
	}
	if db.rollbackCount != 1 {
		t.Fatalf("inconsistent mutation did not roll back: %#v", db)
	}
}

type tenantRuntimePrimaryMutationHook struct {
	afterCalled bool
}

func (s *tenantRuntimePrimaryMutationHook) ModifyBefore(_ context.Context, action string, data interface{}) error {
	if action == Update {
		data.(map[string]interface{})["id"] = 99
	}
	return nil
}

func (s *tenantRuntimePrimaryMutationHook) ModifyAfter(context.Context, string, interface{}) error {
	s.afterCalled = true
	return nil
}

func TestRuntimeRejectsPrimaryKeyMutationAfterRowLock(t *testing.T) {
	db := &tenantRuntimeStubDB{lockCounts: []int{1}}
	service := &tenantRuntimePrimaryMutationHook{}
	resource := testUserResource(t)
	resource.Service = service

	_, err := NewRuntime(db, nil).Update(
		testTenantContext(t, 61),
		resource,
		map[string]interface{}{"id": 1, "nickName": "Alice"},
	)
	if err == nil {
		t.Fatal("expected primary key mutation rejected")
	}
	if service.afterCalled || len(db.queries) != 1 || db.rollbackCount != 1 {
		t.Fatalf("primary key mutation crossed write boundary: service=%#v db=%#v", service, db)
	}
}
