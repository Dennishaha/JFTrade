package indicatorruntime

import "math"

func newRollingCCIStates(requirements indicatorRequirements) map[int]*rollingCCIState {
	if len(requirements.cci) == 0 {
		return nil
	}
	states := make(map[int]*rollingCCIState, len(requirements.cci))
	for _, period := range requirements.cci {
		if period <= 0 {
			continue
		}
		states[period] = &rollingCCIState{period: period}
	}
	return states
}

func (r *indicatorRuntime) pushCCIStates(high, low, closeValue float64) {
	if r == nil || len(r.cciStates) == 0 {
		return
	}
	typicalPrice := (high + low + closeValue) / 3
	for _, state := range r.cciStates {
		state.push(typicalPrice)
	}
}

func (r *indicatorRuntime) cciValue(period int) any {
	current, ok := r.cciSnapshotValue(period)
	if !ok {
		return nil
	}
	return current
}

func (r *indicatorRuntime) cciSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.cciStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateCCI(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
}

func (s *rollingCCIState) push(typicalPrice float64) {
	if s == nil || s.period <= 0 {
		return
	}
	evicted, evictedOK := s.window.push(typicalPrice, s.period)
	s.sum += typicalPrice
	if evictedOK {
		s.sum -= evicted
	}
	if s.window.len() < s.period {
		s.hasValue = false
		return
	}
	average := s.sum / float64(s.period)
	meanDeviation := 0.0
	for index := 0; index < s.window.len(); index++ {
		value, _ := s.window.at(index)
		meanDeviation += math.Abs(value - average)
	}
	meanDeviation /= float64(s.period)
	if meanDeviation == 0 {
		s.current = 0
		s.hasValue = true
		return
	}
	s.current = (typicalPrice - average) / (0.015 * meanDeviation)
	s.hasValue = true
}

func (s *rollingCCIState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingCCIState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
}
