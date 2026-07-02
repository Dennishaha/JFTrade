package settingsfile

import (
	"os"
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestSettingsStoreRejectsMalformedOrUnreadableInput(t *testing.T) {
	malformed := filepath.Join(t.TempDir(), "malformed.json")
	if err := os.WriteFile(malformed, []byte(`{"appearance":`), 0o600); err != nil {
		t.Fatalf("write malformed settings: %v", err)
	}
	if _, err := New(malformed); err == nil {
		t.Fatal("New malformed settings err = nil")
	}

	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("New with directory path err = nil")
	}
}

func TestEnsureBootstrapFileRepairsExistingSettingsWithoutAppearance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"onboarding":{"completed":true}}`), 0o600); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if store.HasAppearance() {
		t.Fatal("appearance unexpectedly present before bootstrap repair")
	}
	defaults := jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:5173"}
	if err := store.EnsureBootstrapFile(defaults); err != nil {
		t.Fatalf("EnsureBootstrapFile repair: %v", err)
	}
	if !store.HasAppearance() {
		t.Fatal("appearance was not persisted during bootstrap repair")
	}
	before := store.Appearance()
	if err := store.EnsureBootstrapFile(defaults); err != nil {
		t.Fatalf("EnsureBootstrapFile idempotent: %v", err)
	}
	if after := store.Appearance(); after != before {
		t.Fatalf("idempotent bootstrap changed appearance: before=%#v after=%#v", before, after)
	}
}

func TestSettingsStoreReadsPersistedConfigurationBranches(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := store.SaveOnboarding(jfsettings.OnboardingSettings{Completed: true, LastBrokerID: " futu "}); err != nil {
		t.Fatalf("SaveOnboarding: %v", err)
	}
	if _, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{DefaultTradingEnvironment: "REAL", SeenFillRetentionDays: 30}); err != nil {
		t.Fatalf("SaveExecutionSettings: %v", err)
	}
	if _, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{AdminAuthRequired: true}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}

	if got := store.Onboarding(); !got.Completed || got.LastBrokerID != "futu" {
		t.Fatalf("persisted onboarding = %#v", got)
	}
	if got := store.ExecutionSettings(); got.DefaultTradingEnvironment != "REAL" || got.SeenFillRetentionDays != 30 {
		t.Fatalf("persisted execution settings = %#v", got)
	}
	if got := store.SecuritySettings(); !got.AdminAuthRequired {
		t.Fatalf("persisted security settings = %#v", got)
	}
	if defaults := DefaultSecuritySettings(); defaults.AdminAuthRequired {
		t.Fatalf("default security settings = %#v", defaults)
	}
}
