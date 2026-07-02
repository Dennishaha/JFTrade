package servercore

import (
	"bytes"
	"context"
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
	api := newTestServer(t, store)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	payload := map[string]any{
		"enabled": true,
		"config": map[string]any{
			"type":                    "futu",
			"host":                    "127.0.0.1",
			"apiPort":                 11110,
			"websocketPort":           11111,
			"maxWebSocketConnections": 20,
			"useEncryption":           true,
			"websocketKey":            "123456",
			"tradeMarket":             "HK",
			"securityFirm":            "FUTUSECURITIES",
		},
	}
	body, jftradeErr1 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr1)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/brokers/futu/integration", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT integration: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d", resp.StatusCode)
	}

	if got := os.Getenv(futu.EnvOpenDAddr); got != "127.0.0.1:11110" {
		t.Fatalf("%s = %q", futu.EnvOpenDAddr, got)
	}
	if got := os.Getenv("JFTRADE_FUTU_WEBSOCKET_KEY"); got != "123456" {
		t.Fatalf("JFTRADE_FUTU_WEBSOCKET_KEY = %q", got)
	}

	resp, err = jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()

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
	if config.UseEncryption {
		t.Fatalf("useEncryption should be forced false in saved config: %+v", config)
	}
}

func TestSettingsStoreDirectSaveMaintainsRuntimeEnvCompatibility(t *testing.T) {
	t.Setenv(futu.EnvOpenDAddr, "before")
	t.Setenv("JFTRADE_FUTU_API_PORT", "before")

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(BrokerIntegration{
		Config: FutuIntegrationConfig{
			Host:    "127.0.0.5",
			APIPort: 25555,
		},
	})
	if err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	if got := os.Getenv(futu.EnvOpenDAddr); got != "127.0.0.5:25555" {
		t.Fatalf("%s = %q", futu.EnvOpenDAddr, got)
	}
	if got := os.Getenv("JFTRADE_FUTU_API_PORT"); got != "25555" {
		t.Fatalf("JFTRADE_FUTU_API_PORT = %q", got)
	}
}

func TestBrokerSettingsExposeNullIntegrationUntilFirstSave(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	api := newTestServer(t, store)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET settings status = %d", resp.StatusCode)
	}

	var response struct {
		OK   bool `json:"ok"`
		Data struct {
			Brokers []struct {
				Integration *BrokerIntegration    `json:"integration"`
				Defaults    FutuIntegrationConfig `json:"defaults"`
			} `json:"brokers"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if !response.OK || len(response.Data.Brokers) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Data.Brokers[0].Integration != nil {
		t.Fatalf("expected nil integration before first save, got %+v", response.Data.Brokers[0].Integration)
	}
	if response.Data.Brokers[0].Defaults.APIPort != defaultFutuAPIPort {
		t.Fatalf("defaults apiPort = %d", response.Data.Brokers[0].Defaults.APIPort)
	}
	if response.Data.Brokers[0].Defaults.WebSocketPort != defaultFutuWebSocketPort {
		t.Fatalf("defaults websocketPort = %d", response.Data.Brokers[0].Defaults.WebSocketPort)
	}
}

func TestFutuRuntimeAndHealthStayNeutralWithoutSavedEnabledIntegration(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	api := newTestServer(t, store)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	runtimeResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/brokers/futu/runtime")
	if err != nil {
		t.Fatalf("GET runtime: %v", err)
	}
	defer func() { jftradeCheckTestError(t, runtimeResp.Body.Close()) }()
	if runtimeResp.StatusCode != http.StatusOK {
		t.Fatalf("GET runtime status = %d", runtimeResp.StatusCode)
	}

	var runtimeEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Session struct {
				Connectivity string  `json:"connectivity"`
				CheckedAt    string  `json:"checkedAt"`
				LastError    *string `json:"lastError"`
			} `json:"session"`
		} `json:"data"`
	}
	if err := json.NewDecoder(runtimeResp.Body).Decode(&runtimeEnvelope); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	if !runtimeEnvelope.OK {
		t.Fatalf("unexpected runtime envelope: %+v", runtimeEnvelope)
	}
	if runtimeEnvelope.Data.Session.Connectivity != "disconnected" {
		t.Fatalf("runtime connectivity = %q", runtimeEnvelope.Data.Session.Connectivity)
	}
	if runtimeEnvelope.Data.Session.CheckedAt != "" {
		t.Fatalf("runtime checkedAt = %q", runtimeEnvelope.Data.Session.CheckedAt)
	}
	if runtimeEnvelope.Data.Session.LastError != nil {
		t.Fatalf("runtime lastError = %v", *runtimeEnvelope.Data.Session.LastError)
	}

	healthResp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/futu-opend")
	if err != nil {
		t.Fatalf("GET OpenD health: %v", err)
	}
	defer func() { jftradeCheckTestError(t, healthResp.Body.Close()) }()
	if healthResp.StatusCode != http.StatusOK {
		t.Fatalf("GET OpenD health status = %d", healthResp.StatusCode)
	}

	var healthEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			CheckedAt string `json:"checkedAt"`
			Diagnosis struct {
				Code                string `json:"code"`
				ManualRetryRequired bool   `json:"manualRetryRequired"`
			} `json:"diagnosis"`
			Runtime struct {
				Connectivity string  `json:"connectivity"`
				LastError    *string `json:"lastError"`
			} `json:"runtime"`
		} `json:"data"`
	}
	if err := json.NewDecoder(healthResp.Body).Decode(&healthEnvelope); err != nil {
		t.Fatalf("decode OpenD health: %v", err)
	}
	if !healthEnvelope.OK {
		t.Fatalf("unexpected OpenD health envelope: %+v", healthEnvelope)
	}
	if healthEnvelope.Data.CheckedAt != "" {
		t.Fatalf("health checkedAt = %q", healthEnvelope.Data.CheckedAt)
	}
	if healthEnvelope.Data.Runtime.Connectivity != "disconnected" {
		t.Fatalf("health connectivity = %q", healthEnvelope.Data.Runtime.Connectivity)
	}
	if healthEnvelope.Data.Runtime.LastError != nil {
		t.Fatalf("health lastError = %v", *healthEnvelope.Data.Runtime.LastError)
	}
	if healthEnvelope.Data.Diagnosis.Code != "NONE" {
		t.Fatalf("health diagnosis code = %q", healthEnvelope.Data.Diagnosis.Code)
	}
	if healthEnvelope.Data.Diagnosis.ManualRetryRequired {
		t.Fatal("expected manualRetryRequired=false without a saved enabled integration")
	}

	saved, err := store.SaveIntegration(BrokerIntegration{
		BrokerID: "futu",
		Enabled:  false,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type:          "futu",
			Host:          "127.0.0.1",
			APIPort:       11110,
			WebSocketPort: 11111,
			TradeMarket:   "HK",
			SecurityFirm:  "FUTUSECURITIES",
		}),
	})
	if err != nil {
		t.Fatalf("saveIntegration disabled: %v", err)
	}
	if saved.Enabled {
		t.Fatal("expected saved integration to remain disabled")
	}

	healthResp, err = jftradeTestHTTPGet(t, srv.URL+"/api/v1/system/futu-opend")
	if err != nil {
		t.Fatalf("GET OpenD health after disabled save: %v", err)
	}
	defer func() { jftradeCheckTestError(t, healthResp.Body.Close()) }()
	if err := json.NewDecoder(healthResp.Body).Decode(&healthEnvelope); err != nil {
		t.Fatalf("decode OpenD health after disabled save: %v", err)
	}
	if healthEnvelope.Data.Diagnosis.Code != "NONE" || healthEnvelope.Data.Runtime.LastError != nil {
		t.Fatalf("expected disabled integration to stay neutral, got %+v", healthEnvelope.Data)
	}
}

func TestFutuRuntimeAndHealthDiagnoseEnabledButUnreachableOpenD(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(BrokerIntegration{
		Enabled: true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Host:                    "127.0.0.1",
			APIPort:                 1,
			WebSocketPort:           2,
			WebSocketKey:            "diagnostic-key",
			MaxWebSocketConnections: 3,
			TradeMarket:             "HK",
		}),
	})
	if err != nil {
		t.Fatalf("SaveIntegration: %v", err)
	}
	api := newTestServer(t, store)

	guide := api.futuOpenDInstallGuide()
	settings := guide["settings"].(map[string]any)
	if settings["apiPort"] != 1 || settings["websocketKeyRequired"] != true || settings["marketDataTransport"] != liveQuoteTransportMode {
		t.Fatalf("install guide settings = %#v", settings)
	}

	runtime := api.brokerRuntime(context.Background())
	session := runtime["session"].(map[string]any)
	connection := session["connection"].(map[string]any)
	if session["connectivity"] != "disconnected" || session["accountsDiscovered"] != 0 || connection["apiPort"] != 1 {
		t.Fatalf("runtime session = %#v", session)
	}
	if session["globalState"] != nil {
		t.Fatalf("unreachable runtime globalState = %#v", session["globalState"])
	}

	health := api.futuOpenDHealth(context.Background())
	diagnosis := health["diagnosis"].(map[string]any)
	healthRuntime := health["runtime"].(map[string]any)
	if health["status"] != "offline" || healthRuntime["connectivity"] != "disconnected" {
		t.Fatalf("health = %#v", health)
	}
	if diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true || diagnosis["restartOpenDRecommended"] != true {
		t.Fatalf("diagnosis = %#v", diagnosis)
	}
	if healthRuntime["websocketKeyConfigured"] != true || healthRuntime["marketDataTransport"] != liveQuoteTransportMode {
		t.Fatalf("health runtime = %#v", healthRuntime)
	}
}

func TestManagedBrokerAccountCRUDReflectsInBrokerSettings(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	api := newTestServer(t, store)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	payload := map[string]any{
		"brokerId":           "futu",
		"accountId":          "12345678",
		"displayName":        "Main Sim",
		"tradingEnvironment": "SIMULATE",
		"market":             "HK",
		"securityFirm":       "FUTUSECURITIES",
		"enabled":            true,
	}
	body, jftradeErr2 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr2)
	resp, err := jftradeTestHTTPPost(t, srv.URL+"/api/v1/settings/broker-accounts", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST managed account: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

	resp, err = jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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
	body, jftradeErr3 := json.Marshal(updatedPayload)
	jftradeCheckTestError(t, jftradeErr3)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/broker-accounts/"+url.PathEscape(createEnvelope.Data.ID), bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest update managed account: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT managed account: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT managed account status = %d", resp.StatusCode)
	}

	resp, err = jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings after update: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if err := json.NewDecoder(resp.Body).Decode(&settingsEnvelope); err != nil {
		t.Fatalf("decode settings after update: %v", err)
	}
	if got := settingsEnvelope.Data.Accounts[0].DisplayName; got != "Primary Sim" {
		t.Fatalf("unexpected displayName after update = %q", got)
	}

	req, err = http.NewRequestWithContext(t.Context(), http.MethodDelete, srv.URL+"/api/v1/settings/broker-accounts/"+url.PathEscape(createEnvelope.Data.ID), nil)
	if err != nil {
		t.Fatalf("NewRequest delete managed account: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE managed account: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE managed account status = %d", resp.StatusCode)
	}

	resp, err = jftradeTestHTTPGet(t, srv.URL+"/api/v1/settings/brokers")
	if err != nil {
		t.Fatalf("GET broker settings after delete: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if err := json.NewDecoder(resp.Body).Decode(&settingsEnvelope); err != nil {
		t.Fatalf("decode settings after delete: %v", err)
	}
	if len(settingsEnvelope.Data.Accounts) != 0 {
		t.Fatalf("expected zero accounts after delete: %+v", settingsEnvelope.Data.Accounts)
	}
}

func TestUIAppearanceSavePersistsToSettings(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewSettingsStore(settingsPath)
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	api := newTestServer(t, store)
	srv := httptest.NewServer(api)
	t.Cleanup(srv.Close)

	payload := map[string]any{
		"appearance": map[string]any{
			"upColor":   "#0055aa",
			"downColor": "#aa2200",
		},
	}
	body, jftradeErr4 := json.Marshal(payload)
	jftradeCheckTestError(t, jftradeErr4)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, srv.URL+"/api/v1/settings/ui", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest UI appearance: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT UI appearance: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT UI appearance status = %d", resp.StatusCode)
	}

	rawSettings, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("ReadFile settings: %v", err)
	}
	var decoded struct {
		Appearance UIAppearanceSettings `json:"appearance"`
	}
	if err := json.Unmarshal(rawSettings, &decoded); err != nil {
		t.Fatalf("Unmarshal settings: %v", err)
	}
	if decoded.Appearance.UpColor != "#0055aa" || decoded.Appearance.DownColor != "#aa2200" {
		t.Fatalf("persisted appearance = %+v", decoded.Appearance)
	}
}
