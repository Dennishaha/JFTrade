package servercore

import (
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"sort"
	"strings"
	"time"
)

func (s *serverApplication) liveStatsSummary() map[string]any {
	if s == nil {
		return map[string]any{"connected": 0, "limit": 0, "atLimit": false, "activeInstruments": []string{}}
	}
	liveWebSocket := s.runtimes.LiveWebSocket()
	if liveWebSocket == nil {
		return map[string]any{"connected": 0, "limit": 0, "atLimit": false, "activeInstruments": []string{}}
	}
	stats := liveWebSocket.Stats()
	activeInstruments := liveWebSocket.ActiveInstrumentIDs()
	sort.Strings(activeInstruments)
	return map[string]any{
		"connected":         stats.Connected,
		"limit":             stats.Limit,
		"atLimit":           stats.AtLimit,
		"activeInstruments": activeInstruments,
	}
}

func (s *serverApplication) marketdataRuntimeSummary() map[string]any {
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

func (s *serverApplication) strategyRuntimeSummary() map[string]any {
	strategyRuntime := s.runtimes.StrategyRuntime()
	if strategyRuntime == nil {
		return map[string]any{
			"status":                 "idle",
			"activeStrategies":       0,
			"supportsBacktestParity": true,
			"activeInstances":        []stratsrv.RuntimeActiveInstanceSummary{},
		}
	}
	return strategyRuntime.SummaryMap()
}

func optionalTimeString(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	text := value.UTC().Format(time.RFC3339Nano)
	return &text
}

func stringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
