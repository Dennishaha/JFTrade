package indicatorruntime

type indicatorSeriesSnapshot struct {
	current     float64
	previous    float64
	hasCurrent  bool
	hasPrevious bool
}

type indicatorMACDSnapshot struct {
	diff              float64
	signal            float64
	histogram         float64
	previousDiff      float64
	previousSignal    float64
	previousHistogram float64
	hasPrevious       bool
}

type indicatorKDJSnapshot struct {
	k           float64
	d           float64
	j           float64
	previousK   float64
	previousD   float64
	previousJ   float64
	hasPrevious bool
}

type indicatorScalarSnapshot struct {
	current    float64
	hasCurrent bool
}

type reusableKDJSeriesBuffer struct {
	series    kdjSeries
	highDeque indexDeque
	lowDeque  indexDeque
}

type tradingPeriodLabelCacheKey struct {
	symbol               string
	unit                 string
	includeExtendedHours bool
}

type stopLossWindowStartCacheEntry struct {
	valid           bool
	requestedStart  int
	intervalMinutes int
	seriesLength    int
	resolvedStart   int
}

type stopLossWindowSelectionCacheEntry struct {
	valid                bool
	period               int
	timeUnit             string
	symbol               string
	upperBound           int
	includeExtendedHours bool
	indices              []int
}

type stopLossWindowExtremaCacheEntry struct {
	valid        bool
	windowStart  int
	seriesLength int
	peakClose    float64
	troughClose  float64
}

type snapshotKeyCache struct {
	ma             map[movingAverageConfig]string
	maLegacy       map[movingAverageConfig]string
	securitySource map[securitySourceConfig]string
	rsi            map[int]string
	rsiSource      map[sourcePeriodConfig]string
	macd           map[macdConfig]string
	bollinger      map[bollingerConfig]string
	kdj            map[kdjConfig]string
	atr            map[int]string
	stdev          map[int]string
	stdevSource    map[sourcePeriodConfig]string
	variance       map[sourcePeriodConfig]string
	windows        map[windowConfig]string
	cum            map[sourceConfig]string
	stoch          map[sourcePeriodConfig]string
	cci            map[int]string
	cciSource      map[sourcePeriodConfig]string
	williamsR      map[int]string
	vwap           map[sourceConfig]string
	mfi            map[sourcePeriodConfig]string
	dmi            map[dmiConfig]string
	supertrend     map[supertrendConfig]string
	sar            map[sarConfig]string
	stopLoss       map[stopLossConfig]string
	rsiDivergence  map[rsiDivergenceConfig]string
	macdDivergence map[macdDivergenceConfig]string
	kdjDivergence  map[kdjDivergenceConfig]string
	advanced       map[advancedIndicatorConfig]string
	resultCapacity int
}

func (s *indicatorScalarSnapshot) ScalarValue() (float64, bool) {
	if s == nil || !s.hasCurrent {
		return 0, false
	}
	return s.current, true
}

func (s *indicatorSeriesSnapshot) PreferredScalarValue() (float64, bool) {
	if s == nil || !s.hasCurrent {
		return 0, false
	}
	return s.current, true
}

func (s *indicatorSeriesSnapshot) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if s == nil || name != "value" {
		return 0, 0, false, false, false
	}
	return s.current, s.previous, s.hasCurrent, s.hasPrevious, true
}

func (s *indicatorSeriesSnapshot) FieldValue(name string) (any, bool) {
	switch name {
	case "value":
		if s.hasCurrent {
			return s.current, true
		}
		return nil, true
	case "previous":
		if s.hasPrevious {
			return s.previous, true
		}
		return nil, true
	default:
		return nil, false
	}
}

func (s *indicatorMACDSnapshot) PreferredScalarValue() (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.diff, true
}

func (s *indicatorMACDSnapshot) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if s == nil {
		return 0, 0, false, false, false
	}
	switch name {
	case "diff":
		return s.diff, s.previousDiff, true, s.hasPrevious, true
	case "signal":
		return s.signal, s.previousSignal, true, s.hasPrevious, true
	case "histogram":
		return s.histogram, s.previousHistogram, true, s.hasPrevious, true
	default:
		return 0, 0, false, false, false
	}
}

func (s *indicatorMACDSnapshot) FieldValue(name string) (any, bool) {
	switch name {
	case "diff":
		return s.diff, true
	case "signal":
		return s.signal, true
	case "histogram":
		return s.histogram, true
	case "previousDiff":
		if s.hasPrevious {
			return s.previousDiff, true
		}
		return nil, true
	case "previousSignal":
		if s.hasPrevious {
			return s.previousSignal, true
		}
		return nil, true
	case "previousHistogram":
		if s.hasPrevious {
			return s.previousHistogram, true
		}
		return nil, true
	default:
		return nil, false
	}
}

func (s *indicatorKDJSnapshot) PreferredScalarValue() (float64, bool) {
	if s == nil {
		return 0, false
	}
	return s.k, true
}

func (s *indicatorKDJSnapshot) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if s == nil {
		return 0, 0, false, false, false
	}
	switch name {
	case "k":
		return s.k, s.previousK, true, s.hasPrevious, true
	case "d":
		return s.d, s.previousD, true, s.hasPrevious, true
	case "j":
		return s.j, s.previousJ, true, s.hasPrevious, true
	default:
		return 0, 0, false, false, false
	}
}

func (s *indicatorKDJSnapshot) FieldValue(name string) (any, bool) {
	switch name {
	case "k":
		return s.k, true
	case "d":
		return s.d, true
	case "j":
		return s.j, true
	case "previousK":
		if s.hasPrevious {
			return s.previousK, true
		}
		return nil, true
	case "previousD":
		if s.hasPrevious {
			return s.previousD, true
		}
		return nil, true
	case "previousJ":
		if s.hasPrevious {
			return s.previousJ, true
		}
		return nil, true
	default:
		return nil, false
	}
}

type indexDeque struct {
	indices []int
}
