package task

import (
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

// Mode 描述 Task 模块使用的调度后端模式。
type Mode string

const (
	ModeAuto               Mode = "auto"
	ModeLocal              Mode = "local"
	ModeRedis              Mode = "redis"
	defaultTimeout              = 5 * time.Minute
	defaultLockTTL              = 6 * time.Minute
	defaultRetryDelay           = 5 * time.Second
	defaultShutdownTimeout      = 30 * time.Second
)

// Config 描述 Task 模块的纯业务配置。
type Config struct {
	Mode      Mode            `json:"mode"`
	Timezone  string          `json:"timezone"`
	Log       LogConfig       `json:"log"`
	Execution ExecutionConfig `json:"execution"`
	Queue     QueueConfig     `json:"queue"`
}

// LogConfig 描述任务日志保留配置。
type LogConfig struct {
	KeepDays int `json:"keepDays"`
}

// ExecutionConfig 描述任务执行超时和租约配置。
type ExecutionConfig struct {
	Timeout time.Duration `json:"timeout"`
	LockTTL time.Duration `json:"lockTTL"`
}

// QueueConfig 描述任务队列运行配置。
type QueueConfig struct {
	Concurrency     int           `json:"concurrency"`
	MaxRetry        int           `json:"maxRetry"`
	RetryDelay      time.Duration `json:"retryDelay"`
	ShutdownTimeout time.Duration `json:"shutdownTimeout"`
}

// Validate 校验 Task 模块配置。
func (c Config) Validate() error {
	switch c.Mode {
	case ModeAuto, ModeLocal, ModeRedis:
	default:
		return gerror.New("module.task.mode 只支持 auto、local 或 redis")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return gerror.Wrap(err, "module.task.timezone 无效")
	}
	if c.Log.KeepDays <= 0 {
		return gerror.New("module.task.log.keepDays 必须大于 0")
	}
	if c.Execution.Timeout <= 0 {
		return gerror.New("module.task.execution.timeout 必须大于 0")
	}
	if c.Execution.LockTTL-c.Execution.Timeout < time.Second {
		return gerror.New("module.task.execution.lockTTL 必须至少比 timeout 长 1 秒")
	}
	if c.Queue.Concurrency <= 0 {
		return gerror.New("module.task.queue.concurrency 必须大于 0")
	}
	if c.Queue.MaxRetry < 0 {
		return gerror.New("module.task.queue.maxRetry 不能小于 0")
	}
	if c.Queue.RetryDelay <= 0 {
		return gerror.New("module.task.queue.retryDelay 必须大于 0")
	}
	if c.Queue.ShutdownTimeout <= 0 {
		return gerror.New("module.task.queue.shutdownTimeout 必须大于 0")
	}
	return nil
}

// ModuleConfig 声明 Task 模块元信息、中间件和默认配置。
func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "任务调度",
		Description: "任务调度模块，支持分布式任务，由redis整个集群的任务",
		Middlewares: []module.ComponentRef{"middleware#Definition"},
		Defaults: Config{
			Mode:     ModeAuto,
			Timezone: "Asia/Shanghai",
			Log:      LogConfig{KeepDays: 20},
			Execution: ExecutionConfig{
				Timeout: defaultTimeout,
				LockTTL: defaultLockTTL,
			},
			Queue: QueueConfig{
				Concurrency:     10,
				MaxRetry:        3,
				RetryDelay:      defaultRetryDelay,
				ShutdownTimeout: defaultShutdownTimeout,
			},
		},
	}
}
