package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestPortfolioCashBalancesEndpointReturnsEmptyBalances(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/portfolio/main/cash-balances")
	if err != nil {
		t.Fatalf("GET portfolio cash balances: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET portfolio cash balances status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode portfolio cash balances: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if got := envelope.Data["balances"]; got == nil {
		t.Fatal("expected balances in portfolio cash balances response")
	}
}
