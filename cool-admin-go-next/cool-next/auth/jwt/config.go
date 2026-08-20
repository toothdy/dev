// Package jwt 提供固定 HS256 的 JWT 签发与验证
package jwt

import (
	"fmt"
	"strings"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

const (
	Algorithm         = "HS256"              // 首期唯一签名算法
	DefaultIssuer     = "cool-admin-go-next" // 默认签发者
	DefaultAudience   = "cool-admin"         // 默认受众
	DefaultAccessTTL  = 2 * time.Hour        // 默认 Access 有效期
	DefaultRefreshTTL = 15 * 24 * time.Hour  // 默认 Refresh 有效期
	DefaultClockSkew  = 30 * time.Second     // 默认时钟偏差
	minKeyBytes       = 32                   // HS256 最小密钥字节数
)

// JWT 配置
type Config struct {
	Issuer       string            `json:"issuer"`       // 签发者
	Audience     string            `json:"audience"`     // 受众
	Algorithm    string            `json:"algorithm"`    // 签名算法
	CurrentKeyID string            `json:"currentKeyId"` // 当前签发密钥 ID
	AccessTTL    time.Duration     `json:"accessTTL"`    // Access 有效期
	RefreshTTL   time.Duration     `json:"refreshTTL"`   // Refresh 有效期
	ClockSkew    time.Duration     `json:"clockSkew"`    // 允许的时钟偏差
	Keys         map[string]string `json:"keys"`         // 验签密钥集合
}

// 返回 JWT 默认配置
func DefaultConfig() Config {
	return Config{
		Issuer:     DefaultIssuer,
		Audience:   DefaultAudience,
		Algorithm:  Algorithm,
		AccessTTL:  DefaultAccessTTL,
		RefreshTTL: DefaultRefreshTTL,
		ClockSkew:  DefaultClockSkew,
		Keys:       map[string]string{},
	}
}

// 校验 JWT 安全配置
func (config Config) Validate() error {
	if strings.TrimSpace(config.Issuer) == "" || strings.TrimSpace(config.Issuer) != config.Issuer {
		return exception.Core("JWT Issuer 无效")
	}
	if strings.TrimSpace(config.Audience) == "" || strings.TrimSpace(config.Audience) != config.Audience {
		return exception.Core("JWT Audience 无效")
	}
	if config.Algorithm != Algorithm {
		return exception.Core("JWT Algorithm 只支持 HS256")
	}
	if config.AccessTTL <= 0 || config.RefreshTTL <= 0 || config.AccessTTL >= config.RefreshTTL {
		return exception.Core("JWT TTL 必须为正数且 AccessTTL 小于 RefreshTTL")
	}
	if config.ClockSkew < 0 || config.ClockSkew >= config.AccessTTL {
		return exception.Core("JWT ClockSkew 必须非负且小于 AccessTTL")
	}
	if strings.TrimSpace(config.CurrentKeyID) == "" || strings.TrimSpace(config.CurrentKeyID) != config.CurrentKeyID {
		return exception.Core("JWT CurrentKeyID 无效")
	}
	if len(config.Keys) == 0 {
		return exception.Core("JWT Keys 不能为空")
	}
	if _, exists := config.Keys[config.CurrentKeyID]; !exists {
		return exception.Core("JWT CurrentKeyID 不存在")
	}
	for keyID, secret := range config.Keys {
		if strings.TrimSpace(keyID) == "" || strings.TrimSpace(keyID) != keyID {
			return exception.Core("JWT Key ID 无效")
		}
		if strings.TrimSpace(secret) != secret || len([]byte(secret)) < minKeyBytes {
			return exception.Core(fmt.Sprintf("JWT Key %q 至少需要 %d 个字节", keyID, minKeyBytes))
		}
	}

	return nil
}
