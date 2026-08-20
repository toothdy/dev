package codegen

import (
	"bufio"
	"errors"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/packages"
)

var componentDirectories = map[string]struct{}{
	"controller": {},
	"dto":        {},
	"entity":     {},
	"event":      {},
	"middleware": {},
	"queue":      {},
	"schedule":   {},
	"service":    {},
}

// FindProjectRoot 从指定目录向上定位 go.mod。
func FindProjectRoot(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		current, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("读取当前目录失败: %w", err)
		}
		start = current
	}
	absolute, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("解析起始目录 %q 失败: %w", start, err)
	}
	for current := filepath.Clean(absolute); ; current = filepath.Dir(current) {
		info, statErr := os.Stat(filepath.Join(current, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return current, nil
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("检查 %s/go.mod 失败: %w", current, statErr)
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return "", fmt.Errorf("从 %s 向上未找到 go.mod", absolute)
}

// DiscoverModules 发现 modules 的直接子目录。
func DiscoverModules(root string) ([]DiscoveredModule, error) {
	modulesDir := filepath.Join(root, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		return nil, fmt.Errorf("读取模块目录 %s 失败: %w", modulesDir, err)
	}
	modules := make([]DiscoveredModule, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") || name == "testdata" || strings.HasPrefix(name, "_") {
			continue
		}
		modules = append(modules, DiscoveredModule{Key: name, Dir: filepath.Join(modulesDir, name)})
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Key < modules[j].Key })
	return modules, nil
}

// Scan 递归加载所有已选模块包。
func Scan(options ScanOptions) (*Project, error) {
	root, err := FindProjectRoot(options.Root)
	if err != nil {
		return nil, err
	}
	modulePath, err := readModulePath(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil, err
	}
	modules, err := DiscoverModules(root)
	if err != nil {
		return nil, err
	}
	staleGeneratedFiles := make([]string, 0)
	for index := range modules {
		stale, validateErr := validateModuleRoot(modules[index])
		if validateErr != nil {
			return nil, validateErr
		}
		if stale != "" {
			staleGeneratedFiles = append(staleGeneratedFiles, stale)
		}
		if err = validatePortableComponents(modules[index].Dir); err != nil {
			return nil, fmt.Errorf("模块 %s: %w", modules[index].Key, err)
		}
	}
	loaded, loadErr := packages.Load(&packages.Config{
		Mode:       packages.LoadSyntax | packages.NeedModule,
		Dir:        root,
		Env:        append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod"),
		Tests:      false,
		BuildFlags: []string{"-tags=cool_generate"},
		Overlay:    options.Overlay,
	}, "./modules/...")
	if loadErr != nil {
		return nil, fmt.Errorf("加载模块项目失败: %w", loadErr)
	}
	if packageErr := collectPackageErrors(loaded); packageErr != nil {
		return nil, fmt.Errorf("加载模块项目失败: %w", packageErr)
	}
	for index := range modules {
		prefix := modulePath + "/modules/" + modules[index].Key
		for _, loadedPackage := range loaded {
			if loadedPackage.PkgPath == prefix || strings.HasPrefix(loadedPackage.PkgPath, prefix+"/") {
				modules[index].Packages = append(modules[index].Packages, loadedPackage)
			}
		}
		sort.Slice(modules[index].Packages, func(i, j int) bool {
			return modules[index].Packages[i].PkgPath < modules[index].Packages[j].PkgPath
		})
	}
	sort.Strings(staleGeneratedFiles)
	return &Project{Root: root, ModulePath: modulePath, Modules: modules, StaleGeneratedFiles: staleGeneratedFiles}, nil
}

func validateModuleRoot(discovered DiscoveredModule) (string, error) {
	entries, err := os.ReadDir(discovered.Dir)
	if err != nil {
		return "", fmt.Errorf("模块 %s 读取根目录失败: %w", discovered.Key, err)
	}
	hasConfig := false
	stale := ""
	allowedFiles := map[string]struct{}{"config.go": {}, "db.json": {}, "menu.json": {}}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if _, ok := componentDirectories[name]; !ok {
				return "", fmt.Errorf("模块 %s 根目录包含未准入路径: %s", discovered.Key, name)
			}
			continue
		}
		if name == generatedFileName {
			path := filepath.Join(discovered.Dir, name)
			if !hasGeneratedHeader(path) {
				return "", fmt.Errorf("模块 %s 根目录包含无标准 Header 的 %s", discovered.Key, name)
			}
			stale = path
			continue
		}
		if _, ok := allowedFiles[name]; !ok {
			return "", fmt.Errorf("模块 %s 根目录包含未准入路径: %s", discovered.Key, name)
		}
		if name == "config.go" {
			hasConfig = true
		}
	}
	if !hasConfig {
		return "", fmt.Errorf("模块 %s 根目录缺少 config.go", discovered.Key)
	}
	return stale, nil
}

func readModulePath(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取 %s 失败: %w", path, err)
	}
	parsed, err := modfile.Parse(path, content, nil)
	if err != nil {
		return "", fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	if parsed.Module == nil || strings.TrimSpace(parsed.Module.Mod.Path) == "" {
		return "", fmt.Errorf("%s 缺少 module path", path)
	}
	return parsed.Module.Mod.Path, nil
}

func collectPackageErrors(loaded []*packages.Package) error {
	messages := make([]string, 0)
	for _, loadedPackage := range loaded {
		for _, packageErr := range loadedPackage.Errors {
			messages = append(messages, packageErr.Error())
		}
	}
	if len(messages) == 0 {
		return nil
	}
	sort.Strings(messages)
	return errors.New(strings.Join(messages, "; "))
}

func validatePortableComponents(moduleDir string) error {
	return filepath.WalkDir(moduleDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleDir && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || isGeneratedFile(path) {
			return nil
		}
		relative, err := filepath.Rel(moduleDir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) < 2 {
			return nil
		}
		if _, ok := componentDirectories[parts[0]]; !ok {
			return nil
		}
		if hasPlatformSuffix(entry.Name()) || hasPlatformBuildConstraint(path) {
			return fmt.Errorf("自动组件不能使用平台专属文件: %s", relative)
		}
		return nil
	})
}

func hasPlatformSuffix(name string) bool {
	base := strings.TrimSuffix(name, ".go")
	known := []string{"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "386", "amd64", "arm", "arm64", "loong64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}
	for _, suffix := range known {
		if strings.HasSuffix(base, "_"+suffix) {
			return true
		}
	}
	return false
}

func hasPlatformBuildConstraint(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "package ") {
			return false
		}
		if strings.HasPrefix(line, "//go:build") {
			return line != "//go:build !cool_generate"
		}
		if strings.HasPrefix(line, "// +build") {
			return line != "// +build !cool_generate"
		}
	}
	if err := scanner.Err(); err != nil {
		return false
	}
	return false
}

func isGeneratedFile(path string) bool {
	if filepath.Base(path) == generatedFileName || filepath.Base(path) == globalGeneratedFile {
		return true
	}
	return hasGeneratedHeader(path)
}

func hasGeneratedHeader(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for index := 0; index < 5 && scanner.Scan(); index++ {
		if strings.Contains(scanner.Text(), generatedHeader) {
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		return false
	}
	return false
}

func packageFunctions(moduleDir string, loadedPackage *packages.Package) []SourceFunction {
	functions := make([]SourceFunction, 0)
	for index, file := range loadedPackage.Syntax {
		path := loadedPackage.CompiledGoFiles[index]
		relative, err := filepath.Rel(moduleDir, path)
		if err != nil || strings.HasPrefix(relative, "..") || isGeneratedFile(path) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !function.Name.IsExported() {
				continue
			}
			functions = append(functions, SourceFunction{
				Package: loadedPackage,
				File:    file,
				Decl:    function,
				Path:    filepath.ToSlash(relative),
				Pos:     loadedPackage.Fset.Position(function.Pos()),
			})
		}
	}
	sort.Slice(functions, func(i, j int) bool {
		if functions[i].Package.PkgPath != functions[j].Package.PkgPath {
			return functions[i].Package.PkgPath < functions[j].Package.PkgPath
		}
		return functions[i].Decl.Name.Name < functions[j].Decl.Name.Name
	})
	return functions
}
