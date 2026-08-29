package service

import (
	"reflect"
	"testing"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/modules/base/entity"
)

func TestRoleIDsKeepsSubmittedStates(t *testing.T) {
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
			roles, submitted := roleIDs(mutable)
			if submitted != test.submitted || !reflect.DeepEqual(roles, test.want) {
				t.Fatalf("roleIDs() = %#v, %t", roles, submitted)
			}
		})
	}
}
