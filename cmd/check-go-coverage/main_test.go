package main

import (
	"bytes"
	"flag"
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
