package queue

import (
	"testing"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestNewRedisClientUsesDefaultConfig(t *testing.T) {
	resource, err := NewRedisClient(module.RedisDefaultConfig{
		Configured: true,
		Config: gredis.Config{
			Address: "redis.internal:6380",
			Db:      2,
			User:    "task-user",
			Pass:    "task-pass",
		},
	})
	if err != nil {
		t.Fatalf("create Redis client: %v", err)
	}
	if !resource.Configured || resource.Client == nil {
		t.Fatal("expected configured Redis resource")
	}
	t.Cleanup(func() { _ = resource.Client.Close() })
	standalone, ok := resource.Client.(*redis.Client)
	if !ok {
		t.Fatalf("expected standalone Redis client, got %T", resource.Client)
	}
	options := standalone.Options()
	if options.Addr != "redis.internal:6380" || options.DB != 2 || options.Username != "task-user" || options.Password != "task-pass" {
		t.Fatalf("unexpected Redis options: %#v", options)
	}
}

func TestNewRedisClientKeepsUnconfiguredState(t *testing.T) {
	resource, err := NewRedisClient(module.RedisDefaultConfig{})
	if err != nil {
		t.Fatalf("create unconfigured Redis resource: %v", err)
	}
	if resource.Configured || resource.Client != nil {
		t.Fatalf("unexpected Redis resource: %#v", resource)
	}
}
