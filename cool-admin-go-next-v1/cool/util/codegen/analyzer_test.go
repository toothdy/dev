package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAnalyzeRecognizesTypedComponents(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "entity", "user.go"), `package entity
import "github.com/toothdy/cool-admin-go-next/cool/entity"
func User() entity.Definition { return entity.Definition{} }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "user.go"), `package service
import "github.com/gogf/gf/v2/database/gdb"
type UserService struct{}
func NewUserService(db gdb.DB) *UserService { return &UserService{} }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "controller", "user.go"), `package controller
import (
   "github.com/toothdy/cool-admin-go-next/cool/controller"
   "github.com/toothdy/cool-admin-go-next/cool/entity"
   "example.com/analyzer/modules/sample/service"
)
func UserController(service *service.UserService, userModel entity.Definition) controller.Definition {
   return controller.Definition{}
}
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "schedule", "runtime.go"), `package schedule
import "context"
type Runtime struct{}
func (r *Runtime) Start(ctx context.Context) error { return nil }
func (r *Runtime) Stop(ctx context.Context) error { return nil }
func NewRuntime() *Runtime { return &Runtime{} }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "demo.go"), `package service
import "github.com/toothdy/cool-admin-go-next/cool/task"
func DemoDefinition() task.HandlerDefinition { return task.HandlerDefinition{} }
`)

	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描测试项目失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("分析组件失败: %v", err)
	}
	kinds := make([]ComponentKind, 0, len(analyses[0].Components))
	for _, component := range analyses[0].Components {
		kinds = append(kinds, component.Kind)
	}
	want := []ComponentKind{ComponentController, ComponentModel, ComponentRuntime, ComponentHandler, ComponentProvider}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("组件分类不符: got=%#v want=%#v", kinds, want)
	}
}

func TestAnalyzeRejectsVariadicProvider(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "user.go"), `package service
type UserService struct{}
func NewUserService(values ...string) *UserService { return &UserService{} }
`)

	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描测试项目失败: %v", err)
	}
	_, err = Analyze(project)
	if err == nil || !strings.Contains(err.Error(), "可变参数") {
		t.Fatalf("应拒绝可变 Provider: %v", err)
	}
}

func TestAnalyzeRejectsInvalidControllerSignature(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "controller", "user.go"), `package controller
func UserController() string { return "invalid" }
`)

	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描测试项目失败: %v", err)
	}
	_, err = Analyze(project)
	if err == nil || !strings.Contains(err.Error(), "Controller") {
		t.Fatalf("应拒绝无效 Controller: %v", err)
	}
}

func TestAnalyzeIgnoresNonCanonicalNewHelper(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "user.go"), `package service
type UserService struct{}
func NewUserService() *UserService { return &UserService{} }
func NewUserServiceWithCache(cache string) *UserService { return &UserService{} }
`)

	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("扫描测试项目失败: %v", err)
	}
	analyses, err := Analyze(project)
	if err != nil {
		t.Fatalf("非标准 New 辅助构造函数不应参与分析: %v", err)
	}
	if len(analyses) != 1 || len(analyses[0].Components) != 1 || analyses[0].Components[0].Function != "NewUserService" {
		t.Fatalf("辅助构造函数被错误识别: %#v", analyses)
	}
}

func createAnalysisProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	projectRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("解析主项目目录失败: %v", err)
	}
	goMod := "module example.com/analyzer\n\ngo 1.26.0\n\nrequire github.com/toothdy/cool-admin-go-next v0.0.0\n\nreplace github.com/toothdy/cool-admin-go-next => " + projectRoot + "\n"
	writeScannerFile(t, filepath.Join(root, "go.mod"), goMod)
	if err = os.MkdirAll(filepath.Join(root, "modules", "sample"), 0o755); err != nil {
		t.Fatalf("创建测试模块失败: %v", err)
	}
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "config.go"), validAnalysisConfig)
	return root
}

const validAnalysisConfig = `package sample
import "github.com/toothdy/cool-admin-go-next/cool/module"
type Config struct {
   Enabled bool ` + "`json:\"enabled\"`" + `
}
func ModuleConfig() module.Declaration[Config] {
   return module.Declaration[Config]{
      Name: "示例模块",
      Description: "示例模块描述",
      Order: 10,
      Defaults: Config{Enabled: true},
   }
}
`
