package crud

import (
	"context"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/db/recycle"
)

type recycleDeleteManagerStub struct {
	request recycle.DeleteRequest
	called  bool
}

func (s *recycleDeleteManagerStub) RunDelete(_ context.Context, request recycle.DeleteRequest, _ recycle.DeleteWork) error {
	s.called = true
	s.request = request
	return nil
}

func TestRuntimeDefaultDeleteUsesRecycleManager(t *testing.T) {
	manager := &recycleDeleteManagerStub{}
	resource := testUserResource(t)
	runtime := NewRuntime(nil, nil, manager)

	result, err := runtime.DeleteWithData(testPlatformContext(), resource, []interface{}{2, 1, 2}, map[string]interface{}{"reason": "cleanup"})
	if err != nil || result != nil {
		t.Fatalf("managed delete failed: result=%#v err=%v", result, err)
	}
	if !manager.called || manager.request.Resource != resource.Spec.Name || manager.request.Entity != resource.Spec.Model.Name {
		t.Fatalf("unexpected recycle delete request: %#v", manager.request)
	}
	if !reflect.DeepEqual(manager.request.IDs, []interface{}{2, 1}) {
		t.Fatalf("managed delete IDs were not normalized: %#v", manager.request.IDs)
	}
	params, ok := manager.request.Params.(map[string]interface{})
	if !ok || params["reason"] != "cleanup" || !reflect.DeepEqual(params["ids"], []interface{}{2, 1}) {
		t.Fatalf("managed delete params were not preserved: %#v", manager.request.Params)
	}
}
