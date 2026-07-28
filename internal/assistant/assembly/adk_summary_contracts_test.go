package assembly

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestADKStrategySummariesHideSourceDetailsAndCountLinkedInstances(t *testing.T) {
	definitions := []StrategyDefinitionSummary{{
		ID: "definition-1", Name: "Mean revert", Version: "1.2.0", Runtime: "pine-plan", SourceFormat: "pine-v6",
		Symbol: "US.AAPL", Interval: "1d", Script: "strategy(\"Mean revert\")\nplot(close)", VisualNodeCount: 2, VisualEdgeCount: 1,
	}}
	instances := []StrategyInstanceSummary{{
		ID: "instance-1", DefinitionID: "definition-1", DefinitionName: "Mean revert", DefinitionVersion: "1.2.0",
		Status: "RUNNING", ActualStatus: "running", Symbols: []string{"US.AAPL"}, ActiveSymbols: []string{"US.AAPL"},
		ExecutionMode: "live", Market: "US", AccountID: "account-1", LogCount: 4, LatestLog: "order submitted", LastError: "",
	}}
	payload := SummarizeADKStrategyDefinitions(definitions, instances)
	if payload["definitionCount"] != 1 || payload["instanceCount"] != 1 {
		t.Fatalf("summary counts = %#v", payload)
	}
	definition := payload["definitions"].([]map[string]any)[0]
	if _, ok := definition["script"]; ok || definition["scriptPreview"] == "" || definition["linkedInstanceCount"] != 1 {
		t.Fatalf("definition summary = %#v", definition)
	}
	instance := payload["instances"].([]map[string]any)[0]
	if instance["definitionId"] != "definition-1" || instance["activeSymbolCount"] != 1 || instance["latestLog"] != "order submitted" {
		t.Fatalf("instance summary = %#v", instance)
	}
}

func TestADKBacktestSummariesRetainCountsWithoutEmbeddingRawSeries(t *testing.T) {
	result := &backtest.RunResult{
		QuoteCurrency: "USD", FinalBalance: 101_500, PnL: 1_500, TotalTrades: 3, WinRate: 0.66,
		Trades:  []backtest.TradeEvent{{Time: "2025-01-01", Side: "BUY", Price: "10"}},
		Candles: []backtest.Candle{{Time: "2025-01-01", Close: "10.5"}},
		Logs:    []string{"started", "completed"}, RuntimeErrors: []string{"warning"},
	}
	useExtendedHours := true
	payload := SummarizeADKBacktestRuns([]BacktestRunSummary{{
		ID: "run-1", Status: "completed", DefinitionID: "definition-1", Symbol: "US.AAPL", Interval: "1d",
		InitialBalance: 100_000, UseExtendedHours: &useExtendedHours, Result: result,
	}})
	if payload["runCount"] != 1 {
		t.Fatalf("run summary count = %#v", payload)
	}
	run := payload["runs"].([]map[string]any)[0]
	if _, ok := run["result"]; ok || run["candlesCount"] != 1 || run["tradeCount"] != 3 || run["latestLog"] != "completed" {
		t.Fatalf("run summary = %#v", run)
	}
	if run["useExtendedHours"] != true || run["totalReturn"] != 0.015 {
		t.Fatalf("run metadata = %#v", run)
	}

	filtered, total := FilterADKBacktestRuns([]BacktestRunSummary{{ID: "a", DefinitionID: "definition-1", Status: "queued"}, {ID: "b", DefinitionID: "definition-2", Status: "completed"}}, "definition-1", "", "queued", 1)
	if total != 1 || len(filtered) != 1 || filtered[0].ID != "a" {
		t.Fatalf("filtered runs = %#v total=%d", filtered, total)
	}
}
