package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/tools/cover"
)

type goRunner interface {
	Run(directory string, args []string, stdout, stderr io.Writer) error
}

type execGoRunner struct{}

func (execGoRunner) Run(directory string, args []string, stdout, stderr io.Writer) error {
	command := exec.Command("go", args...)
	command.Dir = directory
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func executeCoverageCheck(
	cfg config,
	workDir string,
	stdout io.Writer,
	stderr io.Writer,
	runner goRunner,
) ([]string, error) {
	repoRoot, err := findRepoRoot(workDir)
	if err != nil {
		return nil, err
	}
	profile, err := os.CreateTemp(cfg.tempDir, "jftrade-go-coverage-*.out")
	if err != nil {
		return nil, fmt.Errorf("create coverage profile: %w", err)
	}
	profilePath := profile.Name()
	if err := profile.Close(); err != nil {
		_ = os.Remove(profilePath)
		return nil, fmt.Errorf("close coverage profile: %w", err)
	}
	defer os.Remove(profilePath)

	args := []string{
		"test",
		"-count=1",
		"-timeout", cfg.testTimeout.String(),
		"-coverprofile", profilePath,
	}
	args = append(args, cfg.packages...)
	if err := runner.Run(repoRoot, args, stdout, stderr); err != nil {
		return nil, fmt.Errorf("go test failed: %w", err)
	}

	profiles, err := cover.ParseProfiles(profilePath)
	if err != nil {
		return nil, fmt.Errorf("parse coverage profile: %w", err)
	}
	analysis, err := analyzeProfiles(profiles)
	if err != nil {
		return nil, err
	}
	printCoverageReport(stdout, analysis, cfg)
	return evaluateCoverage(analysis, cfg), nil
}

func findRepoRoot(start string) (string, error) {
	directory, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	for {
		info, statErr := os.Stat(filepath.Join(directory, "go.mod"))
		if statErr == nil && !info.IsDir() {
			return directory, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", fmt.Errorf("inspect go.mod: %w", statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", fmt.Errorf("find repository root from %s: go.mod not found", start)
		}
		directory = parent
	}
}
