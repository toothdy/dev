package integration_test

import (
	"context"
	"testing"

	"github.com/toothdy/cool-admin-go-next/modules"
)

func TestGeneratedModuleKeepsTaskContract(t *testing.T) {
	var specFound bool
	var specIndex int
	specs := modules.Specs()
	for index, spec := range specs {
		if spec.Key == "task" {
			specFound = true
			specIndex = index
			break
		}
	}
	if !specFound {
		t.Fatalf("generated module specs do not contain Task: %#v", specs)
	}
	spec := specs[specIndex]
	if spec.Key != "task" || spec.DB != "modules/task/db.json" {
		t.Fatalf("unexpected generated Task module: %#v", spec)
	}
	if len(spec.Models) != 2 || spec.Models[0].TableName != "task_info" || spec.Models[1].TableName != "task_log" {
		t.Fatalf("unexpected generated Task models: %#v", spec.Models)
	}
	if err := spec.Configure(context.Background()); err != nil {
		t.Fatalf("configure Task spec: %v", err)
	}
	if len(spec.TaskHandlers) != 1 || spec.TaskHandlers[0].Name != "taskDemoService.test" || spec.TaskHandlers[0].Handler == nil {
		t.Fatalf("unexpected generated Task handlers: %#v", spec.TaskHandlers)
	}
	if spec.Runtime == nil || spec.Controllers == nil || spec.Middlewares == nil {
		t.Fatal("generated Task factories must be configured")
	}
}
