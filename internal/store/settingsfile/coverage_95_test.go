package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestSettingsNormalizationCoverageForFallbackAndBoundaries(t *testing.T) {
	execution := NormalizeExecutionSettings(jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      "unsupported",
		BrokerOrderHistoryLookbackDays: -1,
		SeenFillRetentionDays:          5001,
	})
	if execution.DefaultTradingEnvironment != "SIMULATE" || execution.BrokerOrderHistoryLookbackDays != 30 || execution.SeenFillRetentionDays != 3650 {
		t.Fatalf("NormalizeExecutionSettings = %#v", execution)
	}

	allNotifications := NormalizeSystemNotificationSettings(jfsettings.SystemNotificationSettings{
		Mode: " all ", Levels: []string{"warn"}, Categories: []string{"broker.connection"},
	})
	if allNotifications.Mode != "all" || allNotifications.Levels != nil || allNotifications.Categories != nil {
		t.Fatalf("NormalizeSystemNotificationSettings(all) = %#v", allNotifications)
	}
	customNotifications := NormalizeSystemNotificationSettings(jfsettings.SystemNotificationSettings{
		Mode: "custom", Levels: []string{" WARN ", "warn", ""}, Categories: []string{" broker ", "broker"},
	})
	if !reflect.DeepEqual(customNotifications.Levels, []string{"warn"}) || !reflect.DeepEqual(customNotifications.Categories, []string{"broker"}) {
		t.Fatalf("NormalizeSystemNotificationSettings(custom) = %#v", customNotifications)
	}
	if normalizeSystemNotificationMode("unknown") != "" {
		t.Fatal("unknown notification mode should normalize to empty")
	}

	worker := NormalizePineWorkerSettings(jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: -1, InstanceWorkerLimit: 2000, NodeBinaryPath: ` "'node'" `,
	})
	if worker.BacktestWorkerLimit != 1 || worker.InstanceWorkerLimit != 1000 || worker.NodeBinaryPath != "node" {
		t.Fatalf("NormalizePineWorkerSettings = %#v", worker)
	}
	if got := clampOrDefaultInt(1, 20, 5, 30); got != 5 {
		t.Fatalf("clampOrDefaultInt below minimum = %d", got)
	}
	if got := normalizeHexColor("#12xz34", "#abcdef"); got != "#abcdef" {
		t.Fatalf("normalizeHexColor invalid = %q", got)
	}
}

func TestSettingsInterfaceAndAccountNormalizationCoverage(t *testing.T) {
	defaults := jfsettings.LaunchDefaults{APIBind: "0.0.0.0:3000", GUIBind: "127.0.0.1:3003"}
	fromDefaults := InterfaceSettingsFromDefaults(defaults)
	if fromDefaults.GUIAPIBaseURL != "http://127.0.0.1:3000" {
		t.Fatalf("InterfaceSettingsFromDefaults = %#v", fromDefaults)
	}
	if got := NormalizeInterfaceSettings(jfsettings.InterfaceSettings{APIBind: "invalid"}, jfsettings.LaunchDefaults{}); got.APIBind != "invalid" || got.GUIAPIBaseURL != "" {
		t.Fatalf("NormalizeInterfaceSettings invalid bind = %#v", got)
	}
	if got := normalizeBrowserHost("[::]"); got != "127.0.0.1" {
		t.Fatalf("normalizeBrowserHost IPv6 wildcard = %q", got)
	}

	config := NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{})
	if config.Host != defaultFutuHost || config.APIPort != defaultFutuAPIPort || config.WebSocketPort != defaultFutuWebSocketPort || config.MaxWebSocketConnections != defaultMaxWebSocketClients || config.UseEncryption {
		t.Fatalf("NormalizeFutuConfig = %#v", config)
	}
	blankFirm := "  "
	account := NormalizeManagedBrokerAccount(jfsettings.ManagedBrokerAccount{AccountID: " account ", SecurityFirm: &blankFirm})
	if account.SecurityFirm != nil || account.DisplayName != "account" || account.BrokerID != "futu" || account.Market != "HK" {
		t.Fatalf("NormalizeManagedBrokerAccount = %#v", account)
	}
}

func TestSettingsPersistenceReportsAtomicReplaceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	replaceErr := errors.New("atomic replacement failed")
	store.replaceFile = func(string, string) error { return replaceErr }
	if _, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{}); !errors.Is(err, replaceErr) {
		t.Fatalf("SaveADKSettings replacement error = %v", err)
	}

	store.replaceFile = nil
	if _, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{}); err != nil {
		t.Fatalf("SaveADKSettings default replacement error = %v", err)
	}
}

func TestSettingsPersistenceRejectsAFileInItsDirectoryPath(t *testing.T) {
	root := t.TempDir()
	parentFile := filepath.Join(root, "settings-parent-file")
	if err := os.WriteFile(parentFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile parent: %v", err)
	}

	// A launcher can be given a path whose parent was replaced by a regular
	// file after startup. Persistence must return the operating-system error and
	// retain the in-memory value instead of claiming that settings were saved.
	store := &Store{path: filepath.Join(parentFile, "settings.json"), replaceFile: replaceFile}
	if _, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{RunTimeoutMs: 120_000}); err == nil {
		t.Fatal("SaveADKSettings accepted a settings path below a regular file")
	}
	if got := store.ADKSettings(); got != DefaultADKRuntimeSettings() {
		t.Fatalf("failed persistence changed runtime ADK settings: %#v", got)
	}
	if err := store.EnsureBootstrapFile(jfsettings.LaunchDefaults{}); err == nil {
		t.Fatal("EnsureBootstrapFile accepted a settings path below a regular file")
	}
}

func TestSettingsStoreCoverageForPersistedValuesAndAccountLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := store.ExecutionSettings(); got != DefaultExecutionSettings() {
		t.Fatalf("default execution settings = %#v", got)
	}
	if got := store.SecuritySettings(); got != DefaultSecuritySettings() {
		t.Fatalf("default security settings = %#v", got)
	}
	if _, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{
		DefaultTradingEnvironment:      "REAL",
		BrokerOrderHistoryLookbackDays: 500,
		SeenFillRetentionDays:          0,
	}); err != nil {
		t.Fatalf("SaveExecutionSettings: %v", err)
	}
	if got := store.ExecutionSettings(); got.BrokerOrderHistoryLookbackDays != 365 || got.SeenFillRetentionDays != 90 {
		t.Fatalf("persisted execution normalization = %#v", got)
	}
	if _, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{WebAccessEnabled: true, PasswordHash: "hash"}); err != nil {
		t.Fatalf("SaveSecuritySettings: %v", err)
	}
	if got := store.SecuritySettings(); !got.WebAccessEnabled || !got.PasswordConfigured {
		t.Fatalf("persisted security settings = %#v", got)
	}

	first, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{AccountID: "account-1"})
	if err != nil {
		t.Fatalf("CreateManagedAccount first: %v", err)
	}
	updated, err := store.UpdateManagedAccount(first.ID, jfsettings.ManagedBrokerAccount{AccountID: "account-1", DisplayName: "Primary"})
	if err != nil || updated.ID != first.ID || updated.DisplayName != "Primary" {
		t.Fatalf("UpdateManagedAccount = %#v, %v", updated, err)
	}
	if err := store.DeleteManagedAccount(first.ID); err != nil {
		t.Fatalf("DeleteManagedAccount: %v", err)
	}
	if _, err := store.UpdateManagedAccount("missing", jfsettings.ManagedBrokerAccount{AccountID: "missing"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UpdateManagedAccount missing error = %v", err)
	}
	if err := store.DeleteManagedAccount("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteManagedAccount missing error = %v", err)
	}

	firstIntegration, err := store.SaveIntegration(jfsettings.BrokerIntegration{})
	if err != nil {
		t.Fatalf("SaveIntegration first: %v", err)
	}
	secondIntegration, err := store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true})
	if err != nil || secondIntegration.CreatedAt != firstIntegration.CreatedAt {
		t.Fatalf("SaveIntegration preserves creation time = %#v, %v", secondIntegration, err)
	}
	saved := store.SavedIntegration()
	if saved == nil || saved.CreatedAt != firstIntegration.CreatedAt {
		t.Fatalf("SavedIntegration = %#v", saved)
	}
	if got := store.Integration(); !got.Enabled || got.BrokerID != "futu" {
		t.Fatalf("persisted integration = %#v", got)
	}
}

func TestSettingsFileCoverageForInterfaceAndLoadFailures(t *testing.T) {
	if got := InterfaceSettingsFromDefaults(jfsettings.LaunchDefaults{APIBind: "127.0.0.1:3000"}); got.GUIBind != "" || got.GUIAPIBaseURL != "" {
		t.Fatalf("InterfaceSettingsFromDefaults without GUI = %#v", got)
	}
	if got := apiBaseURLForBind("not-a-bind"); got != "" {
		t.Fatalf("apiBaseURLForBind invalid = %q", got)
	}
	if got := apiBaseURLForBind(":3000"); got != "http://127.0.0.1:3000" {
		t.Fatalf("apiBaseURLForBind wildcard = %q", got)
	}
	if got := NormalizeMCPServerSettings(jfsettings.MCPServerSettings{Port: 80, AuthMode: "other"}); got.Port != jfsettings.DefaultMCPServerPort || got.AuthMode != "token" {
		t.Fatalf("NormalizeMCPServerSettings invalid = %#v", got)
	}
	if got := NormalizeSystemNotificationSettings(jfsettings.SystemNotificationSettings{Mode: "other"}); got.Mode != "important" {
		t.Fatalf("invalid notification mode = %#v", got)
	}

	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(emptyPath, []byte(" \n"), 0o600); err != nil {
		t.Fatalf("write empty settings: %v", err)
	}
	if _, err := New(emptyPath); err != nil {
		t.Fatalf("New empty settings: %v", err)
	}
	if err := syncSettingsDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("syncSettingsDirectory accepted a missing directory")
	}
}
