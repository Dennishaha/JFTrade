//go:build release_assets

package pineworkerassets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestSelectReturnsEmbeddedBundleWhenStaged(t *testing.T) {
	name := BundleName()
	assetPath := filepath.ToSlash(filepath.Join(binDir, name))
	expectedData, err := fs.ReadFile(assetFS(), assetPath)
	if err != nil {
		t.Skipf("no staged %s release asset: %v", name, err)
	}
	if len(expectedData) == 0 {
		t.Skipf("staged %s release asset is empty", name)
	}

	asset, ok, err := Select()
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if !ok {
		t.Fatalf("Select ok = false, want true for staged %s", name)
	}
	if asset.Name != name {
		t.Fatalf("Select name = %q, want %q", asset.Name, name)
	}
	if string(asset.Data) != string(expectedData) {
		t.Fatalf("Select data mismatch for %s", name)
	}
	sum := sha256.Sum256(expectedData)
	if asset.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("Select sha = %q, want %q", asset.SHA256, hex.EncodeToString(sum[:]))
	}
}
