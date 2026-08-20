package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/task"
)

func TestEncodeKeepsSixFieldsAndStableExecutionID(t *testing.T) {
	tenantID := int64(9)
	payload := Payload{
		TaskID: 7, JobID: "job-1", TenantID: &tenantID,
		ScheduledAt: time.Unix(100, 200).UTC(), Manual: true,
	}
	first, err := Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Encode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("execution ID 必须稳定: %q %q", first.ID, second.ID)
	}
	decoded, err := Decode(first)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.TaskID != payload.TaskID || decoded.JobID != payload.JobID || decoded.TenantID == nil || *decoded.TenantID != tenantID || !decoded.ScheduledAt.Equal(payload.ScheduledAt) || !decoded.Manual || decoded.ExecutionID != ExecutionID(payload) {
		t.Fatalf("Task 消息字段不完整: %#v", decoded)
	}
	if decoded.ExecutionID == first.ID {
		t.Fatal("业务 ExecutionID 与 transport ID 必须分离")
	}
}

func TestRedeliveryKeepsExecutionAndAttemptWithFreshTransportID(t *testing.T) {
	payload := Payload{
		TaskID: 7, JobID: "job-redelivery", ScheduledAt: time.Now(), Manual: true,
		ExecutionID: "execution-stable", Attempt: 2,
	}
	first, err := EncodeRedelivery(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EncodeRedelivery(payload)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.RetryBase != payload.Attempt || second.RetryBase != payload.Attempt {
		t.Fatalf("重投 transport 元数据异常: first=%#v second=%#v", first, second)
	}
	decoded, err := Decode(first)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ExecutionID != payload.ExecutionID || decoded.Attempt != payload.Attempt {
		t.Fatalf("重投改变了业务执行身份: %#v", decoded)
	}
}

func TestMessageFactoryUsesScheduledTimeForExecutionID(t *testing.T) {
	factory := MessageFactory(Payload{TaskID: 8, JobID: "job-2"})
	firstTime := time.Unix(100, 0).UTC()
	first, err := factory(firstTime)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := factory(firstTime)
	if err != nil {
		t.Fatal(err)
	}
	next, err := factory(firstTime.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != repeated.ID || first.ID == next.ID {
		t.Fatalf("计划批次 execution ID 异常: %q %q %q", first.ID, repeated.ID, next.ID)
	}
}

func TestExecutionIDKeepsAdjacent1500MillisecondBatchesDistinct(t *testing.T) {
	firstTime := time.Unix(100, 500*int64(time.Millisecond)).UTC()
	first := ExecutionID(Payload{TaskID: 8, JobID: "job-1500", ScheduledAt: firstTime})
	next := ExecutionID(Payload{TaskID: 8, JobID: "job-1500", ScheduledAt: firstTime.Add(1500 * time.Millisecond)})
	if first == next {
		t.Fatalf("相邻 1500ms 批次键发生碰撞: %q", first)
	}
}

func TestConsumerMapsInvalidAndPermanentErrorsToSkipRetry(t *testing.T) {
	var (
		receivedAttempt int
		isRetryManaged  bool
	)
	consumer, err := BuildConsumer(func(_ context.Context, payload Payload) error {
		receivedAttempt = payload.Attempt
		isRetryManaged = payload.IsRetryManaged
		return task.Permanent(errors.New("invalid handler"))
	})
	if err != nil {
		t.Fatal(err)
	}
	if consumeErr := consumer(context.Background(), task.Message{ID: "bad", Payload: []byte("{")}); !task.IsSkipRetry(consumeErr) {
		t.Fatalf("非法消息必须跳过重试: %v", consumeErr)
	}
	message, err := Encode(Payload{TaskID: 1, JobID: "job", ScheduledAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	message.RetryCount = 2
	message.IsRetryManaged = true
	if consumeErr := consumer(context.Background(), message); !task.IsSkipRetry(consumeErr) || !task.IsPermanent(consumeErr) {
		t.Fatalf("永久错误必须跳过队列重试: %v", consumeErr)
	}
	if receivedAttempt != 2 {
		t.Fatalf("队列重试次数未传给 Executor: %d", receivedAttempt)
	}
	if !isRetryManaged {
		t.Fatal("队列管理的重试标记未传给 Executor")
	}
}
