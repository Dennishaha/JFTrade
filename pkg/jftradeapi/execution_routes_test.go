package jftradeapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestExecutionOrderEndpointsReturnPlaceholders(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/execution/orders")
	if err != nil {
		t.Fatalf("GET execution orders: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET execution orders status = %d", resp.StatusCode)
	}

	var ordersEnvelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ordersEnvelope); err != nil {
		t.Fatalf("decode execution orders: %v", err)
	}
	if !ordersEnvelope.OK {
		t.Fatal("expected orders ok=true")
	}
	if got := ordersEnvelope.Data["orders"]; got == nil {
		t.Fatal("expected orders in execution orders response")
	}

	eventsResp, err := http.Get(srv.URL + "/api/v1/execution/orders/demo-order/events")
	if err != nil {
		t.Fatalf("GET execution order events: %v", err)
	}
	defer eventsResp.Body.Close()
	if eventsResp.StatusCode != http.StatusOK {
		t.Fatalf("GET execution order events status = %d", eventsResp.StatusCode)
	}

	var eventsEnvelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(eventsResp.Body).Decode(&eventsEnvelope); err != nil {
		t.Fatalf("decode execution order events: %v", err)
	}
	if !eventsEnvelope.OK {
		t.Fatal("expected events ok=true")
	}
	if got := eventsEnvelope.Data["events"]; got == nil {
		t.Fatal("expected events in execution order events response")
	}
	if got := eventsEnvelope.Data["internalOrderId"]; got != "" {
		t.Fatalf("internalOrderId = %v, want empty string", got)
	}
}
