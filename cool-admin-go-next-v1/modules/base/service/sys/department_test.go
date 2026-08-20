package sys

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/util/gvalid"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
)

func TestDepartmentUpdateMutationPreservesPresentedFields(t *testing.T) {
	row, fields, err := departmentUpdateMutation(map[string]interface{}{
		"name":     "  技术部  ",
		"parentId": nil,
		"orderNum": 0,
	})
	if err != nil {
		t.Fatalf("build department update mutation failed: %v", err)
	}
	wantFields := []string{"name", "parentId", "orderNum"}
	if !reflect.DeepEqual(fields, wantFields) {
		t.Fatalf("unexpected department update fields: %#v", fields)
	}
	if row.Name != "技术部" || row.ParentID != nil || row.OrderNum != 0 {
		t.Fatalf("unexpected department update row: %#v", row)
	}
	if row.UserID != nil || row.TenantID != nil || row.CreateTime != nil || row.UpdateTime != nil {
		t.Fatalf("missing department fields must remain unset: %#v", row)
	}
}

func TestDepartmentUpdateMutationRejectsNullRequiredFields(t *testing.T) {
	for _, field := range []string{"name", "orderNum"} {
		t.Run(field, func(t *testing.T) {
			if _, _, err := departmentUpdateMutation(map[string]interface{}{field: nil}); err == nil {
				t.Fatalf("expected null %s to be rejected", field)
			}
		})
	}
}

func TestDepartmentUpdateMutationBuildsScopedPartialSQL(t *testing.T) {
	db := newBaseTenantServiceTestDB(t)
	ctx := baseTenantServiceContext(t, 31)
	row, fields, err := departmentUpdateMutation(map[string]interface{}{"parentId": nil, "orderNum": 0})
	if err != nil {
		t.Fatalf("build department update mutation failed: %v", err)
	}
	row.UpdateTime = "2026-07-29 18:00:00"
	fields = append(fields, "updateTime")
	sqlText, err := gdb.ToSQL(ctx, func(ctx context.Context) error {
		dbModel, modelErr := tenant.ScopedModel(ctx, db, baseModel.BaseSysDepartment(), "")
		if modelErr != nil {
			return modelErr
		}
		_, modelErr = dbModel.Fields(fields).Where("id", 7).Data(row).Update()
		return modelErr
	})
	if err != nil {
		t.Fatalf("build department update SQL failed: %v", err)
	}
	setSQL := strings.SplitN(sqlText, " WHERE ", 2)[0]
	if !strings.Contains(strings.ToLower(setSQL), "`parentid`=null") || !strings.Contains(strings.ToLower(setSQL), "`ordernum`=0") || !strings.Contains(strings.ToLower(setSQL), "`updatetime`=") {
		t.Fatalf("department partial fields missing from SQL: %s", sqlText)
	}
	for _, column := range []string{"createTime", "tenantId", "userId", "name"} {
		if strings.Contains(setSQL, "`"+column+"`") {
			t.Fatalf("department partial SQL unexpectedly updates %s: %s", column, sqlText)
		}
	}
	if !strings.Contains(sqlText, "`tenantId` = 31") {
		t.Fatalf("department partial SQL lost tenant predicate: %s", sqlText)
	}
}

func TestDepartmentRequestsRejectInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		items   []dto.DepartmentOrderItem
		message string
	}{
		{"empty order", nil, ""},
		{"invalid order ID", []dto.DepartmentOrderItem{{ID: 0}}, "ID不能为空"},
		{"negative order", []dto.DepartmentOrderItem{{ID: 1, OrderNum: -1}}, "orderNum最小值需要为0"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := gvalid.New().Bail().Data(dto.DepartmentOrderReq{DepartmentOrders: item.items}).Run(context.Background()); err != nil {
				if item.message != "" && !strings.Contains(err.Error(), item.message) {
					t.Fatalf("expected %q, got %v", item.message, err)
				}
				return
			}
			if item.message != "" {
				t.Fatalf("expected %q, got no error", item.message)
			}
		})
	}
}

func TestDepartmentDeleteUserFlag(t *testing.T) {
	for _, value := range []interface{}{true, 1, "true", "on"} {
		if !booleanValue(value) {
			t.Fatalf("expected %#v to enable deleteUser", value)
		}
	}
	for _, value := range []interface{}{nil, false, 0, "false"} {
		if booleanValue(value) {
			t.Fatalf("expected %#v to disable deleteUser", value)
		}
	}
}

func TestDepartmentRelationPathsRejectMissingScope(t *testing.T) {
	service := newTestDepartmentService(nil, baseModel.BaseSysDepartment())
	if _, err := service.Delete(context.Background(), crud.DeleteRequest{IDs: []interface{}{1}}); err == nil {
		t.Fatal("department relation delete must reject missing scope")
	}
}