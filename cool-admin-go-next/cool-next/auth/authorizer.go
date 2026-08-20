package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 协议无关的授权请求
type Authorization struct {
	Subject    Kind   // 身份种类
	SubjectID  uint64 // 身份 ID
	Permission string // 权限标识
	Resource   string // 协议资源
}

// 业务授权器
type Authorizer interface {
	Authorize(context.Context, Authorization) (bool, error)
}

// 执行统一授权判断
func Authorize(ctx context.Context, authorizer Authorizer, permission, resource string) error {
	if authorizer == nil {
		return exception.Core("授权器不能为空")
	}
	if strings.TrimSpace(permission) == "" {
		return exception.Core("授权权限标识不能为空")
	}
	if strings.TrimSpace(resource) == "" {
		return exception.Core("授权资源不能为空")
	}

	identity, exists := identityFromContext(ctx)
	if !exists {
		return unauthenticatedError()
	}
	authorization, err := authorizationFromIdentity(identity, permission, resource)
	if err != nil {
		return err
	}

	allowed, err := authorizer.Authorize(ctx, authorization)
	if err != nil {
		return exception.WrapCore(err, "执行权限校验失败")
	}
	if !allowed {
		return exception.Comm("权限不足", http.StatusForbidden)
	}

	return nil
}

// 从已验证身份构造授权请求
func authorizationFromIdentity(identity verifiedIdentity, permission, resource string) (Authorization, error) {
	authorization := Authorization{
		Subject:    identity.subject,
		Permission: permission,
		Resource:   resource,
	}

	switch identity.subject {
	case AdminKind:
		authorization.SubjectID = identity.admin.UserID
	case AppKind:
		authorization.SubjectID = identity.app.ID
	default:
		return Authorization{}, exception.Core("授权身份种类无效")
	}
	if authorization.SubjectID == 0 {
		return Authorization{}, exception.Core("授权身份 ID 不能为空")
	}

	return authorization, nil
}
