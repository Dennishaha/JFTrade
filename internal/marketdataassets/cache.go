package marketdataassets

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const cachedAssetRetention = 7 * 24 * time.Hour

// MaterializeCached selects the embedded bundle and publishes it into a
// content-addressed private cache. Cleanup on the returned asset is a no-op.
func MaterializeCached(cacheRoot string) (*MaterializedAsset, bool, error) {
	asset, available, err := Select()
	if err != nil || !available {
		return nil, available, err
	}
	return materializeCachedAsset(asset, cacheRoot, time.Now())
}

func materializeCachedAsset(
	asset Asset,
	cacheRoot string,
	_ time.Time,
) (*MaterializedAsset, bool, error) {
	files, digest, available, err := validatedAsset(asset)
	if err != nil || !available {
		return nil, available, err
	}
	cacheRoot = filepath.Clean(strings.TrimSpace(cacheRoot))
	if cacheRoot == "." || !filepath.IsAbs(cacheRoot) {
		return nil, false, fmt.Errorf("market-data sidecar cache root must be absolute")
	}
	if err := ensurePrivateCacheRoot(cacheRoot); err != nil {
		return nil, false, err
	}
	target := filepath.Join(cacheRoot, strings.ToLower(digest))
	if err := validateCachedAsset(target, files, digest); err == nil {
		return persistentAsset(asset, target, digest), true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		// A missing target is the normal concurrent-publish case. Removing it
		// after another publisher wins would race with its atomic rename.
		if removeErr := removeInvalidCacheTarget(target); removeErr != nil {
			return nil, false, errors.Join(err, removeErr)
		}
	}

	staging, err := os.MkdirTemp(cacheRoot, ".staging-")
	if err != nil {
		return nil, false, fmt.Errorf("create market-data sidecar cache staging directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(staging) }
	if err := os.Chmod(staging, assetDirectoryMode); err != nil {
		cleanup()
		return nil, false, fmt.Errorf("restrict market-data sidecar cache staging directory: %w", err)
	}
	if err := writeAssetFiles(staging, files); err != nil {
		cleanup()
		return nil, false, err
	}
	if err := validateCachedAsset(staging, files, digest); err != nil {
		cleanup()
		return nil, false, fmt.Errorf("verify staged market-data sidecar cache: %w", err)
	}
	if err := publishCachedAsset(staging, target, files, digest); err != nil {
		cleanup()
		return nil, false, err
	}
	return persistentAsset(asset, target, digest), true, nil
}

func publishCachedAsset(staging string, target string, files []AssetFile, digest string) error {
	if err := os.Rename(staging, target); err != nil {
		defer func() { // A concurrent publisher may have won the race.
			_ = os.RemoveAll(staging)
		}()
		if validateErr := validateCachedAsset(target, files, digest); validateErr != nil {
			return errors.Join(
				fmt.Errorf("publish market-data sidecar cache: %w", err),
				validateErr,
			)
		}
	}
	return nil
}

// PruneCached removes stale non-current bundles after the current helper has
// started successfully. Cleanup is best-effort and never affects the caller.
func PruneCached(cacheRoot string, currentDigest string) {
	pruneCachedAssets(cacheRoot, strings.ToLower(currentDigest), time.Now())
}

func validatedAsset(asset Asset) ([]AssetFile, string, bool, error) {
	if asset.Name == "" || len(asset.Files) == 0 {
		return nil, "", false, nil
	}
	files, err := normalizeAssetFiles(asset.Files)
	if err != nil {
		return nil, "", false, err
	}
	found := false
	for _, file := range files {
		if file.Path == asset.Name && len(file.Data) > 0 {
			found = true
		}
	}
	if !found {
		return nil, "", false, nil
	}
	digest, err := digestAssetFiles(files)
	if err != nil {
		return nil, "", false, err
	}
	if !strings.EqualFold(digest, asset.SHA256) {
		return nil, "", false, errors.New(
			"market-data sidecar bundle SHA256 changed while materializing",
		)
	}
	return files, digest, true, nil
}

func ensurePrivateCacheRoot(cacheRoot string) error {
	info, err := os.Lstat(cacheRoot)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(cacheRoot, assetDirectoryMode); err != nil {
			return fmt.Errorf("create market-data sidecar cache directory: %w", err)
		}
	case err != nil:
		return fmt.Errorf("inspect market-data sidecar cache directory: %w", err)
	case info.Mode()&fs.ModeSymlink != 0 || !info.IsDir():
		return fmt.Errorf("market-data sidecar cache root must be a regular directory")
	}
	if err := os.Chmod(cacheRoot, assetDirectoryMode); err != nil {
		return fmt.Errorf("restrict market-data sidecar cache directory: %w", err)
	}
	return nil
}

func writeAssetFiles(root string, files []AssetFile) error {
	for _, file := range files {
		path, err := materializedFilePath(root, file.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), assetDirectoryMode); err != nil {
			return fmt.Errorf("create market-data sidecar cache bundle directory: %w", err)
		}
		if err := os.Chmod(filepath.Dir(path), assetDirectoryMode); err != nil {
			return fmt.Errorf("restrict market-data sidecar cache bundle directory: %w", err)
		}
		if err := os.WriteFile(path, file.Data, assetFileMode); err != nil {
			return fmt.Errorf("write market-data sidecar cache bundle file: %w", err)
		}
		if err := os.Chmod(path, assetFileMode); err != nil {
			return fmt.Errorf("restrict market-data sidecar cache bundle file: %w", err)
		}
	}
	return nil
}

func validateCachedAsset(root string, files []AssetFile, digest string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("inspect cached market-data sidecar bundle: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("cached market-data sidecar bundle root is unsafe")
	}
	if privateModeSupported() && info.Mode().Perm() != assetDirectoryMode {
		return fmt.Errorf("cached market-data sidecar bundle root permissions are unsafe")
	}
	expected := make(map[string]struct{}, len(files))
	for _, file := range files {
		expected[filepath.Clean(filepath.FromSlash(file.Path))] = struct{}{}
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("cached market-data sidecar bundle contains a symlink")
		}
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if privateModeSupported() && info.Mode().Perm() != assetDirectoryMode {
				return fmt.Errorf("cached market-data sidecar bundle directory permissions are unsafe")
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("cached market-data sidecar bundle contains a non-regular file")
		}
		if privateModeSupported() && info.Mode().Perm() != assetFileMode {
			return fmt.Errorf("cached market-data sidecar bundle file permissions are unsafe")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, ok := expected[filepath.Clean(relative)]; !ok {
			return fmt.Errorf("cached market-data sidecar bundle contains an unexpected file")
		}
		delete(expected, filepath.Clean(relative))
		return nil
	})
	if err != nil {
		return err
	}
	if len(expected) != 0 {
		return fmt.Errorf("cached market-data sidecar bundle is incomplete")
	}
	actual, err := digestMaterializedFiles(root, files)
	if err != nil {
		return err
	}
	if !strings.EqualFold(actual, digest) {
		return fmt.Errorf("cached market-data sidecar bundle SHA256 mismatch")
	}
	return nil
}

func removeInvalidCacheTarget(target string) error {
	if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect invalid market-data sidecar cache: %w", err)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("remove invalid market-data sidecar cache: %w", err)
	}
	return nil
}

func persistentAsset(asset Asset, root string, digest string) *MaterializedAsset {
	path, _ := materializedFilePath(root, asset.Name)
	return &MaterializedAsset{Path: path, Name: asset.Name, SHA256: digest}
}

func pruneCachedAssets(cacheRoot string, currentDigest string, now time.Time) {
	entries, err := os.ReadDir(cacheRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == currentDigest || strings.HasPrefix(name, ".staging-") ||
			!entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < cachedAssetRetention {
			continue
		}
		_ = os.RemoveAll(filepath.Join(cacheRoot, name))
	}
}

func privateModeSupported() bool {
	return runtime.GOOS != "windows"
}
