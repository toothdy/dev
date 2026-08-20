package global

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/toothdy/cool-admin-go-next/cool/security"
	baseSysService "github.com/toothdy/cool-admin-go-next/modules/base/service/sys"
)

const maxLogParamsBytes = 64 * 1024

// 操作日志提交端口
type LogSubmitter interface {
	Submit(ctx context.Context, request baseSysService.LogRecordRequest) bool
}

var sensitiveLogKeys = map[string]struct{}{
	"authorization": {},
	"captchaid":     {},
	"oldpassword":   {},
	"password":      {},
	"refreshtoken":  {},
	"token":         {},
	"verifycode":    {},
}

// 创建 admin 请求日志中间件
func NewLog(service LogSubmitter) ghttp.HandlerFunc {
	return func(r *ghttp.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin/") {
			r.Middleware.Next()
			return
		}
		request := buildLogRecordRequest(r)
		if service != nil {
			defer func() {
				service.Submit(r.Context(), request)
			}()
		}
		r.Middleware.Next()
	}
}

func buildLogRecordRequest(r *ghttp.Request) baseSysService.LogRecordRequest {
	request := baseSysService.LogRecordRequest{
		Action: r.URL.Path,
		IP:     r.GetClientIp(),
		Params: logParams(r),
	}
	if user, ok := security.UserFromContext(r.Context()); ok {
		request.UserID = &user.UserId
		if tenantID, hasTenant := user.TenantId.TenantID(); hasTenant {
			request.TenantID = tenantID
		}
	}
	return request
}

func logParams(r *ghttp.Request) string {
	switch r.URL.Path {
	case "/admin/base/open/login", "/admin/base/open/refreshToken", "/admin/base/comm/personUpdate":
		return `{}`
	}
	var value interface{}
	if r.Method == http.MethodGet {
		value = r.GetQueryMap()
	} else if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(r.GetBody(), &value); err != nil {
			value = map[string]interface{}{"invalidJson": true}
		}
	} else {
		value = r.GetFormMap()
	}
	value = redactLogValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return `{}`
	}
	if len(encoded) <= maxLogParamsBytes {
		return string(encoded)
	}
	prefix := string(encoded[:maxLogParamsBytes/3])
	truncated, _ := json.Marshal(map[string]interface{}{
		"truncated": true,
		"preview":   prefix,
	})
	return string(truncated)
}

func redactLogValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			if _, sensitive := sensitiveLogKeys[strings.ToLower(key)]; sensitive {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactLogValue(item)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for index, item := range typed {
			redacted[index] = redactLogValue(item)
		}
		return redacted
	default:
		return value
	}
}
