package servercore

import (
	"path/filepath"
	"testing"
)

func TestLaunchDefaultsForDevelopmentMode(t *testing.T) {
	defaults := LaunchDefaultsForExecutableDir(false, filepath.Join(t.TempDir(), "ignored"))

	if defaults.APIBind != defaultDevelopmentAPIBind {
		t.Fatalf("APIBind = %q, want %q", defaults.APIBind, defaultDevelopmentAPIBind)
	}
	if defaults.GUIBind != "" {
		t.Fatalf("GUIBind = %q, want empty", defaults.GUIBind)
	}
	if defaults.SettingsPath != filepath.Join(defaultRuntimeDir, defaultSettingsFilename) {
		t.Fatalf("SettingsPath = %q", defaults.SettingsPath)
	}
	if defaults.BacktestDBPath != filepath.Join(defaultRuntimeDir, defaultBacktestDBFilename) {
		t.Fatalf("BacktestDBPath = %q", defaults.BacktestDBPath)
	}
}

func TestLaunchDefaultsForEmbeddedFrontendMode(t *testing.T) {
	executableDir := filepath.Join(t.TempDir(), "release")
	defaults := LaunchDefaultsForExecutableDir(true, executableDir)

	if defaults.APIBind != defaultReleaseAPIBind {
		t.Fatalf("APIBind = %q, want %q", defaults.APIBind, defaultReleaseAPIBind)
	}
	if defaults.GUIBind != defaultReleaseGUIBind {
		t.Fatalf("GUIBind = %q, want %q", defaults.GUIBind, defaultReleaseGUIBind)
	}
	wantSettingsPath := filepath.Join(executableDir, defaultRuntimeDir, defaultSettingsFilename)
	if defaults.SettingsPath != wantSettingsPath {
		t.Fatalf("SettingsPath = %q, want %q", defaults.SettingsPath, wantSettingsPath)
	}
	wantBacktestPath := filepath.Join(executableDir, defaultRuntimeDir, defaultBacktestDBFilename)
	if defaults.BacktestDBPath != wantBacktestPath {
		t.Fatalf("BacktestDBPath = %q, want %q", defaults.BacktestDBPath, wantBacktestPath)
	}
}

func TestAPIBaseURLForBindNormalizesWildcardHost(t *testing.T) {
	if got := apiBaseURLForBind("0.0.0.0:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("apiBaseURLForBind() = %q", got)
	}
}
