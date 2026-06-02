package jftradeapi

import (
	"context"
	"encoding/json"
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
	liveStreamConnectTimeout     = 8 * time.Second
	liveStreamRetryBaseDelay     = 5 * time.Second
	liveStreamRetryMaxDelay      = 30 * time.Second
)

type liveSSEWriter struct {
	writer  http.ResponseWriter
	flusher http.Flusher
}

func (writer liveSSEWriter) WriteEvent(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer.writer, "data: %s\n\n", data); err != nil {
		return err
	}
	writer.flusher.Flush()
	return nil
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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.ensureLiveNotificationBridge(r.Context())
	dispatcher := newLiveStreamDispatcher(s, r.Context(), liveSSEWriter{
		writer:  w,
		flusher: flusher,
	}, nil)
	if err := dispatcher.writeInitialEvents(); err != nil {
		return
	}
	_ = dispatcher.run()
}

func writeHeartbeat(writer liveEventWriter) error {
	return writer.WriteEvent(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)})
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
	tickers, err := s.liveMarketExchange().QueryTickers(refreshCtx, instrumentIDs...)
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
	stream := s.liveMarketExchange().NewStream()
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

func (s *Server) writeLiveMarketTicks(ctx context.Context, writer liveEventWriter, lastSentByInstrument map[string]string) error {
	s.refreshLiveMarketTicksIfNeeded(ctx)
	for _, sample := range s.latestTickerSamples(s.activeMarketInstrumentIDs(), liveTickSampleFreshness) {
		if sample == nil || lastSentByInstrument[sample.InstrumentID] == sample.ObservedAt {
			continue
		}
		event := liveTickEventFromSample(sample)
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
