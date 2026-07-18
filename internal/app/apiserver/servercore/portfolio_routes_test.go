package servercore

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestPortfolioCashBalancesEndpointReturnsEmptyBalances(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/portfolio/futu/cash-balances")
	if err != nil {
		t.Fatalf("GET portfolio cash balances: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

func TestPortfolioReconciliationEndpointsReturnConnectivityEnvelope(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	tests := []struct {
		name string
		path string
		key  string
	}{
		{
			name: "cash reconciliation",
			path: "/api/v1/portfolio/futu/cash-reconciliation",
			key:  "balances",
		},
		{
			name: "positions reconciliation",
			path: "/api/v1/portfolio/futu/reconciliation",
			key:  "positions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := jftradeTestHTTPGet(t, srv.URL+tt.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tt.path, err)
			}
			defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d", tt.path, resp.StatusCode)
			}

			var envelope struct {
				OK   bool           `json:"ok"`
				Data map[string]any `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode %s: %v", tt.path, err)
			}
			if !envelope.OK {
				t.Fatal("expected ok=true")
			}
			if got := envelope.Data[tt.key]; got == nil {
				t.Fatalf("expected %s in response", tt.key)
			}
			if got := envelope.Data["connectivity"]; got != "disconnected" {
				t.Fatalf("connectivity = %#v, want disconnected", got)
			}
			if got := envelope.Data["checkedAt"]; got == nil || got == "" {
				t.Fatalf("checkedAt = %#v, want timestamp", got)
			}
			if _, ok := envelope.Data["lastError"]; !ok {
				t.Fatal("expected lastError in response")
			}
		})
	}
}

func TestPortfolioRoutesReturnDegradedEmptyStateWithoutConfiguredBroker(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/portfolio/unknown/cash-balances")
	if err != nil {
		t.Fatalf("GET portfolio cash balances: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET portfolio cash balances status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Balances []any `json:"balances"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode portfolio degraded response: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true")
	}
	if len(envelope.Data.Balances) != 0 {
		t.Fatalf("degraded balances = %+v, want empty", envelope.Data.Balances)
	}
}
