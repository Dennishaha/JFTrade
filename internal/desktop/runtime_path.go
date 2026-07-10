package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ProductDataDir resolves the per-user data directory used by packaged desktop builds.
func ProductDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	configDir, configErr := os.UserConfigDir()
	if configErr != nil {
		configDir = ""
	}
	return productDataDir(runtime.GOOS, homeDir, configDir, os.Getenv), nil
}

func productDataDir(goos string, homeDir string, configDir string, getenv func(string) string) string {
	homeDir = strings.TrimSpace(homeDir)
	configDir = strings.TrimSpace(configDir)
	if getenv == nil {
		getenv = func(string) string { return "" }
	}

	switch goos {
	case "darwin":
		if configDir == "" {
			configDir = filepath.Join(homeDir, "Library", "Application Support")
		}
		return filepath.Join(configDir, "JFTrade")
	case "windows":
		base := strings.TrimSpace(getenv("LOCALAPPDATA"))
		if base == "" {
			base = configDir
		}
		return filepath.Join(base, "JFTrade")
	default:
		base := strings.TrimSpace(getenv("XDG_DATA_HOME"))
		if base == "" {
			base = filepath.Join(homeDir, ".local", "share")
		}
		return filepath.Join(base, "jftrade")
	}
}
