package recycle

import (
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

const (
	defaultCleanupInterval = 24 * time.Hour
	defaultCleanupBatch    = 500
	defaultLockName        = "cool-admin:recycle:cleanup"
)

// Config 描述 Recycle 模块的纯业务配置。
type Config struct {
	CleanupInterval time.Duration `json:"cleanupInterval"`
	CleanupBatch    int           `json:"cleanupBatch"`
	LockName        string        `json:"lockName"`
}

// Validate 校验 Recycle 模块配置。
func (c Config) Validate() error {
	if c.CleanupInterval <= 0 {
		return gerror.New("module.recycle.cleanupInterval 必须大于 0")
	}
	if c.CleanupBatch <= 0 {
		return gerror.New("module.recycle.cleanupBatch 必须大于 0")
	}
	if strings.TrimSpace(c.LockName) == "" {
		return gerror.New("module.recycle.lockName 不能为空")
	}
	return nil
}

// ModuleConfig 声明 Recycle 模块元信息和默认配置。
func ModuleConfig() module.Declaration[Config] {
	return module.Declaration[Config]{
		Name:        "数据回收",
		Description: "收集被删除的数据，管理和恢复",
		Defaults: Config{
			CleanupInterval: defaultCleanupInterval,
			CleanupBatch:    defaultCleanupBatch,
			LockName:        defaultLockName,
		},
	}
}
