package event

import (
	"context"
	"errors"
	"testing"
)

type recycleReconciler struct {
	calls int
	err   error
}

func (r *recycleReconciler) Reconcile(context.Context) error {
	r.calls++
	return r.err
}

func TestTaskRestoreHookReconcilesEngine(t *testing.T) {
	expected := errors.New("reconcile failed")
	reconciler := &recycleReconciler{err: expected}
	err := taskRestoreHook(reconciler)(context.Background(), "task.info")
	if !errors.Is(err, expected) || reconciler.calls != 1 {
		t.Fatalf("任务恢复 Hook 未透传 Engine 对账结果: calls=%d err=%v", reconciler.calls, err)
	}
}
