package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenGeneratedFiles(t *testing.T) {
	root := t.TempDir()
	raw := filepath.Join(root, "raw")
	next := filepath.Join(root, "next")
	require.NoError(t, os.MkdirAll(filepath.Join(raw, "proto"), 0o755))
	require.NoError(t, os.Mkdir(next, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(raw, "proto", "pineworker.pb.go"), []byte("content"), 0o600))

	require.NoError(t, flattenGeneratedFiles(raw, next))
	content, err := os.ReadFile(filepath.Join(next, "pineworker.pb.go"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestFlattenGeneratedFilesRejectsNameCollisions(t *testing.T) {
	root := t.TempDir()
	raw := filepath.Join(root, "raw")
	next := filepath.Join(root, "next")
	for _, directory := range []string{filepath.Join(raw, "one"), filepath.Join(raw, "two"), next} {
		require.NoError(t, os.MkdirAll(directory, 0o755))
	}
	for _, directory := range []string{"one", "two"} {
		require.NoError(t, os.WriteFile(filepath.Join(raw, directory, "same.go"), []byte(directory), 0o600))
	}

	err := flattenGeneratedFiles(raw, next)
	require.ErrorContains(t, err, "generated filename collision")
}

func TestEnforceGeneratedLineLimitMatchesWcSemantics(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "generated.go")
	require.NoError(t, os.WriteFile(path, []byte("first\nsecond\nthird without newline"), 0o600))
	require.NoError(t, enforceGeneratedLineLimit(directory, 2))
	err := enforceGeneratedLineLimit(directory, 1)
	require.ErrorContains(t, err, "exceeds 1 lines")
}
