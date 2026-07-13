package protogen

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

// Command describes one external command invocation.
type Command struct {
	Name   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Runner executes external commands for a generator.
type Runner interface {
	Run(Command) error
}

// RunnerFunc adapts a function into a Runner.
type RunnerFunc func(Command) error

// Run executes the adapted function.
func (run RunnerFunc) Run(command Command) error {
	return run(command)
}

// ExecRunner executes commands with os/exec.
type ExecRunner struct{}

// Run executes command without involving a command shell.
func (ExecRunner) Run(command Command) error {
	process := exec.Command(command.Name, command.Args...)
	process.Dir = command.Dir
	process.Env = command.Env
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	return process.Run()
}

// ExitCode returns a child process exit code when one is available.
func ExitCode(err error) int {
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) && exitErr.ExitCode() > 0 {
		return exitErr.ExitCode()
	}
	return 1
}

// InheritedEnvironment returns a copy of the current process environment.
func InheritedEnvironment() []string {
	return append([]string(nil), os.Environ()...)
}
