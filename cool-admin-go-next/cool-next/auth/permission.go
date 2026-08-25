package auth

import (
	"fmt"
	"strings"

	"github.com/toothdy/cool-admin-go-next/cool-next/core/exception"
)

// 参与菜单权限校验的后台路由前缀
const adminPathPrefix = "/admin/"

// 约定只校验登录、不校验菜单权限的通用接口路径段
const commPathSegment = "comm"

// 字典数据供后台通用组件读取，只校验登录
const adminDictDataPath = "/admin/dict/info/data"

// 按最终路由路径推导后台权限标识，与 cool-admin-node 的 URL 反推等价。
//
// 返回空串表示无需菜单权限：ignoreToken 路由、非后台路由、通用接口和后台字典数据接口。
//
// 路径段按字符形状校验，不使用 go/token.IsIdentifier —— 后者拒绝 Go 关键字，
// 而 /admin/base/sys/menu/import、/admin/dict/type 等真实路由的路径段正是关键字。
// 权限标识只作为映射键与字符串使用，不会成为 Go 标识符。
func DerivePermission(fullPath string, ignoreToken bool) (string, error) {
	if ignoreToken || !strings.HasPrefix(fullPath, adminPathPrefix) {
		return "", nil
	}
	if fullPath == adminDictDataPath {
		return "", nil
	}
	remainder := strings.Trim(strings.TrimPrefix(fullPath, adminPathPrefix), "/")
	if remainder == "" {
		return "", exception.Core(fmt.Sprintf("后台路由 %q 无法推导权限标识", fullPath))
	}

	segments := strings.Split(remainder, "/")
	for index, segment := range segments {
		if segment == commPathSegment && index != len(segments)-1 {
			return "", nil
		}
	}
	for _, segment := range segments {
		if !validPermissionSegment(segment) {
			return "", exception.Core(fmt.Sprintf("后台路由 %q 的路径段 %q 不是合法权限标识", fullPath, segment))
		}
	}

	return strings.Join(segments, ":"), nil
}

// 权限标识段允许的字符形状
func validPermissionSegment(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z', character == '_':
		case character >= '0' && character <= '9':
			if index == 0 {
				return false
			}
		default:
			return false
		}
	}

	return true
}
