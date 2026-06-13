package indicatorruntime

import (
	"sort"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func (r *indicatorRuntime) oldSourceValuesAt(index int) map[string]float64 {
	values := map[string]float64{}
	if r == nil || index < 0 {
		return values
	}
	if index < len(r.opens) {
		values["open"] = r.opens[index]
	}
	if index < len(r.highs) {
		values["high"] = r.highs[index]
	}
	if index < len(r.lows) {
		values["low"] = r.lows[index]
	}
	if index < len(r.closes) {
		values["close"] = r.closes[index]
	}
	if index < len(r.volumes) {
		values["volume"] = r.volumes[index]
	}
	if index < len(r.highs) && index < len(r.lows) {
		values["hl2"] = (r.highs[index] + r.lows[index]) / 2
	}
	if index < len(r.highs) && index < len(r.lows) && index < len(r.closes) {
		values["hlc3"] = (r.highs[index] + r.lows[index] + r.closes[index]) / 3
	}
	if index < len(r.opens) && index < len(r.highs) && index < len(r.lows) && index < len(r.closes) {
		values["ohlc4"] = (r.opens[index] + r.highs[index] + r.lows[index] + r.closes[index]) / 4
	}
	return values
}

func appendTailValue(values []float64, value float64, limit int) []float64 {
	if limit <= 0 {
		return values[:0]
	}
	if len(values) < limit {
		return append(values, value)
	}
	copy(values, values[1:])
	values[len(values)-1] = value
	return values
}

func sortedLookbacks(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	result := make([]int, 0, len(values))
	for lookback := range values {
		if lookback > 0 {
			result = append(result, lookback)
		}
	}
	sort.Ints(result)
	return result
}

func (r *indicatorRuntime) seriesForSource(source string) []float64 {
	if r == nil {
		return nil
	}
	switch normalizeSourceOrClose(source) {
	case "open":
		return r.opens
	case "high":
		return r.highs
	case "low":
		return r.lows
	case "volume":
		return r.volumes
	case "hl2":
		return derivedSeriesHL2(r.highs, r.lows)
	case "hlc3":
		return derivedSeriesHLC3(r.highs, r.lows, r.closes)
	case "ohlc4":
		return derivedSeriesOHLC4(r.opens, r.highs, r.lows, r.closes)
	default:
		return r.closes
	}
}

func (r *indicatorRuntime) securitySourceSnapshotValues(config securitySourceConfig, cache *snapshotSeriesCache) (float64, float64, bool, bool) {
	if r == nil || len(r.closes) == 0 {
		return 0, 0, false, false
	}
	if usesTradingPeriodWindow(config.timeUnit, r.intervalMinutes, r.symbol, r.endTimes, r.includeExtendedHours) {
		labelKeys := r.tradingPeriodLabels[normalizeIndicatorTimeUnit(config.timeUnit)]
		if len(labelKeys) != len(r.endTimes) || len(labelKeys) == 0 {
			if cache != nil {
				labelKeys = cache.getTradingPeriodLabels(r.endTimes, r.symbol, config.timeUnit, r.includeExtendedHours)
			} else {
				labelKeys = buildTradingPeriodLabels(nil, r.endTimes, r.symbol, config.timeUnit, r.includeExtendedHours)
			}
		}
		return calculateTradingPeriodSourceSnapshotWithLookback(r.opens, r.highs, r.lows, r.closes, r.volumes, labelKeys, config.source, config.lookback)
	}
	if usesFixedIntradayTimeframe(config.timeUnit, r.intervalMinutes) {
		values, _, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok || len(values) == 0 {
			return 0, 0, false, false
		}
		currentIndex := len(values) - 1 - config.lookback
		if currentIndex < 0 {
			return 0, 0, false, false
		}
		current := values[currentIndex]
		previousIndex := currentIndex - 1
		if previousIndex < 0 {
			return current, 0, true, false
		}
		return current, values[previousIndex], true, true
	}
	windowBars := resolveBarCount(1, config.timeUnit, r.intervalMinutes)
	currentEnd := max(len(r.closes)-windowBars*config.lookback, 0)
	currentStart := max(currentEnd-windowBars, 0)
	current, currentOK := aggregateSourceWindow(r.opens, r.highs, r.lows, r.closes, r.volumes, currentStart, currentEnd, config.source)
	previousEnd := currentStart
	previousStart := max(previousEnd-windowBars, 0)
	previous, previousOK := aggregateSourceWindow(r.opens, r.highs, r.lows, r.closes, r.volumes, previousStart, previousEnd, config.source)
	return current, previous, currentOK, previousOK
}

func (r *indicatorRuntime) fixedTimeframeSeries(timeUnit string, source string) ([]float64, []float64, bool) {
	if r == nil {
		return nil, nil, false
	}
	targetMinutes, ok := indicatorTimeUnitMinutes(timeUnit)
	if !ok || r.intervalMinutes <= 0 || targetMinutes < r.intervalMinutes || targetMinutes%r.intervalMinutes != 0 {
		return nil, nil, false
	}
	if targetMinutes == r.intervalMinutes {
		return r.seriesForSource(source), r.volumes, len(r.closes) > 0
	}
	if !hasUsableEndTimes(r.endTimes) {
		return aggregateFixedBarSeries(r.opens, r.highs, r.lows, r.closes, r.volumes, targetMinutes/r.intervalMinutes, source)
	}
	return aggregateTimeBucketSeries(r.opens, r.highs, r.lows, r.closes, r.volumes, r.endTimes, targetMinutes, source)
}

func hasUsableEndTimes(values []time.Time) bool {
	for _, value := range values {
		if !value.IsZero() {
			return true
		}
	}
	return false
}

func derivedSeriesHL2(highs, lows []float64) []float64 {
	limit := min(len(highs), len(lows))
	values := make([]float64, limit)
	for index := 0; index < limit; index++ {
		values[index] = (highs[index] + lows[index]) / 2
	}
	return values
}

func derivedSeriesHLC3(highs, lows, closes []float64) []float64 {
	limit := min(len(highs), len(lows), len(closes))
	values := make([]float64, limit)
	for index := 0; index < limit; index++ {
		values[index] = (highs[index] + lows[index] + closes[index]) / 3
	}
	return values
}

func derivedSeriesOHLC4(opens, highs, lows, closes []float64) []float64 {
	limit := min(len(opens), len(highs), len(lows), len(closes))
	values := make([]float64, limit)
	for index := 0; index < limit; index++ {
		values[index] = (opens[index] + highs[index] + lows[index] + closes[index]) / 4
	}
	return values
}

func classifyKLineSession(symbol string, kline types.KLine) market.Session {
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return market.SessionUnknown
	}
	return market.ClassifySession(resolvedSymbol, observedAt)
}

func simpleMovingAverageFromSelected(values []float64, selected []int) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	sum := 0.0
	for index := len(selected) - 1; index >= 0; index-- {
		sum += values[selected[index]]
	}
	return sum / float64(len(selected)), true
}

func exponentialMovingAverageFromSelected(values []float64, selected []int, period int) (float64, bool) {
	if period <= 0 || len(selected) < period {
		return 0, false
	}
	multiplier := 2 / float64(period+1)
	current := values[selected[len(selected)-1]]
	for index := len(selected) - 2; index >= 0; index-- {
		current = current + (values[selected[index]]-current)*multiplier
	}
	return current, true
}

func linearWeightedMovingAverageFromSelected(values []float64, selected []int, period int) (float64, bool) {
	if period <= 0 || len(selected) < period {
		return 0, false
	}
	weightSum := float64(period * (period + 1) / 2)
	weightedSum := 0.0
	weight := 1.0
	for index := len(selected) - 1; index >= 0; index-- {
		weightedSum += values[selected[index]] * weight
		weight++
	}
	return weightedSum / weightSum, true
}

func volumeWeightedMovingAverageFromSelected(values, volumes []float64, selected []int) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	volumeSum := 0.0
	weightedSum := 0.0
	for index := len(selected) - 1; index >= 0; index-- {
		position := selected[index]
		if position >= len(volumes) {
			return 0, false
		}
		volume := volumes[position]
		volumeSum += volume
		weightedSum += values[position] * volume
	}
	if volumeSum == 0 {
		return 0, false
	}
	return weightedSum / volumeSum, true
}

func snapshotValueToMap(snapshot any, keys [2]string) map[string]any {
	if snapshot == nil {
		return nil
	}
	if values, ok := snapshot.(map[string]any); ok {
		return values
	}
	reader, ok := snapshot.(interface{ FieldValue(string) (any, bool) })
	if !ok {
		return nil
	}
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := reader.FieldValue(key)
		if ok {
			result[key] = value
		}
	}
	return result
}
