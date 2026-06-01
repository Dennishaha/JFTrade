package jftradeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) serveMarketRoutes(w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/v1/ws/live":
		s.handleLiveWebSocket(w, r)
	case r.URL.Path == "/api/v1/stream/live" || r.URL.Path == "/api/v1/streams/console":
		s.handleEventStream(w, r)
	case r.URL.Path == "/api/v1/market-data/instruments" && r.Method == http.MethodGet:
		s.writeOK(w, map[string]any{"query": r.URL.Query().Get("query"), "totalReturned": 0, "entries": []any{}})
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodGet:
		s.writeOK(w, s.marketSubscriptionsResponse())
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodPost:
		s.handleAcquireMarketSubscription(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions" && r.Method == http.MethodDelete:
		s.handleClearMarketSubscriptions(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions/release" && r.Method == http.MethodPost:
		s.handleReleaseMarketSubscription(w, r)
	case r.URL.Path == "/api/v1/market-data/subscriptions/heartbeat" && r.Method == http.MethodPost:
		s.handleHeartbeatMarketSubscription(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/securities/") && r.Method == http.MethodGet:
		s.handleMarketSecurityDetails(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/snapshots/") && r.Method == http.MethodGet:
		s.handleMarketSnapshot(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/candles/") && r.Method == http.MethodGet:
		s.handleMarketCandles(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/market-data/depth/") && r.Method == http.MethodGet:
		s.handleMarketDepth(w, r)
	default:
		return false
	}
	return true
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	write := func() bool {
		_, err := fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{"type": "heartbeat", "at": time.Now().UTC().Format(time.RFC3339Nano)}))
		if flusher != nil {
			flusher.Flush()
		}
		return err == nil
	}
	if !write() {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !write() {
				return
			}
		}
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
