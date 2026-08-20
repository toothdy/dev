package module_test

import (
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module"
)

type declarationConfig struct {
	Enabled bool `json:"enabled"`
}

func declarationFixture() module.Declaration[declarationConfig] {
	return module.Declaration[declarationConfig]{
		Name:              "示例模块",
		Description:       "用于验证模块声明",
		Order:             10,
		Middlewares:       []module.ComponentRef{"middleware#Definition"},
		GlobalMiddlewares: []module.ComponentRef{"middleware/global#Definition"},
		Defaults:          declarationConfig{Enabled: true},
	}
}

func TestDeclarationCarriesMetadataMiddlewareRefsAndDefaults(t *testing.T) {
	declaration := declarationFixture()
	if declaration.Name != "示例模块" || declaration.Description != "用于验证模块声明" || declaration.Order != 10 {
		t.Fatalf("模块元信息未完整保留: %#v", declaration)
	}
	if len(declaration.Middlewares) != 1 || declaration.Middlewares[0] != "middleware#Definition" {
		t.Fatalf("模块中间件引用未完整保留: %#v", declaration.Middlewares)
	}
	if len(declaration.GlobalMiddlewares) != 1 || declaration.GlobalMiddlewares[0] != "middleware/global#Definition" {
		t.Fatalf("全局中间件引用未完整保留: %#v", declaration.GlobalMiddlewares)
	}
	if !declaration.Defaults.Enabled {
		t.Fatal("模块默认配置未完整保留")
	}
}

func TestComponentRefIsACompileTimeStringValue(t *testing.T) {
	const reference module.ComponentRef = "middleware#Definition"
	if string(reference) != "middleware#Definition" {
		t.Fatalf("组件引用不是静态字符串值: %q", reference)
	}
}
