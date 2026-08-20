package service

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool/task"
)

// DemoHandler 返回与 Node TaskDemoService 对齐的演示结果。
func DemoHandler(_ context.Context, invocation task.Invocation) (interface{}, error) {
	return map[string]interface{}{
		"taskId": invocation.TaskID,
		"data":   invocation.Data,
		"args":   invocation.Arguments,
	}, nil
}

// DemoDefinition 返回与 Node TaskDemoService 对齐的演示任务定义。
func DemoDefinition() task.HandlerDefinition {
	return task.HandlerDefinition{
		Name:    "taskDemoService.test",
		Handler: DemoHandler,
	}
}
