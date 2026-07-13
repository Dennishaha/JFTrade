package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
)

const defaultFutuSourceDirectory = "FTAPIProtoFiles_10.5.6508"

type cliConfig struct {
	source string
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr, protogen.ExecRunner{}))
}

func runCLI(args []string, stdout, stderr io.Writer, runner protogen.Runner) int {
	home, err := os.UserHomeDir()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "resolve user home directory: %v\n", err)
		return 1
	}
	cfg, err := parseCLIConfig(args, filepath.Join(home, "Downloads", defaultFutuSourceDirectory), stderr)
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
	err = generateFutuProto(generatorConfig{
		repoRoot: repoRoot, sourceDirectory: cfg.source,
		runner: runner, stdout: stdout, stderr: stderr,
	})
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return protogen.ExitCode(err)
	}
	return 0
}

func parseCLIConfig(args []string, defaultSource string, stderr io.Writer) (cliConfig, error) {
	cfg := cliConfig{}
	flags := flag.NewFlagSet("generate-futu-proto", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&cfg.source, "source", defaultSource, "directory containing Futu OpenAPI 10.5.6508 proto files")
	if err := flags.Parse(args); err != nil {
		return cliConfig{}, err
	}
	if flags.NArg() != 0 {
		err := fmt.Errorf("unexpected positional arguments: %v", flags.Args())
		_, _ = fmt.Fprintln(stderr, err)
		return cliConfig{}, err
	}
	if cfg.source == "" {
		err := errors.New("-source must not be empty")
		_, _ = fmt.Fprintln(stderr, err)
		return cliConfig{}, err
	}
	return cfg, nil
}
