package app

import (
	"context"
	"strings"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/toothdy/cool-admin-go-next/cool/security"
)

// Redis 会话客户端提供器
type redisSessionClientProvider func() (security.RedisCommander, error)

/**
 * 解析应用会话存储
 * @param ctx 上下文
 * @param configured 显式注入的会话存储
 * @param redisProvider Redis 客户端提供器
 * @returns 会话存储和初始化错误
 */
func resolveSessionStore(
	ctx context.Context,
	configured security.SessionStore,
	redisProvider redisSessionClientProvider,
) (security.SessionStore, error) {
	if configured != nil {
		return configured, nil
	}
	redisConfig, err := g.Cfg().Get(ctx, "redis.default")
	if err != nil {
		return nil, gerror.Wrap(err, "读取默认认证 Redis 配置失败")
	}
	if redisConfig == nil {
		return security.NewMemorySessionStore(), nil
	}
	parsedConfig, parseErr := gredis.ConfigFromMap(redisConfig.Map())
	if parseErr != nil || strings.TrimSpace(parsedConfig.Address) == "" {
		return nil, gerror.New("redis.default 配置无效")
	}
	if redisProvider == nil {
		return nil, gerror.New("默认认证 Redis 客户端提供器不可用")
	}
	client, err := redisProvider()
	if err != nil {
		return nil, gerror.Wrap(err, "初始化默认认证 Redis 客户端失败")
	}
	if client == nil {
		return nil, gerror.New("默认认证 Redis 客户端不可用")
	}
	if _, err = client.Do(ctx, "PING"); err != nil {
		return nil, gerror.Wrap(err, "连接默认认证 Redis 失败")
	}
	store, err := security.NewRedisSessionStore(client, "")
	if err != nil {
		return nil, gerror.Wrap(err, "初始化默认认证 Redis 会话存储失败")
	}
	return store, nil
}

/**
 * 获取 GoFrame 默认 Redis 客户端
 * @returns Redis 命令客户端
 */
func defaultRedisSessionClient() (client security.RedisCommander, err error) {
	defer func() {
		if recover() != nil {
			client = nil
			err = gerror.New("初始化 GoFrame Redis 客户端失败")
		}
	}()
	return g.Redis(), nil
}
