package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
)

func TestTenantScopeExplainMySQLIntegration(t *testing.T) {
	if os.Getenv("COOL_CUSTOM_API_INTEGRATION") != "1" {
		t.Skip("set COOL_CUSTOM_API_INTEGRATION=1 to run tenant explain integration test")
	}
	ctx := context.Background()
	db := g.DB()
	if _, err := schema.NewSyncer(db).Sync(ctx, baseModelDefinitions()); err != nil {
		t.Fatalf("schema sync failed: %v", err)
	}

	testCases := []struct {
		name          string
		query         string
		args          []interface{}
		requiredIndex string
	}{
		{
			name:          "page",
			query:         "EXPLAIN SELECT id FROM base_sys_param WHERE tenantId = ? ORDER BY id DESC LIMIT 15",
			args:          []interface{}{relationScopeTenantA},
			requiredIndex: "idx_base_sys_param_tenant_id",
		},
		{
			name:          "update",
			query:         "EXPLAIN UPDATE base_sys_param SET remark = ? WHERE id = ? AND tenantId = ?",
			args:          []interface{}{"tenant-explain", relationScopeTenantA, relationScopeTenantA},
			requiredIndex: "idx_base_sys_param_tenant_id",
		},
		{
			name:          "delete",
			query:         "EXPLAIN DELETE FROM base_sys_param WHERE id = ? AND tenantId = ?",
			args:          []interface{}{relationScopeTenantA, relationScopeTenantA},
			requiredIndex: "idx_base_sys_param_tenant_id",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			rows, err := db.GetAll(ctx, testCase.query, testCase.args...)
			if err != nil {
				t.Fatalf("explain tenant %s failed: %v", testCase.name, err)
			}
			if len(rows) != 1 {
				t.Fatalf("unexpected tenant %s plan rows: %#v", testCase.name, rows)
			}
			plan := rows[0]
			possibleIndexes := plan["possible_keys"].String()
			selectedIndex := plan["key"].String()
			accessType := plan["type"].String()
			if !strings.Contains(possibleIndexes, testCase.requiredIndex) {
				t.Fatalf("tenant %s plan cannot use tenant index: %#v", testCase.name, plan.Map())
			}
			if selectedIndex == "" || accessType == "ALL" {
				t.Fatalf("tenant %s plan performs full scan: %#v", testCase.name, plan.Map())
			}
			t.Logf("tenant %s plan: key=%s type=%s possible=%s", testCase.name, selectedIndex, accessType, possibleIndexes)
		})
	}
}
