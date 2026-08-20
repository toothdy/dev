package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/toothdy/cool-admin-go-next/cool/util/codegen"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(arguments []string) int {
	if len(arguments) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cool <generate|check|build|run>")
		return 2
	}
	command := arguments[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	if err := flags.Parse(arguments[1:]); err != nil {
		return 2
	}
	switch command {
	case "generate":
		if err := codegen.Execute(codegen.GenerateOptions{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "check":
		if err := codegen.Execute(codegen.GenerateOptions{IsCheck: true}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	case "build", "run":
		if err := codegen.Execute(codegen.GenerateOptions{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		root, err := codegen.FindProjectRoot("")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		goCommand := "build"
		childArguments := []string{goCommand}
		if command == "run" {
			goCommand = "run"
			childArguments = append([]string{goCommand, "."}, flags.Args()...)
		} else {
			childArguments = append([]string{goCommand}, flags.Args()...)
			if len(flags.Args()) == 0 {
				childArguments = append(childArguments, ".")
			}
		}
		return runGo(root, childArguments)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", command)
		return 2
	}
}

func runGo(root string, arguments []string) int {
	command := exec.Command("go", arguments...)
	command.Dir = root
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	forwarded := make(chan os.Signal, 1)
	signal.Notify(forwarded, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(forwarded)
	if err := command.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	done := make(chan struct{})
	go func() {
		select {
		case received := <-forwarded:
			if command.Process != nil {
				_ = command.Process.Signal(received)
			}
		case <-done:
		}
	}()
	err := command.Wait()
	close(done)
	if err == nil {
		return 0
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}
