package service

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/task"
)

func TestInfoServiceParseDraftNormalizesNodeFields(t *testing.T) {
	service := newValidationInfoService(t)
	cronDraft, err := service.parseDraft(map[string]interface{}{
		"name": "cron", "taskType": 0, "cron": "*/5 * * * * *", "every": 5000,
		"repeatCount": json.Number("3"), "service": `taskDemoService.test({"value": [1, 2]})`,
	}, nil)
	if err != nil {
		t.Fatalf("解析 Cron 任务失败: %v", err)
	}
	if cronDraft.Every != nil || cronDraft.Limit == nil || *cronDraft.Limit != 3 {
		t.Fatalf("Cron 字段归一化异常: %#v", cronDraft)
	}

	intervalDraft, err := service.parseDraft(map[string]interface{}{
		"name": "interval", "taskType": 1, "cron": "*/5 * * * * *", "every": 1000,
		"service": "taskDemoService.test()",
	}, nil)
	if err != nil {
		t.Fatalf("解析间隔任务失败: %v", err)
	}
	if intervalDraft.Cron != "" || intervalDraft.Every == nil || *intervalDraft.Every != 1000 {
		t.Fatalf("间隔字段归一化异常: %#v", intervalDraft)
	}
}

func TestInfoServiceParseDraftRejectsInvalidInput(t *testing.T) {
	service := newValidationInfoService(t)
	for _, input := range []map[string]interface{}{
		{"name": "bad", "status": "running", "taskType": 0, "cron": "* * * * * *", "service": "taskDemoService.test()"},
		{"name": "bad", "status": 1.5, "taskType": 0, "cron": "* * * * * *", "service": "taskDemoService.test()"},
		{"name": "bad", "taskType": 1, "every": 999, "service": "taskDemoService.test()"},
		{"name": "bad", "taskType": 1, "every": maxTaskEveryMilliseconds + 1, "service": "taskDemoService.test()"},
		{"name": "bad", "taskType": 1, "every": int64(math.MaxInt64), "service": "taskDemoService.test()"},
		{"name": "bad", "taskType": 0, "cron": "* * * * * *", "service": "unknownService.test()"},
	} {
		if _, err := service.parseDraft(input, nil); err == nil {
			t.Fatalf("非法任务应被拒绝: %#v", input)
		}
	}
}

func TestInfoServiceParseDraftAcceptsEveryProductBounds(t *testing.T) {
	service := newValidationInfoService(t)
	for _, every := range []int64{minTaskEveryMilliseconds, maxTaskEveryMilliseconds} {
		draft, err := service.parseDraft(map[string]interface{}{
			"name": "valid", "taskType": 1, "every": every, "service": "taskDemoService.test()",
		}, nil)
		if err != nil || draft.Every == nil || *draft.Every != every {
			t.Fatalf("合法 every 被拒绝: every=%d draft=%#v err=%v", every, draft, err)
		}
	}
}

func TestInfoServiceMapRestoresLimitAndPreservesData(t *testing.T) {
	service := newValidationInfoService(t)
	limit := 3
	info := TaskInfo{
		Name: "limited", Cron: "0 * * * * *", Limit: &limit, Status: 0,
		Data: "  raw data  ", Service: "taskDemoService.test()", TaskType: 0, LockOwner: "internal-owner",
	}
	mapped := taskInfoMap(info)
	if _, isExposed := mapped["lockOwner"]; isExposed {
		t.Fatalf("任务详情泄露了执行租约所有者: %#v", mapped)
	}
	draft, err := service.parseDraft(mapped, &info)
	if err != nil {
		t.Fatalf("恢复任务失败: %v", err)
	}
	if draft.Limit == nil || *draft.Limit != limit || draft.Data != info.Data {
		t.Fatalf("恢复字段异常: %#v", draft)
	}
}

func TestInfoServiceScheduleChangedIgnoresPresentationFields(t *testing.T) {
	limit := 2
	every := int64(1000)
	current := TaskInfo{
		Status: 1, TaskType: 1, Every: &every, Limit: &limit,
		Service: "taskDemoService.test()", Data: "data", Name: "before", Remark: "before", Type: 0,
	}
	draft := infoDraftFromTask(current)
	draft.Name = "after"
	draft.Remark = "after"
	draft.Type = 1
	if infoScheduleChanged(current, draft) {
		t.Fatal("展示字段变化不应替换 jobId")
	}
	draft.Data = "changed"
	if !infoScheduleChanged(current, draft) {
		t.Fatal("业务数据变化应替换 jobId")
	}
}

func newValidationInfoService(t *testing.T) *InfoService {
	t.Helper()
	builder := task.NewRegistryBuilder()
	if err := builder.Register(task.HandlerDefinition{
		Name:    "taskDemoService.test",
		Handler: func(context.Context, task.Invocation) (interface{}, error) { return nil, nil },
	}); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return &InfoService{registry: registry, location: time.FixedZone("test", 8*60*60)}
}
