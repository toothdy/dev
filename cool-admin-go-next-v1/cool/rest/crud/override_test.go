package crud

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

type pageOverrideService struct {
	called bool
}

/**
 * 重写分页查询
 * @param ctx 上下文
 * @param request 查询请求
 * @returns 分页结果
 */
func (s *pageOverrideService) Page(ctx context.Context, request QueryRequest) (interface{}, error) {
	s.called = true
	return PageResult{Pagination: Pagination{Page: request.Page, Size: request.Size, Total: 1}}, nil
}

type addCaptureService struct {
	data map[string]interface{}
}

type batchCaptureService struct {
	added   []map[string]interface{}
	updated []map[string]interface{}
}

func (s *batchCaptureService) Add(_ context.Context, request AddRequest) (interface{}, error) {
	s.added = append(s.added, cloneMap(request.Data))
	return map[string]interface{}{"id": int64(len(s.added))}, nil
}

func (s *batchCaptureService) Update(_ context.Context, request UpdateRequest) (interface{}, error) {
	s.updated = append(s.updated, cloneMap(request.Data))
	return nil, nil
}

type deleteCaptureService struct {
	request DeleteRequest
}

func (s *deleteCaptureService) Delete(_ context.Context, request DeleteRequest) (interface{}, error) {
	s.request = request
	return map[string]interface{}{}, nil
}

/**
 * 重写新增
 * @param ctx 上下文
 * @param request 新增请求
 * @returns 新增结果
 */
func (s *addCaptureService) Add(ctx context.Context, request AddRequest) (interface{}, error) {
	s.data = request.Data
	return map[string]interface{}{"id": int64(1)}, nil
}

/**
 * 测试业务 Service 重写分页
 * @param t 测试对象
 * @returns null
 */
func TestRuntimePageUsesServiceOverride(t *testing.T) {
	service := &pageOverrideService{}
	runtime := NewRuntime(nil, nil)
	resource := Resource{Spec: ResourceSpec{Name: "user", Service: service}}

	result, err := runtime.Page(context.Background(), resource, QueryRequest{Page: 2, Size: 10})
	if err != nil {
		t.Fatalf("page override failed: %v", err)
	}
	if !service.called {
		t.Fatal("expected page override to be called")
	}
	pageResult, ok := result.(PageResult)
	if !ok {
		t.Fatalf("expected PageResult, got %T", result)
	}
	if pageResult.Pagination.Page != 2 {
		t.Fatalf("expected page 2, got %d", pageResult.Pagination.Page)
	}
}

/**
 * 测试新增合并 InsertParam 后传给重写方法
 * @param t 测试对象
 * @returns null
 */
func TestRuntimeAddMergesInsertParamBeforeOverride(t *testing.T) {
	service := &addCaptureService{}
	runtime := NewRuntime(nil, nil)
	resource := Resource{
		Spec: ResourceSpec{
			Name:    "user",
			Service: service,
			InsertParam: func(ctx context.Context) map[string]interface{} {
				return map[string]interface{}{"userId": int64(7), "name": "default"}
			},
		},
	}

	_, err := runtime.Add(context.Background(), resource, map[string]interface{}{"name": "neo"})
	if err != nil {
		t.Fatalf("add override failed: %v", err)
	}
	if service.data["userId"] != int64(7) {
		t.Fatalf("expected merged userId 7, got %#v", service.data)
	}
	if service.data["name"] != "default" {
		t.Fatalf("expected default name to override input, got %#v", service.data)
	}
}

func TestRuntimeSupportsNodeBatchAddAndUpdate(t *testing.T) {
	service := &batchCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(nil, nil)
	ctx := tenant.WithoutTenant(context.Background())

	result, err := runtime.AddMany(ctx, resource, []map[string]interface{}{{"username": "a"}, {"username": "b"}})
	if err != nil {
		t.Fatalf("batch add failed: %v", err)
	}
	ids := result.(map[string]interface{})["id"].([]interface{})
	if len(ids) != 2 || ids[0] != int64(1) || ids[1] != int64(2) || len(service.added) != 2 {
		t.Fatalf("unexpected batch add result: %#v %#v", result, service.added)
	}
	if _, err = runtime.UpdateMany(ctx, resource, []map[string]interface{}{{"id": 1}, {"id": 2}}); err != nil {
		t.Fatalf("batch update failed: %v", err)
	}
	if len(service.updated) != 2 {
		t.Fatalf("expected two batch updates, got %#v", service.updated)
	}
}

/**
 * 测试单条自定义更新的只读字段归一化
 * @param t 测试对象
 * @returns null
 */
func TestRuntimeNormalizesCustomUpdateInput(t *testing.T) {
	service := &batchCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	resource.ReadonlyFields["password"] = true
	resource.ReadonlyFields["invalidReadonly"] = true
	runtime := NewRuntime(nil, nil)
	input := map[string]interface{}{
		"id":              1,
		"nickName":        "Alice",
		"password":        "secret",
		"tenantId":        int64(9),
		"createTime":      "2026-07-29 17:59:32",
		"updateTime":      "2026-07-29 17:59:32",
		"unknown":         "value",
		"invalidReadonly": "invalid",
	}
	wantInput := cloneMap(input)

	if _, err := runtime.Update(tenant.WithoutTenant(context.Background()), resource, input); err != nil {
		t.Fatalf("custom update failed: %v", err)
	}
	if len(service.updated) != 1 {
		t.Fatalf("expected one custom update, got %#v", service.updated)
	}
	wantData := map[string]interface{}{
		"id":              1,
		"nickName":        "Alice",
		"unknown":         "value",
		"invalidReadonly": "invalid",
	}
	if !reflect.DeepEqual(service.updated[0], wantData) {
		t.Fatalf("unexpected normalized update data: got %#v want %#v", service.updated[0], wantData)
	}
	if !reflect.DeepEqual(input, wantInput) {
		t.Fatalf("custom update mutated caller input: got %#v want %#v", input, wantInput)
	}
}

/**
 * 测试批量自定义更新的只读字段归一化
 * @param t 测试对象
 * @returns null
 */
func TestRuntimeNormalizesCustomBatchUpdateInputs(t *testing.T) {
	service := &batchCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	resource.ReadonlyFields["password"] = true
	runtime := NewRuntime(nil, nil)
	inputs := []map[string]interface{}{
		{
			"id":         1,
			"nickName":   "Alice",
			"password":   "secret-a",
			"tenantId":   int64(9),
			"createTime": "2026-07-29 17:59:32",
			"updateTime": "2026-07-29 17:59:32",
			"unknown":    "first",
		},
		{
			"id":         2,
			"nickName":   "Bob",
			"password":   "secret-b",
			"tenantId":   int64(10),
			"createTime": "2026-07-29 18:00:00",
			"updateTime": "2026-07-29 18:00:00",
			"unknown":    "second",
		},
	}
	wantInputs := []map[string]interface{}{cloneMap(inputs[0]), cloneMap(inputs[1])}

	if _, err := runtime.UpdateMany(tenant.WithoutTenant(context.Background()), resource, inputs); err != nil {
		t.Fatalf("custom batch update failed: %v", err)
	}
	wantData := []map[string]interface{}{
		{"id": 1, "nickName": "Alice", "unknown": "first"},
		{"id": 2, "nickName": "Bob", "unknown": "second"},
	}
	if !reflect.DeepEqual(service.updated, wantData) {
		t.Fatalf("unexpected normalized batch update data: got %#v want %#v", service.updated, wantData)
	}
	if !reflect.DeepEqual(inputs, wantInputs) {
		t.Fatalf("custom batch update mutated caller inputs: got %#v want %#v", inputs, wantInputs)
	}
}

func TestRuntimeRejectsOversizedBatchBeforeCallingService(t *testing.T) {
	service := &batchCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(nil, nil)
	inputs := make([]map[string]interface{}, MaxBatchSize+1)

	if _, err := runtime.AddMany(context.Background(), resource, inputs); err == nil {
		t.Fatal("expected oversized add batch rejection")
	}
	if _, err := runtime.UpdateMany(context.Background(), resource, inputs); err == nil {
		t.Fatal("expected oversized update batch rejection")
	}
	if len(service.added) != 0 || len(service.updated) != 0 {
		t.Fatalf("oversized batch reached service: %#v %#v", service.added, service.updated)
	}
}

func TestRuntimeDeleteWithDataPassesClonedInputToOverride(t *testing.T) {
	service := &deleteCaptureService{}
	runtime := NewRuntime(nil, nil)
	resource := Resource{Spec: ResourceSpec{Name: "department", Service: service}}
	input := map[string]interface{}{"ids": []interface{}{1}, "deleteUser": true}

	if _, err := runtime.DeleteWithData(context.Background(), resource, []interface{}{1}, input); err != nil {
		t.Fatalf("delete override failed: %v", err)
	}
	input["deleteUser"] = false
	if service.request.Data["deleteUser"] != true {
		t.Fatalf("expected cloned delete data, got %#v", service.request.Data)
	}
}

type hookCaptureService struct {
	before   interface{}
	after    interface{}
	afterErr error
}

/**
 * 记录默认 CRUD 修改前数据
 * @param ctx 上下文
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func (s *hookCaptureService) ModifyBefore(ctx context.Context, action string, data interface{}) error {
	s.before = cloneHookData(data)
	return nil
}

/**
 * 记录默认 CRUD 修改后数据
 * @param ctx 上下文
 * @param action 操作
 * @param data 数据
 * @returns error
 */
func (s *hookCaptureService) ModifyAfter(ctx context.Context, action string, data interface{}) error {
	s.after = cloneHookData(data)
	return s.afterErr
}

type hookStubDB struct {
	gdb.DB
	rolledBack bool
}

func (d *hookStubDB) Transaction(ctx context.Context, callback func(context.Context, gdb.TX) error) error {
	err := callback(ctx, &hookStubTX{})
	d.rolledBack = err != nil
	return err
}

type hookStubTX struct {
	gdb.TX
}

func (tx *hookStubTX) Ctx(context.Context) gdb.TX {
	return tx
}

func (tx *hookStubTX) GetAll(query string, args ...interface{}) (gdb.Result, error) {
	count := len(args)
	if strings.Contains(query, "`tenantId` = ?") {
		count--
	}
	rows := make(gdb.Result, count)
	for index := range rows {
		rows[index] = gdb.Record{}
	}
	return rows, nil
}

func (tx *hookStubTX) Exec(query string, args ...interface{}) (sql.Result, error) {
	affected := int64(1)
	if strings.HasPrefix(query, "DELETE ") {
		affected = int64(len(args))
	}
	return hookSQLResult{affected: affected}, nil
}

type hookSQLResult struct {
	affected int64
}

func (hookSQLResult) LastInsertId() (int64, error) {
	return 10, nil
}

func (r hookSQLResult) RowsAffected() (int64, error) {
	return r.affected, nil
}

func cloneHookMap(data interface{}) map[string]interface{} {
	values, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	cloned := map[string]interface{}{}
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneHookData(data interface{}) interface{} {
	if values, ok := data.(map[string]interface{}); ok {
		return cloneHookMap(values)
	}
	if values, ok := data.([]interface{}); ok {
		return append([]interface{}{}, values...)
	}
	return data
}

func TestRuntimeDefaultCRUDAfterHookReceivesBeforeData(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Runtime, Resource) error
		want interface{}
	}{
		{
			name: "add receives merged input",
			run: func(runtime *Runtime, resource Resource) error {
				_, err := runtime.Add(tenant.WithoutTenant(context.Background()), resource, map[string]interface{}{"username": "alice"})
				return err
			},
			want: map[string]interface{}{"username": "alice", "nickName": "Alice"},
		},
		{
			name: "delete receives ids",
			run: func(runtime *Runtime, resource Resource) error {
				_, err := runtime.Delete(tenant.WithoutTenant(context.Background()), resource, []interface{}{1, 2})
				return err
			},
			want: []interface{}{1, 2},
		},
		{
			name: "update receives input",
			run: func(runtime *Runtime, resource Resource) error {
				_, err := runtime.Update(tenant.WithoutTenant(context.Background()), resource, map[string]interface{}{"id": 1, "nickName": "Alice"})
				return err
			},
			want: map[string]interface{}{"id": 1, "nickName": "Alice"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &hookCaptureService{}
			resource := testUserResource(t)
			resource.Service = service
			resource.InsertParam = func(ctx context.Context) map[string]interface{} {
				return map[string]interface{}{"nickName": "Alice"}
			}
			runtime := NewRuntime(&hookStubDB{}, nil)

			if err := test.run(runtime, resource); err != nil {
				t.Fatalf("default CRUD failed: %v", err)
			}
			if !reflect.DeepEqual(service.before, test.want) {
				t.Fatalf("unexpected before data: got %#v want %#v", service.before, test.want)
			}
			if !reflect.DeepEqual(service.after, service.before) {
				t.Fatalf("expected after data to match before data: before %#v after %#v", service.before, service.after)
			}
		})
	}
}

/**
 * 测试默认更新 Hook 接收归一化后的输入
 * @param t 测试对象
 * @returns null
 */
func TestRuntimeDefaultUpdateHooksReceiveNormalizedInput(t *testing.T) {
	service := &hookCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	resource.ReadonlyFields["password"] = true
	runtime := NewRuntime(&hookStubDB{}, nil)
	input := map[string]interface{}{
		"id":         1,
		"nickName":   "Alice",
		"password":   "secret",
		"tenantId":   int64(9),
		"createTime": "2026-07-29 17:59:32",
		"updateTime": "2026-07-29 17:59:32",
	}
	wantInput := cloneMap(input)
	wantHookData := map[string]interface{}{"id": 1, "nickName": "Alice"}

	if _, err := runtime.Update(tenant.WithoutTenant(context.Background()), resource, input); err != nil {
		t.Fatalf("default update failed: %v", err)
	}
	if !reflect.DeepEqual(service.before, wantHookData) {
		t.Fatalf("unexpected before hook data: got %#v want %#v", service.before, wantHookData)
	}
	if !reflect.DeepEqual(service.after, wantHookData) {
		t.Fatalf("unexpected after hook data: got %#v want %#v", service.after, wantHookData)
	}
	if !reflect.DeepEqual(input, wantInput) {
		t.Fatalf("default update mutated caller input: got %#v want %#v", input, wantInput)
	}
}

func TestRuntimeDefaultUpdateAndDeleteReturnNilData(t *testing.T) {
	service := &hookCaptureService{}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(&hookStubDB{}, nil)
	ctx := tenant.WithoutTenant(context.Background())

	updated, err := runtime.Update(ctx, resource, map[string]interface{}{"id": 1, "nickName": "Alice"})
	if err != nil || updated != nil {
		t.Fatalf("expected nil update data, got %#v, %v", updated, err)
	}
	deleted, err := runtime.Delete(ctx, resource, []interface{}{1})
	if err != nil || deleted != nil {
		t.Fatalf("expected nil delete data, got %#v, %v", deleted, err)
	}
}

func TestRuntimeRollsBackWhenAfterHookFails(t *testing.T) {
	db := &hookStubDB{}
	service := &hookCaptureService{afterErr: errors.New("after hook failed")}
	resource := testUserResource(t)
	resource.Service = service
	runtime := NewRuntime(db, nil)

	if _, err := runtime.Add(tenant.WithoutTenant(context.Background()), resource, map[string]interface{}{"username": "alice"}); err == nil {
		t.Fatal("expected after hook failure")
	}
	if !db.rolledBack {
		t.Fatal("after hook failure must roll back the transaction")
	}
}
