package indicatorruntime
func (s *rollingCumState) push(value float64) {
	if s == nil {
		return
	}
	if s.hasCurrent {
		s.previous = s.current
		s.hasPrevious = true
	}
	s.current += value
	s.hasCurrent = true
}


func newRollingCumStates(requirements indicatorRequirements) map[sourceConfig]*rollingCumState {
	if len(requirements.cum) == 0 {
		return nil
	}
	states := make(map[sourceConfig]*rollingCumState, len(requirements.cum))
	for _, config := range requirements.cum {
		if _, ok := parseOHLCVSource(config.source); !ok {
			continue
		}
		states[config] = &rollingCumState{}
	}
	if len(states) == 0 {
		return nil
	}
	return states
}


func (r *indicatorRuntime) pushCumStates(openValue, high, low, closeValue, volume float64) {
	if r == nil || len(r.cumStates) == 0 {
		return
	}
	for config, state := range r.cumStates {
		value, ok := ohlcvSourceValue(config.source, openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.push(value)
	}
}

