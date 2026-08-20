package module_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool/module"
)

func TestSpecCarriesConfigureCallback(t *testing.T) {
	called := false
	spec := module.Spec{
		Configure: func(context.Context) error {
			called = true
			return nil
		},
	}
	if err := spec.Configure(context.Background()); err != nil || !called {
		t.Fatalf("模块配置回调未保留: called=%v err=%v", called, err)
	}
}

func TestApplicationDependenciesAreNamedStructs(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeFor[module.AuthOptions](),
		reflect.TypeFor[module.I18nOptions](),
		reflect.TypeFor[module.CRUDOptions](),
		reflect.TypeFor[module.RedisDefaultConfig](),
	}
	for _, dependencyType := range types {
		if dependencyType.Kind() != reflect.Struct || dependencyType.Name() == "" {
			t.Fatalf("应用依赖必须是命名结构: %s", dependencyType)
		}
	}
}
