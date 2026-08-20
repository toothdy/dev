package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/toothdy/cool-admin-go-next/modules/recycle"
)

type serviceStub struct {
	calls atomic.Int64
}

var _ func(Service, recycle.Config) *Data = NewData

func (s *serviceStub) ClearExpired(context.Context) (int, error) {
	s.calls.Add(1)
	return 0, nil
}

func (s *serviceStub) Restore(context.Context, []int64) error {
	return nil
}

func TestDataRuntimeStartsAndStopsCleanup(t *testing.T) {
	service := &serviceStub{}
	runtime := NewData(service, recycle.Config{CleanupInterval: time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := runtime.Start(ctx); err != nil {
		t.Fatalf("start runtime failed: %v", err)
	}
	deadline := time.Now().Add(100 * time.Millisecond)
	for service.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.calls.Load() == 0 {
		t.Fatal("cleanup did not run")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := runtime.Stop(stopCtx); err != nil {
		t.Fatalf("stop runtime failed: %v", err)
	}
}
