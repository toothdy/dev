package service

import (
	"math"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

func TestScheduleForInfoBuildsOpaqueScheduleAndTaskMessage(t *testing.T) {
	var (
		tenantID = int64(12)
		every    = int64(5000)
		start    = time.Unix(100, 0).UTC()
		end      = start.Add(time.Hour)
	)
	info := TaskInfo{
		ID: 23, JobID: "job-23", TenantID: &tenantID,
		CreateTime: "2026-07-28 10:00:00", Every: &every,
		StartDate: gtime.NewFromTime(start), EndDate: gtime.NewFromTime(end),
	}
	schedule, err := scheduleForInfo(info, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ID != "23" || schedule.Every != 5*time.Second || schedule.Message == nil {
		t.Fatalf("通用计划字段异常: %#v", schedule)
	}
	scheduledAt := start.Add(time.Minute)
	message, err := schedule.Message(scheduledAt)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := taskQueue.Decode(message)
	if err != nil {
		t.Fatal(err)
	}
	if payload.TaskID != info.ID || payload.JobID != info.JobID || payload.TenantID == nil || *payload.TenantID != tenantID || !payload.ScheduledAt.Equal(scheduledAt) || payload.Manual {
		t.Fatalf("Task 计划消息异常: %#v", payload)
	}
}

func TestValidateStoredScheduleRejectsLegacySubsecondInterval(t *testing.T) {
	for _, every := range []int64{999, maxTaskEveryMilliseconds + 1, math.MaxInt64} {
		if err := validateStoredSchedule(TaskInfo{TaskType: 1, Every: &every}); err == nil {
			t.Fatalf("Reconcile 不能重新调度历史非法间隔任务: %d", every)
		}
	}
	for _, every := range []int64{minTaskEveryMilliseconds, maxTaskEveryMilliseconds} {
		if err := validateStoredSchedule(TaskInfo{TaskType: 1, Every: &every}); err != nil {
			t.Fatalf("合法间隔任务不应被 Reconcile 拒绝: every=%d err=%v", every, err)
		}
	}
}
