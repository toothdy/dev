package codegen

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// GenerateOptions 定义一次生成或检查操作。
type GenerateOptions struct {
	Root    string
	IsCheck bool
}

// Execute 扫描、分析、渲染并检查或更新生成文件。
func Execute(options GenerateOptions) error {
	project, files, err := BuildGeneratedFiles(options)
	if err != nil {
		return err
	}
	if err = ValidateGeneratedFiles(project, files); err != nil {
		return err
	}
	if options.IsCheck {
		if err = CheckGeneratedFiles(files); err != nil {
			return err
		}
		stale, staleErr := staleGeneratedFiles(project.Root, files)
		if staleErr != nil {
			return staleErr
		}
		if len(stale) > 0 {
			return fmt.Errorf("存在陈旧生成文件: %s", strings.Join(stale, ", "))
		}
		return nil
	}
	return ReconcileGeneratedFiles(project.Root, files)
}

// BuildGeneratedFiles 在内存中构建全部候选输出。
func BuildGeneratedFiles(options GenerateOptions) (*Project, []GeneratedFile, error) {
	project, err := Scan(ScanOptions{Root: options.Root})
	if err != nil {
		return nil, nil, err
	}
	analyses, err := Analyze(project)
	if err != nil {
		return nil, nil, err
	}
	graph, err := ResolveProjectGraph(analyses)
	if err != nil {
		return nil, nil, err
	}
	files, err := RenderProject(project, analyses, graph)
	if err != nil {
		return nil, nil, err
	}
	return project, files, nil
}

// ValidateGeneratedFiles 使用 go/packages Overlay 类型检查全部候选输出。
func ValidateGeneratedFiles(project *Project, files []GeneratedFile) error {
	overlay := make(map[string][]byte, len(files))
	for _, file := range files {
		absolute, err := filepath.Abs(file.Path)
		if err != nil {
			return fmt.Errorf("解析生成路径 %s 失败: %w", file.Path, err)
		}
		overlay[absolute] = file.Content
	}
	loaded, err := packages.Load(&packages.Config{
		Mode:    packages.LoadSyntax,
		Dir:     project.Root,
		Env:     append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod"),
		Tests:   false,
		Overlay: overlay,
	}, "./modules/...")
	if err != nil {
		return fmt.Errorf("生成结果 Overlay 加载失败: %w", err)
	}
	if err = collectPackageErrors(loaded); err != nil {
		return fmt.Errorf("生成结果 Overlay 类型检查失败: %w", err)
	}
	return nil
}

func staleGeneratedFiles(root string, expected []GeneratedFile) ([]string, error) {
	keep := make(map[string]struct{}, len(expected))
	for _, file := range expected {
		keep[filepath.Clean(file.Path)] = struct{}{}
	}
	stale := make([]string, 0)
	err := filepath.WalkDir(filepath.Join(root, "modules"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || (entry.Name() != generatedFileName && entry.Name() != globalGeneratedFile) {
			return nil
		}
		if _, ok := keep[filepath.Clean(path)]; !ok {
			stale = append(stale, path)
		}
		return nil
	})
	sort.Strings(stale)
	return stale, err
}
