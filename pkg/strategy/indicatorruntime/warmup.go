package indicatorruntime

import (
	"github.com/c9s/bbgo/pkg/types"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

// WarmupBarsFromScript parses a DSL strategy script and returns the minimum
// number of bars that should be loaded before the first replay bar.
func WarmupBarsFromScript(script string, interval types.Interval) (int, error) {
	program, err := strategydsl.ParseScript(script)
	if err != nil {
		return 0, err
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		return 0, err
	}
	return WarmupBarsFromPlan(plan, interval)
}

// WarmupBarsFromPlan returns the minimum number of bars that should be loaded
// before the first replay bar so the strategy's indicator requirements can be
// evaluated on the first replayed candle.
func WarmupBarsFromPlan(plan strategyir.Requirements, interval types.Interval) (int, error) {
	requirements, err := indicatorRequirementsFromPlan(plan)
	if err != nil {
		return 0, err
	}
	return calculateIndicatorWarmupBars(requirements, resolveIntervalMinutes(interval)), nil
}

func calculateIndicatorWarmupBars(requirements indicatorRequirements, intervalMinutes int) int {
	warmup := 0
	for _, config := range requirements.ma {
		warmup = max(warmup, resolveBarCount(config.period, config.timeUnit, intervalMinutes))
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
		warmup = max(warmup, resolveBarCount(config.timeValue, config.timeUnit, intervalMinutes))
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
