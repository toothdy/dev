package auth

import (
	"context"
	"strings"
)

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
	if authorization == "" {
		return ctx, invalidCredentialError()
	}
	resource := ""
	if strings.TrimSpace(permission) != "" {
		var err error
		resource, err = HTTPResource(method, requestPath)
		if err != nil {
			return ctx, err
		}
	}

	return service.Authenticate(ctx, authorization, Rule{Permission: permission, Resource: resource})
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
	if authorization == "" {
		return ctx, invalidCredentialError()
	}
	resource := ""
	if strings.TrimSpace(permission) != "" {
		var err error
		resource, err = GRPCResource(fullMethod)
		if err != nil {
			return ctx, err
		}
	}

	return service.Authenticate(ctx, authorization, Rule{Permission: permission, Resource: resource})
}
