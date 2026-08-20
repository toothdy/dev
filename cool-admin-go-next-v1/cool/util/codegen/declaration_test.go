package codegen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeConfigDeclaration(t *testing.T) {
	root := createAnalysisProject(t)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描声明项目失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("分析 ModuleConfig 失败: %v", err)
	}
	declaration := analyses[0].Declaration
	if declaration.Name != "示例模块" || declaration.Description != "示例模块描述" || declaration.Order != 10 {
		t.Fatalf("静态声明元信息不符: %#v", declaration)
	}
	if declaration.ConfigType == nil || resultTypeName(declaration.ConfigType) != "Config" || declaration.Defaults == nil {
		t.Fatalf("Config 类型或 Defaults 未提取: %#v", declaration)
	}
}

func TestAnalyzeRejectsMissingConfig(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), "package sample\ntype Config struct{}\n")
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描缺失 ModuleConfig 声明项目失败: %v", err)
	}
	_, err = Analyze(project)
	if err == nil || !strings.Contains(err.Error(), "ModuleConfig") || !strings.Contains(err.Error(), "sample") {
		t.Fatalf("缺失 ModuleConfig 未被拒绝: %v", err)
	}
}

func TestAnalyzeRejectsInvalidConfigTagAndRuntimeType(t *testing.T) {
	tests := []struct {
		name       string
		configType string
		want       string
	}{
		{name: "缺少标签", configType: "type Config struct { Enabled bool }", want: "json"},
		{name: "重复标签", configType: "type Config struct { First bool `json:\"same\"`; Second bool `json:\"same\"` }", want: "重复"},
		{name: "运行时资源", configType: "type Config struct { Callback func() `json:\"callback\"` }", want: "纯配置"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := createAnalysisProject(t)
			content := `package sample
import "github.com/toothdy/cool-admin-go-next/cool/module"
` + test.configType + `
func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{Name: "示例", Description: "描述", Defaults: Config{}}
}
`
			writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), content)
			project, err := Scan(ScanOptions{Root: root})
			if err != nil {
				t.Fatalf("扫描非法配置项目失败: %v", err)
			}
			_, err = Analyze(project)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("非法 Config 未被拒绝，期望 %q: %v", test.want, err)
			}
		})
	}
}

func TestAnalyzeRejectsRootConfigResponsibilities(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), `package sample
import (
   "context"
   "github.com/gogf/gf/v2/frame/g"
   "github.com/toothdy/cool-admin-go-next/cool/module"
)
type Config struct { Enabled bool `+"`json:\"enabled\"`"+` }
func ModuleConfig() module.Declaration[Config] {
   _ = g.Cfg()
   return module.Declaration[Config]{Name: "示例", Description: "描述", Defaults: Config{}}
}
func NewRuntime(context.Context) string { return "" }
`)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描根职责项目失败: %v", err)
	}
	_, err = Analyze(project)
	if err == nil || (!strings.Contains(err.Error(), "g.Cfg") && !strings.Contains(err.Error(), "根配置")) {
		t.Fatalf("根 config.go 装配职责未被拒绝: %v", err)
	}
}

func TestAnalyzeDoesNotRecognizeLoadConfigProvider(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "config.go"), `package service
type Config struct{}
func LoadConfig() Config { return Config{} }
`)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描 LoadConfig 项目失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("分析 LoadConfig 项目失败: %v", err)
	}
	for _, component := range analyses[0].Components {
		if component.Function == "LoadConfig" {
			t.Fatalf("LoadConfig 仍被识别为 Provider: %#v", component)
		}
	}
}
