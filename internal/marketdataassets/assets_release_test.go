//go:build release_assets

package marketdataassets

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSelectReturnsStagedPlatformAssetWhenPresent(t *testing.T) {
	if DevelopmentOverridesAllowed() {
		t.Fatal("release-assets build accepted development helper overrides")
	}
	name := BinaryName()
	assetRoot := path.Join(binDir, assetDirectoryName(runtime.GOOS, runtime.GOARCH))
	assetPath := path.Join(assetRoot, name)
	expectedData, err := fs.ReadFile(assetFS(), assetPath)
	if err != nil {
		t.Skipf("no staged %s release asset: %v", name, err)
	}
	if len(expectedData) == 0 {
		t.Skipf("staged %s release asset is empty", name)
	}

	asset, available, err := Select()
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	executableData := []byte(nil)
	for _, file := range asset.Files {
		if file.Path == name {
			executableData = file.Data
			break
		}
	}
	if !available || asset.Name != name || len(executableData) == 0 || string(executableData) != string(expectedData) {
		t.Fatalf("Select = %#v available=%v, want staged %s", asset, available, name)
	}
	digest, err := digestAssetFiles(asset.Files)
	if err != nil || asset.SHA256 != digest {
		t.Fatalf("Select SHA256 = %q, want %q (error %v)", asset.SHA256, digest, err)
	}
}

func TestStagedDarwinBundleEmbedsPythonLoaderAsRegularFile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("PyInstaller macOS loader layout only applies to darwin")
	}
	root := path.Join(binDir, assetDirectoryName(runtime.GOOS, runtime.GOARCH))
	if _, err := fs.Stat(assetFS(), root); err != nil {
		t.Skipf("no staged %s release bundle: %v", BinaryName(), err)
	}
	for _, relative := range []string{
		"_internal/Python",
		"_internal/Python.framework/Python",
		"_internal/Python.framework/Versions/Current/Python",
	} {
		info, err := fs.Stat(assetFS(), path.Join(root, relative))
		if err != nil {
			t.Fatalf("embedded materialized Python loader %q: %v", relative, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			t.Fatalf("embedded Python loader %q is not a non-empty regular file", relative)
		}
	}
}

func TestMaterializeCachedReleaseAssetReusesBundleAndFallsBack(t *testing.T) {
	if _, available, err := Select(); err != nil || !available {
		t.Skipf("no staged release asset: available=%v err=%v", available, err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	first, available, err := MaterializeCached(cacheRoot)
	if err != nil || !available || first == nil {
		t.Fatalf("first cached release asset = %#v available=%v err=%v", first, available, err)
	}
	info, err := os.Stat(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second, available, err := MaterializeCached(cacheRoot)
	if err != nil || !available || second == nil || second.Path != first.Path {
		t.Fatalf("reused release asset = %#v available=%v err=%v", second, available, err)
	}
	reusedInfo, err := os.Stat(second.Path)
	if err != nil || !reusedInfo.ModTime().Equal(info.ModTime()) {
		t.Fatalf("release cache hit rewrote helper: info=%#v err=%v", reusedInfo, err)
	}

	unsafeRoot := filepath.Join(t.TempDir(), "cache-file")
	if err := os.WriteFile(unsafeRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MaterializeCached(unsafeRoot); err == nil {
		t.Fatal("unsafe persistent cache unexpectedly succeeded")
	}
	fallback, available, err := Materialize()
	if err != nil || !available || fallback == nil {
		t.Fatalf("temporary fallback = %#v available=%v err=%v", fallback, available, err)
	}
	if err := fallback.Cleanup(); err != nil {
		t.Fatalf("temporary fallback cleanup: %v", err)
	}
}
