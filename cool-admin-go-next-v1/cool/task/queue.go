package task

import (
	"context"
	"errors"
	"time"
)

// Message 是队列传递的业务无关消息。
type Message struct {
	ID             string
	Payload        []byte
	RetryCount     int
	RetryBase      int
	IsRetryManaged bool
}

// Consumer 消费一条队列消息。
type Consumer func(ctx context.Context, message Message) error

// Queue 提供单次消息投递能力。
type Queue interface {
	Enqueue(ctx context.Context, message Message) error
}

type skipRetryError struct {
	cause error
}

type busyError struct {
	cause   error
	delay   time.Duration
	message Message
}

func (e *skipRetryError) Error() string { return e.cause.Error() }
func (e *skipRetryError) Unwrap() error { return e.cause }

// SkipRetry 将消费错误标记为不再进入队列重试。
func SkipRetry(err error) error {
	if err == nil || IsSkipRetry(err) {
		return err
	}
	return &skipRetryError{cause: err}
}

// IsSkipRetry 判断消费错误是否禁止队列重试。
func IsSkipRetry(err error) bool {
	var target *skipRetryError
	return errors.As(err, &target)
}

func (e *busyError) Error() string { return e.cause.Error() }
func (e *busyError) Unwrap() error { return e.cause }

// Busy 将消费结果标记为需要使用新 transport ID 延迟重投。
func Busy(err error, delay time.Duration, message Message) error {
	if err == nil || delay <= 0 || message.ID == "" {
		return err
	}
	return &busyError{cause: err, delay: delay, message: cloneMessage(message)}
}

// BusyRedelivery 读取类型化 busy 携带的延迟重投消息。
func BusyRedelivery(err error) (Message, time.Duration, bool) {
	var target *busyError
	if !errors.As(err, &target) {
		return Message{}, 0, false
	}
	return cloneMessage(target.message), target.delay, true
}

func cloneMessage(message Message) Message {
	message.Payload = append([]byte(nil), message.Payload...)
	return message
}
