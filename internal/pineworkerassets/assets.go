package pineworkerassets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
)

const binDir = "bin"

type Asset struct {
	Name   string
	Data   []byte
	SHA256 string
}

func Select() (Asset, bool, error) {
	return SelectForPlatform(runtime.GOOS, runtime.GOARCH)
}

func SelectForPlatform(goos string, goarch string) (Asset, bool, error) {
	name, err := BinaryName(goos, goarch)
	if err != nil {
		return Asset{}, false, err
	}
	data, err := fs.ReadFile(assetFS(), filepath.ToSlash(filepath.Join(binDir, name)))
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

func BinaryName(goos string, goarch string) (string, error) {
	platform := Platform(goos, goarch)
	if platform == "" {
		return "", fmt.Errorf("unsupported pine worker platform: %s/%s", goos, goarch)
	}
	name := "worker-" + platform
	if goos == "windows" {
		name += ".exe"
	}
	return name, nil
}

func Platform(goos string, goarch string) string {
	switch strings.ToLower(strings.TrimSpace(goos)) + "/" + strings.ToLower(strings.TrimSpace(goarch)) {
	case "darwin/arm64":
		return "darwin-arm64"
	case "darwin/amd64":
		return "darwin-x64"
	case "linux/amd64":
		return "linux-x64"
	case "linux/arm64":
		return "linux-arm64"
	case "windows/amd64":
		return "windows-x64"
	case "windows/arm64":
		return "windows-arm64"
	default:
		return ""
	}
}

func isMissingAsset(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "file does not exist") || strings.Contains(err.Error(), "no such file"))
}
