//go:build release_assets

package frontendassets

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSystemEmbedsUnderscorePrefixedAssets(t *testing.T) {
	// Vite/Rollup can emit chunk assets whose filenames start with "_".
	// This release-only test guards the packaged zip filesystem from dropping
	// those assets when they are present in a staged frontend build.
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

func TestFileSystemEmbedsDocumentationAndLegalNotices(t *testing.T) {
	frontendFS, available, err := FileSystem()
	if err != nil {
		t.Fatalf("FileSystem: %v", err)
	}
	if !available {
		t.Fatal("expected embedded frontend assets to be available")
	}
	for _, path := range []string{
		"docs/index.html",
		"docs/legal/license.html",
		"docs/legal/third-party-notices.html",
	} {
		info, err := fs.Stat(frontendFS, path)
		if err != nil {
			t.Fatalf("embedded filesystem missing %s: %v", path, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("embedded documentation %s is empty or invalid", path)
		}
	}

	for path, required := range map[string][]string{
		"docs/legal/license.html": {
			"AGPL-3.0-only",
			"GNU AFFERO GENERAL PUBLIC LICENSE",
			"Copyright (C) 2026 JFTrade Contributors",
		},
		"docs/legal/third-party-notices.html": {
			"pinets",
			"github.com/c9s/bbgo@v1.64.2",
			"Copyright Suneido Software Corp.",
			"Apache License",
		},
	} {
		data, err := fs.ReadFile(frontendFS, path)
		if err != nil {
			t.Fatalf("read embedded documentation %s: %v", path, err)
		}
		for _, needle := range required {
			if !strings.Contains(string(data), needle) {
				t.Fatalf("embedded documentation %s is missing %q", path, needle)
			}
		}
	}
}

func TestFileSystemDoesNotEmbedRemovedGoPineRuntimeReferences(t *testing.T) {
	frontendFS, available, err := FileSystem()
	if err != nil {
		t.Fatalf("FileSystem: %v", err)
	}
	if !available {
		t.Fatal("expected embedded frontend assets to be available")
	}

	forbidden := []string{
		"pkg/strategy/pineruntime",
		"BenchmarkPineRuntime",
		"BenchmarkRunExecutesPineGoldenMatrix",
	}
	if err := fs.WalkDir(frontendFS, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || strings.HasPrefix(path, "docs/") || !isFrontendTextAsset(path) {
			return nil
		}
		file, err := frontendFS.Open(path)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		text := string(data)
		for _, value := range forbidden {
			if strings.Contains(text, value) {
				t.Fatalf("embedded frontend asset %s still references removed Go Pine runtime %q", path, value)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walk embedded frontend assets: %v", err)
	}
}

func isFrontendTextAsset(path string) bool {
	switch filepath.Ext(path) {
	case ".html", ".js", ".css", ".json", ".txt", ".map":
		return true
	default:
		return false
	}
}
