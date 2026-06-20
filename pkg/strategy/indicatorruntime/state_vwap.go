package indicatorruntime

import "time"

type rollingVWAPState struct {
	periodKey   string
	totalPV     float64
	totalVolume float64
	hasCurrent  bool
}

func (s *rollingVWAPState) push(periodKey string, value, volume float64) {
	if s == nil {
		return
	}
	if periodKey == "" {
		s.periodKey = ""
		s.totalPV = 0
		s.totalVolume = 0
		s.hasCurrent = false
		return
	}
	if !s.hasCurrent || s.periodKey != periodKey {
		s.periodKey = periodKey
		s.totalPV = 0
		s.totalVolume = 0
		s.hasCurrent = true
	}
	s.totalPV += value * volume
	s.totalVolume += volume
}

func (s *rollingVWAPState) value() (float64, bool) {
	if s == nil || !s.hasCurrent || s.totalVolume <= 0 {
		return 0, false
	}
	return s.totalPV / s.totalVolume, true
}

func newRollingVWAPStates(requirements indicatorRequirements) (map[sourceConfig]*rollingVWAPState, map[advancedIndicatorConfig]*rollingVWAPState) {
	sessionStates := make(map[sourceConfig]*rollingVWAPState, len(requirements.vwap))
	for _, config := range requirements.vwap {
		sessionStates[config] = &rollingVWAPState{}
	}
	anchoredStates := map[advancedIndicatorConfig]*rollingVWAPState{}
	for _, config := range requirements.advanced {
		if config.kind == "anchored_vwap" {
			anchoredStates[config] = &rollingVWAPState{}
		}
	}
	return sessionStates, anchoredStates
}

func (r *indicatorRuntime) pushVWAPStates(openValue, high, low, closeValue, volume float64, at time.Time) {
	if r == nil || (len(r.vwapStates) == 0 && len(r.anchoredVWAPStates) == 0) {
		return
	}
	periodKeys := map[string]string{}
	periodKey := func(unit string) string {
		if key, ok := periodKeys[unit]; ok {
			return key
		}
		key, _ := vwapPeriodKey(r.symbol, at, unit, r.includeExtendedHours)
		periodKeys[unit] = key
		return key
	}
	for config, state := range r.vwapStates {
		value, ok := ohlcvSourceValue(config.source, openValue, high, low, closeValue, volume)
		if ok {
			state.push(periodKey("day"), value, volume)
		}
	}
	for config, state := range r.anchoredVWAPStates {
		value, ok := ohlcvSourceValue(config.source, openValue, high, low, closeValue, volume)
		if ok {
			state.push(periodKey(config.timeUnit), value, volume)
		}
	}
}
