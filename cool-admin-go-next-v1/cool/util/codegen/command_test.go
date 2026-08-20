package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAlwaysBuildsWholeProject(t *testing.T) {
	root := createAnalysisProject(t)
	if err := os.MkdirAll(filepath.Join(root, "modules", "broken", "service"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := Execute(GenerateOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "config.go") {
		t.Fatalf("全项目生成未发现另一模块错误: %v", err)
	}
}

func TestConventionOnlyModuleEntersGeneratedSpecs(t *testing.T) {
	root := createAnalysisProject(t)
	if err := Execute(GenerateOptions{Root: root}); err != nil {
		t.Fatalf("generate convention-only module failed: %v", err)
	}
	writeScannerFile(t, filepath.Join(root, "modules", "specs_test.go"), `package modules
import "testing"
func TestGeneratedSpecsContainsSample(t *testing.T) {
   specs := Specs()
   if len(specs) != 1 || specs[0].Key != "sample" {
      t.Fatalf("unexpected generated specs: %#v", specs)
   }
}
`)
	command := exec.Command("go", "test", "./modules", "-run", "TestGeneratedSpecsContainsSample", "-count=1")
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run generated Specs test failed: %v\n%s", err, output)
	}
}

func TestGenerateFailurePreservesPreviousOutput(t *testing.T) {
	root := createAnalysisProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "invalid.go"), `package service
type Service struct{}
func NewService(values ...string) *Service { return &Service{} }
`)
	output := filepath.Join(root, "modules", "sample", generatedFileName)
	previous := []byte(generatedHeader + "\npackage sample\n")
	if err := os.WriteFile(output, previous, 0o644); err != nil {
		t.Fatalf("写入旧生成文件失败: %v", err)
	}

	err := Execute(GenerateOptions{Root: root})
	if err == nil {
		t.Fatal("无效 Provider 应使生成失败")
	}
	content, readErr := os.ReadFile(output)
	if readErr != nil || string(content) != string(previous) {
		t.Fatalf("生成失败不得覆盖旧输出: content=%q err=%v", content, readErr)
	}
}
