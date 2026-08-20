package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/google/uuid"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

const (
	maxLogDetailLength          = 4000
	manualClaimWait             = 50 * time.Millisecond
	manualClaimPollInterval     = 10 * time.Millisecond
	busyRedeliveryDelay         = 100 * time.Millisecond
	maxLeaseRenewFailureBackoff = 100 * time.Millisecond
)

// ErrTaskBusy 表示当前执行暂时无法领取任务租约。
var ErrTaskBusy = errors.New("任务执行租约繁忙")

// ExecutorConfig 描述任务执行的超时、租约与重试配置。
type ExecutorConfig struct {
	Timeout    time.Duration
	LockTTL    time.Duration
	MaxRetry   int
	RetryDelay time.Duration
}

// Executor 统一执行本地和 Redis 后端投递的 Task Payload。
type Executor struct {
	store    executorStore
	registry *task.Registry
	config   ExecutorConfig
}

type executorStore interface {
	FindInternal(ctx context.Context, id int64) (TaskInfo, bool, error)
	Claim(ctx context.Context, info TaskInfo, payload taskQueue.Payload, claimToken string, lockTTL time.Duration) (TaskLease, bool, error)
	Renew(ctx context.Context, info TaskInfo, claimToken string, lockTTL time.Duration) (time.Time, error)
	Release(ctx context.Context, info TaskInfo, claimToken string) error
	WriteLog(ctx context.Context, info TaskInfo, status int, detail string) error
}

// BuildExecutor 创建统一任务执行器。
func BuildExecutor(store *Store, registry *task.Registry, config ExecutorConfig) (*Executor, error) {
	if store == nil || registry == nil {
		return nil, gerror.New("任务执行器缺少 Store 或处理器注册表")
	}
	if config.Timeout <= 0 || config.LockTTL-config.Timeout < time.Second || config.MaxRetry < 0 || config.RetryDelay <= 0 {
		return nil, gerror.New("任务执行器配置无效")
	}
	return &Executor{store: store, registry: registry, config: config}, nil
}

// Execute 校验、领取并执行一个任务批次。
func (e *Executor) Execute(ctx context.Context, payload taskQueue.Payload) error {
	info, exists, err := e.store.FindInternal(ctx, payload.TaskID)
	if err != nil || !exists {
		return err
	}
	if info.JobID != payload.JobID {
		return nil
	}
	if !payload.Manual {
		if payload.Attempt == 0 && !canRunScheduled(info, payload.ScheduledAt) {
			return nil
		}
		if payload.Attempt > 0 && !canResumeScheduled(info, payload.ScheduledAt) {
			return nil
		}
	}
	expression, err := task.ParseExpression(info.Service)
	if err != nil {
		return task.Permanent(err)
	}
	definition, isFound := e.registry.Find(expression.Key)
	if !isFound {
		return task.Permanent(gerror.Newf("任务处理器 %s 未注册", expression.Key))
	}
	maxRetry := e.config.MaxRetry
	if definition.HasMaxRetry {
		maxRetry = definition.MaxRetry
	}
	if payload.Attempt > maxRetry {
		return task.SkipRetry(gerror.New("任务重试次数已耗尽"))
	}
	executionContext, err := taskTenantContext(ctx, info.TenantID)
	if err != nil {
		return err
	}
	lease, claimed, err := e.claim(ctx, info, payload)
	if err != nil || !claimed {
		return err
	}
	handlerContext, cancelHandler := context.WithCancelCause(executionContext)
	guard := startLeaseGuard(lease.ExpiresAt, e.config.LockTTL, func(renewContext context.Context) (time.Time, error) {
		return e.store.Renew(renewContext, info, lease.Token, e.config.LockTTL)
	}, cancelHandler)
	defer func() {
		if guard.Stop() {
			if releaseErr := e.store.Release(context.Background(), info, lease.Token); releaseErr != nil {
				g.Log().Error(context.Background(), releaseErr)
			}
		}
		cancelHandler(context.Canceled)
	}()
	timeout := e.config.Timeout
	if definition.Timeout > 0 {
		timeout = definition.Timeout
	}
	lastAttempt := maxRetry
	if payload.IsRetryManaged {
		lastAttempt = payload.Attempt
	}
	for attempt := payload.Attempt; attempt <= lastAttempt; attempt++ {
		if err = guard.Err(); err != nil {
			return err
		}
		invocation := task.Invocation{
			TaskID: info.ID, TenantID: cloneTaskTenantID(info.TenantID),
			ScheduledAt: payload.ScheduledAt, Attempt: attempt, Data: info.Data,
			Arguments: append([]json.RawMessage{}, expression.Arguments...),
		}
		result, invokeErr, isPending, handlerDone := invokeHandler(handlerContext, timeout, definition.Handler, invocation)
		if err = guard.Err(); err != nil {
			return err
		}
		status := 1
		detail := resultDetail(result)
		if invokeErr != nil {
			status = 0
			detail = invokeErr.Error()
		}
		if logErr := e.store.WriteLog(ctx, info, status, truncateDetail(detail)); logErr != nil {
			if isPending {
				if err = waitForPendingHandler(handlerDone, guard); err != nil {
					return err
				}
			}
			return logErr
		}
		if invokeErr == nil {
			return nil
		}
		if isPending {
			// 处理器真正退出前持续持有租约，避免超时重试与旧调用并发。
			if err = waitForPendingHandler(handlerDone, guard); err != nil {
				return err
			}
		}
		if task.IsPermanent(invokeErr) {
			return invokeErr
		}
		if attempt == maxRetry {
			return task.SkipRetry(invokeErr)
		}
		if payload.IsRetryManaged {
			return invokeErr
		}
		delay := e.config.RetryDelay * time.Duration(1<<attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-guard.Lost():
			timer.Stop()
			return guard.Err()
		case <-timer.C:
		}
	}
	return err
}

// claim 使用独立 token 领取租约，并让手动执行只做短暂有界等待。
func (e *Executor) claim(ctx context.Context, info TaskInfo, payload taskQueue.Payload) (TaskLease, bool, error) {
	waitDeadline := time.Now().Add(manualClaimWait)
	for {
		claimToken := uuid.NewString()
		lease, claimed, err := e.store.Claim(ctx, info, payload, claimToken, e.config.LockTTL)
		if err != nil || claimed || (payload.Attempt == 0 && !payload.Manual) {
			return lease, claimed, err
		}
		current, exists, err := e.store.FindInternal(ctx, info.ID)
		if err != nil || !exists || current.JobID != info.JobID {
			return TaskLease{}, false, err
		}
		if !payload.Manual && !canResumeScheduled(current, payload.ScheduledAt) {
			return TaskLease{}, false, nil
		}
		if current.LockExpireTime == nil || !current.LockExpireTime.Time.After(time.Now()) {
			info = current
			continue
		}
		if !payload.Manual || !time.Now().Before(waitDeadline) {
			redelivery, encodeErr := taskQueue.EncodeRedelivery(payload)
			if encodeErr != nil {
				return TaskLease{}, false, encodeErr
			}
			return TaskLease{}, false, task.Busy(ErrTaskBusy, busyRedeliveryDelay, redelivery)
		}
		deadline := current.LockExpireTime.Time
		if payload.Manual {
			pollAt := time.Now().Add(manualClaimPollInterval)
			if pollAt.Before(deadline) {
				deadline = pollAt
			}
			if waitDeadline.Before(deadline) {
				deadline = waitDeadline
			}
		}
		if err = waitUntil(ctx, deadline); err != nil {
			return TaskLease{}, false, err
		}
		info = current
	}
}

// waitForPendingHandler 等待已被取消但尚未退出的处理器或租约丢失。
func waitForPendingHandler(done <-chan struct{}, guard *leaseGuard) error {
	select {
	case <-done:
		return nil
	case <-guard.Lost():
		return guard.Err()
	}
}

type leaseGuard struct {
	stopContext   context.Context
	stop          context.CancelFunc
	done          chan struct{}
	lost          chan struct{}
	renew         func(context.Context) (time.Time, error)
	cancel        context.CancelCauseFunc
	lockTTL       time.Duration
	initialExpiry time.Time
	stopOnce      sync.Once
	mu            sync.RWMutex
	err           error
	isOwned       bool
}

// startLeaseGuard 从 Claim 成功时开始统一保护处理器和等待流程。
func startLeaseGuard(initialExpiry time.Time, lockTTL time.Duration, renew func(context.Context) (time.Time, error), cancel context.CancelCauseFunc) *leaseGuard {
	stopContext, stop := context.WithCancel(context.Background())
	guard := &leaseGuard{
		stopContext: stopContext, stop: stop, done: make(chan struct{}), lost: make(chan struct{}),
		renew: renew, cancel: cancel, lockTTL: lockTTL, initialExpiry: initialExpiry,
	}
	go guard.run()
	return guard
}

// Stop 停止守护并返回当前执行是否仍可释放租约。
func (g *leaseGuard) Stop() bool {
	g.stopOnce.Do(g.stop)
	<-g.done
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.isOwned
}

// Lost 返回租约丢失通知。
func (g *leaseGuard) Lost() <-chan struct{} {
	return g.lost
}

// Err 返回租约守护的终止错误。
func (g *leaseGuard) Err() error {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.err
}

func (g *leaseGuard) run() {
	defer close(g.done)
	var (
		expiresAt     = g.initialExpiry
		renewInterval = g.lockTTL / 3
	)
	if renewInterval <= 0 {
		renewInterval = g.lockTTL
	}
	failureBackoff := renewInterval / 4
	if failureBackoff <= 0 {
		failureBackoff = time.Millisecond
	}
	if failureBackoff > maxLeaseRenewFailureBackoff {
		failureBackoff = maxLeaseRenewFailureBackoff
	}
	nextRenewAt := nextLeaseRenewAt(time.Now(), expiresAt, renewInterval)
	for {
		now := time.Now()
		if !now.Before(expiresAt) {
			g.markLost()
			return
		}
		wakeAt := nextRenewAt
		if expiresAt.Before(wakeAt) {
			wakeAt = expiresAt
		}
		timer := time.NewTimer(time.Until(wakeAt))
		select {
		case <-g.stopContext.Done():
			timer.Stop()
			if time.Now().Before(expiresAt) {
				g.mu.Lock()
				g.isOwned = true
				g.mu.Unlock()
				return
			}
			g.markLost()
			return
		case <-timer.C:
		}
		now = time.Now()
		if !now.Before(expiresAt) {
			g.markLost()
			return
		}
		renewContext, cancelRenew := context.WithDeadline(g.stopContext, expiresAt)
		renewedUntil, err := g.renew(renewContext)
		cancelRenew()
		if err == nil && renewedUntil.After(now) {
			expiresAt = renewedUntil
			nextRenewAt = nextLeaseRenewAt(time.Now(), expiresAt, renewInterval)
			continue
		}
		if errors.Is(err, ErrTaskLeaseLost) {
			g.markLost()
			return
		}
		if g.stopContext.Err() != nil {
			if time.Now().Before(expiresAt) {
				g.mu.Lock()
				g.isOwned = true
				g.mu.Unlock()
				return
			}
			g.markLost()
			return
		}
		nextRenewAt = time.Now().Add(failureBackoff)
		if expiresAt.Before(nextRenewAt) {
			nextRenewAt = expiresAt
		}
	}
}

func nextLeaseRenewAt(now time.Time, expiresAt time.Time, renewInterval time.Duration) time.Time {
	next := now.Add(renewInterval)
	safeBoundary := now.Add(expiresAt.Sub(now) / 2)
	if safeBoundary.Before(next) {
		return safeBoundary
	}
	return next
}

func (g *leaseGuard) markLost() {
	g.mu.Lock()
	if g.err == nil {
		g.err = ErrTaskLeaseLost
		g.cancel(ErrTaskLeaseLost)
		close(g.lost)
	}
	g.mu.Unlock()
}

// waitUntil 等待指定时刻或上游取消。
func waitUntil(ctx context.Context, deadline time.Time) error {
	delay := time.Until(deadline)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func canRunScheduled(info TaskInfo, scheduledAt time.Time) bool {
	if info.Status != 1 {
		return false
	}
	if info.StartDate != nil && scheduledAt.Before(info.StartDate.Time) {
		return false
	}
	if info.EndDate != nil && scheduledAt.After(info.EndDate.Time) {
		return false
	}
	state := repeatState(info)
	return info.Limit == nil || state.Count < *info.Limit
}

func canResumeScheduled(info TaskInfo, scheduledAt time.Time) bool {
	if info.LastExecuteTime == nil || !scheduledBatchTime(info.LastExecuteTime.Time).Equal(scheduledBatchTime(scheduledAt)) {
		return false
	}
	if info.Status == 1 {
		return true
	}
	state := repeatState(info)
	return info.Status == 0 && info.Limit != nil && state.Count >= *info.Limit
}

func repeatState(info TaskInfo) RepeatState {
	state := RepeatState{Version: 1, Generation: info.JobID}
	if err := json.Unmarshal([]byte(info.RepeatConf), &state); err != nil || state.Generation != info.JobID {
		return RepeatState{Version: 1, Generation: info.JobID}
	}
	return state
}

type handlerResult struct {
	value      interface{}
	err        error
	panicValue interface{}
}

func invokeHandler(ctx context.Context, timeout time.Duration, handler task.Handler, invocation task.Invocation) (interface{}, error, bool, <-chan struct{}) {
	attemptContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resultChannel := make(chan handlerResult, 1)
	done := make(chan struct{})
	go func() {
		result := handlerResult{}
		defer func() {
			if recovered := recover(); recovered != nil {
				result.panicValue = recovered
			}
			resultChannel <- result
			close(done)
		}()
		result.value, result.err = handler(attemptContext, invocation)
	}()
	select {
	case result := <-resultChannel:
		if result.panicValue != nil {
			g.Log().Errorf(context.Background(), "Task 处理器 panic: %v\n%s", result.panicValue, debug.Stack())
			return nil, gerror.New("任务处理器发生异常"), false, done
		}
		return result.value, result.err, false, done
	case <-attemptContext.Done():
		if errors.Is(attemptContext.Err(), context.DeadlineExceeded) {
			return nil, gerror.Wrap(attemptContext.Err(), "任务处理器执行超时"), true, done
		}
		return nil, gerror.Wrap(attemptContext.Err(), "任务处理器执行已取消"), true, done
	}
}

func resultDetail(result interface{}) string {
	if result == nil {
		return ""
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%T", result)
	}
	return string(encoded)
}

func truncateDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxLogDetailLength {
		return value
	}
	return value[:maxLogDetailLength]
}
