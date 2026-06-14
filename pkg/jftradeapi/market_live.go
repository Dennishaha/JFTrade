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
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

// handleLiveWebSocket godoc
// @Summary 连接实时 WebSocket
// @Description 建立行情与运行态实时推送连接。
// @Tags streaming
// @Produce json
// @Success 101 {string} string "Switching Protocols"
// @Failure 503 {object} envelope
// @Router /api/v1/ws/live [get]
func (s *Server) handleLiveWebSocket(c *gin.Context) {
	w := c.Writer
	r := c.Request
	limit := s.effectiveLiveStreamLimit()
	if !s.tryAcquireLiveStreamSlot(limit) {
		s.writeError(c, http.StatusServiceUnavailable, "LIVE_WS_LIMIT_REACHED", fmt.Sprintf("live websocket connection limit reached (%d)", limit))
		return
	}
	defer s.releaseLiveStreamSlot()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() {
		_ = conn.Close()
	}()

	s.ensureLiveNotificationBridge(r.Context())
	client := s.liveSocketClients.register()
	defer s.liveSocketClients.unregister(client.id)

	clientClosed := liveWebSocketClientClosed(conn, client)
	depthUpdated, unsubscribeDepth := s.subscribeLiveWebSocketDepthUpdates(client)
	defer unsubscribeDepth()

	dispatcher := newLiveWebSocketDispatcher(s, r.Context(), conn, client, clientClosed, depthUpdated)
	if err := dispatcher.writeInitialEvents(); err != nil {
		return
	}
	_ = dispatcher.run()
}

func liveWebSocketClientClosed(
	conn *websocket.Conn,
	client *liveWebSocketClient,
) <-chan struct{} {
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var message liveWebSocketClientMessage
			if err := json.Unmarshal(payload, &message); err != nil {
				continue
			}
			if message.Type != "subscribe" {
				continue
			}
			client.setSubscriptions(message.Subscriptions)
		}
	}()
	return closed
}

func (s *Server) subscribeLiveWebSocketDepthUpdates(
	client *liveWebSocketClient,
) (<-chan struct{}, func()) {
	updateCh := make(chan struct{}, 1)
	exchange := s.futuExchange()
	if exchange == nil {
		return updateCh, func() {}
	}

	unsubscribe := exchange.OnOrderBookUpdate(func(updatedSymbol string) {
		instrumentID := strings.ToUpper(strings.TrimSpace(updatedSymbol))
		for _, depthSubscription := range client.snapshot().Depth {
			if depthSubscription.InstrumentID != instrumentID {
				continue
			}
			select {
			case updateCh <- struct{}{}:
			default:
			}
			return
		}
	})
	return updateCh, unsubscribe
}

func writeHeartbeat(writer liveEventWriter, payload map[string]any) error {
	return writer.WriteEvent(payload)
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
