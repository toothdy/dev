package service

import (
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnentity"
	"github.com/toothdy/cool-admin-go-next/cool-next/core/gnservice"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

func TestRoleIDsKeepsSubmittedStates(t *testing.T) {
	descriptor, err := gnentity.Compile[entity.User, uint64](entity.UserSchema())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		fields    []gnservice.FieldValue
		want      []uint64
		submitted bool
	}{
		{name: "未提交"},
		{name: "null", fields: []gnservice.FieldValue{gnservice.Null("roleIdList")}, want: []uint64{}, submitted: true},
		{name: "空数组", fields: []gnservice.FieldValue{gnservice.Value("roleIdList", []uint64{})}, want: []uint64{}, submitted: true},
		{name: "规范化", fields: []gnservice.FieldValue{gnservice.Value("roleIdList", []uint64{2, 0, 1, 2})}, want: []uint64{1, 2}, submitted: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutable, mutableErr := gnservice.NewMutable[entity.User, uint64](descriptor, test.fields)
			if mutableErr != nil {
				t.Fatal(mutableErr)
			}
			roles, submitted := roleIDs(mutable)
			if submitted != test.submitted || !reflect.DeepEqual(roles, test.want) {
				t.Fatalf("roleIDs() = %#v, %t", roles, submitted)
			}
		})
	}
}
