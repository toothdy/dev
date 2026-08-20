package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/google/uuid"
	"github.com/toothdy/cool-admin-go-next/cool/task"
)

// Payload 描述 Task 模块的执行消息。
type Payload struct {
	TaskID         int64     `json:"taskId"`
	JobID          string    `json:"jobId"`
	TenantID       *int64    `json:"tenantId"`
	ScheduledAt    time.Time `json:"scheduledAt"`
	Manual         bool      `json:"manual"`
	ExecutionID    string    `json:"executionId"`
	Attempt        int       `json:"attempt,omitempty"`
	IsRetryManaged bool      `json:"-"`
}

// Dispatch 消费一条已解码的 Task 消息。
type Dispatch func(ctx context.Context, payload Payload) error

// Encode 将 Task Payload 编码为通用队列消息。
func Encode(payload Payload) (task.Message, error) {
	if payload.ExecutionID == "" {
		payload.ExecutionID = ExecutionID(payload)
	}
	if err := validatePayload(payload); err != nil {
		return task.Message{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return task.Message{}, gerror.Wrap(err, "编码 Task 消息失败")
	}
	return task.Message{ID: deliveryID(payload), Payload: encoded}, nil
}

// EncodeRedelivery 使用新 transport ID 编码同一次业务执行。
func EncodeRedelivery(payload Payload) (task.Message, error) {
	if err := validatePayload(payload); err != nil {
		return task.Message{}, err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return task.Message{}, gerror.Wrap(err, "编码 Task 重投消息失败")
	}
	return task.Message{ID: uuid.NewString(), Payload: encoded, RetryBase: payload.Attempt}, nil
}

// Decode 从通用队列消息解码 Task Payload。
func Decode(message task.Message) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		return Payload{}, gerror.Wrap(err, "解析 Task 消息失败")
	}
	if err := validatePayload(payload); err != nil {
		return Payload{}, err
	}
	return payload, nil
}

// BuildConsumer 创建通用 Queue 到 Task Executor 的适配器。
func BuildConsumer(dispatch Dispatch) (task.Consumer, error) {
	if dispatch == nil {
		return nil, gerror.New("Task 消息消费函数不能为空")
	}
	return func(ctx context.Context, message task.Message) error {
		if message.RetryCount < 0 {
			return task.SkipRetry(gerror.New("Task 消息重试次数无效"))
		}
		payload, err := Decode(message)
		if err != nil {
			return task.SkipRetry(err)
		}
		if payload.Attempt > math.MaxInt-message.RetryCount {
			return task.SkipRetry(gerror.New("Task 消息重试次数溢出"))
		}
		payload.Attempt += message.RetryCount
		payload.IsRetryManaged = message.IsRetryManaged
		if err = dispatch(ctx, payload); task.IsPermanent(err) {
			return task.SkipRetry(err)
		}
		return err
	}, nil
}

// MessageFactory 创建按计划时间生成消息的工厂。
func MessageFactory(payload Payload) task.MessageFactory {
	base := payload
	base.TenantID = cloneTenantID(payload.TenantID)
	return func(scheduledAt time.Time) (task.Message, error) {
		current := base
		current.ScheduledAt = scheduledAt
		current.ExecutionID = ""
		current.TenantID = cloneTenantID(base.TenantID)
		return Encode(current)
	}
}

// ExecutionID 返回同一任务世代和计划时间的稳定执行 ID。
func ExecutionID(payload Payload) string {
	value := fmt.Sprintf("%d:%s:%d", payload.TaskID, payload.JobID, payload.ScheduledAt.UnixNano())
	return strings.ReplaceAll(uuid.NewSHA1(uuid.NameSpaceOID, []byte(value)).String(), "-", "")
}

func deliveryID(payload Payload) string {
	value := "delivery:" + payload.ExecutionID
	return strings.ReplaceAll(uuid.NewSHA1(uuid.NameSpaceOID, []byte(value)).String(), "-", "")
}

func validatePayload(payload Payload) error {
	if payload.TaskID <= 0 {
		return gerror.New("Task 消息缺少 taskId")
	}
	if strings.TrimSpace(payload.JobID) == "" {
		return gerror.New("Task 消息缺少 jobId")
	}
	if payload.ScheduledAt.IsZero() {
		return gerror.New("Task 消息缺少 scheduledAt")
	}
	if strings.TrimSpace(payload.ExecutionID) == "" {
		return gerror.New("Task 消息缺少 executionId")
	}
	if payload.Attempt < 0 {
		return gerror.New("Task 消息 attempt 无效")
	}
	return nil
}

func cloneTenantID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
