package indicatorruntime

import "math"

func newRollingATRStates(requirements indicatorRequirements) map[int]*rollingATRState {
	if len(requirements.atr) == 0 {
		return nil
	}
	states := make(map[int]*rollingATRState, len(requirements.atr))
	for _, period := range requirements.atr {
		if period <= 0 {
			continue
		}
		states[period] = &rollingATRState{period: period}
	}
	return states
}

func (r *indicatorRuntime) pushATRStates(high, low, closeValue, previousClose float64, hasPreviousClose bool) {
	if r == nil || len(r.atrStates) == 0 {
		return
	}
	for _, state := range r.atrStates {
		state.push(high, low, closeValue, previousClose, hasPreviousClose)
	}
}

func (r *indicatorRuntime) atrSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.atrStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateATR(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
}

func (s *rollingATRState) push(high, low, _ float64, previousClose float64, hasPreviousClose bool) {
	if s == nil || s.period <= 0 {
		return
	}
	trueRange := high - low
	if hasPreviousClose {
		trueRange = maxFloat(trueRange, maxFloat(math.Abs(high-previousClose), math.Abs(low-previousClose)))
	}
	evicted, evictedOK := s.window.push(trueRange, s.period)
	s.windowSum += trueRange
	if evictedOK {
		s.windowSum -= evicted
	}
	if s.window.len() < s.period {
		s.hasValue = false
		return
	}
	s.current = s.windowSum / float64(s.period)
	s.hasValue = true
}

func (s *rollingATRState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingATRState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
}
