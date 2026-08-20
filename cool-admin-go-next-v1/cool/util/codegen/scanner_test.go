package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFindProjectRootFromNestedDirectory(t *testing.T) {
	root := createScannerProject(t)
	nested := filepath.Join(root, "modules", "base", "service", "sys")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("创建嵌套目录失败: %v", err)
	}

	resolved, err := FindProjectRoot(nested)
	if err != nil {
		t.Fatalf("定位项目根目录失败: %v", err)
	}
	if resolved != root {
		t.Fatalf("项目根目录不符: got=%s want=%s", resolved, root)
	}
}

func TestDiscoverModulesUsesOnlyDirectChildren(t *testing.T) {
	root := createScannerProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "base", "config.go"), "package base\n")
	writeScannerFile(t, filepath.Join(root, "modules", "base", "service", "sys", "sys.go"), "package sys\n")
	writeScannerFile(t, filepath.Join(root, "modules", "dict", "config.go"), "package dict\n")
	writeScannerFile(t, filepath.Join(root, "modules", ".hidden", "hidden.go"), "package hidden\n")
	writeScannerFile(t, filepath.Join(root, "modules", "testdata", "fixture", "fixture.go"), "package fixture\n")

	modules, err := DiscoverModules(root)
	if err != nil {
		t.Fatalf("发现模块失败: %v", err)
	}
	keys := make([]string, 0, len(modules))
	for _, discovered := range modules {
		keys = append(keys, discovered.Key)
	}
	if !reflect.DeepEqual(keys, []string{"base", "dict"}) {
		t.Fatalf("模块列表不符: %#v", keys)
	}
}

func TestScanRejectsPlatformSpecificComponent(t *testing.T) {
	root := createScannerProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "base", "config.go"), "package base\n")
	writeScannerFile(t, filepath.Join(root, "modules", "base", "service", "store_linux.go"), "package service\n")

	_, err := Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "平台专属") {
		t.Fatalf("应拒绝平台专属组件: %v", err)
	}
}

func TestScanRequiresRootConfigGo(t *testing.T) {
	root := createScannerProject(t)
	writeScannerFile(t, filepath.Join(root, "modules", "sample", "service", "demo.go"), "package service\n")
	_, err := Scan(ScanOptions{Root: root})
	if err == nil || !strings.Contains(err.Error(), "sample") || !strings.Contains(err.Error(), "config.go") {
		t.Fatalf("缺失 config.go 未被拒绝: %v", err)
	}
}

func TestScanAcceptsCompleteModuleRootAllowlist(t *testing.T) {
	root := createScannerProject(t)
	moduleDir := filepath.Join(root, "modules", "sample")
	writeScannerFile(t, filepath.Join(moduleDir, "config.go"), "package sample\n")
	writeScannerFile(t, filepath.Join(moduleDir, "db.json"), "[]")
	writeScannerFile(t, filepath.Join(moduleDir, "menu.json"), "[]")
	for _, directory := range []string{"controller", "dto", "entity", "middleware", "schedule", "service", "event", "queue"} {
		if err := os.MkdirAll(filepath.Join(moduleDir, directory), 0o755); err != nil {
			t.Fatalf("创建允许目录失败: %v", err)
		}
	}
	project, err := Scan(ScanOptions{Root: root})
	if err != nil || len(project.Modules) != 1 {
		t.Fatalf("完整 allowlist 模块扫描失败: project=%#v err=%v", project, err)
	}
}

func TestScanRejectsUnknownRootEntry(t *testing.T) {
	tests := []string{"config", "handler", "db", "hooks", "job", "providers.go", "module_gen.go", "config_test.go", "other.txt"}
	for _, entry := range tests {
		t.Run(entry, func(t *testing.T) {
			root := createScannerProject(t)
			moduleDir := filepath.Join(root, "modules", "sample")
			writeScannerFile(t, filepath.Join(moduleDir, "config.go"), "package sample\n")
			path := filepath.Join(moduleDir, entry)
			if filepath.Ext(entry) == "" {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("创建非法目录失败: %v", err)
				}
			} else {
				writeScannerFile(t, path, "package sample\n")
			}
			_, err := Scan(ScanOptions{Root: root})
			if err == nil || !strings.Contains(err.Error(), "sample") || !strings.Contains(err.Error(), entry) {
				t.Fatalf("非法根条目未被拒绝: %v", err)
			}
		})
	}
}

func TestScanToleratesGeneratedModuleGenOnlyForCleanup(t *testing.T) {
	root := createScannerProject(t)
	moduleDir := filepath.Join(root, "modules", "sample")
	writeScannerFile(t, filepath.Join(moduleDir, "config.go"), "package sample\n")
	legacy := filepath.Join(moduleDir, generatedFileName)
	writeScannerFile(t, legacy, "//go:build !cool_generate\n\n"+generatedHeader+"\n\npackage sample\n")
	project, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("带 Header 的旧生成文件应被容忍: %v", err)
	}
	if len(project.StaleGeneratedFiles) != 1 || project.StaleGeneratedFiles[0] != legacy {
		t.Fatalf("旧生成文件未进入清理清单: %#v", project.StaleGeneratedFiles)
	}
}

func createScannerProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeScannerFile(t, filepath.Join(root, "go.mod"), "module example.com/scanner\n\ngo 1.26.0\n")
	if err := os.MkdirAll(filepath.Join(root, "modules"), 0o755); err != nil {
		t.Fatalf("创建 modules 失败: %v", err)
	}
	return root
}

func writeScannerFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
}
