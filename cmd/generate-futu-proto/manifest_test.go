package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseChecksumManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.sha256")
	digest := strings.Repeat("a", 64)
	require.NoError(t, os.WriteFile(path, []byte("# comment\n\n"+digest+"  Common.proto\n"), 0o600))

	actual, err := parseChecksumManifest(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Common.proto": digest}, actual)
}

func TestParseChecksumManifestRejectsInvalidEntries(t *testing.T) {
	digest := strings.Repeat("a", 64)
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"invalid digest", "ABC Common.proto\n", "invalid checksum manifest entry"},
		{"path filename", digest + " nested/Common.proto\n", "invalid checksum manifest filename"},
		{"windows path filename", digest + " nested\\Common.proto\n", "invalid checksum manifest filename"},
		{"duplicate filename", digest + " Common.proto\n" + digest + " Common.proto\n", "invalid checksum manifest filename"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.sha256")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			_, err := parseChecksumManifest(path)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestValidateManifestFilesReportsAllDifferences(t *testing.T) {
	err := validateManifestFiles(
		map[string]string{"Common.proto": "digest", "Unexpected.proto": "digest"},
		[]string{"Common.proto", "Missing.proto"},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing: Missing.proto")
	assert.Contains(t, err.Error(), "unexpected: Unexpected.proto")
}

func TestVerifyFutuInputsRejectsMissingAndMismatchedFiles(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "source")
	require.NoError(t, os.Mkdir(source, 0o755))
	manifest := filepath.Join(directory, "manifest.sha256")
	digest := sha256.Sum256([]byte("expected"))
	require.NoError(t, os.WriteFile(manifest, []byte(fmt.Sprintf("%x  Common.proto\n", digest)), 0o600))

	err := verifyFutuInputs(source, manifest, []string{"Common.proto"})
	require.ErrorContains(t, err, "missing proto file")
	require.NoError(t, os.WriteFile(filepath.Join(source, "Common.proto"), []byte("actual"), 0o600))
	err = verifyFutuInputs(source, manifest, []string{"Common.proto"})
	require.ErrorContains(t, err, "futu proto checksum mismatch")
}
