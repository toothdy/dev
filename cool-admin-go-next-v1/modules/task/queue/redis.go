package queue

import (
	"crypto/tls"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/redis/go-redis/v9"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

// RedisClient 表示 Task 自管 Redis 连接及其配置状态。
type RedisClient struct {
	Client     redis.UniversalClient
	Configured bool
}

// NewRedisClient 根据应用级 redis.default 快照创建 Task 连接池。
func NewRedisClient(source module.RedisDefaultConfig) (RedisClient, error) {
	resource := RedisClient{Configured: source.Configured}
	if !source.Configured {
		return resource, nil
	}
	config := source.Config
	addresses := make([]string, 0)
	for _, address := range strings.Split(config.Address, ",") {
		if trimmed := strings.TrimSpace(address); trimmed != "" {
			addresses = append(addresses, trimmed)
		}
	}
	if len(addresses) == 0 {
		return RedisClient{}, gerror.New("redis.default.address 不能为空")
	}
	var tlsConfig *tls.Config
	if config.TLS {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: config.TLSSkipVerify}
	}
	resource.Client = redis.NewUniversalClient(&redis.UniversalOptions{
		Addrs: addresses, DB: config.Db, Username: config.User, Password: config.Pass,
		DialTimeout: config.DialTimeout, ReadTimeout: config.ReadTimeout, WriteTimeout: config.WriteTimeout,
		PoolSize: config.MaxActive, MasterName: config.MasterName, SentinelUsername: config.SentinelUser,
		SentinelPassword: config.SentinelPass, TLSConfig: tlsConfig,
	})
	return resource, nil
}
