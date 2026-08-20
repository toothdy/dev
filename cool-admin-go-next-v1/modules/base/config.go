package base

import (
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

const (
	defaultLogQueueSize       = 1024
	defaultLogShutdownTimeout = 5 * time.Second
	defaultLogWriteTimeout    = 2 * time.Second
	defaultLogCleanupTimeout  = 30 * time.Minute
)

// Switch 表示功能开关。
type Switch struct {
	Enable bool `json:"enable"`
}

// Log 表示操作日志中间件配置。
type Log struct {
	Enable          bool          `json:"enable"`
	QueueSize       int           `json:"queueSize"`
	ShutdownTimeout time.Duration `json:"shutdownTimeout"`
	WriteTimeout    time.Duration `json:"writeTimeout"`
	CleanupTimeout  time.Duration `json:"cleanupTimeout"`
}

// Middleware 表示 Base 全局中间件配置。
type Middleware struct {
	Authority Switch `json:"authority"`
	Log       Log    `json:"log"`
}

// Config 表示 Base 模块纯业务配置。
type Config struct {
	AllowKeys  []string   `json:"allowKeys"`
	Middleware Middleware `json:"middleware"`
}

// Validate 校验 Base 模块配置。
func (config Config) Validate() error {
	if config.Middleware.Log.QueueSize <= 0 {
		return gerror.New("module.base.middleware.log.queueSize 必须大于 0")
	}
	if config.Middleware.Log.ShutdownTimeout <= 0 {
		return gerror.New("module.base.middleware.log.shutdownTimeout 必须大于 0")
	}
	if config.Middleware.Log.WriteTimeout <= 0 {
		return gerror.New("module.base.middleware.log.writeTimeout 必须大于 0")
	}
	if config.Middleware.Log.CleanupTimeout <= 0 {
		return gerror.New("module.base.middleware.log.cleanupTimeout 必须大于 0")
	}
	return nil
}

// ModuleConfig 声明 Base 模块元信息、中间件与默认配置。
func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "权限管理",
		Description: "基础的权限管理功能，包括登录，权限校验",
		Order:       10,
		GlobalMiddlewares: []module.ComponentRef{
			"middleware/global#TranslateDefinition",
			"middleware/global#AuthorityDefinitions",
			"middleware/global#LogDefinition",
		},
		Defaults: Config{
			AllowKeys: []string{},
			Middleware: Middleware{
				Authority: Switch{Enable: true},
				Log: Log{
					Enable:          true,
					QueueSize:       defaultLogQueueSize,
					ShutdownTimeout: defaultLogShutdownTimeout,
					WriteTimeout:    defaultLogWriteTimeout,
					CleanupTimeout:  defaultLogCleanupTimeout,
				},
			},
		},
	}
}
