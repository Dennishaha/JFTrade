package jftradeapi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureBootstrapFilePersistsInterfaceDefaults(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	defaults := launchDefaults{
		apiBind: defaultReleaseAPIBind,
		guiBind: defaultReleaseGUIBind,
	}
	if err := store.ensureBootstrapFile(defaults); err != nil {
		t.Fatalf("ensureBootstrapFile: %v", err)
	}

	rawSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var decoded struct {
		Interfaces  InterfaceSettings    `json:"interfaces"`
		Appearance  UIAppearanceSettings `json:"appearance"`
		Integration *BrokerIntegration   `json:"integration"`
	}
	if err := json.Unmarshal(rawSettings, &decoded); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if decoded.Interfaces.APIBind != defaultReleaseAPIBind {
		t.Fatalf("apiBind = %q", decoded.Interfaces.APIBind)
	}
	if decoded.Interfaces.GUIBind != defaultReleaseGUIBind {
		t.Fatalf("guiBind = %q", decoded.Interfaces.GUIBind)
	}
	if decoded.Interfaces.GUIAPIBaseURL != apiBaseURLForBind(defaultReleaseAPIBind) {
		t.Fatalf("guiApiBaseUrl = %q", decoded.Interfaces.GUIAPIBaseURL)
	}
	if decoded.Appearance.UpColor != "#16c784" || decoded.Appearance.DownColor != "#ea3943" {
		t.Fatalf("appearance settings = %+v", decoded.Appearance)
	}
	if decoded.Integration != nil {
		t.Fatalf("expected bootstrap to avoid persisting integration, got %+v", decoded.Integration)
	}
}

func TestInterfaceSettingsUsesStoredOverride(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settings := `{
  "interfaces": {
    "apiBind": "127.0.0.1:18080",
    "guiBind": "127.0.0.1:18081",
    "guiApiBaseUrl": "http://127.0.0.1:18080"
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0o600); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}

	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	resolved := store.interfaceSettings(launchDefaults{apiBind: defaultReleaseAPIBind, guiBind: defaultReleaseGUIBind})
	if resolved.APIBind != "127.0.0.1:18080" {
		t.Fatalf("apiBind = %q", resolved.APIBind)
	}
	if resolved.GUIBind != "127.0.0.1:18081" {
		t.Fatalf("guiBind = %q", resolved.GUIBind)
	}
	if resolved.GUIAPIBaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("guiApiBaseUrl = %q", resolved.GUIAPIBaseURL)
	}
}
