package task

import (
	"context"
	"errors"
	"testing"
	"time"
)

type schedulerStub struct {
	startErr  error
	healthErr error
	starts    int
	stops     int
}

func (s *schedulerStub) Start(context.Context) error                         { s.starts++; return s.startErr }
func (s *schedulerStub) Healthy(context.Context) error                       { return s.healthErr }
func (s *schedulerStub) Upsert(context.Context, Schedule) (time.Time, error) { return time.Now(), nil }
func (s *schedulerStub) Remove(context.Context, string) error                { return nil }
func (s *schedulerStub) Enqueue(context.Context, Message) error              { return nil }
func (s *schedulerStub) NextRunTime(string) (time.Time, bool)                { return time.Time{}, false }
func (s *schedulerStub) Stop(context.Context) error                          { s.stops++; return nil }

func TestAutoSchedulerFallsBackOnlyDuringStartup(t *testing.T) {
	primary := &schedulerStub{startErr: errors.New("redis unavailable")}
	fallback := &schedulerStub{}
	scheduler, err := NewAutoScheduler(primary, fallback)
	if err != nil {
		t.Fatal(err)
	}
	if err = scheduler.Start(context.Background()); err != nil {
		t.Fatalf("start auto scheduler failed: %v", err)
	}
	if primary.starts != 1 || primary.stops != 1 || fallback.starts != 1 {
		t.Fatalf("unexpected startup calls: primary=%#v fallback=%#v", primary, fallback)
	}
	primary.startErr = nil
	fallback.healthErr = errors.New("local unavailable")
	if err = scheduler.Healthy(context.Background()); !errors.Is(err, fallback.healthErr) {
		t.Fatalf("runtime health must stay on selected fallback: %v", err)
	}
}
