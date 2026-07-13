package main

import (
	"bytes"
	"flag"
	"io"
	"testing"

	"github.com/jftrade/jftrade-main/cmd/internal/protogen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCLIConfig(t *testing.T) {
	var stderr bytes.Buffer
	cfg, err := parseCLIConfig(nil, &stderr)
	require.NoError(t, err)
	assert.Equal(t, defaultMaxGeneratedLines, cfg.maxGeneratedLines)

	cfg, err = parseCLIConfig([]string{"-max-generated-lines=42"}, &stderr)
	require.NoError(t, err)
	assert.Equal(t, 42, cfg.maxGeneratedLines)
}

func TestParseCLIConfigRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"-max-generated-lines=0"}, {"-max-generated-lines=-1"}, {"positional"}} {
		var stderr bytes.Buffer
		_, err := parseCLIConfig(args, &stderr)
		require.Error(t, err)
	}
}

func TestParseCLIConfigHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseCLIConfig([]string{"-help"}, &stderr)
	assert.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, stderr.String(), "-max-generated-lines")
}

func TestRunCLIReturnsParameterExitCode(t *testing.T) {
	exitCode := runCLI(
		[]string{"-max-generated-lines=0"},
		io.Discard,
		io.Discard,
		protogen.RunnerFunc(nil),
	)
	assert.Equal(t, 2, exitCode)
}
