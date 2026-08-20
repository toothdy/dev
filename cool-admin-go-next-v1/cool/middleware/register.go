package middleware

import (
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"
)

// 验证中间件并返回稳定排序后的副本
func Validate(definitions []Definition) ([]Definition, error) {
	items := append([]Definition{}, definitions...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Order < items[j].Order
	})

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Name == "" {
			return nil, gerror.New("中间件名称不能为空")
		}
		if _, ok := seen[item.Name]; ok {
			return nil, gerror.Newf("中间件名称重复: %s", item.Name)
		}
		if item.Handler == nil {
			return nil, gerror.Newf("中间件处理器不能为空: %s", item.Name)
		}
		if strings.HasPrefix(item.Name, "cool.") && item.Name != RecoveryName && item.Name != ErrorName {
			return nil, gerror.Newf("中间件不能使用 cool.* 保留名称: %s", item.Name)
		}
		if item.Order < 200 && !isAllowedCoreDefinition(item) {
			return nil, gerror.Newf("中间件占用核心保留 order: %s=%d", item.Name, item.Order)
		}
		seen[item.Name] = struct{}{}
	}
	return items, nil
}

// 验证模块局部中间件，禁止占用核心保留 Order
func ValidateModule(definitions []Definition) ([]Definition, error) {
	for _, item := range definitions {
		if item.Order < 200 {
			return nil, gerror.Newf("中间件占用核心保留 order: %s=%d", item.Name, item.Order)
		}
	}
	return Validate(definitions)
}

// 按顺序验证并注册中间件
func Register(server *ghttp.Server, definitions []Definition) error {
	if server == nil {
		return gerror.New("HTTP 服务不能为空")
	}
	items, err := Validate(definitions)
	if err != nil {
		return err
	}
	handlers := make([]ghttp.HandlerFunc, 0, len(items))
	for _, item := range items {
		handlers = append(handlers, item.Handler)
	}
	if len(handlers) > 0 {
		server.Use(handlers...)
	}
	return nil
}

func isAllowedCoreDefinition(item Definition) bool {
	return (item.core && item.Name == RecoveryName && item.Order == RecoveryOrder) ||
		(item.Name == "base.translate" && item.Order == 100) ||
		(item.core && item.Name == ErrorName && item.Order == ErrorOrder)
}
