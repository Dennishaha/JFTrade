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
	cfg, err := parseCLIConfig(nil, `C:\Users\test\Downloads\FTAPIProtoFiles_10.8.6808`, &stderr)
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\test\Downloads\FTAPIProtoFiles_10.8.6808`, cfg.source)

	cfg, err = parseCLIConfig([]string{"-source", "/tmp/futu"}, "unused", &stderr)
	require.NoError(t, err)
	assert.Equal(t, "/tmp/futu", cfg.source)
}

func TestParseCLIConfigRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"-source="}, {"positional"}} {
		var stderr bytes.Buffer
		_, err := parseCLIConfig(args, "default", &stderr)
		require.Error(t, err)
	}
}

func TestParseCLIConfigHelp(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseCLIConfig([]string{"-help"}, "default", &stderr)
	assert.ErrorIs(t, err, flag.ErrHelp)
	assert.Contains(t, stderr.String(), "-source")
}

func TestRunCLIReturnsParameterExitCode(t *testing.T) {
	exitCode := runCLI([]string{"-source="}, io.Discard, io.Discard, protogen.RunnerFunc(nil))
	assert.Equal(t, 2, exitCode)
}
