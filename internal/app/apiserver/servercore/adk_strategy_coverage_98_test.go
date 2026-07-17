package servercore

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestCoverage98StrategyADKInputAndSummaryBoundaries(t *testing.T) {
	t.Run("empty drafts remain a no-op while validation payload gives a publishable hint", func(t *testing.T) {
		if err := ValidateADKStrategyDraftScript(" \n\t "); err != nil {
			t.Fatalf("empty draft validation = %v", err)
		}
		if _, err := ValidateADKStrategyScript("strategy.validate_pine", "\t"); err == nil || !strings.Contains(err.Error(), "非空") {
			t.Fatalf("empty strict validation error = %v", err)
		}

		payload := StrategyValidatePineToolPayload(map[string]any{"script": " "})
		if payload["ok"] != false {
			t.Fatalf("empty validation payload = %#v", payload)
		}
		errors, ok := payload["errors"].([]string)
		if !ok || len(errors) != 1 || !strings.Contains(errors[0], "必填") {
			t.Fatalf("empty validation errors = %#v", payload["errors"])
		}
		if hint, ok := payload["saveHint"].(map[string]any); !ok || strings.TrimSpace(hint["message"].(string)) == "" {
			t.Fatalf("empty validation save hint = %#v", payload["saveHint"])
		}
	})

	t.Run("valid scripts can omit requirements without losing compiled contract", func(t *testing.T) {
		payload := StrategyValidatePineToolPayload(map[string]any{
			"script": `//@version=6
strategy("Coverage strategy", overlay=true)
fast = ta.sma(close, 2)
if close > fast
    strategy.entry("long", strategy.long)`,
			"includeRequirements": false,
		})
		if payload["ok"] != true || payload["requirements"] != nil || strings.TrimSpace(payload["normalizedScript"].(string)) == "" {
			t.Fatalf("validation payload without requirements = %#v", payload)
		}
	})

	t.Run("metadata exposes every supported risk safeguard", func(t *testing.T) {
		metadata := StrategyMetadataPayload(&strategyir.Program{Metadata: strategyir.StrategyMetadata{
			AllowedEntryDirection:        "long",
			MaxIntradayLossValue:         4.5,
			MaxIntradayLossType:          "percent_of_equity",
			MaxIntradayLossAlert:         "daily loss limit",
			MaxConsLossDays:              3,
			MaxConsLossDaysAlert:         "three losing days",
			MaxDrawdownValue:             10,
			MaxDrawdownType:              "percent_of_equity",
			MaxDrawdownAlert:             "drawdown limit",
			MaxIntradayFilledOrders:      2,
			MaxIntradayFilledOrdersAlert: "filled order limit",
			MaxPositionSize:              5,
		}})
		risk, ok := metadata["risk"].(map[string]any)
		if !ok || risk["allowedEntryDirection"] != "long" || risk["maxPositionSize"] != 5.0 {
			t.Fatalf("strategy risk metadata = %#v", metadata)
		}
		if got, ok := risk["maxIntradayLoss"].(map[string]any); !ok || got["value"] != 4.5 || got["alertMessage"] != "daily loss limit" {
			t.Fatalf("intraday loss metadata = %#v", risk["maxIntradayLoss"])
		}
		if got, ok := risk["maxConsLossDays"].(map[string]any); !ok || got["count"] != 3 || got["alertMessage"] != "three losing days" {
			t.Fatalf("consecutive-loss metadata = %#v", risk["maxConsLossDays"])
		}
	})

	t.Run("queued backtests retain their explicit extended-hours selection", func(t *testing.T) {
		extended := true
		summary := SummarizeADKBacktestRuns([]BacktestRunSummary{{
			ID: "queued-run", Status: "QUEUED", Symbol: "US.AAPL", Interval: "1d", UseExtendedHours: &extended,
		}})
		runs, ok := summary["runs"].([]map[string]any)
		if !ok || len(runs) != 1 || runs[0]["useExtendedHours"] != true || runs[0]["totalReturn"] != nil {
			t.Fatalf("queued backtest summary = %#v", summary)
		}
	})
}
