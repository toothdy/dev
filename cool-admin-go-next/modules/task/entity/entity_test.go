package entity_test

import (
	"slices"
	"testing"

	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
)

func TestInfoDescriptorMatchesNodeColumns(t *testing.T) {
	descriptor, err := coreentity.Compile[entity.Info, uint64](entity.InfoSchema())
	if err != nil {
		t.Fatalf("编译任务信息 Descriptor 失败: %v", err)
	}
	if descriptor.Table() != "task_info" {
		t.Errorf("表名 = %q，期望 task_info", descriptor.Table())
	}
	nullable := map[string]bool{
		"jobId": true, "repeatConf": true, "name": false, "cron": true, "limit": true,
		"every": true, "remark": true, "status": false, "startDate": true, "endDate": true,
		"data": true, "service": true, "type": false, "nextRunTime": true, "taskType": false,
		"lastExecuteTime": true, "lockExpireTime": true,
	}
	for name, expected := range nullable {
		field, exists := descriptor.JSON(name)
		if !exists {
			t.Errorf("缺少字段 %s", name)
			continue
		}
		if field.Nullable() != expected {
			t.Errorf("字段 %s 可空 = %v，期望 %v", name, field.Nullable(), expected)
		}
	}
	for name, expected := range map[string]string{"status": "1", "type": "0", "taskType": "0"} {
		field, _ := descriptor.JSON(name)
		if got := field.Constraints().Default; got != expected {
			t.Errorf("字段 %s 默认值 = %q，期望 %q", name, got, expected)
		}
	}
}

func TestLogDescriptorCarriesTaskIndex(t *testing.T) {
	descriptor, err := coreentity.Compile[entity.Log, uint64](entity.LogSchema())
	if err != nil {
		t.Fatalf("编译任务日志 Descriptor 失败: %v", err)
	}
	if descriptor.Table() != "task_log" {
		t.Errorf("表名 = %q，期望 task_log", descriptor.Table())
	}
	detail, exists := descriptor.JSON("detail")
	if !exists || !detail.Nullable() || detail.Constraints().HasSize {
		t.Error("detail 必须是可空且不带长度约束的文本列")
	}
	if !slices.ContainsFunc(descriptor.Indexes(), func(index coreentity.Index) bool {
		return index.Name == "idx_task_log_task_id" && slices.Equal(index.Fields, []string{"taskId"})
	}) {
		t.Errorf("索引 = %+v，缺少 idx_task_log_task_id", descriptor.Indexes())
	}
}
