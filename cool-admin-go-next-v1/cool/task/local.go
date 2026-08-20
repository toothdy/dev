package task

import (
	"container/heap"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/robfig/cron/v3"
)

const localRetryPendingPerWorker = 64

type localSchedule struct {
	entryID  cron.EntryID
	plan     Schedule
	schedule cron.Schedule
}

type localRedelivery struct {
	message Message
	readyAt time.Time
	order   uint64
}

type localRedeliveryHeap []localRedelivery

func (h localRedeliveryHeap) Len() int { return len(h) }

func (h localRedeliveryHeap) Less(first int, second int) bool {
	if h[first].readyAt.Equal(h[second].readyAt) {
		return h[first].order < h[second].order
	}
	return h[first].readyAt.Before(h[second].readyAt)
}

func (h localRedeliveryHeap) Swap(first int, second int) {
	h[first], h[second] = h[second], h[first]
}

func (h *localRedeliveryHeap) Push(value interface{}) {
	*h = append(*h, value.(localRedelivery))
}

func (h *localRedeliveryHeap) Pop() interface{} {
	previous := *h
	last := len(previous) - 1
	value := previous[last]
	previous[last] = localRedelivery{}
	*h = previous[:last]
	return value
}

// LocalScheduler 是进程内 Cron 调度器。
type LocalScheduler struct {
	mu            sync.RWMutex
	retryMu       sync.Mutex
	cron          *cron.Cron
	location      *time.Location
	consumer      Consumer
	semaphore     chan struct{}
	deliverySlots chan struct{}
	retryQueue    chan localRedelivery
	retryLimit    int
	retryPending  int
	retryOrder    uint64
	retryClosed   bool
	workers       sync.WaitGroup
	stopContext   context.Context
	stop          context.CancelFunc
	schedules     map[string]localSchedule
	isStarted     bool
	isStopping    bool
}

// NewLocalScheduler 创建进程内调度器。
func NewLocalScheduler(concurrency int, location *time.Location, consumer Consumer) (*LocalScheduler, error) {
	if concurrency <= 0 {
		return nil, fmt.Errorf("本地任务并发数必须大于 0")
	}
	if location == nil {
		return nil, fmt.Errorf("本地任务时区不能为空")
	}
	if consumer == nil {
		return nil, fmt.Errorf("本地任务消费函数不能为空")
	}
	parser := cron.NewParser(
		cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)
	stopContext, stop := context.WithCancel(context.Background())
	return &LocalScheduler{
		cron:          cron.New(cron.WithParser(parser), cron.WithLocation(location)),
		location:      location,
		consumer:      consumer,
		semaphore:     make(chan struct{}, concurrency),
		deliverySlots: make(chan struct{}, concurrency*localRetryPendingPerWorker),
		retryQueue:    make(chan localRedelivery, concurrency*localRetryPendingPerWorker),
		retryLimit:    concurrency * localRetryPendingPerWorker,
		stopContext:   stopContext,
		stop:          stop,
		schedules:     map[string]localSchedule{},
	}, nil
}

// Start 启动本地 Cron。
func (s *LocalScheduler) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.isStopping {
		return fmt.Errorf("本地任务调度器正在停止")
	}
	if s.isStarted {
		return nil
	}
	s.cron.Start()
	s.workers.Add(1)
	go s.runRedeliveries()
	s.isStarted = true
	return nil
}

// Healthy 检查本地调度器能否接收变更。
func (s *LocalScheduler) Healthy(context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isStopping {
		return fmt.Errorf("本地任务调度器正在停止")
	}
	if !s.isStarted {
		return fmt.Errorf("本地任务调度器尚未启动")
	}
	return nil
}

// Upsert 注册或替换一个周期计划。
func (s *LocalScheduler) Upsert(_ context.Context, plan Schedule) (time.Time, error) {
	s.mu.RLock()
	isStopping := s.isStopping
	s.mu.RUnlock()
	if isStopping {
		return time.Time{}, fmt.Errorf("本地任务调度器正在停止")
	}
	if plan.ID == "" || plan.Message == nil {
		return time.Time{}, fmt.Errorf("任务计划缺少 ID 或消息工厂")
	}
	schedule, err := s.compileSchedule(plan)
	if err != nil {
		return time.Time{}, err
	}
	next := nextWithinWindow(schedule, time.Now().In(s.location), plan.StartDate, plan.EndDate)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("任务计划已超过结束时间")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.schedules[plan.ID]; ok {
		s.cron.Remove(current.entryID)
	}
	entryID := s.cron.Schedule(schedule, cron.FuncJob(func() {
		s.dispatchSchedule(plan, schedule)
	}))
	s.schedules[plan.ID] = localSchedule{entryID: entryID, plan: plan, schedule: schedule}
	return next, nil
}

// Remove 删除一个周期计划。
func (s *LocalScheduler) Remove(_ context.Context, scheduleID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current, ok := s.schedules[scheduleID]; ok {
		s.cron.Remove(current.entryID)
		delete(s.schedules, scheduleID)
	}
	return nil
}

// Enqueue 提交一次消息。
func (s *LocalScheduler) Enqueue(ctx context.Context, message Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.isStopping {
		return fmt.Errorf("本地任务调度器正在停止")
	}
	if !s.isStarted {
		return fmt.Errorf("本地任务调度器尚未启动")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.ID == "" {
		return fmt.Errorf("队列消息 ID 不能为空")
	}
	if !s.reserveDelivery() {
		return fmt.Errorf("本地任务待处理队列已满")
	}
	s.run(cloneMessage(message))
	return nil
}

// NextRunTime 返回当前计划的下次执行时间。
func (s *LocalScheduler) NextRunTime(scheduleID string) (time.Time, bool) {
	s.mu.RLock()
	item, ok := s.schedules[scheduleID]
	s.mu.RUnlock()
	if !ok {
		return time.Time{}, false
	}
	next := s.cron.Entry(item.entryID).Next
	if next.IsZero() {
		next = nextWithinWindow(item.schedule, time.Now().In(s.location), item.plan.StartDate, item.plan.EndDate)
	}
	if next.IsZero() || item.plan.EndDate != nil && next.After(*item.plan.EndDate) {
		return time.Time{}, false
	}
	return next, true
}

// Stop 停止产生消息并等待已分发消息结束。
func (s *LocalScheduler) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.isStopping {
		s.mu.Unlock()
		return nil
	}
	s.isStopping = true
	s.retryMu.Lock()
	s.retryClosed = true
	s.retryMu.Unlock()
	s.stop()
	waitCron := s.cron.Stop()
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		<-waitCron.Done()
		s.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *LocalScheduler) compileSchedule(plan Schedule) (cron.Schedule, error) {
	if plan.Cron != "" && plan.Every > 0 {
		return nil, fmt.Errorf("Cron 和执行间隔不能同时设置")
	}
	if plan.Cron != "" {
		return cron.NewParser(
			cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		).Parse(plan.Cron)
	}
	if plan.Every <= 0 {
		return nil, fmt.Errorf("任务计划缺少 Cron 或执行间隔")
	}
	anchor := plan.Anchor
	if anchor.IsZero() {
		anchor = time.Now().In(s.location)
	}
	return alignedInterval{every: plan.Every, anchor: anchor}, nil
}

func (s *LocalScheduler) dispatchSchedule(plan Schedule, schedule cron.Schedule) {
	now := time.Now().In(s.location)
	if plan.StartDate != nil && now.Before(*plan.StartDate) || plan.EndDate != nil && now.After(*plan.EndDate) {
		return
	}
	scheduledAt := currentOccurrence(schedule, now)
	message, err := plan.Message(scheduledAt)
	if err != nil {
		g.Log().Errorf(context.Background(), "生成计划 %s 消息失败: %v", plan.ID, err)
		return
	}
	if message.ID == "" {
		g.Log().Errorf(context.Background(), "计划 %s 生成了空消息 ID", plan.ID)
		return
	}
	if !s.reserveDelivery() {
		g.Log().Errorf(context.Background(), "计划 %s 的本地任务待处理队列已满", plan.ID)
		return
	}
	s.run(cloneMessage(message))
}

func (s *LocalScheduler) run(message Message) {
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		select {
		case s.semaphore <- struct{}{}:
		case <-s.stopContext.Done():
			s.releaseDelivery()
			return
		}
		err := s.consumer(s.stopContext, message)
		<-s.semaphore
		next, delay, isBusy := BusyRedelivery(err)
		if !isBusy {
			s.releaseDelivery()
			if err != nil {
				g.Log().Errorf(context.Background(), "消费本地任务消息 %s 失败: %v", message.ID, err)
			}
			return
		}
		if !s.scheduleRedelivery(next, delay) {
			s.releaseDelivery()
			if s.stopContext.Err() == nil {
				g.Log().Errorf(context.Background(), "本地任务消息 %s 无法登记重投", message.ID)
			}
		}
	}()
}

func (s *LocalScheduler) reserveDelivery() bool {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	if s.retryClosed || len(s.deliverySlots) >= cap(s.deliverySlots) {
		return false
	}
	s.deliverySlots <- struct{}{}
	return true
}

func (s *LocalScheduler) releaseDelivery() {
	<-s.deliverySlots
}

func (s *LocalScheduler) scheduleRedelivery(message Message, delay time.Duration) bool {
	s.retryMu.Lock()
	defer s.retryMu.Unlock()
	if s.retryClosed || s.retryPending >= s.retryLimit {
		return false
	}
	s.retryOrder++
	s.retryPending++
	s.retryQueue <- localRedelivery{
		message: cloneMessage(message), readyAt: time.Now().Add(delay), order: s.retryOrder,
	}
	return true
}

func (s *LocalScheduler) runRedeliveries() {
	defer s.workers.Done()
	pending := &localRedeliveryHeap{}
	heap.Init(pending)
	defer func() {
		s.retryMu.Lock()
		pendingCount := s.retryPending
		s.retryPending = 0
		s.retryMu.Unlock()
		for index := 0; index < pendingCount; index++ {
			s.releaseDelivery()
		}
	}()
	for {
		var (
			timer  *time.Timer
			timerC <-chan time.Time
		)
		if pending.Len() > 0 {
			delay := time.Until((*pending)[0].readyAt)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			timerC = timer.C
		}
		select {
		case <-s.stopContext.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case redelivery := <-s.retryQueue:
			if timer != nil {
				timer.Stop()
			}
			heap.Push(pending, redelivery)
		case <-timerC:
			redelivery := heap.Pop(pending).(localRedelivery)
			s.retryMu.Lock()
			s.retryPending--
			s.retryMu.Unlock()
			s.run(redelivery.message)
		}
	}
}

type alignedInterval struct {
	every  time.Duration
	anchor time.Time
}

func (s alignedInterval) Next(after time.Time) time.Time {
	if after.Before(s.anchor) {
		return s.anchor
	}
	steps := after.Sub(s.anchor)/s.every + 1
	return s.anchor.Add(steps * s.every)
}

func nextWithinWindow(schedule cron.Schedule, after time.Time, startDate *time.Time, endDate *time.Time) time.Time {
	if startDate != nil && after.Before(startDate.Add(-time.Nanosecond)) {
		after = startDate.Add(-time.Nanosecond)
	}
	next := schedule.Next(after)
	if endDate != nil && next.After(*endDate) {
		return time.Time{}
	}
	return next
}

func currentOccurrence(schedule cron.Schedule, now time.Time) time.Time {
	// 秒级 Cron 的触发误差通常远小于一秒，向前取一个小窗口恢复计划时间。
	window := time.Second
	if interval, ok := schedule.(alignedInterval); ok {
		window = interval.every
	}
	next := schedule.Next(now.Add(-window - time.Nanosecond))
	for next.After(now) {
		window *= 2
		next = schedule.Next(now.Add(-window - time.Nanosecond))
	}
	return next
}
