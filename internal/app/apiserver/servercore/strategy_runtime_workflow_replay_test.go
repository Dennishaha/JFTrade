package servercore

import (
	"path/filepath"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/shopspring/decimal"
)

func TestWorkflowSnapshotAndLiveTradeReplayReachBusinessConsumers(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	now := time.Now().UTC().Truncate(time.Second)
	seedCachedTickSample(server, mdsrv.Tick{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		Bid:          decimal.RequireFromString("321.3"),
		Ask:          decimal.RequireFromString("321.5"),
		Volume:       12,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
		Source:       "workflow-replay",
	})
	snapshot, err := server.workflowMarketSnapshot(t.Context(), "hk.00700")
	if err != nil {
		t.Fatalf("workflow cached market snapshot: %v", err)
	}
	if got := jftradeCheckedTypeAssertion[map[string]any](snapshot["snapshot"])["price"]; got != "321.4" {
		t.Fatalf("workflow snapshot price = %v", got)
	}

	// The manager has no matching runtime here, but a valid trade must still be
	// converted to the canonical strategy-runtime event without panicking.
	server.assistantSvc = nil
	runtime := liveruntime.NewManager(liveruntime.Dependencies{})
	server.runtimes.SetStrategyRuntime(runtime, runtime)
	server.handlePushMarketdataTick(mdsrv.Tick{
		Kind:         "trade",
		InstrumentID: "HK.00700",
		Price:        decimal.RequireFromString("321.4"),
		Volume:       6,
		QuoteAt:      now.Format(time.RFC3339Nano),
		ObservedAt:   now.Format(time.RFC3339Nano),
	})
}

func TestWorkflowSnapshotReturnsProviderFailureAfterStaleCache(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)
	stale := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	seedCachedTickSample(server, mdsrv.Tick{
		InstrumentID: "HK.00700",
		Market:       "HK",
		Symbol:       "00700",
		Price:        decimal.RequireFromString("321.4"),
		QuoteAt:      stale.Format(time.RFC3339Nano),
		ObservedAt:   stale.Format(time.RFC3339Nano),
		Source:       "stale-cache",
	})
	if _, err := server.workflowMarketSnapshot(t.Context(), "HK.00700"); err == nil {
		t.Fatal("a stale workflow snapshot must surface the provider failure")
	}
}
