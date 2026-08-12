package assembly

import (
	"context"
	"errors"
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

	if _, _, err := deps.ExecutionOrders(t.Context(), BrokerReadInput{}); err == nil {
		t.Fatal("execution orders unavailable error = nil")
	}
	if _, err := deps.ExecutionOrderEvents("order-1"); err == nil {
		t.Fatal("execution order events unavailable error = nil")
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
	if got, err := deps.ListStrategyDefinitions(); err == nil || got != nil {
		t.Fatalf("strategy definitions = %#v, %v; want unavailable error", got, err)
	}
	if got := deps.ListBacktestRuns(); got != nil {
		t.Fatalf("backtest runs fallback = %#v, want nil", got)
	}
}

func TestApplicationAdapterPropagatesExecutionProjectionFailures(t *testing.T) {
	want := errors.New("execution storage unavailable")
	adapter := NewApplicationAdapter(ApplicationPorts{Trading: func() *trdsrv.Service {
		return trdsrv.NewService(
			trdsrv.WithListOrders(func(context.Context, trdsrv.ExecutionOrderFilter) (trdsrv.ExecutionOrders, error) {
				return trdsrv.ExecutionOrders{}, want
			}),
			trdsrv.WithGetOrderEvents(func(context.Context, string) (trdsrv.ExecutionOrderEvents, error) {
				return trdsrv.ExecutionOrderEvents{}, want
			}),
		)
	}})
	deps := adapter.ToolDeps()
	for name, call := range map[string]func() error{
		"orders": func() error { _, _, err := deps.ExecutionOrders(t.Context(), BrokerReadInput{}); return err },
		"events": func() error { _, err := deps.ExecutionOrderEvents("order-1"); return err },
	} {
		if !errors.Is(call(), want) {
			t.Fatalf("%s error did not preserve storage failure", name)
		}
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

func TestApplicationAdapterForwardsMarketCandles(t *testing.T) {
	provider := &assistantMarketDataProvider{response: mdsrv.CandlesResponse{"source": "fixture"}}
	adapter := NewApplicationAdapter(ApplicationPorts{
		MarketData: func() *mdsrv.Service { return mdsrv.NewService(provider) },
	})
	result, err := adapter.ToolDeps().MarketCandles(t.Context(), "US", "AAPL", "1d", 25)
	if err != nil {
		t.Fatalf("MarketCandles: %v", err)
	}
	if result.(map[string]any)["source"] != "fixture" || provider.query.Market != "US" ||
		provider.query.Symbol != "AAPL" || provider.query.Period != "1d" || provider.query.Limit != 25 {
		t.Fatalf("market candle forwarding = %#v, query=%#v", result, provider.query)
	}
}

type assistantMarketDataProvider struct {
	mdsrv.Provider
	response mdsrv.CandlesResponse
	query    mdsrv.HistoricalCandlesQuery
}

func (p *assistantMarketDataProvider) GetHistoricalCandles(_ context.Context, query mdsrv.HistoricalCandlesQuery) (mdsrv.CandlesResponse, error) {
	p.query = query
	return p.response, nil
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
