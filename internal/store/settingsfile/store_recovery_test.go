package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
	defaults := jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:3003"}
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
	if _, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{
		WebAccessEnabled: true,
		PasswordHash:     "test-argon2-verifier",
	}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}

	if got := store.Onboarding(); !got.Completed || got.LastBrokerID != "futu" {
		t.Fatalf("persisted onboarding = %#v", got)
	}
	if got := store.ExecutionSettings(); got.DefaultTradingEnvironment != "REAL" || got.SeenFillRetentionDays != 30 {
		t.Fatalf("persisted execution settings = %#v", got)
	}
	if got := store.SecuritySettings(); !got.WebAccessEnabled || !got.PasswordConfigured {
		t.Fatalf("persisted security settings = %#v", got)
	}
	if defaults := DefaultSecuritySettings(); defaults.WebAccessEnabled || defaults.PublicAccessEnabled || defaults.PasswordConfigured || defaults.WebPort != jfsettings.DefaultWebAccessPort {
		t.Fatalf("default security settings must keep Web access disabled: %#v", defaults)
	}
}

func TestLegacyAdminAuthSettingMigratesToDisabledWebAccess(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"security":{"adminAuthRequired":true}}`), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New legacy settings: %v", err)
	}
	if got := store.SecuritySettings(); got.WebAccessEnabled || got.PublicAccessEnabled || got.PasswordConfigured {
		t.Fatalf("legacy setting enabled Web access: %#v", got)
	}
	persisted, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	if strings.Contains(string(persisted), "adminAuthRequired") {
		t.Fatalf("legacy admin auth field was not removed: %s", persisted)
	}
	if mode := fileMode(t, settingsPath); mode != 0o600 {
		t.Fatalf("settings mode = %#o, want 0600", mode)
	}
}

func TestFailedSecurityReplaceKeepsDiskAndRuntimeStateUnchanged(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	original := jfsettings.SecuritySettings{
		WebAccessEnabled: true,
		PasswordHash:     "original-password-verifier",
	}
	if _, err := store.SaveSecuritySettings(original); err != nil {
		t.Fatalf("save original security settings: %v", err)
	}
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read original settings: %v", err)
	}
	store.replaceFile = func(string, string) error { return errors.New("replace failed") }

	updated := jfsettings.SecuritySettings{
		WebAccessEnabled:    true,
		PublicAccessEnabled: true,
		PasswordHash:        "replacement-password-verifier",
	}
	if _, err := store.SaveSecuritySettings(updated); err == nil {
		t.Fatal("SaveSecuritySettings error = nil")
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after failed replace: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("settings changed after failed replace:\nbefore=%s\nafter=%s", before, after)
	}
	if got := store.SecuritySettings(); got.PasswordHash != original.PasswordHash || got.PublicAccessEnabled {
		t.Fatalf("runtime settings changed after failed replace: %#v", got)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(directory, ".settings-*.tmp"))
	if err != nil {
		t.Fatalf("glob temporary settings files: %v", err)
	}
	if len(temporaryFiles) != 0 {
		t.Fatalf("temporary settings files were not cleaned up: %#v", temporaryFiles)
	}
}

func TestFailedMCPServerReplaceKeepsDiskAndRuntimeStateUnchanged(t *testing.T) {
	directory := t.TempDir()
	settingsPath := filepath.Join(directory, "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	original := jfsettings.MCPServerSettings{Port: 6697, AuthMode: "token", TokenHash: "original-token-verifier"}
	if _, err := store.SaveMCPServerSettings(original); err != nil {
		t.Fatalf("save original MCP settings: %v", err)
	}
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read original settings: %v", err)
	}
	store.replaceFile = func(string, string) error { return errors.New("replace failed") }

	if _, err := store.SaveMCPServerSettings(jfsettings.MCPServerSettings{
		Enabled: true, Port: 7443, AuthMode: "none", TokenHash: "replacement-token-verifier",
	}); err == nil {
		t.Fatal("SaveMCPServerSettings error = nil")
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings after failed replace: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("settings changed after failed replace:\nbefore=%s\nafter=%s", before, after)
	}
	if got := store.MCPServerSettings(); got.TokenHash != original.TokenHash || got.Enabled || got.AuthMode != "token" {
		t.Fatalf("runtime MCP settings changed after failed replace: %#v", got)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Mode().Perm()
}
