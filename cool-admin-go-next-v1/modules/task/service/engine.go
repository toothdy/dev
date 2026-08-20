package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/google/uuid"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

// TaskEngine 以 MySQL 权威状态对账通用调度后端。
type TaskEngine struct {
	mu          sync.Mutex
	syncMu      sync.Mutex
	knownMu     sync.Mutex
	store       *Store
	registry    *task.Registry
	scheduler   task.Scheduler
	executor    *Executor
	location    *time.Location
	backend     string
	keepDays    int
	stopContext context.Context
	stop        context.CancelFunc
	workers     sync.WaitGroup
	isStarted   bool
	knownTasks  map[int64]struct{}
}

// BuildEngine 创建 Task 状态对账引擎。
func BuildEngine(store *Store, registry *task.Registry, scheduler task.Scheduler, executor *Executor, location *time.Location, backend string, keepDays int) (*TaskEngine, error) {
	if store == nil || registry == nil || scheduler == nil || executor == nil || location == nil {
		return nil, gerror.New("Task Engine 依赖不完整")
	}
	if keepDays <= 0 {
		return nil, gerror.New("Task 日志保留天数必须大于 0")
	}
	stopContext, stop := context.WithCancel(context.Background())
	return &TaskEngine{
		store: store, registry: registry, scheduler: scheduler, executor: executor,
		location: location, backend: backend, keepDays: keepDays,
		stopContext: stopContext, stop: stop, knownTasks: map[int64]struct{}{},
	}, nil
}

// Start 启动调度后端、初始对账和后台维护。
func (e *TaskEngine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.isStarted {
		return nil
	}
	if err := e.scheduler.Start(ctx); err != nil {
		_ = e.scheduler.Stop(context.Background())
		return err
	}
	if err := e.Reconcile(ctx); err != nil {
		_ = e.scheduler.Stop(context.Background())
		return err
	}
	e.isStarted = true
	e.workers.Add(1)
	go e.maintenance()
	return nil
}

// Stop 停止后台维护和调度后端。
func (e *TaskEngine) Stop(ctx context.Context) error {
	e.mu.Lock()
	if !e.isStarted {
		e.mu.Unlock()
		return nil
	}
	e.stop()
	e.isStarted = false
	e.mu.Unlock()
	e.workers.Wait()
	return e.scheduler.Stop(ctx)
}

// Healthy 检查调度后端能否接收写操作。
func (e *TaskEngine) Healthy(ctx context.Context) error {
	return e.scheduler.Healthy(ctx)
}

// Reconcile 以 MySQL 启用任务为准同步计划。
func (e *TaskEngine) Reconcile(ctx context.Context) error {
	items, err := e.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	enabled := make(map[int64]struct{}, len(items))
	for _, info := range items {
		enabled[info.ID] = struct{}{}
		if err = e.syncTask(ctx, info.ID); err != nil {
			g.Log().Errorf(ctx, "同步任务 %d 失败: %v", info.ID, err)
		}
	}
	e.knownMu.Lock()
	staleTaskIDs := make([]int64, 0)
	for taskID := range e.knownTasks {
		if _, exists := enabled[taskID]; !exists {
			staleTaskIDs = append(staleTaskIDs, taskID)
		}
	}
	e.knownMu.Unlock()
	for _, taskID := range staleTaskIDs {
		if err = e.RemoveTask(ctx, taskID); err != nil {
			g.Log().Errorf(ctx, "移除任务 %d 计划失败: %v", taskID, err)
		}
	}
	return nil
}

// SyncTask 同步单个任务定义。
func (e *TaskEngine) SyncTask(ctx context.Context, taskID int64) error {
	return e.syncTask(ctx, taskID)
}

// RemoveTask 删除运行计划。
func (e *TaskEngine) RemoveTask(ctx context.Context, taskID int64) error {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	return e.removeSchedule(ctx, taskID)
}

// Once 提交一次独立的手动执行。
func (e *TaskEngine) Once(ctx context.Context, info TaskInfo) error {
	message, err := taskQueue.Encode(taskQueue.Payload{
		TaskID: info.ID, JobID: info.JobID, TenantID: cloneTaskTenantID(info.TenantID),
		ScheduledAt: time.Now().In(e.location), Manual: true, ExecutionID: uuid.NewString(),
	})
	if err != nil {
		return err
	}
	return e.scheduler.Enqueue(ctx, message)
}

// Dispatch 将 Task Payload 交给统一 Executor。
func (e *TaskEngine) Dispatch(ctx context.Context, payload taskQueue.Payload) error {
	executeErr := e.executor.Execute(ctx, payload)
	if payload.Manual {
		return executeErr
	}
	if refreshErr := e.refreshNextRun(ctx, payload); refreshErr != nil {
		g.Log().Errorf(ctx, "刷新任务 %d 下次执行时间失败: %v", payload.TaskID, refreshErr)
	}
	return executeErr
}

// refreshNextRun 刷新周期任务的下次执行时间并停止已结束计划。
func (e *TaskEngine) refreshNextRun(ctx context.Context, payload taskQueue.Payload) error {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	info, exists, err := e.store.FindInternal(ctx, payload.TaskID)
	if err != nil || !exists || info.JobID != payload.JobID {
		return err
	}
	if info.Status != 1 {
		return e.removeSchedule(ctx, info.ID)
	}
	after := time.Now().In(e.location)
	if after.Before(payload.ScheduledAt) {
		after = payload.ScheduledAt
	}
	schedule, err := scheduleForInfo(info, e.location)
	if err != nil {
		return err
	}
	next, err := task.CalculateNextRun(schedule, after, e.location)
	if err != nil {
		if !errors.Is(err, task.ErrScheduleExpired) {
			return err
		}
		updated, updateErr := e.store.UpdateRuntimeForJob(ctx, info.ID, info.JobID, TaskInfoDO{
			Status: 0, NextRunTime: gdb.Raw("NULL"),
		})
		if updateErr != nil || !updated {
			return updateErr
		}
		return e.removeSchedule(ctx, info.ID)
	}
	_, err = e.store.UpdateRuntimeForJob(ctx, info.ID, info.JobID, TaskInfoDO{NextRunTime: next})
	return err
}

func (e *TaskEngine) removeSchedule(ctx context.Context, taskID int64) error {
	if err := e.scheduler.Remove(ctx, scheduleID(taskID)); err != nil {
		return err
	}
	e.knownMu.Lock()
	delete(e.knownTasks, taskID)
	e.knownMu.Unlock()
	return nil
}

func (e *TaskEngine) syncTask(ctx context.Context, taskID int64) error {
	e.syncMu.Lock()
	defer e.syncMu.Unlock()
	info, exists, err := e.store.FindInternal(ctx, taskID)
	if err != nil {
		return err
	}
	if !exists || info.Status != 1 {
		return e.removeSchedule(ctx, taskID)
	}
	return e.syncInfo(ctx, info)
}

func (e *TaskEngine) syncInfo(ctx context.Context, info TaskInfo) error {
	if err := validateStoredSchedule(info); err != nil {
		return e.disableInvalidTask(ctx, info, err)
	}
	expression, err := task.ParseExpression(info.Service)
	if err != nil {
		return e.disableInvalidTask(ctx, info, err)
	}
	if _, isFound := e.registry.Find(expression.Key); !isFound {
		return e.disableInvalidTask(ctx, info, gerror.Newf("任务处理器 %s 未注册", expression.Key))
	}
	schedule, err := scheduleForInfo(info, e.location)
	if err != nil {
		return e.disableInvalidTask(ctx, info, err)
	}
	next, err := e.scheduler.Upsert(ctx, schedule)
	if err != nil {
		return err
	}
	state := repeatState(info)
	state.Mode = e.backend
	state.Generation = info.JobID
	encoded, err := json.Marshal(state)
	if err != nil {
		return gerror.Wrap(err, "编码任务重复状态失败")
	}
	updated, err := e.store.UpdateRuntimeForJob(ctx, info.ID, info.JobID, TaskInfoDO{
		RepeatConf: string(encoded), NextRunTime: next,
	})
	if err != nil {
		return err
	}
	if !updated {
		return e.removeSchedule(ctx, info.ID)
	}
	e.knownMu.Lock()
	e.knownTasks[info.ID] = struct{}{}
	e.knownMu.Unlock()
	return nil
}

// validateStoredSchedule 阻止历史非法计划重新进入调度器。
func validateStoredSchedule(info TaskInfo) error {
	if info.TaskType == 1 {
		if info.Every == nil {
			return gerror.New("间隔任务必须填写 every")
		}
		_, err := taskEveryDuration(*info.Every)
		return err
	}
	return nil
}

func (e *TaskEngine) disableInvalidTask(ctx context.Context, info TaskInfo, cause error) error {
	updated, err := e.store.UpdateRuntimeForJob(ctx, info.ID, info.JobID, TaskInfoDO{
		Status: 0, NextRunTime: gdb.Raw("NULL"),
	})
	if err != nil {
		return err
	}
	if !updated {
		return nil
	}
	if err = e.store.WriteLog(ctx, info, 0, truncateDetail(cause.Error())); err != nil {
		return err
	}
	_ = e.removeSchedule(ctx, info.ID)
	return cause
}

func (e *TaskEngine) maintenance() {
	defer e.workers.Done()
	reconcileTicker := time.NewTicker(30 * time.Second)
	cleanupTicker := time.NewTicker(24 * time.Hour)
	defer reconcileTicker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-e.stopContext.Done():
			return
		case <-reconcileTicker.C:
			if err := e.Reconcile(e.stopContext); err != nil {
				g.Log().Error(e.stopContext, err)
			}
		case <-cleanupTicker.C:
			before := time.Now().AddDate(0, 0, -e.keepDays)
			if err := e.store.CleanupLogs(e.stopContext, before); err != nil {
				g.Log().Error(e.stopContext, err)
			}
		}
	}
}

func scheduleForInfo(info TaskInfo, location *time.Location) (task.Schedule, error) {
	anchor := time.Now().In(location)
	if parsed, err := parseTaskStoredTime(info.CreateTime, location); err == nil {
		anchor = parsed
	}
	var every time.Duration
	if info.Every != nil {
		converted, err := taskEveryDuration(*info.Every)
		if err != nil {
			return task.Schedule{}, err
		}
		every = converted
	}
	return task.Schedule{
		ID: scheduleID(info.ID), Cron: info.Cron, Every: every, Anchor: anchor,
		StartDate: taskTimeValue(info.StartDate), EndDate: taskTimeValue(info.EndDate),
		Message: taskQueue.MessageFactory(taskQueue.Payload{
			TaskID: info.ID, JobID: info.JobID, TenantID: cloneTaskTenantID(info.TenantID),
		}),
	}, nil
}

func scheduleID(taskID int64) string {
	return strconv.FormatInt(taskID, 10)
}

func taskTimeValue(value *gtime.Time) *time.Time {
	if value == nil {
		return nil
	}
	result := value.Time
	return &result
}

func cloneTaskTenantID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func parseTaskStoredTime(value string, location *time.Location) (time.Time, error) {
	formats := []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	var err error
	for _, format := range formats {
		var parsed time.Time
		if format == time.RFC3339Nano {
			parsed, err = time.Parse(format, value)
		} else {
			parsed, err = time.ParseInLocation(format, value, location)
		}
		if err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, err
}
