package sys

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	"github.com/toothdy/cool-admin-go-next/modules/base/dto"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestFilterUserPasswordRemovesPasswordWithoutMutatingInput(t *testing.T) {
	user := map[string]interface{}{"id": int64(1), "username": "admin", "password": "secret"}
	filtered := FilterUserPassword(user)
	if _, ok := filtered["password"]; ok {
		t.Fatal("expected password removed from filtered user")
	}
	if filtered["username"] != "admin" {
		t.Fatalf("expected username preserved, got %#v", filtered)
	}
	if _, ok := user["password"]; !ok {
		t.Fatal("expected original user map unchanged")
	}
}

func TestUserRequestsRejectInvalidInput(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		message string
	}{
		{"empty user IDs", dto.MoveReq{DepartmentID: 1}.Validate(), "用户不能为空"},
		{"invalid department", dto.MoveReq{DepartmentID: 0, UserIDs: []int64{1}}.Validate(), "部门参数错误"},
		{"non-positive user ID", dto.MoveReq{DepartmentID: 1, UserIDs: []int64{0}}.Validate(), "用户参数错误"},
		{"duplicate user IDs", dto.MoveReq{DepartmentID: 1, UserIDs: []int64{1, 1}}.Validate(), "用户参数错误"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if item.err == nil || item.err.Error() != item.message {
				t.Fatalf("expected %q, got %v", item.message, item.err)
			}
		})
	}
	service := newTestUserService(nil, baseModel.BaseSysUser())
	if err := service.Move(context.Background(), dto.MoveReq{}); err == nil || err.Error() != "部门参数错误" {
		t.Fatalf("expected department validation error, got %v", err)
	}
}

func TestPersonUpdateRequestValuesUsesOnlyAllowedFields(t *testing.T) {
	values := dto.PersonUpdateRequest{
		NickName: "昵称",
		HeadImg:  "/old.png",
		Phone:    "13800000000",
		Email:    "n@example.com",
		Remark:   "备注",
	}.Values()
	if !reflect.DeepEqual(values, map[string]interface{}{
		"nickName": "昵称",
		"headImg":  "/old.png",
		"phone":    "13800000000",
		"email":    "n@example.com",
		"remark":   "备注",
	}) {
		t.Fatalf("unexpected update values: %#v", values)
	}
	for _, prohibited := range []string{"id", "username", "password", "passwordV", "status", "tenantId"} {
		if _, ok := values[prohibited]; ok {
			t.Fatalf("forbidden field %s leaked into update values", prohibited)
		}
	}
}

func TestPersonUpdateRequestValuesTrackJSONFieldPresence(t *testing.T) {
	var request dto.PersonUpdateRequest
	if err := json.Unmarshal([]byte(`{"nickName":"new"}`), &request); err != nil {
		t.Fatalf("decode partial request failed: %v", err)
	}
	if values := request.Values(); !reflect.DeepEqual(values, map[string]interface{}{"nickName": "new"}) {
		t.Fatalf("expected only nickName to update, got %#v", values)
	}

	if err := json.Unmarshal([]byte(`{"email":""}`), &request); err != nil {
		t.Fatalf("decode explicit empty request failed: %v", err)
	}
	if values := request.Values(); !reflect.DeepEqual(values, map[string]interface{}{"email": ""}) {
		t.Fatalf("expected explicit empty email update, got %#v", values)
	}

	if err := json.Unmarshal([]byte(`{}`), &request); err != nil {
		t.Fatalf("decode empty request failed: %v", err)
	}
	if values := request.Values(); len(values) != 0 {
		t.Fatalf("expected empty update values, got %#v", values)
	}
}

func TestPersonUpdateRequestPasswordChangeValidation(t *testing.T) {
	var request dto.PersonUpdateRequest
	if err := json.Unmarshal([]byte(`{"password":"new-secret"}`), &request); err != nil {
		t.Fatalf("decode password request failed: %v", err)
	}
	if _, _, _, err := request.PasswordChange(); err == nil || err.Error() != "原密码不能为空" {
		t.Fatalf("expected missing old password error, got %v", err)
	}
	if err := json.Unmarshal([]byte(`{"oldPassword":"old-secret","password":"new-secret"}`), &request); err != nil {
		t.Fatalf("decode password change failed: %v", err)
	}
	oldPassword, password, changed, err := request.PasswordChange()
	if err != nil || !changed || oldPassword != "old-secret" || password != "new-secret" {
		t.Fatalf("unexpected password change: old=%q new=%q changed=%v err=%v", oldPassword, password, changed, err)
	}
	if values := request.Values(); len(values) != 0 {
		t.Fatalf("password fields leaked into profile values: %#v", values)
	}
}

func TestApplyTenantMutationOverridesClientTenant(t *testing.T) {
	data := map[string]interface{}{"tenantId": int64(999), "name": "user"}
	tenantIdentity, err := security.NewTenantIdentity(7)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	ctx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: tenantIdentity})
	applyTenantMutation(ctx, data)
	if data["tenantId"] != int64(7) {
		t.Fatalf("expected context tenant 7, got %#v", data)
	}

	legacy := map[string]interface{}{"tenantId": int64(999)}
	applyTenantMutation(context.Background(), legacy)
	if _, ok := legacy["tenantId"]; ok {
		t.Fatalf("anonymous client tenant was not removed: %#v", legacy)
	}
}

func TestApplyTenantMutationHonorsDerivedTenantScope(t *testing.T) {
	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	derivedCtx, err := tenant.ForTenant(platformCtx, 17)
	if err != nil {
		t.Fatalf("derive tenant scope failed: %v", err)
	}
	data := map[string]interface{}{"tenantId": int64(999)}
	applyTenantMutation(derivedCtx, data)
	if data["tenantId"] != int64(17) {
		t.Fatalf("derived tenant scope was ignored: %#v", data)
	}
}

func TestPersonUpdateSkipsDatabaseUpdateForEmptyJSONObject(t *testing.T) {
	var request dto.PersonUpdateRequest
	if err := json.Unmarshal([]byte(`{}`), &request); err != nil {
		t.Fatalf("decode empty request failed: %v", err)
	}
	service := newTestUserService(nil, baseModel.BaseSysUser())
	if err := service.PersonUpdate(context.Background(), 1, request); err != nil {
		t.Fatalf("empty person update should succeed without database access: %v", err)
	}
}

func TestUserUpdateDataContainsOnlyPresentedFields(t *testing.T) {
	values := userUpdateData(map[string]interface{}{
		"id":           1,
		"nickName":     "管理员",
		"departmentId": nil,
		"status":       0,
		"roleIdList":   []interface{}{1},
	})
	expected := map[string]interface{}{
		"nickName":     "管理员",
		"departmentId": nil,
		"status":       0,
	}
	if !reflect.DeepEqual(values, expected) {
		t.Fatalf("unexpected partial user update: %#v", values)
	}
}

func TestOptionalUserRoleIDsDistinguishesMissingAndEmpty(t *testing.T) {
	roleIDs, present, err := optionalUserRoleIDs(map[string]interface{}{})
	if err != nil || present || roleIDs != nil {
		t.Fatalf("missing roleIdList should remain unchanged, ids=%#v present=%v err=%v", roleIDs, present, err)
	}
	roleIDs, present, err = optionalUserRoleIDs(map[string]interface{}{"roleIdList": []interface{}{}})
	if err != nil || !present || len(roleIDs) != 0 {
		t.Fatalf("empty roleIdList should clear roles, ids=%#v present=%v err=%v", roleIDs, present, err)
	}
}

func TestUserRelationPathsRejectMissingScope(t *testing.T) {
	service := newTestUserService(nil, baseModel.BaseSysUser())
	if _, err := service.Info(context.Background(), crud.InfoRequest{ID: 1}); err == nil {
		t.Fatal("user relation info must reject missing scope")
	}
}
