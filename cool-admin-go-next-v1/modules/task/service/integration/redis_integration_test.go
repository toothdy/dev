package integration_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
	taskService "github.com/toothdy/cool-admin-go-next/modules/task/service"
)

const taskRedisIntegrationWaitTimeout = 10 * time.Second

func TestTaskRedisWorkerRoundTrip(t *testing.T) {
	if os.Getenv("COOL_TASK_REDIS_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_REDIS_INTEGRATION=1 to run Task Redis integration tests")
	}
	ctx := context.Background()
	client, configured, err := loadTaskRedisClient(ctx)
	if err != nil {
		t.Fatalf("load Redis config failed: %v", err)
	}
	if !configured {
		t.Skip("redis.default is not configured")
	}
	t.Cleanup(func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("close Redis client failed: %v", closeErr)
		}
	})
	dispatched := make(chan taskQueue.Payload, 1)
	consumer, err := taskQueue.BuildConsumer(func(_ context.Context, payload taskQueue.Payload) error {
		dispatched <- payload
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	namespace := uuid.NewString()
	scheduler, err := taskService.BuildRedisScheduler(taskService.RedisOptions{
		Client: client, Concurrency: 1, MaxRetry: 1, RetryDelay: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, DelayedTaskCheckInterval: 20 * time.Millisecond,
		Location: time.UTC, Consumer: consumer, Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if stopErr := scheduler.Stop(stopContext); stopErr != nil {
			t.Errorf("stop Redis scheduler failed: %v", stopErr)
		}
		cleanupTaskRedisNamespace(t, client, namespace)
	})
	if err = scheduler.Start(ctx); err != nil {
		t.Fatalf("start Redis scheduler failed: %v", err)
	}
	payload := taskQueue.Payload{
		TaskID: 1, JobID: uuid.NewString(), ScheduledAt: time.Now(), Manual: true, ExecutionID: uuid.NewString(),
	}
	message, err := taskQueue.Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Enqueue(ctx, message); err != nil {
		t.Fatalf("enqueue Redis task failed: %v", err)
	}
	select {
	case received := <-dispatched:
		if received.JobID != payload.JobID || !received.Manual {
			t.Fatalf("unexpected Redis payload: %#v", received)
		}
	case <-time.After(taskRedisIntegrationWaitTimeout):
		t.Fatal("timed out waiting for Redis worker")
	}
}

func TestTaskRedisBusyRedeliveryPreservesExecutionIdentityWithZeroRetryBudget(t *testing.T) {
	if os.Getenv("COOL_TASK_REDIS_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_REDIS_INTEGRATION=1 to run Task Redis integration tests")
	}
	ctx := context.Background()
	client, configured, err := loadTaskRedisClient(ctx)
	if err != nil {
		t.Fatalf("load Redis config failed: %v", err)
	}
	if !configured {
		t.Skip("redis.default is not configured")
	}
	t.Cleanup(func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("close Redis client failed: %v", closeErr)
		}
	})
	type busyDelivery struct {
		transportID string
		payload     taskQueue.Payload
		redelivery  task.Message
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseHandler := func() {
		releaseOnce.Do(func() { close(release) })
	}
	firstBusyReturned := make(chan busyDelivery, 1)
	completed := make(chan busyDelivery, 1)
	var firstBusyReturnedOnce sync.Once
	var completedOnce sync.Once
	taskConsumer, err := taskQueue.BuildConsumer(func(_ context.Context, payload taskQueue.Payload) error {
		select {
		case <-release:
			return nil
		default:
			message, encodeErr := taskQueue.EncodeRedelivery(payload)
			if encodeErr != nil {
				return encodeErr
			}
			return task.Busy(taskService.ErrTaskBusy, 30*time.Millisecond, message)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	consumer := func(consumerContext context.Context, message task.Message) error {
		payload, decodeErr := taskQueue.Decode(message)
		if decodeErr != nil {
			return decodeErr
		}
		consumerErr := taskConsumer(consumerContext, message)
		if redelivery, _, isBusy := task.BusyRedelivery(consumerErr); isBusy {
			firstBusyReturnedOnce.Do(func() {
				select {
				case firstBusyReturned <- busyDelivery{transportID: message.ID, payload: payload, redelivery: redelivery}:
				default:
				}
			})
		} else if consumerErr == nil {
			completedOnce.Do(func() {
				select {
				case completed <- busyDelivery{transportID: message.ID, payload: payload}:
				default:
				}
			})
		}
		return consumerErr
	}
	namespace := uuid.NewString()
	scheduler, err := taskService.BuildRedisScheduler(taskService.RedisOptions{
		Client: client, Concurrency: 1, MaxRetry: 0, RetryDelay: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, DelayedTaskCheckInterval: 20 * time.Millisecond,
		Location: time.UTC, Consumer: consumer, Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if stopErr := scheduler.Stop(stopContext); stopErr != nil {
			t.Errorf("stop Redis scheduler failed: %v", stopErr)
		}
		cleanupTaskRedisNamespace(t, client, namespace)
	})
	t.Cleanup(releaseHandler)
	if err = scheduler.Start(ctx); err != nil {
		t.Fatalf("start Redis scheduler failed: %v", err)
	}
	payload := taskQueue.Payload{
		TaskID: 2, JobID: uuid.NewString(), ScheduledAt: time.Now(), Manual: true,
		ExecutionID: uuid.NewString(), Attempt: 2,
	}
	message, err := taskQueue.Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Enqueue(ctx, message); err != nil {
		t.Fatalf("enqueue Redis busy task failed: %v", err)
	}
	var first busyDelivery
	select {
	case first = <-firstBusyReturned:
	case <-time.After(taskRedisIntegrationWaitTimeout):
		t.Fatal("timed out waiting for the first Redis consumer to return busy")
	}
	if first.transportID != message.ID || first.payload.ExecutionID != payload.ExecutionID || first.payload.Attempt != payload.Attempt ||
		first.redelivery.RetryBase != payload.Attempt || first.redelivery.ID == first.transportID {
		t.Fatalf("首次 Redis busy delivery 身份异常: first=%#v payload=%#v", first, payload)
	}
	releaseHandler()
	var second busyDelivery
	select {
	case second = <-completed:
	case <-time.After(taskRedisIntegrationWaitTimeout):
		t.Fatal("timed out waiting for Redis busy redelivery")
	}
	if second.transportID != first.redelivery.ID || second.payload.ExecutionID != payload.ExecutionID ||
		second.payload.Attempt != payload.Attempt || first.redelivery.RetryBase != second.payload.Attempt {
		t.Fatalf("Redis busy 重投改变了执行身份: first=%#v second=%#v", first, second)
	}
	queueName := "cool-admin-go-next-task-v1:" + namespace
	inspector := asynq.NewInspectorFromRedisClient(client)
	queueInfo := waitForTaskRedisQueue(t, inspector, queueName, func(info *asynq.QueueInfo) bool {
		return info.Active == 0 && info.Pending == 0 && info.Scheduled == 0 && info.Retry == 0
	})
	if queueInfo.Archived != 0 {
		t.Fatalf("maxRetry=0 的 busy delivery 被错误归档: %#v", queueInfo)
	}
}

func TestTaskRedisShutdownTimeoutRequeuesActiveDeliveryWithoutAttemptChange(t *testing.T) {
	if os.Getenv("COOL_TASK_REDIS_INTEGRATION") != "1" {
		t.Skip("set COOL_TASK_REDIS_INTEGRATION=1 to run Task Redis integration tests")
	}
	ctx := context.Background()
	client, configured, err := loadTaskRedisClient(ctx)
	if err != nil {
		t.Fatalf("load Redis config failed: %v", err)
	}
	if !configured {
		t.Skip("redis.default is not configured")
	}
	t.Cleanup(func() {
		if closeErr := client.Close(); closeErr != nil {
			t.Errorf("close Redis client failed: %v", closeErr)
		}
	})
	namespace := uuid.NewString()
	queueName := "cool-admin-go-next-task-v1:" + namespace
	t.Cleanup(func() { cleanupTaskRedisNamespace(t, client, namespace) })

	firstStarted := make(chan taskQueue.Payload, 1)
	firstCanceled := make(chan error, 1)
	firstConsumer, err := taskQueue.BuildConsumer(func(handlerContext context.Context, payload taskQueue.Payload) error {
		firstStarted <- payload
		<-handlerContext.Done()
		firstCanceled <- handlerContext.Err()
		return handlerContext.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	firstScheduler, err := taskService.BuildRedisScheduler(taskService.RedisOptions{
		Client: client, Concurrency: 1, MaxRetry: 3, RetryDelay: 10 * time.Millisecond,
		ShutdownTimeout: 300 * time.Millisecond, DelayedTaskCheckInterval: 20 * time.Millisecond,
		Location: time.UTC, Consumer: firstConsumer, Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if stopErr := firstScheduler.Stop(stopContext); stopErr != nil {
			t.Errorf("stop first Redis scheduler failed: %v", stopErr)
		}
	})
	if err = firstScheduler.Start(ctx); err != nil {
		t.Fatalf("start first Redis scheduler failed: %v", err)
	}
	payload := taskQueue.Payload{
		TaskID: 3, JobID: uuid.NewString(), ScheduledAt: time.Now(), Manual: true,
		ExecutionID: uuid.NewString(), Attempt: 2,
	}
	message, err := taskQueue.Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = firstScheduler.Enqueue(ctx, message); err != nil {
		t.Fatalf("enqueue Redis shutdown task failed: %v", err)
	}
	select {
	case received := <-firstStarted:
		assertTaskRedisBusinessIdentity(t, received, payload)
	case <-time.After(taskRedisIntegrationWaitTimeout):
		t.Fatal("timed out waiting for the first Redis delivery")
	}
	inspector := asynq.NewInspectorFromRedisClient(client)
	active := waitForTaskRedisState(t, inspector, queueName, message.ID, asynq.TaskStateActive)
	if active.Retried != 0 {
		t.Fatalf("首次 active delivery 已增加 Asynq retry: %#v", active)
	}
	stopContext, cancelStop := context.WithTimeout(context.Background(), 3*time.Second)
	err = firstScheduler.Stop(stopContext)
	cancelStop()
	if err != nil {
		t.Fatalf("stop first Redis scheduler failed: %v", err)
	}
	select {
	case cancelErr := <-firstCanceled:
		if !errors.Is(cancelErr, context.Canceled) {
			t.Fatalf("ShutdownTimeout 未取消 active handler Context: %v", cancelErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for ShutdownTimeout to cancel the active handler")
	}
	pending := waitForTaskRedisState(t, inspector, queueName, message.ID, asynq.TaskStatePending)
	if pending.Retried != 0 || pending.MaxRetry != 3 {
		t.Fatalf("ShutdownTimeout 回推改变了 transport 重试身份: %#v", pending)
	}

	type completedDelivery struct {
		transportID string
		payload     taskQueue.Payload
		retryCount  int
	}
	completed := make(chan completedDelivery, 1)
	var completedOnce sync.Once
	secondTaskConsumer, err := taskQueue.BuildConsumer(func(_ context.Context, _ taskQueue.Payload) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	secondConsumer := func(consumerContext context.Context, received task.Message) error {
		decoded, decodeErr := taskQueue.Decode(received)
		if decodeErr != nil {
			return decodeErr
		}
		consumerErr := secondTaskConsumer(consumerContext, received)
		if consumerErr == nil {
			completedOnce.Do(func() {
				select {
				case completed <- completedDelivery{transportID: received.ID, payload: decoded, retryCount: received.RetryCount}:
				default:
				}
			})
		}
		return consumerErr
	}
	secondScheduler, err := taskService.BuildRedisScheduler(taskService.RedisOptions{
		Client: client, Concurrency: 1, MaxRetry: 3, RetryDelay: 10 * time.Millisecond,
		ShutdownTimeout: time.Second, DelayedTaskCheckInterval: 20 * time.Millisecond,
		Location: time.UTC, Consumer: secondConsumer, Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if stopErr := secondScheduler.Stop(stopContext); stopErr != nil {
			t.Errorf("stop second Redis scheduler failed: %v", stopErr)
		}
	})
	if err = secondScheduler.Start(ctx); err != nil {
		t.Fatalf("start second Redis scheduler failed: %v", err)
	}
	select {
	case received := <-completed:
		if received.transportID != message.ID || received.retryCount != 0 {
			t.Fatalf("ShutdownTimeout 后未消费同一原 delivery: %#v", received)
		}
		assertTaskRedisBusinessIdentity(t, received.payload, payload)
	case <-time.After(taskRedisIntegrationWaitTimeout):
		t.Fatal("timed out waiting for the requeued Redis delivery")
	}
}

func assertTaskRedisBusinessIdentity(t *testing.T, received taskQueue.Payload, want taskQueue.Payload) {
	t.Helper()
	if received.TaskID != want.TaskID || received.JobID != want.JobID || received.ExecutionID != want.ExecutionID ||
		received.Attempt != want.Attempt || received.Manual != want.Manual || !received.ScheduledAt.Equal(want.ScheduledAt) {
		t.Fatalf("Redis delivery 改变了业务身份: received=%#v want=%#v", received, want)
	}
}

func waitForTaskRedisState(
	t *testing.T,
	inspector *asynq.Inspector,
	queueName string,
	transportID string,
	want asynq.TaskState,
) *asynq.TaskInfo {
	t.Helper()
	deadline := time.NewTimer(taskRedisIntegrationWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := inspector.GetTaskInfo(queueName, transportID)
		if err == nil && info.State == want {
			return info
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for Redis task %q state %s: info=%#v err=%v", transportID, want, info, err)
		case <-ticker.C:
		}
	}
}

func waitForTaskRedisQueue(
	t *testing.T,
	inspector *asynq.Inspector,
	queueName string,
	ready func(*asynq.QueueInfo) bool,
) *asynq.QueueInfo {
	t.Helper()
	deadline := time.NewTimer(taskRedisIntegrationWaitTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := inspector.GetQueueInfo(queueName)
		if err == nil && ready(info) {
			return info
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out waiting for Redis queue %q: info=%#v err=%v", queueName, info, err)
		case <-ticker.C:
		}
	}
}

func cleanupTaskRedisNamespace(t *testing.T, client redis.UniversalClient, namespace string) {
	t.Helper()
	queueName := "cool-admin-go-next-task-v1:" + namespace
	leaseKey := "cool-admin-go-next:task:v1:scheduler-lease:" + namespace
	inspector := asynq.NewInspectorFromRedisClient(client)
	if err := inspector.DeleteQueue(queueName, true); err != nil && !errors.Is(err, asynq.ErrQueueNotFound) {
		t.Errorf("cleanup Redis integration queue %q failed: %v", queueName, err)
	}
	queuePattern := "asynq:{" + queueName + "}:*"
	var cursor uint64
	for {
		keys, nextCursor, err := client.Scan(context.Background(), cursor, queuePattern, 100).Result()
		if err != nil {
			t.Errorf("scan Redis integration queue %q failed: %v", queueName, err)
			break
		}
		if len(keys) > 0 {
			if err = client.Del(context.Background(), keys...).Err(); err != nil {
				t.Errorf("delete Redis integration queue keys %q failed: %v", queueName, err)
				break
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}
	if err := client.SRem(context.Background(), "asynq:queues", queueName).Err(); err != nil {
		t.Errorf("cleanup Redis integration queue metadata %q failed: %v", queueName, err)
	}
	if err := client.Del(context.Background(), leaseKey).Err(); err != nil {
		t.Errorf("cleanup Redis integration lease %q failed: %v", leaseKey, err)
	}
}
