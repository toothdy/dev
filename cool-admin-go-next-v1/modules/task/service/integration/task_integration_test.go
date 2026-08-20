package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/toothdy/cool-admin-go-next/cool/rest/crud"
	"github.com/toothdy/cool-admin-go-next/cool/db/schema"
	"github.com/toothdy/cool-admin-go-next/cool/entity"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
	taskEntity "github.com/toothdy/cool-admin-go-next/modules/task/entity"
	taskEvent "github.com/toothdy/cool-admin-go-next/modules/task/event"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
	taskService "github.com/toothdy/cool-admin-go-next/modules/task/service"
	taskDTO "github.com/toothdy/cool-admin-go-next/modules/task/dto"
)

type integrationScheduler struct {
	upsertCalls atomic.Int32
	removeErr   error
	removeCalls atomic.Int32
}

func (s *integrationScheduler) Start(context.Context) error   { return nil }
func (s *integrationScheduler) Healthy(context.Context) error { return nil }

func (s *integrationScheduler) Upsert(context.Context, task.Schedule) (time.Time, error) {
	s.upsertCalls.Add(1)
	return time.Now(), nil
}
func (s *integrationScheduler) Remove(context.Context, string) error {
	s.removeCalls.Add(1)
	return s.removeErr
}
func (s *integrationScheduler) Enqueue(context.Context, task.Message) error { return nil }
func (s *integrationScheduler) NextRunTime(string) (time.Time, bool) {
	return time.Time{}, false
}
func (s *integrationScheduler) Stop(context.Context) error { return nil }

type unhealthyInfoEngine struct {
	cause error
}

func (e unhealthyInfoEngine) Healthy(context.Context) error                    { return e.cause }
func (e unhealthyInfoEngine) SyncTask(context.Context, int64) error            { return e.cause }
func (e unhealthyInfoEngine) RemoveTask(context.Context, int64) error          { return e.cause }
func (e unhealthyInfoEngine) Once(context.Context, taskService.TaskInfo) error { return e.cause }

func TestTaskMySQLTenantLogsAndAtomicLease(t *testing.T) {
	if os.Getenv("COOL_TASK_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_INTEGRATION=1 to run Task MySQL integration tests")
	}
	ctx := context.Background()
	db := g.DB()
	models := []entity.Definition{taskEntity.TaskInfo(), taskEntity.TaskLog()}
	if _, err := schema.NewSyncer(db).Sync(ctx, models); err != nil {
		t.Fatalf("sync Task schema failed: %v", err)
	}
	modelByTable := map[string]entity.Definition{}
	for _, definition := range models {
		modelByTable[definition.TableName] = definition
	}
	store, err := taskService.BuildStore(db, modelByTable["task_info"], modelByTable["task_log"])
	if err != nil {
		t.Fatal(err)
	}
	tenantA, err := tenant.ForTenant(ctx, 910001)
	if err != nil {
		t.Fatal(err)
	}
	tenantB, err := tenant.ForTenant(ctx, 910002)
	if err != nil {
		t.Fatal(err)
	}
	firstID := insertIntegrationTask(t, store, tenantA, "tenant-a")
	secondID := insertIntegrationTask(t, store, tenantB, "tenant-b")
	t.Cleanup(func() {
		_ = store.Delete(tenant.WithoutTenant(context.Background()), []int64{firstID, secondID})
	})

	if _, exists, findErr := store.Find(tenantA, secondID); findErr != nil || exists {
		t.Fatalf("tenant A must not see tenant B task: exists=%v err=%v", exists, findErr)
	}
	info, exists, err := store.Find(tenantA, firstID)
	if err != nil || !exists {
		t.Fatalf("read tenant A task failed: exists=%v err=%v", exists, err)
	}
	if err = store.WriteLog(ctx, info, 1, "integration"); err != nil {
		t.Fatalf("write tenant log failed: %v", err)
	}
	pageA, err := store.LogPage(tenantA, firstID, nil, 1, 20)
	if err != nil || pageA["pagination"].(map[string]interface{})["total"].(int) != 1 {
		t.Fatalf("tenant A log page failed: %#v err=%v", pageA, err)
	}
	pageB, err := store.LogPage(tenantB, firstID, nil, 1, 20)
	if err != nil || pageB["pagination"].(map[string]interface{})["total"].(int) != 0 {
		t.Fatalf("tenant B must not see tenant A logs: %#v err=%v", pageB, err)
	}

	payload := taskQueue.Payload{
		TaskID: info.ID, JobID: info.JobID, ScheduledAt: time.Now().Truncate(time.Second),
		ExecutionID: uuid.NewString(),
	}
	results := make(chan bool, 2)
	errorsChannel := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, claimed, claimErr := store.Claim(ctx, info, payload, uuid.NewString(), time.Minute)
			results <- claimed
			errorsChannel <- claimErr
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	claimedCount := 0
	for claimed := range results {
		if claimed {
			claimedCount++
		}
	}
	for claimErr := range errorsChannel {
		if claimErr != nil {
			t.Fatalf("claim task failed: %v", claimErr)
		}
	}
	if claimedCount != 1 {
		t.Fatalf("expected one lease owner, got %d", claimedCount)
	}
	claimedInfo, exists, err := store.Find(tenantA, firstID)
	if err != nil || !exists || claimedInfo.LockOwner == "" || claimedInfo.LockOwner == payload.ExecutionID {
		t.Fatalf("claim did not persist an independent token: info=%#v exists=%v err=%v", claimedInfo, exists, err)
	}

	oldInfo := info
	newJobID := uuid.NewString()
	if err = store.Update(tenantA, firstID, taskService.TaskInfoDO{JobID: newJobID, RepeatConf: "new-generation"}); err != nil {
		t.Fatalf("replace task generation failed: %v", err)
	}
	if err = store.SaveRepeatState(ctx, oldInfo, taskService.RepeatState{Version: 1, Generation: oldInfo.JobID, Count: 9}, true); err != nil {
		t.Fatalf("save stale repeat state failed: %v", err)
	}
	current, exists, err := store.Find(tenantA, firstID)
	if err != nil || !exists {
		t.Fatalf("read current task generation failed: exists=%v err=%v", exists, err)
	}
	if current.JobID != newJobID || current.RepeatConf != "new-generation" || current.Status != 1 {
		t.Fatalf("stale generation overwrote current task: %#v", current)
	}
}

func TestTaskLocalRuntimeEndToEnd(t *testing.T) {
	if os.Getenv("COOL_TASK_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_INTEGRATION=1 to run Task MySQL integration tests")
	}
	ctx := context.Background()
	db := g.DB()
	models := []entity.Definition{taskEntity.TaskInfo(), taskEntity.TaskLog()}
	if _, err := schema.NewSyncer(db).Sync(ctx, models); err != nil {
		t.Fatalf("sync Task schema failed: %v", err)
	}
	cleanupTaskLocalRuntimeFixtures(t, ctx)
	t.Cleanup(func() { cleanupTaskLocalRuntimeFixtures(t, context.Background()) })
	executed := make(chan task.Invocation, 1)
	runtime, err := taskEvent.NewComm(db, models[0], models[1], []task.HandlerDefinition{
		{
			Name: "taskIntegrationService.run",
			Handler: func(_ context.Context, invocation task.Invocation) (interface{}, error) {
				select {
				case executed <- invocation:
				default:
				}
				return map[string]string{"result": "ok"}, nil
			},
		},
	}, taskModule.Config{
		Mode: taskModule.ModeLocal, Timezone: "Local", Log: taskModule.LogConfig{KeepDays: 20},
		Execution: taskModule.ExecutionConfig{Timeout: time.Minute, LockTTL: 2 * time.Minute},
		Queue: taskModule.QueueConfig{
			Concurrency: 1, MaxRetry: 0, RetryDelay: 10 * time.Millisecond, ShutdownTimeout: 3 * time.Second,
		},
	}, time.Local, taskQueue.RedisClient{}, nil)
	if err != nil {
		t.Fatalf("create Task runtime failed: %v", err)
	}
	if err = runtime.Start(ctx); err != nil {
		t.Fatalf("start Task runtime failed: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Stop(context.Background()) })
	tenantContext, err := tenant.ForTenant(ctx, 910003)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Info().Add(tenantContext, crud.AddRequest{Data: map[string]interface{}{
		"name": "task-runtime-integration-" + uuid.NewString(), "taskType": 1, "every": 1000,
		"service": "taskIntegrationService.run({\"value\": 1})", "data": "integration",
	}})
	if err != nil {
		t.Fatalf("add Task through service failed: %v", err)
	}
	id := result.(map[string]interface{})["id"].(int64)
	t.Cleanup(func() {
		_, _ = runtime.Info().Delete(tenantContext, crud.DeleteRequest{IDs: []interface{}{id}})
	})
	var invocation task.Invocation
	select {
	case invocation = <-executed:
		if invocation.TaskID != id || invocation.Data != "integration" || len(invocation.Arguments) != 1 {
			t.Fatalf("unexpected invocation: %#v", invocation)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("timed out waiting for local scheduled task")
	}
	modelByTable := map[string]entity.Definition{}
	for _, definition := range models {
		modelByTable[definition.TableName] = definition
	}
	store, err := taskService.BuildStore(db, modelByTable["task_info"], modelByTable["task_log"])
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		page, pageErr := runtime.Info().Log(tenantContext, taskDTO.InfoLogRequest{ID: id, Page: 1, Size: 20})
		info, exists, findErr := store.Find(tenantContext, id)
		if pageErr == nil && page["pagination"].(map[string]interface{})["total"].(int) >= 1 &&
			findErr == nil && exists && info.NextRunTime != nil && info.NextRunTime.Time.After(invocation.ScheduledAt) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected runtime dispatch to write a log and advance nextRunTime")
}

func cleanupTaskLocalRuntimeFixtures(t *testing.T, ctx context.Context) {
	t.Helper()
	var records []struct {
		ID int64 `orm:"id"`
	}
	if err := g.DB().Model("task_info").Ctx(ctx).Fields("id").
		WhereLike("service", "taskIntegrationService.run%").Scan(&records); err != nil {
		t.Fatalf("read stale Task runtime fixtures failed: %v", err)
	}
	if len(records) == 0 {
		return
	}
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	if _, err := g.DB().Model("task_log").Ctx(ctx).WhereIn("taskId", ids).Delete(); err != nil {
		t.Fatalf("cleanup stale Task runtime logs failed: %v", err)
	}
	if _, err := g.DB().Model("task_info").Ctx(ctx).WhereIn("id", ids).Delete(); err != nil {
		t.Fatalf("cleanup stale Task runtime fixtures failed: %v", err)
	}
}

func TestTaskRedisRetryRecoversLimitedBatchWithoutDoubleAttempts(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910004)
	jobID := uuid.NewString()
	scheduledAt := time.Now().Truncate(time.Second).Add(500 * time.Millisecond)
	limit := 1
	invocations := make([]int, 0, 3)
	invocationTimes := make([]time.Time, 0, 3)
	var invocationMu sync.Mutex
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskRetryService.run",
		Handler: func(_ context.Context, invocation task.Invocation) (interface{}, error) {
			invocationMu.Lock()
			invocations = append(invocations, invocation.Attempt)
			invocationTimes = append(invocationTimes, invocation.ScheduledAt)
			invocationMu.Unlock()
			if invocation.Attempt < 2 {
				return nil, errors.New("retry")
			}
			return "ok", nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 2, 20*time.Millisecond, 2*time.Second)
	scheduler := &integrationScheduler{removeErr: errors.New("scheduler unavailable")}
	engine, err := taskService.BuildEngine(store, registry, scheduler, executor, time.UTC, "redis", 20)
	if err != nil {
		t.Fatal(err)
	}
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskRetryService.run()", &limit)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })

	payload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: scheduledAt,
		ExecutionID: uuid.NewString(), IsRetryManaged: true,
	}
	for attempt := 0; attempt < 3; attempt++ {
		payload.Attempt = attempt
		dispatchErr := engine.Dispatch(context.Background(), payload)
		if attempt < 2 && dispatchErr == nil {
			t.Fatalf("attempt %d must remain retryable", attempt)
		}
		if attempt == 2 && dispatchErr != nil {
			t.Fatalf("successful business attempt must ignore schedule removal error: %v", dispatchErr)
		}
		if attempt == 0 {
			if duplicateErr := engine.Dispatch(context.Background(), payload); duplicateErr != nil {
				t.Fatalf("duplicate initial delivery must be ignored: %v", duplicateErr)
			}
		}
	}
	invocationMu.Lock()
	defer invocationMu.Unlock()
	if len(invocations) != 3 || invocations[0] != 0 || invocations[1] != 1 || invocations[2] != 2 {
		t.Fatalf("unexpected retry attempts: %v", invocations)
	}
	for _, receivedAt := range invocationTimes {
		if !receivedAt.Equal(scheduledAt) {
			t.Fatalf("handler did not receive original millisecond schedule time: got=%v want=%v", receivedAt, scheduledAt)
		}
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("read retried task failed: exists=%v err=%v", exists, err)
	}
	var state taskService.RepeatState
	if err = json.Unmarshal([]byte(info.RepeatConf), &state); err != nil {
		t.Fatal(err)
	}
	if info.LastExecuteTime == nil || !info.LastExecuteTime.Time.Equal(scheduledAt.Truncate(time.Second)) {
		t.Fatalf("lastExecuteTime must use MySQL second precision: %#v", info.LastExecuteTime)
	}
	if state.Count != 1 || info.Status != 0 || scheduler.removeCalls.Load() != 4 {
		t.Fatalf("limited retry changed periodic state: state=%#v status=%d removes=%d", state, info.Status, scheduler.removeCalls.Load())
	}
}

func TestTaskScheduledRetryBusyHasNoPeriodicOrLogSideEffects(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910012)
	jobID := uuid.NewString()
	scheduledAt := time.Now().Truncate(time.Second)
	limit := 3
	repeatConf := `{"version":1,"generation":"` + jobID + `","count":1}`
	var handlerCalls atomic.Int32
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskScheduledBusyService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			handlerCalls.Add(1)
			return nil, nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 2, time.Second, 2*time.Second)
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskScheduledBusyService.run()", &limit)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	if err := store.Update(tenantContext, id, taskService.TaskInfoDO{
		RepeatConf: repeatConf, LastExecuteTime: scheduledAt,
		LockOwner: uuid.NewString(), LockExpireTime: time.Now().Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	payload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: scheduledAt,
		ExecutionID: uuid.NewString(), Attempt: 1, IsRetryManaged: true,
	}
	executeErr := executor.Execute(context.Background(), payload)
	message, _, isBusy := task.BusyRedelivery(executeErr)
	if !isBusy || !errors.Is(executeErr, taskService.ErrTaskBusy) {
		t.Fatalf("周期恢复 busy 未返回类型化重投: %v", executeErr)
	}
	redelivery, err := taskQueue.Decode(message)
	if err != nil {
		t.Fatal(err)
	}
	if redelivery.ExecutionID != payload.ExecutionID || redelivery.Attempt != payload.Attempt {
		t.Fatalf("周期 busy 重投改变了业务身份: %#v", redelivery)
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("读取 busy 任务失败: exists=%v err=%v", exists, err)
	}
	logs, err := store.LogPage(tenantContext, id, nil, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	logTotal := logs["pagination"].(map[string]interface{})["total"].(int)
	if handlerCalls.Load() != 0 || logTotal != 0 || info.Limit == nil || *info.Limit != limit ||
		info.RepeatConf != repeatConf || info.LastExecuteTime == nil || !info.LastExecuteTime.Time.Equal(scheduledAt) {
		t.Fatalf("周期 busy 窗口产生了副作用: calls=%d logs=%d info=%#v", handlerCalls.Load(), logTotal, info)
	}
}

func TestTaskDuplicateInitialMillisecondBatchDoesNotIncrementRepeatCount(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910010)
	jobID := uuid.NewString()
	scheduledAt := time.Now().Truncate(time.Second).Add(500 * time.Millisecond)
	var calls atomic.Int32
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskDuplicateService.run",
		Handler: func(_ context.Context, invocation task.Invocation) (interface{}, error) {
			calls.Add(1)
			if !invocation.ScheduledAt.Equal(scheduledAt) {
				t.Errorf("handler did not receive original millisecond schedule time: got=%v want=%v", invocation.ScheduledAt, scheduledAt)
			}
			return "ok", nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 0, 20*time.Millisecond, 2*time.Second)
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskDuplicateService.run()", nil)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	payload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: scheduledAt,
		ExecutionID: uuid.NewString(), IsRetryManaged: true,
	}
	if err := executor.Execute(context.Background(), payload); err != nil {
		t.Fatalf("initial delivery failed: %v", err)
	}
	if err := executor.Execute(context.Background(), payload); err != nil {
		t.Fatalf("duplicate initial delivery failed: %v", err)
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("read duplicate task failed: exists=%v err=%v", exists, err)
	}
	var state taskService.RepeatState
	if err = json.Unmarshal([]byte(info.RepeatConf), &state); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || state.Count != 1 {
		t.Fatalf("duplicate initial delivery was counted twice: calls=%d state=%#v", calls.Load(), state)
	}
}

func TestTaskTimeoutKeepsLeaseUntilIgnoredHandlerExits(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910005)
	jobID := uuid.NewString()
	scheduledAt := time.Now().Truncate(time.Second)
	started := make(chan struct{})
	release := make(chan struct{})
	var (
		active    atomic.Int32
		maxActive atomic.Int32
	)
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskTimeoutService.run",
		Handler: func(_ context.Context, invocation task.Invocation) (interface{}, error) {
			current := active.Add(1)
			for previous := maxActive.Load(); current > previous && !maxActive.CompareAndSwap(previous, current); previous = maxActive.Load() {
			}
			defer active.Add(-1)
			if invocation.Attempt == 0 {
				close(started)
				<-release
				return nil, errors.New("late failure")
			}
			return "ok", nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 1, 5*time.Second, 6*time.Second)
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskTimeoutService.run()", nil)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	payload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: scheduledAt,
		ExecutionID: uuid.NewString(), IsRetryManaged: true,
	}
	firstResult := make(chan error, 1)
	go func() {
		firstResult <- executor.Execute(context.Background(), payload)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ignored handler")
	}
	time.Sleep(6200 * time.Millisecond)
	select {
	case err := <-firstResult:
		t.Fatalf("timed out attempt returned before handler exited: %v", err)
	default:
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists || info.LockExpireTime == nil || !info.LockExpireTime.Time.After(time.Now()) {
		t.Fatalf("timed out handler lease was not renewed: info=%#v exists=%v err=%v", info, exists, err)
	}
	close(release)
	if err = <-firstResult; err == nil {
		t.Fatal("timed out attempt must remain retryable")
	}
	payload.Attempt = 1
	if err = executor.Execute(context.Background(), payload); err != nil {
		t.Fatalf("retry after timed out handler exited failed: %v", err)
	}
	if maxActive.Load() != 1 {
		t.Fatalf("timed out handler overlapped its retry: max active=%d", maxActive.Load())
	}
}

func TestTaskManualOncePreservesPeriodicState(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910006)
	jobID := uuid.NewString()
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskOnceService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			return "ok", nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 0, 20*time.Millisecond, 2*time.Second)
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskOnceService.run()", nil)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	previousRun := time.Now().Add(-time.Hour).Truncate(time.Second)
	nextRun := time.Now().Add(time.Hour).Truncate(time.Second)
	repeatConf := `{"version":1,"generation":"` + jobID + `","count":7}`
	if err := store.Update(tenantContext, id, taskService.TaskInfoDO{
		Status: 0, RepeatConf: repeatConf, LastExecuteTime: previousRun, NextRunTime: nextRun,
	}); err != nil {
		t.Fatal(err)
	}
	payload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: time.Now(), Manual: true,
		ExecutionID: uuid.NewString(), IsRetryManaged: true,
	}
	if err := executor.Execute(context.Background(), payload); err != nil {
		t.Fatalf("manual once failed: %v", err)
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("read manual task failed: exists=%v err=%v", exists, err)
	}
	if info.Status != 0 || info.RepeatConf != repeatConf || info.LastExecuteTime == nil || !info.LastExecuteTime.Time.Equal(previousRun) || info.NextRunTime == nil || !info.NextRunTime.Time.Equal(nextRun) {
		t.Fatalf("manual once changed periodic state: %#v", info)
	}
}

func TestTaskConcurrentManualOnceWaitsAndRunsBothSerially(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910008)
	jobID := uuid.NewString()
	entered := make(chan struct{}, 2)
	releaseFirst := make(chan struct{})
	var (
		calls       atomic.Int32
		active      atomic.Int32
		maxActive   atomic.Int32
		releaseOnce sync.Once
	)
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseFirst) }) })
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name: "taskConcurrentOnceService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			call := calls.Add(1)
			current := active.Add(1)
			for previous := maxActive.Load(); current > previous && !maxActive.CompareAndSwap(previous, current); previous = maxActive.Load() {
			}
			defer active.Add(-1)
			entered <- struct{}{}
			if call == 1 {
				<-releaseFirst
			}
			return "ok", nil
		},
	})
	executor := integrationTaskExecutor(t, store, registry, 0, time.Second, 3*time.Second)
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskConcurrentOnceService.run()", nil)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	previousRun := time.Now().Add(-time.Hour).Truncate(time.Second)
	nextRun := time.Now().Add(time.Hour).Truncate(time.Second)
	repeatConf := `{"version":1,"generation":"` + jobID + `","count":7}`
	if err := store.Update(tenantContext, id, taskService.TaskInfoDO{
		Status: 0, RepeatConf: repeatConf, LastExecuteTime: previousRun, NextRunTime: nextRun,
	}); err != nil {
		t.Fatal(err)
	}

	consumer, err := taskQueue.BuildConsumer(func(ctx context.Context, payload taskQueue.Payload) error {
		return executor.Execute(ctx, payload)
	})
	if err != nil {
		t.Fatal(err)
	}
	scheduler, err := task.NewLocalScheduler(2, time.UTC, consumer)
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Stop(context.Background()) })
	dispatch := func(executionID string) {
		message, encodeErr := taskQueue.Encode(taskQueue.Payload{
			TaskID: id, JobID: jobID, ScheduledAt: time.Now(), Manual: true, ExecutionID: executionID,
		})
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		if enqueueErr := scheduler.Enqueue(context.Background(), message); enqueueErr != nil {
			t.Fatal(enqueueErr)
		}
	}
	dispatch(uuid.NewString())
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first manual Once")
	}
	dispatch(uuid.NewString())
	select {
	case <-entered:
		t.Fatal("concurrent manual Once executions overlapped")
	case <-time.After(250 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(releaseFirst) })
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("second manual Once was dropped while the lease was busy")
	}
	if calls.Load() != 2 || maxActive.Load() != 1 {
		t.Fatalf("manual Once executions were not serialized: calls=%d maxActive=%d", calls.Load(), maxActive.Load())
	}
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("read manual task failed: exists=%v err=%v", exists, err)
	}
	if info.Status != 0 || info.RepeatConf != repeatConf || info.LastExecuteTime == nil || !info.LastExecuteTime.Time.Equal(previousRun) || info.NextRunTime == nil || !info.NextRunTime.Time.Equal(nextRun) {
		t.Fatalf("concurrent manual Once changed periodic state: %#v", info)
	}
}

func TestTaskLeaseTakeoverRejectsStaleOwnerRenewAndRelease(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910009)
	jobID := uuid.NewString()
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskDemoService.test()", nil)
	t.Cleanup(func() { _ = store.Delete(tenantContext, []int64{id}) })
	info, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists {
		t.Fatalf("read takeover task failed: exists=%v err=%v", exists, err)
	}
	executionID := uuid.NewString()
	oldOwner := uuid.NewString()
	oldPayload := taskQueue.Payload{
		TaskID: id, JobID: jobID, ScheduledAt: time.Now(), Manual: true, ExecutionID: executionID,
	}
	_, claimed, err := store.Claim(context.Background(), info, oldPayload, oldOwner, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("old owner failed to claim lease: claimed=%v err=%v", claimed, err)
	}
	if err = store.Update(tenantContext, id, taskService.TaskInfoDO{LockExpireTime: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Renew(context.Background(), info, oldOwner, time.Minute); !errors.Is(err, taskService.ErrTaskLeaseLost) {
		t.Fatalf("expired lease renewal must not resurrect it: %v", err)
	}
	newOwner := uuid.NewString()
	newPayload := oldPayload
	_, claimed, err = store.Claim(context.Background(), info, newPayload, newOwner, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("new owner failed to take over expired lease: claimed=%v err=%v", claimed, err)
	}
	owned, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists || owned.LockOwner != newOwner || owned.LockExpireTime == nil {
		t.Fatalf("unexpected takeover state: info=%#v exists=%v err=%v", owned, exists, err)
	}
	newExpiry := owned.LockExpireTime.Time
	if _, err = store.Renew(context.Background(), info, oldOwner, time.Minute); !errors.Is(err, taskService.ErrTaskLeaseLost) {
		t.Fatalf("stale owner renewal must lose the lease: %v", err)
	}
	if err = store.Release(context.Background(), info, oldOwner); err != nil {
		t.Fatalf("stale owner release failed: %v", err)
	}
	afterStale, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists || afterStale.LockOwner != newOwner || afterStale.LockExpireTime == nil || !afterStale.LockExpireTime.Time.Equal(newExpiry) {
		t.Fatalf("stale owner changed the new lease: info=%#v exists=%v err=%v", afterStale, exists, err)
	}
	if _, err = store.Renew(context.Background(), info, newOwner, 2*time.Minute); err != nil {
		t.Fatalf("new owner renewal failed: %v", err)
	}
	if err = store.Release(context.Background(), info, newOwner); err != nil {
		t.Fatalf("new owner release failed: %v", err)
	}
	released, exists, err := store.Find(tenantContext, id)
	if err != nil || !exists || released.LockOwner != "" || released.LockExpireTime != nil {
		t.Fatalf("new owner did not release its lease: info=%#v exists=%v err=%v", released, exists, err)
	}
}

func TestTaskDeleteRemainsAvailableWhenSchedulerIsUnhealthy(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910007)
	jobID := uuid.NewString()
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name:    "taskDeleteService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) { return nil, nil },
	})
	infoService, err := taskService.BuildInfoService(
		store, registry, unhealthyInfoEngine{cause: errors.New("redis unavailable")}, time.UTC,
	)
	if err != nil {
		t.Fatal(err)
	}
	id := insertExecutorTask(t, store, tenantContext, jobID, "taskDeleteService.run()", nil)
	if _, err = infoService.Delete(tenantContext, crud.DeleteRequest{IDs: []interface{}{id}}); err != nil {
		t.Fatalf("delete must not depend on scheduler health: %v", err)
	}
	if _, exists, findErr := store.Find(tenantContext, id); findErr != nil || exists {
		t.Fatalf("deleted task still exists: exists=%v err=%v", exists, findErr)
	}
}

func TestTaskReconcileSyncRejectsLegacyInvalidIntervals(t *testing.T) {
	store, tenantContext := integrationTaskStore(t, 910011)
	registry := integrationTaskRegistry(t, task.HandlerDefinition{
		Name:    "taskLegacyIntervalService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) { return nil, nil },
	})
	executor := integrationTaskExecutor(t, store, registry, 0, 20*time.Millisecond, 2*time.Second)
	scheduler := &integrationScheduler{}
	engine, err := taskService.BuildEngine(store, registry, scheduler, executor, time.UTC, "local", 20)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, 3)
	t.Cleanup(func() {
		if len(ids) > 0 {
			_ = store.Delete(tenantContext, ids)
		}
	})
	for _, every := range []int64{999, 100000000001, math.MaxInt64} {
		jobID := uuid.NewString()
		id, insertErr := store.Insert(tenantContext, taskService.TaskInfoDO{
			JobID: jobID, Name: "task-legacy-interval-" + uuid.NewString(), Every: every,
			Status: 1, Service: "taskLegacyIntervalService.run()", Type: 1, TaskType: 1,
		})
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		ids = append(ids, id)
		if err = engine.SyncTask(context.Background(), id); err == nil {
			t.Fatalf("Reconcile 单任务同步必须拒绝历史非法间隔: %d", every)
		}
		info, exists, findErr := store.Find(tenantContext, id)
		if findErr != nil || !exists || info.Status != 0 {
			t.Fatalf("历史非法间隔未被停止: every=%d info=%#v exists=%v err=%v", every, info, exists, findErr)
		}
	}
	if scheduler.upsertCalls.Load() != 0 {
		t.Fatalf("历史非法间隔进入了 Scheduler: upserts=%d", scheduler.upsertCalls.Load())
	}
}

func TestTaskCleanupLogsStrictBeforeBoundaryAcrossTenants(t *testing.T) {
	const expiredLogCount = 1001

	store, tenantContext := integrationTaskStore(t, 910013)
	otherTenantContext, err := tenant.ForTenant(context.Background(), 910014)
	if err != nil {
		t.Fatal(err)
	}
	taskID := insertIntegrationTask(t, store, tenantContext, "cleanup-boundary")
	otherTaskID := insertIntegrationTask(t, store, otherTenantContext, "cleanup-boundary-other")
	t.Cleanup(func() {
		_ = store.Delete(tenantContext, []int64{taskID})
		_ = store.Delete(otherTenantContext, []int64{otherTaskID})
	})

	before := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	oldDetail := "cleanup-old-" + uuid.NewString()
	equalDetail := "cleanup-equal-" + uuid.NewString()
	newDetail := "cleanup-new-" + uuid.NewString()
	otherOldDetail := "cleanup-other-old-" + uuid.NewString()
	type cleanupLogFixture struct {
		CreateTime time.Time `orm:"createTime"`
		UpdateTime time.Time `orm:"updateTime"`
		TenantID   int64     `orm:"tenantId"`
		TaskID     int64     `orm:"taskId"`
		Status     int       `orm:"status"`
		Detail     string    `orm:"detail"`
	}
	fixtures := make([]cleanupLogFixture, 0, expiredLogCount+3)
	for range expiredLogCount {
		fixtures = append(fixtures, cleanupLogFixture{
			CreateTime: before.Add(-time.Second), UpdateTime: before.Add(-time.Second),
			TenantID: 910013, TaskID: taskID, Status: 1, Detail: oldDetail,
		})
	}
	fixtures = append(fixtures,
		cleanupLogFixture{CreateTime: before, UpdateTime: before, TenantID: 910013, TaskID: taskID, Status: 1, Detail: equalDetail},
		cleanupLogFixture{CreateTime: before.Add(time.Second), UpdateTime: before.Add(time.Second), TenantID: 910013, TaskID: taskID, Status: 1, Detail: newDetail},
		cleanupLogFixture{CreateTime: before.Add(-time.Second), UpdateTime: before.Add(-time.Second), TenantID: 910014, TaskID: otherTaskID, Status: 1, Detail: otherOldDetail},
	)
	if _, err = g.DB().Insert(context.Background(), "task_log", fixtures, 500); err != nil {
		t.Fatalf("insert Task cleanup fixtures failed: %v", err)
	}
	if err = store.CleanupLogs(tenantContext, before); err != nil {
		t.Fatalf("cleanup Task logs failed: %v", err)
	}
	for _, expectation := range []struct {
		detail string
		count  int
	}{
		{detail: oldDetail, count: 0},
		{detail: otherOldDetail, count: 0},
		{detail: equalDetail, count: 1},
		{detail: newDetail, count: 1},
	} {
		count, countErr := g.DB().Model("task_log").Ctx(context.Background()).Where("detail", expectation.detail).Count()
		if countErr != nil || count != expectation.count {
			t.Fatalf("unexpected Task cleanup boundary result: detail=%s count=%d want=%d err=%v", expectation.detail, count, expectation.count, countErr)
		}
	}
}

func insertIntegrationTask(t *testing.T, store *taskService.Store, ctx context.Context, suffix string) int64 {
	t.Helper()
	id, err := store.Insert(ctx, taskService.TaskInfoDO{
		JobID: uuid.NewString(), Name: "task-integration-" + suffix + "-" + uuid.NewString(),
		Status: 1, Service: "taskDemoService.test()", Type: 1, TaskType: 0, Cron: "0 * * * * *",
	})
	if err != nil {
		t.Fatalf("insert integration task failed: %v", err)
	}
	return id
}

func integrationTaskStore(t *testing.T, tenantID int64) (*taskService.Store, context.Context) {
	t.Helper()
	if os.Getenv("COOL_TASK_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_INTEGRATION=1 to run Task MySQL integration tests")
	}
	ctx := context.Background()
	db := g.DB()
	models := []entity.Definition{taskEntity.TaskInfo(), taskEntity.TaskLog()}
	if _, err := schema.NewSyncer(db).Sync(ctx, models); err != nil {
		t.Fatalf("sync Task schema failed: %v", err)
	}
	modelByTable := map[string]entity.Definition{}
	for _, definition := range models {
		modelByTable[definition.TableName] = definition
	}
	store, err := taskService.BuildStore(db, modelByTable["task_info"], modelByTable["task_log"])
	if err != nil {
		t.Fatal(err)
	}
	tenantContext, err := tenant.ForTenant(ctx, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	return store, tenantContext
}

func integrationTaskRegistry(t *testing.T, definition task.HandlerDefinition) *task.Registry {
	t.Helper()
	builder := task.NewRegistryBuilder()
	if err := builder.Register(definition); err != nil {
		t.Fatal(err)
	}
	registry, err := builder.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func integrationTaskExecutor(t *testing.T, store *taskService.Store, registry *task.Registry, maxRetry int, timeout time.Duration, lockTTL time.Duration) *taskService.Executor {
	t.Helper()
	executor, err := taskService.BuildExecutor(store, registry, taskService.ExecutorConfig{
		Timeout: timeout, LockTTL: lockTTL, MaxRetry: maxRetry, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func insertExecutorTask(t *testing.T, store *taskService.Store, ctx context.Context, jobID string, service string, limit *int) int64 {
	t.Helper()
	repeatConf, err := json.Marshal(taskService.RepeatState{Version: 1, Generation: jobID})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.Insert(ctx, taskService.TaskInfoDO{
		JobID: jobID, RepeatConf: string(repeatConf), Name: "task-executor-" + uuid.NewString(),
		Cron: "0 * * * * *", Limit: limit, Status: 1, Service: service, Type: 1, TaskType: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
