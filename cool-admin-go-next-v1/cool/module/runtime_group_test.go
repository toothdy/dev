package module_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestRuntimeGroupStartsInDependencyOrderAndStopsInReverseOrder(t *testing.T) {
	calls := []string{}
	group := module.NewRuntimeGroup("task",
		module.RuntimeDefinition{Name: "Queue", Runtime: &runtimeStub{name: "queue", calls: &calls}},
		module.RuntimeDefinition{Name: "Scheduler", Runtime: &runtimeStub{name: "scheduler", calls: &calls}},
	)

	if err := group.Start(context.Background()); err != nil {
		t.Fatalf("start runtime group failed: %v", err)
	}
	if err := group.Stop(context.Background()); err != nil {
		t.Fatalf("stop runtime group failed: %v", err)
	}
	want := []string{"start:queue", "start:scheduler", "stop:scheduler", "stop:queue"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected runtime lifecycle order: %#v", calls)
	}
}

func TestRuntimeGroupRollsBackStartedRuntimesAndNamesFailure(t *testing.T) {
	calls := []string{}
	wantErr := errors.New("scheduler unavailable")
	group := module.NewRuntimeGroup("task",
		module.RuntimeDefinition{Name: "Queue", Runtime: &runtimeStub{name: "queue", calls: &calls}},
		module.RuntimeDefinition{Name: "Scheduler", Runtime: &runtimeStub{name: "scheduler", calls: &calls, startErr: wantErr}},
	)

	err := group.Start(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected startup error, got %v", err)
	}
	if !strings.Contains(err.Error(), "task") || !strings.Contains(err.Error(), "Scheduler") {
		t.Fatalf("startup error should identify module and runtime: %v", err)
	}
	want := []string{"start:queue", "start:scheduler", "stop:queue"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected rollback order: %#v", calls)
	}
}

func TestRuntimeGroupStopContinuesAfterFailureAndNamesRuntime(t *testing.T) {
	calls := []string{}
	wantErr := errors.New("scheduler stop failed")
	group := module.NewRuntimeGroup("task",
		module.RuntimeDefinition{Name: "Queue", Runtime: &runtimeStub{name: "queue", calls: &calls}},
		module.RuntimeDefinition{Name: "Scheduler", Runtime: &runtimeStub{name: "scheduler", calls: &calls, stopErr: wantErr}},
	)

	if err := group.Start(context.Background()); err != nil {
		t.Fatalf("start runtime group failed: %v", err)
	}
	err := group.Stop(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected stop error, got %v", err)
	}
	if !strings.Contains(err.Error(), "task") || !strings.Contains(err.Error(), "Scheduler") {
		t.Fatalf("stop error should identify module and runtime: %v", err)
	}
	want := []string{"start:queue", "start:scheduler", "stop:scheduler", "stop:queue"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("stop should continue in reverse order: %#v", calls)
	}
}

type runtimeStub struct {
	name     string
	calls    *[]string
	startErr error
	stopErr  error
}

func (s *runtimeStub) Start(context.Context) error {
	*s.calls = append(*s.calls, "start:"+s.name)
	return s.startErr
}

func (s *runtimeStub) Stop(context.Context) error {
	*s.calls = append(*s.calls, "stop:"+s.name)
	return s.stopErr
}
