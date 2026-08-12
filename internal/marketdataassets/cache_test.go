package marketdataassets

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMaterializeCachedAssetReusesVerifiedContent(t *testing.T) {
	asset := cachedTestAsset(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	now := time.Now()

	first, available, err := materializeCachedAsset(asset, cacheRoot, now)
	if err != nil || !available || first == nil {
		t.Fatalf("first materialization = %#v, %v, %v", first, available, err)
	}
	info, err := os.Stat(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	firstModTime := info.ModTime()
	second, available, err := materializeCachedAsset(asset, cacheRoot, now.Add(time.Minute))
	if err != nil || !available || second == nil || second.Path != first.Path {
		t.Fatalf("cached materialization = %#v, %v, %v", second, available, err)
	}
	info, err = os.Stat(second.Path)
	if err != nil || !info.ModTime().Equal(firstModTime) {
		t.Fatalf("cache hit rewrote executable: info=%#v err=%v", info, err)
	}
	if err := second.Cleanup(); err != nil {
		t.Fatalf("persistent Cleanup: %v", err)
	}
	if _, err := os.Stat(second.Path); err != nil {
		t.Fatalf("persistent Cleanup removed cache: %v", err)
	}
}

func TestMaterializeCachedAssetRepairsTamperAndSymlink(t *testing.T) {
	asset := cachedTestAsset(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	materialized, _, err := materializeCachedAsset(asset, cacheRoot, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(materialized.Path, []byte("tampered"), assetFileMode); err != nil {
		t.Fatal(err)
	}
	repaired, _, err := materializeCachedAsset(asset, cacheRoot, time.Now())
	if err != nil {
		t.Fatalf("repair tamper: %v", err)
	}
	data, err := os.ReadFile(repaired.Path)
	if err != nil || string(data) != "sidecar" {
		t.Fatalf("repaired executable = %q, %v", data, err)
	}
	dependency := filepath.Join(filepath.Dir(repaired.Path), "lib", "runtime")
	if err := os.Remove(dependency); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(repaired.Path, dependency); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	repaired, _, err = materializeCachedAsset(asset, cacheRoot, time.Now())
	if err != nil {
		t.Fatalf("repair symlink: %v", err)
	}
	info, err := os.Lstat(filepath.Join(filepath.Dir(repaired.Path), "lib", "runtime"))
	if err != nil || info.Mode()&fs.ModeSymlink != 0 {
		t.Fatalf("dependency was not repaired: %#v, %v", info, err)
	}
}

func TestMaterializeCachedAssetPublishesConcurrently(t *testing.T) {
	asset := cachedTestAsset(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	const workers = 6
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			materialized, available, err := materializeCachedAsset(asset, cacheRoot, time.Now())
			if err != nil || !available || materialized == nil {
				errs <- err
				return
			}
			paths <- materialized.Path
		})
	}
	wait.Wait()
	close(paths)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent materialization: %v", err)
		}
	}
	want := ""
	for path := range paths {
		if want == "" {
			want = path
		} else if path != want {
			t.Fatalf("concurrent cache paths differ: %q != %q", path, want)
		}
	}
}

func TestPublishCachedAssetAcceptsOnlyAValidConcurrentWinner(t *testing.T) {
	asset := cachedTestAsset(t)
	files, digest, available, err := validatedAsset(asset)
	if err != nil || !available {
		t.Fatalf("validatedAsset = available %v, error %v", available, err)
	}

	t.Run("valid winner", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		staging := filepath.Join(root, "staging")
		writeCachedFixture(t, target, files)
		writeCachedFixture(t, staging, files)

		if err := publishCachedAsset(staging, target, files, digest); err != nil {
			t.Fatalf("publishCachedAsset rejected valid winner: %v", err)
		}
		if _, err := os.Stat(staging); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("losing staging directory still exists: %v", err)
		}
	})

	t.Run("invalid winner", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		staging := filepath.Join(root, "staging")
		writeCachedFixture(t, target, files)
		writeCachedFixture(t, staging, files)
		if err := os.WriteFile(filepath.Join(target, "unexpected"), []byte("bad"), assetFileMode); err != nil {
			t.Fatal(err)
		}

		if err := publishCachedAsset(staging, target, files, digest); err == nil {
			t.Fatal("publishCachedAsset accepted invalid concurrent winner")
		}
		if _, err := os.Stat(staging); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("failed staging directory still exists: %v", err)
		}
	})
}

func TestMaterializeCachedAssetPrunesOnlyExpiredDigests(t *testing.T) {
	asset := cachedTestAsset(t)
	cacheRoot := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(filepath.Join(cacheRoot, "expired"), assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cacheRoot, "recent"), assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	expired := now.Add(-cachedAssetRetention - time.Hour)
	if err := os.Chtimes(filepath.Join(cacheRoot, "expired"), expired, expired); err != nil {
		t.Fatal(err)
	}
	if _, _, err := materializeCachedAsset(asset, cacheRoot, now); err != nil {
		t.Fatal(err)
	}
	pruneCachedAssets(cacheRoot, asset.SHA256, now)
	if _, err := os.Stat(filepath.Join(cacheRoot, "expired")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expired cache still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cacheRoot, "recent")); err != nil {
		t.Fatalf("recent cache was pruned: %v", err)
	}
}

func TestPruneCachedAssetsIgnoresMissingCacheRoot(t *testing.T) {
	cacheRoot := filepath.Join(t.TempDir(), "missing")
	PruneCached(cacheRoot, "current")
	if _, err := os.Stat(cacheRoot); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("PruneCached created missing cache root: %v", err)
	}
}

func TestMaterializeCachedAssetRejectsUnsafeCacheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cache-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, available, err := materializeCachedAsset(
		cachedTestAsset(t),
		root,
		time.Now(),
	); err == nil || available {
		t.Fatalf("unsafe cache root = available %v, error %v", available, err)
	}
}

func TestEnsurePrivateCacheRootReportsUninspectablePath(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateCacheRoot(filepath.Join(parentFile, "cache")); err == nil ||
		!strings.Contains(err.Error(), "inspect market-data sidecar cache directory") {
		t.Fatalf("ensurePrivateCacheRoot error = %v", err)
	}
}

func TestRemoveInvalidCacheTargetReportsInspectionFailure(t *testing.T) {
	parentFile := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeInvalidCacheTarget(filepath.Join(parentFile, "cache")); err == nil ||
		!strings.Contains(err.Error(), "inspect invalid market-data sidecar cache") {
		t.Fatalf("removeInvalidCacheTarget error = %v", err)
	}
}

func TestRemoveInvalidCacheTargetReportsRemovalFailure(t *testing.T) {
	if !privateModeSupported() {
		t.Skip("platform does not expose POSIX directory permissions")
	}
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, assetDirectoryMode) })

	if err := removeInvalidCacheTarget(target); err == nil ||
		!strings.Contains(err.Error(), "remove invalid market-data sidecar cache") {
		t.Fatalf("removeInvalidCacheTarget error = %v", err)
	}
}

func TestMaterializeCachedAssetRejectsInvalidAssetAndRootInputs(t *testing.T) {
	tests := []struct {
		name  string
		asset Asset
		root  string
	}{
		{name: "empty asset", asset: Asset{}, root: t.TempDir()},
		{
			name: "duplicate path",
			asset: Asset{Name: "sidecar", Files: []AssetFile{
				{Path: "sidecar", Data: []byte("one")},
				{Path: "sidecar", Data: []byte("two")},
			}},
			root: t.TempDir(),
		},
		{
			name:  "missing executable",
			asset: assetWithDigest(t, "sidecar", []AssetFile{{Path: "runtime", Data: []byte("runtime")}}),
			root:  t.TempDir(),
		},
		{
			name: "digest mismatch",
			asset: Asset{
				Name:   "sidecar",
				Files:  []AssetFile{{Path: "sidecar", Data: []byte("sidecar")}},
				SHA256: strings.Repeat("0", 64),
			},
			root: t.TempDir(),
		},
		{
			name:  "relative cache root",
			asset: cachedTestAsset(t),
			root:  "relative-cache",
		},
		{
			name: "bundle file blocks child directory",
			asset: assetWithDigest(t, "sidecar", []AssetFile{
				{Path: "a", Data: []byte("file")},
				{Path: "a/runtime", Data: []byte("runtime")},
				{Path: "sidecar", Data: []byte("sidecar")},
			}),
			root: filepath.Join(t.TempDir(), "cache"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			materialized, available, err := materializeCachedAsset(
				test.asset,
				test.root,
				time.Now(),
			)
			if err == nil && test.name != "empty asset" && test.name != "missing executable" {
				t.Fatal("materializeCachedAsset returned nil error")
			}
			if materialized != nil || available {
				t.Fatalf("materializeCachedAsset = %#v, available %v, error %v", materialized, available, err)
			}
		})
	}
}

func TestWriteAssetFilesReportsInvalidPathsAndFilesystemConflicts(t *testing.T) {
	t.Run("invalid asset path", func(t *testing.T) {
		if err := writeAssetFiles(t.TempDir(), []AssetFile{{Path: "../escape"}}); err == nil {
			t.Fatal("writeAssetFiles accepted an escaping path")
		}
	})
	t.Run("file blocks child directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "blocked"), []byte("file"), assetFileMode); err != nil {
			t.Fatal(err)
		}
		if err := writeAssetFiles(root, []AssetFile{{Path: "blocked/runtime"}}); err == nil {
			t.Fatal("writeAssetFiles created a directory through a file")
		}
	})
	t.Run("directory blocks file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, "blocked"), assetDirectoryMode); err != nil {
			t.Fatal(err)
		}
		if err := writeAssetFiles(root, []AssetFile{{Path: "blocked"}}); err == nil {
			t.Fatal("writeAssetFiles replaced a directory with a file")
		}
	})
}

func TestValidateCachedAssetRejectsUnsafeShapes(t *testing.T) {
	asset := cachedTestAsset(t)
	files, digest, available, err := validatedAsset(asset)
	if err != nil || !available {
		t.Fatalf("validatedAsset = available %v, error %v", available, err)
	}
	tests := []struct {
		name                string
		mutate              func(*testing.T, string)
		digest              string
		requiresPrivateMode bool
	}{
		{name: "root is file", mutate: func(t *testing.T, root string) {
			if err := os.WriteFile(root, []byte("file"), assetFileMode); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "root permissions", requiresPrivateMode: true, mutate: func(t *testing.T, root string) {
			writeCachedFixture(t, root, files)
			if err := os.Chmod(root, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory permissions", requiresPrivateMode: true, mutate: func(t *testing.T, root string) {
			writeCachedFixture(t, root, files)
			if err := os.Chmod(filepath.Join(root, "lib"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "file permissions", requiresPrivateMode: true, mutate: func(t *testing.T, root string) {
			writeCachedFixture(t, root, files)
			if err := os.Chmod(filepath.Join(root, "sidecar"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "unexpected file", mutate: func(t *testing.T, root string) {
			writeCachedFixture(t, root, files)
			if err := os.WriteFile(filepath.Join(root, "extra"), []byte("extra"), assetFileMode); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "incomplete bundle", mutate: func(t *testing.T, root string) {
			if err := os.Mkdir(root, assetDirectoryMode); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "sidecar"), []byte("sidecar"), assetFileMode); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "digest mismatch", digest: strings.Repeat("0", 64), mutate: func(t *testing.T, root string) {
			writeCachedFixture(t, root, files)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.requiresPrivateMode && !privateModeSupported() {
				t.Skip("platform does not expose POSIX private modes")
			}
			root := filepath.Join(t.TempDir(), "bundle")
			test.mutate(t, root)
			wantDigest := test.digest
			if wantDigest == "" {
				wantDigest = digest
			}
			if err := validateCachedAsset(root, files, wantDigest); err == nil {
				t.Fatal("validateCachedAsset accepted an unsafe bundle")
			}
		})
	}
}

func TestValidateCachedAssetRejectsSocket(t *testing.T) {
	if !privateModeSupported() {
		t.Skip("platform does not expose POSIX unix sockets")
	}
	root := filepath.Join(t.TempDir(), "bundle")
	if err := os.Mkdir(root, assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(root, "runtime.sock"))
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	err = validateCachedAsset(
		root,
		[]AssetFile{{Path: "runtime.sock", Data: []byte("not-a-socket")}},
		strings.Repeat("0", 64),
	)
	if err == nil || !strings.Contains(err.Error(), "non-regular file") {
		t.Fatalf("validateCachedAsset error = %v", err)
	}
}

func writeCachedFixture(t *testing.T, root string, files []AssetFile) {
	t.Helper()
	if err := os.Mkdir(root, assetDirectoryMode); err != nil {
		t.Fatal(err)
	}
	if err := writeAssetFiles(root, files); err != nil {
		t.Fatal(err)
	}
}

func cachedTestAsset(t *testing.T) Asset {
	t.Helper()
	return assetWithDigest(t, "sidecar", []AssetFile{
		{Path: "sidecar", Data: []byte("sidecar")},
		{Path: "lib/runtime", Data: []byte("runtime")},
	})
}
