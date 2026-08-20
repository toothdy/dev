package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/module"
	taskQueue "github.com/toothdy/cool-admin-go-next/modules/task/queue"
)

func loadTaskRedisClient(ctx context.Context) (redis.UniversalClient, bool, error) {
	value, err := g.Cfg().Get(ctx, "redis.default")
	if err != nil {
		return nil, false, err
	}
	if value == nil || value.IsEmpty() {
		return nil, false, nil
	}
	config, err := gredis.ConfigFromMap(value.Map())
	if err != nil {
		return nil, true, err
	}
	resource, err := taskQueue.NewRedisClient(module.RedisDefaultConfig{
		Configured: true,
		Config:     *config,
	})
	if err != nil {
		return nil, true, err
	}
	return resource.Client, resource.Configured, nil
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err = os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			t.Fatalf("repository root not found from %s", current)
		}
		current = parent
	}
}
