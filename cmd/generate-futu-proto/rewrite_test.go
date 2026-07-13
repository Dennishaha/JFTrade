package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRewriteGoPackageReplacesOrInsertsOption(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"replace", "syntax = \"proto3\";\r\npackage Qot_Common;\r\noption go_package = \"old\";\r\n"},
		{"insert", "syntax = \"proto3\";\npackage Qot_Common;\nmessage Item {}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Qot_Common.proto")
			require.NoError(t, os.WriteFile(path, []byte(test.content), 0o600))
			require.NoError(t, rewriteGoPackage(path))
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Contains(t, string(content), `option go_package = "github.com/jftrade/jftrade-main/pkg/futu/pb/qotcommon;qotcommon";`)
			assert.NotContains(t, string(content), `go_package = "old"`)
			assert.NotContains(t, string(content), "\r")
		})
	}
}

func TestRewriteGoPackageRejectsMissingPackage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.proto")
	require.NoError(t, os.WriteFile(path, []byte("syntax = \"proto3\";\n"), 0o600))
	err := rewriteGoPackage(path)
	require.ErrorContains(t, err, "no package")
}

func TestOrganizeGeneratedFilesUsesPackageDirectories(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(directory, "Common.pb.go"), []byte("package common\n"), 0o600,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(directory, "NoPackage.pb.go"), []byte("// missing package\n"), 0o600,
	))

	moved, err := organizeGeneratedFiles(directory)
	require.NoError(t, err)
	assert.Equal(t, 1, moved)
	_, err = os.Stat(filepath.Join(directory, "common", "Common.pb.go"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(directory, "NoPackage.pb.go"))
	require.NoError(t, err)
}
