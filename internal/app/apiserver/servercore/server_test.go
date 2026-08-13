package servercore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	apruntime "github.com/jftrade/jftrade-main/internal/app/apiserver/runtime"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
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

func TestPersistenceOnlySettingsStoreKeepsConcreteStore(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	if got := persistenceOnlySettingsStore(store); got != store {
		t.Fatalf("persistenceOnlySettingsStore() = %T, want concrete settingsfile store", got)
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

func TestNewServerUsesStrategyRuntimeDBEnvOverride(t *testing.T) {
	customRuntimeDBPath := filepath.Join(t.TempDir(), "custom", "strategy-runtime-override.db")
	t.Setenv("JFTRADE_STRATEGY_RUNTIME_DB", customRuntimeDBPath)

	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	if server.stores.StrategyCatalog == nil {
		t.Fatal("expected strategy runtime store to be initialized with env override")
	}
	if _, err := os.Stat(customRuntimeDBPath); err != nil {
		t.Fatalf("expected runtime db file at env override path, got error: %v", err)
	}
	if got := apruntime.DeriveStrategyRuntimeDBPath(store.Path()); got != customRuntimeDBPath {
		t.Fatalf("DeriveStrategyRuntimeDBPath() = %s, want %s", got, customRuntimeDBPath)
	}
}

func TestServerCloseStopsMarketdataAndPreventsExchangeRevival(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	saveTestIntegration(t, store, jfsettings.BrokerIntegration{
		BrokerID: "futu",
		Enabled:  true,
		Config: normalizeFutuConfig(jfsettings.FutuIntegrationConfig{
			Type: "futu", Host: "127.0.0.1", APIPort: 1,
			WebSocketPort: 11111, MaxWebSocketConnections: 20,
			TradeMarket: "HK", SecurityFirm: "FUTUSECURITIES",
		}),
		CreatedAt: now,
		UpdatedAt: now,
	})

	server := newTestServer(t, store)
	if exchange := server.futuCoordinator().Exchange(); exchange == nil {
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
	if exchange := server.runtimes.MarketData().Ensure(); exchange != nil {
		t.Fatal("Futu exchange revived after Server.Close")
	}
}
