package codegen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var (
	writerRename = os.Rename
	writerRemove = os.Remove
)

// GeneratedFile 表示一个内存中的候选生成文件。
type GeneratedFile struct {
	Path    string
	Content []byte
}

// CheckGeneratedFiles 只比较候选内容，不修改磁盘。
func CheckGeneratedFiles(files []GeneratedFile) error {
	drift := make([]string, 0)
	for _, file := range sortedGeneratedFiles(files) {
		content, err := os.ReadFile(file.Path)
		if err != nil {
			if os.IsNotExist(err) {
				drift = append(drift, file.Path+" (缺失)")
				continue
			}
			return fmt.Errorf("读取生成文件 %s 失败: %w", file.Path, err)
		}
		if !bytes.Equal(content, file.Content) {
			drift = append(drift, file.Path+" (过期)")
		}
	}
	if len(drift) > 0 {
		return fmt.Errorf("生成文件未同步: %s", strings.Join(drift, ", "))
	}
	return nil
}

// WriteGeneratedFiles 使用同目录临时文件原子替换已变内容。
func WriteGeneratedFiles(files []GeneratedFile) error {
	for _, file := range sortedGeneratedFiles(files) {
		current, err := os.ReadFile(file.Path)
		if err == nil && bytes.Equal(current, file.Content) {
			continue
		}
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("读取生成文件 %s 失败: %w", file.Path, err)
		}
		if err = os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return fmt.Errorf("创建生成目录 %s 失败: %w", filepath.Dir(file.Path), err)
		}
		temporary, err := os.CreateTemp(filepath.Dir(file.Path), ".cool-generate-*")
		if err != nil {
			return fmt.Errorf("创建临时生成文件失败: %w", err)
		}
		temporaryPath := temporary.Name()
		removeTemporary := true
		defer func() {
			if removeTemporary {
				_ = os.Remove(temporaryPath)
			}
		}()
		if _, err = temporary.Write(file.Content); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("写入临时生成文件失败: %w", err)
		}
		if err = temporary.Chmod(0o644); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("设置生成文件权限失败: %w", err)
		}
		if err = temporary.Close(); err != nil {
			return fmt.Errorf("关闭临时生成文件失败: %w", err)
		}
		if err = os.Rename(temporaryPath, file.Path); err != nil {
			return fmt.Errorf("替换生成文件 %s 失败: %w", file.Path, err)
		}
		removeTemporary = false
	}
	return nil
}

type reconciledBackup struct {
	path       string
	backupPath string
	content    []byte
	mode       os.FileMode
}

// ReconcileGeneratedFiles 事务替换全局文件并清理旧模块生成文件。
func ReconcileGeneratedFiles(root string, files []GeneratedFile) error {
	if len(files) != 1 {
		return fmt.Errorf("集中装配必须且只能包含一个候选文件")
	}
	candidate := files[0]
	expectedPath := filepath.Join(root, "modules", globalGeneratedFile)
	if filepath.Clean(candidate.Path) != filepath.Clean(expectedPath) || !bytes.Contains(candidate.Content, []byte(generatedHeader)) {
		return fmt.Errorf("集中装配候选路径或 Header 无效: %s", candidate.Path)
	}
	stale, err := staleGeneratedFiles(root, files)
	if err != nil {
		return err
	}
	for _, path := range stale {
		if filepath.Base(path) != generatedFileName {
			continue
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("读取旧生成文件 %s 失败: %w", path, readErr)
		}
		if !bytes.Contains(content, []byte(generatedHeader)) {
			return fmt.Errorf("拒绝删除无标准 Header 的文件: %s", path)
		}
	}
	current, readErr := os.ReadFile(candidate.Path)
	hasCurrent := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return fmt.Errorf("读取生成文件 %s 失败: %w", candidate.Path, readErr)
	}
	if hasCurrent && !bytes.Contains(current, []byte(generatedHeader)) {
		return fmt.Errorf("拒绝替换无标准 Header 的文件: %s", candidate.Path)
	}
	isChanged := !hasCurrent || !bytes.Equal(current, candidate.Content)

	var stagedPath string
	if isChanged {
		if err = os.MkdirAll(filepath.Dir(candidate.Path), 0o755); err != nil {
			return fmt.Errorf("创建生成目录失败: %w", err)
		}
		stagedPath, err = writeStagedFile(candidate.Path, candidate.Content)
		if err != nil {
			return err
		}
		defer func() { _ = writerRemove(stagedPath) }()
	}

	targets := make([]string, 0, len(stale)+1)
	if isChanged && hasCurrent {
		targets = append(targets, candidate.Path)
	}
	for _, path := range stale {
		if filepath.Base(path) == generatedFileName {
			targets = append(targets, path)
		}
	}
	sort.Strings(targets)
	backups := make([]reconciledBackup, 0, len(targets))
	installed := false
	rollback := func() {
		if installed {
			_ = writerRemove(candidate.Path)
		}
		for index := len(backups) - 1; index >= 0; index-- {
			backup := backups[index]
			if _, statErr := os.Stat(backup.backupPath); statErr == nil {
				_ = writerRename(backup.backupPath, backup.path)
				continue
			}
			_ = os.MkdirAll(filepath.Dir(backup.path), 0o755)
			_ = os.WriteFile(backup.path, backup.content, backup.mode)
		}
	}
	for _, path := range targets {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			rollback()
			return fmt.Errorf("读取事务源文件 %s 失败: %w", path, readErr)
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			rollback()
			return fmt.Errorf("读取事务源文件属性 %s 失败: %w", path, statErr)
		}
		backupPath, reserveErr := reserveBackupPath(path)
		if reserveErr != nil {
			rollback()
			return reserveErr
		}
		if err = writerRename(path, backupPath); err != nil {
			_ = writerRemove(backupPath)
			rollback()
			return fmt.Errorf("暂存旧生成文件 %s 失败: %w", path, err)
		}
		backups = append(backups, reconciledBackup{path: path, backupPath: backupPath, content: content, mode: info.Mode()})
	}
	if isChanged {
		if err = writerRename(stagedPath, candidate.Path); err != nil {
			rollback()
			return fmt.Errorf("安装全局生成文件失败: %w", err)
		}
		installed = true
		stagedPath = ""
	}
	for _, backup := range backups {
		if err = writerRemove(backup.backupPath); err != nil {
			rollback()
			return fmt.Errorf("清理生成文件备份失败: %w", err)
		}
	}
	return nil
}

func writeStagedFile(target string, content []byte) (string, error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".cool-generate-*")
	if err != nil {
		return "", fmt.Errorf("创建临时生成文件失败: %w", err)
	}
	path := temporary.Name()
	if _, err = temporary.Write(content); err != nil {
		_ = temporary.Close()
		_ = writerRemove(path)
		return "", fmt.Errorf("写入临时生成文件失败: %w", err)
	}
	if err = temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		_ = writerRemove(path)
		return "", fmt.Errorf("设置临时生成文件权限失败: %w", err)
	}
	if err = temporary.Close(); err != nil {
		_ = writerRemove(path)
		return "", fmt.Errorf("关闭临时生成文件失败: %w", err)
	}
	return path, nil
}

func reserveBackupPath(target string) (string, error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".cool-backup-*")
	if err != nil {
		return "", fmt.Errorf("创建生成文件备份路径失败: %w", err)
	}
	path := temporary.Name()
	if err = temporary.Close(); err != nil {
		_ = writerRemove(path)
		return "", fmt.Errorf("关闭生成文件备份占位失败: %w", err)
	}
	if err = writerRemove(path); err != nil {
		return "", fmt.Errorf("释放生成文件备份占位失败: %w", err)
	}
	return path, nil
}

// RemoveStaleGeneratedFiles 只删除带标准 Header 的陈旧生成文件。
func RemoveStaleGeneratedFiles(root string, expected []GeneratedFile) error {
	keep := make(map[string]struct{}, len(expected))
	for _, file := range expected {
		keep[filepath.Clean(file.Path)] = struct{}{}
	}
	modulesDir := filepath.Join(root, "modules")
	return filepath.WalkDir(modulesDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (entry.Name() != generatedFileName && entry.Name() != globalGeneratedFile) {
			return nil
		}
		if _, ok := keep[filepath.Clean(path)]; ok {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Contains(content, []byte(generatedHeader)) {
			return fmt.Errorf("拒绝删除无标准 Header 的文件: %s", path)
		}
		return os.Remove(path)
	})
}

func sortedGeneratedFiles(files []GeneratedFile) []GeneratedFile {
	sorted := append([]GeneratedFile{}, files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	return sorted
}
