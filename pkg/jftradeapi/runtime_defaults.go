package jftradeapi

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultDevelopmentAPIBind = "127.0.0.1:3000"
	defaultReleaseAPIBind     = "127.0.0.1:6699"
	defaultReleaseGUIBind     = "127.0.0.1:6688"
	defaultRuntimeDir         = "var/jftrade-api"
	defaultSettingsFilename   = "settings.json"
	defaultBacktestDBFilename = "backtest.db"
)

type launchDefaults struct {
	apiBind        string
	guiBind        string
	settingsPath   string
	backtestDBPath string
}

func resolveLaunchDefaults(embeddedFrontend bool) launchDefaults {
	return launchDefaultsForExecutableDir(embeddedFrontend, resolveExecutableDir())
}

func launchDefaultsForExecutableDir(embeddedFrontend bool, executableDir string) launchDefaults {
	runtimeRoot := defaultRuntimeRoot(embeddedFrontend, executableDir)
	defaults := launchDefaults{
		apiBind:        defaultDevelopmentAPIBind,
		settingsPath:   filepath.Join(defaultRuntimeDir, defaultSettingsFilename),
		backtestDBPath: filepath.Join(defaultRuntimeDir, defaultBacktestDBFilename),
	}
	if !embeddedFrontend {
		return defaults
	}
	return launchDefaults{
		apiBind:        defaultReleaseAPIBind,
		guiBind:        defaultReleaseGUIBind,
		settingsPath:   filepath.Join(runtimeRoot, defaultSettingsFilename),
		backtestDBPath: filepath.Join(runtimeRoot, defaultBacktestDBFilename),
	}
}

func defaultRuntimeRoot(embeddedFrontend bool, executableDir string) string {
	if !embeddedFrontend {
		return defaultRuntimeDir
	}
	trimmedDir := strings.TrimSpace(executableDir)
	if trimmedDir == "" {
		return defaultRuntimeDir
	}
	return filepath.Join(trimmedDir, defaultRuntimeDir)
}

func resolveExecutableDir() string {
	executablePath, err := os.Executable()
	if err != nil {
		return ""
	}
	trimmedPath := strings.TrimSpace(executablePath)
	if trimmedPath == "" {
		return ""
	}
	if resolvedPath, resolveErr := filepath.EvalSymlinks(trimmedPath); resolveErr == nil && strings.TrimSpace(resolvedPath) != "" {
		trimmedPath = resolvedPath
	}
	return filepath.Dir(trimmedPath)
}

func apiBaseURLForBind(bind string) string {
	host, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return ""
	}
	host = normalizeBrowserHost(host)
	if host == "" || port == "" {
		return ""
	}
	return "http://" + net.JoinHostPort(host, port)
}

func normalizeBrowserHost(host string) string {
	switch strings.TrimSpace(host) {
	case "", "0.0.0.0", "::", "[::]":
		return "127.0.0.1"
	default:
		return strings.TrimSpace(host)
	}
}

func portFromBind(bind string, fallback int) int {
	_, port, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return fallback
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort <= 0 {
		return fallback
	}
	return parsedPort
}
