package jftradeapi

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestValidateADKStrategyDraftScriptRejectsTradingViewPineScript(t *testing.T) {
	script := `//@version=6
strategy("TME_Bollinger_RSI_V1")
// TME Bollinger Bands + RSI Mean Reversion Strategy
plot(close)`

	err := validateADKStrategyDraftScript(script)
	if err == nil {
		t.Fatal("validateADKStrategyDraftScript() error = nil, want pine rejection")
	}
	if !strings.Contains(err.Error(), "JFTrade DSL v1") || !strings.Contains(err.Error(), "TradingView Pine Script") {
		t.Fatalf("validateADKStrategyDraftScript() error = %q, want DSL/Pine hint", err)
	}
}

func TestValidateADKStrategyDraftScriptAcceptsJFTradeDSL(t *testing.T) {
	script := `strategy Mean Revert
version 0.1.0
symbol US.TME
interval 1m

on kline_close:
  log "ready"`

	if err := validateADKStrategyDraftScript(script); err != nil {
		t.Fatalf("validateADKStrategyDraftScript() error = %v", err)
	}
}

func TestADKStrategyDefinitionsToolReturnsCompactSummaries(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	definition, err := server.designStore.saveDefinition(strategyDesignDefinition{
		Name:         "TME Demo",
		Version:      "0.1.1",
		Description:  "ADK summary test definition",
		Runtime:      strategyRuntimeDSLPlan,
		SourceFormat: "dsl-v1",
		Symbol:       "US.TME",
		Interval:     "1d",
		Script: `strategy TME Demo
version 0.1.1
symbol US.TME
interval 1d

on kline_close:
  log "demo"`,
		VisualModel: &strategyVisualModel{
			Nodes: []strategyVisualNode{{ID: "n1"}, {ID: "n2"}},
			Edges: []strategyVisualEdge{{ID: "e1", SourceNodeID: "n1", TargetNodeID: "n2"}},
		},
	})
	if err != nil {
		t.Fatalf("saveDefinition: %v", err)
	}
	instance, err := server.strategyStore.instantiateStrategy(definition, strategyInstanceBinding{
		Symbols:       []string{"US.TME"},
		Interval:      "1d",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	if err != nil {
		t.Fatalf("instantiateStrategy: %v", err)
	}
	server.enrichStrategyItem(instance)

	tool, ok := server.adkRuntime.Tools().Get("strategy.definitions")
	if !ok {
		t.Fatal("strategy.definitions tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	definitions, ok := payload["definitions"].([]map[string]any)
	if !ok || len(definitions) == 0 {
		t.Fatalf("definitions payload = %#v, want non-empty []map[string]any", payload["definitions"])
	}
	definitionSummary := definitions[0]
	if _, exists := definitionSummary["script"]; exists {
		t.Fatalf("definition summary should not include raw script: %#v", definitionSummary)
	}
	if definitionSummary["scriptPreview"] == "" {
		t.Fatalf("definition summary missing scriptPreview: %#v", definitionSummary)
	}
	if got := definitionSummary["linkedInstanceCount"]; got != 1 {
		t.Fatalf("linkedInstanceCount = %#v, want 1", got)
	}
	instances, ok := payload["instances"].([]map[string]any)
	if !ok || len(instances) == 0 {
		t.Fatalf("instances payload = %#v, want non-empty []map[string]any", payload["instances"])
	}
	instanceSummary := instances[0]
	if _, exists := instanceSummary["logs"]; exists {
		t.Fatalf("instance summary should not include raw logs: %#v", instanceSummary)
	}
	if got := instanceSummary["definitionId"]; got != definition.ID {
		t.Fatalf("definitionId = %#v, want %q", got, definition.ID)
	}
}

func TestADKBacktestRunsToolReturnsSeriesCountsInsteadOfFullArrays(t *testing.T) {
	store, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, store)

	run := &backtestRunState{
		ID:     "bt-demo",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID:   "dsl-demo",
			Market:         "US",
			Code:           "TME",
			Symbol:         "US.TME",
			Interval:       "1d",
			StartTime:      "2025-01-01T00:00:00Z",
			EndTime:        "2025-12-31T00:00:00Z",
			InitialBalance: 100000,
			RehabType:      "forward",
		},
		Result: &backtest.RunResult{
			QuoteCurrency:   "USD",
			FinalBalance:    101500,
			PnL:             1500,
			MaxDrawdown:     0.12,
			CurrentDrawdown: 0.02,
			TotalTrades:     3,
			WinRate:         0.66,
			Trades:          []backtest.TradeEvent{{Time: "2025-03-01T00:00:00Z", Side: "BUY", Price: "10", Qty: "1"}},
			OrderBook:       []backtest.OrderBookEntry{{OrderID: "o1", Symbol: "US.TME", Side: "BUY", Quantity: "1", Status: "FILLED"}},
			PnLCurve:        []backtest.PnLPoint{{Time: "2025-03-01T00:00:00Z", Equity: 100500}},
			DrawdownCurve:   []backtest.DrawdownPoint{{Time: "2025-03-01T00:00:00Z", Drawdown: 0.03}},
			Candles:         []backtest.Candle{{Time: "2025-03-01T00:00:00Z", Open: "10", High: "11", Low: "9", Close: "10.5", Volume: "100"}},
			Logs:            []string{"line 1", "line 2"},
			RuntimeErrors:   []string{"warn"},
		},
		CreatedAt: "2025-01-01T00:00:00Z",
		UpdatedAt: "2025-01-02T00:00:00Z",
	}
	if err := server.backtestRuns.add(run); err != nil {
		t.Fatalf("backtestRuns.add: %v", err)
	}

	tool, ok := server.adkRuntime.Tools().Get("backtest.runs")
	if !ok {
		t.Fatal("backtest.runs tool not registered")
	}
	output, err := tool.Handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("tool.Handler: %v", err)
	}
	payload, ok := output.(map[string]any)
	if !ok {
		t.Fatalf("tool output type = %T, want map", output)
	}
	runs, ok := payload["runs"].([]map[string]any)
	if !ok || len(runs) != 1 {
		t.Fatalf("runs payload = %#v, want single summarized run", payload["runs"])
	}
	summary := runs[0]
	if _, exists := summary["result"]; exists {
		t.Fatalf("backtest summary should not include raw result: %#v", summary)
	}
	if got := summary["candlesCount"]; got != 1 {
		t.Fatalf("candlesCount = %#v, want 1", got)
	}
	if got := summary["tradeCount"]; got != 3 {
		t.Fatalf("tradeCount = %#v, want 3", got)
	}
	if got := summary["latestLog"]; got != "line 2" {
		t.Fatalf("latestLog = %#v, want line 2", got)
	}
}
