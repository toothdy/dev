package service_test

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	coreentity "github.com/toothdy/cool-admin-go-next/cool-next/core/entity"
	coreservice "github.com/toothdy/cool-admin-go-next/cool-next/core/service"
	"github.com/toothdy/cool-admin-go-next/cool-next/crud"
	coredb "github.com/toothdy/cool-admin-go-next/cool-next/db"
	"github.com/toothdy/cool-admin-go-next/cool-next/db/recycle"
	dbschema "github.com/toothdy/cool-admin-go-next/cool-next/db/schema"
	task "github.com/toothdy/cool-admin-go-next/modules/task"
	"github.com/toothdy/cool-admin-go-next/modules/task/dto"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
	"github.com/toothdy/cool-admin-go-next/modules/task/service"
)

type harness struct {
	runtime   *coredb.Runtime
	info      *service.InfoService
	executor  *service.Executor
	scheduler *service.Scheduler
}

// 用真实 Schema 同步建表，同时验证 limit 这类保留字列可以落地
func newHarness(t *testing.T) *harness {
	t.Helper()

	infoDescriptor, err := coreentity.Compile[entity.Info, uint64](entity.InfoSchema())
	if err != nil {
		t.Fatal(err)
	}
	logDescriptor, err := coreentity.Compile[entity.Log, uint64](entity.LogSchema())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := coredb.New(t.Context(), coredb.Config{
		Group: "task_" + strings.ReplaceAll(t.Name(), "/", "_"),
		Nodes: gdb.ConfigGroup{{
			Type:      "sqlite",
			Link:      fmt.Sprintf("sqlite::@file(%s)", filepath.Join(t.TempDir(), "task.sqlite")),
			CreatedAt: "createTime",
			UpdatedAt: "updateTime",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := dbschema.New(runtime.DB(), runtime.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.Apply(t.Context(), dbschema.Sync, infoDescriptor, logDescriptor); err != nil {
		t.Fatalf("同步任务表结构失败: %v", err)
	}
	recycler, err := recycle.New(runtime, crud.Config{})
	if err != nil {
		t.Fatal(err)
	}
	infoBase, err := coreservice.NewBase[entity.Info, uint64](infoDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	logBase, err := coreservice.NewBase[entity.Log, uint64](logDescriptor, runtime, recycler)
	if err != nil {
		t.Fatal(err)
	}
	demo, err := service.NewDemo()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := service.NewRegistry(demo)
	if err != nil {
		t.Fatal(err)
	}
	executor, err := service.NewExecutor(runtime, infoBase, logBase, registry, task.ModuleConfig().Defaults)
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := service.NewScheduler(executor, infoBase)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Shutdown(t.Context()) })
	info, err := service.NewInfo(runtime, infoBase, logBase, scheduler)
	if err != nil {
		t.Fatal(err)
	}

	return &harness{runtime: runtime, info: info, executor: executor, scheduler: scheduler}
}

func (current *harness) seedTask(t *testing.T, columns g.Map) uint64 {
	t.Helper()

	id, err := current.runtime.DB().Model("task_info").Ctx(t.Context()).Data(columns).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}

	return uint64(id)
}

func (current *harness) seedLog(t *testing.T, columns g.Map) {
	t.Helper()

	if _, err := current.runtime.DB().Model("task_log").Ctx(t.Context()).Data(columns).Insert(); err != nil {
		t.Fatal(err)
	}
}

func (current *harness) taskColumn(t *testing.T, taskID uint64, column string) any {
	t.Helper()

	record, err := current.runtime.DB().Model("task_info").Ctx(t.Context()).Where("id", taskID).One()
	if err != nil {
		t.Fatal(err)
	}
	if record == nil {
		t.Fatalf("任务 %d 不存在", taskID)
	}

	return record[column].Val()
}

func (current *harness) logCount(t *testing.T, taskID uint64) int {
	t.Helper()

	count, err := current.runtime.DB().Model("task_log").Ctx(t.Context()).Where("taskId", taskID).Count()
	if err != nil {
		t.Fatal(err)
	}

	return count
}

func TestExecuteWritesSuccessLogAndReleasesLease(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"name": "每秒执行一次", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test(1,2)"})

	outcome, err := current.executor.Execute(t.Context(), taskID)
	if err != nil {
		t.Fatalf("执行任务失败: %v", err)
	}
	if !outcome.Executed || outcome.Removed {
		t.Fatalf("执行结果 = %+v，期望已执行", outcome)
	}
	if got := current.logCount(t, taskID); got != 1 {
		t.Fatalf("日志条数 = %d，期望 1", got)
	}
	record, err := current.runtime.DB().Model("task_log").Ctx(t.Context()).Where("taskId", taskID).One()
	if err != nil {
		t.Fatal(err)
	}
	if record["status"].Int32() != entity.LogStatusSuccess {
		t.Errorf("日志状态 = %v，期望成功", record["status"])
	}
	if record["detail"].String() != `"任务执行成功"` {
		t.Errorf("日志详情 = %q", record["detail"].String())
	}
	if current.taskColumn(t, taskID, "lockExpireTime") != nil {
		t.Error("执行结束必须释放执行租约")
	}
	if current.taskColumn(t, taskID, "lastExecuteTime") == nil {
		t.Error("执行必须记录最近执行时间")
	}
}

func TestExecuteRecordsFailureForUnknownTarget(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"name": "未注册目标", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "missingService.test()"})

	if _, err := current.executor.Execute(t.Context(), taskID); err != nil {
		t.Fatalf("执行任务失败: %v", err)
	}
	record, err := current.runtime.DB().Model("task_log").Ctx(t.Context()).Where("taskId", taskID).One()
	if err != nil {
		t.Fatal(err)
	}
	if record == nil || record["status"].Int32() != entity.LogStatusFail {
		t.Fatalf("日志 = %v，期望失败记录", record)
	}
	if current.taskColumn(t, taskID, "lockExpireTime") != nil {
		t.Error("失败后同样必须释放执行租约")
	}
}

func TestExecuteSkipsAccordingToTaskState(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	base := g.Map{"status": entity.StatusRunning, "taskType": entity.TaskTypeInterval,
		"every": 1000, "service": "taskDemoService.test()"}
	cases := []struct {
		name    string
		columns g.Map
		removed bool
	}{
		{name: "已停止", columns: g.Map{"name": "停止", "status": entity.StatusStopped}, removed: true},
		{name: "未到开始时间", columns: g.Map{"name": "未开始", "startDate": future}},
		{name: "已过结束时间", columns: g.Map{"name": "已结束", "endDate": past}},
		{name: "租约被其他实例持有", columns: g.Map{"name": "占用中", "lockExpireTime": future}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			current := newHarness(t)
			columns := g.Map{}
			maps.Copy(columns, base)
			maps.Copy(columns, testCase.columns)
			taskID := current.seedTask(t, columns)
			outcome, err := current.executor.Execute(t.Context(), taskID)
			if err != nil {
				t.Fatalf("执行任务失败: %v", err)
			}
			if outcome.Executed {
				t.Error("期望跳过执行")
			}
			if outcome.Removed != testCase.removed {
				t.Errorf("注销标记 = %v，期望 %v", outcome.Removed, testCase.removed)
			}
			if got := current.logCount(t, taskID); got != 0 {
				t.Errorf("日志条数 = %d，期望 0", got)
			}
		})
	}
}

func TestExecuteMarksMissingTaskRemoved(t *testing.T) {
	current := newHarness(t)
	outcome, err := current.executor.Execute(t.Context(), 4242)
	if err != nil {
		t.Fatalf("执行任务失败: %v", err)
	}
	if outcome.Executed || !outcome.Removed {
		t.Errorf("执行结果 = %+v，期望标记注销", outcome)
	}
}

func TestStartRegistersTimerAndStopRemovesIt(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"jobId": "cron-task", "name": "cron任务", "status": entity.StatusStopped,
		"taskType": entity.TaskTypeCron, "cron": "0/5 * * * * *", "service": "taskDemoService.test()"})

	if err := current.info.Start(t.Context(), &dto.StartRequest{ID: taskID}); err != nil {
		t.Fatalf("开始任务失败: %v", err)
	}
	if got := current.taskColumn(t, taskID, "status"); fmt.Sprint(got) != "1" {
		t.Errorf("状态 = %v，期望 1", got)
	}
	if current.taskColumn(t, taskID, "nextRunTime") == nil {
		t.Error("开始任务必须写入下次执行时间")
	}

	if err := current.info.Stop(t.Context(), &dto.TaskRequest{ID: taskID}); err != nil {
		t.Fatalf("停止任务失败: %v", err)
	}
	if got := current.taskColumn(t, taskID, "status"); fmt.Sprint(got) != "0" {
		t.Errorf("状态 = %v，期望 0", got)
	}
}

func TestStartOverridesTaskType(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"jobId": "job-type", "name": "间隔任务", "status": entity.StatusStopped,
		"type": 0, "taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})

	userType := int32(1)
	if err := current.info.Start(t.Context(), &dto.StartRequest{ID: taskID, Type: &userType}); err != nil {
		t.Fatalf("开始任务失败: %v", err)
	}
	if got := current.taskColumn(t, taskID, "type"); fmt.Sprint(got) != "1" {
		t.Errorf("归属类型 = %v，期望 1", got)
	}
}

func TestOnceExecutesWithoutChangingTaskState(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"name": "运行中", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})

	if err := current.info.Once(t.Context(), &dto.TaskRequest{ID: taskID}); err != nil {
		t.Fatalf("立即执行失败: %v", err)
	}
	if got := current.logCount(t, taskID); got != 1 {
		t.Errorf("日志条数 = %d，期望 1", got)
	}
	if current.taskColumn(t, taskID, "nextRunTime") != nil {
		t.Error("立即执行不应改写下次执行时间")
	}
}

// Node 的"立即执行"对已停止任务同样生效，只有定时触发才要求运行状态
func TestOnceRunsStoppedTaskButScheduledTriggerSkipsIt(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"name": "已停止", "status": entity.StatusStopped,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})

	outcome, err := current.executor.Execute(t.Context(), taskID)
	if err != nil {
		t.Fatalf("定时触发失败: %v", err)
	}
	if outcome.Executed || !outcome.Removed {
		t.Errorf("定时触发结果 = %+v，期望跳过并标记注销", outcome)
	}
	if err = current.info.Once(t.Context(), &dto.TaskRequest{ID: taskID}); err != nil {
		t.Fatalf("立即执行失败: %v", err)
	}
	if got := current.logCount(t, taskID); got != 1 {
		t.Errorf("日志条数 = %d，期望 1", got)
	}
	if got := current.taskColumn(t, taskID, "status"); fmt.Sprint(got) != "0" {
		t.Errorf("状态 = %v，立即执行不应改变任务状态", got)
	}
}

func TestMissingTaskIsReported(t *testing.T) {
	current := newHarness(t)
	if err := current.info.Start(t.Context(), &dto.StartRequest{ID: 4242}); err == nil {
		t.Error("开始不存在的任务期望失败")
	}
	if err := current.info.Stop(t.Context(), &dto.TaskRequest{ID: 4242}); err == nil {
		t.Error("停止不存在的任务期望失败")
	}
	if err := current.info.Once(t.Context(), &dto.TaskRequest{ID: 4242}); err == nil {
		t.Error("执行不存在的任务期望失败")
	}
}

func TestLogPagesAndFilters(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"name": "日志任务", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})
	other := current.seedTask(t, g.Map{"name": "其他任务", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})
	for index := range 5 {
		status := entity.LogStatusSuccess
		if index%2 == 0 {
			status = entity.LogStatusFail
		}
		current.seedLog(t, g.Map{"taskId": taskID, "status": status, "detail": fmt.Sprintf("detail-%d", index)})
	}
	current.seedLog(t, g.Map{"taskId": other, "status": entity.LogStatusSuccess, "detail": "other"})

	result, err := current.info.Log(t.Context(), &dto.LogRequest{ID: taskID, Page: 1, Size: 2})
	if err != nil {
		t.Fatalf("查询任务日志失败: %v", err)
	}
	if result.Pagination.Total != 5 || len(result.List) != 2 {
		t.Fatalf("分页 = %+v，条数 = %d", result.Pagination, len(result.List))
	}
	if result.List[0].Detail == nil || *result.List[0].Detail != "detail-4" {
		t.Errorf("首条日志 = %+v，期望按 id 倒序", result.List[0])
	}
	if result.List[0].TaskName == nil || *result.List[0].TaskName != "日志任务" {
		t.Errorf("任务名称 = %+v，期望关联出 taskName", result.List[0].TaskName)
	}

	failed := int32(entity.LogStatusFail)
	filtered, err := current.info.Log(t.Context(), &dto.LogRequest{ID: taskID, Status: &failed})
	if err != nil {
		t.Fatalf("查询任务日志失败: %v", err)
	}
	if filtered.Pagination.Total != 3 {
		t.Errorf("失败日志数 = %d，期望 3", filtered.Pagination.Total)
	}
}

// gcron 真实触发一次间隔任务，覆盖注册、后台 Context 与执行链路
func TestSchedulerRunsIntervalTaskEndToEnd(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"jobId": "job-e2e", "name": "每秒执行一次", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test(1,2)"})

	if err := current.scheduler.Load(t.Context()); err != nil {
		t.Fatalf("装载任务失败: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if current.logCount(t, taskID) > 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("间隔任务未在 5 秒内触发")
}

// 定时器条目复用同名会在每次执行后多出一个条目，任务频率随执行次数翻倍
func TestSchedulerDoesNotDuplicateEntries(t *testing.T) {
	current := newHarness(t)
	taskID := current.seedTask(t, g.Map{"jobId": "job-repeat", "name": "每秒执行一次", "status": entity.StatusRunning,
		"taskType": entity.TaskTypeInterval, "every": 1000, "service": "taskDemoService.test()"})

	if err := current.scheduler.Load(t.Context()); err != nil {
		t.Fatalf("装载任务失败: %v", err)
	}
	// 每轮都在任务运行期间重新启动，覆盖"旧条目收尾时注销新条目"这条路径
	for range 4 {
		time.Sleep(1100 * time.Millisecond)
		if err := current.info.Start(t.Context(), &dto.StartRequest{ID: taskID}); err != nil {
			t.Fatalf("开始任务失败: %v", err)
		}
	}
	time.Sleep(time.Second)
	if got := current.logCount(t, taskID); got > 7 {
		t.Errorf("约 5.4 秒内执行 %d 次，每秒一次的任务出现了重复条目", got)
	} else if got < 3 {
		t.Errorf("约 5.4 秒内只执行 %d 次，任务被误注销", got)
	}
}
