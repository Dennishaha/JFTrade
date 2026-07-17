package indicatorruntime

import (
	"math"
	"sort"
)

func newOBVStates(requirements indicatorRequirements) map[advancedIndicatorConfig]*rollingCumState {
	states := map[advancedIndicatorConfig]*rollingCumState{}
	for _, config := range requirements.advanced {
		if config.kind == "obv" && config.timeUnit == "" {
			states[config] = &rollingCumState{}
		}
	}
	return states
}

func (r *indicatorRuntime) pushOBVStates(openValue, high, low, closeValue, volume float64, previous map[string]float64, hasPrevious bool) {
	for config, state := range r.obvStates {
		current, ok := ohlcvSourceValue(config.source, openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.previous = state.current
		state.hasPrevious = state.hasCurrent
		if hasPrevious {
			switch previousValue := previous[config.source]; {
			case current > previousValue:
				state.current += volume
			case current < previousValue:
				state.current -= volume
			}
		}
		state.hasCurrent = true
	}
}

func (r *indicatorRuntime) advancedIndicatorSnapshot(config advancedIndicatorConfig, cache *snapshotSeriesCache) any {
	if snapshot, ok := r.anchoredVWAPSnapshot(config, cache); ok {
		return snapshot
	}
	values, ok := r.advancedSnapshotSeries(config)
	if !ok {
		return nil
	}
	switch config.kind {
	case "rsi":
		return r.advancedRSISnapshot(config, values, cache)
	case "macd":
		return calculateMACDSnapshot(values, macdConfig{fastPeriod: config.period, slowPeriod: config.right, signalPeriod: config.offset})
	case "atr":
		return r.advancedATRSnapshot(config, cache)
	case "bollinger":
		return calculateBollingerSnapshot(values, bollingerConfig{period: config.period, multiplier: config.multiplier})
	case "bbw", "cog", "linreg", "pivothigh", "pivotlow", "alma", "cmo", "tsi", "dev", "median",
		"percentile_linear_interpolation", "percentile_nearest_rank", "percentrank", "swma":
		return advancedScalarIndicatorSnapshot(config, values, cache)
	case "supertrend":
		return r.advancedSupertrendSnapshot(config)
	case "correlation":
		return r.advancedCorrelationSnapshot(config, values, cache)
	case "obv":
		return r.advancedOBVSnapshot(config, values, cache)
	case "kc", "kcw":
		return r.advancedKeltnerSnapshot(config, cache)
	default:
		return nil
	}
}

func advancedScalarIndicatorSnapshot(config advancedIndicatorConfig, values []float64, cache *snapshotSeriesCache) any {
	value, ok := advancedScalarIndicatorValue(config, values)
	return cache.getScalarSnapshot(config.key, value, ok)
}

func advancedScalarIndicatorValue(config advancedIndicatorConfig, values []float64) (float64, bool) {
	switch config.kind {
	case "bbw":
		return calculateBollingerBandWidth(values, config.period, config.multiplier)
	case "cog":
		return calculateCenterOfGravity(values, config.period)
	case "linreg":
		return calculateLinearRegression(values, config.period, config.offset)
	case "pivothigh":
		return calculatePivot(values, config.left, config.right, true)
	case "pivotlow":
		return calculatePivot(values, config.left, config.right, false)
	case "alma":
		return calculateALMA(values, config.period, config.multiplier, config.parameter)
	case "cmo":
		return calculateCMO(values, config.period)
	case "tsi":
		return calculateTSI(values, config.period, config.right)
	case "dev":
		return calculateMeanDeviation(values, config.period)
	case "median":
		return calculateMedian(values, config.period)
	case "percentile_linear_interpolation":
		return calculatePercentileLinear(values, config.period, config.multiplier)
	case "percentile_nearest_rank":
		return calculatePercentileNearest(values, config.period, config.multiplier)
	case "percentrank":
		return calculatePercentRank(values, config.period)
	case "swma":
		return calculateSWMA(values)
	default:
		return 0, false
	}
}

func (r *indicatorRuntime) advancedSupertrendSnapshot(config advancedIndicatorConfig) any {
	highs, lows, closes, ok := r.fixedTimeframeOHLC(config.timeUnit)
	if !ok {
		return nil
	}
	return calculateSupertrendSnapshot(highs, lows, closes, supertrendConfig{factor: config.multiplier, atrPeriod: config.period})
}

func (r *indicatorRuntime) anchoredVWAPSnapshot(config advancedIndicatorConfig, cache *snapshotSeriesCache) (any, bool) {
	if config.kind != "anchored_vwap" {
		return nil, false
	}
	value, ok := r.anchoredVWAPStates[config].value()
	return cache.getScalarSnapshot(config.key, value, ok), true
}

func (r *indicatorRuntime) advancedSnapshotSeries(config advancedIndicatorConfig) ([]float64, bool) {
	if config.timeUnit == "" {
		return r.seriesForSource(config.source), true
	}
	values, _, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
	return values, ok
}

func (r *indicatorRuntime) advancedRSISnapshot(config advancedIndicatorConfig, values []float64, cache *snapshotSeriesCache) any {
	value, ok := calculateRSIValueFromSeries(calculateRSISeries(values, config.period))
	return cache.getScalarSnapshot(config.key, value, ok)
}

func (r *indicatorRuntime) advancedATRSnapshot(config advancedIndicatorConfig, cache *snapshotSeriesCache) any {
	highs, lows, closes, ok := r.fixedTimeframeOHLC(config.timeUnit)
	if !ok {
		return nil
	}
	value := calculateATR(highs, lows, closes, config.period)
	typed, ok := value.(float64)
	if !ok {
		return nil
	}
	return cache.getScalarSnapshot(config.key, typed, true)
}

func (r *indicatorRuntime) advancedCorrelationSnapshot(config advancedIndicatorConfig, values []float64, cache *snapshotSeriesCache) any {
	values2, ok := r.advancedSourceSeries(config.timeUnit, config.source2)
	if !ok {
		return nil
	}
	value, valueOK := calculateCorrelation(values, values2, config.period)
	return cache.getScalarSnapshot(config.key, value, valueOK)
}

func (r *indicatorRuntime) advancedSourceSeries(timeUnit string, source string) ([]float64, bool) {
	if timeUnit == "" {
		return r.seriesForSource(source), true
	}
	values, _, ok := r.fixedTimeframeSeries(timeUnit, source)
	return values, ok
}

func (r *indicatorRuntime) advancedOBVSnapshot(config advancedIndicatorConfig, values []float64, cache *snapshotSeriesCache) any {
	_, volumes, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
	if !ok {
		return nil
	}
	current, previous, currentOK, previousOK := calculateOBVSnapshot(values, volumes)
	return cache.getSeriesSnapshot(config.key, current, previous, currentOK, previousOK)
}

func (r *indicatorRuntime) advancedKeltnerSnapshot(config advancedIndicatorConfig, cache *snapshotSeriesCache) any {
	snapshot := r.calculateKeltnerSnapshot(config)
	if config.kind != "kcw" || snapshot == nil {
		return snapshot
	}
	width := jftradeOptionalTypeAssertion[float64](snapshot["width"])
	return cache.getScalarSnapshot(config.key, width, true)
}

func calculateBollingerBandWidth(values []float64, period int, multiplier float64) (float64, bool) {
	if period <= 0 || multiplier <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	basis := 0.0
	for _, value := range window {
		basis += value
	}
	basis /= float64(period)
	if basis == 0 {
		return 0, false
	}
	variance := 0.0
	for _, value := range window {
		delta := value - basis
		variance += delta * delta
	}
	deviation := math.Sqrt(variance / float64(period))
	return 2 * multiplier * deviation / basis, true
}

func calculateCenterOfGravity(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	numerator, denominator := 0.0, 0.0
	for index := range period {
		value := window[period-1-index]
		numerator += value * float64(index+1)
		denominator += value
	}
	if denominator == 0 {
		return 0, false
	}
	return -numerator / denominator, true
}

func (r *indicatorRuntime) fixedTimeframeOHLC(timeUnit string) ([]float64, []float64, []float64, bool) {
	highs, _, ok := r.fixedTimeframeSeries(timeUnit, "high")
	if !ok {
		return nil, nil, nil, false
	}
	// fixedTimeframeSeries has the same completeness predicate for the three
	// canonical OHLC sources. Once high is available, low and close are too.
	lows, _, _ := r.fixedTimeframeSeries(timeUnit, "low")
	closes, _, _ := r.fixedTimeframeSeries(timeUnit, "close")
	return highs, lows, closes, true
}

func calculateCMO(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period+1 {
		return 0, false
	}
	start := len(values) - period
	gains, losses := 0.0, 0.0
	for index := start; index < len(values); index++ {
		change := values[index] - values[index-1]
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}
	denominator := gains + losses
	if denominator == 0 {
		return 0, true
	}
	return 100 * (gains - losses) / denominator, true
}

func calculateTSI(values []float64, shortPeriod, longPeriod int) (float64, bool) {
	if shortPeriod <= 0 || longPeriod <= 0 || len(values) < 2 {
		return 0, false
	}
	momentum := make([]float64, 0, len(values)-1)
	absMomentum := make([]float64, 0, len(values)-1)
	for index := 1; index < len(values); index++ {
		change := values[index] - values[index-1]
		momentum = append(momentum, change)
		absMomentum = append(absMomentum, math.Abs(change))
	}
	smoothed := calculateEMASequence(calculateEMASequence(momentum, shortPeriod), longPeriod)
	absSmoothed := calculateEMASequence(calculateEMASequence(absMomentum, shortPeriod), longPeriod)
	denominator := absSmoothed[len(absSmoothed)-1]
	if denominator == 0 {
		return 0, true
	}
	return 100 * smoothed[len(smoothed)-1] / denominator, true
}

func calculateCorrelation(left, right []float64, period int) (float64, bool) {
	limit := min(len(left), len(right))
	if period <= 0 || limit < period {
		return 0, false
	}
	leftWindow := left[limit-period : limit]
	rightWindow := right[limit-period : limit]
	leftMean, rightMean := 0.0, 0.0
	for index := range leftWindow {
		leftMean += leftWindow[index]
		rightMean += rightWindow[index]
	}
	leftMean /= float64(period)
	rightMean /= float64(period)
	numerator, leftSum, rightSum := 0.0, 0.0, 0.0
	for index := range leftWindow {
		leftDelta := leftWindow[index] - leftMean
		rightDelta := rightWindow[index] - rightMean
		numerator += leftDelta * rightDelta
		leftSum += leftDelta * leftDelta
		rightSum += rightDelta * rightDelta
	}
	denominator := math.Sqrt(leftSum * rightSum)
	if denominator == 0 {
		return 0, false
	}
	return numerator / denominator, true
}

func calculateMeanDeviation(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	mean := 0.0
	for _, value := range window {
		mean += value
	}
	mean /= float64(period)
	total := 0.0
	for _, value := range window {
		total += math.Abs(value - mean)
	}
	return total / float64(period), true
}

func calculateMedian(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := append([]float64(nil), values[len(values)-period:]...)
	sort.Float64s(window)
	mid := period / 2
	if period%2 == 1 {
		return window[mid], true
	}
	return (window[mid-1] + window[mid]) / 2, true
}

func calculatePercentileLinear(values []float64, period int, percentage float64) (float64, bool) {
	if period <= 0 || len(values) < period || percentage < 0 || percentage > 100 {
		return 0, false
	}
	window := append([]float64(nil), values[len(values)-period:]...)
	sort.Float64s(window)
	if period == 1 {
		return window[0], true
	}
	rank := (percentage / 100) * float64(period-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return window[lower], true
	}
	weight := rank - float64(lower)
	return window[lower] + (window[upper]-window[lower])*weight, true
}

func calculatePercentileNearest(values []float64, period int, percentage float64) (float64, bool) {
	if period <= 0 || len(values) < period || percentage < 0 || percentage > 100 {
		return 0, false
	}
	window := append([]float64(nil), values[len(values)-period:]...)
	sort.Float64s(window)
	rank := min(max(int(math.Ceil((percentage/100)*float64(period))), 1), period)
	return window[rank-1], true
}

func calculatePercentRank(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	if period == 1 {
		return 0, true
	}
	window := values[len(values)-period:]
	current := window[len(window)-1]
	count := 0
	for _, value := range window {
		if value <= current {
			count++
		}
	}
	return 100 * float64(count-1) / float64(period-1), true
}

func calculateSWMA(values []float64) (float64, bool) {
	if len(values) < 4 {
		return 0, false
	}
	window := values[len(values)-4:]
	return (window[0] + 2*window[1] + 2*window[2] + window[3]) / 6, true
}

func calculateOBVSnapshot(values, volumes []float64) (float64, float64, bool, bool) {
	limit := min(len(values), len(volumes))
	if limit == 0 {
		return 0, 0, false, false
	}
	current, previous := 0.0, 0.0
	for index := 1; index < limit; index++ {
		previous = current
		switch {
		case values[index] > values[index-1]:
			current += volumes[index]
		case values[index] < values[index-1]:
			current -= volumes[index]
		}
	}
	return current, previous, true, limit > 1
}

func calculateLinearRegression(values []float64, period, offset int) (float64, bool) {
	if period <= 0 || offset < 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	meanX := float64(period-1) / 2
	meanY := 0.0
	for _, value := range window {
		meanY += value
	}
	meanY /= float64(period)
	numerator, denominator := 0.0, 0.0
	for index, value := range window {
		dx := float64(index) - meanX
		numerator += dx * (value - meanY)
		denominator += dx * dx
	}
	slope := 0.0
	if denominator != 0 {
		slope = numerator / denominator
	}
	return meanY + slope*(float64(period-1-offset)-meanX), true
}

func calculatePivot(values []float64, left, right int, high bool) (float64, bool) {
	size := left + right + 1
	if left <= 0 || right <= 0 || len(values) < size {
		return 0, false
	}
	window := values[len(values)-size:]
	candidate := window[left]
	for index, value := range window {
		if index == left {
			continue
		}
		if high && value >= candidate {
			return 0, false
		}
		if !high && value <= candidate {
			return 0, false
		}
	}
	return candidate, true
}

func calculateALMA(values []float64, period int, offset, sigma float64) (float64, bool) {
	if period <= 0 || sigma <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	center := offset * float64(period-1)
	width := float64(period) / sigma
	weighted, weights := 0.0, 0.0
	for index, value := range window {
		distance := float64(index) - center
		weight := math.Exp(-(distance * distance) / (2 * width * width))
		weighted += value * weight
		weights += weight
	}
	if weights == 0 {
		return 0, false
	}
	return weighted / weights, true
}

func (r *indicatorRuntime) calculateKeltnerSnapshot(config advancedIndicatorConfig) map[string]any {
	values := r.seriesForSource(config.source)
	highs, lows, closes := r.highs, r.lows, r.closes
	if config.timeUnit != "" {
		var ok bool
		values, _, ok = r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok {
			return nil
		}
		// Fixed-timeframe OHLC sources share one completeness predicate. A
		// successful configured source therefore guarantees the canonical
		// high/low/close series below are available as well.
		highs, _, _ = r.fixedTimeframeSeries(config.timeUnit, "high")
		lows, _, _ = r.fixedTimeframeSeries(config.timeUnit, "low")
		closes, _, _ = r.fixedTimeframeSeries(config.timeUnit, "close")
	}
	if len(values) < config.period || config.period <= 0 {
		return nil
	}
	basisSeries := calculateEMASequence(values, config.period)
	ranges := make([]float64, len(closes))
	for index := range ranges {
		if config.useTR {
			previousClose := closes[index]
			if index > 0 {
				previousClose = closes[index-1]
			}
			ranges[index] = math.Max(highs[index]-lows[index], math.Max(math.Abs(highs[index]-previousClose), math.Abs(lows[index]-previousClose)))
		} else {
			ranges[index] = highs[index] - lows[index]
		}
	}
	rangeSeries := calculateEMASequence(ranges, config.period)
	basis := basisSeries[len(basisSeries)-1]
	band := rangeSeries[len(rangeSeries)-1] * config.multiplier
	width := 0.0
	if basis != 0 {
		width = (band * 2) / basis
	}
	return map[string]any{
		"value": basis,
		"basis": basis,
		"upper": basis + band,
		"lower": basis - band,
		"width": width,
	}
}
