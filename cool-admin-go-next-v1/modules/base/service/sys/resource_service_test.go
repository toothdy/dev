package sys

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/service"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestResourceServicesForwardTheirSpecificModelDefinitions(t *testing.T) {
	cases := []struct {
		name       string
		expected   entity.Definition
		newService func(entity.Definition) *service.Base
	}{
		{
			name:     "user",
			expected: baseModel.BaseSysUser(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestUserService(nil, definition).Base
			},
		},
		{
			name:     "role",
			expected: baseModel.BaseSysRole(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestRoleService(nil, definition).Base
			},
		},
		{
			name:     "menu",
			expected: baseModel.BaseSysMenu(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestMenuService(nil, definition).Base
			},
		},
		{
			name:     "department",
			expected: baseModel.BaseSysDepartment(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestDepartmentService(nil, definition).Base
			},
		},
		{
			name:     "param",
			expected: baseModel.BaseSysParam(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestParamService(nil, definition).Base
			},
		},
		{
			name:     "log",
			expected: baseModel.BaseSysLog(),
			newService: func(definition entity.Definition) *service.Base {
				return newTestLogService(nil, definition).Base
			},
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			service := item.newService(item.expected)
			if service == nil {
				t.Fatal("expected embedded base service")
			}
			if service.Model.Name != item.expected.Name {
				t.Fatalf("expected model name %q, got %q", item.expected.Name, service.Model.Name)
			}
			if service.Model.TableName != item.expected.TableName {
				t.Fatalf("expected table name %q, got %q", item.expected.TableName, service.Model.TableName)
			}
		})
	}
}
