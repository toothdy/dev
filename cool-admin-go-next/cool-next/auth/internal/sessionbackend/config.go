package sessionbackend

import (
	"context"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

const (
	RedisType     = "redis"         // Redis Store 类型
	MemoryType    = "memory"        // Memory Store 类型
	DefaultGroup  = "default"       // 默认 Redis 连接组
	DefaultPrefix = "cool:session:" // 默认 Redis Key 前缀
)

// Store 配置
type Config struct {
	Type   string `json:"type"`   // Store 类型
	Group  string `json:"group"`  // Redis 连接组
	Prefix string `json:"prefix"` // Redis Key 前缀
}

// 返回默认 Redis 配置
func DefaultConfig() Config {
	return Config{Type: RedisType, Group: DefaultGroup, Prefix: DefaultPrefix}
}

// 按显式配置创建 Session Store
func New(ctx context.Context, config Config) (Store, error) {
	if config == (Config{}) {
		config = DefaultConfig()
	}
	config.Type = strings.ToLower(strings.TrimSpace(config.Type))
	if config.Type == "" {
		config.Type = RedisType
	}
	switch config.Type {
	case RedisType:
		if config.Group == "" {
			config.Group = DefaultGroup
		}
		if config.Prefix == "" {
			config.Prefix = DefaultPrefix
		}
		return NewRedis(ctx, config.Group, config.Prefix)
	case MemoryType:
		return NewMemory(), nil
	default:
		return nil, exception.Core("Session Store Type 只支持 redis 或 memory")
	}
}
