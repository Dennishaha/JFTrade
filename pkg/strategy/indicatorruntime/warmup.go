package indicatorruntime

import (
	"math"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

// WarmupBarsFromScript parses a DSL strategy script and returns the minimum
// number of bars that should be loaded before the first replay bar.
func WarmupBarsFromScript(script string, interval types.Interval) (int, error) {
	return WarmupBarsFromScriptForSymbol(script, interval, "")
}

func WarmupBarsFromScriptForSymbol(script string, interval types.Interval, symbol string) (int, error) {
	return WarmupBarsFromScriptForSymbolWithOptions(script, interval, symbol, RuntimeOptions{})
}

func WarmupBarsFromScriptForSymbolWithOptions(script string, interval types.Interval, symbol string, options RuntimeOptions) (int, error) {
	program, err := strategydsl.ParseScript(script)
	if err != nil {
		return 0, err
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		return 0, err
	}
	return WarmupBarsFromPlanForSymbolWithOptions(plan, interval, symbol, options)
}

// WarmupBarsFromPlan returns the minimum number of bars that should be loaded
// before the first replay bar so the strategy's indicator requirements can be
// evaluated on the first replayed candle.
func WarmupBarsFromPlan(plan strategyir.Requirements, interval types.Interval) (int, error) {
	return WarmupBarsFromPlanForSymbol(plan, interval, "")
}

func WarmupBarsFromPlanForSymbol(plan strategyir.Requirements, interval types.Interval, symbol string) (int, error) {
	return WarmupBarsFromPlanForSymbolWithOptions(plan, interval, symbol, RuntimeOptions{})
}

func WarmupBarsFromPlanForSymbolWithOptions(plan strategyir.Requirements, interval types.Interval, symbol string, options RuntimeOptions) (int, error) {
	requirements, err := indicatorRequirementsFromPlan(plan)
	if err != nil {
		return 0, err
	}
	return calculateIndicatorWarmupBars(requirements, resolveIntervalMinutes(interval), symbol, options.IncludeExtendedHours), nil
}

func calculateIndicatorWarmupBars(requirements indicatorRequirements, intervalMinutes int, symbol string, includeExtendedHours bool) int {
	warmup := 0
	for _, config := range requirements.ma {
		warmup = max(warmup, estimateTradingPeriodBars(config.period, config.timeUnit, intervalMinutes, symbol, includeExtendedHours))
	}
	for _, period := range requirements.rsi {
		warmup = max(warmup, period)
	}
	for _, config := range requirements.macd {
		warmup = max(warmup, config.slowPeriod+config.signalPeriod)
	}
	for _, config := range requirements.bollinger {
		warmup = max(warmup, config.period)
	}
	for _, config := range requirements.kdj {
		warmup = max(warmup, config.period+config.m1+config.m2)
	}
	for _, period := range requirements.atr {
		warmup = max(warmup, period+1)
	}
	for _, period := range requirements.cci {
		warmup = max(warmup, period)
	}
	for _, period := range requirements.williamsR {
		warmup = max(warmup, period)
	}
	for _, config := range requirements.stopLoss {
		warmup = max(warmup, estimateTradingPeriodBars(config.timeValue, config.timeUnit, intervalMinutes, symbol, includeExtendedHours))
	}
	for _, config := range requirements.rsiDivergence {
		warmup = max(warmup, config.period+config.lookback)
	}
	for _, config := range requirements.macdDivergence {
		warmup = max(warmup, config.slowPeriod+config.signalPeriod+config.lookback)
	}
	for _, config := range requirements.kdjDivergence {
		warmup = max(warmup, config.period+config.m1+config.m2+config.lookback)
	}
	return warmup
}

func estimateTradingPeriodBars(period int, timeUnit string, intervalMinutes int, symbol string, includeExtendedHours bool) int {
	if period <= 0 {
		return 0
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	minutesPerDay, ok := futu.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
	if !ok {
		return resolveBarCount(period, timeUnit, intervalMinutes)
	}
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "day":
		return max(1, int(math.Ceil(float64(period*minutesPerDay)/float64(intervalMinutes))))
	case "week":
		return max(1, int(math.Ceil(float64(period*minutesPerDay*5)/float64(intervalMinutes))))
	case "month":
		return max(1, int(math.Ceil(float64(period*minutesPerDay*20)/float64(intervalMinutes))))
	default:
		return resolveBarCount(period, timeUnit, intervalMinutes)
	}
}
