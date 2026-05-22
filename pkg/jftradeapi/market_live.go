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
	"github.com/gorilla/websocket"
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

func (s *Server) handleLiveWebSocket(w http.ResponseWriter, r *http.Request) {
	limit := s.effectiveLiveWebSocketLimit()
	if !s.tryAcquireLiveWebSocketSlot(limit) {
		s.writeError(w, http.StatusServiceUnavailable, "LIVE_WS_LIMIT_REACHED", fmt.Sprintf("live websocket connection limit reached (%d)", limit))
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.releaseLiveWebSocketSlot()
		return
	}
	defer func() {
		s.releaseLiveWebSocketSlot()
		_ = conn.Close()
	}()

	s.ensureLiveNotificationBridge(r.Context())
	clientClosed := liveWebSocketClientClosed(conn)
	dispatcher := newLiveWebSocketDispatcher(s, r.Context(), conn, clientClosed)
	if err := dispatcher.writeInitialEvents(); err != nil {
		return
	}
	_ = dispatcher.run()
}

func liveWebSocketClientClosed(conn *websocket.Conn) <-chan struct{} {
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()
	return closed
}

func writeHeartbeat(conn *websocket.Conn) error {
	return conn.WriteJSON(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)})
}

func (s *Server) effectiveLiveWebSocketLimit() int {
	limit := s.store.integration().Config.MaxWebSocketConnections
	if limit <= 0 {
		return defaultMaxWebSocketClients
	}
	return limit
}

func (s *Server) tryAcquireLiveWebSocketSlot(limit int) bool {
	return s.liveSockets.tryAcquire(limit)
}

func (s *Server) releaseLiveWebSocketSlot() {
	s.liveSockets.release()
}

func (s *Server) liveWebSocketStats() (count int, limit int, atLimit bool) {
	limit = s.effectiveLiveWebSocketLimit()
	count = s.liveSockets.count()
	return count, limit, count >= limit
}

func (s *Server) refreshLiveMarketTicksIfNeeded(ctx context.Context) {
	instrumentIDs := s.activeMarketInstrumentIDs()
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

	tickers, err := s.futuExchange().QueryTickers(refreshCtx, instrumentIDs...)
	if err != nil {
		retryDelay := liveRetryDelay(s.liveQuoteState.failureCount)
		s.liveQuoteState.failureCount++
		s.liveQuoteState.retryAfter = time.Now().UTC().Add(retryDelay)
		s.liveQuoteState.lastError = err.Error()
		log.Printf("JFTrade live quote refresh failed; retrying in %s: %v", retryDelay, err)
		return
	}
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
	stream := s.futuExchange().NewStream()
	stream.SetPublicOnly()
	for _, symbol := range symbols {
		stream.Subscribe(bbgotypes.MarketTradeChannel, symbol, bbgotypes.SubscribeOptions{})
	}
	stream.OnMarketTrade(func(trade bbgotypes.Trade) {
		s.recordTradeTickSample(trade)
	})
	s.liveStreamState.stream = stream
	s.liveStreamState.streamKey = streamKey
	s.liveStreamState.mu.Unlock()

	// Run the OpenD push subscription handshake off the websocket dispatch
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

func (s *Server) writeLiveMarketTicks(ctx context.Context, conn *websocket.Conn, lastSentByInstrument map[string]string) error {
	s.refreshLiveMarketTicksIfNeeded(ctx)
	for _, sample := range s.latestTickerSamples(s.activeMarketInstrumentIDs(), liveTickSampleFreshness) {
		if sample == nil || lastSentByInstrument[sample.InstrumentID] == sample.ObservedAt {
			continue
		}
		event := liveTickEventFromSample(sample)
		if event == nil {
			continue
		}
		if err := conn.WriteJSON(event); err != nil {
			return err
		}
		lastSentByInstrument[sample.InstrumentID] = sample.ObservedAt
	}
	return nil
}

func (s *Server) activeMarketInstrumentIDs() []string {
	return s.marketSubscriptions.activeInstrumentIDs()
}
