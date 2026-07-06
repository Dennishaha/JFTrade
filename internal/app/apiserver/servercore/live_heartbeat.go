package servercore

import (
	"context"
	"sort"
	"time"

	"github.com/jftrade/jftrade-main/internal/api/httpserver"
	apilive "github.com/jftrade/jftrade-main/internal/api/live"
)

type liveHeartbeatSampleSummary struct {
	freshCount           int
	staleCount           int
	latestObservedAtText any
}

func (s *Server) liveHeartbeatEvent(heartbeatInterval time.Duration, clients apilive.ClientStats, webSocketInstrumentIDs []string) map[string]any {
	now := time.Now().UTC()
	activeInstrumentIDs := s.activeLiveStreamInstrumentIDs(webSocketInstrumentIDs)
	sampleSummary := s.summarizeLiveHeartbeatSamples(now, activeInstrumentIDs)
	runtimeState := s.marketdataSvc.RuntimeState()
	liveQuoteRetryAfterText, liveQuoteBackoffActive := retryState(runtimeState.QuoteRetryAt)
	liveStreamRetryAfterText, liveStreamBackoffActive := retryState(runtimeState.StreamRetryAt)
	stale := len(activeInstrumentIDs) > 0 && sampleSummary.staleCount > 0

	return map[string]any{
		"type":         "heartbeat",
		"at":           now.Format(time.RFC3339Nano),
		"intervalMs":   heartbeatInterval.Milliseconds(),
		"stale":        stale,
		"staleReasons": liveHeartbeatStaleReasons(len(activeInstrumentIDs), sampleSummary.staleCount, runtimeState.Connected, liveQuoteBackoffActive, liveStreamBackoffActive),
		"transport": map[string]any{
			"mode":              liveHeartbeatTransportMode(len(activeInstrumentIDs), runtimeState.Connected),
			"activeInstruments": len(activeInstrumentIDs),
			"freshInstruments":  sampleSummary.freshCount,
			"staleInstruments":  sampleSummary.staleCount,
			"sampleFreshnessMs": liveHeartbeatStaleThreshold.Milliseconds(),
			"latestObservedAt":  sampleSummary.latestObservedAtText,
		},
		"liveClients": map[string]any{
			"connected": clients.Connected,
			"limit":     clients.Limit,
			"atLimit":   clients.AtLimit,
		},
		"liveQuote": map[string]any{
			"lastRefreshAt": liveHeartbeatRefreshTime(runtimeState.LastRefreshAt),
			"backoffActive": liveQuoteBackoffActive,
			"retryAfter":    liveQuoteRetryAfterText,
			"failureCount":  runtimeState.QuoteFailures,
			"lastError":     stringPointerOrNil(runtimeState.QuoteLastError),
		},
		"liveStream": map[string]any{
			"connected":     runtimeState.Connected,
			"backoffActive": liveStreamBackoffActive,
			"retryAfter":    liveStreamRetryAfterText,
			"failureCount":  runtimeState.StreamFailures,
			"lastError":     stringPointerOrNil(runtimeState.StreamLastError),
		},
	}
}

func (s *Server) summarizeLiveHeartbeatSamples(now time.Time, instrumentIDs []string) liveHeartbeatSampleSummary {
	summary := liveHeartbeatSampleSummary{}
	var latestObservedAt time.Time
	for _, instrumentID := range instrumentIDs {
		observedAt, ok := s.liveHeartbeatObservedAt(instrumentID)
		if !ok {
			summary.staleCount++
			continue
		}
		if latestObservedAt.IsZero() || observedAt.After(latestObservedAt) {
			latestObservedAt = observedAt
			summary.latestObservedAtText = observedAt.Format(time.RFC3339Nano)
		}
		if now.Sub(observedAt) <= liveHeartbeatStaleThreshold {
			summary.freshCount++
			continue
		}
		summary.staleCount++
	}
	return summary
}

func (s *Server) liveHeartbeatObservedAt(instrumentID string) (time.Time, bool) {
	sample := s.marketdataSvc.Latest(instrumentID, tickCacheRetention)
	if sample == nil {
		return time.Time{}, false
	}
	observedAt := httpserver.ParseQueryTime(sample.ObservedAt, time.Time{})
	if observedAt.IsZero() {
		return time.Time{}, false
	}
	return observedAt.UTC(), true
}

func liveHeartbeatStaleReasons(activeCount int, staleCount int, liveStreamConnected bool, liveQuoteBackoffActive bool, liveStreamBackoffActive bool) []any {
	reasons := make([]any, 0, 4)
	if staleCount > 0 {
		reasons = append(reasons, "market-data-samples-stale")
	}
	if liveQuoteBackoffActive {
		reasons = append(reasons, "live-quote-backoff")
	}
	if liveStreamBackoffActive {
		reasons = append(reasons, "live-stream-backoff")
	}
	if activeCount > 0 && !liveStreamConnected {
		reasons = append(reasons, "live-stream-disconnected")
	}
	return reasons
}

func liveHeartbeatRefreshTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func liveHeartbeatTransportMode(activeCount int, liveStreamConnected bool) string {
	if liveStreamConnected {
		return "push-stream"
	}
	if activeCount > 0 {
		return "snapshot-poll-fallback"
	}
	return "idle"
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
