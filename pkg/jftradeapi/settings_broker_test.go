package jftradeapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

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
