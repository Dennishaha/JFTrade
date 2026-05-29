package jftradeapi

import (
	"path/filepath"
	"testing"
)

func TestLaunchDefaultsForDevelopmentMode(t *testing.T) {
	defaults := launchDefaultsForExecutableDir(false, filepath.Join(t.TempDir(), "ignored"))

	if defaults.apiBind != defaultDevelopmentAPIBind {
		t.Fatalf("apiBind = %q, want %q", defaults.apiBind, defaultDevelopmentAPIBind)
	}
	if defaults.guiBind != "" {
		t.Fatalf("guiBind = %q, want empty", defaults.guiBind)
	}
	if defaults.settingsPath != filepath.Join(defaultRuntimeDir, defaultSettingsFilename) {
		t.Fatalf("settingsPath = %q", defaults.settingsPath)
	}
	if defaults.backtestDBPath != filepath.Join(defaultRuntimeDir, defaultBacktestDBFilename) {
		t.Fatalf("backtestDBPath = %q", defaults.backtestDBPath)
	}
}

func TestLaunchDefaultsForEmbeddedFrontendMode(t *testing.T) {
	executableDir := filepath.Join(t.TempDir(), "release")
	defaults := launchDefaultsForExecutableDir(true, executableDir)

	if defaults.apiBind != defaultReleaseAPIBind {
		t.Fatalf("apiBind = %q, want %q", defaults.apiBind, defaultReleaseAPIBind)
	}
	if defaults.guiBind != defaultReleaseGUIBind {
		t.Fatalf("guiBind = %q, want %q", defaults.guiBind, defaultReleaseGUIBind)
	}
	wantSettingsPath := filepath.Join(executableDir, defaultRuntimeDir, defaultSettingsFilename)
	if defaults.settingsPath != wantSettingsPath {
		t.Fatalf("settingsPath = %q, want %q", defaults.settingsPath, wantSettingsPath)
	}
	wantBacktestPath := filepath.Join(executableDir, defaultRuntimeDir, defaultBacktestDBFilename)
	if defaults.backtestDBPath != wantBacktestPath {
		t.Fatalf("backtestDBPath = %q, want %q", defaults.backtestDBPath, wantBacktestPath)
	}
}

func TestAPIBaseURLForBindNormalizesWildcardHost(t *testing.T) {
	if got := apiBaseURLForBind("0.0.0.0:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("apiBaseURLForBind() = %q", got)
	}
}
