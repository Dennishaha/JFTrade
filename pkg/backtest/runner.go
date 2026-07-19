// Package backtest provides a standalone backtest runner for Futu strategies
// using bbgo's backtest engine with a local SQLite K-line store.
package backtest

import (
	"context"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

// Run is intentionally disabled in the PineTS hard-cut path.
//
// Historical callers must use RunWithPineWorker with an explicit Pine worker
// runner so Pine Script execution cannot silently fall back to the removed Go
// Pine runtime.
func Run(_ context.Context, cfg RunConfig) *RunResult {
	result := newRunResult(cfg)
	result.Error = "direct Go Pine backtest runner has been removed; configure a PineTS worker and use RunWithPineWorker"
	return result
}

func isMissingPrepareKLineError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "no kline data found for symbol") &&
		strings.Contains(message, "1m before start time")
}

func resolvePineInitialBalance(requested float64, metadata strategyir.StrategyMetadata) float64 {
	if requested > 0 {
		return requested
	}
	if metadata.InitialCapital > 0 {
		return metadata.InitialCapital
	}
	return 100000
}

func deriveStrategyWarmupCandles(script string, interval types.Interval, symbol string, useExtendedHours *bool) (int, error) {
	return indicatorruntime.WarmupBarsFromScriptForSymbolWithOptions(
		script,
		interval,
		symbol,
		indicatorruntime.RuntimeOptions{IncludeExtendedHours: useExtendedHours != nil && *useExtendedHours},
	)
}

func resolveBacktestReadSessionScope(useExtendedHours *bool) string {
	if useExtendedHours == nil {
		return "auto"
	}
	if *useExtendedHours {
		return "extended"
	}
	return "regular"
}

func estimateReplayBarCapacity(start, end time.Time, interval types.Interval) int {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return 0
	}
	intervalDuration := interval.Duration()
	if intervalDuration <= 0 {
		return 0
	}
	return int(end.Sub(start)/intervalDuration) + 1
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
