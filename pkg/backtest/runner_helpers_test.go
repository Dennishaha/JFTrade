package backtest

import (
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

func TestBacktestRunnerHelpers(t *testing.T) {
	t.Run("deriveStrategyWarmupCandles uses indicatorruntime planning and rejects invalid scripts", func(t *testing.T) {
		script := `//@version=6
strategy("Warmup", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 1))`

		includeExtendedHours := true
		warmup, err := deriveStrategyWarmupCandles(script, types.Interval1h, "US.AAPL", &includeExtendedHours)
		if err != nil {
			t.Fatalf("deriveStrategyWarmupCandles() error = %v", err)
		}
		if warmup <= 0 {
			t.Fatalf("deriveStrategyWarmupCandles() = %d, want positive warmup", warmup)
		}

		if _, err := deriveStrategyWarmupCandles("strategy(", types.Interval1m, "US.AAPL", nil); err == nil {
			t.Fatal("deriveStrategyWarmupCandles(invalid) error = nil, want non-nil")
		}
	})

	t.Run("resolveBacktestReadSessionScope normalizes nil and explicit toggles", func(t *testing.T) {
		if got := resolveBacktestReadSessionScope(nil); got != "auto" {
			t.Fatalf("resolveBacktestReadSessionScope(nil) = %q", got)
		}
		includeExtendedHours := true
		if got := resolveBacktestReadSessionScope(&includeExtendedHours); got != "extended" {
			t.Fatalf("resolveBacktestReadSessionScope(true) = %q", got)
		}
		includeExtendedHours = false
		if got := resolveBacktestReadSessionScope(&includeExtendedHours); got != "regular" {
			t.Fatalf("resolveBacktestReadSessionScope(false) = %q", got)
		}
	})

	t.Run("estimateReplayBarCapacity handles invalid inputs and inclusive ranges", func(t *testing.T) {
		start := time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC)
		end := start.Add(3 * time.Minute)

		if got := estimateReplayBarCapacity(time.Time{}, end, types.Interval1m); got != 0 {
			t.Fatalf("estimateReplayBarCapacity(zero start) = %d, want 0", got)
		}
		if got := estimateReplayBarCapacity(start, start, types.Interval1m); got != 0 {
			t.Fatalf("estimateReplayBarCapacity(equal times) = %d, want 0", got)
		}
		if got := estimateReplayBarCapacity(start, end, types.Interval1m); got != 4 {
			t.Fatalf("estimateReplayBarCapacity(3m span, 1m) = %d, want 4", got)
		}
	})
}
