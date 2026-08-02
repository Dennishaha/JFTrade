package yfinanceassets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBinaryNameForUsesGoPlatformNames(t *testing.T) {
	tests := []struct {
		goos, goarch, want string
	}{
		{goos: "darwin", goarch: "arm64", want: "yfinance-sidecar-darwin-arm64"},
		{goos: "linux", goarch: "amd64", want: "yfinance-sidecar-linux-amd64"},
		{goos: "windows", goarch: "amd64", want: "yfinance-sidecar-windows-amd64.exe"},
	}
	for _, test := range tests {
		if got := BinaryNameFor(test.goos, test.goarch); got != test.want {
			t.Errorf("BinaryNameFor(%q, %q) = %q, want %q", test.goos, test.goarch, got, test.want)
		}
	}
}

func TestSelectFromFSReturnsPlatformOnedirAssetMetadata(t *testing.T) {
	data := []byte("yfinance sidecar")
	asset, available, err := selectFromFS(fstest.MapFS{
		"bin/yfinance-sidecar-windows-amd64/yfinance-sidecar-windows-amd64.exe": &fstest.MapFile{Data: data},
		"bin/yfinance-sidecar-windows-amd64/lib/runtime.dll":                    &fstest.MapFile{Data: []byte("runtime")},
	}, "windows", "amd64")
	if err != nil || !available {
		t.Fatalf("selectFromFS() available=%v error=%v", available, err)
	}
	if asset.Name != BinaryNameFor("windows", "amd64") || len(asset.Files) != 2 {
		t.Fatalf("selectFromFS() asset = %#v", asset)
	}
	var executable AssetFile
	for _, file := range asset.Files {
		if file.Path == asset.Name {
			executable = file
			break
		}
	}
	if executable.Path != asset.Name || string(executable.Data) != string(data) {
		t.Fatalf("selectFromFS() executable = %#v", executable)
	}
	digest, err := digestAssetFiles(asset.Files)
	if err != nil || asset.SHA256 != digest {
		t.Fatalf("selectFromFS() digest = %q, want %q (error %v)", asset.SHA256, digest, err)
	}
}

func TestSelectFromFSTreatsMissingAndEmptyAssetsAsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name  string
		files fs.FS
	}{
		{name: "missing", files: fstest.MapFS{}},
		{name: "empty", files: fstest.MapFS{
			"bin/yfinance-sidecar-linux-amd64/yfinance-sidecar-linux-amd64": &fstest.MapFile{},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			asset, available, err := selectFromFS(test.files, "linux", "amd64")
			if err != nil {
				t.Fatalf("selectFromFS() error = %v", err)
			}
			if available || asset.Name != "" || len(asset.Files) != 0 || asset.SHA256 != "" {
				t.Fatalf("selectFromFS() = (%#v, %v), want empty unavailable asset", asset, available)
			}
		})
	}
}

func TestSelectFromFSReturnsUnexpectedReadError(t *testing.T) {
	wantErr := errors.New("asset storage unavailable")
	asset, available, err := selectFromFS(failingAssetFS{err: wantErr}, "linux", "amd64")
	if !errors.Is(err, wantErr) {
		t.Fatalf("selectFromFS() error = %v, want %v", err, wantErr)
	}
	if available || asset.Name != "" || len(asset.Files) != 0 || asset.SHA256 != "" {
		t.Fatalf("selectFromFS() = (%#v, %v), want empty unavailable asset", asset, available)
	}
}

func TestSelectFromFSRejectsInvalidPlatformBundleShapes(t *testing.T) {
	const (
		root       = "bin/yfinance-sidecar-linux-amd64"
		executable = root + "/yfinance-sidecar-linux-amd64"
	)
	tests := []struct {
		name      string
		files     fs.FS
		wantError bool
	}{
		{name: "bundle root is a file", files: fstest.MapFS{root: &fstest.MapFile{Data: []byte("not a directory")}}},
		{name: "executable is missing", files: fstest.MapFS{root + "/lib/runtime.so": &fstest.MapFile{Data: []byte("runtime")}}},
		{
			name: "executable cannot be read",
			files: pathFailingAssetFS{
				FS:   fstest.MapFS{executable: &fstest.MapFile{Data: []byte("sidecar")}},
				path: executable,
				err:  errors.New("executable read denied"),
			},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			asset, available, err := selectFromFS(test.files, "linux", "amd64")
			if (err != nil) != test.wantError {
				t.Fatalf("selectFromFS() error = %v, wantError %v", err, test.wantError)
			}
			if available || asset.Name != "" {
				t.Fatalf("selectFromFS() = (%#v, %v), want unavailable", asset, available)
			}
		})
	}
}

func TestReadAssetFilesRejectsUnsafeOrUnreadableEntries(t *testing.T) {
	const root = "bin/sidecar"
	tests := []struct {
		name  string
		files fs.FS
	}{
		{name: "walk failure", files: failingAssetFS{err: errors.New("walk denied")}},
		{name: "symlink", files: fstest.MapFS{
			root + "/runtime": &fstest.MapFile{Mode: fs.ModeSymlink},
		}},
		{name: "file read failure", files: pathFailingAssetFS{
			FS:   fstest.MapFS{root + "/runtime": &fstest.MapFile{Data: []byte("runtime")}},
			path: root + "/runtime",
			err:  errors.New("file read denied"),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := readAssetFiles(test.files, root); err == nil {
				t.Fatal("readAssetFiles() returned nil error")
			}
		})
	}
}

func TestMaterializeAssetUsesPrivateDirectoryAndCleansUp(t *testing.T) {
	asset := Asset{Name: "yfinance-sidecar-linux-amd64", Files: []AssetFile{
		{Path: "yfinance-sidecar-linux-amd64", Data: []byte("#!/bin/sh\nexit 0\n")},
		{Path: "lib/runtime.so", Data: []byte("runtime")},
	}}
	var err error
	asset.SHA256, err = digestAssetFiles(asset.Files)
	if err != nil {
		t.Fatalf("digestAssetFiles: %v", err)
	}
	materialized, available, err := materializeAsset(asset)
	if err != nil || !available || materialized == nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v", materialized, available, err)
	}
	path := materialized.Path
	info, err := fs.Stat(osDirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		t.Fatalf("materialized executable stat: %v", err)
	}
	assertPrivateMode(t, info.Mode().Perm(), assetFileMode, "executable")
	dependency, err := fs.Stat(osDirFS(filepath.Dir(path)), "lib/runtime.so")
	if err != nil {
		t.Fatalf("materialized dependency stat: %v", err)
	}
	assertPrivateMode(t, dependency.Mode().Perm(), assetFileMode, "dependency")
	dirInfo, err := fs.Stat(osDirFS(filepath.Dir(path)), ".")
	if err != nil {
		t.Fatalf("materialized directory stat: %v", err)
	}
	assertPrivateMode(t, dirInfo.Mode().Perm(), assetDirectoryMode, "directory")
	if err := materialized.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("materialized executable after Cleanup error = %v; want not exist", err)
	}
	if err := materialized.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func TestMaterializeAssetRejectsEmptyAsset(t *testing.T) {
	for _, asset := range []Asset{
		{},
		{Name: "sidecar", Files: nil, SHA256: "ignored"},
	} {
		materialized, available, err := materializeAsset(asset)
		if err != nil || available || materialized != nil {
			t.Fatalf("materializeAsset(%#v) = %#v, %v, %v; want unavailable", asset, materialized, available, err)
		}
	}
}

func TestMaterializeAssetRejectsDigestChanges(t *testing.T) {
	materialized, available, err := materializeAsset(Asset{
		Name:   "yfinance-sidecar-linux-amd64",
		Files:  []AssetFile{{Path: "yfinance-sidecar-linux-amd64", Data: []byte("sidecar")}},
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil || available || materialized != nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v; want digest mismatch", materialized, available, err)
	}
	if !strings.Contains(err.Error(), "SHA256 changed") {
		t.Fatalf("materializeAsset() error = %v, want SHA256 mismatch", err)
	}
}

func TestMaterializeAssetRejectsInvalidBundlePath(t *testing.T) {
	materialized, available, err := materializeAsset(Asset{
		Name:  "sidecar",
		Files: []AssetFile{{Path: "../sidecar", Data: []byte("sidecar")}},
	})
	if err == nil || available || materialized != nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v; want invalid path error", materialized, available, err)
	}
	if !strings.Contains(err.Error(), "invalid yfinance sidecar bundle file path") {
		t.Fatalf("materializeAsset() error = %v, want invalid path error", err)
	}
}

func TestMaterializeAssetRequiresUsableExecutableAndWritableTempRoot(t *testing.T) {
	t.Run("empty executable", func(t *testing.T) {
		asset := assetWithDigest(t, "sidecar", []AssetFile{{Path: "sidecar"}})
		materialized, available, err := materializeAsset(asset)
		if err != nil || available || materialized != nil {
			t.Fatalf("materializeAsset() = %#v, %v, %v; want unavailable", materialized, available, err)
		}
	})
	t.Run("named executable missing", func(t *testing.T) {
		asset := assetWithDigest(t, "sidecar", []AssetFile{{Path: "runtime", Data: []byte("runtime")}})
		materialized, available, err := materializeAsset(asset)
		if err != nil || available || materialized != nil {
			t.Fatalf("materializeAsset() = %#v, %v, %v; want unavailable", materialized, available, err)
		}
	})
	t.Run("temp root is not a directory", func(t *testing.T) {
		tempRoot := filepath.Join(t.TempDir(), "temp-root-file")
		if err := os.WriteFile(tempRoot, []byte("file"), 0o600); err != nil {
			t.Fatalf("write temp root fixture: %v", err)
		}
		t.Setenv("TMPDIR", tempRoot)
		asset := assetWithDigest(t, "sidecar", []AssetFile{{Path: "sidecar", Data: []byte("sidecar")}})
		materialized, available, err := materializeAsset(asset)
		if err == nil || available || materialized != nil {
			t.Fatalf("materializeAsset() = %#v, %v, %v; want temp directory error", materialized, available, err)
		}
	})
	t.Run("bundle file blocks a child directory", func(t *testing.T) {
		asset := assetWithDigest(t, "sidecar", []AssetFile{
			{Path: "a", Data: []byte("file")},
			{Path: "a/runtime", Data: []byte("runtime")},
			{Path: "sidecar", Data: []byte("sidecar")},
		})
		materialized, available, err := materializeAsset(asset)
		if err == nil || available || materialized != nil {
			t.Fatalf("materializeAsset() = %#v, %v, %v; want directory creation error", materialized, available, err)
		}
	})
}

func TestAssetPathAndDigestHelpersRejectDuplicateOrEscapingPaths(t *testing.T) {
	duplicate := []AssetFile{{Path: "runtime"}, {Path: "runtime"}}
	if _, err := normalizeAssetFiles(duplicate); err == nil {
		t.Fatal("normalizeAssetFiles() accepted a duplicate path")
	}
	if _, err := digestAssetFiles(duplicate); err == nil {
		t.Fatal("digestAssetFiles() accepted a duplicate path")
	}
	if _, err := materializedFilePath(t.TempDir(), "../runtime"); err == nil {
		t.Fatal("materializedFilePath() accepted an escaping path")
	}
	if _, err := digestMaterializedFiles(t.TempDir(), []AssetFile{{Path: "../runtime"}}); err == nil {
		t.Fatal("digestMaterializedFiles() accepted an escaping path")
	}
}

func TestCleanupHandlesNilAndAlreadyCleanedAssets(t *testing.T) {
	var nilAsset *MaterializedAsset
	if err := nilAsset.Cleanup(); err != nil {
		t.Fatalf("nil Cleanup: %v", err)
	}
	asset := &MaterializedAsset{}
	if err := asset.Cleanup(); err != nil {
		t.Fatalf("empty Cleanup: %v", err)
	}
}

func TestDigestMaterializedFilesReportsOpenError(t *testing.T) {
	_, err := digestMaterializedFiles(t.TempDir(), []AssetFile{{Path: "missing-sidecar"}})
	if err == nil {
		t.Fatal("digestMaterializedFiles() returned nil error for a missing file")
	}
}

func assertPrivateMode(t *testing.T, got, want fs.FileMode, target string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	if got != want {
		t.Fatalf("materialized %s mode = %o, want %o", target, got, want)
	}
}

func TestIsMissingAssetRecognizesOnlyNotFoundErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil},
		{name: "fs not exist", err: fs.ErrNotExist, want: true},
		{name: "wrapped fs not exist", err: fmt.Errorf("read asset: %w", fs.ErrNotExist), want: true},
		{name: "platform no such file", err: errors.New("no such file or directory"), want: true},
		{name: "other error", err: errors.New("permission denied")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := isMissingAsset(test.err); got != test.want {
				t.Fatalf("isMissingAsset(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

type failingAssetFS struct {
	err error
}

type pathFailingAssetFS struct {
	fs.FS
	path string
	err  error
}

func (files pathFailingAssetFS) Open(name string) (fs.File, error) {
	if name == files.path {
		return nil, files.err
	}
	return files.FS.Open(name)
}

func (files failingAssetFS) Open(string) (fs.File, error) {
	return nil, files.err
}

type osDirFS string

func (dir osDirFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.Join(string(dir), name))
}

func assetWithDigest(t *testing.T, name string, files []AssetFile) Asset {
	t.Helper()
	digest, err := digestAssetFiles(files)
	if err != nil {
		t.Fatalf("digestAssetFiles(): %v", err)
	}
	return Asset{Name: name, Files: files, SHA256: digest}
}
