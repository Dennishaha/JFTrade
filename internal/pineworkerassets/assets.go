package pineworkerassets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

const (
	binDir           = "bin"
	workerBundleName = "worker.mjs"
)

type Asset struct {
	Name   string
	Data   []byte
	SHA256 string
}

func Select() (Asset, bool, error) {
	return selectFromFS(assetFS())
}

func selectFromFS(files fs.FS) (Asset, bool, error) {
	name := BundleName()
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

func BundleName() string {
	return workerBundleName
}

func isMissingAsset(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrNotExist) {
		return true
	}
	// Some platform-specific embed/fs implementations return an unwrapped OS
	// error. Keep this compatibility fallback at the filesystem boundary.
	return strings.Contains(err.Error(), "file does not exist") || strings.Contains(err.Error(), "no such file")
}
