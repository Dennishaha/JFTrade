package servercore

import (
	"context"
	"sort"
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
)

func (s *Server) liveHeartbeatEvent(heartbeatInterval time.Duration, clients apilive.ClientStats, webSocketInstrumentIDs []string) map[string]any {
	now := time.Now().UTC()
	activeInstrumentIDs := s.activeLiveStreamInstrumentIDs(webSocketInstrumentIDs)

	freshCount := 0
	staleCount := 0
	latestObservedAtText := any(nil)
	var latestObservedAt time.Time
	for _, instrumentID := range activeInstrumentIDs {
		sample := s.marketdataSvc.Latest(instrumentID, tickCacheRetention)
		if sample == nil {
			staleCount++
			continue
		}
		observedAt := httpserver.ParseQueryTime(sample.ObservedAt, time.Time{})
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

	runtimeState := s.marketdataSvc.RuntimeState()
	liveQuoteLastRefreshAt := runtimeState.LastRefreshAt
	liveQuoteRetryAfter := runtimeState.QuoteRetryAt
	liveQuoteFailureCount := runtimeState.QuoteFailures
	liveQuoteLastError := runtimeState.QuoteLastError
	liveStreamConnected := runtimeState.Connected
	liveStreamRetryAfter := runtimeState.StreamRetryAt
	liveStreamFailureCount := runtimeState.StreamFailures
	liveStreamLastError := runtimeState.StreamLastError

	liveQuoteRetryAfterText, liveQuoteBackoffActive := retryState(liveQuoteRetryAfter)
	liveStreamRetryAfterText, liveStreamBackoffActive := retryState(liveStreamRetryAfter)
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
			"connected": clients.Connected,
			"limit":     clients.Limit,
			"atLimit":   clients.AtLimit,
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
	if s != nil && s.marketdataSvc != nil {
		s.marketdataSvc.WakeCollector()
	}
}

func (s *Server) activeMarketInstrumentIDs() []string {
	if s.marketdataSvc == nil {
		return nil
	}
	instrumentIDs, err := s.marketdataSvc.GetActiveInstruments(context.Background())
	if err != nil {
		return nil
	}
	return instrumentIDs
}

func (s *Server) activeLiveStreamInstrumentIDs(webSocketInstrumentIDs []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, instrumentID := range s.activeMarketInstrumentIDs() {
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
	if webSocketInstrumentIDs == nil && s.liveWebSocket != nil {
		webSocketInstrumentIDs = s.liveWebSocket.ActiveInstrumentIDs()
	}
	for _, instrumentID := range webSocketInstrumentIDs {
		if _, exists := seen[instrumentID]; exists {
			continue
		}
		seen[instrumentID] = struct{}{}
		result = append(result, instrumentID)
	}
	sort.Strings(result)
	return result
}
