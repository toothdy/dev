package entity

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/entity"
)

func TestTaskModelsKeepNodeFieldsAndTenantIndexes(t *testing.T) {
	models := []entity.Definition{TaskInfo(), TaskLog()}
	if len(models) != 2 {
		t.Fatalf("expected two Task models, got %d", len(models))
	}
	info := TaskInfo()
	wantFields := []string{"jobId", "repeatConf", "name", "cron", "limit", "every", "status", "startDate", "endDate", "data", "service", "type", "nextRunTime", "taskType", "lastExecuteTime", "lockExpireTime", "lockOwner", "tenantId"}
	fields := map[string]bool{}
	for _, field := range info.FieldsValue {
		fields[field.JSONName] = true
	}
	for _, name := range wantFields {
		if !fields[name] {
			t.Fatalf("missing TaskInfo field %s", name)
		}
	}
	if info.TenantMode == 0 || TaskLog().TenantMode == 0 {
		t.Fatal("Task models must explicitly require tenant metadata")
	}
	if info.ResourceKey() != "task.info" || TaskLog().ResourceKey() != "task.log" {
		t.Fatalf("unexpected Task recycle resources: %s %s", info.ResourceKey(), TaskLog().ResourceKey())
	}
}
