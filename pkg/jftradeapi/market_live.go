package jftradeapi

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	bbgotypes "github.com/c9s/bbgo/pkg/types"
)

const (
	liveTickDispatchInterval     = 250 * time.Millisecond
	liveTickFallbackPollInterval = 1 * time.Second
	liveTickFallbackPollTimeout  = 900 * time.Millisecond
	liveTickSampleFreshness      = 1500 * time.Millisecond
	liveHeartbeatStaleThreshold  = liveTickFallbackPollInterval + liveTickSampleFreshness
	liveStreamConnectTimeout     = 8 * time.Second
	liveStreamRetryBaseDelay     = 5 * time.Second
	liveStreamRetryMaxDelay      = 30 * time.Second
	defaultSSEClientRetry        = liveStreamRetryBaseDelay
)

type liveSSEWriter struct {
	sseWriter
}

func newLiveSSEWriter(w http.ResponseWriter, flusher http.Flusher) liveSSEWriter {
	return liveSSEWriter{
		sseWriter: newSSEWriter(w, flusher),
	}
}

func (s *Server) handleLiveEventStream(w http.ResponseWriter, r *http.Request) {
	limit := s.effectiveLiveStreamLimit()
	if !s.tryAcquireLiveStreamSlot(limit) {
		s.writeError(w, http.StatusServiceUnavailable, "LIVE_SSE_LIMIT_REACHED", fmt.Sprintf("live sse connection limit reached (%d)", limit))
		return
	}
	defer s.releaseLiveStreamSlot()

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "LIVE_SSE_UNSUPPORTED", "response writer does not support streaming")
		return
	}

	writeSSEHeaders(w)

	s.ensureLiveNotificationBridge(r.Context())
	writer := newLiveSSEWriter(w, flusher)
	if err := writer.WriteRetryDirective(); err != nil {
		return
	}
	dispatcher := newLiveStreamDispatcher(s, r.Context(), writer, nil)
	if err := dispatcher.writeInitialEvents(); err != nil {
		return
	}
	_ = dispatcher.run()
}

func writeHeartbeat(writer liveEventWriter, payload map[string]any) error {
	return writer.WriteEvent(payload)
}

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
		"retryMs":      int64(defaultSSEClientRetry / time.Millisecond),
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

func (s *Server) effectiveLiveStreamLimit() int {
	limit := s.store.integration().Config.MaxWebSocketConnections
	if limit <= 0 {
		return defaultMaxWebSocketClients
	}
	return limit
}

func (s *Server) tryAcquireLiveStreamSlot(limit int) bool {
	return s.liveStreams.tryAcquire(limit)
}

func (s *Server) releaseLiveStreamSlot() {
	s.liveStreams.release()
}

func (s *Server) liveStreamStats() (count int, limit int, atLimit bool) {
	limit = s.effectiveLiveStreamLimit()
	count = s.liveStreams.count()
	return count, limit, count >= limit
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

func (s *Server) ensureLiveMarketStream(ctx context.Context, instrumentIDs []string) {
	streamKey, symbols := liveMarketStreamKey(s.store.integration().Config, instrumentIDs)
	if len(symbols) == 0 {
		return
	}

	s.liveStreamState.mu.Lock()
	if !s.liveStreamState.retryAfter.IsZero() && time.Now().UTC().Before(s.liveStreamState.retryAfter) {
		s.liveStreamState.mu.Unlock()
		return
	}
	if s.liveStreamState.stream != nil && s.liveStreamState.streamKey == streamKey {
		s.liveStreamState.mu.Unlock()
		return
	}
	if s.liveStreamState.stream != nil {
		_ = s.liveStreamState.stream.Close()
	}
	exchange := s.liveMarketExchange()
	if exchange == nil {
		s.liveStreamState.mu.Unlock()
		return
	}
	stream := exchange.NewStream()
	stream.SetPublicOnly()
	for _, symbol := range symbols {
		stream.Subscribe(bbgotypes.MarketTradeChannel, symbol, bbgotypes.SubscribeOptions{})
	}
	stream.OnMarketTrade(func(trade bbgotypes.Trade) {
		s.recordTradeTickSample(trade)
		if s.strategyRuntimeManager != nil {
			s.strategyRuntimeManager.handleMarketTrade(trade)
		}
	})
	s.liveStreamState.stream = stream
	s.liveStreamState.streamKey = streamKey
	s.liveStreamState.mu.Unlock()

	// Run the OpenD push subscription handshake off the live stream dispatch
	// goroutine so a slow handshake cannot block live tick fan-out, and use a
	// background context with a generous timeout so the connect is not bound
	// to the 900ms fallback poll budget.
	go func() {
		connectCtx, cancel := context.WithTimeout(context.Background(), liveStreamConnectTimeout)
		defer cancel()
		if err := stream.Connect(connectCtx); err != nil {
			retryDelay := s.nextLiveStreamRetryDelay()
			s.liveStreamState.mu.Lock()
			if s.liveStreamState.stream == stream {
				s.liveStreamState.stream = nil
				s.liveStreamState.streamKey = ""
			}
			s.liveStreamState.failureCount++
			s.liveStreamState.retryAfter = time.Now().UTC().Add(retryDelay)
			s.liveStreamState.lastError = err.Error()
			s.liveStreamState.mu.Unlock()
			_ = stream.Close()
			log.Printf("JFTrade live market stream connect failed; retrying in %s: %v", retryDelay, err)
			return
		}

		s.liveStreamState.mu.Lock()
		if s.liveStreamState.stream == stream {
			s.liveStreamState.failureCount = 0
			s.liveStreamState.retryAfter = time.Time{}
			s.liveStreamState.lastError = ""
		}
		s.liveStreamState.mu.Unlock()
	}()
}

func (s *Server) nextLiveStreamRetryDelay() time.Duration {
	s.liveStreamState.mu.Lock()
	failures := s.liveStreamState.failureCount
	s.liveStreamState.mu.Unlock()
	return liveRetryDelay(failures)
}

func liveRetryDelay(failures int) time.Duration {
	delay := liveStreamRetryBaseDelay
	for i := 0; i < failures && delay < liveStreamRetryMaxDelay; i++ {
		delay *= 2
	}
	if delay > liveStreamRetryMaxDelay {
		return liveStreamRetryMaxDelay
	}
	return delay
}

func liveMarketStreamKey(config FutuIntegrationConfig, instrumentIDs []string) (string, []string) {
	seen := map[string]struct{}{}
	symbols := make([]string, 0, len(instrumentIDs))
	for _, instrumentID := range instrumentIDs {
		symbol := strings.ToUpper(strings.TrimSpace(instrumentID))
		if symbol == "" {
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	return strings.Join([]string{
		config.Host,
		strconv.Itoa(config.APIPort),
		config.WebSocketKey,
		strings.Join(symbols, ","),
	}, "|"), symbols
}

func (s *Server) writeLiveMarketTicks(ctx context.Context, writer liveEventWriter, lastSentByInstrument map[string]string, initial bool) error {
	s.refreshLiveMarketTicksIfNeeded(ctx)
	initialObservedAt := ""
	if initial {
		initialObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	for _, sample := range s.latestTickerSamples(s.activeMarketInstrumentIDs(), liveTickSampleFreshness) {
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
	sort.Strings(result)
	return result
}
