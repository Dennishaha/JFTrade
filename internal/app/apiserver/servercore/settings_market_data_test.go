package servercore

import (
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
