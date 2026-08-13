// Package status owns application-level status summaries consumed by the
// system routes. Keeping them outside servercore lets the composition root
// stay focused on wiring and transport.
package status

import (
	"sort"
	"strings"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
)

func LiveStats(connected int, limit int, atLimit bool, activeInstruments []string) *system.LiveStats {
	sorted := append([]string{}, activeInstruments...)
	sort.Strings(sorted)
	return &system.LiveStats{
		Connected: connected, Limit: limit, AtLimit: atLimit, ActiveInstruments: sorted,
	}
}

func MarketDataRuntimeSummary(service *mdsrv.Service) *system.MarketDataRuntime {
	if service == nil {
		return &system.MarketDataRuntime{Status: "unavailable"}
	}
	return marketDataRuntimeSummary(service.RuntimeState())
}

func marketDataRuntimeSummary(state mdsrv.RuntimeState) *system.MarketDataRuntime {
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
	return &system.MarketDataRuntime{
		Status: stateStatus, Connected: state.Connected, Closed: state.Closed,
		Generation: state.Generation, ActiveCount: state.ActiveCount,
		LastRefreshAt: OptionalTimeString(state.LastRefreshAt), QuoteRetryAt: OptionalTimeString(state.QuoteRetryAt),
		QuoteFailures: state.QuoteFailures, QuoteLastError: StringPointerOrNil(state.QuoteLastError),
		StreamRetryAt: OptionalTimeString(state.StreamRetryAt), StreamFailures: state.StreamFailures,
		StreamLastError: StringPointerOrNil(state.StreamLastError),
	}
}

// StrategyRuntimeSummarySource is the small strategy-runtime surface needed
// by the system status route.
type StrategyRuntimeSummarySource interface {
	RuntimeSummary() stratsrv.RuntimeSummary
}

func StrategyRuntimeSummary(runtime StrategyRuntimeSummarySource) *stratsrv.RuntimeSummary {
	if runtime == nil {
		return &stratsrv.RuntimeSummary{Status: "idle", SupportsBacktestParity: true, ActiveInstances: []stratsrv.RuntimeActiveInstanceSummary{}}
	}
	summary := runtime.RuntimeSummary()
	if summary.ActiveInstances == nil {
		summary.ActiveInstances = []stratsrv.RuntimeActiveInstanceSummary{}
	}
	return &summary
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
