// Package status owns application-level status summaries consumed by the
// system routes. Keeping them outside servercore lets the composition root
// stay focused on wiring and transport.
package status

import (
	"sort"
	"strings"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func LiveStats(connected int, limit int, atLimit bool, activeInstruments []string) map[string]any {
	sorted := append([]string(nil), activeInstruments...)
	sort.Strings(sorted)
	return map[string]any{
		"connected":         connected,
		"limit":             limit,
		"atLimit":           atLimit,
		"activeInstruments": sorted,
	}
}

func MarketDataRuntimeSummary(service *mdsrv.Service) map[string]any {
	if service == nil {
		return map[string]any{"status": "unavailable"}
	}
	return marketDataRuntimeSummary(service.RuntimeState())
}

func marketDataRuntimeSummary(state mdsrv.RuntimeState) map[string]any {
	stateStatus := "idle"
	switch {
	case state.Closed:
		stateStatus = "closed"
	case state.Connected:
		stateStatus = "connected"
	case state.StreamLastError != "" || state.QuoteLastError != "":
		stateStatus = "degraded"
	case state.ActiveCount > 0:
		stateStatus = "connecting"
	}
	return map[string]any{
		"status":          stateStatus,
		"connected":       state.Connected,
		"closed":          state.Closed,
		"generation":      state.Generation,
		"activeCount":     state.ActiveCount,
		"lastRefreshAt":   OptionalTimeString(state.LastRefreshAt),
		"quoteRetryAt":    OptionalTimeString(state.QuoteRetryAt),
		"quoteFailures":   state.QuoteFailures,
		"quoteLastError":  StringPointerOrNil(state.QuoteLastError),
		"streamRetryAt":   OptionalTimeString(state.StreamRetryAt),
		"streamFailures":  state.StreamFailures,
		"streamLastError": StringPointerOrNil(state.StreamLastError),
	}
}

// StrategyRuntimeSummarySource is the small strategy-runtime surface needed
// by the system status route.
type StrategyRuntimeSummarySource interface {
	SummaryMap() map[string]any
}

func StrategyRuntimeSummary(runtime StrategyRuntimeSummarySource) map[string]any {
	if runtime == nil {
		return map[string]any{
			"status":                 "idle",
			"activeStrategies":       0,
			"supportsBacktestParity": true,
			"activeInstances":        []map[string]any{},
		}
	}
	return runtime.SummaryMap()
}

func OptionalTimeString(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	text := value.UTC().Format(time.RFC3339Nano)
	return &text
}

func StringPointerOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
