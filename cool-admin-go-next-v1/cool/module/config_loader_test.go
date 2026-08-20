package module_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/toothdy/cool-admin-go-next/cool/module"
)

type loaderNestedConfig struct {
	Enabled bool `json:"enabled"`
	Limit   int  `json:"limit"`
}

type loaderConfig struct {
	Name     string                `json:"name"`
	Nested   loaderNestedConfig    `json:"nested"`
	Items    []string              `json:"items"`
	Labels   map[string]string     `json:"labels"`
	Optional *loaderNestedConfig   `json:"optional"`
	Delay    time.Duration         `json:"delay"`
	Aliases  map[string][]string   `json:"aliases"`
	Pointers []*loaderNestedConfig `json:"pointers"`
}

func loaderDefaults() loaderConfig {
	return loaderConfig{
		Name:   "default",
		Nested: loaderNestedConfig{Enabled: true, Limit: 8},
		Items:  []string{"one"},
		Labels: map[string]string{"source": "default"},
		Optional: &loaderNestedConfig{
			Enabled: true,
			Limit:   9,
		},
		Delay:    time.Second,
		Aliases:  map[string][]string{"default": {"one"}},
		Pointers: []*loaderNestedConfig{{Enabled: true, Limit: 10}},
	}
}

func useLoaderConfig(t *testing.T, content string) {
	t.Helper()
	adapter, err := gcfg.NewAdapterContent(content)
	if err != nil {
		t.Fatalf("创建测试配置失败: %v", err)
	}
	config := g.Cfg()
	previous := config.GetAdapter()
	config.SetAdapter(adapter)
	t.Cleanup(func() { config.SetAdapter(previous) })
}

func TestLoadConfigUsesDeepCopiedDefaults(t *testing.T) {
	useLoaderConfig(t, "")
	first, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("加载默认配置失败: %v", err)
	}
	second, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("再次加载默认配置失败: %v", err)
	}

	first.Items[0] = "changed"
	first.Labels["source"] = "changed"
	first.Optional.Limit = 0
	first.Aliases["default"][0] = "changed"
	first.Pointers[0].Limit = 0
	if second.Items[0] != "one" || second.Labels["source"] != "default" || second.Optional.Limit != 9 || second.Aliases["default"][0] != "one" || second.Pointers[0].Limit != 10 {
		t.Fatalf("两次加载共享了可变默认值: %#v", second)
	}
}

func TestLoadConfigRecursivelyOverlaysExplicitZeroValues(t *testing.T) {
	useLoaderConfig(t, `module:
  sample:
    name: ""
    nested:
      enabled: false
      limit: 0
    items: []`)
	loaded, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("覆盖配置失败: %v", err)
	}
	if loaded.Name != "" || loaded.Nested.Enabled || loaded.Nested.Limit != 0 || len(loaded.Items) != 0 {
		t.Fatalf("显式零值未覆盖默认值: %#v", loaded)
	}
	if loaded.Labels["source"] != "default" || loaded.Delay != time.Second {
		t.Fatalf("未配置字段未保留默认值: %#v", loaded)
	}
}

func TestLoadConfigRejectsInvalidRootUnknownFieldTypeAndNull(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{name: "标量根", content: "module:\n  sample: invalid", wantErr: "module.sample"},
		{name: "数组根", content: "module:\n  sample: [one]", wantErr: "module.sample"},
		{name: "未知字段", content: "module:\n  sample:\n    unknown: true", wantErr: "module.sample.unknown"},
		{name: "错误类型", content: "module:\n  sample:\n    nested:\n      limit: invalid", wantErr: "module.sample.nested.limit"},
		{name: "非空类型 null", content: "module:\n  sample:\n    name: null", wantErr: "module.sample.name"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			useLoaderConfig(t, test.content)
			_, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("期望包含 %q 的错误，实际为 %v", test.wantErr, err)
			}
		})
	}

	useLoaderConfig(t, `module:
  sample:
    optional: null
    labels: null
    items: null`)
	loaded, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("可空类型 null 应成功: %v", err)
	}
	if loaded.Optional != nil || loaded.Labels != nil || loaded.Items != nil {
		t.Fatalf("可空类型未被置空: %#v", loaded)
	}
}

func TestLoadConfigUsesJSONTagsAndDuration(t *testing.T) {
	useLoaderConfig(t, `module:
  sample:
    delay: 2s`)
	loaded, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("解析 duration 失败: %v", err)
	}
	if loaded.Delay != 2*time.Second {
		t.Fatalf("duration 不匹配: %s", loaded.Delay)
	}

	useLoaderConfig(t, `module:
  sample:
    Delay: 2s`)
	_, err = module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err == nil || !strings.Contains(err.Error(), "module.sample.Delay") {
		t.Fatalf("Loader 不应接受 Go 字段名: %v", err)
	}
}

type validatingConfig struct {
	Value string `json:"value"`
}

func (config validatingConfig) Validate() error {
	if config.Value == "invalid" {
		return errors.New("值无效")
	}
	return nil
}

func TestLoadConfigCallsValueValidator(t *testing.T) {
	useLoaderConfig(t, "module:\n  sample:\n    value: invalid")
	loaded, err := module.LoadConfig(context.Background(), "sample", validatingConfig{Value: "valid"})
	if err == nil || !strings.Contains(err.Error(), "module.sample") || !strings.Contains(err.Error(), "值无效") {
		t.Fatalf("校验错误未包含模块上下文: %v", err)
	}
	if loaded != (validatingConfig{}) {
		t.Fatalf("校验失败返回了半成品: %#v", loaded)
	}
}

func TestLoadConfigDoesNotReadLegacyPrefix(t *testing.T) {
	useLoaderConfig(t, "sample:\n  name: legacy")
	loaded, err := module.LoadConfig(context.Background(), "sample", loaderDefaults())
	if err != nil {
		t.Fatalf("加载配置失败: %v", err)
	}
	if loaded.Name != "default" {
		t.Fatalf("读取了旧配置前缀: %q", loaded.Name)
	}
}
