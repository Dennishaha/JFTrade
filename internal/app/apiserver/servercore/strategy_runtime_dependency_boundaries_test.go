package servercore

import (
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/marketdataapp"
	"github.com/jftrade/jftrade-main/internal/integration/yfinance/testkit"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestStrategyRuntimeRemainingDependencyBoundaries(t *testing.T) {
	server := &Server{}
	deps := newStrategyRuntimeDependencies(server)
	if _, ok := deps.CurrentInstance("missing"); ok {
		t.Fatal("nil strategy store returned an instance")
	}
	if err := deps.AppendRuntimeEvent("missing", "", "", ""); err != nil {
		t.Fatalf("nil append runtime event error = %v", err)
	}
	if err := deps.TransitionInstance("missing", "STOPPED", "", ""); err != nil {
		t.Fatalf("nil transition error = %v", err)
	}
	if err := deps.ReconcileRuntimeFailure("missing", "detail"); err != nil {
		t.Fatalf("nil reconcile error = %v", err)
	}
	if _, err := deps.PlaceExecutionOrder(t.Context(), trdsrv.ExecutionOrderCommand{}); err == nil {
		t.Fatal("nil trading placement error = nil")
	}
	if _, err := deps.CancelExecutionOrder(t.Context(), "missing"); err == nil {
		t.Fatal("nil trading cancellation error = nil")
	}
	if count, err := deps.CountRuntimeAudit(t.Context(), runtimeactivity.AuditQuery{}); err != nil || count != 0 {
		t.Fatalf("nil audit count = %d, %v", count, err)
	}
	if err := deps.UpsertObservation(t.Context(), runtimeactivity.ObservationSnapshot{}); err != nil {
		t.Fatalf("nil observation error = %v", err)
	}
	if _, err := deps.AcquireMarketDataLease(t.Context(), "consumer", []mdsrv.InstrumentRef{{Channel: "KLINE"}}); err == nil {
		t.Fatal("nil market-data lease error = nil")
	}
	deps.WakeMarketDataCollector()
}

func TestNewStrategyRuntimeManagerDisabledExchange(t *testing.T) {
	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	manager := liveruntime.NewManager(newStrategyRuntimeDependencies(server))
	if manager.CurrentExchange() != nil {
		t.Fatal("disabled runtime exchange was non-nil")
	}
}

func TestStrategyRuntimeRejectsPollOnlyMarketDataProvider(t *testing.T) {
	settings, err := NewSettingsStore(t.TempDir() + "/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	sidecar := testkit.New(t)
	if err := marketdataapp.RuntimeFromService(server.marketdataSvc).Activate(t.Context(), marketdataapp.Activation{
		ProviderID:       marketdataapp.ProviderYFinance,
		YFinanceEndpoint: sidecar.URL(),
	}); err != nil {
		t.Fatalf("activate yfinance: %v", err)
	}
	deps := newStrategyRuntimeDependencies(server)
	if _, err := deps.AcquireMarketDataLease(
		t.Context(),
		"strategy-runtime:test",
		[]mdsrv.InstrumentRef{{Market: "US", Symbol: "AAPL", Channel: "KLINE"}},
	); err == nil {
		t.Fatal("poll-only provider acquired a live strategy market-data lease")
	}
}
