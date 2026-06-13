package indicatorruntime

import "math"

func newRollingEMAStates(requirements indicatorRequirements, intervalMinutes int, seriesLimit int) map[movingAverageConfig]*rollingEMATailState {
	if len(requirements.ma) == 0 {
		return nil
	}
	states := map[movingAverageConfig]*rollingEMATailState{}
	for _, config := range requirements.ma {
		if isNumericMinuteTimeUnit(config.timeUnit) {
			continue
		}
		kind := normalizeMovingAverageType(config.averageType)
		if kind != "EMA" && kind != "EXPMA" {
			continue
		}
		resolvedPeriod := resolveBarCount(config.period, config.timeUnit, intervalMinutes)
		if resolvedPeriod <= 0 {
			continue
		}
		states[config] = newRollingEMATailState(resolvedPeriod, seriesLimit, 2)
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func newRollingEMATailState(period, limit, tailLen int) *rollingEMATailState {
	if period <= 0 || tailLen <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = minimumIndicatorSeriesLimit
	}
	state := &rollingEMATailState{
		period:  period,
		limit:   limit,
		tailLen: max(tailLen, 1),
		alpha:   2 / float64(period+1),
	}
	state.beta = 1 - state.alpha
	state.tail = make([]float64, 0, state.tailLen)
	state.powers = make([]float64, limit)
	if limit > 0 {
		state.powers[0] = 1
		for index := 1; index < limit; index++ {
			state.powers[index] = state.powers[index-1] * state.beta
		}
	}
	return state
}

func (s *rollingEMATailState) push(value float64, trimmed bool, oldFirst, oldSecond float64, hasOldFirst, hasOldSecond bool) {
	if s == nil {
		return
	}
	if s.windowLen == 0 {
		s.tail = append(s.tail[:0], value)
		s.windowLen = 1
		return
	}
	if !trimmed {
		current := s.tail[len(s.tail)-1]
		s.appendValue(current + (value-current)*s.alpha)
		s.windowLen = min(s.windowLen+1, s.limit)
		return
	}
	if !hasOldFirst || !hasOldSecond || s.windowLen <= 1 || s.limit <= 1 {
		s.tail = s.tail[:0]
		s.appendValue(value)
		s.windowLen = min(s.limit, 1)
		return
	}
	delta := oldSecond - oldFirst
	oldLen := s.windowLen
	startIndex := oldLen - len(s.tail)
	retained := 0
	for index := range s.tail {
		oldIndex := startIndex + index
		if oldIndex == 0 {
			continue
		}
		s.tail[retained] = s.tail[index] + s.powerAt(oldIndex)*delta
		retained++
	}
	s.tail = s.tail[:retained]
	if len(s.tail) == 0 {
		s.appendValue(value)
		s.windowLen = min(oldLen, s.limit)
		return
	}
	shiftedCurrent := s.tail[len(s.tail)-1]
	s.appendValue(shiftedCurrent + (value-shiftedCurrent)*s.alpha)
	s.windowLen = s.limit
}

func (s *rollingEMATailState) appendValue(value float64) {
	if s == nil {
		return
	}
	if len(s.tail) < s.tailLen {
		s.tail = append(s.tail, value)
		return
	}
	copy(s.tail, s.tail[1:])
	s.tail[len(s.tail)-1] = value
}

func (s *rollingEMATailState) powerAt(index int) float64 {
	if s == nil || index < 0 {
		return 0
	}
	if index < len(s.powers) {
		return s.powers[index]
	}
	return math.Pow(s.beta, float64(index))
}

func (s *rollingEMATailState) snapshotValues() (float64, float64, bool, bool) {
	if s == nil || len(s.tail) == 0 {
		return 0, 0, false, false
	}
	current := s.tail[len(s.tail)-1]
	previous := 0.0
	previousOK := s.windowLen-1 >= s.period && len(s.tail) > 1
	if previousOK {
		previous = s.tail[len(s.tail)-2]
	}
	return current, previous, s.windowLen >= s.period, previousOK
}

func (r *indicatorRuntime) pushEMAStates(openValue, high, low, closeValue, volume float64, trimmed bool, oldFirst, oldSecond map[string]float64, hasOldFirst, hasOldSecond bool) {
	if r == nil || len(r.emaStates) == 0 {
		return
	}
	for config, state := range r.emaStates {
		source := normalizeSourceOrClose(config.source)
		value, ok := ohlcvSourceValue(source, openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.push(value, trimmed, oldFirst[source], oldSecond[source], hasOldFirst, hasOldSecond)
	}
}
