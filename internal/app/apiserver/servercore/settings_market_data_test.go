package servercore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
)

func TestServerSettingsStoreDefaultsAndPersistsMarketDataSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(path)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if store.ActiveMarketDataProvider() != jfsettings.MarketDataProviderYFinance {
		t.Fatalf("default provider = %q", store.ActiveMarketDataProvider())
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderFutu); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
	reloaded, err := NewSettingsStore(path)
	if err != nil {
		t.Fatalf("reload settings store: %v", err)
	}
	if got := reloaded.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderFutu {
		t.Fatalf("reloaded provider = %q", got)
	}
}

func TestSettingsStoreMigratesLegacyYFinanceConnectionBlockToFutu(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	legacy := map[string]any{
		"yfinance": map[string]any{
			"enabled": true, "host": "127.0.0.1", "port": 7788,
			"pythonBin": "/opt/python", "timeoutSeconds": 15,
		},
		"activeMarketDataProvider": "yfinance",
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy settings: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}
	store, err := NewSettingsStore(path)
	if err != nil {
		t.Fatalf("NewSettingsStore legacy: %v", err)
	}
	if got := store.ActiveMarketDataProvider(); got != jfsettings.MarketDataProviderFutu {
		t.Fatalf("migrated provider = %q", got)
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated settings: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(persisted, &decoded); err != nil {
		t.Fatalf("decode migrated settings: %v", err)
	}
	if _, ok := decoded["yfinance"]; ok {
		t.Fatalf("legacy yfinance block was not removed: %s", persisted)
	}
	if string(decoded["activeMarketDataProvider"]) != `"futu"` {
		t.Fatalf("migrated active provider = %s", decoded["activeMarketDataProvider"])
	}
}

func TestStartupPersistsFutuFallbackWhenEmbeddedYFinanceCannotActivate(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if err := store.SaveActiveMarketDataProvider(jfsettings.MarketDataProviderYFinance); err != nil {
		t.Fatalf("SaveActiveMarketDataProvider: %v", err)
	}
	server := newTestServer(t, store)
	if active := marketdataapp.RuntimeFromService(server.marketdataSvc).ActiveProviderID(); active != marketdataapp.ProviderFutu {
		t.Fatalf("runtime provider = %q", active)
	}
	if persisted := store.ActiveMarketDataProvider(); persisted != jfsettings.MarketDataProviderFutu {
		t.Fatalf("persisted provider = %q", persisted)
	}
}
