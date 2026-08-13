package servercoretest

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/servercore"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
)

func TestBrokerFundsEndpointReturnsDisconnectedSummary(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	_, err = store.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true, Config: settingsfile.NormalizeFutuConfig(jfsettings.FutuIntegrationConfig{
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

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/brokers/futu/funds")
	if err != nil {
		t.Fatalf("GET broker funds: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
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

func TestBrokerRuntimeDescriptorIncludesReadFeatures(t *testing.T) {
	store, err := servercore.NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	resp, err := jftradeTestHTTPGet(t, srv.URL+"/api/v1/brokers/futu/runtime")
	if err != nil {
		t.Fatalf("GET broker runtime: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET broker runtime status = %d", resp.StatusCode)
	}

	var envelope struct {
		OK   bool           `json:"ok"`
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode broker runtime: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected broker runtime ok=true")
	}

	descriptor, ok := envelope.Data["descriptor"].(map[string]any)
	if !ok {
		t.Fatalf("descriptor = %#v", envelope.Data["descriptor"])
	}
	capabilities, ok := descriptor["capabilities"].([]any)
	if !ok || len(capabilities) == 0 {
		t.Fatalf("capabilities = %#v", descriptor["capabilities"])
	}
	firstCapability, ok := capabilities[0].(map[string]any)
	if !ok {
		t.Fatalf("first capability = %#v", capabilities[0])
	}
	readFeatures, ok := firstCapability["readFeatures"].(map[string]any)
	if !ok {
		t.Fatalf("readFeatures = %#v", firstCapability["readFeatures"])
	}
	marginRatios, ok := readFeatures["marginRatios"].(map[string]any)
	if !ok {
		t.Fatalf("marginRatios capability = %#v", readFeatures["marginRatios"])
	}
	environments, ok := marginRatios["supportedEnvironments"].([]any)
	if !ok || len(environments) != 1 || environments[0] != "REAL" {
		t.Fatalf("marginRatios supportedEnvironments = %#v", marginRatios["supportedEnvironments"])
	}
	maxTradeQuantity, ok := readFeatures["maxTradeQuantity"].(map[string]any)
	if !ok {
		t.Fatalf("maxTradeQuantity capability = %#v", readFeatures["maxTradeQuantity"])
	}
	if got := maxTradeQuantity["requiresPrice"]; got != true {
		t.Fatalf("maxTradeQuantity requiresPrice = %#v, want true", got)
	}
}
