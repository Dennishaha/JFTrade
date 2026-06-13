package indicatorruntime
func newRollingDivergenceWindowStates(lookbacks []int) map[int]*rollingDivergenceWindowState {
	if len(lookbacks) == 0 {
		return nil
	}
	result := make(map[int]*rollingDivergenceWindowState, len(lookbacks))
	for _, lookback := range lookbacks {
		if lookback <= 0 {
			continue
		}
		result[lookback] = &rollingDivergenceWindowState{lookback: lookback}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (s *rollingDivergenceWindowState) refresh(priceSeries, indicatorSeries []float64) {
	if s == nil || s.lookback <= 0 || len(priceSeries) < s.lookback+1 || len(indicatorSeries) < s.lookback+1 {
		if s != nil {
			s.ready = false
		}
		return
	}
	priceStart := len(priceSeries) - s.lookback - 1
	indicatorStart := len(indicatorSeries) - s.lookback - 1
	s.currentPrice = priceSeries[len(priceSeries)-1]
	s.currentIndicator = indicatorSeries[len(indicatorSeries)-1]
	s.previousMaxPrice = priceSeries[priceStart]
	s.previousMinPrice = priceSeries[priceStart]
	s.previousMaxIndicator = indicatorSeries[indicatorStart]
	s.previousMinIndicator = indicatorSeries[indicatorStart]
	for index := 1; index < s.lookback; index++ {
		s.previousMaxPrice = maxFloat(s.previousMaxPrice, priceSeries[priceStart+index])
		s.previousMinPrice = minFloat(s.previousMinPrice, priceSeries[priceStart+index])
		s.previousMaxIndicator = maxFloat(s.previousMaxIndicator, indicatorSeries[indicatorStart+index])
		s.previousMinIndicator = minFloat(s.previousMinIndicator, indicatorSeries[indicatorStart+index])
	}
	s.ready = true
}

func (s *rollingDivergenceWindowState) detect(direction string) bool {
	if s == nil || !s.ready {
		return false
	}
	switch direction {
	case "top":
		return s.currentPrice > s.previousMaxPrice && s.currentIndicator < s.previousMaxIndicator
	case "bottom":
		return s.currentPrice < s.previousMinPrice && s.currentIndicator > s.previousMinIndicator
	default:
		return false
	}
}

