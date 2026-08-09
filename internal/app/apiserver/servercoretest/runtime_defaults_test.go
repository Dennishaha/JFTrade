package servercoretest

import (
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
)

func TestLaunchDefaultsForDevelopmentMode(t *testing.T) {
	defaults := servercore.LaunchDefaultsForExecutableDir(false, filepath.Join(t.TempDir(), "ignored"))

	if defaults.APIBind != apiruntime.DefaultDevelopmentAPIBind {
		t.Fatalf("APIBind = %q, want %q", defaults.APIBind, apiruntime.DefaultDevelopmentAPIBind)
	}
	if defaults.GUIBind != "" {
		t.Fatalf("GUIBind = %q, want empty", defaults.GUIBind)
	}
	if defaults.SettingsPath != filepath.Join(apiruntime.DefaultRuntimeDir, apiruntime.DefaultSettingsFilename) {
		t.Fatalf("SettingsPath = %q", defaults.SettingsPath)
	}
	if defaults.BacktestDBPath != filepath.Join(apiruntime.DefaultRuntimeDir, apiruntime.DefaultBacktestDBFilename) {
		t.Fatalf("BacktestDBPath = %q", defaults.BacktestDBPath)
	}
}

func TestLaunchDefaultsForEmbeddedFrontendMode(t *testing.T) {
	executableDir := filepath.Join(t.TempDir(), "release")
	defaults := servercore.LaunchDefaultsForExecutableDir(true, executableDir)

	if defaults.APIBind != apiruntime.DefaultReleaseAPIBind {
		t.Fatalf("APIBind = %q, want %q", defaults.APIBind, apiruntime.DefaultReleaseAPIBind)
	}
	if defaults.GUIBind != apiruntime.DefaultReleaseGUIBind {
		t.Fatalf("GUIBind = %q, want %q", defaults.GUIBind, apiruntime.DefaultReleaseGUIBind)
	}
	wantSettingsPath := filepath.Join(executableDir, apiruntime.DefaultRuntimeDir, apiruntime.DefaultSettingsFilename)
	if defaults.SettingsPath != wantSettingsPath {
		t.Fatalf("SettingsPath = %q, want %q", defaults.SettingsPath, wantSettingsPath)
	}
	wantBacktestPath := filepath.Join(executableDir, apiruntime.DefaultRuntimeDir, apiruntime.DefaultBacktestDBFilename)
	if defaults.BacktestDBPath != wantBacktestPath {
		t.Fatalf("BacktestDBPath = %q, want %q", defaults.BacktestDBPath, wantBacktestPath)
	}
}

func TestAPIBaseURLForBindNormalizesWildcardHost(t *testing.T) {
	if got := servercore.APIBaseURLForBind("0.0.0.0:6699"); got != "http://127.0.0.1:6699" {
		t.Fatalf("servercore.APIBaseURLForBind() = %q", got)
	}
}
