package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

type executorStoreStub struct {
	mu            sync.Mutex
	info          TaskInfo
	claimExpires  time.Duration
	claimTokens   []string
	renewTokens   []string
	releaseTokens []string
	renewTimes    []time.Time
	isBusy        bool
	logCalls      int
	renewFunc     func(context.Context) (time.Time, error)
}

func (s *executorStoreStub) FindInternal(context.Context, int64) (TaskInfo, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info, true, nil
}

func (s *executorStoreStub) Claim(_ context.Context, _ TaskInfo, _ taskQueue.Payload, claimToken string, _ time.Duration) (TaskLease, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimTokens = append(s.claimTokens, claimToken)
	if s.isBusy {
		return TaskLease{}, false, nil
	}
	return TaskLease{Token: claimToken, ExpiresAt: time.Now().Add(s.claimExpires)}, true, nil
}

func (s *executorStoreStub) Renew(ctx context.Context, _ TaskInfo, claimToken string, _ time.Duration) (time.Time, error) {
	s.mu.Lock()
	s.renewTokens = append(s.renewTokens, claimToken)
	s.renewTimes = append(s.renewTimes, time.Now())
	renewFunc := s.renewFunc
	s.mu.Unlock()
	if renewFunc != nil {
		return renewFunc(ctx)
	}
	return time.Now().Add(s.claimExpires), nil
}

func (s *executorStoreStub) Release(_ context.Context, _ TaskInfo, claimToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseTokens = append(s.releaseTokens, claimToken)
	return nil
}

func (s *executorStoreStub) WriteLog(context.Context, TaskInfo, int, string) error {
	s.mu.Lock()
	s.logCalls++
	s.mu.Unlock()
	return nil
}

func TestInvokeHandlerRecoversPanicAndEnforcesTimeout(t *testing.T) {
	_, err, isTimeout, _ := invokeHandler(context.Background(), time.Second, func(context.Context, task.Invocation) (interface{}, error) {
		panic("boom")
	}, task.Invocation{})
	if err == nil || isTimeout {
		t.Fatalf("应恢复 panic: err=%v timeout=%v", err, isTimeout)
	}

	release := make(chan struct{})
	_, err, isTimeout, done := invokeHandler(context.Background(), 20*time.Millisecond, func(context.Context, task.Invocation) (interface{}, error) {
		<-release
		return nil, nil
	}, task.Invocation{})
	close(release)
	<-done
	if err == nil || !isTimeout {
		t.Fatalf("应返回超时: err=%v timeout=%v", err, isTimeout)
	}
}

func TestInvokeHandlerPreservesPermanentError(t *testing.T) {
	want := task.Permanent(errors.New("invalid input"))
	_, err, isTimeout, _ := invokeHandler(context.Background(), time.Second, func(context.Context, task.Invocation) (interface{}, error) {
		return nil, want
	}, task.Invocation{})
	if !errors.Is(err, want) || !task.IsPermanent(err) || isTimeout {
		t.Fatalf("处理器结果异常: err=%v timeout=%v", err, isTimeout)
	}
}

func TestCanRunScheduledHonorsWindowAndLimit(t *testing.T) {
	now := time.Now()
	limit := 1
	info := TaskInfo{Status: 1, JobID: "job", Limit: &limit, RepeatConf: `{"version":1,"generation":"job","count":1}`}
	if canRunScheduled(info, now) {
		t.Fatal("达到 limit 的任务不能执行")
	}
	info.Limit = nil
	info.StartDate = gtime.NewFromTime(now.Add(time.Minute))
	if canRunScheduled(info, now) {
		t.Fatal("开始时间前的任务不能执行")
	}
}

func TestCanResumeScheduledRequiresSameLimitedBatch(t *testing.T) {
	storedAt := time.Now().Truncate(time.Second)
	scheduledAt := storedAt.Add(500 * time.Millisecond)
	limit := 1
	info := TaskInfo{
		Status: 0, JobID: "job", Limit: &limit,
		RepeatConf:      `{"version":1,"generation":"job","count":1}`,
		LastExecuteTime: gtime.NewFromTime(storedAt),
	}
	if !canResumeScheduled(info, scheduledAt) {
		t.Fatal("达到 limit 后必须允许毫秒计划时间对应的同一批次恢复重试")
	}
	if canResumeScheduled(info, scheduledAt.Add(1500*time.Millisecond)) {
		t.Fatal("不能把其他批次当作恢复重试")
	}
	info.Limit = nil
	if canResumeScheduled(info, scheduledAt) {
		t.Fatal("普通停止任务不能通过恢复重试重新执行")
	}
}

func TestScheduledBatchTimeUsesMySQLSecondPrecision(t *testing.T) {
	scheduledAt := time.Unix(100, 500*int64(time.Millisecond)).UTC()
	if normalized := scheduledBatchTime(scheduledAt); !normalized.Equal(time.Unix(100, 0).UTC()) {
		t.Fatalf("计划批次时间未截断到秒: %v", normalized)
	}
}

func TestLeaseGuardRecoversTransientRenewFailure(t *testing.T) {
	var renewCalls atomic.Int32
	handlerContext, cancelHandler := context.WithCancelCause(context.Background())
	guard := startLeaseGuard(time.Now().Add(120*time.Millisecond), 60*time.Millisecond, func(context.Context) (time.Time, error) {
		if renewCalls.Add(1) == 1 {
			return time.Time{}, errors.New("temporary database error")
		}
		return time.Now().Add(120 * time.Millisecond), nil
	}, cancelHandler)
	deadline := time.Now().Add(100 * time.Millisecond)
	for renewCalls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if renewCalls.Load() < 2 {
		t.Fatalf("租约守护未从瞬时续租失败恢复: calls=%d", renewCalls.Load())
	}
	isOwned := guard.Stop()
	if !isOwned || guard.Err() != nil || handlerContext.Err() != nil {
		t.Fatalf("恢复后的租约守护状态异常: owned=%v err=%v handler=%v", isOwned, guard.Err(), handlerContext.Err())
	}
}

func TestLeaseGuardStopsAfterConfirmedExpiry(t *testing.T) {
	var renewCalls atomic.Int32
	handlerContext, cancelHandler := context.WithCancelCause(context.Background())
	guard := startLeaseGuard(time.Now().Add(55*time.Millisecond), 30*time.Millisecond, func(context.Context) (time.Time, error) {
		renewCalls.Add(1)
		return time.Time{}, errors.New("database unavailable")
	}, cancelHandler)
	select {
	case <-guard.Lost():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("租约守护越过确认期限后仍未停止")
	}
	countAtLoss := renewCalls.Load()
	time.Sleep(30 * time.Millisecond)
	if renewCalls.Load() != countAtLoss || !errors.Is(guard.Err(), ErrTaskLeaseLost) || !errors.Is(context.Cause(handlerContext), ErrTaskLeaseLost) || guard.Stop() {
		t.Fatalf("租约丢失后的守护状态异常: calls=%d/%d err=%v cause=%v", countAtLoss, renewCalls.Load(), guard.Err(), context.Cause(handlerContext))
	}
}

func TestExecutorLeaseGuardDoesNotReleaseAfterConcurrentLeaseLoss(t *testing.T) {
	const (
		handlerTimeout = 2 * time.Second
		lockTTL        = 4 * time.Second
	)
	renewStarted := make(chan struct{})
	registry := executorTestRegistry(t, task.HandlerDefinition{
		Name: "leaseRaceService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			<-renewStarted
			return "ok", nil
		},
	})
	store := &executorStoreStub{
		info:         TaskInfo{ID: 1, JobID: "job-1", Service: "leaseRaceService.run()"},
		claimExpires: 100 * time.Millisecond,
	}
	var renewOnce sync.Once
	store.renewFunc = func(ctx context.Context) (time.Time, error) {
		renewOnce.Do(func() { close(renewStarted) })
		<-ctx.Done()
		return time.Time{}, ErrTaskLeaseLost
	}
	executor, err := BuildExecutor(&Store{}, registry, ExecutorConfig{
		Timeout: handlerTimeout, LockTTL: lockTTL, MaxRetry: 0, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("生产构造器拒绝了合法的 Timeout/LockTTL: %v", err)
	}
	executor.store = store
	err = executor.Execute(context.Background(), taskQueue.Payload{
		TaskID: 1, JobID: "job-1", ScheduledAt: time.Now(), Manual: true,
		ExecutionID: "stable-execution", IsRetryManaged: true,
	})
	if err != nil {
		t.Fatalf("处理器成功结果异常: %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.releaseTokens) != 0 {
		t.Fatalf("Stop/续租丢失竞态错误释放了租约: %v", store.releaseTokens)
	}
}

func TestExecutorGuardRenewsBeforeAndAfterHandlerTimeout(t *testing.T) {
	const (
		handlerTimeout = 2 * time.Second
		lockTTL        = 4 * time.Second
	)
	started := make(chan struct{})
	timedOut := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	var isTimedOut atomic.Bool
	registry := executorTestRegistry(t, task.HandlerDefinition{
		Name:    "leaseGuardService.run",
		Timeout: handlerTimeout,
		Handler: func(ctx context.Context, _ task.Invocation) (interface{}, error) {
			close(started)
			<-ctx.Done()
			isTimedOut.Store(true)
			close(timedOut)
			<-release
			return nil, errors.New("late failure")
		},
	})
	store := &executorStoreStub{
		info:         TaskInfo{ID: 1, JobID: "job-1", Service: "leaseGuardService.run()"},
		claimExpires: lockTTL,
	}
	executor, err := BuildExecutor(&Store{}, registry, ExecutorConfig{
		Timeout: handlerTimeout, LockTTL: lockTTL, MaxRetry: 0, RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("生产构造器拒绝了合法的 Timeout/LockTTL: %v", err)
	}
	executor.store = store
	renewed := make(chan bool, 32)
	store.renewFunc = func(context.Context) (time.Time, error) {
		renewed <- isTimedOut.Load()
		return time.Now().Add(store.claimExpires), nil
	}
	payload := taskQueue.Payload{
		TaskID: 1, JobID: "job-1", ScheduledAt: time.Now(), Manual: true,
		ExecutionID: "stable-execution", IsRetryManaged: true,
	}
	result := make(chan error, 1)
	go func() { result <- executor.Execute(context.Background(), payload) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("处理器未启动")
	}
	for {
		select {
		case afterTimeout := <-renewed:
			if !afterTimeout {
				goto renewedBeforeTimeout
			}
		case <-timedOut:
			t.Fatal("Handler 超时前未发生续租")
		case <-time.After(5 * time.Second):
			t.Fatal("等待 Handler 超时前续租超时")
		}
	}

renewedBeforeTimeout:
	select {
	case <-timedOut:
	case <-time.After(5 * time.Second):
		t.Fatal("Handler 未按契约超时")
	}
	for {
		select {
		case afterTimeout := <-renewed:
			if afterTimeout {
				goto renewedAfterTimeout
			}
		case <-time.After(5 * time.Second):
			t.Fatal("Handler 超时后租约未继续续租")
		}
	}

renewedAfterTimeout:
	store.mu.Lock()
	claimTokens := append([]string(nil), store.claimTokens...)
	store.mu.Unlock()
	select {
	case err := <-result:
		t.Fatalf("忽略取消的 Handler 退出前 Executor 已返回: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	if err = <-result; err == nil {
		t.Fatal("超时 attempt 必须保持失败")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(claimTokens) != 1 || claimTokens[0] == payload.ExecutionID || len(store.releaseTokens) != 1 || store.releaseTokens[0] != claimTokens[0] {
		t.Fatalf("Claim token 未与 ExecutionID 分离或释放 token 不匹配: claims=%v releases=%v", claimTokens, store.releaseTokens)
	}
	for _, token := range store.renewTokens {
		if token != claimTokens[0] {
			t.Fatalf("续租使用了错误 token: renews=%v claim=%v", store.renewTokens, claimTokens)
		}
	}
}

func TestExecutorManualBusyReturnsFreshRedeliveryWithoutAttemptChange(t *testing.T) {
	lockExpireTime := gtime.NewFromTime(time.Now().Add(time.Second))
	store := &executorStoreStub{
		info:         TaskInfo{ID: 1, JobID: "job-1", Service: "busyService.run()", LockExpireTime: lockExpireTime},
		claimExpires: time.Second,
		isBusy:       true,
	}
	executor := &Executor{store: store, config: ExecutorConfig{LockTTL: time.Second}}
	payload := taskQueue.Payload{
		TaskID: 1, JobID: "job-1", ScheduledAt: time.Now(), Manual: true,
		ExecutionID: "stable-execution", Attempt: 2,
	}
	_, claimed, err := executor.claim(context.Background(), store.info, payload)
	message, _, isBusy := task.BusyRedelivery(err)
	if claimed || !isBusy || !errors.Is(err, ErrTaskBusy) {
		t.Fatalf("手动忙租约未返回类型化 busy: claimed=%v err=%v", claimed, err)
	}
	redelivery, decodeErr := taskQueue.Decode(message)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if redelivery.ExecutionID != payload.ExecutionID || redelivery.Attempt != payload.Attempt || message.RetryBase != payload.Attempt {
		t.Fatalf("busy 重投改变了业务执行身份: payload=%#v message=%#v", redelivery, message)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	seen := map[string]struct{}{}
	for _, token := range store.claimTokens {
		if token == payload.ExecutionID {
			t.Fatal("Claim token 复用了 ExecutionID")
		}
		seen[token] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("每次 Claim 未生成唯一 token: %v", store.claimTokens)
	}
}

func TestExecutorScheduledRetryBusyReturnsFreshRedeliveryWithoutSideEffects(t *testing.T) {
	scheduledAt := time.Now().Truncate(time.Second)
	lockExpireTime := gtime.NewFromTime(time.Now().Add(time.Second))
	lastExecuteTime := gtime.NewFromTime(scheduledAt)
	limit := 3
	repeatConf := `{"version":1,"generation":"job-1","count":1}`
	store := &executorStoreStub{
		info: TaskInfo{
			ID: 1, JobID: "job-1", Service: "scheduledBusyService.run()", Status: 1,
			Limit: &limit, RepeatConf: repeatConf, LastExecuteTime: lastExecuteTime, LockExpireTime: lockExpireTime,
		},
		claimExpires: time.Second,
		isBusy:       true,
	}
	var handlerCalls atomic.Int32
	registry := executorTestRegistry(t, task.HandlerDefinition{
		Name: "scheduledBusyService.run",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			handlerCalls.Add(1)
			return nil, nil
		},
	})
	executor := &Executor{
		store: store, registry: registry,
		config: ExecutorConfig{Timeout: time.Second, LockTTL: time.Second, MaxRetry: 2, RetryDelay: time.Millisecond},
	}
	payload := taskQueue.Payload{
		TaskID: 1, JobID: "job-1", ScheduledAt: scheduledAt,
		ExecutionID: "stable-execution", Attempt: 1, IsRetryManaged: true,
	}
	err := executor.Execute(context.Background(), payload)
	message, _, isBusy := task.BusyRedelivery(err)
	if !isBusy || !errors.Is(err, ErrTaskBusy) {
		t.Fatalf("周期恢复 busy 未返回类型化重投: %v", err)
	}
	redelivery, decodeErr := taskQueue.Decode(message)
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	if redelivery.ExecutionID != payload.ExecutionID || redelivery.Attempt != payload.Attempt || message.RetryBase != payload.Attempt {
		t.Fatalf("周期 busy 重投改变了业务身份: payload=%#v message=%#v", redelivery, message)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if handlerCalls.Load() != 0 || store.logCalls != 0 || store.info.Limit == nil || *store.info.Limit != limit ||
		store.info.RepeatConf != repeatConf || store.info.LastExecuteTime != lastExecuteTime {
		t.Fatalf("周期 busy 窗口产生了副作用: calls=%d logs=%d info=%#v", handlerCalls.Load(), store.logCalls, store.info)
	}
}

func executorTestRegistry(t *testing.T, definition task.HandlerDefinition) *task.Registry {
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
