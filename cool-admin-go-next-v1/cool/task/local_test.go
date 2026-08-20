package task

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLocalSchedulerRunsAlignedIntervalAndCanRemoveIt(t *testing.T) {
	var (
		mu       sync.Mutex
		messages []Message
	)
	scheduler, err := NewLocalScheduler(2, time.UTC, func(_ context.Context, message Message) error {
		mu.Lock()
		messages = append(messages, message)
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("create scheduler failed: %v", err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatalf("start scheduler failed: %v", err)
	}
	defer scheduler.Stop(context.Background())

	anchor := time.Now().UTC().Add(-5 * time.Second).Truncate(10 * time.Millisecond)
	next, err := scheduler.Upsert(context.Background(), Schedule{
		ID:     "task:1",
		Every:  30 * time.Millisecond,
		Anchor: anchor,
		Message: func(scheduledAt time.Time) (Message, error) {
			return Message{ID: fmt.Sprintf("task:1:%d", scheduledAt.UnixNano()), Payload: []byte("scheduled")}, nil
		},
	})
	if err != nil || !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("upsert interval failed: next=%v err=%v", next, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		count := len(messages)
		mu.Unlock()
		if count > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	count := len(messages)
	mu.Unlock()
	if count == 0 {
		t.Fatal("expected interval dispatch")
	}
	if err = scheduler.Remove(context.Background(), "task:1"); err != nil {
		t.Fatalf("remove schedule failed: %v", err)
	}
	if _, ok := scheduler.NextRunTime("task:1"); ok {
		t.Fatal("removed schedule must not expose next run time")
	}
}

func TestLocalSchedulerParsesSixFieldCronAndRejectsInvalidSchedule(t *testing.T) {
	scheduler, err := NewLocalScheduler(1, time.FixedZone("test", 8*60*60), func(context.Context, Message) error { return nil })
	if err != nil {
		t.Fatalf("create scheduler failed: %v", err)
	}
	factory := func(time.Time) (Message, error) {
		return Message{ID: "cron-message"}, nil
	}
	if _, err = scheduler.Upsert(context.Background(), Schedule{ID: "task:1", Cron: "*/5 * * * * *", Message: factory}); err != nil {
		t.Fatalf("expected six-field cron: %v", err)
	}
	if _, err = scheduler.Upsert(context.Background(), Schedule{ID: "task:2", Message: factory}); err == nil {
		t.Fatal("expected missing schedule error")
	}
	if _, err = scheduler.Upsert(context.Background(), Schedule{ID: "task:3", Every: time.Second}); err == nil {
		t.Fatal("expected missing message factory error")
	}
}

func TestLocalSchedulerEnqueueCopiesCallerPayload(t *testing.T) {
	release := make(chan struct{})
	received := make(chan Message, 1)
	scheduler, err := NewLocalScheduler(1, time.UTC, func(_ context.Context, message Message) error {
		<-release
		received <- message
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer scheduler.Stop(context.Background())

	payload := []byte("before")
	if err = scheduler.Enqueue(context.Background(), Message{ID: "manual-1", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	payload[0] = 'x'
	close(release)
	select {
	case message := <-received:
		if string(message.Payload) != "before" {
			t.Fatalf("consumer observed caller mutation: %q", message.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for local message")
	}
}

func TestLocalSchedulerBusyWaitDoesNotOccupyWorkerSlots(t *testing.T) {
	executed := make(chan struct{}, 1)
	scheduler, err := NewLocalScheduler(2, time.UTC, func(_ context.Context, message Message) error {
		if string(message.Payload) == "busy" {
			return Busy(errors.New("lease busy"), 200*time.Millisecond, Message{
				ID: message.ID + "-retry", Payload: message.Payload,
			})
		}
		executed <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer scheduler.Stop(context.Background())
	for index := 0; index < 6; index++ {
		if err = scheduler.Enqueue(context.Background(), Message{ID: fmt.Sprintf("busy-%d", index), Payload: []byte("busy")}); err != nil {
			t.Fatal(err)
		}
	}
	if err = scheduler.Enqueue(context.Background(), Message{ID: "ready", Payload: []byte("ready")}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-executed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("busy redeliveries occupied all local worker slots")
	}
}

func TestLocalSchedulerBusyPendingResourcesStayBounded(t *testing.T) {
	var calls atomic.Int32
	scheduler, err := NewLocalScheduler(2, time.UTC, func(_ context.Context, message Message) error {
		calls.Add(1)
		return Busy(errors.New("lease busy"), time.Hour, Message{ID: message.ID + "-retry"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Stop(context.Background()) })
	baseline := runtime.NumGoroutine()
	total := scheduler.retryLimit + 16
	rejected := 0
	for index := 0; index < total; index++ {
		if err = scheduler.Enqueue(context.Background(), Message{ID: fmt.Sprintf("busy-%d", index)}); err != nil {
			rejected++
		}
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() < int32(scheduler.retryLimit) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != int32(scheduler.retryLimit) || rejected != total-scheduler.retryLimit {
		t.Fatalf("有界 delivery 接收结果异常: calls=%d rejected=%d limit=%d", calls.Load(), rejected, scheduler.retryLimit)
	}
	deadline = time.Now().Add(time.Second)
	for runtime.NumGoroutine() > baseline+4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	scheduler.retryMu.Lock()
	pending := scheduler.retryPending
	limit := scheduler.retryLimit
	scheduler.retryMu.Unlock()
	if pending != limit {
		t.Fatalf("busy pending 未受容量约束: pending=%d limit=%d", pending, limit)
	}
	if current := runtime.NumGoroutine(); current > baseline+4 {
		t.Fatalf("每条 busy delivery 仍长期占用 goroutine: baseline=%d current=%d", baseline, current)
	}
	if err = scheduler.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLocalSchedulerStopCancelsPendingRedeliveries(t *testing.T) {
	consumed := make(chan struct{}, 1)
	scheduler, err := NewLocalScheduler(1, time.UTC, func(_ context.Context, message Message) error {
		consumed <- struct{}{}
		return Busy(errors.New("lease busy"), time.Hour, Message{ID: message.ID + "-retry"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Stop(context.Background()) })
	if err = scheduler.Enqueue(context.Background(), Message{ID: "busy"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-consumed:
	case <-time.After(time.Second):
		t.Fatal("busy delivery 未消费")
	}
	deadline := time.Now().Add(time.Second)
	pendingBeforeStop := 0
	for {
		scheduler.retryMu.Lock()
		pendingBeforeStop = scheduler.retryPending
		scheduler.retryMu.Unlock()
		if pendingBeforeStop == 1 || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pendingBeforeStop != 1 {
		t.Fatalf("busy 重投未进入 pending: %d", pendingBeforeStop)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = scheduler.Stop(stopContext); err != nil {
		t.Fatalf("停止含 pending 的本地队列失败: %v", err)
	}
	scheduler.retryMu.Lock()
	pending := scheduler.retryPending
	scheduler.retryMu.Unlock()
	if pending != 0 {
		t.Fatalf("停止后仍保留 busy pending: %d", pending)
	}
	if slots := len(scheduler.deliverySlots); slots != 0 {
		t.Fatalf("停止后仍占用 delivery slot: %d", slots)
	}
	select {
	case <-consumed:
		t.Fatal("停止后仍触发 busy 重投")
	default:
	}
}
