//go:build release_assets

package yfinanceassets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"testing"
)

func TestSelectReturnsStagedPlatformAssetWhenPresent(t *testing.T) {
	name := BinaryName()
	assetPath := filepath.ToSlash(filepath.Join(binDir, name))
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
	if !available || asset.Name != name || string(asset.Data) != string(expectedData) {
		t.Fatalf("Select = %#v available=%v, want staged %s", asset, available, name)
	}
	sum := sha256.Sum256(expectedData)
	if asset.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("Select SHA256 = %q, want %q", asset.SHA256, hex.EncodeToString(sum[:]))
	}
}
