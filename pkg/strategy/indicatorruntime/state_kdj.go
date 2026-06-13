package indicatorruntime

import (
	"math"
)

func newRollingKDJStates(requirements indicatorRequirements, seriesLimit int) map[kdjConfig]*rollingKDJState {
	lookbacks := map[kdjConfig]map[int]struct{}{}
	for _, config := range requirements.kdj {
		if _, ok := lookbacks[config]; !ok {
			lookbacks[config] = map[int]struct{}{}
		}
	}
	for _, config := range requirements.kdjDivergence {
		base := kdjConfig{period: config.period, m1: config.m1, m2: config.m2}
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
	states := make(map[kdjConfig]*rollingKDJState, len(lookbacks))
	for config, lookbackSet := range lookbacks {
		if config.period <= 0 || config.m1 <= 0 || config.m2 <= 0 {
			continue
		}
		states[config] = newRollingKDJState(config, seriesLimit, sortedLookbacks(lookbackSet))
	}
	if len(states) == 0 {
		return nil
	}
	return states
}


func newRollingKDJState(config kdjConfig, limit int, lookbacks []int) *rollingKDJState {
	maxLookback := 1
	for _, lookback := range lookbacks {
		maxLookback = max(maxLookback, lookback)
	}
	tailLen := max(maxLookback+1, 2)
	if config.period <= 0 || config.m1 <= 0 || config.m2 <= 0 || tailLen <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = minimumIndicatorSeriesLimit
	}
	prefixCap := min(limit, config.period+1)
	state := &rollingKDJState{
		config:            config,
		limit:             limit,
		tailLen:           max(tailLen, 1),
		kAlpha:            1 / float64(config.m1),
		dAlpha:            1 / float64(config.m2),
		kTail:             make([]float64, 0, max(tailLen, 1)),
		dTail:             make([]float64, 0, max(tailLen, 1)),
		jTail:             make([]float64, 0, max(tailLen, 1)),
		prefixK:           make([]float64, 0, prefixCap),
		prefixD:           make([]float64, 0, prefixCap),
		prefixJ:           make([]float64, 0, prefixCap),
		boundaryK:         make([]float64, limit),
		boundaryDByK:      make([]float64, limit),
		boundaryDByD:      make([]float64, limit),
		closeTail:         make([]float64, 0, tailLen),
		divergenceWindows: newRollingDivergenceWindowStates(lookbacks),
	}
	state.kBeta = 1 - state.kAlpha
	state.dBeta = 1 - state.dAlpha
	if limit > 0 {
		state.boundaryK[0] = 1
		state.boundaryDByD[0] = 1
		for step := 1; step < limit; step++ {
			state.boundaryK[step] = state.boundaryK[step-1] * state.kBeta
			state.boundaryDByD[step] = state.boundaryDByD[step-1] * state.dBeta
			state.boundaryDByK[step] = state.dBeta*state.boundaryDByK[step-1] + state.dAlpha*state.boundaryK[step]
		}
	}
	return state
}

func (s *rollingKDJState) push(highs, lows, closes []float64, high, low, closeValue float64, trimmed bool) {
	if s == nil || s.config.period <= 0 {
		return
	}
	windowStart := s.index - s.config.period + 1
	s.highDeque.popExpired(windowStart)
	s.lowDeque.popExpired(windowStart)
	s.highDeque.pushMax(s.index, high)
	s.lowDeque.pushMin(s.index, low)
	s.index++
	if trimmed {
		s.trimState(highs, lows, closes)
	}
	highestHigh := high
	lowestLow := low
	if value, ok := s.highDeque.frontValue(); ok {
		highestHigh = value
	}
	if value, ok := s.lowDeque.frontValue(); ok {
		lowestLow = value
	}
	rsv := calculateKDJRSV(highestHigh, lowestLow, closeValue)
	previousK := 50.0
	previousD := 50.0
	if len(s.kTail) > 0 {
		previousK = s.kTail[len(s.kTail)-1]
		previousD = s.dTail[len(s.dTail)-1]
	}
	currentK := s.kBeta*previousK + s.kAlpha*rsv
	currentD := s.dBeta*previousD + s.dAlpha*currentK
	currentJ := 3*currentK - 2*currentD
	s.appendValues(currentK, currentD, currentJ)
	if !trimmed {
		prefixCap := min(s.limit, s.config.period+1)
		if len(s.prefixK) < prefixCap {
			s.prefixK = append(s.prefixK, currentK)
			s.prefixD = append(s.prefixD, currentD)
			s.prefixJ = append(s.prefixJ, currentJ)
		}
	}
	if trimmed {
		s.windowLen = s.limit
		s.pushDivergenceSample(closeValue)
		return
	}
	s.windowLen = min(s.windowLen+1, s.limit)
	s.pushDivergenceSample(closeValue)
}

func (s *rollingKDJState) trimState(highs, lows, closes []float64) {
	if s == nil {
		return
	}
	if s.windowLen <= 1 || s.limit <= 1 || len(closes) <= 1 {
		s.kTail = s.kTail[:0]
		s.dTail = s.dTail[:0]
		s.jTail = s.jTail[:0]
		s.prefixK = s.prefixK[:0]
		s.prefixD = s.prefixD[:0]
		s.prefixJ = s.prefixJ[:0]
		return
	}
	prefixLen := min(len(closes)-1, s.config.period+1)
	oldBoundaryK := 0.0
	oldBoundaryD := 0.0
	hasBoundary := len(s.prefixK) > s.config.period && len(s.prefixD) > s.config.period && prefixLen >= s.config.period
	if hasBoundary {
		oldBoundaryK = s.prefixK[s.config.period]
		oldBoundaryD = s.prefixD[s.config.period]
	}
	if prefixLen > 0 {
		prefixSeries := calculateKDJSeriesWithBuffer(&s.prefixBuffer, highs[1:1+prefixLen], lows[1:1+prefixLen], closes[1:1+prefixLen], s.config)
		s.prefixK = reuseFloat64Slice(s.prefixK, prefixLen)
		s.prefixD = reuseFloat64Slice(s.prefixD, prefixLen)
		s.prefixJ = reuseFloat64Slice(s.prefixJ, prefixLen)
		copy(s.prefixK, prefixSeries.k)
		copy(s.prefixD, prefixSeries.d)
		copy(s.prefixJ, prefixSeries.j)
	} else {
		s.prefixK = s.prefixK[:0]
		s.prefixD = s.prefixD[:0]
		s.prefixJ = s.prefixJ[:0]
	}
	deltaK := 0.0
	deltaD := 0.0
	if hasBoundary {
		deltaK = s.prefixK[s.config.period-1] - oldBoundaryK
		deltaD = s.prefixD[s.config.period-1] - oldBoundaryD
	}
	startIndex := s.windowLen - len(s.kTail)
	retained := 0
	for index := range s.kTail {
		oldIndex := startIndex + index
		if oldIndex == 0 {
			continue
		}
		newIndex := oldIndex - 1
		if newIndex < len(s.prefixK) {
			s.kTail[retained] = s.prefixK[newIndex]
			s.dTail[retained] = s.prefixD[newIndex]
			s.jTail[retained] = s.prefixJ[newIndex]
			retained++
			continue
		}
		steps := oldIndex - s.config.period
		shiftK := s.boundaryKAt(steps) * deltaK
		shiftD := s.boundaryDByKAt(steps)*deltaK + s.boundaryDByDAt(steps)*deltaD
		oldK := s.kTail[index]
		oldD := s.dTail[index]
		oldJ := s.jTail[index]
		s.kTail[retained] = oldK + shiftK
		s.dTail[retained] = oldD + shiftD
		s.jTail[retained] = oldJ + (3*shiftK - 2*shiftD)
		retained++
	}
	s.kTail = s.kTail[:retained]
	s.dTail = s.dTail[:retained]
	s.jTail = s.jTail[:retained]
}

func (s *rollingKDJState) appendValues(kValue, dValue, jValue float64) {
	if s == nil {
		return
	}
	if len(s.kTail) < s.tailLen {
		s.kTail = append(s.kTail, kValue)
		s.dTail = append(s.dTail, dValue)
		s.jTail = append(s.jTail, jValue)
		return
	}
	copy(s.kTail, s.kTail[1:])
	copy(s.dTail, s.dTail[1:])
	copy(s.jTail, s.jTail[1:])
	s.kTail[len(s.kTail)-1] = kValue
	s.dTail[len(s.dTail)-1] = dValue
	s.jTail[len(s.jTail)-1] = jValue
}

func (s *rollingKDJState) snapshotValues() (float64, float64, float64, float64, float64, float64, bool, bool) {
	if s == nil || len(s.kTail) == 0 || len(s.dTail) == 0 || len(s.jTail) == 0 {
		return 0, 0, 0, 0, 0, 0, false, false
	}
	last := len(s.kTail) - 1
	previousOK := s.windowLen > 1 && last > 0
	previousK := 0.0
	previousD := 0.0
	previousJ := 0.0
	if previousOK {
		previousK = s.kTail[last-1]
		previousD = s.dTail[last-1]
		previousJ = s.jTail[last-1]
	}
	return s.kTail[last], s.dTail[last], s.jTail[last], previousK, previousD, previousJ, true, previousOK
}

func (s *rollingKDJState) detectDivergence(closes []float64, direction string, lookback int) bool {
	if window := s.divergenceWindows[lookback]; window != nil {
		return window.detect(direction)
	}
	if s == nil || lookback <= 0 || len(s.jTail) < lookback+1 {
		return false
	}
	alignedCloses, alignedJ := alignSeries(closes, s.jTail)
	return detectDivergence(alignedCloses, alignedJ, direction, lookback)
}

func (s *rollingKDJState) pushDivergenceSample(price float64) {
	if s == nil || len(s.divergenceWindows) == 0 {
		return
	}
	s.closeTail = appendTailValue(s.closeTail, price, s.tailLen)
	for _, window := range s.divergenceWindows {
		window.refresh(s.closeTail, s.jTail)
	}
}

func (s *rollingKDJState) boundaryKAt(step int) float64 {
	if s == nil || step < 0 {
		return 0
	}
	if step < len(s.boundaryK) {
		return s.boundaryK[step]
	}
	return math.Pow(s.kBeta, float64(step))
}

func (s *rollingKDJState) boundaryDByKAt(step int) float64 {
	if s == nil || step < 0 {
		return 0
	}
	if step < len(s.boundaryDByK) {
		return s.boundaryDByK[step]
	}
	shift := 0.0
	kShift := 1.0
	for index := 1; index <= step; index++ {
		kShift *= s.kBeta
		shift = s.dBeta*shift + s.dAlpha*kShift
	}
	return shift
}

func (s *rollingKDJState) boundaryDByDAt(step int) float64 {
	if s == nil || step < 0 {
		return 0
	}
	if step < len(s.boundaryDByD) {
		return s.boundaryDByD[step]
	}
	return math.Pow(s.dBeta, float64(step))
}


func (r *indicatorRuntime) pushKDJStates(highs, lows, closes []float64, high, low, closeValue float64, trimmed bool) {
	if r == nil || len(r.kdjStates) == 0 {
		return
	}
	for _, state := range r.kdjStates {
		state.push(highs, lows, closes, high, low, closeValue, trimmed)
	}
}

