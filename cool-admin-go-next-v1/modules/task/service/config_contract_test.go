package service_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
)

func TestConfigKeepsTaskContract(t *testing.T) {
	declaration := taskModule.ModuleConfig()
	if declaration.Name != "任务调度" || declaration.Description != "任务调度模块，支持分布式任务，由redis整个集群的任务" || declaration.Order != 0 {
		t.Fatalf("unexpected Task declaration: %#v", declaration)
	}
	if len(declaration.Middlewares) != 1 || declaration.Middlewares[0] != "middleware#Definition" {
		t.Fatalf("unexpected Task middlewares: %#v", declaration.Middlewares)
	}
	if len(declaration.GlobalMiddlewares) != 0 {
		t.Fatalf("unexpected Task global middlewares: %#v", declaration.GlobalMiddlewares)
	}
	assertTaskDefaults(t, declaration.Defaults)
}

func TestTaskConfigContainsOnlyTaggedPureValues(t *testing.T) {
	configType := reflect.TypeFor[taskModule.Config]()
	wantFields := []struct {
		name string
		tag  string
	}{
		{name: "Mode", tag: "mode"},
		{name: "Timezone", tag: "timezone"},
		{name: "Log", tag: "log"},
		{name: "Execution", tag: "execution"},
		{name: "Queue", tag: "queue"},
	}
	if configType.NumField() != len(wantFields) {
		t.Fatalf("Task Config must contain only pure business fields: %s", configType)
	}
	for index, want := range wantFields {
		field := configType.Field(index)
		if field.Name != want.name || field.Tag.Get("json") != want.tag {
			t.Fatalf("unexpected Task Config field %d: %#v", index, field)
		}
	}
	if _, exists := configType.MethodByName("Validate"); !exists {
		t.Fatal("Task Config Validate must use a value receiver")
	}
}

func TestTaskConfigUsesModuleNamespaceOnly(t *testing.T) {
	withTaskConfig(t, `task:
  mode: local
  log:
    keepDays: 1`)
	config, err := module.LoadConfig(context.Background(), "task", taskModule.ModuleConfig().Defaults)
	if err != nil {
		t.Fatalf("load Task defaults: %v", err)
	}
	assertTaskDefaults(t, config)

	withTaskConfig(t, `module:
  task:
    mode: local
    queue:
      maxRetry: 0`)
	config, err = module.LoadConfig(context.Background(), "task", taskModule.ModuleConfig().Defaults)
	if err != nil {
		t.Fatalf("load module.task config: %v", err)
	}
	if config.Mode != taskModule.ModeLocal || config.Queue.MaxRetry != 0 {
		t.Fatalf("explicit module.task values were not preserved: %#v", config)
	}
}

func TestTaskConfigRejectsInvalidValuesWithModulePath(t *testing.T) {
	withTaskConfig(t, `module:
  task:
    execution:
      timeout: 6m
      lockTTL: 5m`)
	_, err := module.LoadConfig(context.Background(), "task", taskModule.ModuleConfig().Defaults)
	if err == nil || !strings.Contains(err.Error(), "module.task.execution.lockTTL") {
		t.Fatalf("expected module.task lease validation error, got %v", err)
	}
}

func assertTaskDefaults(t *testing.T, config taskModule.Config) {
	t.Helper()
	if config.Mode != taskModule.ModeAuto || config.Timezone != "Asia/Shanghai" || config.Log.KeepDays != 20 {
		t.Fatalf("unexpected Task defaults: %#v", config)
	}
	if config.Execution.Timeout != 5*time.Minute || config.Execution.LockTTL != 6*time.Minute {
		t.Fatalf("unexpected Task execution defaults: %#v", config.Execution)
	}
	if config.Queue.Concurrency != 10 || config.Queue.MaxRetry != 3 || config.Queue.RetryDelay != 5*time.Second || config.Queue.ShutdownTimeout != 30*time.Second {
		t.Fatalf("unexpected Task queue defaults: %#v", config.Queue)
	}
}

func withTaskConfig(t *testing.T, content string) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatalf("create config adapter: %v", err)
	}
	config := g.Cfg()
	previous := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() { config.SetAdapter(previous) })
}
