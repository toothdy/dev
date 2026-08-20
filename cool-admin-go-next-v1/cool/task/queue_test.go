package task

import (
	"errors"
	"testing"
	"time"
)

func TestSkipRetryClassifiesQueueErrors(t *testing.T) {
	cause := errors.New("invalid queue payload")
	err := SkipRetry(cause)
	if !IsSkipRetry(err) || !errors.Is(err, cause) {
		t.Fatalf("unexpected skip retry error: %v", err)
	}
	if SkipRetry(err) != err {
		t.Fatal("skip retry marker must be idempotent")
	}
	if SkipRetry(nil) != nil || IsSkipRetry(cause) {
		t.Fatal("ordinary errors must remain retryable")
	}
}

func TestBusyRedeliveryCarriesCopiedMessageAndDelay(t *testing.T) {
	payload := []byte("before")
	wantDelay := 25 * time.Millisecond
	err := Busy(errors.New("busy"), wantDelay, Message{ID: "delivery-2", Payload: payload, RetryBase: 2})
	payload[0] = 'x'
	message, delay, isBusy := BusyRedelivery(err)
	if !isBusy || delay != wantDelay || message.ID != "delivery-2" || message.RetryBase != 2 || string(message.Payload) != "before" {
		t.Fatalf("unexpected busy redelivery: message=%#v delay=%v busy=%v", message, delay, isBusy)
	}
}

func TestCloneMessageCopiesPayload(t *testing.T) {
	original := Message{ID: "message-1", Payload: []byte("before")}
	cloned := cloneMessage(original)
	original.Payload[0] = 'x'
	if string(cloned.Payload) != "before" {
		t.Fatalf("message payload shares caller memory: %q", cloned.Payload)
	}
}
