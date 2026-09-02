package gnservice

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	"github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/tx"
)

type serviceDescriptorResolver struct {
	metadata gnentity.Metadata
}

type serviceMutationHook struct {
	afterErr  error
	beforeErr error
	events    []string
	resultIDs []uint64
}

type delegatingService struct {
	*Base[inputEntity, uint64]
}

func (service *delegatingService) Add(ctx context.Context, input AddInput[inputEntity]) (AddResult[uint64], error) {
	return service.Base.Add(ctx, input)
}

func (hook *serviceMutationHook) ModifyBefore(_ context.Context, _ *Mutation[inputEntity, uint64]) error {
	hook.events = append(hook.events, "before")

	return hook.beforeErr
}

func (hook *serviceMutationHook) ModifyAfter(_ context.Context, mutation *Mutation[inputEntity, uint64]) error {
	hook.events = append(hook.events, "after")
	hook.resultIDs = mutation.ResultIDs()

	return hook.afterErr
}

func (resolver serviceDescriptorResolver) Resolve(value any) (gnentity.Metadata, bool) {
	if reflect.TypeOf(value) != reflect.TypeFor[inputEntity]() {
		return nil, false
	}

	return resolver.metadata, resolver.metadata != nil
}

func TestBaseReadOperationsApplyActionPlan(t *testing.T) {
	descriptor := inputDescriptor(t)
	if field, exists := descriptor.Field("roleIds"); !exists || field.Persistent() {
		t.Fatalf("roleIds field = %#v/%v", field, exists)
	}
	runtime := newServiceRuntime(t, "read")
	createServiceFixture(t, runtime)
	base, err := NewBase[inputEntity, uint64](descriptor, runtime, newDisabledRecycleStore(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewQuery(nil, 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	infoCtx := serviceOperationContext(t, descriptor, crud.ActionInfo, crud.QueryOp{}, nil)
	record, err := base.Info(infoCtx, uint64(2))
	if err != nil {
		t.Fatal(err)
	}
	if id, exists := record.Get("id"); !exists || id != uint64(2) {
		t.Fatalf("info id = %#v, %t", id, exists)
	}
	if enabled, exists := record.Get("enabled"); !exists || enabled != false {
		t.Fatalf("info enabled = %#v, %t", enabled, exists)
	}

	request, err := crud.NewQueryRequest([]crud.RequestValue{crud.RequestField("enabled", false)})
	if err != nil {
		t.Fatal(err)
	}
	query, err = NewQuery(request, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	op := crud.QueryOp{
		FieldEq:    []crud.FieldEq{crud.Eq(crud.NewColumnRef("enabled"))},
		AddOrderBy: []crud.Order{crud.Desc(crud.NewColumnRef("id"))},
	}
	listCtx := serviceOperationContext(t, descriptor, crud.ActionList, op, request)
	list, err := base.List(listCtx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("list length = %d", len(list))
	}

	pageCtx := serviceOperationContext(t, descriptor, crud.ActionPage, op, request)
	page, err := base.Page(pageCtx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.List) != 1 || page.Pagination.Total != 2 || page.Pagination.Page != 1 || page.Pagination.Size != 1 {
		t.Fatalf("page = %#v", page)
	}
	if id, _ := page.List[0].Get("id"); id != uint64(2) {
		t.Fatalf("page first id = %#v", id)
	}
	model, err := base.Model(pageCtx)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID uint64 `orm:"id"`
	}
	pagination, err := base.EntityRenderPage(pageCtx, model, query, &rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID != 2 || pagination.Total != 2 {
		t.Fatalf("entity page = %#v, %#v", rows, pagination)
	}
}

func TestBaseRequiresMatchingOperationAndTransactionScope(t *testing.T) {
	descriptor := inputDescriptor(t)
	runtime := newServiceRuntime(t, "scope")
	createServiceFixture(t, runtime)
	base, err := NewBase[inputEntity, uint64](descriptor, runtime, newDisabledRecycleStore(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	if base.Descriptor() != descriptor {
		t.Fatal("Descriptor() changed descriptor identity")
	}
	if _, err = base.Model(t.Context()); err != nil {
		t.Fatal(err)
	}
	if base.GetOrmManager() != runtime.DB() {
		t.Fatal("GetOrmManager() changed database identity")
	}
	if _, err = base.Tx(t.Context()); err == nil {
		t.Fatal("Tx() without scope error = nil")
	}
	if _, err = base.Info(t.Context(), uint64(1)); err == nil {
		t.Fatal("Info() without operation error = nil")
	}

	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		if _, currentErr := base.Tx(ctx); currentErr != nil {
			return currentErr
		}
		_, currentErr := base.Model(ctx)
		return currentErr
	})
	if err != nil {
		t.Fatal(err)
	}
	other := newServiceRuntime(t, "other")
	err = other.Runner().Within(t.Context(), func(ctx context.Context) error {
		if _, currentErr := base.Model(ctx); currentErr == nil {
			t.Fatal("cross-group Model() error = nil")
		}
		if _, currentErr := base.Tx(ctx); currentErr == nil {
			t.Fatal("cross-group Tx() error = nil")
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestBaseWriteOperationsPreserveShapeAndFieldSafety(t *testing.T) {
	descriptor := inputDescriptor(t)
	runtime := newServiceRuntime(t, "write")
	createServiceFixture(t, runtime)
	base, err := NewBase[inputEntity, uint64](descriptor, runtime, newDisabledRecycleStore(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	clientOnly := newWriteMutable(t, descriptor, Value("id", uint64(99)), Value("note", "client"), Value("roleIds", []uint64{1, 2}))
	serverValue := newWriteMutable(t, descriptor, Value("id", uint64(100)), Value("note", "client"))
	if err = serverValue.Set("note", "server"); err != nil {
		t.Fatal(err)
	}
	addInput, err := NewAddArray[inputEntity, uint64](descriptor, []*Mutable[inputEntity]{clientOnly, serverValue})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		writeCtx := serviceWriteOperationContext(t, ctx, descriptor, crud.ActionAdd, nil, []string{"note"})
		result, currentErr := base.Add(writeCtx, addInput)
		if currentErr != nil {
			return currentErr
		}
		if !result.IsMany() || !reflect.DeepEqual(result.Many(), []uint64{3, 4}) {
			t.Fatalf("Add() result = %#v", result.Many())
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := runtime.DB().GetAll(t.Context(), "SELECT id, note FROM service_inputs WHERE id IN (3, 4) ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0]["note"].IsNil() || rows[1]["note"].String() != "server" {
		t.Fatalf("inserted rows = %#v", rows)
	}

	updateValue, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{
		Value("count", uint64(0)),
		Value("enabled", false),
		Null("note"),
	})
	if err != nil {
		t.Fatal(err)
	}
	updateItem, err := NewUpdateItem(descriptor, uint64(3), updateValue)
	if err != nil {
		t.Fatal(err)
	}
	updateInput, err := NewUpdateObject(descriptor, updateItem)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.Update(serviceWriteOperationContext(t, ctx, descriptor, crud.ActionUpdate, nil, nil), updateInput)
	})
	if err != nil {
		t.Fatal(err)
	}
	row, err := runtime.DB().GetOne(t.Context(), "SELECT count, enabled, note FROM service_inputs WHERE id = ?", 3)
	if err != nil {
		t.Fatal(err)
	}
	if row["count"].Uint64() != 0 || row["enabled"].Bool() || !row["note"].IsNil() {
		t.Fatalf("updated row = %#v", row)
	}

	hidden := newWriteMutable(t, descriptor, Value("note", "hidden"))
	hiddenInput, err := NewAddObject[inputEntity, uint64](descriptor, hidden)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		_, currentErr := base.Add(
			serviceWriteOperationContext(t, ctx, descriptor, crud.ActionAdd, []string{"note"}, nil),
			hiddenInput,
		)
		return currentErr
	})
	if err == nil {
		t.Fatal("Add(hidden) error = nil")
	}
	if err = hidden.Set("note", "server-hidden"); err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		_, currentErr := base.Add(
			serviceWriteOperationContext(t, ctx, descriptor, crud.ActionAdd, []string{"note"}, nil),
			hiddenInput,
		)
		return currentErr
	})
	if err != nil {
		t.Fatal(err)
	}

	readonly, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("count", uint64(9))})
	if err != nil {
		t.Fatal(err)
	}
	readonlyItem, err := NewUpdateItem(descriptor, uint64(3), readonly)
	if err != nil {
		t.Fatal(err)
	}
	readonlyInput, err := NewUpdateObject(descriptor, readonlyItem)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.Update(
			serviceWriteOperationContext(t, ctx, descriptor, crud.ActionUpdate, nil, []string{"count"}),
			readonlyInput,
		)
	})
	if err == nil {
		t.Fatal("Update(readonly) error = nil")
	}
	if err = readonly.Set("count", uint64(9)); err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.Update(
			serviceWriteOperationContext(t, ctx, descriptor, crud.ActionUpdate, nil, []string{"count"}),
			readonlyInput,
		)
	})
	if err != nil {
		t.Fatal(err)
	}

	transient, err := NewMutable[inputEntity, uint64](descriptor, []FieldValue{Value("roleIds", []uint64{2, 3})})
	if err != nil {
		t.Fatal(err)
	}
	transientItem, err := NewUpdateItem(descriptor, uint64(3), transient)
	if err != nil {
		t.Fatal(err)
	}
	transientInput, err := NewUpdateObject(descriptor, transientItem)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.Update(serviceWriteOperationContext(t, ctx, descriptor, crud.ActionUpdate, nil, nil), transientInput)
	})
	if err == nil {
		t.Fatal("Update(transient only) error = nil")
	}

	deleteInput, err := NewDeleteInput[inputEntity](descriptor, []uint64{3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.Delete(serviceWriteOperationContext(t, ctx, descriptor, crud.ActionDelete, nil, nil), deleteInput)
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := serviceRowCount(t, runtime); count != 2 {
		t.Fatalf("row count after delete = %d", count)
	}
	if _, err = base.Add(t.Context(), addInput); err == nil {
		t.Fatal("Add() without transaction error = nil")
	}
}

func TestExecuteMutationRunsBatchHooksOnceAndRollsBack(t *testing.T) {
	descriptor := inputDescriptor(t)
	runtime := newServiceRuntime(t, "hooks")
	createServiceFixture(t, runtime)
	base, err := NewBase[inputEntity, uint64](descriptor, runtime, newDisabledRecycleStore(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	service := &delegatingService{Base: base}
	dispatcher, err := crud.NewDispatcher(runtime.Runner())
	if err != nil {
		t.Fatal(err)
	}
	input, err := NewAddArray[inputEntity, uint64](descriptor, []*Mutable[inputEntity]{
		newWriteMutable(t, descriptor, Value("note", "third")),
		newWriteMutable(t, descriptor, Value("note", "fourth")),
	})
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := NewAddMutation[inputEntity, uint64](input)
	if err != nil {
		t.Fatal(err)
	}
	hook := &serviceMutationHook{}
	plan := serviceWritePlan(t, t.Context(), descriptor, crud.ActionAdd, nil, nil)
	var result AddResult[uint64]
	err = dispatcher.Dispatch(t.Context(), crud.ActionAdd, ActionModeDelegate, plan, func(ctx context.Context) error {
		var currentErr error
		result, currentErr = ExecuteMutation(ctx, mutation, hook, hook, func(callCtx context.Context) (AddResult[uint64], error) {
			return service.Add(callCtx, mutation.AddInput())
		})

		return currentErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Many(), []uint64{3, 4}) {
		t.Fatalf("ExecuteMutation() result = %#v", result.Many())
	}
	if !reflect.DeepEqual(hook.events, []string{"before", "after"}) || !reflect.DeepEqual(hook.resultIDs, []uint64{3, 4}) {
		t.Fatalf("hook = events:%#v ids:%#v", hook.events, hook.resultIDs)
	}
	modifyCalled := false
	updatePlan := serviceWritePlan(t, t.Context(), descriptor, crud.ActionUpdate, nil, nil)
	err = dispatcher.Dispatch(t.Context(), crud.ActionUpdate, ActionModeBase, updatePlan, func(ctx context.Context) error {
		_, currentErr := ExecuteMutation(ctx, mutation, hook, hook, func(context.Context) (AddResult[uint64], error) {
			modifyCalled = true
			return AddResult[uint64]{}, nil
		})
		return currentErr
	})
	if err == nil || modifyCalled {
		t.Fatalf("mismatched action = called:%v error:%v", modifyCalled, err)
	}

	want := errors.New("after failed")
	failedInput, err := NewAddObject[inputEntity, uint64](descriptor, newWriteMutable(t, descriptor, Value("note", "rollback")))
	if err != nil {
		t.Fatal(err)
	}
	failedMutation, err := NewAddMutation[inputEntity, uint64](failedInput)
	if err != nil {
		t.Fatal(err)
	}
	failedHook := &serviceMutationHook{afterErr: want}
	err = dispatcher.Dispatch(t.Context(), crud.ActionAdd, ActionModeBase, plan, func(ctx context.Context) error {
		_, currentErr := ExecuteMutation(ctx, failedMutation, failedHook, failedHook, func(callCtx context.Context) (AddResult[uint64], error) {
			return base.Add(callCtx, failedMutation.AddInput())
		})
		return currentErr
	})
	if !errors.Is(err, want) {
		t.Fatalf("ExecuteMutation() error = %v", err)
	}
	if count := serviceRowCount(t, runtime); count != 4 {
		t.Fatalf("row count after hook rollback = %d", count)
	}
	overrideMutation, err := NewAddMutation[inputEntity, uint64](failedInput)
	if err != nil {
		t.Fatal(err)
	}
	overrideHook := &serviceMutationHook{}
	err = dispatcher.Dispatch(t.Context(), crud.ActionAdd, ActionModeOverride, plan, func(ctx context.Context) error {
		_, currentErr := ExecuteMutation(ctx, overrideMutation, overrideHook, overrideHook, func(context.Context) (AddResult[uint64], error) {
			return AddResult[uint64]{one: 99}, nil
		})

		return currentErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(overrideHook.events, []string{"before", "after"}) || !reflect.DeepEqual(overrideHook.resultIDs, []uint64{99}) {
		t.Fatalf("override hook = events:%#v ids:%#v", overrideHook.events, overrideHook.resultIDs)
	}
	err = dispatcher.Dispatch(t.Context(), crud.ActionAdd, ActionModeOverride, plan, func(ctx context.Context) error {
		if _, _, exists := tx.Current(ctx); !exists {
			t.Fatal("override transaction is missing")
		}
		if operation, exists := crud.CurrentOperation(ctx); !exists || operation.Plan() != plan {
			t.Fatalf("override operation = %#v, %t", operation, exists)
		}
		_, currentErr := base.Add(ctx, failedInput)

		return currentErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if count := serviceRowCount(t, runtime); count != 5 {
		t.Fatalf("row count after override Base.Add = %d", count)
	}
	if _, err = ExecuteMutation(t.Context(), mutation, hook, hook, func(context.Context) (AddResult[uint64], error) {
		return AddResult[uint64]{}, nil
	}); err == nil {
		t.Fatal("ExecuteMutation() without transaction error = nil")
	}
}

func TestNativeSQLValidation(t *testing.T) {
	valid := []string{
		"SELECT id FROM service_inputs",
		"/* read */ WITH selected AS (SELECT id FROM service_inputs) SELECT id FROM selected; -- done",
		"SELECT '; DELETE FROM ignored' AS value",
		"SELECT $$; UPDATE ignored$$ AS value",
	}
	for _, query := range valid {
		if _, err := NativeSQL(query); err != nil {
			t.Fatalf("NativeSQL(%q) error = %v", query, err)
		}
	}

	invalid := []string{
		"",
		"UPDATE service_inputs SET enabled = 1",
		"SELECT id FROM service_inputs; SELECT id FROM service_inputs",
		"WITH removed AS (DELETE FROM service_inputs RETURNING id) SELECT id FROM removed",
		"SELECT id INTO copied FROM service_inputs",
		"SELECT 'unterminated",
		"SELECT 1 /* unterminated",
	}
	for _, query := range invalid {
		if _, err := NativeSQL(query); err == nil {
			t.Fatalf("NativeSQL(%q) error = nil", query)
		}
	}
}

func TestNativeQueryUsesParametersAndCurrentTransaction(t *testing.T) {
	descriptor := inputDescriptor(t)
	runtime := newServiceRuntime(t, "native")
	createServiceFixture(t, runtime)
	base, err := NewBase[inputEntity, uint64](descriptor, runtime, newDisabledRecycleStore(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	statement, err := NativeSQL(
		"SELECT id, note FROM service_inputs WHERE enabled = ? ORDER BY id",
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		ID   uint64 `orm:"id"`
		Note string `orm:"note"`
	}
	if err = base.NativeQuery(t.Context(), statement, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 1 || rows[1].ID != 2 {
		t.Fatalf("native rows = %#v", rows)
	}
	query, err := NewQuery(nil, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	var pageRows []struct {
		ID uint64 `orm:"id"`
	}
	pagination, err := base.SQLRenderPage(t.Context(), statement, query, &pageRows)
	if err != nil {
		t.Fatal(err)
	}
	if len(pageRows) != 1 || pageRows[0].ID != 2 || pagination != (Pagination{Page: 1, Size: 1, Total: 2}) {
		t.Fatalf("sql page = %#v, %#v", pageRows, pagination)
	}
	if err = base.NativeQuery(t.Context(), statement, nil); err == nil {
		t.Fatal("NativeQuery(nil) error = nil")
	}

	rollback := errors.New("rollback")
	err = runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		transaction, currentErr := base.Tx(ctx)
		if currentErr != nil {
			return currentErr
		}
		_, currentErr = transaction.Ctx(ctx).Exec(
			"INSERT INTO service_inputs (id, createTime, updateTime, count, enabled, at, data, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			3, "2026-08-10 00:00:00", "2026-08-10 00:00:00", 0, false, "2026-08-10 00:00:00", []byte{3}, "third",
		)
		if currentErr != nil {
			return currentErr
		}
		var count struct {
			Value int `orm:"value"`
		}
		countStatement, currentErr := NativeSQL("SELECT COUNT(*) AS value FROM service_inputs")
		if currentErr != nil {
			return currentErr
		}
		if currentErr = base.NativeQuery(ctx, countStatement, &count); currentErr != nil {
			return currentErr
		}
		if count.Value != 3 {
			t.Fatalf("transaction count = %d", count.Value)
		}

		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("Within() error = %v", err)
	}
	count, err := runtime.DB().GetCount(t.Context(), "SELECT COUNT(*) FROM service_inputs")
	if err != nil || count != 2 {
		t.Fatalf("row count after rollback = %d, %v", count, err)
	}

	other := newServiceRuntime(t, "native-other")
	err = other.Runner().Within(t.Context(), func(ctx context.Context) error {
		return base.NativeQuery(ctx, statement, &rows)
	})
	if err == nil {
		t.Fatal("cross-group NativeQuery() error = nil")
	}
}

func TestNewBaseRejectsInvalidDependencies(t *testing.T) {
	descriptor := inputDescriptor(t)
	if _, err := NewBase[inputEntity, uint64](descriptor, nil, nil); err == nil {
		t.Fatal("NewBase(nil runtime) error = nil")
	}
	var nilDescriptor gnentity.Descriptor[inputEntity, uint64]
	runtime := newServiceRuntime(t, "invalid")
	if _, err := NewBase[inputEntity, uint64](nilDescriptor, runtime, newDisabledRecycleStore(t, runtime)); err == nil {
		t.Fatal("NewBase(nil descriptor) error = nil")
	}
	if _, err := NewBase[inputEntity, uint64](descriptor, runtime, nil); err == nil {
		t.Fatal("NewBase(nil recycler) error = nil")
	}
}

func serviceOperationContext(
	t *testing.T,
	descriptor gnentity.Descriptor[inputEntity, uint64],
	action crud.Action,
	op crud.QueryOp,
	request *crud.QueryRequest,
) context.Context {
	t.Helper()
	plan, err := crud.CompilePlan(
		t.Context(),
		serviceDescriptorResolver{metadata: descriptor},
		crud.PlanInput{Action: action, Entity: inputEntity{}, Query: op},
		request,
	)
	if err != nil {
		t.Fatal(err)
	}

	return crud.WithOperation(t.Context(), plan)
}

func serviceWriteOperationContext(
	t *testing.T,
	ctx context.Context,
	descriptor gnentity.Descriptor[inputEntity, uint64],
	action crud.Action,
	hiddenFields []string,
	readonlyFields []string,
) context.Context {
	t.Helper()

	return crud.WithOperation(ctx, serviceWritePlan(t, ctx, descriptor, action, hiddenFields, readonlyFields))
}

func serviceWritePlan(
	t *testing.T,
	ctx context.Context,
	descriptor gnentity.Descriptor[inputEntity, uint64],
	action crud.Action,
	hiddenFields []string,
	readonlyFields []string,
) *crud.ActionPlan {
	t.Helper()
	plan, err := crud.CompilePlan(
		ctx,
		serviceDescriptorResolver{metadata: descriptor},
		crud.PlanInput{
			Action: action,
			Entity: inputEntity{},
			Fields: crud.FieldPolicyInput{
				HiddenFields:   serviceColumnRefs(hiddenFields),
				ReadonlyFields: serviceColumnRefs(readonlyFields),
			},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	return plan
}

func serviceColumnRefs(fields []string) []crud.ColumnRef {
	result := make([]crud.ColumnRef, len(fields))
	for index, field := range fields {
		result[index] = crud.NewColumnRef(field)
	}

	return result
}

func newWriteMutable(
	t *testing.T,
	descriptor gnentity.Descriptor[inputEntity, uint64],
	fields ...FieldValue,
) *Mutable[inputEntity] {
	t.Helper()
	fields = append([]FieldValue{
		Value("count", uint64(1)),
		Value("enabled", true),
		Value("at", time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)),
		Value("data", []byte{1}),
	}, fields...)
	value, err := NewMutable[inputEntity, uint64](descriptor, fields)
	if err != nil {
		t.Fatal(err)
	}

	return value
}

func serviceRowCount(t *testing.T, runtime *db.Runtime) int {
	t.Helper()
	count, err := runtime.DB().GetCount(t.Context(), "SELECT COUNT(*) FROM service_inputs")
	if err != nil {
		t.Fatal(err)
	}

	return count
}

func newServiceRuntime(t *testing.T, suffix string) *db.Runtime {
	t.Helper()
	group := "service_" + strings.ReplaceAll(t.Name()+"_"+suffix, "/", "_")
	runtime, err := db.New(t.Context(), db.Config{
		Group: group,
		Nodes: gdb.ConfigGroup{{
			Type: "sqlite",
			Link: fmt.Sprintf("sqlite::@file(%s)", filepath.Join(t.TempDir(), "service.sqlite")),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	return runtime
}

func newDisabledRecycleStore(t *testing.T, runtime *db.Runtime) *recycle.Store {
	t.Helper()
	store, err := recycle.New(runtime, crud.Config{})
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func createServiceFixture(t *testing.T, runtime *db.Runtime) {
	t.Helper()
	_, err := runtime.DB().Exec(t.Context(), `
		CREATE TABLE service_inputs (
			id INTEGER PRIMARY KEY,
			createTime DATETIME,
			updateTime DATETIME,
			count INTEGER NOT NULL,
			enabled INTEGER NOT NULL,
			at DATETIME NOT NULL,
			data BLOB NOT NULL,
			note TEXT
		)
	`)
	if err != nil {
		t.Fatal(err)
	}
	for _, values := range [][]any{
		{1, "2026-08-10 00:00:00", "2026-08-10 00:00:00", 0, false, "2026-08-10 00:00:00", []byte{1}, "first"},
		{2, "2026-08-10 00:00:00", "2026-08-10 00:00:00", 7, false, "2026-08-10 00:00:00", []byte{2}, "second"},
	} {
		if _, err = runtime.DB().Exec(
			t.Context(),
			"INSERT INTO service_inputs (id, createTime, updateTime, count, enabled, at, data, note) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
			values...,
		); err != nil {
			t.Fatal(err)
		}
	}
}
