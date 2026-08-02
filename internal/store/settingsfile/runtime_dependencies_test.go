package settingsfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestRuntimeDependencySettingsPersistNormalizedPythonPath(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	saved, err := store.SaveRuntimeDependencySettings(jftsettings.RuntimeDependencySettings{
		PythonBinaryPath: `  "C:\Program Files\Python311\python.exe"  `,
	})
	if err != nil {
		t.Fatalf("SaveRuntimeDependencySettings: %v", err)
	}
	if saved.PythonBinaryPath != `C:\Program Files\Python311\python.exe` {
		t.Fatalf("saved = %#v", saved)
	}

	reloaded, err := New(settingsPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.RuntimeDependencySettings(); got != saved {
		t.Fatalf("reloaded = %#v, want %#v", got, saved)
	}
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if !strings.Contains(string(raw), `"runtimeDependencies"`) || !strings.Contains(string(raw), `"pythonBinaryPath"`) {
		t.Fatalf("settings JSON = %s", raw)
	}
}

func TestNormalizeRuntimeDependencySettingsTrimsOuterQuotes(t *testing.T) {
	got := NormalizeRuntimeDependencySettings(jftsettings.RuntimeDependencySettings{PythonBinaryPath: ` ' /opt/python/bin/python3 ' `})
	if got.PythonBinaryPath != "/opt/python/bin/python3" {
		t.Fatalf("normalized = %#v", got)
	}
}
