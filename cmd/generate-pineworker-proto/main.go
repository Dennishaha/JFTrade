package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
)

const defaultMaxGeneratedLines = 1200

type cliConfig struct {
	maxGeneratedLines int
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr, protogen.ExecRunner{}))
}

func runCLI(args []string, stdout, stderr io.Writer, runner protogen.Runner) int {
	cfg, err := parseCLIConfig(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		return 2
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "get working directory: %v\n", err)
		return 1
	}
	repoRoot, err := protogen.FindRepoRoot(workingDirectory)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	err = generatePineworkerProto(generatorConfig{
		repoRoot: repoRoot, maxGeneratedLines: cfg.maxGeneratedLines,
		runner: runner, stdout: stdout, stderr: stderr,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return protogen.ExitCode(err)
	}
	return 0
}

func parseCLIConfig(args []string, stderr io.Writer) (cliConfig, error) {
	cfg := cliConfig{}
	flags := flag.NewFlagSet("generate-pineworker-proto", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.IntVar(
		&cfg.maxGeneratedLines,
		"max-generated-lines",
		defaultMaxGeneratedLines,
		"maximum newline count allowed in each generated Go file",
	)
	if err := flags.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if flags.NArg() != 0 {
		err := fmt.Errorf("unexpected positional arguments: %v", flags.Args())
		_, _ = fmt.Fprintln(stderr, err)
		return cliConfig{}, err
	}
	if cfg.maxGeneratedLines <= 0 {
		err := errors.New("-max-generated-lines must be greater than zero")
		_, _ = fmt.Fprintln(stderr, err)
		return cliConfig{}, err
	}
	return cfg, nil
}
