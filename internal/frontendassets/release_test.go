//go:build release_assets

package frontendassets

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSystemEmbedsUnderscorePrefixedAssets(t *testing.T) {
	diskEntries, err := os.ReadDir("dist/assets")
	if err != nil {
		t.Fatalf("ReadDir dist/assets: %v", err)
	}

	underscoreAssets := make([]string, 0)
	for _, entry := range diskEntries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		underscoreAssets = append(underscoreAssets, filepath.ToSlash(filepath.Join("assets", entry.Name())))
	}
	if len(underscoreAssets) == 0 {
		t.Skip("no underscore-prefixed assets staged in dist/assets")
	}

	frontendFS, available, err := FileSystem()
	if err != nil {
		t.Fatalf("FileSystem: %v", err)
	}
	if !available {
		t.Fatal("expected embedded frontend assets to be available")
	}

	for _, assetPath := range underscoreAssets {
		if _, err := fs.Stat(frontendFS, assetPath); err != nil {
			t.Fatalf("embedded filesystem missing %s: %v", assetPath, err)
		}
	}
}
