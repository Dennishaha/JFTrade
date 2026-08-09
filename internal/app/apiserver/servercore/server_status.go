package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/status"
)

func liveStatsSummary(s *serverApplication) map[string]any {
	if s == nil {
		return status.LiveStats(0, 0, false, nil)
	}
	liveWebSocket := s.runtimes.LiveWebSocket()
	if liveWebSocket == nil {
		return status.LiveStats(0, 0, false, nil)
	}
	stats := liveWebSocket.Stats()
	activeInstruments := liveWebSocket.ActiveInstrumentIDs()
	return status.LiveStats(stats.Connected, stats.Limit, stats.AtLimit, activeInstruments)
}

func marketdataRuntimeSummary(s *serverApplication) map[string]any {
	return status.MarketDataRuntimeSummary(s.marketdataSvc)
}

func strategyRuntimeSummary(s *serverApplication) map[string]any {
	if s == nil {
		return status.StrategyRuntimeSummary(nil)
	}
	return status.StrategyRuntimeSummary(s.runtimes.StrategyRuntime())
}
