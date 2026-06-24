package servercore

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldStartForAPIOnlyArgs(t *testing.T) {
	if !shouldStartForArgs([]string{"api"}) {
		t.Fatal("expected api command to start JFTrade sidecar")
	}
	if !shouldStartForArgs([]string{"serve-api"}) {
		t.Fatal("expected serve-api command to start JFTrade sidecar")
	}
	if shouldStartForArgs([]string{"run"}) {
		t.Fatal("expected removed bbgo run command to be ignored")
	}
}

func TestPersistenceOnlySettingsStoreUnwrapsCompatibilityStore(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if got := persistenceOnlySettingsStore(store); got != store.Store {
		t.Fatalf("persistenceOnlySettingsStore() = %T, want embedded settingsfile store", got)
	}
}

func TestExchangeCalendarOperationContextIgnoresRequestCancellation(t *testing.T) {
	requestCtx, requestCancel := context.WithCancel(context.Background())
	requestCancel()

	operationCtx, operationCancel := exchangeCalendarOperationContext(requestCtx)
	defer operationCancel()

	select {
	case <-operationCtx.Done():
		t.Fatalf("operation context inherited request cancellation: %v", operationCtx.Err())
	default:
	}
}

func TestBrokerRuntimeDescriptorIncludesReadFeatures(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
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

func TestRequestObservabilityMiddlewarePropagatesRequestID(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	srv := newHTTPTestServer(t, store)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/system/status", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set(requestIDHeader, "test-request-id")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET system status: %v", err)
	}
	defer func() { jftradeCheckTestError(t, resp.Body.Close()) }()
	if got := resp.Header.Get(requestIDHeader); got != "test-request-id" {
		t.Fatalf("%s = %q, want propagated request id", requestIDHeader, got)
	}
}

func TestNewServerUsesStrategyRuntimeDBEnvOverride(t *testing.T) {
	customRuntimeDBPath := filepath.Join(t.TempDir(), "custom", "strategy-runtime-override.db")
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", customRuntimeDBPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if server.strategyRuntimeStore == nil {
		t.Fatal("expected strategy runtime store to be initialized with env override")
	}
	if _, err := os.Stat(customRuntimeDBPath); err != nil {
		t.Fatalf("expected runtime db file at env override path, got error: %v", err)
	}
	if got := deriveStrategyRuntimeDBPath(store.path); got != customRuntimeDBPath {
		t.Fatalf("deriveStrategyRuntimeDBPath() = %s, want %s", got, customRuntimeDBPath)
	}
}

func TestServerCloseStopsMarketdataAndPreventsExchangeRevival(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	store.mu.Lock()
	store.data.Integration = &BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(FutuIntegrationConfig{
			Type: "futu", Host: "127.0.0.1", APIPort: 1,
			WebSocketPort: 11111, MaxWebSocketConnections: 20,
			TradeMarket: "HK", SecurityFirm: "FUTUSECURITIES",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	}
	store.mu.Unlock()

	server := newTestServer(t, store)
	if exchange := server.futuExchange(); exchange == nil {
		t.Fatal("expected exchange before Close")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := server.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if state := server.marketdataSvc.RuntimeState(); !state.Closed || state.Connected {
		t.Fatalf("marketdata state after Close = %#v", state)
	}
	if exchange := server.marketdataRuntime.Ensure(); exchange != nil {
		t.Fatal("Futu exchange revived after Server.Close")
	}
}
