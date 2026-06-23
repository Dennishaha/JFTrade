package indicatorruntime

import (
	"strings"
	"testing"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestWarmupBarsFromScriptMatchesPlanWithExtendedHours(t *testing.T) {
	script := `//@version=6
strategy("Warmup Script", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(close, slow)
    strategy.entry("Long", strategy.long, qty=1)`

	program, err := strategypine.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	options := RuntimeOptions{IncludeExtendedHours: true}
	fromScript, err := WarmupBarsFromScriptForSymbolWithOptions(script, types.Interval1m, "US.AAPL", options)
	if err != nil {
		t.Fatalf("WarmupBarsFromScriptForSymbolWithOptions() error = %v", err)
	}
	fromPlan, err := WarmupBarsFromPlanForSymbolWithOptions(plan, types.Interval1m, "US.AAPL", options)
	if err != nil {
		t.Fatalf("WarmupBarsFromPlanForSymbolWithOptions() error = %v", err)
	}

	dayMinutes, ok := market.TradingMinutesPerTradingDay("US.AAPL", true)
	if !ok {
		t.Fatal("TradingMinutesPerTradingDay(US.AAPL, true) = not found")
	}
	want := 20 * dayMinutes
	if fromScript != want || fromPlan != want {
		t.Fatalf("warmup from script/plan = %d/%d, want %d", fromScript, fromPlan, want)
	}
}

func TestWarmupBarsFromScriptFallsBackToGenericTradingCalendarForUnknownSymbol(t *testing.T) {
	script := `//@version=6
strategy("Unknown Symbol Warmup", overlay=true)
monthly = request.security(syminfo.tickerid, "M", ta.sma(close, 1))`

	warmupBars, err := WarmupBarsFromScriptForSymbol(script, types.Interval5m, "CRYPTO.BTC")
	if err != nil {
		t.Fatalf("WarmupBarsFromScriptForSymbol() error = %v", err)
	}

	want := tradingSessionMinutesPerMonth / 5
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromScriptForSymbol() = %d, want %d", warmupBars, want)
	}
}

func TestWarmupBarsFromScriptRejectsInvalidScript(t *testing.T) {
	if _, err := WarmupBarsFromScript("strategy(",
		types.Interval1m); err == nil {
		t.Fatal("WarmupBarsFromScript() error = nil, want parse failure")
	}
}

func TestWarmupBarsFromPlanRejectsInvalidIndicatorKeys(t *testing.T) {
	plan := strategyir.Requirements{
		Indicators: []strategyir.IndicatorRequirement{{Kind: "broken", Key: "not-a-valid-indicator-key"}},
	}

	_, err := WarmupBarsFromPlanForSymbol(plan, types.Interval1m, "US.AAPL")
	if err == nil {
		t.Fatal("WarmupBarsFromPlanForSymbol() error = nil, want invalid key error")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("WarmupBarsFromPlanForSymbol() error = %v, want invalid-key context", err)
	}
}
