package indicatorruntime

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

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
	vwapStates           map[sourceConfig]*rollingVWAPState
	anchoredVWAPStates   map[advancedIndicatorConfig]*rollingVWAPState
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
	if err := validateFixedTimeframeRequirements(requirements, resolveIntervalMinutes(interval)); err != nil {
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
	vwapStates, anchoredVWAPStates := newRollingVWAPStates(requirements)
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
		vwapStates:           vwapStates,
		anchoredVWAPStates:   anchoredVWAPStates,
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
	r.pushVWAPStates(openValue, highValue, lowValue, closeValue, volumeValue, kline.EndTime.Time())
}

func (r *indicatorRuntime) snapshot() map[string]any {
	if r == nil {
		return nil
	}
	cache, result := r.prepareSnapshotState()
	r.appendMovingAverageSnapshots(result, cache)
	r.appendSecuritySourceSnapshots(result, cache)
	r.appendRSISnapshots(result, cache)
	r.appendMACDSnapshots(result, cache)
	r.appendBollingerSnapshots(result)
	r.appendKDJSnapshots(result, cache)
	r.appendATRSnapshots(result, cache)
	r.appendStdDevSnapshots(result, cache)
	r.appendVarianceSnapshots(result, cache)
	r.appendWindowSnapshots(result, cache)
	r.appendCumSnapshots(result, cache)
	r.appendStochSnapshots(result, cache)
	r.appendCCISnapshots(result, cache)
	r.appendWilliamsRSnapshots(result, cache)
	r.appendVWAPSnapshots(result, cache)
	r.appendMFISnapshots(result, cache)
	r.appendDMISnapshots(result)
	r.appendSupertrendSnapshots(result)
	r.appendSARSnapshots(result, cache)
	r.appendAdvancedSnapshots(result, cache)
	r.appendStopLossSnapshots(result, cache)
	r.appendDivergenceSnapshots(result, cache)
	if len(result) == 0 {
		return nil
	}
	return result
}
