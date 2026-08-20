package codegen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestModuleLayoutGuard(t *testing.T) {
	root, err := FindProjectRoot("")
	if err != nil {
		t.Fatalf("定位项目根目录失败: %v", err)
	}
	for _, moduleDir := range moduleDirectories(t, root) {
		validateModuleLayout(t, moduleDir)
	}
}

func TestConfigGuard(t *testing.T) {
	root, err := FindProjectRoot("")
	if err != nil {
		t.Fatalf("定位项目根目录失败: %v", err)
	}
	modules := make(map[string]struct{})
	for _, moduleDir := range moduleDirectories(t, root) {
		key := filepath.Base(moduleDir)
		modules[key] = struct{}{}
		validateConfigDeclaration(t, moduleDir)
		validateModuleCodeConfigAccess(t, moduleDir)
	}
	for _, key := range []string{"base", "dict", "recycle", "task"} {
		if _, ok := modules[key]; !ok {
			t.Errorf("缺少必需模块 %s 及其 Config() 声明", key)
		}
	}
	validateManifestModuleTaskKey(t, filepath.Join(root, "manifest", "config", "config.yaml"))
}

func TestModuleDocumentationAndCLIGuard(t *testing.T) {
	root, err := FindProjectRoot("")
	if err != nil {
		t.Fatalf("定位项目根目录失败: %v", err)
	}
	paths := []string{
		filepath.Join(root, "README.md"),
		filepath.Join(root, "docs", "module-development.md"),
		filepath.Join(root, ".github", "workflows", "go.yml"),
		filepath.Join(root, "cmd", "cool", "main.go"),
	}
	forbidden := []string{
		"--module",
		"handler/**",
		"LoadConfig(context.Context)",
		"/module_gen.go",
		"`module_gen.go`",
	}
	for _, path := range paths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Errorf("读取协议文件 %s 失败: %v", path, readErr)
			continue
		}
		for _, fragment := range forbidden {
			if strings.Contains(string(content), fragment) {
				t.Errorf("协议文件 %s 仍包含旧约定 %q", path, fragment)
			}
		}
	}
}

// moduleDirectories 返回所有模块直接子目录。
func moduleDirectories(t *testing.T, root string) []string {
	t.Helper()
	modulesDir := filepath.Join(root, "modules")
	entries, err := os.ReadDir(modulesDir)
	if err != nil {
		t.Fatalf("读取 modules 目录失败: %v", err)
	}
	directories := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") || entry.Name() == "testdata" {
			continue
		}
		directories = append(directories, filepath.Join(modulesDir, entry.Name()))
	}
	return directories
}

// validateModuleLayout 验证模块根目录和历史聚合文件已符合新协议。
func validateModuleLayout(t *testing.T, moduleDir string) {
	t.Helper()
	allowedFiles := map[string]struct{}{
		"config.go": {},
		"db.json":   {},
		"menu.json": {},
	}
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		t.Errorf("读取模块目录 %s 失败: %v", moduleDir, err)
		return
	}
	hasConfig := false
	for _, entry := range entries {
		if entry.IsDir() {
			if _, ok := componentDirectories[entry.Name()]; !ok {
				t.Errorf("模块 %s 根目录包含未准入目录 %s", filepath.Base(moduleDir), entry.Name())
			}
			continue
		}
		if _, ok := allowedFiles[entry.Name()]; !ok {
			t.Errorf("模块 %s 根目录包含未准入文件 %s", filepath.Base(moduleDir), entry.Name())
		}
		if entry.Name() == "config.go" {
			hasConfig = true
		}
	}
	if !hasConfig {
		t.Errorf("模块 %s 根目录缺少 config.go", filepath.Base(moduleDir))
	}
	forbiddenFiles := map[string]struct{}{
		"module_gen.go":  {},
		"modules_gen.go": {},
		"providers.go":   {},
		"register.go":    {},
	}
	err = filepath.WalkDir(moduleDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, relativeErr := filepath.Rel(moduleDir, path)
		if relativeErr != nil {
			return relativeErr
		}
		if _, forbidden := forbiddenFiles[entry.Name()]; forbidden {
			t.Errorf("模块 %s 包含禁用文件 %s", filepath.Base(moduleDir), filepath.ToSlash(relative))
		}
		if filepath.ToSlash(relative) == "entity/models.go" {
			t.Errorf("模块 %s 包含禁用的实体汇总文件 %s", filepath.Base(moduleDir), filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Errorf("扫描模块 %s 失败: %v", filepath.Base(moduleDir), err)
	}
}

// validateConfigDeclaration 验证根 config.go 恰好声明一个 ModuleConfig。
func validateConfigDeclaration(t *testing.T, moduleDir string) {
	t.Helper()
	path := filepath.Join(moduleDir, "config.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Errorf("解析 %s 失败: %v", path, err)
		return
	}
	count := 0
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == "ModuleConfig" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("模块 %s 必须恰好声明一个 Config()，实际: %d", filepath.Base(moduleDir), count)
	}
}

// validateModuleCodeConfigAccess 禁止模块生产代码直接调用 g.Cfg。
func validateModuleCodeConfigAccess(t *testing.T, moduleDir string) {
	t.Helper()
	err := filepath.WalkDir(moduleDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != moduleDir && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") || hasGeneratedHeader(path) {
			return nil
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		aliases := make(map[string]struct{})
		hasDotImport := false
		for _, imported := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil || importPath != "github.com/gogf/gf/v2/frame/g" {
				continue
			}
			alias := "g"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias == "." {
				hasDotImport = true
				continue
			}
			aliases[alias] = struct{}{}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if identifier, ok := call.Fun.(*ast.Ident); ok && hasDotImport && identifier.Name == "Cfg" {
				t.Errorf("模块代码 %s 禁止直接调用 g.Cfg()", path)
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Cfg" {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			if _, ok = aliases[identifier.Name]; ok {
				t.Errorf("模块代码 %s 禁止直接调用 g.Cfg()", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Errorf("扫描模块配置访问 %s 失败: %v", filepath.Base(moduleDir), err)
	}
}

// validateManifestModuleTaskKey 验证 Task 配置只位于 module.task。
func validateManifestModuleTaskKey(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取配置 %s 失败: %v", path, err)
	}
	var document yaml.Node
	if err = yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("解析配置 %s 失败: %v", path, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		t.Fatalf("配置 %s 顶层必须是对象", path)
	}
	root := document.Content[0]
	if mappingValue(root, "task") != nil {
		t.Errorf("配置 %s 仍包含旧顶层 task 键", path)
	}
	moduleNode := mappingValue(root, "module")
	if moduleNode == nil || moduleNode.Kind != yaml.MappingNode || mappingValue(moduleNode, "task") == nil {
		t.Errorf("配置 %s 缺少 module.task 对象", path)
	}
}

// mappingValue 返回 YAML 映射中指定键的值节点。
func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}
