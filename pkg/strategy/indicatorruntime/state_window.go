package indicatorruntime
func (s *rollingWindowState) push(value float64) {
	if s == nil || s.config.period <= 0 {
		return
	}
	function := normalizeWindowFunction(s.config.function)
	capacity := s.config.period
	switch function {
	case "change", "mom", "roc", "rising", "falling":
		capacity = s.config.period + 1
	}
	s.values.push(value, capacity)
	switch function {
	case "highest":
		s.pushNumeric(s.calculateHighest())
	case "lowest":
		s.pushNumeric(s.calculateLowest())
	case "sum":
		s.pushNumeric(s.calculateSum())
	case "change", "mom":
		s.pushNumeric(s.calculateMomentum())
	case "roc":
		s.pushNumeric(s.calculateRateOfChange())
	case "rising":
		s.pushBool(s.calculateRising())
	case "falling":
		s.pushBool(s.calculateFalling())
	}
}

func (s *rollingWindowState) pushNumeric(value float64, ok bool) {
	if s == nil {
		return
	}
	if !ok {
		s.hasCurrent = false
		return
	}
	if s.hasCurrent {
		s.previous = s.current
		s.hasPrevious = true
	}
	s.current = value
	s.hasCurrent = true
}

func (s *rollingWindowState) pushBool(value bool, ok bool) {
	if s == nil {
		return
	}
	if !ok {
		s.hasCurrent = false
		return
	}
	s.boolCurrent = value
	s.hasCurrent = true
}

func (s *rollingWindowState) calculateSum() (float64, bool) {
	if s == nil || s.values.len() < s.config.period {
		return 0, false
	}
	total := 0.0
	for index := 0; index < s.values.len(); index++ {
		value, _ := s.values.at(index)
		total += value
	}
	return total, true
}

func (s *rollingWindowState) calculateHighest() (float64, bool) {
	if s == nil || s.values.len() < s.config.period {
		return 0, false
	}
	value, _ := s.values.at(0)
	for index := 1; index < s.values.len(); index++ {
		next, _ := s.values.at(index)
		value = maxFloat(value, next)
	}
	return value, true
}

func (s *rollingWindowState) calculateLowest() (float64, bool) {
	if s == nil || s.values.len() < s.config.period {
		return 0, false
	}
	value, _ := s.values.at(0)
	for index := 1; index < s.values.len(); index++ {
		next, _ := s.values.at(index)
		value = minFloat(value, next)
	}
	return value, true
}

func (s *rollingWindowState) calculateMomentum() (float64, bool) {
	if s == nil || s.values.len() < s.config.period+1 {
		return 0, false
	}
	base, _ := s.values.at(0)
	current, _ := s.values.last()
	return current - base, true
}

func (s *rollingWindowState) calculateRateOfChange() (float64, bool) {
	baseChange, ok := s.calculateMomentum()
	if !ok {
		return 0, false
	}
	base, _ := s.values.at(0)
	if base == 0 {
		return 0, false
	}
	return baseChange / base * 100, true
}

func (s *rollingWindowState) calculateRising() (bool, bool) {
	if s == nil || s.values.len() < s.config.period+1 {
		return false, false
	}
	current, _ := s.values.last()
	for index := 0; index < s.values.len()-1; index++ {
		previous, _ := s.values.at(index)
		if current <= previous {
			return false, true
		}
	}
	return true, true
}

func (s *rollingWindowState) calculateFalling() (bool, bool) {
	if s == nil || s.values.len() < s.config.period+1 {
		return false, false
	}
	current, _ := s.values.last()
	for index := 0; index < s.values.len()-1; index++ {
		previous, _ := s.values.at(index)
		if current >= previous {
			return false, true
		}
	}
	return true, true
}


func newRollingWindowStates(requirements indicatorRequirements) map[windowConfig]*rollingWindowState {
	if len(requirements.windows) == 0 {
		return nil
	}
	states := make(map[windowConfig]*rollingWindowState, len(requirements.windows))
	for _, config := range requirements.windows {
		if config.period <= 0 || normalizeWindowFunction(config.function) == "" {
			continue
		}
		if _, ok := parseOHLCVSource(config.source); !ok {
			continue
		}
		resolved := config
		resolved.function = normalizeWindowFunction(config.function)
		resolved.source, _ = parseOHLCVSource(config.source)
		states[config] = &rollingWindowState{config: resolved}
	}
	if len(states) == 0 {
		return nil
	}
	return states
}


func (r *indicatorRuntime) pushWindowStates(openValue, high, low, closeValue, volume float64) {
	if r == nil || len(r.windowStates) == 0 {
		return
	}
	for _, state := range r.windowStates {
		value, ok := ohlcvSourceValue(state.config.source, openValue, high, low, closeValue, volume)
		if !ok {
			continue
		}
		state.push(value)
	}
}

