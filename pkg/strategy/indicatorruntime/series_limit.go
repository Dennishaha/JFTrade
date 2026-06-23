package indicatorruntime

import (
	"strconv"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func ohlcvSourceValue(source string, openValue, high, low, closeValue, volume float64) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "open":
		return openValue, true
	case "high":
		return high, true
	case "low":
		return low, true
	case "close":
		return closeValue, true
	case "volume":
		return volume, true
	case "hl2":
		return (high + low) / 2, true
	case "hlc3":
		return (high + low + closeValue) / 3, true
	case "ohlc4":
		return (openValue + high + low + closeValue) / 4, true
	default:
		return 0, false
	}
}

func calculateIndicatorSeriesLimit(requirements indicatorRequirements, intervalMinutes int) int {
	limit := minimumIndicatorSeriesLimit
	for _, config := range requirements.ma {
		limit = max(limit, resolveBarCount(config.period, config.timeUnit, intervalMinutes)+1)
	}
	for _, config := range requirements.securitySource {
		limit = max(limit, resolveBarCount(config.lookback+2, config.timeUnit, intervalMinutes)+1)
	}
	for _, period := range requirements.rsi {
		limit = max(limit, period+1)
	}
	for _, config := range requirements.macd {
		limit = max(limit, config.slowPeriod+config.signalPeriod+1)
	}
	for _, config := range requirements.bollinger {
		limit = max(limit, config.period+1)
	}
	for _, period := range requirements.stdev {
		limit = max(limit, period+1)
	}
	for _, config := range requirements.windows {
		limit = max(limit, config.period+2)
	}
	if len(requirements.cum) > 0 {
		limit = max(limit, 2)
	}
	for _, config := range requirements.stoch {
		limit = max(limit, config.period+1)
	}
	for _, config := range requirements.kdj {
		limit = max(limit, config.period+config.m1+config.m2+1)
	}
	for _, period := range requirements.atr {
		limit = max(limit, period+2)
	}
	for _, period := range requirements.cci {
		limit = max(limit, period+1)
	}
	for _, period := range requirements.williamsR {
		limit = max(limit, period+1)
	}
	if len(requirements.sar) > 0 {
		limit = max(limit, 3)
	}
	for _, config := range requirements.stopLoss {
		limit = max(limit, resolveBarCount(config.timeValue, config.timeUnit, intervalMinutes)+1)
	}
	for _, config := range requirements.rsiDivergence {
		limit = max(limit, config.period+config.lookback+1)
	}
	for _, config := range requirements.macdDivergence {
		limit = max(limit, config.slowPeriod+config.signalPeriod+config.lookback+1)
	}
	for _, config := range requirements.kdjDivergence {
		limit = max(limit, config.period+config.m1+config.m2+config.lookback+1)
	}
	for _, config := range requirements.advanced {
		if config.kind == "anchored_vwap" {
			periodBars := tradingSessionMinutesPerDay
			switch config.timeUnit {
			case "week":
				periodBars = tradingSessionMinutesPerWeek
			case "month":
				periodBars = tradingSessionMinutesPerMonth
			}
			limit = max(limit, periodBars/max(intervalMinutes, 1)+2)
			continue
		}
		lookback := max(config.left+config.right+2, config.period+config.offset+2)
		if config.timeUnit != "" {
			lookback = resolveBarCount(lookback, config.timeUnit, intervalMinutes)
		}
		limit = max(limit, lookback)
	}
	return limit
}

func resolveIntervalMinutes(interval types.Interval) int {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	if value == "" {
		return 1
	}
	var unit string
	switch {
	case strings.HasSuffix(value, "mo"):
		unit = "mo"
		value = strings.TrimSuffix(value, "mo")
	case strings.HasSuffix(value, "min"):
		unit = "min"
		value = strings.TrimSuffix(value, "min")
	case strings.HasSuffix(value, "m"):
		unit = "m"
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "h"):
		unit = "h"
		value = strings.TrimSuffix(value, "h")
	case strings.HasSuffix(value, "d"):
		unit = "d"
		value = strings.TrimSuffix(value, "d")
	case strings.HasSuffix(value, "w"):
		unit = "w"
		value = strings.TrimSuffix(value, "w")
	default:
		return 1
	}
	amount, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || amount <= 0 {
		return 1
	}
	switch unit {
	case "min", "m":
		return amount
	case "h":
		return amount * 60
	case "d":
		return amount * tradingSessionMinutesPerDay
	case "w":
		return amount * tradingSessionMinutesPerWeek
	case "mo":
		return amount * tradingSessionMinutesPerMonth
	default:
		return 1
	}
}

func buildTradingPeriodLabels(destination []int64, endTimes []time.Time, symbol string, normalizedUnit string, includeExtendedHours bool) []int64 {
	if len(endTimes) == 0 {
		return nil
	}
	labels := reuseInt64Slice(destination, len(endTimes))
	for index, endTime := range endTimes {
		labelStart, ok := market.TradingPeriodLabelStart(symbol, endTime, normalizedUnit, includeExtendedHours)
		if !ok {
			labels[index] = invalidTradingPeriodLabelKey
			continue
		}
		labels[index] = labelStart.Unix()
	}
	return labels
}

func fillEMASequence(destination []float64, values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	sequence := reuseFloat64Slice(destination, len(values))
	multiplier := 2 / float64(period+1)
	previous := values[0]
	sequence[0] = previous
	for index := 1; index < len(values); index++ {
		previous = previous + (values[index]-previous)*multiplier
		sequence[index] = previous
	}
	return sequence
}
