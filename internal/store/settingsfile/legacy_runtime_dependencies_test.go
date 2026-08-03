package settingsfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRuntimeDependencySettingsAreIgnoredAndDroppedOnSave(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	legacy := `{"runtimeDependencies":{"pythonBinaryPath":"/opt/python/bin/python3"}}`
	if err := os.WriteFile(settingsPath, []byte(legacy), 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("load legacy settings: %v", err)
	}
	if _, err := store.SavePineWorkerSettings(DefaultPineWorkerSettings()); err != nil {
		t.Fatalf("save settings after legacy load: %v", err)
	}

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read rewritten settings: %v", err)
	}
	if strings.Contains(string(raw), "runtimeDependencies") || strings.Contains(string(raw), "pythonBinaryPath") {
		t.Fatalf("legacy runtime dependency settings were preserved: %s", raw)
	}
}
