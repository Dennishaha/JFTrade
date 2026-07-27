package assembly

import (
	"context"
	"strings"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	bt "github.com/jftrade/jftrade-main/pkg/backtest"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestApplicationAdapterReportsUnavailableDomainServices(t *testing.T) {
	deps := NewApplicationAdapter(ApplicationPorts{}).ToolDeps()

	orders, ok := deps.ExecutionOrders().([]trdsrv.ExecutionOrder)
	if !ok || len(orders) != 0 {
		t.Fatalf("execution orders fallback = %#v, want typed empty trading orders", orders)
	}
	events := deps.ExecutionOrderEvents("order-1")
	if events == nil {
		t.Fatal("execution order events fallback is nil")
	}
	for name, invoke := range map[string]func() error{
		"broker orders": func() error {
			_, err := deps.BrokerOrders(t.Context(), BrokerReadInput{})
			return err
		},
		"strategy draft": func() error {
			_, err := deps.SaveStrategyDraft(StrategyDraftInput{})
			return err
		},
		"backtest enqueue": func() error {
			_, err := deps.EnqueueBacktest(BacktestStartInput{})
			return err
		},
		"workflow snapshot": func() error {
			_, err := NewApplicationAdapter(ApplicationPorts{}).WorkflowMarketSnapshot(
				t.Context(),
				"US.AAPL",
			)
			return err
		},
	} {
		if err := invoke(); err == nil {
			t.Fatalf("%s error = nil, want unavailable service error", name)
		}
	}
	if got := deps.ListStrategyDefinitions(); got != nil {
		t.Fatalf("strategy definitions fallback = %#v, want nil", got)
	}
	if got := deps.ListBacktestRuns(); got != nil {
		t.Fatalf("backtest runs fallback = %#v, want nil", got)
	}
}

func TestApplicationAdapterNormalizesCrossDomainInputs(t *testing.T) {
	if scope, err := normalizeTradingBrokerScope(" history "); err != nil || scope != "HISTORY" {
		t.Fatalf("history scope = %q, %v", scope, err)
	}
	if scope, err := normalizeTradingBrokerScope(""); err != nil || scope != "CURRENT" {
		t.Fatalf("default scope = %q, %v", scope, err)
	}
	if _, err := normalizeTradingBrokerScope("archive"); err == nil {
		t.Fatal("invalid broker scope error = nil")
	}
	values := mergeBrokerValues(
		[]string{" submitted, filled ", "FILLED"},
		[]string{"cancelled", "SUBMITTED"},
	)
	if got := strings.Join(values, ","); got != "submitted,filled,cancelled" {
		t.Fatalf("merged broker values = %q", got)
	}

	for _, test := range []struct {
		input  string
		market string
		symbol string
		ok     bool
	}{
		{input: " us.aapl ", market: "US", symbol: "AAPL", ok: true},
		{input: "US.BRK.B", market: "US", symbol: "BRK.B", ok: true},
		{input: "US.", ok: false},
		{input: "AAPL", ok: false},
	} {
		market, symbol, ok := splitWorkflowInstrumentID(test.input)
		if market != test.market || symbol != test.symbol || ok != test.ok {
			t.Fatalf(
				"splitWorkflowInstrumentID(%q) = %q/%q/%v",
				test.input,
				market,
				symbol,
				ok,
			)
		}
	}
}

func TestApplicationAdapterNormalizesStrategyVisualModels(t *testing.T) {
	model, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []any{map[string]any{"id": "node-1"}},
		"edges": []any{map[string]any{"sourceNodeId": "node-1", "targetNodeId": "node-2"}},
	})
	if err != nil {
		t.Fatalf("normalize visual model: %v", err)
	}
	if model.Engine != "logic-flow" || model.Version != 1 ||
		model.Nodes[0].Properties == nil || model.Edges[0].Type != "polyline" {
		t.Fatalf("normalized visual model = %#v", model)
	}
	if _, err := strategyVisualModelFromInput("not-an-object"); err == nil {
		t.Fatal("string visual model error = nil")
	}
	if _, err := strategyVisualModelFromInput(map[string]any{
		"nodes": []any{map[string]any{
			"id":         "legacy",
			"properties": map[string]any{"blockKind": "technicalIndicator"},
		}},
	}); err == nil {
		t.Fatal("legacy visual block error = nil")
	}
}

func TestApplicationAdapterProjectsBacktestState(t *testing.T) {
	if got := backtestDataReadinessFromService(nil); got != (BacktestDataReadiness{}) {
		t.Fatalf("nil readiness = %#v", got)
	}
	readiness := backtestDataReadinessFromService(&btsrv.DataReadiness{
		Status: "syncing_data",
		Sync: &btsrv.SyncStarted{
			TaskID:       "sync-1",
			Symbol:       "US.AAPL",
			Intervals:    []bbgotypes.Interval{bbgotypes.Interval1m},
			Since:        "2026-01-01",
			Until:        "2026-01-02",
			SessionScope: "regular",
		},
		Progress: &bt.SyncProgress{Status: "running"},
	})
	if readiness.DataSync == nil || readiness.DataSync.Status != "running" ||
		len(readiness.DataSync.Intervals) != 1 || readiness.DataSync.Intervals[0] != "1m" {
		t.Fatalf("readiness projection = %#v", readiness)
	}

	run := backtestRunSummaryFromService(&btsrv.RunState{
		ID: "run-1", Status: "completed",
		Request: btsrv.StartRequest{DefinitionID: "strategy-1", Market: "US", Symbol: "AAPL"},
	})
	if run.ID != "run-1" || run.DefinitionID != "strategy-1" || run.Symbol != "AAPL" {
		t.Fatalf("run projection = %#v", run)
	}
}

func TestApplicationWorkflowSnapshotRejectsInvalidInstrument(t *testing.T) {
	adapter := NewApplicationAdapter(ApplicationPorts{
		MarketData: func() *mdsrv.Service {
			return &mdsrv.Service{}
		},
	})
	if _, err := adapter.WorkflowMarketSnapshot(context.Background(), "invalid"); err == nil {
		t.Fatal("invalid workflow instrument error = nil")
	}
}
