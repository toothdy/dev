package base

import (
	"time"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/module"
)

const (
	defaultAccessTTL    = 2 * time.Hour
	defaultRefreshTTL   = 15 * 24 * time.Hour
	defaultUploadBytes  = 10 << 20
	defaultCaptchaTTL   = 30 * time.Minute
	defaultCleanupLimit = 30 * time.Minute
)

// Config 是 Base 模块运行配置。
type Config struct {
	JWT       JWTConfig     `json:"jwt"`
	Upload    UploadConfig  `json:"upload"`
	AllowKeys []string      `json:"allowKeys"`
	Captcha   CaptchaConfig `json:"captcha"`
	Log       LogConfig     `json:"log"`
	Coding    CodingConfig  `json:"coding"`
}

// JWTConfig 定义 Base 登录令牌有效期。
type JWTConfig struct {
	AccessTTL  time.Duration `json:"accessTTL"`
	RefreshTTL time.Duration `json:"refreshTTL"`
}

// UploadConfig 定义本地上传边界。
type UploadConfig struct {
	Root          string `json:"root"`
	PublicBaseURL string `json:"publicBaseURL"`
	MaxBytes      int64  `json:"maxBytes"`
}

// CaptchaConfig 定义验证码默认参数。
type CaptchaConfig struct {
	TTL    time.Duration `json:"ttl"`
	Width  int           `json:"width"`
	Height int           `json:"height"`
	Color  string        `json:"color"`
}

// LogConfig 定义操作日志清理任务。
type LogConfig struct {
	CleanupPattern string        `json:"cleanupPattern"`
	CleanupTimeout time.Duration `json:"cleanupTimeout"`
}

// CodingConfig 定义开发代码工具可访问的项目工作区。
type CodingConfig struct {
	Workspace string `json:"workspace"`
}

// ModuleConfig 声明 Base 模块及其默认配置。
func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "权限管理",
		Description: "基础的权限管理功能，包括登录，权限校验",
		Order:       10,
		GlobalMiddlewares: []module.ComponentRef{
			module.Ref("middleware.global.NewLogHandler"),
			module.Ref("middleware.global.NewTranslateHandler"),
		},
		Defaults: Config{
			JWT: JWTConfig{
				AccessTTL:  defaultAccessTTL,
				RefreshTTL: defaultRefreshTTL,
			},
			Upload: UploadConfig{
				Root:          "resource/public/uploads",
				PublicBaseURL: "http://127.0.0.1:8001",
				MaxBytes:      defaultUploadBytes,
			},
			AllowKeys: []string{},
			Captcha: CaptchaConfig{
				TTL:    defaultCaptchaTTL,
				Width:  150,
				Height: 50,
				Color:  "#fff",
			},
			Log: LogConfig{
				CleanupPattern: "@daily",
				CleanupTimeout: defaultCleanupLimit,
			},
			Coding: CodingConfig{Workspace: "."},
		},
	}
}
