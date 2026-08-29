package service

import (
	"reflect"
	"testing"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

func TestUserPageOrder(t *testing.T) {
	descriptor, err := coreentity.Compile[entity.User, uint64](entity.UserSchema())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name           string
		order          string
		sort           string
		wantColumn     string
		wantDescending bool
		wantError      bool
	}{
		{name: "默认排序", wantColumn: "id", wantDescending: true},
		{name: "创建时间倒序", order: "createTime", sort: "desc", wantColumn: "createTime", wantDescending: true},
		{name: "实体字段正序", order: "email", sort: "asc", wantColumn: "email"},
		{name: "隐藏字段", order: "password", sort: "asc", wantError: true},
		{name: "临时字段", order: "roleIdList", sort: "asc", wantError: true},
		{name: "非法字段", order: "id DESC", sort: "asc", wantError: true},
		{name: "非法方向", order: "id", sort: "descending", wantError: true},
		{name: "缺少字段", sort: "desc", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			column, isDescending, err := userPageOrder(descriptor, test.order, test.sort)
			if (err != nil) != test.wantError {
				t.Fatalf("userPageOrder() error = %v", err)
			}
			if column != test.wantColumn || isDescending != test.wantDescending {
				t.Fatalf("userPageOrder() = %q, %t", column, isDescending)
			}
		})
	}
}

func TestUserRoleIDsKeepsSubmittedStates(t *testing.T) {
	descriptor, err := coreentity.Compile[entity.User, uint64](entity.UserSchema())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		fields    []coreservice.FieldValue
		want      []uint64
		submitted bool
	}{
		{name: "未提交"},
		{name: "null", fields: []coreservice.FieldValue{coreservice.Null("roleIdList")}, want: []uint64{}, submitted: true},
		{name: "空数组", fields: []coreservice.FieldValue{coreservice.Value("roleIdList", []uint64{})}, want: []uint64{}, submitted: true},
		{name: "规范化", fields: []coreservice.FieldValue{coreservice.Value("roleIdList", []uint64{2, 0, 1, 2})}, want: []uint64{1, 2}, submitted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, mutableErr := coreservice.NewMutable[entity.User, uint64](descriptor, test.fields)
			if mutableErr != nil {
				t.Fatal(mutableErr)
			}
			roleIDs, submitted, roleErr := userRoleIDs(mutable)
			if roleErr != nil {
				t.Fatal(roleErr)
			}
			if submitted != test.submitted || !reflect.DeepEqual(roleIDs, test.want) {
				t.Fatalf("userRoleIDs() = %#v, %t", roleIDs, submitted)
			}
		})
	}
}
