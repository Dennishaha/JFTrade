package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestShouldStartForAPIOnlyArgs(t *testing.T) {
	if !shouldStartForArgs([]string{"api"}) {
		t.Fatal("expected api command to start JFTrade sidecar")
	}
	if !shouldStartForArgs([]string{"serve-api"}) {
		t.Fatal("expected serve-api command to start JFTrade sidecar")
	}
	if !shouldStartForArgs([]string{"run", "--config", "./config/jftrade.yaml"}) {
		t.Fatal("expected bbgo run command to start JFTrade sidecar")
	}
}

func TestMarketDataSubscriptionHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	postJSON := func(path string, payload map[string]any) map[string]any {
		body, _ := json.Marshal(payload)
		resp, err := http.Post(srv.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("POST %s status = %d", path, resp.StatusCode)
		}
		var envelope struct {
			OK   bool           `json:"ok"`
			Data map[string]any `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if !envelope.OK {
			t.Fatalf("POST %s returned ok=false", path)
		}
		return envelope.Data
	}

	data := postJSON("/api/v1/market-data/subscriptions", map[string]any{
		"channel":    "KLINE",
		"market":     "HK",
		"symbol":     "00700",
		"interval":   "1m",
		"consumerId": "chart-main",
	})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 1 {
		t.Fatalf("totalActiveSubscriptions after acquire = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/heartbeat", map[string]any{"consumerId": "chart-main"})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 1 {
		t.Fatalf("totalActiveSubscriptions after heartbeat = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/release", map[string]any{
		"channel":    "KLINE",
		"market":     "HK",
		"symbol":     "00700",
		"interval":   "1m",
		"consumerId": "chart-main",
	})
	if got := int(data["totalActiveSubscriptions"].(float64)); got != 0 {
		t.Fatalf("totalActiveSubscriptions after release = %d", got)
	}
}

func TestStrategiesEndpointReturnsList(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/strategies")
	if err != nil {
		t.Fatalf("GET strategies: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET strategies status = %d", resp.StatusCode)
	}
	var envelope struct {
		OK   bool  `json:"ok"`
		Data []any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode strategies: %v", err)
	}
	if !envelope.OK || envelope.Data == nil {
		t.Fatalf("unexpected strategies response: %+v", envelope)
	}
}
