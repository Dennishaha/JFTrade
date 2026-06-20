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
	if config.kind == "anchored_vwap" {
		value, ok := r.anchoredVWAPStates[config].value()
		return cache.getScalarSnapshot(config.key, value, ok)
	}
	values := r.seriesForSource(config.source)
	if config.timeUnit != "" {
		aggregated, _, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok {
			return nil
		}
		values = aggregated
	}
	switch config.kind {
	case "rsi":
		value, ok := calculateRSIValueFromSeries(calculateRSISeries(values, config.period))
		return cache.getScalarSnapshot(config.key, value, ok)
	case "macd":
		return calculateMACDSnapshot(values, macdConfig{fastPeriod: config.period, slowPeriod: config.right, signalPeriod: config.offset})
	case "atr":
		highs, lows, closes, ok := r.fixedTimeframeOHLC(config.timeUnit)
		if !ok {
			return nil
		}
		value := calculateATR(highs, lows, closes, config.period)
		if typed, ok := value.(float64); ok {
			return cache.getScalarSnapshot(config.key, typed, true)
		}
		return nil
	case "bollinger":
		return calculateBollingerSnapshot(values, bollingerConfig{period: config.period, multiplier: config.multiplier})
	case "bbw":
		value, ok := calculateBollingerBandWidth(values, config.period, config.multiplier)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "cog":
		value, ok := calculateCenterOfGravity(values, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "supertrend":
		highs, lows, closes, ok := r.fixedTimeframeOHLC(config.timeUnit)
		if !ok {
			return nil
		}
		return calculateSupertrendSnapshot(highs, lows, closes, supertrendConfig{factor: config.multiplier, atrPeriod: config.period})
	case "linreg":
		value, ok := calculateLinearRegression(values, config.period, config.offset)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "pivothigh":
		value, ok := calculatePivot(values, config.left, config.right, true)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "pivotlow":
		value, ok := calculatePivot(values, config.left, config.right, false)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "alma":
		value, ok := calculateALMA(values, config.period, config.multiplier, config.parameter)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "cmo":
		value, ok := calculateCMO(values, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "tsi":
		value, ok := calculateTSI(values, config.period, config.right)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "correlation":
		values2 := r.seriesForSource(config.source2)
		if config.timeUnit != "" {
			aggregated, _, ok := r.fixedTimeframeSeries(config.timeUnit, config.source2)
			if !ok {
				return nil
			}
			values2 = aggregated
		}
		value, ok := calculateCorrelation(values, values2, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "dev":
		value, ok := calculateMeanDeviation(values, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "median":
		value, ok := calculateMedian(values, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "percentile_linear_interpolation":
		value, ok := calculatePercentileLinear(values, config.period, config.multiplier)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "percentile_nearest_rank":
		value, ok := calculatePercentileNearest(values, config.period, config.multiplier)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "percentrank":
		value, ok := calculatePercentRank(values, config.period)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "swma":
		value, ok := calculateSWMA(values)
		return cache.getScalarSnapshot(config.key, value, ok)
	case "obv":
		_, volumes, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok {
			return nil
		}
		current, previous, currentOK, previousOK := calculateOBVSnapshot(values, volumes)
		return cache.getSeriesSnapshot(config.key, current, previous, currentOK, previousOK)
	case "kc", "kcw":
		snapshot := r.calculateKeltnerSnapshot(config)
		if config.kind == "kcw" {
			if snapshot == nil {
				return nil
			}
			width := jftradeOptionalTypeAssertion[float64](snapshot["width"])
			return cache.getScalarSnapshot(config.key, width, true)
		}
		return snapshot
	default:
		return nil
	}
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
	for index := 0; index < period; index++ {
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
	lows, _, ok := r.fixedTimeframeSeries(timeUnit, "low")
	if !ok {
		return nil, nil, nil, false
	}
	closes, _, ok := r.fixedTimeframeSeries(timeUnit, "close")
	if !ok {
		return nil, nil, nil, false
	}
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
	if len(smoothed) == 0 || len(absSmoothed) == 0 {
		return 0, false
	}
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
	rank := int(math.Ceil((percentage / 100) * float64(period)))
	if rank < 1 {
		rank = 1
	}
	if rank > period {
		rank = period
	}
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
		highs, _, ok = r.fixedTimeframeSeries(config.timeUnit, "high")
		if !ok {
			return nil
		}
		lows, _, ok = r.fixedTimeframeSeries(config.timeUnit, "low")
		if !ok {
			return nil
		}
		closes, _, ok = r.fixedTimeframeSeries(config.timeUnit, "close")
		if !ok {
			return nil
		}
	}
	if len(values) < config.period || config.period <= 0 {
		return nil
	}
	basisSeries := calculateEMASequence(values, config.period)
	if len(basisSeries) == 0 {
		return nil
	}
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
	if len(rangeSeries) == 0 {
		return nil
	}
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
