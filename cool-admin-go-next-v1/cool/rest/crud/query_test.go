package crud

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func testUserResource(t *testing.T) Resource {
	t.Helper()
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:          "user",
			Prefix:        "/admin/base/sys/user",
			Model:         baseModel.BaseSysUser(),
			API:          []string{Add, Delete, Update, Info, List, Page},
			KeywordFields: []string{"name", "username", "nickName"},
			EqualFields:   []string{"status", "departmentId"},
			SortFields:    []string{"id", "createTime", "username"},
			HiddenFields:  []string{"password"},
			DefaultSort:   "id",
			DefaultOrder:  "DESC",
		},
	})
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}
	resource, ok := registry.Resource("user")
	if !ok {
		t.Fatal("expected user resource")
	}
	return resource
}

func testPlatformScope() tenant.Scope {
	return tenant.Resolve(testPlatformContext())
}

func testPlatformContext() context.Context {
	return security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
}

func testTenantScope(t *testing.T, tenantID int64) tenant.Scope {
	t.Helper()
	identity, err := security.NewTenantIdentity(tenantID)
	if err != nil {
		t.Fatalf("create tenant identity failed: %v", err)
	}
	ctx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: identity})
	return tenant.Resolve(ctx)
}

func TestBuildInsertQueryMapsCamelCaseFields(t *testing.T) {
	resource := testUserResource(t)
	query, err := buildInsertQuery(resource, map[string]interface{}{
		"username":     "alice",
		"departmentId": 1,
		"password":     "secret",
	}, testPlatformScope())
	if err != nil {
		t.Fatalf("build insert failed: %v", err)
	}

	expected := "INSERT INTO `base_sys_user` (`createTime`, `departmentId`, `password`, `tenantId`, `updateTime`, `username`) VALUES (?, ?, ?, ?, ?, ?)"
	if query.SQL != expected {
		t.Fatalf("expected sql %s, got %s", expected, query.SQL)
	}
	if len(query.Args) != 6 || query.Args[1] != 1 || query.Args[2] != "secret" || query.Args[3] != nil || query.Args[5] != "alice" || query.Args[0] == "" || query.Args[4] == "" {
		t.Fatalf("unexpected args: %#v", query.Args)
	}
}

func TestBuildInsertQueryOverridesForgedTenantID(t *testing.T) {
	resource := testUserResource(t)
	input := map[string]interface{}{
		"username": "alice",
		"tenantId": int64(999),
	}
	query, err := buildInsertQuery(resource, input, testTenantScope(t, 7))
	if err != nil {
		t.Fatalf("build tenant insert failed: %v", err)
	}

	expectedSQL := "INSERT INTO `base_sys_user` (`createTime`, `tenantId`, `updateTime`, `username`) VALUES (?, ?, ?, ?)"
	if query.SQL != expectedSQL {
		t.Fatalf("expected sql %s, got %s", expectedSQL, query.SQL)
	}
	if len(query.Args) != 4 || query.Args[1] != int64(7) {
		t.Fatalf("expected server tenant parameter, got %#v", query.Args)
	}
	if input["tenantId"] != int64(999) {
		t.Fatalf("query builder must not mutate caller input: %#v", input)
	}
}

func TestBuildInsertQueryRejectsUnknownAndReadonlyFields(t *testing.T) {
	resource := testUserResource(t)
	if _, err := buildInsertQuery(resource, map[string]interface{}{"unknown": "value"}, testPlatformScope()); err == nil {
		t.Fatal("expected unknown field error")
	}
	if _, err := buildInsertQuery(resource, map[string]interface{}{"id": 1, "username": "alice"}, testPlatformScope()); err == nil {
		t.Fatal("expected readonly id error")
	}
}

func TestBuildUpdateQueryRequiresIDAndSkipsIDUpdate(t *testing.T) {
	resource := testUserResource(t)
	query, idValue, err := buildUpdateQuery(resource, map[string]interface{}{
		"id":       1,
		"nickName": "Alice",
	}, testPlatformScope())
	if err != nil {
		t.Fatalf("build update failed: %v", err)
	}

	expected := "UPDATE `base_sys_user` SET `nickName` = ?, `updateTime` = CURRENT_TIMESTAMP WHERE `id` = ?"
	if query.SQL != expected {
		t.Fatalf("expected sql %s, got %s", expected, query.SQL)
	}
	if idValue != 1 || len(query.Args) != 2 || query.Args[0] != "Alice" || query.Args[1] != 1 {
		t.Fatalf("unexpected update args: id=%#v args=%#v", idValue, query.Args)
	}
	if _, _, err = buildUpdateQuery(resource, map[string]interface{}{"nickName": "Alice"}, testPlatformScope()); err == nil {
		t.Fatal("expected missing id error")
	}
}

func TestBuildUpdateQueryIgnoresReadonlyFields(t *testing.T) {
	resource := testUserResource(t)
	query, _, err := buildUpdateQuery(resource, map[string]interface{}{
		"id":         1,
		"nickName":   "Alice",
		"tenantId":   2,
		"createTime": "2026-01-01 00:00:00",
		"updateTime": "2026-01-02 00:00:00",
	}, testPlatformScope())
	if err != nil {
		t.Fatalf("build update failed: %v", err)
	}
	if strings.Contains(query.SQL, "`tenantId`") || strings.Contains(query.SQL, "`createTime`") {
		t.Fatalf("expected readonly fields ignored, got %s", query.SQL)
	}
	if !strings.Contains(query.SQL, "`nickName` = ?") {
		t.Fatalf("expected nickName updated, got %s", query.SQL)
	}
}

func TestBuildDeleteQueryUsesPlaceholders(t *testing.T) {
	resource := testUserResource(t)
	query, err := buildDeleteQuery(resource, []interface{}{1, 2}, testPlatformScope())
	if err != nil {
		t.Fatalf("build delete failed: %v", err)
	}

	expected := "DELETE FROM `base_sys_user` WHERE `id` IN (?, ?)"
	if query.SQL != expected {
		t.Fatalf("expected sql %s, got %s", expected, query.SQL)
	}
	if len(query.Args) != 2 || query.Args[0] != 1 || query.Args[1] != 2 {
		t.Fatalf("unexpected args: %#v", query.Args)
	}
}

func TestTenantScopeAddsParameterizedPredicates(t *testing.T) {
	resource := testUserResource(t)
	scope := testTenantScope(t, 12)

	infoQuery, err := buildInfoQuery(resource, 9, scope)
	if err != nil {
		t.Fatalf("build tenant info failed: %v", err)
	}
	if !strings.Contains(infoQuery.SQL, "WHERE `id` = ? AND `tenantId` = ?") || !reflect.DeepEqual(infoQuery.Args, []interface{}{9, int64(12)}) {
		t.Fatalf("unexpected tenant info query: %s %#v", infoQuery.SQL, infoQuery.Args)
	}

	listQuery, err := buildListQuery(resource, QueryRequest{
		FieldEq: map[string]interface{}{"status": 1},
	}, scope)
	if err != nil {
		t.Fatalf("build tenant list failed: %v", err)
	}
	if !strings.Contains(listQuery.SQL, "WHERE `tenantId` = ? AND `status` = ?") || !reflect.DeepEqual(listQuery.Args, []interface{}{int64(12), 1, MaxListSize}) {
		t.Fatalf("unexpected tenant list query: %s %#v", listQuery.SQL, listQuery.Args)
	}

	dataQuery, countQuery, _, err := buildPageQuery(resource, QueryRequest{Page: 2, Size: 10}, scope)
	if err != nil {
		t.Fatalf("build tenant page failed: %v", err)
	}
	if !strings.Contains(dataQuery.SQL, "WHERE `tenantId` = ?") || !reflect.DeepEqual(dataQuery.Args, []interface{}{int64(12), 10, 10}) {
		t.Fatalf("unexpected tenant page query: %s %#v", dataQuery.SQL, dataQuery.Args)
	}
	if !strings.Contains(countQuery.SQL, "WHERE `tenantId` = ?") || !reflect.DeepEqual(countQuery.Args, []interface{}{int64(12)}) {
		t.Fatalf("unexpected tenant count query: %s %#v", countQuery.SQL, countQuery.Args)
	}

	updateQuery, _, err := buildUpdateQuery(resource, map[string]interface{}{
		"id":       9,
		"nickName": "Alice",
	}, scope)
	if err != nil {
		t.Fatalf("build tenant update failed: %v", err)
	}
	if !strings.Contains(updateQuery.SQL, "WHERE `id` = ? AND `tenantId` = ?") || !reflect.DeepEqual(updateQuery.Args, []interface{}{"Alice", 9, int64(12)}) {
		t.Fatalf("unexpected tenant update query: %s %#v", updateQuery.SQL, updateQuery.Args)
	}

	deleteQuery, err := buildDeleteQuery(resource, []interface{}{9, 10}, scope)
	if err != nil {
		t.Fatalf("build tenant delete failed: %v", err)
	}
	if !strings.Contains(deleteQuery.SQL, "WHERE `id` IN (?, ?) AND `tenantId` = ?") || !reflect.DeepEqual(deleteQuery.Args, []interface{}{9, 10, int64(12)}) {
		t.Fatalf("unexpected tenant delete query: %s %#v", deleteQuery.SQL, deleteQuery.Args)
	}

	for _, querySQL := range []string{infoQuery.SQL, listQuery.SQL, dataQuery.SQL, countQuery.SQL, updateQuery.SQL, deleteQuery.SQL} {
		if strings.Contains(querySQL, "12") {
			t.Fatalf("tenant id must not be interpolated into sql: %s", querySQL)
		}
	}
}

func TestPlatformAndForTenantScopesHaveDistinctPredicates(t *testing.T) {
	resource := testUserResource(t)
	platformQuery, err := buildInfoQuery(resource, 5, testPlatformScope())
	if err != nil {
		t.Fatalf("build platform info failed: %v", err)
	}
	if strings.Contains(platformQuery.SQL, "`tenantId` = ?") || !reflect.DeepEqual(platformQuery.Args, []interface{}{5}) {
		t.Fatalf("platform scope must not add tenant predicate: %s %#v", platformQuery.SQL, platformQuery.Args)
	}

	platformCtx := security.ContextWithUser(context.Background(), security.UserContext{TenantId: security.PlatformTenant()})
	derivedCtx, err := tenant.ForTenant(platformCtx, 21)
	if err != nil {
		t.Fatalf("derive tenant context failed: %v", err)
	}
	derivedQuery, err := buildInfoQuery(resource, 5, tenant.Resolve(derivedCtx))
	if err != nil {
		t.Fatalf("build derived tenant info failed: %v", err)
	}
	if !strings.Contains(derivedQuery.SQL, "`tenantId` = ?") || !reflect.DeepEqual(derivedQuery.Args, []interface{}{5, int64(21)}) {
		t.Fatalf("ForTenant must constrain platform query: %s %#v", derivedQuery.SQL, derivedQuery.Args)
	}
}

func TestTenantAwareQueryBuildersRejectMissingScope(t *testing.T) {
	resource := testUserResource(t)
	scope := tenant.Resolve(context.Background())
	tests := []struct {
		name string
		run  func() error
	}{
		{name: "insert", run: func() error {
			_, err := buildInsertQuery(resource, map[string]interface{}{"username": "alice"}, scope)
			return err
		}},
		{name: "info", run: func() error {
			_, err := buildInfoQuery(resource, 1, scope)
			return err
		}},
		{name: "list", run: func() error {
			_, err := buildListQuery(resource, QueryRequest{}, scope)
			return err
		}},
		{name: "page", run: func() error {
			_, _, _, err := buildPageQuery(resource, QueryRequest{}, scope)
			return err
		}},
		{name: "update", run: func() error {
			_, _, err := buildUpdateQuery(resource, map[string]interface{}{"id": 1, "nickName": "Alice"}, scope)
			return err
		}},
		{name: "delete", run: func() error {
			_, err := buildDeleteQuery(resource, []interface{}{1}, scope)
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); err == nil {
				t.Fatal("expected missing scope rejected")
			}
		})
	}
}

func TestSelectColumnsHidePasswordAndAliasCamelCase(t *testing.T) {
	resource := testUserResource(t)
	columns := selectColumns(resource)
	if strings.Contains(columns, "`password`") {
		t.Fatalf("expected password hidden, got %s", columns)
	}
	if !strings.Contains(columns, "`departmentId` AS `departmentId`") {
		t.Fatalf("expected department alias, got %s", columns)
	}
	if !strings.Contains(columns, "`nickName` AS `nickName`") {
		t.Fatalf("expected nickName alias, got %s", columns)
	}
}

func TestBuildListQueryUsesWhitelistedFiltersAndSort(t *testing.T) {
	resource := testUserResource(t)
	query, err := buildListQuery(resource, QueryRequest{
		Keyword: "admin",
		Sort:    "username",
		Order:   "ASC",
		FieldEq: map[string]interface{}{
			"status": 1,
		},
	}, testPlatformScope())
	if err != nil {
		t.Fatalf("build list failed: %v", err)
	}
	if !strings.Contains(query.SQL, "`name` LIKE ?") || !strings.Contains(query.SQL, "`username` LIKE ?") || !strings.Contains(query.SQL, "`nickName` LIKE ?") {
		t.Fatalf("expected keyword fields, got %s", query.SQL)
	}
	if !strings.Contains(query.SQL, "`status` = ?") {
		t.Fatalf("expected status condition, got %s", query.SQL)
	}
	if !strings.Contains(query.SQL, "ORDER BY `username` ASC") {
		t.Fatalf("expected username sort, got %s", query.SQL)
	}
	if len(query.Args) != 5 || query.Args[4] != MaxListSize {
		t.Fatalf("expected filters followed by list limit, got %#v", query.Args)
	}
}

func TestRequestQueryMapsNodeSortConvention(t *testing.T) {
	resource := testUserResource(t)
	request := requestQuery(resource, resource.PageQuery, map[string]interface{}{
		"page":  1,
		"size":  15,
		"order": "createTime",
		"sort":  "desc",
	})
	if request.Sort != "createTime" {
		t.Fatalf("expected sort field createTime from Node order, got %s", request.Sort)
	}
	if request.Order != "desc" {
		t.Fatalf("expected order desc from Node sort, got %s", request.Order)
	}
}

func TestBuildListQueryRejectsUnsafeFilterAndSort(t *testing.T) {
	resource := testUserResource(t)
	if _, err := buildListQuery(resource, QueryRequest{FieldEq: map[string]interface{}{"username": "admin"}}, testPlatformScope()); err == nil {
		t.Fatal("expected unsupported eq field error")
	}
	if _, err := buildListQuery(resource, QueryRequest{Sort: "username desc; drop table"}, testPlatformScope()); err == nil {
		t.Fatal("expected unsupported sort field error")
	}
}

func TestBuildPageQueryNormalizesPagination(t *testing.T) {
	resource := testUserResource(t)
	dataQuery, countQuery, request, err := buildPageQuery(resource, QueryRequest{Page: -1, Size: 10000}, testPlatformScope())
	if err != nil {
		t.Fatalf("build page failed: %v", err)
	}
	if request.Page != defaultPage || request.Size != MaxPageSize {
		t.Fatalf("unexpected normalized page request: %#v", request)
	}
	if !strings.Contains(dataQuery.SQL, "LIMIT ? OFFSET ?") {
		t.Fatalf("expected limit offset, got %s", dataQuery.SQL)
	}
	if !strings.HasPrefix(countQuery.SQL, "SELECT COUNT(*) FROM `base_sys_user`") {
		t.Fatalf("unexpected count sql: %s", countQuery.SQL)
	}
}

func TestBuildQuerySupportsNodeArrayFilterAndMultiSort(t *testing.T) {
	resource := testUserResource(t)
	query, err := buildListQuery(resource, QueryRequest{
		Sort:  "username,createTime",
		Order: "ASC,DESC",
		FieldEq: map[string]interface{}{
			"status": []interface{}{0, 1},
		},
	}, testPlatformScope())
	if err != nil {
		t.Fatalf("build list failed: %v", err)
	}
	if !strings.Contains(query.SQL, "`status` IN (?,?)") || !strings.Contains(query.SQL, "ORDER BY `username` ASC, `createTime` DESC") {
		t.Fatalf("unexpected Node-compatible query: %s", query.SQL)
	}
	if len(query.Args) != 3 || query.Args[0] != 0 || query.Args[1] != 1 || query.Args[2] != MaxListSize {
		t.Fatalf("unexpected array filter args: %#v", query.Args)
	}
}

func TestBuildPageQuerySupportsExportLimit(t *testing.T) {
	resource := testUserResource(t)
	query, _, _, err := buildPageQuery(resource, QueryRequest{Page: 2, Size: 15, IsExport: true, MaxExportLimit: 1000}, testPlatformScope())
	if err != nil {
		t.Fatalf("build export page failed: %v", err)
	}
	if !strings.HasSuffix(query.SQL, "LIMIT ?") || len(query.Args) != 1 || query.Args[0] != 1000 {
		t.Fatalf("unexpected export query: %s %#v", query.SQL, query.Args)
	}
}

func TestBuildPageQueryUsesServerLimitWhenExportLimitIsMissing(t *testing.T) {
	resource := testUserResource(t)
	query, _, _, err := buildPageQuery(resource, QueryRequest{Page: 2, Size: 15, IsExport: true}, testPlatformScope())
	if err != nil {
		t.Fatalf("build export page failed: %v", err)
	}
	if !strings.HasSuffix(query.SQL, "LIMIT ?") || len(query.Args) != 1 || query.Args[0] != MaxExportSize {
		t.Fatalf("unexpected export query without max limit: %s %#v", query.SQL, query.Args)
	}
}

func TestBuildPageQueryCapsClientExportLimit(t *testing.T) {
	resource := testUserResource(t)
	query, _, request, err := buildPageQuery(resource, QueryRequest{Page: 1, Size: 15, IsExport: true, MaxExportLimit: MaxExportSize + 1}, testPlatformScope())
	if err != nil {
		t.Fatalf("build capped export failed: %v", err)
	}
	if request.MaxExportLimit != MaxExportSize || len(query.Args) != 1 || query.Args[0] != MaxExportSize {
		t.Fatalf("unexpected capped export query: %#v %s %#v", request, query.SQL, query.Args)
	}
}

func TestInfoIgnoreFieldsOnlyFilterInfoColumnsAndRecords(t *testing.T) {
	registry, err := NewRegistry([]ResourceSpec{
		{
			Name:             "user-info",
			Prefix:           "/admin/base/sys/user-info",
			Model:            baseModel.BaseSysUser(),
			InfoIgnoreFields: []string{"password"},
		},
	})
	if err != nil {
		t.Fatalf("create registry failed: %v", err)
	}
	resource, ok := registry.Resource("user-info")
	if !ok {
		t.Fatal("expected user-info resource")
	}

	infoQuery, err := buildInfoQuery(resource, 1, testPlatformScope())
	if err != nil {
		t.Fatalf("build info query failed: %v", err)
	}
	if strings.Contains(infoQuery.SQL, "`password`") {
		t.Fatalf("expected info query to ignore password, got %s", infoQuery.SQL)
	}

	listQuery, err := buildListQuery(resource, QueryRequest{}, testPlatformScope())
	if err != nil {
		t.Fatalf("build list query failed: %v", err)
	}
	if !strings.Contains(listQuery.SQL, "`password` AS `password`") {
		t.Fatalf("expected list query to retain password, got %s", listQuery.SQL)
	}
	pageQuery, _, _, err := buildPageQuery(resource, QueryRequest{}, testPlatformScope())
	if err != nil {
		t.Fatalf("build page query failed: %v", err)
	}
	if !strings.Contains(pageQuery.SQL, "`password` AS `password`") {
		t.Fatalf("expected page query to retain password, got %s", pageQuery.SQL)
	}

	row := gdb.Record{
		"password": gvar.New("secret"),
		"username": gvar.New("alice"),
	}
	infoRecord := mapRecord(resource, row, true)
	if _, ok := infoRecord["password"]; ok {
		t.Fatalf("expected info response to ignore password, got %#v", infoRecord)
	}
	listRecord := mapRecord(resource, row, false)
	if listRecord["password"] != "secret" {
		t.Fatalf("expected list response to retain password, got %#v", listRecord)
	}
}
