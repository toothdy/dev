package artifact

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseManifest(t *testing.T) {
	manifest, err := ParseManifest(validManifestData(t))
	if err != nil {
		t.Fatalf("解析合法 Manifest 失败: %v", err)
	}
	if manifest.Key != "echo-plugin" || manifest.Runtime.ABI != "cool.plugin/v1" {
		t.Fatalf("Manifest 内容错误: %+v", manifest)
	}
}

func TestParseManifestRejectsInvalidInput(t *testing.T) {
	tests := map[string]func(map[string]any){
		"未知字段":    func(value map[string]any) { value["unknown"] = true },
		"缺少布尔字段":  func(value map[string]any) { delete(value, "singleton") },
		"保留 key":  func(value map[string]any) { value["key"] = "plugin" },
		"非法 hook": func(value map[string]any) { value["hook"] = "Echo" },
		"版本带 v":   func(value map[string]any) { value["version"] = "v1.0.0" },
		"预发布前导零":  func(value map[string]any) { value["version"] = "1.0.0-01" },
		"配置不是对象":  func(value map[string]any) { value["config"] = []string{} },
		"资源路径不规范": func(value map[string]any) { value["logo"] = "assets/../logo.png" },
		"嵌套未知字段": func(value map[string]any) {
			value["runtime"].(map[string]any)["engine"] = "wazero"
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := validManifestValue()
			mutate(value)
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("编码测试 Manifest 失败: %v", err)
			}
			if _, err := ParseManifest(data); err == nil {
				t.Fatal("非法 Manifest 未被拒绝")
			}
		})
	}
}

func TestParseManifestRejectsTrailingJSONAndLongText(t *testing.T) {
	trailing := append(validManifestData(t), []byte(` {}`)...)
	if _, err := ParseManifest(trailing); err == nil {
		t.Fatal("多余 JSON 未被拒绝")
	}

	value := validManifestValue()
	value["name"] = strings.Repeat("名", 101)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("编码长名称失败: %v", err)
	}
	if _, err := ParseManifest(data); err == nil {
		t.Fatal("超长名称未被拒绝")
	}

	invalidUTF8 := append([]byte(nil), validManifestData(t)...)
	invalidUTF8[len(invalidUTF8)-2] = 0xff
	if _, err := ParseManifest(invalidUTF8); err == nil {
		t.Fatal("非法 UTF-8 未被拒绝")
	}
}

func TestManifestCheckHostVersion(t *testing.T) {
	manifest, err := ParseManifest(validManifestData(t))
	if err != nil {
		t.Fatalf("解析 Manifest 失败: %v", err)
	}
	tests := []struct {
		version string
		wantErr bool
	}{
		{version: "2.0.0"},
		{version: "2.0.1"},
		{version: "3.0.0-beta.1"},
		{version: "2.0.0-beta.1", wantErr: true},
		{version: "1.99.99", wantErr: true},
		{version: "invalid", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			err := manifest.CheckHostVersion(test.version)
			if (err != nil) != test.wantErr {
				t.Fatalf("CheckHostVersion(%q) error = %v, wantErr = %v", test.version, err, test.wantErr)
			}
		})
	}
}

func validManifestData(t *testing.T) []byte {
	t.Helper()
	data, err := json.Marshal(validManifestValue())
	if err != nil {
		t.Fatalf("编码合法 Manifest 失败: %v", err)
	}

	return data
}

func validManifestValue() map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"name":          "Echo 插件",
		"key":           "echo-plugin",
		"hook":          "echo",
		"singleton":     true,
		"version":       "1.2.3-beta.1+build.5",
		"description":   "测试插件",
		"author":        "COOL",
		"logo":          "assets/logo.png",
		"readme":        "README.md",
		"runtime": map[string]any{
			"abi":            "cool.plugin/v1",
			"module":         "plugin.wasm",
			"minHostVersion": "2.0.0",
		},
		"config": map[string]any{"message": "hello"},
	}
}
