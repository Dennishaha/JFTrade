package servercore

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
)

func TestMarketDataSubscriptionHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	postJSON := func(path string, payload map[string]any) map[string]any {
		body, jftradeErr1 := json.Marshal(payload)
		jftradeCheckTestError(t, jftradeErr1)
		resp, err := jftradeTestHTTPPost(t, srv.URL+path, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"channel": "KLINE", "market": "HK", "symbol": "00700", "interval": "1m"},
		},
	})
	if got := int(jftradeCheckedTypeAssertion[float64](data["totalActiveSubscriptions"])); got != 1 {
		t.Fatalf("totalActiveSubscriptions after acquire = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/heartbeat", map[string]any{"consumerId": "chart-main"})
	if got := int(jftradeCheckedTypeAssertion[float64](data["totalActiveSubscriptions"])); got != 1 {
		t.Fatalf("totalActiveSubscriptions after heartbeat = %d", got)
	}

	data = postJSON("/api/v1/market-data/subscriptions/release", map[string]any{
		"consumerId": "chart-main",
		"instruments": []any{
			map[string]any{"channel": "KLINE", "market": "HK", "symbol": "00700", "interval": "1m"},
		},
	})
	if released := jftradeCheckedTypeAssertion[bool](data["released"]); !released {
		t.Fatal("expected released=true after release")
	}

	// Verify subscriptions are cleared via GET.
	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/market-data/subscriptions")
	if err != nil {
		t.Fatalf("GET subscriptions: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	var getEnv struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getEnv); err != nil {
		t.Fatalf("decode subscriptions: %v", err)
	}
	if got := int(jftradeCheckedTypeAssertion[float64](getEnv.Data["totalActiveSubscriptions"])); got != 0 {
		t.Fatalf("totalActiveSubscriptions after release = %d", got)
	}
}
