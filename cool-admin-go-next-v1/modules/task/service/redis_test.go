package service

import (
	"testing"
	"time"
)

func TestBuildRedisSchedulerConfigKeepsProductionDefaults(t *testing.T) {
	config := buildRedisSchedulerConfig(RedisOptions{})
	if config.QueueName != redisQueue || config.TaskType != redisTaskType || config.LeaseKey != redisLeaseKey {
		t.Fatalf("Redis 生产命名空间被改变: queue=%q taskType=%q leaseKey=%q", config.QueueName, config.TaskType, config.LeaseKey)
	}
	if config.DelayedTaskCheckInterval != 0 {
		t.Fatalf("Redis 延迟任务检查间隔默认值被覆盖: %v", config.DelayedTaskCheckInterval)
	}
}

func TestBuildRedisSchedulerConfigAppliesTestNamespaceAndCheckInterval(t *testing.T) {
	config := buildRedisSchedulerConfig(RedisOptions{
		Namespace: "integration-123", DelayedTaskCheckInterval: 20 * time.Millisecond,
	})
	if config.QueueName != redisQueue+":integration-123" ||
		config.TaskType != redisTaskType+":integration-123" ||
		config.LeaseKey != redisLeaseKey+":integration-123" {
		t.Fatalf("Redis 测试命名空间未完整隔离: queue=%q taskType=%q leaseKey=%q", config.QueueName, config.TaskType, config.LeaseKey)
	}
	if config.DelayedTaskCheckInterval != 20*time.Millisecond {
		t.Fatalf("Redis 延迟任务检查间隔未透传: %v", config.DelayedTaskCheckInterval)
	}
}
