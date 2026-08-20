package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

// Redis 调用记录
type redisSessionCall struct {
	command string
	args    []any
}

// Redis 会话客户端测试替身
type redisSessionClientStub struct {
	calls  []redisSessionCall
	result *gvar.Var
	err    error
}

/**
 * 记录并执行 Redis 命令
 * @param ctx 上下文
 * @param command Redis 命令
 * @param args 命令参数
 * @returns Redis 结果和错误
 */
func (s *redisSessionClientStub) Do(_ context.Context, command string, args ...any) (*gvar.Var, error) {
	s.calls = append(s.calls, redisSessionCall{command: command, args: append([]any(nil), args...)})
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return gvar.New("PONG"), nil
}

/**
 * 验证未配置 Redis 时使用内存会话
 * @param t 测试上下文
 * @returns null
 */
func TestResolveSessionStoreFallsBackToMemoryWithoutRedisConfig(t *testing.T) {
	setSessionStoreTestConfig(t, "")
	providerCalled := false
	store, err := resolveSessionStore(context.Background(), nil, func() (security.RedisCommander, error) {
		providerCalled = true
		return &redisSessionClientStub{}, nil
	})
	if err != nil {
		t.Fatalf("resolve session store failed: %v", err)
	}
	if providerCalled {
		t.Fatal("Redis provider should not be called without redis.default")
	}
	if _, ok := store.(*security.MemorySessionStore); !ok {
		t.Fatalf("expected memory session store, got %T", store)
	}
}

/**
 * 验证配置 Redis 时使用 Redis 会话
 * @param t 测试上下文
 * @returns null
 */
func TestResolveSessionStoreUsesRedisWhenConfigured(t *testing.T) {
	setSessionStoreTestConfig(t, `redis:
  default:
    address: "127.0.0.1:6379"`)
	client := &redisSessionClientStub{}
	store, err := resolveSessionStore(context.Background(), nil, func() (security.RedisCommander, error) {
		return client, nil
	})
	if err != nil {
		t.Fatalf("resolve session store failed: %v", err)
	}
	if _, ok := store.(*security.RedisSessionStore); !ok {
		t.Fatalf("expected Redis session store, got %T", store)
	}
	if len(client.calls) != 1 || client.calls[0].command != "PING" || len(client.calls[0].args) != 0 {
		t.Fatalf("expected one Redis PING, got %#v", client.calls)
	}
}

/**
 * 验证 Redis 不可用时启动失败
 * @param t 测试上下文
 * @returns null
 */
func TestResolveSessionStoreFailsWhenConfiguredRedisIsUnavailable(t *testing.T) {
	setSessionStoreTestConfig(t, `redis:
  default:
    address: "127.0.0.1:6379"`)
	wantErr := errors.New("redis unavailable")
	client := &redisSessionClientStub{err: wantErr}
	store, err := resolveSessionStore(context.Background(), nil, func() (security.RedisCommander, error) {
		return client, nil
	})
	if store != nil {
		t.Fatalf("failed Redis must not fall back to %T", store)
	}
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "连接默认认证 Redis 失败") {
		t.Fatalf("unexpected Redis error: %v", err)
	}
}

/**
 * 验证无效 Redis 配置不会回退内存会话
 * @param t 测试上下文
 * @returns null
 */
func TestResolveSessionStoreRejectsInvalidRedisConfig(t *testing.T) {
	setSessionStoreTestConfig(t, `redis:
  default: {}`)
	providerCalled := false
	store, err := resolveSessionStore(context.Background(), nil, func() (security.RedisCommander, error) {
		providerCalled = true
		return &redisSessionClientStub{}, nil
	})
	if store != nil {
		t.Fatalf("invalid Redis config must not fall back to %T", store)
	}
	if providerCalled {
		t.Fatal("Redis provider should not be called for invalid configuration")
	}
	if err == nil || !strings.Contains(err.Error(), "redis.default 配置无效") {
		t.Fatalf("unexpected Redis configuration error: %v", err)
	}
}

/**
 * 验证显式会话存储优先于 Redis 配置
 * @param t 测试上下文
 * @returns null
 */
func TestResolveSessionStorePrefersConfiguredStore(t *testing.T) {
	setSessionStoreTestConfig(t, `redis:
  default:
    address: "127.0.0.1:6379"`)
	configured := security.NewMemorySessionStore()
	providerCalled := false
	store, err := resolveSessionStore(context.Background(), configured, func() (security.RedisCommander, error) {
		providerCalled = true
		return &redisSessionClientStub{}, nil
	})
	if err != nil {
		t.Fatalf("resolve session store failed: %v", err)
	}
	if providerCalled {
		t.Fatal("Redis provider should not be called for an explicitly configured store")
	}
	if store != configured {
		t.Fatalf("expected explicitly configured store, got %T", store)
	}
}

/**
 * 设置会话存储测试配置
 * @param t 测试上下文
 * @param content YAML 配置内容
 * @returns null
 */
func setSessionStoreTestConfig(t *testing.T, content string) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatalf("create config adapter failed: %v", err)
	}
	config := g.Cfg()
	previousAdapter := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() {
		config.SetAdapter(previousAdapter)
	})
}
