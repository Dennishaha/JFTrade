package jftradeapi

import (
	"context"
	"log"
	"sort"
	"time"
)

func (s *Server) liveHeartbeatEvent(heartbeatInterval time.Duration) map[string]any {
	now := time.Now().UTC()
	activeInstrumentIDs := s.activeLiveStreamInstrumentIDs()

	freshCount := 0
	staleCount := 0
	latestObservedAtText := any(nil)
	var latestObservedAt time.Time
	for _, instrumentID := range activeInstrumentIDs {
		sample := s.latestTickerSample(instrumentID, tickCacheRetention)
		if sample == nil {
			staleCount++
			continue
		}
		observedAt := parseQueryTime(sample.ObservedAt, time.Time{})
		if observedAt.IsZero() {
			staleCount++
			continue
		}
		observedAt = observedAt.UTC()
		if latestObservedAt.IsZero() || observedAt.After(latestObservedAt) {
			latestObservedAt = observedAt
			latestObservedAtText = observedAt.Format(time.RFC3339Nano)
		}
		if now.Sub(observedAt) <= liveHeartbeatStaleThreshold {
			freshCount++
			continue
		}
		staleCount++
	}

	s.liveQuoteState.mu.Lock()
	liveQuoteLastRefreshAt := s.liveQuoteState.lastRefreshAt
	liveQuoteRetryAfter := s.liveQuoteState.retryAfter
	liveQuoteFailureCount := s.liveQuoteState.failureCount
	liveQuoteLastError := s.liveQuoteState.lastError
	s.liveQuoteState.mu.Unlock()

	s.liveStreamState.mu.Lock()
	liveStreamConnected := s.liveStreamState.stream != nil
	liveStreamRetryAfter := s.liveStreamState.retryAfter
	liveStreamFailureCount := s.liveStreamState.failureCount
	liveStreamLastError := s.liveStreamState.lastError
	s.liveStreamState.mu.Unlock()

	liveQuoteRetryAfterText, liveQuoteBackoffActive := retryState(liveQuoteRetryAfter)
	liveStreamRetryAfterText, liveStreamBackoffActive := retryState(liveStreamRetryAfter)
	liveClients, liveClientLimit, liveClientLimitReached := s.liveStreamStats()

	staleReasons := make([]any, 0, 4)
	stale := len(activeInstrumentIDs) > 0 && staleCount > 0
	if staleCount > 0 {
		staleReasons = append(staleReasons, "market-data-samples-stale")
	}
	if liveQuoteBackoffActive {
		staleReasons = append(staleReasons, "live-quote-backoff")
	}
	if liveStreamBackoffActive {
		staleReasons = append(staleReasons, "live-stream-backoff")
	}
	if len(activeInstrumentIDs) > 0 && !liveStreamConnected {
		staleReasons = append(staleReasons, "live-stream-disconnected")
	}

	liveQuoteLastRefreshAtText := any(nil)
	if !liveQuoteLastRefreshAt.IsZero() {
		liveQuoteLastRefreshAtText = liveQuoteLastRefreshAt.UTC().Format(time.RFC3339Nano)
	}

	transportMode := "idle"
	if len(activeInstrumentIDs) > 0 {
		transportMode = "snapshot-poll-fallback"
	}
	if liveStreamConnected {
		transportMode = "push-stream"
	}

	return map[string]any{
		"type":         "heartbeat",
		"at":           now.Format(time.RFC3339Nano),
		"intervalMs":   heartbeatInterval.Milliseconds(),
		"stale":        stale,
		"staleReasons": staleReasons,
		"transport": map[string]any{
			"mode":              transportMode,
			"activeInstruments": len(activeInstrumentIDs),
			"freshInstruments":  freshCount,
			"staleInstruments":  staleCount,
			"sampleFreshnessMs": liveHeartbeatStaleThreshold.Milliseconds(),
			"latestObservedAt":  latestObservedAtText,
		},
		"liveClients": map[string]any{
			"connected": liveClients,
			"limit":     liveClientLimit,
			"atLimit":   liveClientLimitReached,
		},
		"liveQuote": map[string]any{
			"lastRefreshAt": liveQuoteLastRefreshAtText,
			"backoffActive": liveQuoteBackoffActive,
			"retryAfter":    liveQuoteRetryAfterText,
			"failureCount":  liveQuoteFailureCount,
			"lastError":     stringPointerOrNil(liveQuoteLastError),
		},
		"liveStream": map[string]any{
			"connected":     liveStreamConnected,
			"backoffActive": liveStreamBackoffActive,
			"retryAfter":    liveStreamRetryAfterText,
			"failureCount":  liveStreamFailureCount,
			"lastError":     stringPointerOrNil(liveStreamLastError),
		},
	}
}

func (s *Server) refreshLiveMarketTicksIfNeeded(ctx context.Context) {
	instrumentIDs := s.activeLiveStreamInstrumentIDs()
	if len(instrumentIDs) == 0 {
		return
	}
	s.ensureLiveMarketStream(ctx, instrumentIDs)
	if s.allHaveFreshTickerSamples(instrumentIDs, liveTickFallbackPollInterval) {
		return
	}

	s.liveQuoteState.mu.Lock()
	defer s.liveQuoteState.mu.Unlock()

	now := time.Now().UTC()
	if !s.liveQuoteState.retryAfter.IsZero() && now.Before(s.liveQuoteState.retryAfter) {
		return
	}
	if !s.liveQuoteState.lastRefreshAt.IsZero() && now.Sub(s.liveQuoteState.lastRefreshAt) < liveTickFallbackPollInterval {
		return
	}
	if s.allHaveFreshTickerSamples(instrumentIDs, liveTickFallbackPollInterval) {
		return
	}
	s.liveQuoteState.lastRefreshAt = now

	refreshCtx, cancel := context.WithTimeout(ctx, liveTickFallbackPollTimeout)
	defer cancel()

	queryStart := time.Now()
	exchange := s.liveMarketExchange()
	if exchange == nil {
		return
	}
	tickers, err := exchange.QueryTickers(refreshCtx, instrumentIDs...)
	queryElapsed := time.Since(queryStart)
	if err != nil {
		retryDelay := liveRetryDelay(s.liveQuoteState.failureCount)
		s.liveQuoteState.failureCount++
		s.liveQuoteState.retryAfter = time.Now().UTC().Add(retryDelay)
		s.liveQuoteState.lastError = err.Error()
		log.Printf("JFTrade live quote refresh failed after %v (timeout=%v, instruments=%d); retrying in %s: %v",
			queryElapsed, liveTickFallbackPollTimeout, len(instrumentIDs), retryDelay, err)
		return
	}
	log.Printf("JFTrade live quote refresh OK in %v (instruments=%d, ticks=%d)",
		queryElapsed, len(instrumentIDs), len(tickers))
	s.liveQuoteState.failureCount = 0
	s.liveQuoteState.retryAfter = time.Time{}
	s.liveQuoteState.lastError = ""
	for _, instrumentID := range instrumentIDs {
		ticker, ok := tickers[instrumentID]
		if !ok {
			continue
		}
		s.recordTickerSample(instrumentID, &ticker)
	}
}

func (s *Server) writeLiveMarketTicks(
	ctx context.Context,
	writer liveEventWriter,
	instrumentIDs []string,
	lastSentByInstrument map[string]string,
	initial bool,
) error {
	s.refreshLiveMarketTicksIfNeeded(ctx)
	initialObservedAt := ""
	if initial {
		initialObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	for _, sample := range s.latestTickerSamples(instrumentIDs, liveTickSampleFreshness) {
		if sample == nil || lastSentByInstrument[sample.InstrumentID] == sample.ObservedAt {
			continue
		}
		event := liveTickEventFromSampleAt(sample, initialObservedAt)
		if event == nil {
			continue
		}
		if err := writer.WriteEvent(event); err != nil {
			return err
		}
		lastSentByInstrument[sample.InstrumentID] = sample.ObservedAt
	}
	return nil
}

func (s *Server) activeMarketInstrumentIDs() []string {
	return s.marketSubscriptions.activeInstrumentIDs()
}

func (s *Server) activeLiveStreamInstrumentIDs() []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, instrumentID := range s.marketSubscriptions.activeInstrumentIDs() {
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		result = append(result, instrumentID)
	}
	if s.strategyRuntimeManager != nil {
		for _, instrumentID := range s.strategyRuntimeManager.activeInstrumentIDs() {
			if _, exists := seen[instrumentID]; exists {
				continue
			}
			seen[instrumentID] = struct{}{}
			result = append(result, instrumentID)
		}
	}
	for _, instrumentID := range s.liveSocketClients.activeInstrumentIDs() {
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		result = append(result, instrumentID)
	}
	sort.Strings(result)
	return result
}
