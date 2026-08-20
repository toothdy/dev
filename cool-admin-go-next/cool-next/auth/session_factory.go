package auth

import (
	"context"

	sessionbackend "github.com/toothdy/cool-admin-go-next/cool-next/auth/internal/sessionbackend"
)

// Session 后端配置
type SessionConfig = sessionbackend.Config

// 返回默认 Session 后端配置
func DefaultSessionConfig() SessionConfig {
	return sessionbackend.DefaultConfig()
}

// 按配置创建鉴权 Session Store
func NewSessionStore(ctx context.Context, config SessionConfig) (SessionStore, error) {
	store, err := sessionbackend.New(ctx, config)
	if err != nil {
		return nil, err
	}

	return sessionbackend.NewAdapter(store)
}

var _ SessionStore = (*sessionbackend.Adapter)(nil)
