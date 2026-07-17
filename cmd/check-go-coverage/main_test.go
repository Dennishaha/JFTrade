package main

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigDefaults(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig(nil, &stderr)
	require.NoError(t, err)
	assert.Equal(t, defaultBusinessThreshold, cfg.businessThreshold)
	assert.Equal(t, defaultCriticalThreshold, cfg.criticalThreshold)
	assert.Equal(t, defaultModuleThreshold, cfg.moduleThreshold)
	assert.Equal(t, defaultTestTimeout, cfg.testTimeout)
	assert.Equal(t, packageList{"./..."}, cfg.packages)
	assert.Empty(t, stderr.String())
}

func TestParseConfigExplicitValues(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseConfig([]string{
		"-business-threshold=91.5",
		"-critical-threshold=96",
		"-module-threshold=86.25",
		"-timeout=2m30s",
		"-package=./internal/...",
		"-package=./pkg/...",
	}, &stderr)
	require.NoError(t, err)
	assert.Equal(t, 91.5, cfg.businessThreshold)
	assert.Equal(t, 96.0, cfg.criticalThreshold)
	assert.Equal(t, 86.25, cfg.moduleThreshold)
	assert.Equal(t, 150*time.Second, cfg.testTimeout)
	assert.Equal(t, packageList{"./internal/...", "./pkg/..."}, cfg.packages)
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"negative threshold", []string{"-business-threshold=-1"}, "must be between 0 and 100"},
		{"not a number threshold", []string{"-module-threshold=NaN"}, "must be between 0 and 100"},
		{"large threshold", []string{"-critical-threshold=101"}, "must be between 0 and 100"},
		{"zero timeout", []string{"-timeout=0s"}, "must be greater than zero"},
		{"empty package", []string{"-package="}, "package pattern must not be empty"},
		{"blank package", []string{"-package=  "}, "package pattern must not be empty"},
		{"positional package", []string{"./internal/..."}, "unexpected positional arguments"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			_, err := parseConfig(test.args, &stderr)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.want)
		})
	}
}

func TestParseConfigHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseConfig([]string{"-help"}, &stderr)
	assert.ErrorIs(t, err, flag.ErrHelp)
	assert.True(t, strings.Contains(stderr.String(), "-package"))
}

func TestRunCLIUsesDefaultDependenciesForHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	assert.Equal(t, 0, runCLI([]string{"-help"}, &stdout, &stderr))
	assert.Contains(t, stderr.String(), "-package")
}

func TestRunCLIWithStopsBeforeExecutionForHelpAndInvalidConfiguration(t *testing.T) {
	neverGetwd := func() (string, error) {
		t.Fatal("getwd should not be called")
		return "", nil
	}
	neverExecute := func(config, string, io.Writer, io.Writer, goRunner) ([]string, error) {
		t.Fatal("execute should not be called")
		return nil, nil
	}

	var stderr bytes.Buffer
	assert.Equal(t, 0, runCLIWith([]string{"-help"}, io.Discard, &stderr, neverGetwd, neverExecute, nil))
	assert.Equal(t, 2, runCLIWith([]string{"-unknown-flag"}, io.Discard, io.Discard, neverGetwd, neverExecute, nil))
}

func TestRunCLIWithReportsWorkingDirectoryAndExecutionFailures(t *testing.T) {
	t.Run("working directory", func(t *testing.T) {
		var stderr bytes.Buffer
		status := runCLIWith(
			nil,
			io.Discard,
			&stderr,
			func() (string, error) { return "", errors.New("working directory unavailable") },
			func(config, string, io.Writer, io.Writer, goRunner) ([]string, error) {
				t.Fatal("execute should not be called")
				return nil, nil
			},
			nil,
		)
		assert.Equal(t, 1, status)
		assert.Contains(t, stderr.String(), "get working directory: working directory unavailable")
	})

	t.Run("coverage execution", func(t *testing.T) {
		var stderr bytes.Buffer
		status := runCLIWith(
			nil,
			io.Discard,
			&stderr,
			func() (string, error) { return "/workspace", nil },
			func(config, string, io.Writer, io.Writer, goRunner) ([]string, error) {
				return nil, errors.New("coverage command failed")
			},
			nil,
		)
		assert.Equal(t, 1, status)
		assert.Contains(t, stderr.String(), "coverage command failed")
	})
}

func TestRunCLIWithReportsViolationsAndSuccess(t *testing.T) {
	t.Run("violations", func(t *testing.T) {
		var stderr bytes.Buffer
		status := runCLIWith(
			nil,
			io.Discard,
			&stderr,
			func() (string, error) { return "/workspace", nil },
			func(config, string, io.Writer, io.Writer, goRunner) ([]string, error) {
				return []string{"internal/example is below 95%", "pkg/example has no coverage data"}, nil
			},
			nil,
		)
		assert.Equal(t, 1, status)
		assert.Contains(t, stderr.String(), "Coverage gate failed:")
		assert.Contains(t, stderr.String(), "- internal/example is below 95%")
		assert.Contains(t, stderr.String(), "- pkg/example has no coverage data")
	})

	t.Run("success", func(t *testing.T) {
		status := runCLIWith(
			nil,
			io.Discard,
			io.Discard,
			func() (string, error) { return "/workspace", nil },
			func(cfg config, workDir string, _ io.Writer, _ io.Writer, runner goRunner) ([]string, error) {
				assert.Equal(t, "/workspace", workDir)
				assert.Equal(t, defaultBusinessThreshold, cfg.businessThreshold)
				assert.Nil(t, runner)
				return nil, nil
			},
			nil,
		)
		assert.Equal(t, 0, status)
	})
}

func TestMainDelegatesToCLIAndProcessExit(t *testing.T) {
	originalArgs := os.Args
	originalCLI := runCoverageCLI
	originalExit := exitProcess
	t.Cleanup(func() {
		os.Args = originalArgs
		runCoverageCLI = originalCLI
		exitProcess = originalExit
	})

	os.Args = []string{"check-go-coverage", "-package=./internal/..."}
	called := false
	runCoverageCLI = func(args []string, _ io.Writer, _ io.Writer) int {
		called = true
		assert.Equal(t, []string{"-package=./internal/..."}, args)
		return 17
	}
	var exitCode int
	exitProcess = func(code int) { exitCode = code }

	main()
	assert.True(t, called)
	assert.Equal(t, 17, exitCode)
}
