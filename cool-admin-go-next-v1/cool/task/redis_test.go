package task

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

type closeTrackingRedisClient struct {
	redis.UniversalClient
	closeCount atomic.Int32
}

type redisWorkerStub struct {
	stop     func()
	shutdown func()
}

func (s *redisWorkerStub) Start(asynq.Handler) error { return nil }
func (s *redisWorkerStub) Stop()                     { s.stop() }
func (s *redisWorkerStub) Shutdown()                 { s.shutdown() }

type redisProducerStub struct {
	stop func(context.Context) error
}

func (s *redisProducerStub) Start(context.Context) error { return nil }
func (s *redisProducerStub) Stop(ctx context.Context) error {
	return s.stop(ctx)
}
func (s *redisProducerStub) Upsert(context.Context, Schedule) (time.Time, error) {
	return time.Time{}, nil
}
func (s *redisProducerStub) Remove(context.Context, string) error { return nil }

func (c *closeTrackingRedisClient) Close() error {
	c.closeCount.Add(1)
	return nil
}

func TestRedisSchedulerMapsSkipRetryAndPreservesRawPayload(t *testing.T) {
	cause := errors.New("invalid task payload")
	var received Message
	scheduler := &RedisScheduler{
		consumer: func(_ context.Context, message Message) error {
			received = message
			return SkipRetry(cause)
		},
	}
	err := scheduler.processTask(context.Background(), asynq.NewTask("task-type", []byte(`{"taskId":1}`)))
	if !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("skip retry was not mapped to Asynq: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("skip retry mapping lost original cause: %v", err)
	}
	if string(received.Payload) != `{"taskId":1}` {
		t.Fatalf("Redis scheduler changed raw payload: %s", received.Payload)
	}
	if !received.IsRetryManaged {
		t.Fatal("Redis scheduler must delegate retry attempts to Asynq")
	}
}

func TestRedisSchedulerRetriesBusyPersistenceWithoutReexecutingDelivery(t *testing.T) {
	wantMessage := Message{ID: "transport-2", Payload: []byte(`{"executionId":"stable","attempt":2}`), RetryBase: 2}
	wantDelay := 75 * time.Millisecond
	var (
		consumerCalls   atomic.Int32
		redeliveryCalls atomic.Int32
		redelivered     Message
		delay           time.Duration
	)
	scheduler := &RedisScheduler{
		consumer: func(context.Context, Message) error {
			consumerCalls.Add(1)
			return Busy(errors.New("lease busy"), wantDelay, wantMessage)
		},
		redeliver: func(_ context.Context, message Message, after time.Duration) error {
			redelivered = message
			delay = after
			if redeliveryCalls.Add(1) < 3 {
				return errors.New("temporary enqueue failure")
			}
			return nil
		},
	}
	if err := scheduler.processTask(context.Background(), asynq.NewTask("task-type", []byte("original"))); err != nil {
		t.Fatalf("busy delivery should be acknowledged after redelivery: %v", err)
	}
	if redelivered.ID != wantMessage.ID || string(redelivered.Payload) != string(wantMessage.Payload) || redelivered.RetryBase != wantMessage.RetryBase || delay != wantDelay {
		t.Fatalf("unexpected Redis redelivery: message=%#v delay=%v", redelivered, delay)
	}
	if consumerCalls.Load() != 1 || redeliveryCalls.Load() != 3 {
		t.Fatalf("暂时 enqueue 故障重新执行业务或未原地重试: consumer=%d redelivery=%d", consumerCalls.Load(), redeliveryCalls.Load())
	}
}

func TestRedisSchedulerKeepsBusyDeliveryWhenContextCloses(t *testing.T) {
	cause := errors.New("lease busy")
	redelivered := false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scheduler := &RedisScheduler{
		consumer: func(context.Context, Message) error {
			return Busy(cause, time.Second, Message{ID: "transport-2"})
		},
		redeliver: func(context.Context, Message, time.Duration) error {
			redelivered = true
			return nil
		},
	}
	if err := scheduler.processTask(ctx, asynq.NewTask("task-type", nil)); !errors.Is(err, cause) || redelivered {
		t.Fatalf("Asynq worker context 关闭时错误确认了旧 delivery: err=%v redelivered=%v", err, redelivered)
	}
}

func TestRedisSchedulerKeepsBusyDeliveryWhenPersistenceWaitIsCanceled(t *testing.T) {
	cause := errors.New("lease busy")
	redeliveryErr := errors.New("enqueue unavailable")
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &RedisScheduler{
		consumer: func(context.Context, Message) error {
			return Busy(cause, time.Second, Message{ID: "transport-2"})
		},
		redeliver: func(context.Context, Message, time.Duration) error {
			cancel()
			return redeliveryErr
		},
	}
	if err := scheduler.processTask(ctx, asynq.NewTask("task-type", nil)); !errors.Is(err, cause) {
		t.Fatalf("重投等待取消时丢失 busy delivery: %v", err)
	}
}

func TestRedisSchedulerStopPreservesActiveDeliveryUntilGracefulShutdown(t *testing.T) {
	var (
		eventsMu sync.Mutex
		events   []string
	)
	record := func(event string) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}
	coordinatorContext, stopCoordinator := context.WithCancel(context.Background())
	workerContext, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerDone := make(chan struct{})
	scheduler := &RedisScheduler{
		consumer: func(ctx context.Context, _ Message) error {
			close(handlerStarted)
			select {
			case <-releaseHandler:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		coordinatorContext: coordinatorContext,
		stopCoordinator:    stopCoordinator,
		stopDone:           make(chan struct{}),
		isStarted:          true,
	}
	scheduler.server = &redisWorkerStub{
		stop: func() { record("worker-stop") },
		shutdown: func() {
			record("worker-shutdown")
			select {
			case <-workerContext.Done():
				t.Fatal("active handler context was canceled before graceful shutdown")
			default:
			}
			close(releaseHandler)
			<-handlerDone
		},
	}
	scheduler.producer = &redisProducerStub{stop: func(context.Context) error {
		if coordinatorContext.Err() == nil {
			t.Fatal("producer stopped before coordinator")
		}
		record("producer-stop")
		return nil
	}}
	go func() {
		defer close(handlerDone)
		if err := scheduler.processTask(workerContext, asynq.NewTask("task-type", nil)); err != nil {
			t.Errorf("active delivery failed during graceful shutdown: %v", err)
		}
	}()
	select {
	case <-handlerStarted:
	case <-time.After(time.Second):
		t.Fatal("active handler did not start")
	}
	if err := scheduler.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventsMu.Lock()
	defer eventsMu.Unlock()
	want := []string{"worker-stop", "producer-stop", "worker-shutdown"}
	if len(events) != len(want) {
		t.Fatalf("unexpected shutdown events: %v", events)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("unexpected shutdown order: got=%v want=%v", events, want)
		}
	}
}

func TestRedisSchedulerDoesNotCloseInjectedClient(t *testing.T) {
	underlying := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	client := &closeTrackingRedisClient{UniversalClient: underlying}
	scheduler, err := NewRedisScheduler(RedisSchedulerConfig{
		RedisClient:     client,
		Concurrency:     1,
		MaxRetry:        1,
		RetryDelay:      time.Millisecond,
		ShutdownTimeout: time.Second,
		Location:        time.UTC,
		Consumer:        func(context.Context, Message) error { return nil },
		QueueName:       "test-task-queue",
		TaskType:        "test:task:type",
		LeaseKey:        "test:task:lease",
		LeaseTTL:        time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if client.closeCount.Load() != 0 {
		t.Fatalf("scheduler closed injected Redis client %d times", client.closeCount.Load())
	}
	if err = underlying.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCalculateNextRunUsesIntervalAnchorAndEndDate(t *testing.T) {
	anchor := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	factory := func(time.Time) (Message, error) { return Message{ID: "message"}, nil }
	next, err := calculateNextRun(Schedule{ID: "task:1", Every: 5 * time.Second, Anchor: anchor, Message: factory}, time.UTC)
	if err != nil || !next.After(time.Now().Add(-time.Second)) {
		t.Fatalf("calculate interval failed: next=%v err=%v", next, err)
	}
	end := time.Now().Add(-time.Second)
	if _, err = calculateNextRun(Schedule{ID: "task:1", Every: time.Second, Anchor: anchor, EndDate: &end, Message: factory}, time.UTC); !errors.Is(err, ErrScheduleExpired) {
		t.Fatal("expected expired schedule error")
	}
}

func TestCalculateNextRunUsesRequestedReferenceTime(t *testing.T) {
	anchor := time.Unix(100, 0).UTC()
	after := anchor.Add(12 * time.Second)
	next, err := CalculateNextRun(Schedule{Every: 5 * time.Second, Anchor: anchor}, after, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if want := anchor.Add(15 * time.Second); !next.Equal(want) {
		t.Fatalf("unexpected next run: got %v want %v", next, want)
	}
}
