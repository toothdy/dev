package base_test

import (
	"reflect"
	"testing"
	"time"

	baseModule "github.com/toothdy/cool-admin-go-next/modules/base"
)

func TestConfigKeepsNodeMetadataAndMiddlewareReferences(t *testing.T) {
	declaration := baseModule.ModuleConfig()
	if declaration.Name != "权限管理" || declaration.Description != "基础的权限管理功能，包括登录，权限校验" || declaration.Order != 10 {
		t.Fatalf("unexpected Base declaration metadata: %#v", declaration)
	}
	want := []string{
		"middleware/global#TranslateDefinition",
		"middleware/global#AuthorityDefinitions",
		"middleware/global#LogDefinition",
	}
	if len(declaration.Middlewares) != 0 || len(declaration.GlobalMiddlewares) != len(want) {
		t.Fatalf("unexpected Base middleware references: %#v", declaration)
	}
	for index, reference := range declaration.GlobalMiddlewares {
		if string(reference) != want[index] {
			t.Fatalf("unexpected Base global middleware reference at %d: %q", index, reference)
		}
	}
}

func TestConfigDefaultsContainOnlyBaseBusinessConfig(t *testing.T) {
	defaults := baseModule.ModuleConfig().Defaults
	if len(defaults.AllowKeys) != 0 || !defaults.Middleware.Authority.Enable || !defaults.Middleware.Log.Enable {
		t.Fatalf("unexpected Base defaults: %#v", defaults)
	}
	if defaults.Middleware.Log.QueueSize != 1024 || defaults.Middleware.Log.ShutdownTimeout != 5*time.Second || defaults.Middleware.Log.WriteTimeout != 2*time.Second || defaults.Middleware.Log.CleanupTimeout != 30*time.Minute {
		t.Fatalf("unexpected Base log defaults: %#v", defaults.Middleware.Log)
	}
	typeOfConfig := reflect.TypeOf(defaults)
	for index := 0; index < typeOfConfig.NumField(); index++ {
		field := typeOfConfig.Field(index)
		if field.Tag.Get("json") == "" {
			t.Fatalf("Base Config field %s is missing json tag", field.Name)
		}
	}
	for _, forbidden := range []string{"SSO", "JWT", "I18n"} {
		if _, exists := typeOfConfig.FieldByName(forbidden); exists {
			t.Fatalf("application option %s leaked into Base Config", forbidden)
		}
	}
}

func TestConfigValidateRejectsInvalidLogSettings(t *testing.T) {
	config := baseModule.ModuleConfig().Defaults
	tests := []struct {
		name   string
		mutate func(*baseModule.Config)
	}{
		{name: "queue", mutate: func(config *baseModule.Config) { config.Middleware.Log.QueueSize = 0 }},
		{name: "shutdown", mutate: func(config *baseModule.Config) { config.Middleware.Log.ShutdownTimeout = 0 }},
		{name: "write", mutate: func(config *baseModule.Config) { config.Middleware.Log.WriteTimeout = 0 }},
		{name: "cleanup", mutate: func(config *baseModule.Config) { config.Middleware.Log.CleanupTimeout = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := config
			test.mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected invalid Base log config rejected")
			}
		})
	}
}
