package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func TestBrokerIntegrationSavePersistsAndUpdatesRuntimeEnv(t *testing.T) {
	t.Setenv(futu.EnvOpenDAddr, "")
	t.Setenv("FUTU_OPEND_WEBSOCKET_KEY", "")
	t.Setenv("JFTRADE_FUTU_WEBSOCKET_KEY", "")

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload := map[string]any{
		"enabled": true,
		"config": map[string]any{
			"type":                    "futu",
			"host":                    "127.0.0.1",
			"apiPort":                 11110,
			"websocketPort":           11111,
			"maxWebSocketConnections": 20,
			"useEncryption":           false,
			"websocketKey":            "123456",
			"tradeMarket":             "HK",
			"securityFirm":            "FUTUSECURITIES",
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings/brokers/futu/integration", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT integration: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	if got := os.Getenv(futu.EnvOpenDAddr); got != "127.0.0.1:11110" {
		t.Fatalf("%s = %q", futu.EnvOpenDAddr, got)
	}
	if got := os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY"); got != "123456" {
		t.Fatalf("JFTRADE_FUTU_WEBSOCKET_KEY = %q", got)
	}

	resp, err = http.Get(srv.URL + "/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET settings: %v", err)
	}
	defer resp.Body.Close()

	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Brokers []struct {
				Integration BrokerIntegration `json:"integration"`
			} `json:"brokers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if !response.OK || len(response.Data.Brokers) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	config := response.Data.Brokers[0].Integration.Config
	if config.APIPort != 11110 || config.WebSocketPort != 11111 || config.WebSocketKey != "123456" {
		t.Fatalf("unexpected saved config: %+v", config)
	}
}

func TestLiveWebSocketSendsHeartbeat(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/ws/live"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial live websocket: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if event["type"] != "heartbeat" || event["at"] == "" {
		t.Fatalf("unexpected event: %+v", event)
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

func TestManagedBrokerAccountCRUDReflectsInBrokerSettings(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := httptest.NewServer(NewServer(store))
	defer srv.Close()

	payload := map[string]any{
		"brokerId":           "futu",
		"accountId":          "12345678",
		"displayName":        "Main Sim",
		"tradingEnvironment": "SIMULATE",
		"market":             "HK",
		"securityFirm":       "FUTUSECURITIES",
		"enabled":            true,
	}
	body, _ := json.Marshal(payload)
	resp, err := http.Post(srv.URL+"/api/v1/settings/broker-accounts", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST managed account: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST managed account status = %d", resp.StatusCode)
	}

	var createEnvelope struct {
		OK   bool                 `json:"ok"`
		Data ManagedBrokerAccount `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&createEnvelope); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if !createEnvelope.OK || createEnvelope.Data.ID == "" {
		t.Fatalf("unexpected create response: %+v", createEnvelope)
	}

	resp, err = http.Get(srv.URL + "/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings: %v", err)
	}
	defer resp.Body.Close()
	var settingsEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Accounts []ManagedBrokerAccount `json:"accounts"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&settingsEnvelope); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if !settingsEnvelope.OK || len(settingsEnvelope.Data.Accounts) != 1 {
		t.Fatalf("unexpected broker settings after create: %+v", settingsEnvelope)
	}
	if settingsEnvelope.Data.Accounts[0].AccountID != "12345678" {
		t.Fatalf("unexpected account: %+v", settingsEnvelope.Data.Accounts[0])
	}

	updatedPayload := map[string]any{
		"brokerId":           "futu",
		"accountId":          "12345678",
		"displayName":        "Primary Sim",
		"tradingEnvironment": "SIMULATE",
		"market":             "HK",
		"securityFirm":       "FUTUSECURITIES",
		"enabled":            true,
	}
	body, _ = json.Marshal(updatedPayload)
	req, err := http.NewRequest(http.MethodPut, srv.URL+"/api/v1/settings/broker-accounts/"+url.PathEscape(createEnvelope.Data.ID), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest update managed account: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT managed account: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT managed account status = %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings after update: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&settingsEnvelope); err != nil {
		t.Fatalf("decode settings after update: %v", err)
	}
	if got := settingsEnvelope.Data.Accounts[0].DisplayName; got != "Primary Sim" {
		t.Fatalf("unexpected displayName after update = %q", got)
	}

	req, err = http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/settings/broker-accounts/"+url.PathEscape(createEnvelope.Data.ID), nil)
	if err != nil {
		t.Fatalf("NewRequest delete managed account: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE managed account: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE managed account status = %d", resp.StatusCode)
	}

	resp, err = http.Get(srv.URL + "/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings after delete: %v", err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&settingsEnvelope); err != nil {
		t.Fatalf("decode settings after delete: %v", err)
	}
	if len(settingsEnvelope.Data.Accounts) != 0 {
		t.Fatalf("expected zero accounts after delete: %+v", settingsEnvelope.Data.Accounts)
	}
}
