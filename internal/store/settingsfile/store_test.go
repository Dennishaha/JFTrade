package settingsfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestEnsureBootstrapFileInitializesDefaults(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	defaults := jfsettings.LaunchDefaults{
		APIBind:      runtime.DefaultReleaseAPIBind,
		GUIBind:      runtime.DefaultReleaseGUIBind,
		SettingsPath: settingsPath,
	}
	if err := store.EnsureBootstrapFile(defaults); err != nil {
		t.Fatalf("EnsureBootstrapFile: %v", err)
	}

	if !store.HasAppearance() {
		t.Fatalf("expected bootstrap appearance")
	}
	if got := store.InterfaceSettings(defaults); got.APIBind != runtime.DefaultReleaseAPIBind || got.GUIBind != runtime.DefaultReleaseGUIBind {
		t.Fatalf("InterfaceSettings = %#v", got)
	}
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("settings file not written: %v", err)
	}
}

func TestSettingsPersistenceAndNormalization(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{DefaultTradingEnvironment: "real", SeenFillRetentionDays: 5000}); err != nil {
		t.Fatalf("SaveExecutionSettings: %v", err)
	} else if got.DefaultTradingEnvironment != "REAL" || got.SeenFillRetentionDays != 3650 {
		t.Fatalf("execution normalization = %#v", got)
	}

	if got, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{AdminAuthRequired: true}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	} else if !got.AdminAuthRequired {
		t.Fatalf("security normalization = %#v", got)
	}

	if got, err := store.SaveOnboarding(jfsettings.OnboardingSettings{Completed: false, CompletedAt: "now", LastBrokerID: " futu "}); err != nil {
		t.Fatalf("SaveOnboarding: %v", err)
	} else if got.CompletedAt != "" || got.LastBrokerID != "futu" {
		t.Fatalf("onboarding normalization = %#v", got)
	}

	reloaded, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	if got := reloaded.ExecutionSettings(); got.DefaultTradingEnvironment != "REAL" || got.SeenFillRetentionDays != 3650 {
		t.Fatalf("reloaded execution = %#v", got)
	}
	if got := reloaded.SecuritySettings(); !got.AdminAuthRequired {
		t.Fatalf("reloaded security = %#v", got)
	}
}

func TestSaveIntegrationPersistsWithoutChangingRuntimeEnv(t *testing.T) {
	env := map[string]string{
		"FUTU_OPEND_ADDR":             "existing-address",
		"FUTU_OPEND_WEBSOCKET_KEY":    "existing-opend-key",
		"JFTRADE_FUTU_WEBSOCKET_KEY":  "existing-jftrade-key",
		"JFTRADE_FUTU_API_PORT":       "30001",
		"JFTRADE_FUTU_WEBSOCKET_PORT": "30002",
	}
	for key, value := range env {
		t.Setenv(key, value)
	}

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	input := jfsettings.BrokerIntegration{
		Enabled: true,
		Config: jfsettings.FutuIntegrationConfig{
			Host:          "127.0.0.2",
			APIPort:       22222,
			WebSocketPort: 22223,
			WebSocketKey:  "secret",
		},
	}
	got, err := store.SaveIntegration(input)
	if err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if got.BrokerID != "futu" || got.CreatedAt == "" || got.UpdatedAt == "" {
		t.Fatalf("integration timestamps = %#v", got)
	}
	for key, want := range env {
		if got := os.Getenv(key); got != want {
			t.Fatalf("%s = %q, want unchanged %q", key, got, want)
		}
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("settings json: %v", err)
	}
	if decoded["integration"] == nil {
		t.Fatalf("integration not persisted: %s", string(data))
	}
}

func TestManagedAccountsDefaults(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	account, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{AccountID: " acc-1 "})
	if err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	if account.BrokerID != "futu" || account.TradingEnvironment != "SIMULATE" || account.Market != "HK" || account.DisplayName != "acc-1" {
		t.Fatalf("account defaults = %#v", account)
	}
	if account.ID != "futu|SIMULATE|acc-1|HK" {
		t.Fatalf("account ID = %q", account.ID)
	}
}

func TestCreateManagedAccountRequiresAccountIDAndOwnsServerFields(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{}); err == nil {
		t.Fatal("CreateManagedAccount without accountId succeeded")
	}

	account, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		ID:        "client-id",
		AccountID: "acc-2",
		CreatedAt: "client-created",
		UpdatedAt: "client-updated",
	})
	if err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	if account.ID != "futu|SIMULATE|acc-2|HK" {
		t.Fatalf("account ID = %q, want generated scope id", account.ID)
	}
	if account.CreatedAt == "" || account.CreatedAt == "client-created" {
		t.Fatalf("CreatedAt = %q, want server generated timestamp", account.CreatedAt)
	}
	if account.UpdatedAt == "client-updated" {
		t.Fatalf("UpdatedAt = %q, want server controlled value", account.UpdatedAt)
	}
}
