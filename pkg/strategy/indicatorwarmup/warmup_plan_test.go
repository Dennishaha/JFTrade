package indicatorwarmup

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestWarmupBarsFromPlanUsesLargestIndicatorRequirement(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Max", overlay=true)
fast = ta.sma(close, 5)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(fast, signal)
    alert("go")`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlanForSymbol(plan, types.Interval1m, "US.AAPL")
	if err != nil {
		t.Fatalf("WarmupBarsFromPlanForSymbol() error = %v", err)
	}

	const want = 20 * tradingSessionMinutesPerDay
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlanForSymbol() = %d, want %d", warmupBars, want)
	}
}

func TestWarmupBarsFromPlanForSymbolUsesMarketTradingProfiles(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Markets", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	testCases := []struct {
		symbol string
		want   int
	}{
		{symbol: "US.AAPL", want: 20 * 390},
		{symbol: "HK.00700", want: 20 * 330},
		{symbol: "SH.600519", want: 20 * 240},
		{symbol: "SZ.000001", want: 20 * 240},
	}

	for _, tt := range testCases {
		warmupBars, warmupErr := WarmupBarsFromPlanForSymbol(plan, types.Interval1m, tt.symbol)
		if warmupErr != nil {
			t.Fatalf("WarmupBarsFromPlanForSymbol(%s) error = %v", tt.symbol, warmupErr)
		}
		if warmupBars != tt.want {
			t.Fatalf("WarmupBarsFromPlanForSymbol(%s) = %d, want %d", tt.symbol, warmupBars, tt.want)
		}
	}
}

func TestWarmupBarsFromPlanForSymbolUsesExtendedTradingDayWhenEnabled(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Extended", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 5))`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlanForSymbolWithOptions(plan, types.Interval1m, "US.AAPL", RuntimeOptions{IncludeExtendedHours: true})
	if err != nil {
		t.Fatalf("WarmupBarsFromPlanForSymbolWithOptions() error = %v", err)
	}

	const want = 5 * 24 * 60
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlanForSymbolWithOptions() = %d, want %d", warmupBars, want)
	}
}

func TestWarmupBarsFromPlanDoesNotApplyRuntimeSeriesFloor(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Small", overlay=true)
fast = ta.sma(close, 5)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlan(plan, types.Interval1m)
	if err != nil {
		t.Fatalf("WarmupBarsFromPlan() error = %v", err)
	}

	if warmupBars != 5 {
		t.Fatalf("WarmupBarsFromPlan() = %d, want 5", warmupBars)
	}
}

func TestWarmupBarsFromPlanHandlesDivergenceAndProtectLookback(t *testing.T) {
	program := indicatorTestProgram(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "signal", Expression: "rsi(14)"},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 2},
			Condition: "divergence_top(signal, 8)",
			Then: []strategyir.Statement{&strategyir.ProtectStmt{
				Range:                strategyir.SourceRange{StartLine: 3},
				Direction:            "auto",
				Mode:                 "stop_loss",
				TimeValueExpression:  "2",
				TimeUnit:             "hour",
				PercentageExpression: "1%",
				WindowPolicy:         "continuous",
			}},
		},
	)

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlan(plan, types.Interval5m)
	if err != nil {
		t.Fatalf("WarmupBarsFromPlan() error = %v", err)
	}

	const want = 24
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlan() = %d, want %d", warmupBars, want)
	}
}

func indicatorTestProgram(statements ...strategyir.Statement) *strategyir.Program {
	return &strategyir.Program{
		SourceFormat: strategypine.SourceFormatPineV6,
		Hooks: []strategyir.HookBlock{{
			Kind:       strategyir.HookKLineClose,
			Statements: statements,
		}},
	}
}
