package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGoRunner struct {
	args        []string
	directory   string
	profilePath string
	err         error
}

func (runner *fakeGoRunner) Run(directory string, args []string, stdout, stderr io.Writer) error {
	runner.directory = directory
	runner.args = append([]string(nil), args...)
	for index := range args {
		if args[index] == "-coverprofile" && index+1 < len(args) {
			runner.profilePath = args[index+1]
			break
		}
	}
	if runner.err != nil {
		return runner.err
	}
	_, _ = io.WriteString(stdout, "fake go test output\n")
	return os.WriteFile(runner.profilePath, []byte(`mode: set
github.com/jftrade/jftrade-main/internal/example/example.go:1.1,2.1 1 1
`), 0o600)
}

func TestExecuteCoverageCheckBuildsCommandAndCleansProfile(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o600))
	subdirectory := filepath.Join(repoRoot, "nested", "directory")
	require.NoError(t, os.MkdirAll(subdirectory, 0o755))
	tempDir := t.TempDir()
	runner := &fakeGoRunner{}
	var stdout bytes.Buffer

	violations, err := executeCoverageCheck(config{
		businessThreshold: 0,
		criticalThreshold: 0,
		moduleThreshold:   0,
		testTimeout:       2 * time.Minute,
		packages:          packageList{"./internal/...", "./pkg/..."},
		tempDir:           tempDir,
	}, subdirectory, &stdout, io.Discard, runner)
	require.NoError(t, err)
	assert.Equal(t, repoRoot, runner.directory)
	assert.Equal(t, []string{
		"test", "-count=1", "-timeout", "2m0s", "-coverprofile", runner.profilePath,
		"./internal/...", "./pkg/...",
	}, runner.args)
	assert.Contains(t, stdout.String(), "fake go test output")
	assert.NotEmpty(t, violations, "missing critical packages must be reported")
	_, statErr := os.Stat(runner.profilePath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestExecuteCoverageCheckReturnsGoTestErrorAndCleansProfile(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o600))
	runner := &fakeGoRunner{err: errors.New("test failure")}

	_, err := executeCoverageCheck(config{
		testTimeout: time.Minute,
		packages:    packageList{"./..."},
		tempDir:     t.TempDir(),
	}, repoRoot, io.Discard, io.Discard, runner)
	require.EqualError(t, err, "go test failed: test failure")
	_, statErr := os.Stat(runner.profilePath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestFindRepoRootRejectsDirectoryWithoutGoMod(t *testing.T) {
	_, err := findRepoRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod not found")
}
