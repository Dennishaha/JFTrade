package servercore

import (
	"path/filepath"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func TestBrokerReadQueryDefaultMarket(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: settingsfile.NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "US",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newTestServer(t, store)
	query := srv.tradingSvc.ReadQuery("futu", "REAL", "123", "")
	if query.Market != "US" {
		t.Errorf("expected market US (from settings), got %q", query.Market)
	}
	if query.AccountID != "123" {
		t.Errorf("expected accountId 123, got %q", query.AccountID)
	}
	if query.TradingEnvironment != "REAL" {
		t.Errorf("expected SIMULATE when empty, got %q", query.TradingEnvironment)
	}
}
