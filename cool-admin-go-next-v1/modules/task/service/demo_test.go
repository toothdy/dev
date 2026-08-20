package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/task"
)

func TestDemoDefinitionKeepsNodeTaskContract(t *testing.T) {
	definition := DemoDefinition()
	if definition.Name != "taskDemoService.test" {
		t.Fatalf("unexpected handler name: %q", definition.Name)
	}
	if definition.Handler == nil {
		t.Fatal("demo handler must be configured")
	}
	result, err := definition.Handler(context.Background(), task.Invocation{
		TaskID:    42,
		Data:      "payload",
		Arguments: []json.RawMessage{json.RawMessage("1"), json.RawMessage(`"two"`)},
	})
	if err != nil {
		t.Fatalf("invoke demo handler: %v", err)
	}
	values, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if values["taskId"] != int64(42) || values["data"] != "payload" {
		t.Fatalf("unexpected handler result: %#v", values)
	}
}
