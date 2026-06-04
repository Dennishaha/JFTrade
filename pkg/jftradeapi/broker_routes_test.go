package jftradeapi

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestBrokerFundsEndpointReturnsDisconnectedSummary(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.saveIntegration(BrokerIntegration{Enabled: true, Config: normalizeFutuConfig(FutuIntegrationConfig{
		Type:          "futu",
		Host:          "127.0.0.1",
		APIPort:       1,
		WebSocketPort: 11111,
		TradeMarket:   "HK",
	})})
	if err != nil {
		t.Fatalf("saveIntegration: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := http.Get(srv.URL + "/api/v1/brokers/futu/funds")
	if err != nil {
		t.Fatalf("GET broker funds: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET broker funds status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode broker funds: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := envelope.Data["connectivity"]; got != "disconnected" {
		t.Fatalf("broker funds connectivity = %v", got)
	}
	if _, ok := envelope.Data["currencyBalances"]; !ok {
		t.Fatal("expected currencyBalances in broker funds response")
	}
	if _, ok := envelope.Data["marketAssets"]; !ok {
		t.Fatal("expected marketAssets in broker funds response")
	}
}
