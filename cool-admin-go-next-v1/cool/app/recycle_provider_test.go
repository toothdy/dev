package app

import (
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestRecycleProviderIndexRequiresExactlyOneProviderWhenEnabled(t *testing.T) {
	if _, err := recycleProviderIndex(nil, true); err == nil || !strings.Contains(err.Error(), "softDelete") {
		t.Fatalf("expected missing required recycle provider rejected, got %v", err)
	}
	if index, err := recycleProviderIndex(nil, false); err != nil || index != -1 {
		t.Fatalf("optional missing recycle provider should be accepted: index=%d err=%v", index, err)
	}
	provider := module.RecycleProvider(func(module.RuntimeDeps) (*recycle.Manager, module.Runtime, error) {
		return nil, nil, nil
	})
	if index, err := recycleProviderIndex([]module.Spec{{RecycleProvider: provider}}, true); err != nil || index != 0 {
		t.Fatalf("single recycle provider was not selected: index=%d err=%v", index, err)
	}
	if _, err := recycleProviderIndex([]module.Spec{
		{RecycleProvider: provider}, {RecycleProvider: provider},
	}, true); err == nil || !strings.Contains(err.Error(), "只能注册一个") {
		t.Fatalf("expected duplicate recycle provider rejected, got %v", err)
	}
}
