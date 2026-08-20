package sys

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	baseEntity "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestParseLogKeepDaysRejectsUnsafeValues(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "invalid"} {
		if _, err := parseLogKeepDays(value); err == nil {
			t.Fatalf("expected invalid log keep value rejected: %q", value)
		}
	}
	keepDays, err := parseLogKeepDays(" 31 ")
	if err != nil || keepDays != 31 {
		t.Fatalf("unexpected log keep parse result: %d, %v", keepDays, err)
	}
}

func TestLogRetentionCutoffMatchesNodeStartOfDay(t *testing.T) {
	location := time.Local
	now := time.Date(2026, time.July, 29, 21, 30, 45, 0, location)
	want := time.Date(2026, time.June, 28, 0, 0, 0, 0, location)
	if got := logRetentionCutoff(now, 31); !got.Equal(want) {
		t.Fatalf("unexpected log retention cutoff: got %s want %s", got, want)
	}
}

func TestLogClearExpiredBuildsCrossTenantBatchDelete(t *testing.T) {
	service := newTestLogService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysLog())
	before := time.Date(2026, time.June, 28, 0, 0, 0, 0, time.Local)
	sqlText, err := gdb.ToSQL(baseTenantServiceContext(t, 17), func(ctx context.Context) error {
		_, clearErr := service.clearExpiredBefore(ctx, before)
		return clearErr
	})
	if err != nil {
		t.Fatalf("build expired log cleanup SQL failed: %v", err)
	}
	lowerSQL := strings.ToLower(sqlText)
	if !strings.Contains(lowerSQL, "delete from `base_sys_log`") ||
		!strings.Contains(lowerSQL, "`createtime` < '2026-06-28 00:00:00'") ||
		!strings.Contains(lowerSQL, "limit 1000") {
		t.Fatalf("unexpected expired log cleanup SQL: %s", sqlText)
	}
	if strings.Contains(lowerSQL, "tenantid") {
		t.Fatalf("scheduled cleanup must cross tenant boundaries: %s", sqlText)
	}
}

func TestNormalizeLogParamsMatchesNodeJSONTransformer(t *testing.T) {
	object := normalizeLogParams(`{"password":"***"}`)
	values, ok := object.(map[string]interface{})
	if !ok || values["password"] != "***" {
		t.Fatalf("expected decoded params object, got %#v", object)
	}
	plain := normalizeLogParams("not-json")
	if plain != "not-json" {
		t.Fatalf("expected invalid JSON to remain text, got %#v", plain)
	}
}

func TestLogRecordDerivesTenantFromContext(t *testing.T) {
	service := newTestLogService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysLog())
	sqlText, err := gdb.ToSQL(baseTenantServiceContext(t, 17), func(ctx context.Context) error {
		return service.Record(ctx, LogRecordRequest{Action: "/admin/test", TenantID: 99})
	})
	if err != nil {
		t.Fatalf("build tenant log record failed: %v", err)
	}
	if !strings.Contains(sqlText, "`tenantId`") || !strings.Contains(sqlText, "17") || strings.Contains(sqlText, "99") {
		t.Fatalf("request tenant changed log ownership: %s", sqlText)
	}
}

func TestAnonymousLogRecordWritesPlatformNull(t *testing.T) {
	service := newTestLogService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysLog())
	sqlText, err := gdb.ToSQL(context.Background(), func(ctx context.Context) error {
		return service.Record(ctx, LogRecordRequest{Action: "/admin/base/open/login", TenantID: 99})
	})
	if err != nil || !strings.Contains(strings.ToUpper(sqlText), "NULL") || strings.Contains(sqlText, "99") {
		t.Fatalf("unexpected anonymous log SQL: %s, %v", sqlText, err)
	}
}

func TestLogManagementRejectsMissingScope(t *testing.T) {
	service := newTestLogService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysLog())
	if _, err := service.Page(context.Background(), crud.QueryRequest{}); err == nil {
		t.Fatal("expected missing log page scope rejected")
	}
	if err := service.Clear(context.Background()); err == nil {
		t.Fatal("expected missing log clear scope rejected")
	}
}

func TestLogJoinConditionsScopeBothTables(t *testing.T) {
	service := newTestLogService(newBaseTenantServiceTestDB(t), baseEntity.BaseSysLog())
	ctx := baseTenantServiceContext(t, 23)
	logCondition, err := service.logTenantCondition(ctx, service.Model, "a")
	if err != nil || logCondition.SQL != "`a`.`tenantId` = ?" || len(logCondition.Args) != 1 || logCondition.Args[0] != int64(23) {
		t.Fatalf("unexpected log condition: %#v, %v", logCondition, err)
	}
	userCondition, err := service.logTenantCondition(ctx, service.userModel, "b")
	if err != nil || userCondition.SQL != "`b`.`tenantId` = ?" || len(userCondition.Args) != 1 || userCondition.Args[0] != int64(23) {
		t.Fatalf("unexpected log user condition: %#v, %v", userCondition, err)
	}
}
