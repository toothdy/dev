package main

import (
	"bytes"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/artifact"
)

func TestInitializeProject(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "echo")
	if err := initializeProject(context.Background(), directory, "example.com/cool/echo"); err != nil {
		t.Fatalf("初始化插件失败: %v", err)
	}

	manifestData, err := os.ReadFile(filepath.Join(directory, artifact.ManifestFile))
	if err != nil {
		t.Fatalf("读取生成的 Manifest 失败: %v", err)
	}
	manifest, err := artifact.ParseManifest(manifestData)
	if err != nil {
		t.Fatalf("生成的 Manifest 无效: %v", err)
	}
	if manifest.Key != "echo-plugin" || string(manifest.Config["prefix"]) != `"echo: "` {
		t.Fatalf("生成的 Manifest 内容错误: %+v", manifest)
	}

	goModule, err := os.ReadFile(filepath.Join(directory, "go.mod"))
	if err != nil {
		t.Fatalf("读取生成的 go.mod 失败: %v", err)
	}
	if !bytes.Contains(goModule, []byte("module example.com/cool/echo")) || !bytes.Contains(goModule, []byte("replace "+sdkModule)) {
		t.Fatalf("开发版 go.mod 缺少 module 或 replace:\n%s", goModule)
	}
	for _, name := range []string{"plugin/plugin.go", "cmd/wasm/main.go"} {
		if _, err := parser.ParseFile(token.NewFileSet(), filepath.Join(directory, filepath.FromSlash(name)), nil, parser.AllErrors); err != nil {
			t.Fatalf("生成的 %s 语法无效: %v", name, err)
		}
	}
	if err := initializeProject(context.Background(), directory, "example.com/cool/echo"); err == nil {
		t.Fatal("非空目录被覆盖")
	}
}

func TestRenderPublishedGoModule(t *testing.T) {
	content := renderGoModule("example.com/plugin", sdkDependency{version: "v2.3.4"})
	if strings.Contains(content, "replace ") || !strings.Contains(content, sdkModule+" v2.3.4") {
		t.Fatalf("发布版依赖错误:\n%s", content)
	}
}

func TestProjectBuildAndPack(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "echo")
	if err := initializeProject(context.Background(), directory, "example.com/cool/echo"); err != nil {
		t.Fatalf("初始化插件失败: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "assets"), 0o755); err != nil {
		t.Fatalf("创建 assets 失败: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "assets", "data.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatalf("写入资源失败: %v", err)
	}

	var calls []string
	validated := false
	tool := &projectTool{
		execute: func(_ context.Context, _ string, stdout, _ io.Writer, environment []string, args ...string) error {
			calls = append(calls, args[0])
			switch args[0] {
			case "env":
				_, _ = io.WriteString(stdout, "go1.26.4\n")
			case "build":
				if !slices.Contains(environment, "GOOS=wasip1") || !slices.Contains(environment, "GOARCH=wasm") {
					return fmt.Errorf("缺少 WASI 构建环境: %v", environment)
				}
				output := argumentAfter(args, "-o")
				if output == "" {
					return fmt.Errorf("缺少构建输出参数")
				}
				return os.WriteFile(output, []byte("\x00asmtest"), 0o644)
			}
			return nil
		},
		validateWASM: func(context.Context, []byte) error {
			validated = true
			return nil
		},
	}

	firstPath, err := tool.pack(context.Background(), directory, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("打包插件失败: %v", err)
	}
	if !validated || !slices.Equal(calls, []string{"env", "test", "build"}) {
		t.Fatalf("命令顺序错误: validated=%v calls=%v", validated, calls)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("读取插件包失败: %v", err)
	}
	packageData, err := artifact.Read(first)
	if err != nil {
		t.Fatalf("生成的插件包无效: %v", err)
	}
	if string(packageData.Files["assets/data.txt"]) != "asset" {
		t.Fatal("assets 文件未进入插件包")
	}

	calls = nil
	secondPath, err := tool.pack(context.Background(), directory, io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("再次打包插件失败: %v", err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatalf("读取第二个插件包失败: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("相同项目两次打包结果不一致")
	}
}

func TestValidateGoVersionAndModule(t *testing.T) {
	for _, version := range []string{"go1.26.0", "go1.27rc1"} {
		if err := validateGoVersion(version); err != nil {
			t.Fatalf("版本 %s 应合法: %v", version, err)
		}
	}
	for _, version := range []string{"go1.25.9", "devel go1.27"} {
		if err := validateGoVersion(version); err == nil {
			t.Fatalf("版本 %s 应被拒绝", version)
		}
	}
	for _, module := range []string{"bad module", "example.com/../plugin", "example.com/plugin/"} {
		if err := validateModulePath(module); err == nil {
			t.Fatalf("module %q 应被拒绝", module)
		}
	}
}

func argumentAfter(arguments []string, name string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}

	return ""
}
