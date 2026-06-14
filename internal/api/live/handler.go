package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	livecore "github.com/jftrade/jftrade-main/internal/live"
)

const defaultConnectionLimit = 20

type ClientStats struct {
	Connected int
	Limit     int
	AtLimit   bool
}

type TickEvent struct {
	InstrumentID string
	ObservedAt   string
	Payload      map[string]any
}

type Backend interface {
	ConnectionLimit() int
	Heartbeat(time.Duration, ClientStats, []string) map[string]any
	MarketTicks(context.Context, []string, string) ([]TickEvent, error)
	NotificationsAfter(uint64) []livecore.Event
	EnsureNotificationBridge(context.Context)
	SecurityDetails(context.Context, string, string) (map[string]any, error)
	SubscribeDepth(context.Context, string, int32)
	Depth(context.Context, string, string, int32) (map[string]any, error)
	SubscribeDepthUpdates(func(string)) func()
}

type Options struct {
	HeartbeatInterval       time.Duration
	DataInterval            time.Duration
	ConsoleRefreshInterval  time.Duration
	SecurityDetailsInterval time.Duration
	DepthRefreshInterval    time.Duration
}

type clientMessage struct {
	Type          string                 `json:"type"`
	Subscriptions livecore.Subscriptions `json:"subscriptions"`
}

type Handler struct {
	backend  Backend
	upgrader websocket.Upgrader
	options  Options
	registry livecore.ClientRegistry

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	active    sync.WaitGroup

	mu          sync.Mutex
	pending     int
	connections map[*websocket.Conn]struct{}
}

func NewHandler(backend Backend, options Options) *Handler {
	ctx, cancel := context.WithCancel(context.Background())
	options = normalizeOptions(options)
	return &Handler{
		backend: backend,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool {
			return true
		}},
		options:     options,
		ctx:         ctx,
		cancel:      cancel,
		connections: map[*websocket.Conn]struct{}{},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.backend == nil {
		http.NotFound(w, r)
		return
	}
	limit := h.connectionLimit()
	if !h.tryAcquire(limit) {
		writeLimitError(w, limit)
		return
	}
	defer h.active.Done()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.release(nil)
		return
	}
	defer func() {
		h.release(conn)
		_ = conn.Close()
	}()

	h.backend.EnsureNotificationBridge(r.Context())
	client := h.registry.Register()
	defer h.registry.Unregister(client.ID())

	clientClosed := readClientMessages(conn, client)
	depthUpdated, unsubscribeDepth := h.subscribeDepthUpdates(client)
	defer unsubscribeDepth()

	requestCtx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		select {
		case <-h.ctx.Done():
			cancel()
		case <-requestCtx.Done():
		}
	}()

	dispatcher := newDispatcher(h, requestCtx, conn, client, clientClosed, depthUpdated)
	if err := dispatcher.writeInitialEvents(); err != nil {
		return
	}
	_ = dispatcher.run()
}

func (h *Handler) Stats() ClientStats {
	if h == nil {
		return ClientStats{}
	}
	limit := h.connectionLimit()
	h.mu.Lock()
	count := h.pending + len(h.connections)
	h.mu.Unlock()
	return ClientStats{Connected: count, Limit: limit, AtLimit: count >= limit}
}

func (h *Handler) ActiveInstrumentIDs() []string {
	if h == nil {
		return nil
	}
	return h.registry.ActiveInstrumentIDs()
}

func (h *Handler) Close() error {
	if h == nil {
		return nil
	}
	h.closeOnce.Do(func() {
		h.cancel()
		h.mu.Lock()
		connections := make([]*websocket.Conn, 0, len(h.connections))
		for conn := range h.connections {
			connections = append(connections, conn)
		}
		h.mu.Unlock()
		for _, conn := range connections {
			_ = conn.Close()
		}
		h.active.Wait()
	})
	return nil
}

func (h *Handler) connectionLimit() int {
	limit := h.backend.ConnectionLimit()
	if limit <= 0 {
		return defaultConnectionLimit
	}
	return limit
}

func (h *Handler) tryAcquire(limit int) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	select {
	case <-h.ctx.Done():
		return false
	default:
	}
	if h.pending+len(h.connections) >= limit {
		return false
	}
	h.active.Add(1)
	h.pending++
	return true
}

func (h *Handler) release(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn != nil {
		delete(h.connections, conn)
		return
	}
	if h.pending > 0 {
		h.pending--
	}
}

func (h *Handler) promoteConnection(conn *websocket.Conn) {
	h.mu.Lock()
	if h.pending > 0 {
		h.pending--
	}
	h.connections[conn] = struct{}{}
	h.mu.Unlock()
}

func (h *Handler) subscribeDepthUpdates(client *livecore.Client) (<-chan struct{}, func()) {
	updateCh := make(chan struct{}, 1)
	unsubscribe := h.backend.SubscribeDepthUpdates(func(updatedSymbol string) {
		instrumentID := strings.ToUpper(strings.TrimSpace(updatedSymbol))
		for _, subscription := range client.Snapshot().Depth {
			if subscription.InstrumentID != instrumentID {
				continue
			}
			select {
			case updateCh <- struct{}{}:
			default:
			}
			return
		}
	})
	if unsubscribe == nil {
		unsubscribe = func() {}
	}
	return updateCh, unsubscribe
}

func readClientMessages(conn *websocket.Conn, client *livecore.Client) <-chan struct{} {
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var message clientMessage
			if err := json.Unmarshal(payload, &message); err != nil || message.Type != "subscribe" {
				continue
			}
			client.SetSubscriptions(message.Subscriptions)
		}
	}()
	return closed
}

func writeLimitError(w http.ResponseWriter, limit int) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":    "LIVE_WS_LIMIT_REACHED",
			"message": fmt.Sprintf("live websocket connection limit reached (%d)", limit),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func normalizeOptions(options Options) Options {
	if options.HeartbeatInterval <= 0 {
		options.HeartbeatInterval = 15 * time.Second
	}
	if options.DataInterval <= 0 {
		options.DataInterval = 250 * time.Millisecond
	}
	if options.ConsoleRefreshInterval <= 0 {
		options.ConsoleRefreshInterval = 15 * time.Second
	}
	if options.SecurityDetailsInterval <= 0 {
		options.SecurityDetailsInterval = 3 * time.Second
	}
	if options.DepthRefreshInterval <= 0 {
		options.DepthRefreshInterval = 15 * time.Second
	}
	return options
}
