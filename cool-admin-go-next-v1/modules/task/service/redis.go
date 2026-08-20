package service

import (
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/task"
)

const (
	redisTaskType = "cool-admin-go-next:task:v1:execute"
	redisQueue    = "cool-admin-go-next-task-v1"
	redisLeaseKey = "cool-admin-go-next:task:v1:scheduler-lease"
)

const redisLeaseTTL = 15 * time.Second

// RedisOptions 描述 Task Redis 调度后端参数。
type RedisOptions struct {
	Client                   redis.UniversalClient
	Concurrency              int
	MaxRetry                 int
	RetryDelay               time.Duration
	ShutdownTimeout          time.Duration
	DelayedTaskCheckInterval time.Duration
	Location                 *time.Location
	Consumer                 task.Consumer
	Namespace                string
}

// BuildRedisScheduler 构建使用 Go 专属命名空间的 Redis 调度后端。
func BuildRedisScheduler(options RedisOptions) (task.Scheduler, error) {
	return task.NewRedisScheduler(buildRedisSchedulerConfig(options))
}

func buildRedisSchedulerConfig(options RedisOptions) task.RedisSchedulerConfig {
	queueName := redisQueue
	taskType := redisTaskType
	leaseKey := redisLeaseKey
	if namespace := strings.TrimSpace(options.Namespace); namespace != "" {
		queueName += ":" + namespace
		taskType += ":" + namespace
		leaseKey += ":" + namespace
	}
	return task.RedisSchedulerConfig{
		RedisClient: options.Client, Concurrency: options.Concurrency, MaxRetry: options.MaxRetry,
		RetryDelay: options.RetryDelay, ShutdownTimeout: options.ShutdownTimeout,
		DelayedTaskCheckInterval: options.DelayedTaskCheckInterval,
		Location:                 options.Location, Consumer: options.Consumer, QueueName: queueName,
		TaskType: taskType, LeaseKey: leaseKey, LeaseTTL: redisLeaseTTL,
	}
}
