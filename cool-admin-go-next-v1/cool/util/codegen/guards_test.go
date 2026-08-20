package codegen

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestMainApplicationDependenciesExcludeCodegenToolchain(t *testing.T) {
	root, err := FindProjectRoot("")
	if err != nil {
		t.Fatalf("locate project root failed: %v", err)
	}
	command := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", ".")
	command.Dir = root
	command.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("list main application dependencies failed: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "golang.org/x/tools") {
			t.Fatalf("main application depends on codegen toolchain: %s", scanner.Text())
		}
	}
	if err = scanner.Err(); err != nil {
		t.Fatalf("read main application dependencies failed: %v", err)
	}
}

func TestGeneratedWiringExcludesReflectionAndRuntimeDI(t *testing.T) {
	root, err := FindProjectRoot("")
	if err != nil {
		t.Fatalf("locate project root failed: %v", err)
	}
	expected := filepath.Join(root, "modules", globalGeneratedFile)
	forbiddenImports := map[string]struct{}{
		"reflect":                   {},
		"github.com/google/wire":    {},
		"go.uber.org/dig":           {},
		"go.uber.org/fx":            {},
		"github.com/sarulabs/di/v2": {},
	}
	generatedPaths := make([]string, 0, 1)
	err = filepath.WalkDir(filepath.Join(root, "modules"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || !hasGeneratedHeader(path) {
			return nil
		}
		generatedPaths = append(generatedPaths, path)
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Errorf("parse generated file %s failed: %v", path, parseErr)
			return nil
		}
		for _, imported := range parsed.Imports {
			importPath, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Errorf("parse generated import %s failed: %v", imported.Path.Value, unquoteErr)
				continue
			}
			if imported.Name != nil && imported.Name.Name == "_" {
				t.Errorf("generated file %s contains blank import %s", path, importPath)
			}
			if _, forbidden := forbiddenImports[importPath]; forbidden {
				t.Errorf("generated file %s imports forbidden runtime dependency %s", path, importPath)
			}
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == "init" {
				t.Errorf("generated file %s contains init registration", path)
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == "Module" {
				t.Errorf("generated file %s contains forbidden .Module() call", path)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan generated files failed: %v", err)
	}
	sort.Strings(generatedPaths)
	if len(generatedPaths) != 1 || filepath.Clean(generatedPaths[0]) != filepath.Clean(expected) {
		t.Fatalf("the standard generated header must appear only in %s, got: %v", expected, generatedPaths)
	}
}
