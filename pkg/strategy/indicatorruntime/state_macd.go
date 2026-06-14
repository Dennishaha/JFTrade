package indicatorruntime

import (
	"math"
)

func newRollingMACDStates(requirements indicatorRequirements, seriesLimit int) map[macdConfig]*rollingMACDState {
	lookbacks := map[macdConfig]map[int]struct{}{}
	for _, config := range requirements.macd {
		if _, ok := lookbacks[config]; !ok {
			lookbacks[config] = map[int]struct{}{}
		}
	}
	for _, config := range requirements.macdDivergence {
		base := macdConfig{fastPeriod: config.fastPeriod, slowPeriod: config.slowPeriod, signalPeriod: config.signalPeriod}
		if _, ok := lookbacks[base]; !ok {
			lookbacks[base] = map[int]struct{}{}
		}
		if config.lookback > 0 {
			lookbacks[base][config.lookback] = struct{}{}
		}
	}
	if len(lookbacks) == 0 {
		return nil
	}
	states := make(map[macdConfig]*rollingMACDState, len(lookbacks))
	for config, lookbackSet := range lookbacks {
		if config.fastPeriod <= 0 || config.slowPeriod <= 0 || config.signalPeriod <= 0 {
			continue
		}
		states[config] = newRollingMACDState(config, seriesLimit, sortedLookbacks(lookbackSet))
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func newRollingMACDState(config macdConfig, limit int, lookbacks []int) *rollingMACDState {
	if config.fastPeriod <= 0 || config.slowPeriod <= 0 || config.signalPeriod <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = minimumIndicatorSeriesLimit
	}
	maxLookback := 1
	for _, lookback := range lookbacks {
		maxLookback = max(maxLookback, lookback)
	}
	tailLen := max(maxLookback+1, 2)
	state := &rollingMACDState{
		config:      config,
		minimum:     max(config.fastPeriod, config.slowPeriod) + config.signalPeriod,
		fast:        newRollingEMATailState(config.fastPeriod, limit, tailLen),
		slow:        newRollingEMATailState(config.slowPeriod, limit, tailLen),
		closeTail:   make([]float64, 0, tailLen),
		diffTail:    make([]float64, 0, tailLen),
		signalAlpha: 2 / float64(config.signalPeriod+1),
	}
	state.signalBeta = 1 - state.signalAlpha
	if limit > 1 && state.signalBeta != 0 {
		adjustment := 0.0
		for index := 1; index < limit; index++ {
			adjustment += math.Pow(state.signalBeta, float64(limit-1-index)) * (state.fast.powerAt(index) - state.slow.powerAt(index))
		}
		state.signalShiftAdjustment = adjustment
	}
	state.divergenceWindows = newRollingDivergenceWindowStates(lookbacks)
	return state
}

func (s *rollingMACDState) push(value float64, trimmed bool, oldFirst, oldSecond float64, hasOldFirst, hasOldSecond bool) {
	if s == nil || s.fast == nil || s.slow == nil {
		return
	}
	shiftedWeightedSum := s.signalWeightedSum
	if trimmed && hasOldFirst && hasOldSecond && s.signalBeta != 0 {
		shiftedWeightedSum += (oldSecond - oldFirst) * s.signalShiftAdjustment
	}
	s.fast.push(value, trimmed, oldFirst, oldSecond, hasOldFirst, hasOldSecond)
	s.slow.push(value, trimmed, oldFirst, oldSecond, hasOldFirst, hasOldSecond)
	currentDiff, ok := s.currentDiff()
	if !ok {
		s.signalWeightedSum = 0
		return
	}
	if s.signalBeta == 0 {
		s.signalWeightedSum = 0
		s.pushDivergenceSample(value)
		return
	}
	if trimmed {
		s.signalWeightedSum = shiftedWeightedSum*s.signalBeta + currentDiff
		s.pushDivergenceSample(value)
		return
	}
	if s.fast.windowLen == 1 {
		s.signalWeightedSum = currentDiff
		s.pushDivergenceSample(value)
		return
	}
	s.signalWeightedSum = s.signalWeightedSum*s.signalBeta + currentDiff
	s.pushDivergenceSample(value)
}

func (s *rollingMACDState) currentDiff() (float64, bool) {
	if s == nil || s.fast == nil || s.slow == nil || len(s.fast.tail) == 0 || len(s.slow.tail) == 0 {
		return 0, false
	}
	return s.fast.tail[len(s.fast.tail)-1] - s.slow.tail[len(s.slow.tail)-1], true
}

func (s *rollingMACDState) previousDiff() (float64, bool) {
	if s == nil || s.fast == nil || s.slow == nil || len(s.fast.tail) < 2 || len(s.slow.tail) < 2 {
		return 0, false
	}
	return s.fast.tail[len(s.fast.tail)-2] - s.slow.tail[len(s.slow.tail)-2], true
}

func (s *rollingMACDState) currentSignal() float64 {
	if s == nil {
		return 0
	}
	if s.signalBeta == 0 {
		value, _ := s.currentDiff()
		return value
	}
	return s.signalAlpha * s.signalWeightedSum
}

func (s *rollingMACDState) previousSignal() (float64, bool) {
	if s == nil || s.fast == nil || s.fast.windowLen < 2 {
		return 0, false
	}
	if s.signalBeta == 0 {
		return s.previousDiff()
	}
	currentDiff, ok := s.currentDiff()
	if !ok {
		return 0, false
	}
	return s.signalAlpha * ((s.signalWeightedSum - currentDiff) / s.signalBeta), true
}

func (s *rollingMACDState) snapshotValues() (float64, float64, float64, float64, bool, bool) {
	currentDiff, currentOK := s.currentDiff()
	if !currentOK || s.fast == nil || s.fast.windowLen < s.minimum {
		return 0, 0, 0, 0, false, false
	}
	previousDiff, previousDiffOK := s.previousDiff()
	previousSignal, previousSignalOK := s.previousSignal()
	return currentDiff, s.currentSignal(), previousDiff, previousSignal, true, previousDiffOK && previousSignalOK
}

func (s *rollingMACDState) detectDivergence(closes []float64, direction string, lookback int) bool {
	if window := s.divergenceWindows[lookback]; window != nil {
		return window.detect(direction)
	}
	if s == nil || lookback <= 0 || len(closes) < lookback+1 || s.fast == nil || s.slow == nil || len(s.fast.tail) < lookback+1 || len(s.slow.tail) < lookback+1 {
		return false
	}
	currentPrice := closes[len(closes)-1]
	currentIndicator, ok := s.currentDiff()
	if !ok {
		return false
	}
	start := len(s.fast.tail) - lookback - 1
	priceStart := len(closes) - lookback - 1
	switch direction {
	case "top":
		maxPrice := closes[priceStart]
		maxIndicator := s.fast.tail[start] - s.slow.tail[start]
		for index := 1; index < lookback; index++ {
			maxPrice = maxFloat(maxPrice, closes[priceStart+index])
			maxIndicator = maxFloat(maxIndicator, s.fast.tail[start+index]-s.slow.tail[start+index])
		}
		return currentPrice > maxPrice && currentIndicator < maxIndicator
	case "bottom":
		minPrice := closes[priceStart]
		minIndicator := s.fast.tail[start] - s.slow.tail[start]
		for index := 1; index < lookback; index++ {
			minPrice = minFloat(minPrice, closes[priceStart+index])
			minIndicator = minFloat(minIndicator, s.fast.tail[start+index]-s.slow.tail[start+index])
		}
		return currentPrice < minPrice && currentIndicator > minIndicator
	default:
		return false
	}
}

func (s *rollingMACDState) pushDivergenceSample(price float64) {
	if s == nil || len(s.divergenceWindows) == 0 {
		return
	}
	s.closeTail = appendTailValue(s.closeTail, price, cap(s.fast.tail))
	indicatorTail := reuseFloat64Slice(s.diffTail, len(s.fast.tail))
	for index := range s.fast.tail {
		indicatorTail[index] = s.fast.tail[index] - s.slow.tail[index]
	}
	s.diffTail = indicatorTail
	for _, window := range s.divergenceWindows {
		window.refresh(s.closeTail, indicatorTail)
	}
}

func (r *indicatorRuntime) pushMACDStates(closeValue float64, trimmed bool, oldFirstClose, oldSecondClose float64, hasOldFirstClose, hasOldSecondClose bool) {
	if r == nil || len(r.macdStates) == 0 {
		return
	}
	for _, state := range r.macdStates {
		state.push(closeValue, trimmed, oldFirstClose, oldSecondClose, hasOldFirstClose, hasOldSecondClose)
	}
}
