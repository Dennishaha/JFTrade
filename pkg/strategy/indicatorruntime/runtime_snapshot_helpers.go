package indicatorruntime

func (r *indicatorRuntime) prepareSnapshotState() (*snapshotSeriesCache, map[string]any) {
	cache := r.snapshotCache
	if cache == nil {
		cache = newSnapshotSeriesCache()
		r.snapshotCache = cache
	}
	cache.reset()

	result := r.snapshotResult
	if result == nil {
		result = make(map[string]any, r.snapshotKeys.resultCapacity)
		r.snapshotResult = result
	} else {
		clearMap(result)
	}
	return cache, result
}

func (r *indicatorRuntime) appendMovingAverageSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.ma {
		snapshot := r.movingAverageSnapshot(config, cache)
		if snapshot == nil {
			continue
		}
		result[r.snapshotKeys.ma[config]] = snapshot
		if legacyKey, ok := r.snapshotKeys.maLegacy[config]; ok {
			result[legacyKey] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendSecuritySourceSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.securitySource {
		key := r.snapshotKeys.securitySource[config]
		current, previous, currentOK, previousOK := r.securitySourceSnapshotValues(config, cache)
		snapshot := cache.getSeriesSnapshot(key, current, previous, currentOK, previousOK)
		if snapshot != nil {
			result[key] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendRSISnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, period := range r.requirements.rsi {
		key := r.snapshotKeys.rsi[period]
		current, currentOK := r.rsiSnapshotValue(period, cache)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.rsiSource {
		key := r.snapshotKeys.rsiSource[config]
		current, currentOK := calculateRSIValueFromSeries(calculateRSISeries(r.seriesForSource(config.source), config.period))
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendMACDSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.macd {
		if state, ok := r.macdStates[config]; ok {
			currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK := state.snapshotValues()
			result[r.snapshotKeys.macd[config]] = cache.getMACDSnapshotValues(config, currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK)
			continue
		}
		result[r.snapshotKeys.macd[config]] = cache.getMACDSnapshot(config, cache.getMACDSeries(r.closes, config))
	}
}

func (r *indicatorRuntime) appendBollingerSnapshots(result map[string]any) {
	for _, config := range r.requirements.bollinger {
		result[r.snapshotKeys.bollinger[config]] = r.bollingerSnapshot(config)
	}
}

func (r *indicatorRuntime) appendKDJSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.kdj {
		if state, ok := r.kdjStates[config]; ok {
			currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK := state.snapshotValues()
			result[r.snapshotKeys.kdj[config]] = cache.getKDJSnapshotValues(config, currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK)
			continue
		}
		result[r.snapshotKeys.kdj[config]] = cache.getKDJSnapshot(config, cache.getKDJSeries(r.highs, r.lows, r.closes, config))
	}
}

func (r *indicatorRuntime) appendATRSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, period := range r.requirements.atr {
		key := r.snapshotKeys.atr[period]
		current, currentOK := r.atrSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendStdDevSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, period := range r.requirements.stdev {
		key := r.snapshotKeys.stdev[period]
		current, currentOK := r.stdDevSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.stdevSource {
		key := r.snapshotKeys.stdevSource[config]
		current, currentOK := calculateStdDevValue(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendVarianceSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.variance {
		key := r.snapshotKeys.variance[config]
		current, currentOK := calculateVarianceValue(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendWindowSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.windows {
		key := r.snapshotKeys.windows[config]
		state := r.windowStates[config]
		if config.function == "rising" || config.function == "falling" {
			if state != nil && state.hasCurrent {
				result[key] = state.boolCurrent
			}
			continue
		}
		if state != nil {
			result[key] = cache.getWindowSnapshot(config, state.current, state.previous, state.hasCurrent, state.hasPrevious)
		}
	}
}

func (r *indicatorRuntime) appendCumSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.cum {
		key := r.snapshotKeys.cum[config]
		if state := r.cumStates[config]; state != nil {
			result[key] = cache.getSeriesSnapshot(key, state.current, state.previous, state.hasCurrent, state.hasPrevious)
		}
	}
}

func (r *indicatorRuntime) appendStochSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.stoch {
		key := r.snapshotKeys.stoch[config]
		if snapshot := r.stochSnapshot(config, cache); snapshot != nil {
			result[key] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendCCISnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, period := range r.requirements.cci {
		key := r.snapshotKeys.cci[period]
		current, currentOK := r.cciSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.cciSource {
		key := r.snapshotKeys.cciSource[config]
		current, currentOK := calculateCCIFromValues(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendWilliamsRSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, period := range r.requirements.williamsR {
		key := r.snapshotKeys.williamsR[period]
		current, currentOK := r.williamsRSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendVWAPSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.vwap {
		key := r.snapshotKeys.vwap[config]
		current, currentOK := r.vwapStates[config].value()
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendMFISnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.mfi {
		key := r.snapshotKeys.mfi[config]
		current, currentOK := calculateMFIValue(r.seriesForSource(config.source), r.volumes, config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
}

func (r *indicatorRuntime) appendDMISnapshots(result map[string]any) {
	for _, config := range r.requirements.dmi {
		if snapshot := calculateDMISnapshot(r.highs, r.lows, r.closes, config); snapshot != nil {
			result[r.snapshotKeys.dmi[config]] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendSupertrendSnapshots(result map[string]any) {
	for _, config := range r.requirements.supertrend {
		if snapshot := calculateSupertrendSnapshot(r.highs, r.lows, r.closes, config); snapshot != nil {
			result[r.snapshotKeys.supertrend[config]] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendSARSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.sar {
		key := r.snapshotKeys.sar[config]
		current, previous, currentOK, previousOK := calculateSARSnapshotValues(r.highs, r.lows, r.closes, config)
		result[key] = cache.getSeriesSnapshot(key, current, previous, currentOK, previousOK)
	}
}

func (r *indicatorRuntime) appendAdvancedSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.advanced {
		key := r.snapshotKeys.advanced[config]
		if config.kind == "obv" && config.timeUnit == "" {
			if state := r.obvStates[config]; state != nil {
				result[key] = cache.getSeriesSnapshot(key, state.current, state.previous, state.hasCurrent, state.hasPrevious)
			}
			continue
		}
		if snapshot := r.advancedIndicatorSnapshot(config, cache); snapshot != nil {
			result[key] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendStopLossSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.stopLoss {
		snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache(r.closes, r.endTimes, r.sessions, config, r.intervalMinutes, r.symbol, r.includeExtendedHours, cache)
		if snapshot != nil {
			result[r.snapshotKeys.stopLoss[config]] = snapshot
		}
	}
}

func (r *indicatorRuntime) appendDivergenceSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	r.appendRSIDivergenceSnapshots(result)
	r.appendMACDDivergenceSnapshots(result, cache)
	r.appendKDJDivergenceSnapshots(result, cache)
}

func (r *indicatorRuntime) appendRSIDivergenceSnapshots(result map[string]any) {
	for _, config := range r.requirements.rsiDivergence {
		if state, ok := r.rsiStates[config.period]; ok {
			result[r.snapshotKeys.rsiDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		result[r.snapshotKeys.rsiDivergence[config]] = detectRSIDivergence(r.closes, r.rsiSeries(config.period), config.direction, config.lookback)
	}
}

func (r *indicatorRuntime) appendMACDDivergenceSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.macdDivergence {
		baseConfig := macdConfig{fastPeriod: config.fastPeriod, slowPeriod: config.slowPeriod, signalPeriod: config.signalPeriod}
		if state, ok := r.macdStates[baseConfig]; ok {
			result[r.snapshotKeys.macdDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		result[r.snapshotKeys.macdDivergence[config]] = detectMACDDivergence(r.closes, cache.getMACDSeries(r.closes, baseConfig).diff, config.direction, config.lookback)
	}
}

func (r *indicatorRuntime) appendKDJDivergenceSnapshots(result map[string]any, cache *snapshotSeriesCache) {
	for _, config := range r.requirements.kdjDivergence {
		baseConfig := kdjConfig{period: config.period, m1: config.m1, m2: config.m2}
		if state, ok := r.kdjStates[baseConfig]; ok {
			result[r.snapshotKeys.kdjDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		jSeries := cache.getKDJSeries(r.highs, r.lows, r.closes, baseConfig).j
		result[r.snapshotKeys.kdjDivergence[config]] = detectKDJDivergence(r.closes, jSeries, config.direction, config.lookback)
	}
}
