package indicatorruntime

import "math"

func newRollingBollingerStates(requirements indicatorRequirements) map[bollingerConfig]*rollingBollingerState {
	if len(requirements.bollinger) == 0 {
		return nil
	}
	states := make(map[bollingerConfig]*rollingBollingerState, len(requirements.bollinger))
	for _, config := range requirements.bollinger {
		if config.period <= 0 {
			continue
		}
		states[config] = &rollingBollingerState{period: config.period, multiplier: config.multiplier}
	}
	return states
}

func (r *indicatorRuntime) pushBollingerStates(closeValue float64) {
	if r == nil || len(r.bollingerStates) == 0 {
		return
	}
	for _, state := range r.bollingerStates {
		state.push(closeValue)
	}
}

func (r *indicatorRuntime) bollingerSnapshot(config bollingerConfig) any {
	if r == nil {
		return nil
	}
	if state, ok := r.bollingerStates[config]; ok {
		return state.snapshotValue()
	}
	return calculateBollingerSnapshot(r.closes, config)
}

func (s *rollingBollingerState) push(value float64) {
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
}

func (s *rollingBollingerState) snapshot() map[string]any {
	if s == nil || s.window.len() < s.period {
		return nil
	}
	middle := s.sum / float64(s.period)
	variance := s.sumSquares/float64(s.period) - middle*middle
	if variance < 0 {
		variance = 0
	}
	standardDeviation := math.Sqrt(variance)
	return map[string]any{
		"middle": middle,
		"upper":  middle + standardDeviation*s.multiplier,
		"lower":  middle - standardDeviation*s.multiplier,
	}
}

func (s *rollingBollingerState) snapshotValue() any {
	if s == nil || s.window.len() < s.period {
		return nil
	}
	return s
}

func (s *rollingBollingerState) PreferredScalarValue() (float64, bool) {
	if s == nil || s.window.len() < s.period {
		return 0, false
	}
	return s.sum / float64(s.period), true
}

func (s *rollingBollingerState) FieldValue(name string) (any, bool) {
	if s == nil || s.window.len() < s.period {
		return nil, false
	}
	middle := s.sum / float64(s.period)
	variance := s.sumSquares/float64(s.period) - middle*middle
	if variance < 0 {
		variance = 0
	}
	standardDeviation := math.Sqrt(variance)
	switch name {
	case "middle":
		return middle, true
	case "upper":
		return middle + standardDeviation*s.multiplier, true
	case "lower":
		return middle - standardDeviation*s.multiplier, true
	default:
		return nil, false
	}
}
