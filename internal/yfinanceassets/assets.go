package yfinanceassets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	binDir             = "bin"
	sidecarBaseName    = "yfinance-sidecar"
	tempDirPrefix      = "jftrade-yfinance-sidecar-"
	assetFileMode      = 0o700
	assetDirectoryMode = 0o700
)

// AssetFile is one file from a platform-specific PyInstaller onedir bundle.
// Path is relative to the bundle root and always uses slash separators.
type AssetFile struct {
	Path string
	Data []byte
}

// Asset is the selected platform-specific sidecar bundle. SHA256 is the
// lowercase hexadecimal digest of the sorted relative paths and file bytes.
type Asset struct {
	Name   string
	Files  []AssetFile
	SHA256 string
}

// MaterializedAsset is an embedded onedir bundle released to a private
// temporary directory. Path points to its executable. Call Cleanup when the
// process using Path exits.
type MaterializedAsset struct {
	Path   string
	Name   string
	SHA256 string

	tempDir string
}

// Select returns the embedded sidecar for the current runtime platform.
func Select() (Asset, bool, error) {
	return selectFromFS(assetFS(), runtime.GOOS, runtime.GOARCH)
}

// BinaryName returns the platform-specific embedded executable name.
func BinaryName() string {
	return BinaryNameFor(runtime.GOOS, runtime.GOARCH)
}

// BinaryNameFor returns the staged executable name for a Go platform tuple.
func BinaryNameFor(goos, goarch string) string {
	name := fmt.Sprintf("%s-%s-%s", sidecarBaseName, strings.ToLower(goos), strings.ToLower(goarch))
	if strings.EqualFold(goos, "windows") {
		return name + ".exe"
	}
	return name
}

// Materialize selects and releases the embedded sidecar bundle to a private
// temporary directory. The boolean is false when no release asset is embedded
// in this build.
func Materialize() (*MaterializedAsset, bool, error) {
	asset, available, err := Select()
	if err != nil || !available {
		return nil, available, err
	}
	return materializeAsset(asset)
}

// Release is an alias for Materialize for callers that describe the embedded
// executable as a release asset.
func Release() (*MaterializedAsset, bool, error) {
	return Materialize()
}

// Cleanup removes the private temporary directory holding the executable.
func (asset *MaterializedAsset) Cleanup() error {
	if asset == nil || asset.tempDir == "" {
		return nil
	}
	err := os.RemoveAll(asset.tempDir)
	if err == nil {
		asset.Path = ""
		asset.tempDir = ""
	}
	return err
}

// Close implements io.Closer and is equivalent to Cleanup.
func (asset *MaterializedAsset) Close() error {
	return asset.Cleanup()
}

func selectFromFS(files fs.FS, goos, goarch string) (Asset, bool, error) {
	name := BinaryNameFor(goos, goarch)
	root := pathpkg.Join(binDir, assetDirectoryName(goos, goarch))
	rootInfo, err := fs.Stat(files, root)
	if err != nil {
		if isMissingAsset(err) {
			return Asset{}, false, nil
		}
		return Asset{}, false, err
	}
	if !rootInfo.IsDir() {
		return Asset{}, false, nil
	}
	if executable, err := fs.ReadFile(files, pathpkg.Join(root, name)); err != nil {
		if isMissingAsset(err) {
			return Asset{}, false, nil
		}
		return Asset{}, false, err
	} else if len(executable) == 0 {
		return Asset{}, false, nil
	}
	assetFiles, err := readAssetFiles(files, root)
	if err != nil {
		return Asset{}, false, err
	}
	digest, err := digestAssetFiles(assetFiles)
	if err != nil {
		return Asset{}, false, err
	}
	return Asset{Name: name, Files: assetFiles, SHA256: digest}, true, nil
}

func materializeAsset(asset Asset) (*MaterializedAsset, bool, error) {
	if asset.Name == "" || len(asset.Files) == 0 {
		return nil, false, nil
	}
	assetFiles, err := normalizeAssetFiles(asset.Files)
	if err != nil {
		return nil, false, err
	}
	executableFound := false
	for _, file := range assetFiles {
		if file.Path != asset.Name {
			continue
		}
		executableFound = true
		if len(file.Data) == 0 {
			return nil, false, nil
		}
		break
	}
	if !executableFound {
		return nil, false, nil
	}
	digest, err := digestAssetFiles(assetFiles)
	if err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(digest, asset.SHA256) {
		return nil, false, errors.New("yfinance sidecar bundle SHA256 changed while materializing")
	}
	tempDir, err := os.MkdirTemp("", tempDirPrefix)
	if err != nil {
		return nil, false, fmt.Errorf("create yfinance sidecar temp directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(tempDir)
	}
	if err := os.Chmod(tempDir, assetDirectoryMode); err != nil {
		cleanup()
		return nil, false, fmt.Errorf("restrict yfinance sidecar temp directory: %w", err)
	}
	for _, file := range assetFiles {
		path, err := materializedFilePath(tempDir, file.Path)
		if err != nil {
			cleanup()
			return nil, false, err
		}
		if err := os.MkdirAll(filepath.Dir(path), assetDirectoryMode); err != nil {
			cleanup()
			return nil, false, fmt.Errorf("create yfinance sidecar bundle directory: %w", err)
		}
		if err := os.WriteFile(path, file.Data, assetFileMode); err != nil {
			cleanup()
			return nil, false, fmt.Errorf("write yfinance sidecar bundle file: %w", err)
		}
		if err := os.Chmod(path, assetFileMode); err != nil {
			cleanup()
			return nil, false, fmt.Errorf("restrict yfinance sidecar bundle file: %w", err)
		}
	}
	path, err := materializedFilePath(tempDir, asset.Name)
	if err != nil {
		cleanup()
		return nil, false, err
	}
	digest, err = digestMaterializedFiles(tempDir, assetFiles)
	if err != nil {
		cleanup()
		return nil, false, fmt.Errorf("hash yfinance sidecar bundle: %w", err)
	}
	if !strings.EqualFold(digest, asset.SHA256) {
		cleanup()
		return nil, false, errors.New("yfinance sidecar bundle SHA256 changed while materializing")
	}
	return &MaterializedAsset{
		Path: path, Name: asset.Name, SHA256: digest, tempDir: tempDir,
	}, true, nil
}

func assetDirectoryName(goos, goarch string) string {
	return strings.TrimSuffix(BinaryNameFor(goos, goarch), ".exe")
}

func readAssetFiles(files fs.FS, root string) ([]AssetFile, error) {
	var assetFiles []AssetFile
	err := fs.WalkDir(files, root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("yfinance sidecar bundle contains unsupported symlink %q", name)
		}
		data, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("read yfinance sidecar bundle file %q: %w", name, err)
		}
		relative := strings.TrimPrefix(name, root+"/")
		if relative == name || relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
			return fmt.Errorf("invalid yfinance sidecar bundle file path %q", name)
		}
		assetFiles = append(assetFiles, AssetFile{Path: relative, Data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return normalizeAssetFiles(assetFiles)
}

func normalizeAssetFiles(files []AssetFile) ([]AssetFile, error) {
	copyFiles := append([]AssetFile(nil), files...)
	sort.Slice(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path })
	for index, file := range copyFiles {
		clean := pathpkg.Clean(file.Path)
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || pathpkg.IsAbs(file.Path) || clean != file.Path {
			return nil, fmt.Errorf("invalid yfinance sidecar bundle file path %q", file.Path)
		}
		if index > 0 && copyFiles[index-1].Path == file.Path {
			return nil, fmt.Errorf("duplicate yfinance sidecar bundle file path %q", file.Path)
		}
		copyFiles[index].Path = clean
	}
	return copyFiles, nil
}

func digestAssetFiles(files []AssetFile) (string, error) {
	assetFiles, err := normalizeAssetFiles(files)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	for _, file := range assetFiles {
		if _, err := io.WriteString(hash, file.Path); err != nil {
			return "", err
		}
		if err := writeHashSeparator(hash); err != nil {
			return "", err
		}
		if _, err := hash.Write(file.Data); err != nil {
			return "", err
		}
		if err := writeHashSeparator(hash); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeHashSeparator(hash io.Writer) error {
	_, err := hash.Write([]byte{0})
	return err
}

func materializedFilePath(root, relative string) (string, error) {
	if _, err := normalizeAssetFiles([]AssetFile{{Path: relative}}); err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(relative))
	return path, nil
}

func digestMaterializedFiles(root string, files []AssetFile) (string, error) {
	materialized := make([]AssetFile, 0, len(files))
	for _, file := range files {
		path, err := materializedFilePath(root, file.Path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		materialized = append(materialized, AssetFile{Path: file.Path, Data: data})
	}
	return digestAssetFiles(materialized)
}

func isMissingAsset(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	return strings.Contains(err.Error(), "file does not exist") || strings.Contains(err.Error(), "no such file")
}
