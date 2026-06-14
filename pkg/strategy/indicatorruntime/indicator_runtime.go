package indicatorruntime

import (
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

const minimumIndicatorSeriesLimit = 256

const invalidTradingPeriodLabelKey int64 = -1 << 63

type indicatorRuntime struct {
	requirements         indicatorRequirements
	symbol               string
	intervalMinutes      int
	includeExtendedHours bool
	seriesLimit          int
	tradingPeriodUnits   []string
	tradingPeriodLabels  map[string][]int64
	snapshotCache        *snapshotSeriesCache
	snapshotKeys         snapshotKeyCache
	snapshotResult       map[string]any
	maStates             map[movingAverageConfig]*rollingMovingAverageSnapshotState
	emaStates            map[movingAverageConfig]*rollingEMATailState
	macdStates           map[macdConfig]*rollingMACDState
	kdjStates            map[kdjConfig]*rollingKDJState
	rsiStates            map[int]*rollingRSIState
	atrStates            map[int]*rollingATRState
	stdevStates          map[int]*rollingStdDevState
	windowStates         map[windowConfig]*rollingWindowState
	cumStates            map[sourceConfig]*rollingCumState
	stochStates          map[sourcePeriodConfig]*rollingStochState
	bollingerStates      map[bollingerConfig]*rollingBollingerState
	cciStates            map[int]*rollingCCIState
	williamsRStates      map[int]*rollingWilliamsRState
	obvStates            map[advancedIndicatorConfig]*rollingCumState
	opens                []float64
	highs                []float64
	lows                 []float64
	closes               []float64
	volumes              []float64
	endTimes             []time.Time
	sessions             []market.Session
}

type snapshotSeriesCache struct {
	ema                   map[int][]float64
	sma                   map[int][]float64
	smma                  map[int][]float64
	wma                   map[int][]float64
	tma                   map[int][]float64
	hma                   map[int][]float64
	rsi                   map[int][]float64
	macd                  map[macdConfig]macdSeries
	kdj                   map[kdjConfig]kdjSeries
	emaBuffers            map[int][]float64
	macdBuffers           map[macdConfig]macdSeries
	kdjBuffers            map[kdjConfig]*reusableKDJSeriesBuffer
	tradingPeriodLabels   map[tradingPeriodLabelCacheKey][]int64
	tradingPeriodBuffers  map[tradingPeriodLabelCacheKey][]int64
	maSnapshots           map[movingAverageConfig]*indicatorSeriesSnapshot
	seriesSnapshots       map[string]*indicatorSeriesSnapshot
	windowSnapshots       map[windowConfig]*indicatorSeriesSnapshot
	macdSnapshots         map[macdConfig]*indicatorMACDSnapshot
	kdjSnapshots          map[kdjConfig]*indicatorKDJSnapshot
	scalarValues          map[string]*indicatorScalarSnapshot
	stopLossSnapshots     map[stopLossConfig]map[string]any
	tradingWindowIndices  []int
	tradingWindowValues   []float64
	tradingWindowVolumes  []float64
	stopLossWindowStart   stopLossWindowStartCacheEntry
	stopLossWindowSelect  stopLossWindowSelectionCacheEntry
	stopLossWindowExtrema stopLossWindowExtremaCacheEntry
}

type macdSeries struct {
	diff   []float64
	signal []float64
}

type kdjSeries struct {
	k []float64
	d []float64
	j []float64
}

type rollingRSIState struct {
	period            int
	maxLength         int
	tailLen           int
	gains             rollingFloatWindow
	losses            rollingFloatWindow
	gainSum           float64
	lossSum           float64
	series            []float64
	valueTail         []float64
	closeTail         []float64
	divergenceWindows map[int]*rollingDivergenceWindowState
}

type rollingMovingAverageSnapshotState struct {
	kind        string
	period      int
	values      rollingFloatWindow
	volumes     rollingFloatWindow
	sum         float64
	weightedSum float64
	volumeSum   float64
	current     float64
	previous    float64
	hasCurrent  bool
	hasPrevious bool
}

type rollingWindowState struct {
	config      windowConfig
	values      rollingFloatWindow
	current     float64
	previous    float64
	boolCurrent bool
	hasCurrent  bool
	hasPrevious bool
}

type rollingCumState struct {
	current     float64
	previous    float64
	hasCurrent  bool
	hasPrevious bool
}

type rollingStochState struct {
	source      string
	period      int
	index       int
	highDeque   monotonicWindowValueDeque
	lowDeque    monotonicWindowValueDeque
	current     float64
	previous    float64
	hasCurrent  bool
	hasPrevious bool
}

type rollingEMATailState struct {
	period    int
	limit     int
	tailLen   int
	alpha     float64
	beta      float64
	windowLen int
	tail      []float64
	powers    []float64
}

type rollingMACDState struct {
	config                macdConfig
	minimum               int
	fast                  *rollingEMATailState
	slow                  *rollingEMATailState
	closeTail             []float64
	diffTail              []float64
	divergenceWindows     map[int]*rollingDivergenceWindowState
	signalAlpha           float64
	signalBeta            float64
	signalWeightedSum     float64
	signalShiftAdjustment float64
}

type rollingKDJState struct {
	config            kdjConfig
	limit             int
	tailLen           int
	windowLen         int
	index             int
	kAlpha            float64
	kBeta             float64
	dAlpha            float64
	dBeta             float64
	highDeque         monotonicWindowValueDeque
	lowDeque          monotonicWindowValueDeque
	kTail             []float64
	dTail             []float64
	jTail             []float64
	prefixK           []float64
	prefixD           []float64
	prefixJ           []float64
	boundaryK         []float64
	boundaryDByK      []float64
	boundaryDByD      []float64
	prefixBuffer      reusableKDJSeriesBuffer
	closeTail         []float64
	divergenceWindows map[int]*rollingDivergenceWindowState
}

type rollingATRState struct {
	period    int
	window    rollingFloatWindow
	windowSum float64
	current   float64
	hasValue  bool
}

type rollingStdDevState struct {
	period     int
	window     rollingFloatWindow
	sum        float64
	sumSquares float64
	current    float64
	hasValue   bool
}

type rollingBollingerState struct {
	period     int
	multiplier float64
	window     rollingFloatWindow
	sum        float64
	sumSquares float64
}

type rollingCCIState struct {
	period   int
	window   rollingFloatWindow
	sum      float64
	current  float64
	hasValue bool
}

type rollingWilliamsRState struct {
	period    int
	index     int
	highDeque monotonicWindowValueDeque
	lowDeque  monotonicWindowValueDeque
	current   float64
	hasValue  bool
}

type windowValue struct {
	index int
	value float64
}

type rollingDivergenceWindowState struct {
	lookback             int
	currentPrice         float64
	currentIndicator     float64
	previousMaxPrice     float64
	previousMinPrice     float64
	previousMaxIndicator float64
	previousMinIndicator float64
	ready                bool
}

type monotonicWindowValueDeque struct {
	values []windowValue
	start  int
}

type rollingFloatWindow struct {
	values []float64
	start  int
	count  int
}

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

func newIndicatorRuntime(script string, interval types.Interval, symbol string) *indicatorRuntime {
	return newIndicatorRuntimeWithOptions(script, interval, symbol, RuntimeOptions{})
}

func newIndicatorRuntimeWithOptions(script string, interval types.Interval, symbol string, options RuntimeOptions) *indicatorRuntime {
	requirements := parseIndicatorRequirements(script)
	return newIndicatorRuntimeWithRequirements(requirements, interval, symbol, options)
}

func newIndicatorRuntimeFromPlan(plan strategyir.Requirements, interval types.Interval, symbol string) (*indicatorRuntime, error) {
	return newIndicatorRuntimeFromPlanWithOptions(plan, interval, symbol, RuntimeOptions{})
}

func newIndicatorRuntimeFromPlanWithOptions(plan strategyir.Requirements, interval types.Interval, symbol string, options RuntimeOptions) (*indicatorRuntime, error) {
	requirements, err := indicatorRequirementsFromPlan(plan)
	if err != nil {
		return nil, err
	}
	return newIndicatorRuntimeWithRequirements(requirements, interval, symbol, options), nil
}

func newIndicatorRuntimeWithRequirements(requirements indicatorRequirements, interval types.Interval, symbol string, options RuntimeOptions) *indicatorRuntime {
	if requirements.isEmpty() {
		return nil
	}
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	intervalMinutes := resolveIntervalMinutes(interval)
	seriesLimit := calculateIndicatorSeriesLimit(requirements, intervalMinutes)
	runtime := &indicatorRuntime{
		requirements:         requirements,
		symbol:               normalizedSymbol,
		intervalMinutes:      intervalMinutes,
		includeExtendedHours: options.IncludeExtendedHours,
		seriesLimit:          seriesLimit,
		tradingPeriodUnits:   collectTradingPeriodUnits(requirements, intervalMinutes, normalizedSymbol, options.IncludeExtendedHours),
		tradingPeriodLabels:  map[string][]int64{},
		snapshotCache:        newSnapshotSeriesCache(),
		snapshotKeys:         buildSnapshotKeyCache(requirements),
		maStates:             newRollingMovingAverageStates(requirements, intervalMinutes),
		emaStates:            newRollingEMAStates(requirements, intervalMinutes, seriesLimit),
		macdStates:           newRollingMACDStates(requirements, seriesLimit),
		kdjStates:            newRollingKDJStates(requirements, seriesLimit),
		rsiStates:            newRollingRSIStates(requirements, seriesLimit),
		atrStates:            newRollingATRStates(requirements),
		stdevStates:          newRollingStdDevStates(requirements),
		windowStates:         newRollingWindowStates(requirements),
		cumStates:            newRollingCumStates(requirements),
		stochStates:          newRollingStochStates(requirements),
		bollingerStates:      newRollingBollingerStates(requirements),
		cciStates:            newRollingCCIStates(requirements),
		williamsRStates:      newRollingWilliamsRStates(requirements),
		obvStates:            newOBVStates(requirements),
	}
	if seriesLimit > 0 {
		runtime.opens = make([]float64, 0, seriesLimit)
		runtime.highs = make([]float64, 0, seriesLimit)
		runtime.lows = make([]float64, 0, seriesLimit)
		runtime.closes = make([]float64, 0, seriesLimit)
		runtime.volumes = make([]float64, 0, seriesLimit)
		runtime.endTimes = make([]time.Time, 0, seriesLimit)
		runtime.sessions = make([]market.Session, 0, seriesLimit)
		for _, unit := range runtime.tradingPeriodUnits {
			runtime.tradingPeriodLabels[unit] = make([]int64, 0, seriesLimit)
		}
	}
	return runtime
}

func (r *indicatorRuntime) push(kline types.KLine, session market.Session) {
	if r == nil {
		return
	}
	closeValue := kline.Close.Float64()
	openValue := kline.Open.Float64()
	highValue := kline.High.Float64()
	lowValue := kline.Low.Float64()
	volumeValue := kline.Volume.Float64()
	oldFirst := r.oldSourceValuesAt(0)
	oldSecond := r.oldSourceValuesAt(1)
	oldFirstClose := oldFirst["close"]
	oldSecondClose := oldSecond["close"]
	hasOldFirstClose := len(r.closes) > 0
	hasOldSecondClose := len(r.closes) > 1
	previousClose := 0.0
	hasPreviousClose := len(r.closes) > 0
	if hasPreviousClose {
		previousClose = r.closes[len(r.closes)-1]
	}
	resolvedSession := session
	if resolvedSession == market.SessionUnknown {
		resolvedSession = classifyKLineSession(r.symbol, kline)
	}
	seriesLimit := r.seriesLimit
	if seriesLimit <= 0 {
		seriesLimit = minimumIndicatorSeriesLimit
	}
	trimmed := len(r.closes)+1 > seriesLimit
	r.pushKDJStates(r.highs, r.lows, r.closes, highValue, lowValue, closeValue, trimmed)
	r.opens = append(r.opens, openValue)
	r.highs = append(r.highs, highValue)
	r.lows = append(r.lows, lowValue)
	r.closes = append(r.closes, closeValue)
	r.volumes = append(r.volumes, volumeValue)
	r.endTimes = append(r.endTimes, kline.EndTime.Time())
	r.sessions = append(r.sessions, resolvedSession)
	r.appendTradingPeriodLabels(kline.EndTime.Time())
	if trimmed {
		r.opens = trimFloatSeriesInPlace(r.opens, seriesLimit)
		r.highs = trimFloatSeriesInPlace(r.highs, seriesLimit)
		r.lows = trimFloatSeriesInPlace(r.lows, seriesLimit)
		r.closes = trimFloatSeriesInPlace(r.closes, seriesLimit)
		r.volumes = trimFloatSeriesInPlace(r.volumes, seriesLimit)
		r.endTimes = trimTimeSeriesInPlace(r.endTimes, seriesLimit)
		r.sessions = trimSessionSeriesInPlace(r.sessions, seriesLimit)
		for _, unit := range r.tradingPeriodUnits {
			r.tradingPeriodLabels[unit] = trimInt64SeriesInPlace(r.tradingPeriodLabels[unit], seriesLimit)
		}
	}
	r.pushMovingAverageStates(openValue, highValue, lowValue, closeValue, volumeValue)
	r.pushEMAStates(openValue, highValue, lowValue, closeValue, volumeValue, trimmed, oldFirst, oldSecond, hasOldFirstClose, hasOldSecondClose)
	r.pushMACDStates(closeValue, trimmed, oldFirstClose, oldSecondClose, hasOldFirstClose, hasOldSecondClose)
	r.pushRSIStates(closeValue, previousClose, hasPreviousClose)
	r.pushATRStates(highValue, lowValue, closeValue, previousClose, hasPreviousClose)
	r.pushStdDevStates(closeValue)
	r.pushWindowStates(openValue, highValue, lowValue, closeValue, volumeValue)
	r.pushCumStates(openValue, highValue, lowValue, closeValue, volumeValue)
	r.pushStochStates(openValue, highValue, lowValue, closeValue, volumeValue)
	r.pushBollingerStates(closeValue)
	r.pushCCIStates(highValue, lowValue, closeValue)
	r.pushWilliamsRStates(highValue, lowValue, closeValue)
	r.pushOBVStates(openValue, highValue, lowValue, closeValue, volumeValue, oldFirst, hasPreviousClose)
}

func (r *indicatorRuntime) snapshot() map[string]any {
	if r == nil {
		return nil
	}
	cache := r.snapshotCache
	if cache == nil {
		cache = newSnapshotSeriesCache()
		r.snapshotCache = cache
	}
	cache.reset()
	result := r.snapshotResult
	if result == nil {
		result = make(map[string]any, r.snapshotKeys.resultCapacity)
		r.snapshotResult = result
	} else {
		clearMap(result)
	}
	for _, config := range r.requirements.ma {
		snapshot := r.movingAverageSnapshot(config, cache)
		if snapshot == nil {
			continue
		}
		result[r.snapshotKeys.ma[config]] = snapshot
		if legacyKey, ok := r.snapshotKeys.maLegacy[config]; ok {
			result[legacyKey] = snapshot
		}
	}
	for _, config := range r.requirements.securitySource {
		key := r.snapshotKeys.securitySource[config]
		current, previous, currentOK, previousOK := r.securitySourceSnapshotValues(config, cache)
		snapshot := cache.getSeriesSnapshot(key, current, previous, currentOK, previousOK)
		if snapshot != nil {
			result[key] = snapshot
		}
	}
	for _, period := range r.requirements.rsi {
		key := r.snapshotKeys.rsi[period]
		current, currentOK := r.rsiSnapshotValue(period, cache)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.rsiSource {
		key := r.snapshotKeys.rsiSource[config]
		current, currentOK := calculateRSIValueFromSeries(calculateRSISeries(r.seriesForSource(config.source), config.period))
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.macd {
		if state, ok := r.macdStates[config]; ok {
			currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK := state.snapshotValues()
			result[r.snapshotKeys.macd[config]] = cache.getMACDSnapshotValues(config, currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK)
			continue
		}
		series := cache.getMACDSeries(r.closes, config)
		result[r.snapshotKeys.macd[config]] = cache.getMACDSnapshot(config, series)
	}
	for _, config := range r.requirements.bollinger {
		result[r.snapshotKeys.bollinger[config]] = r.bollingerSnapshot(config)
	}
	for _, config := range r.requirements.kdj {
		if state, ok := r.kdjStates[config]; ok {
			currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK := state.snapshotValues()
			result[r.snapshotKeys.kdj[config]] = cache.getKDJSnapshotValues(config, currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK)
			continue
		}
		result[r.snapshotKeys.kdj[config]] = cache.getKDJSnapshot(config, cache.getKDJSeries(r.highs, r.lows, r.closes, config))
	}
	for _, period := range r.requirements.atr {
		key := r.snapshotKeys.atr[period]
		current, currentOK := r.atrSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, period := range r.requirements.stdev {
		key := r.snapshotKeys.stdev[period]
		current, currentOK := r.stdDevSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.stdevSource {
		key := r.snapshotKeys.stdevSource[config]
		current, currentOK := calculateStdDevValue(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.variance {
		key := r.snapshotKeys.variance[config]
		current, currentOK := calculateVarianceValue(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.windows {
		key := r.snapshotKeys.windows[config]
		state := r.windowStates[config]
		if config.function == "rising" || config.function == "falling" {
			if state != nil && state.hasCurrent {
				result[key] = state.boolCurrent
			}
			continue
		}
		if state != nil {
			result[key] = cache.getWindowSnapshot(config, state.current, state.previous, state.hasCurrent, state.hasPrevious)
		}
	}
	for _, config := range r.requirements.cum {
		key := r.snapshotKeys.cum[config]
		if state := r.cumStates[config]; state != nil {
			result[key] = cache.getSeriesSnapshot(key, state.current, state.previous, state.hasCurrent, state.hasPrevious)
		}
	}
	for _, config := range r.requirements.stoch {
		key := r.snapshotKeys.stoch[config]
		if state := r.stochStates[config]; state != nil {
			result[key] = cache.getSeriesSnapshot(key, state.current, state.previous, state.hasCurrent, state.hasPrevious)
		}
	}
	for _, period := range r.requirements.cci {
		key := r.snapshotKeys.cci[period]
		current, currentOK := r.cciSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.cciSource {
		key := r.snapshotKeys.cciSource[config]
		current, currentOK := calculateCCIFromValues(r.seriesForSource(config.source), config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, period := range r.requirements.williamsR {
		key := r.snapshotKeys.williamsR[period]
		current, currentOK := r.williamsRSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.vwap {
		key := r.snapshotKeys.vwap[config]
		current, currentOK := calculateSessionVWAP(r.seriesForSource(config.source), r.volumes, r.endTimes)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.mfi {
		key := r.snapshotKeys.mfi[config]
		current, currentOK := calculateMFIValue(r.seriesForSource(config.source), r.volumes, config.period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, config := range r.requirements.dmi {
		if snapshot := calculateDMISnapshot(r.highs, r.lows, r.closes, config); snapshot != nil {
			result[r.snapshotKeys.dmi[config]] = snapshot
		}
	}
	for _, config := range r.requirements.supertrend {
		if snapshot := calculateSupertrendSnapshot(r.highs, r.lows, r.closes, config); snapshot != nil {
			result[r.snapshotKeys.supertrend[config]] = snapshot
		}
	}
	for _, config := range r.requirements.sar {
		key := r.snapshotKeys.sar[config]
		current, previous, currentOK, previousOK := calculateSARSnapshotValues(r.highs, r.lows, r.closes, config)
		result[key] = cache.getSeriesSnapshot(key, current, previous, currentOK, previousOK)
	}
	for _, config := range r.requirements.advanced {
		key := r.snapshotKeys.advanced[config]
		if config.kind == "obv" && config.timeUnit == "" {
			if state := r.obvStates[config]; state != nil {
				result[key] = cache.getSeriesSnapshot(key, state.current, state.previous, state.hasCurrent, state.hasPrevious)
			}
			continue
		}
		if snapshot := r.advancedIndicatorSnapshot(config, cache); snapshot != nil {
			result[key] = snapshot
		}
	}
	for _, config := range r.requirements.stopLoss {
		snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache(r.closes, r.endTimes, r.sessions, config, r.intervalMinutes, r.symbol, r.includeExtendedHours, cache)
		if snapshot == nil {
			continue
		}
		result[r.snapshotKeys.stopLoss[config]] = snapshot
	}
	for _, config := range r.requirements.rsiDivergence {
		if state, ok := r.rsiStates[config.period]; ok {
			result[r.snapshotKeys.rsiDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		result[r.snapshotKeys.rsiDivergence[config]] = detectRSIDivergence(r.closes, r.rsiSeries(config.period), config.direction, config.lookback)
	}
	for _, config := range r.requirements.macdDivergence {
		baseConfig := macdConfig{fastPeriod: config.fastPeriod, slowPeriod: config.slowPeriod, signalPeriod: config.signalPeriod}
		if state, ok := r.macdStates[baseConfig]; ok {
			result[r.snapshotKeys.macdDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		result[r.snapshotKeys.macdDivergence[config]] = detectMACDDivergence(r.closes, cache.getMACDSeries(r.closes, baseConfig).diff, config.direction, config.lookback)
	}
	for _, config := range r.requirements.kdjDivergence {
		baseConfig := kdjConfig{period: config.period, m1: config.m1, m2: config.m2}
		if state, ok := r.kdjStates[baseConfig]; ok {
			result[r.snapshotKeys.kdjDivergence[config]] = state.detectDivergence(r.closes, config.direction, config.lookback)
			continue
		}
		result[r.snapshotKeys.kdjDivergence[config]] = detectKDJDivergence(r.closes, cache.getKDJSeries(r.highs, r.lows, r.closes, kdjConfig{period: config.period, m1: config.m1, m2: config.m2}).j, config.direction, config.lookback)
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
