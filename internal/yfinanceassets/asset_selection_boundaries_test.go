package yfinanceassets

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestSelectFromFSReturnsPlatformAssetMetadata(t *testing.T) {
	data := []byte("yfinance sidecar")
	asset, available, err := selectFromFS(fstest.MapFS{
		"bin/yfinance-sidecar-windows-amd64.exe": &fstest.MapFile{Data: data},
	}, "windows", "amd64")
	if err != nil || !available {
		t.Fatalf("selectFromFS() available=%v error=%v", available, err)
	}
	sum := sha256.Sum256(data)
	if asset.Name != BinaryNameFor("windows", "amd64") || string(asset.Data) != string(data) || asset.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("selectFromFS() asset = %#v", asset)
	}
}

func TestSelectFromFSTreatsMissingAndEmptyAssetsAsUnavailable(t *testing.T) {
	for _, test := range []struct {
		name  string
		files fs.FS
	}{
		{name: "missing", files: fstest.MapFS{}},
		{name: "empty", files: fstest.MapFS{"bin/yfinance-sidecar-linux-amd64": &fstest.MapFile{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			asset, available, err := selectFromFS(test.files, "linux", "amd64")
			if err != nil {
				t.Fatalf("selectFromFS() error = %v", err)
			}
			if available || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
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
	if available || asset.Name != "" || len(asset.Data) != 0 || asset.SHA256 != "" {
		t.Fatalf("selectFromFS() = (%#v, %v), want empty unavailable asset", asset, available)
	}
}

func TestMaterializeAssetUsesPrivateDirectoryAndCleansUp(t *testing.T) {
	data := []byte("#!/bin/sh\nexit 0\n")
	sum := sha256.Sum256(data)
	materialized, available, err := materializeAsset(Asset{
		Name:   "yfinance-sidecar-linux-amd64",
		Data:   data,
		SHA256: hex.EncodeToString(sum[:]),
	})
	if err != nil || !available || materialized == nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v", materialized, available, err)
	}
	path := materialized.Path
	info, err := fs.Stat(osDirFS(filepath.Dir(path)), filepath.Base(path))
	if err != nil {
		t.Fatalf("materialized file stat: %v", err)
	}
	assertPrivateMode(t, info.Mode().Perm(), assetFileMode, "file")
	dirInfo, err := fs.Stat(osDirFS(filepath.Dir(path)), ".")
	if err != nil {
		t.Fatalf("materialized directory stat: %v", err)
	}
	assertPrivateMode(t, dirInfo.Mode().Perm(), assetDirectoryMode, "directory")
	if err := materialized.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, err := fs.Stat(osDirFS(filepath.Dir(path)), filepath.Base(path)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("materialized file after Cleanup error = %v, want not exist", err)
	}
	if err := materialized.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func TestMaterializeAssetRejectsEmptyAsset(t *testing.T) {
	for _, asset := range []Asset{
		{},
		{Name: "sidecar", Data: nil, SHA256: "ignored"},
	} {
		materialized, available, err := materializeAsset(asset)
		if err != nil || available || materialized != nil {
			t.Fatalf("materializeAsset(%#v) = %#v, %v, %v; want unavailable", asset, materialized, available, err)
		}
	}
}

func TestMaterializeAssetCleansUpWhenDigestChanges(t *testing.T) {
	materialized, available, err := materializeAsset(Asset{
		Name:   "yfinance-sidecar-linux-amd64",
		Data:   []byte("sidecar"),
		SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil || available || materialized != nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v; want digest mismatch", materialized, available, err)
	}
	if !strings.Contains(err.Error(), "SHA256 changed") {
		t.Fatalf("materializeAsset() error = %v, want SHA256 mismatch", err)
	}
}

func TestMaterializeAssetReturnsWriteErrorAndCleansUp(t *testing.T) {
	// filepath.Base(".") resolves to the temporary directory itself, so the
	// write fails consistently on every supported platform without relying on
	// permissions or a mutable filesystem hook.
	materialized, available, err := materializeAsset(Asset{
		Name: "./",
		Data: []byte("sidecar"),
	})
	if err == nil || available || materialized != nil {
		t.Fatalf("materializeAsset() = %#v, %v, %v; want write error", materialized, available, err)
	}
	if !strings.Contains(err.Error(), "write yfinance sidecar executable") {
		t.Fatalf("materializeAsset() error = %v, want write error", err)
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

func TestFileSHA256ReportsOpenError(t *testing.T) {
	_, err := fileSHA256(filepath.Join(t.TempDir(), "missing-sidecar"))
	if err == nil {
		t.Fatal("fileSHA256() returned nil error for a missing file")
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

func (files failingAssetFS) Open(string) (fs.File, error) {
	return nil, files.err
}

type osDirFS string

func (dir osDirFS) Open(name string) (fs.File, error) {
	return os.Open(filepath.Join(string(dir), name))
}
