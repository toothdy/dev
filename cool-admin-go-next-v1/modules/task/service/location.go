package service

import (
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	taskModule "github.com/toothdy/cool-admin-go-next/modules/task"
)

// NewLocation 从 Task 纯配置派生时区资源。
func NewLocation(config taskModule.Config) (*time.Location, error) {
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return nil, gerror.Wrap(err, "module.task.timezone 无效")
	}
	return location, nil
}
