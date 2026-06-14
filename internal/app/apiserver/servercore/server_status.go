package servercore

import (
	"sort"
	"strings"
	"time"
)

func (s *Server) liveStatsSummary() map[string]any {
	if s == nil || s.liveWebSocket == nil {
		return map[string]any{"connected": 0, "limit": 0, "atLimit": false, "activeInstruments": []string{}}
	}
	stats := s.liveWebSocket.Stats()
	activeInstruments := s.liveWebSocket.ActiveInstrumentIDs()
	sort.Strings(activeInstruments)
	return map[string]any{
		"connected":         stats.Connected,
		"limit":             stats.Limit,
		"atLimit":           stats.AtLimit,
		"activeInstruments": activeInstruments,
	}
}

func (s *Server) marketdataRuntimeSummary() map[string]any {
	if s == nil || s.marketdataSvc == nil {
		return map[string]any{"status": "unavailable"}
	}
	state := s.marketdataSvc.RuntimeState()
	status := "idle"
	switch {
	case state.Closed:
		status = "closed"
	case state.Connected:
		status = "connected"
	case state.StreamLastError != "" || state.QuoteLastError != "":
		status = "degraded"
	case state.ActiveCount > 0:
		status = "connecting"
	}
	return map[string]any{
		"status":          status,
		"connected":       state.Connected,
		"closed":          state.Closed,
		"generation":      state.Generation,
		"activeCount":     state.ActiveCount,
		"lastRefreshAt":   optionalTimeString(state.LastRefreshAt),
		"quoteRetryAt":    optionalTimeString(state.QuoteRetryAt),
		"quoteFailures":   state.QuoteFailures,
		"quoteLastError":  stringPointerOrNil(state.QuoteLastError),
		"streamRetryAt":   optionalTimeString(state.StreamRetryAt),
		"streamFailures":  state.StreamFailures,
		"streamLastError": stringPointerOrNil(state.StreamLastError),
	}
}

func (s *Server) strategyRuntimeSummary() map[string]any {
	if s.strategyRuntimeManager == nil {
		return map[string]any{
			"status":                 "idle",
			"activeStrategies":       0,
			"supportsBacktestParity": true,
			"activeInstances":        []strategyRuntimeActiveInstanceSummary{},
		}
	}
	return s.strategyRuntimeManager.runtimeSummary()
}

func optionalTimeString(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func stringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
