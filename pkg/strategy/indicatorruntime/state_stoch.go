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
