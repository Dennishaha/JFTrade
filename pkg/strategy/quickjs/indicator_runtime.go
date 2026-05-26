package quickjs

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
)

const minimumIndicatorSeriesLimit = 256

type indicatorRuntime struct {
	requirements    indicatorRequirements
	symbol          string
	intervalMinutes int
	seriesLimit     int
	highs           []float64
	lows            []float64
	closes          []float64
	volumes         []float64
	endTimes        []time.Time
	sessions        []futu.MarketSession
}

func newIndicatorRuntime(script string, interval types.Interval, symbol string) *indicatorRuntime {
	requirements := parseIndicatorRequirements(script)
	if requirements.isEmpty() {
		return nil
	}
	intervalMinutes := resolveIntervalMinutes(interval)
	return &indicatorRuntime{
		requirements:    requirements,
		symbol:          strings.ToUpper(strings.TrimSpace(symbol)),
		intervalMinutes: intervalMinutes,
		seriesLimit:     calculateIndicatorSeriesLimit(requirements, intervalMinutes),
	}
}

func (r *indicatorRuntime) push(kline types.KLine, session futu.MarketSession) {
	if r == nil {
		return
	}
	r.highs = append(r.highs, kline.High.Float64())
	r.lows = append(r.lows, kline.Low.Float64())
	r.closes = append(r.closes, kline.Close.Float64())
	r.volumes = append(r.volumes, kline.Volume.Float64())
	r.endTimes = append(r.endTimes, kline.EndTime.Time())
	resolvedSession := session
	if resolvedSession == futu.MarketSessionUnknown {
		resolvedSession = classifyKLineSession(r.symbol, kline)
	}
	r.sessions = append(r.sessions, resolvedSession)
	seriesLimit := r.seriesLimit
	if seriesLimit <= 0 {
		seriesLimit = minimumIndicatorSeriesLimit
	}
	if len(r.closes) > seriesLimit {
		start := len(r.closes) - seriesLimit
		r.highs = append([]float64(nil), r.highs[start:]...)
		r.lows = append([]float64(nil), r.lows[start:]...)
		r.closes = append([]float64(nil), r.closes[start:]...)
		r.volumes = append([]float64(nil), r.volumes[start:]...)
		r.endTimes = append([]time.Time(nil), r.endTimes[start:]...)
		r.sessions = append([]futu.MarketSession(nil), r.sessions[start:]...)
	}
}

func (r *indicatorRuntime) snapshot() map[string]any {
	if r == nil {
		return nil
	}
	result := map[string]any{}
	for _, config := range r.requirements.ma {
		snapshot := buildMovingAverageSnapshot(r.closes, r.volumes, config, r.intervalMinutes)
		if snapshot == nil {
			continue
		}
		result[maIndicatorKey(config)] = snapshot
		if config.averageType == "MA" && normalizeIndicatorTimeUnit(config.timeUnit) == "" {
			result[legacyMAIndicatorKey(config.period)] = snapshot
		}
	}
	for _, period := range r.requirements.rsi {
		result[rsiIndicatorKey(period)] = calculateRSI(r.closes, period)
	}
	for _, config := range r.requirements.macd {
		result[macdIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod)] = calculateMACDSnapshot(r.closes, config)
	}
	for _, config := range r.requirements.bollinger {
		result[bollingerIndicatorKey(config.period, config.multiplier)] = calculateBollingerSnapshot(r.closes, config)
	}
	for _, config := range r.requirements.kdj {
		result[kdjIndicatorKey(config.period, config.m1, config.m2)] = calculateKDJSnapshot(r.highs, r.lows, r.closes, config)
	}
	for _, period := range r.requirements.atr {
		result[atrIndicatorKey(period)] = calculateATR(r.highs, r.lows, r.closes, period)
	}
	for _, period := range r.requirements.cci {
		result[cciIndicatorKey(period)] = calculateCCI(r.highs, r.lows, r.closes, period)
	}
	for _, period := range r.requirements.williamsR {
		result[williamsRIndicatorKey(period)] = calculateWilliamsR(r.highs, r.lows, r.closes, period)
	}
	for _, config := range r.requirements.stopLoss {
		snapshot := buildStopLossSnapshot(r.closes, r.endTimes, r.sessions, config, r.intervalMinutes)
		if snapshot == nil {
			continue
		}
		result[stopLossIndicatorKey(config)] = snapshot
	}
	for _, config := range r.requirements.rsiDivergence {
		result[rsiDivergenceIndicatorKey(config.period, config.direction, config.lookback)] = calculateRSIDivergence(r.closes, config)
	}
	for _, config := range r.requirements.macdDivergence {
		result[macdDivergenceIndicatorKey(config.fastPeriod, config.slowPeriod, config.signalPeriod, config.direction, config.lookback)] = calculateMACDDivergence(r.closes, config)
	}
	for _, config := range r.requirements.kdjDivergence {
		result[kdjDivergenceIndicatorKey(config.period, config.m1, config.m2, config.direction, config.lookback)] = calculateKDJDivergence(r.highs, r.lows, r.closes, config)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func classifyKLineSession(symbol string, kline types.KLine) futu.MarketSession {
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return futu.MarketSessionUnknown
	}
	return futu.ClassifyMarketSession(resolvedSymbol, observedAt)
}

func buildMovingAverageSnapshot(values, volumes []float64, config movingAverageConfig, intervalMinutes int) map[string]any {
	effectiveConfig := config
	effectiveConfig.period = resolveBarCount(config.period, config.timeUnit, intervalMinutes)
	effectiveConfig.timeUnit = ""
	current, currentOK := calculateMovingAverageValue(values, volumes, effectiveConfig)
	previous, previousOK := calculateMovingAverageValue(
		values[:max(len(values)-1, 0)],
		volumes[:max(len(volumes)-1, 0)],
		effectiveConfig,
	)
	if !currentOK && !previousOK {
		return nil
	}
	result := map[string]any{"value": nil, "previous": nil}
	if currentOK {
		result["value"] = current
	}
	if previousOK {
		result["previous"] = previous
	}
	return result
}

func buildStopLossSnapshot(closes []float64, endTimes []time.Time, sessions []futu.MarketSession, config stopLossConfig, intervalMinutes int) map[string]any {
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
		windowStart = resolveSessionAwareWindowStart(endTimes, sessions, windowStart, intervalMinutes)
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
		peakClose, troughClose = maxMinSlice(closes[windowStart:])
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
	triggered := false
	triggerPercent := 0.0
	switch direction {
	case "long":
		triggered = longTriggered
		triggerPercent = longTriggerPercent
	case "short":
		triggered = shortTriggered
		triggerPercent = shortTriggerPercent
	default:
		triggered = longTriggered || shortTriggered
		if longTriggered && !shortTriggered {
			triggerPercent = longTriggerPercent
		} else if shortTriggered && !longTriggered {
			triggerPercent = shortTriggerPercent
		} else {
			triggerPercent = max(longTriggerPercent, shortTriggerPercent)
		}
	}
	return map[string]any{
		"mode":                mode,
		"triggered":           triggered,
		"direction":           direction,
		"windowBars":          float64(len(closes) - 1 - windowStart),
		"percentage":          config.percentage,
		"windowPolicy":        windowPolicy,
		"sessionAware":        windowPolicy == "session",
		"referenceClose":      reference,
		"currentClose":        current,
		"changePercent":       changePercent,
		"triggerPercent":      triggerPercent,
		"longTriggered":       longTriggered,
		"shortTriggered":      shortTriggered,
		"longTriggerPercent":  longTriggerPercent,
		"shortTriggerPercent": shortTriggerPercent,
		"peakClose":           peakClose,
		"troughClose":         troughClose,
		"longDrawdownPercent": longDrawdownPercent,
		"shortReboundPercent": shortReboundPercent,
	}
}

func resolveSessionAwareWindowStart(endTimes []time.Time, sessions []futu.MarketSession, windowStart int, intervalMinutes int) int {
	if windowStart < 0 {
		return -1
	}
	if intervalMinutes <= 0 || intervalMinutes >= tradingSessionMinutesPerDay {
		return windowStart
	}
	seriesLength := len(endTimes)
	if len(sessions) > seriesLength {
		seriesLength = len(sessions)
	}
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

func readMarketSessionAt(sessions []futu.MarketSession, index int) futu.MarketSession {
	if index < 0 || index >= len(sessions) {
		return futu.MarketSessionUnknown
	}
	return sessions[index]
}

func readTimeAt(values []time.Time, index int) time.Time {
	if index < 0 || index >= len(values) {
		return time.Time{}
	}
	return values[index]
}

func isSessionBoundary(previousSession, currentSession futu.MarketSession, previousTime, currentTime time.Time, intervalMinutes int) bool {
	if previousSession != futu.MarketSessionUnknown && currentSession != futu.MarketSessionUnknown && previousSession != currentSession {
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
	if period <= 0 || len(values) < period {
		return 0, false
	}
	previous, ok := simpleMovingAverage(values[:period], period)
	if !ok {
		return 0, false
	}
	for index := period; index < len(values); index++ {
		previous = (previous*float64(period-1) + values[index]) / float64(period)
	}
	return previous, true
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
	sequence := calculateSMASequence(values, period)
	if len(sequence) < period {
		return 0, false
	}
	return simpleMovingAverage(sequence, period)
}

func hullMovingAverage(values []float64, period int) (float64, bool) {
	if period <= 0 || len(values) < period {
		return 0, false
	}
	halfPeriod := max(1, period/2)
	sqrtPeriod := max(1, int(math.Round(math.Sqrt(float64(period)))))
	fastSequence := calculateWMASequence(values, halfPeriod)
	slowSequence := calculateWMASequence(values, period)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return 0, false
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
	if len(combined) < sqrtPeriod {
		return 0, false
	}
	return linearWeightedMovingAverage(combined, sqrtPeriod)
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

func calculateWMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) < period {
		return nil
	}
	result := make([]float64, 0, len(values)-period+1)
	for end := period; end <= len(values); end++ {
		value, ok := linearWeightedMovingAverage(values[:end], period)
		if ok {
			result = append(result, value)
		}
	}
	return result
}

func calculateRSI(values []float64, period int) any {
	series := calculateRSISeries(values, period)
	if len(series) == 0 {
		return nil
	}
	return series[len(series)-1]
}

func calculateRSISeries(values []float64, period int) []float64 {
	if period <= 0 || len(values) <= period {
		return nil
	}
	result := make([]float64, 0, len(values)-period)
	for end := period; end < len(values); end++ {
		gains := 0.0
		losses := 0.0
		for index := end - period + 1; index <= end; index++ {
			delta := values[index] - values[index-1]
			if delta >= 0 {
				gains += delta
			} else {
				losses += math.Abs(delta)
			}
		}
		if losses == 0 {
			result = append(result, 100.0)
			continue
		}
		relativeStrength := gains / losses
		result = append(result, 100-100/(1+relativeStrength))
	}
	return result
}

func calculateMACDSnapshot(values []float64, config macdConfig) map[string]any {
	minimum := max(config.fastPeriod, config.slowPeriod) + config.signalPeriod
	if minimum <= 0 || len(values) < minimum {
		return nil
	}
	fastSequence := calculateEMASequence(values, config.fastPeriod)
	slowSequence := calculateEMASequence(values, config.slowPeriod)
	if len(fastSequence) == 0 || len(slowSequence) == 0 {
		return nil
	}
	diffSequence := make([]float64, 0, len(values))
	for index := range values {
		diffSequence = append(diffSequence, fastSequence[index]-slowSequence[index])
	}
	signalSequence := calculateEMASequence(diffSequence, config.signalPeriod)
	if len(signalSequence) == 0 {
		return nil
	}
	currentIndex := len(diffSequence) - 1
	result := map[string]any{
		"diff":      diffSequence[currentIndex],
		"signal":    signalSequence[currentIndex],
		"histogram": (diffSequence[currentIndex] - signalSequence[currentIndex]) * 2,
	}
	if currentIndex > 0 {
		previousIndex := currentIndex - 1
		result["previousDiff"] = diffSequence[previousIndex]
		result["previousSignal"] = signalSequence[previousIndex]
		result["previousHistogram"] = (diffSequence[previousIndex] - signalSequence[previousIndex]) * 2
	}
	return result
}

func calculateEMASequence(values []float64, period int) []float64 {
	if period <= 0 || len(values) == 0 {
		return nil
	}
	multiplier := 2 / float64(period+1)
	sequence := make([]float64, 0, len(values))
	previous := values[0]
	for index, value := range values {
		if index == 0 {
			sequence = append(sequence, value)
			continue
		}
		previous = previous + (value-previous)*multiplier
		sequence = append(sequence, previous)
	}
	return sequence
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
	kValues, dValues, jValues := calculateKDJSeries(highs, lows, closes, config)
	if len(kValues) == 0 {
		return nil
	}
	last := len(kValues) - 1
	result := map[string]any{
		"k": kValues[last],
		"d": dValues[last],
		"j": jValues[last],
	}
	if last > 0 {
		result["previousK"] = kValues[last-1]
		result["previousD"] = dValues[last-1]
		result["previousJ"] = jValues[last-1]
	}
	return result
}

func calculateKDJSeries(highs, lows, closes []float64, config kdjConfig) ([]float64, []float64, []float64) {
	kValues := make([]float64, 0, len(closes))
	dValues := make([]float64, 0, len(closes))
	jValues := make([]float64, 0, len(closes))
	previousK := 50.0
	previousD := 50.0
	for index := range closes {
		start := max(0, index-config.period+1)
		highestHigh := highs[start]
		lowestLow := lows[start]
		for cursor := start + 1; cursor <= index; cursor++ {
			highestHigh = maxFloat(highestHigh, highs[cursor])
			lowestLow = minFloat(lowestLow, lows[cursor])
		}
		rsv := 50.0
		if highestHigh != lowestLow {
			rsv = ((closes[index] - lowestLow) / (highestHigh - lowestLow)) * 100
		}
		nextK := ((float64(config.m1)-1)*previousK + rsv) / float64(config.m1)
		nextD := ((float64(config.m2)-1)*previousD + nextK) / float64(config.m2)
		nextJ := 3*nextK - 2*nextD
		kValues = append(kValues, nextK)
		dValues = append(dValues, nextD)
		jValues = append(jValues, nextJ)
		previousK = nextK
		previousD = nextD
	}
	return kValues, dValues, jValues
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
	for index := period - 1; index < len(trueRanges); index++ {
		sum := 0.0
		for cursor := index - period + 1; cursor <= index; cursor++ {
			sum += trueRanges[cursor]
		}
		result = append(result, sum/float64(period))
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
	for index := period - 1; index < len(typicalPrices); index++ {
		window := typicalPrices[index-period+1 : index+1]
		sum := 0.0
		for _, value := range window {
			sum += value
		}
		average := sum / float64(period)
		meanDeviation := 0.0
		for _, value := range window {
			meanDeviation += math.Abs(value - average)
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
	result := make([]float64, 0, len(closes)-period+1)
	for index := period - 1; index < len(closes); index++ {
		start := index - period + 1
		highestHigh := highs[start]
		lowestLow := lows[start]
		for cursor := start + 1; cursor <= index; cursor++ {
			highestHigh = maxFloat(highestHigh, highs[cursor])
			lowestLow = minFloat(lowestLow, lows[cursor])
		}
		if highestHigh == lowestLow {
			result = append(result, -50)
			continue
		}
		result = append(result, -100*(highestHigh-closes[index])/(highestHigh-lowestLow))
	}
	return result
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
