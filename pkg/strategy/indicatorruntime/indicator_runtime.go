package indicatorruntime

import (
	"math"
	"sort"
	"strconv"
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
	bollingerStates      map[bollingerConfig]*rollingBollingerState
	cciStates            map[int]*rollingCCIState
	williamsRStates      map[int]*rollingWilliamsRState
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
	rsi            map[int]string
	macd           map[macdConfig]string
	bollinger      map[bollingerConfig]string
	kdj            map[kdjConfig]string
	atr            map[int]string
	cci            map[int]string
	williamsR      map[int]string
	stopLoss       map[stopLossConfig]string
	rsiDivergence  map[rsiDivergenceConfig]string
	macdDivergence map[macdDivergenceConfig]string
	kdjDivergence  map[kdjDivergenceConfig]string
	resultCapacity int
}

func newSnapshotSeriesCache() *snapshotSeriesCache {
	return &snapshotSeriesCache{
		ema:                  map[int][]float64{},
		sma:                  map[int][]float64{},
		smma:                 map[int][]float64{},
		wma:                  map[int][]float64{},
		tma:                  map[int][]float64{},
		hma:                  map[int][]float64{},
		rsi:                  map[int][]float64{},
		macd:                 map[macdConfig]macdSeries{},
		kdj:                  map[kdjConfig]kdjSeries{},
		emaBuffers:           map[int][]float64{},
		macdBuffers:          map[macdConfig]macdSeries{},
		kdjBuffers:           map[kdjConfig]*reusableKDJSeriesBuffer{},
		tradingPeriodLabels:  map[tradingPeriodLabelCacheKey][]int64{},
		tradingPeriodBuffers: map[tradingPeriodLabelCacheKey][]int64{},
		maSnapshots:          map[movingAverageConfig]*indicatorSeriesSnapshot{},
		macdSnapshots:        map[macdConfig]*indicatorMACDSnapshot{},
		kdjSnapshots:         map[kdjConfig]*indicatorKDJSnapshot{},
		scalarValues:         map[string]*indicatorScalarSnapshot{},
		stopLossSnapshots:    map[stopLossConfig]map[string]any{},
	}
}

func (c *snapshotSeriesCache) reset() {
	if c == nil {
		return
	}
	clearMap(c.ema)
	clearMap(c.sma)
	clearMap(c.smma)
	clearMap(c.wma)
	clearMap(c.tma)
	clearMap(c.hma)
	clearMap(c.rsi)
	clearMap(c.macd)
	clearMap(c.kdj)
	clearMap(c.tradingPeriodLabels)
	c.stopLossWindowStart.valid = false
	c.stopLossWindowSelect.valid = false
	c.stopLossWindowExtrema.valid = false
}

func clearMap[K comparable, V any](values map[K]V) {
	for key := range values {
		delete(values, key)
	}
}

func (c *snapshotSeriesCache) getEMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateEMASequence(values, period)
	}
	if sequence, ok := c.ema[period]; ok {
		return sequence
	}
	sequence := fillEMASequence(c.emaBuffers[period], values, period)
	c.emaBuffers[period] = sequence
	c.ema[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getSMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateSMASequence(values, period)
	}
	if sequence, ok := c.sma[period]; ok {
		return sequence
	}
	sequence := calculateSMASequence(values, period)
	c.sma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getSMMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateSMMASequence(values, period)
	}
	if sequence, ok := c.smma[period]; ok {
		return sequence
	}
	sequence := calculateSMMASequence(values, period)
	c.smma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getWMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateWMASequence(values, period)
	}
	if sequence, ok := c.wma[period]; ok {
		return sequence
	}
	sequence := calculateWMASequence(values, period)
	c.wma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getTradingPeriodLabels(endTimes []time.Time, symbol string, unit string, includeExtendedHours bool) []int64 {
	normalizedUnit := normalizeIndicatorTimeUnit(unit)
	if c == nil {
		return buildTradingPeriodLabels(nil, endTimes, symbol, normalizedUnit, includeExtendedHours)
	}
	cacheKey := tradingPeriodLabelCacheKey{
		symbol:               symbol,
		unit:                 normalizedUnit,
		includeExtendedHours: includeExtendedHours,
	}
	if labels, ok := c.tradingPeriodLabels[cacheKey]; ok {
		return labels
	}
	labels := buildTradingPeriodLabels(c.tradingPeriodBuffers[cacheKey], endTimes, symbol, normalizedUnit, includeExtendedHours)
	c.tradingPeriodBuffers[cacheKey] = labels
	c.tradingPeriodLabels[cacheKey] = labels
	return labels
}

func (c *snapshotSeriesCache) getTMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateTMASequence(values, period)
	}
	if sequence, ok := c.tma[period]; ok {
		return sequence
	}
	sequence := calculateTMASequenceWithCache(values, period, c)
	c.tma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getHMASequence(values []float64, period int) []float64 {
	if c == nil {
		return calculateHMASequence(values, period)
	}
	if sequence, ok := c.hma[period]; ok {
		return sequence
	}
	sequence := calculateHMASequenceWithCache(values, period, c)
	c.hma[period] = sequence
	return sequence
}

func (c *snapshotSeriesCache) getRSISeries(closes []float64, period int) []float64 {
	if c == nil {
		return calculateRSISeries(closes, period)
	}
	if series, ok := c.rsi[period]; ok {
		return series
	}
	series := calculateRSISeries(closes, period)
	c.rsi[period] = series
	return series
}

func (c *snapshotSeriesCache) getMACDSeries(closes []float64, config macdConfig) macdSeries {
	if c == nil {
		return calculateMACDSeries(closes, config)
	}
	if series, ok := c.macd[config]; ok {
		return series
	}
	series := calculateMACDSeriesWithCache(closes, config, c)
	c.macdBuffers[config] = series
	c.macd[config] = series
	return series
}

func (c *snapshotSeriesCache) getKDJSeries(highs, lows, closes []float64, config kdjConfig) kdjSeries {
	if c == nil {
		kValues, dValues, jValues := calculateKDJSeries(highs, lows, closes, config)
		return kdjSeries{k: kValues, d: dValues, j: jValues}
	}
	if series, ok := c.kdj[config]; ok {
		return series
	}
	buffer, ok := c.kdjBuffers[config]
	if !ok {
		buffer = &reusableKDJSeriesBuffer{}
		c.kdjBuffers[config] = buffer
	}
	series := calculateKDJSeriesWithBuffer(buffer, highs, lows, closes, config)
	c.kdj[config] = series
	return series
}

func (c *snapshotSeriesCache) getScalarSnapshot(key string, current float64, currentOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		return &indicatorScalarSnapshot{current: current, hasCurrent: true}
	}
	snapshot, ok := c.scalarValues[key]
	if !ok {
		snapshot = &indicatorScalarSnapshot{}
		c.scalarValues[key] = snapshot
	}
	snapshot.current = current
	snapshot.hasCurrent = true
	return snapshot
}

func (c *snapshotSeriesCache) getStopLossSnapshot(config stopLossConfig) map[string]any {
	if c == nil {
		return make(map[string]any, 18)
	}
	snapshot, ok := c.stopLossSnapshots[config]
	if !ok {
		snapshot = make(map[string]any, 18)
		c.stopLossSnapshots[config] = snapshot
	}
	return snapshot
}

func (c *snapshotSeriesCache) getMovingAverageSnapshot(config movingAverageConfig, current, previous float64, currentOK, previousOK bool) any {
	if !currentOK && !previousOK {
		return nil
	}
	if c == nil {
		return &indicatorSeriesSnapshot{current: current, previous: previous, hasCurrent: currentOK, hasPrevious: previousOK}
	}
	snapshot, ok := c.maSnapshots[config]
	if !ok {
		snapshot = &indicatorSeriesSnapshot{}
		c.maSnapshots[config] = snapshot
	}
	snapshot.current = current
	snapshot.previous = previous
	snapshot.hasCurrent = currentOK
	snapshot.hasPrevious = previousOK
	return snapshot
}

func (c *snapshotSeriesCache) getMACDSnapshot(config macdConfig, series macdSeries) any {
	if len(series.diff) == 0 || len(series.signal) == 0 {
		return nil
	}
	currentIndex := len(series.diff) - 1
	currentDiff := series.diff[currentIndex]
	currentSignal := series.signal[currentIndex]
	previousDiff := 0.0
	previousSignal := 0.0
	previousOK := currentIndex > 0
	if previousOK {
		previousIndex := currentIndex - 1
		previousDiff = series.diff[previousIndex]
		previousSignal = series.signal[previousIndex]
	}
	return c.getMACDSnapshotValues(config, currentDiff, currentSignal, previousDiff, previousSignal, true, previousOK)
}

func (c *snapshotSeriesCache) getMACDSnapshotValues(config macdConfig, currentDiff, currentSignal, previousDiff, previousSignal float64, currentOK, previousOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		snapshot := &indicatorMACDSnapshot{
			diff:      currentDiff,
			signal:    currentSignal,
			histogram: (currentDiff - currentSignal) * 2,
		}
		if previousOK {
			snapshot.previousDiff = previousDiff
			snapshot.previousSignal = previousSignal
			snapshot.previousHistogram = (previousDiff - previousSignal) * 2
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.macdSnapshots[config]
	if !ok {
		snapshot = &indicatorMACDSnapshot{}
		c.macdSnapshots[config] = snapshot
	}
	snapshot.diff = currentDiff
	snapshot.signal = currentSignal
	snapshot.histogram = (currentDiff - currentSignal) * 2
	if previousOK {
		snapshot.previousDiff = previousDiff
		snapshot.previousSignal = previousSignal
		snapshot.previousHistogram = (previousDiff - previousSignal) * 2
		snapshot.hasPrevious = true
	} else {
		snapshot.previousDiff = 0
		snapshot.previousSignal = 0
		snapshot.previousHistogram = 0
		snapshot.hasPrevious = false
	}
	return snapshot
}

func (c *snapshotSeriesCache) getKDJSnapshot(config kdjConfig, series kdjSeries) any {
	if len(series.k) == 0 || len(series.d) == 0 || len(series.j) == 0 {
		return nil
	}
	last := len(series.k) - 1
	if c == nil {
		snapshot := &indicatorKDJSnapshot{k: series.k[last], d: series.d[last], j: series.j[last]}
		if last > 0 {
			snapshot.previousK = series.k[last-1]
			snapshot.previousD = series.d[last-1]
			snapshot.previousJ = series.j[last-1]
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.kdjSnapshots[config]
	if !ok {
		snapshot = &indicatorKDJSnapshot{}
		c.kdjSnapshots[config] = snapshot
	}
	snapshot.k = series.k[last]
	snapshot.d = series.d[last]
	snapshot.j = series.j[last]
	if last > 0 {
		snapshot.previousK = series.k[last-1]
		snapshot.previousD = series.d[last-1]
		snapshot.previousJ = series.j[last-1]
		snapshot.hasPrevious = true
	} else {
		snapshot.previousK = 0
		snapshot.previousD = 0
		snapshot.previousJ = 0
		snapshot.hasPrevious = false
	}
	return snapshot
}

func (c *snapshotSeriesCache) getKDJSnapshotValues(config kdjConfig, currentK, currentD, currentJ, previousK, previousD, previousJ float64, currentOK, previousOK bool) any {
	if !currentOK {
		return nil
	}
	if c == nil {
		snapshot := &indicatorKDJSnapshot{k: currentK, d: currentD, j: currentJ}
		if previousOK {
			snapshot.previousK = previousK
			snapshot.previousD = previousD
			snapshot.previousJ = previousJ
			snapshot.hasPrevious = true
		}
		return snapshot
	}
	snapshot, ok := c.kdjSnapshots[config]
	if !ok {
		snapshot = &indicatorKDJSnapshot{}
		c.kdjSnapshots[config] = snapshot
	}
	snapshot.k = currentK
	snapshot.d = currentD
	snapshot.j = currentJ
	if previousOK {
		snapshot.previousK = previousK
		snapshot.previousD = previousD
		snapshot.previousJ = previousJ
		snapshot.hasPrevious = true
	} else {
		snapshot.previousK = 0
		snapshot.previousD = 0
		snapshot.previousJ = 0
		snapshot.hasPrevious = false
	}
	return snapshot
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
		bollingerStates:      newRollingBollingerStates(requirements),
		cciStates:            newRollingCCIStates(requirements),
		williamsRStates:      newRollingWilliamsRStates(requirements),
	}
	if seriesLimit > 0 {
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
	oldFirstClose := 0.0
	oldSecondClose := 0.0
	hasOldFirstClose := len(r.closes) > 0
	hasOldSecondClose := len(r.closes) > 1
	if hasOldFirstClose {
		oldFirstClose = r.closes[0]
	}
	if hasOldSecondClose {
		oldSecondClose = r.closes[1]
	}
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
	r.pushKDJStates(r.highs, r.lows, r.closes, kline.High.Float64(), kline.Low.Float64(), closeValue, trimmed)
	r.highs = append(r.highs, kline.High.Float64())
	r.lows = append(r.lows, kline.Low.Float64())
	r.closes = append(r.closes, closeValue)
	r.volumes = append(r.volumes, kline.Volume.Float64())
	r.endTimes = append(r.endTimes, kline.EndTime.Time())
	r.sessions = append(r.sessions, resolvedSession)
	r.appendTradingPeriodLabels(kline.EndTime.Time())
	if trimmed {
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
	r.pushMovingAverageStates(closeValue, kline.Volume.Float64())
	r.pushEMAStates(closeValue, trimmed, oldFirstClose, oldSecondClose, hasOldFirstClose, hasOldSecondClose)
	r.pushMACDStates(closeValue, trimmed, oldFirstClose, oldSecondClose, hasOldFirstClose, hasOldSecondClose)
	r.pushRSIStates(closeValue, previousClose, hasPreviousClose)
	r.pushATRStates(kline.High.Float64(), kline.Low.Float64(), closeValue, previousClose, hasPreviousClose)
	r.pushBollingerStates(closeValue)
	r.pushCCIStates(kline.High.Float64(), kline.Low.Float64(), closeValue)
	r.pushWilliamsRStates(kline.High.Float64(), kline.Low.Float64(), closeValue)
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
	for _, period := range r.requirements.rsi {
		key := r.snapshotKeys.rsi[period]
		current, currentOK := r.rsiSnapshotValue(period, cache)
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
	for _, period := range r.requirements.cci {
		key := r.snapshotKeys.cci[period]
		current, currentOK := r.cciSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
	}
	for _, period := range r.requirements.williamsR {
		key := r.snapshotKeys.williamsR[period]
		current, currentOK := r.williamsRSnapshotValue(period)
		result[key] = cache.getScalarSnapshot(key, current, currentOK)
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

func buildSnapshotKeyCache(requirements indicatorRequirements) snapshotKeyCache {
	cache := snapshotKeyCache{
		resultCapacity: len(requirements.ma) + len(requirements.rsi) + len(requirements.macd) + len(requirements.bollinger) + len(requirements.kdj) + len(requirements.atr) + len(requirements.cci) + len(requirements.williamsR) + len(requirements.stopLoss) + len(requirements.rsiDivergence) + len(requirements.macdDivergence) + len(requirements.kdjDivergence),
	}
	if len(requirements.ma) > 0 {
		cache.ma = make(map[movingAverageConfig]string, len(requirements.ma))
		for _, config := range requirements.ma {
			cache.ma[config] = maIndicatorKey(config)
			if config.averageType == "MA" && normalizeIndicatorTimeUnit(config.timeUnit) == "" {
				if cache.maLegacy == nil {
					cache.maLegacy = make(map[movingAverageConfig]string)
				}
				cache.maLegacy[config] = legacyMAIndicatorKey(config.period)
				cache.resultCapacity++
			}
		}
	}
	if len(requirements.rsi) > 0 {
		cache.rsi = make(map[int]string, len(requirements.rsi))
		for _, period := range requirements.rsi {
			cache.rsi[period] = rsiIndicatorKey(period)
		}
	}
	if len(requirements.macd) > 0 {
		cache.macd = make(map[macdConfig]string, len(requirements.macd))
		for _, config := range requirements.macd {
			cache.macd[config] = macdIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod)
		}
	}
	if len(requirements.bollinger) > 0 {
		cache.bollinger = make(map[bollingerConfig]string, len(requirements.bollinger))
		for _, config := range requirements.bollinger {
			cache.bollinger[config] = bollingerIndicatorKey(config.period, config.multiplier)
		}
	}
	if len(requirements.kdj) > 0 {
		cache.kdj = make(map[kdjConfig]string, len(requirements.kdj))
		for _, config := range requirements.kdj {
			cache.kdj[config] = kdjIndicatorKey(config.period, config.m1, config.m2)
		}
	}
	if len(requirements.atr) > 0 {
		cache.atr = make(map[int]string, len(requirements.atr))
		for _, period := range requirements.atr {
			cache.atr[period] = atrIndicatorKey(period)
		}
	}
	if len(requirements.cci) > 0 {
		cache.cci = make(map[int]string, len(requirements.cci))
		for _, period := range requirements.cci {
			cache.cci[period] = cciIndicatorKey(period)
		}
	}
	if len(requirements.williamsR) > 0 {
		cache.williamsR = make(map[int]string, len(requirements.williamsR))
		for _, period := range requirements.williamsR {
			cache.williamsR[period] = williamsRIndicatorKey(period)
		}
	}
	if len(requirements.stopLoss) > 0 {
		cache.stopLoss = make(map[stopLossConfig]string, len(requirements.stopLoss))
		for _, config := range requirements.stopLoss {
			cache.stopLoss[config] = stopLossIndicatorKey(config)
		}
	}
	if len(requirements.rsiDivergence) > 0 {
		cache.rsiDivergence = make(map[rsiDivergenceConfig]string, len(requirements.rsiDivergence))
		for _, config := range requirements.rsiDivergence {
			cache.rsiDivergence[config] = rsiDivergenceIndicatorKey(config.period, config.direction, config.lookback)
		}
	}
	if len(requirements.macdDivergence) > 0 {
		cache.macdDivergence = make(map[macdDivergenceConfig]string, len(requirements.macdDivergence))
		for _, config := range requirements.macdDivergence {
			cache.macdDivergence[config] = macdDivergenceIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod, config.direction, config.lookback)
		}
	}
	if len(requirements.kdjDivergence) > 0 {
		cache.kdjDivergence = make(map[kdjDivergenceConfig]string, len(requirements.kdjDivergence))
		for _, config := range requirements.kdjDivergence {
			cache.kdjDivergence[config] = kdjDivergenceIndicatorKey(config.period, config.m1, config.m2, config.direction, config.lookback)
		}
	}
	return cache
}

func collectTradingPeriodUnits(requirements indicatorRequirements, intervalMinutes int, symbol string, includeExtendedHours bool) []string {
	if len(requirements.ma) == 0 || strings.TrimSpace(symbol) == "" {
		return nil
	}
	dayMinutes, ok := market.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
	if !ok || intervalMinutes <= 0 || intervalMinutes >= dayMinutes {
		return nil
	}
	units := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, config := range requirements.ma {
		unit := normalizeIndicatorTimeUnit(config.timeUnit)
		switch unit {
		case "day", "week", "month":
		default:
			continue
		}
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		units = append(units, unit)
	}
	return units
}

func trimFloatSeriesInPlace(values []float64, limit int) []float64 {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimTimeSeriesInPlace(values []time.Time, limit int) []time.Time {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimSessionSeriesInPlace(values []market.Session, limit int) []market.Session {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimInt64SeriesInPlace(values []int64, limit int) []int64 {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	start := len(values) - limit
	copy(values, values[start:])
	return values[:limit]
}

func trimWindowValuesInPlace(values []windowValue, windowStart int) []windowValue {
	expired := 0
	for expired < len(values) && values[expired].index < windowStart {
		expired++
	}
	if expired == 0 {
		return values
	}
	copy(values, values[expired:])
	return values[:len(values)-expired]
}

func (d *monotonicWindowValueDeque) compact() {
	if d == nil || d.start == 0 {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
		return
	}
	copy(d.values, d.values[d.start:])
	d.values = d.values[:len(d.values)-d.start]
	d.start = 0
}

func (d *monotonicWindowValueDeque) popExpired(windowStart int) {
	if d == nil {
		return
	}
	for d.start < len(d.values) && d.values[d.start].index < windowStart {
		d.start++
	}
	if d.start > len(d.values)/2 {
		d.compact()
	}
}

func (d *monotonicWindowValueDeque) pushMax(index int, value float64) {
	if d == nil {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
	}
	for len(d.values) > d.start && d.values[len(d.values)-1].value <= value {
		d.values = d.values[:len(d.values)-1]
	}
	d.values = append(d.values, windowValue{index: index, value: value})
}

func (d *monotonicWindowValueDeque) pushMin(index int, value float64) {
	if d == nil {
		return
	}
	if d.start >= len(d.values) {
		d.values = d.values[:0]
		d.start = 0
	}
	for len(d.values) > d.start && d.values[len(d.values)-1].value >= value {
		d.values = d.values[:len(d.values)-1]
	}
	d.values = append(d.values, windowValue{index: index, value: value})
}

func (d *monotonicWindowValueDeque) frontValue() (float64, bool) {
	if d == nil || d.start >= len(d.values) {
		return 0, false
	}
	return d.values[d.start].value, true
}

func (w *rollingFloatWindow) ensureCapacity(capacity int) {
	if w == nil || capacity <= 0 {
		return
	}
	if len(w.values) == capacity {
		return
	}
	w.values = make([]float64, capacity)
	w.start = 0
	w.count = 0
}

func (w *rollingFloatWindow) push(value float64, capacity int) (float64, bool) {
	if w == nil || capacity <= 0 {
		return 0, false
	}
	w.ensureCapacity(capacity)
	if w.count < capacity {
		index := (w.start + w.count) % capacity
		w.values[index] = value
		w.count++
		return 0, false
	}
	evicted := w.values[w.start]
	w.values[w.start] = value
	w.start = (w.start + 1) % capacity
	return evicted, true
}

func (w *rollingFloatWindow) len() int {
	if w == nil {
		return 0
	}
	return w.count
}

func (w *rollingFloatWindow) last() (float64, bool) {
	if w == nil || w.count == 0 || len(w.values) == 0 {
		return 0, false
	}
	index := (w.start + w.count - 1) % len(w.values)
	return w.values[index], true
}

func (w *rollingFloatWindow) at(offset int) (float64, bool) {
	if w == nil || offset < 0 || offset >= w.count || len(w.values) == 0 {
		return 0, false
	}
	index := (w.start + offset) % len(w.values)
	return w.values[index], true
}

func appendTailValue(values []float64, value float64, limit int) []float64 {
	if limit <= 0 {
		return values[:0]
	}
	if len(values) < limit {
		return append(values, value)
	}
	copy(values, values[1:])
	values[len(values)-1] = value
	return values
}

func sortedLookbacks(values map[int]struct{}) []int {
	if len(values) == 0 {
		return nil
	}
	result := make([]int, 0, len(values))
	for lookback := range values {
		if lookback > 0 {
			result = append(result, lookback)
		}
	}
	sort.Ints(result)
	return result
}

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

func newRollingMovingAverageStates(requirements indicatorRequirements, intervalMinutes int) map[movingAverageConfig]*rollingMovingAverageSnapshotState {
	if len(requirements.ma) == 0 {
		return nil
	}
	states := map[movingAverageConfig]*rollingMovingAverageSnapshotState{}
	for _, config := range requirements.ma {
		resolved := config
		resolved.period = resolveBarCount(config.period, config.timeUnit, intervalMinutes)
		resolved.timeUnit = ""
		kind := normalizeMovingAverageType(resolved.averageType)
		switch kind {
		case "MA", "SMA", "BOLL", "VWMA":
			if resolved.period <= 0 {
				continue
			}
			states[config] = &rollingMovingAverageSnapshotState{kind: kind, period: resolved.period}
		}
	}
	if len(states) == 0 {
		return nil
	}
	return states
}

func newRollingEMAStates(requirements indicatorRequirements, intervalMinutes int, seriesLimit int) map[movingAverageConfig]*rollingEMATailState {
	if len(requirements.ma) == 0 {
		return nil
	}
	states := map[movingAverageConfig]*rollingEMATailState{}
	for _, config := range requirements.ma {
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

func newRollingATRStates(requirements indicatorRequirements) map[int]*rollingATRState {
	if len(requirements.atr) == 0 {
		return nil
	}
	states := make(map[int]*rollingATRState, len(requirements.atr))
	for _, period := range requirements.atr {
		if period <= 0 {
			continue
		}
		states[period] = &rollingATRState{period: period}
	}
	return states
}

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

func newRollingCCIStates(requirements indicatorRequirements) map[int]*rollingCCIState {
	if len(requirements.cci) == 0 {
		return nil
	}
	states := make(map[int]*rollingCCIState, len(requirements.cci))
	for _, period := range requirements.cci {
		if period <= 0 {
			continue
		}
		states[period] = &rollingCCIState{period: period}
	}
	return states
}

func newRollingWilliamsRStates(requirements indicatorRequirements) map[int]*rollingWilliamsRState {
	if len(requirements.williamsR) == 0 {
		return nil
	}
	states := make(map[int]*rollingWilliamsRState, len(requirements.williamsR))
	for _, period := range requirements.williamsR {
		if period <= 0 {
			continue
		}
		states[period] = &rollingWilliamsRState{period: period}
	}
	return states
}

func (r *indicatorRuntime) pushRSIStates(closeValue, previousClose float64, hasPreviousClose bool) {
	if r == nil || len(r.rsiStates) == 0 {
		return
	}
	for _, state := range r.rsiStates {
		state.push(closeValue, previousClose, hasPreviousClose)
	}
}

func (r *indicatorRuntime) pushMovingAverageStates(closeValue, volume float64) {
	if r == nil || len(r.maStates) == 0 {
		return
	}
	for _, state := range r.maStates {
		state.push(closeValue, volume)
	}
}

func (r *indicatorRuntime) pushEMAStates(closeValue float64, trimmed bool, oldFirstClose, oldSecondClose float64, hasOldFirstClose, hasOldSecondClose bool) {
	if r == nil || len(r.emaStates) == 0 {
		return
	}
	for _, state := range r.emaStates {
		state.push(closeValue, trimmed, oldFirstClose, oldSecondClose, hasOldFirstClose, hasOldSecondClose)
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

func (r *indicatorRuntime) pushKDJStates(highs, lows, closes []float64, high, low, closeValue float64, trimmed bool) {
	if r == nil || len(r.kdjStates) == 0 {
		return
	}
	for _, state := range r.kdjStates {
		state.push(highs, lows, closes, high, low, closeValue, trimmed)
	}
}

func (r *indicatorRuntime) pushATRStates(high, low, closeValue, previousClose float64, hasPreviousClose bool) {
	if r == nil || len(r.atrStates) == 0 {
		return
	}
	for _, state := range r.atrStates {
		state.push(high, low, closeValue, previousClose, hasPreviousClose)
	}
}

func (r *indicatorRuntime) pushBollingerStates(closeValue float64) {
	if r == nil || len(r.bollingerStates) == 0 {
		return
	}
	for _, state := range r.bollingerStates {
		state.push(closeValue)
	}
}

func (r *indicatorRuntime) pushCCIStates(high, low, closeValue float64) {
	if r == nil || len(r.cciStates) == 0 {
		return
	}
	typicalPrice := (high + low + closeValue) / 3
	for _, state := range r.cciStates {
		state.push(typicalPrice)
	}
}

func (r *indicatorRuntime) pushWilliamsRStates(high, low, closeValue float64) {
	if r == nil || len(r.williamsRStates) == 0 {
		return
	}
	for _, state := range r.williamsRStates {
		state.push(high, low, closeValue)
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

func (r *indicatorRuntime) movingAverageSnapshot(config movingAverageConfig, cache *snapshotSeriesCache) any {
	if r == nil {
		return nil
	}
	if usesTradingPeriodWindow(config.timeUnit, r.intervalMinutes, r.symbol, r.endTimes, r.includeExtendedHours) {
		return r.movingAverageSnapshotForTradingWindow(config, cache)
	}
	if state, ok := r.emaStates[config]; ok {
		current, previous, currentOK, previousOK := state.snapshotValues()
		return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
	}
	if state, ok := r.maStates[config]; ok {
		return state.snapshotValue()
	}
	effectiveConfig := config
	effectiveConfig.period = resolveBarCount(config.period, config.timeUnit, r.intervalMinutes)
	effectiveConfig.timeUnit = ""
	current, previous, currentOK, previousOK := calculateMovingAverageSnapshotValuesWithCache(r.closes, r.volumes, effectiveConfig, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func (r *indicatorRuntime) movingAverageSnapshotForTradingWindow(config movingAverageConfig, cache *snapshotSeriesCache) any {
	if r == nil {
		return nil
	}
	unit := normalizeIndicatorTimeUnit(config.timeUnit)
	labelKeys := r.tradingPeriodLabels[unit]
	if len(labelKeys) == len(r.endTimes) && len(labelKeys) > 0 {
		if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(r.closes, r.volumes, labelKeys, config); handled {
			if !currentOK {
				return nil
			}
			return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
		}
	}
	return buildMovingAverageSnapshotForTradingWindow(r.closes, r.volumes, r.endTimes, config, r.symbol, r.includeExtendedHours, cache)
}

func (r *indicatorRuntime) appendTradingPeriodLabels(endTime time.Time) {
	if r == nil || len(r.tradingPeriodUnits) == 0 {
		return
	}
	for _, unit := range r.tradingPeriodUnits {
		labelStart, ok := market.TradingPeriodLabelStart(r.symbol, endTime, unit, r.includeExtendedHours)
		key := invalidTradingPeriodLabelKey
		if ok {
			key = labelStart.Unix()
		}
		r.tradingPeriodLabels[unit] = append(r.tradingPeriodLabels[unit], key)
	}
}

func (r *indicatorRuntime) atrValue(period int) any {
	current, ok := r.atrSnapshotValue(period)
	if !ok {
		return nil
	}
	return current
}

func (r *indicatorRuntime) atrSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.atrStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateATR(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
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

func (r *indicatorRuntime) cciValue(period int) any {
	current, ok := r.cciSnapshotValue(period)
	if !ok {
		return nil
	}
	return current
}

func (r *indicatorRuntime) cciSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.cciStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateCCI(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
}

func (r *indicatorRuntime) williamsRValue(period int) any {
	current, ok := r.williamsRSnapshotValue(period)
	if !ok {
		return nil
	}
	return current
}

func (r *indicatorRuntime) williamsRSnapshotValue(period int) (float64, bool) {
	if r == nil {
		return 0, false
	}
	if state, ok := r.williamsRStates[period]; ok {
		return state.currentValue()
	}
	value, ok := calculateWilliamsR(r.highs, r.lows, r.closes, period).(float64)
	return value, ok
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
	evictedGain, evictedGainOK := s.gains.push(gain, s.period)
	evictedLoss, evictedLossOK := s.losses.push(loss, s.period)
	s.gainSum += gain
	s.lossSum += loss
	if evictedGainOK {
		s.gainSum -= evictedGain
	}
	if evictedLossOK {
		s.lossSum -= evictedLoss
	}
	if s.gains.len() < s.period {
		return
	}
	value := 100.0
	if s.lossSum != 0 {
		relativeStrength := s.gainSum / s.lossSum
		value = 100 - 100/(1+relativeStrength)
	}
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

func (s *rollingMovingAverageSnapshotState) push(value, volume float64) {
	if s == nil || s.period <= 0 {
		return
	}
	previousCurrent := s.current
	previousHasCurrent := s.hasCurrent
	evictedValue, evictedValueOK := s.values.push(value, s.period)
	s.sum += value
	if s.kind == "VWMA" {
		evictedVolume, evictedVolumeOK := s.volumes.push(volume, s.period)
		s.weightedSum += value * volume
		s.volumeSum += volume
		if evictedValueOK && evictedVolumeOK {
			s.weightedSum -= evictedValue * evictedVolume
			s.volumeSum -= evictedVolume
		}
	} else if evictedValueOK {
		s.sum -= evictedValue
	}
	if s.kind == "VWMA" && evictedValueOK {
		s.sum -= evictedValue
	}
	s.previous = previousCurrent
	s.hasPrevious = previousHasCurrent
	if s.values.len() < s.period {
		s.hasCurrent = false
		return
	}
	if s.kind == "VWMA" {
		if s.volumeSum == 0 {
			s.hasCurrent = false
			return
		}
		s.current = s.weightedSum / s.volumeSum
		s.hasCurrent = true
		return
	}
	s.current = s.sum / float64(s.period)
	s.hasCurrent = true
}

func (s *rollingMovingAverageSnapshotState) snapshot() map[string]any {
	if s == nil || (!s.hasCurrent && !s.hasPrevious) {
		return nil
	}
	result := map[string]any{"value": nil, "previous": nil}
	if s.hasCurrent {
		result["value"] = s.current
	}
	if s.hasPrevious {
		result["previous"] = s.previous
	}
	return result
}

func (s *rollingMovingAverageSnapshotState) snapshotValue() any {
	if s == nil || (!s.hasCurrent && !s.hasPrevious) {
		return nil
	}
	return s
}

func (s *rollingMovingAverageSnapshotState) PreferredScalarValue() (float64, bool) {
	if s == nil || !s.hasCurrent {
		return 0, false
	}
	return s.current, true
}

func (s *rollingMovingAverageSnapshotState) SeriesField(name string) (float64, float64, bool, bool, bool) {
	if s == nil || name != "value" {
		return 0, 0, false, false, false
	}
	return s.current, s.previous, s.hasCurrent, s.hasPrevious, true
}

func (s *rollingMovingAverageSnapshotState) FieldValue(name string) (any, bool) {
	if s == nil {
		return nil, false
	}
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

func (s *rollingATRState) push(high, low, _ float64, previousClose float64, hasPreviousClose bool) {
	if s == nil || s.period <= 0 {
		return
	}
	trueRange := high - low
	if hasPreviousClose {
		trueRange = maxFloat(trueRange, maxFloat(math.Abs(high-previousClose), math.Abs(low-previousClose)))
	}
	evicted, evictedOK := s.window.push(trueRange, s.period)
	s.windowSum += trueRange
	if evictedOK {
		s.windowSum -= evicted
	}
	if s.window.len() < s.period {
		s.hasValue = false
		return
	}
	s.current = s.windowSum / float64(s.period)
	s.hasValue = true
}

func (s *rollingATRState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingATRState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
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

func (s *rollingCCIState) push(typicalPrice float64) {
	if s == nil || s.period <= 0 {
		return
	}
	evicted, evictedOK := s.window.push(typicalPrice, s.period)
	s.sum += typicalPrice
	if evictedOK {
		s.sum -= evicted
	}
	if s.window.len() < s.period {
		s.hasValue = false
		return
	}
	average := s.sum / float64(s.period)
	meanDeviation := 0.0
	for index := 0; index < s.window.len(); index++ {
		value, _ := s.window.at(index)
		meanDeviation += math.Abs(value - average)
	}
	meanDeviation /= float64(s.period)
	if meanDeviation == 0 {
		s.current = 0
		s.hasValue = true
		return
	}
	s.current = (typicalPrice - average) / (0.015 * meanDeviation)
	s.hasValue = true
}

func (s *rollingCCIState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingCCIState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
}

func (s *rollingWilliamsRState) push(high, low, closeValue float64) {
	if s == nil || s.period <= 0 {
		return
	}
	windowStart := s.index - s.period + 1
	s.highDeque.popExpired(windowStart)
	s.lowDeque.popExpired(windowStart)
	s.highDeque.pushMax(s.index, high)
	s.lowDeque.pushMin(s.index, low)
	s.index++
	if s.index < s.period {
		s.hasValue = false
		return
	}
	highestHigh, _ := s.highDeque.frontValue()
	lowestLow, _ := s.lowDeque.frontValue()
	if highestHigh == lowestLow {
		s.current = -50
		s.hasValue = true
		return
	}
	s.current = -100 * (highestHigh - closeValue) / (highestHigh - lowestLow)
	s.hasValue = true
}

func (s *rollingWilliamsRState) value() any {
	current, ok := s.currentValue()
	if !ok {
		return nil
	}
	return current
}

func (s *rollingWilliamsRState) currentValue() (float64, bool) {
	if s == nil || !s.hasValue {
		return 0, false
	}
	return s.current, true
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

func classifyKLineSession(symbol string, kline types.KLine) market.Session {
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return market.SessionUnknown
	}
	return market.ClassifySession(resolvedSymbol, observedAt)
}

func buildMovingAverageSnapshot(values, volumes []float64, config movingAverageConfig, intervalMinutes int) map[string]any {
	return buildMovingAverageSnapshotForSymbol(values, volumes, nil, config, intervalMinutes, "", nil)
}

func buildMovingAverageSnapshotWithCache(values, volumes []float64, config movingAverageConfig, intervalMinutes int, cache *snapshotSeriesCache) map[string]any {
	return buildMovingAverageSnapshotForSymbol(values, volumes, nil, config, intervalMinutes, "", cache)
}

func buildMovingAverageSnapshotForSymbol(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, intervalMinutes int, symbol string, cache *snapshotSeriesCache) map[string]any {
	return snapshotValueToMap(
		movingAverageSnapshotForSymbol(values, volumes, endTimes, config, intervalMinutes, symbol, false, cache),
		[...]string{"value", "previous"},
	)
}

func movingAverageSnapshotForSymbol(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, intervalMinutes int, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) any {
	if usesTradingPeriodWindow(config.timeUnit, intervalMinutes, symbol, endTimes, includeExtendedHours) {
		return buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes, config, symbol, includeExtendedHours, cache)
	}
	effectiveConfig := config
	effectiveConfig.period = resolveBarCount(config.period, config.timeUnit, intervalMinutes)
	effectiveConfig.timeUnit = ""
	current, previous, currentOK, previousOK := calculateMovingAverageSnapshotValuesWithCache(values, volumes, effectiveConfig, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func buildMovingAverageSnapshotForTradingWindow(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) any {
	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes, endTimes, config, symbol, includeExtendedHours, cache); handled {
		if !currentOK {
			return nil
		}
		return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
	}
	current, currentOK := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, config, symbol, len(values), includeExtendedHours, cache)
	if !currentOK {
		return nil
	}
	previous, previousOK := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, config, symbol, max(len(values)-1, 0), includeExtendedHours, cache)
	return cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK)
}

func calculateTradingWindowMovingAverageSnapshotOnline(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool) (float64, float64, bool, bool, bool) {
	return calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes, endTimes, config, symbol, includeExtendedHours, nil)
}

func calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, float64, bool, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
		current, currentOK, handled := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, len(values))
		if !handled {
			return 0, 0, false, false, false
		}
		previous, previousOK, _ := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, max(len(values)-1, 0))
		return current, previous, currentOK, previousOK, true
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, 0, false, false, false
	}
	if len(values) == 0 || len(values) != len(endTimes) {
		return 0, 0, false, false, true
	}
	currentLimit := len(values)
	previousLimit := max(len(values)-1, 0)
	labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
	currentState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousState := tradingWindowMovingAverageState{aggregator: aggregator}
	for index := currentLimit - 1; index >= 0; index-- {
		if index >= len(labelKeys) {
			break
		}
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !currentState.done {
			currentState.push(config.period, key, values[index], volumes, index)
		}
		if index < previousLimit && !previousState.done {
			previousState.push(config.period, key, values[index], volumes, index)
		}
		if currentState.done && (previousLimit == 0 || previousState.done) {
			break
		}
	}
	current, currentOK := currentState.value()
	previous, previousOK := previousState.value()
	return current, previous, currentOK, previousOK, true
}

func calculateMovingAverageCurrentValue(values, volumes []float64, config movingAverageConfig) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	effectiveConfig := config
	effectiveConfig.period = len(values)
	effectiveConfig.timeUnit = ""
	current, _, currentOK, _ := calculateMovingAverageSnapshotValuesWithCache(values, volumes, effectiveConfig, nil)
	return current, currentOK
}

func calculateTradingWindowMovingAverageCurrentValue(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, bool) {
	if current, currentOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, symbol, upperBound, includeExtendedHours, cache); handled {
		return current, currentOK
	}
	selected := selectTradingWindowIndicesWithCache(endTimes, config.period, config.timeUnit, symbol, upperBound, includeExtendedHours, cache)
	if len(selected) == 0 {
		return 0, false
	}
	return calculateMovingAverageCurrentValueFromSelected(values, volumes, selected, config, cache)
}

func calculateTradingWindowMovingAverageCurrentValueOnline(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool) (float64, bool, bool) {
	return calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, symbol, upperBound, includeExtendedHours, nil)
}

func calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes []float64, endTimes []time.Time, config movingAverageConfig, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) (float64, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
		return calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, upperBound)
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, false, false
	}
	if len(values) == 0 || len(values) != len(endTimes) || upperBound <= 0 {
		return 0, false, true
	}
	limit := min(upperBound, len(endTimes))
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	labelKeys := cache.getTradingPeriodLabels(endTimes, symbol, config.timeUnit, includeExtendedHours)
	for index := limit - 1; index >= 0; index-- {
		if index >= len(labelKeys) {
			break
		}
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !hasKey || key != lastKey {
			if orderedKeys == config.period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		if !aggregator.push(values[index], volumes, index) {
			return 0, false, true
		}
	}
	current, currentOK := aggregator.value()
	return current, currentOK, true
}

func calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes []float64, labelKeys []int64, config movingAverageConfig) (float64, float64, bool, bool, bool) {
	normalizedType := normalizeMovingAverageType(config.averageType)
	if normalizedType == "EMA" || normalizedType == "EXPMA" || normalizedType == "SMMA" || normalizedType == "TMA" || normalizedType == "HMA" {
		current, currentOK, handled := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, len(values))
		if !handled {
			return 0, 0, false, false, false
		}
		previous, previousOK, _ := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, normalizedType, config.period, max(len(values)-1, 0))
		return current, previous, currentOK, previousOK, true
	}
	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled {
		return 0, 0, false, false, false
	}
	if len(values) == 0 || len(values) != len(labelKeys) {
		return 0, 0, false, false, true
	}
	currentState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousState := tradingWindowMovingAverageState{aggregator: aggregator}
	previousLimit := max(len(values)-1, 0)
	for index := len(values) - 1; index >= 0; index-- {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if !currentState.done {
			currentState.push(config.period, key, values[index], volumes, index)
		}
		if index < previousLimit && !previousState.done {
			previousState.push(config.period, key, values[index], volumes, index)
		}
		if currentState.done && (previousLimit == 0 || previousState.done) {
			break
		}
	}
	current, currentOK := currentState.value()
	previous, previousOK := previousState.value()
	return current, previous, currentOK, previousOK, true
}

type tradingWindowSelectionSummary struct {
	startKey   int64
	startIndex int
	endIndex   int
	count      int
	valid      bool
}

func summarizeTradingWindowSelectionFromKeys(labelKeys []int64, period int, upperBound int) tradingWindowSelectionSummary {
	if period <= 0 || upperBound <= 0 || len(labelKeys) == 0 {
		return tradingWindowSelectionSummary{}
	}
	limit := min(upperBound, len(labelKeys))
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	summary := tradingWindowSelectionSummary{startIndex: limit, endIndex: -1}
	for index := limit - 1; index >= 0; index-- {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey {
			continue
		}
		if summary.endIndex < 0 {
			summary.endIndex = index
		}
		if !hasKey || key != lastKey {
			if orderedKeys == period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		summary.startKey = key
		summary.startIndex = index
		summary.count++
	}
	if summary.count == 0 || summary.endIndex < 0 || summary.startIndex >= limit {
		return tradingWindowSelectionSummary{}
	}
	summary.valid = true
	return summary
}

func calculateEMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	alpha := 2 / float64(summary.count+1)
	current := 0.0
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		if count == 0 {
			current = values[index]
			count = 1
			continue
		}
		current = current + (values[index]-current)*alpha
		count++
	}
	if count != summary.count {
		return 0, false
	}
	return current, true
}

func calculateTradingWindowEMAValueFromKeys(values []float64, labelKeys []int64, period int, upperBound int) (float64, bool, bool) {
	if len(values) == 0 || len(values) != len(labelKeys) || upperBound <= 0 {
		return 0, false, true
	}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, period, upperBound)
	if !summary.valid {
		return 0, false, true
	}
	current, ok := calculateEMAFromTradingWindowSelection(values, labelKeys, summary)
	return current, ok, true
}

func calculateSMMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	sum := 0.0
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		sum += values[index]
		count++
	}
	if count != summary.count || count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

func calculateSingleValueFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count != 1 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		return values[index], true
	}
	return 0, false
}

func calculateHMAFromTradingWindowSelection(values []float64, labelKeys []int64, summary tradingWindowSelectionSummary) (float64, bool) {
	if !summary.valid || summary.count <= 0 || len(values) == 0 || len(values) != len(labelKeys) {
		return 0, false
	}
	if summary.count == 1 {
		return calculateSingleValueFromTradingWindowSelection(values, labelKeys, summary)
	}
	if summary.count != 2 {
		return 0, false
	}
	collected := [2]float64{}
	count := 0
	for index := summary.startIndex; index <= summary.endIndex && index < len(values); index++ {
		key := labelKeys[index]
		if key == invalidTradingPeriodLabelKey || key < summary.startKey {
			continue
		}
		if count >= len(collected) {
			return 0, false
		}
		collected[count] = values[index]
		count++
	}
	if count != summary.count {
		return 0, false
	}
	slowWMA := (collected[0] + 2*collected[1]) / 3
	return 2*collected[1] - slowWMA, true
}

func calculateTradingWindowSequenceValueFromKeys(values []float64, labelKeys []int64, averageType string, period int, upperBound int) (float64, bool, bool) {
	if len(values) == 0 || len(values) != len(labelKeys) || upperBound <= 0 {
		return 0, false, true
	}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, period, upperBound)
	if !summary.valid {
		return 0, false, true
	}
	switch averageType {
	case "EMA", "EXPMA":
		current, ok := calculateEMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "SMMA":
		current, ok := calculateSMMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "TMA":
		current, ok := calculateSingleValueFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	case "HMA":
		current, ok := calculateHMAFromTradingWindowSelection(values, labelKeys, summary)
		return current, ok, true
	default:
		return 0, false, false
	}
}

func calculateMovingAverageCurrentValueFromSelected(values, volumes []float64, selected []int, config movingAverageConfig, cache *snapshotSeriesCache) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	period := len(selected)
	switch normalizeMovingAverageType(config.averageType) {
	case "LWMA":
		return linearWeightedMovingAverageFromSelected(values, selected, period)
	case "VWMA":
		return volumeWeightedMovingAverageFromSelected(values, volumes, selected)
	case "SMA", "BOLL", "MA":
		fallthrough
	default:
		if normalized := normalizeMovingAverageType(config.averageType); normalized != "EMA" && normalized != "EXPMA" && normalized != "SMMA" && normalized != "TMA" && normalized != "HMA" {
			return simpleMovingAverageFromSelected(values, selected)
		}
	}
	selectedValues, selectedVolumes := materializeTradingWindowSeriesFromSelected(values, volumes, selected, cache)
	return calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, config)
}

type tradingWindowMovingAverageAggregator struct {
	kind        string
	count       int
	sum         float64
	weightedSum float64
	volumeSum   float64
	missingData bool
}

type tradingWindowMovingAverageState struct {
	aggregator  tradingWindowMovingAverageAggregator
	orderedKeys int
	lastKey     int64
	hasKey      bool
	done        bool
}

func newTradingWindowMovingAverageAggregator(config movingAverageConfig) (tradingWindowMovingAverageAggregator, bool) {
	switch normalizeMovingAverageType(config.averageType) {
	case "SMA", "BOLL", "MA":
		return tradingWindowMovingAverageAggregator{kind: "sma"}, true
	case "LWMA":
		return tradingWindowMovingAverageAggregator{kind: "lwma"}, true
	case "VWMA":
		return tradingWindowMovingAverageAggregator{kind: "vwma"}, true
	default:
		return tradingWindowMovingAverageAggregator{}, false
	}
}

func (a *tradingWindowMovingAverageAggregator) push(value float64, volumes []float64, index int) bool {
	if a == nil {
		return false
	}
	switch a.kind {
	case "sma":
		a.sum += value
		a.count++
		return true
	case "lwma":
		a.weightedSum += a.sum + value
		a.sum += value
		a.count++
		return true
	case "vwma":
		if index >= len(volumes) {
			a.missingData = true
			return false
		}
		volume := volumes[index]
		a.weightedSum += value * volume
		a.volumeSum += volume
		a.count++
		return true
	default:
		return false
	}
}

func (a tradingWindowMovingAverageAggregator) value() (float64, bool) {
	if a.count == 0 || a.missingData {
		return 0, false
	}
	switch a.kind {
	case "sma":
		return a.sum / float64(a.count), true
	case "lwma":
		weightSum := float64(a.count * (a.count + 1) / 2)
		if weightSum == 0 {
			return 0, false
		}
		return a.weightedSum / weightSum, true
	case "vwma":
		if a.volumeSum == 0 {
			return 0, false
		}
		return a.weightedSum / a.volumeSum, true
	default:
		return 0, false
	}
}

func (s *tradingWindowMovingAverageState) push(period int, key int64, value float64, volumes []float64, index int) {
	if s == nil || s.done {
		return
	}
	if !s.hasKey || key != s.lastKey {
		if s.orderedKeys == period {
			s.done = true
			return
		}
		s.lastKey = key
		s.hasKey = true
		s.orderedKeys++
	}
	if !s.aggregator.push(value, volumes, index) {
		s.done = true
	}
}

func (s tradingWindowMovingAverageState) value() (float64, bool) {
	return s.aggregator.value()
}

func simpleMovingAverageFromSelected(values []float64, selected []int) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	sum := 0.0
	for index := len(selected) - 1; index >= 0; index-- {
		sum += values[selected[index]]
	}
	return sum / float64(len(selected)), true
}

func exponentialMovingAverageFromSelected(values []float64, selected []int, period int) (float64, bool) {
	if period <= 0 || len(selected) < period {
		return 0, false
	}
	multiplier := 2 / float64(period+1)
	current := values[selected[len(selected)-1]]
	for index := len(selected) - 2; index >= 0; index-- {
		current = current + (values[selected[index]]-current)*multiplier
	}
	return current, true
}

func linearWeightedMovingAverageFromSelected(values []float64, selected []int, period int) (float64, bool) {
	if period <= 0 || len(selected) < period {
		return 0, false
	}
	weightSum := float64(period * (period + 1) / 2)
	weightedSum := 0.0
	weight := 1.0
	for index := len(selected) - 1; index >= 0; index-- {
		weightedSum += values[selected[index]] * weight
		weight++
	}
	return weightedSum / weightSum, true
}

func volumeWeightedMovingAverageFromSelected(values, volumes []float64, selected []int) (float64, bool) {
	if len(selected) == 0 {
		return 0, false
	}
	volumeSum := 0.0
	weightedSum := 0.0
	for index := len(selected) - 1; index >= 0; index-- {
		position := selected[index]
		if position >= len(volumes) {
			return 0, false
		}
		volume := volumes[position]
		volumeSum += volume
		weightedSum += values[position] * volume
	}
	if volumeSum == 0 {
		return 0, false
	}
	return weightedSum / volumeSum, true
}

func selectTradingWindowSeries(values, volumes []float64, endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool) ([]float64, []float64) {
	if len(values) == 0 || len(values) != len(endTimes) {
		return nil, nil
	}
	selected := selectTradingWindowIndices(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	return materializeTradingWindowSeriesFromSelected(values, volumes, selected, nil)
}

func selectTradingWindowSeriesWithCache(values, volumes []float64, endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) ([]float64, []float64) {
	if len(values) == 0 || len(values) != len(endTimes) {
		return nil, nil
	}
	selected := selectTradingWindowIndicesWithCache(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours, cache)
	return materializeTradingWindowSeriesFromSelected(values, volumes, selected, cache)
}

func selectTradingWindowIndicesWithCache(endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) []int {
	if cache == nil {
		return selectTradingWindowIndices(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	}
	selected := selectTradingWindowIndicesInto(cache.tradingWindowIndices, endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	cache.tradingWindowIndices = selected
	return selected
}

func materializeTradingWindowSeriesFromSelected(values, volumes []float64, selected []int, cache *snapshotSeriesCache) ([]float64, []float64) {
	if len(selected) == 0 {
		if cache != nil {
			cache.tradingWindowValues = cache.tradingWindowValues[:0]
			cache.tradingWindowVolumes = cache.tradingWindowVolumes[:0]
		}
		return nil, nil
	}
	windowValues := []float64(nil)
	windowVolumes := []float64(nil)
	if cache != nil {
		windowValues = cache.tradingWindowValues[:0]
		windowVolumes = cache.tradingWindowVolumes[:0]
	}
	if cap(windowValues) < len(selected) {
		windowValues = make([]float64, 0, len(selected))
	}
	if cap(windowVolumes) < len(selected) {
		windowVolumes = make([]float64, 0, len(selected))
	}
	for index := len(selected) - 1; index >= 0; index-- {
		position := selected[index]
		windowValues = append(windowValues, values[position])
		if position < len(volumes) {
			windowVolumes = append(windowVolumes, volumes[position])
		}
	}
	if cache != nil {
		cache.tradingWindowValues = windowValues
		cache.tradingWindowVolumes = windowVolumes
	}
	return windowValues, windowVolumes
}

func selectTradingWindowIndicesInto(destination []int, endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool) []int {
	if period <= 0 || upperBound <= 0 || len(endTimes) == 0 {
		return nil
	}
	limit := min(upperBound, len(endTimes))
	selected := destination[:0]
	if cap(selected) < limit {
		selected = make([]int, 0, limit)
	}
	orderedKeys := 0
	lastKey := int64(0)
	hasKey := false
	normalizedUnit := normalizeIndicatorTimeUnit(timeUnit)
	for index := limit - 1; index >= 0; index-- {
		labelStart, ok := market.TradingPeriodLabelStart(symbol, endTimes[index], normalizedUnit, includeExtendedHours)
		if !ok {
			continue
		}
		key := labelStart.Unix()
		if !hasKey || key != lastKey {
			if orderedKeys == period {
				break
			}
			lastKey = key
			hasKey = true
			orderedKeys++
		}
		selected = append(selected, index)
	}
	return selected
}

func selectTradingWindowIndices(endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool) []int {
	if period <= 0 || upperBound <= 0 || len(endTimes) == 0 {
		return nil
	}
	limit := min(upperBound, len(endTimes))
	selected := make([]int, 0, limit)
	keys := make(map[string]struct{}, period)
	orderedKeys := 0
	normalizedUnit := normalizeIndicatorTimeUnit(timeUnit)
	for index := limit - 1; index >= 0; index-- {
		key, ok := market.TradingPeriodKey(symbol, endTimes[index], normalizedUnit, includeExtendedHours)
		if !ok {
			continue
		}
		if _, exists := keys[key]; !exists {
			if orderedKeys == period {
				break
			}
			keys[key] = struct{}{}
			orderedKeys++
		}
		selected = append(selected, index)
	}
	return selected
}

func usesTradingPeriodWindow(timeUnit string, intervalMinutes int, symbol string, endTimes []time.Time, includeExtendedHours bool) bool {
	switch normalizeIndicatorTimeUnit(timeUnit) {
	case "day", "week", "month":
	default:
		return false
	}
	if len(endTimes) == 0 || strings.TrimSpace(symbol) == "" {
		return false
	}
	dayMinutes, ok := market.TradingMinutesPerTradingDay(symbol, includeExtendedHours)
	if !ok {
		return false
	}
	return intervalMinutes > 0 && intervalMinutes < dayMinutes
}

func snapshotValueToMap(snapshot any, keys [2]string) map[string]any {
	if snapshot == nil {
		return nil
	}
	if values, ok := snapshot.(map[string]any); ok {
		return values
	}
	reader, ok := snapshot.(interface{ FieldValue(string) (any, bool) })
	if !ok {
		return nil
	}
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := reader.FieldValue(key)
		if ok {
			result[key] = value
		}
	}
	return result
}

func calculateMovingAverageSnapshotValues(values, volumes []float64, config movingAverageConfig) (float64, float64, bool, bool) {
	return calculateMovingAverageSnapshotValuesWithCache(values, volumes, config, nil)
}

func calculateMovingAverageSnapshotValuesWithCache(values, volumes []float64, config movingAverageConfig, cache *snapshotSeriesCache) (float64, float64, bool, bool) {
	switch normalizeMovingAverageType(config.averageType) {
	case "EMA", "EXPMA":
		return emaSnapshotValues(cache.getEMASequence(values, config.period), len(values), config.period)
	case "SMMA":
		return lastTwoSequenceValues(cache.getSMMASequence(values, config.period))
	case "LWMA":
		return lastTwoSequenceValues(cache.getWMASequence(values, config.period))
	case "TMA":
		return lastTwoSequenceValues(cache.getTMASequence(values, config.period))
	case "HMA":
		return lastTwoSequenceValues(cache.getHMASequence(values, config.period))
	case "VWMA":
		current, currentOK := volumeWeightedMovingAverage(values, volumes, config.period)
		previous, previousOK := volumeWeightedMovingAverage(
			values[:max(len(values)-1, 0)],
			volumes[:max(len(volumes)-1, 0)],
			config.period,
		)
		return current, previous, currentOK, previousOK
	case "SMA", "BOLL", "MA":
		fallthrough
	default:
		return lastTwoSequenceValues(cache.getSMASequence(values, config.period))
	}
}

func emaSnapshotValues(sequence []float64, sourceLen int, period int) (float64, float64, bool, bool) {
	if period <= 0 || sourceLen < period || len(sequence) == 0 {
		return 0, 0, false, false
	}
	current := sequence[len(sequence)-1]
	previous := 0.0
	previousOK := sourceLen-1 >= period && len(sequence) > 1
	if previousOK {
		previous = sequence[len(sequence)-2]
	}
	return current, previous, true, previousOK
}

func lastTwoSequenceValues(sequence []float64) (float64, float64, bool, bool) {
	if len(sequence) == 0 {
		return 0, 0, false, false
	}
	current := sequence[len(sequence)-1]
	previous := 0.0
	previousOK := len(sequence) > 1
	if previousOK {
		previous = sequence[len(sequence)-2]
	}
	return current, previous, true, previousOK
}

func buildStopLossSnapshot(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int) map[string]any {
	return buildStopLossSnapshotForSymbol(closes, endTimes, sessions, config, intervalMinutes, "")
}

func buildStopLossSnapshotForSymbol(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, symbol string) map[string]any {
	return buildStopLossSnapshotForSymbolWithOptions(closes, endTimes, sessions, config, intervalMinutes, symbol, false)
}

func buildStopLossSnapshotForSymbolWithOptions(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, symbol string, includeExtendedHours bool) map[string]any {
	return buildStopLossSnapshotForSymbolWithOptionsAndCache(closes, endTimes, sessions, config, intervalMinutes, symbol, includeExtendedHours, nil)
}

func buildStopLossSnapshotForSymbolWithOptionsAndCache(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) map[string]any {
	if usesTradingPeriodWindow(config.timeUnit, intervalMinutes, symbol, endTimes, includeExtendedHours) {
		return buildStopLossSnapshotForTradingWindowWithCache(closes, endTimes, sessions, config, intervalMinutes, symbol, includeExtendedHours, cache)
	}
	lookback := resolveBarCount(config.timeValue, config.timeUnit, intervalMinutes)
	if lookback <= 0 || len(closes) <= lookback {
		return nil
	}
	windowStart := len(closes) - 1 - lookback
	if windowStart < 0 {
		return nil
	}
	windowPolicy := normalizeStopLossWindowPolicy(config.windowPolicy)
	if windowPolicy == "session" {
		windowStart = resolveSessionAwareWindowStartWithCache(endTimes, sessions, windowStart, intervalMinutes, cache)
		if windowStart < 0 {
			return nil
		}
	}
	reference := closes[windowStart]
	current := closes[len(closes)-1]
	if reference <= 0 || math.IsNaN(reference) || math.IsInf(reference, 0) || math.IsNaN(current) || math.IsInf(current, 0) {
		return nil
	}
	changePercent := ((current - reference) / reference) * 100
	mode := normalizeStopLossMode(config.mode)
	direction := normalizeStopLossDirection(config.direction)
	longTriggered := false
	shortTriggered := false
	longTriggerPercent := math.Abs(changePercent)
	shortTriggerPercent := math.Abs(changePercent)
	peakClose := current
	troughClose := current
	longDrawdownPercent := 0.0
	shortReboundPercent := 0.0
	switch mode {
	case "takeProfit":
		longTriggered = changePercent >= config.percentage
		shortTriggered = changePercent <= -config.percentage
	case "trailingStop":
		peakClose, troughClose = maxMinSliceFromWindowStartWithCache(closes, windowStart, cache)
		if peakClose <= 0 || troughClose <= 0 || math.IsNaN(peakClose) || math.IsNaN(troughClose) || math.IsInf(peakClose, 0) || math.IsInf(troughClose, 0) {
			return nil
		}
		longDrawdownPercent = ((peakClose - current) / peakClose) * 100
		shortReboundPercent = ((current - troughClose) / troughClose) * 100
		longTriggered = longDrawdownPercent >= config.percentage
		shortTriggered = shortReboundPercent >= config.percentage
		longTriggerPercent = longDrawdownPercent
		shortTriggerPercent = shortReboundPercent
	default:
		longTriggered = changePercent <= -config.percentage
		shortTriggered = changePercent >= config.percentage
	}
	triggerPercent := 0.0
	switch direction {
	case "long":
		triggerPercent = longTriggerPercent
	case "short":
		triggerPercent = shortTriggerPercent
	default:
		if longTriggered && !shortTriggered {
			triggerPercent = longTriggerPercent
		} else if shortTriggered && !longTriggered {
			triggerPercent = shortTriggerPercent
		} else {
			triggerPercent = max(longTriggerPercent, shortTriggerPercent)
		}
	}
	return fillStopLossSnapshot(
		cache,
		config,
		mode,
		direction,
		float64(len(closes)-1-windowStart),
		windowPolicy,
		reference,
		current,
		changePercent,
		triggerPercent,
		longTriggered,
		shortTriggered,
		longTriggerPercent,
		shortTriggerPercent,
		peakClose,
		troughClose,
		longDrawdownPercent,
		shortReboundPercent,
	)
}

func buildStopLossSnapshotForTradingWindow(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, symbol string, includeExtendedHours bool) map[string]any {
	return buildStopLossSnapshotForTradingWindowWithCache(closes, endTimes, sessions, config, intervalMinutes, symbol, includeExtendedHours, nil)
}

func buildStopLossSnapshotForTradingWindowWithCache(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, symbol string, includeExtendedHours bool, cache *snapshotSeriesCache) map[string]any {
	selectedIndices := selectStopLossTradingWindowIndicesWithCache(endTimes, config.timeValue, config.timeUnit, symbol, len(closes), includeExtendedHours, cache)
	if len(selectedIndices) < 2 {
		return nil
	}
	reference := closes[selectedIndices[len(selectedIndices)-1]]
	current := closes[selectedIndices[0]]
	if reference <= 0 || math.IsNaN(reference) || math.IsInf(reference, 0) || math.IsNaN(current) || math.IsInf(current, 0) {
		return nil
	}
	changePercent := ((current - reference) / reference) * 100
	mode := normalizeStopLossMode(config.mode)
	direction := normalizeStopLossDirection(config.direction)
	longTriggered := false
	shortTriggered := false
	longTriggerPercent := math.Abs(changePercent)
	shortTriggerPercent := math.Abs(changePercent)
	peakClose := current
	troughClose := current
	longDrawdownPercent := 0.0
	shortReboundPercent := 0.0
	switch mode {
	case "takeProfit":
		longTriggered = changePercent >= config.percentage
		shortTriggered = changePercent <= -config.percentage
	case "trailingStop":
		peakClose, troughClose = maxMinSelectedCloses(closes, selectedIndices)
		if peakClose <= 0 || troughClose <= 0 || math.IsNaN(peakClose) || math.IsNaN(troughClose) || math.IsInf(peakClose, 0) || math.IsInf(troughClose, 0) {
			return nil
		}
		longDrawdownPercent = ((peakClose - current) / peakClose) * 100
		shortReboundPercent = ((current - troughClose) / troughClose) * 100
		longTriggered = longDrawdownPercent >= config.percentage
		shortTriggered = shortReboundPercent >= config.percentage
		longTriggerPercent = longDrawdownPercent
		shortTriggerPercent = shortReboundPercent
	default:
		longTriggered = changePercent <= -config.percentage
		shortTriggered = changePercent >= config.percentage
	}
	triggerPercent := 0.0
	switch direction {
	case "long":
		triggerPercent = longTriggerPercent
	case "short":
		triggerPercent = shortTriggerPercent
	default:
		if longTriggered && !shortTriggered {
			triggerPercent = longTriggerPercent
		} else if shortTriggered && !longTriggered {
			triggerPercent = shortTriggerPercent
		} else {
			triggerPercent = max(longTriggerPercent, shortTriggerPercent)
		}
	}
	windowPolicy := normalizeStopLossWindowPolicy(config.windowPolicy)
	return fillStopLossSnapshot(
		cache,
		config,
		mode,
		direction,
		float64(len(selectedIndices)-1),
		windowPolicy,
		reference,
		current,
		changePercent,
		triggerPercent,
		longTriggered,
		shortTriggered,
		longTriggerPercent,
		shortTriggerPercent,
		peakClose,
		troughClose,
		longDrawdownPercent,
		shortReboundPercent,
	)
}

func fillStopLossSnapshot(cache *snapshotSeriesCache, config stopLossConfig, mode, direction string, windowBars float64, windowPolicy string, reference, current, changePercent, triggerPercent float64, longTriggered, shortTriggered bool, longTriggerPercent, shortTriggerPercent, peakClose, troughClose, longDrawdownPercent, shortReboundPercent float64) map[string]any {
	triggered := false
	if direction == "long" {
		triggered = longTriggered
	} else if direction == "short" {
		triggered = shortTriggered
	} else {
		triggered = longTriggered || shortTriggered
	}
	snapshot := cache.getStopLossSnapshot(config)
	snapshot["mode"] = mode
	snapshot["triggered"] = triggered
	snapshot["direction"] = direction
	snapshot["windowBars"] = windowBars
	snapshot["percentage"] = config.percentage
	snapshot["windowPolicy"] = windowPolicy
	snapshot["sessionAware"] = windowPolicy == "session"
	snapshot["referenceClose"] = reference
	snapshot["currentClose"] = current
	snapshot["changePercent"] = changePercent
	snapshot["triggerPercent"] = triggerPercent
	snapshot["longTriggered"] = longTriggered
	snapshot["shortTriggered"] = shortTriggered
	snapshot["longTriggerPercent"] = longTriggerPercent
	snapshot["shortTriggerPercent"] = shortTriggerPercent
	snapshot["peakClose"] = peakClose
	snapshot["troughClose"] = troughClose
	snapshot["longDrawdownPercent"] = longDrawdownPercent
	snapshot["shortReboundPercent"] = shortReboundPercent
	return snapshot
}

func selectStopLossTradingWindowIndicesWithCache(endTimes []time.Time, period int, timeUnit string, symbol string, upperBound int, includeExtendedHours bool, cache *snapshotSeriesCache) []int {
	if cache == nil {
		return selectTradingWindowIndices(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours)
	}
	selection := &cache.stopLossWindowSelect
	if selection.valid && selection.period == period && selection.timeUnit == timeUnit && selection.symbol == symbol && selection.upperBound == upperBound && selection.includeExtendedHours == includeExtendedHours {
		return selection.indices
	}
	selected := selectTradingWindowIndicesWithCache(endTimes, period, timeUnit, symbol, upperBound, includeExtendedHours, cache)
	selection.indices = append(selection.indices[:0], selected...)
	selection.valid = true
	selection.period = period
	selection.timeUnit = timeUnit
	selection.symbol = symbol
	selection.upperBound = upperBound
	selection.includeExtendedHours = includeExtendedHours
	return selection.indices
}

func resolveSessionAwareWindowStartWithCache(endTimes []time.Time, sessions []market.Session, windowStart int, intervalMinutes int, cache *snapshotSeriesCache) int {
	if cache == nil {
		return resolveSessionAwareWindowStart(endTimes, sessions, windowStart, intervalMinutes)
	}
	seriesLength := sessionAwareSeriesLength(endTimes, sessions)
	entry := &cache.stopLossWindowStart
	if entry.valid && entry.requestedStart == windowStart && entry.intervalMinutes == intervalMinutes && entry.seriesLength == seriesLength {
		return entry.resolvedStart
	}
	resolved := resolveSessionAwareWindowStart(endTimes, sessions, windowStart, intervalMinutes)
	entry.valid = true
	entry.requestedStart = windowStart
	entry.intervalMinutes = intervalMinutes
	entry.seriesLength = seriesLength
	entry.resolvedStart = resolved
	return resolved
}

func maxMinSliceFromWindowStartWithCache(closes []float64, windowStart int, cache *snapshotSeriesCache) (float64, float64) {
	if cache == nil {
		return maxMinSlice(closes[windowStart:])
	}
	entry := &cache.stopLossWindowExtrema
	if entry.valid && entry.windowStart == windowStart && entry.seriesLength == len(closes) {
		return entry.peakClose, entry.troughClose
	}
	peakClose, troughClose := maxMinSlice(closes[windowStart:])
	entry.valid = true
	entry.windowStart = windowStart
	entry.seriesLength = len(closes)
	entry.peakClose = peakClose
	entry.troughClose = troughClose
	return peakClose, troughClose
}

func maxMinSelectedCloses(closes []float64, selectedIndices []int) (float64, float64) {
	if len(selectedIndices) == 0 {
		return 0, 0
	}
	peakClose := closes[selectedIndices[0]]
	troughClose := peakClose
	for _, index := range selectedIndices[1:] {
		value := closes[index]
		peakClose = max(peakClose, value)
		troughClose = min(troughClose, value)
	}
	return peakClose, troughClose
}

func sessionAwareSeriesLength(endTimes []time.Time, sessions []market.Session) int {
	seriesLength := len(endTimes)
	if len(sessions) > seriesLength {
		seriesLength = len(sessions)
	}
	return seriesLength
}

func resolveSessionAwareWindowStart(endTimes []time.Time, sessions []market.Session, windowStart int, intervalMinutes int) int {
	if windowStart < 0 {
		return -1
	}
	if intervalMinutes <= 0 || intervalMinutes >= tradingSessionMinutesPerDay {
		return windowStart
	}
	seriesLength := sessionAwareSeriesLength(endTimes, sessions)
	if seriesLength == 0 {
		return windowStart
	}
	if seriesLength <= windowStart {
		return -1
	}
	for index := windowStart + 1; index < seriesLength; index++ {
		if isSessionBoundary(
			readMarketSessionAt(sessions, index-1),
			readMarketSessionAt(sessions, index),
			readTimeAt(endTimes, index-1),
			readTimeAt(endTimes, index),
			intervalMinutes,
		) {
			return -1
		}
	}
	return windowStart
}

func readMarketSessionAt(sessions []market.Session, index int) market.Session {
	if index < 0 || index >= len(sessions) {
		return market.SessionUnknown
	}
	return sessions[index]
}

func readTimeAt(values []time.Time, index int) time.Time {
	if index < 0 || index >= len(values) {
		return time.Time{}
	}
	return values[index]
}

func isSessionBoundary(previousSession, currentSession market.Session, previousTime, currentTime time.Time, intervalMinutes int) bool {
	if previousSession != market.SessionUnknown && currentSession != market.SessionUnknown && previousSession != currentSession {
		return true
	}
	return isSessionBreak(previousTime, currentTime, intervalMinutes)
}

func isSessionBreak(previous, current time.Time, intervalMinutes int) bool {
	if previous.IsZero() || current.IsZero() {
		return false
	}
	if !current.After(previous) {
		return true
	}
	expectedGap := time.Duration(max(intervalMinutes, 1)) * time.Minute
	return current.Sub(previous) > expectedGap*2
}

func maxMinSlice(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	maximum := values[0]
	minimum := values[0]
	for _, value := range values[1:] {
		maximum = max(maximum, value)
		minimum = min(minimum, value)
	}
	return maximum, minimum
}

func calculateIndicatorSeriesLimit(requirements indicatorRequirements, intervalMinutes int) int {
	limit := minimumIndicatorSeriesLimit
	for _, config := range requirements.ma {
		limit = max(limit, resolveBarCount(config.period, config.timeUnit, intervalMinutes)+1)
	}
	for _, period := range requirements.rsi {
		limit = max(limit, period+1)
	}
	for _, config := range requirements.macd {
		limit = max(limit, config.slowPeriod+config.signalPeriod+1)
	}
	for _, config := range requirements.bollinger {
		limit = max(limit, config.period+1)
	}
	for _, config := range requirements.kdj {
		limit = max(limit, config.period+config.m1+config.m2+1)
	}
	for _, period := range requirements.atr {
		limit = max(limit, period+2)
	}
	for _, period := range requirements.cci {
		limit = max(limit, period+1)
	}
	for _, period := range requirements.williamsR {
		limit = max(limit, period+1)
	}
	for _, config := range requirements.stopLoss {
		limit = max(limit, resolveBarCount(config.timeValue, config.timeUnit, intervalMinutes)+1)
	}
	for _, config := range requirements.rsiDivergence {
		limit = max(limit, config.period+config.lookback+1)
	}
	for _, config := range requirements.macdDivergence {
		limit = max(limit, config.slowPeriod+config.signalPeriod+config.lookback+1)
	}
	for _, config := range requirements.kdjDivergence {
		limit = max(limit, config.period+config.m1+config.m2+config.lookback+1)
	}
	return limit
}

func resolveIntervalMinutes(interval types.Interval) int {
	value := strings.ToLower(strings.TrimSpace(string(interval)))
	if value == "" {
		return 1
	}
	unit := ""
	switch {
	case strings.HasSuffix(value, "mo"):
		unit = "mo"
		value = strings.TrimSuffix(value, "mo")
	case strings.HasSuffix(value, "min"):
		unit = "min"
		value = strings.TrimSuffix(value, "min")
	case strings.HasSuffix(value, "m"):
		unit = "m"
		value = strings.TrimSuffix(value, "m")
	case strings.HasSuffix(value, "h"):
		unit = "h"
		value = strings.TrimSuffix(value, "h")
	case strings.HasSuffix(value, "d"):
		unit = "d"
		value = strings.TrimSuffix(value, "d")
	case strings.HasSuffix(value, "w"):
		unit = "w"
		value = strings.TrimSuffix(value, "w")
	default:
		return 1
	}
	amount, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || amount <= 0 {
		return 1
	}
	switch unit {
	case "min", "m":
		return amount
	case "h":
		return amount * 60
	case "d":
		return amount * tradingSessionMinutesPerDay
	case "w":
		return amount * tradingSessionMinutesPerWeek
	case "mo":
		return amount * tradingSessionMinutesPerMonth
	default:
		return 1
	}
}

func calculateMovingAverageValue(values, volumes []float64, config movingAverageConfig) (float64, bool) {
	switch normalizeMovingAverageType(config.averageType) {
	case "EMA", "EXPMA":
		return exponentialMovingAverage(values, config.period)
	case "SMMA":
		return smoothedMovingAverage(values, config.period)
	case "LWMA":
		return linearWeightedMovingAverage(values, config.period)
	case "TMA":
		return triangularMovingAverage(values, config.period)
	case "HMA":
		return hullMovingAverage(values, config.period)
	case "VWMA":
		return volumeWeightedMovingAverage(values, volumes, config.period)
	case "SMA", "BOLL", "MA":
		fallthrough
	default:
		return simpleMovingAverage(values, config.period)
	}
}

func simpleMovingAverage(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	sum := 0.0
	for _, value := range values[len(values)-period:] {
		sum += value
	}
	return sum / float64(period), true
}

func exponentialMovingAverage(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	sequence := calculateEMASequence(values, period)
	if len(sequence) == 0 {
		return 0, false
	}
	return sequence[len(sequence)-1], true
}

func smoothedMovingAverage(values []float64, period int) (float64, bool) {
	sequence := calculateSMMASequence(values, period)
	if len(sequence) == 0 {
		return 0, false
	}
	return sequence[len(sequence)-1], true
}

func linearWeightedMovingAverage(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	window := values[len(values)-period:]
	weightSum := 0.0
	weightedSum := 0.0
	for index, value := range window {
		weight := float64(index + 1)
		weightSum += weight
		weightedSum += value * weight
	}
	if weightSum == 0 {
		return 0, false
	}
	return weightedSum / weightSum, true
}

func triangularMovingAverage(values []float64, period int) (float64, bool) {
	sequence := calculateTMASequence(values, period)
	if len(sequence) == 0 {
		return 0, false
	}
	return sequence[len(sequence)-1], true
}

func calculateTMASequence(values []float64, period int) []float64 {
	return calculateTMASequenceWithCache(values, period, nil)
}

func calculateTMASequenceWithCache(values []float64, period int, cache *snapshotSeriesCache) []float64 {
	sequence := cache.getSMASequence(values, period)
	if len(sequence) < period {
		return nil
	}
	return calculateSMASequence(sequence, period)
}

func hullMovingAverage(values []float64, period int) (float64, bool) {
	sequence := calculateHMASequence(values, period)
	if len(sequence) == 0 {
		return 0, false
	}
	return sequence[len(sequence)-1], true
}

func calculateHMASequence(values []float64, period int) []float64 {
	return calculateHMASequenceWithCache(values, period, nil)
}

func calculateHMASequenceWithCache(values []float64, period int, cache *snapshotSeriesCache) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	halfPeriod := max(1, period/2)
	sqrtPeriod := max(1, int(math.Round(math.Sqrt(float64(period)))))
	fastSequence := cache.getWMASequence(values, halfPeriod)
	slowSequence := cache.getWMASequence(values, period)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return nil
	}
	combined := make([]float64, 0, len(values)-period+1)
	for end := period; end <= len(values); end++ {
		fastIndex := end - halfPeriod
		slowIndex := end - period
		if fastIndex < 0 || fastIndex >= len(fastSequence) || slowIndex < 0 || slowIndex >= len(slowSequence) {
			continue
		}
		combined = append(combined, 2*fastSequence[fastIndex]-slowSequence[slowIndex])
	}
	return calculateWMASequence(combined, sqrtPeriod)
}

func volumeWeightedMovingAverage(values, volumes []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period || len(volumes) < period {
		return 0, false
	}
	windowValues := values[len(values)-period:]
	windowVolumes := volumes[len(volumes)-period:]
	volumeSum := 0.0
	weightedSum := 0.0
	for index, value := range windowValues {
		volume := windowVolumes[index]
		volumeSum += volume
		weightedSum += value * volume
	}
	if volumeSum == 0 {
		return 0, false
	}
	return weightedSum / volumeSum, true
}

func calculateSMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	windowSum := 0.0
	for index, value := range values {
		windowSum += value
		if index >= period {
			windowSum -= values[index-period]
		}
		if index >= period-1 {
			result = append(result, windowSum/float64(period))
		}
	}
	return result
}

func calculateSMMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	first, ok := simpleMovingAverage(values[:period], period)
	if !ok {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	result = append(result, first)
	previous := first
	for index := period; index < len(values); index++ {
		previous = (previous*float64(period-1) + values[index]) / float64(period)
		result = append(result, previous)
	}
	return result
}

func calculateWMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	weightSum := float64(period * (period + 1) / 2)
	windowSum := 0.0
	weightedSum := 0.0
	for index := 0; index < period; index++ {
		weight := float64(index + 1)
		windowSum += values[index]
		weightedSum += values[index] * weight
	}
	result = append(result, weightedSum/weightSum)
	for index := period; index < len(values); index++ {
		outgoing := values[index-period]
		previousWindowSum := windowSum
		windowSum += values[index] - outgoing
		weightedSum = weightedSum - previousWindowSum + values[index]*float64(period)
		result = append(result, weightedSum/weightSum)
	}
	return result
}

func calculateRSI(values []float64, period int) any {
	series := calculateRSISeries(values, period)
	return calculateRSIFromSeries(series)
}

func calculateRSIFromSeries(series []float64) any {
	value, ok := calculateRSIValueFromSeries(series)
	if !ok {
		return nil
	}
	return value
}

func calculateRSIValueFromSeries(series []float64) (float64, bool) {
	if len(series) == 0 {
		return 0, false
	}
	return series[len(series)-1], true
}

func calculateRSISeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) <= period {
		return nil
	}
	gains := make([]float64, len(values)-1)
	losses := make([]float64, len(values)-1)
	for index := 1; index < len(values); index++ {
		delta := values[index] - values[index-1]
		if delta >= 0 {
			gains[index-1] = delta
			continue
		}
		losses[index-1] = math.Abs(delta)
	}
	result := make([]float64, 0, len(values)-period)
	rollingGains := 0.0
	rollingLosses := 0.0
	for index := 0; index < period; index++ {
		rollingGains += gains[index]
		rollingLosses += losses[index]
	}
	appendRSIValue := func(totalGains, totalLosses float64) {
		if totalLosses == 0 {
			result = append(result, 100.0)
			return
		}
		relativeStrength := totalGains / totalLosses
		result = append(result, 100-100/(1+relativeStrength))
	}
	appendRSIValue(rollingGains, rollingLosses)
	for index := period; index < len(gains); index++ {
		rollingGains += gains[index] - gains[index-period]
		rollingLosses += losses[index] - losses[index-period]
		appendRSIValue(rollingGains, rollingLosses)
	}
	return result
}

func calculateMACDSnapshot(values []float64, config macdConfig) map[string]any {
	return calculateMACDSnapshotFromSeries(calculateMACDSeries(values, config))
}

func calculateMACDSeries(values []float64, config macdConfig) macdSeries {
	return calculateMACDSeriesWithCache(values, config, nil)
}

func calculateMACDSeriesWithCache(values []float64, config macdConfig, cache *snapshotSeriesCache) macdSeries {
	minimum := max(config.fastPeriod, config.slowPeriod) + config.signalPeriod
	if minimum <= 0 || len(values) < minimum {
		return macdSeries{}
	}
	fastSequence := cache.getEMASequence(values, config.fastPeriod)
	slowSequence := cache.getEMASequence(values, config.slowPeriod)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return macdSeries{}
	}
	buffer := macdSeries{}
	if cache != nil {
		buffer = cache.macdBuffers[config]
	}
	diffSequence := reuseFloat64Slice(buffer.diff, len(values))
	signalSequence := reuseFloat64Slice(buffer.signal, len(values))
	signalMultiplier := 2 / float64(config.signalPeriod+1)
	for index := range values {
		diff := fastSequence[index] - slowSequence[index]
		diffSequence[index] = diff
		if index == 0 {
			signalSequence[index] = diff
			continue
		}
		signalSequence[index] = signalSequence[index-1] + (diff-signalSequence[index-1])*signalMultiplier
	}
	return macdSeries{diff: diffSequence, signal: signalSequence}
}

func calculateMACDSnapshotFromSeries(series macdSeries) map[string]any {
	if len(series.diff) == 0 || len(series.signal) == 0 {
		return nil
	}
	currentIndex := len(series.diff) - 1
	result := map[string]any{
		"diff":      series.diff[currentIndex],
		"signal":    series.signal[currentIndex],
		"histogram": (series.diff[currentIndex] - series.signal[currentIndex]) * 2,
	}
	if currentIndex > 0 {
		previousIndex := currentIndex - 1
		result["previousDiff"] = series.diff[previousIndex]
		result["previousSignal"] = series.signal[previousIndex]
		result["previousHistogram"] = (series.diff[previousIndex] - series.signal[previousIndex]) * 2
	}
	return result
}

func calculateEMASequence(values []float64, period int) []float64 {
	return fillEMASequence(nil, values, period)
}

func calculateBollingerSnapshot(values []float64, config bollingerConfig) map[string]any {
	if config.period <= 0 || len(values) < config.period {
		return nil
	}
	windowValues := values[len(values)-config.period:]
	middle, ok := simpleMovingAverage(windowValues, config.period)
	if !ok {
		return nil
	}
	variance := 0.0
	for _, value := range windowValues {
		delta := value - middle
		variance += delta * delta
	}
	standardDeviation := math.Sqrt(variance / float64(len(windowValues)))
	return map[string]any{
		"middle": middle,
		"upper":  middle + standardDeviation*config.multiplier,
		"lower":  middle - standardDeviation*config.multiplier,
	}
}

func calculateKDJSnapshot(highs, lows, closes []float64, config kdjConfig) map[string]any {
	if config.period <= 0 || len(closes) == 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	return calculateKDJSnapshotFromSeries(kdjSeriesFromSlices(calculateKDJSeries(highs, lows, closes, config)))
}

func calculateKDJRSV(highestHigh, lowestLow, closeValue float64) float64 {
	if highestHigh == lowestLow {
		return 50
	}
	return ((closeValue - lowestLow) / (highestHigh - lowestLow)) * 100
}

func kdjSeriesFromSlices(kValues, dValues, jValues []float64) kdjSeries {
	return kdjSeries{k: kValues, d: dValues, j: jValues}
}

func calculateKDJSnapshotFromSeries(series kdjSeries) map[string]any {
	if len(series.k) == 0 || len(series.d) == 0 || len(series.j) == 0 {
		return nil
	}
	last := len(series.k) - 1
	result := map[string]any{
		"k": series.k[last],
		"d": series.d[last],
		"j": series.j[last],
	}
	if last > 0 {
		result["previousK"] = series.k[last-1]
		result["previousD"] = series.d[last-1]
		result["previousJ"] = series.j[last-1]
	}
	return result
}

func calculateKDJSeries(highs, lows, closes []float64, config kdjConfig) ([]float64, []float64, []float64) {
	series := calculateKDJSeriesWithBuffer(nil, highs, lows, closes, config)
	return series.k, series.d, series.j
}

func calculateKDJSeriesWithBuffer(buffer *reusableKDJSeriesBuffer, highs, lows, closes []float64, config kdjConfig) kdjSeries {
	if config.period <= 0 || len(closes) == 0 || len(highs) != len(closes) || len(lows) != len(closes) {
		return kdjSeries{}
	}
	if buffer == nil {
		buffer = &reusableKDJSeriesBuffer{}
	}
	buffer.series.k = reuseFloat64Slice(buffer.series.k, len(closes))
	buffer.series.d = reuseFloat64Slice(buffer.series.d, len(closes))
	buffer.series.j = reuseFloat64Slice(buffer.series.j, len(closes))
	dequeCapacity := min(len(closes), max(config.period, 1))
	buffer.highDeque.reset(dequeCapacity)
	buffer.lowDeque.reset(dequeCapacity)
	previousK := 50.0
	previousD := 50.0
	for index := range closes {
		windowStart := max(0, index-config.period+1)
		buffer.highDeque.popExpired(windowStart)
		buffer.lowDeque.popExpired(windowStart)
		buffer.highDeque.pushMax(highs, index)
		buffer.lowDeque.pushMin(lows, index)
		highestHigh := highs[buffer.highDeque.front()]
		lowestLow := lows[buffer.lowDeque.front()]
		rsv := calculateKDJRSV(highestHigh, lowestLow, closes[index])
		nextK := ((float64(config.m1)-1)*previousK + rsv) / float64(config.m1)
		nextD := ((float64(config.m2)-1)*previousD + nextK) / float64(config.m2)
		nextJ := 3*nextK - 2*nextD
		buffer.series.k[index] = nextK
		buffer.series.d[index] = nextD
		buffer.series.j[index] = nextJ
		previousK = nextK
		previousD = nextD
	}
	return buffer.series
}

func calculateATR(highs, lows, closes []float64, period int) any {
	values := calculateATRSeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateATRSeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	trueRanges := make([]float64, len(closes))
	for index := range closes {
		if index == 0 {
			trueRanges[index] = highs[index] - lows[index]
			continue
		}
		trueRanges[index] = maxFloat(
			highs[index]-lows[index],
			maxFloat(math.Abs(highs[index]-closes[index-1]), math.Abs(lows[index]-closes[index-1])),
		)
	}
	result := make([]float64, 0, len(closes)-period+1)
	windowSum := 0.0
	for index, trueRange := range trueRanges {
		windowSum += trueRange
		if index >= period {
			windowSum -= trueRanges[index-period]
		}
		if index >= period-1 {
			result = append(result, windowSum/float64(period))
		}
	}
	return result
}

func calculateCCI(highs, lows, closes []float64, period int) any {
	values := calculateCCISeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateCCISeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	typicalPrices := make([]float64, len(closes))
	for index := range closes {
		typicalPrices[index] = (highs[index] + lows[index] + closes[index]) / 3
	}
	result := make([]float64, 0, len(closes)-period+1)
	rollingSum := 0.0
	for index := period - 1; index < len(typicalPrices); index++ {
		if index == period-1 {
			for cursor := 0; cursor < period; cursor++ {
				rollingSum += typicalPrices[cursor]
			}
		} else {
			rollingSum += typicalPrices[index] - typicalPrices[index-period]
		}
		average := rollingSum / float64(period)
		meanDeviation := 0.0
		for cursor := index - period + 1; cursor <= index; cursor++ {
			meanDeviation += math.Abs(typicalPrices[cursor] - average)
		}
		meanDeviation /= float64(period)
		if meanDeviation == 0 {
			result = append(result, 0)
			continue
		}
		result = append(result, (typicalPrices[index]-average)/(0.015*meanDeviation))
	}
	return result
}

func calculateWilliamsR(highs, lows, closes []float64, period int) any {
	values := calculateWilliamsRSeries(highs, lows, closes, period)
	if len(values) == 0 {
		return nil
	}
	return values[len(values)-1]
}

func calculateWilliamsRSeries(highs, lows, closes []float64, period int) []float64 {
	if period <= 0 || len(closes) < period || len(highs) != len(closes) || len(lows) != len(closes) {
		return nil
	}
	highestHighs, lowestLows := calculateRollingHighLow(highs, lows, period)
	if len(highestHighs) == 0 || len(lowestLows) == 0 {
		return nil
	}
	result := make([]float64, 0, len(closes)-period+1)
	for index := period - 1; index < len(closes); index++ {
		highestHigh := highestHighs[index]
		lowestLow := lowestLows[index]
		if highestHigh == lowestLow {
			result = append(result, -50)
			continue
		}
		result = append(result, -100*(highestHigh-closes[index])/(highestHigh-lowestLow))
	}
	return result
}

func calculateRollingHighLow(highs, lows []float64, period int) ([]float64, []float64) {
	if period <= 0 || len(highs) == 0 || len(highs) != len(lows) {
		return nil, nil
	}
	highestHighs := make([]float64, len(highs))
	lowestLows := make([]float64, len(lows))
	var highDeque, lowDeque indexDeque
	for index := range highs {
		windowStart := max(0, index-period+1)
		highDeque.popExpired(windowStart)
		lowDeque.popExpired(windowStart)
		highDeque.pushMax(highs, index)
		lowDeque.pushMin(lows, index)
		highestHighs[index] = highs[highDeque.front()]
		lowestLows[index] = lows[lowDeque.front()]
	}
	return highestHighs, lowestLows
}

type indexDeque struct {
	indices []int
}

func (d *indexDeque) reset(capacity int) {
	if capacity <= 0 {
		d.indices = d.indices[:0]
		return
	}
	if cap(d.indices) < capacity {
		d.indices = make([]int, 0, capacity)
		return
	}
	d.indices = d.indices[:0]
}

func (d *indexDeque) popExpired(windowStart int) {
	expired := 0
	for expired < len(d.indices) && d.indices[expired] < windowStart {
		expired++
	}
	if expired == 0 {
		return
	}
	copy(d.indices, d.indices[expired:])
	d.indices = d.indices[:len(d.indices)-expired]
}

func (d *indexDeque) pushMax(values []float64, index int) {
	for len(d.indices) > 0 && values[d.indices[len(d.indices)-1]] <= values[index] {
		d.indices = d.indices[:len(d.indices)-1]
	}
	d.indices = append(d.indices, index)
}

func (d *indexDeque) pushMin(values []float64, index int) {
	for len(d.indices) > 0 && values[d.indices[len(d.indices)-1]] >= values[index] {
		d.indices = d.indices[:len(d.indices)-1]
	}
	d.indices = append(d.indices, index)
}

func (d *indexDeque) front() int {
	if len(d.indices) == 0 {
		return 0
	}
	return d.indices[0]
}

func reuseFloat64Slice(values []float64, length int) []float64 {
	if length <= 0 {
		return nil
	}
	if cap(values) < length {
		return make([]float64, length)
	}
	return values[:length]
}

func reuseInt64Slice(values []int64, length int) []int64 {
	if length <= 0 {
		return nil
	}
	if cap(values) < length {
		return make([]int64, length)
	}
	return values[:length]
}

func buildTradingPeriodLabels(destination []int64, endTimes []time.Time, symbol string, normalizedUnit string, includeExtendedHours bool) []int64 {
	if len(endTimes) == 0 {
		return nil
	}
	labels := reuseInt64Slice(destination, len(endTimes))
	for index, endTime := range endTimes {
		labelStart, ok := market.TradingPeriodLabelStart(symbol, endTime, normalizedUnit, includeExtendedHours)
		if !ok {
			labels[index] = invalidTradingPeriodLabelKey
			continue
		}
		labels[index] = labelStart.Unix()
	}
	return labels
}

func fillEMASequence(destination []float64, values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	sequence := reuseFloat64Slice(destination, len(values))
	multiplier := 2 / float64(period+1)
	previous := values[0]
	sequence[0] = previous
	for index := 1; index < len(values); index++ {
		previous = previous + (values[index]-previous)*multiplier
		sequence[index] = previous
	}
	return sequence
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
