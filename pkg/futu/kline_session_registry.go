package futu

import (
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/types"
)

const (
	trackedKLineSessionTTL      = 24 * time.Hour
	maxTrackedKLineSessions     = 8192
	marketSessionSampleTTL      = 12 * time.Hour
	maxMarketSessionSamplesEach = 256
)

type klineSessionRecord struct {
	session    MarketSession
	recordedAt time.Time
}

type marketSessionSample struct {
	at      time.Time
	session MarketSession
}

// RegisterKLineSession stores a session label for a concrete kline so runtime
// consumers can reuse the exchange-layer result instead of re-classifying it.
func (e *Exchange) RegisterKLineSession(kline types.KLine, session MarketSession) {
	if e == nil || session == MarketSessionUnknown {
		return
	}
	key := klineSessionCacheKey(kline)
	if key == "" {
		return
	}
	now := time.Now().UTC()

	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	if e.klineSessions == nil {
		e.klineSessions = make(map[string]klineSessionRecord)
	}
	pruneKLineSessionCacheLocked(e.klineSessions, now)
	e.klineSessions[key] = klineSessionRecord{session: session, recordedAt: now}
}

// RecordMarketSessionSample stores a live quote/session observation from the
// market-data layer so closed kline callbacks can reuse the latest session tag.
func (e *Exchange) RecordMarketSessionSample(symbol string, session MarketSession, observedAt time.Time) {
	if e == nil || session == MarketSessionUnknown {
		return
	}
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	if e.marketSessionSamples == nil {
		e.marketSessionSamples = make(map[string][]marketSessionSample)
	}
	samples := append(e.marketSessionSamples[normalizedSymbol], marketSessionSample{at: observedAt, session: session})
	e.marketSessionSamples[normalizedSymbol] = pruneMarketSessionSamples(samples, observedAt)
}

// ResolveKLineSession returns the exchange-layer session tag for the provided
// kline when an exact historical record or a nearby live quote sample exists.
func (e *Exchange) ResolveKLineSession(kline types.KLine) (MarketSession, bool) {
	if e == nil {
		return MarketSessionUnknown, false
	}
	key := klineSessionCacheKey(kline)
	normalizedSymbol := strings.ToUpper(strings.TrimSpace(kline.Symbol))
	startAt := kline.StartTime.Time().UTC()
	endAt := kline.EndTime.Time().UTC()
	now := time.Now().UTC()

	e.sessionMu.RLock()
	if record, ok := e.klineSessions[key]; ok && now.Sub(record.recordedAt) <= trackedKLineSessionTTL {
		e.sessionMu.RUnlock()
		return record.session, true
	}
	samples := append([]marketSessionSample(nil), e.marketSessionSamples[normalizedSymbol]...)
	e.sessionMu.RUnlock()

	if session, ok := resolveSessionFromSamples(samples, startAt, endAt, kline.Interval.Duration()); ok {
		return session, true
	}
	return MarketSessionUnknown, false
}

func resolveKLineSessionByClock(symbol string, kline types.KLine) MarketSession {
	resolvedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if resolvedSymbol == "" {
		resolvedSymbol = strings.ToUpper(strings.TrimSpace(kline.Symbol))
	}
	observedAt := kline.StartTime.Time().UTC()
	if observedAt.IsZero() {
		observedAt = kline.EndTime.Time().UTC()
	}
	if resolvedSymbol == "" || observedAt.IsZero() {
		return MarketSessionUnknown
	}
	return ClassifyMarketSession(resolvedSymbol, observedAt)
}

func klineSessionCacheKey(kline types.KLine) string {
	startAt := kline.StartTime.Time().UTC()
	endAt := kline.EndTime.Time().UTC()
	if strings.TrimSpace(kline.Symbol) == "" || startAt.IsZero() || endAt.IsZero() {
		return ""
	}
	return fmt.Sprintf(
		"%s|%s|%d|%d",
		strings.ToUpper(strings.TrimSpace(kline.Symbol)),
		strings.TrimSpace(string(kline.Interval)),
		startAt.UnixMilli(),
		endAt.UnixMilli(),
	)
}

func pruneKLineSessionCacheLocked(cache map[string]klineSessionRecord, now time.Time) {
	if len(cache) == 0 {
		return
	}
	for key, record := range cache {
		if now.Sub(record.recordedAt) > trackedKLineSessionTTL {
			delete(cache, key)
		}
	}
	if len(cache) <= maxTrackedKLineSessions {
		return
	}
	for key := range cache {
		delete(cache, key)
		if len(cache) <= maxTrackedKLineSessions {
			return
		}
	}
}

func pruneMarketSessionSamples(samples []marketSessionSample, now time.Time) []marketSessionSample {
	if len(samples) == 0 {
		return nil
	}
	keep := samples[:0]
	for _, sample := range samples {
		if sample.session == MarketSessionUnknown {
			continue
		}
		if now.Sub(sample.at.UTC()) > marketSessionSampleTTL {
			continue
		}
		keep = append(keep, marketSessionSample{at: sample.at.UTC(), session: sample.session})
	}
	if len(keep) > maxMarketSessionSamplesEach {
		keep = append([]marketSessionSample(nil), keep[len(keep)-maxMarketSessionSamplesEach:]...)
		return keep
	}
	return append([]marketSessionSample(nil), keep...)
}

func resolveSessionFromSamples(samples []marketSessionSample, startAt, endAt time.Time, interval time.Duration) (MarketSession, bool) {
	if len(samples) == 0 {
		return MarketSessionUnknown, false
	}
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	if interval < time.Minute {
		interval = time.Minute
	}
	windowStart := startAt.UTC().Add(-interval)
	windowEnd := endAt.UTC().Add(interval)
	if startAt.IsZero() {
		windowStart = windowEnd.Add(-interval * 2)
	}
	if endAt.IsZero() {
		windowEnd = windowStart.Add(interval * 2)
	}

	var best marketSessionSample
	matched := false
	for _, sample := range samples {
		at := sample.at.UTC()
		if at.Before(windowStart) || at.After(windowEnd) {
			continue
		}
		if !matched || at.After(best.at) {
			best = marketSessionSample{at: at, session: sample.session}
			matched = true
		}
	}
	if !matched {
		return MarketSessionUnknown, false
	}
	return best.session, true
}
