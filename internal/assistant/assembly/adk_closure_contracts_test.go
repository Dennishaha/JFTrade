package assembly

import (
	"context"
	"errors"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowruntime"
	assistanttestkit "github.com/jftrade/jftrade-main/internal/assistant/testkit"
)

func TestADKToolDependencyClosuresForwardNormalizedOwnerPorts(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	var observedMarket, observedSymbol, observedPeriod string
	var observedLimit int
	var observedWatchlist WatchlistListInput
	auditCalled := false
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		SystemStatus: func() map[string]any { return map[string]any{"status": "ready"} },
		ADKEnabled:   func() bool { return true },
		FutuOpenDHealth: func(context.Context) (any, error) {
			return map[string]any{"connectivity": "connected"}, nil
		},
		MarketSnapshot: func(_ context.Context, market, symbol string) (any, error) {
			observedMarket, observedSymbol = market, symbol
			return map[string]any{"symbol": symbol}, nil
		},
		MarketCandles: func(_ context.Context, market, symbol, period string, limit int) (any, error) {
			observedMarket, observedSymbol, observedPeriod, observedLimit = market, symbol, period, limit
			return []string{"candle"}, nil
		},
		WatchlistList: func(_ context.Context, input WatchlistListInput) (any, error) {
			observedWatchlist = input
			return map[string]any{"group": input.Group}, nil
		},
		RecordAudit: func(context.Context, string, string, string, map[string]any) { auditCalled = true },
	})

	status, err := getAssemblyTool(t, registry, "system.status").Handler(t.Context(), nil)
	if err != nil || status.(map[string]any)["adk"].(map[string]any)["enabled"] != true {
		t.Fatalf("system.status = %#v, err=%v", status, err)
	}
	health, err := getAssemblyTool(t, registry, "system.futu_opend").Handler(t.Context(), nil)
	if err != nil || health.(map[string]any)["connectivity"] != "connected" {
		t.Fatalf("system.futu_opend = %#v, err=%v", health, err)
	}

	if _, err := getAssemblyTool(t, registry, "market.snapshot").Handler(t.Context(), map[string]any{"market": "us", "symbol": "aapl"}); err != nil {
		t.Fatalf("market.snapshot: %v", err)
	}
	if observedMarket != "US" || observedSymbol != "AAPL" {
		t.Fatalf("snapshot query = %q/%q", observedMarket, observedSymbol)
	}
	if _, err := getAssemblyTool(t, registry, "market.candles").Handler(t.Context(), map[string]any{"market": "hk", "symbol": "00700", "period": "60m", "limit": "12"}); err != nil {
		t.Fatalf("market.candles: %v", err)
	}
	if observedMarket != "HK" || observedSymbol != "00700" || observedPeriod != "1h" || observedLimit != 12 {
		t.Fatalf("candle query = %q/%q/%q/%d", observedMarket, observedSymbol, observedPeriod, observedLimit)
	}
	if _, err := getAssemblyTool(t, registry, "watchlist.list").Handler(t.Context(), map[string]any{"groupName": " Favorites ", "market": "us", "limit": 3, "includeQuotes": true}); err != nil {
		t.Fatalf("watchlist.list: %v", err)
	}
	if observedWatchlist.Group != "Favorites" || observedWatchlist.Market != "US" || observedWatchlist.Limit != 3 || !observedWatchlist.IncludeQuotes {
		t.Fatalf("watchlist query = %#v", observedWatchlist)
	}

	RecordWorkflowAudit(t.Context(), ToolDeps{RecordAudit: func(context.Context, string, string, string, map[string]any) { auditCalled = true }}, "workflow.saved", "workflow-1", "saved", map[string]any{"status": "ok"})
	if !auditCalled {
		t.Fatal("owner audit callback was not invoked")
	}
}

func TestADKToolDependencyClosuresFailClosedWhenPortsAreMissing(t *testing.T) {
	registry := assistanttestkit.NewToolRegistry()
	portErr := func(context.Context, string, string) (any, error) { return nil, errors.New("owner port unavailable") }
	RegisterJFTradeADKTools(nil, registry, ToolDeps{
		FutuOpenDHealth: func(context.Context) (any, error) { return nil, errors.New("owner port unavailable") },
		MarketSnapshot:  portErr,
		MarketCandles: func(context.Context, string, string, string, int) (any, error) {
			return nil, errors.New("owner port unavailable")
		},
	})
	for _, name := range []string{"system.futu_opend", "market.snapshot", "market.candles", "watchlist.list"} {
		tool := getAssemblyTool(t, registry, name)
		if _, err := tool.Handler(context.Background(), map[string]any{"market": "US", "symbol": "AAPL"}); err == nil {
			t.Errorf("%s succeeded without owner port", name)
		}
	}
}

func getAssemblyTool(t *testing.T, registry *jfadk.ToolRegistry, name string) jfadk.RegisteredTool {
	t.Helper()
	tool, ok := registry.Get(name)
	if !ok {
		t.Fatalf("tool %q is not registered", name)
	}
	return tool
}
