package middleware

import (
	"context"
	"net/http"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/exception"
	"github.com/toothdy/cool-admin-go-next/cool/middleware"
	taskEvent "github.com/toothdy/cool-admin-go-next/modules/task/event"
)

const taskHealthOrder = 350

var protectedPaths = map[string]struct{}{
	"/admin/task/info/add":    {},
	"/admin/task/info/update": {},
	"/admin/task/info/start":  {},
	"/admin/task/info/stop":   {},
	"/admin/task/info/once":   {},
}

// HealthChecker 描述调度写操作需要的健康检查。
type HealthChecker interface {
	Healthy(ctx context.Context) error
}

// Definition 返回 Task 调度写保护中间件。
func Definition(comm *taskEvent.Comm) middleware.Definition {
	return middleware.Definition{
		Name: "task.health", Order: taskHealthOrder, Handler: newHandler(comm),
	}
}

// newHandler 创建仅保护调度写操作的中间件。
func newHandler(checker HealthChecker) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		if !isProtectedWrite(r.Method, r.URL.Path) {
			r.Middleware.Next()
			return
		}
		if checker == nil {
			r.SetError(exception.Comm("任务调度服务暂不可用"))
			return
		}
		if err := checker.Healthy(r.Context()); err != nil {
			r.SetError(exception.Comm("任务调度服务暂不可用"))
			return
		}
		r.Middleware.Next()
	}
}

func isProtectedWrite(method string, path string) bool {
	if method != http.MethodPost {
		return false
	}
	_, isProtected := protectedPaths[path]
	return isProtected
}
