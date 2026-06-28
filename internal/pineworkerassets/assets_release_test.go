//go:build release_assets

package pineworkerassets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestSelectForPlatformReturnsEmbeddedAssetWhenStaged(t *testing.T) {
	const goos = "linux"
	const goarch = "amd64"

	name, err := BinaryName(goos, goarch)
	if err != nil {
		t.Fatalf("BinaryName: %v", err)
	}
	assetPath := filepath.ToSlash(filepath.Join(binDir, name))
	expectedData, err := fs.ReadFile(assetFS(), assetPath)
	if err != nil {
		t.Skipf("no staged %s release asset: %v", name, err)
	}
	if len(expectedData) == 0 {
		t.Skipf("staged %s release asset is empty", name)
	}

	asset, ok, err := SelectForPlatform(goos, goarch)
	if err != nil {
		t.Fatalf("SelectForPlatform: %v", err)
	}
	if !ok {
		t.Fatalf("SelectForPlatform ok = false, want true for staged %s", name)
	}
	if asset.Name != name {
		t.Fatalf("SelectForPlatform name = %q, want %q", asset.Name, name)
	}
	if string(asset.Data) != string(expectedData) {
		t.Fatalf("SelectForPlatform data mismatch for %s", name)
	}
	sum := sha256.Sum256(expectedData)
	if asset.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("SelectForPlatform sha = %q, want %q", asset.SHA256, hex.EncodeToString(sum[:]))
	}
}
