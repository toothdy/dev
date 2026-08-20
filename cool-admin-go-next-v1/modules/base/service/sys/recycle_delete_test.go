package sys

import (
	"testing"

	baseModel "github.com/toothdy/cool-admin-go-next/modules/base/entity/sys"
)

func TestServiceConstructorKeepsNilRecycleManagerDisabled(t *testing.T) {
	service := NewParamService(nil, baseModel.BaseSysParam(), nil)
	if service.recycle != nil {
		t.Fatal("nil Manager 不应启用受管删除")
	}
}
