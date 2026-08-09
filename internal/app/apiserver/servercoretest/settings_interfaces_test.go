package servercoretest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	apiruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestEnsureBootstrapFilePersistsInterfaceDefaults(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	defaults := jfsettings.LaunchDefaults{
		APIBind: apiruntime.DefaultReleaseAPIBind,
		GUIBind: apiruntime.DefaultReleaseGUIBind,
	}
	if err := store.EnsureBootstrapFile(defaults); err != nil {
		t.Fatalf("ensureBootstrapFile: %v", err)
	}

	rawSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var decoded struct {
		Interfaces  jfsettings.InterfaceSettings    `json:"interfaces"`
		Appearance  jfsettings.UIAppearanceSettings `json:"appearance"`
		Integration *jfsettings.BrokerIntegration   `json:"integration"`
	}
	if err := json.Unmarshal(rawSettings, &decoded); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if decoded.Interfaces.APIBind != apiruntime.DefaultReleaseAPIBind {
		t.Fatalf("apiBind = %q", decoded.Interfaces.APIBind)
	}
	if decoded.Interfaces.GUIBind != apiruntime.DefaultReleaseGUIBind {
		t.Fatalf("guiBind = %q", decoded.Interfaces.GUIBind)
	}
	if decoded.Interfaces.LiveWebSocketConnectionLimit != 20 {
		t.Fatalf("liveWebSocketConnectionLimit = %d", decoded.Interfaces.LiveWebSocketConnectionLimit)
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
    "liveWebSocketConnectionLimit": 12
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0o600); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}

	store, err := servercore.NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}

	resolved := store.InterfaceSettings(jfsettings.LaunchDefaults{APIBind: apiruntime.DefaultReleaseAPIBind, GUIBind: apiruntime.DefaultReleaseGUIBind})
	if resolved.APIBind != "127.0.0.1:18080" {
		t.Fatalf("apiBind = %q", resolved.APIBind)
	}
	if resolved.GUIBind != "127.0.0.1:18081" {
		t.Fatalf("guiBind = %q", resolved.GUIBind)
	}
	if resolved.LiveWebSocketConnectionLimit != 12 {
		t.Fatalf("liveWebSocketConnectionLimit = %d", resolved.LiveWebSocketConnectionLimit)
	}
}
