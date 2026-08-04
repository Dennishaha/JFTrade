//go:build !release_assets

package marketdataassets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSelectReturnsUnavailableWithoutReleaseAssets(t *testing.T) {
	if !DevelopmentOverridesAllowed() {
		t.Fatal("development build disabled helper overrides")
	}
	asset, available, err := Select()
	if err != nil {
		t.Fatalf("Select error = %v", err)
	}
	if available || asset.Name != "" || len(asset.Files) != 0 || asset.SHA256 != "" {
		t.Fatalf("Select = %#v available=%v, want unavailable empty asset", asset, available)
	}
	materialized, available, err := Materialize()
	if err != nil {
		t.Fatalf("Materialize error = %v", err)
	}
	if available || materialized != nil {
		t.Fatalf("Materialize = %#v available=%v, want unavailable", materialized, available)
	}
}

func TestReleaseReturnsUnavailableWithoutReleaseAssets(t *testing.T) {
	materialized, available, err := Release()
	if err != nil {
		t.Fatalf("Release error = %v", err)
	}
	if available || materialized != nil {
		t.Fatalf("Release = %#v available=%v, want unavailable", materialized, available)
	}
}

func TestDevelopmentCacheWrappersRemainSafeWithoutEmbeddedAssets(t *testing.T) {
	materialized, available, err := MaterializeCached(filepath.Join(t.TempDir(), "cache"))
	if err != nil || available || materialized != nil {
		t.Fatalf("MaterializeCached = %#v, %v, %v; want unavailable", materialized, available, err)
	}

	cacheRoot := t.TempDir()
	expired := filepath.Join(cacheRoot, "expired")
	if err := os.Mkdir(expired, assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-cachedAssetRetention - time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}
	PruneCached(cacheRoot, "current")
	if _, err := os.Stat(expired); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("PruneCached left expired bundle: %v", err)
	}
}
