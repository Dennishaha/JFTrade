package indicatorruntime

func newRollingWilliamsRStates(requirements indicatorRequirements) map[int]*rollingWilliamsRState {
	if len(requirements.williamsR) == 0 {
		return nil
	}
	states := make(map[int]*rollingWilliamsRState, len(requirements.williamsR))
	for _, period := range requirements.williamsR {
		if period <= 0 {
			continue
		}
		states[period] = &rollingWilliamsRState{period: period}
	}
	return states
}

func (r *indicatorRuntime) pushWilliamsRStates(high, low, closeValue float64) {
	if r == nil || len(r.williamsRStates) == 0 {
		return
	}
	for _, state := range r.williamsRStates {
		state.push(high, low, closeValue)
	}
}

func (r *indicatorRuntime) williamsRValue(period int) any {
	current, ok := r.williamsRSnapshotValue(period)
	if !ok {
		return nil
	}
	return current
}

func (r *indicatorRuntime) williamsRSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.williamsRStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateWilliamsR(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
}

func (s *rollingWilliamsRState) push(high, low, closeValue float64) {
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
		s.hasValue = false
		return
	}
	highestHigh, _ := s.highDeque.frontValue()
	lowestLow, _ := s.lowDeque.frontValue()
	if highestHigh == lowestLow {
		s.current = -50
		s.hasValue = true
		return
	}
	s.current = -100 * (highestHigh - closeValue) / (highestHigh - lowestLow)
	s.hasValue = true
}

func (s *rollingWilliamsRState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingWilliamsRState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
}
