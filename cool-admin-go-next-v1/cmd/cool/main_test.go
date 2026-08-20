package main

import "testing"

func TestRunRejectsUnknownCommand(t *testing.T) {
	if code := run([]string{"unknown"}); code != 2 {
		t.Fatalf("未知命令退出码不符: %d", code)
	}
}

func TestRunRejectsModuleFlagForEveryCommand(t *testing.T) {
	for _, command := range []string{"generate", "check", "build", "run"} {
		t.Run(command, func(t *testing.T) {
			if code := run([]string{command, "--module", "dict"}); code != 2 {
				t.Fatalf("%s --module 退出码不符: %d", command, code)
			}
		})
	}
}
