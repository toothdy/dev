package auth

import (
	"net/http"
	"path"
	"strings"
	"unicode"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 构造规范化 HTTP 权限资源
func HTTPResource(method, requestPath string) (string, error) {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	switch normalizedMethod {
	case http.MethodConnect, http.MethodDelete, http.MethodGet, http.MethodHead, http.MethodOptions,
		http.MethodPatch, http.MethodPost, http.MethodPut, http.MethodTrace:
	default:
		return "", exception.Core("鉴权 HTTP Method 无效")
	}
	if !validResourcePath(requestPath) || path.Clean(requestPath) != requestPath {
		return "", exception.Core("鉴权 HTTP Path 未规范化")
	}

	return normalizedMethod + " " + requestPath, nil
}

// 构造规范化 gRPC 权限资源
func GRPCResource(fullMethod string) (string, error) {
	if !validResourcePath(fullMethod) || path.Clean(fullMethod) != fullMethod {
		return "", exception.Core("鉴权 gRPC FullMethod 未规范化")
	}
	parts := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", exception.Core("鉴权 gRPC FullMethod 无效")
	}

	return fullMethod, nil
}

// 校验协议资源路径
func validResourcePath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}

	return true
}
