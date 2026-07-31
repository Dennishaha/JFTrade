package marketdataapp

import (
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

// LiveClientStats is the transport-neutral live-client projection included in
// market-data heartbeat events.
type LiveClientStats struct {
	Connected int
	Limit     int
	AtLimit   bool
}

type liveHeartbeatSampleSummary struct {
	freshCount           int
	staleCount           int
	latestObservedAtText any
}

type liveHeartbeatRetryState struct {
	retryAfter any
	active     bool
}

// LiveHeartbeat builds the provider-aware heartbeat payload shared by live
// transports. The caller owns active-instrument aggregation and transport
// client accounting; marketdataapp owns all market-data policy projections.
func LiveHeartbeat(
	service *marketdata.Service,
	clients LiveClientStats,
	activeInstrumentIDs []string,
	heartbeatInterval time.Duration,
	staleThreshold time.Duration,
	cacheRetention time.Duration,
) map[string]any {
	state := marketdata.RuntimeState{}
	if service != nil {
		state = service.RuntimeState()
	}
	return liveHeartbeatAt(
		time.Now().UTC(),
		service,
		clients,
		activeInstrumentIDs,
		heartbeatInterval,
		staleThreshold,
		cacheRetention,
		state,
	)
}

func liveHeartbeatAt(
	now time.Time,
	service *marketdata.Service,
	clients LiveClientStats,
	activeInstrumentIDs []string,
	heartbeatInterval time.Duration,
	staleThreshold time.Duration,
	cacheRetention time.Duration,
	state marketdata.RuntimeState,
) map[string]any {
	sampleFreshness := SampleFreshness(service, staleThreshold)
	pushAvailable := livePushAvailable(service)
	samples := summarizeLiveHeartbeatSamples(
		service,
		now,
		activeInstrumentIDs,
		sampleFreshness,
		cacheRetention,
	)
	quoteRetry := liveHeartbeatRetry(state.QuoteRetryAt, now)
	streamRetry := liveHeartbeatRetry(state.StreamRetryAt, now)
	activeCount := len(activeInstrumentIDs)
	return map[string]any{
		"type":         "heartbeat",
		"at":           now.Format(time.RFC3339Nano),
		"intervalMs":   heartbeatInterval.Milliseconds(),
		"stale":        activeCount > 0 && samples.staleCount > 0,
		"staleReasons": liveHeartbeatStaleReasons(activeCount, samples.staleCount, state, quoteRetry, streamRetry, pushAvailable),
		"transport":    liveHeartbeatTransport(activeCount, samples, sampleFreshness, state.Connected, pushAvailable),
		"liveClients":  liveHeartbeatClients(clients),
		"liveQuote":    liveHeartbeatQuote(state, quoteRetry),
		"liveStream":   liveHeartbeatStream(state, streamRetry, pushAvailable),
	}
}

// SampleFreshness expands the push-oriented stale threshold for a poll-only
// provider so one normal polling interval plus its request timeout is healthy.
func SampleFreshness(service *marketdata.Service, staleThreshold time.Duration) time.Duration {
	runtime := RuntimeFromService(service)
	if runtime == nil || runtime.PushAvailable() {
		return staleThreshold
	}
	policy := runtime.QuotePollingPolicy()
	if delayedFreshness := policy.Interval + policy.Timeout; delayedFreshness > staleThreshold {
		return delayedFreshness
	}
	return staleThreshold
}

func livePushAvailable(service *marketdata.Service) bool {
	runtime := RuntimeFromService(service)
	return runtime == nil || runtime.PushAvailable()
}

func summarizeLiveHeartbeatSamples(
	service *marketdata.Service,
	now time.Time,
	instrumentIDs []string,
	freshness time.Duration,
	cacheRetention time.Duration,
) liveHeartbeatSampleSummary {
	summary := liveHeartbeatSampleSummary{}
	var latestObservedAt time.Time
	for _, instrumentID := range instrumentIDs {
		observedAt, ok := liveHeartbeatObservedAt(service, instrumentID, cacheRetention)
		if !ok {
			summary.staleCount++
			continue
		}
		if latestObservedAt.IsZero() || observedAt.After(latestObservedAt) {
			latestObservedAt = observedAt
			summary.latestObservedAtText = observedAt.Format(time.RFC3339Nano)
		}
		if now.Sub(observedAt) <= freshness {
			summary.freshCount++
			continue
		}
		summary.staleCount++
	}
	return summary
}

func liveHeartbeatObservedAt(
	service *marketdata.Service,
	instrumentID string,
	cacheRetention time.Duration,
) (time.Time, bool) {
	if service == nil {
		return time.Time{}, false
	}
	sample := service.Latest(instrumentID, cacheRetention)
	if sample == nil {
		return time.Time{}, false
	}
	observedAt := parseHeartbeatTime(sample.ObservedAt)
	return observedAt, !observedAt.IsZero()
}

func parseHeartbeatTime(value string) time.Time {
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func liveHeartbeatRetry(retryAfter time.Time, now time.Time) liveHeartbeatRetryState {
	if retryAfter.IsZero() {
		return liveHeartbeatRetryState{}
	}
	return liveHeartbeatRetryState{
		retryAfter: retryAfter.UTC().Format(time.RFC3339Nano),
		active:     now.UTC().Before(retryAfter),
	}
}

func liveHeartbeatStaleReasons(
	activeCount int,
	staleCount int,
	state marketdata.RuntimeState,
	quoteRetry liveHeartbeatRetryState,
	streamRetry liveHeartbeatRetryState,
	pushAvailable bool,
) []any {
	reasons := make([]any, 0, 4)
	if staleCount > 0 {
		reasons = append(reasons, "market-data-samples-stale")
	}
	if quoteRetry.active {
		reasons = append(reasons, "live-quote-backoff")
	}
	if pushAvailable && streamRetry.active {
		reasons = append(reasons, "live-stream-backoff")
	}
	if pushAvailable && activeCount > 0 && !state.Connected {
		reasons = append(reasons, "live-stream-disconnected")
	}
	return reasons
}

func liveHeartbeatTransportMode(
	activeCount int,
	liveStreamConnected bool,
	pushAvailable bool,
) string {
	if !pushAvailable {
		if activeCount > 0 {
			return "snapshot-poll-delayed"
		}
		return "idle"
	}
	if liveStreamConnected {
		return "push-stream"
	}
	if activeCount > 0 {
		return "snapshot-poll-fallback"
	}
	return "idle"
}

func liveHeartbeatTransport(
	activeCount int,
	samples liveHeartbeatSampleSummary,
	freshness time.Duration,
	connected bool,
	pushAvailable bool,
) map[string]any {
	return map[string]any{
		"mode":              liveHeartbeatTransportMode(activeCount, connected, pushAvailable),
		"activeInstruments": activeCount,
		"freshInstruments":  samples.freshCount,
		"staleInstruments":  samples.staleCount,
		"sampleFreshnessMs": freshness.Milliseconds(),
		"latestObservedAt":  samples.latestObservedAtText,
	}
}

func liveHeartbeatClients(clients LiveClientStats) map[string]any {
	return map[string]any{
		"connected": clients.Connected,
		"limit":     clients.Limit,
		"atLimit":   clients.AtLimit,
	}
}

func liveHeartbeatQuote(
	state marketdata.RuntimeState,
	retry liveHeartbeatRetryState,
) map[string]any {
	return map[string]any{
		"lastRefreshAt": liveHeartbeatRefreshTime(state.LastRefreshAt),
		"backoffActive": retry.active,
		"retryAfter":    retry.retryAfter,
		"failureCount":  state.QuoteFailures,
		"lastError":     liveHeartbeatOptionalString(state.QuoteLastError),
	}
}

func liveHeartbeatStream(
	state marketdata.RuntimeState,
	retry liveHeartbeatRetryState,
	pushAvailable bool,
) map[string]any {
	return map[string]any{
		"supported":     pushAvailable,
		"connected":     state.Connected,
		"backoffActive": retry.active,
		"retryAfter":    retry.retryAfter,
		"failureCount":  state.StreamFailures,
		"lastError":     liveHeartbeatOptionalString(state.StreamLastError),
	}
}

func liveHeartbeatRefreshTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func liveHeartbeatOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
