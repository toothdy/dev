package codegen

import (
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderProjectProducesOnlyGlobalFile(t *testing.T) {
	modelType := namedTestType(modelPackagePath, "Definition", types.NewStruct(nil, nil))
	configType := namedTestType("example.com/project/modules/sample", "Config", types.NewStruct(nil, nil))
	serviceType := namedTestType("example.com/project/modules/sample/service", "UserService", types.NewStruct(nil, nil))
	analysis := Analysis{
		Module:      DiscoveredModule{Key: "sample", Dir: "/project/modules/sample"},
		Declaration: ModuleDeclaration{Key: "sample", Name: "示例", Description: "描述", ConfigType: configType},
		Components: []Component{
			{Kind: ComponentModel, ModuleKey: "sample", Function: "User", ImportPath: "example.com/project/modules/sample/entity", Output: typeID(modelType), OutputType: modelType},
			{Kind: ComponentProvider, ModuleKey: "sample", Function: "NewUserService", ImportPath: "example.com/project/modules/sample/service", Output: typeID(serviceType), OutputType: serviceType},
		},
	}
	graph, err := ResolveProjectGraph([]Analysis{analysis})
	if err != nil {
		t.Fatalf("解析渲染图失败: %v", err)
	}
	project := &Project{Root: "/project", ModulePath: "example.com/project", Modules: []DiscoveredModule{analysis.Module}}

	files, err := RenderProject(project, []Analysis{analysis}, graph)
	if err != nil {
		t.Fatalf("渲染项目失败: %v", err)
	}
	if len(files) != 1 || files[0].Path != filepath.Join(project.Root, "modules", globalGeneratedFile) {
		t.Fatalf("Renderer 产生了非全局候选: %#v", files)
	}
	text := string(files[0].Content)
	for _, expected := range []string{
		"//go:build !cool_generate",
		generatedHeader,
		"func Specs() []module.Spec",
		"module.LoadConfig",
		".User()",
		".NewUserService(",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("生成结果缺少 %q:\n%s", expected, text)
		}
	}
	if strings.Contains(text, "reflect") || strings.Contains(text, "time.Now") || strings.Contains(text, "func Module()") || strings.Contains(text, "init()") {
		t.Fatalf("生成结果不得包含运行时反射或时间戳:\n%s", text)
	}
}

func TestGeneratedOverlayTypeChecks(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "entity", "user.go"), `package entity
import "github.com/toothdy/cool-admin-go-next/cool/entity"
func User() entity.Definition { return entity.Definition{} }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "user.go"), `package service
type UserService struct{}
func NewUserService() *UserService { return &UserService{} }
`)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "controller", "user.go"), `package controller
import (
   "github.com/toothdy/cool-admin-go-next/cool/controller"
   "github.com/toothdy/cool-admin-go-next/cool/entity"
   "example.com/analyzer/modules/sample/service"
)
func UserController(service *service.UserService, userModel entity.Definition) controller.Definition {
   return controller.Definition{Module: "sample"}
}
func UserAuditController(service *service.UserService, userModel entity.Definition) (controller.Definition, error) {
   return controller.Definition{Module: "sample"}, nil
}
`)
	project, files, err := BuildGeneratedFiles(GenerateOptions{Root: root})
	if err != nil {
		t.Fatalf("构建生成候选失败: %v", err)
	}
	if err = ValidateGeneratedFiles(project, files); err != nil {
		t.Fatalf("生成候选类型检查失败: %v", err)
	}
	for _, file := range files {
		if strings.Contains(string(file.Content), "panic(err)") {
			t.Fatalf("Controller 构造错误不得生成 panic: %s", file.Path)
		}
	}
}

func TestGeneratedOverlayTypeChecksCrossModuleDependencies(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "base.go"), `package service
type Base struct{}
func NewBase() *Base { return &Base{} }
`)
	dictConfig := strings.Replace(validAnalysisConfig, "package sample", "package dict", 1)
	dictConfig = strings.Replace(dictConfig, "示例模块", "字典模块", 1)
	writeScannerFile(t, filepath.Join(root, "modules", "dict", "config.go"), dictConfig)
	writeScannerFile(t, filepath.Join(root, "modules", "dict", "service", "dict.go"), `package service
import sampleService "example.com/analyzer/modules/sample/service"
type DictService struct{}
func NewDictService(baseService *sampleService.Base) *DictService { return &DictService{} }
`)
	project, files, err := BuildGeneratedFiles(GenerateOptions{Root: root})
	if err != nil {
		t.Fatalf("构建跨模块候选失败: %v", err)
	}
	if err = ValidateGeneratedFiles(project, files); err != nil {
		t.Fatalf("跨模块候选类型检查失败: %v\n%s", err, files[0].Content)
	}
	text := string(files[0].Content)
	if strings.Index(text, "Key: \"sample\"") > strings.Index(text, "Key: \"dict\"") {
		t.Fatalf("依赖模块未排在消费者之前:\n%s", text)
	}
}

func TestTypeIDUsesFullImportPath(t *testing.T) {
	named := types.NewNamed(types.NewTypeName(token.NoPos, types.NewPackage("example.com/a", "a"), "Value", nil), types.NewStruct(nil, nil), nil)
	if got := typeID(named); got != "example.com/a.Value" {
		t.Fatalf("类型 ID 不符: %s", got)
	}
}
