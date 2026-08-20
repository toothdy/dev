package route

import (
	"net/http"
	"path"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/errors/gerror"
)

var supportedMethods = map[string]struct{}{
	http.MethodDelete:  {},
	http.MethodGet:     {},
	http.MethodHead:    {},
	http.MethodOptions: {},
	http.MethodPatch:   {},
	http.MethodPost:    {},
	http.MethodPut:     {},
}

// 返回规范化的 METHOD:path 路由键
func Key(method string, requestPath string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if _, ok := supportedMethods[method]; !ok {
		return "", gerror.Newf("不支持的 HTTP method: %s", method)
	}
	normalized, err := NormalizePath(requestPath)
	if err != nil {
		return "", err
	}
	return method + ":" + normalized, nil
}

// 清理静态 metadata 路径
func NormalizePath(requestPath string) (string, error) {
	if requestPath == "" || !strings.HasPrefix(requestPath, "/") {
		return "", gerror.New("路由路径必须以 / 开头")
	}
	if strings.ContainsAny(requestPath, "?#") {
		return "", gerror.New("路由路径不能包含 query 或 fragment")
	}
	if strings.ContainsAny(requestPath, ":*{}") {
		return "", gerror.New("路由路径不能包含动态参数或 wildcard")
	}
	for _, char := range requestPath {
		if unicode.IsControl(char) {
			return "", gerror.New("路由路径不能包含控制字符")
		}
	}
	for _, segment := range strings.Split(requestPath, "/") {
		if segment == ".." {
			return "", gerror.New("路由路径不能包含 ..")
		}
	}
	normalized := path.Clean(requestPath)
	if normalized != "/" {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	return normalized, nil
}
