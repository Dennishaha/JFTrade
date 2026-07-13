package protogen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFileCopiesContent(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source.txt")
	target := filepath.Join(directory, "nested", "target.txt")
	require.NoError(t, os.WriteFile(source, []byte("content"), 0o640))
	require.NoError(t, CopyFile(source, target))
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "content", string(content))
}

func TestReplaceDirectoriesReplacesAllTargets(t *testing.T) {
	directory := t.TempDir()
	sourceOne := makeDirectoryWithFile(t, directory, "source-one", "new-one")
	sourceTwo := makeDirectoryWithFile(t, directory, "source-two", "new-two")
	targetOne := makeDirectoryWithFile(t, directory, "target-one", "old-one")
	targetTwo := makeDirectoryWithFile(t, directory, "target-two", "old-two")

	require.NoError(t, ReplaceDirectories([]Replacement{
		{Source: sourceOne, Target: targetOne},
		{Source: sourceTwo, Target: targetTwo},
	}))
	assert.Equal(t, "new-one", readMarker(t, targetOne))
	assert.Equal(t, "new-two", readMarker(t, targetTwo))
}

func TestReplaceDirectoriesRollsBackAllTargets(t *testing.T) {
	directory := t.TempDir()
	sourceOne := makeDirectoryWithFile(t, directory, "source-one", "new-one")
	sourceTwo := makeDirectoryWithFile(t, sourceOne, "nested-source", "new-two")
	targetOne := makeDirectoryWithFile(t, directory, "target-one", "old-one")
	targetTwo := makeDirectoryWithFile(t, directory, "target-two", "old-two")

	err := ReplaceDirectories([]Replacement{
		{Source: sourceOne, Target: targetOne},
		{Source: sourceTwo, Target: targetTwo},
	})
	require.Error(t, err)
	assert.Equal(t, "old-one", readMarker(t, targetOne))
	assert.Equal(t, "old-two", readMarker(t, targetTwo))
}

func TestExitCodeUsesWrappedExitCoder(t *testing.T) {
	err := errors.Join(errors.New("context"), fakeExitError{code: 42})
	assert.Equal(t, 42, ExitCode(err))
	assert.Equal(t, 1, ExitCode(errors.New("ordinary")))
}

type fakeExitError struct {
	code int
}

func (err fakeExitError) Error() string {
	return "exit failure"
}

func (err fakeExitError) ExitCode() int {
	return err.code
}

func makeDirectoryWithFile(t *testing.T, parent, name, content string) string {
	t.Helper()
	directory := filepath.Join(parent, name)
	require.NoError(t, os.MkdirAll(directory, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "marker"), []byte(content), 0o600))
	return directory
}

func readMarker(t *testing.T, directory string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(directory, "marker"))
	require.NoError(t, err)
	return string(content)
}
