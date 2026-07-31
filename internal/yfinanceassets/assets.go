package yfinanceassets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	binDir             = "bin"
	sidecarBaseName    = "yfinance-sidecar"
	tempDirPrefix      = "jftrade-yfinance-sidecar-"
	assetFileMode      = 0o700
	assetDirectoryMode = 0o700
)

// Asset is the selected platform-specific sidecar executable as embedded
// bytes. SHA256 is the lowercase hexadecimal digest of Data.
type Asset struct {
	Name   string
	Data   []byte
	SHA256 string
}

// MaterializedAsset is an embedded executable released to a private temporary
// directory. Call Cleanup when the process using Path exits.
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

// Materialize selects and releases the embedded sidecar executable to a
// private temporary directory. The boolean is false when no release asset is
// embedded in this build.
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
	data, err := fs.ReadFile(files, filepath.ToSlash(filepath.Join(binDir, name)))
	if err != nil {
		if isMissingAsset(err) {
			return Asset{}, false, nil
		}
		return Asset{}, false, err
	}
	if len(data) == 0 {
		return Asset{}, false, nil
	}
	sum := sha256.Sum256(data)
	return Asset{Name: name, Data: data, SHA256: hex.EncodeToString(sum[:])}, true, nil
}

func materializeAsset(asset Asset) (*MaterializedAsset, bool, error) {
	if asset.Name == "" || len(asset.Data) == 0 {
		return nil, false, nil
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
	path := filepath.Join(tempDir, filepath.Base(asset.Name))
	if err := os.WriteFile(path, asset.Data, assetFileMode); err != nil {
		cleanup()
		return nil, false, fmt.Errorf("write yfinance sidecar executable: %w", err)
	}
	if err := os.Chmod(path, assetFileMode); err != nil {
		cleanup()
		return nil, false, fmt.Errorf("restrict yfinance sidecar executable: %w", err)
	}
	digest, err := fileSHA256(path)
	if err != nil {
		cleanup()
		return nil, false, fmt.Errorf("hash yfinance sidecar executable: %w", err)
	}
	if !strings.EqualFold(digest, asset.SHA256) {
		cleanup()
		return nil, false, errors.New("yfinance sidecar executable SHA256 changed while materializing")
	}
	return &MaterializedAsset{
		Path: path, Name: filepath.Base(asset.Name), SHA256: digest, tempDir: tempDir,
	}, true, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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
