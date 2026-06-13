package indicatorruntime

import "time"

func newSnapshotSeriesCache() *snapshotSeriesCache {
	return &snapshotSeriesCache{
		ema:                  map[int][]float64{},
		sma:                  map[int][]float64{},
		smma:                 map[int][]float64{},
		wma:                  map[int][]float64{},
		tma:                  map[int][]float64{},
		hma:                  map[int][]float64{},
		rsi:                  map[int][]float64{},
		macd:                 map[macdConfig]macdSeries{},
		kdj:                  map[kdjConfig]kdjSeries{},
		emaBuffers:           map[int][]float64{},
		macdBuffers:          map[macdConfig]macdSeries{},
		kdjBuffers:           map[kdjConfig]*reusableKDJSeriesBuffer{},
		tradingPeriodLabels:  map[tradingPeriodLabelCacheKey][]int64{},
		tradingPeriodBuffers: map[tradingPeriodLabelCacheKey][]int64{},
		maSnapshots:          map[movingAverageConfig]*indicatorSeriesSnapshot{},
		seriesSnapshots:      map[string]*indicatorSeriesSnapshot{},
		windowSnapshots:      map[windowConfig]*indicatorSeriesSnapshot{},
		macdSnapshots:        map[macdConfig]*indicatorMACDSnapshot{},
		kdjSnapshots:         map[kdjConfig]*indicatorKDJSnapshot{},
		scalarValues:         map[string]*indicatorScalarSnapshot{},
		stopLossSnapshots:    map[stopLossConfig]map[string]any{},
	}
}

func (c *snapshotSeriesCache) reset() {
	if c == nil {
		return
	}
	clearMap(c.ema)
	clearMap(c.sma)
	clearMap(c.smma)
	clearMap(c.wma)
	clearMap(c.tma)
	clearMap(c.hma)
	clearMap(c.rsi)
	clearMap(c.macd)
	clearMap(c.kdj)
	clearMap(c.tradingPeriodLabels)
	c.stopLossWindowStart.valid = false
	c.stopLossWindowSelect.valid = false
	c.stopLossWindowExtrema.valid = false
}

func (c *snapshotSeriesCache) getEMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateEMASequence(values, period)
	}
	if sequence, ok := c.ema[period]; ok {
		return sequence
	}
	sequence := fillEMASequence(c.emaBuffers[period], values, period)
	c.emaBuffers[period] = sequence
	c.ema[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getSMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateSMASequence(values, period)
	}
	if sequence, ok := c.sma[period]; ok {
		return sequence
	}
	sequence := calculateSMASequence(values, period)
	c.sma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getSMMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateSMMASequence(values, period)
	}
	if sequence, ok := c.smma[period]; ok {
		return sequence
	}
	sequence := calculateSMMASequence(values, period)
	c.smma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getWMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateWMASequence(values, period)
	}
	if sequence, ok := c.wma[period]; ok {
		return sequence
	}
	sequence := calculateWMASequence(values, period)
	c.wma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getTradingPeriodLabels(endTimes []time.Time, symbol string, unit string, includeExtendedHours bool) []int64 {
	normalizedUnit := normalizeIndicatorTimeUnit(unit)
	if c == nil {
		return buildTradingPeriodLabels(nil, endTimes, symbol, normalizedUnit, includeExtendedHours)
	}
	cacheKey := tradingPeriodLabelCacheKey{
		symbol:               symbol,
		unit:                 normalizedUnit,
		includeExtendedHours: includeExtendedHours,
	}
	if labels, ok := c.tradingPeriodLabels[cacheKey]; ok {
		return labels
	}
	labels := buildTradingPeriodLabels(c.tradingPeriodBuffers[cacheKey], endTimes, symbol, normalizedUnit, includeExtendedHours)
	c.tradingPeriodBuffers[cacheKey] = labels
	c.tradingPeriodLabels[cacheKey] = labels
	return labels
}

func (c *snapshotSeriesCache) getTMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateTMASequence(values, period)
	}
	if sequence, ok := c.tma[period]; ok {
		return sequence
	}
	sequence := calculateTMASequenceWithCache(values, period, c)
	c.tma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getHMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateHMASequence(values, period)
	}
	if sequence, ok := c.hma[period]; ok {
		return sequence
	}
	sequence := calculateHMASequenceWithCache(values, period, c)
	c.hma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getRSISeries(closes []float64, period int) []float64 {
	if c == nil {
		return calculateRSISeries(closes, period)
	}
	if series, ok := c.rsi[period]; ok {
		return series
	}
	series := calculateRSISeries(closes, period)
	c.rsi[period] = series
	return series
}

func (c *snapshotSeriesCache) getMACDSeries(closes []float64, config macdConfig) macdSeries {
	if c == nil {
		return calculateMACDSeries(closes, config)
	}
	if series, ok := c.macd[config]; ok {
		return series
	}
	series := calculateMACDSeriesWithCache(closes, config, c)
	c.macdBuffers[config] = series
	c.macd[config] = series
	return series
}

func (c *snapshotSeriesCache) getKDJSeries(highs, lows, closes []float64, config kdjConfig) kdjSeries {
	if c == nil {
		kValues, dValues, jValues := calculateKDJSeries(highs, lows, closes, config)
		return kdjSeries{k: kValues, d: dValues, j: jValues}
	}
	if series, ok := c.kdj[config]; ok {
		return series
	}
	buffer, ok := c.kdjBuffers[config]
	if !ok {
		buffer = &reusableKDJSeriesBuffer{}
		c.kdjBuffers[config] = buffer
	}
	series := calculateKDJSeriesWithBuffer(buffer, highs, lows, closes, config)
	c.kdj[config] = series
	return series
}

func (c *snapshotSeriesCache) getScalarSnapshot(key string, current float64, currentOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		return &indicatorScalarSnapshot{current: current, hasCurrent: true}
	}
	snapshot, ok := c.scalarValues[key]
	if !ok {
		snapshot = &indicatorScalarSnapshot{}
		c.scalarValues[key] = snapshot
	}
	snapshot.current = current
	snapshot.hasCurrent = true
	return snapshot
}

func (c *snapshotSeriesCache) getSeriesSnapshot(key string, current, previous float64, currentOK, previousOK bool) any {
	if !currentOK && !previousOK {
		return nil
	}
	if c == nil {
		return &indicatorSeriesSnapshot{current: current, previous: previous, hasCurrent: currentOK, hasPrevious: previousOK}
	}
	snapshot, ok := c.seriesSnapshots[key]
	if !ok {
		snapshot = &indicatorSeriesSnapshot{}
		c.seriesSnapshots[key] = snapshot
	}
	snapshot.current = current
	snapshot.previous = previous
	snapshot.hasCurrent = currentOK
	snapshot.hasPrevious = previousOK
	return snapshot
}

func (c *snapshotSeriesCache) getStopLossSnapshot(config stopLossConfig) map[string]any {
	if c == nil {
		return make(map[string]any, 18)
	}
	snapshot, ok := c.stopLossSnapshots[config]
	if !ok {
		snapshot = make(map[string]any, 18)
		c.stopLossSnapshots[config] = snapshot
	}
	return snapshot
}

func (c *snapshotSeriesCache) getMovingAverageSnapshot(config movingAverageConfig, current, previous float64, currentOK, previousOK bool) any {
	if !currentOK && !previousOK {
		return nil
	}
	if c == nil {
		return &indicatorSeriesSnapshot{current: current, previous: previous, hasCurrent: currentOK, hasPrevious: previousOK}
	}
	snapshot, ok := c.maSnapshots[config]
	if !ok {
		snapshot = &indicatorSeriesSnapshot{}
		c.maSnapshots[config] = snapshot
	}
	snapshot.current = current
	snapshot.previous = previous
	snapshot.hasCurrent = currentOK
	snapshot.hasPrevious = previousOK
	return snapshot
}

func (c *snapshotSeriesCache) getWindowSnapshot(config windowConfig, current, previous float64, currentOK, previousOK bool) any {
	if !currentOK && !previousOK {
		return nil
	}
	if c == nil {
		return &indicatorSeriesSnapshot{current: current, previous: previous, hasCurrent: currentOK, hasPrevious: previousOK}
	}
	snapshot, ok := c.windowSnapshots[config]
	if !ok {
		snapshot = &indicatorSeriesSnapshot{}
		c.windowSnapshots[config] = snapshot
	}
	snapshot.current = current
	snapshot.previous = previous
	snapshot.hasCurrent = currentOK
	snapshot.hasPrevious = previousOK
	return snapshot
}

func (c *snapshotSeriesCache) getMACDSnapshot(config macdConfig, series macdSeries) any {
	if len(series.diff) == 0 || len(series.signal) == 0 {
		return nil
	}
	currentIndex := len(series.diff) - 1
	currentDiff := series.diff[currentIndex]
	currentSignal := series.signal[currentIndex]
	previousDiff := 0.0
	previousSignal := 0.0
	previousOK := currentIndex > 0
	if previousOK {
		previousIndex := currentIndex - 1
		previousDiff = series.diff[previousIndex]
		previousSignal = series.signal[previousIndex]
	}
	return c.getMACDSnapshotValues(config, currentDiff, currentSignal, previousDiff, previousSignal, true, previousOK)
}

func (c *snapshotSeriesCache) getMACDSnapshotValues(config macdConfig, currentDiff, currentSignal, previousDiff, previousSignal float64, currentOK, previousOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		snapshot := &indicatorMACDSnapshot{
			diff:      currentDiff,
			signal:    currentSignal,
			histogram: (currentDiff - currentSignal) * 2,
		}
		if previousOK {
			snapshot.previousDiff = previousDiff
			snapshot.previousSignal = previousSignal
			snapshot.previousHistogram = (previousDiff - previousSignal) * 2
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.macdSnapshots[config]
	if !ok {
		snapshot = &indicatorMACDSnapshot{}
		c.macdSnapshots[config] = snapshot
	}
	snapshot.diff = currentDiff
	snapshot.signal = currentSignal
	snapshot.histogram = (currentDiff - currentSignal) * 2
	if previousOK {
		snapshot.previousDiff = previousDiff
		snapshot.previousSignal = previousSignal
		snapshot.previousHistogram = (previousDiff - previousSignal) * 2
		snapshot.hasPrevious = true
	} else {
		snapshot.previousDiff = 0
		snapshot.previousSignal = 0
		snapshot.previousHistogram = 0
		snapshot.hasPrevious = false
	}
	return snapshot
}

func (c *snapshotSeriesCache) getKDJSnapshot(config kdjConfig, series kdjSeries) any {
	if len(series.k) == 0 || len(series.d) == 0 || len(series.j) == 0 {
		return nil
	}
	last := len(series.k) - 1
	if c == nil {
		snapshot := &indicatorKDJSnapshot{k: series.k[last], d: series.d[last], j: series.j[last]}
		if last > 0 {
			snapshot.previousK = series.k[last-1]
			snapshot.previousD = series.d[last-1]
			snapshot.previousJ = series.j[last-1]
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.kdjSnapshots[config]
	if !ok {
		snapshot = &indicatorKDJSnapshot{}
		c.kdjSnapshots[config] = snapshot
	}
	snapshot.k = series.k[last]
	snapshot.d = series.d[last]
	snapshot.j = series.j[last]
	if last > 0 {
		snapshot.previousK = series.k[last-1]
		snapshot.previousD = series.d[last-1]
		snapshot.previousJ = series.j[last-1]
		snapshot.hasPrevious = true
	} else {
		snapshot.previousK = 0
		snapshot.previousD = 0
		snapshot.previousJ = 0
		snapshot.hasPrevious = false
	}
	return snapshot
}

func (c *snapshotSeriesCache) getKDJSnapshotValues(config kdjConfig, currentK, currentD, currentJ, previousK, previousD, previousJ float64, currentOK, previousOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		snapshot := &indicatorKDJSnapshot{k: currentK, d: currentD, j: currentJ}
		if previousOK {
			snapshot.previousK = previousK
			snapshot.previousD = previousD
			snapshot.previousJ = previousJ
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.kdjSnapshots[config]
	if !ok {
		snapshot = &indicatorKDJSnapshot{}
		c.kdjSnapshots[config] = snapshot
	}
	snapshot.k = currentK
	snapshot.d = currentD
	snapshot.j = currentJ
	if previousOK {
		snapshot.previousK = previousK
		snapshot.previousD = previousD
		snapshot.previousJ = previousJ
		snapshot.hasPrevious = true
	} else {
		snapshot.previousK = 0
		snapshot.previousD = 0
		snapshot.previousJ = 0
		snapshot.hasPrevious = false
	}
	return snapshot
}
