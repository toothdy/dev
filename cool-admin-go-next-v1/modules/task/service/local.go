package service

import (
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/task"
)

// LocalOptions 描述 Task 本地调度后端参数。
type LocalOptions struct {
	Concurrency int
	Location    *time.Location
	Consumer    task.Consumer
}

// BuildLocalScheduler 构建 Task 本地调度后端。
func BuildLocalScheduler(options LocalOptions) (task.Scheduler, error) {
	return task.NewLocalScheduler(options.Concurrency, options.Location, options.Consumer)
}
