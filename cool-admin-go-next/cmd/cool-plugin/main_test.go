package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/toothdy/cool-admin-go-next/cool-next/plugin/artifact"
)

func TestRunHelpAndUsage(t *testing.T) {
	commands := stubProjectCommands()
	tests := []struct {
		name     string
		args     []string
		wantCode int
		contains string
	}{
		{name: "帮助", args: []string{"--help"}, wantCode: exitSuccess, contains: "cool-plugin init"},
		{name: "缺少命令", wantCode: exitUsage, contains: "Cool Go 插件开发工具"},
		{name: "未知命令", args: []string{"unknown"}, wantCode: exitUsage, contains: "未知命令"},
		{name: "目录参数过多", args: []string{"check", "one", "two"}, wantCode: exitUsage, contains: "最多接受一个目录"},
		{name: "init 缺少 module", args: []string{"init"}, wantCode: exitUsage, contains: "--module"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := runCommands(context.Background(), test.args, "/workspace", &stdout, &stderr, commands)
			if code != test.wantCode {
				t.Fatalf("退出码 = %d, want %d", code, test.wantCode)
			}
			if !strings.Contains(stdout.String()+stderr.String(), test.contains) {
				t.Fatalf("输出 %q 不包含 %q", stdout.String()+stderr.String(), test.contains)
			}
		})
	}
}

func TestRunCommands(t *testing.T) {
	commands := stubProjectCommands()
	var initializedDirectory, initializedModule string
	commands.initialize = func(_ context.Context, directory, module string) error {
		initializedDirectory = directory
		initializedModule = module
		return nil
	}

	var stdout, stderr bytes.Buffer
	code := runCommands(
		context.Background(),
		[]string{"init", "--module", "example.com/plugin", "example"},
		"/workspace",
		&stdout,
		&stderr,
		commands,
	)
	if code != exitSuccess || initializedDirectory != "/workspace/example" || initializedModule != "example.com/plugin" {
		t.Fatalf("init 转发错误: code=%d directory=%q module=%q", code, initializedDirectory, initializedModule)
	}

	stdout.Reset()
	stderr.Reset()
	code = runCommands(context.Background(), []string{"check"}, "/workspace", &stdout, &stderr, commands)
	if code != exitSuccess || !strings.Contains(stdout.String(), "echo-plugin v1.0.0") {
		t.Fatalf("check 输出错误: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	commands.test = func(context.Context, string, io.Writer, io.Writer) error { return errors.New("测试失败") }
	stdout.Reset()
	stderr.Reset()
	code = runCommands(context.Background(), []string{"test"}, "/workspace", &stdout, &stderr, commands)
	if code != exitFailure || !strings.Contains(stderr.String(), "测试失败") {
		t.Fatalf("失败转发错误: code=%d stderr=%q", code, stderr.String())
	}
}

func stubProjectCommands() projectCommands {
	return projectCommands{
		initialize: func(context.Context, string, string) error { return nil },
		check: func(context.Context, string, io.Writer, io.Writer) (artifactProject, error) {
			return artifactProject{manifest: artifact.Manifest{Key: "echo-plugin", Version: "1.0.0"}}, nil
		},
		test:  func(context.Context, string, io.Writer, io.Writer) error { return nil },
		build: func(context.Context, string, io.Writer, io.Writer) error { return nil },
		pack: func(context.Context, string, io.Writer, io.Writer) (string, error) {
			return "/workspace/echo-plugin_v1.0.0.cool", nil
		},
	}
}
