// Package session 提供鉴权 Session 后端的兼容入口
package session

import (
	"context"

	"github.com/toothdy/cool-admin-go-next/cool-next/auth"
)

const (
	RedisType     = auth.RedisType
	MemoryType    = auth.MemoryType
	DefaultGroup  = auth.DefaultGroup
	DefaultPrefix = auth.DefaultPrefix
)

var (
	ErrNotFound      = auth.ErrSessionNotFound
	ErrRefreshReplay = auth.ErrRefreshReplay
)

type Config = auth.SessionConfig
type Store = auth.Store
type MemoryStore = auth.MemoryStore
type RedisStore = auth.RedisStore

// 返回默认 Redis 配置
func DefaultConfig() Config {
	return auth.DefaultSessionConfig()
}

// 按显式配置创建 Session Store
func New(ctx context.Context, config Config) (Store, error) {
	return auth.NewSessionStore(ctx, config)
}

// 创建进程内 Session Store
func NewMemory() *MemoryStore {
	return auth.NewMemory()
}

// 创建并探测指定连接组的 Redis Session Store
func NewRedis(ctx context.Context, group, prefix string) (*RedisStore, error) {
	return auth.NewRedis(ctx, group, prefix)
}
