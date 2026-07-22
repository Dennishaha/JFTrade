package indicatorruntime

import (
	"math"
)

func newRollingRSIStates(requirements indicatorRequirements, seriesLimit int) map[int]*rollingRSIState {
	lookbacks := map[int]map[int]struct{}{}
	for _, period := range requirements.rsi {
		if period > 0 {
			if _, ok := lookbacks[period]; !ok {
				lookbacks[period] = map[int]struct{}{}
			}
		}
	}
	for _, config := range requirements.rsiDivergence {
		if config.period > 0 {
			if _, ok := lookbacks[config.period]; !ok {
				lookbacks[config.period] = map[int]struct{}{}
			}
			if config.lookback > 0 {
				lookbacks[config.period][config.lookback] = struct{}{}
			}
		}
	}
	if len(lookbacks) == 0 {
		return nil
	}
	states := make(map[int]*rollingRSIState, len(lookbacks))
	for period, lookbackSet := range lookbacks {
		states[period] = newRollingRSIState(period, max(seriesLimit-period, 0), sortedLookbacks(lookbackSet))
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func newRollingRSIState(period, maxLength int, lookbacks []int) *rollingRSIState {
	if period <= 0 {
		return nil
	}
	maxLookback := 0
	for _, lookback := range lookbacks {
		maxLookback = max(maxLookback, lookback)
	}
	return &rollingRSIState{
		period:            period,
		maxLength:         maxLength,
		tailLen:           maxLookback + 1,
		divergenceWindows: newRollingDivergenceWindowStates(lookbacks),
	}
}

func (r *indicatorRuntime) pushRSIStates(closeValue, previousClose float64, hasPreviousClose bool) {
	if r == nil || len(r.rsiStates) == 0 {
		return
	}
	for _, state := range r.rsiStates {
		state.push(closeValue, previousClose, hasPreviousClose)
	}
}

func (r *indicatorRuntime) rsiSeries(period int) []float64 {
	if r == nil {
		return nil
	}
	if state, ok := r.rsiStates[period]; ok {
		return state.seriesValues()
	}
	return calculateRSISeries(r.closes, period)
}

func (r *indicatorRuntime) rsiSnapshotValue(period int, cache *snapshotSeriesCache) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.rsiStates[period]; ok {
		return state.currentValue()
	}
	series := calculateRSISeries(r.closes, period)
	if cache != nil {
		series = cache.getRSISeries(r.closes, period)
	}
	return calculateRSIValueFromSeries(series)
}

func (s *rollingRSIState) push(currentClose, previousClose float64, hasPreviousClose bool) {
	if s == nil || !hasPreviousClose || s.period <= 0 {
		return
	}
	gain := 0.0
	loss := 0.0
	delta := currentClose - previousClose
	if delta >= 0 {
		gain = delta
	} else {
		loss = math.Abs(delta)
	}
	if s.initialChanges < s.period {
		s.initialChanges++
		s.initialGainSum += gain
		s.initialLossSum += loss
		if s.initialChanges < s.period {
			return
		}
		s.averageGain = s.initialGainSum / float64(s.period)
		s.averageLoss = s.initialLossSum / float64(s.period)
	} else {
		s.averageGain = (s.averageGain*float64(s.period-1) + gain) / float64(s.period)
		s.averageLoss = (s.averageLoss*float64(s.period-1) + loss) / float64(s.period)
	}
	value := rsiFromWilderAverages(s.averageGain, s.averageLoss)
	s.pushDivergenceSample(currentClose, value)
	s.series = append(s.series, value)
	if s.maxLength <= 0 {
		s.series = s.series[:0]
		return
	}
	if len(s.series) > s.maxLength {
		s.series = trimFloatSeriesInPlace(s.series, s.maxLength)
	}
}

func (s *rollingRSIState) seriesValues() []float64 {
	if s == nil {
		return nil
	}
	return s.series
}

func (s *rollingRSIState) detectDivergence(closes []float64, direction string, lookback int) bool {
	if s == nil {
		return false
	}
	if window := s.divergenceWindows[lookback]; window != nil {
		return window.detect(direction)
	}
	if lookback <= 0 {
		return false
	}
	alignedCloses, alignedSeries := alignSeries(closes, s.series)
	return detectDivergence(alignedCloses, alignedSeries, direction, lookback)
}

func (s *rollingRSIState) pushDivergenceSample(price, value float64) {
	if s == nil || s.tailLen <= 0 || len(s.divergenceWindows) == 0 {
		return
	}
	s.closeTail = appendTailValue(s.closeTail, price, s.tailLen)
	s.valueTail = appendTailValue(s.valueTail, value, s.tailLen)
	for _, window := range s.divergenceWindows {
		window.refresh(s.closeTail, s.valueTail)
	}
}

func (s *rollingRSIState) currentValue() (float64, bool) {
	if s == nil || len(s.series) == 0 {
		return 0, false
	}
	return s.series[len(s.series)-1], true
}
