package sys

import (
	"context"
	"time"

	"github.com/toothdy/cool-admin-go-next/cool/db/tenant"
)

// 丢弃客户端租户字段，并在租户模式下使用登录上下文覆盖
func applyTenantMutation(ctx context.Context, data map[string]interface{}) {
	delete(data, "tenantId")
	if tenantID, ok := contextTenantID(ctx); ok {
		data["tenantId"] = tenantID
	}
}

func contextTenantID(ctx context.Context) (int64, bool) {
	return tenant.Resolve(ctx).TenantID()
}

func mutationTimestamp() string {
	return time.Now().Format("2006-01-02 15:04:05")
}
