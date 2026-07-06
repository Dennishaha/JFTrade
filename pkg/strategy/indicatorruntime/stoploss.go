package indicatorruntime

import (
	"math"
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

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
	windowStart, windowPolicy, ok := stopLossWindowStart(closes, endTimes, sessions, config, intervalMinutes, cache)
	if !ok {
		return nil
	}
	reference := closes[windowStart]
	current := closes[len(closes)-1]
	if invalidStopLossPrice(reference) || invalidStopLossPrice(current) {
		return nil
	}
	changePercent := ((current - reference) / reference) * 100
	mode := normalizeStopLossMode(config.mode)
	direction := normalizeStopLossDirection(config.direction)
	var longTriggered, shortTriggered bool
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
		stopLossTriggerPercent(direction, longTriggered, shortTriggered, longTriggerPercent, shortTriggerPercent),
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

func stopLossWindowStart(closes []float64, endTimes []time.Time, sessions []market.Session, config stopLossConfig, intervalMinutes int, cache *snapshotSeriesCache) (int, string, bool) {
	lookback := resolveBarCount(config.timeValue, config.timeUnit, intervalMinutes)
	if lookback <= 0 || len(closes) <= lookback {
		return 0, "", false
	}
	windowStart := len(closes) - 1 - lookback
	if windowStart < 0 {
		return 0, "", false
	}
	windowPolicy := normalizeStopLossWindowPolicy(config.windowPolicy)
	if windowPolicy != "session" {
		return windowStart, windowPolicy, true
	}
	windowStart = resolveSessionAwareWindowStartWithCache(endTimes, sessions, windowStart, intervalMinutes, cache)
	if windowStart < 0 {
		return 0, "", false
	}
	return windowStart, windowPolicy, true
}

func invalidStopLossPrice(value float64) bool {
	return value <= 0 || math.IsNaN(value) || math.IsInf(value, 0)
}

func stopLossTriggerPercent(direction string, longTriggered bool, shortTriggered bool, longTriggerPercent float64, shortTriggerPercent float64) float64 {
	switch direction {
	case "long":
		return longTriggerPercent
	case "short":
		return shortTriggerPercent
	default:
		if longTriggered && !shortTriggered {
			return longTriggerPercent
		}
		if shortTriggered && !longTriggered {
			return shortTriggerPercent
		}
		return max(longTriggerPercent, shortTriggerPercent)
	}
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
	var longTriggered, shortTriggered bool
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
	var triggerPercent float64
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
	var triggered bool
	switch direction {
	case "long":
		triggered = longTriggered
	case "short":
		triggered = shortTriggered
	default:
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
