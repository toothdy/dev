package codegen

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeServiceHandlerDefinition(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "demo.go"), `package service
import "github.com/toothdy/cool-admin-go-next/cool/task"
func DemoDefinition() task.HandlerDefinition { return task.HandlerDefinition{} }
`)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描 Task 定义失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("分析 Task 定义失败: %v", err)
	}
	if len(analyses[0].Components) != 1 || analyses[0].Components[0].Kind != ComponentHandler || analyses[0].Components[0].Function != "DemoDefinition" {
		t.Fatalf("service Task 定义分类错误: %#v", analyses[0].Components)
	}
}

func TestAnalyzeRejectsHandlerDefinitionOutsideService(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "event", "demo.go"), `package event
import "github.com/toothdy/cool-admin-go-next/cool/task"
func DemoDefinition() task.HandlerDefinition { return task.HandlerDefinition{} }
`)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描错位 Task 定义失败: %v", err)
	}
	_, err = Analyze(project)
	if err == nil || !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "DemoDefinition") {
		t.Fatalf("service 外 Task 定义未被拒绝: %v", err)
	}
}

func TestAnalyzeResolvesMiddlewareReferencesInDeclarationOrder(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), analysisConfigWithRefs(
		`[]module.ComponentRef{"middleware#Second", "middleware#First"}`,
		`[]module.ComponentRef{"middleware/global#Global"}`,
	))
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "middleware", "middleware.go"), `package middleware
import "github.com/toothdy/cool-admin-go-next/cool/middleware"
func First() middleware.Definition { return middleware.Definition{} }
func Second() []middleware.Definition { return nil }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "middleware", "global", "global.go"), `package global
import "github.com/toothdy/cool-admin-go-next/cool/middleware"
func Global() (middleware.Definition, error) { return middleware.Definition{}, nil }
`)
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描中间件引用失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("解析中间件引用失败: %v", err)
	}
	functions := make([]string, 0, len(analyses[0].Components))
	for _, component := range analyses[0].Components {
		functions = append(functions, component.Function)
	}
	if !reflect.DeepEqual(functions, []string{"Second", "First", "Global"}) {
		t.Fatalf("中间件未按声明顺序保存: %#v", functions)
	}
}

func TestAnalyzeRejectsInvalidMiddlewareReferences(t *testing.T) {
	tests := []struct {
		name        string
		middlewares string
		globals     string
		want        string
	}{
		{name: "格式错误", middlewares: `[]module.ComponentRef{"middleware"}`, want: "格式"},
		{name: "重复引用", middlewares: `[]module.ComponentRef{"middleware#First", "middleware#First"}`, want: "重复"},
		{name: "作用域错误", globals: `[]module.ComponentRef{"middleware#First"}`, want: "作用域"},
		{name: "漏声明", want: "未声明"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := createAnalysisProject(t)
			writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), analysisConfigWithRefs(test.middlewares, test.globals))
			writeScannerFile(t, filepath.Join(root, "modules", "sample", "middleware", "middleware.go"), `package middleware
import "github.com/toothdy/cool-admin-go-next/cool/middleware"
func First() middleware.Definition { return middleware.Definition{} }
`)
			project, err := Scan(ScanOptions{Root: root})
			if err != nil {
				t.Fatalf("扫描非法中间件引用失败: %v", err)
			}
			_, err = Analyze(project)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("非法中间件引用未被拒绝，期望 %q: %v", test.want, err)
			}
		})
	}
}

func analysisConfigWithRefs(middlewares string, globals string) string {
	middlewareField := ""
	if middlewares != "" {
		middlewareField = "Middlewares: " + middlewares + ","
	}
	globalField := ""
	if globals != "" {
		globalField = "GlobalMiddlewares: " + globals + ","
	}
	return `package sample
import "github.com/toothdy/cool-admin-go-next/cool/module"
type Config struct{}
func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{
      Name: "示例模块",
      Description: "示例模块描述",
      ` + middlewareField + `
      ` + globalField + `
      Defaults: Config{},
   }
}
`
}
