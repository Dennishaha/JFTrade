package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestFailedSettingSavesRollbackAllRuntimeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := store.SaveIntegration(jfsettings.BrokerIntegration{
		Enabled: false,
		Config:  jfsettings.FutuIntegrationConfig{Host: "original-broker-host"},
	}); err != nil {
		t.Fatalf("SaveIntegration original: %v", err)
	}
	if _, err := store.SaveAppearance(jfsettings.UIAppearanceSettings{UpColor: "#112233", DownColor: "#445566"}); err != nil {
		t.Fatalf("SaveAppearance original: %v", err)
	}
	if _, err := store.SaveOnboarding(jfsettings.OnboardingSettings{
		Completed: true, CompletedAt: "2026-01-02T03:04:05Z", LastBrokerID: "futu",
	}); err != nil {
		t.Fatalf("SaveOnboarding original: %v", err)
	}
	if _, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{
		DefaultTradingEnvironment: "REAL", BrokerOrderHistoryLookbackDays: 60, SeenFillRetentionDays: 120,
	}); err != nil {
		t.Fatalf("SaveExecutionSettings original: %v", err)
	}
	if _, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{
		WebAccessEnabled: true, WebPort: 7443, PasswordHash: "original-password-verifier",
	}); err != nil {
		t.Fatalf("SaveSecuritySettings original: %v", err)
	}
	if _, err := store.SaveSystemNotificationSettings(jfsettings.SystemNotificationSettings{
		Enabled: true, Mode: "custom", Levels: []string{"info"}, Categories: []string{"original.category"}, SoundEnabled: true,
	}); err != nil {
		t.Fatalf("SaveSystemNotificationSettings original: %v", err)
	}
	if _, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{
		RunTimeoutMs: 120_000, StreamIdleTimeoutMs: 60_000,
	}); err != nil {
		t.Fatalf("SaveADKSettings original: %v", err)
	}
	if _, err := store.SavePineWorkerSettings(jfsettings.PineWorkerSettings{
		BacktestWorkerLimit: 3, InstanceWorkerLimit: 11, NodeBinaryPath: "/original/node",
	}); err != nil {
		t.Fatalf("SavePineWorkerSettings original: %v", err)
	}
	if _, err := store.SaveMCPServerSettings(jfsettings.MCPServerSettings{
		Port: 6697, AuthMode: "token", TokenHash: "original-token-verifier",
	}); err != nil {
		t.Fatalf("SaveMCPServerSettings original: %v", err)
	}
	calendar := jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:        false,
		ErrorNotificationsEnabled: false,
		RefreshIntervalHours:      48,
		WarmupMarkets:             []string{"US"},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{{
			Market: "US", EnabledSourceIDs: []string{"builtin_rules"}, FallbackToBuiltin: true,
		}},
	}.WithErrorNotificationsEnabledSet(true)
	if _, err := store.SaveExchangeCalendarSettings(calendar); err != nil {
		t.Fatalf("SaveExchangeCalendarSettings original: %v", err)
	}

	wantData := snapshotFileDataForTest(store)
	wantDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile original settings: %v", err)
	}
	replaceErr := errors.New("forced settings replacement failure")
	store.replaceFile = func(string, string) error { return replaceErr }

	tests := []struct {
		name string
		save func() error
	}{
		{
			name: "integration",
			save: func() error {
				_, err := store.SaveIntegration(jfsettings.BrokerIntegration{
					Enabled: true, Config: jfsettings.FutuIntegrationConfig{Host: "replacement-broker-host"},
				})
				return err
			},
		},
		{
			name: "appearance",
			save: func() error {
				_, err := store.SaveAppearance(jfsettings.UIAppearanceSettings{UpColor: "#abcdef", DownColor: "#fedcba"})
				return err
			},
		},
		{
			name: "onboarding",
			save: func() error {
				_, err := store.SaveOnboarding(jfsettings.OnboardingSettings{Completed: false, LastBrokerID: "other"})
				return err
			},
		},
		{
			name: "execution",
			save: func() error {
				_, err := store.SaveExecutionSettings(jfsettings.ExecutionSettings{
					DefaultTradingEnvironment: "SIMULATE", BrokerOrderHistoryLookbackDays: 10, SeenFillRetentionDays: 20,
				})
				return err
			},
		},
		{
			name: "security",
			save: func() error {
				_, err := store.SaveSecuritySettings(jfsettings.SecuritySettings{
					WebAccessEnabled: true, PublicAccessEnabled: true, WebPort: 7555, PasswordHash: "replacement-password-verifier",
				})
				return err
			},
		},
		{
			name: "system notifications",
			save: func() error {
				_, err := store.SaveSystemNotificationSettings(jfsettings.SystemNotificationSettings{Mode: "all"})
				return err
			},
		},
		{
			name: "adk",
			save: func() error {
				_, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{
					RunTimeoutMs: 240_000, StreamIdleTimeoutMs: 90_000,
				})
				return err
			},
		},
		{
			name: "pine worker",
			save: func() error {
				_, err := store.SavePineWorkerSettings(jfsettings.PineWorkerSettings{
					BacktestWorkerLimit: 8, InstanceWorkerLimit: 21, NodeBinaryPath: "/replacement/node",
				})
				return err
			},
		},
		{
			name: "mcp server",
			save: func() error {
				_, err := store.SaveMCPServerSettings(jfsettings.MCPServerSettings{
					Enabled: true, Port: 7666, AuthMode: "none", TokenHash: "replacement-token-verifier",
				})
				return err
			},
		},
		{
			name: "exchange calendar",
			save: func() error {
				_, err := store.SaveExchangeCalendarSettings(jfsettings.ExchangeCalendarSettings{
					AutoRefreshEnabled: true, RefreshIntervalHours: 12, WarmupMarkets: []string{"HK"},
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.save(); !errors.Is(err, replaceErr) {
				t.Fatalf("save error = %v, want %v", err, replaceErr)
			}
			assertFileDataAndDiskUnchanged(t, store, path, wantData, wantDisk)
		})
	}
}

func TestFailedBootstrapAndMigrationRollbackRuntimeState(t *testing.T) {
	t.Run("initial integration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		store, err := New(path)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		replaceErr := errors.New("forced integration replacement failure")
		store.replaceFile = func(string, string) error { return replaceErr }

		_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true})
		if !errors.Is(err, replaceErr) {
			t.Fatalf("SaveIntegration error = %v, want %v", err, replaceErr)
		}
		if got := snapshotFileDataForTest(store); !reflect.DeepEqual(got, fileData{}) {
			t.Fatalf("failed initial integration save changed runtime state: %#v", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("settings file exists after failed integration save: %v", err)
		}
	})

	t.Run("bootstrap", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		store, err := New(path)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		replaceErr := errors.New("forced bootstrap replacement failure")
		store.replaceFile = func(string, string) error { return replaceErr }

		err = store.EnsureBootstrapFile(jfsettings.LaunchDefaults{
			APIBind: "127.0.0.1:3000", GUIBind: "127.0.0.1:3003",
		})
		if !errors.Is(err, replaceErr) {
			t.Fatalf("EnsureBootstrapFile error = %v, want %v", err, replaceErr)
		}
		if got := snapshotFileDataForTest(store); !reflect.DeepEqual(got, fileData{}) {
			t.Fatalf("failed bootstrap changed runtime state: %#v", got)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("settings file exists after failed bootstrap: %v", err)
		}
	})

	t.Run("legacy security migration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "settings.json")
		legacy := []byte(`{"security":{"adminAuthRequired":true}}`)
		if err := os.WriteFile(path, legacy, 0o600); err != nil {
			t.Fatalf("WriteFile legacy settings: %v", err)
		}
		replaceErr := errors.New("forced migration replacement failure")
		store := &Store{
			path:        path,
			replaceFile: func(string, string) error { return replaceErr },
		}

		if err := store.load(); !errors.Is(err, replaceErr) {
			t.Fatalf("load migration error = %v, want %v", err, replaceErr)
		}
		if store.data.Security == nil || store.data.Security.AdminAuthRequired == nil || !*store.data.Security.AdminAuthRequired {
			t.Fatalf("failed migration did not restore legacy runtime state: %#v", store.data.Security)
		}
		gotDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile legacy settings: %v", err)
		}
		if !reflect.DeepEqual(gotDisk, legacy) {
			t.Fatalf("legacy settings changed after failed migration: got %q want %q", gotDisk, legacy)
		}
	})
}

func TestFailedManagedAccountCRUDRollsBackBackingArray(t *testing.T) {
	replaceErr := errors.New("forced account replacement failure")
	tests := []struct {
		name   string
		mutate func(*Store, []jfsettings.ManagedBrokerAccount) error
	}{
		{
			name: "create",
			mutate: func(store *Store, _ []jfsettings.ManagedBrokerAccount) error {
				_, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{AccountID: "account-d", DisplayName: "Delta"})
				return err
			},
		},
		{
			name: "create replaces matching scope",
			mutate: func(store *Store, _ []jfsettings.ManagedBrokerAccount) error {
				_, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{AccountID: "account-a", DisplayName: "Replacement Alpha"})
				return err
			},
		},
		{
			name: "update",
			mutate: func(store *Store, accounts []jfsettings.ManagedBrokerAccount) error {
				_, err := store.UpdateManagedAccount(accounts[1].ID, jfsettings.ManagedBrokerAccount{
					AccountID: "account-b", DisplayName: "Replacement Beta",
				})
				return err
			},
		},
		{
			name: "delete first account",
			mutate: func(store *Store, accounts []jfsettings.ManagedBrokerAccount) error {
				return store.DeleteManagedAccount(accounts[0].ID)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			store, err := New(path)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			for _, account := range []jfsettings.ManagedBrokerAccount{
				{AccountID: "account-a", DisplayName: "Alpha"},
				{AccountID: "account-b", DisplayName: "Beta"},
				{AccountID: "account-c", DisplayName: "Gamma"},
			} {
				if _, err := store.CreateManagedAccount(account); err != nil {
					t.Fatalf("CreateManagedAccount fixture %s: %v", account.AccountID, err)
				}
			}
			wantAccounts := store.ManagedAccounts()
			wantData := snapshotFileDataForTest(store)
			wantDisk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile original settings: %v", err)
			}
			store.replaceFile = func(string, string) error { return replaceErr }

			if err := test.mutate(store, wantAccounts); !errors.Is(err, replaceErr) {
				t.Fatalf("account mutation error = %v, want %v", err, replaceErr)
			}
			assertFileDataAndDiskUnchanged(t, store, path, wantData, wantDisk)
			if got := store.ManagedAccounts(); !reflect.DeepEqual(got, wantAccounts) {
				t.Fatalf("accounts changed after failed mutation:\ngot  = %#v\nwant = %#v", got, wantAccounts)
			}
		})
	}
}

func snapshotFileDataForTest(store *Store) fileData {
	store.mu.RLock()
	defer store.mu.RUnlock()

	snapshot := store.data
	if store.data.Accounts != nil {
		snapshot.Accounts = make([]jfsettings.ManagedBrokerAccount, len(store.data.Accounts))
		copy(snapshot.Accounts, store.data.Accounts)
	}
	return snapshot
}

func assertFileDataAndDiskUnchanged(t *testing.T, store *Store, path string, wantData fileData, wantDisk []byte) {
	t.Helper()
	if got := snapshotFileDataForTest(store); !reflect.DeepEqual(got, wantData) {
		t.Fatalf("runtime state changed after failed persistence:\ngot  = %#v\nwant = %#v", got, wantData)
	}
	gotDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile settings after failed persistence: %v", err)
	}
	if !reflect.DeepEqual(gotDisk, wantDisk) {
		t.Fatalf("settings file changed after failed persistence:\ngot  = %s\nwant = %s", gotDisk, wantDisk)
	}
}
