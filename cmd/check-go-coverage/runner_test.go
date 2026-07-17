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
	profile     string
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
	profile := runner.profile
	if profile == "" {
		profile = `mode: set
github.com/jftrade/jftrade-main/internal/example/example.go:1.1,2.1 1 1
`
	}
	return os.WriteFile(runner.profilePath, []byte(profile), 0o600)
}

func TestExecGoRunnerRunsGoCommandInRequestedDirectory(t *testing.T) {
	var stdout bytes.Buffer
	err := (execGoRunner{}).Run(t.TempDir(), []string{"version"}, &stdout, io.Discard)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "go version")
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
		"test", "-count=1", "-timeout", "2m0s", "-coverpkg=./...", "-coverprofile", runner.profilePath,
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

func TestExecuteCoverageCheckReportsProfileAndOutputFailures(t *testing.T) {
	repoRoot := coverageTestRepoRoot(t)
	baseConfig := config{
		businessThreshold: 0,
		criticalThreshold: 0,
		moduleThreshold:   0,
		testTimeout:       time.Minute,
		packages:          packageList{"./..."},
		tempDir:           t.TempDir(),
	}

	t.Run("invalid profile", func(t *testing.T) {
		runner := &fakeGoRunner{profile: "not a coverage profile"}
		_, err := executeCoverageCheck(baseConfig, repoRoot, io.Discard, io.Discard, runner)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "merge coverage profile")
	})

	t.Run("no business statements", func(t *testing.T) {
		runner := &fakeGoRunner{profile: `mode: set
github.com/jftrade/jftrade-main/cmd/generate-futu-proto/main.go:1.1,2.1 1 1
`}
		_, err := executeCoverageCheck(baseConfig, repoRoot, io.Discard, io.Discard, runner)
		require.EqualError(t, err, "coverage profile contains no business statements")
	})

	t.Run("report writer", func(t *testing.T) {
		runner := &fakeGoRunner{}
		_, err := executeCoverageCheck(baseConfig, repoRoot, failingWriter{}, io.Discard, runner)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "write coverage summary")
	})
}

func TestExecuteCoverageCheckReportsTemporaryProfileCreationFailure(t *testing.T) {
	missingTempDir := filepath.Join(t.TempDir(), "missing")
	_, err := executeCoverageCheck(config{
		testTimeout: time.Minute,
		packages:    packageList{"./..."},
		tempDir:     missingTempDir,
	}, coverageTestRepoRoot(t), io.Discard, io.Discard, &fakeGoRunner{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create coverage profile")
}

func TestFindRepoRootRejectsDirectoryWithoutGoMod(t *testing.T) {
	_, err := findRepoRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod not found")
}

func coverageTestRepoRoot(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o600))
	return repoRoot
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("output unavailable")
}
