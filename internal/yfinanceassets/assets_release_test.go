//go:build release_assets

package yfinanceassets

import (
	"io/fs"
	"path"
	"runtime"
	"testing"
)

func TestSelectReturnsStagedPlatformAssetWhenPresent(t *testing.T) {
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
