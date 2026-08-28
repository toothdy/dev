package service

import (
	"context"
	"reflect"
	"testing"
)

func TestDepartmentDomainQueriesAndLocks(t *testing.T) {
	fixture := newUserTestFixture(t)
	if _, err := fixture.runtime.DB().Exec(t.Context(), `
		ALTER TABLE base_sys_department ADD COLUMN userId INTEGER;
		INSERT INTO base_sys_department (id, name, userId) VALUES
			(6, '产品', 99),
			(7, '财务', 100);
		INSERT INTO base_sys_role_department (roleId, departmentId) VALUES (20, 5);
	`); err != nil {
		t.Fatal(err)
	}

	names, err := fixture.service.department.Names(t.Context(), []uint64{6, 5, 5, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, map[uint64]string{5: "研发", 6: "产品"}) {
		t.Fatalf("Names() = %#v", names)
	}

	visibleIDs, err := fixture.service.department.VisibleIDs(t.Context(), 99, []uint64{20, 20, 0})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(visibleIDs, []uint64{5, 6}) {
		t.Fatalf("VisibleIDs() = %#v", visibleIDs)
	}

	err = fixture.runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return fixture.service.department.LockDepartments(ctx, []uint64{6, 5, 5, 0})
	})
	if err != nil {
		t.Fatal(err)
	}
	err = fixture.runtime.Runner().Within(t.Context(), func(ctx context.Context) error {
		return fixture.service.department.LockDepartments(ctx, []uint64{999})
	})
	if err == nil {
		t.Fatal("LockDepartments() missing department error = nil")
	}
}
