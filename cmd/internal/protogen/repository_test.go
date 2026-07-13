package protogen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindRepoRoot(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n"), 0o600))
	nested := filepath.Join(repoRoot, "one", "two")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	actual, err := FindRepoRoot(nested)
	require.NoError(t, err)
	assert.Equal(t, repoRoot, actual)
}

func TestFindRepoRootRejectsMissingModule(t *testing.T) {
	_, err := FindRepoRoot(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "go.mod not found")
}
