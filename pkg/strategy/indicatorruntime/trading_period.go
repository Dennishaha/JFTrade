package indicatorruntime

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func collectTradingPeriodUnits(requirements indicatorRequirements, intervalMinutes int, symbol string, includeExtendedHours bool) []string {
	if (len(requirements.ma) == 0 && len(requirements.securitySource) == 0) || strings.TrimSpace(symbol) == "" {
		return nil
	}
	dayMinutes, ok := market.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
	if !ok || intervalMinutes <= 0 || intervalMinutes >= dayMinutes {
		return nil
	}
	units := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, config := range requirements.ma {
		unit := normalizeIndicatorTimeUnit(config.timeUnit)
		switch unit {
		case "day", "week", "month":
		default:
			continue
		}
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		units = append(units, unit)
	}
	for _, config := range requirements.securitySource {
		unit := normalizeIndicatorTimeUnit(config.timeUnit)
		switch unit {
		case "day", "week", "month":
		default:
			continue
		}
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		units = append(units, unit)
	}
	return units
}

func calculateTradingPeriodSourceSnapshotWithLookback(opens, highs, lows, closes, volumes []float64, labelKeys []int64, source string, lookback int) (float64, float64, bool, bool) {
	if len(closes) == 0 || len(labelKeys) == 0 {
		return 0, 0, false, false
	}
	limit := min(len(closes), len(labelKeys))
	orderedKeys := make([]int64, 0, lookback+2)
	seen := map[int64]struct{}{}
	for index := limit - 1; index >= 0; index-- {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		orderedKeys = append(orderedKeys, key)
		if len(orderedKeys) >= lookback+2 {
			break
		}
	}
	if lookback < 0 || len(orderedKeys) <= lookback {
		return 0, 0, false, false
	}
	currentKey := orderedKeys[lookback]
	currentStart, currentEnd := periodBoundsForKey(labelKeys, currentKey, limit)
	current, currentOK := aggregateSourceWindow(opens, highs, lows, closes, volumes, currentStart, currentEnd, source)
	if len(orderedKeys) <= lookback+1 {
		return current, 0, currentOK, false
	}
	previousKey := orderedKeys[lookback+1]
	previousStart, previousEnd := periodBoundsForKey(labelKeys, previousKey, currentStart)
	previous, previousOK := aggregateSourceWindow(opens, highs, lows, closes, volumes, previousStart, previousEnd, source)
	return current, previous, currentOK, previousOK
}

func periodBoundsForKey(labelKeys []int64, key int64, upperBound int) (int, int) {
	limit := min(upperBound, len(labelKeys))
	start := limit
	end := limit
	hasEnd := false
	for index := limit - 1; index >= 0; index-- {
		if labelKeys[index] != key {
			if hasEnd {
				break
			}
			continue
		}
		if !hasEnd {
			end = index + 1
			hasEnd = true
		}
		start = index
	}
	if start == limit {
		return 0, 0
	}
	return start, end
}

func aggregateSourceWindow(opens, highs, lows, closes, volumes []float64, start, end int, source string) (float64, bool) {
	limit := min(len(opens), len(highs), len(lows), len(closes), len(volumes))
	if limit == 0 {
		return 0, false
	}
	start = max(start, 0)
	end = min(end, limit)
	if start >= end {
		return 0, false
	}
	openValue := opens[start]
	highValue := highs[start]
	lowValue := lows[start]
	closeValue := closes[end-1]
	volumeValue := 0.0
	for index := start; index < end; index++ {
		highValue = max(highValue, highs[index])
		lowValue = min(lowValue, lows[index])
		volumeValue += volumes[index]
	}
	return ohlcvSourceValue(source, openValue, highValue, lowValue, closeValue, volumeValue)
}

func aggregateFixedBarSeries(opens, highs, lows, closes, volumes []float64, windowBars int, source string) ([]float64, []float64, bool) {
	limit := min(len(opens), len(highs), len(lows), len(closes), len(volumes))
	if limit == 0 || windowBars <= 0 {
		return nil, nil, false
	}
	values := make([]float64, 0, int(math.Ceil(float64(limit)/float64(windowBars))))
	aggregatedVolumes := make([]float64, 0, cap(values))
	for start := 0; start < limit; start += windowBars {
		end := min(start+windowBars, limit)
		value, ok := aggregateSourceWindow(opens, highs, lows, closes, volumes, start, end, source)
		if !ok {
			continue
		}
		volume, volumeOK := aggregateSourceWindow(opens, highs, lows, closes, volumes, start, end, "volume")
		if !volumeOK {
			volume = 0
		}
		values = append(values, value)
		aggregatedVolumes = append(aggregatedVolumes, volume)
	}
	return values, aggregatedVolumes, len(values) > 0
}

func aggregateTimeBucketSeries(opens, highs, lows, closes, volumes []float64, endTimes []time.Time, targetMinutes int, source string) ([]float64, []float64, bool) {
	limit := min(len(opens), len(highs), len(lows), len(closes), len(volumes), len(endTimes))
	if limit == 0 || targetMinutes <= 0 {
		return nil, nil, false
	}
	values := make([]float64, 0)
	aggregatedVolumes := make([]float64, 0)
	bucketStart := 0
	currentBucket := timeframeBucketKey(endTimes[0], targetMinutes)
	for index := 1; index <= limit; index++ {
		if index < limit && timeframeBucketKey(endTimes[index], targetMinutes) == currentBucket {
			continue
		}
		value, ok := aggregateSourceWindow(opens, highs, lows, closes, volumes, bucketStart, index, source)
		if ok {
			volume, volumeOK := aggregateSourceWindow(opens, highs, lows, closes, volumes, bucketStart, index, "volume")
			if !volumeOK {
				volume = 0
			}
			values = append(values, value)
			aggregatedVolumes = append(aggregatedVolumes, volume)
		}
		if index < limit {
			bucketStart = index
			currentBucket = timeframeBucketKey(endTimes[index], targetMinutes)
		}
	}
	return values, aggregatedVolumes, len(values) > 0
}

func timeframeBucketKey(value time.Time, targetMinutes int) int64 {
	if targetMinutes <= 0 {
		return value.Unix()
	}
	return value.Unix() / int64(targetMinutes*60)
}

func (r *indicatorRuntime) appendTradingPeriodLabels(endTime time.Time) {
	if r == nil || len(r.tradingPeriodUnits) == 0 {
		return
	}
	for _, unit := range r.tradingPeriodUnits {
		labelStart, ok := market.TradingPeriodLabelStart(r.symbol, endTime, unit, r.includeExtendedHours)
		key := invalidTradingPeriodLabelKey
		if ok {
			key = labelStart.Unix()
		}
		r.tradingPeriodLabels[unit] = append(r.tradingPeriodLabels[unit], key)
	}
}

func selectTradingWindowSeriesWithCache(values, volumes []float64, endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) ([]float64, []float64) {
	if len(values) == 0 || len(values) != len(endTimes) {
		return nil, nil
	}
	selected := selectTradingWindowIndicesWithCache(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours, cache)
	return materializeTradingWindowSeriesFromSelected(values, volumes, selected, cache)
}

func selectTradingWindowIndicesWithCache(endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) []int {
	if cache == nil {
		return selectTradingWindowIndices(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	}
	selected := selectTradingWindowIndicesInto(cache.tradingWindowIndices, endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	cache.tradingWindowIndices = selected
	return selected
}

func materializeTradingWindowSeriesFromSelected(values, volumes []float64, selected []int, cache *snapshotSeriesCache) ([]float64, []float64) {
	if len(selected) == 0 {
		if cache != nil {
			cache.tradingWindowValues = cache.tradingWindowValues[:0]
			cache.tradingWindowVolumes = cache.tradingWindowVolumes[:0]
		}
		return nil, nil
	}
	windowValues := []float64(nil)
	windowVolumes := []float64(nil)
	if cache != nil {
		windowValues = cache.tradingWindowValues[:0]
		windowVolumes = cache.tradingWindowVolumes[:0]
	}
	if cap(windowValues) < len(selected) {
		windowValues = make([]float64, 0, len(selected))
	}
	if cap(windowVolumes) < len(selected) {
		windowVolumes = make([]float64, 0, len(selected))
	}
	for index := len(selected) - 1; index >= 0; index-- {
		position := selected[index]
		windowValues = append(windowValues, values[position])
		if position < len(volumes) {
			windowVolumes = append(windowVolumes, volumes[position])
		}
	}
	if cache != nil {
		cache.tradingWindowValues = windowValues
		cache.tradingWindowVolumes = windowVolumes
	}
	return windowValues, windowVolumes
}

func selectTradingWindowIndicesInto(destination []int, endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool) []int {
	if period <= 0 || upperBound <= 0 || len(endTimes) == 0 {
		return nil
	}
	limit := min(upperBound, len(endTimes))
	selected := destination[:0]
	if cap(selected) < limit {
		selected = make([]int, 0, limit)
	}
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	normalizedUnit := normalizeIndicatorTimeUnit(timeUnit)
	for index := limit - 1; index >= 0; index-- {
		labelStart, ok := market.TradingPeriodLabelStart(symbol, endTimes[index], normalizedUnit, includeExtendedHours)
		if !ok {
			continue
		}
		key := labelStart.Unix()
		if !hasKey || key != lastKey {
			if orderedKeys == period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		selected = append(selected, index)
	}
	return selected
}

func selectTradingWindowIndices(endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool) []int {
	if period <= 0 || upperBound <= 0 || len(endTimes) == 0 {
		return nil
	}
	limit := min(upperBound, len(endTimes))
	selected := make([]int, 0, limit)
	keys := make(map[string]struct{}, period)
	orderedKeys := 0
	normalizedUnit := normalizeIndicatorTimeUnit(timeUnit)
	for index := limit - 1; index >= 0; index-- {
		key, ok := market.TradingPeriodKey(symbol, endTimes[index], normalizedUnit, includeExtendedHours)
		if !ok {
			continue
		}
		if _, exists := keys[key]; !exists {
			if orderedKeys == period {
				break
			}
			keys[key] = struct{}{}
			orderedKeys++
		}
		selected = append(selected, index)
	}
	return selected
}

func usesTradingPeriodWindow(timeUnit string, intervalMinutes int, symbol string, endTimes []time.Time, includeExtendedHours bool) bool {
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "day", "week", "month":
	default:
		return false
	}
	if len(endTimes) == 0 || strings.TrimSpace(symbol) == "" {
		return false
	}
	dayMinutes, ok := market.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
	if !ok {
		return false
	}
	return intervalMinutes > 0 && intervalMinutes < dayMinutes
}

func usesFixedIntradayTimeframe(timeUnit string, intervalMinutes int) bool {
	if !isNumericMinuteTimeUnit(timeUnit) {
		return false
	}
	minutes, ok := indicatorTimeUnitMinutes(timeUnit)
	if !ok || intervalMinutes <= 0 {
		return false
	}
	return minutes >= intervalMinutes && minutes%intervalMinutes == 0
}

func isNumericMinuteTimeUnit(timeUnit string) bool {
	normalized := normalizeIndicatorTimeUnit(timeUnit)
	if !strings.HasSuffix(normalized, "m") {
		return false
	}
	_, err := strconv.Atoi(strings.TrimSuffix(normalized, "m"))
	return err == nil
}
