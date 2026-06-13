package indicatorruntime

func newRollingMovingAverageStates(requirements indicatorRequirements, intervalMinutes int) map[movingAverageConfig]*rollingMovingAverageSnapshotState {
	if len(requirements.ma) == 0 {
		return nil
	}
	states := map[movingAverageConfig]*rollingMovingAverageSnapshotState{}
	for _, config := range requirements.ma {
		if isNumericMinuteTimeUnit(config.timeUnit) {
			continue
		}
		resolved := config
		resolved.period = resolveBarCount(config.period, config.timeUnit, intervalMinutes)
		resolved.timeUnit = ""
		kind := normalizeMovingAverageType(resolved.averageType)
		switch kind {
		case "MA", "SMA", "BOLL", "VWMA":
			if resolved.period <= 0 {
				continue
			}
			states[config] = &rollingMovingAverageSnapshotState{kind: kind, period: resolved.period}
		}
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func (r *indicatorRuntime) pushMovingAverageStates(openValue, high, low, closeValue, volume float64) {
	if r == nil || len(r.maStates) == 0 {
		return
	}
	for config, state := range r.maStates {
		value, ok := ohlcvSourceValue(normalizeSourceOrClose(config.source), openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.push(value, volume)
	}
}

func (r *indicatorRuntime) movingAverageSnapshot(config movingAverageConfig, cache *snapshotSeriesCache) any {
	if r == nil {
		return nil
	}
	if usesTradingPeriodWindow(config.timeUnit, r.intervalMinutes, r.symbol, r.endTimes, r.includeExtendedHours) {
		return r.movingAverageSnapshotForTradingWindow(config, cache)
	}
	if usesFixedIntradayTimeframe(config.timeUnit, r.intervalMinutes) {
		values, volumes, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok {
			return nil
		}
		effectiveConfig := config
		effectiveConfig.timeUnit = ""
		current, previous, currentOK, previousOK := calculateMovingAverageSnapshotValues(values, volumes, effectiveConfig)
		return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
	}
	if state, ok := r.emaStates[config]; ok {
		current, previous, currentOK, previousOK := state.snapshotValues()
		return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
	}
	if state, ok := r.maStates[config]; ok {
		return state.snapshotValue()
	}
	effectiveConfig := config
	effectiveConfig.period = resolveBarCount(config.period, config.timeUnit, r.intervalMinutes)
	effectiveConfig.timeUnit = ""
	current, previous, currentOK, previousOK := calculateMovingAverageSnapshotValuesWithCache(r.seriesForSource(config.source), r.volumes, effectiveConfig, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func (r *indicatorRuntime) movingAverageSnapshotForTradingWindow(config movingAverageConfig, cache *snapshotSeriesCache) any {
	if r == nil {
		return nil
	}
	unit := normalizeIndicatorTimeUnit(config.timeUnit)
	labelKeys := r.tradingPeriodLabels[unit]
	values := r.seriesForSource(config.source)
	if len(labelKeys) == len(r.endTimes) && len(labelKeys) > 0 {
		if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, r.volumes, labelKeys, config); handled {
			if !currentOK {
				return nil
			}
			return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
		}
	}
	return buildMovingAverageSnapshotForTradingWindow(values, r.volumes, r.endTimes, config, r.symbol, r.includeExtendedHours, cache)
}

func (s *rollingMovingAverageSnapshotState) push(value, volume float64) {
	if s == nil || s.period <= 0 {
		return
	}
	previousCurrent := s.current
	previousHasCurrent := s.hasCurrent
	evictedValue, evictedValueOK := s.values.push(value, s.period)
	s.sum += value
	if s.kind == "VWMA" {
		evictedVolume, evictedVolumeOK := s.volumes.push(volume, s.period)
		s.weightedSum += value * volume
		s.volumeSum += volume
		if evictedValueOK && evictedVolumeOK {
			s.weightedSum -= evictedValue * evictedVolume
			s.volumeSum -= evictedVolume
		}
	} else if evictedValueOK {
		s.sum -= evictedValue
	}
	if s.kind == "VWMA" && evictedValueOK {
		s.sum -= evictedValue
	}
	s.previous = previousCurrent
	s.hasPrevious = previousHasCurrent
	if s.values.len() < s.period {
		s.hasCurrent = false
		return
	}
	if s.kind == "VWMA" {
		if s.volumeSum == 0 {
			s.hasCurrent = false
			return
		}
		s.current = s.weightedSum / s.volumeSum
		s.hasCurrent = true
		return
	}
	s.current = s.sum / float64(s.period)
	s.hasCurrent = true
}

func (s *rollingMovingAverageSnapshotState) snapshot() map[string]any {
	if s == nil || (!s.hasCurrent && !s.hasPrevious) {
		return nil
	}
	result := map[string]any{"value": nil, "previous": nil}
	if s.hasCurrent {
		result["value"] = s.current
	}
	if s.hasPrevious {
		result["previous"] = s.previous
	}
	return result
}

func (s *rollingMovingAverageSnapshotState) snapshotValue() any {
	if s == nil || (!s.hasCurrent && !s.hasPrevious) {
		return nil
	}
	return s
}

func (s *rollingMovingAverageSnapshotState) PreferredScalarValue() (float64, bool) {
	if s == nil || !s.hasCurrent {
		return 0, false
	}
	return s.current, true
}

func (s *rollingMovingAverageSnapshotState) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if s == nil || name != "value" {
		return 0, 0, false, false, false
	}
	return s.current, s.previous, s.hasCurrent, s.hasPrevious, true
}

func (s *rollingMovingAverageSnapshotState) FieldValue(name string) (any, bool) {
	if s == nil {
		return nil, false
	}
	switch name {
	case "value":
		if s.hasCurrent {
			return s.current, true
		}
		return nil, true
	case "previous":
		if s.hasPrevious {
			return s.previous, true
		}
		return nil, true
	default:
		return nil, false
	}
}

func calculateMovingAverageSnapshotValues(values, volumes []float64, config movingAverageConfig) (float64, float64, bool, bool) {
	return calculateMovingAverageSnapshotValuesWithCache(values, volumes, config, nil)
}

func calculateMovingAverageSnapshotValuesWithCache(values, volumes []float64, config movingAverageConfig, cache *snapshotSeriesCache) (float64, float64, bool, bool) {
	switch normalizeMovingAverageType(config.averageType) {
	case "EMA", "EXPMA":
		return emaSnapshotValues(cache.getEMASequence(values, config.period), len(values), config.period)
	case "SMMA":
		return lastTwoSequenceValues(cache.getSMMASequence(values, config.period))
	case "LWMA":
		return lastTwoSequenceValues(cache.getWMASequence(values, config.period))
	case "TMA":
		return lastTwoSequenceValues(cache.getTMASequence(values, config.period))
	case "HMA":
		return lastTwoSequenceValues(cache.getHMASequence(values, config.period))
	case "VWMA":
		current, currentOK := volumeWeightedMovingAverage(values, volumes, config.period)
		previous, previousOK := volumeWeightedMovingAverage(
			values[:max(len(values)-1, 0)],
			volumes[:max(len(volumes)-1, 0)],
			config.period,
		)
		return current, previous, currentOK, previousOK
	case "SMA", "BOLL", "MA":
		fallthrough
	default:
		return lastTwoSequenceValues(cache.getSMASequence(values, config.period))
	}
}

func emaSnapshotValues(sequence []float64, sourceLen int, period int) (float64, float64, bool, bool) {
	if period <= 0 || sourceLen < period || len(sequence) == 0 {
		return 0, 0, false, false
	}
	current := sequence[len(sequence)-1]
	previous := 0.0
	previousOK := sourceLen-1 >= period && len(sequence) > 1
	if previousOK {
		previous = sequence[len(sequence)-2]
	}
	return current, previous, true, previousOK
}

func lastTwoSequenceValues(sequence []float64) (float64, float64, bool, bool) {
	if len(sequence) == 0 {
		return 0, 0, false, false
	}
	current := sequence[len(sequence)-1]
	previous := 0.0
	previousOK := len(sequence) > 1
	if previousOK {
		previous = sequence[len(sequence)-2]
	}
	return current, previous, true, previousOK
}
