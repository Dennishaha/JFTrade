package indicatorruntime

func (s *rollingStochState) push(high, low, sourceValue float64) {
	if s == nil || s.period <= 0 {
		return
	}
	windowStart := s.index - s.period + 1
	s.highDeque.popExpired(windowStart)
	s.lowDeque.popExpired(windowStart)
	s.highDeque.pushMax(s.index, high)
	s.lowDeque.pushMin(s.index, low)
	s.index++
	if s.index < s.period {
		s.hasCurrent = false
		return
	}
	highestHigh, _ := s.highDeque.frontValue()
	lowestLow, _ := s.lowDeque.frontValue()
	if s.hasCurrent {
		s.previous = s.current
		s.hasPrevious = true
	}
	if highestHigh == lowestLow {
		s.current = 50
		s.hasCurrent = true
		return
	}
	s.current = 100 * (sourceValue - lowestLow) / (highestHigh - lowestLow)
	s.hasCurrent = true
}

func newRollingStochStates(requirements indicatorRequirements) map[sourcePeriodConfig]*rollingStochState {
	if len(requirements.stoch) == 0 {
		return nil
	}
	states := make(map[sourcePeriodConfig]*rollingStochState, len(requirements.stoch))
	for _, config := range requirements.stoch {
		if normalizeIndicatorTimeUnit(config.timeUnit) != "" {
			continue
		}
		source, ok := parseOHLCVSource(config.source)
		if !ok || source == "volume" || config.period <= 0 {
			continue
		}
		resolved := sourcePeriodConfig{source: source, period: config.period}
		states[resolved] = &rollingStochState{source: source, period: config.period}
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func (r *indicatorRuntime) pushStochStates(openValue, high, low, closeValue, volume float64) {
	if r == nil || len(r.stochStates) == 0 {
		return
	}
	for config, state := range r.stochStates {
		sourceValue, ok := ohlcvSourceValue(config.source, openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.push(high, low, sourceValue)
	}
}

func (r *indicatorRuntime) stochSnapshot(config sourcePeriodConfig, cache *snapshotSeriesCache) any {
	if r == nil {
		return nil
	}
	if normalizeIndicatorTimeUnit(config.timeUnit) != "" {
		values, _, ok := r.fixedTimeframeSeries(config.timeUnit, config.source)
		if !ok {
			return nil
		}
		highs, lows, ok := r.fixedTimeframeHighLow(config.timeUnit)
		if !ok {
			return nil
		}
		current, previous, currentOK, previousOK := calculateStochSnapshotValues(values, highs, lows, config.period)
		return cache.getSeriesSnapshot(stochIndicatorKey(config), current, previous, currentOK, previousOK)
	}
	if state := r.stochStates[config]; state != nil {
		return cache.getSeriesSnapshot(stochIndicatorKey(config), state.current, state.previous, state.hasCurrent, state.hasPrevious)
	}
	return nil
}

func (r *indicatorRuntime) fixedTimeframeHighLow(timeUnit string) ([]float64, []float64, bool) {
	highs, _, ok := r.fixedTimeframeSeries(timeUnit, "high")
	if !ok {
		return nil, nil, false
	}
	lows, _, ok := r.fixedTimeframeSeries(timeUnit, "low")
	if !ok {
		return nil, nil, false
	}
	return highs, lows, true
}

func calculateStochSnapshotValues(values, highs, lows []float64, period int) (float64, float64, bool, bool) {
	current, currentOK := calculateStochAt(values, highs, lows, period, len(values)-1)
	previous, previousOK := calculateStochAt(values, highs, lows, period, len(values)-2)
	return current, previous, currentOK, previousOK
}

func calculateStochAt(values, highs, lows []float64, period int, end int) (float64, bool) {
	if period <= 0 || end < 0 || len(values) != len(highs) || len(values) != len(lows) || end >= len(values) || end+1 < period {
		return 0, false
	}
	start := end - period + 1
	highest := highs[start]
	lowest := lows[start]
	for index := start + 1; index <= end; index++ {
		if highs[index] > highest {
			highest = highs[index]
		}
		if lows[index] < lowest {
			lowest = lows[index]
		}
	}
	if highest == lowest {
		return 50, true
	}
	return 100 * (values[end] - lowest) / (highest - lowest), true
}
