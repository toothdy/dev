package service

import (
	"testing"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
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
		{name: "默认排序", wantColumn: "id"},
		{name: "创建时间倒序", order: "createTime", sort: "desc", wantColumn: "createTime", wantDescending: true},
		{name: "实体字段正序", order: "email", sort: "asc", wantColumn: "email"},
		{name: "隐藏字段", order: entity.PasswordFieldName, sort: "asc", wantError: true},
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
