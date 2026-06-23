package settingsfile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestStoreDefaultsExposePathAndNormalizedDefaults(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if got := store.Path(); got != settingsPath {
		t.Fatalf("Path = %q, want %q", got, settingsPath)
	}

	integration := store.Integration()
	if integration.BrokerID != "futu" || integration.Enabled {
		t.Fatalf("Integration = %#v", integration)
	}
	if integration.Config != DefaultFutuConfig() {
		t.Fatalf("Integration config = %#v", integration.Config)
	}
	if integration.CreatedAt == "" || integration.UpdatedAt == "" {
		t.Fatalf("Integration timestamps = %#v", integration)
	}

	if got := store.Appearance(); got != DefaultUIAppearanceSettings() {
		t.Fatalf("Appearance = %#v", got)
	}
	if got := store.Onboarding(); got != DefaultOnboardingSettings() {
		t.Fatalf("Onboarding = %#v", got)
	}
	if got := store.ADKSettings(); got != DefaultADKRuntimeSettings() {
		t.Fatalf("ADKSettings = %#v", got)
	}
	if got := store.ExchangeCalendarSettings(); !reflect.DeepEqual(got, DefaultExchangeCalendarSettings()) {
		t.Fatalf("ExchangeCalendarSettings = %#v", got)
	}
	if got := store.ManagedAccounts(); len(got) != 0 {
		t.Fatalf("ManagedAccounts length = %d, want 0", len(got))
	}
}

func TestSaveAppearanceAndADKSettingsPersistNormalizedValues(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	appearance, err := store.SaveAppearance(jfsettings.UIAppearanceSettings{
		UpColor:   " #ABCDEF ",
		DownColor: "not-a-color",
	})
	if err != nil {
		t.Fatalf("SaveAppearance: %v", err)
	}
	if want := (jfsettings.UIAppearanceSettings{
		UpColor:   "#abcdef",
		DownColor: DefaultUIAppearanceSettings().DownColor,
	}); appearance != want {
		t.Fatalf("appearance = %#v, want %#v", appearance, want)
	}

	adk, err := store.SaveADKSettings(jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        1,
		StreamIdleTimeoutMs: 9999999,
	})
	if err != nil {
		t.Fatalf("SaveADKSettings: %v", err)
	}
	if want := (jfsettings.ADKRuntimeSettings{
		RunTimeoutMs:        60_000,
		StreamIdleTimeoutMs: 900_000,
	}); adk != want {
		t.Fatalf("adk = %#v, want %#v", adk, want)
	}

	reloaded, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	if got := reloaded.Appearance(); got != appearance {
		t.Fatalf("reloaded appearance = %#v, want %#v", got, appearance)
	}
	if got := reloaded.ADKSettings(); got != adk {
		t.Fatalf("reloaded adk = %#v, want %#v", got, adk)
	}
}

func TestSaveExchangeCalendarSettingsNormalizesPoliciesAndOverrides(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	saved, err := store.SaveExchangeCalendarSettings(jfsettings.ExchangeCalendarSettings{
		AutoRefreshEnabled:   false,
		RefreshIntervalHours: 9999,
		WarmupMarkets:        []string{" us ", "HK", "us", ""},
		SourcePolicies: []jfsettings.ExchangeCalendarSourcePolicy{
			{
				Market:             " hk ",
				PreferredSourceIDs: []string{"hkex_official", "hkex_official", "builtin_rules"},
				EnabledSourceIDs:   []string{" builtin_rules ", "hkex_official", ""},
				FallbackToBuiltin:  true,
				RequireOfficial:    true,
				StaleAfterHours:    -2,
			},
			{
				Market: "",
			},
		},
		ManualOverrides: []jfsettings.ExchangeCalendarManualOverride{
			{
				Market: " us ",
				Date:   " 2026-01-02 ",
				Status: " HOLIDAY ",
				Reason: "  observed holiday  ",
				Sessions: []jfsettings.ExchangeCalendarSessionWindow{
					{Kind: " Pre ", StartMinute: 0, EndMinute: 60},
					{Kind: "", StartMinute: 0, EndMinute: 1},
					{Kind: "regular", StartMinute: 120, EndMinute: 120},
				},
				Observed: true,
			},
			{
				Market: "hk",
				Date:   "",
				Status: "open",
			},
		},
	})
	if err != nil {
		t.Fatalf("SaveExchangeCalendarSettings: %v", err)
	}

	if saved.AutoRefreshEnabled {
		t.Fatalf("AutoRefreshEnabled = true, want false")
	}
	if saved.RefreshIntervalHours != 24*30 {
		t.Fatalf("RefreshIntervalHours = %d, want %d", saved.RefreshIntervalHours, 24*30)
	}
	if !reflect.DeepEqual(saved.WarmupMarkets, []string{"US", "HK"}) {
		t.Fatalf("WarmupMarkets = %#v", saved.WarmupMarkets)
	}
	if len(saved.SourcePolicies) != 1 {
		t.Fatalf("SourcePolicies length = %d, want 1", len(saved.SourcePolicies))
	}
	if want := (jfsettings.ExchangeCalendarSourcePolicy{
		Market:             "HK",
		PreferredSourceIDs: []string{"hk_gov_1823_ical", "builtin_rules"},
		EnabledSourceIDs:   []string{"builtin_rules", "hk_gov_1823_ical"},
		FallbackToBuiltin:  true,
		RequireOfficial:    true,
		StaleAfterHours:    0,
	}); !reflect.DeepEqual(saved.SourcePolicies[0], want) {
		t.Fatalf("SourcePolicy = %#v, want %#v", saved.SourcePolicies[0], want)
	}
	if len(saved.ManualOverrides) != 1 {
		t.Fatalf("ManualOverrides length = %d, want 1", len(saved.ManualOverrides))
	}
	if want := (jfsettings.ExchangeCalendarManualOverride{
		Market:   "US",
		Date:     "2026-01-02",
		Status:   "holiday",
		Sessions: []jfsettings.ExchangeCalendarSessionWindow{{Kind: "pre", StartMinute: 0, EndMinute: 60}},
		Reason:   "observed holiday",
		Observed: true,
	}); !reflect.DeepEqual(saved.ManualOverrides[0], want) {
		t.Fatalf("ManualOverride = %#v, want %#v", saved.ManualOverrides[0], want)
	}

	reloaded, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New reload: %v", err)
	}
	if got := reloaded.ExchangeCalendarSettings(); !reflect.DeepEqual(got, saved) {
		t.Fatalf("reloaded calendars = %#v, want %#v", got, saved)
	}
}

func TestManagedAccountLifecyclePreservesScopeAndHandlesMissingIDs(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := New(settingsPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	securityFirm := " FUTUSECURITIES "
	created, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		BrokerID:           " FUTU ",
		AccountID:          "acc-1",
		DisplayName:        "Primary",
		TradingEnvironment: " real ",
		Market:             " us ",
		SecurityFirm:       &securityFirm,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	if created.ID != "futu|REAL|acc-1|US" {
		t.Fatalf("created ID = %q", created.ID)
	}
	if created.SecurityFirm == nil || *created.SecurityFirm != "FUTUSECURITIES" {
		t.Fatalf("created SecurityFirm = %#v", created.SecurityFirm)
	}

	replaced, err := store.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		AccountID:          "acc-1",
		DisplayName:        "Updated Primary",
		TradingEnvironment: "REAL",
		Market:             "US",
		Enabled:            false,
	})
	if err != nil {
		t.Fatalf("CreateManagedAccount replace: %v", err)
	}
	if replaced.ID != created.ID || replaced.CreatedAt != created.CreatedAt {
		t.Fatalf("replaced = %#v, created = %#v", replaced, created)
	}

	accounts := store.ManagedAccounts()
	if len(accounts) != 1 {
		t.Fatalf("ManagedAccounts length = %d, want 1", len(accounts))
	}
	accounts[0].DisplayName = "mutated copy"
	if got := store.ManagedAccounts()[0].DisplayName; got != "Updated Primary" {
		t.Fatalf("ManagedAccounts leaked internal slice, got %q", got)
	}

	blankFirm := "   "
	updated, err := store.UpdateManagedAccount(created.ID, jfsettings.ManagedBrokerAccount{
		BrokerID:           "futu",
		AccountID:          "acc-1",
		DisplayName:        "",
		TradingEnvironment: "real",
		Market:             "us",
		SecurityFirm:       &blankFirm,
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("UpdateManagedAccount: %v", err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt {
		t.Fatalf("updated identity = %#v, created = %#v", updated, created)
	}
	if updated.DisplayName != "acc-1" || updated.SecurityFirm != nil {
		t.Fatalf("updated normalization = %#v", updated)
	}

	if err := store.DeleteManagedAccount(created.ID); err != nil {
		t.Fatalf("DeleteManagedAccount: %v", err)
	}
	if got := store.ManagedAccounts(); len(got) != 0 {
		t.Fatalf("ManagedAccounts after delete = %#v", got)
	}
	if _, err := store.UpdateManagedAccount(created.ID, jfsettings.ManagedBrokerAccount{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("UpdateManagedAccount missing err = %v, want os.ErrNotExist", err)
	}
	if err := store.DeleteManagedAccount(created.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteManagedAccount missing err = %v, want os.ErrNotExist", err)
	}
}
