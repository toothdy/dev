// Package session 提供鉴权 Session 后端的兼容入口
package session

import (
	"context"
	"time"

	sessionbackend "github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessionbackend"
)

const (
	RedisType     = sessionbackend.RedisType
	MemoryType    = sessionbackend.MemoryType
	DefaultGroup  = sessionbackend.DefaultGroup
	DefaultPrefix = sessionbackend.DefaultPrefix
)

var (
	ErrNotFound      = sessionbackend.ErrNotFound
	ErrRefreshReplay = sessionbackend.ErrRefreshReplay
)

type Config = sessionbackend.Config
type Session = sessionbackend.Session
type Store = sessionbackend.Store
type MemoryStore = sessionbackend.MemoryStore
type RedisStore = sessionbackend.RedisStore
type Adapter = sessionbackend.Adapter

// 返回默认 Redis 配置
func DefaultConfig() Config {
	return sessionbackend.DefaultConfig()
}

// 按显式配置创建 Session Store
func New(ctx context.Context, config Config) (Store, error) {
	return sessionbackend.New(ctx, config)
}

// 构造管理端 Session
func NewAdmin(
	sessionID string,
	userID uint64,
	username string,
	passwordV int,
	roleIDs []uint64,
	accessJTI string,
	refreshJTI string,
	expiresAt time.Time,
) (Session, error) {
	return sessionbackend.NewAdmin(sessionID, userID, username, passwordV, roleIDs, accessJTI, refreshJTI, expiresAt)
}

// 构造应用端 Session
func NewApp(
	sessionID string,
	userID uint64,
	accessJTI string,
	refreshJTI string,
	expiresAt time.Time,
) (Session, error) {
	return sessionbackend.NewApp(sessionID, userID, accessJTI, refreshJTI, expiresAt)
}

// 创建进程内 Session Store
func NewMemory() *MemoryStore {
	return sessionbackend.NewMemory()
}

// 创建并探测指定连接组的 Redis Session Store
func NewRedis(ctx context.Context, group, prefix string) (*RedisStore, error) {
	return sessionbackend.NewRedis(ctx, group, prefix)
}

// 创建认证端口适配器
func NewAdapter(store Store) (*Adapter, error) {
	return sessionbackend.NewAdapter(store)
}
