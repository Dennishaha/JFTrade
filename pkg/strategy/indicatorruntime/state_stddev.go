package indicatorruntime

import "math"

func newRollingStdDevStates(requirements indicatorRequirements) map[int]*rollingStdDevState {
	if len(requirements.stdev) == 0 {
		return nil
	}
	states := make(map[int]*rollingStdDevState, len(requirements.stdev))
	for _, period := range requirements.stdev {
		if period <= 0 {
			continue
		}
		states[period] = &rollingStdDevState{period: period}
	}
	return states
}

func (r *indicatorRuntime) pushStdDevStates(closeValue float64) {
	if r == nil || len(r.stdevStates) == 0 {
		return
	}
	for _, state := range r.stdevStates {
		state.push(closeValue)
	}
}

func (r *indicatorRuntime) stdDevSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.stdevStates[period]; ok {
		return state.currentValue()
	}
	return calculateStdDev(r.closes, period)
}

func (s *rollingStdDevState) push(value float64) {
	if s == nil || s.period <= 0 {
		return
	}
	evicted, evictedOK := s.window.push(value, s.period)
	s.sum += value
	s.sumSquares += value * value
	if evictedOK {
		s.sum -= evicted
		s.sumSquares -= evicted * evicted
	}
	if s.window.len() < s.period {
		s.hasValue = false
		return
	}
	mean := s.sum / float64(s.period)
	variance := s.sumSquares/float64(s.period) - mean*mean
	if variance < 0 {
		variance = 0
	}
	s.current = math.Sqrt(variance)
	s.hasValue = true
}

func (s *rollingStdDevState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
}
