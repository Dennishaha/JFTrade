package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/cover"
)

func TestMergeCoverageProfileKeepsOneBlockWithAnyObservedExecution(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(profilePath, []byte(`mode: set
github.com/jftrade/jftrade-main/pkg/example/example.go:1.1,2.1 2 0
github.com/jftrade/jftrade-main/pkg/example/example.go:1.1,2.1 2 3
github.com/jftrade/jftrade-main/pkg/example/example.go:3.1,4.1 1 0
`), 0o600))

	require.NoError(t, mergeCoverageProfile(profilePath))
	profiles, err := cover.ParseProfiles(profilePath)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].Blocks, 2)
	assert.Equal(t, 3, profiles[0].Blocks[0].Count)
	assert.Equal(t, 0, profiles[0].Blocks[1].Count)
}

func TestMergeCoverageProfileRejectsMalformedInput(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "coverage.out")
	require.NoError(t, os.WriteFile(profilePath, []byte("mode: set\ninvalid\n"), 0o600))
	assert.Error(t, mergeCoverageProfile(profilePath))
}
