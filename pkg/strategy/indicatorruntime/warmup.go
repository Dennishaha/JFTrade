package indicatorruntime

import (
	"math"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

// WarmupBarsFromScript parses a Pine strategy script and returns the minimum
// number of bars that should be loaded before the first replay bar.
func WarmupBarsFromScript(script string, interval types.Interval) (int, error) {
	return WarmupBarsFromScriptForSymbol(script, interval, "")
}

func WarmupBarsFromScriptForSymbol(script string, interval types.Interval, symbol string) (int, error) {
	return WarmupBarsFromScriptForSymbolWithOptions(script, interval, symbol, RuntimeOptions{})
}

func WarmupBarsFromScriptForSymbolWithOptions(script string, interval types.Interval, symbol string, options RuntimeOptions) (int, error) {
	program, err := strategypine.ParseScript(script)
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
	for _, config := range requirements.securitySource {
		warmup = max(warmup, estimateTradingPeriodBars(config.lookback+2, config.timeUnit, intervalMinutes, symbol, includeExtendedHours))
	}
	for _, period := range requirements.rsi {
		warmup = max(warmup, period)
	}
	for _, config := range requirements.rsiSource {
		warmup = max(warmup, config.period)
	}
	for _, config := range requirements.macd {
		warmup = max(warmup, config.slowPeriod+config.signalPeriod)
	}
	for _, config := range requirements.bollinger {
		warmup = max(warmup, config.period)
	}
	for _, period := range requirements.stdev {
		warmup = max(warmup, period)
	}
	for _, config := range requirements.stdevSource {
		warmup = max(warmup, config.period)
	}
	for _, config := range requirements.variance {
		warmup = max(warmup, config.period)
	}
	for _, config := range requirements.windows {
		switch config.function {
		case "change", "mom", "roc", "rising", "falling":
			warmup = max(warmup, config.period+1)
		default:
			warmup = max(warmup, config.period)
		}
	}
	if len(requirements.cum) > 0 {
		warmup = max(warmup, 1)
	}
	for _, config := range requirements.stoch {
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
	for _, config := range requirements.cciSource {
		warmup = max(warmup, config.period)
	}
	for _, period := range requirements.williamsR {
		warmup = max(warmup, period)
	}
	for _, config := range requirements.mfi {
		warmup = max(warmup, config.period+1)
	}
	for _, config := range requirements.dmi {
		warmup = max(warmup, config.diLength+config.adxSmoothing+1)
	}
	for _, config := range requirements.supertrend {
		warmup = max(warmup, config.atrPeriod+1)
	}
	if len(requirements.sar) > 0 {
		warmup = max(warmup, 2)
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
	minutesPerDay, ok := market.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
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
