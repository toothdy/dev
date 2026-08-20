package codegen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteGeneratedFilesDoesNotRewriteUnchangedContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "modules", "sample", generatedFileName)
	file := GeneratedFile{Path: path, Content: []byte(generatedHeader + "\npackage sample\n")}
	if err := WriteGeneratedFiles([]GeneratedFile{file}); err != nil {
		t.Fatalf("首次写入生成文件失败: %v", err)
	}
	first, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取生成文件失败: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err = WriteGeneratedFiles([]GeneratedFile{file}); err != nil {
		t.Fatalf("重复写入生成文件失败: %v", err)
	}
	second, err := os.Stat(path)
	if err != nil {
		t.Fatalf("读取生成文件失败: %v", err)
	}
	if !first.ModTime().Equal(second.ModTime()) {
		t.Fatalf("内容未变时不应改写文件")
	}
}

func TestReconcileRemovesOnlyGeneratedModuleFiles(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "modules", globalGeneratedFile)
	stalePath := filepath.Join(root, "modules", "sample", generatedFileName)
	writeScannerFile(t, stalePath, generatedHeader+"\npackage sample\n")
	candidate := GeneratedFile{Path: globalPath, Content: []byte(generatedHeader + "\npackage modules\n")}
	if err := ReconcileGeneratedFiles(root, []GeneratedFile{candidate}); err != nil {
		t.Fatalf("事务替换失败: %v", err)
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("旧模块生成文件未删除: %v", err)
	}
	if content, err := os.ReadFile(globalPath); err != nil || string(content) != string(candidate.Content) {
		t.Fatalf("全局候选未写入: content=%q err=%v", content, err)
	}
}

func TestReconcileRefusesHandwrittenModuleGenWithoutPartialChanges(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "modules", globalGeneratedFile)
	stalePath := filepath.Join(root, "modules", "sample", generatedFileName)
	oldGlobal := []byte(generatedHeader + "\npackage modules\n// old\n")
	writeScannerFile(t, globalPath, string(oldGlobal))
	writeScannerFile(t, stalePath, "package sample\n")
	err := ReconcileGeneratedFiles(root, []GeneratedFile{{Path: globalPath, Content: []byte(generatedHeader + "\npackage modules\n// new\n")}})
	if err == nil || !strings.Contains(err.Error(), "Header") {
		t.Fatalf("手写 module_gen.go 未被拒绝: %v", err)
	}
	content, readErr := os.ReadFile(globalPath)
	if readErr != nil || string(content) != string(oldGlobal) {
		t.Fatalf("预检失败后全局文件被修改: content=%q err=%v", content, readErr)
	}
}

func TestReconcileRollsBackRenameFailure(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "modules", globalGeneratedFile)
	stalePath := filepath.Join(root, "modules", "sample", generatedFileName)
	oldGlobal := []byte(generatedHeader + "\npackage modules\n// old\n")
	oldStale := []byte(generatedHeader + "\npackage sample\n")
	writeScannerFile(t, globalPath, string(oldGlobal))
	writeScannerFile(t, stalePath, string(oldStale))
	originalRename := writerRename
	calls := 0
	writerRename = func(oldPath string, newPath string) error {
		calls++
		if calls == 2 {
			return errors.New("注入 rename 失败")
		}
		return originalRename(oldPath, newPath)
	}
	t.Cleanup(func() { writerRename = originalRename })
	err := ReconcileGeneratedFiles(root, []GeneratedFile{{Path: globalPath, Content: []byte(generatedHeader + "\npackage modules\n// new\n")}})
	if err == nil {
		t.Fatal("注入 rename 失败未返回错误")
	}
	for path, want := range map[string][]byte{globalPath: oldGlobal, stalePath: oldStale} {
		content, readErr := os.ReadFile(path)
		if readErr != nil || string(content) != string(want) {
			t.Fatalf("事务回滚不完整 %s: content=%q err=%v", path, content, readErr)
		}
	}
}

func TestReconcileKeepsMtimeForUnchangedGlobalAndStillRemovesStale(t *testing.T) {
	root := t.TempDir()
	globalPath := filepath.Join(root, "modules", globalGeneratedFile)
	stalePath := filepath.Join(root, "modules", "sample", generatedFileName)
	content := []byte(generatedHeader + "\npackage modules\n")
	writeScannerFile(t, globalPath, string(content))
	writeScannerFile(t, stalePath, generatedHeader+"\npackage sample\n")
	before, err := os.Stat(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err = ReconcileGeneratedFiles(root, []GeneratedFile{{Path: globalPath, Content: content}}); err != nil {
		t.Fatalf("清理陈旧文件失败: %v", err)
	}
	after, err := os.Stat(globalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("未变化全局文件的 mtime 被修改")
	}
	if _, err = os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatalf("陈旧模块文件未清理: %v", err)
	}
}

func TestCheckGeneratedFilesReportsDriftWithoutWriting(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "modules", "sample", generatedFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建模块目录失败: %v", err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("写入旧文件失败: %v", err)
	}
	err := CheckGeneratedFiles([]GeneratedFile{{Path: path, Content: []byte("new")}})
	if err == nil {
		t.Fatal("应报告生成文件过期")
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil || string(content) != "old" {
		t.Fatalf("check 不得改写文件: content=%q err=%v", content, readErr)
	}
}
