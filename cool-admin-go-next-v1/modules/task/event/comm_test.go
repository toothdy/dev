package event

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	_ "github.com/toothdy/cool-admin-go-next/cool/db/driver"
	"github.com/toothdy/cool-admin-go-next/cool/task"
	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
	"github.com/toothdy/cool-admin-go-next/modules/task/entity"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

func TestNewCommBuildsRuntimeWithoutStartingDatabaseWork(t *testing.T) {
	runtime, err := NewComm(g.DB(), entity.TaskInfo(), entity.TaskLog(), []task.HandlerDefinition{{
		Name: "taskDemoService.test",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			return nil, nil
		},
	}}, testConfig(), time.UTC, taskQueue.RedisClient{}, nil)
	if err != nil {
		t.Fatalf("build Task runtime failed: %v", err)
	}
	if runtime.Info() == nil {
		t.Fatal("expected InfoService to be available")
	}
	if err = runtime.Stop(context.Background()); err != nil {
		t.Fatalf("stop inactive Task runtime failed: %v", err)
	}
}

func TestNewCommRejectsHandlerTimeoutOutsideLease(t *testing.T) {
	_, err := NewComm(g.DB(), entity.TaskInfo(), entity.TaskLog(), []task.HandlerDefinition{{
		Name: "taskDemoService.test", Timeout: 2 * time.Minute,
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			return nil, nil
		},
	}}, testConfig(), time.UTC, taskQueue.RedisClient{}, nil)
	if err == nil {
		t.Fatal("expected handler timeout to be rejected")
	}
}

func TestNewCommRejectsRedisModeWithoutDefaultRedis(t *testing.T) {
	config := testConfig()
	config.Mode = taskModule.ModeRedis
	_, err := NewComm(g.DB(), entity.TaskInfo(), entity.TaskLog(), []task.HandlerDefinition{{
		Name: "taskDemoService.test",
		Handler: func(context.Context, task.Invocation) (interface{}, error) {
			return nil, nil
		},
	}}, config, time.UTC, taskQueue.RedisClient{}, nil)
	if err == nil {
		t.Fatal("expected Redis mode without redis.default to fail")
	}
}

func testConfig() taskModule.Config {
	return taskModule.Config{
		Mode: taskModule.ModeLocal, Timezone: "UTC", Log: taskModule.LogConfig{KeepDays: 20},
		Execution: taskModule.ExecutionConfig{Timeout: time.Minute, LockTTL: 2 * time.Minute},
		Queue: taskModule.QueueConfig{
			Concurrency: 1, MaxRetry: 0, RetryDelay: time.Millisecond, ShutdownTimeout: time.Second,
		},
	}
}
