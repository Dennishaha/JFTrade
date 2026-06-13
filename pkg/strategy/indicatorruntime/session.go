package indicatorruntime

import (
	"time"

	"github.com/jftrade/jftrade-main/pkg/market"
)

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
