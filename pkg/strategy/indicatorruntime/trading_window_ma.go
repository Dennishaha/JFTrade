package indicatorruntime

import (
	"time"
)

func buildMovingAverageSnapshot(values, volumes []float64, config movingAverageConfig, intervalMinutes int) map[string]any {
	return buildMovingAverageSnapshotForSymbol(values, volumes, nil, config, intervalMinutes, "", nil)
}

func buildMovingAverageSnapshotWithCache(values, volumes []float64, config movingAverageConfig, intervalMinutes int, cache *snapshotSeriesCache) map[string]any {
	return buildMovingAverageSnapshotForSymbol(values, volumes, nil, config, intervalMinutes, "", cache)
}

func buildMovingAverageSnapshotForSymbol(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, intervalMinutes int, symbol string, cache *snapshotSeriesCache) map[string]any {
	return snapshotValueToMap(
		movingAverageSnapshotForSymbol(values, volumes, endTimes, config, intervalMinutes, symbol, false, cache),
		[...]string{"value", "previous"},
	)
}

func movingAverageSnapshotForSymbol(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, intervalMinutes int, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) any {
	if usesTradingPeriodWindow(config.timeUnit, intervalMinutes, symbol, endTimes, includeExtendedHours) {
		return buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes, config, symbol, includeExtendedHours, cache)
	}
	effectiveConfig := config
	effectiveConfig.period = resolveBarCount(config.period, config.timeUnit, intervalMinutes)
	effectiveConfig.timeUnit = ""
	current, previous, currentOK, previousOK := calculateMovingAverageSnapshotValuesWithCache(values, volumes, effectiveConfig, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func buildMovingAverageSnapshotForTradingWindow(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) any {
	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes, endTimes, config, symbol, includeExtendedHours, cache); handled {
		if !currentOK {
			return nil
		}
		return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
	}
	current, currentOK := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, config, symbol, len(values), includeExtendedHours, cache)
	if !currentOK {
		return nil
	}
	previous, previousOK := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, config, symbol, max(len(values)-1, 0), includeExtendedHours, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func calculateTradingWindowMovingAverageSnapshotOnline(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool) (float64, float64, bool, bool, bool) {
	return calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes, endTimes, config, symbol, includeExtendedHours, nil)
}

func calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, float64, bool, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
		current, currentOK, handled := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, len(values))
		if !handled {
			return 0, 0, false, false, false
		}
		previous, previousOK, _ := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, max(len(values)-1, 0))
		return current, previous, currentOK, previousOK, true
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, 0, false, false, false
	}
	if len(values) == 0 || len(values) != len(endTimes) {
		return 0, 0, false, false, true
	}
	currentLimit := len(values)
	previousLimit := max(len(values)-1, 0)
	labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
	currentState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousState := tradingWindowMovingAverageState{aggregator: aggregator}
	for index := currentLimit - 1; index >= 0; index-- {
		if index >= len(labelKeys) {
			break
		}
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !currentState.done {
			currentState.push(config.period, key, values[index], volumes, index)
		}
		if index < previousLimit && !previousState.done {
			previousState.push(config.period, key, values[index], volumes, index)
		}
		if currentState.done && (previousLimit == 0 || previousState.done) {
			break
		}
	}
	current, currentOK := currentState.value()
	previous, previousOK := previousState.value()
	return current, previous, currentOK, previousOK, true
}

func calculateMovingAverageCurrentValue(values, volumes []float64, config movingAverageConfig) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	effectiveConfig := config
	effectiveConfig.period = len(values)
	effectiveConfig.timeUnit = ""
	current, _, currentOK, _ := calculateMovingAverageSnapshotValuesWithCache(values, volumes, effectiveConfig, nil)
	return current, currentOK
}

func calculateTradingWindowMovingAverageCurrentValue(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, bool) {
	if current, currentOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, symbol, upperBound, includeExtendedHours, cache); handled {
		return current, currentOK
	}
	selected := selectTradingWindowIndicesWithCache(endTimes, config.period, config.timeUnit, symbol, upperBound, includeExtendedHours, cache)
	if len(selected) == 0 {
		return 0, false
	}
	return calculateMovingAverageCurrentValueFromSelected(values, volumes, selected, config, cache)
}

func calculateTradingWindowMovingAverageCurrentValueOnline(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool) (float64, bool, bool) {
	return calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, symbol, upperBound, includeExtendedHours, nil)
}

func calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
		return calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, upperBound)
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, false, false
	}
	if len(values) == 0 || len(values) != len(endTimes) || upperBound <= 0 {
		return 0, false, true
	}
	limit := min(upperBound, len(endTimes))
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
	for index := limit - 1; index >= 0; index-- {
		if index >= len(labelKeys) {
			break
		}
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !hasKey || key != lastKey {
			if orderedKeys == config.period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		if !aggregator.push(values[index], volumes, index) {
			return 0, false, true
		}
	}
	current, currentOK := aggregator.value()
	return current, currentOK, true
}

func calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes []float64, labelKeys []int64, config movingAverageConfig) (float64, float64, bool, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		current, currentOK, handled := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, len(values))
		if !handled {
			return 0, 0, false, false, false
		}
		previous, previousOK, _ := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, max(len(values)-1, 0))
		return current, previous, currentOK, previousOK, true
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, 0, false, false, false
	}
	if len(values) == 0 || len(values) != len(labelKeys) {
		return 0, 0, false, false, true
	}
	currentState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousLimit := max(len(values)-1, 0)
	for index := len(values) - 1; index >= 0; index-- {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !currentState.done {
			currentState.push(config.period, key, values[index], volumes, index)
		}
		if index < previousLimit && !previousState.done {
			previousState.push(config.period, key, values[index], volumes, index)
		}
		if currentState.done && (previousLimit == 0 || previousState.done) {
			break
		}
	}
	current, currentOK := currentState.value()
	previous, previousOK := previousState.value()
	return current, previous, currentOK, previousOK, true
}

type tradingWindowSelectionSummary struct {
	startKey   int64
	startIndex int
	endIndex   int
	count      int
	valid      bool
}

func summarizeTradingWindowSelectionFromKeys(labelKeys []int64, period int, upperBound int) tradingWindowSelectionSummary {
	if period <= 0 || upperBound <= 0 || len(labelKeys) == 0 {
		return tradingWindowSelectionSummary{}
	}
	limit := min(upperBound, len(labelKeys))
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	summary := tradingWindowSelectionSummary{startIndex: limit, endIndex: -1}
	for index := limit - 1; index >= 0; index-- {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if summary.endIndex < 0 {
			summary.endIndex = index
		}
		if !hasKey || key != lastKey {
			if orderedKeys == period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		summary.startKey = key
		summary.startIndex = index
		summary.count++
	}
	if summary.count == 0 || summary.endIndex < 0 || summary.startIndex >= limit {
		return tradingWindowSelectionSummary{}
	}
	summary.valid = true
	return summary
}

func calculateEMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	alpha := 2 / float64(summary.count+1)
	current := 0.0
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		if count == 0 {
			current = values[index]
			count = 1
			continue
		}
		current = current + (values[index]-current)*alpha
		count++
	}
	if count != summary.count {
		return 0, false
	}
	return current, true
}

func calculateTradingWindowEMAValueFromKeys(values []float64, labelKeys []int64, period int, upperBound int) (float64, bool, bool) {
	if len(values) == 0 || len(values) != len(labelKeys) || upperBound <= 0 {
		return 0, false, true
	}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, period, upperBound)
	if !summary.valid {
		return 0, false, true
	}
	current, ok := calculateEMAFromTradingWindowSelection(values, labelKeys, summary)
	return current, ok, true
}

func calculateSMMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	sum := 0.0
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		sum += values[index]
		count++
	}
	if count != summary.count || count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

func calculateSingleValueFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count != 1 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		return values[index], true
	}
	return 0, false
}

func calculateHMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	if summary.count == 1 {
		return calculateSingleValueFromTradingWindowSelection(values, labelKeys, summary)
	}
	if summary.count != 2 {
		return 0, false
	}
	collected := [2]float64{}
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		if count >= len(collected) {
			return 0, false
		}
		collected[count] = values[index]
		count++
	}
	if count != summary.count {
		return 0, false
	}
	slowWMA := (collected[0] + 2*collected[1]) / 3
	return 2*collected[1] - slowWMA, true
}

func calculateTradingWindowSequenceValueFromKeys(values []float64, labelKeys []int64, averageType string, period int, upperBound int) (float64, bool, bool) {
	if len(values) == 0 || len(values) != len(labelKeys) || upperBound <= 0 {
		return 0, false, true
	}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, period, upperBound)
	if !summary.valid {
		return 0, false, true
	}
	switch averageType {
	case "EMA", "EXPMA":
		current, ok := calculateEMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "SMMA":
		current, ok := calculateSMMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "TMA":
		current, ok := calculateSingleValueFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "HMA":
		current, ok := calculateHMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	default:
		return 0, false, false
	}
}

func calculateMovingAverageCurrentValueFromSelected(values, volumes []float64, selected []int, config movingAverageConfig, cache *snapshotSeriesCache) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	period := len(selected)
	switch normalizeMovingAverageType(config.averageType) {
	case "LWMA":
		return linearWeightedMovingAverageFromSelected(values, selected, period)
	case "VWMA":
		return volumeWeightedMovingAverageFromSelected(values, volumes, selected)
	case "SMA", "BOLL", "MA":
		fallthrough
	default:
		if normalized := normalizeMovingAverageType(config.averageType); normalized != "EMA" && normalized != "EXPMA" && normalized != "SMMA" && normalized != "TMA" && normalized != "HMA" {
			return simpleMovingAverageFromSelected(values, selected)
		}
	}
	selectedValues, selectedVolumes := materializeTradingWindowSeriesFromSelected(values, volumes, selected, cache)
	return calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, config)
}

type tradingWindowMovingAverageAggregator struct {
	kind        string
	count       int
	sum         float64
	weightedSum float64
	volumeSum   float64
	missingData bool
}

type tradingWindowMovingAverageState struct {
	aggregator  tradingWindowMovingAverageAggregator
	orderedKeys int
	lastKey     int64
	hasKey      bool
	done        bool
}

func newTradingWindowMovingAverageAggregator(config movingAverageConfig) (tradingWindowMovingAverageAggregator, bool) {
	switch normalizeMovingAverageType(config.averageType) {
	case "SMA", "BOLL", "MA":
		return tradingWindowMovingAverageAggregator{kind: "sma"}, true
	case "LWMA":
		return tradingWindowMovingAverageAggregator{kind: "lwma"}, true
	case "VWMA":
		return tradingWindowMovingAverageAggregator{kind: "vwma"}, true
	default:
		return tradingWindowMovingAverageAggregator{}, false
	}
}

func (a *tradingWindowMovingAverageAggregator) push(value float64, volumes []float64, index int) bool {
	if a == nil {
		return false
	}
	switch a.kind {
	case "sma":
		a.sum += value
		a.count++
		return true
	case "lwma":
		a.weightedSum += a.sum + value
		a.sum += value
		a.count++
		return true
	case "vwma":
		if index >= len(volumes) {
			a.missingData = true
			return false
		}
		volume := volumes[index]
		a.weightedSum += value * volume
		a.volumeSum += volume
		a.count++
		return true
	default:
		return false
	}
}

func (a tradingWindowMovingAverageAggregator) value() (float64, bool) {
	if a.count == 0 || a.missingData {
		return 0, false
	}
	switch a.kind {
	case "sma":
		return a.sum / float64(a.count), true
	case "lwma":
		weightSum := float64(a.count * (a.count + 1) / 2)
		if weightSum == 0 {
			return 0, false
		}
		return a.weightedSum / weightSum, true
	case "vwma":
		if a.volumeSum == 0 {
			return 0, false
		}
		return a.weightedSum / a.volumeSum, true
	default:
		return 0, false
	}
}

func (s *tradingWindowMovingAverageState) push(period int, key int64, value float64, volumes []float64, index int) {
	if s == nil || s.done {
		return
	}
	if !s.hasKey || key != s.lastKey {
		if s.orderedKeys == period {
			s.done = true
			return
		}
		s.lastKey = key
		s.hasKey = true
		s.orderedKeys++
	}
	if !s.aggregator.push(value, volumes, index) {
		s.done = true
	}
}

func (s tradingWindowMovingAverageState) value() (float64, bool) {
	return s.aggregator.value()
}
