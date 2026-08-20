package auth

import (
	"context"
	"strings"
)

// 从 Authorization 值提取 Bearer Token
func BearerToken(authorization string) (string, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", invalidCredentialError()
	}

	return parts[1], nil
}

// 按 HTTP 路由元数据执行认证和授权
func (service *Service) AuthenticateHTTP(
	ctx context.Context,
	authorization string,
	method string,
	requestPath string,
	permission string,
	ignoreToken bool,
) (context.Context, error) {
	if ignoreToken {
		return ctx, nil
	}
	token, err := BearerToken(authorization)
	if err != nil {
		return ctx, err
	}
	resource := ""
	if strings.TrimSpace(permission) != "" {
		resource, err = HTTPResource(method, requestPath)
		if err != nil {
			return ctx, err
		}
	}

	return service.Authenticate(ctx, token, Rule{Permission: permission, Resource: resource})
}

// 按 gRPC 方法元数据执行认证和授权
func (service *Service) AuthenticateGRPC(
	ctx context.Context,
	authorization string,
	fullMethod string,
	permission string,
	ignoreToken bool,
) (context.Context, error) {
	if ignoreToken {
		return ctx, nil
	}
	token, err := BearerToken(authorization)
	if err != nil {
		return ctx, err
	}
	resource := ""
	if strings.TrimSpace(permission) != "" {
		resource, err = GRPCResource(fullMethod)
		if err != nil {
			return ctx, err
		}
	}

	return service.Authenticate(ctx, token, Rule{Permission: permission, Resource: resource})
}
